package catalog

import (
	"os"
	"testing"
	"time"
)

// PSY-1322 sub-stream schedule parser, against a real wfmu.org/drummer
// snapshot (captured 2026-07-02, a Thursday — the fixture's rolling week runs
// Thursday..Wednesday with the Thursday group partial, starting 12-3pm).

func TestParseWFMUSubstreamSchedule_Fixture(t *testing.T) {
	body, err := os.ReadFile("testdata/wfmu_substream_drummer.html")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	entries, skipped, err := parseWFMUSubstreamSchedule(body)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if skipped != 0 {
		t.Errorf("skipped = %d, want 0 (every slot row in the snapshot is well-formed)", skipped)
	}

	// Ground truth extracted from the snapshot by independent regex: 33 slot
	// rows across 32 distinct codes (one show airs twice in the window).
	totalSlots := 0
	byCode := map[string]WFMUScheduleEntry{}
	for _, e := range entries {
		totalSlots += len(e.Slots)
		byCode[e.Code] = e
	}
	if len(entries) != 32 {
		t.Errorf("entries = %d, want 32 distinct shows", len(entries))
	}
	if totalSlots != 33 {
		t.Errorf("total slots = %d, want 33", totalSlots)
	}

	// First slot of the partial Thursday group: Wound Liquor, 12-3pm.
	wq, ok := byCode["WQ"]
	if !ok {
		t.Fatal("code WQ (Wound Liquor) not parsed")
	}
	if wq.Name != "Wound Liquor with Olleh" {
		t.Errorf("WQ name = %q", wq.Name)
	}
	if len(wq.Slots) != 1 || wq.Slots[0].DayOfWeek != 4 || wq.Slots[0].Start != "12:00" || wq.Slots[0].End != "15:00" {
		t.Errorf("WQ slots = %+v, want one Thursday(4) 12:00-15:00", wq.Slots)
	}

	// An overnight-adjacent format: "10pm-12am" must parse 22:00-00:00 (the
	// end<=start wrap RadioSchedule uses), not reverse or fail.
	ue, ok := byCode["UE"]
	if !ok {
		t.Fatal("code UE (The Cool Blue Flame) not parsed")
	}
	if len(ue.Slots) == 0 || ue.Slots[0].Start != "22:00" || ue.Slots[0].End != "00:00" {
		t.Errorf("UE slots = %+v, want 22:00-00:00", ue.Slots)
	}

	// Day attribution: the fixture's second group is Friday; its first slot is
	// Give The Drummer Some, 9am-12pm.
	ds, ok := byCode["DS"]
	if !ok {
		t.Fatal("code DS not parsed")
	}
	if len(ds.Slots) == 0 || ds.Slots[0].DayOfWeek != 5 || ds.Slots[0].Start != "09:00" || ds.Slots[0].End != "12:00" {
		t.Errorf("DS slots = %+v, want Friday(5) 09:00-12:00", ds.Slots)
	}
}

// A page whose markup shifted enough that no day headers or no slot rows
// parse must come back empty (the apply's recognized-floor then disables
// everything) rather than erroring or attributing slots to a stale day.
func TestParseWFMUSubstreamSchedule_UnrecognizableMarkup(t *testing.T) {
	entries, _, err := parseWFMUSubstreamSchedule([]byte("<html><body><table><tr><td>12-3pm</td><td><a href=\"/playlists/ZZ\">Show</a></td></tr></table></body></html>"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("slot rows before any day header must be ignored, got %d entries", len(entries))
	}
}

func TestWFMULocalWeekday(t *testing.T) {
	// 2026-07-03 02:00 UTC is still Thursday 2026-07-02 22:00 in ET — the
	// provider-local weekday, not the UTC one, decides the partial day.
	utc := time.Date(2026, 7, 3, 2, 0, 0, 0, time.UTC)
	if got := wfmuLocalWeekday(utc); got != 4 {
		t.Errorf("wfmuLocalWeekday = %d, want 4 (ET Thursday)", got)
	}
}
