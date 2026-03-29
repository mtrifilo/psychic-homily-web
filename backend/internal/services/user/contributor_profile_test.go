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

func TestNewContributorProfileService(t *testing.T) {
	svc := NewContributorProfileService(nil)
	assert.NotNil(t, svc)
}

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
