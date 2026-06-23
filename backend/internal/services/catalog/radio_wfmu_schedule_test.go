package catalog

import (
	"os"
	"testing"

	catalogm "psychic-homily-backend/internal/models/catalog"
)

func loadScheduleFixture(t *testing.T) []WFMUScheduleEntry {
	t.Helper()
	body, err := os.ReadFile("testdata/wfmu_table.html")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	entries, _, err := parseWFMUScheduleTable(body)
	if err != nil {
		t.Fatalf("parseWFMUScheduleTable: %v", err)
	}
	return entries
}

// The Summer-2026 fixture's one stacked two-show cell (Monday 3-6pm: "Jim Price (3-3:01)"
// / "Scott Williams (3:01-6)" — two show links, inline meridiem-less times, no program_time
// span) is now parsed (PSY-1186): both shows resolve, their bare times anchored PM via the
// "3pm" band, so NO cell is skipped.
func TestParseWFMUScheduleTable_ParsesStackedCell(t *testing.T) {
	body, err := os.ReadFile("testdata/wfmu_table.html")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	entries, skipped, err := parseWFMUScheduleTable(body)
	if err != nil {
		t.Fatalf("parseWFMUScheduleTable: %v", err)
	}
	if skipped != 0 {
		t.Errorf("expected 0 skipped cells (the stacked cell is now parsed), got %d", skipped)
	}
	jp := findEntryByName(entries, "Jim Price")
	if jp == nil {
		t.Fatal("Jim Price (stacked) not found")
	}
	if jp.Code != "JP" || !hasSlot(jp, 1, "15:00", "15:01") {
		t.Errorf("Jim Price: want code JP + Mon 15:00-15:01; got code=%q slots=%+v", jp.Code, jp.Slots)
	}
	sw := findEntryByName(entries, "Scott Williams")
	if sw == nil {
		t.Fatal("Scott Williams (stacked) not found")
	}
	if sw.Code != "SW" || !hasSlot(sw, 1, "15:01", "18:00") {
		t.Errorf("Scott Williams: want code SW + Mon 15:01-18:00; got code=%q slots=%+v", sw.Code, sw.Slots)
	}
}

func findEntryByName(entries []WFMUScheduleEntry, name string) *WFMUScheduleEntry {
	for i := range entries {
		if entries[i].Name == name {
			return &entries[i]
		}
	}
	return nil
}

func hasSlot(e *WFMUScheduleEntry, day int, start, end string) bool {
	for _, s := range e.Slots {
		if s.DayOfWeek == day && s.Start == start && s.End == end {
			return true
		}
	}
	return false
}

// The parser reconstructs the grid against the real Summer 2026 fixture: known shows must
// resolve to the right (day, start, end). Days: 0=Sun..6=Sat.
func TestParseWFMUScheduleTable_KnownShows(t *testing.T) {
	entries := loadScheduleFixture(t)
	if len(entries) < 30 {
		t.Fatalf("expected a populated grid, got %d entries", len(entries))
	}

	// Wake (code WA): the weekday morning show, Mon–Fri 6–9am.
	wake := findEntryByName(entries, "Wake")
	if wake == nil {
		t.Fatal("Wake not found")
	}
	if wake.Code != "WA" {
		t.Errorf("Wake code = %q, want WA", wake.Code)
	}
	for _, day := range []int{1, 2, 3, 4, 5} { // Mon..Fri
		if !hasSlot(wake, day, "06:00", "09:00") {
			t.Errorf("Wake missing slot day=%d 06:00-09:00; slots=%+v", day, wake.Slots)
		}
	}

	// Garbage Time: Tuesday 9am–Noon (the ticket's AC example).
	gt := findEntryByName(entries, "Garbage Time")
	if gt == nil {
		t.Fatal("Garbage Time not found")
	}
	if !hasSlot(gt, 2, "09:00", "12:00") {
		t.Errorf("Garbage Time missing Tue 09:00-12:00; slots=%+v", gt.Slots)
	}

	// Six Degrees: Sunday 10pm–Mid → overnight-adjacent (end 00:00 < start 22:00).
	six := findEntryByName(entries, "Six Degrees")
	if six == nil {
		t.Fatal("Six Degrees not found")
	}
	if !hasSlot(six, 0, "22:00", "00:00") {
		t.Errorf("Six Degrees missing Sun 22:00-00:00; slots=%+v", six.Slots)
	}

	// Travel Zone: Monday Mid–3am (00:00–03:00).
	tz := findEntryByName(entries, "Travel Zone")
	if tz == nil {
		t.Fatal("Travel Zone not found")
	}
	if !hasSlot(tz, 1, "00:00", "03:00") {
		t.Errorf("Travel Zone missing Mon 00:00-03:00; slots=%+v", tz.Slots)
	}

	// The Glen Jones Radio Programme: Sunday Noon–3pm. Its show-title-link is an ABSOLUTE
	// archives URL (.../Playlists/GJ/archives.html), so the code must come from the
	// KDBprogram-GJ id, not the href — regression guard for that fix.
	gj := findEntryByName(entries, "The Glen Jones Radio Programme")
	if gj == nil {
		t.Fatal("The Glen Jones Radio Programme not found (absolute-archives-URL code extraction)")
	}
	if gj.Code != "GJ" {
		t.Errorf("Glen Jones code = %q, want GJ", gj.Code)
	}
	if !hasSlot(gj, 0, "12:00", "15:00") {
		t.Errorf("Glen Jones missing Sun 12:00-15:00; slots=%+v", gj.Slots)
	}
}

// Every parsed slot must be a valid RadioSchedule slot (day 0–6, HH:MM), so the assembled
// schedule passes RadioSchedule.Validate downstream.
func TestParseWFMUScheduleTable_AllSlotsValid(t *testing.T) {
	entries := loadScheduleFixture(t)
	for _, e := range entries {
		if e.Code == "" || len(e.Slots) == 0 {
			t.Errorf("entry %q has empty code or no slots", e.Name)
		}
		sched := catalogm.RadioSchedule{Timezone: wfmuScheduleTimezone, Slots: e.Slots}
		if err := sched.Validate(); err != nil {
			t.Errorf("entry %q (%s) produced an invalid schedule: %v", e.Name, e.Code, err)
		}
	}
}

func TestParseWFMUTimeRange(t *testing.T) {
	tests := []struct {
		in         string
		start, end string
		ok         bool
	}{
		{"6-9am", "06:00", "09:00", true},
		{"9am-Noon", "09:00", "12:00", true},
		{"Noon-3pm", "12:00", "15:00", true},
		{"3-6pm", "15:00", "18:00", true},
		{"3-6am", "03:00", "06:00", true},
		{"8-10pm", "20:00", "22:00", true},
		{"11am-1pm", "11:00", "13:00", true},
		{"9pm-Mid", "21:00", "00:00", true},
		{"10pm-Mid", "22:00", "00:00", true},
		{"Mid-3am", "00:00", "03:00", true},
		// Bare-left meridiem inheritance must not reverse an AM→PM crossing range.
		{"9-3pm", "09:00", "15:00", true},    // 9am-3pm, NOT 21:00-15:00
		{"10-Noon", "10:00", "12:00", true},  // 10am-Noon, NOT 22:00-12:00
		{"11-1pm", "11:00", "13:00", true},   // 11am-1pm
		{"1-3pm", "13:00", "15:00", true},    // both forward → the later (PM) reading
		{"11pm-1am", "23:00", "01:00", true}, // true cross-midnight wrap (explicit meridiems)
		{"garbage", "", "", false},
		{"", "", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			start, end, ok := parseWFMUTimeRange(tt.in)
			if ok != tt.ok || start != tt.start || end != tt.end {
				t.Errorf("parseWFMUTimeRange(%q) = (%q,%q,%v), want (%q,%q,%v)",
					tt.in, start, end, ok, tt.start, tt.end, tt.ok)
			}
		})
	}
}
