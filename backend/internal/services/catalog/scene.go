package catalog

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"gorm.io/gorm"

	"psychic-homily-backend/db"
	apperrors "psychic-homily-backend/internal/errors"
	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/services/geo"
	"psychic-homily-backend/internal/services/shared"
)

// SceneService handles computed city-level aggregations for "scene" pages.
// No new tables — all data is derived from existing venue, show, and artist tables.
type SceneService struct {
	db *gorm.DB
	// geocoder resolves a (city, state) to its centroid coordinates from the
	// embedded offline dataset — the same geocoder VenueService/ShowService hold
	// (PSY-985/PSY-981). Stateless, so sharing geo.Default() is safe.
	//
	// COUPLING (PSY-1255 step C): metro keying relies on this geocoder and the
	// venues.metro column having been written by the SAME geo build — scopeFor
	// (here) and the principal-city/slug lookup (geo.MetroPrincipalByCBSA, a
	// package fn on geo.Default()) must agree, or list-grouping and per-scene
	// scoping could desync. In production both are geo.Default(); a test that
	// injects a different stub geocoder would break that invariant.
	geocoder geo.Geocoder
}

// NewSceneService creates a new scene service.
func NewSceneService(database *gorm.DB) *SceneService {
	if database == nil {
		database = db.GetDB()
	}
	return &SceneService{db: database, geocoder: geo.Default()}
}

// Thresholds for a city to qualify as a "scene".
const (
	sceneMinVenues = 2
	sceneMinShows  = 3

	// sceneGraphRosterLimit caps the scene graph node set to the top-N metro-roster
	// artists by approved show activity (PSY-1277). Mirrors stationGraphDefaultNodeLimit
	// (PSY-1081) — same graph-density budget, not a silent cap: MetroRosterTotal +
	// RosterTruncated surface the full roster size.
	sceneGraphRosterLimit = 75

	// sceneThisWeekDays is the "happening this week" window (PSY-1309): the
	// ≤7-day slice of a scene's upcoming shows that drives the Atlas globe's
	// pulse treatment and the preview panel's "This week" row.
	sceneThisWeekDays = 7

	// sceneRosterActiveOrderBy is the canonical active-first roster ordering,
	// shared by GetActiveArtists (the paginated roster) and GetRepresentativeEmbed
	// (the full-roster embed pick) so the "representative" embed matches the top of
	// the displayed roster. a.id is the final UNIQUE tiebreak — without it a tie on
	// the first three keys is nondeterministic under LIMIT/pagination, which would
	// let the embed (a single LIMIT 1 row) flip between homonym bands (PSY-1294
	// review). Columns are aliases/`a.`-qualified, so both queries must SELECT
	// is_active + show_count and alias the artists table `a`.
	sceneRosterActiveOrderBy = "is_active DESC, show_count DESC, a.name ASC, a.id ASC"
)

// usCountry is the country passed to the geocoder when resolving a scene's
// metro. Scenes only key on a Census CBSA for US places; everything else falls
// back to (city, state) — see scopeFor.
const usCountry = "US"

// sceneScope captures how a scene is keyed for querying (PSY-1255 step C). A US
// place that resolves to a Census CBSA is keyed by that metro code, so the
// scene's roster = bands BASED in the whole metro (boroughs/suburbs rolled up),
// not just touring acts who played one city's venue (the PSY-1233 played-here
// model this replaces). A non-US / no-CBSA place falls back to the literal
// (city, state) so the Atlas globe can still grow globally.
//
// city/state always hold the scene's DISPLAY identity: the metro's principal
// city for a metro scene, the literal city for a fallback scene.
type sceneScope struct {
	metro string // CBSA code; "" => (city, state) fallback scene
	city  string
	state string
}

func (sc sceneScope) isMetro() bool { return sc.metro != "" }

// scopeFor resolves a scene's (principal) city/state to its query scope. A US
// city that pins a CBSA becomes a metro scope; anything else is a city/state
// fallback. ResolveMetro already refuses an unpinned ambiguous name, so a
// wrong-namesake metro is never selected.
func (s *SceneService) scopeFor(city, state string) sceneScope {
	if s.geocoder != nil {
		if m, ok := s.geocoder.ResolveMetro(city, state, usCountry); ok {
			return sceneScope{metro: m.CBSACode, city: city, state: state}
		}
	}
	return sceneScope{city: city, state: state}
}

// venuePredicate returns a WHERE fragment (on the given venues alias) selecting
// the scene's venues, plus its positional bind args. Splice it as the FIRST
// predicate of a raw WHERE so its args lead the arg list. The fallback branch is
// case-insensitive + trimmed to match BOTH the ListScenes fallback grouping key
// and artistPredicate — otherwise a mixed-case no-CBSA scene could list but then
// 404 on its detail page (the matching MUST agree across list/detail/existence).
func (sc sceneScope) venuePredicate(alias string) (string, []any) {
	if sc.isMetro() {
		return alias + ".metro = ?", []any{sc.metro}
	}
	return "LOWER(TRIM(" + alias + ".city)) = LOWER(TRIM(?)) AND LOWER(TRIM(" + alias + ".state)) = LOWER(TRIM(?))",
		[]any{sc.city, sc.state}
}

// artistPredicate returns a WHERE fragment (on the given artists alias) selecting
// the scene's roster — bands whose HOME metro is the scene's CBSA, or whose home
// city/state match for a fallback scene — plus its positional bind args. The
// city/state branch is case-insensitive + trimmed; an artist with a NULL/blank
// home city is excluded (we can't claim they're based here). For a metro scene,
// roster membership is primarily artists.metro = CBSA (PSY-1255); PSY-1237 also
// matches NULL-metro artists whose home (city, state) is any CBSA member place
// from the geo dataset (Brooklyn↔New York City, Cambridge↔Boston, etc.).
func (s *SceneService) artistPredicate(scope sceneScope, alias string) (string, []any) {
	if scope.isMetro() {
		pred := alias + ".metro = ?"
		args := []any{scope.metro}
		if members, ok := geo.MetroMemberPlaces(scope.metro); ok && len(members) > 0 {
			orParts := make([]string, 0, len(members))
			for _, m := range members {
				orParts = append(orParts,
					"(LOWER(TRIM("+alias+".city)) = LOWER(TRIM(?)) AND LOWER(TRIM("+alias+".state)) = LOWER(TRIM(?)))")
				args = append(args, m.City, m.State)
			}
			nullMetro := alias + ".metro IS NULL AND " + alias + ".city IS NOT NULL AND " + alias + ".city <> '' AND " +
				alias + ".state IS NOT NULL AND " + alias + ".state <> ''"
			pred = "(" + pred + " OR (" + nullMetro + " AND (" + strings.Join(orParts, " OR ") + ")))"
		}
		return pred, args
	}
	return "LOWER(TRIM(" + alias + ".city)) = LOWER(TRIM(?)) AND LOWER(TRIM(" + alias + ".state)) = LOWER(TRIM(?))",
		[]any{scope.city, scope.state}
}

// verifiedVenueCount counts the scene's verified venues — the existence gate
// shared by GetSceneDetail / GetActiveArtists / GetSceneGraph.
func (s *SceneService) verifiedVenueCount(scope sceneScope) (int64, error) {
	q := s.db.Model(&catalogm.Venue{}).Where("verified = true")
	if scope.isMetro() {
		q = q.Where("metro = ?", scope.metro)
	} else {
		q = q.Where("LOWER(TRIM(city)) = LOWER(TRIM(?)) AND LOWER(TRIM(state)) = LOWER(TRIM(?))", scope.city, scope.state)
	}
	var n int64
	if err := q.Count(&n).Error; err != nil {
		return 0, err
	}
	return n, nil
}

// sceneGenreCounts returns each scene's genre distribution (per-genre distinct
// artist counts) keyed by the SAME key ListScenes groups on — the artist's CBSA
// metro, or lower/trimmed city|state for a non-metro roster — in one grouped query
// (no per-scene N+1). NULLIF folds an empty-string metro into the city|state
// fallback so its key matches the Go-side `g.Metro == ""` branch. Artists with
// neither a metro nor a home city+state are excluded (unattributable to a scene).
// The caller rolls these up to genre families (dominantGenreFamily) for the Atlas
// dot tint (PSY-1315).
func (s *SceneService) sceneGenreCounts() (map[string][]contracts.GenreCount, error) {
	type genreRow struct {
		SceneKey string `gorm:"column:scene_key"`
		Slug     string `gorm:"column:slug"`
		Count    int    `gorm:"column:count"`
	}
	var rows []genreRow
	err := s.db.Raw(`
		SELECT COALESCE(NULLIF(a.metro, ''), LOWER(TRIM(a.city)) || '|' || LOWER(TRIM(a.state))) AS scene_key,
		       t.slug AS slug,
		       COUNT(DISTINCT a.id) AS count
		FROM artists a
		JOIN entity_tags et ON et.entity_type = 'artist' AND et.entity_id = a.id
		JOIN tags t ON t.id = et.tag_id AND t.category = 'genre'
		WHERE (a.metro IS NOT NULL AND a.metro <> '')
		   OR (a.city IS NOT NULL AND a.city <> '' AND a.state IS NOT NULL AND a.state <> '')
		GROUP BY scene_key, t.slug
	`).Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("failed to aggregate scene genres: %w", err)
	}

	out := make(map[string][]contracts.GenreCount)
	for _, r := range rows {
		out[r.SceneKey] = append(out[r.SceneKey], contracts.GenreCount{Slug: r.Slug, Count: r.Count})
	}
	return out, nil
}

// ListScenes returns the metros — and non-US / no-CBSA fallback cities — that
// meet scene thresholds: 2+ verified venues AND 3+ approved shows (past or
// upcoming). Verified venues roll up to their Census CBSA (PSY-1255 step C), so
// a metro's boroughs/suburbs form ONE scene displayed under the metro's
// principal city; a venue with no CBSA groups by its literal (city, state).
func (s *SceneService) ListScenes() ([]*contracts.SceneListResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	now := time.Now().UTC()

	type groupRow struct {
		Metro         string `gorm:"column:metro"`
		City          string `gorm:"column:city"`
		State         string `gorm:"column:state"`
		VenueCount    int    `gorm:"column:venue_count"`
		ShowCount     int    `gorm:"column:show_count"`
		UpcomingCount int    `gorm:"column:upcoming_count"`
		ThisWeekCount int    `gorm:"column:this_week_count"`
	}

	// Group verified venues by CBSA metro (or by (city,state) when the venue has
	// no metro), counting distinct venues + approved shows + the upcoming subset
	// in ONE pass — the FILTER aggregate replaces the former per-scene N+1 query.
	// COALESCE(MAX(v.metro), '') is the group's CBSA (all rows in a metro group
	// share it; '' for a fallback group); MIN(city)/MIN(state) is the literal
	// city/state of a fallback group (a metro group displays its principal city
	// instead, so its MIN city is unused).
	// this_week_count is the ≤sceneThisWeekDays slice of the upcoming set
	// (PSY-1309): it drives the Atlas globe's "happening this week" pulse, so it
	// must share the scene scoping of the other counts — one more FILTER
	// aggregate in the same pass, not a new query.
	weekAhead := now.AddDate(0, 0, sceneThisWeekDays)
	var groups []groupRow
	err := s.db.Raw(`
		SELECT COALESCE(MAX(v.metro), '') AS metro,
		       MIN(v.city)  AS city,
		       MIN(v.state) AS state,
		       COUNT(DISTINCT v.id) AS venue_count,
		       COUNT(DISTINCT s.id) AS show_count,
		       COUNT(DISTINCT s.id) FILTER (WHERE s.event_date >= ?) AS upcoming_count,
		       COUNT(DISTINCT s.id) FILTER (WHERE s.event_date >= ? AND s.event_date < ?) AS this_week_count
		FROM venues v
		LEFT JOIN show_venues sv ON sv.venue_id = v.id
		LEFT JOIN shows s ON s.id = sv.show_id AND s.status = ?
		WHERE v.verified = true
		  AND v.city IS NOT NULL AND v.city != ''
		  AND v.state IS NOT NULL AND v.state != ''
		GROUP BY COALESCE(v.metro, LOWER(TRIM(v.city)) || '|' || LOWER(TRIM(v.state)))
		HAVING COUNT(DISTINCT v.id) >= ?
		   AND COUNT(DISTINCT s.id) >= ?
	`, now, now, weekAhead, catalogm.ShowStatusApproved, sceneMinVenues, sceneMinShows).Scan(&groups).Error
	if err != nil {
		return nil, fmt.Errorf("failed to list scenes: %w", err)
	}

	// Dominant genre family per scene for the Atlas dot tint (PSY-1315), in ONE
	// batched query — group the genre distribution of artists BASED in each scene
	// by the SAME key ListScenes groups venues by (a.metro CBSA, or city|state for a
	// fallback scene), so there is no per-scene N+1. This metro-keyed roster is a
	// coarser match than the detail page's GetSceneGenreDistribution (which also
	// folds in PSY-1237 NULL-metro member-place artists), which is fine for a dot
	// color. Aggregation into families + the confident-dominance test happen in Go
	// (dominantGenreFamily) so the SQL stays a plain grouped scan.
	genresByScene, err := s.sceneGenreCounts()
	if err != nil {
		return nil, err
	}

	results := make([]*contracts.SceneListResponse, 0, len(groups))
	for i := range groups {
		g := &groups[i]

		// Display identity + map point. A metro scene shows under its principal
		// (highest-population) city, placed at that city's centroid. A fallback
		// scene (or a metro whose principal we somehow can't resolve) uses its
		// literal city/state geocoded the SAME way as GetShowCities (PSY-985/981),
		// so a scene plots at the same point here and on the shows-by-city map. A
		// geocoder miss leaves coords nil: the scene still lists, just unplaceable.
		city, state := g.City, g.State
		var lat, lng *float64
		if g.Metro != "" {
			if mp, ok := geo.MetroPrincipalByCBSA(g.Metro); ok {
				city, state = mp.City, mp.State
				latV, lngV := mp.Latitude, mp.Longitude
				lat, lng = &latV, &lngV
			}
		}
		if lat == nil && s.geocoder != nil {
			lat, lng, _ = geo.LookupPointers(s.geocoder, city, state, "")
		}

		// Match the genre aggregation on the SAME key the venue groups use: the CBSA
		// for a metro scene, else the lower/trimmed city|state of a fallback scene.
		sceneKey := g.Metro
		if sceneKey == "" {
			sceneKey = strings.ToLower(strings.TrimSpace(g.City)) + "|" + strings.ToLower(strings.TrimSpace(g.State))
		}

		results = append(results, &contracts.SceneListResponse{
			City:              city,
			State:             state,
			Slug:              buildSceneSlug(city, state),
			VenueCount:        g.VenueCount,
			UpcomingShowCount: g.UpcomingCount,
			TotalShowCount:    g.ShowCount,
			ShowsThisWeek:     g.ThisWeekCount,
			Latitude:          lat,
			Longitude:         lng,
			DominantGenre:     dominantGenreFamily(genresByScene[sceneKey]),
		})
	}

	// Sort by total show count descending, then upcoming shows as tiebreaker.
	// Simple insertion sort is fine for a small number of scenes.
	for i := 1; i < len(results); i++ {
		for j := i; j > 0; j-- {
			if results[j].TotalShowCount > results[j-1].TotalShowCount ||
				(results[j].TotalShowCount == results[j-1].TotalShowCount &&
					results[j].UpcomingShowCount > results[j-1].UpcomingShowCount) {
				results[j], results[j-1] = results[j-1], results[j]
			} else {
				break
			}
		}
	}

	return results, nil
}

// GetSceneDetail returns computed aggregation stats and pulse for a scene,
// addressed by its (principal) city/state. Venue/show stats span the scene's
// whole metro (or its literal city for a no-CBSA fallback scene); the artist
// roster + "new artists" count bands BASED in the metro, regardless of where
// they've played (PSY-1255 step C).
func (s *SceneService) GetSceneDetail(city, state string) (*contracts.SceneDetailResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	now := time.Now().UTC()
	scope := s.scopeFor(city, state)
	vp, vargs := scope.venuePredicate("v")
	ap, aargs := s.artistPredicate(scope, "a2")
	// venueArgs returns the venue-predicate args (copied to avoid append aliasing
	// across queries) followed by extra args, in placeholder order.
	venueArgs := func(extra ...any) []any { return append(append([]any{}, vargs...), extra...) }

	// Venue count (verified only) — gates scene existence.
	venueCount, err := s.verifiedVenueCount(scope)
	if err != nil {
		return nil, fmt.Errorf("failed to count venues: %w", err)
	}
	if venueCount < sceneMinVenues {
		return nil, apperrors.ErrSceneNotFound(fmt.Sprintf("scene not found: %s, %s", city, state))
	}

	// Upcoming show count (metro-wide)
	var upcomingShowCount int64
	if err := s.db.Raw(`
		SELECT COUNT(DISTINCT s.id)
		FROM shows s
		JOIN show_venues sv ON sv.show_id = s.id
		JOIN venues v ON v.id = sv.venue_id
		WHERE `+vp+`
		  AND s.status = ?
		  AND s.event_date >= ?
	`, venueArgs(catalogm.ShowStatusApproved, now)...).Scan(&upcomingShowCount).Error; err != nil {
		return nil, fmt.Errorf("failed to count upcoming shows: %w", err)
	}

	// Artist count: bands BASED in the metro (the roster size), regardless of
	// whether they've played a local show — this is the scene's headline figure
	// under the new "based-in" model.
	var artistCount int64
	if err := s.db.Raw(`SELECT COUNT(*) FROM artists a2 WHERE `+ap, aargs...).Scan(&artistCount).Error; err != nil {
		return nil, fmt.Errorf("failed to count artists: %w", err)
	}

	// Festival count: festivals held in the scene's metro (PSY-1278) — the
	// denormalized festivals.metro column rolls member-city festivals up (a
	// St. Paul festival counts toward the Minneapolis-principal Twin Cities
	// scene). A no-CBSA fallback scene matches its literal city/state, same as
	// the venue/artist predicates.
	festivalQ := s.db.Model(&catalogm.Festival{})
	if scope.isMetro() {
		festivalQ = festivalQ.Where("metro = ?", scope.metro)
	} else {
		festivalQ = festivalQ.Where("LOWER(TRIM(city)) = LOWER(TRIM(?)) AND LOWER(TRIM(state)) = LOWER(TRIM(?))", scope.city, scope.state)
	}
	var festivalCount int64
	if err := festivalQ.Count(&festivalCount).Error; err != nil {
		return nil, fmt.Errorf("failed to count festivals: %w", err)
	}

	// ── Pulse computations ──

	// Current month boundaries
	thisMonthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	nextMonthStart := thisMonthStart.AddDate(0, 1, 0)
	prevMonthStart := thisMonthStart.AddDate(0, -1, 0)

	// Shows this month
	var showsThisMonth int64
	s.db.Raw(`
		SELECT COUNT(DISTINCT s.id)
		FROM shows s
		JOIN show_venues sv ON sv.show_id = s.id
		JOIN venues v ON v.id = sv.venue_id
		WHERE `+vp+`
		  AND s.status = ?
		  AND s.event_date >= ? AND s.event_date < ?
	`, venueArgs(catalogm.ShowStatusApproved, thisMonthStart, nextMonthStart)...).Scan(&showsThisMonth)

	// Shows previous month
	var showsPrevMonth int64
	s.db.Raw(`
		SELECT COUNT(DISTINCT s.id)
		FROM shows s
		JOIN show_venues sv ON sv.show_id = s.id
		JOIN venues v ON v.id = sv.venue_id
		WHERE `+vp+`
		  AND s.status = ?
		  AND s.event_date >= ? AND s.event_date < ?
	`, venueArgs(catalogm.ShowStatusApproved, prevMonthStart, thisMonthStart)...).Scan(&showsPrevMonth)

	// Trend string
	diff := int(showsThisMonth) - int(showsPrevMonth)
	showsTrend := "0"
	if diff > 0 {
		showsTrend = fmt.Sprintf("+%d", diff)
	} else if diff < 0 {
		showsTrend = fmt.Sprintf("%d", diff)
	}

	// New artists in last 30 days: metro-based bands whose FIRST approved show
	// ANYWHERE (not just locally) was in the last 30 days — the "newly gigging
	// local band" signal under the based-in model (PSY-1255 step C).
	thirtyDaysAgo := now.AddDate(0, 0, -30)
	naArgs := append([]any{catalogm.ShowStatusApproved}, aargs...)
	naArgs = append(naArgs, thirtyDaysAgo)
	var newArtists30d int64
	s.db.Raw(`
		SELECT COUNT(*)
		FROM (
			SELECT sa.artist_id, MIN(s.event_date) AS first_show
			FROM show_artists sa
			JOIN shows s ON s.id = sa.show_id
			WHERE s.status = ?
			  AND sa.artist_id IN (SELECT a2.id FROM artists a2 WHERE `+ap+`)
			GROUP BY sa.artist_id
			HAVING MIN(s.event_date) >= ?
		) AS new_artists
	`, naArgs...).Scan(&newArtists30d)

	// Active venues this month: venues with at least 1 approved show this month
	var activeVenuesThisMonth int64
	s.db.Raw(`
		SELECT COUNT(DISTINCT v.id)
		FROM venues v
		JOIN show_venues sv ON sv.venue_id = v.id
		JOIN shows s ON s.id = sv.show_id
		WHERE `+vp+`
		  AND s.status = ?
		  AND s.event_date >= ? AND s.event_date < ?
	`, venueArgs(catalogm.ShowStatusApproved, thisMonthStart, nextMonthStart)...).Scan(&activeVenuesThisMonth)

	// Shows by month: last 6 months (from 5 months ago through current month)
	showsByMonth := make([]int, 6)
	for i := 5; i >= 0; i-- {
		monthStart := thisMonthStart.AddDate(0, -i, 0)
		monthEnd := monthStart.AddDate(0, 1, 0)
		var count int64
		s.db.Raw(`
			SELECT COUNT(DISTINCT s.id)
			FROM shows s
			JOIN show_venues sv ON sv.show_id = s.id
			JOIN venues v ON v.id = sv.venue_id
			WHERE `+vp+`
			  AND s.status = ?
			  AND s.event_date >= ? AND s.event_date < ?
		`, venueArgs(catalogm.ShowStatusApproved, monthStart, monthEnd)...).Scan(&count)
		showsByMonth[5-i] = int(count)
	}

	return &contracts.SceneDetailResponse{
		City:        city,
		State:       state,
		Slug:        buildSceneSlug(city, state),
		Description: s.sceneDescription(buildSceneSlug(city, state)),
		Stats: contracts.SceneStats{
			VenueCount:        int(venueCount),
			ArtistCount:       int(artistCount),
			UpcomingShowCount: int(upcomingShowCount),
			FestivalCount:     int(festivalCount),
		},
		Pulse: contracts.ScenePulse{
			ShowsThisMonth:        int(showsThisMonth),
			ShowsPrevMonth:        int(showsPrevMonth),
			ShowsTrend:            showsTrend,
			NewArtists30d:         int(newArtists30d),
			ActiveVenuesThisMonth: int(activeVenuesThisMonth),
			ShowsByMonth:          showsByMonth,
		},
	}, nil
}

// GetSceneUpcomingShows returns the scene's next approved shows within
// windowDays, soonest first (id as the same-date tiebreak), capped at limit —
// the Atlas preview panel's "This week" row (PSY-1309). Scoped by the scene's
// venue predicate so a metro scene includes member-city shows (a Tempe show
// counts toward Phoenix) — the literal-city upcoming-shows endpoint can't do
// that. VenueName is the first venue on the bill (MIN by name: deterministic,
// and multi-venue shows are rare).
func (s *SceneService) GetSceneUpcomingShows(city, state string, windowDays, limit int) ([]contracts.SceneShowSummary, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	scope := s.scopeFor(city, state)
	if n, err := s.verifiedVenueCount(scope); err != nil {
		return nil, fmt.Errorf("failed to count venues: %w", err)
	} else if n < sceneMinVenues {
		return nil, apperrors.ErrSceneNotFound(fmt.Sprintf("scene not found: %s, %s", city, state))
	}

	vp, vargs := scope.venuePredicate("v")
	now := time.Now().UTC()
	windowEnd := now.AddDate(0, 0, windowDays)

	type showRow struct {
		ID        uint      `gorm:"column:id"`
		Slug      string    `gorm:"column:slug"`
		Title     string    `gorm:"column:title"`
		EventDate time.Time `gorm:"column:event_date"`
		VenueName string    `gorm:"column:venue_name"`
	}
	// Placeholder order: venue predicate, then status/window bounds.
	args := append(append([]any{}, vargs...), catalogm.ShowStatusApproved, now, windowEnd, limit)
	var rows []showRow
	if err := s.db.Raw(`
		SELECT s.id, COALESCE(s.slug, '') AS slug, s.title, s.event_date, MIN(v.name) AS venue_name
		FROM shows s
		JOIN show_venues sv ON sv.show_id = s.id
		JOIN venues v ON v.id = sv.venue_id
		WHERE `+vp+`
		  AND s.status = ?
		  AND s.event_date >= ?
		  AND s.event_date < ?
		GROUP BY s.id, s.slug, s.title, s.event_date -- id is the PK; slug/title/date ride along
		ORDER BY s.event_date ASC, s.id ASC
		LIMIT ?
	`, args...).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("failed to get scene upcoming shows: %w", err)
	}

	// Bill artists per show, position order — the row's display-name source
	// when the title is empty (see SceneShowSummary.ArtistNames, PSY-1325).
	ids := make([]uint, len(rows))
	for i, r := range rows {
		ids[i] = r.ID
	}
	artistsByShow, err := shared.BatchResolveShowArtistNames(s.db, ids)
	if err != nil {
		return nil, fmt.Errorf("failed to get scene show artists: %w", err)
	}

	results := make([]contracts.SceneShowSummary, len(rows))
	for i, r := range rows {
		results[i] = contracts.SceneShowSummary{
			ID:          r.ID,
			Slug:        r.Slug,
			Title:       r.Title,
			EventDate:   r.EventDate.UTC().Format("2006-01-02"),
			VenueName:   r.VenueName,
			ArtistNames: artistsByShow[r.ID],
		}
	}
	return results, nil
}

// GetSceneNewArtistsSince returns bands based in the scene whose catalog row
// was created after `since` (up to `now`), newest first, capped at `limit`,
// plus the TOTAL number in the window — the "new bands based here" stream of
// the weekly scene digest (PSY-1342). The total lets the caller render a
// "+N more" affordance so the cap never SILENTLY drops bands: the digest
// advances its per-follow cursor to `now` after a send, so any band beyond the
// cap would otherwise fall before next cycle's window and be lost forever.
// Uses the same roster scope (artistPredicate) as GetActiveArtists, so the
// additions match the roster the user sees on the scene page. Created-at (not
// updated-at) is the "new to the scene" signal: stable, "new to the catalog,"
// where updated-at would re-surface an old band on any edit. Unlike
// GetSceneUpcomingShows this does NOT gate on venue count — a followed scene
// that temporarily dips below the venue threshold still has a real roster.
func (s *SceneService) GetSceneNewArtistsSince(city, state string, since, now time.Time, limit int) ([]contracts.SceneNewArtist, int, error) {
	if s.db == nil {
		return nil, 0, fmt.Errorf("database not initialized")
	}

	scope := s.scopeFor(city, state)
	ap, aargs := s.artistPredicate(scope, "a")

	// Total in the window (uncapped) so the caller can show "+N more".
	var total int64
	countArgs := append(append([]any{}, aargs...), since, now)
	if err := s.db.Raw(`
		SELECT COUNT(*) FROM artists a
		WHERE `+ap+` AND a.created_at > ? AND a.created_at <= ?
	`, countArgs...).Scan(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to count scene new artists: %w", err)
	}
	if total == 0 {
		return nil, 0, nil
	}

	type row struct {
		ID   uint   `gorm:"column:id"`
		Slug string `gorm:"column:slug"`
		Name string `gorm:"column:name"`
	}
	args := append(append([]any{}, aargs...), since, now, limit)
	var rows []row
	if err := s.db.Raw(`
		SELECT a.id, COALESCE(a.slug, '') AS slug, a.name
		FROM artists a
		WHERE `+ap+`
		  AND a.created_at > ?
		  AND a.created_at <= ?
		ORDER BY a.created_at DESC, a.id DESC
		LIMIT ?
	`, args...).Scan(&rows).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to get scene new artists: %w", err)
	}

	out := make([]contracts.SceneNewArtist, len(rows))
	for i, r := range rows {
		out[i] = contracts.SceneNewArtist{ID: r.ID, Slug: r.Slug, Name: r.Name}
	}
	return out, int(total), nil
}

// GetActiveArtists returns the scene's roster — bands BASED in the metro — with
// the ACTIVE ones sorted first, then by total approved show count, then name
// (PSY-1255 step C). "Active" = an upcoming approved show OR one within
// `periodDays`, played ANYWHERE (the default window is ~6 months; see the
// handler). Pre-step-C this returned only bands who'd played a LOCAL show in the
// window; membership is now metro residence, decoupled from where the band has
// gigged, so the full roster paginates here with is_active flagging the live ones.
func (s *SceneService) GetActiveArtists(city, state string, activeWindowDays, limit, offset int) ([]*contracts.SceneArtistResponse, int64, error) {
	if s.db == nil {
		return nil, 0, fmt.Errorf("database not initialized")
	}

	scope := s.scopeFor(city, state)
	if n, err := s.verifiedVenueCount(scope); err != nil {
		return nil, 0, fmt.Errorf("failed to count venues: %w", err)
	} else if n < sceneMinVenues {
		return nil, 0, apperrors.ErrSceneNotFound(fmt.Sprintf("scene not found: %s, %s", city, state))
	}

	ap, aargs := s.artistPredicate(scope, "a")
	activeCutoff := time.Now().UTC().AddDate(0, 0, -activeWindowDays)

	// Total roster size = every band based in the metro.
	var total int64
	if err := s.db.Raw(`SELECT COUNT(*) FROM artists a WHERE `+ap, aargs...).Scan(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to count scene artists: %w", err)
	}

	type artistRow struct {
		ID               uint    `gorm:"column:id"`
		Slug             *string `gorm:"column:slug"`
		Name             string  `gorm:"column:name"`
		City             *string `gorm:"column:city"`
		State            *string `gorm:"column:state"`
		ShowCount        int     `gorm:"column:show_count"`
		IsActive         bool    `gorm:"column:is_active"`
		BandcampEmbedURL *string `gorm:"column:bandcamp_embed_url"`
	}

	// Each roster artist's total approved show count + most-recent show date (any
	// venue) come from one grouped subquery; is_active = that latest show falls in
	// the active window (an upcoming show has event_date >= now >= cutoff, so it's
	// covered). Computing is_active in SQL keeps the active-first ordering correct
	// across pagination. Placeholder order: cutoff (SELECT), status (subquery),
	// roster predicate, then LIMIT/OFFSET.
	rowsArgs := append([]any{activeCutoff, catalogm.ShowStatusApproved}, aargs...)
	rowsArgs = append(rowsArgs, limit, offset)
	var rows []artistRow
	if err := s.db.Raw(`
		SELECT a.id, a.slug, a.name, a.city, a.state,
		       a.bandcamp_embed_url,
		       COALESCE(ss.show_count, 0) AS show_count,
		       COALESCE(ss.last_show >= ?, false) AS is_active
		FROM artists a
		LEFT JOIN (
			SELECT sa.artist_id,
			       COUNT(DISTINCT s.id) AS show_count,
			       MAX(s.event_date) AS last_show
			FROM show_artists sa
			JOIN shows s ON s.id = sa.show_id
			WHERE s.status = ?
			GROUP BY sa.artist_id
		) ss ON ss.artist_id = a.id
		WHERE `+ap+`
		ORDER BY `+sceneRosterActiveOrderBy+`
		LIMIT ? OFFSET ?
	`, rowsArgs...).Scan(&rows).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to get scene artists: %w", err)
	}

	results := make([]*contracts.SceneArtistResponse, len(rows))
	for i, r := range rows {
		slug := ""
		if r.Slug != nil {
			slug = *r.Slug
		}
		results[i] = &contracts.SceneArtistResponse{
			ID:               r.ID,
			Slug:             slug,
			Name:             r.Name,
			City:             r.City,
			State:            r.State,
			ShowCount:        r.ShowCount,
			IsActive:         r.IsActive,
			BandcampEmbedURL: r.BandcampEmbedURL,
		}
	}

	return results, total, nil
}

// GetRepresentativeEmbed returns the single band whose Bandcamp embed represents
// the scene — the top band with a non-null bandcamp_embed_url in the same
// active-first ordering as GetActiveArtists, computed over the FULL metro roster
// (PSY-1294). Returns nil when no band based here has an embed. This is the
// full-roster FALLBACK: the handler first looks for the embed in the fetched
// roster page (where the top embed-having band almost always is) and only calls
// this when the page has none but the roster is larger, so it decouples the
// /atlas preview's player from the fetched window without a query per preview.
func (s *SceneService) GetRepresentativeEmbed(city, state string, activeWindowDays int) (*contracts.SceneRepresentativeEmbed, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	scope := s.scopeFor(city, state)
	if n, err := s.verifiedVenueCount(scope); err != nil {
		return nil, fmt.Errorf("failed to count venues: %w", err)
	} else if n < sceneMinVenues {
		return nil, apperrors.ErrSceneNotFound(fmt.Sprintf("scene not found: %s, %s", city, state))
	}

	ap, aargs := s.artistPredicate(scope, "a")
	activeCutoff := time.Now().UTC().AddDate(0, 0, -activeWindowDays)

	type embedRow struct {
		Slug             *string `gorm:"column:slug"`
		Name             string  `gorm:"column:name"`
		BandcampEmbedURL string  `gorm:"column:bandcamp_embed_url"`
	}

	// Same active-first ordering as GetActiveArtists (shared sceneRosterActiveOrderBy
	// const), restricted to bands that HAVE an embed, top 1. The is_active and
	// show_count SELECT aliases are REQUIRED by that ORDER BY and must stay even
	// though embedRow ignores them — dropping them makes Postgres fail to resolve
	// the ORDER BY. Active bands surface first; a dormant band is the fallback
	// (PSY-1294 decision). Placeholder order mirrors GetActiveArtists: cutoff
	// (SELECT), status (subquery), then the roster predicate args.
	args := append([]any{activeCutoff, catalogm.ShowStatusApproved}, aargs...)
	var row embedRow
	result := s.db.Raw(`
		SELECT a.slug, a.name, a.bandcamp_embed_url,
		       COALESCE(ss.last_show >= ?, false) AS is_active,
		       COALESCE(ss.show_count, 0) AS show_count
		FROM artists a
		LEFT JOIN (
			SELECT sa.artist_id,
			       COUNT(DISTINCT s.id) AS show_count,
			       MAX(s.event_date) AS last_show
			FROM show_artists sa
			JOIN shows s ON s.id = sa.show_id
			WHERE s.status = ?
			GROUP BY sa.artist_id
		) ss ON ss.artist_id = a.id
		WHERE `+ap+` AND a.bandcamp_embed_url IS NOT NULL AND a.bandcamp_embed_url <> ''
		ORDER BY `+sceneRosterActiveOrderBy+`
		LIMIT 1
	`, args...).Scan(&row)
	if result.Error != nil {
		return nil, fmt.Errorf("failed to get representative embed: %w", result.Error)
	}
	// No matching row leaves `row` zero-valued. The WHERE filters to a non-empty
	// bandcamp_embed_url, so an empty URL here unambiguously means no band based in
	// this metro has an embed — the preview shows no player. (Uses the file's
	// zero-value no-row idiom, cf. parseSceneSlugParts.)
	if row.BandcampEmbedURL == "" {
		return nil, nil
	}

	slug := ""
	if row.Slug != nil {
		slug = *row.Slug
	}
	return &contracts.SceneRepresentativeEmbed{
		EmbedURL:   row.BandcampEmbedURL,
		ArtistName: row.Name,
		ArtistSlug: slug,
	}, nil
}

// parseSceneSlugParts splits a scene slug "city-state" into its raw lowercase
// city and 2-letter state. The LAST '-' separates the state; earlier '-' are
// part of a multi-word city ("los-angeles-ca" → "los angeles", "ca"). Returns
// ("","") for a slug with no usable separator. Shared by ParseSceneSlug and the
// entity-existence probe so both resolve a metro the same way.
func parseSceneSlugParts(slug string) (city, state string) {
	slug = strings.TrimSpace(strings.ToLower(slug))
	i := strings.LastIndex(slug, "-")
	if i <= 0 || i == len(slug)-1 {
		return "", ""
	}
	return strings.ReplaceAll(slug[:i], "-", " "), slug[i+1:]
}

// ParseSceneSlug resolves a scene slug to the scene's canonical DISPLAY identity
// (city, state). A US slug whose (city,state) pins a Census CBSA resolves to that
// metro's PRINCIPAL city — so an old member slug ("tempe-az", "brooklyn-ny")
// lands on its metro's canonical scene instead of 404ing (PSY-1255 step C). A
// slug with no CBSA falls back to matching a verified venue's literal (city,
// state). Scene EXISTENCE (>= sceneMinVenues) is enforced downstream by
// GetSceneDetail, so this may resolve a slug whose metro has no qualifying scene.
func (s *SceneService) ParseSceneSlug(slug string) (string, string, error) {
	if s.db == nil {
		return "", "", fmt.Errorf("database not initialized")
	}

	if city, state := parseSceneSlugParts(slug); city != "" && s.geocoder != nil {
		if m, ok := s.geocoder.ResolveMetro(city, state, usCountry); ok {
			if mp, ok := geo.MetroPrincipalByCBSA(m.CBSACode); ok {
				return mp.City, mp.State, nil
			}
		}
	}

	// No CBSA (non-US, or a US place not in any metro): resolve to a verified
	// venue's literal city/state, exactly as before.
	type cityState struct {
		City  string
		State string
	}
	var result cityState
	err := s.db.Raw(`
		SELECT DISTINCT city, state
		FROM venues
		WHERE verified = true
		  AND LOWER(REPLACE(city, ' ', '-')) || '-' || LOWER(state) = ?
		ORDER BY city, state
		LIMIT 1
	`, strings.ToLower(slug)).Scan(&result).Error
	if err != nil {
		return "", "", fmt.Errorf("failed to resolve scene slug: %w", err)
	}
	if result.City == "" {
		return "", "", apperrors.ErrSceneNotFound(fmt.Sprintf("scene not found for slug: %s", slug))
	}

	return result.City, result.State, nil
}

// buildSceneSlug generates a URL-safe slug from city and state.
// Example: "Phoenix", "AZ" → "phoenix-az"
func buildSceneSlug(city, state string) string {
	slug := strings.ToLower(city)
	slug = strings.ReplaceAll(slug, " ", "-")
	slug = slug + "-" + strings.ToLower(state)
	return slug
}

// Thresholds for genre intelligence.
const (
	sceneGenreMinTaggedArtists     = 30
	sceneDiversityMinTaggedArtists = 50
	sceneDiversityMinGenres        = 5
	venueGenreMinShows             = 10
)

// GetSceneGenreDistribution returns genre tags ranked by the number of distinct
// artists BASED in the metro that carry that genre tag (PSY-1255 step C).
// Returns empty if fewer than 30 tagged artists exist for the scene.
func (s *SceneService) GetSceneGenreDistribution(city, state string) ([]contracts.GenreCount, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	scope := s.scopeFor(city, state)
	ap, aargs := s.artistPredicate(scope, "a")

	type genreRow struct {
		TagID uint   `gorm:"column:tag_id"`
		Name  string `gorm:"column:name"`
		Slug  string `gorm:"column:slug"`
		Count int    `gorm:"column:count"`
	}

	var rows []genreRow
	err := s.db.Raw(`
		SELECT t.id AS tag_id, t.name, t.slug, COUNT(DISTINCT a.id) AS count
		FROM artists a
		JOIN entity_tags et ON et.entity_type = 'artist' AND et.entity_id = a.id
		JOIN tags t ON t.id = et.tag_id AND t.category = 'genre'
		WHERE `+ap+`
		GROUP BY t.id, t.name, t.slug
		ORDER BY count DESC
	`, aargs...).Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("failed to get scene genre distribution: %w", err)
	}

	// Check if total tagged artists meets threshold
	totalTagged := 0
	for _, r := range rows {
		totalTagged += r.Count
	}
	if totalTagged < sceneGenreMinTaggedArtists {
		return []contracts.GenreCount{}, nil
	}

	result := make([]contracts.GenreCount, len(rows))
	for i, r := range rows {
		result[i] = contracts.GenreCount{
			TagID: r.TagID,
			Name:  r.Name,
			Slug:  r.Slug,
			Count: r.Count,
		}
	}

	return result, nil
}

// GetGenreDiversityIndex computes the normalized Shannon entropy of the genre
// distribution across the bands BASED in the metro (PSY-1255 step C). Returns a
// value in [0, 1] where higher values indicate more genre diversity. Returns -1
// when there is insufficient data (fewer than 50 tagged artists or fewer than 5
// genres).
func (s *SceneService) GetGenreDiversityIndex(city, state string) (float64, error) {
	if s.db == nil {
		return 0, fmt.Errorf("database not initialized")
	}

	scope := s.scopeFor(city, state)
	ap, aargs := s.artistPredicate(scope, "a")

	type genreRow struct {
		Count int `gorm:"column:count"`
	}

	var rows []genreRow
	err := s.db.Raw(`
		SELECT COUNT(DISTINCT a.id) AS count
		FROM artists a
		JOIN entity_tags et ON et.entity_type = 'artist' AND et.entity_id = a.id
		JOIN tags t ON t.id = et.tag_id AND t.category = 'genre'
		WHERE `+ap+`
		GROUP BY t.id
		ORDER BY count DESC
	`, aargs...).Scan(&rows).Error
	if err != nil {
		return 0, fmt.Errorf("failed to get genre diversity index: %w", err)
	}

	// Check thresholds
	totalTagged := 0
	counts := make([]int, len(rows))
	for i, r := range rows {
		totalTagged += r.Count
		counts[i] = r.Count
	}

	if totalTagged < sceneDiversityMinTaggedArtists || len(counts) < sceneDiversityMinGenres {
		return -1, nil
	}

	return NormalizedShannonEntropy(counts), nil
}

// NormalizedShannonEntropy computes normalized Shannon entropy in [0, 1].
// Exported for testing.
func NormalizedShannonEntropy(counts []int) float64 {
	total := 0
	for _, c := range counts {
		total += c
	}
	if total == 0 {
		return 0
	}
	entropy := 0.0
	for _, c := range counts {
		if c == 0 {
			continue
		}
		p := float64(c) / float64(total)
		entropy -= p * math.Log2(p)
	}
	maxEntropy := math.Log2(float64(len(counts)))
	if maxEntropy == 0 {
		return 0
	}
	return entropy / maxEntropy
}

// DiversityLabel returns a human-readable label for a diversity index value.
func DiversityLabel(index float64) string {
	if index < 0 {
		return ""
	}
	if index >= 0.8 {
		return "Highly diverse"
	}
	if index >= 0.5 {
		return "Mixed"
	}
	if index >= 0.2 {
		return "Genre-focused"
	}
	return ""
}

// ──────────────────────────────────────────────
// Scene graph (PSY-367)
// ──────────────────────────────────────────────

// Cluster sizing constants for the scene graph. v1 cluster signal is each artist's
// most-frequently-played venue within the scene; clusters smaller than the
// threshold roll up to a single "other" bucket, and the visible palette caps at
// the Okabe-Ito 8-color set (see docs/features/scene-graph-layout.md §5).
const (
	sceneClusterMinSize       = 6 // first-class cluster floor (else rolled to "other")
	sceneClusterMaxFirstClass = 8 // cap = Okabe-Ito palette size
	sceneClusterOtherID       = "other"
	sceneClusterOtherLabel    = "Other"
)

// allowedSceneEdgeTypes whitelists relationship types that the scene graph
// surfaces. shared_bills + shared_label + member_of carry signal at scene scale;
// `similar` is an editorial vote that doesn't compose well across many artists,
// and `radio_cooccurrence` is station-level rather than scene-level. Constrain
// here so the API surface stays predictable as new types are added.
var allowedSceneEdgeTypes = map[string]bool{
	"shared_bills": true,
	"shared_label": true,
	"member_of":    true,
	"side_project": true,
}

// sceneArtistRow is the unified result of the scene-artists + primary-venue CTE.
type sceneArtistRow struct {
	ArtistID         uint    `gorm:"column:artist_id"`
	Name             string  `gorm:"column:name"`
	Slug             *string `gorm:"column:slug"`
	City             *string `gorm:"column:city"`
	State            *string `gorm:"column:state"`
	PrimaryVenueID   *uint   `gorm:"column:primary_venue_id"`
	PrimaryVenueName *string `gorm:"column:primary_venue_name"`
	RosterTotal      int     `gorm:"column:roster_total"` // full metro roster (same on every row)
}

// sceneRelationshipRow is the in-scope relationship payload from artist_relationships.
type sceneRelationshipRow struct {
	SourceArtistID   uint            `gorm:"column:source_artist_id"`
	TargetArtistID   uint            `gorm:"column:target_artist_id"`
	RelationshipType string          `gorm:"column:relationship_type"`
	Score            float32         `gorm:"column:score"`
	Detail           json.RawMessage `gorm:"column:detail"`
}

// GetSceneGraph returns the typed-edge artist relationship graph scoped to a
// single scene (city + state). Cluster IDs are computed at query time from each
// artist's most-frequent venue within the scene; the result is read-only (no
// vote data) and includes derived fields (`is_isolate`, `is_cross_cluster`)
// that the frontend would otherwise have to recompute on every render.
//
// types filters to the subset of allowed scene edge types (see
// allowedSceneEdgeTypes); empty/nil means "all allowed types".
//
// clusterBy selects the cluster signal (PSY-1262): "venue" (default) keeps
// the PSY-367 most-played-venue clusters; "community" projects the persisted
// Leiden similarity partition (artists.community_id, labeled "Around
// {artist}"). Unknown values fall back to venue; artists without a community
// (or when the partition has never been computed) roll into "other".
func (s *SceneService) GetSceneGraph(city, state string, types []string, clusterBy string) (*contracts.SceneGraphResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	// Validate scene exists (mirrors GetActiveArtists / GetSceneDetail).
	scope := s.scopeFor(city, state)
	if n, err := s.verifiedVenueCount(scope); err != nil {
		return nil, fmt.Errorf("failed to count venues: %w", err)
	} else if n < sceneMinVenues {
		return nil, apperrors.ErrSceneNotFound(fmt.Sprintf("scene not found: %s, %s", city, state))
	}

	// Whitelist + dedupe types. Empty input means "all allowed types"; an input
	// that was non-empty but resolved to nothing (every value unknown to the
	// allowlist) must short-circuit to zero edges, not silently fall back to
	// "all types".
	resolvedTypes := resolveSceneEdgeTypes(types)
	noEdgesByFilter := len(types) > 0 && len(resolvedTypes) == 0

	// 1. Single CTE query — top-N metro-roster artists (by approved show activity)
	//    + each one's most-frequent venue ACROSS the metro.
	rows, rosterTotal, err := s.querySceneArtistsWithPrimaryVenue(scope)
	if err != nil {
		return nil, fmt.Errorf("failed to query scene artists: %w", err)
	}

	resp := &contracts.SceneGraphResponse{
		Scene: contracts.SceneGraphInfo{
			Slug:             buildSceneSlug(city, state),
			City:             city,
			State:            state,
			ArtistCount:      len(rows),
			MetroRosterTotal: rosterTotal,
			RosterTruncated:  rosterTotal > len(rows),
		},
		Clusters: []contracts.SceneGraphCluster{},
		Nodes:    []contracts.SceneGraphNode{},
		Links:    []contracts.SceneGraphLink{},
	}

	if len(rows) == 0 {
		return resp, nil
	}

	// 2+3. Cluster pass + per-artist assignment. Venue mode counts artists per
	//    primary venue (PSY-367); community mode projects the persisted Leiden
	//    partition (PSY-1262). Both share the first-class rules: size >=
	//    sceneClusterMinSize, capped at the Okabe-Ito palette, long tail rolls
	//    into "other".
	artistIDs := make([]uint, 0, len(rows))
	for _, r := range rows {
		artistIDs = append(artistIDs, r.ArtistID)
	}

	var clusterByArtist map[uint]string
	if clusterBy == "community" {
		communityClusters, communityByArtist, err := s.buildCommunityClusters(artistIDs)
		if err != nil {
			return nil, fmt.Errorf("failed to build community clusters: %w", err)
		}
		resp.Clusters = communityClusters
		clusterByArtist = communityByArtist
	} else {
		clusters, clusterIDByVenue := buildSceneClusters(rows)
		resp.Clusters = clusters
		clusterByArtist = make(map[uint]string, len(rows))
		for _, r := range rows {
			clusterID := sceneClusterOtherID
			if r.PrimaryVenueID != nil {
				if cid, ok := clusterIDByVenue[*r.PrimaryVenueID]; ok {
					clusterID = cid
				}
			}
			clusterByArtist[r.ArtistID] = clusterID
		}
	}

	// 4. Batch query upcoming-show-count for every node (mirror GetArtistGraph §4).
	upcomingByArtist := s.batchUpcomingShowCount(artistIDs)

	// 5. Query in-scope relationships — both endpoints in the scene's artist set.
	var links []sceneRelationshipRow
	if !noEdgesByFilter {
		fetched, err := queryRelationshipsAmongArtists(s.db, artistIDs, resolvedTypes, RadioBackboneAlpha())
		if err != nil {
			return nil, fmt.Errorf("failed to query scene relationships: %w", err)
		}
		links = fetched
	}

	// 6. Build the link payload + flag cross-cluster ties.
	connected := make(map[uint]bool, len(rows))
	for _, l := range links {
		srcCluster := clusterByArtist[l.SourceArtistID]
		tgtCluster := clusterByArtist[l.TargetArtistID]

		var detail any
		if len(l.Detail) > 0 {
			_ = json.Unmarshal(l.Detail, &detail)
		}

		resp.Links = append(resp.Links, contracts.SceneGraphLink{
			SourceID:       l.SourceArtistID,
			TargetID:       l.TargetArtistID,
			Type:           l.RelationshipType,
			Score:          float64(l.Score),
			Detail:         detail,
			IsCrossCluster: srcCluster != tgtCluster,
		})

		connected[l.SourceArtistID] = true
		connected[l.TargetArtistID] = true
	}
	resp.Scene.EdgeCount = len(resp.Links)

	// 7. Build node list with is_isolate set from the post-filter link set.
	for _, r := range rows {
		slug := ""
		if r.Slug != nil {
			slug = *r.Slug
		}
		ncity := ""
		if r.City != nil {
			ncity = *r.City
		}
		nstate := ""
		if r.State != nil {
			nstate = *r.State
		}
		resp.Nodes = append(resp.Nodes, contracts.SceneGraphNode{
			ID:                r.ArtistID,
			Name:              r.Name,
			Slug:              slug,
			City:              ncity,
			State:             nstate,
			UpcomingShowCount: upcomingByArtist[r.ArtistID],
			ClusterID:         clusterByArtist[r.ArtistID],
			IsIsolate:         !connected[r.ArtistID],
		})
	}

	return resp, nil
}

// resolveSceneEdgeTypes filters the caller's requested types against the
// scene-graph allowlist and returns a deterministic slice. Empty input → all
// allowed types.
func resolveSceneEdgeTypes(requested []string) []string {
	if len(requested) == 0 {
		out := make([]string, 0, len(allowedSceneEdgeTypes))
		for t := range allowedSceneEdgeTypes {
			out = append(out, t)
		}
		// Deterministic order so query plans + tests don't churn.
		sortStringsAsc(out)
		return out
	}
	seen := make(map[string]bool, len(requested))
	out := make([]string, 0, len(requested))
	for _, t := range requested {
		if !allowedSceneEdgeTypes[t] || seen[t] {
			continue
		}
		seen[t] = true
		out = append(out, t)
	}
	sortStringsAsc(out)
	return out
}

// sortStringsAsc is a tiny insertion sort on strings — same shape as the
// insertion sort patterns in artist_relationship_service.go. Kept inline to
// avoid pulling in `sort` for one call site.
func sortStringsAsc(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}

// querySceneArtistsWithPrimaryVenue runs the CTE that selects the top-N bands
// BASED in the scene's metro (PSY-1255 step C), ranked by approved show count
// at metro venues (tie-break: most recent show, then name). Each selected
// artist's most-frequently-played venue ACROSS the metro is resolved in the
// same pass. Returns the capped roster rows and the full metro roster size
// (for the PSY-1277 truncation flag).
func (s *SceneService) querySceneArtistsWithPrimaryVenue(scope sceneScope) ([]sceneArtistRow, int, error) {
	ap, aargs := s.artistPredicate(scope, "a")
	vp, vargs := scope.venuePredicate("v")
	const q = `
		WITH scene_artists AS (
			SELECT a.id AS artist_id FROM artists a WHERE %s
		),
		roster_total AS (
			SELECT COUNT(*)::bigint AS total FROM scene_artists
		),
		artist_metro_activity AS (
			SELECT
				sa.artist_id,
				COUNT(DISTINCT s.id) AS show_count,
				MAX(s.event_date) AS last_show
			FROM show_artists sa
			JOIN shows s ON s.id = sa.show_id AND s.status = ?
			JOIN show_venues sv ON sv.show_id = s.id
			JOIN venues v ON v.id = sv.venue_id AND %s
			WHERE sa.artist_id IN (SELECT artist_id FROM scene_artists)
			GROUP BY sa.artist_id
		),
		ranked_roster AS (
			SELECT
				sca.artist_id,
				ROW_NUMBER() OVER (
					ORDER BY COALESCE(ama.show_count, 0) DESC,
					         ama.last_show DESC NULLS LAST,
					         a.name ASC
				) AS rn
			FROM scene_artists sca
			JOIN artists a ON a.id = sca.artist_id
			LEFT JOIN artist_metro_activity ama ON ama.artist_id = sca.artist_id
		),
		selected_roster AS (
			SELECT artist_id FROM ranked_roster WHERE rn <= ?
		),
		artist_venue_counts AS (
			SELECT
				sa.artist_id,
				v.id AS venue_id,
				v.name AS venue_name,
				COUNT(DISTINCT s.id) AS plays,
				MAX(s.event_date) AS last_played,
				ROW_NUMBER() OVER (
					PARTITION BY sa.artist_id
					ORDER BY COUNT(DISTINCT s.id) DESC, MAX(s.event_date) DESC
				) AS rn
			FROM show_artists sa
			JOIN shows s ON s.id = sa.show_id
			JOIN show_venues sv ON sv.show_id = s.id
			JOIN venues v ON v.id = sv.venue_id
			WHERE %s AND s.status = ?
				AND sa.artist_id IN (SELECT artist_id FROM selected_roster)
			GROUP BY sa.artist_id, v.id, v.name
		)
		SELECT
			a.id   AS artist_id,
			a.name AS name,
			a.slug AS slug,
			a.city AS city,
			a.state AS state,
			avc.venue_id   AS primary_venue_id,
			avc.venue_name AS primary_venue_name,
			rt.total AS roster_total
		FROM artists a
		JOIN selected_roster sr ON sr.artist_id = a.id
		CROSS JOIN roster_total rt
		LEFT JOIN artist_venue_counts avc ON avc.artist_id = a.id AND avc.rn = 1
		ORDER BY a.name ASC
	`
	args := append(append([]any{}, aargs...), catalogm.ShowStatusApproved)
	args = append(args, vargs...)
	args = append(args, sceneGraphRosterLimit)
	args = append(args, vargs...)
	args = append(args, catalogm.ShowStatusApproved)
	var rows []sceneArtistRow
	if err := s.db.Raw(fmt.Sprintf(q, ap, vp, vp), args...).Scan(&rows).Error; err != nil {
		return nil, 0, err
	}
	rosterTotal := 0
	if len(rows) > 0 {
		rosterTotal = rows[0].RosterTotal
	} else {
		// Empty capped roster — still need the full metro count for the truncation flag.
		var n int64
		countQ := "SELECT COUNT(*) FROM artists a WHERE " + ap
		if err := s.db.Raw(countQ, aargs...).Scan(&n).Error; err != nil {
			return nil, 0, err
		}
		rosterTotal = int(n)
	}
	return rows, rosterTotal, nil
}

// queryRelationshipsAmongArtists fetches all stored relationships where BOTH
// source and target artist IDs are in the given artist set, optionally
// filtered to the resolved type list. Since PSY-1323 the derive steps keep
// one-off co-bills (minShows=1) with a low score rather than excluding them,
// and this query applies NO weight filter — every stored edge among the
// roster is returned. The blast radius is bounded by the roster cap upstream
// and by the frontend's score-scaled edge rendering (edgeGrammar.edgeWidth),
// which de-emphasizes low-score one-off edges visually; if dense scenes
// hairball after the PSY-1323 re-derive, add a min-score filter or per-node
// cap here. Shared by the scene graph (PSY-367) and the festival graph
// (PSY-1080).
//
// backboneAlpha applies the PSY-1293 disparity-filter backbone to the dense
// radio_cooccurrence edges: when alpha > 0, a radio_cooccurrence edge is kept
// only if its backbone_significance < alpha (NULL significance = not in the
// backbone = dropped); non-radio relationship types are always kept. alpha <= 0
// disables the filter entirely (all edges kept, the pre-PSY-1293 behavior) — the
// festival graph passes 0 as the backbone is a scene-scale tool (see the ego
// treatment in GetArtistGraph, which UNIONs the backbone rather than replacing
// its top-k floor).
func queryRelationshipsAmongArtists(db *gorm.DB, artistIDs []uint, types []string, backboneAlpha float64) ([]sceneRelationshipRow, error) {
	if len(artistIDs) < 2 {
		return nil, nil
	}
	var rows []sceneRelationshipRow
	q := db.Table("artist_relationships").
		Select("source_artist_id, target_artist_id, relationship_type, score, detail").
		Where("source_artist_id IN ? AND target_artist_id IN ?", artistIDs, artistIDs)
	if len(types) > 0 {
		q = q.Where("relationship_type IN ?", types)
	}
	if backboneAlpha > 0 {
		q = q.Where("relationship_type <> ? OR backbone_significance < ?",
			catalogm.RelationshipTypeRadioCooccurrence, backboneAlpha)
	}
	if err := q.Scan(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

// batchUpcomingShowCount returns a map of artist_id → upcoming approved show
// count, mirroring the batch pattern in GetArtistGraph step 4. Returns an empty
// map (never nil) so callers can index without a nil check. Delegates to the
// shared graph helper (PSY-1081).
func (s *SceneService) batchUpcomingShowCount(artistIDs []uint) map[uint]int {
	return batchArtistUpcomingShowCounts(s.db, artistIDs)
}

// buildSceneClusters converts the per-artist primary-venue rows into a sorted
// list of cluster definitions. Clusters >= sceneClusterMinSize are first-class
// (capped at sceneClusterMaxFirstClass = 8, the Okabe-Ito palette size); the
// remainder roll up to a single "other" cluster. Returns the cluster list
// (caller-facing) and a venue_id → cluster_id lookup (used to assign nodes).
func buildSceneClusters(rows []sceneArtistRow) ([]contracts.SceneGraphCluster, map[uint]string) {
	type venueCount struct {
		venueID uint
		name    string
		count   int
	}
	byVenue := make(map[uint]*venueCount)
	for _, r := range rows {
		if r.PrimaryVenueID == nil {
			continue
		}
		entry, ok := byVenue[*r.PrimaryVenueID]
		if !ok {
			name := ""
			if r.PrimaryVenueName != nil {
				name = *r.PrimaryVenueName
			}
			entry = &venueCount{venueID: *r.PrimaryVenueID, name: name}
			byVenue[*r.PrimaryVenueID] = entry
		}
		entry.count++
	}

	venues := make([]*venueCount, 0, len(byVenue))
	for _, v := range byVenue {
		venues = append(venues, v)
	}
	// Sort by count desc, then name asc for deterministic ordering on ties.
	for i := 1; i < len(venues); i++ {
		for j := i; j > 0; j-- {
			a, b := venues[j], venues[j-1]
			better := a.count > b.count || (a.count == b.count && a.name < b.name)
			if !better {
				break
			}
			venues[j], venues[j-1] = b, a
		}
	}

	clusterIDByVenue := make(map[uint]string, len(venues))
	clusters := make([]contracts.SceneGraphCluster, 0, len(venues)+1)
	otherSize := 0

	for i, v := range venues {
		if v.count >= sceneClusterMinSize && len(clusters) < sceneClusterMaxFirstClass {
			cid := fmt.Sprintf("v_%d", v.venueID)
			clusterIDByVenue[v.venueID] = cid
			clusters = append(clusters, contracts.SceneGraphCluster{
				ID:         cid,
				Label:      v.name,
				Size:       v.count,
				ColorIndex: i,
			})
			continue
		}
		// Falls into "other" — reuses the bucket id; no per-venue mapping needed.
		clusterIDByVenue[v.venueID] = sceneClusterOtherID
		otherSize += v.count
	}

	// Artists with no primary venue (defensive — e.g. data anomalies) land in "other".
	for _, r := range rows {
		if r.PrimaryVenueID == nil {
			otherSize++
		}
	}

	if otherSize > 0 {
		clusters = append(clusters, contracts.SceneGraphCluster{
			ID:         sceneClusterOtherID,
			Label:      sceneClusterOtherLabel,
			Size:       otherSize,
			ColorIndex: -1,
		})
	}

	return clusters, clusterIDByVenue
}

// buildCommunityClusters projects the persisted Leiden partition (PSY-1262)
// onto the scene's artist set: communities are counted by IN-SCENE members,
// first-classed by the same size/palette rules as venue clusters, labeled
// "Around {artist}" from artist_communities, and everything else — artists
// with NULL community_id, or members of communities below the size floor —
// rolls into "other". A never-computed partition therefore degrades to a
// single "other" cluster rather than erroring.
//
// Deliberate semantics worth knowing (adversarial review, 2026-07-02):
//   - Community identity is GLOBAL: the "Around {artist}" anchor is the
//     community's globally strongest member and may be based OUTSIDE this
//     scene — the same community carries the same label on every scene,
//     which is the point of one global partition. Cluster Size is in-scene;
//     artist_communities.member_count is global.
//   - In-scene connectivity is NOT validated: the Leiden guarantee holds on
//     the global input graph, but two in-scene members can be linked only
//     through out-of-scene artists or non-scene edge types, so a colored
//     cluster may render as visually separate islands (venue clusters make
//     no connectivity claim either).
//   - Assignments + labels are read in ONE statement so a request can't
//     straddle the nightly partition swap and mislabel clusters (the swap is
//     writer-atomic, but two separate reads would get two snapshots).
func (s *SceneService) buildCommunityClusters(artistIDs []uint) ([]contracts.SceneGraphCluster, map[uint]string, error) {
	type artistCommunityRow struct {
		ID          uint    `gorm:"column:id"`
		CommunityID *int    `gorm:"column:community_id"`
		LabelName   *string `gorm:"column:label_name"`
	}
	var rows []artistCommunityRow
	if err := s.db.Table("artists a").
		Select("a.id, a.community_id, la.name AS label_name").
		Joins("LEFT JOIN artist_communities ac ON ac.id = a.community_id").
		Joins("LEFT JOIN artists la ON la.id = ac.label_artist_id").
		Where("a.id IN ?", artistIDs).
		Scan(&rows).Error; err != nil {
		return nil, nil, fmt.Errorf("failed to load community assignments: %w", err)
	}

	communityByArtist := make(map[uint]int, len(rows))
	counts := make(map[int]int)
	labelByCommunity := make(map[int]string)
	for _, r := range rows {
		if r.CommunityID == nil {
			continue
		}
		communityByArtist[r.ID] = *r.CommunityID
		counts[*r.CommunityID]++
		if r.LabelName != nil {
			labelByCommunity[*r.CommunityID] = "Around " + *r.LabelName
		}
	}

	type communityCount struct {
		communityID int
		count       int
	}
	ordered := make([]communityCount, 0, len(counts))
	for c, n := range counts {
		ordered = append(ordered, communityCount{communityID: c, count: n})
	}
	// Size desc, community id asc on ties — deterministic like the venue sort.
	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].count != ordered[j].count {
			return ordered[i].count > ordered[j].count
		}
		return ordered[i].communityID < ordered[j].communityID
	})

	clusterIDByCommunity := make(map[int]string, len(ordered))
	clusters := make([]contracts.SceneGraphCluster, 0, len(ordered)+1)
	otherSize := 0
	for _, cc := range ordered {
		if cc.count >= sceneClusterMinSize && len(clusters) < sceneClusterMaxFirstClass {
			cid := fmt.Sprintf("c_%d", cc.communityID)
			clusterIDByCommunity[cc.communityID] = cid
			label, ok := labelByCommunity[cc.communityID]
			if !ok {
				// Label row missing (partition/label drift) — degrade honestly.
				label = fmt.Sprintf("Community %d", cc.communityID)
			}
			clusters = append(clusters, contracts.SceneGraphCluster{
				ID:         cid,
				Label:      label,
				Size:       cc.count,
				ColorIndex: len(clusters),
			})
			continue
		}
		clusterIDByCommunity[cc.communityID] = sceneClusterOtherID
		otherSize += cc.count
	}

	clusterByArtist := make(map[uint]string, len(artistIDs))
	for _, id := range artistIDs {
		comm, ok := communityByArtist[id]
		if !ok {
			clusterByArtist[id] = sceneClusterOtherID
			otherSize++
			continue
		}
		clusterByArtist[id] = clusterIDByCommunity[comm]
	}

	if otherSize > 0 {
		clusters = append(clusters, contracts.SceneGraphCluster{
			ID:         sceneClusterOtherID,
			Label:      sceneClusterOtherLabel,
			Size:       otherSize,
			ColorIndex: -1,
		})
	}

	return clusters, clusterByArtist, nil
}
