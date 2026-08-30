package catalog

import (
	"fmt"

	"gorm.io/gorm"
)

// This file exists so that "a merge re-points revisions" cannot be written
// without answering "and what happens to the redaction those rows were
// carrying".
//
// Revision privacy is decided at READ time from the entity the revision points
// at (see the revisiondiff package doc). A merge breaks that: it moves the
// loser's revisions onto the winner and then deletes the row the gate would
// have read. The venue merge closes it with a provenance stamp. Nothing
// structural stopped the next merge path from omitting the same stamp, and
// there are two others: MergeArtists, and MergeDuplicateShow, which runs from
// cmd/dedup-shows with no admin in the loop.
//
// repointRevisions takes the decision as a REQUIRED parameter, so a merge
// cannot be routed through it without its author picking one of the values
// below. What keeps a merge from bypassing it altogether is
// TestNoRevisionRepointOutsideTheHelper, which fails on an entity_id write
// against revisions anywhere else in the backend.

// polymorphicEntityType is the value stored in a polymorphic reference table's
// entity_type column. One vocabulary is shared by every operation that has to
// find an entity's references — the merges that re-point them and the deletes
// that sweep them (PSY-1868) — instead of two lists that can drift.
//
// Named rather than spelled at each call site because the failure mode of a
// typo is silent: 'venues' matches no rows, the statement succeeds, and the
// rows stay pointed at an entity that is deleted moments later.
type polymorphicEntityType string

const (
	entityTypeVenue    polymorphicEntityType = "venue"
	entityTypeArtist   polymorphicEntityType = "artist"
	entityTypeShow     polymorphicEntityType = "show"
	entityTypeRelease  polymorphicEntityType = "release"
	entityTypeLabel    polymorphicEntityType = "label"
	entityTypeFestival polymorphicEntityType = "festival"
)

// allPolymorphicEntityTypes is every catalog entity that can appear in an
// entity_type column, in the order a reader would list them.
//
// TestEveryEntityTypeHasADeletePath pins these six BY NAME rather than by
// iterating this slice, so adding a seventh fails a test instead of passing
// vacuously through a loop over the very list that was just extended.
var allPolymorphicEntityTypes = []polymorphicEntityType{
	entityTypeVenue, entityTypeArtist, entityTypeShow,
	entityTypeRelease, entityTypeLabel, entityTypeFestival,
}

// valid reports whether the type is one of the six the catalog writes into an
// entity_type column. Guards against a call site conjuring a string through the
// type conversion.
func (e polymorphicEntityType) valid() bool {
	for _, known := range allPolymorphicEntityTypes {
		if e == known {
			return true
		}
	}
	return false
}

// mergeable reports whether a MERGE exists for this entity type, which is a
// strictly narrower question than valid().
//
// Three of the six can be merged; the other three have a delete path and no
// merge. The re-point helpers gate on this rather than on valid() so that
// widening the vocabulary for the delete sweep did not quietly widen the fence
// that keeps a merge from being invoked for an entity type no merge supports —
// where every re-point would match zero rows and report success.
func (e polymorphicEntityType) mergeable() bool {
	switch e {
	case entityTypeVenue, entityTypeArtist, entityTypeShow:
		return true
	default:
		return false
	}
}

// revisionProvenance is the decision a merge makes about the redaction posture
// of the revisions it carries off the losing entity.
//
// There is no default. The zero value is rejected at runtime, so a caller that
// passes an uninitialized variable fails loudly on the first merge rather than
// silently publishing history that was being withheld.
type revisionProvenance int

const (
	// provenanceUndecided is the zero value. It is never a legal argument —
	// its only job is to make "I forgot to decide" a rejected call rather than
	// an accidental no-stamp.
	provenanceUndecided revisionProvenance = iota

	// stampFromUnverifiedVenue marks the loser's rows with
	// revisions.from_unverified_venue before re-pointing them, so the
	// read-time address masking survives a merge into a verified venue.
	//
	// VENUE ONLY. The column and the gate that reads it are venue-scoped
	// (admin.applyPrivacyRedaction skips every other entity_type), so stamping
	// an artist's or a show's revisions would record a claim nothing honors
	// and imply a protection that does not exist. repointRevisions rejects it.
	//
	// The mark only ever goes TRUE. A chain of merges therefore cannot launder
	// an address that a single merge withholds.
	stampFromUnverifiedVenue

	// stampFromGatedShow marks the loser's rows with revisions.from_gated_show
	// before re-pointing them, so the read-time ENTITY suppression survives a
	// merge into an approved show.
	//
	// SHOW ONLY, for the same reason its venue counterpart is venue-only: the
	// column and the gate that reads it are show-scoped
	// (admin.revisionVisibleTo returns early for every other entity_type), so
	// stamping anything else would record a claim nothing honors.
	// repointRevisions rejects it.
	//
	// The mark only ever goes TRUE and nothing clears it, so a chain of merges
	// cannot launder a private show's history through an approved one. That is
	// also why the call site only asks for it when the WINNER is approved: a
	// stamp applied to rows landing on a gated winner would outlive the reason
	// for it and survive that show being published. See reassignShowRevisions.
	//
	// It is coarser than the live gate it preserves. GET /shows/{id} serves a
	// gated show to its own submitter; a stamped row is served to admins only,
	// because the show whose submitted_by would answer the question was deleted
	// by the merge. Losing an author's access to their own merged-away show's
	// history is the price of a marker that cannot be laundered.
	stampFromGatedShow

	// noRedactionCarryover states that the losing entity's revisions carry no
	// read-time redaction that the re-point would strip, so they move as they
	// are.
	//
	// True today for artists, whose revision history is published in full. It
	// is NOT true by default for anything that gains a gate later, and it is no
	// longer true for shows: see stampFromGatedShow and the call site in
	// MergeDuplicateShow, which decides per merge.
	noRedactionCarryover
)

// String makes a rejected provenance readable in the error rather than
// printing an integer the reader has to count out against this file.
func (p revisionProvenance) String() string {
	switch p {
	case stampFromUnverifiedVenue:
		return "stampFromUnverifiedVenue"
	case stampFromGatedShow:
		return "stampFromGatedShow"
	case noRedactionCarryover:
		return "noRedactionCarryover"
	case provenanceUndecided:
		return "provenanceUndecided"
	default:
		return fmt.Sprintf("revisionProvenance(%d)", int(p))
	}
}

// repointRevisions moves the losing entity's revision rows onto the canonical
// entity, carrying out the caller's provenance decision in the SAME statement.
//
// One UPDATE rather than a stamp followed by a move, because the stamp has to
// be applied to rows selected by the entity_id the move overwrites, and the
// losing entity is usually deleted moments later — a stamp that runs second
// has nothing left to identify. Two statements at a call site can be reordered
// by an edit that looks harmless; the set clause of one UPDATE cannot.
//
// Must run inside the merge's transaction, which is where the rest of the
// merge's atomicity comes from.
//
// Returns the number of revisions re-pointed, for the caller's merge summary.
func repointRevisions(
	tx *gorm.DB,
	entity polymorphicEntityType,
	canonicalID, mergeFromID uint,
	provenance revisionProvenance,
) (int64, error) {
	if !entity.mergeable() {
		return 0, fmt.Errorf("repoint revisions: %q has no merge path", string(entity))
	}
	if canonicalID == 0 || mergeFromID == 0 {
		return 0, fmt.Errorf("repoint revisions: canonical and merge-from ids are required")
	}
	// A self-merge is a no-op move but NOT a no-op stamp: it would mark the
	// surviving entity's own history as carried off an unverified venue or a
	// gated show.
	if canonicalID == mergeFromID {
		return 0, fmt.Errorf("repoint revisions: cannot re-point %s %d onto itself", entity, canonicalID)
	}

	// The provenance decision is the only thing that varies, and each stamping
	// decision appends one hardcoded fragment. Never caller input.
	setClause := "entity_id = ?"
	switch provenance {
	case stampFromUnverifiedVenue:
		if entity != entityTypeVenue {
			return 0, fmt.Errorf(
				"repoint revisions: %s is venue-only, but entity type is %q",
				provenance, string(entity))
		}
		setClause += ", from_unverified_venue = TRUE"
	case stampFromGatedShow:
		if entity != entityTypeShow {
			return 0, fmt.Errorf(
				"repoint revisions: %s is show-only, but entity type is %q",
				provenance, string(entity))
		}
		setClause += ", from_gated_show = TRUE"
	case noRedactionCarryover:
		// Nothing to preserve — see the constant.
	default:
		return 0, fmt.Errorf(
			"repoint revisions: %s is not a provenance decision; pass "+
				"stampFromUnverifiedVenue, stampFromGatedShow or "+
				"noRedactionCarryover", provenance)
	}

	// #nosec G201 -- setClause is assembled only from the fixed literals in the
	// switch above, one per provenance decision; the ids and the entity type are
	// bound parameters. Worded so that adding a decision does not falsify it.
	sql := fmt.Sprintf(
		"UPDATE revisions SET %s WHERE entity_type = ? AND entity_id = ?", setClause)
	r := tx.Exec(sql, canonicalID, string(entity), mergeFromID)
	if r.Error != nil {
		return 0, fmt.Errorf("failed to re-point %s revisions: %w", entity, r.Error)
	}
	return r.RowsAffected, nil
}
