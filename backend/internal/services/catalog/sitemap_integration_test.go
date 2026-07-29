package catalog

import (
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
	if err := td.DB.Create(&catalogm.Venue{Name: "Slugged Venue", Slug: strPtr("slugged-venue"), City: "Phoenix", State: "AZ"}).Error; err != nil {
		t.Fatalf("seed venue: %v", err)
	}

	entries, err := NewSitemapService(td.DB).Entries()
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

	for _, e := range entries.Shows {
		if e.UpdatedAt.IsZero() {
			t.Errorf("show %q has a zero UpdatedAt — <lastmod> would be meaningless", e.Slug)
		}
	}
}

// TestSitemapEntriesEmptyFamiliesSerialiseAsSlices guards the generator against
// a nil family. The consumer iterates each list directly; a JSON null would
// force a nil check that is easy to forget and silent when omitted.
func TestSitemapEntriesEmptyFamiliesSerialiseAsSlices(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	td := testutil.SetupTestPostgres(t)
	defer td.Cleanup()

	entries, err := NewSitemapService(td.DB).Entries()
	if err != nil {
		t.Fatalf("Entries: %v", err)
	}

	if entries.Shows == nil || entries.Artists == nil || entries.Venues == nil {
		t.Fatalf("empty families must be non-nil slices, got shows=%v artists=%v venues=%v",
			entries.Shows, entries.Artists, entries.Venues)
	}
}

// TestSitemapEntriesOrderIsTotal pins the ordering contract: freshest first,
// slug as the tiebreak. Equal timestamps are the norm after a bulk ingest, so
// without the tiebreak the response order is undefined and the freshness
// monitor cannot diff two fetches meaningfully.
func TestSitemapEntriesOrderIsTotal(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	td := testutil.SetupTestPostgres(t)
	defer td.Cleanup()

	sameInstant := time.Now().UTC().Truncate(time.Second)
	for _, slug := range []string{"charlie", "alpha", "bravo"} {
		a := &catalogm.Artist{Name: "Artist " + slug, Slug: strPtr(slug)}
		if err := td.DB.Create(a).Error; err != nil {
			t.Fatalf("seed artist %q: %v", slug, err)
		}
		// UpdatedAt is maintained by GORM on write, so force the collision
		// explicitly rather than hoping three inserts land in the same second.
		if err := td.DB.Model(a).UpdateColumn("updated_at", sameInstant).Error; err != nil {
			t.Fatalf("pin updated_at for %q: %v", slug, err)
		}
	}

	entries, err := NewSitemapService(td.DB).Entries()
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
			t.Fatalf("artists = %v, want %v (ties must break on slug ASC)", got, want)
		}
	}
}

// TestSitemapEntriesIssuesOneQueryPerFamily is the regression guard for the
// defect this service was built to fix. The old generator read these columns
// off the public list endpoints, which Preload venues and artists per show —
// 4.6 MB and 15.5 s, past the fetch budget, silently yielding an empty sitemap.
// Three projections, three queries. If a Preload ever creeps in here, the count
// climbs and this fails.
func TestSitemapEntriesIssuesOneQueryPerFamily(t *testing.T) {
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

	var queries int
	counting := td.DB.Session(&gorm.Session{Logger: queryCounter{Interface: td.DB.Logger, n: &queries}})

	if _, err := NewSitemapService(counting).Entries(); err != nil {
		t.Fatalf("Entries: %v", err)
	}

	if queries != 3 {
		t.Errorf("Entries issued %d queries, want 3 (one projection per family) — a Preload or N+1 has crept in", queries)
	}
}
