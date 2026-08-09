package admin

import (
	"gorm.io/gorm"

	"psychic-homily-backend/internal/logger"
	"psychic-homily-backend/internal/services/geo"
)

// metroOnlyEntityTypes are the entity types whose ONLY system-derived location
// column is metro. Venues are deliberately absent: they derive latitude,
// longitude and timezone from the same location and go through
// applyDerivedVenueLocation, which writes metro as one of its four columns.
// Adding "venue" here would give a venue write a metro without the coordinates
// and zone that belong with it.
var metroOnlyEntityTypes = map[string]bool{
	"artist":   true,
	"festival": true,
}

// applyDerivedEntityMetro recomputes, into updates, the metro an artist's or a
// festival's (city, state, country) resolves to (PSY-1255 step B, PSY-1278),
// whenever the write it is building touches any of those three. A write that
// does not relocate the entity is left completely alone, and an entity type that
// derives nothing from its location is a no-op.
//
// This is the metro-only sibling of applyDerivedVenueLocation, and it exists for
// the same reason: both admin write paths that apply an edit as an untyped GORM
// update map — ApprovePendingEdit (contribution + trusted-tier auto-apply) and
// RevisionService.Rollback — have to maintain the derived columns, and asking
// two copies to stay in step is how Rollback shipped without the venue
// re-derivation (PSY-1709) and then without this one (PSY-1744). One function,
// two callers, compiler-enforced.
//
// A path that moves an artist's city without re-deriving leaves the metro of the
// city it moved AWAY from, which is what keys the entity into a scene: the
// artist keeps showing up in the old metro's scene page and count until the
// nightly cmd/backfill-entity-metro reconciler happens to correct it. Milder
// than the venue case (no timezone, so no mis-dated shows), same shape.
//
// metro is on no editor's field list, so recomputing it can never clobber a
// value a human chose — it is ours. A resolution MISS writes NULL rather than
// leaving the old code, for the same reason the venue helper does: a stale code
// is worse than an absent one.
//
// The row is read through the entity's TABLE rather than its model so the read
// resolves the same way the write does — both callers apply the map with
// Table(entityType + "s").Updates(...) — and so one type list serves both
// entities instead of two near-identical typed branches drifting apart.
//
// db is the handle to read through; see applyDerivedVenueLocation for why both
// callers pass s.db and what a future caller holding a transaction must do
// instead.
//
// Best effort: a row that cannot be read leaves metro alone rather than failing
// the whole edit, which would throw away the fields that CAN be applied.
func applyDerivedEntityMetro(db *gorm.DB, entityType string, entityID uint, updates map[string]interface{}) {
	if !metroOnlyEntityTypes[entityType] {
		return
	}
	_, cityChanged := updates["city"]
	_, stateChanged := updates["state"]
	_, countryChanged := updates["country"]
	if !cityChanged && !stateChanged && !countryChanged {
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
	city := updatedString(updates, "city", derefOrEmpty(current.City))
	state := updatedString(updates, "state", derefOrEmpty(current.State))
	country := updatedString(updates, "country", derefOrEmpty(current.Country))

	updates["metro"] = geo.MetroPointer(geo.Default(), city, state, country)
}

// derefOrEmpty reads a nullable location column as the empty string, the shape
// the geocoder takes for "not set".
func derefOrEmpty(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
