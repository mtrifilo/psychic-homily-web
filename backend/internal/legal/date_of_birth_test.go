package legal

import (
	"errors"
	"testing"
	"time"
)

func mustDate(t *testing.T, value string) time.Time {
	t.Helper()
	d, err := time.Parse(DateOfBirthLayout, value)
	if err != nil {
		t.Fatalf("bad fixture %q: %v", value, err)
	}
	return d
}

func at(t *testing.T, value string) time.Time {
	t.Helper()
	instant, err := time.Parse(time.RFC3339, value)
	if err != nil {
		t.Fatalf("bad instant %q: %v", value, err)
	}
	return instant
}

func TestParseDateOfBirth(t *testing.T) {
	// 15:00 UTC on October 23 is already October 24 at UTC+14.
	now := at(t, "2026-10-23T15:00:00Z")

	for _, tc := range []struct {
		value string
		ok    bool
	}{
		{"1998-04-27", true},
		{"2000-02-29", true},  // a real leap day
		{"2026-10-23", true},  // today in UTC
		{"2026-10-24", true},  // today at UTC+14
		{"2026-10-25", false}, // tomorrow everywhere
		{"1900-02-29", false}, // 1900 is not a leap year
		{"1998-02-30", false},
		{"1998-13-01", false},
		{"1998-4-27", false}, // not zero padded
		{"04/27/1998", false},
		{"1998-04-27T00:00:00Z", false},
		{"", false},
		{"1875-10-25", true},  // MaxPlausibleAge, the next birthday tomorrow at UTC+14
		{"1875-10-24", false}, // turns MaxPlausibleAge + 1 today at UTC+14
	} {
		got, err := ParseDateOfBirth(tc.value, now)
		if tc.ok {
			if err != nil {
				t.Errorf("ParseDateOfBirth(%q) refused: %v", tc.value, err)
				continue
			}
			if got.Location() != time.UTC || got.Hour() != 0 || got.Format(DateOfBirthLayout) != tc.value {
				t.Errorf("ParseDateOfBirth(%q) = %v, want that date at midnight UTC", tc.value, got)
			}
			continue
		}
		if !errors.Is(err, ErrDateOfBirthInvalid) {
			t.Errorf("ParseDateOfBirth(%q) = %v, %v; want ErrDateOfBirthInvalid", tc.value, got, err)
		}
	}
}

// Someone counts as a year older from the first moment it is their birthday
// anywhere on Earth: the 24th starts at UTC+14 at 10:00 UTC on the 23rd.
func TestAgeOnCountsTheBirthdayFromTheFirstPlaceItArrives(t *testing.T) {
	dob := mustDate(t, "2010-10-24")
	for _, tc := range []struct {
		now  string
		want int
	}{
		{"2026-10-23T09:00:00Z", 15},
		{"2026-10-23T09:59:59Z", 15},
		{"2026-10-23T10:00:00Z", 16},
		{"2026-10-23T11:00:00Z", 16},
		{"2026-10-24T00:30:00Z", 16},
		{"2026-10-24T23:59:59Z", 16},
		{"2027-01-01T00:00:00Z", 16},
	} {
		if got := AgeOn(dob, at(t, tc.now)); got != tc.want {
			t.Errorf("AgeOn(2010-10-24, %s) = %d, want %d", tc.now, got, tc.want)
		}
	}
}

// The zone the submission instant is expressed in does not matter, only the
// instant.
func TestAgeOnIgnoresTheZoneOfNow(t *testing.T) {
	dob := mustDate(t, "2010-10-24")
	newYork, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Skipf("no zone database: %v", err)
	}
	// 05:30 in New York on the 23rd is 09:30 UTC, still the 23rd at UTC+14.
	if got := AgeOn(dob, time.Date(2026, time.October, 23, 5, 30, 0, 0, newYork)); got != 15 {
		t.Errorf("AgeOn at 09:30 UTC the day before = %d, want 15", got)
	}
	// 06:30 in New York on the 23rd is 10:30 UTC, the 24th at UTC+14.
	if got := AgeOn(dob, time.Date(2026, time.October, 23, 6, 30, 0, 0, newYork)); got != 16 {
		t.Errorf("AgeOn at 10:30 UTC the day before = %d, want 16", got)
	}
}

func TestAgeOnLeapDayBirthday(t *testing.T) {
	dob := mustDate(t, "2008-02-29")
	for _, tc := range []struct {
		now  string
		want int
	}{
		// 2024 is a leap year: February 29 starts at UTC+14 at 10:00 UTC on the 28th.
		{"2024-02-28T09:00:00Z", 15},
		{"2024-02-28T10:00:00Z", 16},
		{"2025-02-28T09:00:00Z", 16},
		{"2025-02-28T10:00:00Z", 17},
		// 2023 is not: the year turns on March 1, which starts at UTC+14 at
		// 10:00 UTC on February 28.
		{"2023-02-28T09:00:00Z", 14},
		{"2023-02-28T10:00:00Z", 15},
	} {
		if got := AgeOn(dob, at(t, tc.now)); got != tc.want {
			t.Errorf("AgeOn(2008-02-29, %s) = %d, want %d", tc.now, got, tc.want)
		}
	}
}

func TestAgeOnReachesTheMinimumOnTheSixteenthBirthday(t *testing.T) {
	now := at(t, "2026-10-24T00:30:00Z")
	if AgeOn(mustDate(t, "2010-10-24"), now) < MinSignupAge {
		t.Error("16 on the birthday must meet the minimum")
	}
	if AgeOn(mustDate(t, "2010-10-25"), now) >= MinSignupAge {
		t.Error("a birthday that has not arrived anywhere must not meet the minimum")
	}
}
