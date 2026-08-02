package admin

import (
	"log/slog"

	"gorm.io/gorm"

	"psychic-homily-backend/internal/services/shared"
)

// normalizedGeocodedTimezone canonicalizes a geocoder-derived venue zone, or
// returns nil when Postgres does not recognize it.
//
// The admin package has three write paths that set venues.timezone without
// going through VenueService.applyGeocoding — the pending-edit approval, and
// the bulk importer's two venue-creating seams — so the invariant PSY-1707
// establishes needs a second home here rather than being restated three times.
//
// Log-and-NULL rather than fail, matching the VenueService twin: the value is
// derived internally from the GeoNames dataset, so a bad one is our bug, and
// NULL is a shape every reader already handles (it is what a geocode MISS
// produces). A request-supplied zone would warrant a 422 instead; no venue
// write path accepts one today.
func normalizedGeocodedTimezone(db *gorm.DB, tz *string) *string {
	// No database means nothing is being persisted -- applyGeocoding is also used
	// as a pure derivation helper in unit tests -- so there is nothing to guard
	// and the derived value passes through untouched. This is safe rather than
	// fail-open because the column can only be written through a service that
	// HAS a handle; validation lives on every such path.
	if db == nil {
		return tz
	}
	rejected := ""
	if tz != nil {
		rejected = *tz
	}
	canonical, err := shared.NormalizeIANATimezone(db, tz)
	if err != nil {
		slog.Error("venue geocode produced a timezone Postgres does not recognize; storing NULL",
			"rejected_timezone", rejected, "error", err)
		return nil
	}
	return canonical
}
