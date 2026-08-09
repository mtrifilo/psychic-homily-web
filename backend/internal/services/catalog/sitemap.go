package catalog

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"

	"psychic-homily-backend/db"
	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/services/geo"
	"psychic-homily-backend/internal/services/shared"
	"psychic-homily-backend/internal/utils"
)

// sceneWeekSitemapWindow is how many recent ISO weeks (including the current
// one) are eligible for the sitemap, per scene. Decided PSY-1622: 8 weeks,
// not 52 or unbounded — recent weeks are the citable ones.
const sceneWeekSitemapWindow = 8

// SitemapService answers the sitemap generator's one question: which slugs are
// indexable, and when did each last change.
//
// It deliberately avoids the public list services, which hydrate joins and full
// response bodies the generator throws away. That coupling is not a style
// preference — it is the defect this service exists to fix; see
// contracts.SitemapEntry for the incident. Keep the queries here projections:
// the moment this starts Preloading it inherits the same runaway payload.
type SitemapService struct {
	db       *gorm.DB
	geocoder geo.Geocoder
}

func NewSitemapService(database *gorm.DB) *SitemapService {
	if database == nil {
		database = db.GetDB()
	}
	return &SitemapService{db: database, geocoder: geo.Default()}
}

// Entries returns the indexable slug set for every URL family the sitemap
// currently covers. A failure in any one family fails the whole call — the
// generator must never be handed a partial result.
//
// When family is non-empty, only that family's field is populated and the rest
// stay empty slices. The frontend shards by family via generateSitemaps(), and
// each shard fetches `?family=…` so Next's Data Cache keys (and ~1.5 MB
// budgets) stay independent. An unknown family is an error.
//
// ctx is threaded to the queries so an abandoned request (the generator gives
// up at 30 s) does not leave unbounded scans running to completion.
func (s *SitemapService) Entries(ctx context.Context, family string) (*contracts.SitemapEntries, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	family = strings.TrimSpace(family)
	out := &contracts.SitemapEntries{
		Shows:      []contracts.SitemapEntry{},
		Artists:    []contracts.SitemapEntry{},
		Venues:     []contracts.SitemapEntry{},
		VenueYears: []contracts.SitemapEntry{},
		Scenes:     []contracts.SitemapEntry{},
		SceneWeeks: []contracts.SitemapEntry{},
		Labels:     []contracts.SitemapEntry{},
		Releases:   []contracts.SitemapEntry{},
		Festivals:  []contracts.SitemapEntry{},
		Tags:       []contracts.SitemapEntry{},
	}

	known := map[string]bool{
		// Keep in sync with the Huma `family` enum on GetSitemapEntriesRequest
		// and frontend/app/sitemap-shards.ts FAMILY_SHARD_IDS.
		"shows": true, "artists": true, "venues": true, "venue_years": true,
		"scenes": true, "scene_weeks": true,
		"labels": true, "releases": true, "festivals": true, "tags": true,
	}
	if family != "" && !known[family] {
		return nil, fmt.Errorf("unknown sitemap family %q", family)
	}

	want := func(name string) bool {
		return family == "" || family == name
	}

	if want("shows") {
		// Only approved shows are publicly reachable: GetShowHandler rejects
		// every other status, so advertising them would fill the index with
		// dead URLs. Grep ShowStatusApproved rather than trusting any single
		// site — the reachability rule is enforced in many places and can drift.
		shows, err := s.entriesFor(
			ctx,
			s.db.Model(&catalogm.Show{}).Where("status = ?", catalogm.ShowStatusApproved),
		)
		if err != nil {
			return nil, fmt.Errorf("failed to collect show sitemap entries: %w", err)
		}
		out.Shows = shows
	}

	if want("artists") {
		artists, err := s.entriesFor(ctx, s.db.Model(&catalogm.Artist{}))
		if err != nil {
			return nil, fmt.Errorf("failed to collect artist sitemap entries: %w", err)
		}
		out.Artists = artists
	}

	if want("venues") {
		venues, err := s.entriesFor(ctx, s.db.Model(&catalogm.Venue{}))
		if err != nil {
			return nil, fmt.Errorf("failed to collect venue sitemap entries: %w", err)
		}
		out.Venues = venues
	}

	if want("venue_years") {
		venueYears, err := s.venueYearEntries(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to collect venue-year sitemap entries: %w", err)
		}
		out.VenueYears = venueYears
	}

	if want("scenes") {
		scenes, err := s.sceneEntries(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to collect scene sitemap entries: %w", err)
		}
		out.Scenes = scenes
	}

	if want("scene_weeks") {
		weeks, err := s.sceneWeekEntries(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to collect scene-week sitemap entries: %w", err)
		}
		out.SceneWeeks = weeks
	}

	if want("labels") {
		labels, err := s.entriesFor(ctx, s.db.Model(&catalogm.Label{}))
		if err != nil {
			return nil, fmt.Errorf("failed to collect label sitemap entries: %w", err)
		}
		out.Labels = labels
	}

	if want("releases") {
		releases, err := s.entriesFor(ctx, s.db.Model(&catalogm.Release{}))
		if err != nil {
			return nil, fmt.Errorf("failed to collect release sitemap entries: %w", err)
		}
		out.Releases = releases
	}

	if want("festivals") {
		// Festival.Slug is a non-null string column (not *string), so the
		// entriesFor empty-slug predicate still applies for the empty-string
		// case GenerateSlug can produce.
		festivals, err := s.entriesFor(ctx, s.db.Model(&catalogm.Festival{}))
		if err != nil {
			return nil, fmt.Errorf("failed to collect festival sitemap entries: %w", err)
		}
		out.Festivals = festivals
	}

	if want("tags") {
		tags, err := s.entriesFor(ctx, s.db.Model(&catalogm.Tag{}))
		if err != nil {
			return nil, fmt.Errorf("failed to collect tag sitemap entries: %w", err)
		}
		out.Tags = tags
	}

	return out, nil
}

// entriesFor projects slug + updated_at from an already-scoped query, skipping
// rows with no slug: slug is nullable on several models, and a row without
// one has no canonical URL to index. (GenerateSlug returns "" for an all-
// non-ASCII name, hence the empty-string check as well as the NULL one.)
//
// Taking a scope keeps each family's visibility predicate at the call site
// where it can be read next to the model it applies to.
//
// CONSTRAINT: the scope must resolve to a SINGLE table. slug and updated_at are
// referenced unqualified here, so a joined scope will either error with
// "column reference is ambiguous" or silently bind the wrong table's
// updated_at. A family needing a join wants its own projection, not this.
func (s *SitemapService) entriesFor(ctx context.Context, scope *gorm.DB) ([]contracts.SitemapEntry, error) {
	// Deterministic order, so two fetches of an unchanged catalogue diff
	// cleanly. Ordered by slug rather than recency because the partial unique
	// index on slug (migration 000013) can supply that order, while no index on
	// updated_at exists for any of these tables — sorting on it would buy a
	// guaranteed sort node for an ordering no consumer reads.
	entries := []contracts.SitemapEntry{}
	err := scope.
		WithContext(ctx).
		Where("slug IS NOT NULL AND slug <> ''").
		Order("slug ASC").
		Select("slug", "updated_at").
		Scan(&entries).Error
	if err != nil {
		return nil, err
	}
	return entries, nil
}

// venueYearEntries projects one SitemapEntry per venue-local calendar year that
// venue has at least one approved PAST show in — the crawlable year archives at
// /venues/{slug}/shows/{year} (PSY-1756).
//
// PAST only, and that is the whole definition of the surface rather than a
// filter applied to it: the archive page renders the same `time_filter=past`
// histogram the venue page's year strip is built from, so a year announced here
// that the histogram does not carry would be a URL the site itself 404s. The two
// must be derived the same way, which is why this builds the year through
// shared.VenueLocalYearSQL and the boundary through
// shared.VenueLocalDateCondition rather than restating either.
//
// Unlike the entity families this is NOT one row per table row, so entriesFor
// cannot serve it: the grain is (venue, year) and the slug is composite.
// UpdatedAt is MAX(show.updated_at) within the bucket — the closest durable
// "this page's content changed" signal, matching sceneWeekEntries.
//
// Venues with no past shows produce NO rows, so a venue whose archive would be
// empty is never advertised. That is the same rule the page enforces (it 404s a
// year the histogram does not carry); both sides fall out of this one query
// shape rather than needing to be kept in sync by hand.
//
// COST, recorded rather than discovered later. This family is the most
// expensive one in Entries and the only one whose input set grows without
// bound: it scans every approved PAST show (the coarse `event_date < now()`
// bound prunes nothing on an archive) with one lateral execution per row, and
// past shows never age out. The other families are either single-table scans
// (entriesFor) or windowed (sceneWeekEntries, 8 weeks per scene). Two thresholds
// to watch as the catalogue grows, neither of which is close today: the ~1.5 MB
// Next Data Cache budget per shard (app/sitemap.ts weighs it and fails the
// build), and the 50,000-URL sitemap limit — this family is venues x years, so
// it will reach both before any other. The proportionate fix at that point is a
// rollup refreshed on the same hourly cadence, not a cleverer query here.
func (s *SitemapService) venueYearEntries(ctx context.Context) ([]contracts.SitemapEntry, error) {
	type row struct {
		Slug      string    `gorm:"column:slug"`
		Year      int       `gorm:"column:year"`
		UpdatedAt time.Time `gorm:"column:updated_at"`
	}

	var rows []row
	// The lateral in VenueTZJoin correlates on shows.id, so `shows` must already
	// be joined when it lands — hence the join order here. Its inner alias is
	// `iv`; the archive's own venue is aliased `av` so the two cannot collide.
	//
	// NOTE the asymmetry this inherits from VenueTZJoin, which is deliberate and
	// shared with every other venue-local surface: a show billed at two venues is
	// bucketed in the PRIMARY venue's zone even when listed under the secondary
	// one. Bucketing it here in `av`'s zone instead would put this family half a
	// day away from the page it points at, for the shows that straddle a New Year
	// boundary.
	err := s.db.WithContext(ctx).
		Table("show_venues").
		Joins("JOIN shows ON show_venues.show_id = shows.id").
		Joins("JOIN venues av ON av.id = show_venues.venue_id").
		Joins(shared.VenueTZJoin).
		Where("shows.status = ?", catalogm.ShowStatusApproved).
		Where("av.slug IS NOT NULL AND av.slug <> ''").
		Where(shared.VenueLocalDateCondition("past")).
		Select("av.slug AS slug, " + shared.VenueLocalYearSQL + " AS year, MAX(shows.updated_at) AS updated_at").
		Group("av.slug, " + shared.VenueLocalYearSQL).
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}

	entries := make([]contracts.SitemapEntry, 0, len(rows))
	for _, r := range rows {
		entries = append(entries, contracts.SitemapEntry{
			Slug:      r.Slug + "/shows/" + strconv.Itoa(r.Year),
			UpdatedAt: r.UpdatedAt,
		})
	}
	// Deterministic order, so two fetches of an unchanged catalogue diff cleanly
	// — the same reason entriesFor sorts. Sorted in Go rather than SQL because
	// the composite slug is assembled here, and the two orders are not the same:
	// ordering by (slug, year) in SQL groups a venue's years together, while
	// ordering the assembled string interleaves venues whose slugs are prefixes
	// of one another ("club-2/shows/2025" sorts before "club-2-x/shows/1999",
	// because '/' is below '-'). The emitted document has to be sorted the way
	// it is read.
	sort.Slice(entries, func(i, j int) bool { return entries[i].Slug < entries[j].Slug })
	return entries, nil
}

// sceneGroupRow is one qualifying scene for the sitemap projections.
type sceneGroupRow struct {
	Metro string `gorm:"column:metro"`
	City  string `gorm:"column:city"`
	State string `gorm:"column:state"`
}

// listQualifyingScenes returns the same threshold-gated scene set as
// ListScenes (2+ verified venues, 3+ approved shows), without the genre /
// coordinate hydration that list endpoint pays for.
func (s *SitemapService) listQualifyingScenes(ctx context.Context) ([]sceneGroupRow, error) {
	var groups []sceneGroupRow
	err := s.db.WithContext(ctx).Raw(`
		SELECT COALESCE(MAX(v.metro), '') AS metro,
		       MIN(v.city)  AS city,
		       MIN(v.state) AS state
		FROM venues v
		LEFT JOIN show_venues sv ON sv.venue_id = v.id
		LEFT JOIN shows s ON s.id = sv.show_id AND s.status = ?
		WHERE true
		  `+sceneVenueEligibilitySQL+`
		GROUP BY `+sceneGroupKeySQL+`
		HAVING COUNT(DISTINCT v.id) >= ?
		   AND COUNT(DISTINCT s.id) >= ?
	`, catalogm.ShowStatusApproved, sceneMinVenues, sceneMinShows).Scan(&groups).Error
	if err != nil {
		return nil, err
	}
	return groups, nil
}

// sceneEntries projects one SitemapEntry per qualifying scene. Scenes are
// computed aggregations (no scenes.updated_at), so lastmod is MAX(show.updated_at)
// among approved shows at the scene's venues — the closest durable signal to
// "this page's content changed".
func (s *SitemapService) sceneEntries(ctx context.Context) ([]contracts.SitemapEntry, error) {
	type row struct {
		Metro     string    `gorm:"column:metro"`
		City      string    `gorm:"column:city"`
		State     string    `gorm:"column:state"`
		UpdatedAt time.Time `gorm:"column:updated_at"`
	}
	var rows []row
	err := s.db.WithContext(ctx).Raw(`
		SELECT COALESCE(MAX(v.metro), '') AS metro,
		       MIN(v.city)  AS city,
		       MIN(v.state) AS state,
		       MAX(s.updated_at) AS updated_at
		FROM venues v
		JOIN show_venues sv ON sv.venue_id = v.id
		JOIN shows s ON s.id = sv.show_id AND s.status = ?
		WHERE true
		  `+sceneVenueEligibilitySQL+`
		GROUP BY `+sceneGroupKeySQL+`
		HAVING COUNT(DISTINCT v.id) >= ?
		   AND COUNT(DISTINCT s.id) >= ?
	`, catalogm.ShowStatusApproved, sceneMinVenues, sceneMinShows).Scan(&rows).Error
	if err != nil {
		return nil, err
	}

	entries := make([]contracts.SitemapEntry, 0, len(rows))
	bySlug := map[string]time.Time{}
	for _, r := range rows {
		city, state := metroDisplayIdentity(r.Metro, r.City, r.State)
		slug := buildSceneSlug(city, state)
		if prev, ok := bySlug[slug]; !ok || r.UpdatedAt.After(prev) {
			bySlug[slug] = r.UpdatedAt
		}
	}
	for slug, updatedAt := range bySlug {
		entries = append(entries, contracts.SitemapEntry{Slug: slug, UpdatedAt: updatedAt})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Slug < entries[j].Slug })
	return entries, nil
}

// sceneWeekEntries projects archived week permalinks for the last
// sceneWeekSitemapWindow weeks per qualifying scene, excluding weeks with zero
// approved shows. Slug is the composite "{scene-slug}/{iso-week}" matching the
// canonical URL sceneWeekPage.tsx already declares.
//
// Week boundaries are resolved in each scene's own timezone — the same rule
// GetSceneWeek uses — so a show at 21:00 Sunday Chicago does not fall into the
// wrong ISO week when bucketed in UTC.
func (s *SitemapService) sceneWeekEntries(ctx context.Context) ([]contracts.SitemapEntry, error) {
	groups, err := s.listQualifyingScenes(ctx)
	if err != nil {
		return nil, err
	}
	if len(groups) == 0 {
		return []contracts.SitemapEntry{}, nil
	}

	// Two venue groups can resolve to the same display slug (a metro group plus
	// a no-metro fallback that shares the principal city — common in E2E seed
	// data). Collapse first so we query each canonical scene once.
	type sceneIdent struct {
		city, state, slug string
	}
	unique := make([]sceneIdent, 0, len(groups))
	seenSlug := map[string]bool{}
	for _, g := range groups {
		city, state := metroDisplayIdentity(g.Metro, g.City, g.State)
		slug := buildSceneSlug(city, state)
		if seenSlug[slug] {
			continue
		}
		seenSlug[slug] = true
		unique = append(unique, sceneIdent{city: city, state: state, slug: slug})
	}

	entries := make([]contracts.SitemapEntry, 0, len(unique)*sceneWeekSitemapWindow)
	for _, sc := range unique {
		scope := metroScopeFor(s.geocoder, sc.city, sc.state)
		loc := s.sceneLocation(scope, sc.state)

		nowLocal := time.Now().In(loc)
		y, w := nowLocal.ISOWeek()
		currentStart := ISOWeekStart(y, w, loc)
		windowStart := currentStart.AddDate(0, 0, -7*(sceneWeekSitemapWindow-1))
		windowEnd := currentStart.AddDate(0, 0, 7) // half-open end of current week

		vp, vargs := scope.venuePredicate("v")
		type showRow struct {
			EventDate time.Time `gorm:"column:event_date"`
			UpdatedAt time.Time `gorm:"column:updated_at"`
		}
		args := append(append([]any{}, vargs...), catalogm.ShowStatusApproved, windowStart.UTC(), windowEnd.UTC())
		var shows []showRow
		if err := s.db.WithContext(ctx).Raw(`
			SELECT s.event_date, s.updated_at
			FROM shows s
			JOIN show_venues sv ON sv.show_id = s.id
			JOIN venues v ON v.id = sv.venue_id
			WHERE `+vp+`
			  AND s.status = ?
			  AND s.event_date >= ?
			  AND s.event_date < ?
		`, args...).Scan(&shows).Error; err != nil {
			return nil, err
		}

		type weekAgg struct {
			updatedAt time.Time
			count     int
		}
		byWeek := map[string]*weekAgg{}
		for _, sh := range shows {
			key := ISOWeekKey(sh.EventDate.In(loc))
			agg := byWeek[key]
			if agg == nil {
				agg = &weekAgg{}
				byWeek[key] = agg
			}
			agg.count++
			if sh.UpdatedAt.After(agg.updatedAt) {
				agg.updatedAt = sh.UpdatedAt
			}
		}

		// Emit only weeks inside the window that had at least one show. Iterate
		// the window keys rather than the map so a quiet week stays omitted
		// (the zero-show exclusion) and order stays deterministic after the
		// final sort.
		for i := 0; i < sceneWeekSitemapWindow; i++ {
			start := windowStart.AddDate(0, 0, 7*i)
			key := ISOWeekKey(start)
			agg := byWeek[key]
			if agg == nil || agg.count == 0 {
				continue
			}
			entries = append(entries, contracts.SitemapEntry{
				Slug:      sc.slug + "/" + key,
				UpdatedAt: agg.updatedAt,
			})
		}
	}

	sort.Slice(entries, func(i, j int) bool { return entries[i].Slug < entries[j].Slug })
	return entries, nil
}

// sceneLocation mirrors SceneService.sceneLocation: modal verified-venue
// timezone, falling back to the state map. Duplicated rather than shared so
// SitemapService does not grow a SceneService dependency for one query.
func (s *SitemapService) sceneLocation(scope sceneScope, state string) *time.Location {
	if s.db == nil {
		return utils.EventLocation(nil, state)
	}
	vp, vargs := scope.venuePredicate("v")
	var tz string
	err := s.db.Raw(`
		SELECT v.timezone
		FROM venues v
		WHERE `+vp+`
		  AND v.verified = true
		  AND v.timezone IS NOT NULL
		  AND v.timezone <> ''
		GROUP BY v.timezone
		ORDER BY COUNT(*) DESC, v.timezone ASC
		LIMIT 1
	`, vargs...).Scan(&tz).Error
	if err != nil || tz == "" {
		return utils.EventLocation(nil, state)
	}
	return utils.EventLocation(&tz, state)
}
