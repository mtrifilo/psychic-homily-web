package catalog

import (
	"fmt"
	"testing"
	"time"

	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/testutil"
)

// seedBilledShows creates n approved shows at one venue, each with the same
// three artists billed in a fixed, non-alphabetical position order.
//
// Sharing the artists across every show is deliberate: it is the case where the
// batched artist read has to dedupe, and the case where a naive per-show reader
// re-fetches the same rows n times.
func seedBilledShows(t *testing.T, db *gorm.DB, n int) []uint {
	t.Helper()

	tz := "America/Phoenix"
	venue := &catalogm.Venue{Name: "Batching Test Venue", City: "Phoenix", State: "AZ", Timezone: &tz}
	if err := db.Create(venue).Error; err != nil {
		t.Fatalf("seed venue: %v", err)
	}

	// Created out of bill order so a test that passes only because the artists
	// happen to come back in id order is not mistaken for a passing test.
	headliner := &catalogm.Artist{Name: "Zeta Headliner"}
	support := &catalogm.Artist{Name: "Alpha Support"}
	opener := &catalogm.Artist{Name: "Mu Opener"}
	for _, a := range []*catalogm.Artist{headliner, support, opener} {
		if err := db.Create(a).Error; err != nil {
			t.Fatalf("seed artist %s: %v", a.Name, err)
		}
	}
	bill := []struct {
		artist   *catalogm.Artist
		position int
		setType  string
	}{
		{headliner, 0, "headliner"},
		{support, 1, "performer"},
		{opener, 2, "opener"},
	}

	showIDs := make([]uint, 0, n)
	for i := 0; i < n; i++ {
		show := &catalogm.Show{
			Title:     fmt.Sprintf("Batching Test Show %d", i),
			EventDate: time.Now().Add(time.Duration(24*(i+1)) * time.Hour),
			City:      stringPtr("Phoenix"),
			State:     stringPtr("AZ"),
			Status:    catalogm.ShowStatusApproved,
		}
		if err := db.Create(show).Error; err != nil {
			t.Fatalf("seed show %d: %v", i, err)
		}
		if err := db.Create(&catalogm.ShowVenue{ShowID: show.ID, VenueID: venue.ID}).Error; err != nil {
			t.Fatalf("seed show_venue %d: %v", i, err)
		}
		for _, b := range bill {
			sa := &catalogm.ShowArtist{
				ShowID:   show.ID,
				ArtistID: b.artist.ID,
				Position: b.position,
				SetType:  b.setType,
			}
			if err := db.Create(sa).Error; err != nil {
				t.Fatalf("seed show_artist %d: %v", i, err)
			}
		}
		showIDs = append(showIDs, show.ID)
	}

	return showIDs
}

// TestShowListBillsAreBatched verifies PSY-1830: the show list endpoints resolve
// the whole page's bills in a fixed number of queries instead of two per row.
//
// The assertion is O(1)-in-N rather than a hardcoded budget: the same endpoint is
// measured over a 2-row page and a 10-row page and the counts must be IDENTICAL.
// A per-row reader cannot satisfy that no matter what its constant is, and a
// fixed number would rot the first time an unrelated per-page query is added.
func TestShowListBillsAreBatched(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	td := testutil.SetupTestPostgres(t)
	defer td.Cleanup()

	seedBilledShows(t, td.DB, 10)

	var n int
	countingDB := td.DB.Session(&gorm.Session{
		Logger: queryCounter{Interface: gormlogger.Default.LogMode(gormlogger.Silent), n: &n},
	})
	svc := NewShowService(countingDB)

	measure := func(fn func()) int {
		n = 0
		fn()
		return n
	}

	getShows := func(limit int) func() {
		return func() {
			resp, _, err := svc.GetShows(map[string]interface{}{}, contracts.ShowsQuery{Limit: limit})
			if err != nil {
				t.Fatalf("GetShows(limit=%d): %v", limit, err)
			}
			if len(resp) != limit {
				t.Fatalf("GetShows(limit=%d) returned %d shows", limit, len(resp))
			}
		}
	}

	small := measure(getShows(2))
	large := measure(getShows(10))
	t.Logf("GetShows queries: 2-row page=%d, 10-row page=%d", small, large)

	if small != large {
		t.Errorf("GetShows must issue the same number of queries regardless of page size, "+
			"got %d for 2 rows and %d for 10 rows (%.1f extra queries per row)",
			small, large, float64(large-small)/8.0)
	}

	// The same invariant for the cursor-paged public list and the admin queue,
	// the two other callers that page over buildShowResponses.
	upcomingSmall := measure(func() {
		if _, _, _, err := svc.GetUpcomingShows("UTC", "", 2, false, nil); err != nil {
			t.Fatalf("GetUpcomingShows(2): %v", err)
		}
	})
	upcomingLarge := measure(func() {
		if _, _, _, err := svc.GetUpcomingShows("UTC", "", 10, false, nil); err != nil {
			t.Fatalf("GetUpcomingShows(10): %v", err)
		}
	})
	t.Logf("GetUpcomingShows queries: 2-row page=%d, 10-row page=%d", upcomingSmall, upcomingLarge)
	if upcomingSmall != upcomingLarge {
		t.Errorf("GetUpcomingShows must not scale queries with page size, got %d vs %d",
			upcomingSmall, upcomingLarge)
	}
}

// TestShowListBillOrderingUnchanged pins the response contract the batching had
// to preserve: every list endpoint still returns each show's bill in position
// order with set_type and the headliner flag intact, and a show with no billed
// artists still serializes as an empty bill rather than a null one.
func TestShowListBillOrderingUnchanged(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	td := testutil.SetupTestPostgres(t)
	defer td.Cleanup()

	seedBilledShows(t, td.DB, 3)

	// A fourth show with no bill at all, to prove the empty case is [] and not
	// null now that the bill arrives from a map rather than a per-show query.
	billless := &catalogm.Show{
		Title:     "No Bill Yet",
		EventDate: time.Now().Add(200 * time.Hour),
		City:      stringPtr("Phoenix"),
		State:     stringPtr("AZ"),
		Status:    catalogm.ShowStatusApproved,
	}
	if err := td.DB.Create(billless).Error; err != nil {
		t.Fatalf("seed billless show: %v", err)
	}

	svc := NewShowService(td.DB)

	assertBill := func(t *testing.T, label string, resp *contracts.ShowResponse) {
		t.Helper()
		if resp.ID == billless.ID {
			if resp.Artists == nil {
				t.Errorf("%s: show %d with no bill must carry an empty artists slice, got nil (serializes as null)",
					label, resp.ID)
			}
			if len(resp.Artists) != 0 {
				t.Errorf("%s: show %d has no show_artists rows but got %d artists", label, resp.ID, len(resp.Artists))
			}
			return
		}

		wantNames := []string{"Zeta Headliner", "Alpha Support", "Mu Opener"}
		if len(resp.Artists) != len(wantNames) {
			t.Fatalf("%s: show %d wanted %d artists, got %d", label, resp.ID, len(wantNames), len(resp.Artists))
		}
		for i, want := range wantNames {
			got := resp.Artists[i]
			if got.Name != want {
				t.Errorf("%s: show %d artist[%d] = %q, want %q (position ASC)", label, resp.ID, i, got.Name, want)
			}
			if got.Position != i {
				t.Errorf("%s: show %d artist[%d] position = %d, want %d", label, resp.ID, i, got.Position, i)
			}
		}
		if resp.Artists[0].SetType != "headliner" {
			t.Errorf("%s: show %d headliner set_type = %q", label, resp.ID, resp.Artists[0].SetType)
		}
		if resp.Artists[0].IsHeadliner == nil || !*resp.Artists[0].IsHeadliner {
			t.Errorf("%s: show %d position-0 artist must be flagged headliner", label, resp.ID)
		}
		if resp.Artists[2].IsHeadliner == nil || *resp.Artists[2].IsHeadliner {
			t.Errorf("%s: show %d opener must not be flagged headliner", label, resp.ID)
		}
	}

	listed, _, err := svc.GetShows(map[string]interface{}{}, contracts.ShowsQuery{Limit: 50})
	if err != nil {
		t.Fatalf("GetShows: %v", err)
	}
	if len(listed) != 4 {
		t.Fatalf("GetShows returned %d shows, want 4", len(listed))
	}
	for _, resp := range listed {
		assertBill(t, "GetShows", resp)
	}

	upcoming, _, _, err := svc.GetUpcomingShows("UTC", "", 50, false, nil)
	if err != nil {
		t.Fatalf("GetUpcomingShows: %v", err)
	}
	if len(upcoming) != 4 {
		t.Fatalf("GetUpcomingShows returned %d shows, want 4", len(upcoming))
	}
	for _, resp := range upcoming {
		assertBill(t, "GetUpcomingShows", resp)
	}

	// The batched list and the single-show read must agree exactly: they share
	// one assembler, and this is what pins that they still do.
	for _, resp := range listed {
		single, err := svc.GetShow(resp.ID)
		if err != nil {
			t.Fatalf("GetShow(%d): %v", resp.ID, err)
		}
		if len(single.Artists) != len(resp.Artists) {
			t.Fatalf("GetShow(%d) bill length %d != list bill length %d",
				resp.ID, len(single.Artists), len(resp.Artists))
		}
		for i := range resp.Artists {
			if single.Artists[i].ID != resp.Artists[i].ID ||
				single.Artists[i].Position != resp.Artists[i].Position ||
				single.Artists[i].SetType != resp.Artists[i].SetType {
				t.Errorf("GetShow(%d) artist[%d] = %+v, list gave %+v",
					resp.ID, i, single.Artists[i], resp.Artists[i])
			}
		}
	}
}

// TestPendingShowsBillsBatched covers the admin queue, which pages over pending
// rather than approved shows and orders by source_venue: same batching, same
// bill contract, different base query.
func TestPendingShowsBillsBatched(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	td := testutil.SetupTestPostgres(t)
	defer td.Cleanup()

	showIDs := seedBilledShows(t, td.DB, 6)
	if err := td.DB.Model(&catalogm.Show{}).Where("id IN ?", showIDs).
		Update("status", catalogm.ShowStatusPending).Error; err != nil {
		t.Fatalf("flip shows to pending: %v", err)
	}

	var n int
	countingDB := td.DB.Session(&gorm.Session{
		Logger: queryCounter{Interface: gormlogger.Default.LogMode(gormlogger.Silent), n: &n},
	})
	svc := NewShowService(countingDB)

	measure := func(limit int) (int, []*contracts.ShowResponse) {
		n = 0
		shows, _, err := svc.GetPendingShows(limit, 0, nil)
		if err != nil {
			t.Fatalf("GetPendingShows(%d): %v", limit, err)
		}
		return n, shows
	}

	small, _ := measure(2)
	large, shows := measure(6)
	t.Logf("GetPendingShows queries: 2-row page=%d, 6-row page=%d", small, large)

	if small != large {
		t.Errorf("GetPendingShows must not scale queries with page size, got %d vs %d", small, large)
	}
	if len(shows) != 6 {
		t.Fatalf("GetPendingShows returned %d shows, want 6", len(shows))
	}
	for _, resp := range shows {
		if len(resp.Artists) != 3 {
			t.Fatalf("pending show %d has %d artists, want 3", resp.ID, len(resp.Artists))
		}
		if resp.Artists[0].Name != "Zeta Headliner" || resp.Artists[2].Name != "Mu Opener" {
			t.Errorf("pending show %d bill out of position order: %q, %q, %q",
				resp.ID, resp.Artists[0].Name, resp.Artists[1].Name, resp.Artists[2].Name)
		}
	}
}
