package shared

import (
	"testing"
)

// The predicate's whole job is to separate a zone the site KNOWS from the
// America/Phoenix answer utils.GetTimezoneForState gives to everything else, so
// the cases that matter are the ones where the two produce the same location.
func TestIsShowTimezoneResolved(t *testing.T) {
	cases := []struct {
		name     string
		timezone *string
		state    string
		want     bool
	}{
		{"explicit IANA zone", strptr("Europe/Berlin"), "", true},
		{"explicit zone outranks a blank state", strptr("Europe/Berlin"), "", true},
		{"explicit zone outranks a non-US state", strptr("Europe/Berlin"), "England", true},
		{"US state, no zone column", nil, "AZ", true},
		{"US state is case insensitive", nil, "az", true},
		{"empty zone string is not a zone", strptr(""), "IL", true},
		{"malformed zone falls through to a US state", strptr("Not/AZone"), "IL", true},
		{"malformed zone with no usable state", strptr("Not/AZone"), "England", false},
		{"non-US state", nil, "England", false},
		{"blank state", nil, "", false},
		{"whitespace state", nil, "  ", false},
		{"nothing at all", nil, "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsShowTimezoneResolved(tc.timezone, tc.state); got != tc.want {
				t.Errorf("IsShowTimezoneResolved(%v, %q) = %v, want %v", tc.timezone, tc.state, got, tc.want)
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
}

func TestPublishedZone(t *testing.T) {
	loc, _ := EventZone(strptr("Europe/Berlin"), "")
	if zone := PublishedZone(loc, true); zone == nil || *zone != "Europe/Berlin" {
		t.Errorf("PublishedZone(Berlin, true) = %v, want Europe/Berlin", zone)
	}
	if zone := PublishedZone(loc, false); zone != nil {
		t.Errorf("PublishedZone(Berlin, false) = %q, want nil", *zone)
	}
	if zone := PublishedZone(nil, true); zone != nil {
		t.Errorf("PublishedZone(nil, true) = %q, want nil", *zone)
	}
}
