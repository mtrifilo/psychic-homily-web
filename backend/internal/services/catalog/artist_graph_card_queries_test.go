package catalog

import (
	"context"
	"testing"
	"time"

	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/testutil"
)

// queryCounter is a GORM logger whose Trace runs once per executed SQL
// statement, letting a test count the queries a service method issues.
type queryCounter struct {
	gormlogger.Interface
	n *int
}

func (q queryCounter) Trace(ctx context.Context, begin time.Time, fc func() (string, int64), err error) {
	*q.n++
}

// TestGraphCardQuerySlimming verifies PSY-1352: the graph-card's artist and
// next-show reads issue far fewer queries via the lean GetArtistSummary /
// GetNextShowForArtist than the full GetArtist / GetShowsForArtist they
// replaced — and the changed portion at least halves.
func TestGraphCardQuerySlimming(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	td := testutil.SetupTestPostgres(t)
	defer td.Cleanup()

	// Seed: an artist on one upcoming, approved show at a venue.
	artist := &catalogm.Artist{Name: "Query Count Test Band"}
	if err := td.DB.Create(artist).Error; err != nil {
		t.Fatalf("seed artist: %v", err)
	}
	tz := "America/Phoenix"
	venue := &catalogm.Venue{Name: "The Test Venue", City: "Phoenix", State: "AZ", Timezone: &tz}
	if err := td.DB.Create(venue).Error; err != nil {
		t.Fatalf("seed venue: %v", err)
	}
	show := &catalogm.Show{
		Title:     "Upcoming Show",
		EventDate: time.Now().Add(14 * 24 * time.Hour),
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

	// A DB session whose logger counts every statement; the service issues its
	// queries through it.
	var n int
	countingDB := td.DB.Session(&gorm.Session{
		Logger: queryCounter{Interface: gormlogger.Default.LogMode(gormlogger.Silent), n: &n},
	})
	svc := NewArtistService(countingDB)

	measure := func(fn func()) int {
		n = 0
		fn()
		return n
	}

	fullArtist := measure(func() { _, _ = svc.GetArtist(artist.ID) })
	leanArtist := measure(func() { _, _ = svc.GetArtistSummary(artist.ID) })
	fullShows := measure(func() { _, _, _ = svc.GetShowsForArtist(artist.ID, "UTC", 1, "upcoming") })
	leanShow := measure(func() { _, _ = svc.GetNextShowForArtist(artist.ID, "UTC") })

	t.Logf("graph-card changed reads (queries): artist %d→%d, next-show %d→%d",
		fullArtist, leanArtist, fullShows, leanShow)

	if leanArtist >= fullArtist {
		t.Errorf("GetArtistSummary (%d) must issue fewer queries than GetArtist (%d)", leanArtist, fullArtist)
	}
	if leanShow >= fullShows {
		t.Errorf("GetNextShowForArtist (%d) must issue fewer queries than GetShowsForArtist (%d)", leanShow, fullShows)
	}
	// The AC: the changed portion of the graph-card at least halves.
	oldTotal, newTotal := fullArtist+fullShows, leanArtist+leanShow
	if newTotal*2 > oldTotal {
		t.Errorf("changed graph-card reads must at least halve: old=%d new=%d", oldTotal, newTotal)
	}

	// The lean methods must still return correct data (no shape regression).
	summary, err := svc.GetArtistSummary(artist.ID)
	if err != nil || summary == nil || summary.Name != "Query Count Test Band" {
		t.Errorf("GetArtistSummary returned wrong data: %+v (err %v)", summary, err)
	}
	if summary != nil && summary.Stats != nil {
		t.Errorf("GetArtistSummary must NOT populate the stats block, got %+v", summary.Stats)
	}
	next, err := svc.GetNextShowForArtist(artist.ID, "UTC")
	if err != nil || next == nil {
		t.Fatalf("GetNextShowForArtist returned no show: %+v (err %v)", next, err)
	}
	if next.ID != show.ID || next.Venue == nil || next.Venue.Name != "The Test Venue" {
		t.Errorf("GetNextShowForArtist wrong data: %+v", next)
	}
}
