package admin

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"

	adminm "psychic-homily-backend/internal/models/admin"
	authm "psychic-homily-backend/internal/models/auth"
	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/services/shared/revisiondiff"
	"psychic-homily-backend/internal/testutil"
)

// =============================================================================
// INTEGRATION TESTS (With Real Database)
// =============================================================================

type RevisionServiceIntegrationTestSuite struct {
	suite.Suite
	testDB *testutil.TestDatabase
	db     *gorm.DB
	svc    *RevisionService
}

func (s *RevisionServiceIntegrationTestSuite) SetupSuite() {
	s.testDB = testutil.SetupTestPostgres(s.T())
	s.db = s.testDB.DB

	s.svc = NewRevisionService(s.testDB.DB)
}

func (s *RevisionServiceIntegrationTestSuite) TearDownSuite() {
	s.testDB.Cleanup()
}

func (s *RevisionServiceIntegrationTestSuite) TearDownTest() {
	sqlDB, err := s.db.DB()
	s.Require().NoError(err)
	_, _ = sqlDB.Exec("DELETE FROM revisions")
	_, _ = sqlDB.Exec("DELETE FROM venues")
	_, _ = sqlDB.Exec("DELETE FROM users")
}

func TestRevisionServiceIntegrationSuite(t *testing.T) {
	suite.Run(t, new(RevisionServiceIntegrationTestSuite))
}

// =============================================================================
// HELPERS
// =============================================================================

func (s *RevisionServiceIntegrationTestSuite) createTestUser() *authm.User {
	user := &authm.User{
		Email:         stringPtr(fmt.Sprintf("rev-user-%d@test.com", time.Now().UnixNano())),
		FirstName:     stringPtr("Test"),
		LastName:      stringPtr("User"),
		IsActive:      true,
		EmailVerified: true,
	}
	err := s.db.Create(user).Error
	s.Require().NoError(err)
	return user
}

func (s *RevisionServiceIntegrationTestSuite) createTestVenue(name string) *catalogm.Venue {
	slug := fmt.Sprintf("test-venue-%d", time.Now().UnixNano())
	venue := &catalogm.Venue{
		Name:  name,
		Slug:  &slug,
		City:  "Phoenix",
		State: "AZ",
	}
	err := s.db.Create(venue).Error
	s.Require().NoError(err)
	return venue
}

// =============================================================================
// RecordRevision tests
// =============================================================================

func (s *RevisionServiceIntegrationTestSuite) TestRecordRevision_Success() {
	user := s.createTestUser()

	changes := []adminm.FieldChange{
		{Field: "name", OldValue: "Old Name", NewValue: "New Name"},
		{Field: "city", OldValue: "Phoenix", NewValue: "Tempe"},
	}

	err := s.svc.RecordRevision("venue", 42, user.ID, changes, "Updated venue info")
	s.NoError(err)

	// Verify the revision was created
	var revision adminm.Revision
	err = s.db.First(&revision).Error
	s.Require().NoError(err)
	s.Equal("venue", revision.EntityType)
	s.Equal(uint(42), revision.EntityID)
	s.Equal(user.ID, revision.UserID)
	s.Require().NotNil(revision.Summary)
	s.Equal("Updated venue info", *revision.Summary)
	s.Require().NotNil(revision.FieldChanges)

	// Verify field changes deserialization
	var parsedChanges []adminm.FieldChange
	err = json.Unmarshal(*revision.FieldChanges, &parsedChanges)
	s.NoError(err)
	s.Len(parsedChanges, 2)
	s.Equal("name", parsedChanges[0].Field)
	s.Equal("Old Name", parsedChanges[0].OldValue)
	s.Equal("New Name", parsedChanges[0].NewValue)
}

func (s *RevisionServiceIntegrationTestSuite) TestRecordRevision_EmptyChanges() {
	user := s.createTestUser()

	err := s.svc.RecordRevision("artist", 1, user.ID, []adminm.FieldChange{}, "no changes")
	s.NoError(err)

	// Verify no revision was created
	var count int64
	s.db.Model(&adminm.Revision{}).Count(&count)
	s.Equal(int64(0), count)
}

func (s *RevisionServiceIntegrationTestSuite) TestRecordRevision_NilChanges() {
	user := s.createTestUser()

	err := s.svc.RecordRevision("artist", 1, user.ID, nil, "no changes")
	s.NoError(err)

	var count int64
	s.db.Model(&adminm.Revision{}).Count(&count)
	s.Equal(int64(0), count)
}

func (s *RevisionServiceIntegrationTestSuite) TestRecordRevision_EmptySummary() {
	user := s.createTestUser()

	changes := []adminm.FieldChange{
		{Field: "name", OldValue: "Old", NewValue: "New"},
	}

	err := s.svc.RecordRevision("artist", 1, user.ID, changes, "")
	s.NoError(err)

	var revision adminm.Revision
	err = s.db.First(&revision).Error
	s.Require().NoError(err)
	s.Nil(revision.Summary) // Empty summary stored as nil
}

// =============================================================================
// GetEntityHistory tests
// =============================================================================

func (s *RevisionServiceIntegrationTestSuite) TestGetEntityHistory_Success() {
	user := s.createTestUser()

	// Create 3 revisions for the same entity
	for i := 0; i < 3; i++ {
		changes := []adminm.FieldChange{
			{Field: "name", OldValue: fmt.Sprintf("Name %d", i), NewValue: fmt.Sprintf("Name %d", i+1)},
		}
		err := s.svc.RecordRevision("artist", 10, user.ID, changes, fmt.Sprintf("Edit %d", i+1))
		s.Require().NoError(err)
	}

	revisions, total, err := s.svc.GetEntityHistory("artist", 10, 10, 0)
	s.NoError(err)
	s.Equal(int64(3), total)
	s.Len(revisions, 3)

	// Verify ordering (newest first)
	s.Require().NotNil(revisions[0].Summary)
	s.Equal("Edit 3", *revisions[0].Summary)
	s.Require().NotNil(revisions[2].Summary)
	s.Equal("Edit 1", *revisions[2].Summary)
}

func (s *RevisionServiceIntegrationTestSuite) TestGetEntityHistory_Pagination() {
	user := s.createTestUser()

	for i := 0; i < 5; i++ {
		changes := []adminm.FieldChange{
			{Field: "name", OldValue: "old", NewValue: "new"},
		}
		err := s.svc.RecordRevision("venue", 20, user.ID, changes, fmt.Sprintf("Edit %d", i+1))
		s.Require().NoError(err)
	}

	// Page 1
	revisions, total, err := s.svc.GetEntityHistory("venue", 20, 2, 0)
	s.NoError(err)
	s.Equal(int64(5), total)
	s.Len(revisions, 2)

	// Page 2
	revisions2, _, err := s.svc.GetEntityHistory("venue", 20, 2, 2)
	s.NoError(err)
	s.Len(revisions2, 2)

	// No overlap
	s.NotEqual(revisions[0].ID, revisions2[0].ID)

	// Page 3
	revisions3, _, err := s.svc.GetEntityHistory("venue", 20, 2, 4)
	s.NoError(err)
	s.Len(revisions3, 1)
}

func (s *RevisionServiceIntegrationTestSuite) TestGetEntityHistory_DefaultLimit() {
	revisions, total, err := s.svc.GetEntityHistory("artist", 999, 0, 0)
	s.NoError(err)
	s.Equal(int64(0), total)
	s.Empty(revisions)
}

func (s *RevisionServiceIntegrationTestSuite) TestGetEntityHistory_MaxLimit() {
	revisions, total, err := s.svc.GetEntityHistory("artist", 999, 200, 0)
	s.NoError(err)
	s.Equal(int64(0), total)
	s.Empty(revisions)
}

func (s *RevisionServiceIntegrationTestSuite) TestGetEntityHistory_FiltersByEntity() {
	user := s.createTestUser()

	changes := []adminm.FieldChange{{Field: "name", OldValue: "a", NewValue: "b"}}
	s.Require().NoError(s.svc.RecordRevision("artist", 1, user.ID, changes, "artist edit"))
	s.Require().NoError(s.svc.RecordRevision("venue", 1, user.ID, changes, "venue edit"))
	s.Require().NoError(s.svc.RecordRevision("artist", 2, user.ID, changes, "other artist edit"))

	revisions, total, err := s.svc.GetEntityHistory("artist", 1, 10, 0)
	s.NoError(err)
	s.Equal(int64(1), total)
	s.Len(revisions, 1)
	s.Require().NotNil(revisions[0].Summary)
	s.Equal("artist edit", *revisions[0].Summary)
}

func (s *RevisionServiceIntegrationTestSuite) TestGetEntityHistory_PreloadsUser() {
	user := s.createTestUser()

	changes := []adminm.FieldChange{{Field: "name", OldValue: "a", NewValue: "b"}}
	s.Require().NoError(s.svc.RecordRevision("artist", 1, user.ID, changes, "test"))

	revisions, _, err := s.svc.GetEntityHistory("artist", 1, 10, 0)
	s.NoError(err)
	s.Require().Len(revisions, 1)
	s.Equal(user.ID, revisions[0].User.ID)
	s.Equal(*user.Email, *revisions[0].User.Email)
}

// =============================================================================
// GetRevision tests
// =============================================================================

func (s *RevisionServiceIntegrationTestSuite) TestGetRevision_Found() {
	user := s.createTestUser()

	changes := []adminm.FieldChange{
		{Field: "name", OldValue: "Old", NewValue: "New"},
	}
	err := s.svc.RecordRevision("artist", 5, user.ID, changes, "test edit")
	s.Require().NoError(err)

	// Get the created revision's ID
	var created adminm.Revision
	s.db.First(&created)

	revision, err := s.svc.GetRevision(created.ID)
	s.NoError(err)
	s.NotNil(revision)
	s.Equal(created.ID, revision.ID)
	s.Equal("artist", revision.EntityType)
	s.Equal(uint(5), revision.EntityID)
	s.Equal(user.ID, revision.User.ID)
}

func (s *RevisionServiceIntegrationTestSuite) TestGetRevision_NotFound() {
	revision, err := s.svc.GetRevision(99999)
	s.NoError(err)
	s.Nil(revision)
}

// =============================================================================
// GetUserRevisions tests
// =============================================================================

func (s *RevisionServiceIntegrationTestSuite) TestGetUserRevisions_Success() {
	user1 := s.createTestUser()
	user2 := s.createTestUser()

	changes := []adminm.FieldChange{{Field: "name", OldValue: "a", NewValue: "b"}}

	// User 1 makes 3 edits
	for i := 0; i < 3; i++ {
		s.Require().NoError(s.svc.RecordRevision("artist", uint(i+1), user1.ID, changes, fmt.Sprintf("user1 edit %d", i)))
	}

	// User 2 makes 1 edit
	s.Require().NoError(s.svc.RecordRevision("venue", 1, user2.ID, changes, "user2 edit"))

	// Get user1's revisions
	revisions, total, err := s.svc.GetUserRevisions(user1.ID, 10, 0)
	s.NoError(err)
	s.Equal(int64(3), total)
	s.Len(revisions, 3)

	// All revisions belong to user1
	for _, r := range revisions {
		s.Equal(user1.ID, r.UserID)
	}

	// Get user2's revisions
	revisions2, total2, err := s.svc.GetUserRevisions(user2.ID, 10, 0)
	s.NoError(err)
	s.Equal(int64(1), total2)
	s.Len(revisions2, 1)
}

func (s *RevisionServiceIntegrationTestSuite) TestGetUserRevisions_Pagination() {
	user := s.createTestUser()

	changes := []adminm.FieldChange{{Field: "name", OldValue: "a", NewValue: "b"}}
	for i := 0; i < 5; i++ {
		s.Require().NoError(s.svc.RecordRevision("artist", uint(i+1), user.ID, changes, ""))
	}

	revisions, total, err := s.svc.GetUserRevisions(user.ID, 2, 0)
	s.NoError(err)
	s.Equal(int64(5), total)
	s.Len(revisions, 2)

	revisions2, _, err := s.svc.GetUserRevisions(user.ID, 2, 2)
	s.NoError(err)
	s.Len(revisions2, 2)

	// No overlap
	s.NotEqual(revisions[0].ID, revisions2[0].ID)
}

func (s *RevisionServiceIntegrationTestSuite) TestGetUserRevisions_Empty() {
	user := s.createTestUser()

	revisions, total, err := s.svc.GetUserRevisions(user.ID, 10, 0)
	s.NoError(err)
	s.Equal(int64(0), total)
	s.Empty(revisions)
}

// =============================================================================
// Rollback tests
// =============================================================================

func (s *RevisionServiceIntegrationTestSuite) TestRollback_Success() {
	user := s.createTestUser()
	adminUser := s.createTestUser()
	venue := s.createTestVenue("Original Name")

	// Record a revision changing the venue name
	changes := []adminm.FieldChange{
		{Field: "name", OldValue: "Original Name", NewValue: "Changed Name"},
	}
	err := s.svc.RecordRevision("venue", venue.ID, user.ID, changes, "renamed venue")
	s.Require().NoError(err)

	// Apply the change to the venue (simulating what an edit handler would do)
	s.db.Table("venues").Where("id = ?", venue.ID).Updates(map[string]interface{}{
		"name": "Changed Name",
	})

	// Verify venue has changed name
	var updatedVenue catalogm.Venue
	s.db.First(&updatedVenue, venue.ID)
	s.Equal("Changed Name", updatedVenue.Name)

	// Get the revision to rollback
	var revision adminm.Revision
	s.db.Where("entity_type = ? AND entity_id = ?", "venue", venue.ID).First(&revision)

	// Rollback
	err = s.svc.Rollback(revision.ID, adminUser.ID)
	s.NoError(err)

	// Verify venue name is restored
	var restoredVenue catalogm.Venue
	s.db.First(&restoredVenue, venue.ID)
	s.Equal("Original Name", restoredVenue.Name)

	// Verify a rollback revision was created
	var allRevisions []adminm.Revision
	s.db.Where("entity_type = ? AND entity_id = ?", "venue", venue.ID).
		Order("created_at DESC").
		Find(&allRevisions)
	s.Len(allRevisions, 2) // original + rollback

	rollbackRevision := allRevisions[0]
	s.Equal(adminUser.ID, rollbackRevision.UserID)
	s.Require().NotNil(rollbackRevision.Summary)
	s.Contains(*rollbackRevision.Summary, "Rollback of revision #")

	// Verify rollback revision has inverted changes
	var rollbackChanges []adminm.FieldChange
	err = json.Unmarshal(*rollbackRevision.FieldChanges, &rollbackChanges)
	s.NoError(err)
	s.Len(rollbackChanges, 1)
	s.Equal("name", rollbackChanges[0].Field)
	s.Equal("Changed Name", rollbackChanges[0].OldValue)
	s.Equal("Original Name", rollbackChanges[0].NewValue)
}

// TestRollback_NullableTimestampBackToNull covers rolling back the first time a
// nullable timestamp column is populated, which is the only transition the show
// API can currently produce for doors_at/music_at.
//
// The regression this guards: revisiondiff used to encode "was unset" as "",
// and Rollback feeds recorded old values straight into a GORM update map, so
// the rollback died on `invalid input syntax for type timestamp with time zone:
// ""` and took every other field in the same revision down with it.
func (s *RevisionServiceIntegrationTestSuite) TestRollback_NullableTimestampBackToNull() {
	user := s.createTestUser()
	adminUser := s.createTestUser()

	show := &catalogm.Show{
		Title:     "Rollback Times",
		EventDate: time.Now().UTC().AddDate(0, 0, 14),
		Status:    catalogm.ShowStatusApproved,
	}
	s.Require().NoError(s.db.Create(show).Error)

	doors := show.EventDate.Add(-time.Hour)
	// Derive the changes through revisiondiff rather than hand-writing them, so
	// this test fails if the encoding of an unset timestamp regresses, not just
	// if Rollback mishandles a nil it was handed.
	changes := revisiondiff.Compare(
		&contracts.ShowResponse{Title: "Rollback Times"},
		&contracts.ShowResponse{Title: "Retitled", DoorsAt: &doors},
		revisiondiff.ShowFields,
	)
	s.Require().Len(changes, 2, "expected title + doors_at changes, got %v", changes)
	s.Require().NoError(s.svc.RecordRevision("show", show.ID, user.ID, changes, "set doors"))
	s.Require().NoError(s.db.Table("shows").Where("id = ?", show.ID).
		Updates(map[string]interface{}{"title": "Retitled", "doors_at": doors}).Error)

	var revision adminm.Revision
	s.Require().NoError(s.db.Where("entity_type = ? AND entity_id = ?", "show", show.ID).
		First(&revision).Error)

	s.Require().NoError(s.svc.Rollback(revision.ID, adminUser.ID),
		"rolling back a revision that first set a nullable timestamp must succeed")

	var restored catalogm.Show
	s.Require().NoError(s.db.First(&restored, show.ID).Error)
	s.Nil(restored.DoorsAt, "doors_at should be back to NULL")
	s.Equal("Rollback Times", restored.Title, "the bundled field must roll back too")
}

func (s *RevisionServiceIntegrationTestSuite) TestRollback_RevisionNotFound() {
	err := s.svc.Rollback(99999, 1)
	s.Error(err)
	s.Contains(err.Error(), "revision not found")
}

func (s *RevisionServiceIntegrationTestSuite) TestRollback_EntityNotFound() {
	user := s.createTestUser()

	// Record a revision for an entity that doesn't exist
	changes := []adminm.FieldChange{
		{Field: "name", OldValue: "Old", NewValue: "New"},
	}
	err := s.svc.RecordRevision("venue", 99999, user.ID, changes, "test")
	s.Require().NoError(err)

	var revision adminm.Revision
	s.db.First(&revision)

	err = s.svc.Rollback(revision.ID, user.ID)
	s.Error(err)
	s.Contains(err.Error(), "entity not found")
}

// Rollback replays revisions.field_changes through an untyped Updates(), so a
// stored number arrives as float64 exactly as it does on the approve path.
// Without narrowing the driver takes that for an integer column and truncates
// it, so an undo would restore a value that is not the one being undone.
func (s *RevisionServiceIntegrationTestSuite) TestRollback_NarrowsNumericValues() {
	user := s.createTestUser()
	venue := s.createTestVenue("Rollback Capacity Venue")

	const original = 550
	s.Require().NoError(s.db.Model(&catalogm.Venue{}).Where("id = ?", venue.ID).
		Update("capacity", original).Error)

	// A revision that moved capacity 550 -> 3600, as the approve path records it.
	s.Require().NoError(s.svc.RecordRevision("venue", venue.ID, user.ID,
		[]adminm.FieldChange{{Field: "capacity", OldValue: original, NewValue: 3600}},
		"counted the room"))
	s.Require().NoError(s.db.Model(&catalogm.Venue{}).Where("id = ?", venue.ID).
		Update("capacity", 3600).Error)

	var recorded adminm.Revision
	s.Require().NoError(s.db.Where("entity_type = ? AND entity_id = ?", "venue", venue.ID).
		Order("id DESC").First(&recorded).Error)

	s.Require().NoError(s.svc.Rollback(recorded.ID, user.ID))

	var restored catalogm.Venue
	s.Require().NoError(s.db.First(&restored, venue.ID).Error)
	s.Require().NotNil(restored.Capacity)
	s.Equal(original, *restored.Capacity, "rollback must restore the exact prior capacity")
}

// A rollback restores history, so it deliberately does NOT apply the capacity
// range. Values stored before the bound existed must stay undoable.
func (s *RevisionServiceIntegrationTestSuite) TestRollback_RestoresOutOfRangeHistoricalValue() {
	user := s.createTestUser()
	venue := s.createTestVenue("Legacy Capacity Venue")

	s.Require().NoError(s.svc.RecordRevision("venue", venue.ID, user.ID,
		[]adminm.FieldChange{{Field: "capacity", OldValue: 0, NewValue: 550}},
		"replaced a legacy zero"))
	s.Require().NoError(s.db.Model(&catalogm.Venue{}).Where("id = ?", venue.ID).
		Update("capacity", 550).Error)

	var recorded adminm.Revision
	s.Require().NoError(s.db.Where("entity_type = ? AND entity_id = ?", "venue", venue.ID).
		Order("id DESC").First(&recorded).Error)

	s.Require().NoError(s.svc.Rollback(recorded.ID, user.ID),
		"undo must not be blocked by a bound the historical value predates")

	var restored catalogm.Venue
	s.Require().NoError(s.db.First(&restored, venue.ID).Error)
	s.Require().NotNil(restored.Capacity)
	s.Equal(0, *restored.Capacity)
}

// =============================================================================
// Read-time privacy redaction
// =============================================================================

func (s *RevisionServiceIntegrationTestSuite) createVerifiedTestVenue(name string) *catalogm.Venue {
	venue := s.createTestVenue(name)
	// Verified is set with an explicit UPDATE rather than on Create so both
	// helpers share one path and neither can drift onto GORM's
	// zero-value-vs-column-default behavior for bools.
	s.Require().NoError(s.db.Model(&catalogm.Venue{}).Where("id = ?", venue.ID).
		Update("verified", true).Error)
	venue.Verified = true
	return venue
}

// addressChanges is the diff an approved contributor address edit records.
func addressChanges() []adminm.FieldChange {
	return []adminm.FieldChange{
		{Field: "name", OldValue: "Old Room", NewValue: "The Basement"},
		{Field: "address", OldValue: "1 Old St", NewValue: "1234 Secret St"},
		{Field: "zipcode", OldValue: "85003", NewValue: "85004"},
	}
}

// changesFor unmarshals a served revision's field_changes into a field->change
// map for assertions.
func (s *RevisionServiceIntegrationTestSuite) changesFor(r adminm.Revision) map[string]adminm.FieldChange {
	s.Require().NotNil(r.FieldChanges)
	var parsed []adminm.FieldChange
	s.Require().NoError(json.Unmarshal(*r.FieldChanges, &parsed))
	byField := make(map[string]adminm.FieldChange, len(parsed))
	for _, c := range parsed {
		byField[c.Field] = c
	}
	return byField
}

// assertAddressMasked pins the actual acceptance criterion first — neither the
// old nor the new street address appears ANYWHERE in the served bytes — and
// only then the shape of the mask. A field-by-field check alone would pass if a
// future writer duplicated the value under another key.
func (s *RevisionServiceIntegrationTestSuite) assertAddressMasked(r adminm.Revision) {
	s.Require().NotNil(r.FieldChanges)
	raw := string(*r.FieldChanges)
	s.NotContains(raw, "1234 Secret St", "new address must not be served")
	s.NotContains(raw, "1 Old St", "old address must not be served either")
	s.NotContains(raw, "85004")
	s.NotContains(raw, "85003")

	byField := s.changesFor(r)
	s.Equal(revisiondiff.RedactedValue, byField["address"].OldValue)
	s.Equal(revisiondiff.RedactedValue, byField["address"].NewValue)
	s.Equal(revisiondiff.RedactedValue, byField["zipcode"].OldValue)
	s.Equal(revisiondiff.RedactedValue, byField["zipcode"].NewValue)

	// The edit itself stays visible — this is redaction, not deletion.
	s.Len(byField, 3)
	s.Equal("Old Room", byField["name"].OldValue)
	s.Equal("The Basement", byField["name"].NewValue)
}

func (s *RevisionServiceIntegrationTestSuite) TestGetEntityHistory_RedactsUnverifiedVenueAddress() {
	user := s.createTestUser()
	venue := s.createTestVenue("Unverified Room")
	s.Require().False(venue.Verified)

	s.Require().NoError(s.svc.RecordRevision("venue", venue.ID, user.ID, addressChanges(), "moved"))

	revisions, total, err := s.svc.GetEntityHistory("venue", venue.ID, 10, 0)
	s.Require().NoError(err)
	s.Equal(int64(1), total)
	s.Require().Len(revisions, 1)
	s.assertAddressMasked(revisions[0])

	// Redaction is a serving concern only. The stored row must still hold the
	// real values, which is what rollback and any future policy change read.
	var stored adminm.Revision
	s.Require().NoError(s.db.First(&stored).Error)
	s.Contains(string(*stored.FieldChanges), "1234 Secret St",
		"the stored row must not be rewritten by a read")
}

// The DB-error half of fail-closed. An aborted transaction makes the
// verified-venue lookup fail for real, which must mask rather than publish.
func (s *RevisionServiceIntegrationTestSuite) TestGetEntityHistory_LookupErrorFailsClosed() {
	user := s.createTestUser()
	venue := s.createVerifiedTestVenue("Verified Room")

	s.Require().NoError(s.svc.RecordRevision("venue", venue.ID, user.ID, addressChanges(), "moved"))

	tx := s.db.Begin()
	defer func() { _ = tx.Rollback() }()
	// Poison the transaction: every later statement on it errors until rollback.
	s.Require().Error(tx.Exec("SELECT 1/0").Error)

	revisions, _, err := NewRevisionService(tx).GetEntityHistory("venue", venue.ID, 10, 0)
	s.Require().Error(err, "the history query itself fails on a poisoned tx")
	s.Empty(revisions)

	// The lookup alone, on the same poisoned tx: a VERIFIED venue must still
	// come back unverified, because the gate cannot prove otherwise.
	verified := NewRevisionService(tx).verifiedVenueIDs([]uint{venue.ID})
	s.Empty(verified, "a lookup error must not admit a venue to the verified set")
}

// nil db is the other fail-closed input: no lookup is possible, so nothing is
// verified and every venue revision masks.
func (s *RevisionServiceIntegrationTestSuite) TestApplyPrivacyRedaction_NilDBFailsClosed() {
	raw := json.RawMessage(`[{"field":"address","old_value":"1 Old St","new_value":"1234 Secret St"}]`)
	revisions := []adminm.Revision{{ID: 1, EntityType: "venue", EntityID: 7, FieldChanges: &raw}}

	(&RevisionService{db: nil}).applyPrivacyRedaction(revisions)

	s.NotContains(string(*revisions[0].FieldChanges), "1234 Secret St")
	s.NotContains(string(*revisions[0].FieldChanges), "1 Old St")
}

func (s *RevisionServiceIntegrationTestSuite) TestGetEntityHistory_ServesVerifiedVenueAddress() {
	user := s.createTestUser()
	venue := s.createVerifiedTestVenue("Verified Room")

	s.Require().NoError(s.svc.RecordRevision("venue", venue.ID, user.ID, addressChanges(), "moved"))

	revisions, _, err := s.svc.GetEntityHistory("venue", venue.ID, 10, 0)
	s.Require().NoError(err)
	s.Require().Len(revisions, 1)

	byField := s.changesFor(revisions[0])
	s.Equal("1 Old St", byField["address"].OldValue)
	s.Equal("1234 Secret St", byField["address"].NewValue)
	s.Equal("85004", byField["zipcode"].NewValue)
}

// A revision pointing at a venue row that no longer exists must mask, not
// publish: the gate has no evidence that venue was ever verified.
func (s *RevisionServiceIntegrationTestSuite) TestGetEntityHistory_MissingVenueFailsClosed() {
	user := s.createTestUser()

	s.Require().NoError(s.svc.RecordRevision("venue", 987654, user.ID, addressChanges(), "moved"))

	revisions, _, err := s.svc.GetEntityHistory("venue", 987654, 10, 0)
	s.Require().NoError(err)
	s.Require().Len(revisions, 1)
	s.assertAddressMasked(revisions[0])
}

// Non-venue entities carry no private field, so their diffs must come back
// exactly as stored — the gate must not quietly reshape unrelated history.
func (s *RevisionServiceIntegrationTestSuite) TestGetEntityHistory_LeavesNonVenueEntitiesUntouched() {
	user := s.createTestUser()

	changes := []adminm.FieldChange{
		{Field: "city", OldValue: "Phoenix", NewValue: "Tempe"},
		{Field: "description", OldValue: "", NewValue: "a band"},
	}
	s.Require().NoError(s.svc.RecordRevision("artist", 31, user.ID, changes, "moved"))

	var stored adminm.Revision
	s.Require().NoError(s.db.Where("entity_type = ?", "artist").First(&stored).Error)

	revisions, _, err := s.svc.GetEntityHistory("artist", 31, 10, 0)
	s.Require().NoError(err)
	s.Require().Len(revisions, 1)
	s.JSONEq(string(*stored.FieldChanges), string(*revisions[0].FieldChanges))
}

func (s *RevisionServiceIntegrationTestSuite) TestGetRevision_RedactsUnverifiedVenueAddress() {
	user := s.createTestUser()
	venue := s.createTestVenue("Unverified Room")

	s.Require().NoError(s.svc.RecordRevision("venue", venue.ID, user.ID, addressChanges(), "moved"))

	var stored adminm.Revision
	s.Require().NoError(s.db.First(&stored).Error)

	revision, err := s.svc.GetRevision(stored.ID)
	s.Require().NoError(err)
	s.Require().NotNil(revision)
	s.assertAddressMasked(*revision)
}

func (s *RevisionServiceIntegrationTestSuite) TestGetUserRevisions_RedactsUnverifiedVenueAddress() {
	user := s.createTestUser()
	unverified := s.createTestVenue("Unverified Room")
	verified := s.createVerifiedTestVenue("Verified Room")

	s.Require().NoError(s.svc.RecordRevision("venue", unverified.ID, user.ID, addressChanges(), "moved"))
	s.Require().NoError(s.svc.RecordRevision("venue", verified.ID, user.ID, addressChanges(), "moved"))

	revisions, _, err := s.svc.GetUserRevisions(user.ID, 10, 0)
	s.Require().NoError(err)
	s.Require().Len(revisions, 2)

	// One page, two venues, opposite verdicts — proves the batched lookup keys
	// each revision to its OWN venue rather than to the page as a whole.
	byEntity := map[uint]adminm.Revision{}
	for _, r := range revisions {
		byEntity[r.EntityID] = r
	}
	s.assertAddressMasked(byEntity[unverified.ID])
	s.Equal("1234 Secret St", s.changesFor(byEntity[verified.ID])["address"].NewValue)
}

// The stored row is never touched, so it is still the truth an admin rollback
// restores. If Rollback read the served copy it would write "(hidden)" into
// venues.address.
func (s *RevisionServiceIntegrationTestSuite) TestRollback_RestoresRealAddressForUnverifiedVenue() {
	user := s.createTestUser()
	adminUser := s.createTestUser()
	venue := s.createTestVenue("Unverified Room")

	s.Require().NoError(s.svc.RecordRevision("venue", venue.ID, user.ID, addressChanges(), "moved"))
	s.Require().NoError(s.db.Model(&catalogm.Venue{}).Where("id = ?", venue.ID).
		Update("address", "1234 Secret St").Error)

	var stored adminm.Revision
	s.Require().NoError(s.db.First(&stored).Error)

	s.Require().NoError(s.svc.Rollback(stored.ID, adminUser.ID))

	var restored catalogm.Venue
	s.Require().NoError(s.db.First(&restored, venue.ID).Error)
	s.Require().NotNil(restored.Address)
	s.Equal("1 Old St", *restored.Address, "rollback must restore the real address, not the mask")
}
