package catalog

import (
	"testing"
	"time"
)

func TestParseCalendarDateKey(t *testing.T) {
	phoenix, err := time.LoadLocation("America/Phoenix")
	if err != nil {
		t.Fatalf("failed to load location: %v", err)
	}

	t.Run("valid keys parse to local midnight", func(t *testing.T) {
		got, err := ParseCalendarDateKey("2026-07-31", phoenix)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.Format("2006-01-02 15:04:05") != "2026-07-31 00:00:00" {
			t.Errorf("got %s, want 2026-07-31 00:00:00", got.Format("2006-01-02 15:04:05"))
		}
		if got.Location() != phoenix {
			t.Errorf("got location %s, want America/Phoenix", got.Location())
		}
	})

	t.Run("leap day of a leap year is real", func(t *testing.T) {
		if _, err := ParseCalendarDateKey("2028-02-29", phoenix); err != nil {
			t.Errorf("2028-02-29 rejected, but 2028 is a leap year: %v", err)
		}
	})

	// Go's parser NORMALIZES an out-of-range date rather than rejecting it —
	// "2026-02-30" becomes March 2nd — so without the round-trip check every
	// impossible date would silently become a second URL for a real day's
	// content. These are the cases that check the round-trip, not the parser.
	invalid := []string{
		"2026-02-30",           // February never has 30 days
		"2026-13-01",           // month out of range
		"2026-07-32",           // day out of range
		"2027-02-29",           // 2027 is not a leap year
		"2026-7-31",            // unpadded month
		"2026-07-31T00:00:00Z", // an instant, not a calendar date
		"2026-W31",             // an ISO week key, not a date
		"tonight",
		"",
	}
	for _, key := range invalid {
		t.Run("rejects "+key, func(t *testing.T) {
			if _, err := ParseCalendarDateKey(key, phoenix); err == nil {
				t.Errorf("ParseCalendarDateKey(%q) accepted an invalid key", key)
			}
		})
	}
}

// The 6am boundary is the whole reason /tonight cannot be "today's date".
func TestTonightDate_RollsOverAtSixAM(t *testing.T) {
	phoenix, err := time.LoadLocation("America/Phoenix")
	if err != nil {
		t.Fatalf("failed to load location: %v", err)
	}

	tests := []struct {
		name string
		now  time.Time
		want string
	}{
		{
			name: "evening is its own night",
			now:  time.Date(2026, time.July, 31, 21, 0, 0, 0, phoenix),
			want: "2026-07-31",
		},
		{
			// The case the rule exists for: someone at the venue at 01:00 is
			// standing at a show the site files under the previous date.
			name: "just after midnight is still the previous night",
			now:  time.Date(2026, time.August, 1, 1, 0, 0, 0, phoenix),
			want: "2026-07-31",
		},
		{
			name: "05:59 is the last minute of the previous night",
			now:  time.Date(2026, time.August, 1, 5, 59, 59, 0, phoenix),
			want: "2026-07-31",
		},
		{
			name: "06:00 opens the new night",
			now:  time.Date(2026, time.August, 1, 6, 0, 0, 0, phoenix),
			want: "2026-08-01",
		},
		{
			// A month boundary is where an off-by-one would be least visible.
			name: "rolls back across a month boundary",
			now:  time.Date(2026, time.August, 1, 2, 30, 0, 0, phoenix),
			want: "2026-07-31",
		},
		{
			name: "rolls back across a year boundary",
			now:  time.Date(2027, time.January, 1, 3, 0, 0, 0, phoenix),
			want: "2026-12-31",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := tonightDate(tc.now)
			if got.Format("2006-01-02") != tc.want {
				t.Errorf("tonightDate(%s) = %s, want %s",
					tc.now.Format(time.RFC3339), got.Format("2006-01-02"), tc.want)
			}
			if h, m, s := got.Clock(); h != 0 || m != 0 || s != 0 {
				t.Errorf("tonightDate(%s) = %s, want local midnight",
					tc.now.Format(time.RFC3339), got.Format(time.RFC3339))
			}
		})
	}
}

// The viewer's clock must never enter this answer: the same instant asked in
// two zones has to name the same Phoenix night, which is what stops a reader in
// Berlin from being shown tomorrow's listings for a Phoenix room.
func TestTonightDate_AnswersInTheSceneZoneNotTheViewers(t *testing.T) {
	phoenix, err := time.LoadLocation("America/Phoenix")
	if err != nil {
		t.Fatalf("failed to load location: %v", err)
	}
	berlin, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Fatalf("failed to load location: %v", err)
	}

	// 22:00 Friday in Phoenix is already 07:00 SATURDAY in Berlin.
	instant := time.Date(2026, time.July, 31, 22, 0, 0, 0, phoenix)

	if got := tonightDate(instant.In(phoenix)).Format("2006-01-02"); got != "2026-07-31" {
		t.Errorf("scene-zone answer = %s, want 2026-07-31", got)
	}
	// Proof that passing the VIEWER's zone would give a different night — which
	// is why GetSceneDay converts to the scene's location before calling this.
	if got := tonightDate(instant.In(berlin)).Format("2006-01-02"); got == "2026-07-31" {
		t.Fatal("Berlin-rendered instant gave the same night; the test no longer proves the zone matters")
	}
}
