package catalog

import (
	"fmt"
	"time"

	"gorm.io/gorm"

	"psychic-homily-backend/db"
	catalogm "psychic-homily-backend/internal/models/catalog"
	engagementm "psychic-homily-backend/internal/models/engagement"
	"psychic-homily-backend/internal/services/contracts"
)

// ChartsService computes top charts / trending content from engagement signals.
// No new tables — all data is derived from existing bookmark, show, artist, venue,
// and release tables.
type ChartsService struct {
	db *gorm.DB
}

// NewChartsService creates a new charts service.
func NewChartsService(database *gorm.DB) *ChartsService {
	if database == nil {
		database = db.GetDB()
	}
	return &ChartsService{db: database}
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

// showArtistNames returns bill-ordered artist names for each show, in one
// query. Shared by the show-row chart modules (trending / most-anticipated).
func (s *ChartsService) showArtistNames(showIDs []uint) (map[uint][]string, error) {
	if len(showIDs) == 0 {
		return nil, nil
	}
	type artistNameRow struct {
		ShowID uint   `gorm:"column:show_id"`
		Name   string `gorm:"column:name"`
	}
	var artistRows []artistNameRow
	err := s.db.Raw(`
		SELECT sa.show_id, a.name
		FROM show_artists sa
		JOIN artists a ON a.id = sa.artist_id
		WHERE sa.show_id IN ?
		ORDER BY sa.show_id, sa.position
	`, showIDs).Scan(&artistRows).Error
	if err != nil {
		return nil, fmt.Errorf("failed to get show artists: %w", err)
	}
	artistMap := make(map[uint][]string)
	for _, ar := range artistRows {
		artistMap[ar.ShowID] = append(artistMap[ar.ShowID], ar.Name)
	}
	return artistMap, nil
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

	// venue_id ASC is the repo's primary-venue pick (see the pv lateral in
	// show.go) — keep it so a multi-venue show names the same venue here as
	// on its show page.
	mostAnticipatedFromSQL = `FROM shows s
		LEFT JOIN LATERAL (
			SELECT iv.name, iv.slug, iv.city
			FROM show_venues sv
			JOIN venues iv ON iv.id = sv.venue_id
			WHERE sv.show_id = s.id
			ORDER BY sv.venue_id ASC
			LIMIT 1
		) v ON TRUE`

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

// GetMostAnticipatedShows returns the mode-discriminated most-anticipated
// module: upcoming approved shows with saves >= the floor, ranked by save
// count (ties by soonest date, then id). When fewer than
// mostAnticipatedMinQualifying shows clear the floor, it returns
// soonest-upcoming fallback mode instead — ALL upcoming approved shows
// date-ordered with SaveCount nil on every row (fail-closed: sub-floor
// counts never leak into a rendered payload).
func (s *ChartsService) GetMostAnticipatedShows(limit int) (*contracts.MostAnticipatedShows, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	startOfToday := time.Now().UTC().Truncate(24 * time.Hour)

	type showRow struct {
		ShowID    uint      `gorm:"column:show_id"`
		Title     string    `gorm:"column:title"`
		Slug      string    `gorm:"column:slug"`
		Date      time.Time `gorm:"column:event_date"`
		VenueName string    `gorm:"column:venue_name"`
		VenueSlug string    `gorm:"column:venue_slug"`
		City      string    `gorm:"column:city"`
		SaveCount int       `gorm:"column:save_count"`
	}

	// Probe past the caller's limit so a small page size can't force fallback:
	// the mode is a statement about how many shows QUALIFY, not about how many
	// the caller asked to see. Rows are sliced back to limit after the check.
	probeLimit := limit
	if probeLimit < mostAnticipatedMinQualifying {
		probeLimit = mostAnticipatedMinQualifying
	}

	var ranked []showRow
	err := s.db.Raw(`
		SELECT`+mostAnticipatedColumnsSQL+`,
			COUNT(ub.id) AS save_count
		`+mostAnticipatedFromSQL+`
		LEFT JOIN user_bookmarks ub ON ub.entity_id = s.id
			AND ub.entity_type = ?
			AND ub.action = ?
		`+mostAnticipatedEligibilitySQL+`
		GROUP BY s.id, v.name, v.slug, v.city
		HAVING COUNT(ub.id) >= ?
		ORDER BY save_count DESC, s.event_date ASC, s.id ASC
		LIMIT ?
	`, engagementm.BookmarkEntityShow, engagementm.BookmarkActionSave,
		catalogm.ShowStatusApproved, startOfToday, mostAnticipatedSaveFloor, probeLimit).Scan(&ranked).Error
	if err != nil {
		return nil, fmt.Errorf("failed to get most-anticipated shows: %w", err)
	}

	mode := contracts.MostAnticipatedModeRanked
	rows := ranked
	if len(rows) > limit {
		rows = rows[:limit]
	}
	if len(ranked) < mostAnticipatedMinQualifying {
		mode = contracts.MostAnticipatedModeSoonestUpcoming
		rows = nil
		err := s.db.Raw(`
			SELECT`+mostAnticipatedColumnsSQL+`
			`+mostAnticipatedFromSQL+`
			`+mostAnticipatedEligibilitySQL+`
			ORDER BY s.event_date ASC, s.id ASC
			LIMIT ?
		`, catalogm.ShowStatusApproved, startOfToday, limit).Scan(&rows).Error
		if err != nil {
			return nil, fmt.Errorf("failed to get soonest-upcoming fallback shows: %w", err)
		}
	}

	result := &contracts.MostAnticipatedShows{
		Mode:  mode,
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

// chartWindowStart returns the inclusive lower bound for a chart window, or
// nil for all-time. Defaulting for empty/unknown values is owned by
// ChartWindow.OrDefault so handler and service can't drift apart.
func chartWindowStart(window contracts.ChartWindow, now time.Time) *time.Time {
	days := 0
	switch window.OrDefault() {
	case contracts.ChartWindowMonth:
		days = 30
	case contracts.ChartWindowAllTime:
		return nil
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
	return &t
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
func appendChartShowWindow(query string, args []any, now time.Time, start *time.Time) (string, []any) {
	query += `
			AND s.is_cancelled = FALSE
			AND s.event_date <= ?`
	args = append(args, now)
	if start != nil {
		query += `
			AND s.event_date >= ?`
		args = append(args, *start)
	}
	return query, args
}

// GetMostActiveArtists returns artists ranked by approved, non-cancelled
// shows played within the window (see appendChartShowWindow for the exact
// eligibility semantics). Headline share uses headlineSlotPredicate. Artists
// with zero shows in the window are never returned.
func (s *ChartsService) GetMostActiveArtists(window contracts.ChartWindow, limit int) ([]contracts.MostActiveArtist, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	now := time.Now().UTC()
	start := chartWindowStart(window, now)

	type artistRow struct {
		ArtistID      uint   `gorm:"column:artist_id"`
		Name          string `gorm:"column:name"`
		Slug          string `gorm:"column:slug"`
		City          string `gorm:"column:city"`
		State         string `gorm:"column:state"`
		ShowCount     int    `gorm:"column:show_count"`
		HeadlineCount int    `gorm:"column:headline_count"`
	}

	query := `
		SELECT
			a.id AS artist_id,
			a.name,
			COALESCE(a.slug, '') AS slug,
			COALESCE(a.city, '') AS city,
			COALESCE(a.state, '') AS state,
			COUNT(*) AS show_count,
			COALESCE(SUM(CASE WHEN ` + headlineSlotPredicate + ` THEN 1 ELSE 0 END), 0) AS headline_count
		FROM show_artists sa
		JOIN artists a ON a.id = sa.artist_id
		JOIN shows s ON s.id = sa.show_id
		WHERE s.status = ?`
	args := []any{catalogm.ShowStatusApproved}
	query, args = appendChartShowWindow(query, args, now, start)
	query += `
		GROUP BY a.id, a.name, a.slug, a.city, a.state
		ORDER BY show_count DESC, a.name ASC, a.id ASC
		LIMIT ?`
	args = append(args, limit)

	var rows []artistRow
	if err := s.db.Raw(query, args...).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("failed to get most-active artists: %w", err)
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
		}
		artistIDs[i] = r.ArtistID
	}

	// Enrich with each artist's most recent show in the window (one query).
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
		lastQuery, lastArgs = appendChartShowWindow(lastQuery, lastArgs, now, start)
		// s.id and v.name tiebreaks keep the picked row deterministic when an
		// artist plays two shows on one date or a show has multiple venue links.
		lastQuery += `
			ORDER BY sa.artist_id, s.event_date DESC, s.id DESC, v.name ASC`

		var lastRows []lastShowRow
		if err := s.db.Raw(lastQuery, lastArgs...).Scan(&lastRows).Error; err != nil {
			return nil, fmt.Errorf("failed to get last shows for most-active artists: %w", err)
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

	return results, nil
}

// GetBusiestVenues returns venues ranked by approved, non-cancelled shows
// HOSTED (past tense) within the window — distinct from GetActiveVenues,
// which scores venues by upcoming shows + follows. Venues with zero shows in
// the window are never returned.
func (s *ChartsService) GetBusiestVenues(window contracts.ChartWindow, limit int) ([]contracts.BusiestVenue, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	now := time.Now().UTC()
	start := chartWindowStart(window, now)

	type venueRow struct {
		VenueID   uint   `gorm:"column:venue_id"`
		Name      string `gorm:"column:name"`
		Slug      string `gorm:"column:slug"`
		City      string `gorm:"column:city"`
		State     string `gorm:"column:state"`
		ShowCount int    `gorm:"column:show_count"`
	}

	query := `
		SELECT
			v.id AS venue_id,
			v.name,
			COALESCE(v.slug, '') AS slug,
			COALESCE(v.city, '') AS city,
			COALESCE(v.state, '') AS state,
			COUNT(*) AS show_count
		FROM show_venues sv
		JOIN venues v ON v.id = sv.venue_id
		JOIN shows s ON s.id = sv.show_id
		WHERE s.status = ?`
	// COUNT(*) == COUNT(DISTINCT s.id) here: show_venues' composite PK
	// (show_id, venue_id) guarantees one row per show within a venue group.
	args := []any{catalogm.ShowStatusApproved}
	query, args = appendChartShowWindow(query, args, now, start)
	query += `
		GROUP BY v.id, v.name, v.slug, v.city, v.state
		ORDER BY show_count DESC, v.name ASC, v.id ASC
		LIMIT ?`
	args = append(args, limit)

	var rows []venueRow
	if err := s.db.Raw(query, args...).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("failed to get busiest venues: %w", err)
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
		}
	}
	return results, nil
}

// GetOpenersToWatch returns artists ranked by support slots played within the
// window — slots that are NOT headline slots (headline = set_type 'headliner'
// OR position 0, the shared predicate). Artists with ANY headline slot in the
// window are excluded entirely: this chart surfaces artists who are always on
// the bill but never top it. Cancelled and future shows never count.
func (s *ChartsService) GetOpenersToWatch(window contracts.ChartWindow, limit int) ([]contracts.OpenerToWatch, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	now := time.Now().UTC()
	start := chartWindowStart(window, now)

	type openerRow struct {
		ArtistID         uint   `gorm:"column:artist_id"`
		Name             string `gorm:"column:name"`
		Slug             string `gorm:"column:slug"`
		City             string `gorm:"column:city"`
		State            string `gorm:"column:state"`
		SupportSlotCount int    `gorm:"column:support_slot_count"`
	}

	// One pass: group every in-window slot per artist, keep only groups with
	// ZERO headline slots (HAVING) — so COUNT(*) is exactly the support-slot
	// count, and "never headlines" is judged over the same window being
	// ranked. The CASE form also counts NULL set_type rows as support,
	// matching GetMostActiveArtists' NULL semantics.
	query := `
		SELECT
			a.id AS artist_id,
			a.name,
			COALESCE(a.slug, '') AS slug,
			COALESCE(a.city, '') AS city,
			COALESCE(a.state, '') AS state,
			COUNT(*) AS support_slot_count
		FROM show_artists sa
		JOIN artists a ON a.id = sa.artist_id
		JOIN shows s ON s.id = sa.show_id
		WHERE s.status = ?`
	args := []any{catalogm.ShowStatusApproved}
	query, args = appendChartShowWindow(query, args, now, start)
	query += `
		GROUP BY a.id, a.name, a.slug, a.city, a.state
		HAVING SUM(CASE WHEN ` + headlineSlotPredicate + ` THEN 1 ELSE 0 END) = 0
		ORDER BY support_slot_count DESC, a.name ASC, a.id ASC
		LIMIT ?`
	args = append(args, limit)

	var rows []openerRow
	if err := s.db.Raw(query, args...).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("failed to get openers to watch: %w", err)
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
		}
	}
	return results, nil
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
// with zero in-window plays are never returned.
func (s *ChartsService) GetOnTheRadioArtists(window contracts.ChartWindow, limit int) ([]contracts.OnTheRadioArtist, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	now := time.Now().UTC()
	start := chartWindowStart(window, now)

	type radioRow struct {
		ArtistID     uint   `gorm:"column:artist_id"`
		Name         string `gorm:"column:name"`
		Slug         string `gorm:"column:slug"`
		City         string `gorm:"column:city"`
		State        string `gorm:"column:state"`
		PlayCount    int    `gorm:"column:play_count"`
		StationCount int    `gorm:"column:station_count"`
		IsNew        bool   `gorm:"column:is_new"`
	}

	// GROUP BY a.id alone: the artists PK makes the selected columns
	// functionally dependent, so the aggregate hashes an 8-byte key instead of
	// the full text tuple. The sibling show-chart modules predate this and
	// enumerate every selected column — both forms are correct; prefer this
	// one, it matters more here (radio_plays outgrows show_artists by orders
	// of magnitude). The aired pair (station-local date bound + air-window
	// gate) is the shared radio.go definition — see stationLocalAiredDateBoundSQL.
	query := `
		SELECT
			a.id AS artist_id,
			a.name,
			COALESCE(a.slug, '') AS slug,
			COALESCE(a.city, '') AS city,
			COALESCE(a.state, '') AS state,
			COUNT(*) AS play_count,
			COUNT(DISTINCT ` + broadcasterKeySQL + `) AS station_count,
			BOOL_OR(rp.is_new) AS is_new
		FROM radio_plays rp
		JOIN radio_episodes re ON re.id = rp.episode_id
		JOIN radio_shows rsh ON rsh.id = re.show_id
		JOIN radio_stations rst ON rst.id = rsh.station_id
		JOIN artists a ON a.id = rp.artist_id
		` + stationTimezoneJoinSQL + `
		WHERE ` + pseudoArtistExclusionSQL + `
			AND ` + stationLocalAiredDateBoundSQL("re.") + `
			AND ` + airedEpisodeVisibleSQL("re.")
	args := []any{now, now}
	if start != nil {
		query += `
			AND re.air_date >= ?`
		args = append(args, start.Format("2006-01-02"))
	}
	query += `
		GROUP BY a.id
		ORDER BY play_count DESC, a.name ASC, a.id ASC
		LIMIT ?`
	args = append(args, limit)

	var rows []radioRow
	if err := s.db.Raw(query, args...).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("failed to get on-the-radio artists: %w", err)
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
		}
	}
	return results, nil
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
	`, engagementm.BookmarkEntityRelease, engagementm.BookmarkActionBookmark, thirtyDaysAgo, limit).Scan(&rows).Error
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
	if len(releaseIDs) > 0 {
		type artistNameRow struct {
			ReleaseID uint   `gorm:"column:release_id"`
			Name      string `gorm:"column:name"`
		}
		var artistRows []artistNameRow
		err := s.db.Raw(`
			SELECT ar.release_id, a.name
			FROM artist_releases ar
			JOIN artists a ON a.id = ar.artist_id
			WHERE ar.release_id IN ?
			ORDER BY ar.release_id, ar.position
		`, releaseIDs).Scan(&artistRows).Error
		if err != nil {
			return nil, fmt.Errorf("failed to get release artists: %w", err)
		}

		// Build map of release_id -> artist names
		artistMap := make(map[uint][]string)
		for _, ar := range artistRows {
			artistMap[ar.ReleaseID] = append(artistMap[ar.ReleaseID], ar.Name)
		}

		// Assign to results
		for i := range results {
			if names, ok := artistMap[results[i].ReleaseID]; ok {
				results[i].ArtistNames = names
			}
		}
	}

	return results, nil
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
