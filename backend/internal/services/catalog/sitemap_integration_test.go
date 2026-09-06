package catalog

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"

	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/testutil"
)

func sitemapSlugsOf(entries []contracts.SitemapEntry) []string {
	out := make([]string, len(entries))
	for i, e := range entries {
		out[i] = e.Slug
	}
	return out
}

// TestSitemapEntriesIndexability covers the inclusion rules: a URL belongs in
// the sitemap only if it actually resolves for an anonymous visitor. A row with
// no slug has no canonical URL, and a non-approved show 404s — advertising
// either fills the index with dead links.
func TestSitemapEntriesIndexability(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	td := testutil.SetupTestPostgres(t)
	defer td.Cleanup()

	eventDate := time.Now().Add(7 * 24 * time.Hour)
	seedShows := []catalogm.Show{
		{Title: "Approved With Slug", Slug: strPtr("approved-with-slug"), EventDate: eventDate, Status: catalogm.ShowStatusApproved},
		{Title: "Approved No Slug", EventDate: eventDate, Status: catalogm.ShowStatusApproved},
		{Title: "Pending", Slug: strPtr("pending-show"), EventDate: eventDate, Status: catalogm.ShowStatusPending},
		{Title: "Rejected", Slug: strPtr("rejected-show"), EventDate: eventDate, Status: catalogm.ShowStatusRejected},
		{Title: "Private", Slug: strPtr("private-show"), EventDate: eventDate, Status: catalogm.ShowStatusPrivate},
	}
	for i := range seedShows {
		if err := td.DB.Create(&seedShows[i]).Error; err != nil {
			t.Fatalf("seed show %q: %v", seedShows[i].Title, err)
		}
	}

	if err := td.DB.Create(&catalogm.Artist{Name: "Slugged Artist", Slug: strPtr("slugged-artist")}).Error; err != nil {
		t.Fatalf("seed artist: %v", err)
	}
	if err := td.DB.Create(&catalogm.Artist{Name: "Unslugged Artist"}).Error; err != nil {
		t.Fatalf("seed unslugged artist: %v", err)
	}
	// Distinct from NULL: GenerateSlug returns "" for an all-non-ASCII name, so
	// the empty-string half of the predicate is reachable and needs its own row
	// — without it, simplifying to `slug IS NOT NULL` would pass CI.
	if err := td.DB.Create(&catalogm.Artist{Name: "Empty Slug Artist", Slug: strPtr("")}).Error; err != nil {
		t.Fatalf("seed empty-slug artist: %v", err)
	}
	if err := td.DB.Create(&catalogm.Venue{Name: "Slugged Venue", Slug: strPtr("slugged-venue"), City: "Phoenix", State: "AZ"}).Error; err != nil {
		t.Fatalf("seed venue: %v", err)
	}
	if err := td.DB.Create(&catalogm.Label{Name: "Slugged Label", Slug: strPtr("slugged-label")}).Error; err != nil {
		t.Fatalf("seed label: %v", err)
	}
	if err := td.DB.Create(&catalogm.Release{Title: "Slugged Release", Slug: strPtr("slugged-release")}).Error; err != nil {
		t.Fatalf("seed release: %v", err)
	}
	if err := td.DB.Create(&catalogm.Festival{
		Name: "Slugged Fest", Slug: "slugged-fest", SeriesSlug: "slugged-fest",
		EditionYear: 2026, StartDate: "2026-08-01", EndDate: "2026-08-03",
	}).Error; err != nil {
		t.Fatalf("seed festival: %v", err)
	}
	if err := td.DB.Create(&catalogm.Tag{Name: "Slugged Tag", Slug: "slugged-tag", Category: catalogm.TagCategoryGenre}).Error; err != nil {
		t.Fatalf("seed tag: %v", err)
	}

	entries, err := NewSitemapService(td.DB).Entries(context.Background(), "")
	if err != nil {
		t.Fatalf("Entries: %v", err)
	}

	showSlugs := sitemapSlugsOf(entries.Shows)
	if len(showSlugs) != 1 || showSlugs[0] != "approved-with-slug" {
		t.Errorf("shows = %v, want exactly [approved-with-slug] — non-approved and slugless shows must be excluded", showSlugs)
	}

	artistSlugs := sitemapSlugsOf(entries.Artists)
	if len(artistSlugs) != 1 || artistSlugs[0] != "slugged-artist" {
		t.Errorf("artists = %v, want exactly [slugged-artist]", artistSlugs)
	}

	venueSlugs := sitemapSlugsOf(entries.Venues)
	if len(venueSlugs) != 1 || venueSlugs[0] != "slugged-venue" {
		t.Errorf("venues = %v, want exactly [slugged-venue]", venueSlugs)
	}

	if got := sitemapSlugsOf(entries.Labels); len(got) != 1 || got[0] != "slugged-label" {
		t.Errorf("labels = %v, want [slugged-label]", got)
	}
	if got := sitemapSlugsOf(entries.Releases); len(got) != 1 || got[0] != "slugged-release" {
		t.Errorf("releases = %v, want [slugged-release]", got)
	}
	if got := sitemapSlugsOf(entries.Festivals); len(got) != 1 || got[0] != "slugged-fest" {
		t.Errorf("festivals = %v, want [slugged-fest]", got)
	}
	if got := sitemapSlugsOf(entries.Tags); len(got) != 1 || got[0] != "slugged-tag" {
		t.Errorf("tags = %v, want [slugged-tag]", got)
	}

	for _, e := range entries.Shows {
		if e.UpdatedAt.IsZero() {
			t.Errorf("show %q has a zero UpdatedAt — <lastmod> would be meaningless", e.Slug)
		}
	}
}

// TestSitemapEntriesOrderIsTotal pins the ordering contract: slug ascending,
// which is a total order over a unique column. Insertion order must not leak
// into the response, so that two fetches of an unchanged catalogue diff cleanly.
func TestSitemapEntriesOrderIsTotal(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	td := testutil.SetupTestPostgres(t)
	defer td.Cleanup()

	for _, slug := range []string{"charlie", "alpha", "bravo"} {
		if err := td.DB.Create(&catalogm.Artist{Name: "Artist " + slug, Slug: strPtr(slug)}).Error; err != nil {
			t.Fatalf("seed artist %q: %v", slug, err)
		}
	}

	entries, err := NewSitemapService(td.DB).Entries(context.Background(), "artists")
	if err != nil {
		t.Fatalf("Entries: %v", err)
	}

	got := sitemapSlugsOf(entries.Artists)
	want := []string{"alpha", "bravo", "charlie"}
	if len(got) != len(want) {
		t.Fatalf("artists = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("artists = %v, want %v (order must be slug ASC, not insertion order)", got, want)
		}
	}
	if entries.Shows == nil || len(entries.Shows) != 0 {
		t.Errorf("shows under artists filter = %v, want empty non-nil slice", entries.Shows)
	}
}

// TestSitemapEntriesFamilyFilterIsolatesOneFamily is the Data Cache budget
// guard for PSY-1622: each generateSitemaps() shard fetches ?family=… so a
// shows-only request must not also pay for releases.
func TestSitemapEntriesFamilyFilterIsolatesOneFamily(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	td := testutil.SetupTestPostgres(t)
	defer td.Cleanup()

	if err := td.DB.Create(&catalogm.Artist{Name: "A", Slug: strPtr("artist-a")}).Error; err != nil {
		t.Fatalf("seed artist: %v", err)
	}
	if err := td.DB.Create(&catalogm.Label{Name: "L", Slug: strPtr("label-l")}).Error; err != nil {
		t.Fatalf("seed label: %v", err)
	}

	entries, err := NewSitemapService(td.DB).Entries(context.Background(), "labels")
	if err != nil {
		t.Fatalf("Entries: %v", err)
	}
	if got := sitemapSlugsOf(entries.Labels); len(got) != 1 || got[0] != "label-l" {
		t.Errorf("labels = %v, want [label-l]", got)
	}
	if len(entries.Artists) != 0 {
		t.Errorf("artists under labels filter = %v, want empty", entries.Artists)
	}

	// A family with nothing to announce still answers with an empty ARRAY. The
	// catalogue here has no venues, so scene_weeks resolves no scenes at all,
	// and a nil slice would reach the generator as JSON null where it iterates
	// per family.
	empty, err := NewSitemapService(td.DB).Entries(context.Background(), "scene_weeks")
	if err != nil {
		t.Fatalf("Entries(scene_weeks): %v", err)
	}
	if empty.SceneWeeks == nil {
		t.Error("scene_weeks on an empty catalogue = nil, want an empty slice")
	}
	if len(empty.SceneWeeks) != 0 {
		t.Errorf("scene_weeks on an empty catalogue = %v, want empty", sitemapSlugsOf(empty.SceneWeeks))
	}
}

// subShardedFamily is how the test below seeds one row of a family with an
// explicit primary key, which is what makes the bucket a row lands in
// predictable, and how it reads the field that family's rows come back in.
//
// One table keyed by family, so a family added to sitemapShardsByFamily fails
// once with a message naming what to add rather than going unchecked.
type subShardedFamily struct {
	seed func(db *gorm.DB, id uint, slug string) error
	rows func(*contracts.SitemapEntries) []contracts.SitemapEntry
}

var subShardedFamilies = map[string]subShardedFamily{
	"shows": {
		seed: func(db *gorm.DB, id uint, slug string) error {
			return db.Create(&catalogm.Show{
				ID:        id,
				Title:     "Bucketed Show " + slug,
				Slug:      strPtr(slug),
				EventDate: time.Date(2026, 9, 17, 3, 30, 0, 0, time.UTC),
				Status:    catalogm.ShowStatusApproved,
			}).Error
		},
		rows: func(e *contracts.SitemapEntries) []contracts.SitemapEntry { return e.Shows },
	},
	"artists": {
		seed: func(db *gorm.DB, id uint, slug string) error {
			return db.Create(&catalogm.Artist{ID: id, Name: "Bucketed Artist " + slug, Slug: strPtr(slug)}).Error
		},
		rows: func(e *contracts.SitemapEntries) []contracts.SitemapEntry { return e.Artists },
	},
	"releases": {
		seed: func(db *gorm.DB, id uint, slug string) error {
			return db.Create(&catalogm.Release{ID: id, Title: "Bucketed Release " + slug, Slug: strPtr(slug)}).Error
		},
		rows: func(e *contracts.SitemapEntries) []contracts.SitemapEntry { return e.Releases },
	},
}

// TestSitemapSubShardsBucketEveryFamilyExactly is the behavioural half of the
// sub-shard guard. Its sibling unit test checks the shard TABLE; this checks
// what the DATABASE does with it, which is a different question: the partition
// argument rests on Postgres agreeing with Go about `id % N`, and no amount of
// reading the table shows that.
//
// Driven by the shard table, so a newly sub-sharded family inherits every
// assertion:
//
//   - Set equality against the unsharded family. A gap (URLs silently absent
//     from the sitemap) and an overlap (a URL announced twice) both fail it.
//   - Exact placement. A row whose primary key is k modulo N must be served by
//     the shard named for residue k, which is what pins the database's `%` to
//     the residue the id promises.
//   - No shard serves the whole family. That is the missing-`shard.narrow`
//     failure at a new family's call site in Entries, which nothing else
//     notices: the wire enum accepts the ids, every shard answers 200, and each
//     one returns the whole over-cap family with a green build.
//   - A shard populates its own family only, since anything else leaking into
//     the response is paid for by every shard.
func TestSitemapSubShardsBucketEveryFamilyExactly(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	td := testutil.SetupTestPostgres(t)
	defer td.Cleanup()

	service := NewSitemapService(td.DB)

	// EVERY family is seeded before ANY is asserted. Seeding inside the
	// assertion loop would make the cross-family isolation check vacuous for
	// whichever family Go's map iteration reached first, since the others would
	// still be empty and could not leak.
	seededIDs := map[string][]uint{}
	wantShardByFamily := map[string]map[string]string{}
	for family, shards := range sitemapShardsByFamily {
		under, ok := subShardedFamilies[family]
		if !ok {
			t.Fatalf("family %q is sub-sharded but this test cannot seed or read it, so add it to subShardedFamilies", family)
		}

		buckets := len(shards)
		// Two full turns of the modulus plus one id far above the run, so every
		// residue carries more than one row and the placement assertion is not
		// satisfied by a shard that happens to serve the first N ids in order.
		ids := make([]uint, 0, 2*buckets+1)
		for id := 1; id <= 2*buckets; id++ {
			ids = append(ids, uint(id))
		}
		ids = append(ids, uint(10*buckets+3))
		seededIDs[family] = ids

		wantShard := map[string]string{}
		for _, id := range ids {
			slug := fmt.Sprintf("%s-bucketed-%d", family, id)
			if err := under.seed(td.DB, id, slug); err != nil {
				t.Fatalf("seed %s %d: %v", family, id, err)
			}
			wantShard[slug] = sitemapShardID(family, int(id)%buckets)
		}
		wantShardByFamily[family] = wantShard
	}

	for family, shards := range sitemapShardsByFamily {
		read := subShardedFamilies[family].rows
		ids := seededIDs[family]
		wantShard := wantShardByFamily[family]

		whole, err := service.Entries(context.Background(), family)
		if err != nil {
			t.Fatalf("Entries(%s): %v", family, err)
		}
		wholeSlugs := sitemapSlugsOf(read(whole))
		if len(wholeSlugs) != len(ids) {
			t.Fatalf("unsharded %s served %d slugs, want the %d seeded", family, len(wholeSlugs), len(ids))
		}

		owner := map[string]string{}
		for _, shard := range shards {
			entries, err := service.Entries(context.Background(), shard.id)
			if err != nil {
				t.Fatalf("Entries(%s): %v", shard.id, err)
			}
			slugs := sitemapSlugsOf(read(entries))
			if len(slugs) == len(wholeSlugs) {
				t.Errorf("shard %q served all %d rows of %q, so its call site in Entries is not narrowing",
					shard.id, len(slugs), family)
			}
			for otherFamily, other := range subShardedFamilies {
				if otherFamily == family {
					continue
				}
				if got := len(other.rows(entries)); got != 0 {
					t.Errorf("Entries(%s) populated %d %s rows, and a sub-shard addresses one family",
						shard.id, got, otherFamily)
				}
			}
			for _, slug := range slugs {
				if prev, dup := owner[slug]; dup {
					t.Errorf("slug %q is served by both %q and %q, so the %q buckets overlap", slug, prev, shard.id, family)
				}
				owner[slug] = shard.id
			}
		}

		for _, slug := range wholeSlugs {
			if _, ok := owner[slug]; !ok {
				t.Errorf("slug %q of family %q belongs to no bucket, so the partition has a gap and this URL leaves the sitemap", slug, family)
			}
		}
		if len(owner) != len(wholeSlugs) {
			t.Errorf("%q buckets served %d slugs, the whole family has %d", family, len(owner), len(wholeSlugs))
		}
		for slug, want := range wantShard {
			if got := owner[slug]; got != want {
				t.Errorf("%s %q is served by %q, want %q: the database's modulo disagrees with the residue the id names",
					family, slug, got, want)
			}
		}
	}
}

// TestSitemapShowSubShardsKeepTheApprovedOnlyPredicate pins the one visibility
// rule narrowing could lose. Only approved shows are publicly reachable, so a
// bucket that dropped the status filter while adding its own predicate would
// announce URLs GetShowHandler rejects, and every other assertion here would
// still pass.
func TestSitemapShowSubShardsKeepTheApprovedOnlyPredicate(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	td := testutil.SetupTestPostgres(t)
	defer td.Cleanup()

	approved := &catalogm.Show{
		ID:        41,
		Title:     "Approved Bucketed Show",
		Slug:      strPtr("approved-bucketed-show"),
		EventDate: time.Date(2026, 9, 17, 3, 30, 0, 0, time.UTC),
		Status:    catalogm.ShowStatusApproved,
	}
	if err := td.DB.Create(approved).Error; err != nil {
		t.Fatalf("seed approved show: %v", err)
	}
	// Same residue as the approved row, so both are served by one shard and the
	// filter is the only thing that can separate them.
	pending := &catalogm.Show{
		ID:        41 + uint(sitemapShardsPerFamily["shows"]),
		Title:     "Pending Bucketed Show",
		Slug:      strPtr("pending-bucketed-show"),
		EventDate: time.Date(2026, 9, 18, 1, 0, 0, 0, time.UTC),
		Status:    catalogm.ShowStatusPending,
	}
	if err := td.DB.Create(pending).Error; err != nil {
		t.Fatalf("seed pending show: %v", err)
	}

	service := NewSitemapService(td.DB)
	for _, shard := range sitemapShardsByFamily["shows"] {
		entries, err := service.Entries(context.Background(), shard.id)
		if err != nil {
			t.Fatalf("Entries(%s): %v", shard.id, err)
		}
		for _, slug := range sitemapSlugsOf(entries.Shows) {
			if slug == "pending-bucketed-show" {
				t.Errorf("shard %q announced a pending show, so narrowing dropped the approved-only predicate", shard.id)
			}
		}
	}
}

// TestSitemapEntriesIssuesOneQueryPerSimpleFamily is the regression guard for
// the defect this service was built to fix — see contracts.SitemapEntry.
// Scene and scene_weeks use their own multi-query projections (joins + timezone
// resolution); the simple slug tables must stay one SELECT each.
func TestSitemapEntriesIssuesOneQueryPerSimpleFamily(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	td := testutil.SetupTestPostgres(t)
	defer td.Cleanup()

	venue := &catalogm.Venue{Name: "Counted Venue", Slug: strPtr("counted-venue"), City: "Phoenix", State: "AZ"}
	if err := td.DB.Create(venue).Error; err != nil {
		t.Fatalf("seed venue: %v", err)
	}
	for i, slug := range []string{"counted-one", "counted-two", "counted-three"} {
		artist := &catalogm.Artist{Name: "Counted Artist " + slug, Slug: strPtr("artist-" + slug)}
		if err := td.DB.Create(artist).Error; err != nil {
			t.Fatalf("seed artist: %v", err)
		}
		show := &catalogm.Show{
			Title:     "Counted Show",
			Slug:      strPtr(slug),
			EventDate: time.Now().Add(time.Duration(i+1) * 24 * time.Hour),
			Status:    catalogm.ShowStatusApproved,
		}
		if err := td.DB.Create(show).Error; err != nil {
			t.Fatalf("seed show: %v", err)
		}
		if err := td.DB.Create(&catalogm.ShowVenue{ShowID: show.ID, VenueID: venue.ID}).Error; err != nil {
			t.Fatalf("seed show_venue: %v", err)
		}
		if err := td.DB.Create(&catalogm.ShowArtist{ShowID: show.ID, ArtistID: artist.ID, Position: 0}).Error; err != nil {
			t.Fatalf("seed show_artist: %v", err)
		}
	}

	simpleFamilies := []string{"shows", "artists", "venues", "labels", "releases", "festivals", "tags"}
	for _, family := range simpleFamilies {
		var queries int
		counting := td.DB.Session(&gorm.Session{Logger: queryCounter{Interface: td.DB.Logger, n: &queries}})
		if _, err := NewSitemapService(counting).Entries(context.Background(), family); err != nil {
			t.Fatalf("Entries(%s): %v", family, err)
		}
		if queries != 1 {
			t.Errorf("Entries(%s) issued %d queries, want 1 — a Preload or N+1 has crept in", family, queries)
		}
	}
}

// TestSitemapEntriesSceneWeeksExcludesZeroShowWeeks pins the PSY-1622 decision:
// last 8 weeks per scene, no thin empty weeks.
func TestSitemapEntriesSceneWeeksExcludesZeroShowWeeks(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	td := testutil.SetupTestPostgres(t)
	defer td.Cleanup()

	loc, err := time.LoadLocation("America/Phoenix")
	if err != nil {
		t.Fatalf("load loc: %v", err)
	}
	y, w := time.Now().In(loc).ISOWeek()
	weekStart := ISOWeekStart(y, w, loc)
	currentKey := ISOWeekKey(weekStart)

	seedSceneWeekGroup(t, td.DB, "phx", "Phoenix", "AZ", seedMetro("Phoenix", "AZ"), "America/Phoenix", weekStart)

	entries, err := NewSitemapService(td.DB).Entries(context.Background(), "scene_weeks")
	if err != nil {
		t.Fatalf("Entries: %v", err)
	}

	wantSlug := "phoenix-az/" + currentKey
	found := false
	for _, e := range entries.SceneWeeks {
		if e.Slug == wantSlug {
			found = true
		}
		if e.UpdatedAt.IsZero() {
			t.Errorf("scene-week %q has zero UpdatedAt", e.Slug)
		}
	}
	if !found {
		t.Errorf("scene_weeks = %v, want to include %q", sitemapSlugsOf(entries.SceneWeeks), wantSlug)
	}

	scenes, err := NewSitemapService(td.DB).Entries(context.Background(), "scenes")
	if err != nil {
		t.Fatalf("Entries(scenes): %v", err)
	}
	if got := sitemapSlugsOf(scenes.Scenes); len(got) != 1 || got[0] != "phoenix-az" {
		t.Errorf("scenes = %v, want [phoenix-az]", got)
	}
}

// TestSitemapEntriesVenueYearsMatchesThePastHistogram is the load-bearing
// guarantee of the venue_years family (PSY-1756): every year it announces has to
// be a year /venues/{slug}/shows/{year} will actually render, and that page is
// built from GetVenueShowYears(time_filter=past). A year announced here that the
// histogram does not carry is a URL the site 404s — the failure this family
// exists to avoid rather than cause.
//
// It also pins the two exclusion rules that fall out of the same query: an
// UPCOMING-only year is not an archive, and a venue with no slug has no URL.
func TestSitemapEntriesVenueYearsMatchesThePastHistogram(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	td := testutil.SetupTestPostgres(t)
	defer td.Cleanup()

	venue := &catalogm.Venue{
		Name:     "Archive Room",
		Slug:     strPtr("archive-room"),
		City:     "Phoenix",
		State:    "AZ",
		Timezone: strPtr("America/Phoenix"),
	}
	if err := td.DB.Create(venue).Error; err != nil {
		t.Fatalf("seed venue: %v", err)
	}
	// A venue with past shows but NO slug: it has no URL, so it must contribute
	// no entry even though those shows are indexable in their own right.
	unslugged := &catalogm.Venue{
		Name:     "Unslugged Room",
		City:     "Phoenix",
		State:    "AZ",
		Timezone: strPtr("America/Phoenix"),
	}
	if err := td.DB.Create(unslugged).Error; err != nil {
		t.Fatalf("seed unslugged venue: %v", err)
	}

	loc, err := time.LoadLocation("America/Phoenix")
	if err != nil {
		t.Fatalf("load loc: %v", err)
	}
	lastYear := time.Now().In(loc).Year() - 1

	seed := []struct {
		slug  string
		when  time.Time
		venue *catalogm.Venue
	}{
		// Two in the same past year: the grain is (venue, YEAR), not per show.
		{"vy-past-a", time.Date(lastYear, time.March, 4, 20, 0, 0, 0, loc), venue},
		{"vy-past-b", time.Date(lastYear, time.November, 9, 20, 0, 0, 0, loc), venue},
		// Upcoming: not an archive, so its year must not be announced.
		{"vy-upcoming", time.Now().In(loc).AddDate(1, 0, 0), venue},
		// Past, but at the venue with no slug.
		{"vy-unslugged", time.Date(lastYear, time.May, 1, 20, 0, 0, 0, loc), unslugged},
	}
	for _, s := range seed {
		show := &catalogm.Show{
			Title:     "Archive " + s.slug,
			Slug:      strPtr(s.slug),
			EventDate: s.when.UTC(),
			Status:    catalogm.ShowStatusApproved,
		}
		if err := td.DB.Create(show).Error; err != nil {
			t.Fatalf("seed show %s: %v", s.slug, err)
		}
		if err := td.DB.Create(&catalogm.ShowVenue{ShowID: show.ID, VenueID: s.venue.ID}).Error; err != nil {
			t.Fatalf("seed show_venue %s: %v", s.slug, err)
		}
	}

	entries, err := NewSitemapService(td.DB).Entries(context.Background(), "venue_years")
	if err != nil {
		t.Fatalf("Entries(venue_years): %v", err)
	}

	got := sitemapSlugsOf(entries.VenueYears)
	want := fmt.Sprintf("archive-room/shows/%d", lastYear)
	if len(got) != 1 || got[0] != want {
		t.Fatalf("venue_years = %v, want exactly [%s]", got, want)
	}
	if entries.VenueYears[0].UpdatedAt.IsZero() {
		t.Error("venue-year entry has zero UpdatedAt — <lastmod> would be empty")
	}

	// The page's own authority on which years exist. Anything this family
	// announces must appear here, or the URL 404s.
	years, err := NewVenueService(td.DB).GetVenueShowYears(venue.ID, "past")
	if err != nil {
		t.Fatalf("GetVenueShowYears: %v", err)
	}
	histogram := map[int]bool{}
	for _, y := range years {
		histogram[y.Year] = true
	}
	if !histogram[lastYear] {
		t.Fatalf("past histogram = %+v, want to carry %d", years, lastYear)
	}
	if len(histogram) != 1 {
		t.Errorf("past histogram = %+v, want exactly one year (the upcoming show must not appear)", years)
	}
}

// seedSceneWeekGroup seeds one qualifying scene group with the shows spread
// over every room.
func seedSceneWeekGroup(t *testing.T, db *gorm.DB, label, city, state string, metro *string, tz string, weekStart time.Time) {
	t.Helper()
	seedSceneGroupOverRooms(t, db, label, city, state, metro, tz, weekStart, sceneMinVenues)
}

// seedSceneGroupOverRooms seeds one qualifying scene group: sceneMinVenues
// verified rooms sharing a (city, state, metro), and sceneMinShows approved
// shows at them, all dated midweek of the ISO week starting at weekStart in
// that week's own location. label makes the seeded slugs unique when two groups
// share a city spelling family.
//
// roomsHostingShows is how many of those rooms the shows are spread over. The
// remainder are verified rooms with no approved show, which count toward the
// venue floor when the eligibility query joins shows with a LEFT JOIN and drop
// out of the count when it joins them with an inner one.
//
// Only verified rooms form a scene. Verified is set on the insert rather than
// by a follow-up Update: Venue.Verified carries no `default` tag, and that tag
// is what makes GORM omit a zero value and let the column default decide.
func seedSceneGroupOverRooms(t *testing.T, db *gorm.DB, label, city, state string, metro *string, tz string, weekStart time.Time, roomsHostingShows int) {
	t.Helper()

	if roomsHostingShows < 1 || roomsHostingShows > sceneMinVenues {
		t.Fatalf("roomsHostingShows = %d, want between 1 and sceneMinVenues (%d)", roomsHostingShows, sceneMinVenues)
	}

	rooms := make([]uint, 0, sceneMinVenues)
	for i := 0; i < sceneMinVenues; i++ {
		v := &catalogm.Venue{
			Name:     fmt.Sprintf("%s Room %d", label, i),
			Slug:     strPtr(fmt.Sprintf("%s-room-%d", label, i)),
			City:     city,
			State:    state,
			Metro:    metro,
			Timezone: strPtr(tz),
			Verified: true,
		}
		if err := db.Create(v).Error; err != nil {
			t.Fatalf("seed venue %s/%d: %v", label, i, err)
		}
		rooms = append(rooms, v.ID)
	}

	for i := 0; i < sceneMinShows; i++ {
		show := &catalogm.Show{
			Title:     fmt.Sprintf("%s Show %d", label, i),
			Slug:      strPtr(fmt.Sprintf("%s-show-%d", label, i)),
			EventDate: weekStart.Add(48 * time.Hour).UTC(),
			Status:    catalogm.ShowStatusApproved,
		}
		if err := db.Create(show).Error; err != nil {
			t.Fatalf("seed show %s/%d: %v", label, i, err)
		}
		if err := db.Create(&catalogm.ShowVenue{ShowID: show.ID, VenueID: rooms[i%roomsHostingShows]}).Error; err != nil {
			t.Fatalf("seed show_venue %s/%d: %v", label, i, err)
		}
	}
}

// sceneCollisionGroups returns the qualifying groups of a fixture whose groups
// publish ONE slug, and fails unless the fixture is exactly that shape.
func sceneCollisionGroups(t *testing.T, svc *SitemapService) []sceneVenueGroup {
	t.Helper()

	groups, err := svc.listQualifyingScenes(context.Background())
	if err != nil {
		t.Fatalf("listQualifyingScenes: %v", err)
	}
	if len(groups) != 2 {
		t.Fatalf("listQualifyingScenes returned %d groups, want the 2 that collide on one slug: %+v", len(groups), groups)
	}
	if a, b := sceneGroupSlug(groups[0]), sceneGroupSlug(groups[1]); a != b {
		t.Fatalf("fixture must collide: the groups publish %q and %q", a, b)
	}
	return groups
}

// forEachSceneGroupOrder runs assert over both orders a colliding pair can
// reach a projection in. Which row a GROUP BY hands over first is the planner's
// to decide, so handing the slice in is the only way to reach the loser-first
// order, and that order is what separates delegating the winner choice from
// keeping the first row that arrives.
//
// It separates them only where the colliding groups' DISPLAY identities differ,
// since everything downstream of the collapse is derived from that identity.
// Each caller's doc says whether its fixture discriminates or guards.
func forEachSceneGroupOrder(t *testing.T, groups []sceneVenueGroup, assert func(where string, perm []sceneVenueGroup)) {
	t.Helper()

	for _, perm := range [][]sceneVenueGroup{
		{groups[0], groups[1]},
		{groups[1], groups[0]},
	} {
		assert(fmt.Sprintf("with metro=%q city=%q first", perm[0].Metro, perm[0].City), perm)
	}
}

// saintJeromeSpellings is one non-US city under two spellings: two venue groups
// publishing saintJeromeSlug with no metro drift involved, since
// sceneGroupKeySQL only lower/trims while buildSceneSlug also maps spaces to
// dashes.
var saintJeromeSpellings = [2]struct{ label, city string }{
	{"spaced", "Saint Jerome"},
	{"hyphenated", "Saint-Jerome"},
}

const saintJeromeSlug = "saint-jerome-qc"

// seedSaintJeromeCollision seeds both spellings, each at the week start
// weekStartFor gives it, and returns the city ParseSceneSlug resolves the slug
// to.
//
// The winner is derived rather than named: Go compares the group minima
// byte-wise while Postgres orders them under the database's collation, so a
// collation that disagrees fails here instead of going green on the wrong
// spelling.
func seedSaintJeromeCollision(t *testing.T, db *gorm.DB, weekStartFor func(city string) time.Time) string {
	t.Helper()

	for _, sp := range saintJeromeSpellings {
		if metro := seedMetro(sp.city, "QC"); metro != nil {
			t.Fatalf("fixture city %q pins CBSA %q; this collision must involve no metro drift", sp.city, *metro)
		}
		seedSceneWeekGroup(t, db, sp.label, sp.city, "QC", nil, "America/Toronto", weekStartFor(sp.city))
	}

	city, state, err := NewSceneService(db).ParseSceneSlug(saintJeromeSlug)
	if err != nil {
		t.Fatalf("ParseSceneSlug: %v", err)
	}
	if state != "QC" {
		t.Fatalf("ParseSceneSlug resolved state %q, want QC", state)
	}
	if city != saintJeromeSpellings[0].city && city != saintJeromeSpellings[1].city {
		t.Fatalf("ParseSceneSlug resolved city %q, want one of the two seeded spellings", city)
	}
	return city
}

// assertSceneWeekCollisionResolvesTo asserts a two-group fixture whose groups
// publish ONE slug emits exactly wantSlug, then that the exported path agrees,
// which is what ties the projection it exercises to the family the sitemap
// actually serves.
func assertSceneWeekCollisionResolvesTo(t *testing.T, svc *SitemapService, wantSlug string) {
	t.Helper()
	ctx := context.Background()

	forEachSceneGroupOrder(t, sceneCollisionGroups(t, svc), func(where string, perm []sceneVenueGroup) {
		entries, err := svc.sceneWeekEntries(ctx, perm)
		if err != nil {
			t.Fatalf("sceneWeekEntries %s: %v", where, err)
		}
		if got := sitemapSlugsOf(entries); len(got) != 1 || got[0] != wantSlug {
			t.Fatalf("%s, scene_weeks = %v, want exactly [%s]", where, got, wantSlug)
		}
	})

	entries, err := svc.Entries(ctx, "scene_weeks")
	if err != nil {
		t.Fatalf("Entries: %v", err)
	}
	if got := sitemapSlugsOf(entries.SceneWeeks); len(got) != 1 || got[0] != wantSlug {
		t.Fatalf("Entries(scene_weeks) = %v, want exactly [%s]", got, wantSlug)
	}
	for _, e := range entries.SceneWeeks {
		if e.UpdatedAt.IsZero() {
			t.Errorf("scene-week %q has zero UpdatedAt", e.Slug)
		}
	}
}

// TestSitemapEntriesSceneWeeksFollowTheSpellingTheSlugResolvesTo is the
// collision shape that decides which SHOWS the sitemap publishes weeks for, and
// the one that discriminates: it fails if the winner is taken from the scan's
// first row rather than resolved.
//
// Neither spelling pins a CBSA, so each group's display identity is its own
// literal city, and the surviving identity is what builds the scope selecting
// the rooms whose shows become the week permalinks. Take the group the slug does
// not resolve to and every URL in this family names a week computed from rooms
// /scenes/saint-jerome-qc never shows.
//
// The two groups' shows sit in DIFFERENT ISO weeks, so an emitted key names its
// group outright instead of merely counting it.
func TestSitemapEntriesSceneWeeksFollowTheSpellingTheSlugResolvesTo(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	td := testutil.SetupTestPostgres(t)
	defer td.Cleanup()

	loc, err := time.LoadLocation("America/Toronto")
	if err != nil {
		t.Fatalf("load loc: %v", err)
	}
	y, w := time.Now().In(loc).ISOWeek()
	currentStart := ISOWeekStart(y, w, loc)

	weekStartByCity := map[string]time.Time{
		saintJeromeSpellings[0].city: currentStart,
		saintJeromeSpellings[1].city: currentStart.AddDate(0, 0, -7),
	}
	resolvedCity := seedSaintJeromeCollision(t, td.DB, func(city string) time.Time {
		return weekStartByCity[city]
	})

	assertSceneWeekCollisionResolvesTo(t, NewSitemapService(td.DB), saintJeromeSlug+"/"+ISOWeekKey(weekStartByCity[resolvedCity]))
}

// TestSitemapEntriesSceneWeeksExcludeDriftedRoomsFromTheMetroScope covers the
// other collision shape: a CBSA group and a no-metro fallback group whose
// literal city is that metro's principal city, which is what a stale
// venues.metro produces.
//
// It is a CHARACTERIZATION test, and cannot fail on the winner rule. The two
// groups share a DISPLAY identity (the principal city), and everything
// downstream is derived from that identity, so the scope is the metro whichever
// group survives and venuePredicate leaves the drifted rooms out of the emitted
// weeks either way, exactly as /scenes/phoenix-az itself leaves them out.
//
// What it does pin is that correspondence: derive the scope from a group's own
// key instead of its display identity and this fixture emits the drifted
// rooms' week, a week away from the page the URL points at.
func TestSitemapEntriesSceneWeeksExcludeDriftedRoomsFromTheMetroScope(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	td := testutil.SetupTestPostgres(t)
	defer td.Cleanup()

	metro := seedMetro("Phoenix", "AZ")
	if metro == nil {
		t.Fatal("fixture requires Phoenix, AZ to pin a CBSA")
	}
	loc, err := time.LoadLocation("America/Phoenix")
	if err != nil {
		t.Fatalf("load loc: %v", err)
	}
	y, w := time.Now().In(loc).ISOWeek()
	currentStart := ISOWeekStart(y, w, loc)

	seedSceneWeekGroup(t, td.DB, "metro", "Phoenix", "AZ", metro, "America/Phoenix", currentStart)
	seedSceneWeekGroup(t, td.DB, "drifted", "Phoenix", "AZ", nil, "America/Phoenix", currentStart.AddDate(0, 0, -7))

	assertSceneWeekCollisionResolvesTo(t, NewSitemapService(td.DB), "phoenix-az/"+ISOWeekKey(currentStart))
}

// stampSceneShowsUpdatedAt sets shows.updated_at on every approved show at a
// city's rooms, which is the column the scenes family publishes as lastmod.
//
// It writes with Exec rather than through the model so GORM's autoUpdateTime
// hook cannot stamp the row with the current time instead, and it asserts the
// row count so a fixture whose predicate matches nothing fails here rather than
// as a puzzling assertion further down.
func stampSceneShowsUpdatedAt(t *testing.T, db *gorm.DB, city string, at time.Time) {
	t.Helper()
	res := db.Exec(`
		UPDATE shows SET updated_at = ?
		WHERE id IN (
			SELECT sv.show_id
			FROM show_venues sv
			JOIN venues v ON v.id = sv.venue_id
			WHERE v.city = ?
		)`, at, city)
	if res.Error != nil {
		t.Fatalf("stamp shows for %q: %v", city, res.Error)
	}
	if res.RowsAffected != int64(sceneMinShows) {
		t.Fatalf("stamped %d shows at venues with city = %q, want the %d that one seeded group carries; the fixture, not this helper, decides that count",
			res.RowsAffected, city, sceneMinShows)
	}
}

// TestSitemapEntriesEverySceneWithWeekPermalinksHasARootEntry pins the property
// that makes the two scene families one family: /scenes/{slug}/{week} is a
// child URL, so announcing it without announcing /scenes/{slug} indexes a
// scene's archive and hides the page that links to it.
//
// The fixture is a scene sized exactly at the venue floor where one verified
// room has never hosted an approved show. Join shows with an inner JOIN and that
// room leaves the venue count, so the scene clears the floor for its weeks and
// misses it for its root.
//
// The property is asserted over the whole document rather than the one slug, so
// a future family member that qualifies differently is covered by the same
// test. The named assertions below it keep the property from passing vacuously
// on an empty week document.
func TestSitemapEntriesEverySceneWithWeekPermalinksHasARootEntry(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	td := testutil.SetupTestPostgres(t)
	defer td.Cleanup()

	loc, err := time.LoadLocation("America/Phoenix")
	if err != nil {
		t.Fatalf("load loc: %v", err)
	}
	y, w := time.Now().In(loc).ISOWeek()
	weekStart := ISOWeekStart(y, w, loc)

	seedSceneGroupOverRooms(t, td.DB, "tuc", "Tucson", "AZ", seedMetro("Tucson", "AZ"), "America/Phoenix", weekStart, 1)
	seedSceneWeekGroup(t, td.DB, "phx", "Phoenix", "AZ", seedMetro("Phoenix", "AZ"), "America/Phoenix", weekStart)

	ctx := context.Background()
	svc := NewSitemapService(td.DB)
	scenes, err := svc.Entries(ctx, "scenes")
	if err != nil {
		t.Fatalf("Entries(scenes): %v", err)
	}
	weeks, err := svc.Entries(ctx, "scene_weeks")
	if err != nil {
		t.Fatalf("Entries(scene_weeks): %v", err)
	}

	rootSlugs := sitemapSlugsOf(scenes.Scenes)
	hasRoot := make(map[string]bool, len(rootSlugs))
	for _, slug := range rootSlugs {
		hasRoot[slug] = true
	}
	for _, e := range weeks.SceneWeeks {
		scene, week, ok := strings.Cut(e.Slug, "/")
		if !ok {
			t.Fatalf("scene-week slug %q is not {scene}/{week}", e.Slug)
		}
		if !hasRoot[scene] {
			t.Errorf("scene_weeks announces %q but scenes has no %q entry (week %s); scenes = %v", e.Slug, scene, week, rootSlugs)
		}
	}

	if len(weeks.SceneWeeks) == 0 {
		t.Fatal("scene_weeks is empty, so the property above proved nothing")
	}
	for _, want := range []string{"tucson-az", "phoenix-az"} {
		if !hasRoot[want] {
			t.Errorf("scenes = %v, want to include %q", rootSlugs, want)
		}
	}
	for _, e := range scenes.Scenes {
		if e.UpdatedAt.IsZero() {
			t.Errorf("scene %q has zero UpdatedAt", e.Slug)
		}
	}
}

// TestSitemapEntriesSceneRootLastmodComesFromTheResolvingGroup covers the one
// thing a slug collision can still get wrong on this family. Both groups
// publish the same root URL, so the emitted slug SET is the same either way and
// only the lastmod discriminates: merge the colliding groups' newest shows and
// /scenes/{slug} claims it changed when a room that page does not list changed.
//
// It reuses the spelling collision, where the two groups' DISPLAY identities
// differ, so the permutation discriminates. The group the slug does NOT resolve
// to carries the newer timestamp, which is what a projection taking the max
// across the collision would emit.
func TestSitemapEntriesSceneRootLastmodComesFromTheResolvingGroup(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	td := testutil.SetupTestPostgres(t)
	defer td.Cleanup()

	loc, err := time.LoadLocation("America/Toronto")
	if err != nil {
		t.Fatalf("load loc: %v", err)
	}
	y, w := time.Now().In(loc).ISOWeek()
	currentStart := ISOWeekStart(y, w, loc)

	resolvedCity := seedSaintJeromeCollision(t, td.DB, func(string) time.Time {
		return currentStart
	})

	// The resolving group is stamped OLDER than the group that loses the slug.
	wantLastmod := time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)
	loserLastmod := wantLastmod.AddDate(0, 1, 0)
	for _, sp := range saintJeromeSpellings {
		at := loserLastmod
		if sp.city == resolvedCity {
			at = wantLastmod
		}
		stampSceneShowsUpdatedAt(t, td.DB, sp.city, at)
	}

	svc := NewSitemapService(td.DB)
	assertSceneRootEntry := func(where string, entries []contracts.SitemapEntry) {
		t.Helper()
		if got := sitemapSlugsOf(entries); len(got) != 1 || got[0] != saintJeromeSlug {
			t.Fatalf("%s: scenes = %v, want exactly [%s]", where, got, saintJeromeSlug)
		}
		if got := entries[0].UpdatedAt.UTC(); !got.Equal(wantLastmod) {
			t.Errorf("%s: lastmod = %s, want the resolving group's %s (the other group carries %s)",
				where, got, wantLastmod, loserLastmod)
		}
	}

	forEachSceneGroupOrder(t, sceneCollisionGroups(t, svc), func(where string, perm []sceneVenueGroup) {
		assertSceneRootEntry(where, svc.sceneEntries(perm))
	})

	entries, err := svc.Entries(context.Background(), "scenes")
	if err != nil {
		t.Fatalf("Entries(scenes): %v", err)
	}
	assertSceneRootEntry("Entries(scenes)", entries.Scenes)
}
