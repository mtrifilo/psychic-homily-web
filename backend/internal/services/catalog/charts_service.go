package catalog

import (
	"fmt"
	"time"

	"gorm.io/gorm"

	"psychic-homily-backend/db"
	catalogm "psychic-homily-backend/internal/models/catalog"
	engagementm "psychic-homily-backend/internal/models/engagement"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/services/geo"
)

// ChartsService computes top charts / trending content from engagement signals.
// No new tables — all data is derived from existing bookmark, show, artist, venue,
// and release tables.
type ChartsService struct {
	db *gorm.DB
	// cache holds the module pages (client-controlled key space, capped);
	// mastheadCache holds only the summary + ticker so module-key overflow
	// can never starve the hottest, shortest-TTL entries of a slot. nil
	// caches disable caching entirely — the integration test suite
	// constructs ChartsService without them so per-test DB state stays
	// authoritative. See charts_cache.go.
	cache         *chartsCache
	mastheadCache *chartsCache
}

// NewChartsService creates a new charts service.
func NewChartsService(database *gorm.DB) *ChartsService {
	if database == nil {
		database = db.GetDB()
	}
	return &ChartsService{
		db:            database,
		cache:         newChartsCache(),
		mastheadCache: newChartsCache(),
	}
}

// GetTrendingShows returns upcoming shows ranked by save count.
// Only includes future shows with approved status. Shows without engagement data are
// included and ranked by soonest date, so the chart is never empty when shows exist.
//
// DEPRECATED in favor of GetMostAnticipatedShows, which replaces it for the
// redesigned charts page; this stays only until the frontend hook migrates
// off /charts/trending-shows. Known divergences fixed in the replacement and
// deliberately NOT back-ported here (don't "fix" a route slated for
// deletion): no is_cancelled filter, multi-venue shows duplicate one row per
// venue, and the bound is the current instant rather than start-of-today.
func (s *ChartsService) GetTrendingShows(limit int) ([]contracts.TrendingShow, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	now := time.Now().UTC()

	type trendingRow struct {
		ShowID    uint      `gorm:"column:show_id"`
		Title     string    `gorm:"column:title"`
		Slug      string    `gorm:"column:slug"`
		Date      time.Time `gorm:"column:event_date"`
		VenueName string    `gorm:"column:venue_name"`
		VenueSlug string    `gorm:"column:venue_slug"`
		City      string    `gorm:"column:city"`
		SaveCount int       `gorm:"column:save_count"`
	}

	var rows []trendingRow
	err := s.db.Raw(`
		SELECT
			s.id AS show_id,
			s.title,
			COALESCE(s.slug, '') AS slug,
			s.event_date,
			COALESCE(v.name, '') AS venue_name,
			COALESCE(v.slug, '') AS venue_slug,
			COALESCE(v.city, '') AS city,
			COALESCE(COUNT(ub.id), 0) AS save_count
		FROM shows s
		LEFT JOIN show_venues sv ON sv.show_id = s.id
		LEFT JOIN venues v ON v.id = sv.venue_id
		LEFT JOIN user_bookmarks ub ON ub.entity_id = s.id
			AND ub.entity_type = ?
			AND ub.action = ?
		WHERE s.status = ?
			AND s.event_date >= ?
		GROUP BY s.id, s.title, s.slug, s.event_date, v.name, v.slug, v.city
		ORDER BY save_count DESC, s.event_date ASC
		LIMIT ?
	`, engagementm.BookmarkEntityShow, engagementm.BookmarkActionSave,
		catalogm.ShowStatusApproved, now, limit).Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("failed to get trending shows: %w", err)
	}

	results := make([]contracts.TrendingShow, len(rows))
	showIDs := make([]uint, len(rows))
	for i, r := range rows {
		results[i] = contracts.TrendingShow{
			ShowID:      r.ShowID,
			Title:       r.Title,
			Slug:        r.Slug,
			Date:        r.Date,
			VenueName:   r.VenueName,
			VenueSlug:   r.VenueSlug,
			City:        r.City,
			ArtistNames: []string{},
			SaveCount:   r.SaveCount,
		}
		showIDs[i] = r.ShowID
	}

	// Fetch artist names for all shows in one query
	artistMap, err := s.showArtistNames(showIDs)
	if err != nil {
		return nil, err
	}
	for i := range results {
		if names, ok := artistMap[results[i].ShowID]; ok {
			results[i].ArtistNames = names
		}
	}

	return results, nil
}

// chartCoreCount counts a module's full qualifying set by wrapping its core
// query (SELECT … GROUP BY/HAVING, no ORDER/LIMIT) as a subquery. Used only
// when a page comes back empty at a non-zero offset: the page then carries no
// COUNT(*) OVER() value, but a beyond-the-end page must still report the real
// total (not zero). This second statement is a different snapshot than the
// empty page — benign, since the page it disagrees with has no rows.
func (s *ChartsService) chartCoreCount(coreSQL string, args []any, what string) (int, error) {
	var total int
	if err := s.db.Raw(`SELECT COUNT(*) FROM (`+coreSQL+`) core_rows`, args...).Scan(&total).Error; err != nil {
		return 0, fmt.Errorf("failed to count %s: %w", what, err)
	}
	return total, nil
}

// resolveChartPageTotal is the shared total-resolution rule for module pages:
// the page's own COUNT(*) OVER() when it has rows (atomic with the page), the
// core re-count for a beyond-the-end offset, and zero only when the set is
// genuinely empty (empty first page). rowCount > 0 with a zero total means
// the module's SELECT is missing the COUNT(*) OVER() AS total column (or its
// scan field) — gorm zero-fills unmatched columns silently, so fail loudly
// instead of shipping Total=0 next to a populated page.
// countKey caches the re-count offset-independently (chartCountKey), so a
// client walking junk beyond-the-end offsets pays the count aggregation once
// per TTL rather than per request.
func (s *ChartsService) resolveChartPageTotal(window contracts.ChartWindow, pageTotal, rowCount, offset int, coreSQL string, args []any, countKey, what string) (int, error) {
	if rowCount > 0 {
		if pageTotal <= 0 {
			return 0, fmt.Errorf("%s page query is missing the COUNT(*) OVER() AS total column", what)
		}
		return pageTotal, nil
	}
	if offset > 0 {
		return chartsCached(s.cache, countKey, chartWindowTTL(s.cache, window, chartsModuleTTL), func() (int, error) {
			return s.chartCoreCount(coreSQL, args, what)
		})
	}
	return 0, nil
}

// appendPageArgs copies the core args before appending the page bounds — the
// copy is load-bearing: coreArgs is reused verbatim by the beyond-the-end
// re-count, so appending in place would alias its backing array.
func appendPageArgs(coreArgs []any, limit, offset int) []any {
	return append(append([]any{}, coreArgs...), limit, offset)
}

// namesByOwnerID runs a two-column enrichment query — it must SELECT the
// owning entity's id AS owner_id plus a name, bind exactly one `IN ?` on ids,
// and own its ORDER BY (which fixes the name order in the result slices) —
// and folds the rows into owner_id → names. `what` labels the error. The
// query must be a compile-time constant, never interpolated. Shared by every
// chart-module name enrichment so the empty-input and error conventions
// can't drift between modules. Returns a nil map on empty input (lookups on
// nil maps are safe no-ops).
func (s *ChartsService) namesByOwnerID(query string, ids []uint, what string) (map[uint][]string, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	type nameRow struct {
		OwnerID uint   `gorm:"column:owner_id"`
		Name    string `gorm:"column:name"`
	}
	var rows []nameRow
	if err := s.db.Raw(query, ids).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("failed to get %s: %w", what, err)
	}
	names := make(map[uint][]string)
	for _, r := range rows {
		// A zero owner id means the query broke the AS owner_id contract —
		// gorm's Scan leaves unmatched columns at zero WITHOUT erroring, so
		// without this guard every name would silently fold under key 0 and
		// the module would ship empty enrichment. Fail loudly instead.
		if r.OwnerID == 0 {
			return nil, fmt.Errorf("%s enrichment query is missing the AS owner_id alias", what)
		}
		names[r.OwnerID] = append(names[r.OwnerID], r.Name)
	}
	return names, nil
}

// referencesByOwnerID is the linkable-identity twin of namesByOwnerID. The
// query contract is owner_id + id + name + slug, with exactly one `IN ?`.
// It keeps release chart enrichment batched while letting every named artist
// and label navigate to its entity page.
func (s *ChartsService) referencesByOwnerID(query string, ids []uint, what string) (map[uint][]contracts.ChartEntityReference, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	type referenceRow struct {
		OwnerID uint   `gorm:"column:owner_id"`
		ID      uint   `gorm:"column:id"`
		Name    string `gorm:"column:name"`
		Slug    string `gorm:"column:slug"`
	}
	var rows []referenceRow
	if err := s.db.Raw(query, ids).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("failed to get %s: %w", what, err)
	}
	references := make(map[uint][]contracts.ChartEntityReference)
	for _, row := range rows {
		if row.OwnerID == 0 {
			return nil, fmt.Errorf("%s enrichment query is missing the AS owner_id alias", what)
		}
		references[row.OwnerID] = append(references[row.OwnerID], contracts.ChartEntityReference{
			ID:   row.ID,
			Name: row.Name,
			Slug: row.Slug,
		})
	}
	return references, nil
}

// showArtistNames returns bill-ordered artist names for each show, in one
// query. Shared by the show-row chart modules (trending / most-anticipated).
func (s *ChartsService) showArtistNames(showIDs []uint) (map[uint][]string, error) {
	return s.namesByOwnerID(`
		SELECT sa.show_id AS owner_id, a.name
		FROM show_artists sa
		JOIN artists a ON a.id = sa.artist_id
		WHERE sa.show_id IN ?
		ORDER BY sa.show_id, sa.position
	`, showIDs, "show artists")
}

// mostAnticipatedSaveFloor is the minimum save count a show needs to appear
// in the ranked most-anticipated chart; below it, counts are too sparse to
// be a signal (and rendering them reads as a dead site).
// mostAnticipatedMinQualifying is the minimum number of qualifying shows for
// ranked mode to be worth rendering — below it the module falls back to
// soonest-upcoming (date-ordered, counts omitted), so the module has rows
// whenever any upcoming show exists.
const (
	mostAnticipatedSaveFloor     = 3
	mostAnticipatedMinQualifying = 5
)

// The ranked and fallback most-anticipated queries share these fragments so
// their show universes can't drift apart — a mode flip must never change
// WHICH shows are eligible, only how they're ordered and what's counted.
//
// The LATERAL venue pick returns at most one deterministic venue per show
// (unlike a bare show_venues LEFT JOIN, which yields one row per venue for a
// multi-venue show — duplicating the show in the payload and, worse here,
// double-counting it toward the min-qualifying mode check).
const (
	mostAnticipatedColumnsSQL = `
			s.id AS show_id,
			s.title,
			COALESCE(s.slug, '') AS slug,
			s.event_date,
			COALESCE(v.name, '') AS venue_name,
			COALESCE(v.slug, '') AS venue_slug,
			COALESCE(v.city, '') AS city`

	// Upcoming + approved + not cancelled: the same non-cancelled rule the
	// past-window charts enforce via appendChartShowWindow — a cancelled show
	// must never rank as "anticipated". Binds (status, start-of-today): event
	// dates are midnight timestamps, so bounding against the current instant
	// would drop tonight's shows the moment the day starts — the rows users
	// are most likely to act on. Same start-of-today idea as
	// GetUpcomingShows, but fixed to UTC (a public chart has no requester
	// timezone to resolve against).
	mostAnticipatedEligibilitySQL = `WHERE s.status = ?
			AND s.is_cancelled = FALSE
			AND s.event_date >= ?`
)

// primaryVenueLateralSQL renders the repo's primary-venue pick as a LATERAL
// subquery: at most one deterministic venue per show, lowest venue_id first —
// the same rule as the pv lateral in show.go, so a multi-venue show names the
// same venue here as on its show page. `cols` selects from venues aliased iv;
// `showIDExpr` anchors the pick (s.id, ub.entity_id, ...). Both must be
// compile-time literals, never runtime input. Consumers: the most-anticipated
// queries and the personal-stats top venue. (GetTrendingShows predates it and
// deliberately keeps its one-row-per-venue join until deletion; the
// most-active last-show lookup breaks venue ties by name for display, a
// different job.) New venue-ATTRIBUTING chart queries must build through
// this so the pick rule can't drift between surfaces.
func primaryVenueLateralSQL(cols, showIDExpr string) string {
	return `(
			SELECT ` + cols + `
			FROM show_venues sv
			JOIN venues iv ON iv.id = sv.venue_id
			WHERE sv.show_id = ` + showIDExpr + `
			ORDER BY sv.venue_id ASC
			LIMIT 1
		)`
}

// mostAnticipatedHorizon returns the shared upcoming-horizon bounds for the
// most-anticipated module and its rank lookup. pastCalendar is true when the
// calendar window has already ended (no upcoming shows remain in-scope) —
// both surfaces must agree so a badge never claims a rank the module page
// cannot show.
func mostAnticipatedHorizon(window contracts.ChartWindow) (startOfToday time.Time, windowEnd *time.Time, pastCalendar bool) {
	startOfToday = time.Now().UTC().Truncate(24 * time.Hour)
	calendarStart, calendarEnd, isCalendar := window.CalendarBounds()
	if isCalendar && !calendarEnd.After(startOfToday) {
		return startOfToday, nil, true
	}
	if isCalendar && calendarStart.After(startOfToday) {
		startOfToday = calendarStart
	}
	switch window {
	case contracts.ChartWindowMonth:
		end := startOfToday.AddDate(0, 0, 30)
		windowEnd = &end
	case contracts.ChartWindowQuarter:
		end := startOfToday.AddDate(0, 0, 90)
		windowEnd = &end
	default:
		if isCalendar {
			windowEnd = &calendarEnd
		}
	}
	return startOfToday, windowEnd, false
}

// GetMostAnticipatedShows returns the mode-discriminated most-anticipated
// module: upcoming approved shows with saves >= the floor, ranked by save
// count (ties by soonest date, then id). Rolling month/quarter values bound
// the upcoming horizon to 30/90 days; calendar values use their UTC end.
// When fewer than
// mostAnticipatedMinQualifying shows clear the floor IN TOTAL, it returns
// soonest-upcoming fallback mode instead — ALL upcoming approved shows
// date-ordered with SaveCount and Rank nil on every row (fail-closed:
// sub-floor counts never leak into a rendered payload).
//
// Pagination applies to ranked mode only: the mode is decided by the TOTAL
// qualifying count (the page's COUNT(*) OVER(), so a small page size or deep
// offset can't force fallback), ranks are offset-stable (offset+i+1), and
// Total counts all qualifying shows. Fallback is the module's floor, not a
// ranked list — it ignores offset and reports the upcoming-show universe as
// its Total.
func (s *ChartsService) GetMostAnticipatedShows(window contracts.ChartWindow, scene string, limit, offset int) (*contracts.MostAnticipatedShows, error) {
	if !chartSceneExists(scene) {
		// Unknown scene: the documented empty envelope — the shape the scoped
		// fallback would produce — without a cache slot or DB round trip.
		return &contracts.MostAnticipatedShows{
			Mode:  contracts.MostAnticipatedModeSoonestUpcoming,
			Shows: []contracts.MostAnticipatedShow{},
		}, nil
	}
	key := fmt.Sprintf("most-anticipated|%s|%s|%d|%d", window.OrDefault(), scene, limit, offset)
	return chartsCached(s.cache, key, chartWindowTTL(s.cache, window, chartsModuleTTL), func() (*contracts.MostAnticipatedShows, error) {
		return s.getMostAnticipatedShowsUncached(window, scene, limit, offset)
	})
}

func (s *ChartsService) getMostAnticipatedShowsUncached(window contracts.ChartWindow, scene string, limit, offset int) (*contracts.MostAnticipatedShows, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	startOfToday, windowEnd, pastCalendar := mostAnticipatedHorizon(window.OrDefault())
	if pastCalendar {
		return &contracts.MostAnticipatedShows{
			Mode:  contracts.MostAnticipatedModeSoonestUpcoming,
			Shows: []contracts.MostAnticipatedShow{},
		}, nil
	}

	type showRow struct {
		ShowID    uint      `gorm:"column:show_id"`
		Title     string    `gorm:"column:title"`
		Slug      string    `gorm:"column:slug"`
		Date      time.Time `gorm:"column:event_date"`
		VenueName string    `gorm:"column:venue_name"`
		VenueSlug string    `gorm:"column:venue_slug"`
		City      string    `gorm:"column:city"`
		SaveCount int       `gorm:"column:save_count"`
		Total     int       `gorm:"column:total"`
	}

	// Local so there's no package-level mutable SQL (a const can't call the
	// helper); both mode queries below share it, so their show universes and
	// venue picks can't drift.
	mostAnticipatedFromSQL := `FROM shows s
			LEFT JOIN LATERAL ` + primaryVenueLateralSQL("iv.name, iv.slug, iv.city", "s.id") + ` v ON TRUE`

	// Scene scoping goes into the SHARED eligibility fragment so both modes
	// scope identically — a scoped fallback lists the scene's upcoming shows,
	// never the global calendar. Venue-metro attribution (any linked venue),
	// per the scene-scoping block above. NOTE the DISPLAYED venue stays the
	// unscoped lowest-id primary pick: a rare multi-metro show can appear in
	// a scene labeled with its out-of-scene primary venue — scope and display
	// attribution are deliberately different jobs (same rule as the show
	// page).
	eligibilitySQL, eligibilityArgs := appendShowSceneScope(
		mostAnticipatedEligibilitySQL, []any{catalogm.ShowStatusApproved, startOfToday}, scene)
	if windowEnd != nil {
		eligibilitySQL += `
			AND s.event_date < ?`
		eligibilityArgs = append(eligibilityArgs, *windowEnd)
	}

	// COUNT(*) OVER() counts qualifying groups post-HAVING — the full ranked
	// set, atomic with the page, so mode and Total can't disagree with the
	// rows they ship alongside.
	rankedCoreSQL := `
		SELECT` + mostAnticipatedColumnsSQL + `,
			COUNT(ub.id) AS save_count,
			COUNT(*) OVER() AS total
		` + mostAnticipatedFromSQL + `
		LEFT JOIN user_bookmarks ub ON ub.entity_id = s.id
			AND ub.entity_type = ?
			AND ub.action = ?
		` + eligibilitySQL + `
		GROUP BY s.id, v.name, v.slug, v.city
		HAVING COUNT(ub.id) >= ?`
	rankedCoreArgs := append(append([]any{engagementm.BookmarkEntityShow, engagementm.BookmarkActionSave},
		eligibilityArgs...), mostAnticipatedSaveFloor)

	var ranked []showRow
	err := s.db.Raw(rankedCoreSQL+`
		ORDER BY save_count DESC, s.event_date ASC, s.id ASC
		LIMIT ? OFFSET ?
	`, appendPageArgs(rankedCoreArgs, limit, offset)...).Scan(&ranked).Error
	if err != nil {
		return nil, fmt.Errorf("failed to get most-anticipated shows: %w", err)
	}

	pageTotal := 0
	if len(ranked) > 0 {
		pageTotal = ranked[0].Total
	}
	total, err := s.resolveChartPageTotal(window, pageTotal, len(ranked), offset, rankedCoreSQL, rankedCoreArgs,
		chartCountKey("most-anticipated", string(window.OrDefault()), scene), "most-anticipated shows")
	if err != nil {
		return nil, err
	}

	mode := contracts.MostAnticipatedModeRanked
	rows := ranked
	if total < mostAnticipatedMinQualifying {
		mode = contracts.MostAnticipatedModeSoonestUpcoming
		rows = nil
		err := s.db.Raw(`
			SELECT`+mostAnticipatedColumnsSQL+`,
				COUNT(*) OVER() AS total
			`+mostAnticipatedFromSQL+`
			`+eligibilitySQL+`
			ORDER BY s.event_date ASC, s.id ASC
			LIMIT ?
		`, append(append([]any{}, eligibilityArgs...), limit)...).Scan(&rows).Error
		if err != nil {
			return nil, fmt.Errorf("failed to get soonest-upcoming fallback shows: %w", err)
		}
		// Fallback Total = its own universe (all upcoming approved shows),
		// from the same statement. An empty fallback means zero upcoming
		// shows, so the zero total is genuine.
		total = 0
		if len(rows) > 0 {
			total = rows[0].Total
		}
	}

	result := &contracts.MostAnticipatedShows{
		Mode:  mode,
		Total: total,
		Shows: make([]contracts.MostAnticipatedShow, len(rows)),
	}
	showIDs := make([]uint, len(rows))
	for i, r := range rows {
		result.Shows[i] = contracts.MostAnticipatedShow{
			ShowID:      r.ShowID,
			Title:       r.Title,
			Slug:        r.Slug,
			Date:        r.Date,
			VenueName:   r.VenueName,
			VenueSlug:   r.VenueSlug,
			City:        r.City,
			ArtistNames: []string{},
		}
		if mode == contracts.MostAnticipatedModeRanked {
			count := r.SaveCount
			result.Shows[i].SaveCount = &count
			rank := offset + i + 1
			result.Shows[i].Rank = &rank
		}
		showIDs[i] = r.ShowID
	}

	artistMap, err := s.showArtistNames(showIDs)
	if err != nil {
		return nil, err
	}
	for i := range result.Shows {
		if names, ok := artistMap[result.Shows[i].ShowID]; ok {
			result.Shows[i].ArtistNames = names
		}
	}

	return result, nil
}

// chartBounds carries the shared query bounds for rolling, all-time, and UTC
// calendar windows. Calendar end is exclusive; rolling/all-time retain the
// existing inclusive now ceiling.
type chartBounds struct {
	start        *time.Time
	end          time.Time
	endExclusive bool
	calendarEnd  *time.Time
}

func chartWindowBounds(window contracts.ChartWindow, now time.Time) chartBounds {
	now = now.UTC()
	if start, end, ok := window.OrDefault().CalendarBounds(); ok {
		queryEnd := end
		if now.Before(queryEnd) {
			queryEnd = now
		}
		return chartBounds{start: &start, end: queryEnd, endExclusive: true, calendarEnd: &end}
	}

	days := 0
	switch window.OrDefault() {
	case contracts.ChartWindowMonth:
		days = 30
	case contracts.ChartWindowAllTime:
		return chartBounds{end: now}
	case contracts.ChartWindowQuarter:
		days = 90
	default:
		// Unreachable while OrDefault enumerates every window; keeps quarter
		// semantics if a new ChartWindow value lands there without a case here.
		days = 90
	}
	// Truncate to midnight UTC: event dates are midnight timestamps, so a
	// time-of-day lower bound would exclude the show exactly N days ago.
	t := now.AddDate(0, 0, -days).Truncate(24 * time.Hour)
	return chartBounds{start: &t, end: now}
}

// headlineSlotPredicate is the SQL condition for "this show_artists row
// (aliased sa) is a headline slot". There is no schema-level definition of
// "headliner" — this predicate IS it, and it must stay in sync with the
// discovery pipeline's headliner detection (services/pipeline/discovery.go).
// Sensitivity differs by consumer: in GetMostActiveArtists a spurious
// position-0 row only skews headline_pct; in GetOpenersToWatch it EXCLUDES
// the artist from the chart entirely.
const headlineSlotPredicate = `sa.set_type = 'headliner' OR sa.position = 0`

// appendChartShowWindow appends the shared chart-eligibility fragment for
// shows aliased `s` — non-cancelled, played on/before now (event dates are
// midnight timestamps, so a show later today already counts as played), and
// inside the optional window lower bound — plus the matching args. Every
// windowed SHOW chart query in this file builds its WHERE clause through this
// helper so eligibility rules can't drift between modules; radio-backed
// modules (GetOnTheRadioArtists) window on radio_episodes.air_date instead
// and pair it with the shared aired gate (airedEpisodeVisibleSQL).
func appendChartShowWindow(query string, args []any, bounds chartBounds) (string, []any) {
	query += `
			AND s.is_cancelled = FALSE`
	// The bounds themselves are the shared appendWindowBounds logic over a
	// different column — one definition of "on/before now, inside the
	// optional window" for every chart surface.
	return appendWindowBounds(query, args, "s.event_date", bounds)
}

// ── Scene scoping (charts scene dimension) ──────────────────────────────────
//
// A chart's `scene` is a US Census CBSA metro code — the same key as
// artists.metro / venues.metro (entity-metro migration). "" = global, no
// scoping. The three appenders below define "in the scene" for chart
// surfaces; every scoped module builds through exactly one so per-module
// semantics can't drift — with exactly TWO documented inline exceptions
// whose predicates must stay textually in sync with the appenders: the
// freshly-added ticker's branch fragments (UNION assembly interleaves
// per-branch args) and the summary's radio-play EXISTS (rp.artist_id has no
// joined artists alias to hand an appender):
//
//   - ARTIST-ranked modules scope on the artist's HOME metro (scenes = bands
//     BASED in a metro): most-active, openers-to-watch, on-the-radio — and
//     new-releases via the release's credited artists.
//   - SHOW/VENUE modules scope on the venue's metro (where it happened):
//     busiest-venues (direct equality), most-anticipated + the summary's
//     show count (any-venue EXISTS), the summary's active-scenes count, the
//     ticker's venue branch.
//
// Two deliberate boundaries, both disclosed on the ticket:
//   - Strict metro equality — TIGHTER than the scene page roster, which also
//     folds in NULL-metro artists whose home city/state is a metro member
//     place. Scene-scoped chart counts can therefore undercount vs the scene
//     page; aligning them would join the geo member-place set into every
//     chart query, which isn't worth it until evidence says otherwise.
//   - Fallback (city|state) scenes — non-US or no-CBSA — are not addressable:
//     GetChartScenes lists CBSA metros only, and an unknown-but-well-formed
//     scene value yields empty results with a valid envelope (never an
//     error), so a stale bookmarked URL degrades gracefully.

// chartSceneExists reports whether scene is the global scope ("") or names a
// REAL CBSA metro in the static geo dataset. The pattern tag bounds the
// param's SHAPE at the HTTP layer; this bounds EXISTENCE — and it is a
// security control, not a nicety: without it, any of ~10^10 well-formed junk
// scene values mints a distinct cache key and a guaranteed DB aggregate,
// which a single client under the public rate ceiling could use to keep the
// module cache full of junk and force all chart traffic to run uncached.
// Gating here collapses the scene key space to the ~900 real CBSA codes and
// short-circuits unknown scenes to the documented empty envelope with ZERO
// cache or DB work. Safe by provenance: artists.metro / venues.metro are
// only ever written from geo.ResolveMetro over this same dataset, so a scene
// that fails this lookup cannot match any row anyway.
func chartSceneExists(scene string) bool {
	if scene == "" {
		return true
	}
	_, ok := geo.MetroPrincipalByCBSA(scene)
	return ok
}

// appendEntityMetroScope appends the home-metro equality predicate for the
// entity aliased `alias` (artists or venues — both carry a metro column) when
// scene is non-empty. `alias` must be a compile-time literal. Callers gate
// scene through chartSceneExists first, so a non-empty scene here is a real
// CBSA code.
func appendEntityMetroScope(query string, args []any, alias, scene string) (string, []any) {
	if scene == "" {
		return query, args
	}
	query += `
			AND ` + alias + `.metro = ?`
	args = append(args, scene)
	return query, args
}

// appendShowSceneScope appends the venue-metro scene predicate for shows
// aliased `s`: the show counts when ANY of its venues sits in the metro. A
// multi-venue show can count toward each of its venues' scenes — the same
// attribution the scenes directory uses when it groups a show under every
// linked venue's scene key.
func appendShowSceneScope(query string, args []any, scene string) (string, []any) {
	if scene == "" {
		return query, args
	}
	query += `
			AND EXISTS (SELECT 1 FROM show_venues scv JOIN venues scvv ON scvv.id = scv.venue_id
				WHERE scv.show_id = s.id AND scvv.metro = ?)`
	args = append(args, scene)
	return query, args
}

// appendReleaseSceneScope appends the credited-artist home-metro scene
// predicate for releases aliased `r`: the release counts when ANY credited
// artist is based in the metro.
func appendReleaseSceneScope(query string, args []any, scene string) (string, []any) {
	if scene == "" {
		return query, args
	}
	query += `
			AND EXISTS (SELECT 1 FROM artist_releases scar JOIN artists sca ON sca.id = scar.artist_id
				WHERE scar.release_id = r.id AND sca.metro = ?)`
	args = append(args, scene)
	return query, args
}

// GetMostActiveArtists returns artists ranked by approved, non-cancelled
// shows played within the window (see appendChartShowWindow for the exact
// eligibility semantics), paginated with offset-stable ranks and the window
// total. Headline share uses headlineSlotPredicate. Artists with zero shows
// in the window are never returned. scene scopes to artists BASED in the
// metro (home metro), counting all their in-window shows wherever played.
func (s *ChartsService) GetMostActiveArtists(window contracts.ChartWindow, scene string, limit, offset int) ([]contracts.MostActiveArtist, int, error) {
	if !chartSceneExists(scene) {
		return []contracts.MostActiveArtist{}, 0, nil
	}
	return cachedChartPage(s.cache, "most-active-artists", window, scene, limit, offset, func() ([]contracts.MostActiveArtist, int, error) {
		return s.getMostActiveArtistsUncached(window, scene, limit, offset)
	})
}

func (s *ChartsService) getMostActiveArtistsUncached(window contracts.ChartWindow, scene string, limit, offset int) ([]contracts.MostActiveArtist, int, error) {
	if s.db == nil {
		return nil, 0, fmt.Errorf("database not initialized")
	}

	now := time.Now().UTC()
	bounds := chartWindowBounds(window, now)

	type artistRow struct {
		ArtistID      uint   `gorm:"column:artist_id"`
		Name          string `gorm:"column:name"`
		Slug          string `gorm:"column:slug"`
		City          string `gorm:"column:city"`
		State         string `gorm:"column:state"`
		ShowCount     int    `gorm:"column:show_count"`
		HeadlineCount int    `gorm:"column:headline_count"`
		Total         int    `gorm:"column:total"`
	}

	coreSQL := `
		SELECT
			a.id AS artist_id,
			a.name,
			COALESCE(a.slug, '') AS slug,
			COALESCE(a.city, '') AS city,
			COALESCE(a.state, '') AS state,
			COUNT(*) AS show_count,
			COALESCE(SUM(CASE WHEN ` + headlineSlotPredicate + ` THEN 1 ELSE 0 END), 0) AS headline_count,
			COUNT(*) OVER() AS total
		FROM show_artists sa
		JOIN artists a ON a.id = sa.artist_id
		JOIN shows s ON s.id = sa.show_id
		WHERE s.status = ?`
	coreArgs := []any{catalogm.ShowStatusApproved}
	coreSQL, coreArgs = appendChartShowWindow(coreSQL, coreArgs, bounds)
	coreSQL, coreArgs = appendEntityMetroScope(coreSQL, coreArgs, "a", scene)
	coreSQL += `
		GROUP BY a.id, a.name, a.slug, a.city, a.state`

	query := coreSQL + `
		ORDER BY show_count DESC, a.name ASC, a.id ASC
		LIMIT ? OFFSET ?`
	args := appendPageArgs(coreArgs, limit, offset)

	var rows []artistRow
	if err := s.db.Raw(query, args...).Scan(&rows).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to get most-active artists: %w", err)
	}

	pageTotal := 0
	if len(rows) > 0 {
		pageTotal = rows[0].Total
	}
	total, err := s.resolveChartPageTotal(window, pageTotal, len(rows), offset, coreSQL, coreArgs,
		chartCountKey("most-active-artists", string(window.OrDefault()), scene), "most-active artists")
	if err != nil {
		return nil, 0, err
	}

	results := make([]contracts.MostActiveArtist, len(rows))
	artistIDs := make([]uint, len(rows))
	for i, r := range rows {
		headlinePct := 0
		if r.ShowCount > 0 {
			headlinePct = int(float64(r.HeadlineCount)/float64(r.ShowCount)*100 + 0.5)
		}
		results[i] = contracts.MostActiveArtist{
			ArtistID:    r.ArtistID,
			Name:        r.Name,
			Slug:        r.Slug,
			City:        r.City,
			State:       r.State,
			ShowCount:   r.ShowCount,
			HeadlinePct: headlinePct,
			Rank:        offset + i + 1,
		}
		artistIDs[i] = r.ArtistID
	}

	// Enrich with each artist's most recent show in the window (one query).
	// Deliberately NOT scene-scoped: the module already selected artists BASED
	// in the scene; their most recent show is a fact about the artist wherever
	// it happened (a Phoenix band's last show can be in LA).
	if len(artistIDs) > 0 {
		type lastShowRow struct {
			ArtistID  uint      `gorm:"column:artist_id"`
			EventDate time.Time `gorm:"column:event_date"`
			ShowSlug  string    `gorm:"column:show_slug"`
			VenueName string    `gorm:"column:venue_name"`
		}
		lastQuery := `
			SELECT DISTINCT ON (sa.artist_id)
				sa.artist_id,
				s.event_date,
				COALESCE(s.slug, '') AS show_slug,
				COALESCE(v.name, '') AS venue_name
			FROM show_artists sa
			JOIN shows s ON s.id = sa.show_id
			LEFT JOIN show_venues sv ON sv.show_id = s.id
			LEFT JOIN venues v ON v.id = sv.venue_id
			WHERE sa.artist_id IN ?
				AND s.status = ?`
		lastArgs := []any{artistIDs, catalogm.ShowStatusApproved}
		lastQuery, lastArgs = appendChartShowWindow(lastQuery, lastArgs, bounds)
		// s.id and v.name tiebreaks keep the picked row deterministic when an
		// artist plays two shows on one date or a show has multiple venue links.
		lastQuery += `
			ORDER BY sa.artist_id, s.event_date DESC, s.id DESC, v.name ASC`

		var lastRows []lastShowRow
		if err := s.db.Raw(lastQuery, lastArgs...).Scan(&lastRows).Error; err != nil {
			return nil, 0, fmt.Errorf("failed to get last shows for most-active artists: %w", err)
		}

		lastByArtist := make(map[uint]lastShowRow, len(lastRows))
		for _, lr := range lastRows {
			lastByArtist[lr.ArtistID] = lr
		}
		for i := range results {
			if lr, ok := lastByArtist[results[i].ArtistID]; ok {
				date := lr.EventDate
				results[i].LastShowDate = &date
				results[i].LastShowSlug = lr.ShowSlug
				results[i].LastShowVenue = lr.VenueName
			}
		}
	}

	return results, total, nil
}

// GetBusiestVenues returns venues ranked by approved, non-cancelled shows
// HOSTED (past tense) within the window — distinct from GetActiveVenues,
// which scores venues by upcoming shows + follows — paginated with
// offset-stable ranks and the window total. Venues with zero shows in the
// window are never returned. scene scopes to venues IN the metro (the venue's
// own metro — direct equality, no EXISTS needed since the venue is the ranked
// entity).
func (s *ChartsService) GetBusiestVenues(window contracts.ChartWindow, scene string, limit, offset int) ([]contracts.BusiestVenue, int, error) {
	if !chartSceneExists(scene) {
		return []contracts.BusiestVenue{}, 0, nil
	}
	return cachedChartPage(s.cache, "busiest-venues", window, scene, limit, offset, func() ([]contracts.BusiestVenue, int, error) {
		return s.getBusiestVenuesUncached(window, scene, limit, offset)
	})
}

func (s *ChartsService) getBusiestVenuesUncached(window contracts.ChartWindow, scene string, limit, offset int) ([]contracts.BusiestVenue, int, error) {
	if s.db == nil {
		return nil, 0, fmt.Errorf("database not initialized")
	}

	now := time.Now().UTC()
	bounds := chartWindowBounds(window, now)

	type venueRow struct {
		VenueID   uint   `gorm:"column:venue_id"`
		Name      string `gorm:"column:name"`
		Slug      string `gorm:"column:slug"`
		City      string `gorm:"column:city"`
		State     string `gorm:"column:state"`
		ShowCount int    `gorm:"column:show_count"`
		Total     int    `gorm:"column:total"`
	}

	coreSQL := `
		SELECT
			v.id AS venue_id,
			v.name,
			COALESCE(v.slug, '') AS slug,
			COALESCE(v.city, '') AS city,
			COALESCE(v.state, '') AS state,
			COUNT(*) AS show_count,
			COUNT(*) OVER() AS total
		FROM show_venues sv
		JOIN venues v ON v.id = sv.venue_id
		JOIN shows s ON s.id = sv.show_id
		WHERE s.status = ?`
	// COUNT(*) == COUNT(DISTINCT s.id) here: show_venues' composite PK
	// (show_id, venue_id) guarantees one row per show within a venue group.
	coreArgs := []any{catalogm.ShowStatusApproved}
	coreSQL, coreArgs = appendChartShowWindow(coreSQL, coreArgs, bounds)
	coreSQL, coreArgs = appendEntityMetroScope(coreSQL, coreArgs, "v", scene)
	coreSQL += `
		GROUP BY v.id, v.name, v.slug, v.city, v.state`

	query := coreSQL + `
		ORDER BY show_count DESC, v.name ASC, v.id ASC
		LIMIT ? OFFSET ?`
	args := appendPageArgs(coreArgs, limit, offset)

	var rows []venueRow
	if err := s.db.Raw(query, args...).Scan(&rows).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to get busiest venues: %w", err)
	}

	pageTotal := 0
	if len(rows) > 0 {
		pageTotal = rows[0].Total
	}
	total, err := s.resolveChartPageTotal(window, pageTotal, len(rows), offset, coreSQL, coreArgs,
		chartCountKey("busiest-venues", string(window.OrDefault()), scene), "busiest venues")
	if err != nil {
		return nil, 0, err
	}

	results := make([]contracts.BusiestVenue, len(rows))
	for i, r := range rows {
		results[i] = contracts.BusiestVenue{
			VenueID:   r.VenueID,
			Name:      r.Name,
			Slug:      r.Slug,
			City:      r.City,
			State:     r.State,
			ShowCount: r.ShowCount,
			Rank:      offset + i + 1,
		}
	}
	return results, total, nil
}

// GetOpenersToWatch returns artists ranked by support slots played within the
// window — slots that are NOT headline slots (headline = set_type 'headliner'
// OR position 0, the shared predicate). Artists with ANY headline slot in the
// window are excluded entirely: this chart surfaces artists who are always on
// the bill but never top it. Cancelled and future shows never count.
// Paginated with offset-stable ranks and the window total. scene scopes to
// artists BASED in the metro (home metro); the never-headlines judgment still
// spans ALL their in-window slots wherever played.
func (s *ChartsService) GetOpenersToWatch(window contracts.ChartWindow, scene string, limit, offset int) ([]contracts.OpenerToWatch, int, error) {
	if !chartSceneExists(scene) {
		return []contracts.OpenerToWatch{}, 0, nil
	}
	return cachedChartPage(s.cache, "openers-to-watch", window, scene, limit, offset, func() ([]contracts.OpenerToWatch, int, error) {
		return s.getOpenersToWatchUncached(window, scene, limit, offset)
	})
}

func (s *ChartsService) getOpenersToWatchUncached(window contracts.ChartWindow, scene string, limit, offset int) ([]contracts.OpenerToWatch, int, error) {
	if s.db == nil {
		return nil, 0, fmt.Errorf("database not initialized")
	}

	now := time.Now().UTC()
	bounds := chartWindowBounds(window, now)

	type openerRow struct {
		ArtistID         uint   `gorm:"column:artist_id"`
		Name             string `gorm:"column:name"`
		Slug             string `gorm:"column:slug"`
		City             string `gorm:"column:city"`
		State            string `gorm:"column:state"`
		SupportSlotCount int    `gorm:"column:support_slot_count"`
		Total            int    `gorm:"column:total"`
	}

	// One pass: group every in-window slot per artist, keep only groups with
	// ZERO headline slots (HAVING) — so COUNT(*) is exactly the support-slot
	// count, and "never headlines" is judged over the same window being
	// ranked. The CASE form also counts NULL set_type rows as support,
	// matching GetMostActiveArtists' NULL semantics. COUNT(*) OVER() runs
	// after the HAVING filter, so total counts qualifying openers only.
	coreSQL := `
		SELECT
			a.id AS artist_id,
			a.name,
			COALESCE(a.slug, '') AS slug,
			COALESCE(a.city, '') AS city,
			COALESCE(a.state, '') AS state,
			COUNT(*) AS support_slot_count,
			COUNT(*) OVER() AS total
		FROM show_artists sa
		JOIN artists a ON a.id = sa.artist_id
		JOIN shows s ON s.id = sa.show_id
		WHERE s.status = ?`
	coreArgs := []any{catalogm.ShowStatusApproved}
	coreSQL, coreArgs = appendChartShowWindow(coreSQL, coreArgs, bounds)
	// Artist-home scoping filters WHICH artists appear without touching their
	// slot rows, so the HAVING below still judges "never headlines" over the
	// artist's full in-window slot set.
	coreSQL, coreArgs = appendEntityMetroScope(coreSQL, coreArgs, "a", scene)
	coreSQL += `
		GROUP BY a.id, a.name, a.slug, a.city, a.state
		HAVING SUM(CASE WHEN ` + headlineSlotPredicate + ` THEN 1 ELSE 0 END) = 0`

	query := coreSQL + `
		ORDER BY support_slot_count DESC, a.name ASC, a.id ASC
		LIMIT ? OFFSET ?`
	args := appendPageArgs(coreArgs, limit, offset)

	var rows []openerRow
	if err := s.db.Raw(query, args...).Scan(&rows).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to get openers to watch: %w", err)
	}

	pageTotal := 0
	if len(rows) > 0 {
		pageTotal = rows[0].Total
	}
	total, err := s.resolveChartPageTotal(window, pageTotal, len(rows), offset, coreSQL, coreArgs,
		chartCountKey("openers-to-watch", string(window.OrDefault()), scene), "openers to watch")
	if err != nil {
		return nil, 0, err
	}

	results := make([]contracts.OpenerToWatch, len(rows))
	for i, r := range rows {
		results[i] = contracts.OpenerToWatch{
			ArtistID:         r.ArtistID,
			Name:             r.Name,
			Slug:             r.Slug,
			City:             r.City,
			State:            r.State,
			SupportSlotCount: r.SupportSlotCount,
			Rank:             offset + i + 1,
		}
	}
	return results, total, nil
}

// broadcasterKeySQL is the SQL identity of "one broadcaster" for station
// counting: stations grouped under a radio_network collapse to the network
// (WFMU's flagship + stream-only sub-channels are one broadcaster, not four),
// standalone stations count individually. Negating the station id keeps the
// two key spaces disjoint — both columns are positive serials. The collapse
// also keeps counts stable across the known WFMU family show mis-attribution
// churn, since every family member maps to the same network. Any future
// surface that shows a per-artist station count (drill-downs, artist-page
// radio strips) must count through this same identity or its numbers will
// contradict the chart's.
const broadcasterKeySQL = `COALESCE(rst.network_id, -rst.id)`

// GetOnTheRadioArtists returns artists ranked by resolved radio plays within
// the window — the zero-engagement discovery signal from station playlists.
// Only plays with a resolved artist_id count (unmatched plays are excluded),
// pseudo-artist rows ("Music behind DJ:" segments) are excluded like every
// other radio aggregation, and only plays on episodes that have actually
// aired count: air_date on/before the station-local today (resolved through
// pg_timezone_names exactly like the "Latest playlists" feed) plus the shared
// air-window gate (airedEpisodeVisibleSQL). Without the aired pair, WFMU's
// pre-published upcoming episodes (which can already carry plays) would count
// before airing. The window LOWER bound stays UTC-day like the show charts —
// a rolling-window boundary is arbitrary; only the aired gate is
// correctness-critical.
//
// IsNew is true when ANY in-window play was flagged new rotation. Artists
// with zero in-window plays are never returned. Paginated with offset-stable
// ranks and the window total. This is the heaviest
// all_time module (radio_plays grows by ingestion, not curation, and
// pg_timezone_names is materialized per request), so it leans hardest on the
// module cache.
func (s *ChartsService) GetOnTheRadioArtists(window contracts.ChartWindow, scene string, limit, offset int) ([]contracts.OnTheRadioArtist, int, error) {
	if !chartSceneExists(scene) {
		return []contracts.OnTheRadioArtist{}, 0, nil
	}
	return cachedChartPage(s.cache, "on-the-radio", window, scene, limit, offset, func() ([]contracts.OnTheRadioArtist, int, error) {
		return s.getOnTheRadioArtistsUncached(window, scene, limit, offset)
	})
}

func (s *ChartsService) getOnTheRadioArtistsUncached(window contracts.ChartWindow, scene string, limit, offset int) ([]contracts.OnTheRadioArtist, int, error) {
	if s.db == nil {
		return nil, 0, fmt.Errorf("database not initialized")
	}

	now := time.Now().UTC()
	bounds := chartWindowBounds(window, now)

	type radioRow struct {
		ArtistID     uint   `gorm:"column:artist_id"`
		Name         string `gorm:"column:name"`
		Slug         string `gorm:"column:slug"`
		City         string `gorm:"column:city"`
		State        string `gorm:"column:state"`
		PlayCount    int    `gorm:"column:play_count"`
		StationCount int    `gorm:"column:station_count"`
		IsNew        bool   `gorm:"column:is_new"`
		Total        int    `gorm:"column:total"`
	}

	// GROUP BY a.id alone: the artists PK makes the selected columns
	// functionally dependent, so the aggregate hashes an 8-byte key instead of
	// the full text tuple. The sibling show-chart modules predate this and
	// enumerate every selected column — both forms are correct; prefer this
	// one, it matters more here (radio_plays outgrows show_artists by orders
	// of magnitude). The aired pair (station-local date bound + air-window
	// gate) is the shared radio.go definition — see stationLocalAiredDateBoundSQL.
	coreSQL := `
		SELECT
			a.id AS artist_id,
			a.name,
			COALESCE(a.slug, '') AS slug,
			COALESCE(a.city, '') AS city,
			COALESCE(a.state, '') AS state,
			COUNT(*) AS play_count,
			COUNT(DISTINCT ` + broadcasterKeySQL + `) AS station_count,
			BOOL_OR(rp.is_new) AS is_new,
			COUNT(*) OVER() AS total
		FROM radio_plays rp
		JOIN radio_episodes re ON re.id = rp.episode_id
		JOIN radio_shows rsh ON rsh.id = re.show_id
		JOIN radio_stations rst ON rst.id = rsh.station_id
		JOIN artists a ON a.id = rp.artist_id
		` + stationTimezoneJoinSQL + `
		WHERE `
	var coreArgs []any
	coreSQL, coreArgs = appendRadioAiredWindow(coreSQL, coreArgs, now, bounds)
	// scene scopes to the ARTIST's home metro (a is the resolved play's
	// artist): "our scene's bands on the air", not "stations broadcasting
	// from the metro" — stations have no metro column at all.
	coreSQL, coreArgs = appendEntityMetroScope(coreSQL, coreArgs, "a", scene)
	coreSQL += `
		GROUP BY a.id`

	query := coreSQL + `
		ORDER BY play_count DESC, a.name ASC, a.id ASC
		LIMIT ? OFFSET ?`
	args := appendPageArgs(coreArgs, limit, offset)

	var rows []radioRow
	if err := s.db.Raw(query, args...).Scan(&rows).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to get on-the-radio artists: %w", err)
	}

	pageTotal := 0
	if len(rows) > 0 {
		pageTotal = rows[0].Total
	}
	total, err := s.resolveChartPageTotal(window, pageTotal, len(rows), offset, coreSQL, coreArgs,
		chartCountKey("on-the-radio", string(window.OrDefault()), scene), "on-the-radio artists")
	if err != nil {
		return nil, 0, err
	}

	results := make([]contracts.OnTheRadioArtist, len(rows))
	for i, r := range rows {
		results[i] = contracts.OnTheRadioArtist{
			ArtistID:     r.ArtistID,
			Name:         r.Name,
			Slug:         r.Slug,
			City:         r.City,
			State:        r.State,
			PlayCount:    r.PlayCount,
			StationCount: r.StationCount,
			IsNew:        r.IsNew,
			Rank:         offset + i + 1,
		}
	}
	return results, total, nil
}

// GetPopularArtists returns artists ranked by a composite score of followers and upcoming shows.
// Score = follow_count * 2 + upcoming_show_count.
func (s *ChartsService) GetPopularArtists(limit int) ([]contracts.PopularArtist, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	now := time.Now().UTC()

	type artistRow struct {
		ArtistID          uint   `gorm:"column:artist_id"`
		Name              string `gorm:"column:name"`
		Slug              string `gorm:"column:slug"`
		ImageURL          string `gorm:"column:image_url"`
		FollowCount       int    `gorm:"column:follow_count"`
		UpcomingShowCount int    `gorm:"column:upcoming_show_count"`
		Score             int    `gorm:"column:score"`
	}

	var rows []artistRow
	err := s.db.Raw(`
		SELECT
			a.id AS artist_id,
			a.name,
			COALESCE(a.slug, '') AS slug,
			COALESCE(a.bandcamp_embed_url, '') AS image_url,
			COALESCE(follow_counts.cnt, 0) AS follow_count,
			COALESCE(show_counts.cnt, 0) AS upcoming_show_count,
			(COALESCE(follow_counts.cnt, 0) * 2 + COALESCE(show_counts.cnt, 0)) AS score
		FROM artists a
		LEFT JOIN (
			SELECT entity_id, COUNT(*) AS cnt
			FROM user_bookmarks
			WHERE entity_type = ? AND action = ?
			GROUP BY entity_id
		) follow_counts ON follow_counts.entity_id = a.id
		LEFT JOIN (
			SELECT sa.artist_id, COUNT(DISTINCT s.id) AS cnt
			FROM show_artists sa
			JOIN shows s ON s.id = sa.show_id
			WHERE s.status = ? AND s.event_date >= ?
			GROUP BY sa.artist_id
		) show_counts ON show_counts.artist_id = a.id
		WHERE (COALESCE(follow_counts.cnt, 0) > 0 OR COALESCE(show_counts.cnt, 0) > 0)
		ORDER BY score DESC, a.name ASC
		LIMIT ?
	`, engagementm.BookmarkEntityArtist, engagementm.BookmarkActionFollow,
		catalogm.ShowStatusApproved, now, limit).Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("failed to get popular artists: %w", err)
	}

	results := make([]contracts.PopularArtist, len(rows))
	for i, r := range rows {
		results[i] = contracts.PopularArtist{
			ArtistID:          r.ArtistID,
			Name:              r.Name,
			Slug:              r.Slug,
			ImageURL:          r.ImageURL,
			FollowCount:       r.FollowCount,
			UpcomingShowCount: r.UpcomingShowCount,
			Score:             r.Score,
		}
	}

	return results, nil
}

// GetActiveVenues returns venues ranked by a composite score of UPCOMING
// shows and followers (score = upcoming_show_count * 2 + follow_count) —
// distinct from GetBusiestVenues, which counts past shows hosted in a window.
func (s *ChartsService) GetActiveVenues(limit int) ([]contracts.ActiveVenue, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	now := time.Now().UTC()

	type venueRow struct {
		VenueID           uint   `gorm:"column:venue_id"`
		Name              string `gorm:"column:name"`
		Slug              string `gorm:"column:slug"`
		City              string `gorm:"column:city"`
		State             string `gorm:"column:state"`
		UpcomingShowCount int    `gorm:"column:upcoming_show_count"`
		FollowCount       int    `gorm:"column:follow_count"`
		Score             int    `gorm:"column:score"`
	}

	var rows []venueRow
	err := s.db.Raw(`
		SELECT
			v.id AS venue_id,
			v.name,
			COALESCE(v.slug, '') AS slug,
			COALESCE(v.city, '') AS city,
			COALESCE(v.state, '') AS state,
			COALESCE(show_counts.cnt, 0) AS upcoming_show_count,
			COALESCE(follow_counts.cnt, 0) AS follow_count,
			(COALESCE(show_counts.cnt, 0) * 2 + COALESCE(follow_counts.cnt, 0)) AS score
		FROM venues v
		LEFT JOIN (
			SELECT sv.venue_id, COUNT(DISTINCT s.id) AS cnt
			FROM show_venues sv
			JOIN shows s ON s.id = sv.show_id
			WHERE s.status = ? AND s.event_date >= ?
			GROUP BY sv.venue_id
		) show_counts ON show_counts.venue_id = v.id
		LEFT JOIN (
			SELECT entity_id, COUNT(*) AS cnt
			FROM user_bookmarks
			WHERE entity_type = ? AND action = ?
			GROUP BY entity_id
		) follow_counts ON follow_counts.entity_id = v.id
		WHERE (COALESCE(show_counts.cnt, 0) > 0 OR COALESCE(follow_counts.cnt, 0) > 0)
		ORDER BY score DESC, v.name ASC
		LIMIT ?
	`, catalogm.ShowStatusApproved, now,
		engagementm.BookmarkEntityVenue, engagementm.BookmarkActionFollow, limit).Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("failed to get active venues: %w", err)
	}

	results := make([]contracts.ActiveVenue, len(rows))
	for i, r := range rows {
		results[i] = contracts.ActiveVenue{
			VenueID:           r.VenueID,
			Name:              r.Name,
			Slug:              r.Slug,
			City:              r.City,
			State:             r.State,
			UpcomingShowCount: r.UpcomingShowCount,
			FollowCount:       r.FollowCount,
			Score:             r.Score,
		}
	}

	return results, nil
}

// GetHotReleases returns releases ranked by recent bookmark count, falling back to
// recently added releases when no bookmarks exist so the chart is never empty.
//
// DEPRECATED in favor of GetNewReleases, which replaces it for the redesigned
// charts page; this stays only until the frontend hook migrates off
// /charts/hot-releases. At current engagement volume the bookmark ranking
// silently degrades to "recently added" while claiming to be "hot" — the
// replacement drops engagement inputs and makes the date ordering explicit.
func (s *ChartsService) GetHotReleases(limit int) ([]contracts.HotRelease, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	thirtyDaysAgo := time.Now().UTC().AddDate(0, 0, -30)

	type releaseRow struct {
		ReleaseID     uint    `gorm:"column:release_id"`
		Title         string  `gorm:"column:title"`
		Slug          string  `gorm:"column:slug"`
		ReleaseDate   *string `gorm:"column:release_date"`
		BookmarkCount int     `gorm:"column:bookmark_count"`
	}

	var rows []releaseRow
	err := s.db.Raw(`
		SELECT
			r.id AS release_id,
			r.title,
			COALESCE(r.slug, '') AS slug,
			r.release_date,
			COALESCE(COUNT(ub.id), 0) AS bookmark_count
		FROM releases r
		LEFT JOIN user_bookmarks ub ON ub.entity_id = r.id
			AND ub.entity_type = ?
			AND ub.action = ?
			AND ub.created_at >= ?
		GROUP BY r.id, r.title, r.slug, r.release_date
		ORDER BY bookmark_count DESC, r.created_at DESC
		LIMIT ?
	`, engagementm.BookmarkEntityRelease, engagementm.BookmarkActionReleaseSave, thirtyDaysAgo, limit).Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("failed to get hot releases: %w", err)
	}

	// Build result list, then enrich with artist names
	results := make([]contracts.HotRelease, len(rows))
	releaseIDs := make([]uint, len(rows))
	for i, r := range rows {
		var releaseDate *time.Time
		if r.ReleaseDate != nil {
			if t, err := time.Parse("2006-01-02", *r.ReleaseDate); err == nil {
				releaseDate = &t
			}
		}
		results[i] = contracts.HotRelease{
			ReleaseID:     r.ReleaseID,
			Title:         r.Title,
			Slug:          r.Slug,
			ReleaseDate:   releaseDate,
			ArtistNames:   []string{},
			BookmarkCount: r.BookmarkCount,
		}
		releaseIDs[i] = r.ReleaseID
	}

	// Fetch artist names for all releases in one query
	artistMap, err := s.releaseArtistNames(releaseIDs)
	if err != nil {
		return nil, err
	}
	for i := range results {
		if names, ok := artistMap[results[i].ReleaseID]; ok {
			results[i].ArtistNames = names
		}
	}

	return results, nil
}

// releaseArtistNames returns credit-ordered artist names for each release, in
// one query. Shared by the release chart modules (hot / new releases).
func (s *ChartsService) releaseArtistNames(releaseIDs []uint) (map[uint][]string, error) {
	return s.namesByOwnerID(`
		SELECT ar.release_id AS owner_id, a.name
		FROM artist_releases ar
		JOIN artists a ON a.id = ar.artist_id
		WHERE ar.release_id IN ?
		GROUP BY ar.release_id, a.id, a.name
		ORDER BY ar.release_id, MIN(ar.position), a.name, a.id
	`, releaseIDs, "release artists")
}

func (s *ChartsService) releaseArtistReferences(releaseIDs []uint) (map[uint][]contracts.ChartEntityReference, error) {
	return s.referencesByOwnerID(`
		SELECT ar.release_id AS owner_id, a.id, a.name, COALESCE(a.slug, '') AS slug
		FROM artist_releases ar
		JOIN artists a ON a.id = ar.artist_id
		WHERE ar.release_id IN ?
		GROUP BY ar.release_id, a.id, a.name, a.slug
		ORDER BY ar.release_id, MIN(ar.position), a.name, a.id
	`, releaseIDs, "release artist references")
}

// newReleaseDateSQL is the ordering/window date of the new-releases module:
// the world release date when known, else the UTC day the release entered
// the graph. AT TIME ZONE 'UTC' pins the timestamptz→date cast — a bare
// ::date uses the session timezone, silently moving window edges when the
// server isn't UTC. Day-granular on purpose — releases are day-grain
// entities. Any windowed variant of this query (e.g. scene-scoped) must
// reuse BOTH bounds built on this expression: `<= today` (future-dated
// announcements stay out until release day) and the inclusive `>= start`.
const newReleaseDateSQL = `COALESCE(r.release_date, (r.created_at AT TIME ZONE 'UTC')::date)`

// GetNewReleases returns releases date-ordered (newest first) within the
// window — the honest "what came out / arrived recently" list, with NO
// engagement inputs. Future-dated releases (announced but not yet out) are
// excluded until their release day, mirroring the played-by-now convention of
// the show charts. Ties on the day break by created_at then id, so the
// ordering is fully deterministic — which the offset-stable ranks rely on.
// Paginated with the window total. The ordering expression is served
// by the charts cost-lever expression index (charts_cost_indexes migration).
func (s *ChartsService) GetNewReleases(window contracts.ChartWindow, scene string, limit, offset int) ([]contracts.NewRelease, int, error) {
	if !chartSceneExists(scene) {
		return []contracts.NewRelease{}, 0, nil
	}
	return cachedChartPage(s.cache, "new-releases", window, scene, limit, offset, func() ([]contracts.NewRelease, int, error) {
		return s.getNewReleasesUncached(window, scene, limit, offset)
	})
}

func (s *ChartsService) getNewReleasesUncached(window contracts.ChartWindow, scene string, limit, offset int) ([]contracts.NewRelease, int, error) {
	if s.db == nil {
		return nil, 0, fmt.Errorf("database not initialized")
	}

	now := time.Now().UTC()
	bounds := chartWindowBounds(window, now)

	type releaseRow struct {
		ReleaseID   uint       `gorm:"column:release_id"`
		Title       string     `gorm:"column:title"`
		Slug        string     `gorm:"column:slug"`
		ReleaseType string     `gorm:"column:release_type"`
		ReleaseDate *time.Time `gorm:"column:release_date"`
		AddedAt     time.Time  `gorm:"column:added_at"`
		Total       int        `gorm:"column:total"`
	}

	// formatDay renders the DATE scan back to the contract's day-grain
	// YYYY-MM-DD string (pgx scans DATE as midnight-UTC time.Time).
	formatDay := func(t *time.Time) *string {
		if t == nil {
			return nil
		}
		s := t.Format("2006-01-02")
		return &s
	}

	upperOperator := "<="
	upperDay := now.Format("2006-01-02")
	if bounds.calendarEnd != nil && !bounds.calendarEnd.After(now) {
		upperOperator = "<"
		upperDay = bounds.calendarEnd.Format("2006-01-02")
	}
	coreSQL := `
		SELECT
			r.id AS release_id,
			r.title,
			COALESCE(r.slug, '') AS slug,
			r.release_type,
			r.release_date,
			r.created_at AS added_at,
			COUNT(*) OVER() AS total
		FROM releases r
		WHERE ` + newReleaseDateSQL + ` ` + upperOperator + ` ?`
	coreArgs := []any{upperDay}
	if bounds.start != nil {
		coreSQL += `
			AND ` + newReleaseDateSQL + ` >= ?`
		coreArgs = append(coreArgs, bounds.start.Format("2006-01-02"))
	}
	// scene = releases by artists BASED in the metro (credited-artist home
	// metro) — a release has no venue to attribute a place through.
	coreSQL, coreArgs = appendReleaseSceneScope(coreSQL, coreArgs, scene)

	query := coreSQL + `
		ORDER BY ` + newReleaseDateSQL + ` DESC, r.created_at DESC, r.id DESC
		LIMIT ? OFFSET ?`
	args := appendPageArgs(coreArgs, limit, offset)

	var rows []releaseRow
	if err := s.db.Raw(query, args...).Scan(&rows).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to get new releases: %w", err)
	}

	pageTotal := 0
	if len(rows) > 0 {
		pageTotal = rows[0].Total
	}
	total, err := s.resolveChartPageTotal(window, pageTotal, len(rows), offset, coreSQL, coreArgs,
		chartCountKey("new-releases", string(window.OrDefault()), scene), "new releases")
	if err != nil {
		return nil, 0, err
	}

	results := make([]contracts.NewRelease, len(rows))
	releaseIDs := make([]uint, len(rows))
	for i, r := range rows {
		results[i] = contracts.NewRelease{
			ReleaseID:   r.ReleaseID,
			Title:       r.Title,
			Slug:        r.Slug,
			ReleaseType: r.ReleaseType,
			ReleaseDate: formatDay(r.ReleaseDate),
			AddedAt:     r.AddedAt,
			ArtistNames: []string{},
			LabelNames:  []string{},
			Artists:     []contracts.ChartEntityReference{},
			Labels:      []contracts.ChartEntityReference{},
			Rank:        offset + i + 1,
		}
		releaseIDs[i] = r.ReleaseID
	}

	artistMap, err := s.releaseArtistReferences(releaseIDs)
	if err != nil {
		return nil, 0, err
	}
	labelMap, err := s.releaseLabelReferences(releaseIDs)
	if err != nil {
		return nil, 0, err
	}
	for i := range results {
		if artists, ok := artistMap[results[i].ReleaseID]; ok {
			results[i].Artists = artists
			for _, artist := range artists {
				results[i].ArtistNames = append(results[i].ArtistNames, artist.Name)
			}
		}
		if labels, ok := labelMap[results[i].ReleaseID]; ok {
			results[i].Labels = labels
			for _, label := range labels {
				results[i].LabelNames = append(results[i].LabelNames, label.Name)
			}
		}
	}

	return results, total, nil
}

func (s *ChartsService) releaseLabelReferences(releaseIDs []uint) (map[uint][]contracts.ChartEntityReference, error) {
	return s.referencesByOwnerID(`
		SELECT rl.release_id AS owner_id, l.id, l.name, COALESCE(l.slug, '') AS slug
		FROM release_labels rl
		JOIN labels l ON l.id = rl.label_id
		WHERE rl.release_id IN ?
		ORDER BY rl.release_id, l.name ASC, l.id ASC
	`, releaseIDs, "release label references")
}

// appendWindowBounds appends the generic chart window bounds for `column` —
// on/before now plus the optional window lower bound — and the matching
// args. `column` must be a compile-time literal, never runtime input. TWO
// consumer families ride on this single definition: the summary's created_at
// counts, and (via appendChartShowWindow) every windowed show chart's
// event_date eligibility — where the `<= now` upper bound is LOAD-BEARING:
// it is what keeps future-dated shows out of the played-show charts, so it
// must not be dropped as "redundant" for the created_at case.
func appendWindowBounds(query string, args []any, column string, bounds chartBounds) (string, []any) {
	operator := "<="
	if bounds.endExclusive {
		operator = "<"
	}
	query += `
			AND ` + column + ` ` + operator + ` ?`
	args = append(args, bounds.end)
	if bounds.start != nil {
		query += `
			AND ` + column + ` >= ?`
		args = append(args, *bounds.start)
	}
	return query, args
}

const chartRadioOccurrenceUTC = `COALESCE(re.starts_at, re.air_date::timestamp AT TIME ZONE COALESCE(tzn.name, 'UTC'))`

// appendRadioAiredWindow appends the radio aired pair — pseudo-artist
// exclusion, station-local aired date bound, air-window gate — plus the
// rolling lower bound on air_date OR exact UTC calendar bounds on the
// episode occurrence instant, and the matching args. `starts_at` is already
// UTC-aware; windowless episodes derive their occurrence from station-local
// air_date through the resolved timezone. It must be appended immediately
// after a bare `WHERE ` (the
// fragment starts with a predicate, not AND), and the FROM clause must join
// radio_episodes re, radio_stations rst, and stationTimezoneJoinSQL. Both radio-backed chart surfaces
// (on-the-radio, the summary's radio_plays count) build through this so the
// aired semantics and the placeholder arity can't drift between them.
func appendRadioAiredWindow(query string, args []any, now time.Time, bounds chartBounds) (string, []any) {
	query += pseudoArtistExclusionSQL + `
			AND ` + stationLocalAiredDateBoundSQL("re.") + `
			AND ` + airedEpisodeVisibleSQL("re.")
	args = append(args, now, now)
	if bounds.calendarEnd != nil {
		query += `
			AND ` + chartRadioOccurrenceUTC + ` >= ?
			AND ` + chartRadioOccurrenceUTC + ` < ?`
		args = append(args, *bounds.start, *bounds.calendarEnd)
	} else if bounds.start != nil {
		query += `
			AND re.air_date >= ?`
		args = append(args, bounds.start.Format("2006-01-02"))
	}
	return query, args
}

// GetChartsSummary returns the masthead stat strip counts for the window as
// ONE statement of five scalar subqueries scanned straight into the summary
// shape — one round trip, and a column/field mismatch fails loudly at scan
// time instead of silently zeroing a stat. Each count reuses the shared
// eligibility definition of the module it summarizes:
//   - shows/artists/releases count entities ADDED in the window
//     (appendWindowBounds); shows must be approved and not cancelled — a
//     cancelled show must not inflate the proof-of-life strip that every
//     module below it excludes. The artist/release counts are deliberately
//     ungated beyond that: they measure raw graph growth, expose no names,
//     and the ticker (which does expose names) is the gated surface.
//   - radio plays share the aired pair + pseudo exclusion with the
//     on-the-radio module via appendRadioAiredWindow (unmatched plays DO
//     count here — the strip measures logging activity, not match rate).
//   - active scenes share sceneGroupKeySQL/sceneVenueEligibilitySQL with the
//     scenes list and count scenes with >=1 show played in the window
//     (appendChartShowWindow semantics). NOTE this floor is deliberately
//     lower than the scenes DIRECTORY's listing thresholds (sceneMinVenues/
//     sceneMinShows): "active this window" is a different claim than
//     "established enough to list", so the strip count can exceed the
//     /scenes list length.
//
// Scene scoping (see the scene-scoping block): shows by venue metro, artists
// and radio plays by artist home metro, releases by credited-artist home
// metro. Two scoped-semantics notes: a scoped radio-plays count includes only
// RESOLVED plays (an unmatched play has no artist to place in a scene, so the
// global "logging activity" reading narrows to "scene artists' plays"), and a
// scoped active-scenes count degenerates to 0/1 by construction — it answers
// "was THIS scene active in the window", which is exactly what the scoped
// masthead should say.
func (s *ChartsService) GetChartsSummary(window contracts.ChartWindow, scene string) (*contracts.ChartsSummary, error) {
	// Shortest TTL on the page: the summary is the single heaviest aggregate
	// call, and masthead numbers tolerate 60s staleness invisibly.
	if !chartSceneExists(scene) {
		return &contracts.ChartsSummary{}, nil
	}
	key := fmt.Sprintf("summary|%s|%s", window.OrDefault(), scene)
	cache := s.chartsCacheFor(scene)
	return chartsCached(cache, key, chartWindowTTL(cache, window, chartsMastheadTTL), func() (*contracts.ChartsSummary, error) {
		return s.getChartsSummaryUncached(window, scene)
	})
}

// chartsCacheFor routes a chart payload to a cache instance by KEY
// PROVENANCE: global masthead keys ("" scene) are domain-bounded and get the
// dedicated masthead instance, whose isolation guarantee is exactly that
// client-controlled keys can never starve them of a slot. A non-empty scene
// segment IS client-controlled (any string passing the pattern tag), so
// scoped masthead payloads ride the capped module cache instead — its
// overflow rule (run uncached, never evict a FRESH entry) absorbs key-space
// pressure — and chartSceneExists has already bounded scene values to real
// CBSA codes before any key is built.
// Routing scoped keys into mastheadCache would hand attackers the exact
// starvation the split instance exists to prevent.
func (s *ChartsService) chartsCacheFor(scene string) *chartsCache {
	if scene == "" {
		return s.mastheadCache
	}
	return s.cache
}

func (s *ChartsService) getChartsSummaryUncached(window contracts.ChartWindow, scene string) (*contracts.ChartsSummary, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	now := time.Now().UTC()
	bounds := chartWindowBounds(window, now)

	var query string
	var args []any

	query = `
		SELECT
		(SELECT COUNT(*)
			FROM shows s
			WHERE s.status = ?
				AND s.is_cancelled = FALSE`
	args = append(args, catalogm.ShowStatusApproved)
	query, args = appendWindowBounds(query, args, "s.created_at", bounds)
	query, args = appendShowSceneScope(query, args, scene)

	query += `
		) AS shows_added,
		(SELECT COUNT(*)
			FROM artists a
			WHERE true`
	query, args = appendWindowBounds(query, args, "a.created_at", bounds)
	query, args = appendEntityMetroScope(query, args, "a", scene)

	query += `
		) AS new_artists,
		(SELECT COUNT(*)
			FROM releases r
			WHERE true`
	query, args = appendWindowBounds(query, args, "r.created_at", bounds)
	query, args = appendReleaseSceneScope(query, args, scene)

	query += `
		) AS new_releases,
		(SELECT COUNT(*)
			FROM radio_plays rp
			JOIN radio_episodes re ON re.id = rp.episode_id
			JOIN radio_shows rsh ON rsh.id = re.show_id
			JOIN radio_stations rst ON rst.id = rsh.station_id
			` + stationTimezoneJoinSQL + `
			WHERE `
	query, args = appendRadioAiredWindow(query, args, now, bounds)
	if scene != "" {
		// Artist-home attribution for plays; see the method comment for why
		// the scoped count narrows to resolved plays only. Inline (the second
		// documented exception in the scene-scoping block): there is no
		// joined artists alias here — keep textually in sync with
		// appendEntityMetroScope's predicate shape.
		query += `
			AND EXISTS (SELECT 1 FROM artists sca WHERE sca.id = rp.artist_id AND sca.metro = ?)`
		args = append(args, scene)
	}

	query += `
		) AS radio_plays,
		(SELECT COUNT(DISTINCT ` + sceneGroupKeySQL + `)
			FROM shows s
			JOIN show_venues sv ON sv.show_id = s.id
			JOIN venues v ON v.id = sv.venue_id
			WHERE s.status = ?
			  ` + sceneVenueEligibilitySQL
	args = append(args, catalogm.ShowStatusApproved)
	query, args = appendChartShowWindow(query, args, bounds)
	query, args = appendEntityMetroScope(query, args, "v", scene)
	query += `
		) AS active_scenes`

	type summaryRow struct {
		ShowsAdded   int `gorm:"column:shows_added"`
		NewArtists   int `gorm:"column:new_artists"`
		NewReleases  int `gorm:"column:new_releases"`
		RadioPlays   int `gorm:"column:radio_plays"`
		ActiveScenes int `gorm:"column:active_scenes"`
	}
	var row summaryRow
	if err := s.db.Raw(query, args...).Scan(&row).Error; err != nil {
		return nil, fmt.Errorf("failed to get charts summary: %w", err)
	}

	summary := contracts.ChartsSummary(row)
	return &summary, nil
}

// GetFreshlyAdded returns the most recently added entities across types
// (artist/venue/release/station) interleaved newest-first — the footer
// ticker. Each branch pre-limits to the requested size before the global
// sort so the union never materializes whole tables.
//
// Eligibility: venues must be VERIFIED — the one real moderation gate here,
// matching every public venue surface (user submissions create venues
// unverified; only admins verify). Artists must be anchored to content — an
// approved non-cancelled show, a release (admin-created), or a radio play
// (pipeline-created). NOTE the anchor is NOT a moderation gate: this site
// runs post-moderation, so a user-submitted show is approved on creation and
// anchors its artists immediately — the same names are already public on
// every show and chart surface; the ticker deliberately follows that model
// rather than being stricter than the charts above it. What the anchor DOES
// exclude: artists reachable only through private/pending shows, and
// orphaned artist rows with no content at all. Releases and stations are
// admin-only writes and need no gate.
// scene scopes the branches per the scene-scoping block: artists by home
// metro, venues by their own metro, releases by credited-artist home metro —
// and DROPS the station branch entirely (stations have no metro; a scoped
// ticker claiming a station was "added to the scene" would be a fabrication).
func (s *ChartsService) GetFreshlyAdded(scene string, limit int) ([]contracts.FreshlyAddedItem, error) {
	// Masthead TTL with the summary: the ticker's four ORDER BY created_at
	// DESC branches are also covered by the cost-lever created_at indexes.
	// Cache routing is scope-aware like the summary's — see chartsCacheFor.
	if !chartSceneExists(scene) {
		return []contracts.FreshlyAddedItem{}, nil
	}
	key := fmt.Sprintf("freshly-added|%d|%s", limit, scene)
	return chartsCached(s.chartsCacheFor(scene), key, chartsMastheadTTL, func() ([]contracts.FreshlyAddedItem, error) {
		return s.getFreshlyAddedUncached(scene, limit)
	})
}

func (s *ChartsService) getFreshlyAddedUncached(scene string, limit int) ([]contracts.FreshlyAddedItem, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	type itemRow struct {
		EntityType string    `gorm:"column:entity_type"`
		EntityID   uint      `gorm:"column:entity_id"`
		Name       string    `gorm:"column:name"`
		Slug       string    `gorm:"column:slug"`
		AddedAt    time.Time `gorm:"column:added_at"`
	}

	// Branch scene fragments are empty in the global case so the assembled
	// SQL (and its arg arity) stays semantically identical to the pre-scene
	// query (the artist anchor OR-chain gained grouping parens, nothing
	// else). The fragments are inline rather than routed through the shared
	// appenders because the UNION assembly interleaves per-branch args — keep
	// them textually in sync with appendEntityMetroScope /
	// appendReleaseSceneScope (the scene-scoping block documents this as one
	// of the two inline exceptions).
	artistScene, venueScene, releaseScene := "", "", ""
	var artistArgs, venueArgs, releaseArgs []any
	if scene != "" {
		artistScene = ` AND a.metro = ?`
		artistArgs = []any{scene}
		venueScene = ` AND v.metro = ?`
		venueArgs = []any{scene}
		releaseScene = ` WHERE EXISTS (SELECT 1 FROM artist_releases scar JOIN artists sca ON sca.id = scar.artist_id
				WHERE scar.release_id = r.id AND sca.metro = ?)`
		releaseArgs = []any{scene}
	}

	query := `
		SELECT * FROM (
			(SELECT 'artist' AS entity_type, a.id AS entity_id, a.name, COALESCE(a.slug, '') AS slug, a.created_at AS added_at
			 FROM artists a
			 WHERE (EXISTS (SELECT 1 FROM show_artists sa JOIN shows s ON s.id = sa.show_id
				WHERE sa.artist_id = a.id AND s.status = ? AND s.is_cancelled = FALSE)
				OR EXISTS (SELECT 1 FROM artist_releases ar WHERE ar.artist_id = a.id)
				OR EXISTS (SELECT 1 FROM radio_plays rp WHERE rp.artist_id = a.id))` + artistScene + `
			 ORDER BY a.created_at DESC, a.id DESC LIMIT ?)
			UNION ALL
			(SELECT 'venue', v.id, v.name, COALESCE(v.slug, ''), v.created_at
			 FROM venues v
			 WHERE v.verified = true` + venueScene + `
			 ORDER BY v.created_at DESC, v.id DESC LIMIT ?)
			UNION ALL
			(SELECT 'release', r.id, r.title, COALESCE(r.slug, ''), r.created_at
			 FROM releases r` + releaseScene + ` ORDER BY r.created_at DESC, r.id DESC LIMIT ?)`
	args := []any{catalogm.ShowStatusApproved}
	args = append(args, artistArgs...)
	args = append(args, limit)
	args = append(args, venueArgs...)
	args = append(args, limit)
	args = append(args, releaseArgs...)
	args = append(args, limit)
	if scene == "" {
		query += `
			UNION ALL
			(SELECT 'station', rst.id, rst.name, rst.slug, rst.created_at
			 FROM radio_stations rst ORDER BY rst.created_at DESC, rst.id DESC LIMIT ?)`
		args = append(args, limit)
	}
	query += `
		) x
		ORDER BY x.added_at DESC, x.entity_type ASC, x.entity_id DESC
		LIMIT ?`
	args = append(args, limit)

	var rows []itemRow
	err := s.db.Raw(query, args...).Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("failed to get freshly added items: %w", err)
	}

	results := make([]contracts.FreshlyAddedItem, len(rows))
	for i, r := range rows {
		results[i] = contracts.FreshlyAddedItem(r)
	}
	return results, nil
}

// chartSceneFloor is the minimum approved, non-cancelled shows a metro needs
// in the requested window to appear in the scene switcher — the zero-row rule
// applied to the filter itself (a switcher option whose charts would be
// near-empty reads as a dead site). 5 is the ticket's proposed value; it is
// pending validation against the stage metro distribution (flip-option
// comment on the ticket) — change it there and here together.
const chartSceneFloor = 5

// GetChartScenes returns the scene switcher's option list: CBSA metros with
// at least chartSceneFloor approved, non-cancelled shows played in the
// window, scene vitals included, busiest first. Only venue-metro attribution
// counts (the same key the module endpoints scope by), and only real CBSA
// metros appear — (city|state) fallback scenes are not chart scopes. The artist
// vital deliberately uses Charts' strict artists.metro scope; the venue vital
// uses the scene directory's verified-venue scope. Display identity includes
// the official CBSA name plus its principal city. A stored metro code missing
// from the embedded geo domain is omitted so enumeration and module validation
// cannot disagree.
func (s *ChartsService) GetChartScenes(window contracts.ChartWindow) ([]contracts.ChartScene, error) {
	// Masthead instance on purpose: this key space is domain-bounded by the
	// strict rolling/calendar grammar plus launch/future validation, and
	// the switcher list is nav-critical, so it must not compete for slots
	// with the client-controlled module keys.
	key := fmt.Sprintf("scenes|%s", window.OrDefault())
	return chartsCached(s.mastheadCache, key, chartWindowTTL(s.mastheadCache, window, chartsModuleTTL), func() ([]contracts.ChartScene, error) {
		return s.getChartScenesUncached(window)
	})
}

func (s *ChartsService) getChartScenesUncached(window contracts.ChartWindow) ([]contracts.ChartScene, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	now := time.Now().UTC()
	bounds := chartWindowBounds(window, now)

	type sceneRow struct {
		Metro       string `gorm:"column:metro"`
		City        string `gorm:"column:city"`
		State       string `gorm:"column:state"`
		ShowCount   int    `gorm:"column:show_count"`
		ArtistCount int    `gorm:"column:artist_count"`
		VenueCount  int    `gorm:"column:venue_count"`
	}

	// COUNT(DISTINCT s.id): two venues of one metro on the same show must not
	// double-count it. Venue eligibility matches the scenes directory
	// (verified + usable city/state) so a metro can't appear here on venues
	// the directory would ignore. That gate is DELIBERATELY stricter than
	// the module scene predicates (which scope content under the site's
	// post-moderation model and accept any metro venue): the switcher is a
	// venue-derived NAV surface, and venue `verified` is the one real
	// moderation gate on nav surfaces — the same call the ticker's venue
	// branch locked in. Consequence, on purpose: a metro whose in-window
	// shows sit only at unverified venues never mints a switcher option,
	// though its scene URL still resolves (scoped modules return its data);
	// and the switcher show_count can undercount a scoped module's Total.
	//
	// The city/state pair is taken from ONE deterministic venue row (lowest id)
	// for the SQL envelope; the response replaces it with the canonical geo
	// principal identity after validating that the CBSA still exists.
	//
	// The two aggregate CTEs keep the richer Figma masthead vitals in this one
	// database round trip. VenueCount is intentionally NOT windowed: "venues
	// tracked" describes catalog coverage, while ShowCount alone determines
	// whether the scene clears the requested window's navigation floor.
	query := `
		WITH eligible_scenes AS (
		SELECT
			v.metro,
			(ARRAY_AGG(v.city ORDER BY v.id))[1] AS city,
			(ARRAY_AGG(v.state ORDER BY v.id))[1] AS state,
			COUNT(DISTINCT s.id) AS show_count
		FROM venues v
		JOIN show_venues sv ON sv.venue_id = v.id
		JOIN shows s ON s.id = sv.show_id
		WHERE s.status = ?
		  AND v.metro IS NOT NULL
		  ` + sceneVenueEligibilitySQL
	args := []any{catalogm.ShowStatusApproved}
	query, args = appendChartShowWindow(query, args, bounds)
	query += `
		GROUP BY v.metro
		HAVING COUNT(DISTINCT s.id) >= ?
		),
		venue_counts AS (
			SELECT v.metro, COUNT(DISTINCT v.id) AS venue_count
			FROM venues v
			WHERE v.metro IS NOT NULL
			  ` + sceneVenueEligibilitySQL + `
			GROUP BY v.metro
		),
		artist_counts AS (
			SELECT a.metro, COUNT(DISTINCT a.id) AS artist_count
			FROM artists a
			WHERE a.metro IS NOT NULL AND a.metro <> ''
			GROUP BY a.metro
		)
		SELECT es.metro, es.city, es.state, es.show_count,
		       COALESCE(ac.artist_count, 0) AS artist_count,
		       vc.venue_count
		FROM eligible_scenes es
		JOIN venue_counts vc ON vc.metro = es.metro
		LEFT JOIN artist_counts ac ON ac.metro = es.metro
		ORDER BY es.show_count DESC, es.metro ASC`
	args = append(args, chartSceneFloor)

	var rows []sceneRow
	if err := s.db.Raw(query, args...).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("failed to get chart scenes: %w", err)
	}

	results := make([]contracts.ChartScene, 0, len(rows))
	for _, r := range rows {
		mp, ok := geo.MetroPrincipalByCBSA(r.Metro)
		if !ok {
			// Keep enumeration aligned with chartSceneExists: a stored metro code
			// outside the embedded CBSA domain must never become a selectable key.
			continue
		}
		results = append(results, contracts.ChartScene{
			Metro:       r.Metro,
			Name:        mp.Name,
			City:        mp.City,
			State:       mp.State,
			ShowCount:   r.ShowCount,
			ArtistCount: r.ArtistCount,
			VenueCount:  r.VenueCount,
		})
	}
	return results, nil
}

// GetPersonalChartsStats returns the authed personal stats strip: all-time
// aggregates over the requesting user's own user_bookmarks rows (PSY-352
// composite-PK join-table conventions — aggregate queries, no counters).
// Saved shows and the top venue count save rows uniformly, with no
// status/cancellation gate: this is the user's own private list, and the
// count matches the saved-shows page's total (both count bookmark rows;
// DeleteShow removes those rows transactionally). First activity is the
// MIN(created_at) across ALL the user's bookmark rows (any entity type or
// action) — the day they first
// engaged, not just their first show save.
//
// The top venue attributes each saved show to its primary venue — the
// lowest-venue_id pick shared with the show page and most-anticipated
// (primaryVenueLateralSQL) — so a multi-venue show counts once. Venue-less
// saved shows count toward SavedShows but never toward a venue; the inner
// JOIN (not LEFT JOIN) LATERAL is what drops them there.
//
// ONE statement on purpose: a single snapshot is what makes the shape
// guarantees hold under concurrent writes (TopVenue.SavedShowCount can never
// exceed SavedShows, and TopVenue is always nil when SavedShows is 0) — two
// queries could interleave with a save/unsave and contradict each other.
func (s *ChartsService) GetPersonalChartsStats(userID uint) (*contracts.PersonalChartsStats, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	type personalRow struct {
		SavedShows      int        `gorm:"column:saved_shows"`
		ArtistsFollowed int        `gorm:"column:artists_followed"`
		FirstActivityAt *time.Time `gorm:"column:first_activity_at"`
		VenueID         *uint      `gorm:"column:venue_id"`
		VenueName       *string    `gorm:"column:venue_name"`
		VenueSlug       *string    `gorm:"column:venue_slug"`
		SavedShowCount  *int       `gorm:"column:saved_show_count"`
	}
	// The stats subquery is one pass over the user's bookmark rows (FILTER
	// clauses share the single user_id predicate, so the counts can't
	// desynchronize on a future filter edit); the LEFT JOIN LATERAL folds in
	// the top venue, NULL columns when the user has no venue-linked saves.
	var row personalRow
	err := s.db.Raw(`
		SELECT
			stats.saved_shows,
			stats.artists_followed,
			stats.first_activity_at,
			tv.venue_id,
			tv.venue_name,
			tv.venue_slug,
			tv.saved_show_count
		FROM (
			SELECT
				COUNT(*) FILTER (WHERE entity_type = ? AND action = ?) AS saved_shows,
				COUNT(*) FILTER (WHERE entity_type = ? AND action = ?) AS artists_followed,
				MIN(created_at) AS first_activity_at
			FROM user_bookmarks
			WHERE user_id = ?
		) stats
		LEFT JOIN LATERAL (
			SELECT
				v.venue_id,
				v.venue_name,
				v.venue_slug,
				COUNT(*) AS saved_show_count
			FROM user_bookmarks ub
			JOIN LATERAL `+primaryVenueLateralSQL(
		"iv.id AS venue_id, iv.name AS venue_name, COALESCE(iv.slug, '') AS venue_slug", "ub.entity_id")+` v ON TRUE
			WHERE ub.user_id = ? AND ub.entity_type = ? AND ub.action = ?
			GROUP BY v.venue_id, v.venue_name, v.venue_slug
			ORDER BY saved_show_count DESC, v.venue_name ASC, v.venue_id ASC
			LIMIT 1
		) tv ON TRUE
	`, engagementm.BookmarkEntityShow, engagementm.BookmarkActionSave,
		engagementm.BookmarkEntityArtist, engagementm.BookmarkActionFollow,
		userID,
		userID, engagementm.BookmarkEntityShow, engagementm.BookmarkActionSave).Scan(&row).Error
	if err != nil {
		return nil, fmt.Errorf("failed to get personal charts stats: %w", err)
	}

	stats := &contracts.PersonalChartsStats{
		SavedShows:      row.SavedShows,
		ArtistsFollowed: row.ArtistsFollowed,
		FirstActivityAt: row.FirstActivityAt,
	}
	if row.VenueID != nil && row.VenueName != nil && row.VenueSlug != nil && row.SavedShowCount != nil {
		stats.TopVenue = &contracts.PersonalTopVenue{
			VenueID:        *row.VenueID,
			Name:           *row.VenueName,
			Slug:           *row.VenueSlug,
			SavedShowCount: *row.SavedShowCount,
		}
	}
	return stats, nil
}

// GetChartsOverview returns a condensed summary with top 5 of each chart.
func (s *ChartsService) GetChartsOverview() (*contracts.ChartsOverview, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	const overviewLimit = 5

	shows, err := s.GetTrendingShows(overviewLimit)
	if err != nil {
		return nil, fmt.Errorf("failed to get trending shows for overview: %w", err)
	}

	artists, err := s.GetPopularArtists(overviewLimit)
	if err != nil {
		return nil, fmt.Errorf("failed to get popular artists for overview: %w", err)
	}

	venues, err := s.GetActiveVenues(overviewLimit)
	if err != nil {
		return nil, fmt.Errorf("failed to get active venues for overview: %w", err)
	}

	releases, err := s.GetHotReleases(overviewLimit)
	if err != nil {
		return nil, fmt.Errorf("failed to get hot releases for overview: %w", err)
	}

	return &contracts.ChartsOverview{
		TrendingShows:  shows,
		PopularArtists: artists,
		ActiveVenues:   venues,
		HotReleases:    releases,
	}, nil
}
