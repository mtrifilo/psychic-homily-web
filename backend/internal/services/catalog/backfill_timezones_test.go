package catalog

import (
	"testing"
	"time"

	// Embed the IANA tz database so LoadLocation works in any CI image,
	// independent of system zoneinfo. Test-only — not linked into the server.
	_ "time/tzdata"

	catalogm "psychic-homily-backend/internal/models/catalog"
)

func mustLoc(t *testing.T, name string) *time.Location {
	t.Helper()
	l, err := time.LoadLocation(name)
	if err != nil {
		t.Fatalf("load location %s: %v", name, err)
	}
	return l
}

func TestReanchorEventDate(t *testing.T) {
	phoenix := mustLoc(t, "America/Phoenix")     // UTC-7 year round
	berlin := mustLoc(t, "Europe/Berlin")        // UTC+2 in summer
	la := mustLoc(t, "America/Los_Angeles")      // UTC-7 summer / UTC-8 winter
	toronto := mustLoc(t, "America/Toronto")     // UTC-4 in summer
	ny := mustLoc(t, "America/New_York")         // UTC-4 in summer

	cases := []struct {
		name        string
		stored      time.Time
		geocoded    *time.Location
		assumed     *time.Location
		wantOutcome reanchorOutcome
		wantUTC     string // expected corrected instant (RFC3339, UTC) when reanchored
	}{
		{
			// The ticket's canonical round-trip: a Berlin show stored under the
			// old Phoenix assumption (20:00 MST -> 03:00Z) recovers to 18:00Z,
			// which renders as 20:00 in Berlin.
			name:        "berlin show mis-stored as phoenix re-anchors to 18:00Z",
			stored:      time.Date(2026, 7, 17, 20, 0, 0, 0, phoenix),
			geocoded:    berlin,
			assumed:     phoenix,
			wantOutcome: outcomeReanchored,
			wantUTC:     "2026-07-17T18:00:00Z",
		},
		{
			name:        "montreal show mis-stored as phoenix re-anchors to toronto 20:00",
			stored:      time.Date(2026, 7, 17, 20, 0, 0, 0, phoenix),
			geocoded:    toronto,
			assumed:     phoenix,
			wantOutcome: outcomeReanchored,
			wantUTC:     "2026-07-18T00:00:00Z",
		},
		{
			name:        "correctly-stored LA show is left unchanged",
			stored:      time.Date(2026, 7, 17, 20, 0, 0, 0, la),
			geocoded:    la,
			assumed:     la,
			wantOutcome: outcomeAlreadyCorrect,
		},
		{
			// WA venue: the CLI stored 20:00 in America/Los_Angeles (its full-US
			// map), but the backend's 8-state fallback would "assume" Phoenix.
			// The already-correct-in-geocoded check must win so we DON'T shift it.
			name:        "CLI-stored WA show is not corrupted by a wrong assumed zone",
			stored:      time.Date(2026, 7, 17, 20, 0, 0, 0, la),
			geocoded:    la,
			assumed:     phoenix,
			wantOutcome: outcomeAlreadyCorrect,
		},
		{
			// The exact case the naive "assume Phoenix" rule would corrupt:
			// a winter (PST, UTC-8) CA show stored correctly. Detection keeps it.
			name:        "winter LA show correctly stored is left unchanged",
			stored:      time.Date(2026, 1, 15, 20, 0, 0, 0, la),
			geocoded:    la,
			assumed:     la,
			wantOutcome: outcomeAlreadyCorrect,
		},
		{
			name:        "explicit 9pm show in its real zone is ambiguous, left unchanged",
			stored:      time.Date(2026, 7, 17, 21, 0, 0, 0, ny),
			geocoded:    ny,
			assumed:     ny,
			wantOutcome: outcomeAmbiguous,
		},
		{
			name:        "non-default wall time recoverable in neither zone is left unchanged",
			stored:      time.Date(2026, 7, 17, 21, 30, 0, 0, phoenix),
			geocoded:    berlin,
			assumed:     phoenix,
			wantOutcome: outcomeAmbiguous,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, outcome := reanchorEventDate(c.stored, c.geocoded, c.assumed)
			if outcome != c.wantOutcome {
				t.Fatalf("outcome = %v, want %v (got=%s)", outcome, c.wantOutcome, got.UTC().Format(time.RFC3339))
			}
			if c.wantOutcome == outcomeReanchored {
				if g := got.UTC().Format(time.RFC3339); g != c.wantUTC {
					t.Errorf("corrected instant = %s, want %s", g, c.wantUTC)
				}
			} else if !got.Equal(c.stored) {
				t.Errorf("non-reanchored case must return the stored instant; got %s want %s",
					got.UTC().Format(time.RFC3339), c.stored.UTC().Format(time.RFC3339))
			}
		})
	}
}

// Re-anchoring must be idempotent: feeding a freshly corrected instant back
// through reanchorEventDate (with the same geocoded zone) yields no change.
func TestReanchorEventDateIsIdempotent(t *testing.T) {
	phoenix := mustLoc(t, "America/Phoenix")
	berlin := mustLoc(t, "Europe/Berlin")

	stored := time.Date(2026, 7, 17, 20, 0, 0, 0, phoenix)
	corrected, outcome := reanchorEventDate(stored, berlin, phoenix)
	if outcome != outcomeReanchored {
		t.Fatalf("expected first pass to re-anchor, got outcome %v", outcome)
	}
	// Second pass: geocoded zone is now authoritative; assumed no longer matters.
	if _, outcome := reanchorEventDate(corrected, berlin, phoenix); outcome != outcomeAlreadyCorrect {
		t.Errorf("second pass should be already-correct (idempotent), got outcome %v", outcome)
	}
}

func TestIsDefaultEveningWall(t *testing.T) {
	phoenix := mustLoc(t, "America/Phoenix")
	tests := []struct {
		name string
		t    time.Time
		want bool
	}{
		{"exactly 20:00", time.Date(2026, 7, 17, 20, 0, 0, 0, phoenix), true},
		{"20:00:30 is not default", time.Date(2026, 7, 17, 20, 0, 30, 0, phoenix), false},
		{"20:01 is not default", time.Date(2026, 7, 17, 20, 1, 0, 0, phoenix), false},
		{"19:00 is not default", time.Date(2026, 7, 17, 19, 0, 0, 0, phoenix), false},
		{"sub-second is not default", time.Date(2026, 7, 17, 20, 0, 0, 1, phoenix), false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := isDefaultEveningWall(tc.t); got != tc.want {
				t.Errorf("isDefaultEveningWall(%s) = %v, want %v", tc.t, got, tc.want)
			}
		})
	}
}

func TestPrimaryVenue(t *testing.T) {
	if _, ok := primaryVenue(nil); ok {
		t.Errorf("empty venues should return ok=false")
	}
	got, ok := primaryVenue([]catalogm.Venue{{ID: 7}, {ID: 3}, {ID: 9}})
	if !ok || got.ID != 3 {
		t.Errorf("primaryVenue id = %v (ok=%v), want id=3, ok=true", got, ok)
	}
}

func TestRoundCoord(t *testing.T) {
	if got := roundCoord(52.5200066); got != 52.520007 {
		t.Errorf("roundCoord = %v, want 52.520007", got)
	}
}

func TestFloatPtrEq(t *testing.T) {
	a, b := 1.5, 1.5
	c := 2.0
	if !floatPtrEq(&a, &b) {
		t.Errorf("equal values should be equal")
	}
	if floatPtrEq(&a, &c) {
		t.Errorf("different values should not be equal")
	}
	if !floatPtrEq(nil, nil) {
		t.Errorf("nil,nil should be equal")
	}
	if floatPtrEq(&a, nil) {
		t.Errorf("value,nil should not be equal")
	}
}
