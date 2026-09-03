package catalog

import (
	"context"
	"fmt"
	"sort"
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

// TestSitemapEntriesReleaseSubShardsCoverTheFamilyExactly is the behavioural
// half of the releases sub-shard guard (PSY-1763). Its sibling unit test checks
// the bounds TABLE; this checks what the DATABASE does with them, which is a
// different question — collation decides where a slug actually falls, and no
// amount of reading the table reveals that.
//
// TWO assertions, and they fail for different reasons on purpose:
//
//   - Set equality against the unsharded family. A gap (URLs silently absent
//     from the sitemap) and an overlap (a URL announced twice) both fail it.
//     Note this one is collation-INDEPENDENT: contiguous half-open ranges open
//     at both outer ends are total and disjoint under any total order, so it can
//     never fail for a collation reason, whatever the seeds are.
//   - Exact placement of the awkward slugs. THIS is the collation assertion.
//     `wantShard` below is measured against en_US.utf8, so a collation change
//     that silently moves rows between ranges — the one thing that can break the
//     "a URL does not change shards" property without the URL changing — fails
//     here rather than churning what crawlers refetch.
func TestSitemapEntriesReleaseSubShardsCoverTheFamilyExactly(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	td := testutil.SetupTestPostgres(t)
	defer td.Cleanup()

	// MEASURED against en_US.utf8 (libc), the collation both production and
	// testutil.SetupTestPostgres run. Note `-quiet-start`: glibc weighs
	// punctuation below letters at the primary level, so it collates as
	// "quietstart" and lands in n-s — NOT in the first range, which reading the
	// leading character would suggest. That is exactly why this is pinned.
	wantShard := map[string]string{
		"1999-remastered":   "releases-a-e", // digits sort below every letter
		"-quiet-start":      "releases-n-s", // collates as "quietstart"
		"aardvark-tapes":    "releases-a-e",
		"eventide":          "releases-a-e", // last slug under the first cut point
		"f":                 "releases-f-m", // exactly on a cut point
		"midnight-sessions": "releases-f-m",
		"n":                 "releases-n-s", // exactly on a cut point
		"solar-drift":       "releases-n-s",
		"t":                 "releases-t-z", // exactly on a cut point
		"zephyr":            "releases-t-z",
	}
	seeded := make([]string, 0, len(wantShard))
	for slug := range wantShard {
		seeded = append(seeded, slug)
	}
	sort.Strings(seeded)
	for i, slug := range seeded {
		release := &catalogm.Release{Title: fmt.Sprintf("Release %d", i), Slug: strPtr(slug)}
		if err := td.DB.Create(release).Error; err != nil {
			t.Fatalf("seed release %q: %v", slug, err)
		}
	}

	service := NewSitemapService(td.DB)
	whole, err := service.Entries(context.Background(), "releases")
	if err != nil {
		t.Fatalf("Entries(releases): %v", err)
	}
	wholeSlugs := sitemapSlugsOf(whole.Releases)
	if len(wholeSlugs) != len(seeded) {
		t.Fatalf("unsharded releases = %v, want the %d seeded slugs", wholeSlugs, len(seeded))
	}

	owner := map[string]string{}
	for _, shard := range releaseShards {
		entries, err := service.Entries(context.Background(), shard.id)
		if err != nil {
			t.Fatalf("Entries(%s): %v", shard.id, err)
		}
		// A sub-shard addresses ONE family: anything else leaking into the
		// response would be paid for by every shard, which is the cost
		// sharding exists to avoid.
		if len(entries.Artists)+len(entries.Shows)+len(entries.Labels) != 0 {
			t.Errorf("Entries(%s) populated families other than releases", shard.id)
		}
		for _, slug := range sitemapSlugsOf(entries.Releases) {
			if prev, dup := owner[slug]; dup {
				t.Errorf("slug %q is served by both %q and %q — the shards overlap", slug, prev, shard.id)
			}
			owner[slug] = shard.id
		}
	}

	for _, slug := range wholeSlugs {
		if _, ok := owner[slug]; !ok {
			t.Errorf("slug %q belongs to no sub-shard — the partition has a gap and this URL would leave the sitemap", slug)
		}
	}
	if len(owner) != len(wholeSlugs) {
		t.Errorf("sub-shards served %d slugs, the whole family has %d", len(owner), len(wholeSlugs))
	}

	// The collation assertion. Set equality above holds under any total order,
	// so it cannot catch a collation change reshuffling rows between ranges —
	// only this can.
	for slug, want := range wantShard {
		if got := owner[slug]; got != want {
			t.Errorf("slug %q is served by %q, want %q — the database's collation orders "+
				"slugs differently than when these bounds were measured, so releases have "+
				"moved between shards without their URLs changing", slug, got, want)
		}
	}
}

// TestSitemapEntriesShowSubShardsCoverTheFamilyExactly is the behavioural half
// of the shows sub-shard guard (PSY-2018), the same pair of assertions the
// releases test above makes against a different partition key.
//
//   - Set equality against the unsharded family. A gap (URLs silently absent
//     from the sitemap) and an overlap (a URL announced twice) both fail it.
//   - Exact placement of the boundary instants. The ranges are half-open on UTC
//     month starts, so the instant AT a cut point belongs to the later shard and
//     the last microsecond before it to the earlier one. That is the property a
//     hand-edited bound gets wrong, and the one no set-equality check can see.
//
// The visibility predicate is asserted too: narrowing a scope must not lose the
// approved-only filter, or a shard would announce URLs GetShowHandler rejects.
func TestSitemapEntriesShowSubShardsCoverTheFamilyExactly(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	td := testutil.SetupTestPostgres(t)
	defer td.Cleanup()

	// Keyed by slug so a failure names the row. Every instant is UTC, which is
	// what event_date holds (migration 000028 made the column timestamptz).
	wantShard := map[string]struct {
		at   time.Time
		want string
	}{
		"long-ago":            {time.Date(2020, 3, 1, 0, 0, 0, 0, time.UTC), "shows-before-2026"},
		"last-instant-before": {time.Date(2025, 12, 31, 23, 59, 59, 999999000, time.UTC), "shows-before-2026"},
		"first-enumerated":    {time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), "shows-2026-01"},
		"mid-month":           {time.Date(2026, 9, 17, 3, 30, 0, 0, time.UTC), "shows-2026-09"},
		"month-end":           {time.Date(2026, 9, 30, 23, 59, 59, 999999000, time.UTC), "shows-2026-09"},
		"month-start":         {time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC), "shows-2026-10"},
		"year-end":            {time.Date(2026, 12, 31, 23, 59, 59, 999999000, time.UTC), "shows-2026-12"},
		"year-start":          {time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC), "shows-2027-01"},
		"last-enumerated":     {time.Date(2027, 12, 31, 23, 59, 59, 999999000, time.UTC), "shows-2027-12"},
		"first-after-span":    {time.Date(2028, 1, 1, 0, 0, 0, 0, time.UTC), "shows-from-2028"},
		"far-future":          {time.Date(2031, 6, 15, 12, 0, 0, 0, time.UTC), "shows-from-2028"},
	}

	seeded := make([]string, 0, len(wantShard))
	for slug := range wantShard {
		seeded = append(seeded, slug)
	}
	sort.Strings(seeded)
	for _, slug := range seeded {
		show := &catalogm.Show{
			Title:     "Show " + slug,
			Slug:      strPtr(slug),
			EventDate: wantShard[slug].at,
			Status:    catalogm.ShowStatusApproved,
		}
		if err := td.DB.Create(show).Error; err != nil {
			t.Fatalf("seed show %q: %v", slug, err)
		}
	}
	// Dated inside an enumerated month, so a shard that dropped the status
	// predicate while narrowing would announce it.
	pending := &catalogm.Show{
		Title:     "Pending Show",
		Slug:      strPtr("pending-show"),
		EventDate: time.Date(2026, 9, 18, 1, 0, 0, 0, time.UTC),
		Status:    catalogm.ShowStatusPending,
	}
	if err := td.DB.Create(pending).Error; err != nil {
		t.Fatalf("seed pending show: %v", err)
	}

	service := NewSitemapService(td.DB)
	whole, err := service.Entries(context.Background(), "shows")
	if err != nil {
		t.Fatalf("Entries(shows): %v", err)
	}
	wholeSlugs := sitemapSlugsOf(whole.Shows)
	if len(wholeSlugs) != len(seeded) {
		t.Fatalf("unsharded shows = %v, want the %d approved seeded slugs", wholeSlugs, len(seeded))
	}

	owner := map[string]string{}
	for _, shard := range sitemapShardsByFamily["shows"] {
		entries, err := service.Entries(context.Background(), shard.id)
		if err != nil {
			t.Fatalf("Entries(%s): %v", shard.id, err)
		}
		// A sub-shard addresses ONE family: anything else leaking into the
		// response would be paid for by every shard, which is the cost sharding
		// exists to avoid.
		if len(entries.Artists)+len(entries.Releases)+len(entries.Labels) != 0 {
			t.Errorf("Entries(%s) populated families other than shows", shard.id)
		}
		for _, slug := range sitemapSlugsOf(entries.Shows) {
			if prev, dup := owner[slug]; dup {
				t.Errorf("slug %q is served by both %q and %q — the shards overlap", slug, prev, shard.id)
			}
			owner[slug] = shard.id
		}
	}

	for _, slug := range wholeSlugs {
		if _, ok := owner[slug]; !ok {
			t.Errorf("slug %q belongs to no sub-shard — the partition has a gap and this URL would leave the sitemap", slug)
		}
	}
	if len(owner) != len(wholeSlugs) {
		t.Errorf("sub-shards served %d slugs, the whole family has %d", len(owner), len(wholeSlugs))
	}
	if shard, served := owner["pending-show"]; served {
		t.Errorf("shard %q announced a pending show — narrowing dropped the approved-only predicate", shard)
	}

	for slug, want := range wantShard {
		if got := owner[slug]; got != want.want {
			t.Errorf("a show dated %s is served by %q, want %q — the month bounds are half-open on UTC month starts",
				want.at.Format(time.RFC3339Nano), got, want.want)
		}
	}
}

// TestEverySubShardedFamilyIsActuallyNarrowed is the guard the two family-
// specific tests above cannot be: it is driven by the shard TABLE, so a family
// added to sitemapShardsByFamily inherits it without anyone writing a third
// hand-rolled near-duplicate.
//
// The failure it exists to catch is a missing `shard.narrow` at a new family's
// call site in Entries. Nothing else notices: the table is generic, the wire
// enum accepts the ids, every shard answers 200, and each one returns the WHOLE
// family — so the over-cap payload sharding exists to prevent is served by every
// shard at once, with a green build and a green test run. Here it shows up as
// each shard serving the full row set instead of a slice of it.
//
// Deliberately does NOT assert per-family placement or emptiness: which rows
// land where is the business of the family-specific tests above, which seed for
// it. This asserts only the two properties that must hold for EVERY sub-sharded
// family — the shards partition the family, and no shard is the whole of it.
func TestEverySubShardedFamilyIsActuallyNarrowed(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	td := testutil.SetupTestPostgres(t)
	defer td.Cleanup()

	// Two rows per family, placed in different shards, so "narrowed" is
	// distinguishable from "returns everything". The shows pair straddles a
	// month boundary and the releases pair a slug cut point.
	for i, slug := range []string{"aardvark-tapes", "zephyr"} {
		release := &catalogm.Release{Title: fmt.Sprintf("Narrow Release %d", i), Slug: strPtr(slug)}
		if err := td.DB.Create(release).Error; err != nil {
			t.Fatalf("seed release %q: %v", slug, err)
		}
	}
	for i, at := range []time.Time{
		time.Date(2026, 9, 10, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 10, 10, 0, 0, 0, 0, time.UTC),
	} {
		show := &catalogm.Show{
			Title:     fmt.Sprintf("Narrow Show %d", i),
			Slug:      strPtr(fmt.Sprintf("narrow-show-%d", i)),
			EventDate: at,
			Status:    catalogm.ShowStatusApproved,
		}
		if err := td.DB.Create(show).Error; err != nil {
			t.Fatalf("seed show %d: %v", i, err)
		}
	}

	service := NewSitemapService(td.DB)
	// Read the populated field by family so the assertions below stay generic.
	rowsOf := map[string]func(*contracts.SitemapEntries) []contracts.SitemapEntry{
		"shows":    func(e *contracts.SitemapEntries) []contracts.SitemapEntry { return e.Shows },
		"releases": func(e *contracts.SitemapEntries) []contracts.SitemapEntry { return e.Releases },
	}

	for family, shards := range sitemapShardsByFamily {
		read, ok := rowsOf[family]
		if !ok {
			t.Fatalf("family %q is sub-sharded but this test cannot read its rows — add it to rowsOf", family)
		}

		whole, err := service.Entries(context.Background(), family)
		if err != nil {
			t.Fatalf("Entries(%s): %v", family, err)
		}
		wholeSlugs := sitemapSlugsOf(read(whole))
		if len(wholeSlugs) < 2 {
			t.Fatalf("family %q seeded %d rows, need at least 2 in different shards", family, len(wholeSlugs))
		}

		owner := map[string]string{}
		for _, shard := range shards {
			entries, err := service.Entries(context.Background(), shard.id)
			if err != nil {
				t.Fatalf("Entries(%s): %v", shard.id, err)
			}
			slugs := sitemapSlugsOf(read(entries))
			if len(slugs) == len(wholeSlugs) && len(wholeSlugs) > 1 {
				t.Errorf("shard %q served all %d rows of %q — its call site in Entries is not narrowing",
					shard.id, len(slugs), family)
			}
			for _, slug := range slugs {
				if prev, dup := owner[slug]; dup {
					t.Errorf("slug %q is served by both %q and %q — the %q shards overlap", slug, prev, shard.id, family)
				}
				owner[slug] = shard.id
			}
		}

		for _, slug := range wholeSlugs {
			if _, ok := owner[slug]; !ok {
				t.Errorf("slug %q of family %q belongs to no shard — the partition has a gap", slug, family)
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
