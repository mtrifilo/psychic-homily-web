package catalog

import (
	"encoding/json"
	"testing"
	"time"
)

// decodeEventDate puts a raw JSON value through the same decoding a request body
// takes, because the whole discriminator rests on a property of the DECODED
// value: encoding/json preserves the offset the client wrote, so the submitted
// representation survives into the handler even though it cannot survive into
// the TIMESTAMPTZ column. Constructing time.Time values directly in these tests
// would assume that rather than demonstrate it.
func decodeEventDate(t *testing.T, raw string) time.Time {
	t.Helper()
	var ts time.Time
	if err := json.Unmarshal([]byte(`"`+raw+`"`), &ts); err != nil {
		t.Fatalf("decoding %q: %v", raw, err)
	}
	return ts
}

// The premise the whole write-boundary fix depends on. If this ever fails,
// isDateOnlySubmission is blind and every date-only submission silently becomes
// a UTC-midnight row again.
func TestDecodedEventDateRetainsSubmittedOffset(t *testing.T) {
	bare := decodeEventDate(t, "2026-08-15T00:00:00Z")
	if _, off := bare.Zone(); off != 0 {
		t.Errorf("bare midnight offset = %d, want 0", off)
	}
	if bare.Hour() != 0 {
		t.Errorf("bare midnight hour = %d, want 0", bare.Hour())
	}

	// The SAME INSTANT, written as a real Eastern evening.
	evening := decodeEventDate(t, "2026-08-14T20:00:00-04:00")
	if !evening.Equal(bare) {
		t.Fatalf("expected the same instant, got %s and %s", evening, bare)
	}
	if _, off := evening.Zone(); off != -4*60*60 {
		t.Errorf("evening offset = %d, want -14400", off)
	}
	if evening.Hour() != 20 {
		t.Errorf("evening hour = %d, want 20", evening.Hour())
	}
}

func TestIsDateOnlySubmission(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want bool
	}{
		// A bare calendar date, however the client spells its zero offset.
		{"midnight with Z", "2026-08-15T00:00:00Z", true},
		{"midnight with +00:00", "2026-08-15T00:00:00+00:00", true},
		{"midnight with -00:00", "2026-08-15T00:00:00-00:00", true},

		// Genuine times that must never be touched. The first is the case that
		// makes an instant-based test unusable: it is exactly UTC midnight.
		{"8pm Eastern summer, same instant as UTC midnight", "2026-08-14T20:00:00-04:00", false},
		{"7pm Central summer, same instant as UTC midnight", "2026-08-14T19:00:00-05:00", false},
		{"explicit venue-local midnight", "2026-08-15T00:00:00-07:00", false},
		{"ordinary evening in UTC", "2026-08-15T20:00:00Z", false},
		{"one second past midnight UTC", "2026-08-15T00:00:01Z", false},

		// Positive-offset clients. A Berlin or Kolkata caller stating local
		// midnight is making a real claim about a late set and is left alone;
		// only a zero offset reads as "no time was known".
		{"Berlin local midnight", "2026-08-15T00:00:00+02:00", false},
		{"Kolkata local midnight", "2026-08-15T00:00:00+05:30", false},
		{"Berlin evening", "2026-08-15T20:00:00+02:00", false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := isDateOnlySubmission(decodeEventDate(t, c.raw)); got != c.want {
				t.Errorf("isDateOnlySubmission(%s) = %v, want %v", c.raw, got, c.want)
			}
		})
	}
}

// Sub-second precision must not let a bare date slip through as "not midnight".
func TestIsDateOnlySubmission_RejectsSubSecondOffsets(t *testing.T) {
	if isDateOnlySubmission(decodeEventDate(t, "2026-08-15T00:00:00.001Z")) {
		t.Error("a value one millisecond past midnight is a stated time, not a bare date")
	}
}

func TestAnchorDateOnlyEventDate_NegativeOffsetZone(t *testing.T) {
	phoenix, err := time.LoadLocation("America/Phoenix")
	if err != nil {
		t.Fatalf("America/Phoenix unavailable: %v", err)
	}

	got, reanchored := anchorDateOnlyEventDate(decodeEventDate(t, "2026-08-15T00:00:00Z"), "AZ")
	if !reanchored {
		t.Fatal("expected a bare date to be re-anchored")
	}
	// The stored instant read back on the venue's own clock is the evening of
	// the date that was submitted — not the previous afternoon, which is what
	// 2026-08-15T00:00:00Z renders as in Phoenix.
	if got.In(phoenix).Format("2006-01-02 15:04") != "2026-08-15 20:00" {
		t.Errorf("Phoenix-local = %s, want 2026-08-15 20:00", got.In(phoenix).Format("2006-01-02 15:04"))
	}
	if got.UTC() != time.Date(2026, 8, 16, 3, 0, 0, 0, time.UTC) {
		t.Errorf("stored instant = %s, want 2026-08-16T03:00:00Z", got.UTC())
	}
}

// Chicago and New York are the zones where the old behaviour was worst and where
// a careless fix would be worst: both anchor onto an instant on the FOLLOWING
// UTC day, and New York's summer anchor is itself exactly UTC midnight.
func TestAnchorDateOnlyEventDate_DSTZones(t *testing.T) {
	cases := []struct {
		state      string
		zone       string
		submitted  string
		wantLocal  string
		wantStored time.Time
	}{
		{"NY", "America/New_York", "2026-08-15T00:00:00Z", "2026-08-15 20:00", time.Date(2026, 8, 16, 0, 0, 0, 0, time.UTC)},
		{"NY", "America/New_York", "2026-01-15T00:00:00Z", "2026-01-15 20:00", time.Date(2026, 1, 16, 1, 0, 0, 0, time.UTC)},
		{"IL", "America/Chicago", "2026-08-15T00:00:00Z", "2026-08-15 20:00", time.Date(2026, 8, 16, 1, 0, 0, 0, time.UTC)},
		{"IL", "America/Chicago", "2026-01-15T00:00:00Z", "2026-01-15 20:00", time.Date(2026, 1, 16, 2, 0, 0, 0, time.UTC)},
	}

	for _, c := range cases {
		t.Run(c.state+" "+c.submitted, func(t *testing.T) {
			loc, err := time.LoadLocation(c.zone)
			if err != nil {
				t.Skipf("%s unavailable: %v", c.zone, err)
			}
			got, reanchored := anchorDateOnlyEventDate(decodeEventDate(t, c.submitted), c.state)
			if !reanchored {
				t.Fatal("expected re-anchoring")
			}
			if local := got.In(loc).Format("2006-01-02 15:04"); local != c.wantLocal {
				t.Errorf("local = %s, want %s", local, c.wantLocal)
			}
			if got.UTC() != c.wantStored {
				t.Errorf("stored = %s, want %s", got.UTC(), c.wantStored)
			}
		})
	}
}

// The days either side of a spring-forward and a fall-back. 20:00 exists on all
// of them, so the anchor must simply land on the right evening; a transition
// must not shift it onto an adjacent date.
func TestAnchorDateOnlyEventDate_AcrossDSTTransitions(t *testing.T) {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Skipf("America/New_York unavailable: %v", err)
	}
	// US transitions in 2026: spring forward Mar 8, fall back Nov 1.
	for _, date := range []string{"2026-03-07", "2026-03-08", "2026-03-09", "2026-10-31", "2026-11-01", "2026-11-02"} {
		got, reanchored := anchorDateOnlyEventDate(decodeEventDate(t, date+"T00:00:00Z"), "NY")
		if !reanchored {
			t.Fatalf("%s: expected re-anchoring", date)
		}
		if local := got.In(loc).Format("2006-01-02 15:04"); local != date+" 20:00" {
			t.Errorf("%s: local = %s, want %s 20:00", date, local, date)
		}
	}
}

// Every submission that states a time is returned byte-identical, including the
// one whose instant collides with a bare date.
func TestAnchorDateOnlyEventDate_LeavesStatedTimesAlone(t *testing.T) {
	for _, raw := range []string{
		"2026-08-14T20:00:00-04:00",
		"2026-08-15T00:00:00-07:00",
		"2026-08-15T20:00:00Z",
		"2026-08-15T00:00:00+02:00",
	} {
		submitted := decodeEventDate(t, raw)
		got, reanchored := anchorDateOnlyEventDate(submitted, "AZ")
		if reanchored {
			t.Errorf("%s was re-anchored but states its own time", raw)
		}
		if !got.Equal(submitted) {
			t.Errorf("%s: got %s, want %s", raw, got, submitted)
		}
	}
}

func TestShowEventDateState(t *testing.T) {
	cases := []struct {
		name        string
		venueStates []string
		fallbacks   []string
		want        string
	}{
		{"venue wins over body", []string{"IL"}, []string{"AZ"}, "IL"},
		{"first non-empty venue wins", []string{"", "NY"}, []string{"AZ"}, "NY"},
		{"falls back to body state", []string{""}, []string{"AZ"}, "AZ"},
		{"falls back past an empty body state", nil, []string{"", "TX"}, "TX"},
		{"no venues at all", nil, []string{"AZ"}, "AZ"},
		{"nothing known resolves to empty, not an error", nil, []string{""}, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := showEventDateState(c.venueStates, c.fallbacks...); got != c.want {
				t.Errorf("got %q, want %q", got, c.want)
			}
		})
	}
}

// An unresolvable state must still anchor rather than skip, because "no zone
// known" was exactly the path that used to leave the corrupt midnight value in
// place. EventLocation's documented default carries it.
func TestAnchorDateOnlyEventDate_UnknownStateStillAnchors(t *testing.T) {
	got, reanchored := anchorDateOnlyEventDate(decodeEventDate(t, "2026-08-15T00:00:00Z"), "")
	if !reanchored {
		t.Fatal("expected re-anchoring even with no state")
	}
	if got.UTC() != time.Date(2026, 8, 16, 3, 0, 0, 0, time.UTC) {
		t.Errorf("stored = %s, want the America/Phoenix default 2026-08-16T03:00:00Z", got.UTC())
	}
}
