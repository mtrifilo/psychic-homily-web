package admin

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"

	"psychic-homily-backend/internal/models"
	"psychic-homily-backend/internal/testutil"
)

// =============================================================================
// UNIT TESTS (No Database Required)
// =============================================================================

func TestNewRevisionService(t *testing.T) {
	svc := NewRevisionService(nil)
	assert.NotNil(t, svc)
}

func TestRevisionService_NilDatabase(t *testing.T) {
	svc := &RevisionService{db: nil}

	t.Run("RecordRevision", func(t *testing.T) {
		changes := []models.FieldChange{{Field: "name", OldValue: "old", NewValue: "new"}}
		err := svc.RecordRevision("artist", 1, 1, changes, "test")
		assert.Error(t, err)
		assert.Equal(t, "database not initialized", err.Error())
	})

	t.Run("GetEntityHistory", func(t *testing.T) {
		revisions, total, err := svc.GetEntityHistory("artist", 1, 10, 0)
		assert.Error(t, err)
		assert.Equal(t, "database not initialized", err.Error())
		assert.Nil(t, revisions)
		assert.Zero(t, total)
	})

	t.Run("GetRevision", func(t *testing.T) {
		revision, err := svc.GetRevision(1)
		assert.Error(t, err)
		assert.Equal(t, "database not initialized", err.Error())
		assert.Nil(t, revision)
	})

	t.Run("GetUserRevisions", func(t *testing.T) {
		revisions, total, err := svc.GetUserRevisions(1, 10, 0)
		assert.Error(t, err)
		assert.Equal(t, "database not initialized", err.Error())
		assert.Nil(t, revisions)
		assert.Zero(t, total)
	})

	t.Run("Rollback", func(t *testing.T) {
		err := svc.Rollback(1, 1)
		assert.Error(t, err)
		assert.Equal(t, "database not initialized", err.Error())
	})
}

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

func (s *RevisionServiceIntegrationTestSuite) createTestUser() *models.User {
	user := &models.User{
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

func (s *RevisionServiceIntegrationTestSuite) createTestVenue(name string) *models.Venue {
	slug := fmt.Sprintf("test-venue-%d", time.Now().UnixNano())
	venue := &models.Venue{
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

	changes := []models.FieldChange{
		{Field: "name", OldValue: "Old Name", NewValue: "New Name"},
		{Field: "city", OldValue: "Phoenix", NewValue: "Tempe"},
	}

	err := s.svc.RecordRevision("venue", 42, user.ID, changes, "Updated venue info")
	s.NoError(err)

	// Verify the revision was created
	var revision models.Revision
	err = s.db.First(&revision).Error
	s.Require().NoError(err)
	s.Equal("venue", revision.EntityType)
	s.Equal(uint(42), revision.EntityID)
	s.Equal(user.ID, revision.UserID)
	s.Require().NotNil(revision.Summary)
	s.Equal("Updated venue info", *revision.Summary)
	s.Require().NotNil(revision.FieldChanges)

	// Verify field changes deserialization
	var parsedChanges []models.FieldChange
	err = json.Unmarshal(*revision.FieldChanges, &parsedChanges)
	s.NoError(err)
	s.Len(parsedChanges, 2)
	s.Equal("name", parsedChanges[0].Field)
	s.Equal("Old Name", parsedChanges[0].OldValue)
	s.Equal("New Name", parsedChanges[0].NewValue)
}

func (s *RevisionServiceIntegrationTestSuite) TestRecordRevision_EmptyChanges() {
	user := s.createTestUser()

	err := s.svc.RecordRevision("artist", 1, user.ID, []models.FieldChange{}, "no changes")
	s.NoError(err)

	// Verify no revision was created
	var count int64
	s.db.Model(&models.Revision{}).Count(&count)
	s.Equal(int64(0), count)
}

func (s *RevisionServiceIntegrationTestSuite) TestRecordRevision_NilChanges() {
	user := s.createTestUser()

	err := s.svc.RecordRevision("artist", 1, user.ID, nil, "no changes")
	s.NoError(err)

	var count int64
	s.db.Model(&models.Revision{}).Count(&count)
	s.Equal(int64(0), count)
}

func (s *RevisionServiceIntegrationTestSuite) TestRecordRevision_EmptySummary() {
	user := s.createTestUser()

	changes := []models.FieldChange{
		{Field: "name", OldValue: "Old", NewValue: "New"},
	}

	err := s.svc.RecordRevision("artist", 1, user.ID, changes, "")
	s.NoError(err)

	var revision models.Revision
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
		changes := []models.FieldChange{
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
		changes := []models.FieldChange{
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

	changes := []models.FieldChange{{Field: "name", OldValue: "a", NewValue: "b"}}
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

	changes := []models.FieldChange{{Field: "name", OldValue: "a", NewValue: "b"}}
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

	changes := []models.FieldChange{
		{Field: "name", OldValue: "Old", NewValue: "New"},
	}
	err := s.svc.RecordRevision("artist", 5, user.ID, changes, "test edit")
	s.Require().NoError(err)

	// Get the created revision's ID
	var created models.Revision
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

	changes := []models.FieldChange{{Field: "name", OldValue: "a", NewValue: "b"}}

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

	changes := []models.FieldChange{{Field: "name", OldValue: "a", NewValue: "b"}}
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
	changes := []models.FieldChange{
		{Field: "name", OldValue: "Original Name", NewValue: "Changed Name"},
	}
	err := s.svc.RecordRevision("venue", venue.ID, user.ID, changes, "renamed venue")
	s.Require().NoError(err)

	// Apply the change to the venue (simulating what an edit handler would do)
	s.db.Table("venues").Where("id = ?", venue.ID).Updates(map[string]interface{}{
		"name": "Changed Name",
	})

	// Verify venue has changed name
	var updatedVenue models.Venue
	s.db.First(&updatedVenue, venue.ID)
	s.Equal("Changed Name", updatedVenue.Name)

	// Get the revision to rollback
	var revision models.Revision
	s.db.Where("entity_type = ? AND entity_id = ?", "venue", venue.ID).First(&revision)

	// Rollback
	err = s.svc.Rollback(revision.ID, adminUser.ID)
	s.NoError(err)

	// Verify venue name is restored
	var restoredVenue models.Venue
	s.db.First(&restoredVenue, venue.ID)
	s.Equal("Original Name", restoredVenue.Name)

	// Verify a rollback revision was created
	var allRevisions []models.Revision
	s.db.Where("entity_type = ? AND entity_id = ?", "venue", venue.ID).
		Order("created_at DESC").
		Find(&allRevisions)
	s.Len(allRevisions, 2) // original + rollback

	rollbackRevision := allRevisions[0]
	s.Equal(adminUser.ID, rollbackRevision.UserID)
	s.Require().NotNil(rollbackRevision.Summary)
	s.Contains(*rollbackRevision.Summary, "Rollback of revision #")

	// Verify rollback revision has inverted changes
	var rollbackChanges []models.FieldChange
	err = json.Unmarshal(*rollbackRevision.FieldChanges, &rollbackChanges)
	s.NoError(err)
	s.Len(rollbackChanges, 1)
	s.Equal("name", rollbackChanges[0].Field)
	s.Equal("Changed Name", rollbackChanges[0].OldValue)
	s.Equal("Original Name", rollbackChanges[0].NewValue)
}

func (s *RevisionServiceIntegrationTestSuite) TestRollback_RevisionNotFound() {
	err := s.svc.Rollback(99999, 1)
	s.Error(err)
	s.Contains(err.Error(), "revision not found")
}

func (s *RevisionServiceIntegrationTestSuite) TestRollback_EntityNotFound() {
	user := s.createTestUser()

	// Record a revision for an entity that doesn't exist
	changes := []models.FieldChange{
		{Field: "name", OldValue: "Old", NewValue: "New"},
	}
	err := s.svc.RecordRevision("venue", 99999, user.ID, changes, "test")
	s.Require().NoError(err)

	var revision models.Revision
	s.db.First(&revision)

	err = s.svc.Rollback(revision.ID, user.ID)
	s.Error(err)
	s.Contains(err.Error(), "entity not found")
}
