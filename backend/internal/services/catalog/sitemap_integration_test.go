package catalog

import (
	"context"
	"fmt"
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

	metro := seedMetro("Phoenix", "AZ")
	for i, name := range []string{"Room A", "Room B"} {
		v := &catalogm.Venue{
			Name:     name,
			Slug:     strPtr(fmt.Sprintf("phx-room-%d", i)),
			City:     "Phoenix",
			State:    "AZ",
			Metro:    metro,
			Timezone: strPtr("America/Phoenix"),
		}
		if err := td.DB.Create(v).Error; err != nil {
			t.Fatalf("seed venue: %v", err)
		}
		if err := td.DB.Model(v).Update("verified", true).Error; err != nil {
			t.Fatalf("verify venue: %v", err)
		}
	}
	var venues []catalogm.Venue
	if err := td.DB.Where("city = ?", "Phoenix").Find(&venues).Error; err != nil {
		t.Fatalf("load venues: %v", err)
	}

	loc, err := time.LoadLocation("America/Phoenix")
	if err != nil {
		t.Fatalf("load loc: %v", err)
	}
	nowLocal := time.Now().In(loc)
	y, w := nowLocal.ISOWeek()
	weekStart := ISOWeekStart(y, w, loc)
	currentKey := ISOWeekKey(weekStart)

	for i := 0; i < 3; i++ {
		show := &catalogm.Show{
			Title:     "Week Show",
			Slug:      strPtr(fmt.Sprintf("week-show-%d", i)),
			EventDate: weekStart.Add(48 * time.Hour).UTC(),
			Status:    catalogm.ShowStatusApproved,
		}
		if err := td.DB.Create(show).Error; err != nil {
			t.Fatalf("seed show: %v", err)
		}
		if err := td.DB.Create(&catalogm.ShowVenue{ShowID: show.ID, VenueID: venues[i%len(venues)].ID}).Error; err != nil {
			t.Fatalf("seed show_venue: %v", err)
		}
	}

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
