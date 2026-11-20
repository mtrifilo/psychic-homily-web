package catalog

import (
	"errors"
	"strings"
	"testing"
	"time"

	apperrors "psychic-homily-backend/internal/errors"
)

func TestShowEventDateBound_IsEighteenCalendarMonthsInUTC(t *testing.T) {
	now := time.Date(2026, 11, 20, 23, 30, 0, 0, time.FixedZone("UTC-8", -8*3600))
	want := time.Date(2028, 5, 21, 7, 30, 0, 0, time.UTC)
	if got := showEventDateBound(now); !got.Equal(want) {
		t.Fatalf("showEventDateBound(%s) = %s, want %s", now, got, want)
	}
}

func TestCheckShowEventDateBound_BothSidesAndOverride(t *testing.T) {
	now := time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)
	bound := showEventDateBound(now)

	cases := []struct {
		name     string
		date     time.Time
		override bool
		refused  bool
	}{
		{"a day inside the bound", bound.Add(-24 * time.Hour), false, false},
		{"exactly the bound", bound, false, false},
		{"one second past the bound", bound.Add(time.Second), false, true},
		{"years past the bound", time.Date(2083, 8, 15, 23, 0, 0, 0, time.UTC), false, true},
		{"past the bound with the override", bound.Add(time.Second), true, false},
		{"years past the bound with the override", time.Date(2083, 8, 15, 23, 0, 0, 0, time.UTC), true, false},
		{"in the past", now.AddDate(-1, 0, 0), false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := checkShowEventDateBound(tc.date, now, tc.override)
			if !tc.refused {
				if err != nil {
					t.Fatalf("want accepted, got %v", err)
				}
				return
			}
			var showErr *apperrors.ShowError
			if !errors.As(err, &showErr) || showErr.Code != apperrors.CodeShowEventDateBeyondBound {
				t.Fatalf("want %s, got %v", apperrors.CodeShowEventDateBeyondBound, err)
			}
			for _, want := range []string{"18 months", "allow_far_future_date", "--allow-far-future", bound.Format(time.RFC3339)} {
				if !strings.Contains(showErr.Message, want) {
					t.Errorf("message %q does not name %q", showErr.Message, want)
				}
			}
		})
	}
}
