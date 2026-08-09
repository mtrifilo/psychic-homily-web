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

// revisionEntityType is the revisions.entity_type value a merge re-points.
//
// Named rather than spelled at each call site because the failure mode of a
// typo is silent: 'venues' matches no rows, the UPDATE succeeds, and the
// loser's history stays pointed at an entity that is deleted moments later.
type revisionEntityType string

const (
	revisionEntityVenue  revisionEntityType = "venue"
	revisionEntityArtist revisionEntityType = "artist"
	revisionEntityShow   revisionEntityType = "show"
)

// valid reports whether the type is one of the three the merges handle. Guards
// against a call site conjuring a string through the type conversion.
func (e revisionEntityType) valid() bool {
	switch e {
	case revisionEntityVenue, revisionEntityArtist, revisionEntityShow:
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

	// noRedactionCarryover states that the losing entity's revisions carry no
	// read-time redaction that the re-point would strip, so they move as they
	// are.
	//
	// True today for artists and shows: revision history for both is published
	// in full, so there is nothing for a stamp to preserve. It is NOT true by
	// default for anything that gains a gate later.
	//
	// The known one is shows. Shows are gated at the ENTITY level — an
	// anonymous caller 404s on a pending, rejected or private show — while
	// /revisions/show/{id} publishes its history regardless. Whatever closes
	// that gap needs a provenance stamp of its own, because MergeDuplicateShow
	// re-points a losing show's revisions and deletes the show the gate would
	// read. The call site to change is in MergeDuplicateShow, and it is
	// spelled out there.
	noRedactionCarryover
)

// String makes a rejected provenance readable in the error rather than
// printing an integer the reader has to count out against this file.
func (p revisionProvenance) String() string {
	switch p {
	case stampFromUnverifiedVenue:
		return "stampFromUnverifiedVenue"
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
	entity revisionEntityType,
	canonicalID, mergeFromID uint,
	provenance revisionProvenance,
) (int64, error) {
	if !entity.valid() {
		return 0, fmt.Errorf("repoint revisions: unknown entity type %q", string(entity))
	}
	if canonicalID == 0 || mergeFromID == 0 {
		return 0, fmt.Errorf("repoint revisions: canonical and merge-from ids are required")
	}
	// A self-merge is a no-op move but NOT a no-op stamp: it would mark the
	// surviving entity's own history as carried off an unverified venue.
	if canonicalID == mergeFromID {
		return 0, fmt.Errorf("repoint revisions: cannot re-point %s %d onto itself", entity, canonicalID)
	}

	// The provenance decision is the only thing that varies, and it varies by
	// one hardcoded fragment. Never caller input.
	setClause := "entity_id = ?"
	switch provenance {
	case stampFromUnverifiedVenue:
		if entity != revisionEntityVenue {
			return 0, fmt.Errorf(
				"repoint revisions: %s is venue-only, but entity type is %q",
				provenance, string(entity))
		}
		setClause += ", from_unverified_venue = TRUE"
	case noRedactionCarryover:
		// Nothing to preserve — see the constant.
	default:
		return 0, fmt.Errorf(
			"repoint revisions: %s is not a provenance decision; pass "+
				"stampFromUnverifiedVenue or noRedactionCarryover", provenance)
	}

	// #nosec G201 -- setClause is one of two literals chosen by the switch
	// above; the ids and the entity type are bound parameters.
	sql := fmt.Sprintf(
		"UPDATE revisions SET %s WHERE entity_type = ? AND entity_id = ?", setClause)
	r := tx.Exec(sql, canonicalID, string(entity), mergeFromID)
	if r.Error != nil {
		return 0, fmt.Errorf("failed to re-point %s revisions: %w", entity, r.Error)
	}
	return r.RowsAffected, nil
}
