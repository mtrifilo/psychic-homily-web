package shared

import (
	"testing"

	// Embed the IANA tz database so LoadLocation resolves on any CI image,
	// matching every other timezone-bearing test in this tree.
	_ "time/tzdata"

	"psychic-homily-backend/internal/utils"
)

// The predicate's whole job is to separate a zone the site KNOWS from the
// America/Phoenix answer utils.GetTimezoneForState gives to everything else, so
// the cases that matter are the ones where the two produce the same location.
func TestEventLocationResolved_Nameability(t *testing.T) {
	zone := func(s string) *string { return &s }
	cases := []struct {
		name     string
		timezone *string
		state    string
		want     bool
	}{
		{"explicit IANA zone", zone("Europe/Berlin"), "", true},
		{"explicit zone outranks a non-US state", zone("Europe/Berlin"), "England", true},
		{"US state, no zone column", nil, "AZ", true},
		{"US state is case insensitive", nil, "az", true},
		{"empty zone string is not a zone", zone(""), "IL", true},
		{"malformed zone falls through to a US state", zone("Not/AZone"), "IL", true},
		{"malformed zone with no usable state", zone("Not/AZone"), "England", false},
		{"non-US state", nil, "England", false},
		{"blank state", nil, "", false},
		{"whitespace state", nil, "  ", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, got := EventLocationResolved(tc.timezone, tc.state)
			if got != tc.want {
				t.Errorf("EventLocationResolved(%v, %q) resolved = %v, want %v", tc.timezone, tc.state, got, tc.want)
			}
		})
	}
}

// The location half must stay utils.EventLocation's, branch for branch, or a
// date read here lands on a different day than one read anywhere else.
func TestEventLocationResolved_LocationMatchesTheSharedPrecedence(t *testing.T) {
	zone := func(s string) *string { return &s }
	cases := []struct {
		name     string
		timezone *string
		state    string
		wantLoc  string
	}{
		{"explicit zone wins", zone("Europe/Berlin"), "AZ", "Europe/Berlin"},
		{"malformed zone falls to the state map", zone("Not/AZone"), "IL", "America/Chicago"},
		{"state map when no zone", nil, "IL", "America/Chicago"},
		{"surrender is still readable", nil, "England", "America/Phoenix"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			loc, _ := EventLocationResolved(tc.timezone, tc.state)
			if loc == nil {
				t.Fatal("no location returned; dates still have to be readable")
			}
			if loc.String() != tc.wantLoc {
				t.Errorf("location = %q, want %q", loc.String(), tc.wantLoc)
			}
			if want := utils.EventLocation(tc.timezone, tc.state).String(); loc.String() != want {
				t.Errorf("location = %q, but utils.EventLocation says %q", loc.String(), want)
			}
		})
	}
}

// The Arizona default is the exact value the surrender produces, so a caller
// with an AZ venue and a caller with a London one must not receive the same
// answer even though both read on America/Phoenix.
func TestEventZone_WithholdsTheNameButKeepsTheLocation(t *testing.T) {
	loc, zone := EventZone(nil, "England")
	if loc == nil {
		t.Fatal("EventZone returned no location; dates still have to be readable")
	}
	if loc.String() != "America/Phoenix" {
		t.Errorf("location = %q, want the America/Phoenix fallback", loc.String())
	}
	if zone != nil {
		t.Errorf("zone = %q, want nil for a state outside the US map", *zone)
	}

	loc, zone = EventZone(nil, "AZ")
	if loc == nil || loc.String() != "America/Phoenix" {
		t.Fatalf("location = %v, want America/Phoenix", loc)
	}
	if zone == nil || *zone != "America/Phoenix" {
		t.Errorf("zone = %v, want America/Phoenix named for a state the map carries", zone)
	}

	berlin := "Europe/Berlin"
	if _, zone = EventZone(&berlin, ""); zone == nil || *zone != berlin {
		t.Errorf("zone = %v, want Europe/Berlin", zone)
	}
}
