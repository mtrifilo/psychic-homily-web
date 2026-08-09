package admin

import (
	"gorm.io/gorm"

	"psychic-homily-backend/internal/logger"
	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/geo"
	"psychic-homily-backend/internal/services/shared"
)

// applyDerivedVenueLocation recomputes, into updates, the columns the system
// DERIVES from a venue's (city, state, country) — latitude, longitude, timezone
// and metro — whenever the write it is building touches any of those three. A
// write that does not relocate the venue is left completely alone.
//
// Reached through applyDerivedLocation, which is where the two admin write paths
// that share it are named; not called directly. What makes the venue case the
// severe one: a path that moves a venue's city without re-deriving leaves the
// coordinates and TIMEZONE of the city it moved away from, and every venue-local
// surface (the show-list partition, the ICS feed, reminder rendering) then reads
// that stale zone as the venue's real one and silently mis-dates its shows
// instead of failing visibly. Rollback shipped with exactly that bug (PSY-1709).
//
// The four columns are on no editor's field list, so recomputing them can never
// clobber a value a human chose — they are ours.
//
// A geocode MISS writes NULL across all four rather than leaving the old values,
// for the reason spelled out on VenueService.UpdateVenue's re-geocode branch.
//
// db is the handle to read and validate through. Both callers pass s.db, and for
// both that is correct rather than accidental: Rollback writes through s.db too,
// and ApprovePendingEdit finishes building this map BEFORE it opens its
// transaction, so there is no open tx to validate inside — the write applies an
// already-decided map. A future caller that builds updates WHILE holding a
// transaction must pass that tx, or the validating read lands on a second
// connection outside the transaction carrying the write (the shape PSY-1709 fixed
// in catalog.VenueService.FindOrCreateVenue).
//
// Best effort: a venue row that cannot be read leaves the four columns alone
// rather than failing the whole edit, which would throw away the fields that CAN
// be applied.
//
// NOT covered here: artists and festivals carry a derived metro of their own and
// no timezone, so they go through applyDerivedEntityMetro — the metro-only
// sibling of this function, reached through the same dispatcher.
func applyDerivedVenueLocation(db *gorm.DB, venueID uint, updates map[string]interface{}) {
	if !locationChanged(updates) {
		return
	}

	var current catalogm.Venue
	if err := db.Select("city", "state", "country").First(&current, venueID).Error; err != nil {
		logger.Default().Warn("venue_derived_location_rederive_failed",
			"venue_id", venueID, "error", err.Error())
		return
	}
	// The effective POST-write location: the incoming value where this write
	// carries one, the venue's current value otherwise.
	city := updatedString(updates, "city", current.City)
	state := updatedString(updates, "state", current.State)
	country := updatedString(updates, "country", shared.DerefOrEmpty(current.Country))

	lat, lng, tz := geo.LookupPointers(geo.Default(), city, state, country)
	updates["latitude"] = lat
	updates["longitude"] = lng
	// The PSY-1707 write-boundary invariant. It matters most on the approve path:
	// a CONTRIBUTOR's city/state edit is what triggers the re-derivation, so that
	// is the one an outsider can aim, even though the zone itself is ours.
	updates["timezone"] = shared.NormalizedGeocodedTimezoneOrNull(db, tz, "venue_id", venueID)
	// metro is a sibling of the geocoding (PSY-1255 step B) and goes just as stale
	// when a write relocates the venue.
	updates["metro"] = geo.MetroPointer(geo.Default(), city, state, country)
}
