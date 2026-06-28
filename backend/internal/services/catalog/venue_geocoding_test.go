package catalog

import (
	"testing"

	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/geo"
)

// TestApplyGeocoding verifies the VenueService geocoding hook resolves and sets
// timezone/coordinates from a venue's location, and degrades gracefully on a
// miss. No DB needed — applyGeocoding only touches the in-memory geocoder. (PSY-985)
func TestApplyGeocoding(t *testing.T) {
	s := &VenueService{geocoder: geo.Default()}

	t.Run("US venue resolved by state", func(t *testing.T) {
		v := &catalogm.Venue{City: "Phoenix", State: "AZ"}
		s.applyGeocoding(v)
		if v.Timezone == nil || *v.Timezone != "America/Phoenix" {
			t.Fatalf("timezone = %v, want America/Phoenix", v.Timezone)
		}
		if v.Latitude == nil || v.Longitude == nil {
			t.Errorf("expected lat/lng populated, got lat=%v lng=%v", v.Latitude, v.Longitude)
		}
		// metro is a sibling of the geocoding (PSY-1255 step B).
		if v.Metro == nil || *v.Metro != "38060" {
			t.Errorf("metro = %v, want 38060 (Phoenix CBSA)", v.Metro)
		}
	})

	t.Run("international venue resolved by country name", func(t *testing.T) {
		country := "Netherlands"
		v := &catalogm.Venue{City: "Amsterdam", State: "NL", Country: &country}
		s.applyGeocoding(v)
		if v.Timezone == nil || *v.Timezone != "Europe/Amsterdam" {
			t.Fatalf("timezone = %v, want Europe/Amsterdam", v.Timezone)
		}
	})

	t.Run("geocode miss leaves all fields nil (legacy fallback)", func(t *testing.T) {
		v := &catalogm.Venue{City: "Nowherecityville", State: "ZZ"}
		s.applyGeocoding(v)
		// All-or-nothing invariant: a miss must leave lat/lng/timezone/metro ALL nil.
		// UpdateVenue relies on this — it forwards these pointers straight into the
		// GORM updates map, so a miss must write SQL NULL across all four (PSY-1255).
		if v.Timezone != nil || v.Latitude != nil || v.Longitude != nil || v.Metro != nil {
			t.Errorf("expected all geo fields nil on miss, got tz=%v lat=%v lng=%v metro=%v", v.Timezone, v.Latitude, v.Longitude, v.Metro)
		}
	})

	t.Run("nil geocoder is a no-op", func(t *testing.T) {
		ns := &VenueService{}
		v := &catalogm.Venue{City: "Phoenix", State: "AZ"}
		ns.applyGeocoding(v)
		if v.Timezone != nil {
			t.Errorf("expected no-op with nil geocoder, got %q", *v.Timezone)
		}
	})
}
