package catalog

import (
	"context"
	"fmt"
	"slices"
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

// sitemapFamilies is every entity family Entries can populate.
//
// Ordered, and the ONE list: the `family` enum on GetSitemapEntriesRequest is
// asserted against SitemapFamilyValues() in the handler package's tests, so a
// family added here without being added to the enum fails a test rather than
// producing a 422 nobody expects. Keep in sync with
// frontend/app/sitemap-shards.ts SITEMAP_FAMILIES and with
// contracts.SitemapEntries.
var sitemapFamilies = []string{
	"shows", "artists", "venues", "venue_years", "scenes", "scene_weeks",
	"labels", "releases", "festivals", "tags",
}

// sitemapShard is one bucket of an entity family, addressable on the wire as if
// it were a family of its own. A row belongs to bucket `id % buckets` of its
// family.
//
// WHY FAMILIES GET SUB-SHARDED. Sharding the sitemap per family (PSY-1622)
// keeps each Next Data Cache entry under the per-item cap of 2 MiB, i.e.
// 2,097,152 bytes, MEBIbytes, which is the unit the frontend's
// lib/data-cache-budget/budget.ts insists on for this route family precisely
// because decimal MB put the label and the arithmetic in disagreement. The
// entry holds the response base64-encoded, so the raw body that fits is
// 1,572,864 bytes, and the frontend's gate fails the build at 80% of the cap.
// Three families do not fit one entry each. Measured against production on
// 2026-09-03: releases 2,041,734 raw bytes over 28,720 rows (129.8% of the
// cap), shows 1,267,618 over 13,096 (80.6%), artists 918,744 over 13,493
// (58.4%). No other family is above 8%.
//
// WHAT THE PRIMARY KEY BUYS AS A PARTITION KEY:
//
//   - TOTALITY AND DISJOINTNESS ARE STRUCTURAL. A family enumerates every
//     residue in [0, buckets), so a gap needs a missing residue rather than a
//     mis-typed bound. Primary keys here are `uint` columns fed by a serial
//     sequence and Postgres keeps the dividend's sign in `%`, so the partition
//     covers exactly the non-negative ids the tables hold.
//   - A ROW NEVER CHANGES BUCKET, because a primary key never changes. The only
//     thing that moves a URL between documents is a change of `buckets`.
//   - BUCKETS STAY BALANCED WITHOUT RE-CUTTING, because membership is
//     independent of what a row contains. Measured spread inside a family is
//     under one percentage point of the cache-item cap; see SHOW_SHARD_IDS in
//     frontend/app/sitemap-shards.ts for the per-bucket byte table.
//
// WHAT IT COSTS. A bucket has no locality, so an insert dirties one bucket at
// random rather than the newest range, and doubling `buckets` moves half of a
// family's URLs between documents. That is the whole re-tuning cost, paid once
// per doubling, and sitemapShardsPerFamily states the headroom each family is
// given so a doubling stays rare.
//
// A BUCKET APPEARS IN NO URL. Entries are announced at /shows/{slug} whichever
// bucket serves them, so which document carries a URL is a transport detail.
//
// Each id doubles as the `family` query value the backend accepts for that
// bucket. Deliberately: a backend that predates a new id answers 422, which the
// generator degrades to an empty shard for one deploy window
// (UNKNOWN_FAMILY_STATUSES in frontend/app/sitemap.ts) and the prerender gate
// excuses. A separate query parameter would be silently IGNORED by that same
// old backend, which would answer every sub-shard with the whole over-cap
// family instead.
type sitemapShard struct {
	// family is the entity family this shard serves a bucket of, and the key of
	// the response it populates.
	family string
	// id is the value a caller passes as `family` to request this bucket, and
	// the frontend's route segment for it.
	id string
	// buckets is how many buckets the family is cut into. Equal across every
	// shard of a family, and equal to how many shards that family has.
	buckets int
	// bucket is this shard's residue, in [0, buckets).
	bucket int
}

// sitemapShardsPerFamily is the definition: which families are sub-sharded, and
// into how many buckets each.
//
// HOW N IS CHOSEN. The bar is that a family's largest bucket stays under 50% of
// the cache-item cap with room for the corpus to double, i.e. under 25% today.
// Dividing the 2026-09-03 family shares on sitemapShard above by 8 gives 16.2%
// for releases, 10.1% for shows and 7.3% for artists; the division understates
// by about a point and a half, because each document carries its own envelope,
// so the numbers that clear the bar are the MEASURED maxima: releases 17.6%,
// shows 11.2%, artists 8.1% (the table on SHOW_SHARD_IDS in
// frontend/app/sitemap-shards.ts).
//
// So the table holds one number three times rather than three tuned numbers.
// It stays per family because the families grow at different rates and the next
// doubling will not be wanted by all three at once.
//
// The count is also what the build and the monitor pay per family: 24 sub-shard
// documents plus the seven single-shard families plus the pages shard is 32
// documents, against the 39 this scheme replaces.
//
// WHEN TO DOUBLE ONE. The frontend's data-cache budget gate fails the build at
// 80% of the cap and does not warn first, so the signal to act on is the
// measured share in a build log rather than the gate itself. Doubling a family
// re-buckets every row in it: half its URLs change document and every crawler
// refetches both documents once.
//
// DOUBLING IS FOUR CODE EDITS, each with the gate that catches skipping it, and
// then the prose. Doing only the first leaves the new ids rejected with 422,
// which the generator degrades to empty documents, and every test in this
// package passes for ANY bucket count:
//
//  1. this map;
//  2. the `enum` tag on GetSitemapEntriesRequest.Family in
//     internal/api/handlers/catalog/sitemap.go. It is a struct-tag literal huma
//     reads directly, so it cannot call SitemapFamilyValuesCSV() — print the
//     value to paste with `go test ./internal/api/handlers/catalog/ -run
//     TestSitemapFamilyEnumMatchesTheService`, whose diff is the new tag;
//  3. `bun run api:types` in frontend/, which regenerates the wire enum and is
//     enforced by the drift job in .github/workflows/ci.yml;
//  4. the id list for that family in frontend/app/sitemap-shards.ts, which tsc
//     then checks against that enum in both directions: `satisfies readonly
//     WireFamily[]` catches an id renamed or removed, AssertEveryWireValueServed
//     catches one added.
//
// Then the counts written into prose: the shard and request totals in
// frontend/app/sitemap.ts, frontend/lib/sitemap-prerender/check.ts,
// frontend/lib/sitemap-monitor/fetch.ts and the timeout derivation in
// .github/workflows/sitemap-freshness.yml. Nothing fails when those go stale.
var sitemapShardsPerFamily = map[string]int{
	"shows":    8,
	"artists":  8,
	"releases": 8,
}

// bucketShards enumerates a family's buckets in residue order.
func bucketShards(family string, buckets int) []sitemapShard {
	shards := make([]sitemapShard, 0, buckets)
	for bucket := 0; bucket < buckets; bucket++ {
		shards = append(shards, sitemapShard{
			family:  family,
			id:      sitemapShardID(family, bucket),
			buckets: buckets,
			bucket:  bucket,
		})
	}
	return shards
}

// sitemapShardID is the wire value and route segment for one bucket. The `b`
// separates the residue from the family name so no id can collide with a family
// name or with another family's id.
func sitemapShardID(family string, bucket int) string {
	return fmt.Sprintf("%s-b%d", family, bucket)
}

// sitemapShardsByFamily is every sub-sharded family's buckets, keyed by family.
//
// Keep the ids in sync with SHOW_SHARD_IDS, ARTIST_SHARD_IDS and
// RELEASE_SHARD_IDS in frontend/app/sitemap-shards.ts and with the `family`
// enum on GetSitemapEntriesRequest, the same way sitemapFamilies above is. The
// enum half is test-enforced (TestSitemapFamilyEnumMatchesTheService); the
// frontend half is enforced by `bun run api:types` regenerating the wire enum
// those lists are declared against, which fails tsc on a renamed id through
// `satisfies` and on an ADDED id through AssertEveryWireValueServed.
var sitemapShardsByFamily = buildSitemapShards()

func buildSitemapShards() map[string][]sitemapShard {
	byFamily := make(map[string][]sitemapShard, len(sitemapShardsPerFamily))
	for family, buckets := range sitemapShardsPerFamily {
		byFamily[family] = bucketShards(family, buckets)
	}
	return byFamily
}

// sitemapShards is every sub-shard, flattened in sitemapFamilies order, which
// is the order the frontend's FAMILY_BY_SHARD_ID walks, so the two tables
// enumerate identically. Ranging a map directly would put the enum in a
// different order on every run.
var sitemapShards = flattenSitemapShards()

func flattenSitemapShards() []sitemapShard {
	shards := []sitemapShard{}
	for _, family := range sitemapFamilies {
		shards = append(shards, sitemapShardsByFamily[family]...)
	}
	return shards
}

// SitemapFamilyValues is every accepted value of the `family` query parameter,
// in enum order: the entity families, then the sub-shard ids that address one
// bucket of one.
//
// Paired with SitemapFamilyValuesCSV below, mirroring
// contracts.SetTypeVocabulary / SetTypeVocabularyCSV — the same problem
// (a vocabulary that a struct tag cannot be built from) already has a shape in
// this codebase, and this follows it.
func SitemapFamilyValues() []string {
	values := make([]string, 0, len(sitemapFamilies)+len(sitemapShards))
	values = append(values, sitemapFamilies...)
	for _, shard := range sitemapShards {
		values = append(values, shard.id)
	}
	return values
}

// SitemapFamilyValuesCSV renders the accepted values as a comma-separated list,
// for the OpenAPI enum tag. The tag is a constant literal, so it cannot call
// this; TestSitemapFamilyEnumMatchesTheService is the join that keeps the two
// equal.
func SitemapFamilyValuesCSV() string {
	return strings.Join(SitemapFamilyValues(), ",")
}

// sitemapShardByID returns the bucket a sub-shard id names, or nil.
func sitemapShardByID(id string) *sitemapShard {
	for i := range sitemapShards {
		if sitemapShards[i].id == id {
			return &sitemapShards[i]
		}
	}
	return nil
}

// narrow restricts a family's scope to this shard's bucket. A nil shard is the
// whole family, which is what an unsharded `?family=releases` still asks for.
//
// The predicate is taken on the primary key, so it needs no per-family
// spelling, and it is applied to whichever scope its own family's branch in
// Entries passes in. The failure it cannot catch is a family added to
// sitemapShardsPerFamily whose branch never calls narrow: every bucket then
// serves the whole family. TestSitemapSubShardsBucketEveryFamilyExactly is what
// catches that, by asserting no shard serves the whole row set.
//
// The bucket count and residue are package-owned integers from
// sitemapShardsPerFamily, never caller input: a caller supplies only an id,
// which sitemapShardByID either resolves to one of these shards or rejects.
//
// THE PREDICATE IS NOT SARGABLE, and that is measured rather than assumed. On
// the production catalogue mirrored into a local database (28,725 release,
// 13,163 show and 13,499 artist ROWS, which is the dev seed on top of the
// production slug sets counted on sitemapShard above) the
// planner takes a sequential scan plus a quicksort per bucket: releases 4.9 ms
// and 345 shared buffers against 5.6 ms and 1,123 for the whole family through
// idx_releases_slug, shows 1.5 ms, artists 1.0 ms. So serving a family in eight
// buckets costs about 2.5x the buffer traffic of serving it whole, for roughly
// 40 ms of work per family per regeneration, all from shared buffers. An
// expression index on ((id % N)) would remove the scans and would have to be
// rebuilt on every change of N; the numbers say it buys nothing worth that.
func (shard *sitemapShard) narrow(scope *gorm.DB) *gorm.DB {
	if shard == nil {
		return scope
	}
	return scope.Where("id % ? = ?", shard.buckets, shard.bucket)
}

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
// each shard fetches `?family=…` so Next's Data Cache keys (and ~1.50 MiB
// budgets) stay independent. An unknown family is an error.
//
// family also accepts a SUB-SHARD id, which populates one family's field with
// one bucket of it: `shows-b3` is the approved shows whose primary key is 3
// modulo 8. See sitemapShard for which families are bucketed and why, and
// sitemapShardsPerFamily for how many buckets each gets. It is carried in this
// same parameter rather than a second one on
// purpose: an old backend rejects an unrecognised `family` with 422, which the
// generator already degrades to an empty shard for one deploy window
// (UNKNOWN_FAMILY_STATUSES in frontend/app/sitemap.ts). A separate parameter
// would be IGNORED by an old backend instead, which answers every sub-shard
// with the whole over-cap family and fails the build with no excuse path.
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

	// A sub-shard id resolves to the family it buckets plus the residue to
	// select, so everything downstream reasons in families only.
	//
	// THIS IS ALSO WHAT KEEPS ONE FAMILY'S BUCKET OFF ANOTHER FAMILY'S SCOPE.
	// Because family becomes the shard's own family here, the only want()
	// branch a non-nil shard can reach is that family's. The predicate is taken
	// on the primary key, which every family's table has, so a shard filed
	// under the wrong family would narrow silently rather than error: the guard
	// is the shard table being generated per family and asserted against its
	// ids, not a column that fails to resolve.
	shard := sitemapShardByID(family)
	if shard != nil {
		family = shard.family
	}

	if family != "" && !slices.Contains(sitemapFamilies, family) {
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
		//
		// A nil shard is the whole family; the bucket predicate lives on the
		// shard so the scope here stays the plain single-table projection
		// entriesFor requires.
		shows, err := s.entriesFor(
			ctx,
			shard.narrow(
				s.db.Model(&catalogm.Show{}).Where("status = ?", catalogm.ShowStatusApproved),
			),
		)
		if err != nil {
			return nil, fmt.Errorf("failed to collect show sitemap entries: %w", err)
		}
		out.Shows = shows
	}

	if want("artists") {
		// A nil shard is the whole family; the bucket predicate lives on the
		// shard so the scope here stays the plain single-table projection
		// entriesFor requires.
		artists, err := s.entriesFor(ctx, shard.narrow(s.db.Model(&catalogm.Artist{})))
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

	// scenes and scene_weeks are two projections of ONE group set: the same
	// eligibility decides a scene's root URL and its week permalinks, so the two
	// families cannot apply different floors. They are still two documents
	// fetched as two requests, so a scene approved between them can appear in one
	// and not the other until the next build; what this rules out is a rule
	// difference, not a timing skew.
	if want("scenes") || want("scene_weeks") {
		groups, err := s.listQualifyingScenes(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to list qualifying scenes: %w", err)
		}
		if want("scenes") {
			out.Scenes = s.sceneEntries(groups)
		}
		if want("scene_weeks") {
			weeks, err := s.sceneWeekEntries(ctx, groups)
			if err != nil {
				return nil, fmt.Errorf("failed to collect scene-week sitemap entries: %w", err)
			}
			out.SceneWeeks = weeks
		}
	}

	if want("labels") {
		labels, err := s.entriesFor(ctx, s.db.Model(&catalogm.Label{}))
		if err != nil {
			return nil, fmt.Errorf("failed to collect label sitemap entries: %w", err)
		}
		out.Labels = labels
	}

	if want("releases") {
		// A nil shard is the whole family; the bucket predicate lives on the
		// shard so the scope here stays the plain single-table projection
		// entriesFor requires.
		releases, err := s.entriesFor(ctx, shard.narrow(s.db.Model(&catalogm.Release{})))
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
// to watch as the catalogue grows, neither of which is close today: the ~1.50 MiB
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

// listQualifyingScenes returns the same threshold-gated scene set as
// ListScenes (2+ verified venues, 3+ approved shows), without the genre /
// coordinate hydration that list endpoint pays for. Rows are SQL groups, one
// per sceneGroupKeySQL key, so a caller ENUMERATING scenes must collapse them
// to published identity before using one.
//
// The two counts are projected for sceneGroupOutranks, which is the only thing
// that reads them. They do not decide either collision shape the geo dataset
// can currently produce: a CBSA group beats a fallback group on the scope test,
// and two fallback groups are separated by their city minima. They are here so
// that a shape reaching the count tiebreak ties the way the directory does,
// since they are the same aggregates ListScenes selects over the same grouping.
//
// updated_at is the group's newest APPROVED show, which sceneEntries publishes
// as the root URL's lastmod. It aggregates over the join's show side, so a
// showless venue's row contributes nothing to it. shows.updated_at is NOT NULL
// and the grouping's show floor keeps at least one show row per surviving
// group, so the aggregate is non-null on every row scanned here.
func (s *SitemapService) listQualifyingScenes(ctx context.Context) ([]sceneVenueGroup, error) {
	var groups []sceneVenueGroup
	err := s.db.WithContext(ctx).Raw(`
		SELECT `+sceneGroupIdentitySQL+`,
		       COUNT(DISTINCT v.id) AS venue_count,
		       COUNT(DISTINCT s.id) AS show_count,
		       MAX(s.updated_at)    AS updated_at`+
		sceneQualifyingGroupingSQL,
		catalogm.ShowStatusApproved, sceneMinVenues, sceneMinShows).Scan(&groups).Error
	if err != nil {
		return nil, err
	}
	return groups, nil
}

// sceneEntries projects one SitemapEntry per qualifying scene. Scenes are
// computed aggregations (no scenes.updated_at), so lastmod is the resolving
// group's newest approved show — the closest durable signal to "this page's
// content changed".
//
// A slug collision publishes one root URL whichever group survives, so what the
// collapse decides here is only whose lastmod that URL carries.
func (s *SitemapService) sceneEntries(groups []sceneVenueGroup) []contracts.SitemapEntry {
	unique := collapseSceneGroupsToCanonicalSlug(groups, s.geocoder, "sitemap-scenes")

	entries := make([]contracts.SitemapEntry, 0, len(unique))
	for _, grp := range unique {
		entries = append(entries, contracts.SitemapEntry{
			Slug:      sceneGroupSlug(grp),
			UpdatedAt: grp.UpdatedAt,
		})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Slug < entries[j].Slug })
	return entries
}

// sceneWeekEntries projects archived week permalinks for the last
// sceneWeekSitemapWindow weeks per qualifying scene, excluding weeks with zero
// approved shows. Slug is the composite "{scene-slug}/{iso-week}" matching the
// canonical URL sceneWeekPage.tsx already declares.
//
// Week boundaries are resolved in each scene's own timezone — the same rule
// GetSceneWeek uses — so a show at 21:00 Sunday Chicago does not fall into the
// wrong ISO week when bucketed in UTC.
//
// It takes the group set rather than fetching one, so Entries hands the same
// rows to both scene projections. That also lets a test choose the order a
// contested slug's groups arrive in, which is the only thing that tells
// delegating the winner choice below apart from taking the scan's first row:
// the two differ ONLY on a loser-first arrival, and which row a GROUP BY hands
// over first is the planner's to decide.
func (s *SitemapService) sceneWeekEntries(ctx context.Context, groups []sceneVenueGroup) ([]contracts.SitemapEntry, error) {
	// Two venue groups can resolve to the same display slug, and here the
	// survivor's identity builds the query scope below rather than only naming a
	// row: that scope selects the rooms whose shows become the week permalinks.
	// collapseSceneGroupsToCanonicalSlug carries the rule and the reasoning.
	unique := collapseSceneGroupsToCanonicalSlug(groups, s.geocoder, "sitemap-scene-weeks")

	entries := make([]contracts.SitemapEntry, 0, len(unique)*sceneWeekSitemapWindow)
	for _, grp := range unique {
		city, state := metroDisplayIdentity(grp.Metro, grp.City, grp.State)
		slug := buildSceneSlug(city, state)
		scope := metroScopeFor(s.geocoder, city, state)
		loc := s.sceneLocation(scope, state)

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
				Slug:      slug + "/" + key,
				UpdatedAt: agg.updatedAt,
			})
		}
	}

	sort.Slice(entries, func(i, j int) bool { return entries[i].Slug < entries[j].Slug })
	return entries, nil
}

// sceneLocation mirrors the LOCATION half of SceneService.sceneLocation: modal
// verified-venue timezone, falling back to the state map. Duplicated rather than
// shared so SitemapService does not grow a SceneService dependency for one query.
//
// It returns no publishable zone name, because a sitemap names no zone: its
// dates are lastmod stamps read on the best clock available, and a surrendered
// fallback is still the best clock for that.
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
