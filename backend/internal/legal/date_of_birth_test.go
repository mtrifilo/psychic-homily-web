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

func TestParseDateOfBirth(t *testing.T) {
	now := time.Date(2026, time.October, 23, 15, 0, 0, 0, time.UTC)

	for _, tc := range []struct {
		value string
		ok    bool
	}{
		{"1998-04-27", true},
		{"2000-02-29", true},  // a real leap day
		{"2026-10-23", true},  // born today: a date, if not an eligible one
		{"2026-10-24", false}, // tomorrow
		{"1900-02-29", false}, // 1900 is not a leap year
		{"1998-02-30", false},
		{"1998-13-01", false},
		{"1998-4-27", false}, // not zero padded
		{"04/27/1998", false},
		{"1998-04-27T00:00:00Z", false},
		{"", false},
		{"1875-10-24", true},  // MaxPlausibleAge, the next birthday tomorrow
		{"1875-10-23", false}, // turns MaxPlausibleAge + 1 today
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

func TestAgeOnTurnsOnTheBirthday(t *testing.T) {
	dob := mustDate(t, "2010-10-23")
	for _, tc := range []struct {
		now  time.Time
		want int
	}{
		{time.Date(2026, time.October, 22, 23, 59, 59, 0, time.UTC), 15},
		{time.Date(2026, time.October, 23, 0, 0, 0, 0, time.UTC), 16},
		{time.Date(2026, time.October, 23, 23, 59, 59, 0, time.UTC), 16},
		{time.Date(2027, time.January, 1, 0, 0, 0, 0, time.UTC), 16},
	} {
		if got := AgeOn(dob, tc.now); got != tc.want {
			t.Errorf("AgeOn(2010-10-23, %v) = %d, want %d", tc.now, got, tc.want)
		}
	}
}

// The age is computed on the UTC calendar date, whatever zone the submission
// instant is expressed in: 2026-10-22 20:30 in New York is already October 23
// in UTC, so the sixteenth birthday has arrived.
func TestAgeOnUsesTheUTCDate(t *testing.T) {
	dob := mustDate(t, "2010-10-23")
	newYork, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Skipf("no zone database: %v", err)
	}
	if got := AgeOn(dob, time.Date(2026, time.October, 22, 20, 30, 0, 0, newYork)); got != 16 {
		t.Errorf("AgeOn at 00:30 UTC on the birthday = %d, want 16", got)
	}
	if got := AgeOn(dob, time.Date(2026, time.October, 22, 19, 59, 0, 0, newYork)); got != 15 {
		t.Errorf("AgeOn at 23:59 UTC the day before = %d, want 15", got)
	}
}

func TestAgeOnLeapDayBirthday(t *testing.T) {
	dob := mustDate(t, "2008-02-29")
	for _, tc := range []struct {
		now  time.Time
		want int
	}{
		{time.Date(2024, time.February, 28, 12, 0, 0, 0, time.UTC), 15},
		{time.Date(2024, time.February, 29, 0, 0, 0, 0, time.UTC), 16}, // leap year: the day itself
		{time.Date(2025, time.February, 28, 12, 0, 0, 0, time.UTC), 16},
		{time.Date(2023, time.February, 28, 12, 0, 0, 0, time.UTC), 14}, // non-leap: not yet
		{time.Date(2023, time.March, 1, 0, 0, 0, 0, time.UTC), 15},      // non-leap: March 1
	} {
		if got := AgeOn(dob, tc.now); got != tc.want {
			t.Errorf("AgeOn(2008-02-29, %v) = %d, want %d", tc.now, got, tc.want)
		}
	}
}

func TestMeetsMinimumAgeOnTheSixteenthBirthday(t *testing.T) {
	now := time.Date(2026, time.October, 23, 0, 0, 0, 0, time.UTC)
	if !MeetsMinimumAge(mustDate(t, "2010-10-23"), now) {
		t.Error("16 exactly on the birthday must meet the minimum")
	}
	if MeetsMinimumAge(mustDate(t, "2010-10-24"), now) {
		t.Error("one day short of 16 must not meet the minimum")
	}
}
