package main

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"

	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/testutil"
)

var testNow = time.Date(2026, 11, 20, 18, 0, 0, 0, time.UTC)

const testTicket = "https://wl.seetickets.us/event/igorrr/691689"

func seedReplicas(t *testing.T, database *gorm.DB) (ids []uint, aceID uint) {
	t.Helper()
	tz := "America/Los_Angeles"
	venueSlug := "great-american-music-hall"
	venue := &catalogm.Venue{Name: "Great American Music Hall", Slug: &venueSlug, City: "San Francisco", State: "CA", Verified: true, Timezone: &tz}
	if err := database.Create(venue).Error; err != nil {
		t.Fatal(err)
	}
	artistSlug := "igorrr"
	artist := &catalogm.Artist{Name: "Igorrr", Slug: &artistSlug}
	if err := database.Create(artist).Error; err != nil {
		t.Fatal(err)
	}
	aceSlug := "testing-ace"
	ace := &catalogm.Artist{Name: "Testing Ace", Slug: &aceSlug}
	if err := database.Create(ace).Error; err != nil {
		t.Fatal(err)
	}

	insert := func(at time.Time, artistID uint, ticket string) uint {
		var id uint
		slug := fmt.Sprintf("%s-a%d", at.Format("2006-01-02"), artistID)
		if err := database.Raw(`
			INSERT INTO shows (title, event_date, state, status, source, ticket_url, slug, created_at, updated_at)
			VALUES ('', ?, 'CA', 'approved', 'user', ?, ?, ?, ?) RETURNING id`,
			at, ticket, slug, testNow, testNow).Row().Scan(&id); err != nil {
			t.Fatal(err)
		}
		if err := database.Exec(`INSERT INTO show_artists (show_id, artist_id, position, set_type) VALUES (?, ?, 0, 'headliner')`, id, artistID).Error; err != nil {
			t.Fatal(err)
		}
		if err := database.Exec(`INSERT INTO show_venues (show_id, venue_id) VALUES (?, ?)`, id, venue.ID).Error; err != nil {
			t.Fatal(err)
		}
		return id
	}
	for year := 2026; year <= 2031; year++ {
		// 20:00 Pacific on Oct 3 is 03:00Z on Oct 4.
		ids = append(ids, insert(time.Date(year, 10, 4, 3, 0, 0, 0, time.UTC), artist.ID, testTicket))
	}
	aceID = insert(time.Date(2083, 8, 15, 23, 0, 0, 0, time.UTC), ace.ID, "https://www.axs.com/events/1605473")
	return ids, aceID
}

func liveShowIDs(t *testing.T, database *gorm.DB) []uint {
	t.Helper()
	var got []uint
	if err := database.Raw(`SELECT id FROM shows ORDER BY id`).Scan(&got).Error; err != nil {
		t.Fatal(err)
	}
	return got
}

func TestRun_DryRunThenConfirmThenRerun(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	testDB := testutil.SetupTestPostgres(t)
	defer testDB.Cleanup()
	database := testDB.DB

	ids, aceID := seedReplicas(t, database)
	all := append(append([]uint{}, ids...), aceID)

	// Dry run is the default: it prints the plan and deletes nothing.
	var dry bytes.Buffer
	if code := run(database, options{beyondMonths: 18, now: testNow}, &dry); code != 0 {
		t.Fatalf("dry run exit %d\n%s", code, dry.String())
	}
	out := dry.String()
	for _, want := range []string{
		"(DRY RUN)",
		"Found 1 replicated group(s): 1 to purge (5 copies), 0 left untouched.",
		fmt.Sprintf("KEEP show %d  2026-10-03", ids[0]),
		fmt.Sprintf("DEL  show %d  2031-10-03", ids[5]),
		"Ticket URL: " + testTicket,
		fmt.Sprintf("show %d  2083-08-15", aceID),
		"DRY RUN: no database writes.",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("dry run output lacks %q\n%s", want, out)
		}
	}
	if strings.Contains(out, "[DELETED]") {
		t.Errorf("dry run reported a deletion\n%s", out)
	}
	if got := liveShowIDs(t, database); len(got) != len(all) {
		t.Fatalf("dry run deleted shows: %v left of %v", got, all)
	}

	// Confirm deletes the five later copies and nothing else.
	var live bytes.Buffer
	if code := run(database, options{confirm: true, beyondMonths: 18, now: testNow}, &live); code != 0 {
		t.Fatalf("confirm exit %d\n%s", code, live.String())
	}
	if !strings.Contains(live.String(), fmt.Sprintf("[DELETED] 5 copies; kept show %d", ids[0])) {
		t.Errorf("confirm output lacks the deletion line\n%s", live.String())
	}
	got := liveShowIDs(t, database)
	if len(got) != 2 || got[0] != ids[0] || got[1] != aceID {
		t.Fatalf("after confirm want [%d %d], got %v", ids[0], aceID, got)
	}

	// A second confirmed run finds nothing to do.
	var rerun bytes.Buffer
	if code := run(database, options{confirm: true, beyondMonths: 18, now: testNow}, &rerun); code != 0 {
		t.Fatalf("rerun exit %d\n%s", code, rerun.String())
	}
	if !strings.Contains(rerun.String(), "Found 0 replicated group(s)") {
		t.Errorf("rerun output\n%s", rerun.String())
	}
	if got := liveShowIDs(t, database); len(got) != 2 {
		t.Fatalf("rerun changed shows: %v", got)
	}
}
