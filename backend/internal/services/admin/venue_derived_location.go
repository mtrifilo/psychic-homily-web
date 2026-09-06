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
// The derivation itself — which columns, the geocode-miss policy, the PSY-1707
// timezone write-boundary — is shared.DeriveVenueLocation, the same code
// catalog.VenueService and the data-sync import seams resolve through since
// PSY-1747. This function is only the read-current-row-and-overlay-the-write
// half, which is specific to applying an edit as an untyped map.
//
// The timezone write-boundary matters most on THIS path of all the ones that
// share the derivation: a CONTRIBUTOR's city/state edit is what triggers the
// re-derivation here, so this is the one an outsider can aim, even though the
// resolved zone itself is ours and never theirs to supply.
//
// db is the handle to read and validate through, and the right handle differs by
// caller because the two build this map at different moments.
// ApprovePendingEdit finishes building it BEFORE it opens its transaction, so
// there is no open tx to validate inside and it passes s.db; the write then
// applies an already-decided map. RevisionService.Rollback decides which fields
// survive only after a locked read, so it builds the map WHILE holding its
// transaction and passes that tx. A caller in the second shape that passed s.db
// would land the validating read on a second connection outside the transaction
// carrying the write, which is the shape PSY-1709 fixed in
// catalog.VenueService.FindOrCreateVenue.
//
// Best effort: a venue row that cannot be read leaves the four columns alone
// rather than failing the whole edit, which would throw away the fields that CAN
// be applied. That degradation is the caller's to keep: inside a transaction a
// failed statement aborts the whole transaction, so a caller passing a tx gets a
// failed write rather than a skipped derivation.
//
// NOT covered here: artists and festivals carry a derived metro of their own and
// no timezone, so they go through applyDerivedEntityMetro — the metro-only
// sibling of this function, reached through the same dispatcher.
func applyDerivedVenueLocation(db *gorm.DB, venueID uint, updates map[string]interface{}) {
	if !shared.LocationTouched(updates) {
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
	loc := shared.EffectiveLocation(updates, shared.VenueLocation(&current))
	shared.DeriveVenueLocation(db, geo.Default(), loc, "venue_id", venueID).
		ApplyToUpdates(updates)
}
