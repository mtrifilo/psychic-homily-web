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

// Regression for the adversarial-review CRITICAL finding: a correctly-stored
// explicit-time show must NOT be re-anchored. The trap is that an 11pm Eastern
// show (= 03:00Z in summer) reads as exactly 20:00 in America/Phoenix, so if the
// assumed zone is wrongly Phoenix (the old 8-state map default for unmapped
// states) it would be falsely "recovered" and corrupted 3 hours earlier. With
// the assumed zone correctly resolved from the full StateTimezones map
// (FL -> America/New_York == geocoded), sameZone() short-circuits and the show
// is left untouched.
func TestReanchorEventDate_ExplicitTimeNotCorrupted(t *testing.T) {
	ny := mustLoc(t, "America/New_York")
	phoenix := mustLoc(t, "America/Phoenix")

	// A real 11pm Eastern show in summer: 23:00 EDT (UTC-4) = 03:00Z next day,
	// which is also exactly 20:00 in America/Phoenix (UTC-7).
	stored := time.Date(2026, 7, 17, 23, 0, 0, 0, ny)
	if got := stored.UTC().Format(time.RFC3339); got != "2026-07-18T03:00:00Z" {
		t.Fatalf("test setup wrong: stored=%s, want 2026-07-18T03:00:00Z", got)
	}

	// Correct assumed zone (full map: FL/GA/... -> America/New_York == geocoded).
	if _, outcome := reanchorEventDate(stored, ny, ny); outcome != outcomeAmbiguous {
		t.Errorf("explicit 11pm show with correct assumed zone: outcome=%v, want ambiguous (left untouched)", outcome)
	}

	// The bug the full map prevents: a Phoenix assumed zone (old short-map
	// default) WOULD have wrongly re-anchored this correctly-stored show.
	if _, outcome := reanchorEventDate(stored, ny, phoenix); outcome != outcomeReanchored {
		t.Errorf("sanity: with a wrong Phoenix assumed zone the bug should manifest as a re-anchor; outcome=%v", outcome)
	}
}

// Documents the one residual false-positive (adversarial-review round 2): a
// correctly-stored NON-US explicit-time show whose UTC instant is exactly
// 03:00Z reads as 20:00 in the Phoenix fallback (the assumed zone for
// empty/non-US-state venues) and IS re-anchored. This is inherent ambiguity — a
// 20:00-Phoenix date-only show and a foreign 03:00Z explicit show are
// indistinguishable from the instant — and cannot be closed without dropping
// non-US date-only recovery (the item-5 deliverable). The backstop is the
// mandatory dry-run review before --confirm. This test pins the behavior so a
// future change to it is a conscious decision, not an accident.
func TestReanchorEventDate_NonUSExplicitAt0300Z_KnownLimitation(t *testing.T) {
	phoenix := mustLoc(t, "America/Phoenix")
	kolkata := mustLoc(t, "Asia/Kolkata")

	// 08:30 Kolkata (IST, UTC+5:30) = 03:00Z — a real morning show, correctly
	// stored, that unfortunately coincides with 20:00 Phoenix.
	stored := time.Date(2026, 7, 17, 8, 30, 0, 0, kolkata)
	if got := stored.UTC().Format(time.RFC3339); got != "2026-07-17T03:00:00Z" {
		t.Fatalf("setup: stored=%s want 2026-07-17T03:00:00Z", got)
	}
	_, outcome := reanchorEventDate(stored, kolkata, phoenix)
	if outcome != outcomeReanchored {
		t.Errorf("documented limitation changed: non-US explicit show at 03:00Z outcome=%v (was reanchored). "+
			"If this is intentional, update the dry-run-review guidance accordingly.", outcome)
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
