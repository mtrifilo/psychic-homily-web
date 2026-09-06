package admin

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/utils/tests"

	adminm "psychic-homily-backend/internal/models/admin"
	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/shared/revisiondiff"
)

// =============================================================================
// UNIT TESTS (No Database Required)
// =============================================================================

// Every field a revision can record must be one the rollback derivation can
// observe, for every entity type that records revisions.
//
// This is the drift guard for the map the rollback path reads through. A field
// on a revisiondiff list whose column this file cannot resolve does not fail
// visibly: it fails the whole rollback of any revision that names it, with the
// fail-closed refusal resolveFieldValues raises for an unknown column. An entity
// type with no model there fails EVERY rollback of that type, which is how shows
// would go if the map were still the pending-edit map.
func TestRevisionFieldsAreObservable(t *testing.T) {
	db, err := gorm.Open(tests.DummyDialector{}, &gorm.Config{})
	if err != nil {
		t.Fatalf("open dummy connection: %v", err)
	}
	for _, entityType := range revisiondiff.EntityTypes() {
		newModel, ok := entityModels[entityType]
		if !ok {
			t.Errorf("%s records revisions but has no model in entityModels, so no rollback of one can observe the entity", entityType)
			continue
		}
		columns, err := modelColumns(db, newModel())
		if err != nil {
			t.Fatalf("%s: %v", entityType, err)
		}
		fields, ok := revisiondiff.FieldsFor(entityType)
		if !ok {
			t.Fatalf("no revision field list for %s", entityType)
		}
		for _, field := range fields {
			value, present := columns[field.Name]
			if !present {
				t.Errorf("%s: revision field %q is not a column on the model", entityType, field.Name)
				continue
			}
			if _, err := revisiondiff.EmitValue(value); err != nil {
				t.Errorf("%s.%s: %v", entityType, field.Name, err)
			}
		}
	}
}

// The rollback derivation reads the entity it is about to overwrite, so it must
// take the row FOR UPDATE. Nothing at the read itself says whether a lock was
// attached, since the lock is a clause on the handle, so this reads the emitted
// SQL. The property the lock holds is asserted by the integration tests.
func TestObserveCurrentValuesLocksTheRow(t *testing.T) {
	db, err := gorm.Open(tests.DummyDialector{}, &gorm.Config{DryRun: true})
	if err != nil {
		t.Fatalf("open dummy connection: %v", err)
	}
	var queries []string
	if err := db.Callback().Query().After("gorm:query").Register("capture_rollback_sql", func(tx *gorm.DB) {
		queries = append(queries, tx.Statement.SQL.String())
	}); err != nil {
		t.Fatalf("register callback: %v", err)
	}

	byField := map[string]adminm.FieldChange{
		"title": {Field: "title", OldValue: "Old", NewValue: "New"},
	}
	if _, err := observeRollbackValues(db, "show", 7, []string{"title"}, byField, nil); err != nil {
		t.Fatalf("observeRollbackValues: %v", err)
	}
	if len(queries) != 1 {
		t.Fatalf("issued %d queries, want 1: %q", len(queries), queries)
	}
	if !strings.Contains(queries[0], "FOR UPDATE") {
		t.Errorf("the rollback read is not locked: %q", queries[0])
	}
}

// A revision's claim about a timestamp is an INSTANT, not the text of one offset
// of it. Both sides render through EmitValue, which writes RFC3339 carrying
// whatever location the value was in, so a claim recorded from a UTC value and a
// column read back in the connection's local zone are the same field.
func TestSameFieldValue_TimestampsCompareAsInstants(t *testing.T) {
	utc := "2026-09-20T20:15:54Z"
	offset := "2026-09-20T13:15:54-07:00"
	if !sameFieldValue(utc, offset) {
		t.Errorf("the same instant in two offsets must compare equal: %q vs %q", utc, offset)
	}
	if sameFieldValue(utc, "2026-09-20T20:15:55Z") {
		t.Error("a different instant must not compare equal")
	}
	if sameFieldValue("not a time", utc) {
		t.Error("a non-timestamp string must not compare equal to a timestamp")
	}
}

// =============================================================================
// INTEGRATION TESTS
// =============================================================================

// The rollback revision's old_value is what the ENTITY held, not what the
// revision being undone claimed to have written. The two agree for a field
// nothing touched, and the point of recording the observed one is the rows where
// they do not: a legacy row's claim was never checked against anything.
func (s *RevisionServiceIntegrationTestSuite) TestRollback_RecordsTheObservedOldValue() {
	admin := s.createTestUser()
	artist := s.createTestArtist(
		fmt.Sprintf("Observed Value %d", time.Now().UnixNano()), "Phoenix", "AZ", "")

	changes := []adminm.FieldChange{
		{Field: "description", OldValue: "old blurb", NewValue: "new blurb"},
	}
	s.Require().NoError(s.svc.RecordRevision("artist", artist.ID, admin.ID, changes, "one field"))
	s.applyRecordedChanges("artist", artist.ID, changes)
	revision := s.latestRevision("artist", artist.ID)

	result, err := s.svc.Rollback(context.Background(), revision.ID, admin.ID)
	s.Require().NoError(err)
	s.Equal([]string{"description"}, result.AppliedFields)

	recorded := s.latestRevision("artist", artist.ID)
	s.Require().NotEqual(revision.ID, recorded.ID)
	var recordedChanges []adminm.FieldChange
	s.Require().NoError(json.Unmarshal(*recorded.FieldChanges, &recordedChanges))
	s.Require().Len(recordedChanges, 1)
	s.Equal("new blurb", recordedChanges[0].OldValue,
		"old_value is the value read off the entity under the lock")
	s.Equal("old blurb", recordedChanges[0].NewValue)
}

// Where the claim and the observation are the SAME state spelled two ways, the
// spelling that gets recorded is the entity's. null and "" are one claim about
// an empty field, so the field restores; what history then says the entity held
// is "", because that is what was read off it.
func (s *RevisionServiceIntegrationTestSuite) TestRollback_RecordsTheEntitySpellingNotTheClaims() {
	admin := s.createTestUser()
	artist := s.createTestArtist(
		fmt.Sprintf("Two Spellings %d", time.Now().UnixNano()), "Phoenix", "AZ", "")

	// Written directly: the shipping clients spell an empty field "" here, and
	// null is the spelling a stored row can still carry.
	raw := json.RawMessage(`[{"field":"description","old_value":"restored blurb","new_value":null}]`)
	revision := &adminm.Revision{
		EntityType: "artist", EntityID: artist.ID, UserID: admin.ID, FieldChanges: &raw,
	}
	s.Require().NoError(s.db.Create(revision).Error)

	result, err := s.svc.Rollback(context.Background(), revision.ID, admin.ID)
	s.Require().NoError(err)
	s.Equal([]string{"description"}, result.AppliedFields)

	recorded := s.latestRevision("artist", artist.ID)
	var recordedChanges []adminm.FieldChange
	s.Require().NoError(json.Unmarshal(*recorded.FieldChanges, &recordedChanges))
	s.Require().Len(recordedChanges, 1)
	s.Equal("", recordedChanges[0].OldValue,
		"old_value is the value read off the entity, not the claim that matched it")
	s.NotNil(recordedChanges[0].OldValue)
}

// A field something else has already changed is skipped, not overwritten, and
// its honest siblings still restore. This is the whole ticket: without it the
// rollback discards the later change and records an old_value the entity never
// held, which the next rollback then restores.
func (s *RevisionServiceIntegrationTestSuite) TestRollback_SkipsAFieldChangedSinceTheRevision() {
	admin := s.createTestUser()
	artist := s.createTestArtist(
		fmt.Sprintf("Changed Since %d", time.Now().UnixNano()), "Phoenix", "AZ", "")

	changes := []adminm.FieldChange{
		{Field: "description", OldValue: "old blurb", NewValue: "new blurb"},
		{Field: "city", OldValue: "Phoenix", NewValue: "Tucson"},
	}
	s.Require().NoError(s.svc.RecordRevision("artist", artist.ID, admin.ID, changes, "two fields"))
	s.applyRecordedChanges("artist", artist.ID, changes)

	// A second, later edit moves ONE of the two fields.
	s.Require().NoError(s.db.Table("artists").Where("id = ?", artist.ID).
		Update("description", "a later editor's blurb").Error)

	revision := s.latestRevision("artist", artist.ID)
	result, err := s.svc.Rollback(context.Background(), revision.ID, admin.ID)
	s.Require().NoError(err)

	s.Equal([]string{"city"}, result.AppliedFields)
	s.Require().Len(result.SkippedFields, 1)
	s.Equal("description", result.SkippedFields[0].Field)
	s.Contains(result.SkippedFields[0].Reason, "changed after the revision was recorded")

	var stored map[string]interface{}
	s.Require().NoError(s.db.Table("artists").Where("id = ?", artist.ID).Take(&stored).Error)
	s.Equal("a later editor's blurb", stored["description"], "the later change survives the undo")
	s.Equal("Phoenix", stored["city"], "the untouched field restores")

	recorded := s.latestRevision("artist", artist.ID)
	s.Require().NotNil(recorded.Summary)
	s.Contains(*recorded.Summary, "skipped: description")
	var recordedChanges []adminm.FieldChange
	s.Require().NoError(json.Unmarshal(*recorded.FieldChanges, &recordedChanges))
	s.Require().Len(recordedChanges, 1)
	s.Equal("city", recordedChanges[0].Field)
	s.Equal("Tucson", recordedChanges[0].OldValue)
}

// Every field moved on, so there is nothing to undo. That is the existing
// refusal, message and all: a caller must never be told an undo happened.
func (s *RevisionServiceIntegrationTestSuite) TestRollback_RefusesWhenEveryFieldChangedSince() {
	admin := s.createTestUser()
	artist := s.createTestArtist(
		fmt.Sprintf("All Changed %d", time.Now().UnixNano()), "Phoenix", "AZ", "")

	changes := []adminm.FieldChange{
		{Field: "description", OldValue: "old blurb", NewValue: "new blurb"},
		{Field: "city", OldValue: "Phoenix", NewValue: "Tucson"},
	}
	s.Require().NoError(s.svc.RecordRevision("artist", artist.ID, admin.ID, changes, "two fields"))
	s.Require().NoError(s.db.Table("artists").Where("id = ?", artist.ID).Updates(map[string]interface{}{
		"description": "somebody else entirely",
		"city":        "Flagstaff",
	}).Error)

	revision := s.latestRevision("artist", artist.ID)
	result, err := s.svc.Rollback(context.Background(), revision.ID, admin.ID)
	s.Require().Error(err)
	s.Nil(result)
	s.Contains(err.Error(), "no field of this revision can be restored")
	s.Contains(err.Error(), "description")
	s.Contains(err.Error(), "city")

	var stored map[string]interface{}
	s.Require().NoError(s.db.Table("artists").Where("id = ?", artist.ID).Take(&stored).Error)
	s.Equal("somebody else entirely", stored["description"], "a refused rollback writes nothing")
	s.Equal("Flagstaff", stored["city"])

	var count int64
	s.Require().NoError(s.db.Model(&adminm.Revision{}).
		Where("entity_type = ? AND entity_id = ?", "artist", artist.ID).Count(&count).Error)
	s.EqualValues(1, count, "a refused rollback records no revision")
}

// Undo the undo. The rollback revision records what the entity held, so rolling
// THAT back puts the entity where the first rollback found it — which is the
// property a recorded old_value nobody observed silently breaks.
func (s *RevisionServiceIntegrationTestSuite) TestRollback_OfARollbackRestoresTheValueItReplaced() {
	admin := s.createTestUser()
	artist := s.createTestArtist(
		fmt.Sprintf("Round Trip %d", time.Now().UnixNano()), "Phoenix", "AZ", "")

	changes := []adminm.FieldChange{
		{Field: "description", OldValue: "old blurb", NewValue: "new blurb"},
	}
	s.Require().NoError(s.svc.RecordRevision("artist", artist.ID, admin.ID, changes, "one field"))
	s.applyRecordedChanges("artist", artist.ID, changes)
	original := s.latestRevision("artist", artist.ID)

	_, err := s.svc.Rollback(context.Background(), original.ID, admin.ID)
	s.Require().NoError(err)
	s.Equal("old blurb", s.artistColumn(artist.ID, "description"))

	rollbackRevision := s.latestRevision("artist", artist.ID)
	_, err = s.svc.Rollback(context.Background(), rollbackRevision.ID, admin.ID)
	s.Require().NoError(err)
	s.Equal("new blurb", s.artistColumn(artist.ID, "description"),
		"undoing the undo restores what the first rollback replaced")
}

// A legacy row's claim about what it wrote was never checked against anything.
// The column is the one that was true, so the column is what gets recorded.
func (s *RevisionServiceIntegrationTestSuite) TestRollback_PlantedNewValueIsRefusedNotRecorded() {
	admin := s.createTestUser()
	artist := s.createTestArtist(
		fmt.Sprintf("Planted %d", time.Now().UnixNano()), "Phoenix", "AZ", "")
	s.Require().NoError(s.db.Table("artists").Where("id = ?", artist.ID).
		Update("description", "the real blurb").Error)

	// Written directly: no forward path produces a new_value the column never
	// held, which is exactly why the claim cannot be trusted.
	raw := json.RawMessage(`[{"field":"description","old_value":"planted old","new_value":"planted new"}]`)
	revision := &adminm.Revision{
		EntityType: "artist", EntityID: artist.ID, UserID: admin.ID, FieldChanges: &raw,
	}
	s.Require().NoError(s.db.Create(revision).Error)

	_, err := s.svc.Rollback(context.Background(), revision.ID, admin.ID)
	s.Require().Error(err)
	s.Contains(err.Error(), "no field of this revision can be restored")
	s.Equal("the real blurb", s.artistColumn(artist.ID, "description"),
		"a claim the entity never held cannot write over the entity")
}

// Shows record revisions and accept no pending edits, so they have no
// contributor allowlist at all. A derivation scoped by that allowlist would
// refuse every show rollback outright.
func (s *RevisionServiceIntegrationTestSuite) TestRollback_ShowRevisionIsObservable() {
	admin := s.createTestUser()
	show := &catalogm.Show{
		Title:     "Original Bill",
		EventDate: time.Now().UTC().AddDate(0, 0, 21),
		Status:    catalogm.ShowStatusApproved,
	}
	s.Require().NoError(s.db.Create(show).Error)

	changes := []adminm.FieldChange{{Field: "title", OldValue: "Original Bill", NewValue: "Renamed Bill"}}
	s.Require().NoError(s.svc.RecordRevision("show", show.ID, admin.ID, changes, "retitled"))
	s.applyRecordedChanges("show", show.ID, changes)

	revision := s.latestRevision("show", show.ID)
	result, err := s.svc.Rollback(context.Background(), revision.ID, admin.ID)
	s.Require().NoError(err)
	s.Equal([]string{"title"}, result.AppliedFields)

	var restored catalogm.Show
	s.Require().NoError(s.db.First(&restored, show.ID).Error)
	s.Equal("Original Bill", restored.Title)
}

// A revision can record fields no contributor may edit — a label's status is one
// of them. The derivation's scope is the model's columns, not the allowlist, so
// these restore like any other field.
func (s *RevisionServiceIntegrationTestSuite) TestRollback_AdminOnlyFieldIsObservable() {
	admin := s.createTestUser()
	label := catalogm.Label{Name: "Status Records", Status: "active"}
	s.Require().NoError(s.db.Create(&label).Error)

	changes := []adminm.FieldChange{{Field: "status", OldValue: "active", NewValue: "defunct"}}
	s.Require().NoError(s.svc.RecordRevision("label", label.ID, admin.ID, changes, "closed"))
	s.applyRecordedChanges("label", label.ID, changes)

	revision := s.latestRevision("label", label.ID)
	result, err := s.svc.Rollback(context.Background(), revision.ID, admin.ID)
	s.Require().NoError(err)
	s.Equal([]string{"status"}, result.AppliedFields)

	var restored catalogm.Label
	s.Require().NoError(s.db.First(&restored, label.ID).Error)
	s.EqualValues("active", restored.Status)
}

// A number reaches the comparison as a JSONB float64 on one side and an int
// column on the other, and the recorded old_value is the column's int.
func (s *RevisionServiceIntegrationTestSuite) TestRollback_ObservesANumericColumnAcrossEncodings() {
	admin := s.createTestUser()
	venue := s.createTestVenue("Counted Room")

	changes := []adminm.FieldChange{{Field: "capacity", OldValue: 120, NewValue: 350}}
	s.Require().NoError(s.svc.RecordRevision("venue", venue.ID, admin.ID, changes, "recounted"))
	s.Require().NoError(s.db.Model(&catalogm.Venue{}).Where("id = ?", venue.ID).
		Update("capacity", 350).Error)

	revision := s.latestRevision("venue", venue.ID)
	result, err := s.svc.Rollback(context.Background(), revision.ID, admin.ID)
	s.Require().NoError(err)
	s.Equal([]string{"capacity"}, result.AppliedFields)

	recorded := s.latestRevision("venue", venue.ID)
	var recordedChanges []adminm.FieldChange
	s.Require().NoError(json.Unmarshal(*recorded.FieldChanges, &recordedChanges))
	s.Require().Len(recordedChanges, 1)
	s.EqualValues(350, recordedChanges[0].OldValue)

	var restored catalogm.Venue
	s.Require().NoError(s.db.First(&restored, venue.ID).Error)
	s.Require().NotNil(restored.Capacity)
	s.Equal(120, *restored.Capacity)
}

// An unverified venue withholds its address from every reader, and a rollback
// still has to be able to restore it: revision history serves an admin the
// unmasked diff precisely so the Rollback button beside it can act on the real
// stored value. So the observation reads the COLUMN here, and the value it
// records is the column's.
func (s *RevisionServiceIntegrationTestSuite) TestRollback_ObservesTheColumnForAWithheldField() {
	admin := s.createTestUser()
	venue := s.createTestVenue("Somebodys House")
	s.Require().False(venue.Verified)

	changes := []adminm.FieldChange{{Field: "address", OldValue: "1 Old St", NewValue: secretAddress}}
	s.Require().NoError(s.svc.RecordRevision("venue", venue.ID, admin.ID, changes, "corrected"))
	s.applyRecordedChanges("venue", venue.ID, changes)

	revision := s.latestRevision("venue", venue.ID)
	result, err := s.svc.Rollback(context.Background(), revision.ID, admin.ID)
	s.Require().NoError(err)
	s.Equal([]string{"address"}, result.AppliedFields,
		"a withheld field must not be permanently unrestorable")

	var restored catalogm.Venue
	s.Require().NoError(s.db.First(&restored, venue.ID).Error)
	s.Require().NotNil(restored.Address)
	s.Equal("1 Old St", *restored.Address)

	recorded := s.latestRevision("venue", venue.ID)
	var recordedChanges []adminm.FieldChange
	s.Require().NoError(json.Unmarshal(*recorded.FieldChanges, &recordedChanges))
	s.Require().Len(recordedChanges, 1)
	s.Equal(secretAddress, recordedChanges[0].OldValue,
		"the observed column is what history records, where the same list masks it on read")
}

// The observation and the write are one transaction, so a rollback refused for a
// value it could not observe leaves no half-applied entity behind.
func (s *RevisionServiceIntegrationTestSuite) TestRollback_VanishedEntityIsReportedNotWritten() {
	admin := s.createTestUser()
	artist := s.createTestArtist(
		fmt.Sprintf("Vanished %d", time.Now().UnixNano()), "Phoenix", "AZ", "")

	changes := []adminm.FieldChange{{Field: "description", OldValue: "old blurb", NewValue: "new blurb"}}
	s.Require().NoError(s.svc.RecordRevision("artist", artist.ID, admin.ID, changes, "one field"))
	revision := s.latestRevision("artist", artist.ID)
	s.Require().NoError(s.db.Table("artists").Where("id = ?", artist.ID).Delete(nil).Error)

	result, err := s.svc.Rollback(context.Background(), revision.ID, admin.ID)
	s.Require().Error(err)
	s.Nil(result)
	s.Contains(err.Error(), "entity not found")
}

// artistColumn reads one scalar column off an artist, for the assertions whose
// subject is the value in the database rather than the report.
func (s *RevisionServiceIntegrationTestSuite) artistColumn(artistID uint, column string) interface{} {
	var stored map[string]interface{}
	s.Require().NoError(s.db.Table("artists").Where("id = ?", artistID).Take(&stored).Error)
	return stored[column]
}
