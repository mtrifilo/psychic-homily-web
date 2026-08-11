package shared

import (
	"testing"

	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/geo"
)

// No DB is needed anywhere in this file: DeriveVenueLocation with a nil db skips
// the timezone write-boundary validation and behaves as a pure function over the
// offline geocoder. The validation itself is covered by timezone_test.go and by
// the per-caller integration suites.

// TestVenueLocationReadsCountry pins the country semantics PSY-1747 settled:
// every venue write path derives from v.Country.
//
// This is the decision the copies used to disagree on. The two data-sync import
// seams passed a hardcoded "" and the service paths read the column; the ticket
// resolved it in favour of reading the column, on the evidence that ExportedVenue
// carries no country field at all — so the constant described the DTO, not a
// posture about what a venue's country means.
//
// The "importer-shaped venue" case is the compatibility half: it proves that
// routing data_sync through VenueLocation is behaviour-identical today, because a
// venue the importer builds has a nil Country that derefs to the same "" the
// constant supplied. The "country set" case is the half that would have been
// wrong under the constant.
func TestVenueLocationReadsCountry(t *testing.T) {
	t.Run("importer-shaped venue with no country derives as blank", func(t *testing.T) {
		// Exactly the shape data_sync.importVenue builds: ExportedVenue has no
		// country field, so Country is never assigned.
		v := &catalogm.Venue{Name: "Crescent Ballroom", City: "Phoenix", State: "AZ"}
		if got := VenueLocation(v); got != (Location{City: "Phoenix", State: "AZ", Country: ""}) {
			t.Fatalf("VenueLocation = %+v, want blank country", got)
		}
	})

	t.Run("country column is read when set", func(t *testing.T) {
		country := "Netherlands"
		v := &catalogm.Venue{City: "Amsterdam", State: "NL", Country: &country}
		if got := VenueLocation(v).Country; got != "Netherlands" {
			t.Fatalf("Country = %q, want Netherlands", got)
		}
	})

	t.Run("the country actually reaches the geocoder and changes the answer", func(t *testing.T) {
		// The fixture has to be one where the COUNTRY is load-bearing, or the
		// subtest passes under the hardcoded "" it exists to rule out. Measured
		// against the embedded dataset: bare "Amsterdam" resolves to the Dutch one
		// on population alone, and only the country moves it to Amsterdam, NY. A
		// (city, state) pair like ("Amsterdam", "NL") does NOT work here — geo's
		// resolver reads a country code out of the state field, so it lands on
		// Europe/Amsterdam with or without the country, and pins nothing.
		blank := &catalogm.Venue{City: "Amsterdam"}
		if d := DeriveVenueLocation(nil, geo.Default(), VenueLocation(blank)); d.Timezone == nil ||
			*d.Timezone != "Europe/Amsterdam" {
			t.Fatalf("no-country timezone = %v, want Europe/Amsterdam", d.Timezone)
		}

		us := "US"
		withCountry := &catalogm.Venue{City: "Amsterdam", Country: &us}
		d := DeriveVenueLocation(nil, geo.Default(), VenueLocation(withCountry))
		if d.Timezone == nil || *d.Timezone != "America/New_York" {
			t.Fatalf("timezone = %v, want America/New_York — the country column is "+
				"not reaching the geocoder", d.Timezone)
		}
	})
}

func TestNullableLocation(t *testing.T) {
	city, state := "Tucson", "AZ"
	if got := NullableLocation(&city, &state, nil); got != (Location{City: "Tucson", State: "AZ"}) {
		t.Fatalf("NullableLocation = %+v", got)
	}
	if got := NullableLocation(nil, nil, nil); got != (Location{}) {
		t.Fatalf("all-nil NullableLocation = %+v, want zero Location", got)
	}
}

func TestLocationTouched(t *testing.T) {
	cases := []struct {
		name    string
		updates map[string]interface{}
		want    bool
	}{
		{"empty", map[string]interface{}{}, false},
		{"non-location key only", map[string]interface{}{"name": "x"}, false},
		{"city", map[string]interface{}{"city": "Phoenix"}, true},
		{"state", map[string]interface{}{"state": "AZ"}, true},
		{"country", map[string]interface{}{"country": "US"}, true},
		// A CLEAR is a relocation too: the derived columns must be recomputed
		// from the now-blank location, not left pointing at the old city.
		{"explicit nil clear", map[string]interface{}{"city": nil}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := LocationTouched(tc.updates); got != tc.want {
				t.Fatalf("LocationTouched = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestEffectiveLocation pins the overlay for all three shapes the callers stage
// location columns in. Before PSY-1747 each had its own helper, and each helper
// understood only its own shape — which is exactly why a shared one has to
// understand all of them.
func TestEffectiveLocation(t *testing.T) {
	current := Location{City: "Phoenix", State: "AZ", Country: "US"}

	t.Run("absent keys fall back to the stored value", func(t *testing.T) {
		if got := EffectiveLocation(map[string]interface{}{}, current); got != current {
			t.Fatalf("EffectiveLocation = %+v, want %+v", got, current)
		}
	})

	t.Run("plain string, the admin JSONB and UpdateFestival shape", func(t *testing.T) {
		got := EffectiveLocation(map[string]interface{}{"city": "Tucson"}, current)
		if got != (Location{City: "Tucson", State: "AZ", Country: "US"}) {
			t.Fatalf("EffectiveLocation = %+v", got)
		}
	})

	t.Run("*string, the UpdateArtist NilIfEmpty shape", func(t *testing.T) {
		tucson := "Tucson"
		got := EffectiveLocation(map[string]interface{}{"city": &tucson}, current)
		if got.City != "Tucson" {
			t.Fatalf("City = %q, want Tucson", got.City)
		}
	})

	t.Run("nil *string clears rather than falling back", func(t *testing.T) {
		got := EffectiveLocation(map[string]interface{}{"city": (*string)(nil)}, current)
		if got.City != "" {
			t.Fatalf("City = %q, want blank", got.City)
		}
	})

	t.Run("explicit untyped nil clears rather than falling back", func(t *testing.T) {
		// The rollback shape: undoing "someone added a city" restores SQL NULL.
		// Falling back here would derive from the very city the write erases.
		got := EffectiveLocation(map[string]interface{}{"city": nil, "state": nil}, current)
		if got.City != "" || got.State != "" {
			t.Fatalf("EffectiveLocation = %+v, want city and state blank", got)
		}
		if got.Country != "US" {
			t.Fatalf("Country = %q, want the untouched stored value", got.Country)
		}
	})

	t.Run("uninterpretable value falls back to the stored one", func(t *testing.T) {
		// Deriving from the entity's current value beats deriving from a guess.
		got := EffectiveLocation(map[string]interface{}{"city": 42}, current)
		if got.City != "Phoenix" {
			t.Fatalf("City = %q, want the stored Phoenix", got.City)
		}
	})
}

// TestDeriveVenueLocation pins that a venue's four derived columns move together
// — all resolved on a hit, all nil on a miss. A partial refresh is the drift this
// file exists to prevent.
func TestDeriveVenueLocation(t *testing.T) {
	t.Run("hit resolves all four columns", func(t *testing.T) {
		d := DeriveVenueLocation(nil, geo.Default(), Location{City: "Phoenix", State: "AZ"})
		if d.Timezone == nil || *d.Timezone != "America/Phoenix" {
			t.Fatalf("timezone = %v, want America/Phoenix", d.Timezone)
		}
		if d.Latitude == nil || d.Longitude == nil {
			t.Errorf("lat/lng = %v/%v, want both set", d.Latitude, d.Longitude)
		}
		if d.Metro == nil || *d.Metro != "38060" {
			t.Errorf("metro = %v, want 38060 (Phoenix CBSA)", d.Metro)
		}
	})

	t.Run("miss nils all four rather than leaving stale values", func(t *testing.T) {
		d := DeriveVenueLocation(nil, geo.Default(), Location{City: "Nowheresville Xyzzy", State: "ZZ"})
		if d.Latitude != nil || d.Longitude != nil || d.Timezone != nil || d.Metro != nil {
			t.Fatalf("miss produced %+v, want all nil", d)
		}
	})

	t.Run("nil geocoder is a total miss, not a panic", func(t *testing.T) {
		d := DeriveVenueLocation(nil, nil, Location{City: "Phoenix", State: "AZ"})
		if d.Latitude != nil || d.Longitude != nil || d.Timezone != nil || d.Metro != nil {
			t.Fatalf("nil geocoder produced %+v, want all nil", d)
		}
	})
}

// TestDerivedVenueLocationApply pins that both application shapes write the same
// complete column set. The map form must write its nils EXPLICITLY: an omitted
// key leaves the stale value in the row, which is the PSY-1709 failure mode.
func TestDerivedVenueLocationApply(t *testing.T) {
	d := DeriveVenueLocation(nil, geo.Default(), Location{City: "Phoenix", State: "AZ"})

	t.Run("ApplyTo sets every column on the model", func(t *testing.T) {
		v := &catalogm.Venue{City: "Phoenix", State: "AZ"}
		d.ApplyTo(v)
		if v.Latitude != d.Latitude || v.Longitude != d.Longitude ||
			v.Timezone != d.Timezone || v.Metro != d.Metro {
			t.Fatalf("ApplyTo left a column unset: %+v", v)
		}
	})

	t.Run("ApplyToUpdates writes the DERIVED values, not just the keys", func(t *testing.T) {
		// Without this case the miss case below is satisfied by an implementation
		// that hardcodes nil for all four keys.
		updates := map[string]interface{}{"city": "Phoenix"}
		d.ApplyToUpdates(updates)
		if tz, _ := updates["timezone"].(*string); tz == nil || *tz != "America/Phoenix" {
			t.Errorf("timezone = %v, want America/Phoenix", updates["timezone"])
		}
		if metro, _ := updates["metro"].(*string); metro == nil || *metro != "38060" {
			t.Errorf("metro = %v, want 38060", updates["metro"])
		}
		if lat, _ := updates["latitude"].(*float64); lat == nil {
			t.Errorf("latitude = %v, want a resolved coordinate", updates["latitude"])
		}
		if lng, _ := updates["longitude"].(*float64); lng == nil {
			t.Errorf("longitude = %v, want a resolved coordinate", updates["longitude"])
		}
	})

	t.Run("ApplyToUpdates writes every key, including nils", func(t *testing.T) {
		miss := DeriveVenueLocation(nil, geo.Default(), Location{City: "Nowheresville Xyzzy", State: "ZZ"})
		updates := map[string]interface{}{"city": "Nowheresville Xyzzy"}
		miss.ApplyToUpdates(updates)
		for _, key := range []string{"latitude", "longitude", "timezone", "metro"} {
			value, ok := updates[key]
			if !ok {
				t.Fatalf("%q missing from updates: an omitted key leaves the stale value in the row", key)
			}
			if value != nil {
				// Typed nils (*float64)(nil) / (*string)(nil) are how GORM writes
				// SQL NULL, so compare against the derived field, not literal nil.
				switch key {
				case "latitude":
					if value.(*float64) != nil {
						t.Errorf("%q = %v, want nil on a miss", key, value)
					}
				case "longitude":
					if value.(*float64) != nil {
						t.Errorf("%q = %v, want nil on a miss", key, value)
					}
				default:
					if value.(*string) != nil {
						t.Errorf("%q = %v, want nil on a miss", key, value)
					}
				}
			}
		}
	})
}

func TestDeriveMetro(t *testing.T) {
	t.Run("US city rolls up to its CBSA", func(t *testing.T) {
		got := DeriveMetro(geo.Default(), Location{City: "Phoenix", State: "AZ"})
		if got == nil || *got != "38060" {
			t.Fatalf("metro = %v, want 38060", got)
		}
	})

	t.Run("unresolvable location yields nil, never a stale code", func(t *testing.T) {
		if got := DeriveMetro(geo.Default(), Location{City: "Nowheresville Xyzzy", State: "ZZ"}); got != nil {
			t.Fatalf("metro = %v, want nil", got)
		}
	})

	t.Run("nil geocoder yields nil", func(t *testing.T) {
		if got := DeriveMetro(nil, Location{City: "Phoenix", State: "AZ"}); got != nil {
			t.Fatalf("metro = %v, want nil", got)
		}
	})
}
