package admin

import (
	"errors"

	catalogm "psychic-homily-backend/internal/models/catalog"
)

// ErrPendingEditDisallowedFields is the sentinel returned by
// ApprovePendingEdit when field_changes carries one or more columns not on
// the per-entity allowlist (PSY-572). The pending_edit row is auto-marked
// 'rejected' with a rejection_reason naming the offending fields, and the
// entity is left untouched. Handler callers can errors.Is for this sentinel
// to map to a 400 with the rejected list.
var ErrPendingEditDisallowedFields = errors.New("pending edit carries disallowed fields")

// AllowedEditFields returns the per-entity allowlist of column names that
// the pending-edit pipeline is permitted to mutate, plus a boolean for
// whether the entityType is recognized at all.
//
// The per-entity maps live alongside the entity models in
// internal/models/catalog/{entity}_allowlist.go so each entity owns its
// own contributor-editable surface area (PSY-424 per-entity convention).
// This aggregator is the single read point used by both the suggest-edit
// validator (handlers/admin/pending_edit.go) and the approve gate in
// services/admin/pending_edit.go::ApprovePendingEdit.
func AllowedEditFields(entityType string) (map[string]bool, bool) {
	switch entityType {
	case PendingEditEntityArtist:
		return catalogm.ArtistAllowedEditFields, true
	case PendingEditEntityVenue:
		return catalogm.VenueAllowedEditFields, true
	case PendingEditEntityFestival:
		return catalogm.FestivalAllowedEditFields, true
	case PendingEditEntityRelease:
		return catalogm.ReleaseAllowedEditFields, true
	case PendingEditEntityLabel:
		return catalogm.LabelAllowedEditFields, true
	default:
		return nil, false
	}
}

// FilterAllowedFields partitions the proposed FieldChanges into those
// permitted by the per-entity allowlist and those rejected. Used by
// ApprovePendingEdit as a defence-in-depth gate: even if a malformed or
// maliciously-crafted pending_entity_edits row carries columns the
// suggest-edit validator never sees (e.g. a row inserted via a different
// path, or one whose handler validator was bypassed), Updates() will not
// be called with a non-allowlisted column.
//
// rejected contains the field NAMES only (not full FieldChange structs)
// because the caller logs them and surfaces them in an error message —
// the original values are sensitive to log if the attacker controlled
// them.
//
// Unrecognized entityType returns (nil, all changes) so the caller can
// distinguish "unknown entity type" from "all fields rejected for a
// known entity": rejected == len(changes) AND filtered is nil.
func FilterAllowedFields(entityType string, changes []FieldChange) (filtered []FieldChange, rejected []string) {
	allowed, ok := AllowedEditFields(entityType)
	if !ok {
		// Unknown entity type — nothing is allowlisted, all fields rejected.
		rejected = make([]string, 0, len(changes))
		for _, c := range changes {
			rejected = append(rejected, c.Field)
		}
		return nil, rejected
	}
	filtered = make([]FieldChange, 0, len(changes))
	for _, c := range changes {
		if allowed[c.Field] {
			filtered = append(filtered, c)
		} else {
			rejected = append(rejected, c.Field)
		}
	}
	return filtered, rejected
}
