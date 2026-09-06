package admin

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	adminm "psychic-homily-backend/internal/models/admin"
)

// A revision is one contributor edit across several fields, and the apply-side
// gates answer per FIELD. Refusing the revision whole for one bad stored value
// also refused the undo of every honest field recorded beside it, so a
// contributor could deny undo of their own edit by planting one URL.
func (s *RevisionServiceIntegrationTestSuite) TestRollback_RestoresPassingFieldsAndReportsRefusedOnes() {
	admin := s.createTestUser()
	artist := s.createTestArtist(
		fmt.Sprintf("Partial Rollback %d", time.Now().UnixNano()), "Phoenix", "AZ", "")

	s.Require().NoError(s.db.Table("artists").Where("id = ?", artist.ID).Updates(map[string]interface{}{
		"description": "new blurb",
		"spotify":     "https://open.spotify.com/artist/4Z8W4fKeB5YxbusRsdQVPb",
	}).Error)

	changes := []adminm.FieldChange{
		{Field: "description", OldValue: "old blurb", NewValue: "new blurb"},
		{Field: "spotify",
			OldValue: "https://spotify-account-verify.evil.test/",
			NewValue: "https://open.spotify.com/artist/4Z8W4fKeB5YxbusRsdQVPb"},
	}
	s.Require().NoError(s.svc.RecordRevision("artist", artist.ID, admin.ID, changes, "two fields"))
	revision := s.latestRevision("artist", artist.ID)

	result, err := s.svc.Rollback(context.Background(), revision.ID, admin.ID)
	s.Require().NoError(err)
	s.Equal([]string{"description"}, result.AppliedFields)
	s.Require().Len(result.SkippedFields, 1)
	s.Equal("spotify", result.SkippedFields[0].Field)
	s.NotEmpty(result.SkippedFields[0].Reason, "a skipped field must carry the reason it was refused")

	var stored map[string]interface{}
	s.Require().NoError(s.db.Table("artists").Where("id = ?", artist.ID).Take(&stored).Error)
	s.Equal("old blurb", stored["description"], "the passing field is restored")
	s.Equal("https://open.spotify.com/artist/4Z8W4fKeB5YxbusRsdQVPb", stored["spotify"],
		"the refused field is left exactly as it was")
}

// The recorded rollback revision describes what the rollback did, not what it
// was asked to do: it carries only the restored fields, and its summary names
// the skipped ones so a reader of history cannot mistake a partial undo for a
// whole one.
func (s *RevisionServiceIntegrationTestSuite) TestRollback_RecordedRevisionDescribesOnlyWhatItRestored() {
	admin := s.createTestUser()
	artist := s.createTestArtist(
		fmt.Sprintf("Partial History %d", time.Now().UnixNano()), "Phoenix", "AZ", "")

	changes := []adminm.FieldChange{
		{Field: "description", OldValue: "old blurb", NewValue: "new blurb"},
		{Field: "website", OldValue: "javascript:alert(1)", NewValue: "https://band.example.org"},
	}
	s.Require().NoError(s.svc.RecordRevision("artist", artist.ID, admin.ID, changes, "two fields"))
	// The entity has to hold what the revision recorded writing, or the rollback
	// skips every field as changed since. See applyRecordedChanges.
	s.applyRecordedChanges("artist", artist.ID, changes)
	original := s.latestRevision("artist", artist.ID)

	_, err := s.svc.Rollback(context.Background(), original.ID, admin.ID)
	s.Require().NoError(err)

	recorded := s.latestRevision("artist", artist.ID)
	s.Require().NotEqual(original.ID, recorded.ID)
	s.Require().NotNil(recorded.Summary)
	s.Contains(*recorded.Summary, "skipped: website")

	var recordedChanges []adminm.FieldChange
	s.Require().NoError(json.Unmarshal(*recorded.FieldChanges, &recordedChanges))
	s.Require().Len(recordedChanges, 1)
	s.Equal("description", recordedChanges[0].Field)
}

// Nothing restorable is a refusal, not a rollback that did nothing: the caller
// must not be told an undo happened, and the message names every field and its
// reason because there is no result object to carry them.
func (s *RevisionServiceIntegrationTestSuite) TestRollback_RefusesWhenEveryFieldIsRefused() {
	admin := s.createTestUser()
	artist := s.createTestArtist(
		fmt.Sprintf("Nothing Restorable %d", time.Now().UnixNano()), "Phoenix", "AZ", "")

	changes := []adminm.FieldChange{
		{Field: "website", OldValue: "javascript:alert(1)", NewValue: "https://band.example.org"},
		{Field: "spotify", OldValue: "https://spotify.evil.test/", NewValue: "https://open.spotify.com/artist/x"},
	}
	s.Require().NoError(s.svc.RecordRevision("artist", artist.ID, admin.ID, changes, "all bad"))
	revision := s.latestRevision("artist", artist.ID)

	result, err := s.svc.Rollback(context.Background(), revision.ID, admin.ID)
	s.Require().Error(err)
	s.Nil(result)
	s.Contains(err.Error(), "website")
	s.Contains(err.Error(), "spotify")

	var count int64
	s.Require().NoError(s.db.Model(&adminm.Revision{}).
		Where("entity_type = ? AND entity_id = ?", "artist", artist.ID).Count(&count).Error)
	s.EqualValues(1, count, "a refused rollback records no revision")
}

// The ordinary case still reports itself: every field restored, nothing skipped.
func (s *RevisionServiceIntegrationTestSuite) TestRollback_ReportsNoSkipsWhenEveryFieldPasses() {
	admin := s.createTestUser()
	artist := s.createTestArtist(
		fmt.Sprintf("All Restorable %d", time.Now().UnixNano()), "Phoenix", "AZ", "")

	changes := []adminm.FieldChange{
		{Field: "description", OldValue: "old blurb", NewValue: "new blurb"},
		{Field: "website", OldValue: "https://old.example.org", NewValue: "https://new.example.org"},
	}
	s.Require().NoError(s.svc.RecordRevision("artist", artist.ID, admin.ID, changes, "two good fields"))
	s.applyRecordedChanges("artist", artist.ID, changes)
	revision := s.latestRevision("artist", artist.ID)

	result, err := s.svc.Rollback(context.Background(), revision.ID, admin.ID)
	s.Require().NoError(err)
	s.Equal([]string{"description", "website"}, result.AppliedFields)
	s.Empty(result.SkippedFields)
	s.NotNil(result.SkippedFields, "an empty refusal set is a list, not the absence of one")

	var stored map[string]interface{}
	s.Require().NoError(s.db.Table("artists").Where("id = ?", artist.ID).Take(&stored).Error)
	s.Equal("old blurb", stored["description"])
	s.Equal("https://old.example.org", stored["website"])
}

// applyRecordedChanges writes a revision's NEW values onto the entity, which is
// the state the revision claims to have left behind.
//
// A rollback only restores a field the entity still holds the revision's value
// for, so a fixture that records a revision without applying it is describing an
// entity somebody else has already changed, and every field of it is skipped.
func (s *RevisionServiceIntegrationTestSuite) applyRecordedChanges(entityType string, entityID uint, changes []adminm.FieldChange) {
	updates := make(map[string]interface{}, len(changes))
	for _, c := range changes {
		updates[c.Field] = c.NewValue
	}
	s.Require().NoError(s.db.Table(entityType+"s").Where("id = ?", entityID).Updates(updates).Error)
}

// latestRevision reads the most recent revision recorded against an entity.
func (s *RevisionServiceIntegrationTestSuite) latestRevision(entityType string, entityID uint) adminm.Revision {
	var revision adminm.Revision
	s.Require().NoError(s.db.Where("entity_type = ? AND entity_id = ?", entityType, entityID).
		Order("id DESC").First(&revision).Error)
	return revision
}

// "422 unchanged" is two claims, and the second one is the one an admin cares
// about: a refused rollback must not have written anything on its way to
// refusing.
func (s *RevisionServiceIntegrationTestSuite) TestRollback_RefusedRollbackLeavesTheEntityAlone() {
	admin := s.createTestUser()
	artist := s.createTestArtist(
		fmt.Sprintf("Untouched %d", time.Now().UnixNano()), "Phoenix", "AZ", "")
	s.Require().NoError(s.db.Table("artists").Where("id = ?", artist.ID).Updates(map[string]interface{}{
		"website": "https://current.example.org",
		"spotify": "https://open.spotify.com/artist/current",
	}).Error)

	changes := []adminm.FieldChange{
		{Field: "website", OldValue: "javascript:alert(1)", NewValue: "https://current.example.org"},
		{Field: "spotify", OldValue: "https://spotify.evil.test/", NewValue: "https://open.spotify.com/artist/current"},
	}
	s.Require().NoError(s.svc.RecordRevision("artist", artist.ID, admin.ID, changes, "all planted"))
	revision := s.latestRevision("artist", artist.ID)

	_, err := s.svc.Rollback(context.Background(), revision.ID, admin.ID)
	s.Require().Error(err)

	var stored map[string]interface{}
	s.Require().NoError(s.db.Table("artists").Where("id = ?", artist.ID).Take(&stored).Error)
	s.Equal("https://current.example.org", stored["website"])
	s.Equal("https://open.spotify.com/artist/current", stored["spotify"])
}

// A revision naming one field twice must gate and write the SAME entry. The map
// an update is built from collapses duplicates on its own, so a loop that read
// the slice instead would check one occurrence and write the other.
func (s *RevisionServiceIntegrationTestSuite) TestRollback_CollapsesARepeatedField() {
	admin := s.createTestUser()
	artist := s.createTestArtist(
		fmt.Sprintf("Repeated Field %d", time.Now().UnixNano()), "Phoenix", "AZ", "")

	// Written directly: RecordRevision is fed by diffs that cannot repeat a
	// field, so a row like this only exists if something else wrote it.
	raw := json.RawMessage(`[{"field":"description","old_value":"first","new_value":"live"},` +
		`{"field":"description","old_value":"second","new_value":"live"}]`)
	revision := &adminm.Revision{
		EntityType: "artist", EntityID: artist.ID, UserID: admin.ID, FieldChanges: &raw,
	}
	s.Require().NoError(s.db.Create(revision).Error)
	s.Require().NoError(s.db.Table("artists").Where("id = ?", artist.ID).
		Update("description", "live").Error)

	result, err := s.svc.Rollback(context.Background(), revision.ID, admin.ID)
	s.Require().NoError(err)
	s.Equal([]string{"description"}, result.AppliedFields, "the field is reported once")

	var stored map[string]interface{}
	s.Require().NoError(s.db.Table("artists").Where("id = ?", artist.ID).Take(&stored).Error)
	s.Equal("second", stored["description"], "the written value is the one the update map holds")

	recorded := s.latestRevision("artist", artist.ID)
	var recordedChanges []adminm.FieldChange
	s.Require().NoError(json.Unmarshal(*recorded.FieldChanges, &recordedChanges))
	s.Require().Len(recordedChanges, 1)
	s.Equal("second", recordedChanges[0].NewValue, "history records the value that was written")
}

// A revision recording no field changes has no undo to report, so it is a
// refusal rather than a rollback that did nothing.
func (s *RevisionServiceIntegrationTestSuite) TestRollback_RefusesARevisionWithNoChanges() {
	admin := s.createTestUser()
	artist := s.createTestArtist(
		fmt.Sprintf("No Changes %d", time.Now().UnixNano()), "Phoenix", "AZ", "")

	raw := json.RawMessage(`[]`)
	revision := &adminm.Revision{
		EntityType: "artist", EntityID: artist.ID, UserID: admin.ID, FieldChanges: &raw,
	}
	s.Require().NoError(s.db.Create(revision).Error)

	result, err := s.svc.Rollback(context.Background(), revision.ID, admin.ID)
	s.Require().Error(err)
	s.Nil(result)
	s.Contains(err.Error(), "no field changes")
}
