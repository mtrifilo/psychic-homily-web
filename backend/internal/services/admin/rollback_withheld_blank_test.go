package admin

import (
	"context"
	"encoding/json"
	"testing"

	adminm "psychic-homily-backend/internal/models/admin"
	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/contracts"
)

// =============================================================================
// UNIT TESTS (No Database Required)
// =============================================================================

// Every entity type with a gate has to reach gatedFieldNames, or a recorded
// blank carrying no stamp goes on being read as an honest one and the rollback
// goes on emptying the column.
func TestGatedFieldNamesCoverEveryReporter(t *testing.T) {
	for entityType, newModel := range entityModelsByType {
		if _, gated := newModel().(withheldEditFieldsReporter); !gated {
			continue
		}
		if len(gatedFieldNames[entityType]) == 0 {
			t.Errorf("%s has a withholding gate but names no gated field", entityType)
		}
	}
}

// The per-TYPE list has to cover everything the per-ROW gate can actually
// withhold. A name the gate withholds but the list omits is a blank the rollback
// trusts, which is the defect this whole rule exists to stop.
//
// Venue is the one gated model, so its two columns are populated here to make
// the gate report them; a second gated model belongs beside it.
func TestGatedFieldNamesCoverEverythingTheGateWithholds(t *testing.T) {
	unverified := &catalogm.Venue{
		Address: stringPtr("1 Old St"),
		Zipcode: stringPtr("85003"),
	}
	withheld := unverified.WithheldEditFields()
	if len(withheld) == 0 {
		t.Fatal("fixture withholds nothing, so this test asserts nothing")
	}
	gated := gatedFieldNames[adminm.PendingEditEntityVenue]
	for _, name := range withheld {
		if !gated[name] {
			t.Errorf("venue withholds %q but the gated-name list omits it", name)
		}
	}
}

// The stamp is three-state, and the third state is the whole point: an
// unstamped change must not read as "observed", or every row written before the
// stamp existed is trusted to say its blank came off the column.
func TestOldValueWithheldStampStates(t *testing.T) {
	unstamped := adminm.FieldChange{Field: "address", OldValue: ""}
	if unstamped.OldValueIsWithheld() {
		t.Error("an unstamped change must not claim its value was withheld")
	}
	if !unstamped.OldValueUnstamped() {
		t.Error("an unstamped change must report itself unstamped")
	}

	observed := unstamped.WithOldValueWithheld(false)
	if observed.OldValueIsWithheld() || observed.OldValueUnstamped() {
		t.Error("a change stamped observed is stamped, and is not withheld")
	}
	if !unstamped.OldValueUnstamped() {
		t.Error("WithOldValueWithheld must not mutate its receiver")
	}

	withheld := unstamped.WithOldValueWithheld(true)
	if !withheld.OldValueIsWithheld() || withheld.OldValueUnstamped() {
		t.Error("a change stamped withheld is stamped, and is withheld")
	}
	if observed.OldValueWithheld == withheld.OldValueWithheld {
		t.Error("two stamps must not share one target")
	}
}

// =============================================================================
// INTEGRATION TESTS
// =============================================================================

// stampedChange builds one recorded change carrying the OldValueWithheld stamp,
// which a bare composite literal inside a slice cannot do.
func stampedChange(field string, oldValue, newValue interface{}, withheld bool) adminm.FieldChange {
	return adminm.FieldChange{Field: field, OldValue: oldValue, NewValue: newValue}.
		WithOldValueWithheld(withheld)
}

// withheldAddressRevision records an approved contributor address edit on an
// unverified venue the way the submit path records one: the previous value is
// the blank the contributor was served, stamped as such.
func (s *RevisionServiceIntegrationTestSuite) withheldAddressRevision(venueID, userID uint, newValue string) {
	changes := []adminm.FieldChange{
		stampedChange("address", "", newValue, true),
	}
	s.Require().NoError(s.svc.RecordRevision("venue", venueID, userID, changes, "corrected"))
	s.applyRecordedChanges("venue", venueID, changes)
}

func (s *RevisionServiceIntegrationTestSuite) venueAddress(venueID uint) *string {
	var venue catalogm.Venue
	s.Require().NoError(s.db.First(&venue, venueID).Error)
	return venue.Address
}

// THE DEFECT. An unverified venue's address is withheld from the contributor
// editing it, so the pending edit records "" as the previous value and the undo
// wrote that blank over a real street address.
//
// The value the field held is in the history: the revision that put it there.
// Restoring THAT is an undo; writing the blank is data loss dressed as one.
func (s *RevisionServiceIntegrationTestSuite) TestRollback_RestoresTheAddressHistoryRecordsRatherThanTheWithheldBlank() {
	admin := s.createTestUser()
	venue := s.createTestVenue("Somebodys House")
	s.Require().False(venue.Verified)

	// The edit that put the real address there, back when the column was empty
	// and the blank previous value was honest.
	first := []adminm.FieldChange{
		stampedChange("address", "", "1 Old St", false),
	}
	s.Require().NoError(s.svc.RecordRevision("venue", venue.ID, admin.ID, first, "added"))
	s.applyRecordedChanges("venue", venue.ID, first)

	s.withheldAddressRevision(venue.ID, admin.ID, secretAddress)
	s.Require().Equal(secretAddress, *s.venueAddress(venue.ID))

	revision := s.latestRevision("venue", venue.ID)
	result, err := s.svc.Rollback(context.Background(), revision.ID, admin.ID)
	s.Require().NoError(err)
	s.Equal([]string{"address"}, result.AppliedFields)
	s.Empty(result.SkippedFields)

	s.Require().NotNil(s.venueAddress(venue.ID))
	s.Equal("1 Old St", *s.venueAddress(venue.ID),
		"the undo must restore the address the history records, never the withheld blank")

	recorded := s.latestRevision("venue", venue.ID)
	var recordedChanges []adminm.FieldChange
	s.Require().NoError(json.Unmarshal(*recorded.FieldChanges, &recordedChanges))
	s.Require().Len(recordedChanges, 1)
	s.Equal(secretAddress, recordedChanges[0].OldValue, "history records what the column held")
	s.Equal("1 Old St", recordedChanges[0].NewValue,
		"history records the value this rollback WROTE, not the blank the revision carried")
	s.False(recordedChanges[0].OldValueIsWithheld(),
		"the observation reads a withheld column as the column, so its value is observed")
}

// With nothing in the history to restore, the field is refused rather than
// blanked. A revision recording ONE field has no siblings to carry, so the
// refusal is the whole rollback.
func (s *RevisionServiceIntegrationTestSuite) TestRollback_RefusesAWithheldBlankWithNoHistory() {
	admin := s.createTestUser()
	venue := s.createTestVenue("No History House")
	s.Require().NoError(s.db.Model(venue).Update("address", "1 Old St").Error)

	s.withheldAddressRevision(venue.ID, admin.ID, secretAddress)

	revision := s.latestRevision("venue", venue.ID)
	_, err := s.svc.Rollback(context.Background(), revision.ID, admin.ID)
	s.Require().Error(err)
	s.Contains(err.Error(), withheldBlankReason)

	s.Require().NotNil(s.venueAddress(venue.ID))
	s.Equal(secretAddress, *s.venueAddress(venue.ID),
		"a refused rollback writes nothing, least of all the blank it refused")
}

// The refusal is per FIELD, like every other rollback refusal: an address
// nothing can restore must not strand the undo of the fields recorded beside it.
func (s *RevisionServiceIntegrationTestSuite) TestRollback_RefusedWithheldBlankLeavesItsSiblingsRestorable() {
	admin := s.createTestUser()
	venue := s.createTestVenue("Mixed Revision House")
	s.Require().NoError(s.db.Model(venue).Updates(map[string]interface{}{
		"address": "1 Old St", "capacity": 120,
	}).Error)

	changes := []adminm.FieldChange{
		stampedChange("address", "", secretAddress, true),
		stampedChange("capacity", 120, 350, false),
	}
	s.Require().NoError(s.svc.RecordRevision("venue", venue.ID, admin.ID, changes, "corrected"))
	s.applyRecordedChanges("venue", venue.ID, changes)

	revision := s.latestRevision("venue", venue.ID)
	result, err := s.svc.Rollback(context.Background(), revision.ID, admin.ID)
	s.Require().NoError(err)
	s.Equal([]string{"capacity"}, result.AppliedFields)
	s.Require().Len(result.SkippedFields, 1)
	s.Equal("address", result.SkippedFields[0].Field)
	s.Equal(withheldBlankReason, result.SkippedFields[0].Reason)

	var restored catalogm.Venue
	s.Require().NoError(s.db.First(&restored, venue.ID).Error)
	s.Require().NotNil(restored.Capacity)
	s.Equal(120, *restored.Capacity)
	s.Require().NotNil(restored.Address)
	s.Equal(secretAddress, *restored.Address, "the refused field keeps the value it had")
}

// The ordinary undo of "a contributor filled in an empty address" still empties
// it. That blank IS the column's value, the stamp says so, and refusing it would
// leave a moderator unable to undo a bogus address on a house venue.
func (s *RevisionServiceIntegrationTestSuite) TestRollback_WritesABlankTheStampCallsObserved() {
	admin := s.createTestUser()
	venue := s.createTestVenue("Empty Address House")
	s.Require().Nil(s.venueAddress(venue.ID))

	changes := []adminm.FieldChange{
		stampedChange("address", "", secretAddress, false),
	}
	s.Require().NoError(s.svc.RecordRevision("venue", venue.ID, admin.ID, changes, "added"))
	s.applyRecordedChanges("venue", venue.ID, changes)

	revision := s.latestRevision("venue", venue.ID)
	result, err := s.svc.Rollback(context.Background(), revision.ID, admin.ID)
	s.Require().NoError(err)
	s.Equal([]string{"address"}, result.AppliedFields)

	address := s.venueAddress(venue.ID)
	s.Require().NotNil(address)
	s.Equal("", *address, "an observed blank is the column's own value and is restored")
}

// A row written before the stamp existed says nothing about its blank, and the
// history says everything: a revision that recorded writing a real value proves
// the column was not empty, so the blank beside it was the mask.
func (s *RevisionServiceIntegrationTestSuite) TestRollback_UnstampedBlankIsResolvedFromHistory() {
	admin := s.createTestUser()
	venue := s.createTestVenue("Legacy History House")

	first := []adminm.FieldChange{{Field: "address", OldValue: "", NewValue: "1 Old St"}}
	s.Require().NoError(s.svc.RecordRevision("venue", venue.ID, admin.ID, first, "added"))
	s.applyRecordedChanges("venue", venue.ID, first)

	second := []adminm.FieldChange{{Field: "address", OldValue: "", NewValue: secretAddress}}
	s.Require().NoError(s.svc.RecordRevision("venue", venue.ID, admin.ID, second, "corrected"))
	s.applyRecordedChanges("venue", venue.ID, second)

	revision := s.latestRevision("venue", venue.ID)
	result, err := s.svc.Rollback(context.Background(), revision.ID, admin.ID)
	s.Require().NoError(err)
	s.Equal([]string{"address"}, result.AppliedFields)

	s.Require().NotNil(s.venueAddress(venue.ID))
	s.Equal("1 Old St", *s.venueAddress(venue.ID))
}

// The one case nothing can answer: an unstamped blank with no history behind it.
// It is written, which is what the rollback did before any of this existed. The
// alternative is refusing every legacy undo of a genuinely-empty address, which
// is the more common of the two shapes it could be.
func (s *RevisionServiceIntegrationTestSuite) TestRollback_UnstampedBlankWithNoHistoryKeepsThePriorBehaviour() {
	admin := s.createTestUser()
	venue := s.createTestVenue("Legacy Blank House")
	s.Require().NoError(s.db.Model(venue).Update("address", "1 Old St").Error)

	changes := []adminm.FieldChange{{Field: "address", OldValue: "", NewValue: secretAddress}}
	s.Require().NoError(s.svc.RecordRevision("venue", venue.ID, admin.ID, changes, "corrected"))
	s.applyRecordedChanges("venue", venue.ID, changes)

	revision := s.latestRevision("venue", venue.ID)
	result, err := s.svc.Rollback(context.Background(), revision.ID, admin.ID)
	s.Require().NoError(err)
	s.Equal([]string{"address"}, result.AppliedFields)

	address := s.venueAddress(venue.ID)
	s.Require().NotNil(address)
	s.Equal("", *address)
}

// The history probe reads the entity's OWN revisions. A second venue's history
// must not answer for this one, and neither must a revision recorded after the
// one being undone.
func (s *RevisionServiceIntegrationTestSuite) TestRollback_HistoryProbeIsScopedToTheEntityAndToEarlierRevisions() {
	admin := s.createTestUser()
	neighbour := s.createTestVenue("Neighbour House")
	neighbourChanges := []adminm.FieldChange{{Field: "address", OldValue: "", NewValue: "9 Neighbour Ln"}}
	s.Require().NoError(s.svc.RecordRevision("venue", neighbour.ID, admin.ID, neighbourChanges, "added"))

	venue := s.createTestVenue("Scoped House")
	s.Require().NoError(s.db.Model(venue).Update("address", "1 Old St").Error)
	s.withheldAddressRevision(venue.ID, admin.ID, secretAddress)
	subject := s.latestRevision("venue", venue.ID)

	// Recorded AFTER the revision being undone, so it describes a later state
	// and cannot be the value that preceded it.
	later := []adminm.FieldChange{{Field: "address", OldValue: secretAddress, NewValue: "3 Later Rd"}}
	s.Require().NoError(s.svc.RecordRevision("venue", venue.ID, admin.ID, later, "later"))

	_, err := s.svc.Rollback(context.Background(), subject.ID, admin.ID)
	s.Require().Error(err, "neither another venue's history nor a later revision may answer")
	s.Contains(err.Error(), withheldBlankReason)
}

// =============================================================================
// SUBMIT PATH
// =============================================================================

// The stamp is the server's answer, not the submitter's. It arrives on the
// suggest-edit body inside this same struct, so a submitter can send one; the
// derivation overwrites it either way.
func (s *PendingEditServiceIntegrationTestSuite) TestCreatePendingEdit_StampsWhatTheServerDerived() {
	user := s.createTestUser()
	venue := s.createTestVenue("Stamped House")
	s.Require().NoError(s.db.Model(venue).Update("address", "1 Old St").Error)

	claimed := true
	resp, err := s.svc.CreatePendingEdit(&contracts.CreatePendingEditRequest{
		EntityType: "venue",
		EntityID:   venue.ID,
		UserID:     user.ID,
		Changes: []adminm.FieldChange{
			{Field: "address", OldValue: nil, NewValue: secretAddress},
			// The submitter claims the name was withheld from them. It was not.
			{Field: "name", OldValue: "Stamped House", NewValue: "The Basement", OldValueWithheld: &claimed},
		},
		Summary: "they moved",
	})
	s.Require().NoError(err)

	stored := s.storedChanges(resp.ID)
	s.True(stored["address"].OldValueIsWithheld(),
		"the address of an unverified venue is withheld, and the row has to say so")
	s.Equal("", stored["address"].OldValue)
	s.False(stored["name"].OldValueIsWithheld(),
		"a submitter's claim about the stamp must not survive the derivation")
	s.False(stored["name"].OldValueUnstamped())
}

// A verified venue publishes its address, so the derived previous value is the
// column and the stamp says observed. This is the pairing that makes the stamp
// a fact about THIS row rather than about the field name.
func (s *PendingEditServiceIntegrationTestSuite) TestCreatePendingEdit_VerifiedVenueStampsTheAddressObserved() {
	user := s.createTestUser()
	venue := s.createTestVenue("Published House")
	s.Require().NoError(s.db.Model(venue).Updates(map[string]interface{}{
		"address": "1 Old St", "verified": true,
	}).Error)

	resp, err := s.svc.CreatePendingEdit(&contracts.CreatePendingEditRequest{
		EntityType: "venue",
		EntityID:   venue.ID,
		UserID:     user.ID,
		Changes:    makeChanges("address", "1 Old St", secretAddress),
		Summary:    "they moved",
	})
	s.Require().NoError(err)

	stored := s.storedChanges(resp.ID)
	s.False(stored["address"].OldValueIsWithheld())
	s.Equal("1 Old St", stored["address"].OldValue)
}

// SUBMIT, APPROVE, ROLL BACK, the whole journey the ticket describes, through
// the real services rather than a hand-written revision row.
func (s *PendingEditServiceIntegrationTestSuite) TestSubmitApproveRollback_LeavesTheRealAddressInPlace() {
	first := s.createTestUser()
	second := s.createTestUser()
	reviewer := s.createTestUser()
	venue := s.createTestVenue("Journey House")
	s.Require().False(venue.Verified)

	// The address arrives through the pipeline, so history records it.
	added, err := s.svc.CreatePendingEdit(&contracts.CreatePendingEditRequest{
		EntityType: "venue", EntityID: venue.ID, UserID: first.ID,
		Changes: makeChanges("address", "", "1 Old St"), Summary: "adding the address",
	})
	s.Require().NoError(err)
	_, err = s.svc.ApprovePendingEdit(context.Background(), added.ID, reviewer.ID)
	s.Require().NoError(err)

	// A second contributor edits it. The address is withheld from them now, so
	// the recorded previous value is the blank.
	corrected, err := s.svc.CreatePendingEdit(&contracts.CreatePendingEditRequest{
		EntityType: "venue", EntityID: venue.ID, UserID: second.ID,
		Changes: makeChanges("address", "", secretAddress), Summary: "correcting the address",
	})
	s.Require().NoError(err)
	s.True(s.storedChanges(corrected.ID)["address"].OldValueIsWithheld())
	_, err = s.svc.ApprovePendingEdit(context.Background(), corrected.ID, reviewer.ID)
	s.Require().NoError(err)

	var applied catalogm.Venue
	s.Require().NoError(s.db.First(&applied, venue.ID).Error)
	s.Require().NotNil(applied.Address)
	s.Require().Equal(secretAddress, *applied.Address)

	var revision adminm.Revision
	s.Require().NoError(s.db.Where("entity_type = ? AND entity_id = ?", "venue", venue.ID).
		Order("id DESC").First(&revision).Error)

	result, err := s.revisionSvc.Rollback(context.Background(), revision.ID, reviewer.ID)
	s.Require().NoError(err)
	s.Equal([]string{"address"}, result.AppliedFields)

	var restored catalogm.Venue
	s.Require().NoError(s.db.First(&restored, venue.ID).Error)
	s.Require().NotNil(restored.Address)
	s.Equal("1 Old St", *restored.Address, "the undo restores the address, it does not erase it")
}
