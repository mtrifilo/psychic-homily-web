package admin

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	adminm "psychic-homily-backend/internal/models/admin"
	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/contracts"
)

// =============================================================================
// UNIT TESTS (No Database Required)
// =============================================================================

// The stamp is three-state, and the third state is why it is a pointer: a
// change that records nothing must not read as one that recorded "observed",
// because the two cannot be told apart again once a row is written.
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

// The refusal keys on the STAMP, not on the value's shape. A mask is whatever
// the unset value of the column's type renders as, so a gated int column would
// be masked as 0 and a gated timestamp as year 1; keying on blankness would
// write both of those into the column.
func TestRefuseWithheldOldValues(t *testing.T) {
	fieldOrder := []string{"address", "capacity", "founded_year", "name"}
	byField := map[string]adminm.FieldChange{
		"address":      stampedChange("address", "", "1234 Secret St", true),
		"capacity":     stampedChange("capacity", 0, 350, true),
		"founded_year": stampedChange("founded_year", "", 1985, false),
		"name":         {Field: "name", OldValue: "", NewValue: "The Basement"},
	}

	refusals := refuseWithheldOldValues(fieldOrder, byField)
	if _, refused := refusals["address"]; !refused {
		t.Error("a stamped withheld value must be refused")
	}
	if _, refused := refusals["capacity"]; !refused {
		t.Error("a withheld mask that is not blank must be refused too")
	}
	if _, refused := refusals["founded_year"]; refused {
		t.Error("a blank the stamp calls observed is the column's own value")
	}
	if _, refused := refusals["name"]; refused {
		t.Error("an unstamped change records nothing, so nothing here refuses it")
	}
	if got := refusals["address"]; got != withheldOldValueReason {
		t.Errorf("refusal reason = %q, want the withheld reason", got)
	}
}

// The stamp is storage. Serving it publishes the bit the withholding exists to
// refuse: a field is stamped withheld only when its column is set, so `true`
// tells the reader the venue has a street address on record.
func TestForServingDropsTheStamp(t *testing.T) {
	in := []adminm.FieldChange{
		stampedChange("address", "", "1234 Secret St", true),
		stampedChange("name", "Old Room", "The Basement", false),
	}

	out := adminm.ForServing(in)
	for _, c := range out {
		if !c.OldValueUnstamped() {
			t.Errorf("%s: the served payload must carry no stamp", c.Field)
		}
	}
	if in[0].OldValueUnstamped() {
		t.Error("input mutated: the stored row's stamp is what rollback reads")
	}

	encoded, err := json.Marshal(out[0])
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if body := string(encoded); strings.Contains(body, "old_value_withheld") {
		t.Errorf("the served JSON must not name the stamp: %s", body)
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
// the placeholder the contributor was served, stamped as such.
func (s *RevisionServiceIntegrationTestSuite) withheldAddressRevision(venueID, userID uint, newValue string) {
	changes := []adminm.FieldChange{stampedChange("address", "", newValue, true)}
	s.Require().NoError(s.svc.RecordRevision("venue", venueID, userID, changes, "corrected"))
	s.applyRecordedChanges("venue", venueID, changes)
}

func (s *RevisionServiceIntegrationTestSuite) venueAddress(venueID uint) *string {
	var venue catalogm.Venue
	s.Require().NoError(s.db.First(&venue, venueID).Error)
	return venue.Address
}

// THE INVARIANT. An unverified venue's address is withheld from the contributor
// editing it, so the pending edit records "" as the previous value. Writing that
// back on undo empties a column holding a real address, so the field is refused
// instead. A revision recording ONE field has no siblings to carry, so the
// refusal is the whole rollback.
func (s *RevisionServiceIntegrationTestSuite) TestRollback_RefusesAWithheldAddress() {
	admin := s.createTestUser()
	venue := s.createTestVenue("Somebodys House")
	s.Require().False(venue.Verified)
	s.Require().NoError(s.db.Model(venue).Update("address", "1 Old St").Error)

	s.withheldAddressRevision(venue.ID, admin.ID, secretAddress)

	revision := s.latestRevision("venue", venue.ID)
	_, err := s.svc.Rollback(context.Background(), revision.ID, admin.ID)
	s.Require().Error(err)
	s.Contains(err.Error(), withheldOldValueReason)

	s.Require().NotNil(s.venueAddress(venue.ID))
	s.Equal(secretAddress, *s.venueAddress(venue.ID),
		"a refused rollback writes nothing, least of all the placeholder it refused")
}

// The refusal is per FIELD, like every other rollback refusal: an address
// nothing can restore must not strand the undo of the fields recorded beside it.
func (s *RevisionServiceIntegrationTestSuite) TestRollback_RefusedWithheldAddressLeavesItsSiblingsRestorable() {
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
	s.Equal(withheldOldValueReason, result.SkippedFields[0].Reason)

	var restored catalogm.Venue
	s.Require().NoError(s.db.First(&restored, venue.ID).Error)
	s.Require().NotNil(restored.Capacity)
	s.Equal(120, *restored.Capacity)
	s.Require().NotNil(restored.Address)
	s.Equal(secretAddress, *restored.Address, "the refused field keeps the value it had")

	recorded := s.latestRevision("venue", venue.ID)
	var recordedChanges []adminm.FieldChange
	s.Require().NoError(json.Unmarshal(*recorded.FieldChanges, &recordedChanges))
	s.Require().Len(recordedChanges, 1)
	s.Equal("capacity", recordedChanges[0].Field,
		"history records what was restored and nothing else")
}

// The ordinary undo of "a contributor filled in an empty address" still empties
// it. That blank IS the column's value, the stamp says so, and refusing it would
// leave a moderator unable to undo a bogus address on a house venue.
func (s *RevisionServiceIntegrationTestSuite) TestRollback_WritesABlankTheStampCallsObserved() {
	admin := s.createTestUser()
	venue := s.createTestVenue("Empty Address House")
	s.Require().Nil(s.venueAddress(venue.ID))

	changes := []adminm.FieldChange{stampedChange("address", "", secretAddress, false)}
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

// A row carrying no stamp records nothing about where its blank came from, and
// keeps the behaviour it has: the blank is written. Refusing it would refuse the
// undo of every genuinely empty field recorded before the stamp existed, and
// nothing available here can tell the two apart.
func (s *RevisionServiceIntegrationTestSuite) TestRollback_WritesAnUnstampedBlank() {
	admin := s.createTestUser()
	venue := s.createTestVenue("Unstamped House")
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

	// The response goes back to the submitter, so it carries no stamp at all:
	// `true` there would say the venue has an address on record, which is the
	// bit the withholding refuses.
	for _, c := range resp.FieldChanges {
		s.True(c.OldValueUnstamped(), "%s: the served payload must carry no stamp", c.Field)
	}
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
	contributor := s.createTestUser()
	reviewer := s.createTestUser()
	venue := s.createTestVenue("Journey House")
	s.Require().False(venue.Verified)
	s.Require().NoError(s.db.Model(venue).Update("address", "1 Old St").Error)

	edit, err := s.svc.CreatePendingEdit(&contracts.CreatePendingEditRequest{
		EntityType: "venue", EntityID: venue.ID, UserID: contributor.ID,
		Changes: makeChanges("address", "", secretAddress), Summary: "correcting the address",
	})
	s.Require().NoError(err)
	s.True(s.storedChanges(edit.ID)["address"].OldValueIsWithheld())

	_, err = s.svc.ApprovePendingEdit(context.Background(), edit.ID, reviewer.ID)
	s.Require().NoError(err)

	var applied catalogm.Venue
	s.Require().NoError(s.db.First(&applied, venue.ID).Error)
	s.Require().NotNil(applied.Address)
	s.Require().Equal(secretAddress, *applied.Address)

	var revision adminm.Revision
	s.Require().NoError(s.db.Where("entity_type = ? AND entity_id = ?", "venue", venue.ID).
		Order("id DESC").First(&revision).Error)
	stored := s.revisionChanges(&revision)
	s.True(stored["address"].OldValueIsWithheld(),
		"approve copies the stored change verbatim, so the stamp reaches history")

	_, err = s.revisionSvc.Rollback(context.Background(), revision.ID, reviewer.ID)
	s.Require().Error(err, "the recorded previous value is a placeholder, so there is nothing to restore")
	s.Contains(err.Error(), withheldOldValueReason)

	var afterRollback catalogm.Venue
	s.Require().NoError(s.db.First(&afterRollback, venue.ID).Error)
	s.Require().NotNil(afterRollback.Address)
	s.Equal(secretAddress, *afterRollback.Address,
		"the undo writes nothing, so the address the venue holds survives it")
}

func (s *PendingEditServiceIntegrationTestSuite) revisionChanges(r *adminm.Revision) map[string]adminm.FieldChange {
	var changes []adminm.FieldChange
	s.Require().NoError(json.Unmarshal(*r.FieldChanges, &changes))
	out := make(map[string]adminm.FieldChange, len(changes))
	for _, c := range changes {
		out[c.Field] = c
	}
	return out
}
