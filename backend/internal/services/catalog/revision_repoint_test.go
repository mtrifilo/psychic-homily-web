package catalog

import (
	"testing"

	"gorm.io/gorm"

	catalogm "psychic-homily-backend/internal/models/catalog"
)

// These cover the guards, which are the whole point of the helper: every one
// of them is a merge that would otherwise have written something wrong and
// committed it.
//
// No database. Each case must be rejected BEFORE any statement runs, and the
// zero-value *gorm.DB below makes that self-enforcing: a guard that stopped
// rejecting would reach Exec and panic, failing the test rather than quietly
// passing it.
func unusableTx() *gorm.DB { return &gorm.DB{} }

func TestRepointRevisions_RejectsUndecidedProvenance(t *testing.T) {
	_, err := repointRevisions(unusableTx(), revisionEntityShow, 1, 2, provenanceUndecided)
	if err == nil {
		t.Fatal("the zero provenance must be rejected — it is 'nobody decided', not 'no stamp'")
	}
}

func TestRepointRevisions_RejectsUnknownProvenance(t *testing.T) {
	_, err := repointRevisions(unusableTx(), revisionEntityShow, 1, 2, revisionProvenance(99))
	if err == nil {
		t.Fatal("a provenance outside the declared set must be rejected")
	}
}

// The stamp names a venue column and only the venue read gate honors it.
// Stamping an artist or a show would record a protection that does not exist.
func TestRepointRevisions_RejectsVenueStampOnOtherEntities(t *testing.T) {
	for _, entity := range []revisionEntityType{revisionEntityArtist, revisionEntityShow} {
		t.Run(string(entity), func(t *testing.T) {
			_, err := repointRevisions(unusableTx(), entity, 1, 2, stampFromUnverifiedVenue)
			if err == nil {
				t.Fatalf("stampFromUnverifiedVenue must be rejected for %q", entity)
			}
		})
	}
}

func TestRepointRevisions_RejectsUnknownEntityType(t *testing.T) {
	_, err := repointRevisions(unusableTx(), revisionEntityType("venues"), 1, 2, noRedactionCarryover)
	if err == nil {
		t.Fatal("a mistyped entity type must be rejected — it would match no rows and look like success")
	}
}

// A self-merge is a harmless no-op for the UPDATE and a real corruption for
// the stamp: it would mark the surviving venue's own history as carried off an
// unverified one.
func TestRepointRevisions_RejectsSelfMerge(t *testing.T) {
	_, err := repointRevisions(unusableTx(), revisionEntityVenue, 7, 7, stampFromUnverifiedVenue)
	if err == nil {
		t.Fatal("re-pointing an entity onto itself must be rejected")
	}
}

func TestRepointRevisions_RejectsZeroIDs(t *testing.T) {
	if _, err := repointRevisions(unusableTx(), revisionEntityVenue, 0, 2, noRedactionCarryover); err == nil {
		t.Fatal("a zero canonical id must be rejected")
	}
	if _, err := repointRevisions(unusableTx(), revisionEntityVenue, 1, 0, noRedactionCarryover); err == nil {
		t.Fatal("a zero merge-from id must be rejected")
	}
}

func TestRepointRevisions_RejectsNilTransaction(t *testing.T) {
	_, err := repointRevisions(nil, revisionEntityVenue, 1, 2, noRedactionCarryover)
	if err == nil {
		t.Fatal("a nil transaction must be rejected")
	}
}

// venueRevisionProvenance is the one branch that decides whether a real merge
// stamps. A verified loser must NOT clear an inherited mark, which is why the
// verified case is noRedactionCarryover rather than an "unstamp" value.
func TestVenueRevisionProvenance(t *testing.T) {
	unverified := &catalogm.Venue{ID: 1, Verified: false}
	if got := venueRevisionProvenance(unverified); got != stampFromUnverifiedVenue {
		t.Fatalf("unverified loser: got %s, want stampFromUnverifiedVenue", got)
	}

	verified := &catalogm.Venue{ID: 2, Verified: true}
	if got := venueRevisionProvenance(verified); got != noRedactionCarryover {
		t.Fatalf("verified loser: got %s, want noRedactionCarryover", got)
	}
}
