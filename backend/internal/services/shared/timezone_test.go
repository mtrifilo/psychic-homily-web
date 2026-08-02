package shared

import (
	"strings"
	"testing"

	"psychic-homily-backend/internal/testutil"
)

// NormalizeIANATimezone is validated against a REAL Postgres rather than a
// stub, because the entire point of the function is that Postgres' zone catalog
// and Go's differ. A fake would agree with whatever the test author assumed and
// prove nothing.
func TestNormalizeIANATimezone(t *testing.T) {
	db := testutil.SetupTestPostgres(t).DB

	str := func(s string) *string { return &s }

	cases := []struct {
		name    string
		in      *string
		want    *string // nil means "expect NULL"
		wantErr bool
	}{
		// Absent / empty: store NULL and let the caller's fallback chain decide.
		{name: "nil pointer", in: nil, want: nil},
		{name: "empty string", in: str(""), want: nil},
		{name: "whitespace only", in: str("   "), want: nil},
		{name: "tabs and newlines only", in: str("\t\n "), want: nil},

		// Canonical names pass through unchanged.
		{name: "canonical US zone", in: str("America/Phoenix"), want: str("America/Phoenix")},
		{name: "canonical three-part zone", in: str("America/Indiana/Indianapolis"), want: str("America/Indiana/Indianapolis")},
		{name: "canonical non-US zone", in: str("Europe/Ljubljana"), want: str("Europe/Ljubljana")},
		{name: "canonical UTC", in: str("UTC"), want: str("UTC")},

		// Non-canonical spelling resolves to the catalog's own spelling, which
		// is what gets persisted -- that is the "normalize" half of the name.
		{name: "lowercased", in: str("america/phoenix"), want: str("America/Phoenix")},
		{name: "uppercased", in: str("AMERICA/PHOENIX"), want: str("America/Phoenix")},
		{name: "surrounded by spaces", in: str("  America/Phoenix  "), want: str("America/Phoenix")},
		{name: "surrounded by tabs and newlines", in: str("\tAmerica/Phoenix\n"), want: str("America/Phoenix")},

		// Junk is rejected rather than silently stored.
		{name: "obvious junk", in: str("Not/AZone"), wantErr: true},
		{name: "sql-ish junk", in: str("'; DROP TABLE venues; --"), wantErr: true},
		{name: "empty-ish path", in: str("/"), wantErr: true},
		{name: "plain word", in: str("Phoenix"), wantErr: true},
		{name: "numeric", in: str("-07:00"), wantErr: true},

		// The cases that make time.LoadLocation the WRONG validator: Go accepts
		// both of these, Postgres does not carry them, and AT TIME ZONE would
		// raise on a value Go had blessed.
		{name: "Go-only abbreviation EST", in: str("EST"), wantErr: true},
		{name: "Go-only alias Local", in: str("Local"), wantErr: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := NormalizeIANATimezone(db, tc.in)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected an error for %q, got %v", derefForTest(tc.in), derefForTest(got))
				}
				if got != nil {
					t.Errorf("a rejected value must not also return a zone, got %q", *got)
				}
				// The message has to name the offending value or an operator
				// reading the log cannot tell which venue to fix.
				if !strings.Contains(err.Error(), strings.TrimSpace(derefForTest(tc.in))) {
					t.Errorf("error %q does not name the rejected value", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error for %q: %v", derefForTest(tc.in), err)
			}
			switch {
			case tc.want == nil && got != nil:
				t.Errorf("expected NULL, got %q", *got)
			case tc.want != nil && got == nil:
				t.Errorf("expected %q, got NULL", *tc.want)
			case tc.want != nil && got != nil && *got != *tc.want:
				t.Errorf("expected %q, got %q", *tc.want, *got)
			}
		})
	}
}

// A nil DB must be an error, not a panic and not a silent pass -- the function
// is a gate, and a gate that opens when its dependency is missing is not a gate.
func TestNormalizeIANATimezone_NilDBIsAnError(t *testing.T) {
	value := "America/Phoenix"
	got, err := NormalizeIANATimezone(nil, &value)
	if err == nil {
		t.Fatalf("expected an error with a nil DB, got %v", derefForTest(got))
	}
	if got != nil {
		t.Errorf("expected no zone with a nil DB, got %q", *got)
	}
}

// ...but a nil/blank input short-circuits BEFORE the DB is consulted, so it
// stays usable on a code path that has no handle.
func TestNormalizeIANATimezone_NilInputSkipsTheDB(t *testing.T) {
	got, err := NormalizeIANATimezone(nil, nil)
	if err != nil || got != nil {
		t.Fatalf("nil input must return (nil, nil) without touching the DB, got (%v, %v)", derefForTest(got), err)
	}
	blank := "  "
	got, err = NormalizeIANATimezone(nil, &blank)
	if err != nil || got != nil {
		t.Fatalf("blank input must return (nil, nil) without touching the DB, got (%v, %v)", derefForTest(got), err)
	}
}

func derefForTest(p *string) string {
	if p == nil {
		return "<nil>"
	}
	return *p
}
