package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
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

// digestIn reads the plan digest a run printed.
func digestIn(t *testing.T, out string) string {
	t.Helper()
	m := regexp.MustCompile(`Plan digest: +([0-9a-f]+)`).FindStringSubmatch(out)
	if m == nil {
		t.Fatalf("no plan digest in output\n%s", out)
	}
	return m[1]
}

func TestRun_DryRunThenConfirmThenRerun(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	testDB := testutil.SetupTestPostgres(t)
	defer testDB.Cleanup()
	database := testDB.DB
	backup := filepath.Join(t.TempDir(), "deleted.jsonl")

	ids, aceID := seedReplicas(t, database)
	all := append(append([]uint{}, ids...), aceID)

	// Dry run is the default: it prints the plan and its digest and deletes nothing.
	var dry bytes.Buffer
	if code := run(database, options{beyondMonths: 18, now: testNow, target: "test-db"}, &dry); code != 0 {
		t.Fatalf("dry run exit %d\n%s", code, dry.String())
	}
	out := dry.String()
	for _, want := range []string{
		"(DRY RUN)",
		"Target: test-db",
		"Found 1 replicated group(s): 1 to purge (5 copies), 0 left untouched.",
		fmt.Sprintf("KEEP show %d  2026-10-03", ids[0]),
		fmt.Sprintf("DEL  show %d  2031-10-03", ids[5]),
		"Ticket URL: " + testTicket,
		fmt.Sprintf("show %d  2083-08-15", aceID),
		"DRY RUN: no database writes.",
		"--confirm --expect-plan ",
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
	digest := digestIn(t, out)

	// Confirm without the reviewed digest and a backup, or against another
	// target, is refused outright.
	for name, opts := range map[string]options{
		"no digest":      {confirm: true, backupPath: backup, beyondMonths: 18, now: testNow, target: "test-db"},
		"no backup":      {confirm: true, expectPlan: digest, beyondMonths: 18, now: testNow, target: "test-db"},
		"wrong digest":   {confirm: true, expectPlan: "0000000000000000", backupPath: backup, beyondMonths: 18, now: testNow, target: "test-db"},
		"another target": {confirm: true, expectPlan: digest, backupPath: backup, beyondMonths: 18, now: testNow, target: "other-db"},
	} {
		var refused bytes.Buffer
		if code := run(database, opts, &refused); code != 1 {
			t.Errorf("%s: want exit 1, got %d\n%s", name, code, refused.String())
		}
		if got := liveShowIDs(t, database); len(got) != len(all) {
			t.Fatalf("%s: shows were deleted: %v", name, got)
		}
	}
	if _, err := os.Stat(backup); !os.IsNotExist(err) {
		t.Fatalf("a refused run created the backup file: %v", err)
	}

	// An existing backup file is never overwritten, even on a matching plan.
	existing := filepath.Join(t.TempDir(), "earlier.jsonl")
	if err := os.WriteFile(existing, []byte("prior\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var clash bytes.Buffer
	if code := run(database, options{confirm: true, expectPlan: digest, backupPath: existing, beyondMonths: 18, now: testNow, target: "test-db"}, &clash); code != 1 {
		t.Fatalf("existing backup file: want exit 1, got %d\n%s", code, clash.String())
	}
	if !strings.Contains(clash.String(), "backup file:") {
		t.Errorf("existing backup file: output\n%s", clash.String())
	}
	if kept, _ := os.ReadFile(existing); string(kept) != "prior\n" {
		t.Fatalf("existing backup file was changed: %q", kept)
	}
	if got := liveShowIDs(t, database); len(got) != len(all) {
		t.Fatalf("existing backup file: shows were deleted: %v", got)
	}

	// Confirm with the reviewed digest deletes the five later copies and nothing else.
	var live bytes.Buffer
	if code := run(database, options{confirm: true, expectPlan: digest, backupPath: backup, beyondMonths: 18, now: testNow, target: "test-db"}, &live); code != 0 {
		t.Fatalf("confirm exit %d\n%s", code, live.String())
	}
	if !strings.Contains(live.String(), fmt.Sprintf("[DELETED] 5 copies; kept show %d", ids[0])) {
		t.Errorf("confirm output lacks the deletion line\n%s", live.String())
	}
	if !strings.Contains(live.String(), "/shows/2031-10-04-a") {
		t.Errorf("confirm output lacks the deleted slugs\n%s", live.String())
	}
	got := liveShowIDs(t, database)
	if len(got) != 2 || got[0] != ids[0] || got[1] != aceID {
		t.Fatalf("after confirm want [%d %d], got %v", ids[0], aceID, got)
	}
	lines, err := os.ReadFile(backup)
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(string(lines), "\n"); n != 6 {
		t.Fatalf("backup holds %d lines, want 5 snapshots and 1 outcome\n%s", n, lines)
	}
	if !strings.Contains(string(lines), fmt.Sprintf(`"group_kept_show_id":%d,"group_deleted_show_ids":[%d,%d,%d,%d,%d],"committed":true`,
		ids[0], ids[1], ids[2], ids[3], ids[4], ids[5])) {
		t.Errorf("backup lacks the committed outcome line\n%s", lines)
	}
	if !strings.Contains(string(lines), fmt.Sprintf(`"kept_show_id":%d`, ids[0])) {
		t.Errorf("backup does not name the survivor\n%s", lines)
	}

	// A second run finds nothing to do.
	var rerun bytes.Buffer
	if code := run(database, options{beyondMonths: 18, now: testNow}, &rerun); code != 0 {
		t.Fatalf("rerun exit %d\n%s", code, rerun.String())
	}
	if !strings.Contains(rerun.String(), "Found 0 replicated group(s)") {
		t.Errorf("rerun output\n%s", rerun.String())
	}
	if got := liveShowIDs(t, database); len(got) != 2 {
		t.Fatalf("rerun changed shows: %v", got)
	}
}
