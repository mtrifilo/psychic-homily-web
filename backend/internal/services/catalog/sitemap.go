package catalog

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"

	"psychic-homily-backend/db"
	"psychic-homily-backend/internal/logger"
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
	"shows", "artists", "venues", "venue_years", "shows_months", "venue_cities",
	"scenes", "scene_weeks", "labels", "releases", "festivals", "tags",
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

// wireID is the id a caller requested this shard by, or "" for a nil shard,
// which is the whole family.
func (shard *sitemapShard) wireID() string {
	if shard == nil {
		return ""
	}
	return shard.id
}

// familyShards is how many requests together serve this shard's family: its
// bucket count, or 1 for a nil shard, which serves the whole family at once.
func (shard *sitemapShard) familyShards() int {
	if shard == nil {
		return 1
	}
	return shard.buckets
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
	diet     sitemapDiet
}

// NewSitemapService reads the diet flags once, here, so a change to one takes
// effect at the next process start.
func NewSitemapService(database *gorm.DB) *SitemapService {
	if database == nil {
		database = db.GetDB()
	}
	diet := sitemapDietFromEnv()
	slog.Default().Info("sitemap diet rules",
		sitemapPastShowMaxAgeDaysEnv, diet.pastShowMaxAgeDays,
		sitemapOmitUnlinkedReleasesEnv, diet.omitUnlinkedReleases,
		sitemapOmitIdleArtistsEnv, diet.omitIdleArtists,
	)
	return &SitemapService{db: database, geocoder: geo.Default(), diet: diet}
}

// The diet flags. Each switches on one rule that stops advertising a set of
// rows in one family. The pages those rows address are untouched: they stay
// served, indexable and linked, and every other family keeps announcing what it
// announces with the rule off.
const (
	// sitemapPastShowMaxAgeDaysEnv holds a whole number of days; unset, zero,
	// negative or malformed is off.
	sitemapPastShowMaxAgeDaysEnv = "SITEMAP_PAST_SHOW_MAX_AGE_DAYS"
	// sitemapOmitUnlinkedReleasesEnv and sitemapOmitIdleArtistsEnv are on only
	// for "1", per shared.EnvFlagEnabled.
	sitemapOmitUnlinkedReleasesEnv = "SITEMAP_OMIT_UNLINKED_RELEASES"
	sitemapOmitIdleArtistsEnv      = "SITEMAP_OMIT_IDLE_ARTISTS"
)

// sitemapPastShowMaxAgeDaysLimit is the largest age the past-show rule accepts.
// Postgres raises "date out of range" when today minus the age falls before
// 4713 BC, which would fail the whole shows family, so a larger value is read
// as off. A million days reaches back to the eighth century, which no show
// predates.
const sitemapPastShowMaxAgeDaysLimit = 1_000_000

// sitemapDiet is which diet rules are on. The zero value is every rule off, and
// with every rule off each family issues the same statement it issues without
// this type.
type sitemapDiet struct {
	// pastShowMaxAgeDays omits approved shows whose venue-local date is more
	// than this many days before the venue-local today. 0 is off.
	pastShowMaxAgeDays int
	// omitUnlinkedReleases omits releases with no external link.
	omitUnlinkedReleases bool
	// omitIdleArtists omits artists on no show bill and no release.
	omitIdleArtists bool
}

// sitemapDietFromEnv warns on an age that is set but read as off, other than an
// explicit zero, so a mistyped value is visible in the log rather than only as
// a zero in the startup line.
func sitemapDietFromEnv() sitemapDiet {
	raw := strings.TrimSpace(os.Getenv(sitemapPastShowMaxAgeDaysEnv))
	days := shared.EnvPositiveInt(sitemapPastShowMaxAgeDaysEnv, 0)
	if days > sitemapPastShowMaxAgeDaysLimit {
		days = 0
	}
	if n, err := strconv.Atoi(raw); days == 0 && raw != "" && (err != nil || n != 0) {
		slog.Default().Warn("sitemap past-show max age is not a whole number from 1 to the limit; rule is off",
			"key", sitemapPastShowMaxAgeDaysEnv,
			"value", raw,
			"limit", sitemapPastShowMaxAgeDaysLimit,
		)
	}
	return sitemapDiet{
		pastShowMaxAgeDays:   days,
		omitUnlinkedReleases: shared.EnvFlagEnabled(sitemapOmitUnlinkedReleasesEnv),
		omitIdleArtists:      shared.EnvFlagEnabled(sitemapOmitIdleArtistsEnv),
	}
}

// sitemapOmission is one diet rule as it applies to its family's scan. The
// zero value is a rule that is off.
type sitemapOmission struct {
	// rule is the flag that switches the rule on, and the name its build log
	// line carries.
	rule string
	// family is the sitemap family the rule removes rows from, as its log line
	// names it.
	family string
	// condition is a boolean SQL expression over the family's own table and
	// join, true for a row the rule stops advertising. Empty means the rule is
	// off. It carries no bind parameters.
	condition string
	// join is a relation condition reads, joined to the family scan only while
	// the rule is on. Empty for a condition over the family's table alone.
	//
	// The family scan names id, slug, updated_at and status unqualified, so a
	// join must project no column by any of those names, or the sub-shard and
	// slug predicates turn ambiguous. It must also yield at most one row per
	// family row, or URLs repeat and the omitted count inflates. shared.VenueTZJoin
	// is both: a LIMIT 1 lateral whose columns are all venue_tz_-prefixed.
	join string
}

// pastShows is the shows family's rule: an approved show is omitted once its
// venue-local date is more than pastShowMaxAgeDays before the venue-local today,
// on the clock shared.VenueLocalDateOlderThanDaysSQL owns, which is the one
// venueYearEntries files the show under in its venue's archive.
//
// The zone comes from shared.VenueTZJoin joined to the family scan itself,
// rather than from a subquery over all shows, so a sub-shard request resolves
// zones for its own bucket's rows only.
//
// COST. With the rule on, the shows family resolves a zone for every approved
// show in the request, upcoming and past: one show_venues index hop per row,
// with the zone-snapshot SubPlan at loops=1. Measured on Postgres 18 over
// 13,172 synthetic shows: about 3 ms for one sub-shard and about 37 ms for the
// whole family, against about 1 ms and 2.5 ms with the rule off. Like
// venue_years, its input grows with the show archive.
func (d sitemapDiet) pastShows() sitemapOmission {
	if d.pastShowMaxAgeDays <= 0 {
		return sitemapOmission{}
	}
	return sitemapOmission{
		rule:      sitemapPastShowMaxAgeDaysEnv,
		family:    "shows",
		condition: shared.VenueLocalDateOlderThanDaysSQL(d.pastShowMaxAgeDays),
		join:      shared.VenueTZJoin,
	}
}

// unlinkedReleases is the releases family's rule: a release with no row in
// release_external_links is omitted. Any row counts, whatever its platform:
// every linked release in production carries a Bandcamp or Spotify row
// (measured 2026-10-24), so a platform filter would select the same set.
func (d sitemapDiet) unlinkedReleases() sitemapOmission {
	if !d.omitUnlinkedReleases {
		return sitemapOmission{}
	}
	return sitemapOmission{
		rule:      sitemapOmitUnlinkedReleasesEnv,
		family:    "releases",
		condition: "NOT EXISTS (SELECT 1 FROM release_external_links rel WHERE rel.release_id = releases.id)",
	}
}

// idleArtists is the artists family's rule: an artist with no show_artists row
// and no artist_releases row is omitted. A show_artists row of any show status
// counts, so an artist billed only on a pending show stays advertised.
func (d sitemapDiet) idleArtists() sitemapOmission {
	if !d.omitIdleArtists {
		return sitemapOmission{}
	}
	return sitemapOmission{
		rule:   sitemapOmitIdleArtistsEnv,
		family: "artists",
		condition: "NOT EXISTS (SELECT 1 FROM show_artists sa WHERE sa.artist_id = artists.id)" +
			" AND NOT EXISTS (SELECT 1 FROM artist_releases ar WHERE ar.artist_id = artists.id)",
	}
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
		Shows:       []contracts.SitemapEntry{},
		Artists:     []contracts.SitemapEntry{},
		Venues:      []contracts.SitemapEntry{},
		VenueYears:  []contracts.SitemapEntry{},
		ShowsMonths: []contracts.SitemapEntry{},
		VenueCities: []contracts.SitemapEntry{},
		Scenes:      []contracts.SitemapEntry{},
		SceneWeeks:  []contracts.SitemapEntry{},
		Labels:      []contracts.SitemapEntry{},
		Releases:    []contracts.SitemapEntry{},
		Festivals:   []contracts.SitemapEntry{},
		Tags:        []contracts.SitemapEntry{},
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
		// shard so the scope here stays the single-table projection
		// entriesOmitting requires. A diet rule adds its own join, if any.
		shows, err := s.entriesOmitting(
			ctx,
			shard.narrow(
				s.db.Model(&catalogm.Show{}).Where("status = ?", catalogm.ShowStatusApproved),
			),
			shard, s.diet.pastShows(), rowLastmodSQL,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to collect show sitemap entries: %w", err)
		}
		out.Shows = shows
	}

	if want("artists") {
		// A nil shard is the whole family; the bucket predicate lives on the
		// shard so the scope here stays the single-table projection
		// entriesOmitting requires. A diet rule adds its own join, if any.
		artists, err := s.entriesOmitting(
			ctx, shard.narrow(s.db.Model(&catalogm.Artist{})),
			shard, s.diet.idleArtists(), rowLastmodSQL,
		)
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

	if want("shows_months") {
		showsMonths, err := s.showsMonthEntries(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to collect shows-month sitemap entries: %w", err)
		}
		out.ShowsMonths = showsMonths
	}

	if want("venue_cities") {
		venueCities, err := s.venueCityEntries(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to collect venue-city sitemap entries: %w", err)
		}
		out.VenueCities = venueCities
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
		// shard so the scope here stays the single-table projection
		// entriesOmitting requires. A diet rule adds its own join, if any.
		releases, err := s.entriesOmitting(
			ctx, shard.narrow(s.db.Model(&catalogm.Release{})),
			shard, s.diet.unlinkedReleases(), releaseLastmodSQL,
		)
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
// updated_at. A family needing a join wants its own projection, not this. The
// one exception is a diet rule's join, which entriesOmitting adds only while
// the rule is on and only under the contract on sitemapOmission.join.
func (s *SitemapService) entriesFor(ctx context.Context, scope *gorm.DB) ([]contracts.SitemapEntry, error) {
	return s.entriesWithLastmod(ctx, scope, rowLastmodSQL)
}

// rowLastmodSQL is the lastmod of a family whose page renders its own row and
// nothing whose changes that row misses: the row's updated_at.
const rowLastmodSQL = "updated_at"

// releaseLastmodSQL is the releases family's lastmod: the later of the
// release row's updated_at and the newest release_external_links.created_at
// for it. A link renders on the release page (a listen embed or an outbound
// link) and adding one does not touch releases.updated_at, so the row alone
// misses it. GREATEST skips the NULL a linkless release's subquery yields.
//
// It reads link CREATION only. A deleted link leaves no row to date it by, so
// removing a link does not move lastmod; neither does any link column changing
// in place, since the table has no updated_at.
//
// It is a correlated scalar subquery, not a join, so the family stays one
// single-table scan yielding one row per release, with one probe of
// idx_release_external_links_release_id per row. Its cost therefore scales
// with the rows a request returns, so a sub-shard pays for its own bucket only.
//
// COST. Measured on Postgres 18 over 37,688 synthetic releases and 10,132
// links: about 5.5 ms for one sub-shard and about 60 ms for the whole family,
// against about 3 ms and 9 ms for the row's updated_at alone.
const releaseLastmodSQL = "GREATEST(releases.updated_at, (SELECT MAX(rel_lastmod.created_at) FROM release_external_links rel_lastmod WHERE rel_lastmod.release_id = releases.id))"

// entriesWithLastmod is entriesFor with lastmod taken from lastmodSQL, an SQL
// expression over the scope's single table that carries no bind parameters.
func (s *SitemapService) entriesWithLastmod(ctx context.Context, scope *gorm.DB, lastmodSQL string) ([]contracts.SitemapEntry, error) {
	// Deterministic order, so two fetches of an unchanged catalogue diff
	// cleanly. Ordered by slug rather than recency because the partial unique
	// index on slug (migration 000013) can supply that order, while no index on
	// updated_at exists for any of these tables — sorting on it would buy a
	// guaranteed sort node for an ordering no consumer reads.
	entries := []contracts.SitemapEntry{}
	err := advertisableRows(ctx, scope).
		Select("slug, " + lastmodSQL + " AS updated_at").
		Scan(&entries).Error
	if err != nil {
		return nil, err
	}
	return entries, nil
}

// advertisableRows narrows a family scope to the rows entriesFor advertises, in
// the order it lists them. It is the one statement of both, so entriesOmitting
// cannot drift from entriesFor on either.
func advertisableRows(ctx context.Context, scope *gorm.DB) *gorm.DB {
	return scope.
		WithContext(ctx).
		Where("slug IS NOT NULL AND slug <> ''").
		Order("slug ASC")
}

// entriesOmitting is entriesWithLastmod with one diet rule applied, and the
// scope carries the same single-table constraint. The rule's own join is the one
// relation added, and only while the rule is on; see sitemapOmission.join.
//
// With the rule off it IS entriesFor: the same statement, so the same rows in
// the same order. With it on, the rule's condition is projected as a column of
// that one scan and the rows it marks are dropped here, which keeps a family at
// one statement and makes the logged count exactly the set left out of the
// response. The count covers only rows that would otherwise be advertised,
// because the slug predicate is applied before the projection.
//
// A rule that is on logs one sitemap_diet_omitted line per call, and every
// request writes one, whoever sent it: the generator, the freshness monitor or
// any other caller of the public endpoint. So a line is a snapshot of one
// request, and lines are not additive over time. A family's current total is
// the sum, over the family_shards distinct shard ids, of each shard's latest
// line. A line with an empty shard answered a whole-family request, is a
// total on its own, and carries family_shards 1.
func (s *SitemapService) entriesOmitting(
	ctx context.Context,
	scope *gorm.DB,
	shard *sitemapShard,
	omission sitemapOmission,
	lastmodSQL string,
) ([]contracts.SitemapEntry, error) {
	if omission.condition == "" {
		return s.entriesWithLastmod(ctx, scope, lastmodSQL)
	}

	type row struct {
		contracts.SitemapEntry
		Omitted bool `gorm:"column:sitemap_omitted"`
	}
	if omission.join != "" {
		scope = scope.Joins(omission.join)
	}
	var rows []row
	err := advertisableRows(ctx, scope).
		Select("slug, " + lastmodSQL + " AS updated_at, (" + omission.condition + ") AS sitemap_omitted").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}

	entries := make([]contracts.SitemapEntry, 0, len(rows))
	omitted := 0
	for _, r := range rows {
		if r.Omitted {
			omitted++
			continue
		}
		entries = append(entries, r.SitemapEntry)
	}

	logger.FromContext(ctx).Info("sitemap_diet_omitted",
		"rule", omission.rule,
		"family", omission.family,
		"shard", shard.wireID(),
		"family_shards", shard.familyShards(),
		"omitted", omitted,
		"advertised", len(entries),
	)
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
// COST, recorded rather than discovered later. This family is one of two in
// Entries that run a venue-zone lateral over an input that grows without
// bound: it scans every approved PAST show (the coarse `event_date < now()`
// bound prunes nothing on an archive) with one lateral execution per row, and
// past shows never age out. The other is the shows family while its past-show
// diet rule is on; see sitemapDiet.pastShows for its measured cost. The
// remaining families are either single-table scans (entriesFor) or windowed
// (sceneWeekEntries, 8 weeks per scene). Two thresholds
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

// showsMonthSitemapMaxYear is the last year this family may announce, and it is
// the upper half of the bound the month route applies to its year segment.
//
// A show can carry any event_date the submitter typed, so without this a single
// mistyped year publishes a URL the frontend answers with a hard 404: the route
// refuses a year outside its bound before it renders, and a sitemap must never
// announce an address the site refuses.
//
// ONLY the upper half is stated here. The lower one cannot fire: the upcoming
// condition below already excludes every date behind today, so no surviving row
// can sit below the current year.
const showsMonthSitemapMaxYear = 2100

// showsMonthEntries projects one SitemapEntry per venue-local calendar month
// that has at least one approved UPCOMING show: the crawlable month pages at
// /shows/{year}/{month}.
//
// UPCOMING only, and that is the whole definition of the surface rather than a
// filter applied to it. The set this emits must equal the set
// GetUpcomingShowMonths returns for an unfiltered public read, MINUS the months
// past showsMonthSitemapMaxYear. A month announced here is a month the frontend
// must both serve and index: a month with no upcoming shows still renders, but
// it carries `noindex`, so announcing one would advertise a URL the site itself
// asks crawlers to skip. A month past the bound is refused on shape alone.
//
// THE EQUALITY IS HELD BY A TEST, NOT BY CONSTRUCTION. This query restates the
// partition rather than composing the applier the histogram builds on, so a
// predicate added there does not reach here. The integration test alongside
// this file asserts the two sets match; it is the guard when either side moves.
//
// UNFILTERED, like the histogram. A reader's city filter is a query param on
// these URLs, not part of their identity.
//
// Unlike the entity families this is NOT one row per table row, so entriesFor
// cannot serve it: the grain is (year, month) and the slug is composite.
// UpdatedAt is MAX(show.updated_at) within the bucket, the closest durable
// "this page's content changed" signal, matching venueYearEntries.
//
// The month is ZERO-PADDED because the route accepts exactly that spelling
// (/shows/2026/09, never /shows/2026/9), so an unpadded slug here would
// announce a URL that 404s on shape alone.
//
// One grouped scan of the approved UPCOMING shows with the venue-zone lateral,
// over a set that is small and self-limiting since shows leave it as they
// happen, emitting one row per month that has shows.
func (s *SitemapService) showsMonthEntries(ctx context.Context) ([]contracts.SitemapEntry, error) {
	type row struct {
		Year      int       `gorm:"column:year"`
		Month     int       `gorm:"column:month"`
		UpdatedAt time.Time `gorm:"column:updated_at"`
	}

	var rows []row
	// The lateral in VenueTZJoin correlates on shows.id, so the model must be
	// shows when it lands. It inherits the same deliberate asymmetry every other
	// venue-local surface does: a show billed at two venues is bucketed in the
	// PRIMARY venue's zone.
	err := s.db.WithContext(ctx).
		Model(&catalogm.Show{}).
		Joins(shared.VenueTZJoin).
		Where("shows.status = ?", catalogm.ShowStatusApproved).
		Where(shared.VenueLocalDateCondition("upcoming")).
		Where(shared.VenueLocalYearSQL+" <= ?", showsMonthSitemapMaxYear).
		Select(shared.VenueLocalYearSQL + ` AS "year", ` + shared.VenueLocalMonthSQL + ` AS "month", MAX(shows.updated_at) AS updated_at`).
		Group(shared.VenueLocalYearSQL + ", " + shared.VenueLocalMonthSQL).
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}

	entries := make([]contracts.SitemapEntry, 0, len(rows))
	for _, r := range rows {
		entries = append(entries, contracts.SitemapEntry{
			Slug:      fmt.Sprintf("%04d/%02d", r.Year, r.Month),
			UpdatedAt: r.UpdatedAt,
		})
	}
	// Deterministic order, so two fetches of an unchanged catalogue compare
	// cleanly. The zero-padded slug sorts chronologically as a string, which the
	// year-then-month SQL order would also give; sorting the assembled slug is
	// what makes that a property of the emitted document rather than of the query
	// plan.
	sort.Slice(entries, func(i, j int) bool { return entries[i].Slug < entries[j].Slug })
	return entries, nil
}

// venueCityEntries projects one SitemapEntry per city that holds at least one
// verified room: the directory scoped to that city, at /venues?cities=City,ST.
//
// THE SLUG IS A QUERY VALUE, NOT A PATH TAIL, and it is emitted RAW. The
// generator percent-encodes it once on its way into the document; see the
// query-slug note on contracts.SitemapEntry for why the encoding lives there.
//
// THE SET IS THE DIRECTORY'S OWN CITY FACET, and it is drawn through the same
// applier rather than through a restated predicate: venueListPredicates over
// venueCitiesScope is what GetVenueCities reads, and venue.go records that the
// applier is what stops the page, its total and its facet from describing three
// different sets. Announcing a city the picker does not offer would publish a
// URL the page answers with noindex. This shares that applier with the facet
// and the list; the `venues` family above does NOT, so a condition added to the
// applier reaches these three and not that one.
//
// EVERY SLUG MUST SURVIVE THE PAGE'S OWN PARSER, which is the one way this
// narrows the facet, and addressableCityFilter is its single owner: an empty
// half, a half that is not its own trimmed self, and a half carrying a comma or
// a pipe all produce a value parseCitiesParam either drops or splits
// differently, and the announced URL then serves the unfiltered directory or a
// page that asks not to be indexed. The empty case is deliberately NOT also
// excluded in SQL, so every reason has one place to be read and one place to be
// tested. GetVenueCities keeps such rows, because its numbers have to sum to
// the list's total; a sitemap cannot, because a <loc> is a promise.
//
// UpdatedAt is MAX(venue.updated_at) within the city: the room rows ARE this
// page's content, so its own edits are the signal, where venueYearEntries and
// showsMonthEntries take theirs from the shows they list.
//
// The grain is (city, state), so entriesFor cannot serve it.
func (s *SitemapService) venueCityEntries(ctx context.Context) ([]contracts.SitemapEntry, error) {
	type row struct {
		City      string    `gorm:"column:city"`
		State     string    `gorm:"column:state"`
		UpdatedAt time.Time `gorm:"column:updated_at"`
	}

	browsable := NewVenueService(s.db).venueListPredicates(
		venueCitiesScope(contracts.VenueListFilters{}),
	)

	var rows []row
	err := browsable(s.db.WithContext(ctx).Table("venues")).
		Select("venues.city AS city, venues.state AS state, MAX(venues.updated_at) AS updated_at").
		Group("venues.city, venues.state").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}

	entries := make([]contracts.SitemapEntry, 0, len(rows))
	for _, r := range rows {
		if !addressableCityFilter(r.City) || !addressableCityFilter(r.State) {
			continue
		}
		entries = append(entries, contracts.SitemapEntry{
			Slug:      r.City + "," + r.State,
			UpdatedAt: r.UpdatedAt,
		})
	}
	// Sorted on the ASSEMBLED slug rather than in SQL, because the two orders
	// differ wherever one city name is a prefix of another: ',' sorts below
	// every letter, so "Mesa,AZ" precedes "Mesa Verde,CO".
	sort.Slice(entries, func(i, j int) bool { return entries[i].Slug < entries[j].Slug })
	return entries, nil
}

// addressableCityFilter reports whether one half of a "City,ST" filter value
// survives a round trip through the frontend's parseCitiesParam.
//
// That parser splits on "|" into pairs and on "," into exactly two halves, and
// trims each. So a half carrying either separator, a half that is not its own
// trimmed self, and an empty half all name something other than the row they
// came from.
func addressableCityFilter(half string) bool {
	return half != "" &&
		half == strings.TrimSpace(half) &&
		!strings.ContainsAny(half, ",|")
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

// sitemapQualifyingScenesSQL is the scene set the sitemap publishes, over the
// same grouping the directory uses. Rendered once at init: no runtime input,
// and no zone lateral, since nothing here is venue-local.
var sitemapQualifyingScenesSQL = `
		SELECT ` + sceneGroupIdentitySQL + `,
		       COUNT(DISTINCT v.id)     AS venue_count,
		       COUNT(DISTINCT shows.id) AS show_count,
		       MAX(shows.updated_at)    AS updated_at` +
	sceneQualifyingGroupingSQL(noExtraJoins)

func (s *SitemapService) listQualifyingScenes(ctx context.Context) ([]sceneVenueGroup, error) {
	var groups []sceneVenueGroup
	err := s.db.WithContext(ctx).Raw(sitemapQualifyingScenesSQL,
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
		// The winner's own key builds the scope, so the weeks published under a
		// slug are the weeks of the rooms that group holds, which are the rooms
		// the page that slug serves is scoped to.
		scope := sceneScopeForGroup(grp)
		slug := sceneGroupSlug(grp)
		loc := s.sceneLocation(scope, scope.state)

		nowLocal := time.Now().In(loc)
		y, w := nowLocal.ISOWeek()
		currentStart := ISOWeekStart(y, w, loc)
		windowStart := currentStart.AddDate(0, 0, -7*(sceneWeekSitemapWindow-1))
		windowEnd := currentStart.AddDate(0, 0, 7) // half-open end of current week

		// trackedVenuePredicate, the room set GetSceneWeek draws its rows over:
		// a week whose only shows sit at unverified rooms renders empty, so
		// publishing a permalink for it would index a page with nothing on it.
		vp, vargs := trackedVenuePredicate(scope, "v")
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
	vp, vargs := trackedVenuePredicate(scope, "v")
	var tz string
	err := s.db.Raw(`
		SELECT v.timezone
		FROM venues v
		WHERE `+vp+`
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
