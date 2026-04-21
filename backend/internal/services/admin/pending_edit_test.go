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
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/testutil"
)

// =============================================================================
// UNIT TESTS (No Database Required)
// =============================================================================

func TestIsValidPendingEditEntityType(t *testing.T) {
	assert.True(t, models.IsValidPendingEditEntityType("artist"))
	assert.True(t, models.IsValidPendingEditEntityType("venue"))
	assert.True(t, models.IsValidPendingEditEntityType("festival"))
	assert.True(t, models.IsValidPendingEditEntityType("release"))
	assert.False(t, models.IsValidPendingEditEntityType("show"))
	assert.False(t, models.IsValidPendingEditEntityType(""))
	assert.False(t, models.IsValidPendingEditEntityType("label"))
}

// =============================================================================
// INTEGRATION TESTS (With Real Database)
// =============================================================================

// mockEmailServiceForPendingEdit implements contracts.EmailServiceInterface for testing.
type mockEmailServiceForPendingEdit struct {
	configured             bool
	editApprovedCalls      []editApprovedCall
	editRejectedCalls      []editRejectedCall
	editApprovedErr        error
	editRejectedErr        error
}

type editApprovedCall struct {
	ToEmail    string
	Username   string
	EntityType string
	EntityName string
	EntityURL  string
}

type editRejectedCall struct {
	ToEmail         string
	Username        string
	EntityType      string
	EntityName      string
	RejectionReason string
}

func (m *mockEmailServiceForPendingEdit) IsConfigured() bool { return m.configured }
func (m *mockEmailServiceForPendingEdit) SendVerificationEmail(_, _ string) error { return nil }
func (m *mockEmailServiceForPendingEdit) SendMagicLinkEmail(_, _ string) error { return nil }
func (m *mockEmailServiceForPendingEdit) SendAccountRecoveryEmail(_, _ string, _ int) error {
	return nil
}
func (m *mockEmailServiceForPendingEdit) SendShowReminderEmail(_, _, _, _ string, _ time.Time, _ []string) error {
	return nil
}
func (m *mockEmailServiceForPendingEdit) SendFilterNotificationEmail(_, _, _, _ string) error {
	return nil
}
func (m *mockEmailServiceForPendingEdit) SendTierPromotionEmail(_, _, _, _, _ string, _ []string) error {
	return nil
}
func (m *mockEmailServiceForPendingEdit) SendTierDemotionEmail(_, _, _, _, _ string) error {
	return nil
}
func (m *mockEmailServiceForPendingEdit) SendTierDemotionWarningEmail(_, _, _ string, _, _ float64) error {
	return nil
}
func (m *mockEmailServiceForPendingEdit) SendEditApprovedEmail(toEmail, username, entityType, entityName, entityURL string) error {
	m.editApprovedCalls = append(m.editApprovedCalls, editApprovedCall{
		ToEmail: toEmail, Username: username, EntityType: entityType,
		EntityName: entityName, EntityURL: entityURL,
	})
	return m.editApprovedErr
}
func (m *mockEmailServiceForPendingEdit) SendEditRejectedEmail(toEmail, username, entityType, entityName, rejectionReason string) error {
	m.editRejectedCalls = append(m.editRejectedCalls, editRejectedCall{
		ToEmail: toEmail, Username: username, EntityType: entityType,
		EntityName: entityName, RejectionReason: rejectionReason,
	})
	return m.editRejectedErr
}

type PendingEditServiceIntegrationTestSuite struct {
	suite.Suite
	testDB       *testutil.TestDatabase
	db           *gorm.DB
	svc          *PendingEditService
	revisionSvc  *RevisionService
	mockEmail    *mockEmailServiceForPendingEdit
}

func (s *PendingEditServiceIntegrationTestSuite) SetupSuite() {
	s.testDB = testutil.SetupTestPostgres(s.T())
	s.db = s.testDB.DB
	s.revisionSvc = NewRevisionService(s.db)
	s.mockEmail = &mockEmailServiceForPendingEdit{configured: true}
	s.svc = NewPendingEditService(s.db, s.revisionSvc, s.mockEmail, "http://localhost:3000")
}

func (s *PendingEditServiceIntegrationTestSuite) TearDownSuite() {
	s.testDB.Cleanup()
}

func (s *PendingEditServiceIntegrationTestSuite) TearDownTest() {
	sqlDB, err := s.db.DB()
	s.Require().NoError(err)
	_, _ = sqlDB.Exec("DELETE FROM pending_entity_edits")
	_, _ = sqlDB.Exec("DELETE FROM revisions")
	_, _ = sqlDB.Exec("DELETE FROM artists")
	_, _ = sqlDB.Exec("DELETE FROM venues")
	_, _ = sqlDB.Exec("DELETE FROM festivals")
	_, _ = sqlDB.Exec("DELETE FROM users")
	// Reset mock email state between tests
	s.mockEmail.editApprovedCalls = nil
	s.mockEmail.editRejectedCalls = nil
	s.mockEmail.editApprovedErr = nil
	s.mockEmail.editRejectedErr = nil
}

func TestPendingEditServiceIntegrationSuite(t *testing.T) {
	suite.Run(t, new(PendingEditServiceIntegrationTestSuite))
}

// =============================================================================
// HELPERS
// =============================================================================

func (s *PendingEditServiceIntegrationTestSuite) createTestUser() *models.User {
	user := &models.User{
		Email:         stringPtr(fmt.Sprintf("pe-user-%d@test.com", time.Now().UnixNano())),
		Username:      stringPtr(fmt.Sprintf("pe-user-%d", time.Now().UnixNano())),
		FirstName:     stringPtr("Test"),
		LastName:      stringPtr("User"),
		IsActive:      true,
		EmailVerified: true,
	}
	err := s.db.Create(user).Error
	s.Require().NoError(err)
	return user
}

func (s *PendingEditServiceIntegrationTestSuite) createTestArtist(name string) *models.Artist {
	slug := fmt.Sprintf("test-artist-%d", time.Now().UnixNano())
	artist := &models.Artist{
		Name: name,
		Slug: &slug,
	}
	err := s.db.Create(artist).Error
	s.Require().NoError(err)
	return artist
}

func (s *PendingEditServiceIntegrationTestSuite) createTestVenue(name string) *models.Venue {
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

func (s *PendingEditServiceIntegrationTestSuite) createTestFestival(name string) *models.Festival {
	slug := fmt.Sprintf("test-festival-%d", time.Now().UnixNano())
	festival := &models.Festival{
		Name:        name,
		Slug:        slug,
		SeriesSlug:  slug,
		EditionYear: 2026,
		StartDate:   "2026-06-01",
		EndDate:     "2026-06-03",
	}
	err := s.db.Create(festival).Error
	s.Require().NoError(err)
	return festival
}

func makeChanges(field, oldVal, newVal string) []models.FieldChange {
	return []models.FieldChange{{Field: field, OldValue: oldVal, NewValue: newVal}}
}

// =============================================================================
// CreatePendingEdit tests
// =============================================================================

func (s *PendingEditServiceIntegrationTestSuite) TestCreatePendingEdit_ArtistSuccess() {
	user := s.createTestUser()
	artist := s.createTestArtist("Old Name")

	resp, err := s.svc.CreatePendingEdit(&contracts.CreatePendingEditRequest{
		EntityType: "artist",
		EntityID:   artist.ID,
		UserID:     user.ID,
		Changes:    makeChanges("name", "Old Name", "New Name"),
		Summary:    "Fix artist name",
	})

	s.NoError(err)
	s.Require().NotNil(resp)
	s.Equal("artist", resp.EntityType)
	s.Equal(artist.ID, resp.EntityID)
	s.Equal(user.ID, resp.SubmittedBy)
	s.Equal(models.PendingEditStatusPending, resp.Status)
	s.Equal("Fix artist name", resp.Summary)
	s.Len(resp.FieldChanges, 1)
	s.Equal("name", resp.FieldChanges[0].Field)
	s.NotEmpty(resp.SubmitterName)
}

func (s *PendingEditServiceIntegrationTestSuite) TestCreatePendingEdit_VenueSuccess() {
	user := s.createTestUser()
	venue := s.createTestVenue("Test Venue")

	resp, err := s.svc.CreatePendingEdit(&contracts.CreatePendingEditRequest{
		EntityType: "venue",
		EntityID:   venue.ID,
		UserID:     user.ID,
		Changes: []models.FieldChange{
			{Field: "name", OldValue: "Test Venue", NewValue: "The Test Venue"},
			{Field: "city", OldValue: "Phoenix", NewValue: "Tempe"},
		},
		Summary: "Correct venue name and city",
	})

	s.NoError(err)
	s.Require().NotNil(resp)
	s.Equal("venue", resp.EntityType)
	s.Len(resp.FieldChanges, 2)
}

func (s *PendingEditServiceIntegrationTestSuite) TestCreatePendingEdit_FestivalSuccess() {
	user := s.createTestUser()
	festival := s.createTestFestival("Fest 2026")

	resp, err := s.svc.CreatePendingEdit(&contracts.CreatePendingEditRequest{
		EntityType: "festival",
		EntityID:   festival.ID,
		UserID:     user.ID,
		Changes:    makeChanges("description", "", "A great music festival"),
		Summary:    "Add festival description",
	})

	s.NoError(err)
	s.Require().NotNil(resp)
	s.Equal("festival", resp.EntityType)
}

func (s *PendingEditServiceIntegrationTestSuite) TestCreatePendingEdit_InvalidEntityType() {
	user := s.createTestUser()

	_, err := s.svc.CreatePendingEdit(&contracts.CreatePendingEditRequest{
		EntityType: "show",
		EntityID:   1,
		UserID:     user.ID,
		Changes:    makeChanges("name", "a", "b"),
		Summary:    "test",
	})

	s.Error(err)
	s.Contains(err.Error(), "invalid entity type")
}

func (s *PendingEditServiceIntegrationTestSuite) TestCreatePendingEdit_EntityNotFound() {
	user := s.createTestUser()

	_, err := s.svc.CreatePendingEdit(&contracts.CreatePendingEditRequest{
		EntityType: "artist",
		EntityID:   99999,
		UserID:     user.ID,
		Changes:    makeChanges("name", "a", "b"),
		Summary:    "test",
	})

	s.Error(err)
	s.Contains(err.Error(), "entity not found")
}

func (s *PendingEditServiceIntegrationTestSuite) TestCreatePendingEdit_EmptyChanges() {
	user := s.createTestUser()
	artist := s.createTestArtist("Test")

	_, err := s.svc.CreatePendingEdit(&contracts.CreatePendingEditRequest{
		EntityType: "artist",
		EntityID:   artist.ID,
		UserID:     user.ID,
		Changes:    []models.FieldChange{},
		Summary:    "test",
	})

	s.Error(err)
	s.Contains(err.Error(), "no changes provided")
}

func (s *PendingEditServiceIntegrationTestSuite) TestCreatePendingEdit_EmptySummary() {
	user := s.createTestUser()
	artist := s.createTestArtist("Test")

	_, err := s.svc.CreatePendingEdit(&contracts.CreatePendingEditRequest{
		EntityType: "artist",
		EntityID:   artist.ID,
		UserID:     user.ID,
		Changes:    makeChanges("name", "a", "b"),
		Summary:    "",
	})

	s.Error(err)
	s.Contains(err.Error(), "summary is required")
}

func (s *PendingEditServiceIntegrationTestSuite) TestCreatePendingEdit_DuplicatePending() {
	user := s.createTestUser()
	artist := s.createTestArtist("Test")

	// First edit succeeds
	_, err := s.svc.CreatePendingEdit(&contracts.CreatePendingEditRequest{
		EntityType: "artist",
		EntityID:   artist.ID,
		UserID:     user.ID,
		Changes:    makeChanges("name", "Test", "Test2"),
		Summary:    "First edit",
	})
	s.NoError(err)

	// Second pending edit for same entity by same user fails
	_, err = s.svc.CreatePendingEdit(&contracts.CreatePendingEditRequest{
		EntityType: "artist",
		EntityID:   artist.ID,
		UserID:     user.ID,
		Changes:    makeChanges("name", "Test", "Test3"),
		Summary:    "Second edit",
	})
	s.Error(err)
}

func (s *PendingEditServiceIntegrationTestSuite) TestCreatePendingEdit_DifferentUsersSameEntity() {
	user1 := s.createTestUser()
	user2 := s.createTestUser()
	artist := s.createTestArtist("Test")

	// User 1 creates edit
	_, err := s.svc.CreatePendingEdit(&contracts.CreatePendingEditRequest{
		EntityType: "artist",
		EntityID:   artist.ID,
		UserID:     user1.ID,
		Changes:    makeChanges("name", "Test", "Test2"),
		Summary:    "User 1 edit",
	})
	s.NoError(err)

	// User 2 can also create edit for same entity
	_, err = s.svc.CreatePendingEdit(&contracts.CreatePendingEditRequest{
		EntityType: "artist",
		EntityID:   artist.ID,
		UserID:     user2.ID,
		Changes:    makeChanges("name", "Test", "Test3"),
		Summary:    "User 2 edit",
	})
	s.NoError(err)
}

// =============================================================================
// GetPendingEdit tests
// =============================================================================

func (s *PendingEditServiceIntegrationTestSuite) TestGetPendingEdit_Found() {
	user := s.createTestUser()
	artist := s.createTestArtist("Test")

	created, err := s.svc.CreatePendingEdit(&contracts.CreatePendingEditRequest{
		EntityType: "artist",
		EntityID:   artist.ID,
		UserID:     user.ID,
		Changes:    makeChanges("name", "Test", "New"),
		Summary:    "test",
	})
	s.Require().NoError(err)

	resp, err := s.svc.GetPendingEdit(created.ID)
	s.NoError(err)
	s.Require().NotNil(resp)
	s.Equal(created.ID, resp.ID)
	s.Equal("artist", resp.EntityType)
	s.NotEmpty(resp.SubmitterName)
}

func (s *PendingEditServiceIntegrationTestSuite) TestGetPendingEdit_NotFound() {
	resp, err := s.svc.GetPendingEdit(99999)
	s.NoError(err)
	s.Nil(resp)
}

// =============================================================================
// GetPendingEditsForEntity tests
// =============================================================================

func (s *PendingEditServiceIntegrationTestSuite) TestGetPendingEditsForEntity_Success() {
	user1 := s.createTestUser()
	user2 := s.createTestUser()
	artist := s.createTestArtist("Test")

	// Two users submit edits for the same artist
	_, err := s.svc.CreatePendingEdit(&contracts.CreatePendingEditRequest{
		EntityType: "artist", EntityID: artist.ID, UserID: user1.ID,
		Changes: makeChanges("name", "Test", "A"), Summary: "edit 1",
	})
	s.Require().NoError(err)

	_, err = s.svc.CreatePendingEdit(&contracts.CreatePendingEditRequest{
		EntityType: "artist", EntityID: artist.ID, UserID: user2.ID,
		Changes: makeChanges("name", "Test", "B"), Summary: "edit 2",
	})
	s.Require().NoError(err)

	edits, err := s.svc.GetPendingEditsForEntity("artist", artist.ID)
	s.NoError(err)
	s.Len(edits, 2)
}

func (s *PendingEditServiceIntegrationTestSuite) TestGetPendingEditsForEntity_ExcludesNonPending() {
	user := s.createTestUser()
	reviewer := s.createTestUser()
	artist := s.createTestArtist("Test")

	created, err := s.svc.CreatePendingEdit(&contracts.CreatePendingEditRequest{
		EntityType: "artist", EntityID: artist.ID, UserID: user.ID,
		Changes: makeChanges("name", "Test", "Approved"), Summary: "will be approved",
	})
	s.Require().NoError(err)

	// Approve the edit
	_, err = s.svc.ApprovePendingEdit(created.ID, reviewer.ID)
	s.Require().NoError(err)

	// Only pending edits returned
	edits, err := s.svc.GetPendingEditsForEntity("artist", artist.ID)
	s.NoError(err)
	s.Len(edits, 0)
}

// =============================================================================
// GetUserPendingEdits tests
// =============================================================================

func (s *PendingEditServiceIntegrationTestSuite) TestGetUserPendingEdits_Success() {
	user := s.createTestUser()
	artist1 := s.createTestArtist("Artist 1")
	artist2 := s.createTestArtist("Artist 2")

	_, _ = s.svc.CreatePendingEdit(&contracts.CreatePendingEditRequest{
		EntityType: "artist", EntityID: artist1.ID, UserID: user.ID,
		Changes: makeChanges("name", "Artist 1", "A1"), Summary: "edit 1",
	})
	_, _ = s.svc.CreatePendingEdit(&contracts.CreatePendingEditRequest{
		EntityType: "artist", EntityID: artist2.ID, UserID: user.ID,
		Changes: makeChanges("name", "Artist 2", "A2"), Summary: "edit 2",
	})

	edits, total, err := s.svc.GetUserPendingEdits(user.ID, 10, 0)
	s.NoError(err)
	s.Equal(int64(2), total)
	s.Len(edits, 2)
}

func (s *PendingEditServiceIntegrationTestSuite) TestGetUserPendingEdits_Pagination() {
	user := s.createTestUser()
	for i := 0; i < 5; i++ {
		artist := s.createTestArtist(fmt.Sprintf("Artist %d", i))
		_, _ = s.svc.CreatePendingEdit(&contracts.CreatePendingEditRequest{
			EntityType: "artist", EntityID: artist.ID, UserID: user.ID,
			Changes: makeChanges("name", artist.Name, "new"), Summary: fmt.Sprintf("edit %d", i),
		})
	}

	edits, total, err := s.svc.GetUserPendingEdits(user.ID, 2, 0)
	s.NoError(err)
	s.Equal(int64(5), total)
	s.Len(edits, 2)

	edits2, _, err := s.svc.GetUserPendingEdits(user.ID, 2, 2)
	s.NoError(err)
	s.Len(edits2, 2)
	s.NotEqual(edits[0].ID, edits2[0].ID)
}

// =============================================================================
// ListPendingEdits tests
// =============================================================================

func (s *PendingEditServiceIntegrationTestSuite) TestListPendingEdits_DefaultFilters() {
	user := s.createTestUser()
	artist := s.createTestArtist("Test")
	venue := s.createTestVenue("Test Venue")

	_, _ = s.svc.CreatePendingEdit(&contracts.CreatePendingEditRequest{
		EntityType: "artist", EntityID: artist.ID, UserID: user.ID,
		Changes: makeChanges("name", "Test", "A"), Summary: "artist edit",
	})
	_, _ = s.svc.CreatePendingEdit(&contracts.CreatePendingEditRequest{
		EntityType: "venue", EntityID: venue.ID, UserID: user.ID,
		Changes: makeChanges("name", "Test Venue", "V"), Summary: "venue edit",
	})

	edits, total, err := s.svc.ListPendingEdits(nil)
	s.NoError(err)
	s.Equal(int64(2), total)
	s.Len(edits, 2)
}

func (s *PendingEditServiceIntegrationTestSuite) TestListPendingEdits_FilterByStatus() {
	user := s.createTestUser()
	reviewer := s.createTestUser()
	artist := s.createTestArtist("Test")

	created, _ := s.svc.CreatePendingEdit(&contracts.CreatePendingEditRequest{
		EntityType: "artist", EntityID: artist.ID, UserID: user.ID,
		Changes: makeChanges("name", "Test", "Approved"), Summary: "will approve",
	})
	_, _ = s.svc.ApprovePendingEdit(created.ID, reviewer.ID)

	// Only pending
	edits, total, err := s.svc.ListPendingEdits(&contracts.PendingEditFilters{Status: "pending"})
	s.NoError(err)
	s.Equal(int64(0), total)
	s.Len(edits, 0)

	// Only approved
	edits, total, err = s.svc.ListPendingEdits(&contracts.PendingEditFilters{Status: "approved"})
	s.NoError(err)
	s.Equal(int64(1), total)
	s.Len(edits, 1)
}

func (s *PendingEditServiceIntegrationTestSuite) TestListPendingEdits_FilterByEntityType() {
	user := s.createTestUser()
	artist := s.createTestArtist("Test")
	venue := s.createTestVenue("Test Venue")

	_, _ = s.svc.CreatePendingEdit(&contracts.CreatePendingEditRequest{
		EntityType: "artist", EntityID: artist.ID, UserID: user.ID,
		Changes: makeChanges("name", "Test", "A"), Summary: "artist",
	})
	_, _ = s.svc.CreatePendingEdit(&contracts.CreatePendingEditRequest{
		EntityType: "venue", EntityID: venue.ID, UserID: user.ID,
		Changes: makeChanges("name", "Test Venue", "V"), Summary: "venue",
	})

	edits, total, err := s.svc.ListPendingEdits(&contracts.PendingEditFilters{EntityType: "artist"})
	s.NoError(err)
	s.Equal(int64(1), total)
	s.Len(edits, 1)
	s.Equal("artist", edits[0].EntityType)
}

// =============================================================================
// ApprovePendingEdit tests
// =============================================================================

func (s *PendingEditServiceIntegrationTestSuite) TestApprovePendingEdit_AppliesChangesToArtist() {
	user := s.createTestUser()
	reviewer := s.createTestUser()
	artist := s.createTestArtist("Old Name")

	created, err := s.svc.CreatePendingEdit(&contracts.CreatePendingEditRequest{
		EntityType: "artist", EntityID: artist.ID, UserID: user.ID,
		Changes: []models.FieldChange{
			{Field: "name", OldValue: "Old Name", NewValue: "New Name"},
			{Field: "city", OldValue: nil, NewValue: "Phoenix"},
		},
		Summary: "Update artist info",
	})
	s.Require().NoError(err)

	resp, err := s.svc.ApprovePendingEdit(created.ID, reviewer.ID)
	s.NoError(err)
	s.Require().NotNil(resp)
	s.Equal(models.PendingEditStatusApproved, resp.Status)
	s.Require().NotNil(resp.ReviewedBy)
	s.Equal(reviewer.ID, *resp.ReviewedBy)
	s.NotNil(resp.ReviewedAt)

	// Verify entity was updated
	var updated models.Artist
	s.db.First(&updated, artist.ID)
	s.Equal("New Name", updated.Name)
	s.Require().NotNil(updated.City)
	s.Equal("Phoenix", *updated.City)
}

func (s *PendingEditServiceIntegrationTestSuite) TestApprovePendingEdit_AppliesChangesToVenue() {
	user := s.createTestUser()
	reviewer := s.createTestUser()
	venue := s.createTestVenue("Old Venue")

	created, err := s.svc.CreatePendingEdit(&contracts.CreatePendingEditRequest{
		EntityType: "venue", EntityID: venue.ID, UserID: user.ID,
		Changes: makeChanges("name", "Old Venue", "New Venue"),
		Summary: "Fix venue name",
	})
	s.Require().NoError(err)

	_, err = s.svc.ApprovePendingEdit(created.ID, reviewer.ID)
	s.NoError(err)

	var updated models.Venue
	s.db.First(&updated, venue.ID)
	s.Equal("New Venue", updated.Name)
}

func (s *PendingEditServiceIntegrationTestSuite) TestApprovePendingEdit_RecordsRevision() {
	user := s.createTestUser()
	reviewer := s.createTestUser()
	artist := s.createTestArtist("Test")

	created, _ := s.svc.CreatePendingEdit(&contracts.CreatePendingEditRequest{
		EntityType: "artist", EntityID: artist.ID, UserID: user.ID,
		Changes: makeChanges("name", "Test", "Updated"), Summary: "Update name",
	})

	_, err := s.svc.ApprovePendingEdit(created.ID, reviewer.ID)
	s.NoError(err)

	// Verify revision was created
	var revision models.Revision
	err = s.db.Where("entity_type = ? AND entity_id = ?", "artist", artist.ID).First(&revision).Error
	s.NoError(err)
	s.Equal(user.ID, revision.UserID) // Attributed to submitter, not reviewer
	s.Require().NotNil(revision.Summary)
	s.Equal("Update name", *revision.Summary)

	var changes []models.FieldChange
	s.Require().NoError(json.Unmarshal(*revision.FieldChanges, &changes))
	s.Len(changes, 1)
	s.Equal("name", changes[0].Field)
}

func (s *PendingEditServiceIntegrationTestSuite) TestApprovePendingEdit_NotFound() {
	_, err := s.svc.ApprovePendingEdit(99999, 1)
	s.Error(err)
	s.Contains(err.Error(), "pending edit not found")
}

func (s *PendingEditServiceIntegrationTestSuite) TestApprovePendingEdit_AlreadyApproved() {
	user := s.createTestUser()
	reviewer := s.createTestUser()
	artist := s.createTestArtist("Test")

	created, _ := s.svc.CreatePendingEdit(&contracts.CreatePendingEditRequest{
		EntityType: "artist", EntityID: artist.ID, UserID: user.ID,
		Changes: makeChanges("name", "Test", "New"), Summary: "test",
	})

	_, _ = s.svc.ApprovePendingEdit(created.ID, reviewer.ID)

	// Try to approve again
	_, err := s.svc.ApprovePendingEdit(created.ID, reviewer.ID)
	s.Error(err)
	s.Contains(err.Error(), "not pending")
}

func (s *PendingEditServiceIntegrationTestSuite) TestApprovePendingEdit_EntityDeleted() {
	user := s.createTestUser()
	reviewer := s.createTestUser()
	artist := s.createTestArtist("Will Delete")

	created, _ := s.svc.CreatePendingEdit(&contracts.CreatePendingEditRequest{
		EntityType: "artist", EntityID: artist.ID, UserID: user.ID,
		Changes: makeChanges("name", "Will Delete", "New"), Summary: "test",
	})

	// Delete the entity
	s.db.Delete(&artist)

	// Approve should fail because entity is gone
	_, err := s.svc.ApprovePendingEdit(created.ID, reviewer.ID)
	s.Error(err)
	s.Contains(err.Error(), "entity not found")
}

func (s *PendingEditServiceIntegrationTestSuite) TestApprovePendingEdit_AllowsNewPendingAfter() {
	user := s.createTestUser()
	reviewer := s.createTestUser()
	artist := s.createTestArtist("Test")

	// Create and approve first edit
	created, _ := s.svc.CreatePendingEdit(&contracts.CreatePendingEditRequest{
		EntityType: "artist", EntityID: artist.ID, UserID: user.ID,
		Changes: makeChanges("name", "Test", "V2"), Summary: "first edit",
	})
	_, _ = s.svc.ApprovePendingEdit(created.ID, reviewer.ID)

	// User can now submit another pending edit for same entity
	resp, err := s.svc.CreatePendingEdit(&contracts.CreatePendingEditRequest{
		EntityType: "artist", EntityID: artist.ID, UserID: user.ID,
		Changes: makeChanges("name", "V2", "V3"), Summary: "second edit",
	})
	s.NoError(err)
	s.NotNil(resp)
}

// =============================================================================
// RejectPendingEdit tests
// =============================================================================

func (s *PendingEditServiceIntegrationTestSuite) TestRejectPendingEdit_Success() {
	user := s.createTestUser()
	reviewer := s.createTestUser()
	artist := s.createTestArtist("Test")

	created, _ := s.svc.CreatePendingEdit(&contracts.CreatePendingEditRequest{
		EntityType: "artist", EntityID: artist.ID, UserID: user.ID,
		Changes: makeChanges("name", "Test", "Bad Name"), Summary: "bad edit",
	})

	resp, err := s.svc.RejectPendingEdit(created.ID, reviewer.ID, "Name doesn't follow our naming conventions")
	s.NoError(err)
	s.Require().NotNil(resp)
	s.Equal(models.PendingEditStatusRejected, resp.Status)
	s.Require().NotNil(resp.RejectionReason)
	s.Equal("Name doesn't follow our naming conventions", *resp.RejectionReason)
	s.Require().NotNil(resp.ReviewedBy)
	s.Equal(reviewer.ID, *resp.ReviewedBy)

	// Verify entity was NOT changed
	var artist2 models.Artist
	s.db.First(&artist2, artist.ID)
	s.Equal("Test", artist2.Name)
}

func (s *PendingEditServiceIntegrationTestSuite) TestRejectPendingEdit_EmptyReason() {
	user := s.createTestUser()
	reviewer := s.createTestUser()
	artist := s.createTestArtist("Test")

	created, _ := s.svc.CreatePendingEdit(&contracts.CreatePendingEditRequest{
		EntityType: "artist", EntityID: artist.ID, UserID: user.ID,
		Changes: makeChanges("name", "Test", "X"), Summary: "test",
	})

	_, err := s.svc.RejectPendingEdit(created.ID, reviewer.ID, "")
	s.Error(err)
	s.Contains(err.Error(), "rejection reason is required")
}

func (s *PendingEditServiceIntegrationTestSuite) TestRejectPendingEdit_NotFound() {
	_, err := s.svc.RejectPendingEdit(99999, 1, "reason")
	s.Error(err)
	s.Contains(err.Error(), "pending edit not found")
}

func (s *PendingEditServiceIntegrationTestSuite) TestRejectPendingEdit_AllowsNewPendingAfter() {
	user := s.createTestUser()
	reviewer := s.createTestUser()
	artist := s.createTestArtist("Test")

	created, _ := s.svc.CreatePendingEdit(&contracts.CreatePendingEditRequest{
		EntityType: "artist", EntityID: artist.ID, UserID: user.ID,
		Changes: makeChanges("name", "Test", "Bad"), Summary: "bad edit",
	})
	_, _ = s.svc.RejectPendingEdit(created.ID, reviewer.ID, "bad name")

	// User can submit a new edit after rejection
	resp, err := s.svc.CreatePendingEdit(&contracts.CreatePendingEditRequest{
		EntityType: "artist", EntityID: artist.ID, UserID: user.ID,
		Changes: makeChanges("name", "Test", "Better"), Summary: "improved edit",
	})
	s.NoError(err)
	s.NotNil(resp)
}

// =============================================================================
// CancelPendingEdit tests
// =============================================================================

func (s *PendingEditServiceIntegrationTestSuite) TestCancelPendingEdit_Success() {
	user := s.createTestUser()
	artist := s.createTestArtist("Test")

	created, _ := s.svc.CreatePendingEdit(&contracts.CreatePendingEditRequest{
		EntityType: "artist", EntityID: artist.ID, UserID: user.ID,
		Changes: makeChanges("name", "Test", "New"), Summary: "will cancel",
	})

	err := s.svc.CancelPendingEdit(created.ID, user.ID)
	s.NoError(err)

	// Verify it's deleted
	resp, err := s.svc.GetPendingEdit(created.ID)
	s.NoError(err)
	s.Nil(resp)
}

func (s *PendingEditServiceIntegrationTestSuite) TestCancelPendingEdit_WrongUser() {
	user := s.createTestUser()
	otherUser := s.createTestUser()
	artist := s.createTestArtist("Test")

	created, _ := s.svc.CreatePendingEdit(&contracts.CreatePendingEditRequest{
		EntityType: "artist", EntityID: artist.ID, UserID: user.ID,
		Changes: makeChanges("name", "Test", "New"), Summary: "test",
	})

	err := s.svc.CancelPendingEdit(created.ID, otherUser.ID)
	s.Error(err)
	s.Contains(err.Error(), "only the submitter")
}

func (s *PendingEditServiceIntegrationTestSuite) TestCancelPendingEdit_NotFound() {
	err := s.svc.CancelPendingEdit(99999, 1)
	s.Error(err)
	s.Contains(err.Error(), "pending edit not found")
}

func (s *PendingEditServiceIntegrationTestSuite) TestCancelPendingEdit_AlreadyApproved() {
	user := s.createTestUser()
	reviewer := s.createTestUser()
	artist := s.createTestArtist("Test")

	created, _ := s.svc.CreatePendingEdit(&contracts.CreatePendingEditRequest{
		EntityType: "artist", EntityID: artist.ID, UserID: user.ID,
		Changes: makeChanges("name", "Test", "New"), Summary: "test",
	})
	_, _ = s.svc.ApprovePendingEdit(created.ID, reviewer.ID)

	err := s.svc.CancelPendingEdit(created.ID, user.ID)
	s.Error(err)
	s.Contains(err.Error(), "not pending")
}

func (s *PendingEditServiceIntegrationTestSuite) TestCancelPendingEdit_AllowsNewPendingAfter() {
	user := s.createTestUser()
	artist := s.createTestArtist("Test")

	created, _ := s.svc.CreatePendingEdit(&contracts.CreatePendingEditRequest{
		EntityType: "artist", EntityID: artist.ID, UserID: user.ID,
		Changes: makeChanges("name", "Test", "V1"), Summary: "will cancel",
	})
	_ = s.svc.CancelPendingEdit(created.ID, user.ID)

	// User can create a new pending edit
	resp, err := s.svc.CreatePendingEdit(&contracts.CreatePendingEditRequest{
		EntityType: "artist", EntityID: artist.ID, UserID: user.ID,
		Changes: makeChanges("name", "Test", "V2"), Summary: "new edit",
	})
	s.NoError(err)
	s.NotNil(resp)
}

// =============================================================================
// displayName helper tests
// =============================================================================

func TestDisplayName(t *testing.T) {
	username := "testuser"
	first := "John"
	last := "Doe"
	email := "john@test.com"

	t.Run("PreferUsername", func(t *testing.T) {
		u := &models.User{Username: &username, FirstName: &first, Email: &email}
		assert.Equal(t, "testuser", displayName(u))
	})

	t.Run("FallbackToFirstLast", func(t *testing.T) {
		u := &models.User{FirstName: &first, LastName: &last, Email: &email}
		assert.Equal(t, "John Doe", displayName(u))
	})

	t.Run("FallbackToFirstOnly", func(t *testing.T) {
		u := &models.User{FirstName: &first, Email: &email}
		assert.Equal(t, "John", displayName(u))
	})

	t.Run("FallbackToEmail", func(t *testing.T) {
		u := &models.User{Email: &email}
		assert.Equal(t, "john@test.com", displayName(u))
	})

	t.Run("EmptyUser", func(t *testing.T) {
		u := &models.User{}
		assert.Equal(t, "", displayName(u))
	})
}

// =============================================================================
// Email notification tests
// =============================================================================

func (s *PendingEditServiceIntegrationTestSuite) TestApprovePendingEdit_SendsApprovalEmail() {
	user := s.createTestUser()
	reviewer := s.createTestUser()
	artist := s.createTestArtist("Cool Band")

	created, err := s.svc.CreatePendingEdit(&contracts.CreatePendingEditRequest{
		EntityType: "artist", EntityID: artist.ID, UserID: user.ID,
		Changes: makeChanges("name", "Cool Band", "The Cool Band"), Summary: "Add 'The'",
	})
	s.Require().NoError(err)

	_, err = s.svc.ApprovePendingEdit(created.ID, reviewer.ID)
	s.NoError(err)

	// Verify approval email was sent
	s.Require().Len(s.mockEmail.editApprovedCalls, 1)
	call := s.mockEmail.editApprovedCalls[0]
	s.Equal(*user.Email, call.ToEmail)
	s.Equal("artist", call.EntityType)
	s.Equal("The Cool Band", call.EntityName) // Entity was updated, so name should reflect the update
	s.Contains(call.EntityURL, "/artists/")
	s.Contains(call.EntityURL, "http://localhost:3000")
}

func (s *PendingEditServiceIntegrationTestSuite) TestApprovePendingEdit_EmailErrorDoesNotFail() {
	user := s.createTestUser()
	reviewer := s.createTestUser()
	artist := s.createTestArtist("Test Band")

	created, err := s.svc.CreatePendingEdit(&contracts.CreatePendingEditRequest{
		EntityType: "artist", EntityID: artist.ID, UserID: user.ID,
		Changes: makeChanges("name", "Test Band", "Better Name"), Summary: "improve name",
	})
	s.Require().NoError(err)

	// Make email service return an error
	s.mockEmail.editApprovedErr = fmt.Errorf("email API is down")

	// Approval should still succeed despite email error
	resp, err := s.svc.ApprovePendingEdit(created.ID, reviewer.ID)
	s.NoError(err)
	s.NotNil(resp)
	s.Equal(models.PendingEditStatusApproved, resp.Status)
	s.Len(s.mockEmail.editApprovedCalls, 1) // Email was attempted
}

func (s *PendingEditServiceIntegrationTestSuite) TestRejectPendingEdit_SendsRejectionEmail() {
	user := s.createTestUser()
	reviewer := s.createTestUser()
	venue := s.createTestVenue("Great Venue")

	created, err := s.svc.CreatePendingEdit(&contracts.CreatePendingEditRequest{
		EntityType: "venue", EntityID: venue.ID, UserID: user.ID,
		Changes: makeChanges("name", "Great Venue", "Bad Name"), Summary: "rename",
	})
	s.Require().NoError(err)

	_, err = s.svc.RejectPendingEdit(created.ID, reviewer.ID, "Name does not match official venue name")
	s.NoError(err)

	// Verify rejection email was sent
	s.Require().Len(s.mockEmail.editRejectedCalls, 1)
	call := s.mockEmail.editRejectedCalls[0]
	s.Equal(*user.Email, call.ToEmail)
	s.Equal("venue", call.EntityType)
	s.Equal("Great Venue", call.EntityName) // Entity was NOT updated on rejection
	s.Equal("Name does not match official venue name", call.RejectionReason)
}

func (s *PendingEditServiceIntegrationTestSuite) TestRejectPendingEdit_EmailErrorDoesNotFail() {
	user := s.createTestUser()
	reviewer := s.createTestUser()
	artist := s.createTestArtist("Test Artist")

	created, err := s.svc.CreatePendingEdit(&contracts.CreatePendingEditRequest{
		EntityType: "artist", EntityID: artist.ID, UserID: user.ID,
		Changes: makeChanges("name", "Test Artist", "Wrong"), Summary: "bad edit",
	})
	s.Require().NoError(err)

	// Make email service return an error
	s.mockEmail.editRejectedErr = fmt.Errorf("email API is down")

	// Rejection should still succeed despite email error
	resp, err := s.svc.RejectPendingEdit(created.ID, reviewer.ID, "incorrect info")
	s.NoError(err)
	s.NotNil(resp)
	s.Equal(models.PendingEditStatusRejected, resp.Status)
	s.Len(s.mockEmail.editRejectedCalls, 1) // Email was attempted
}

func (s *PendingEditServiceIntegrationTestSuite) TestApprovePendingEdit_VenueEmailHasCorrectURL() {
	user := s.createTestUser()
	reviewer := s.createTestUser()
	venue := s.createTestVenue("The Rebel Lounge")

	created, err := s.svc.CreatePendingEdit(&contracts.CreatePendingEditRequest{
		EntityType: "venue", EntityID: venue.ID, UserID: user.ID,
		Changes: makeChanges("city", "Phoenix", "Tempe"), Summary: "fix city",
	})
	s.Require().NoError(err)

	_, err = s.svc.ApprovePendingEdit(created.ID, reviewer.ID)
	s.NoError(err)

	s.Require().Len(s.mockEmail.editApprovedCalls, 1)
	call := s.mockEmail.editApprovedCalls[0]
	s.Contains(call.EntityURL, "/venues/")
}

func (s *PendingEditServiceIntegrationTestSuite) TestApprovePendingEdit_FestivalEmailHasCorrectURL() {
	user := s.createTestUser()
	reviewer := s.createTestUser()
	festival := s.createTestFestival("Fest 2026")

	created, err := s.svc.CreatePendingEdit(&contracts.CreatePendingEditRequest{
		EntityType: "festival", EntityID: festival.ID, UserID: user.ID,
		Changes: makeChanges("description", "", "Great festival"), Summary: "add desc",
	})
	s.Require().NoError(err)

	_, err = s.svc.ApprovePendingEdit(created.ID, reviewer.ID)
	s.NoError(err)

	s.Require().Len(s.mockEmail.editApprovedCalls, 1)
	call := s.mockEmail.editApprovedCalls[0]
	s.Contains(call.EntityURL, "/festivals/")
}

// =============================================================================
// Nil email service tests (unit tests, no DB)
// =============================================================================

func TestPendingEditService_NilEmailServiceDoesNotPanic(t *testing.T) {
	// Constructor with nil email service should work fine
	svc := NewPendingEditService(nil, nil, nil, "")
	assert.NotNil(t, svc)

	// sendApprovalEmail and sendRejectionEmail should not panic with nil email service
	svc.sendApprovalEmail(&models.PendingEntityEdit{SubmittedBy: 1, EntityType: "artist", EntityID: 1})
	svc.sendRejectionEmail(&models.PendingEntityEdit{SubmittedBy: 1, EntityType: "artist", EntityID: 1}, "reason")
}

func TestPendingEditService_UnconfiguredEmailServiceDoesNotPanic(t *testing.T) {
	mockEmail := &mockEmailServiceForPendingEdit{configured: false}
	svc := NewPendingEditService(nil, nil, mockEmail, "http://localhost:3000")

	// Should return early without attempting to send
	svc.sendApprovalEmail(&models.PendingEntityEdit{SubmittedBy: 1, EntityType: "artist", EntityID: 1})
	svc.sendRejectionEmail(&models.PendingEntityEdit{SubmittedBy: 1, EntityType: "artist", EntityID: 1}, "reason")

	assert.Len(t, mockEmail.editApprovedCalls, 0)
	assert.Len(t, mockEmail.editRejectedCalls, 0)
}
