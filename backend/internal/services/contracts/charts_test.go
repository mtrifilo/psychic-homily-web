package contracts

import (
	"testing"
	"time"
)

func TestParseChartWindowCalendarValidation(t *testing.T) {
	launch := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	for _, value := range []string{"2025", "2025-q4", "0000", "2026-Q1", "2026-q0", "2026-q5", "2026-q01", " 2026"} {
		if _, err := ParseChartWindow(value, launch); err == nil {
			t.Errorf("ParseChartWindow(%q) expected error", value)
		}
	}
	for _, value := range []string{"2026", "2026-q1"} {
		got, err := ParseChartWindow(value, launch)
		if err != nil || got != ChartWindow(value) {
			t.Errorf("ParseChartWindow(%q) = %q, %v", value, got, err)
		}
	}
	if _, err := ParseChartWindow("2026-q2", launch); err == nil {
		t.Error("quarter beginning after now must be rejected as future")
	}
}

func TestCalendarBoundsUTC(t *testing.T) {
	tests := []struct {
		window    ChartWindow
		wantStart time.Time
		wantEnd   time.Time
	}{
		{ChartWindow("2026"), time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)},
		{ChartWindow("2026-q4"), time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC), time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)},
	}
	for _, tt := range tests {
		start, end, ok := tt.window.CalendarBounds()
		if !ok || !start.Equal(tt.wantStart) || !end.Equal(tt.wantEnd) || start.Location() != time.UTC || end.Location() != time.UTC {
			t.Errorf("CalendarBounds(%q) = %v, %v, %v", tt.window, start, end, ok)
		}
	}
}
