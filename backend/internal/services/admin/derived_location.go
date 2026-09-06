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
// (PSY-1744).
//
// This is the entity-type DISPATCHER and nothing else. What each type derives,
// and how, lives one layer down in services/shared/derived_location.go, which
// PSY-1747 created so that catalog's own write paths resolve the same columns
// through the same code rather than through their own typed copies. Read that
// file before changing the derivation; change this one only to add an entity
// type.
//
// db is the handle to read and validate through; see applyDerivedVenueLocation
// for which handle each caller passes, and why a caller that builds updates
// while holding a transaction must pass that transaction.
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

// applyDerivedEntityMetro recomputes the metro an artist's or a festival's
// location resolves to (PSY-1255 step B, PSY-1278). Reached through
// applyDerivedLocation; not called directly. Why the recompute matters, and why a
// miss writes NULL, are on shared.DeriveMetro.
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
	if !shared.LocationTouched(updates) {
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

	loc := shared.EffectiveLocation(updates,
		shared.NullableLocation(current.City, current.State, current.Country))
	updates["metro"] = shared.DeriveMetro(geo.Default(), loc)
}
