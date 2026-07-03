package catalog

import (
	"testing"
	"time"

	catalogm "psychic-homily-backend/internal/models/catalog"
)

// slotBoundaryDue is the slot-fetch ticker's pure trigger decision (PSY-1333).
// Times below are built in America/New_York; 2026-07-03 is a Friday (weekday 5).

func nySched(slots ...catalogm.RadioScheduleSlot) *catalogm.RadioSchedule {
	return &catalogm.RadioSchedule{Timezone: "America/New_York", Slots: slots}
}

func nyTime(t *testing.T, value string) time.Time {
	t.Helper()
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	ts, err := time.ParseInLocation("2006-01-02 15:04", value, loc)
	if err != nil {
		t.Fatal(err)
	}
	return ts
}

func TestSlotBoundaryDue(t *testing.T) {
	// The user-reported shape: JA In The AM, Friday 09:00–12:00 ET.
	ja := catalogm.RadioScheduleSlot{DayOfWeek: 5, Start: "09:00", End: "12:00"}

	cases := []struct {
		name  string
		sched *catalogm.RadioSchedule
		from  string
		to    string
		want  bool
	}{
		{"start crosses inside the window", nySched(ja), "2026-07-03 08:55", "2026-07-03 09:05", true},
		{"end crosses inside the window", nySched(ja), "2026-07-03 11:55", "2026-07-03 12:05", true},
		{"mid-slot window with no boundary", nySched(ja), "2026-07-03 10:00", "2026-07-03 10:10", false},
		{"window on the wrong weekday", nySched(ja), "2026-07-02 08:55", "2026-07-02 09:05", false},
		{"boundary exactly at from is excluded (half-open left)", nySched(ja), "2026-07-03 09:00", "2026-07-03 09:10", false},
		{"boundary exactly at to is included", nySched(ja), "2026-07-03 08:50", "2026-07-03 09:00", true},
		{
			// A Friday-column slot wrapping past midnight (23:00–02:00): its END
			// lands on SATURDAY, so the day-before iteration must still find it.
			"cross-midnight slot end fires the day after its column day",
			nySched(catalogm.RadioScheduleSlot{DayOfWeek: 5, Start: "23:00", End: "02:00"}),
			"2026-07-04 01:55", "2026-07-04 02:05", true,
		},
		{
			// Same wrap slot: no boundary strictly inside the wrapped span.
			"cross-midnight slot interior is quiet",
			nySched(catalogm.RadioScheduleSlot{DayOfWeek: 5, Start: "23:00", End: "02:00"}),
			"2026-07-04 00:30", "2026-07-04 00:40", false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := slotBoundaryDue(tc.sched, nyTime(t, tc.from), nyTime(t, tc.to))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("slotBoundaryDue(%s..%s) = %v, want %v", tc.from, tc.to, got, tc.want)
			}
		})
	}

	t.Run("nil schedule and empty window are never due", func(t *testing.T) {
		if got, _ := slotBoundaryDue(nil, nyTime(t, "2026-07-03 08:00"), nyTime(t, "2026-07-03 09:30")); got {
			t.Fatal("nil schedule reported due")
		}
		at := nyTime(t, "2026-07-03 09:30")
		if got, _ := slotBoundaryDue(nySched(ja), at, at); got {
			t.Fatal("empty window reported due")
		}
	})

	t.Run("invalid timezone surfaces an error", func(t *testing.T) {
		bad := &catalogm.RadioSchedule{Timezone: "Not/AZone", Slots: []catalogm.RadioScheduleSlot{ja}}
		if _, err := slotBoundaryDue(bad, nyTime(t, "2026-07-03 08:55"), nyTime(t, "2026-07-03 09:05")); err == nil {
			t.Fatal("expected timezone error")
		}
	})
}
