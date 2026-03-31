package user

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"

	"psychic-homily-backend/internal/models"
	adminsvc "psychic-homily-backend/internal/services/admin"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/testutil"
)

// =============================================================================
// UNIT TESTS (No Database Required)
// =============================================================================

func TestValidatePrivacySettings(t *testing.T) {
	t.Run("Valid_Defaults", func(t *testing.T) {
		err := ValidatePrivacySettings(contracts.DefaultPrivacySettings())
		assert.NoError(t, err)
	})

	t.Run("Valid_AllVisible", func(t *testing.T) {
		ps := contracts.PrivacySettings{
			Contributions:   contracts.PrivacyVisible,
			SavedShows:      contracts.PrivacyVisible,
			Attendance:      contracts.PrivacyVisible,
			Following:       contracts.PrivacyVisible,
			Collections:     contracts.PrivacyVisible,
			LastActive:      contracts.PrivacyVisible,
			ProfileSections: contracts.PrivacyVisible,
		}
		assert.NoError(t, ValidatePrivacySettings(ps))
	})

	t.Run("Valid_AllHidden", func(t *testing.T) {
		ps := contracts.PrivacySettings{
			Contributions:   contracts.PrivacyHidden,
			SavedShows:      contracts.PrivacyHidden,
			Attendance:      contracts.PrivacyHidden,
			Following:       contracts.PrivacyHidden,
			Collections:     contracts.PrivacyHidden,
			LastActive:      contracts.PrivacyHidden,
			ProfileSections: contracts.PrivacyHidden,
		}
		assert.NoError(t, ValidatePrivacySettings(ps))
	})

	t.Run("Invalid_BadLevel", func(t *testing.T) {
		ps := contracts.DefaultPrivacySettings()
		ps.Contributions = "invalid"
		err := ValidatePrivacySettings(ps)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid privacy level")
	})

	t.Run("Invalid_CountOnly_LastActive", func(t *testing.T) {
		ps := contracts.DefaultPrivacySettings()
		ps.LastActive = contracts.PrivacyCountOnly
		err := ValidatePrivacySettings(ps)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "only supports 'visible' or 'hidden'")
	})

	t.Run("Invalid_CountOnly_ProfileSections", func(t *testing.T) {
		ps := contracts.DefaultPrivacySettings()
		ps.ProfileSections = contracts.PrivacyCountOnly
		err := ValidatePrivacySettings(ps)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "only supports 'visible' or 'hidden'")
	})

	t.Run("Valid_CountOnly_Contributions", func(t *testing.T) {
		ps := contracts.DefaultPrivacySettings()
		ps.Contributions = contracts.PrivacyCountOnly
		assert.NoError(t, ValidatePrivacySettings(ps))
	})
}

// =============================================================================
// INTEGRATION TESTS (With Real Database)
// =============================================================================

type ContributorProfileServiceIntegrationTestSuite struct {
	suite.Suite
	testDB         *testutil.TestDatabase
	db             *gorm.DB
	profileService *ContributorProfileService
	auditLog       *adminsvc.AuditLogService
}

func (suite *ContributorProfileServiceIntegrationTestSuite) SetupSuite() {
	suite.testDB = testutil.SetupTestPostgres(suite.T())
	suite.db = suite.testDB.DB

	suite.profileService = &ContributorProfileService{db: suite.testDB.DB}
	suite.auditLog = adminsvc.NewAuditLogService(suite.testDB.DB)
}

func (suite *ContributorProfileServiceIntegrationTestSuite) TearDownSuite() {
	suite.testDB.Cleanup()
}

func (suite *ContributorProfileServiceIntegrationTestSuite) TearDownTest() {
	sqlDB, err := suite.db.DB()
	suite.Require().NoError(err)
	_, _ = sqlDB.Exec("DELETE FROM user_profile_sections")
	_, _ = sqlDB.Exec("DELETE FROM audit_logs")
	_, _ = sqlDB.Exec("DELETE FROM pending_venue_edits")
	_, _ = sqlDB.Exec("DELETE FROM pending_entity_edits")
	_, _ = sqlDB.Exec("DELETE FROM tag_votes")
	_, _ = sqlDB.Exec("DELETE FROM entity_tags")
	_, _ = sqlDB.Exec("DELETE FROM tags")
	_, _ = sqlDB.Exec("DELETE FROM artist_relationship_votes")
	_, _ = sqlDB.Exec("DELETE FROM artist_relationships")
	_, _ = sqlDB.Exec("DELETE FROM request_votes")
	_, _ = sqlDB.Exec("DELETE FROM requests")
	_, _ = sqlDB.Exec("DELETE FROM collection_subscribers")
	_, _ = sqlDB.Exec("DELETE FROM collection_items")
	_, _ = sqlDB.Exec("DELETE FROM collections")
	_, _ = sqlDB.Exec("DELETE FROM user_bookmarks")
	_, _ = sqlDB.Exec("DELETE FROM revisions")
	_, _ = sqlDB.Exec("DELETE FROM entity_reports")
	_, _ = sqlDB.Exec("DELETE FROM show_reports")
	_, _ = sqlDB.Exec("DELETE FROM artist_reports")
	_, _ = sqlDB.Exec("DELETE FROM show_artists")
	_, _ = sqlDB.Exec("DELETE FROM show_venues")
	_, _ = sqlDB.Exec("DELETE FROM shows")
	_, _ = sqlDB.Exec("DELETE FROM venues")
	_, _ = sqlDB.Exec("DELETE FROM artists")
	_, _ = sqlDB.Exec("DELETE FROM releases")
	_, _ = sqlDB.Exec("DELETE FROM labels")
	_, _ = sqlDB.Exec("DELETE FROM festivals")
	_, _ = sqlDB.Exec("DELETE FROM users")
}

func TestContributorProfileServiceIntegrationTestSuite(t *testing.T) {
	suite.Run(t, new(ContributorProfileServiceIntegrationTestSuite))
}

// =============================================================================
// HELPERS
// =============================================================================

func (suite *ContributorProfileServiceIntegrationTestSuite) createTestUser(username string) *models.User {
	user := &models.User{
		Email:             stringPtr(fmt.Sprintf("%s-%d@test.com", username, time.Now().UnixNano())),
		Username:          stringPtr(username),
		FirstName:         stringPtr("Test"),
		LastName:          stringPtr("User"),
		Bio:               stringPtr("Music enthusiast"),
		ProfileVisibility: "public",
		IsActive:          true,
		EmailVerified:     true,
	}
	err := suite.db.Create(user).Error
	suite.Require().NoError(err)
	return user
}

func (suite *ContributorProfileServiceIntegrationTestSuite) createPrivateUser(username string) *models.User {
	user := suite.createTestUser(username)
	err := suite.db.Model(user).Update("profile_visibility", "private").Error
	suite.Require().NoError(err)
	user.ProfileVisibility = "private"
	return user
}

func (suite *ContributorProfileServiceIntegrationTestSuite) createShow(submittedBy uint, title string) *models.Show {
	show := &models.Show{
		Title:       title,
		SubmittedBy: &submittedBy,
		Status:      "approved",
		EventDate:   time.Now(),
	}
	err := suite.db.Create(show).Error
	suite.Require().NoError(err)
	return show
}

func (suite *ContributorProfileServiceIntegrationTestSuite) createVenue(submittedBy uint, name string) *models.Venue {
	venue := &models.Venue{
		Name:        name,
		City:        "Phoenix",
		State:       "AZ",
		SubmittedBy: &submittedBy,
	}
	err := suite.db.Create(venue).Error
	suite.Require().NoError(err)
	return venue
}

func (suite *ContributorProfileServiceIntegrationTestSuite) setPrivacySettings(userID uint, ps contracts.PrivacySettings) {
	raw, err := json.Marshal(ps)
	suite.Require().NoError(err)
	rawMsg := json.RawMessage(raw)
	err = suite.db.Model(&models.User{}).Where("id = ?", userID).Update("privacy_settings", &rawMsg).Error
	suite.Require().NoError(err)
}

// =============================================================================
// Group 1: GetPublicProfile
// =============================================================================

func (suite *ContributorProfileServiceIntegrationTestSuite) TestGetPublicProfile_Success() {
	user := suite.createTestUser("contributor1")

	profile, err := suite.profileService.GetPublicProfile("contributor1", nil)

	suite.Require().NoError(err)
	suite.Require().NotNil(profile)
	suite.Equal("contributor1", profile.Username)
	suite.Equal("Music enthusiast", *profile.Bio)
	suite.Equal("Test", *profile.FirstName)
	suite.Equal("public", profile.ProfileVisibility)
	suite.Equal(user.CreatedAt.Unix(), profile.JoinedAt.Unix())
}

func (suite *ContributorProfileServiceIntegrationTestSuite) TestGetPublicProfile_NotFound() {
	profile, err := suite.profileService.GetPublicProfile("nonexistent", nil)

	suite.Require().NoError(err)
	suite.Nil(profile)
}

func (suite *ContributorProfileServiceIntegrationTestSuite) TestGetPublicProfile_PrivateProfile_Anonymous() {
	suite.createPrivateUser("privateperson")

	profile, err := suite.profileService.GetPublicProfile("privateperson", nil)

	suite.Require().NoError(err)
	suite.Nil(profile, "Private profiles should not be visible to anonymous users")
}

func (suite *ContributorProfileServiceIntegrationTestSuite) TestGetPublicProfile_PrivateProfile_OtherUser() {
	suite.createPrivateUser("privateperson2")
	otherUser := suite.createTestUser("otheruser")

	profile, err := suite.profileService.GetPublicProfile("privateperson2", &otherUser.ID)

	suite.Require().NoError(err)
	suite.Nil(profile, "Private profiles should not be visible to other users")
}

func (suite *ContributorProfileServiceIntegrationTestSuite) TestGetPublicProfile_PrivateProfile_Owner() {
	owner := suite.createPrivateUser("privateperson3")

	profile, err := suite.profileService.GetPublicProfile("privateperson3", &owner.ID)

	suite.Require().NoError(err)
	suite.Require().NotNil(profile, "Private profiles should be visible to the owner")
	suite.Equal("privateperson3", profile.Username)
	suite.Equal("private", profile.ProfileVisibility)
}

func (suite *ContributorProfileServiceIntegrationTestSuite) TestGetPublicProfile_IncludesStats() {
	user := suite.createTestUser("statsuser")
	suite.createShow(user.ID, "Show 1")
	suite.createShow(user.ID, "Show 2")
	suite.createVenue(user.ID, "Test Venue")

	profile, err := suite.profileService.GetPublicProfile("statsuser", nil)

	suite.Require().NoError(err)
	suite.Require().NotNil(profile)
	suite.Equal(int64(2), profile.Stats.ShowsSubmitted)
	suite.Equal(int64(1), profile.Stats.VenuesSubmitted)
	suite.Equal(int64(3), profile.Stats.TotalContributions)
}

// =============================================================================
// Group 2: GetOwnProfile
// =============================================================================

func (suite *ContributorProfileServiceIntegrationTestSuite) TestGetOwnProfile_Success() {
	user := suite.createTestUser("ownprofile")

	profile, err := suite.profileService.GetOwnProfile(user.ID)

	suite.Require().NoError(err)
	suite.Require().NotNil(profile)
	suite.Equal("ownprofile", profile.Username)
}

func (suite *ContributorProfileServiceIntegrationTestSuite) TestGetOwnProfile_PrivateBypassesVisibility() {
	user := suite.createPrivateUser("ownprivate")

	profile, err := suite.profileService.GetOwnProfile(user.ID)

	suite.Require().NoError(err)
	suite.Require().NotNil(profile, "GetOwnProfile should always work regardless of visibility")
	suite.Equal("ownprivate", profile.Username)
	suite.Equal("private", profile.ProfileVisibility)
}

func (suite *ContributorProfileServiceIntegrationTestSuite) TestGetOwnProfile_NotFound() {
	profile, err := suite.profileService.GetOwnProfile(99999)

	suite.Require().NoError(err)
	suite.Nil(profile)
}

// =============================================================================
// Group 3: GetContributionStats
// =============================================================================

func (suite *ContributorProfileServiceIntegrationTestSuite) TestGetContributionStats_Empty() {
	user := suite.createTestUser("emptystats")

	stats, err := suite.profileService.GetContributionStats(user.ID)

	suite.Require().NoError(err)
	suite.Require().NotNil(stats)
	suite.Equal(int64(0), stats.TotalContributions)
	suite.Equal(int64(0), stats.ShowsSubmitted)
	suite.Equal(int64(0), stats.VenuesSubmitted)
}

func (suite *ContributorProfileServiceIntegrationTestSuite) TestGetContributionStats_ShowsAndVenues() {
	user := suite.createTestUser("showvenueuser")
	suite.createShow(user.ID, "Show A")
	suite.createShow(user.ID, "Show B")
	suite.createShow(user.ID, "Show C")
	suite.createVenue(user.ID, "Venue A")

	stats, err := suite.profileService.GetContributionStats(user.ID)

	suite.Require().NoError(err)
	suite.Equal(int64(3), stats.ShowsSubmitted)
	suite.Equal(int64(1), stats.VenuesSubmitted)
	suite.Equal(int64(4), stats.TotalContributions)
}

func (suite *ContributorProfileServiceIntegrationTestSuite) TestGetContributionStats_AuditLogActions() {
	user := suite.createTestUser("audituser")

	// Content creation actions
	suite.auditLog.LogAction(user.ID, "create_release", "release", 1, nil)
	suite.auditLog.LogAction(user.ID, "create_release", "release", 2, nil)
	suite.auditLog.LogAction(user.ID, "create_label", "label", 1, nil)
	suite.auditLog.LogAction(user.ID, "edit_artist", "artist", 1, nil)

	stats, err := suite.profileService.GetContributionStats(user.ID)

	suite.Require().NoError(err)
	suite.Equal(int64(2), stats.ReleasesCreated)
	suite.Equal(int64(1), stats.LabelsCreated)
	suite.Equal(int64(1), stats.ArtistsEdited)
	suite.Equal(int64(4), stats.TotalContributions)
}

func (suite *ContributorProfileServiceIntegrationTestSuite) TestGetContributionStats_ModerationActions() {
	user := suite.createTestUser("moderator")

	suite.auditLog.LogAction(user.ID, "approve_show", "show", 1, nil)
	suite.auditLog.LogAction(user.ID, "reject_show", "show", 2, nil)
	suite.auditLog.LogAction(user.ID, "verify_venue", "venue", 1, nil)

	stats, err := suite.profileService.GetContributionStats(user.ID)

	suite.Require().NoError(err)
	suite.Equal(int64(3), stats.ModerationActions)
	suite.Equal(int64(3), stats.TotalContributions)
}

func (suite *ContributorProfileServiceIntegrationTestSuite) TestGetContributionStats_MixedSources() {
	user := suite.createTestUser("mixeduser")

	// Submissions
	suite.createShow(user.ID, "My Show")
	suite.createVenue(user.ID, "My Venue")

	// Audit actions
	suite.auditLog.LogAction(user.ID, "create_release", "release", 1, nil)
	suite.auditLog.LogAction(user.ID, "approve_show", "show", 99, nil)

	stats, err := suite.profileService.GetContributionStats(user.ID)

	suite.Require().NoError(err)
	suite.Equal(int64(1), stats.ShowsSubmitted)
	suite.Equal(int64(1), stats.VenuesSubmitted)
	suite.Equal(int64(1), stats.ReleasesCreated)
	suite.Equal(int64(1), stats.ModerationActions)
	suite.Equal(int64(4), stats.TotalContributions)
}

func (suite *ContributorProfileServiceIntegrationTestSuite) TestGetContributionStats_DoesNotCountOtherUsers() {
	user1 := suite.createTestUser("user1stats")
	user2 := suite.createTestUser("user2stats")

	suite.createShow(user1.ID, "User1 Show")
	suite.createShow(user2.ID, "User2 Show")
	suite.auditLog.LogAction(user2.ID, "create_release", "release", 1, nil)

	stats, err := suite.profileService.GetContributionStats(user1.ID)

	suite.Require().NoError(err)
	suite.Equal(int64(1), stats.ShowsSubmitted)
	suite.Equal(int64(0), stats.ReleasesCreated)
	suite.Equal(int64(1), stats.TotalContributions)
}

// =============================================================================
// Group 3b: GetContributionStats — Expanded Stat Types
// =============================================================================

func (suite *ContributorProfileServiceIntegrationTestSuite) TestGetContributionStats_TagVotes() {
	user := suite.createTestUser("tagvoter")

	// Create a tag
	tag := &models.Tag{Name: "punk", Slug: "punk", Category: "genre"}
	suite.Require().NoError(suite.db.Create(tag).Error)

	// Create an artist to tag-vote on
	artist := &models.Artist{Name: "Bad Brains"}
	suite.Require().NoError(suite.db.Create(artist).Error)

	// Cast tag votes
	suite.Require().NoError(suite.db.Create(&models.TagVote{
		TagID: tag.ID, EntityType: "artist", EntityID: artist.ID, UserID: user.ID, Vote: 1,
	}).Error)

	stats, err := suite.profileService.GetContributionStats(user.ID)
	suite.Require().NoError(err)
	suite.Equal(int64(1), stats.TagVotesCast)
}

func (suite *ContributorProfileServiceIntegrationTestSuite) TestGetContributionStats_RelationshipVotes() {
	user := suite.createTestUser("relvoter")

	// Create two artists for relationship
	artist1 := &models.Artist{Name: "Artist A"}
	artist2 := &models.Artist{Name: "Artist B"}
	suite.Require().NoError(suite.db.Create(artist1).Error)
	suite.Require().NoError(suite.db.Create(artist2).Error)

	source, target := models.CanonicalOrder(artist1.ID, artist2.ID)

	// Create relationship
	suite.Require().NoError(suite.db.Create(&models.ArtistRelationship{
		SourceArtistID: source, TargetArtistID: target,
		RelationshipType: models.RelationshipTypeSimilar,
	}).Error)

	// Cast vote
	suite.Require().NoError(suite.db.Create(&models.ArtistRelationshipVote{
		SourceArtistID: source, TargetArtistID: target,
		RelationshipType: models.RelationshipTypeSimilar,
		UserID: user.ID, Direction: 1,
	}).Error)

	stats, err := suite.profileService.GetContributionStats(user.ID)
	suite.Require().NoError(err)
	suite.Equal(int64(1), stats.RelationshipVotesCast)
}

func (suite *ContributorProfileServiceIntegrationTestSuite) TestGetContributionStats_RequestVotes() {
	user := suite.createTestUser("reqvoter")
	requester := suite.createTestUser("requester")

	// Create a request
	request := &models.Request{
		Title: "Add new band", EntityType: "artist",
		RequesterID: requester.ID, Status: models.RequestStatusPending,
	}
	suite.Require().NoError(suite.db.Create(request).Error)

	// Cast votes
	suite.Require().NoError(suite.db.Create(&models.RequestVote{
		RequestID: request.ID, UserID: user.ID, Vote: 1,
	}).Error)

	stats, err := suite.profileService.GetContributionStats(user.ID)
	suite.Require().NoError(err)
	suite.Equal(int64(1), stats.RequestVotesCast)
}

func (suite *ContributorProfileServiceIntegrationTestSuite) TestGetContributionStats_CollectionItems() {
	user := suite.createTestUser("collector")

	// Create a collection
	collection := &models.Collection{
		Title: "My Favorites", Slug: fmt.Sprintf("my-favorites-%d", time.Now().UnixNano()),
		CreatorID: user.ID,
	}
	suite.Require().NoError(suite.db.Create(collection).Error)

	// Add items
	suite.Require().NoError(suite.db.Create(&models.CollectionItem{
		CollectionID: collection.ID, EntityType: "artist", EntityID: 1,
		AddedByUserID: user.ID,
	}).Error)
	suite.Require().NoError(suite.db.Create(&models.CollectionItem{
		CollectionID: collection.ID, EntityType: "release", EntityID: 2,
		AddedByUserID: user.ID,
	}).Error)

	stats, err := suite.profileService.GetContributionStats(user.ID)
	suite.Require().NoError(err)
	suite.Equal(int64(2), stats.CollectionItemsAdded)
}

func (suite *ContributorProfileServiceIntegrationTestSuite) TestGetContributionStats_CollectionSubscriptions() {
	user := suite.createTestUser("subscriber")
	creator := suite.createTestUser("creator")

	// Create collections
	col1 := &models.Collection{
		Title: "Coll 1", Slug: fmt.Sprintf("coll-1-%d", time.Now().UnixNano()),
		CreatorID: creator.ID,
	}
	col2 := &models.Collection{
		Title: "Coll 2", Slug: fmt.Sprintf("coll-2-%d", time.Now().UnixNano()),
		CreatorID: creator.ID,
	}
	suite.Require().NoError(suite.db.Create(col1).Error)
	suite.Require().NoError(suite.db.Create(col2).Error)

	// Subscribe
	suite.Require().NoError(suite.db.Create(&models.CollectionSubscriber{
		CollectionID: col1.ID, UserID: user.ID,
	}).Error)
	suite.Require().NoError(suite.db.Create(&models.CollectionSubscriber{
		CollectionID: col2.ID, UserID: user.ID,
	}).Error)

	stats, err := suite.profileService.GetContributionStats(user.ID)
	suite.Require().NoError(err)
	suite.Equal(int64(2), stats.CollectionSubscriptions)
}

func (suite *ContributorProfileServiceIntegrationTestSuite) TestGetContributionStats_ShowsAttended() {
	user := suite.createTestUser("attendee")

	// Mark shows as "going" via bookmarks
	suite.Require().NoError(suite.db.Create(&models.UserBookmark{
		UserID: user.ID, EntityType: models.BookmarkEntityShow,
		EntityID: 1, Action: models.BookmarkActionGoing,
	}).Error)
	suite.Require().NoError(suite.db.Create(&models.UserBookmark{
		UserID: user.ID, EntityType: models.BookmarkEntityShow,
		EntityID: 2, Action: models.BookmarkActionGoing,
	}).Error)
	// "interested" should not count as attended
	suite.Require().NoError(suite.db.Create(&models.UserBookmark{
		UserID: user.ID, EntityType: models.BookmarkEntityShow,
		EntityID: 3, Action: models.BookmarkActionInterested,
	}).Error)

	stats, err := suite.profileService.GetContributionStats(user.ID)
	suite.Require().NoError(err)
	suite.Equal(int64(2), stats.ShowsAttended)
}

func (suite *ContributorProfileServiceIntegrationTestSuite) TestGetContributionStats_Revisions() {
	user := suite.createTestUser("reviser")

	fieldChanges := json.RawMessage(`[{"field":"name","old_value":"Old","new_value":"New"}]`)
	suite.Require().NoError(suite.db.Create(&models.Revision{
		EntityType: "artist", EntityID: 1, UserID: user.ID,
		FieldChanges: &fieldChanges,
	}).Error)
	suite.Require().NoError(suite.db.Create(&models.Revision{
		EntityType: "venue", EntityID: 2, UserID: user.ID,
		FieldChanges: &fieldChanges,
	}).Error)

	stats, err := suite.profileService.GetContributionStats(user.ID)
	suite.Require().NoError(err)
	suite.Equal(int64(2), stats.RevisionsMade)
}

func (suite *ContributorProfileServiceIntegrationTestSuite) TestGetContributionStats_PendingEdits() {
	user := suite.createTestUser("pendinguser")

	fieldChanges := json.RawMessage(`[{"field":"name","old_value":"Old","new_value":"New"}]`)
	suite.Require().NoError(suite.db.Create(&models.PendingEntityEdit{
		EntityType: "artist", EntityID: 1, SubmittedBy: user.ID,
		FieldChanges: &fieldChanges, Summary: "Fix name",
		Status: models.PendingEditStatusPending,
	}).Error)

	stats, err := suite.profileService.GetContributionStats(user.ID)
	suite.Require().NoError(err)
	suite.Equal(int64(1), stats.PendingEditsSubmitted)
}

func (suite *ContributorProfileServiceIntegrationTestSuite) TestGetContributionStats_ApprovalRate() {
	user := suite.createTestUser("approvaluser")

	fieldChanges := json.RawMessage(`[{"field":"name","old_value":"Old","new_value":"New"}]`)
	// 3 approved, 1 rejected = 75% approval rate
	for i := 0; i < 3; i++ {
		suite.Require().NoError(suite.db.Create(&models.PendingEntityEdit{
			EntityType: "artist", EntityID: uint(i + 1), SubmittedBy: user.ID,
			FieldChanges: &fieldChanges, Summary: "Edit",
			Status: models.PendingEditStatusApproved,
		}).Error)
	}
	suite.Require().NoError(suite.db.Create(&models.PendingEntityEdit{
		EntityType: "venue", EntityID: 1, SubmittedBy: user.ID,
		FieldChanges: &fieldChanges, Summary: "Edit",
		Status: models.PendingEditStatusRejected,
	}).Error)
	// Pending edits should not affect rate
	suite.Require().NoError(suite.db.Create(&models.PendingEntityEdit{
		EntityType: "venue", EntityID: 2, SubmittedBy: user.ID,
		FieldChanges: &fieldChanges, Summary: "Edit",
		Status: models.PendingEditStatusPending,
	}).Error)

	stats, err := suite.profileService.GetContributionStats(user.ID)
	suite.Require().NoError(err)
	suite.Require().NotNil(stats.ApprovalRate)
	suite.InDelta(0.75, *stats.ApprovalRate, 0.001)
	suite.Equal(int64(5), stats.PendingEditsSubmitted)
}

func (suite *ContributorProfileServiceIntegrationTestSuite) TestGetContributionStats_ApprovalRate_NilWhenNone() {
	user := suite.createTestUser("noapproval")

	stats, err := suite.profileService.GetContributionStats(user.ID)
	suite.Require().NoError(err)
	suite.Nil(stats.ApprovalRate, "ApprovalRate should be nil when no approved/rejected edits exist")
}

func (suite *ContributorProfileServiceIntegrationTestSuite) TestGetContributionStats_ReportsFiled() {
	user := suite.createTestUser("reporter")

	// Entity report
	suite.Require().NoError(suite.db.Create(&models.EntityReport{
		EntityType: "artist", EntityID: 1, ReportedBy: user.ID,
		ReportType: "inaccurate", Status: models.EntityReportStatusPending,
	}).Error)

	// Show report
	show := suite.createShow(user.ID, "Test Show")
	suite.Require().NoError(suite.db.Create(&models.ShowReport{
		ShowID: show.ID, ReportedBy: user.ID,
		ReportType: models.ShowReportTypeCancelled, Status: models.ShowReportStatusPending,
	}).Error)

	stats, err := suite.profileService.GetContributionStats(user.ID)
	suite.Require().NoError(err)
	suite.Equal(int64(2), stats.ReportsFiled)
}

func (suite *ContributorProfileServiceIntegrationTestSuite) TestGetContributionStats_ReportsResolved() {
	user := suite.createTestUser("resolver")
	reporter := suite.createTestUser("filereporter")

	// Resolved entity report
	now := time.Now()
	suite.Require().NoError(suite.db.Create(&models.EntityReport{
		EntityType: "venue", EntityID: 1, ReportedBy: reporter.ID,
		ReportType: "inaccurate", Status: models.EntityReportStatusResolved,
		ReviewedBy: &user.ID, ReviewedAt: &now,
	}).Error)

	// Dismissed show report
	show := suite.createShow(reporter.ID, "Reported Show")
	suite.Require().NoError(suite.db.Create(&models.ShowReport{
		ShowID: show.ID, ReportedBy: reporter.ID,
		ReportType: models.ShowReportTypeInaccurate, Status: models.ShowReportStatusDismissed,
		ReviewedBy: &user.ID, ReviewedAt: &now,
	}).Error)

	stats, err := suite.profileService.GetContributionStats(user.ID)
	suite.Require().NoError(err)
	suite.Equal(int64(2), stats.ReportsResolved)
}

func (suite *ContributorProfileServiceIntegrationTestSuite) TestGetContributionStats_FollowingCount() {
	user := suite.createTestUser("follower")

	// Follow some entities
	suite.Require().NoError(suite.db.Create(&models.UserBookmark{
		UserID: user.ID, EntityType: models.BookmarkEntityArtist,
		EntityID: 1, Action: models.BookmarkActionFollow,
	}).Error)
	suite.Require().NoError(suite.db.Create(&models.UserBookmark{
		UserID: user.ID, EntityType: models.BookmarkEntityVenue,
		EntityID: 1, Action: models.BookmarkActionFollow,
	}).Error)
	// "save" action should not count
	suite.Require().NoError(suite.db.Create(&models.UserBookmark{
		UserID: user.ID, EntityType: models.BookmarkEntityShow,
		EntityID: 1, Action: models.BookmarkActionSave,
	}).Error)

	stats, err := suite.profileService.GetContributionStats(user.ID)
	suite.Require().NoError(err)
	suite.Equal(int64(2), stats.FollowingCount)
}

func (suite *ContributorProfileServiceIntegrationTestSuite) TestGetContributionStats_TotalIncludesNewStats() {
	user := suite.createTestUser("totaluser")

	// Create a show (1 contribution)
	suite.createShow(user.ID, "My Show")

	// Create a revision (1 contribution)
	fieldChanges := json.RawMessage(`[{"field":"name","old_value":"Old","new_value":"New"}]`)
	suite.Require().NoError(suite.db.Create(&models.Revision{
		EntityType: "artist", EntityID: 1, UserID: user.ID,
		FieldChanges: &fieldChanges,
	}).Error)

	// Create a tag vote (1 contribution)
	tag := &models.Tag{Name: "rock", Slug: fmt.Sprintf("rock-%d", time.Now().UnixNano()), Category: "genre"}
	suite.Require().NoError(suite.db.Create(tag).Error)
	artist := &models.Artist{Name: "Test Artist"}
	suite.Require().NoError(suite.db.Create(artist).Error)
	suite.Require().NoError(suite.db.Create(&models.TagVote{
		TagID: tag.ID, EntityType: "artist", EntityID: artist.ID, UserID: user.ID, Vote: 1,
	}).Error)

	// Create a report (1 contribution)
	suite.Require().NoError(suite.db.Create(&models.EntityReport{
		EntityType: "artist", EntityID: 1, ReportedBy: user.ID,
		ReportType: "inaccurate", Status: models.EntityReportStatusPending,
	}).Error)

	stats, err := suite.profileService.GetContributionStats(user.ID)
	suite.Require().NoError(err)
	// 1 show + 1 revision + 1 tag vote + 1 report = 4
	suite.Equal(int64(4), stats.TotalContributions)
}

// =============================================================================
// Group 4: GetContributionHistory
// =============================================================================

func (suite *ContributorProfileServiceIntegrationTestSuite) TestGetContributionHistory_Empty() {
	user := suite.createTestUser("emptyhistory")

	entries, total, err := suite.profileService.GetContributionHistory(user.ID, 20, 0, "")

	suite.Require().NoError(err)
	suite.Equal(int64(0), total)
	suite.Empty(entries)
}

func (suite *ContributorProfileServiceIntegrationTestSuite) TestGetContributionHistory_ShowSubmissions() {
	user := suite.createTestUser("showhistory")
	suite.createShow(user.ID, "My Great Show")

	entries, total, err := suite.profileService.GetContributionHistory(user.ID, 20, 0, "")

	suite.Require().NoError(err)
	suite.Equal(int64(1), total)
	suite.Require().Len(entries, 1)
	suite.Equal("submit_show", entries[0].Action)
	suite.Equal("show", entries[0].EntityType)
	suite.Equal("submission", entries[0].Source)
	suite.Equal("My Great Show", entries[0].EntityName)
}

func (suite *ContributorProfileServiceIntegrationTestSuite) TestGetContributionHistory_AuditLogEntries() {
	user := suite.createTestUser("audithistory")
	suite.auditLog.LogAction(user.ID, "create_release", "release", 1, map[string]interface{}{
		"title": "New Album",
	})

	entries, total, err := suite.profileService.GetContributionHistory(user.ID, 20, 0, "")

	suite.Require().NoError(err)
	suite.Equal(int64(1), total)
	suite.Require().Len(entries, 1)
	suite.Equal("create_release", entries[0].Action)
	suite.Equal("release", entries[0].EntityType)
	suite.Equal("audit_log", entries[0].Source)
	suite.Require().NotNil(entries[0].Metadata)
	suite.Equal("New Album", entries[0].Metadata["title"])
}

func (suite *ContributorProfileServiceIntegrationTestSuite) TestGetContributionHistory_MergesSources() {
	user := suite.createTestUser("mergehistory")
	suite.createShow(user.ID, "Submitted Show")
	suite.createVenue(user.ID, "Submitted Venue")
	suite.auditLog.LogAction(user.ID, "create_release", "release", 1, nil)

	entries, total, err := suite.profileService.GetContributionHistory(user.ID, 20, 0, "")

	suite.Require().NoError(err)
	suite.Equal(int64(3), total)
	suite.Len(entries, 3)

	// Verify both sources are represented
	sources := map[string]bool{}
	for _, e := range entries {
		sources[e.Source] = true
	}
	suite.True(sources["submission"])
	suite.True(sources["audit_log"])
}

func (suite *ContributorProfileServiceIntegrationTestSuite) TestGetContributionHistory_Pagination() {
	user := suite.createTestUser("paginationhistory")
	for i := 0; i < 5; i++ {
		suite.createShow(user.ID, fmt.Sprintf("Show %d", i))
	}

	// Page 1
	page1, total, err := suite.profileService.GetContributionHistory(user.ID, 2, 0, "")
	suite.Require().NoError(err)
	suite.Equal(int64(5), total)
	suite.Len(page1, 2)

	// Page 2
	page2, _, err := suite.profileService.GetContributionHistory(user.ID, 2, 2, "")
	suite.Require().NoError(err)
	suite.Len(page2, 2)

	// Page 3
	page3, _, err := suite.profileService.GetContributionHistory(user.ID, 2, 4, "")
	suite.Require().NoError(err)
	suite.Len(page3, 1)

	// No overlap
	suite.NotEqual(page1[0].ID, page2[0].ID)
}

func (suite *ContributorProfileServiceIntegrationTestSuite) TestGetContributionHistory_EntityTypeFilter() {
	user := suite.createTestUser("filterhistory")
	suite.createShow(user.ID, "A Show")
	suite.createVenue(user.ID, "A Venue")

	// Filter to shows only
	entries, total, err := suite.profileService.GetContributionHistory(user.ID, 20, 0, "show")

	suite.Require().NoError(err)
	suite.Equal(int64(1), total)
	suite.Require().Len(entries, 1)
	suite.Equal("show", entries[0].EntityType)
}

func (suite *ContributorProfileServiceIntegrationTestSuite) TestGetContributionHistory_LimitClamping() {
	user := suite.createTestUser("limithistory")

	// Limit > 100 should be clamped
	_, _, err := suite.profileService.GetContributionHistory(user.ID, 200, 0, "")
	suite.Require().NoError(err)

	// Limit < 1 should default to 20
	_, _, err = suite.profileService.GetContributionHistory(user.ID, 0, 0, "")
	suite.Require().NoError(err)
}

func (suite *ContributorProfileServiceIntegrationTestSuite) TestGetContributionHistory_VenueSubmissionEnriched() {
	user := suite.createTestUser("venueenrich")
	suite.createVenue(user.ID, "The Rebel Lounge")

	entries, _, err := suite.profileService.GetContributionHistory(user.ID, 20, 0, "")

	suite.Require().NoError(err)
	suite.Require().Len(entries, 1)
	suite.Equal("submit_venue", entries[0].Action)
	suite.Equal("venue", entries[0].EntityType)
	suite.Equal("The Rebel Lounge", entries[0].EntityName)
}

func (suite *ContributorProfileServiceIntegrationTestSuite) TestGetContributionHistory_OrderedByCreatedAtDesc() {
	user := suite.createTestUser("orderhistory")

	// Create entries with different timestamps
	show1 := suite.createShow(user.ID, "First Show")
	time.Sleep(10 * time.Millisecond) // Ensure different timestamps
	show2 := suite.createShow(user.ID, "Second Show")

	entries, _, err := suite.profileService.GetContributionHistory(user.ID, 20, 0, "")

	suite.Require().NoError(err)
	suite.Require().Len(entries, 2)
	// Most recent first
	suite.Equal(show2.ID, entries[0].EntityID)
	suite.Equal(show1.ID, entries[1].EntityID)
}

// =============================================================================
// Group 5: Privacy Settings
// =============================================================================

func (suite *ContributorProfileServiceIntegrationTestSuite) TestUpdatePrivacySettings_Success() {
	user := suite.createTestUser("privacyuser")

	settings := contracts.PrivacySettings{
		Contributions:   contracts.PrivacyHidden,
		SavedShows:      contracts.PrivacyVisible,
		Attendance:      contracts.PrivacyCountOnly,
		Following:       contracts.PrivacyHidden,
		Collections:     contracts.PrivacyCountOnly,
		LastActive:      contracts.PrivacyHidden,
		ProfileSections: contracts.PrivacyVisible,
	}

	result, err := suite.profileService.UpdatePrivacySettings(user.ID, settings)

	suite.Require().NoError(err)
	suite.Require().NotNil(result)
	suite.Equal(contracts.PrivacyHidden, result.Contributions)
	suite.Equal(contracts.PrivacyVisible, result.SavedShows)
	suite.Equal(contracts.PrivacyCountOnly, result.Attendance)
	suite.Equal(contracts.PrivacyHidden, result.Following)
	suite.Equal(contracts.PrivacyCountOnly, result.Collections)
	suite.Equal(contracts.PrivacyHidden, result.LastActive)
	suite.Equal(contracts.PrivacyVisible, result.ProfileSections)
}

func (suite *ContributorProfileServiceIntegrationTestSuite) TestUpdatePrivacySettings_Persists() {
	user := suite.createTestUser("privacypersist")

	settings := contracts.PrivacySettings{
		Contributions:   contracts.PrivacyHidden,
		SavedShows:      contracts.PrivacyHidden,
		Attendance:      contracts.PrivacyHidden,
		Following:       contracts.PrivacyHidden,
		Collections:     contracts.PrivacyHidden,
		LastActive:      contracts.PrivacyHidden,
		ProfileSections: contracts.PrivacyHidden,
	}

	_, err := suite.profileService.UpdatePrivacySettings(user.ID, settings)
	suite.Require().NoError(err)

	// Reload and verify
	profile, err := suite.profileService.GetOwnProfile(user.ID)
	suite.Require().NoError(err)
	suite.Require().NotNil(profile.PrivacySettings)
	suite.Equal(contracts.PrivacyHidden, profile.PrivacySettings.Contributions)
	suite.Equal(contracts.PrivacyHidden, profile.PrivacySettings.LastActive)
}

func (suite *ContributorProfileServiceIntegrationTestSuite) TestUpdatePrivacySettings_InvalidLevel() {
	user := suite.createTestUser("privacyinvalid")

	settings := contracts.DefaultPrivacySettings()
	settings.Contributions = "invalid_level"

	result, err := suite.profileService.UpdatePrivacySettings(user.ID, settings)

	suite.Error(err)
	suite.Nil(result)
	suite.Contains(err.Error(), "invalid privacy level")
}

func (suite *ContributorProfileServiceIntegrationTestSuite) TestUpdatePrivacySettings_CountOnlyBinaryField() {
	user := suite.createTestUser("privacybinary")

	settings := contracts.DefaultPrivacySettings()
	settings.LastActive = contracts.PrivacyCountOnly

	result, err := suite.profileService.UpdatePrivacySettings(user.ID, settings)

	suite.Error(err)
	suite.Nil(result)
	suite.Contains(err.Error(), "only supports 'visible' or 'hidden'")
}

func (suite *ContributorProfileServiceIntegrationTestSuite) TestGetPublicProfile_PrivacyGating_ContributionsHidden() {
	user := suite.createTestUser("privgatecontrib")
	suite.createShow(user.ID, "Hidden Show")

	suite.setPrivacySettings(user.ID, contracts.PrivacySettings{
		Contributions:   contracts.PrivacyHidden,
		SavedShows:      contracts.PrivacyHidden,
		Attendance:      contracts.PrivacyHidden,
		Following:       contracts.PrivacyHidden,
		Collections:     contracts.PrivacyHidden,
		LastActive:      contracts.PrivacyHidden,
		ProfileSections: contracts.PrivacyHidden,
	})

	otherUser := suite.createTestUser("viewer1")
	profile, err := suite.profileService.GetPublicProfile("privgatecontrib", &otherUser.ID)

	suite.Require().NoError(err)
	suite.Require().NotNil(profile)
	suite.Nil(profile.Stats, "Stats should be nil when contributions are hidden")
	suite.Nil(profile.StatsCount, "StatsCount should be nil when contributions are hidden")
	suite.Nil(profile.LastActive, "LastActive should be nil when hidden")
	suite.Empty(profile.Sections, "Sections should be empty when hidden")
}

func (suite *ContributorProfileServiceIntegrationTestSuite) TestGetPublicProfile_PrivacyGating_ContributionsCountOnly() {
	user := suite.createTestUser("privgatecountonly")
	suite.createShow(user.ID, "Counted Show")
	suite.createShow(user.ID, "Another Counted Show")

	suite.setPrivacySettings(user.ID, contracts.PrivacySettings{
		Contributions:   contracts.PrivacyCountOnly,
		SavedShows:      contracts.PrivacyHidden,
		Attendance:      contracts.PrivacyHidden,
		Following:       contracts.PrivacyHidden,
		Collections:     contracts.PrivacyHidden,
		LastActive:      contracts.PrivacyVisible,
		ProfileSections: contracts.PrivacyVisible,
	})

	otherUser := suite.createTestUser("viewer2")
	profile, err := suite.profileService.GetPublicProfile("privgatecountonly", &otherUser.ID)

	suite.Require().NoError(err)
	suite.Require().NotNil(profile)
	suite.Nil(profile.Stats, "Full stats should be nil with count_only")
	suite.Require().NotNil(profile.StatsCount, "StatsCount should be present with count_only")
	suite.Equal(int64(2), *profile.StatsCount)
}

func (suite *ContributorProfileServiceIntegrationTestSuite) TestGetPublicProfile_OwnerSeesEverything() {
	user := suite.createTestUser("ownerseesall")
	suite.createShow(user.ID, "My Show")

	suite.setPrivacySettings(user.ID, contracts.PrivacySettings{
		Contributions:   contracts.PrivacyHidden,
		SavedShows:      contracts.PrivacyHidden,
		Attendance:      contracts.PrivacyHidden,
		Following:       contracts.PrivacyHidden,
		Collections:     contracts.PrivacyHidden,
		LastActive:      contracts.PrivacyHidden,
		ProfileSections: contracts.PrivacyHidden,
	})

	profile, err := suite.profileService.GetPublicProfile("ownerseesall", &user.ID)

	suite.Require().NoError(err)
	suite.Require().NotNil(profile)
	suite.Require().NotNil(profile.Stats, "Owner always sees stats")
	suite.Require().NotNil(profile.PrivacySettings, "Owner sees privacy settings")
	suite.Require().NotNil(profile.LastActive, "Owner always sees last active")
}

// =============================================================================
// Group 6: User Tier
// =============================================================================

func (suite *ContributorProfileServiceIntegrationTestSuite) TestGetPublicProfile_DefaultTier() {
	suite.createTestUser("tierdefault")

	profile, err := suite.profileService.GetPublicProfile("tierdefault", nil)

	suite.Require().NoError(err)
	suite.Require().NotNil(profile)
	suite.Equal("new_user", profile.UserTier)
}

func (suite *ContributorProfileServiceIntegrationTestSuite) TestGetPublicProfile_CustomTier() {
	user := suite.createTestUser("tiercustom")
	err := suite.db.Model(user).Update("user_tier", "contributor").Error
	suite.Require().NoError(err)

	profile, err := suite.profileService.GetPublicProfile("tiercustom", nil)

	suite.Require().NoError(err)
	suite.Require().NotNil(profile)
	suite.Equal("contributor", profile.UserTier)
}

func (suite *ContributorProfileServiceIntegrationTestSuite) TestGetOwnProfile_IncludesTier() {
	user := suite.createTestUser("tierown")
	err := suite.db.Model(user).Update("user_tier", "trusted_contributor").Error
	suite.Require().NoError(err)

	profile, err := suite.profileService.GetOwnProfile(user.ID)

	suite.Require().NoError(err)
	suite.Require().NotNil(profile)
	suite.Equal("trusted_contributor", profile.UserTier)
}

// =============================================================================
// Group 7: Profile Sections CRUD
// =============================================================================

func (suite *ContributorProfileServiceIntegrationTestSuite) TestCreateSection_Success() {
	user := suite.createTestUser("sectioncreate")

	section, err := suite.profileService.CreateSection(user.ID, "My Music", "I love punk rock", 0)

	suite.Require().NoError(err)
	suite.Require().NotNil(section)
	suite.Equal("My Music", section.Title)
	suite.Equal("I love punk rock", section.Content)
	suite.Equal(0, section.Position)
	suite.True(section.IsVisible)
	suite.NotZero(section.ID)
}

func (suite *ContributorProfileServiceIntegrationTestSuite) TestCreateSection_MaxSections() {
	user := suite.createTestUser("sectionmax")

	_, err := suite.profileService.CreateSection(user.ID, "Section 1", "Content", 0)
	suite.Require().NoError(err)
	_, err = suite.profileService.CreateSection(user.ID, "Section 2", "Content", 1)
	suite.Require().NoError(err)
	_, err = suite.profileService.CreateSection(user.ID, "Section 3", "Content", 2)
	suite.Require().NoError(err)

	// Fourth should fail
	section, err := suite.profileService.CreateSection(user.ID, "Section 4", "Content", 0)

	suite.Error(err)
	suite.Nil(section)
	suite.Contains(err.Error(), "maximum 3 profile sections allowed")
}

func (suite *ContributorProfileServiceIntegrationTestSuite) TestCreateSection_EmptyTitle() {
	user := suite.createTestUser("sectionempty")

	section, err := suite.profileService.CreateSection(user.ID, "", "Content", 0)

	suite.Error(err)
	suite.Nil(section)
	suite.Contains(err.Error(), "title must be between 1 and 255 characters")
}

func (suite *ContributorProfileServiceIntegrationTestSuite) TestCreateSection_InvalidPosition() {
	user := suite.createTestUser("sectionpos")

	section, err := suite.profileService.CreateSection(user.ID, "Title", "Content", 5)

	suite.Error(err)
	suite.Nil(section)
	suite.Contains(err.Error(), "position must be between 0 and 2")
}

func (suite *ContributorProfileServiceIntegrationTestSuite) TestCreateSection_NegativePosition() {
	user := suite.createTestUser("sectionneg")

	section, err := suite.profileService.CreateSection(user.ID, "Title", "Content", -1)

	suite.Error(err)
	suite.Nil(section)
	suite.Contains(err.Error(), "position must be between 0 and 2")
}

func (suite *ContributorProfileServiceIntegrationTestSuite) TestGetUserSections_OnlyVisible() {
	user := suite.createTestUser("sectionvisible")

	s1, err := suite.profileService.CreateSection(user.ID, "Visible", "Content", 0)
	suite.Require().NoError(err)
	s2, err := suite.profileService.CreateSection(user.ID, "Hidden", "Content", 1)
	suite.Require().NoError(err)

	// Hide the second section
	_, err = suite.profileService.UpdateSection(user.ID, s2.ID, map[string]interface{}{"is_visible": false})
	suite.Require().NoError(err)

	sections, err := suite.profileService.GetUserSections(user.ID)

	suite.Require().NoError(err)
	suite.Len(sections, 1)
	suite.Equal(s1.ID, sections[0].ID)
	suite.Equal("Visible", sections[0].Title)
}

func (suite *ContributorProfileServiceIntegrationTestSuite) TestGetOwnSections_IncludesHidden() {
	user := suite.createTestUser("sectionown")

	_, err := suite.profileService.CreateSection(user.ID, "Visible", "Content", 0)
	suite.Require().NoError(err)
	s2, err := suite.profileService.CreateSection(user.ID, "Hidden", "Content", 1)
	suite.Require().NoError(err)

	// Hide the second section
	_, err = suite.profileService.UpdateSection(user.ID, s2.ID, map[string]interface{}{"is_visible": false})
	suite.Require().NoError(err)

	sections, err := suite.profileService.GetOwnSections(user.ID)

	suite.Require().NoError(err)
	suite.Len(sections, 2)
}

func (suite *ContributorProfileServiceIntegrationTestSuite) TestUpdateSection_Success() {
	user := suite.createTestUser("sectionupdate")

	section, err := suite.profileService.CreateSection(user.ID, "Original", "Original content", 0)
	suite.Require().NoError(err)

	updated, err := suite.profileService.UpdateSection(user.ID, section.ID, map[string]interface{}{
		"title":   "Updated Title",
		"content": "Updated content",
	})

	suite.Require().NoError(err)
	suite.Equal("Updated Title", updated.Title)
	suite.Equal("Updated content", updated.Content)
}

func (suite *ContributorProfileServiceIntegrationTestSuite) TestUpdateSection_NotFound() {
	user := suite.createTestUser("sectionnotfound")

	section, err := suite.profileService.UpdateSection(user.ID, 99999, map[string]interface{}{
		"title": "Nope",
	})

	suite.Error(err)
	suite.Nil(section)
	suite.Equal("profile section not found", err.Error())
}

func (suite *ContributorProfileServiceIntegrationTestSuite) TestUpdateSection_WrongOwner() {
	user1 := suite.createTestUser("sectionowner1")
	user2 := suite.createTestUser("sectionowner2")

	section, err := suite.profileService.CreateSection(user1.ID, "Mine", "Content", 0)
	suite.Require().NoError(err)

	// user2 tries to update user1's section
	result, err := suite.profileService.UpdateSection(user2.ID, section.ID, map[string]interface{}{
		"title": "Hijacked",
	})

	suite.Error(err)
	suite.Nil(result)
	suite.Equal("profile section not found", err.Error())
}

func (suite *ContributorProfileServiceIntegrationTestSuite) TestDeleteSection_Success() {
	user := suite.createTestUser("sectiondelete")

	section, err := suite.profileService.CreateSection(user.ID, "Doomed", "Content", 0)
	suite.Require().NoError(err)

	err = suite.profileService.DeleteSection(user.ID, section.ID)

	suite.NoError(err)

	// Verify it's gone
	sections, err := suite.profileService.GetOwnSections(user.ID)
	suite.Require().NoError(err)
	suite.Empty(sections)
}

func (suite *ContributorProfileServiceIntegrationTestSuite) TestDeleteSection_NotFound() {
	user := suite.createTestUser("sectiondelnotfound")

	err := suite.profileService.DeleteSection(user.ID, 99999)

	suite.Error(err)
	suite.Equal("profile section not found", err.Error())
}

func (suite *ContributorProfileServiceIntegrationTestSuite) TestDeleteSection_WrongOwner() {
	user1 := suite.createTestUser("sectiondelowner1")
	user2 := suite.createTestUser("sectiondelowner2")

	section, err := suite.profileService.CreateSection(user1.ID, "Protected", "Content", 0)
	suite.Require().NoError(err)

	err = suite.profileService.DeleteSection(user2.ID, section.ID)

	suite.Error(err)
	suite.Equal("profile section not found", err.Error())
}

func (suite *ContributorProfileServiceIntegrationTestSuite) TestGetPublicProfile_IncludesSections() {
	user := suite.createTestUser("profilesections")

	_, err := suite.profileService.CreateSection(user.ID, "About Me", "I go to shows", 0)
	suite.Require().NoError(err)
	_, err = suite.profileService.CreateSection(user.ID, "Favorite Genres", "Punk, Indie", 1)
	suite.Require().NoError(err)

	profile, err := suite.profileService.GetPublicProfile("profilesections", nil)

	suite.Require().NoError(err)
	suite.Require().NotNil(profile)
	suite.Len(profile.Sections, 2)
	suite.Equal("About Me", profile.Sections[0].Title)
	suite.Equal("Favorite Genres", profile.Sections[1].Title)
}

func (suite *ContributorProfileServiceIntegrationTestSuite) TestGetOwnProfile_IncludesAllSections() {
	user := suite.createTestUser("ownsections")

	_, err := suite.profileService.CreateSection(user.ID, "Visible", "Content", 0)
	suite.Require().NoError(err)
	s2, err := suite.profileService.CreateSection(user.ID, "Hidden", "Secret", 1)
	suite.Require().NoError(err)
	_, err = suite.profileService.UpdateSection(user.ID, s2.ID, map[string]interface{}{"is_visible": false})
	suite.Require().NoError(err)

	profile, err := suite.profileService.GetOwnProfile(user.ID)

	suite.Require().NoError(err)
	suite.Require().NotNil(profile)
	suite.Len(profile.Sections, 2, "Own profile should include hidden sections")
}

func (suite *ContributorProfileServiceIntegrationTestSuite) TestGetSections_OrderedByPosition() {
	user := suite.createTestUser("sectionorder")

	_, err := suite.profileService.CreateSection(user.ID, "Third", "Content", 2)
	suite.Require().NoError(err)
	_, err = suite.profileService.CreateSection(user.ID, "First", "Content", 0)
	suite.Require().NoError(err)
	_, err = suite.profileService.CreateSection(user.ID, "Second", "Content", 1)
	suite.Require().NoError(err)

	sections, err := suite.profileService.GetUserSections(user.ID)

	suite.Require().NoError(err)
	suite.Require().Len(sections, 3)
	suite.Equal("First", sections[0].Title)
	suite.Equal("Second", sections[1].Title)
	suite.Equal("Third", sections[2].Title)
}

// =============================================================================
// Group 8: GetActivityHeatmap
// =============================================================================

func TestGetActivityHeatmap_NilDB(t *testing.T) {
	svc := &ContributorProfileService{db: nil}
	result, err := svc.GetActivityHeatmap(1)
	assert.Nil(t, result)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "database not initialized")
}

func (suite *ContributorProfileServiceIntegrationTestSuite) TestGetActivityHeatmap_NoActivity() {
	user := suite.createTestUser("heatmap_empty")

	resp, err := suite.profileService.GetActivityHeatmap(user.ID)

	suite.Require().NoError(err)
	suite.Require().NotNil(resp)
	suite.Empty(resp.Days)
}

func (suite *ContributorProfileServiceIntegrationTestSuite) TestGetActivityHeatmap_ShowSubmissions() {
	user := suite.createTestUser("heatmap_shows")
	suite.createShow(user.ID, "Show 1")
	suite.createShow(user.ID, "Show 2")

	resp, err := suite.profileService.GetActivityHeatmap(user.ID)

	suite.Require().NoError(err)
	suite.Require().NotNil(resp)
	suite.Require().Len(resp.Days, 1)
	suite.Equal(2, resp.Days[0].Count)
	// Date should be today
	suite.Equal(time.Now().UTC().Format("2006-01-02"), resp.Days[0].Date)
}

func (suite *ContributorProfileServiceIntegrationTestSuite) TestGetActivityHeatmap_VenueSubmissions() {
	user := suite.createTestUser("heatmap_venues")
	suite.createVenue(user.ID, "Venue 1")

	resp, err := suite.profileService.GetActivityHeatmap(user.ID)

	suite.Require().NoError(err)
	suite.Require().NotNil(resp)
	suite.Require().Len(resp.Days, 1)
	suite.Equal(1, resp.Days[0].Count)
}

func (suite *ContributorProfileServiceIntegrationTestSuite) TestGetActivityHeatmap_AuditLogEntries() {
	user := suite.createTestUser("heatmap_audit")
	helper := &testAuditLogHelper{db: suite.db}
	helper.LogAction(user.ID, "create_release", "release", 1, nil)
	helper.LogAction(user.ID, "edit_artist", "artist", 1, nil)

	resp, err := suite.profileService.GetActivityHeatmap(user.ID)

	suite.Require().NoError(err)
	suite.Require().NotNil(resp)
	suite.Require().Len(resp.Days, 1)
	suite.Equal(2, resp.Days[0].Count)
}

func (suite *ContributorProfileServiceIntegrationTestSuite) TestGetActivityHeatmap_Revisions() {
	user := suite.createTestUser("heatmap_revisions")
	fieldChanges := json.RawMessage(`[{"field":"name","old_value":"Old","new_value":"New"}]`)
	suite.Require().NoError(suite.db.Create(&models.Revision{
		EntityType: "artist", EntityID: 1, UserID: user.ID,
		FieldChanges: &fieldChanges,
	}).Error)

	resp, err := suite.profileService.GetActivityHeatmap(user.ID)

	suite.Require().NoError(err)
	suite.Require().NotNil(resp)
	suite.Require().Len(resp.Days, 1)
	suite.Equal(1, resp.Days[0].Count)
}

func (suite *ContributorProfileServiceIntegrationTestSuite) TestGetActivityHeatmap_PendingEdits() {
	user := suite.createTestUser("heatmap_edits")
	fieldChanges := json.RawMessage(`[{"field":"name","old_value":"Old","new_value":"New"}]`)
	suite.Require().NoError(suite.db.Create(&models.PendingEntityEdit{
		EntityType:   "venue",
		EntityID:     1,
		SubmittedBy:  user.ID,
		Status:       models.PendingEditStatusPending,
		FieldChanges: &fieldChanges,
	}).Error)

	resp, err := suite.profileService.GetActivityHeatmap(user.ID)

	suite.Require().NoError(err)
	suite.Require().NotNil(resp)
	suite.Require().Len(resp.Days, 1)
	suite.Equal(1, resp.Days[0].Count)
}

func (suite *ContributorProfileServiceIntegrationTestSuite) TestGetActivityHeatmap_MultipleTablesAggregated() {
	user := suite.createTestUser("heatmap_multi")

	// Create entries across multiple tables (all today)
	suite.createShow(user.ID, "Heatmap Show")
	suite.createVenue(user.ID, "Heatmap Venue")
	helper := &testAuditLogHelper{db: suite.db}
	helper.LogAction(user.ID, "edit_artist", "artist", 1, nil)
	fieldChanges := json.RawMessage(`[{"field":"name","old_value":"Old","new_value":"New"}]`)
	suite.Require().NoError(suite.db.Create(&models.Revision{
		EntityType: "artist", EntityID: 1, UserID: user.ID,
		FieldChanges: &fieldChanges,
	}).Error)
	suite.Require().NoError(suite.db.Create(&models.PendingEntityEdit{
		EntityType:   "venue",
		EntityID:     1,
		SubmittedBy:  user.ID,
		Status:       models.PendingEditStatusPending,
		FieldChanges: &fieldChanges,
	}).Error)

	resp, err := suite.profileService.GetActivityHeatmap(user.ID)

	suite.Require().NoError(err)
	suite.Require().NotNil(resp)
	suite.Require().Len(resp.Days, 1)
	// 1 show + 1 venue + 1 audit + 1 revision + 1 pending edit = 5
	suite.Equal(5, resp.Days[0].Count)
}

func (suite *ContributorProfileServiceIntegrationTestSuite) TestGetActivityHeatmap_DoesNotCountOtherUsers() {
	user1 := suite.createTestUser("heatmap_user1")
	user2 := suite.createTestUser("heatmap_user2")

	suite.createShow(user1.ID, "User 1 Show")
	suite.createShow(user2.ID, "User 2 Show")

	resp, err := suite.profileService.GetActivityHeatmap(user1.ID)

	suite.Require().NoError(err)
	suite.Require().NotNil(resp)
	suite.Require().Len(resp.Days, 1)
	suite.Equal(1, resp.Days[0].Count, "Should only count user1's activity")
}

func (suite *ContributorProfileServiceIntegrationTestSuite) TestGetActivityHeatmap_OldDataExcluded() {
	user := suite.createTestUser("heatmap_old")

	// Create a show dated today (should be included)
	suite.createShow(user.ID, "Recent Show")

	// Directly insert a show with an old created_at (>365 days ago)
	oldDate := time.Now().UTC().AddDate(0, 0, -400)
	suite.Require().NoError(suite.db.Exec(
		"INSERT INTO shows (title, submitted_by, status, event_date, created_at, updated_at) VALUES (?, ?, 'approved', ?, ?, ?)",
		"Old Show", user.ID, oldDate, oldDate, oldDate,
	).Error)

	resp, err := suite.profileService.GetActivityHeatmap(user.ID)

	suite.Require().NoError(err)
	suite.Require().NotNil(resp)
	// Should only have 1 day (today's show), not the old one
	suite.Require().Len(resp.Days, 1)
	suite.Equal(1, resp.Days[0].Count)
}

func (suite *ContributorProfileServiceIntegrationTestSuite) TestGetActivityHeatmap_DateFormat() {
	user := suite.createTestUser("heatmap_format")
	suite.createShow(user.ID, "Date Format Show")

	resp, err := suite.profileService.GetActivityHeatmap(user.ID)

	suite.Require().NoError(err)
	suite.Require().NotNil(resp)
	suite.Require().Len(resp.Days, 1)
	// Verify date format is YYYY-MM-DD
	_, err = time.Parse("2006-01-02", resp.Days[0].Date)
	suite.NoError(err, "Date should be in YYYY-MM-DD format")
}

// =============================================================================
// Group: Percentile Rankings
// =============================================================================

// createManyUsers creates n users with sequential usernames.
func (suite *ContributorProfileServiceIntegrationTestSuite) createManyUsers(prefix string, n int) []*models.User {
	users := make([]*models.User, n)
	for i := 0; i < n; i++ {
		users[i] = suite.createTestUser(fmt.Sprintf("%s%d", prefix, i))
	}
	return users
}

func (suite *ContributorProfileServiceIntegrationTestSuite) TestPercentileRankings_NilDB() {
	svc := &ContributorProfileService{db: nil}
	result, err := svc.GetPercentileRankings(1)
	suite.Error(err)
	suite.Contains(err.Error(), "database not initialized")
	suite.Nil(result)
}

func (suite *ContributorProfileServiceIntegrationTestSuite) TestPercentileRankings_TooFewUsers() {
	// Only 3 users — should return nil
	for i := 0; i < 3; i++ {
		suite.createTestUser(fmt.Sprintf("fewuser%d", i))
	}

	result, err := suite.profileService.GetPercentileRankings(1)
	suite.NoError(err)
	suite.Nil(result, "Should return nil when fewer than 10 active users")
}

func (suite *ContributorProfileServiceIntegrationTestSuite) TestPercentileRankings_NoContributions() {
	users := suite.createManyUsers("nocontrib", 10)

	result, err := suite.profileService.GetPercentileRankings(users[0].ID)
	suite.Require().NoError(err)
	suite.Require().NotNil(result)

	// With no contributions, user should be at 0th percentile for all dimensions
	for _, r := range result.Rankings {
		suite.Equal(int64(0), r.Value, "Dimension %s should have 0 value", r.Dimension)
		suite.Equal(0, r.Percentile, "Dimension %s should have 0 percentile", r.Dimension)
	}
	suite.Equal(0, result.OverallScore)
}

func (suite *ContributorProfileServiceIntegrationTestSuite) TestPercentileRankings_WithShowContributions() {
	users := suite.createManyUsers("showperc", 10)

	// Give the first user 5 shows, second user 2 shows, rest 0
	for i := 0; i < 5; i++ {
		suite.createShow(users[0].ID, fmt.Sprintf("Show %d", i))
	}
	for i := 0; i < 2; i++ {
		suite.createShow(users[1].ID, fmt.Sprintf("Other Show %d", i))
	}

	result, err := suite.profileService.GetPercentileRankings(users[0].ID)
	suite.Require().NoError(err)
	suite.Require().NotNil(result)

	// Find shows_submitted dimension
	var showsRanking *contracts.PercentileRanking
	for i := range result.Rankings {
		if result.Rankings[i].Dimension == "shows_submitted" {
			showsRanking = &result.Rankings[i]
			break
		}
	}
	suite.Require().NotNil(showsRanking, "Should have shows_submitted dimension")
	suite.Equal(int64(5), showsRanking.Value)
	// User 0 has 5 shows; 9 users have less (8 with 0, 1 with 2) → 9/10 = 90%
	suite.Equal(90, showsRanking.Percentile)
}

func (suite *ContributorProfileServiceIntegrationTestSuite) TestPercentileRankings_MultipleUsers_RelativeRanking() {
	users := suite.createManyUsers("relrank", 10)

	// User 0: 3 shows, user 1: 1 show, users 2-9: 0 shows
	for i := 0; i < 3; i++ {
		suite.createShow(users[0].ID, fmt.Sprintf("Show %d", i))
	}
	suite.createShow(users[1].ID, "Single Show")

	// Check user 0 (highest)
	result0, err := suite.profileService.GetPercentileRankings(users[0].ID)
	suite.Require().NoError(err)
	suite.Require().NotNil(result0)

	// Check user 1 (middle)
	result1, err := suite.profileService.GetPercentileRankings(users[1].ID)
	suite.Require().NoError(err)
	suite.Require().NotNil(result1)

	// Check user 2 (no contributions)
	result2, err := suite.profileService.GetPercentileRankings(users[2].ID)
	suite.Require().NoError(err)
	suite.Require().NotNil(result2)

	var p0, p1, p2 int
	for _, r := range result0.Rankings {
		if r.Dimension == "shows_submitted" {
			p0 = r.Percentile
		}
	}
	for _, r := range result1.Rankings {
		if r.Dimension == "shows_submitted" {
			p1 = r.Percentile
		}
	}
	for _, r := range result2.Rankings {
		if r.Dimension == "shows_submitted" {
			p2 = r.Percentile
		}
	}

	// User 0 > user 1 > user 2
	suite.Greater(p0, p1, "User with 3 shows should rank higher than user with 1")
	suite.Greater(p1, p2, "User with 1 show should rank higher than user with 0")
}

func (suite *ContributorProfileServiceIntegrationTestSuite) TestPercentileRankings_VenuesDimension() {
	users := suite.createManyUsers("venueperc", 10)

	suite.createVenue(users[0].ID, "Venue A")
	suite.createVenue(users[0].ID, "Venue B")
	suite.createVenue(users[0].ID, "Venue C")

	result, err := suite.profileService.GetPercentileRankings(users[0].ID)
	suite.Require().NoError(err)
	suite.Require().NotNil(result)

	var venuesRanking *contracts.PercentileRanking
	for i := range result.Rankings {
		if result.Rankings[i].Dimension == "venues_submitted" {
			venuesRanking = &result.Rankings[i]
			break
		}
	}
	suite.Require().NotNil(venuesRanking)
	suite.Equal(int64(3), venuesRanking.Value)
	// 9 users with less → 9/10 = 90%
	suite.Equal(90, venuesRanking.Percentile)
}

func (suite *ContributorProfileServiceIntegrationTestSuite) TestPercentileRankings_TagsDimension() {
	users := suite.createManyUsers("tagperc", 10)

	// Create a tag
	tag := &models.Tag{
		Name:     "indie-rock",
		Slug:     "indie-rock",
		Category: models.TagCategoryGenre,
	}
	suite.Require().NoError(suite.db.Create(tag).Error)

	// Create an artist to tag
	artist := &models.Artist{Name: "Test Band"}
	suite.Require().NoError(suite.db.Create(artist).Error)

	// Apply tags by user 0
	for i := 0; i < 3; i++ {
		et := &models.EntityTag{
			TagID:         tag.ID,
			EntityType:    "artist",
			EntityID:      artist.ID + uint(i), // different entity IDs to avoid unique constraint
			AddedByUserID: users[0].ID,
		}
		suite.Require().NoError(suite.db.Create(et).Error)
	}

	result, err := suite.profileService.GetPercentileRankings(users[0].ID)
	suite.Require().NoError(err)
	suite.Require().NotNil(result)

	var tagsRanking *contracts.PercentileRanking
	for i := range result.Rankings {
		if result.Rankings[i].Dimension == "tags_applied" {
			tagsRanking = &result.Rankings[i]
			break
		}
	}
	suite.Require().NotNil(tagsRanking)
	suite.Equal(int64(3), tagsRanking.Value)
}

func (suite *ContributorProfileServiceIntegrationTestSuite) TestPercentileRankings_EditsDimension() {
	users := suite.createManyUsers("editperc", 10)

	// Create a revision for user 0
	rev := &models.Revision{
		EntityType:   "artist",
		EntityID:     1,
		UserID:       users[0].ID,
		FieldChanges: nil,
	}
	// field_changes must be non-nil JSONB
	emptyChanges := json.RawMessage(`[]`)
	rev.FieldChanges = &emptyChanges
	suite.Require().NoError(suite.db.Create(rev).Error)

	// Create an approved pending edit for user 0
	changes := json.RawMessage(`[{"field":"name","old_value":"Old","new_value":"New"}]`)
	pe := &models.PendingEntityEdit{
		EntityType:   "artist",
		EntityID:     1,
		SubmittedBy:  users[0].ID,
		FieldChanges: &changes,
		Summary:      "Test edit",
		Status:       models.PendingEditStatusApproved,
	}
	suite.Require().NoError(suite.db.Create(pe).Error)

	result, err := suite.profileService.GetPercentileRankings(users[0].ID)
	suite.Require().NoError(err)
	suite.Require().NotNil(result)

	var editsRanking *contracts.PercentileRanking
	for i := range result.Rankings {
		if result.Rankings[i].Dimension == "edits_approved" {
			editsRanking = &result.Rankings[i]
			break
		}
	}
	suite.Require().NotNil(editsRanking)
	suite.Equal(int64(2), editsRanking.Value, "Should count 1 revision + 1 approved edit")
}

func (suite *ContributorProfileServiceIntegrationTestSuite) TestPercentileRankings_RequestsFulfilledDimension() {
	users := suite.createManyUsers("reqperc", 10)

	// Create a fulfilled request by user 0
	req := &models.Request{
		Title:       "Find artist info",
		EntityType:  "artist",
		RequesterID: users[1].ID,
		FulfillerID: &users[0].ID,
		Status:      models.RequestStatusFulfilled,
	}
	suite.Require().NoError(suite.db.Create(req).Error)

	result, err := suite.profileService.GetPercentileRankings(users[0].ID)
	suite.Require().NoError(err)
	suite.Require().NotNil(result)

	var reqRanking *contracts.PercentileRanking
	for i := range result.Rankings {
		if result.Rankings[i].Dimension == "requests_fulfilled" {
			reqRanking = &result.Rankings[i]
			break
		}
	}
	suite.Require().NotNil(reqRanking)
	suite.Equal(int64(1), reqRanking.Value)
}

func (suite *ContributorProfileServiceIntegrationTestSuite) TestPercentileRankings_WeightedOverallScore() {
	users := suite.createManyUsers("overall", 10)

	// Give user 0 shows (weight 25) and venues (weight 15)
	for i := 0; i < 5; i++ {
		suite.createShow(users[0].ID, fmt.Sprintf("Show %d", i))
	}
	suite.createVenue(users[0].ID, "Venue 1")
	suite.createVenue(users[0].ID, "Venue 2")

	result, err := suite.profileService.GetPercentileRankings(users[0].ID)
	suite.Require().NoError(err)
	suite.Require().NotNil(result)

	// Shows: 90th percentile (weight 25), Venues: 90th (weight 15), others: 0 (weight 10+25+10=45)
	// Expected: (90*25 + 90*15 + 0*10 + 0*25 + 0*10) / (25+15+10+25+10) = 3600 / 85 = 42
	suite.Equal(42, result.OverallScore)
}

func (suite *ContributorProfileServiceIntegrationTestSuite) TestPercentileRankings_AllDimensionsPresent() {
	users := suite.createManyUsers("alldims", 10)

	result, err := suite.profileService.GetPercentileRankings(users[0].ID)
	suite.Require().NoError(err)
	suite.Require().NotNil(result)

	suite.Require().Len(result.Rankings, 5, "Should have 5 ranking dimensions")

	expectedDimensions := map[string]bool{
		"shows_submitted":    false,
		"venues_submitted":   false,
		"tags_applied":       false,
		"edits_approved":     false,
		"requests_fulfilled": false,
	}
	for _, r := range result.Rankings {
		_, exists := expectedDimensions[r.Dimension]
		suite.True(exists, "Unexpected dimension: %s", r.Dimension)
		expectedDimensions[r.Dimension] = true
		suite.NotEmpty(r.Label, "Dimension %s should have a label", r.Dimension)
	}
	for dim, found := range expectedDimensions {
		suite.True(found, "Missing dimension: %s", dim)
	}
}

func (suite *ContributorProfileServiceIntegrationTestSuite) TestPercentileRankings_ExactlyTenUsers() {
	// Exactly 10 users should work (threshold is < 10 returns nil)
	suite.createManyUsers("exact", 10)

	result, err := suite.profileService.GetPercentileRankings(1) // any valid user
	suite.Require().NoError(err)
	suite.Require().NotNil(result, "Exactly 10 users should be enough for rankings")
}
