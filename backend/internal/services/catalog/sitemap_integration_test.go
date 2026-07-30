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