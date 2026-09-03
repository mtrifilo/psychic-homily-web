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

// sitemapShard is one slice of an entity family, addressable on the wire as if
// it were a family of its own.
//
// WHY FAMILIES GET SUB-SHARDED. Sharding the sitemap per family (PSY-1622)
// exists to keep each Next Data Cache entry under the per-item cap of 2 MiB —
// 2,097,152 bytes, MEBIbytes, which is the unit the frontend's
// lib/data-cache-budget/budget.ts insists on for this route family precisely
// because decimal MB put the label and the arithmetic in disagreement. The
// entry holds the response BASE64-encoded, so the raw body that fits is
// 1,572,864 bytes, and the frontend's gate fails the build at 80% of the cap.
// Two families have outgrown a single entry:
//
//   - releases (PSY-1763): 1,530,206 raw bytes over 21,525 rows on 2026-08-09,
//     97.3% of the cap once encoded. Served in SLUG ranges.
//   - shows (PSY-2018): 1,267,618 raw bytes over 13,096 rows on 2026-09-03,
//     81% of the cap once encoded, which is what blocked the production
//     frontend build. Served in UTC EVENT-MONTH ranges.
//
// WHAT A PARTITION KEY HAS TO BE. Stable first: a row that changes shard churns
// what crawlers refetch, for no new information. Then balanced, and balanced as
// the corpus grows rather than only on the day it was cut.
//
//   - A page number (OFFSET/LIMIT over the row order) is perfectly balanced and
//     maximally unstable — one insert near the front shifts every row after it
//     across every page boundary, on every import.
//   - A slug range is stable when the slug is regenerated only by a rename that
//     changes the URL anyway, which is the releases case (see
//     ReleaseService.UpdateRelease), and it stays balanced because the letter
//     mix of titles is a property of how records get named. Measured on
//     releaseShards.
//   - A calendar range over the event date is stable for shows because the date
//     is the fact the URL is built from, and every past bucket is frozen once
//     its month ends. It is NOT self-balancing — the live buckets grow and the
//     old ones do not — so the grain is chosen small enough that a fully
//     ingested bucket still fits, rather than re-cut as the mix moves. Measured
//     on showShards.
//
// HOW IT GROWS. Add a cut point and split ONE range. Every other range keeps
// both its id and its exact contents, so re-tuning churns only the range being
// split — the property page-numbering cannot offer at any shard count. When the
// largest range approaches the warn band in the frontend's
// lib/data-cache-budget/budget.ts, split that one.
//
// Each id doubles as the `family` query value the backend accepts for that
// range. Deliberately: a backend that predates a new id answers 422, which the
// generator degrades to an empty shard for one deploy window
// (UNKNOWN_FAMILY_STATUSES in frontend/app/sitemap.ts) and the prerender gate
// excuses. A separate query parameter would be silently IGNORED by that same
// old backend, which would answer every sub-shard with the whole over-cap
// family instead.
type sitemapShard struct {
	// family is the entity family this shard serves a slice of, and the key of
	// the response it populates.
	family string
	// id is the value a caller passes as `family` to request this slice, and
	// the frontend's route segment for it. It is a legible label for the span,
	// NOT the predicate.
	id string
	// column is the projected column the half-open range is taken on. A
	// package-owned literal, never caller input, and it must exist on the one
	// table its family's scope resolves to (see entriesFor).
	column string
	// from is the inclusive lower bound. nil is unbounded below, which is what
	// keeps a partition total no matter how its values order.
	from any
	// before is the exclusive upper bound. nil is unbounded above.
	before any
}

// releaseShards partitions the releases family by slug. Half-open ranges, open
// at both outer ends, so every slug lands in exactly one — asserted by
// TestSitemapShardsPartitionTheirFamilies rather than left to inspection.
//
// Cut points chosen to minimise the largest range's byte share over the
// measured production catalogue. Serving them from a database holding the real
// production rows, the four shards answered 422,209 / 421,070 / 366,874 /
// 320,964 bytes — 27.6% / 27.5% / 24.0% / 21.0% of the family, and 26.9% /
// 26.8% / 23.4% / 20.4% of the cache item cap once the frontend had written
// them, against 97.3% for the family as a single document.
//
// That split is a property of how records get titled, not of this particular
// import, and two further readings of the same production data say so. The same
// cut points split the ARTISTS corpus — a disjoint set of 9,405 names, run
// through the same predicates in the same database — 28.0 / 31.1 / 23.1 / 17.7.
// And splitting the releases themselves into an older and a newer half by
// updated_at moves no bucket by more than 1.9 points (27.4 / 27.1 / 23.5 / 21.9
// against 27.7 / 27.9 / 24.4 / 20.0), so the mix is not drifting as the
// catalogue grows.
//
// THE BOUNDS ARE EVALUATED BY THE DATABASE'S COLLATION, NOT AS PREFIXES — read
// this before re-cutting the ranges. Production and the test containers run
// en_US.utf8 (libc), which gives punctuation a lower weight than letters at the
// primary level, so a slug does NOT necessarily land in the range its first
// character suggests. Measured, against both:
//
//	'1999-remastered' < 'f'          → true   (digits sort below every letter)
//	'-quiet-start'   >= 'n'          → true   (collates as "quietstart")
//	'Eclair'          < 'f'          → true   (case-insensitive at this level)
//
// So `-quiet-start` is served by releases-n-s, not by the first range. That is
// harmless — totality and disjointness come from the ranges being contiguous
// and open at both outer ends, which holds under ANY total order — but it means
// new cut points must be measured with these same predicates, NOT read off a
// `SELECT left(slug, 1)` histogram, or the shares will not come out as planned.
//
// It also bounds the stability claim above: "stable by construction" holds for
// a FIXED collation. A glibc upgrade or base-image bump can reorder strcoll
// (the hazard Postgres tracks as pg_collation.collversion), which would move
// some rows between ranges with no slug and no URL change. The integration test
// pins the placements of the awkward cases so that surfaces as a red build
// rather than as silent crawler churn.
var releaseShards = []sitemapShard{
	{family: "releases", id: "releases-a-e", column: "slug", before: "f"},
	{family: "releases", id: "releases-f-m", column: "slug", from: "f", before: "n"},
	{family: "releases", id: "releases-n-s", column: "slug", from: "n", before: "t"},
	{family: "releases", id: "releases-t-z", column: "slug", from: "t"},
}

// showShardColumn is the column the shows partition is cut on.
//
// UTC, and that is the whole rule: Show.EventDate is a UTC instant and the
// bounds are UTC month starts, so a show belongs to the bucket its UTC event
// month names. Deliberately NOT the venue-local bucketing venueYearEntries
// uses. That family puts its bucket IN the URL, so it has to agree with the
// page it points at; this one appears in no URL, and a partition key only has
// to be total, disjoint and stable. Venue-local bucketing here would buy
// nothing and cost a join entriesFor cannot take.
const showShardColumn = "event_date"

// showShardYears are the calendar years the shows family is enumerated by month
// for. Ascending and contiguous, asserted by TestShowShardYearsAreContiguous.
//
// EXTENDING THIS IS ROUTINE, ROUGHLY ANNUAL MAINTENANCE. Everything dated at or
// after the year following the last one here lands in the single open tail
// shard, which then grows without bound. The frontend's data-cache budget gate
// fails the build at 80% of the cap, so the failure mode is a red build with
// headroom left rather than a silently uncached shard — but it is still a
// build-blocking failure. Append the next year while the tail is small.
var showShardYears = []int{2026, 2027}

// showShards partitions the shows family by UTC event month, plus one open
// shard below the enumerated span and one above it.
//
// WHY MONTHS, and not the years a first reading of the catalogue suggests.
// Measured against production on 2026-09-03, all 13,096 slugged approved shows
// are dated 2026 or 2027, and 2026 alone is 1,244,629 raw bytes — 79% of the
// 1,572,864-byte raw budget. A year shard would be born inside the warn band
// that blocked the build in the first place. A half-year is no better: 2026-07
// to 2026-12 is 1,221,226 bytes on its own. There is no decade of thin years to
// spread over, there is one dense year that keeps getting denser, because
// discovery ingests a rolling horizon of upcoming shows and past shows never
// age out.
//
// A month is the coarsest grain that fits with room to grow. The two fully
// ingested months on that same day were 2026-09 at 366,852 bytes and 2026-10 at
// 352,557 — 23% of the cache item cap once encoded, against the releases
// sub-shards' 20-27%. That leaves a fully ingested month about 2.5x of headroom
// below the 60% line and 4x below the cap, on a corpus growing at roughly 3,800
// shows a month. The month grain is also what keeps this family under the
// sitemap protocol's 50,000-URL-per-document limit, which a year shard would
// reach on the same growth curve as the byte cap.
//
// TOTALITY. The head shard is open below and the tail shard is open above, so
// every event_date lands in exactly one range whatever the enumerated span is —
// the argument releaseShards rests on, applied to instants. Show.EventDate is
// NOT NULL, so there is no undated bucket to provide.
func showShards() []sitemapShard {
	first := showShardYears[0]
	last := showShardYears[len(showShardYears)-1]

	shards := make([]sitemapShard, 0, len(showShardYears)*12+2)
	shards = append(shards, sitemapShard{
		family: "shows",
		id:     fmt.Sprintf("shows-before-%d", first),
		column: showShardColumn,
		before: monthStartUTC(first, time.January),
	})
	for _, year := range showShardYears {
		for month := time.January; month <= time.December; month++ {
			shards = append(shards, sitemapShard{
				family: "shows",
				id:     fmt.Sprintf("shows-%d-%02d", year, int(month)),
				column: showShardColumn,
				from:   monthStartUTC(year, month),
				before: monthStartUTC(year, month+1),
			})
		}
	}
	shards = append(shards, sitemapShard{
		family: "shows",
		id:     fmt.Sprintf("shows-from-%d", last+1),
		column: showShardColumn,
		from:   monthStartUTC(last+1, time.January),
	})
	return shards
}

// monthStartUTC is the first instant of a UTC calendar month. time.Date
// normalises an out-of-range month, so December + 1 is the following January.
func monthStartUTC(year int, month time.Month) time.Time {
	return time.Date(year, month, 1, 0, 0, 0, 0, time.UTC)
}

// sitemapShards is every sub-shard of every family, in sitemapFamilies order —
// the order the frontend's FAMILY_BY_SHARD_ID walks, so the two tables
// enumerate identically.
//
// Keep the ids in sync with SHOW_SHARD_IDS and RELEASE_SHARD_IDS in
// frontend/app/sitemap-shards.ts and with the `family` enum on
// GetSitemapEntriesRequest, the same way sitemapFamilies above is. The enum
// half is test-enforced (TestSitemapFamilyEnumMatchesTheService); the frontend
// half is enforced by `bun run api:types` regenerating the wire enum those
// lists are declared against, so a renamed id fails tsc there.
var sitemapShards = buildSitemapShards()

func buildSitemapShards() []sitemapShard {
	byFamily := map[string][]sitemapShard{
		"shows":    showShards(),
		"releases": releaseShards,
	}
	shards := []sitemapShard{}
	for _, family := range sitemapFamilies {
		shards = append(shards, byFamily[family]...)
	}
	return shards
}

// SitemapFamilyValues is every accepted value of the `family` query parameter,
// in enum order: the entity families, then the sub-shard ids that address a
// slice of one.
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

// sitemapShardByID returns the slice a sub-shard id names, or nil.
func sitemapShardByID(id string) *sitemapShard {
	for i := range sitemapShards {
		if sitemapShards[i].id == id {
			return &sitemapShards[i]
		}
	}
	return nil
}

// forFamily returns the shard when it slices family, and nil otherwise.
//
// Load-bearing rather than ceremonial: a shard carries the COLUMN its range is
// taken on, and each family's scope resolves to a different table. Applying one
// family's shard to another family's scope would either error on an unknown
// column or, worse, narrow on a column that happens to exist there too.
// Resolving through this makes every call site name the family it is narrowing.
func (shard *sitemapShard) forFamily(family string) *sitemapShard {
	if shard == nil || shard.family != family {
		return nil
	}
	return shard
}

// narrow applies the shard's bounds to its family's scope. A nil shard is the
// whole family, which is what an unsharded `?family=releases` still asks for.
func (shard *sitemapShard) narrow(scope *gorm.DB) *gorm.DB {
	if shard == nil {
		return scope
	}
	if shard.from != nil {
		scope = scope.Where(shard.column+" >= ?", shard.from)
	}
	if shard.before != nil {
		scope = scope.Where(shard.column+" < ?", shard.before)
	}
	return scope
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
// one slice of it: a slug range of releases (PSY-1763) or a UTC event month of
// shows (PSY-2018). See sitemapShard for why those two families outgrew a
// single cache entry, and releaseShards / showShards for why each is keyed the
// way it is. It is carried in this same parameter rather than a second one on
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

	// A sub-shard id resolves to the family it slices plus the range to slice
	// it by, so everything downstream reasons in families only.
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
		// A nil shard is the whole family; the event-month range predicate lives
		// on the shard so the scope here stays the plain single-table projection
		// entriesFor requires.
		shows, err := s.entriesFor(
			ctx,
			shard.forFamily("shows").narrow(
				s.db.Model(&catalogm.Show{}).Where("status = ?", catalogm.ShowStatusApproved),
			),
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
		// A nil shard is the whole family; the range predicate lives on the
		// shard so the scope here stays the plain single-table projection
		// entriesFor requires.
		releases, err := s.entriesFor(ctx, shard.forFamily("releases").narrow(s.db.Model(&catalogm.Release{})))
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
		SELECT `+sceneGroupIdentitySQL+`
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
		SELECT `+sceneGroupIdentitySQL+`,
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
