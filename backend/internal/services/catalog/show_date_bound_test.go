package catalog

import (
	"errors"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	apperrors "psychic-homily-backend/internal/errors"
)

// The bound is stated in three places; this fails when one of them moves alone.
func TestShowEventDateBound_CopiesAgree(t *testing.T) {
	want := strconv.Itoa(ShowEventDateBoundMonths)

	cli, err := os.ReadFile("../../../../cli/src/lib/dateBound.ts")
	if err != nil {
		t.Fatalf("read CLI copy: %v", err)
	}
	m := regexp.MustCompile(`SHOW_EVENT_DATE_BOUND_MONTHS = (\d+);`).FindSubmatch(cli)
	if m == nil || string(m[1]) != want {
		t.Errorf("cli/src/lib/dateBound.ts states %q, want %s", m, want)
	}

	handler, err := os.ReadFile("../../api/handlers/catalog/show.go")
	if err != nil {
		t.Fatalf("read handler: %v", err)
	}
	tag := regexp.MustCompile(`json:"allow_far_future_date,omitempty".*doc:"[^"]*more than (\d+) months ahead`).FindSubmatch(handler)
	if tag == nil || string(tag[1]) != want {
		t.Errorf("allow_far_future_date doc tag states %q, want %s", tag, want)
	}
}

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
			err := CheckShowEventDateBound(tc.date, now, tc.override)
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
			// The owner's sentence, with the months taken from the bound itself
			// and no dates in it.
			want := "Shows can be listed up to " + strconv.Itoa(ShowEventDateBoundMonths) +
				" months ahead. If this date is right, contact us and we'll add it."
			if showErr.Message != want {
				t.Errorf("message %q, want %q", showErr.Message, want)
			}
			if strings.Contains(showErr.Message, "%!") {
				t.Errorf("message carries a fmt error: %q", showErr.Message)
			}
		})
	}
}
