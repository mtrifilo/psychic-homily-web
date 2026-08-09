package admin

import (
	"gorm.io/gorm"

	"psychic-homily-backend/internal/logger"
	"psychic-homily-backend/internal/services/geo"
	"psychic-homily-backend/internal/services/shared"
)

// applyDerivedLocation recomputes, into updates, whatever the system DERIVES
// from the entity's (city, state, country) — the single answer to "what does
// this entity type derive from its location?", so no caller has to know.
// An entity type that derives nothing, and a write that does not relocate the
// entity, are both no-ops.
//
// Both admin write paths that apply an edit as an untyped GORM update map call
// it: ApprovePendingEdit (contribution + trusted-tier auto-apply) and
// RevisionService.Rollback. They must not diverge — a path that moves an entity
// without re-deriving leaves it in its restored city carrying what was resolved
// for the city it moved AWAY from. Rollback shipped twice without exactly that:
// once for the venue timezone (PSY-1709), then for the artist/festival metro
// (PSY-1744). Hence one entry point rather than a comment asking copies to stay
// in step.
//
// NOT a completeness guarantee for the codebase: catalog.UpdateArtist,
// catalog.UpdateFestival and catalog.VenueService each derive the same columns
// again, in their own typed shape, for the paths that go through a service
// rather than an update map. Those copies were not the source of either bug and
// collapsing all of them wants a lower layer (geo/shared) and its own ticket.
//
// db is the handle to read and validate through; see applyDerivedVenueLocation
// for why both callers pass s.db and what a future caller that builds updates
// while holding a transaction must do instead.
func applyDerivedLocation(db *gorm.DB, entityType string, entityID uint, updates map[string]interface{}) {
	switch entityType {
	case "venue":
		// Venues derive coordinates and a timezone as well, so they get the
		// four-column helper. Its metro write is the same one the metro-only
		// branch below makes.
		applyDerivedVenueLocation(db, entityID, updates)
	case "artist", "festival":
		applyDerivedEntityMetro(db, entityType, entityID, updates)
	}
}

// locationChanged reports whether a write touches any component of the location
// the derived columns are computed from. One definition, so adding a fourth
// component means editing one line rather than finding every copy.
func locationChanged(updates map[string]interface{}) bool {
	_, city := updates["city"]
	_, state := updates["state"]
	_, country := updates["country"]
	return city || state || country
}

// updatedString returns the EFFECTIVE post-write value of key: the write's own
// value when it carries one, the fallback (the entity's current value) when it
// does not.
//
// A key present with an explicit nil is a write that CLEARS the column, so it
// resolves to the empty string, not the fallback. That case is real on the
// rollback path — artists and festivals have nullable city/state/country, so
// undoing "someone added a city" restores SQL NULL — and falling back to the
// current value there would derive from the very city the same write erases,
// which is the stale pairing these helpers exist to prevent.
func updatedString(updates map[string]interface{}, key, fallback string) string {
	v, ok := updates[key]
	if !ok {
		return fallback
	}
	switch typed := v.(type) {
	case nil:
		return ""
	case string:
		return typed
	default:
		// Both callers build these three keys from JSONB, so a string or a nil
		// is all that reaches here. Anything else is a shape this function
		// cannot interpret, and deriving from the current value beats deriving
		// from a guess.
		return fallback
	}
}

// applyDerivedEntityMetro recomputes the metro an artist's or a festival's
// location resolves to (PSY-1255 step B, PSY-1278). Reached through
// applyDerivedLocation; not called directly.
//
// A path that moves an artist's city without re-deriving leaves the metro of the
// city it moved AWAY from, and metro is what keys the entity into a scene: the
// artist keeps appearing in the old metro's scene page and counts until the
// nightly reconciler (catalog.ReconcileArtistMetros, run by
// cmd/backfill-entity-metro) happens to correct it. That reconciler covers
// venues too and is the other place the set of metro-carrying entities is
// written down — a fifth one has to be added in both.
//
// metro is on no editor's field list, so recomputing it can never clobber a
// value a human chose. A resolution MISS writes NULL rather than leaving the old
// code: a stale metro is worse than an absent one.
//
// The row is read through the entity's TABLE rather than its model so the read
// resolves the same way the write does — both callers apply the map with
// Table(entityType + "s").Updates(...), spelled out at RevisionService.Rollback
// and ApprovePendingEdit — and so one function serves both entities instead of
// two typed branches drifting apart.
//
// Best effort, as with the venue helper: a row that cannot be read leaves metro
// alone rather than failing the whole edit.
func applyDerivedEntityMetro(db *gorm.DB, entityType string, entityID uint, updates map[string]interface{}) {
	if !locationChanged(updates) {
		return
	}

	var current struct {
		City    *string
		State   *string
		Country *string
	}
	if err := db.Table(entityType+"s").
		Select("city", "state", "country").
		Where("id = ?", entityID).
		Take(&current).Error; err != nil {
		logger.Default().Warn("entity_derived_metro_rederive_failed",
			"entity_type", entityType, "entity_id", entityID, "error", err.Error())
		return
	}

	// The effective POST-write location: the incoming value where this write
	// carries one, the entity's current value otherwise.
	city := updatedString(updates, "city", shared.DerefOrEmpty(current.City))
	state := updatedString(updates, "state", shared.DerefOrEmpty(current.State))
	country := updatedString(updates, "country", shared.DerefOrEmpty(current.Country))

	updates["metro"] = geo.MetroPointer(geo.Default(), city, state, country)
}
