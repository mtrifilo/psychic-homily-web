package community

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	authm "psychic-homily-backend/internal/models/auth"
	catalogm "psychic-homily-backend/internal/models/catalog"
	engagementm "psychic-homily-backend/internal/models/engagement"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/services/engagement"
	usersvc "psychic-homily-backend/internal/services/user"
	"psychic-homily-backend/internal/utils"
)

type ContributorProfileHandlerIntegrationSuite struct {
	suite.Suite
	deps           *testhelpers.IntegrationDeps
	handler        *ContributorProfileHandler
	profileService *usersvc.ContributorProfileService
}

func (s *ContributorProfileHandlerIntegrationSuite) SetupSuite() {
	s.deps = testhelpers.SetupIntegrationDeps(s.T())
	s.profileService = usersvc.NewContributorProfileService(s.deps.DB)
	s.handler = NewContributorProfileHandler(
		s.profileService, s.deps.UserService,
		engagement.NewFollowService(s.deps.DB),
		engagement.NewAttendanceService(s.deps.DB),
		engagement.NewCommentService(s.deps.DB, utils.NewMarkdownRenderer()),
	)
}

func (s *ContributorProfileHandlerIntegrationSuite) TearDownTest() {
	sqlDB, _ := s.deps.DB.DB()
	_, _ = sqlDB.Exec("DELETE FROM user_profile_sections")
	// comments is not covered by CleanupTables; the PSY-1046 field-note
	// tests seed it directly.
	_, _ = sqlDB.Exec("DELETE FROM comments")
	testhelpers.CleanupTables(s.deps.DB)
}

func (s *ContributorProfileHandlerIntegrationSuite) TearDownSuite() {
	s.deps.TestDB.Cleanup()
}

func TestContributorProfileHandlerIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	suite.Run(t, new(ContributorProfileHandlerIntegrationSuite))
}

// --- Helpers ---

func (s *ContributorProfileHandlerIntegrationSuite) createUserWithUsername(username string) *authm.User {
	user := &authm.User{
		Email:         testhelpers.StringPtr(fmt.Sprintf("%s@test.com", username)),
		Username:      testhelpers.StringPtr(username),
		FirstName:     testhelpers.StringPtr("Test"),
		LastName:      testhelpers.StringPtr("User"),
		IsActive:      true,
		EmailVerified: true,
	}
	s.deps.DB.Create(user)
	return user
}

func (s *ContributorProfileHandlerIntegrationSuite) createPrivateUser(username string) *authm.User {
	user := s.createUserWithUsername(username)
	s.deps.DB.Model(user).Update("profile_visibility", "private")
	user.ProfileVisibility = "private"
	return user
}

func (s *ContributorProfileHandlerIntegrationSuite) setPrivacySettings(user *authm.User, settings contracts.PrivacySettings) {
	raw, err := json.Marshal(settings)
	s.Require().NoError(err)
	rawMsg := json.RawMessage(raw)
	s.deps.DB.Model(user).Update("privacy_settings", &rawMsg)
}

// =============================================================================
// GetPublicProfileHandler
// =============================================================================

func (s *ContributorProfileHandlerIntegrationSuite) TestGetPublicProfile_Success() {
	_ = s.createUserWithUsername("publicuser")

	req := &GetPublicProfileRequest{Username: "publicuser"}
	resp, err := s.handler.GetPublicProfileHandler(s.deps.Ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.Equal("publicuser", resp.Body.Username)
	s.Equal("public", resp.Body.ProfileVisibility)
}

func (s *ContributorProfileHandlerIntegrationSuite) TestGetPublicProfile_NotFound() {
	req := &GetPublicProfileRequest{Username: "nonexistent"}
	_, err := s.handler.GetPublicProfileHandler(s.deps.Ctx, req)
	testhelpers.AssertHumaError(s.T(), err, 404)
}

func (s *ContributorProfileHandlerIntegrationSuite) TestGetPublicProfile_PrivateProfile_NotOwner() {
	_ = s.createPrivateUser("privateuser")

	req := &GetPublicProfileRequest{Username: "privateuser"}
	// View as anonymous — should get 404
	_, err := s.handler.GetPublicProfileHandler(s.deps.Ctx, req)
	testhelpers.AssertHumaError(s.T(), err, 404)
}

func (s *ContributorProfileHandlerIntegrationSuite) TestGetPublicProfile_PrivateProfile_AsOwner() {
	user := s.createPrivateUser("privateowner")
	ctx := testhelpers.CtxWithUser(user)

	req := &GetPublicProfileRequest{Username: "privateowner"}
	resp, err := s.handler.GetPublicProfileHandler(ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.Equal("privateowner", resp.Body.Username)
}

func (s *ContributorProfileHandlerIntegrationSuite) TestGetPublicProfile_OwnerSeesPrivacySettings() {
	user := s.createUserWithUsername("ownerview")
	ctx := testhelpers.CtxWithUser(user)

	req := &GetPublicProfileRequest{Username: "ownerview"}
	resp, err := s.handler.GetPublicProfileHandler(ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.NotNil(resp.Body.PrivacySettings, "owner should see privacy settings")
	s.NotNil(resp.Body.Stats, "owner should see stats")
}

func (s *ContributorProfileHandlerIntegrationSuite) TestGetPublicProfile_NonOwnerDoesNotSeePrivacySettings() {
	s.createUserWithUsername("targetuser")
	viewer := s.createUserWithUsername("vieweruser")
	ctx := testhelpers.CtxWithUser(viewer)

	req := &GetPublicProfileRequest{Username: "targetuser"}
	resp, err := s.handler.GetPublicProfileHandler(ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.Nil(resp.Body.PrivacySettings, "non-owner should not see privacy settings")
}

// =============================================================================
// GetOwnProfileHandler
// =============================================================================

func (s *ContributorProfileHandlerIntegrationSuite) TestGetOwnProfile_Success() {
	user := s.createUserWithUsername("ownprofile")
	ctx := testhelpers.CtxWithUser(user)

	resp, err := s.handler.GetOwnProfileHandler(ctx, &struct{}{})
	s.NoError(err)
	s.NotNil(resp)
	s.Equal("ownprofile", resp.Body.Username)
	s.NotNil(resp.Body.PrivacySettings, "own profile should always include privacy settings")
	s.NotNil(resp.Body.Stats, "own profile should always include stats")
	s.NotNil(resp.Body.LastActive, "own profile should always include last_active")
}

func (s *ContributorProfileHandlerIntegrationSuite) TestGetOwnProfile_Unauthenticated() {
	_, err := s.handler.GetOwnProfileHandler(context.Background(), &struct{}{})
	testhelpers.AssertHumaError(s.T(), err, 401)
}

// =============================================================================
// UpdateProfileVisibilityHandler
// =============================================================================

func (s *ContributorProfileHandlerIntegrationSuite) TestUpdateVisibility_SetPrivate() {
	user := s.createUserWithUsername("visuser")
	ctx := testhelpers.CtxWithUser(user)

	req := &UpdateProfileVisibilityRequest{}
	req.Body.Visibility = "private"

	resp, err := s.handler.UpdateProfileVisibilityHandler(ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.True(resp.Body.Success)
	s.Equal("private", resp.Body.Visibility)
}

func (s *ContributorProfileHandlerIntegrationSuite) TestUpdateVisibility_SetPublic() {
	user := s.createPrivateUser("visuser2")
	ctx := testhelpers.CtxWithUser(user)

	req := &UpdateProfileVisibilityRequest{}
	req.Body.Visibility = "public"

	resp, err := s.handler.UpdateProfileVisibilityHandler(ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.True(resp.Body.Success)
	s.Equal("public", resp.Body.Visibility)
}

func (s *ContributorProfileHandlerIntegrationSuite) TestUpdateVisibility_InvalidValue() {
	user := s.createUserWithUsername("visuser3")
	ctx := testhelpers.CtxWithUser(user)

	req := &UpdateProfileVisibilityRequest{}
	req.Body.Visibility = "invalid"

	_, err := s.handler.UpdateProfileVisibilityHandler(ctx, req)
	testhelpers.AssertHumaError(s.T(), err, 422)
}

func (s *ContributorProfileHandlerIntegrationSuite) TestUpdateVisibility_Unauthenticated() {
	req := &UpdateProfileVisibilityRequest{}
	req.Body.Visibility = "private"

	_, err := s.handler.UpdateProfileVisibilityHandler(context.Background(), req)
	testhelpers.AssertHumaError(s.T(), err, 401)
}

// =============================================================================
// UpdatePrivacySettingsHandler
// =============================================================================

func (s *ContributorProfileHandlerIntegrationSuite) TestUpdatePrivacySettings_Success() {
	user := s.createUserWithUsername("privacyuser")
	ctx := testhelpers.CtxWithUser(user)

	req := &UpdatePrivacySettingsRequest{}
	req.Body = contracts.PrivacySettings{
		Contributions:   contracts.PrivacyVisible,
		SavedShows:      contracts.PrivacyHidden,
		Attendance:      contracts.PrivacyCountOnly,
		Following:       contracts.PrivacyVisible,
		Collections:     contracts.PrivacyVisible,
		LastActive:      contracts.PrivacyHidden,
		ProfileSections: contracts.PrivacyVisible,
	}

	resp, err := s.handler.UpdatePrivacySettingsHandler(ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.True(resp.Body.Success)
	s.Equal(contracts.PrivacyHidden, resp.Body.Settings.LastActive)
	s.Equal(contracts.PrivacyCountOnly, resp.Body.Settings.Attendance)
}

func (s *ContributorProfileHandlerIntegrationSuite) TestUpdatePrivacySettings_InvalidLevel() {
	user := s.createUserWithUsername("privacyuser2")
	ctx := testhelpers.CtxWithUser(user)

	req := &UpdatePrivacySettingsRequest{}
	req.Body = contracts.PrivacySettings{
		Contributions:   "invalid_level",
		SavedShows:      contracts.PrivacyHidden,
		Attendance:      contracts.PrivacyHidden,
		Following:       contracts.PrivacyHidden,
		Collections:     contracts.PrivacyHidden,
		LastActive:      contracts.PrivacyHidden,
		ProfileSections: contracts.PrivacyHidden,
	}

	_, err := s.handler.UpdatePrivacySettingsHandler(ctx, req)
	testhelpers.AssertHumaError(s.T(), err, 422)
}

func (s *ContributorProfileHandlerIntegrationSuite) TestUpdatePrivacySettings_BinaryFieldCountOnly() {
	user := s.createUserWithUsername("privacyuser3")
	ctx := testhelpers.CtxWithUser(user)

	req := &UpdatePrivacySettingsRequest{}
	req.Body = contracts.PrivacySettings{
		Contributions:   contracts.PrivacyVisible,
		SavedShows:      contracts.PrivacyHidden,
		Attendance:      contracts.PrivacyHidden,
		Following:       contracts.PrivacyHidden,
		Collections:     contracts.PrivacyHidden,
		LastActive:      contracts.PrivacyCountOnly, // binary-only field
		ProfileSections: contracts.PrivacyVisible,
	}

	_, err := s.handler.UpdatePrivacySettingsHandler(ctx, req)
	testhelpers.AssertHumaError(s.T(), err, 422)
}

func (s *ContributorProfileHandlerIntegrationSuite) TestUpdatePrivacySettings_Unauthenticated() {
	req := &UpdatePrivacySettingsRequest{}
	req.Body = contracts.DefaultPrivacySettings()

	_, err := s.handler.UpdatePrivacySettingsHandler(context.Background(), req)
	testhelpers.AssertHumaError(s.T(), err, 401)
}

// =============================================================================
// GetContributionHistoryHandler
// =============================================================================

func (s *ContributorProfileHandlerIntegrationSuite) TestGetContributionHistory_Success() {
	user := s.createUserWithUsername("contribuser")
	// Create a show submitted by this user to produce a contribution entry
	testhelpers.CreateApprovedShow(s.deps.DB, user.ID, "Test Show")

	req := &GetContributionHistoryRequest{Username: "contribuser", Limit: 20, Offset: 0}
	resp, err := s.handler.GetContributionHistoryHandler(s.deps.Ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.GreaterOrEqual(resp.Body.Total, int64(1))
}

func (s *ContributorProfileHandlerIntegrationSuite) TestGetContributionHistory_UserNotFound() {
	req := &GetContributionHistoryRequest{Username: "ghostuser", Limit: 20, Offset: 0}
	_, err := s.handler.GetContributionHistoryHandler(s.deps.Ctx, req)
	testhelpers.AssertHumaError(s.T(), err, 404)
}

func (s *ContributorProfileHandlerIntegrationSuite) TestGetContributionHistory_PrivateProfile_NotOwner() {
	s.createPrivateUser("privatecontrib")

	req := &GetContributionHistoryRequest{Username: "privatecontrib", Limit: 20, Offset: 0}
	_, err := s.handler.GetContributionHistoryHandler(s.deps.Ctx, req)
	testhelpers.AssertHumaError(s.T(), err, 404)
}

func (s *ContributorProfileHandlerIntegrationSuite) TestGetContributionHistory_PrivateProfile_AsOwner() {
	user := s.createPrivateUser("privatecontribowner")
	ctx := testhelpers.CtxWithUser(user)

	req := &GetContributionHistoryRequest{Username: "privatecontribowner", Limit: 20, Offset: 0}
	resp, err := s.handler.GetContributionHistoryHandler(ctx, req)
	s.NoError(err)
	s.NotNil(resp)
}

func (s *ContributorProfileHandlerIntegrationSuite) TestGetContributionHistory_PrivacyHidden() {
	user := s.createUserWithUsername("hiddencontrib")
	s.setPrivacySettings(user, contracts.PrivacySettings{
		Contributions:   contracts.PrivacyHidden,
		SavedShows:      contracts.PrivacyHidden,
		Attendance:      contracts.PrivacyHidden,
		Following:       contracts.PrivacyHidden,
		Collections:     contracts.PrivacyHidden,
		LastActive:      contracts.PrivacyHidden,
		ProfileSections: contracts.PrivacyHidden,
	})

	// View as a different user
	viewer := s.createUserWithUsername("hiddenviewer")
	ctx := testhelpers.CtxWithUser(viewer)

	req := &GetContributionHistoryRequest{Username: "hiddencontrib", Limit: 20, Offset: 0}
	_, err := s.handler.GetContributionHistoryHandler(ctx, req)
	testhelpers.AssertHumaError(s.T(), err, 404)
}

func (s *ContributorProfileHandlerIntegrationSuite) TestGetContributionHistory_PrivacyCountOnly() {
	user := s.createUserWithUsername("countcontrib")
	testhelpers.CreateApprovedShow(s.deps.DB, user.ID, "Count Only Show")

	s.setPrivacySettings(user, contracts.PrivacySettings{
		Contributions:   contracts.PrivacyCountOnly,
		SavedShows:      contracts.PrivacyHidden,
		Attendance:      contracts.PrivacyHidden,
		Following:       contracts.PrivacyHidden,
		Collections:     contracts.PrivacyHidden,
		LastActive:      contracts.PrivacyHidden,
		ProfileSections: contracts.PrivacyHidden,
	})

	viewer := s.createUserWithUsername("countviewer")
	ctx := testhelpers.CtxWithUser(viewer)

	req := &GetContributionHistoryRequest{Username: "countcontrib", Limit: 20, Offset: 0}
	resp, err := s.handler.GetContributionHistoryHandler(ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.GreaterOrEqual(resp.Body.Total, int64(1), "count_only should still return total count")
	s.Empty(resp.Body.Contributions, "count_only should return empty contributions list")
}

// =============================================================================
// GetOwnContributionsHandler
// =============================================================================

func (s *ContributorProfileHandlerIntegrationSuite) TestGetOwnContributions_Success() {
	user := s.createUserWithUsername("owncontrib")
	testhelpers.CreateApprovedShow(s.deps.DB, user.ID, "Own Show")
	ctx := testhelpers.CtxWithUser(user)

	req := &GetOwnContributionsRequest{Limit: 20, Offset: 0}
	resp, err := s.handler.GetOwnContributionsHandler(ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.GreaterOrEqual(resp.Body.Total, int64(1))
}

func (s *ContributorProfileHandlerIntegrationSuite) TestGetOwnContributions_Unauthenticated() {
	req := &GetOwnContributionsRequest{Limit: 20, Offset: 0}
	_, err := s.handler.GetOwnContributionsHandler(context.Background(), req)
	testhelpers.AssertHumaError(s.T(), err, 401)
}

// =============================================================================
// Section CRUD: CreateSectionHandler
// =============================================================================

func (s *ContributorProfileHandlerIntegrationSuite) TestCreateSection_Success() {
	user := s.createUserWithUsername("sectionuser")
	ctx := testhelpers.CtxWithUser(user)

	req := &CreateSectionRequest{}
	req.Body.Title = "My Section"
	req.Body.Content = "Some content here"
	req.Body.Position = 0

	resp, err := s.handler.CreateSectionHandler(ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.Equal("My Section", resp.Body.Title)
	s.Equal("Some content here", resp.Body.Content)
	s.Equal(0, resp.Body.Position)
	s.True(resp.Body.IsVisible)
}

func (s *ContributorProfileHandlerIntegrationSuite) TestCreateSection_Unauthenticated() {
	req := &CreateSectionRequest{}
	req.Body.Title = "No Auth Section"
	req.Body.Content = "Content"
	req.Body.Position = 0

	_, err := s.handler.CreateSectionHandler(context.Background(), req)
	testhelpers.AssertHumaError(s.T(), err, 401)
}

func (s *ContributorProfileHandlerIntegrationSuite) TestCreateSection_EmptyTitle() {
	user := s.createUserWithUsername("emptytitle")
	ctx := testhelpers.CtxWithUser(user)

	req := &CreateSectionRequest{}
	req.Body.Title = ""
	req.Body.Content = "Content"
	req.Body.Position = 0

	_, err := s.handler.CreateSectionHandler(ctx, req)
	testhelpers.AssertHumaError(s.T(), err, 422)
}

func (s *ContributorProfileHandlerIntegrationSuite) TestCreateSection_InvalidPosition() {
	user := s.createUserWithUsername("badposition")
	ctx := testhelpers.CtxWithUser(user)

	req := &CreateSectionRequest{}
	req.Body.Title = "Valid Title"
	req.Body.Content = "Content"
	req.Body.Position = 5 // max is 2

	_, err := s.handler.CreateSectionHandler(ctx, req)
	testhelpers.AssertHumaError(s.T(), err, 422)
}

func (s *ContributorProfileHandlerIntegrationSuite) TestCreateSection_MaxSectionsExceeded() {
	user := s.createUserWithUsername("maxsections")
	ctx := testhelpers.CtxWithUser(user)

	// Create 3 sections (the max)
	for i := 0; i < 3; i++ {
		req := &CreateSectionRequest{}
		req.Body.Title = fmt.Sprintf("Section %d", i)
		req.Body.Content = "Content"
		req.Body.Position = i
		_, err := s.handler.CreateSectionHandler(ctx, req)
		s.Require().NoError(err)
	}

	// Fourth should fail
	req := &CreateSectionRequest{}
	req.Body.Title = "One Too Many"
	req.Body.Content = "Content"
	req.Body.Position = 0

	_, err := s.handler.CreateSectionHandler(ctx, req)
	testhelpers.AssertHumaError(s.T(), err, 422)
}

// =============================================================================
// Section CRUD: UpdateSectionHandler
// =============================================================================

func (s *ContributorProfileHandlerIntegrationSuite) TestUpdateSection_Success() {
	user := s.createUserWithUsername("updateuser")
	ctx := testhelpers.CtxWithUser(user)

	// Create a section first
	createReq := &CreateSectionRequest{}
	createReq.Body.Title = "Original"
	createReq.Body.Content = "Original content"
	createReq.Body.Position = 0
	createResp, err := s.handler.CreateSectionHandler(ctx, createReq)
	s.Require().NoError(err)

	// Update it
	newTitle := "Updated Title"
	updateReq := &UpdateSectionRequest{SectionID: fmt.Sprintf("%d", createResp.Body.ID)}
	updateReq.Body.Title = &newTitle

	resp, err := s.handler.UpdateSectionHandler(ctx, updateReq)
	s.NoError(err)
	s.NotNil(resp)
	s.Equal("Updated Title", resp.Body.Title)
}

func (s *ContributorProfileHandlerIntegrationSuite) TestUpdateSection_ToggleVisibility() {
	user := s.createUserWithUsername("togglevisuser")
	ctx := testhelpers.CtxWithUser(user)

	createReq := &CreateSectionRequest{}
	createReq.Body.Title = "Visible Section"
	createReq.Body.Content = "Content"
	createReq.Body.Position = 0
	createResp, err := s.handler.CreateSectionHandler(ctx, createReq)
	s.Require().NoError(err)
	s.True(createResp.Body.IsVisible)

	// Toggle to hidden
	hidden := false
	updateReq := &UpdateSectionRequest{SectionID: fmt.Sprintf("%d", createResp.Body.ID)}
	updateReq.Body.IsVisible = &hidden

	resp, err := s.handler.UpdateSectionHandler(ctx, updateReq)
	s.NoError(err)
	s.NotNil(resp)
	s.False(resp.Body.IsVisible)
}

func (s *ContributorProfileHandlerIntegrationSuite) TestUpdateSection_NotFound() {
	user := s.createUserWithUsername("updatemissing")
	ctx := testhelpers.CtxWithUser(user)

	newTitle := "Ghost Update"
	updateReq := &UpdateSectionRequest{SectionID: "99999"}
	updateReq.Body.Title = &newTitle

	_, err := s.handler.UpdateSectionHandler(ctx, updateReq)
	testhelpers.AssertHumaError(s.T(), err, 404)
}

func (s *ContributorProfileHandlerIntegrationSuite) TestUpdateSection_OtherUsersSection() {
	owner := s.createUserWithUsername("sectionowner")
	other := s.createUserWithUsername("sectionthief")

	ownerCtx := testhelpers.CtxWithUser(owner)
	createReq := &CreateSectionRequest{}
	createReq.Body.Title = "Owner Section"
	createReq.Body.Content = "Confidential"
	createReq.Body.Position = 0
	createResp, err := s.handler.CreateSectionHandler(ownerCtx, createReq)
	s.Require().NoError(err)

	// Try to update as the other user
	otherCtx := testhelpers.CtxWithUser(other)
	newTitle := "Hacked"
	updateReq := &UpdateSectionRequest{SectionID: fmt.Sprintf("%d", createResp.Body.ID)}
	updateReq.Body.Title = &newTitle

	_, err = s.handler.UpdateSectionHandler(otherCtx, updateReq)
	testhelpers.AssertHumaError(s.T(), err, 404)
}

func (s *ContributorProfileHandlerIntegrationSuite) TestUpdateSection_InvalidSectionID() {
	user := s.createUserWithUsername("badsectionid")
	ctx := testhelpers.CtxWithUser(user)

	newTitle := "Broken"
	updateReq := &UpdateSectionRequest{SectionID: "not-a-number"}
	updateReq.Body.Title = &newTitle

	_, err := s.handler.UpdateSectionHandler(ctx, updateReq)
	testhelpers.AssertHumaError(s.T(), err, 400)
}

func (s *ContributorProfileHandlerIntegrationSuite) TestUpdateSection_NoFields() {
	user := s.createUserWithUsername("nofielduser")
	ctx := testhelpers.CtxWithUser(user)

	createReq := &CreateSectionRequest{}
	createReq.Body.Title = "No Field Section"
	createReq.Body.Content = "Content"
	createReq.Body.Position = 0
	createResp, err := s.handler.CreateSectionHandler(ctx, createReq)
	s.Require().NoError(err)

	// Send update with no fields
	updateReq := &UpdateSectionRequest{SectionID: fmt.Sprintf("%d", createResp.Body.ID)}
	_, err = s.handler.UpdateSectionHandler(ctx, updateReq)
	testhelpers.AssertHumaError(s.T(), err, 422)
}

func (s *ContributorProfileHandlerIntegrationSuite) TestUpdateSection_Unauthenticated() {
	newTitle := "No Auth"
	updateReq := &UpdateSectionRequest{SectionID: "1"}
	updateReq.Body.Title = &newTitle

	_, err := s.handler.UpdateSectionHandler(context.Background(), updateReq)
	testhelpers.AssertHumaError(s.T(), err, 401)
}

// =============================================================================
// Section CRUD: DeleteSectionHandler
// =============================================================================

func (s *ContributorProfileHandlerIntegrationSuite) TestDeleteSection_Success() {
	user := s.createUserWithUsername("deleteuser")
	ctx := testhelpers.CtxWithUser(user)

	createReq := &CreateSectionRequest{}
	createReq.Body.Title = "Doomed Section"
	createReq.Body.Content = "Content"
	createReq.Body.Position = 0
	createResp, err := s.handler.CreateSectionHandler(ctx, createReq)
	s.Require().NoError(err)

	deleteReq := &DeleteSectionRequest{SectionID: fmt.Sprintf("%d", createResp.Body.ID)}
	resp, err := s.handler.DeleteSectionHandler(ctx, deleteReq)
	s.NoError(err)
	s.NotNil(resp)
	s.True(resp.Body.Success)
	s.Equal("Section deleted", resp.Body.Message)

	// Verify it's gone
	sectionsResp, err := s.handler.GetOwnSectionsHandler(ctx, &struct{}{})
	s.NoError(err)
	s.Empty(sectionsResp.Body.Sections)
}

func (s *ContributorProfileHandlerIntegrationSuite) TestDeleteSection_NotFound() {
	user := s.createUserWithUsername("deletemissing")
	ctx := testhelpers.CtxWithUser(user)

	deleteReq := &DeleteSectionRequest{SectionID: "99999"}
	_, err := s.handler.DeleteSectionHandler(ctx, deleteReq)
	testhelpers.AssertHumaError(s.T(), err, 404)
}

func (s *ContributorProfileHandlerIntegrationSuite) TestDeleteSection_OtherUsersSection() {
	owner := s.createUserWithUsername("delowner")
	other := s.createUserWithUsername("delthief")

	ownerCtx := testhelpers.CtxWithUser(owner)
	createReq := &CreateSectionRequest{}
	createReq.Body.Title = "Protected Section"
	createReq.Body.Content = "Content"
	createReq.Body.Position = 0
	createResp, err := s.handler.CreateSectionHandler(ownerCtx, createReq)
	s.Require().NoError(err)

	otherCtx := testhelpers.CtxWithUser(other)
	deleteReq := &DeleteSectionRequest{SectionID: fmt.Sprintf("%d", createResp.Body.ID)}
	_, err = s.handler.DeleteSectionHandler(otherCtx, deleteReq)
	testhelpers.AssertHumaError(s.T(), err, 404)
}

func (s *ContributorProfileHandlerIntegrationSuite) TestDeleteSection_InvalidSectionID() {
	user := s.createUserWithUsername("delbadid")
	ctx := testhelpers.CtxWithUser(user)

	deleteReq := &DeleteSectionRequest{SectionID: "abc"}
	_, err := s.handler.DeleteSectionHandler(ctx, deleteReq)
	testhelpers.AssertHumaError(s.T(), err, 400)
}

func (s *ContributorProfileHandlerIntegrationSuite) TestDeleteSection_Unauthenticated() {
	deleteReq := &DeleteSectionRequest{SectionID: "1"}
	_, err := s.handler.DeleteSectionHandler(context.Background(), deleteReq)
	testhelpers.AssertHumaError(s.T(), err, 401)
}

// =============================================================================
// GetOwnSectionsHandler
// =============================================================================

func (s *ContributorProfileHandlerIntegrationSuite) TestGetOwnSections_Success() {
	user := s.createUserWithUsername("ownsections")
	ctx := testhelpers.CtxWithUser(user)

	// Create two sections
	for i := 0; i < 2; i++ {
		req := &CreateSectionRequest{}
		req.Body.Title = fmt.Sprintf("Section %d", i)
		req.Body.Content = "Content"
		req.Body.Position = i
		_, err := s.handler.CreateSectionHandler(ctx, req)
		s.Require().NoError(err)
	}

	resp, err := s.handler.GetOwnSectionsHandler(ctx, &struct{}{})
	s.NoError(err)
	s.NotNil(resp)
	s.Len(resp.Body.Sections, 2)
}

func (s *ContributorProfileHandlerIntegrationSuite) TestGetOwnSections_IncludesHiddenSections() {
	user := s.createUserWithUsername("ownsectionshidden")
	ctx := testhelpers.CtxWithUser(user)

	// Create a visible section
	req := &CreateSectionRequest{}
	req.Body.Title = "Visible"
	req.Body.Content = "Content"
	req.Body.Position = 0
	createResp, err := s.handler.CreateSectionHandler(ctx, req)
	s.Require().NoError(err)

	// Hide it
	hidden := false
	updateReq := &UpdateSectionRequest{SectionID: fmt.Sprintf("%d", createResp.Body.ID)}
	updateReq.Body.IsVisible = &hidden
	_, err = s.handler.UpdateSectionHandler(ctx, updateReq)
	s.Require().NoError(err)

	// GetOwnSections should still return it
	resp, err := s.handler.GetOwnSectionsHandler(ctx, &struct{}{})
	s.NoError(err)
	s.Len(resp.Body.Sections, 1)
	s.False(resp.Body.Sections[0].IsVisible)
}

func (s *ContributorProfileHandlerIntegrationSuite) TestGetOwnSections_Empty() {
	user := s.createUserWithUsername("emptysections")
	ctx := testhelpers.CtxWithUser(user)

	resp, err := s.handler.GetOwnSectionsHandler(ctx, &struct{}{})
	s.NoError(err)
	s.NotNil(resp)
	s.Empty(resp.Body.Sections)
}

func (s *ContributorProfileHandlerIntegrationSuite) TestGetOwnSections_Unauthenticated() {
	_, err := s.handler.GetOwnSectionsHandler(context.Background(), &struct{}{})
	testhelpers.AssertHumaError(s.T(), err, 401)
}

// =============================================================================
// GetUserSectionsHandler (public)
// =============================================================================

func (s *ContributorProfileHandlerIntegrationSuite) TestGetUserSections_Success() {
	user := s.createUserWithUsername("pubsections")
	ctx := testhelpers.CtxWithUser(user)

	req := &CreateSectionRequest{}
	req.Body.Title = "Public Section"
	req.Body.Content = "Content"
	req.Body.Position = 0
	_, err := s.handler.CreateSectionHandler(ctx, req)
	s.Require().NoError(err)

	// View as anonymous
	sectReq := &GetUserSectionsRequest{Username: "pubsections"}
	resp, err := s.handler.GetUserSectionsHandler(s.deps.Ctx, sectReq)
	s.NoError(err)
	s.NotNil(resp)
	s.Len(resp.Body.Sections, 1)
	s.Equal("Public Section", resp.Body.Sections[0].Title)
}

func (s *ContributorProfileHandlerIntegrationSuite) TestGetUserSections_HiddenSectionNotVisible() {
	user := s.createUserWithUsername("hiddensecpub")
	ctx := testhelpers.CtxWithUser(user)

	// Create and hide a section
	req := &CreateSectionRequest{}
	req.Body.Title = "Hidden"
	req.Body.Content = "Secret"
	req.Body.Position = 0
	createResp, err := s.handler.CreateSectionHandler(ctx, req)
	s.Require().NoError(err)

	hidden := false
	updateReq := &UpdateSectionRequest{SectionID: fmt.Sprintf("%d", createResp.Body.ID)}
	updateReq.Body.IsVisible = &hidden
	_, err = s.handler.UpdateSectionHandler(ctx, updateReq)
	s.Require().NoError(err)

	// View as anonymous — hidden section should not appear
	viewer := s.createUserWithUsername("secviewer")
	viewerCtx := testhelpers.CtxWithUser(viewer)

	sectReq := &GetUserSectionsRequest{Username: "hiddensecpub"}
	resp, err := s.handler.GetUserSectionsHandler(viewerCtx, sectReq)
	s.NoError(err)
	s.Empty(resp.Body.Sections)
}

func (s *ContributorProfileHandlerIntegrationSuite) TestGetUserSections_OwnerSeesHiddenSections() {
	user := s.createUserWithUsername("ownerseeshidden")
	ctx := testhelpers.CtxWithUser(user)

	req := &CreateSectionRequest{}
	req.Body.Title = "Hidden From Public"
	req.Body.Content = "Secret Content"
	req.Body.Position = 0
	createResp, err := s.handler.CreateSectionHandler(ctx, req)
	s.Require().NoError(err)

	hidden := false
	updateReq := &UpdateSectionRequest{SectionID: fmt.Sprintf("%d", createResp.Body.ID)}
	updateReq.Body.IsVisible = &hidden
	_, err = s.handler.UpdateSectionHandler(ctx, updateReq)
	s.Require().NoError(err)

	// View own sections through public endpoint as owner
	sectReq := &GetUserSectionsRequest{Username: "ownerseeshidden"}
	resp, err := s.handler.GetUserSectionsHandler(ctx, sectReq)
	s.NoError(err)
	s.Len(resp.Body.Sections, 1, "owner should see hidden sections via public endpoint")
}

func (s *ContributorProfileHandlerIntegrationSuite) TestGetUserSections_UserNotFound() {
	sectReq := &GetUserSectionsRequest{Username: "nosuchuser"}
	_, err := s.handler.GetUserSectionsHandler(s.deps.Ctx, sectReq)
	testhelpers.AssertHumaError(s.T(), err, 404)
}

func (s *ContributorProfileHandlerIntegrationSuite) TestGetUserSections_PrivateProfile() {
	s.createPrivateUser("privatesecuser")

	sectReq := &GetUserSectionsRequest{Username: "privatesecuser"}
	_, err := s.handler.GetUserSectionsHandler(s.deps.Ctx, sectReq)
	testhelpers.AssertHumaError(s.T(), err, 404)
}

func (s *ContributorProfileHandlerIntegrationSuite) TestGetUserSections_PrivacySectionsHidden() {
	user := s.createUserWithUsername("sechiddenpriv")
	ctx := testhelpers.CtxWithUser(user)

	// Create a section
	req := &CreateSectionRequest{}
	req.Body.Title = "Will Be Hidden"
	req.Body.Content = "Content"
	req.Body.Position = 0
	_, err := s.handler.CreateSectionHandler(ctx, req)
	s.Require().NoError(err)

	// Set profile_sections privacy to hidden
	s.setPrivacySettings(user, contracts.PrivacySettings{
		Contributions:   contracts.PrivacyVisible,
		SavedShows:      contracts.PrivacyHidden,
		Attendance:      contracts.PrivacyHidden,
		Following:       contracts.PrivacyHidden,
		Collections:     contracts.PrivacyHidden,
		LastActive:      contracts.PrivacyHidden,
		ProfileSections: contracts.PrivacyHidden,
	})

	// View as another user
	viewer := s.createUserWithUsername("privviewer")
	viewerCtx := testhelpers.CtxWithUser(viewer)

	sectReq := &GetUserSectionsRequest{Username: "sechiddenpriv"}
	resp, err := s.handler.GetUserSectionsHandler(viewerCtx, sectReq)
	s.NoError(err)
	s.NotNil(resp)
	s.Empty(resp.Body.Sections, "sections should be empty when privacy is hidden")
}

// =============================================================================
// GetActivityHeatmapHandler
// =============================================================================

func (s *ContributorProfileHandlerIntegrationSuite) TestGetActivityHeatmap_Success() {
	user := s.createUserWithUsername("heatmapuser")
	testhelpers.CreateApprovedShow(s.deps.DB, user.ID, "Heatmap Show")

	req := &GetActivityHeatmapRequest{Username: "heatmapuser"}
	resp, err := s.handler.GetActivityHeatmapHandler(s.deps.Ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.NotEmpty(resp.Body.Days)
}

func (s *ContributorProfileHandlerIntegrationSuite) TestGetActivityHeatmap_UserNotFound() {
	req := &GetActivityHeatmapRequest{Username: "ghostheatmap"}
	_, err := s.handler.GetActivityHeatmapHandler(s.deps.Ctx, req)
	testhelpers.AssertHumaError(s.T(), err, 404)
}

func (s *ContributorProfileHandlerIntegrationSuite) TestGetActivityHeatmap_PrivateProfile_NotOwner() {
	s.createPrivateUser("privateheatmap")

	req := &GetActivityHeatmapRequest{Username: "privateheatmap"}
	_, err := s.handler.GetActivityHeatmapHandler(s.deps.Ctx, req)
	testhelpers.AssertHumaError(s.T(), err, 404)
}

func (s *ContributorProfileHandlerIntegrationSuite) TestGetActivityHeatmap_PrivateProfile_AsOwner() {
	user := s.createPrivateUser("privateheatmapowner")
	ctx := testhelpers.CtxWithUser(user)

	req := &GetActivityHeatmapRequest{Username: "privateheatmapowner"}
	resp, err := s.handler.GetActivityHeatmapHandler(ctx, req)
	s.NoError(err)
	s.NotNil(resp)
}

func (s *ContributorProfileHandlerIntegrationSuite) TestGetActivityHeatmap_PrivacyHidden_ReturnsEmpty() {
	user := s.createUserWithUsername("hiddenheatmap")
	testhelpers.CreateApprovedShow(s.deps.DB, user.ID, "Hidden Heatmap Show")
	s.setPrivacySettings(user, contracts.PrivacySettings{
		Contributions:   contracts.PrivacyHidden,
		SavedShows:      contracts.PrivacyHidden,
		Attendance:      contracts.PrivacyHidden,
		Following:       contracts.PrivacyHidden,
		Collections:     contracts.PrivacyHidden,
		LastActive:      contracts.PrivacyHidden,
		ProfileSections: contracts.PrivacyHidden,
	})

	viewer := s.createUserWithUsername("heatmapviewer")
	ctx := testhelpers.CtxWithUser(viewer)

	req := &GetActivityHeatmapRequest{Username: "hiddenheatmap"}
	resp, err := s.handler.GetActivityHeatmapHandler(ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.Empty(resp.Body.Days, "hidden privacy should return empty days")
}

func (s *ContributorProfileHandlerIntegrationSuite) TestGetActivityHeatmap_PrivacyCountOnly_StillReturnsData() {
	user := s.createUserWithUsername("countheatmap")
	testhelpers.CreateApprovedShow(s.deps.DB, user.ID, "Count Heatmap Show")
	s.setPrivacySettings(user, contracts.PrivacySettings{
		Contributions:   contracts.PrivacyCountOnly,
		SavedShows:      contracts.PrivacyHidden,
		Attendance:      contracts.PrivacyHidden,
		Following:       contracts.PrivacyHidden,
		Collections:     contracts.PrivacyHidden,
		LastActive:      contracts.PrivacyHidden,
		ProfileSections: contracts.PrivacyHidden,
	})

	viewer := s.createUserWithUsername("countheatmapviewer")
	ctx := testhelpers.CtxWithUser(viewer)

	req := &GetActivityHeatmapRequest{Username: "countheatmap"}
	resp, err := s.handler.GetActivityHeatmapHandler(ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.NotEmpty(resp.Body.Days, "count_only should still return heatmap data")
}

func (s *ContributorProfileHandlerIntegrationSuite) TestGetActivityHeatmap_NoActivity() {
	s.createUserWithUsername("noactheatmap")

	req := &GetActivityHeatmapRequest{Username: "noactheatmap"}
	resp, err := s.handler.GetActivityHeatmapHandler(s.deps.Ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.Empty(resp.Body.Days)
}

// =============================================================================
// PSY-1046: public profile list endpoints
// (following / attended-shows / field-notes)
// =============================================================================

// --- Seed helpers ---

func (s *ContributorProfileHandlerIntegrationSuite) createArtistEntity(name, slug string) *catalogm.Artist {
	artist := &catalogm.Artist{Name: name, Slug: testhelpers.StringPtr(slug)}
	s.deps.DB.Create(artist)
	return artist
}

func (s *ContributorProfileHandlerIntegrationSuite) createVenueEntity(name, slug string) *catalogm.Venue {
	venue := &catalogm.Venue{Name: name, Slug: testhelpers.StringPtr(slug), City: "Phoenix", State: "AZ"}
	s.deps.DB.Create(venue)
	return venue
}

func (s *ContributorProfileHandlerIntegrationSuite) createShowAt(title string, eventDate time.Time, venue *catalogm.Venue) *catalogm.Show {
	show := &catalogm.Show{
		Title:     title,
		EventDate: eventDate,
		City:      testhelpers.StringPtr("Phoenix"),
		State:     testhelpers.StringPtr("AZ"),
		Status:    catalogm.ShowStatusApproved,
	}
	s.deps.DB.Create(show)
	// Slug is auto-set in CreateShow but raw inserts skip that.
	s.deps.DB.Model(show).Update("slug", fmt.Sprintf("show-%d", show.ID))
	if venue != nil {
		s.Require().NoError(s.deps.DB.Model(show).Association("Venues").Append(venue))
	}
	return show
}

func (s *ContributorProfileHandlerIntegrationSuite) bookmark(userID uint, entityType engagementm.BookmarkEntityType, entityID uint, action engagementm.BookmarkAction) {
	s.deps.DB.Create(&engagementm.UserBookmark{
		UserID:     userID,
		EntityType: entityType,
		EntityID:   entityID,
		Action:     action,
	})
}

func (s *ContributorProfileHandlerIntegrationSuite) createFieldNote(userID, showID uint, body string, visibility engagementm.CommentVisibility, createdAt time.Time) *engagementm.Comment {
	note := &engagementm.Comment{
		EntityType: engagementm.CommentEntityShow,
		EntityID:   showID,
		Kind:       engagementm.CommentKindFieldNote,
		UserID:     userID,
		Body:       body,
		BodyHTML:   "<p>" + body + "</p>",
		Visibility: visibility,
		CreatedAt:  createdAt,
		UpdatedAt:  createdAt,
	}
	s.deps.DB.Create(note)
	return note
}

// =============================================================================
// GetUserFollowingHandler
// =============================================================================

func (s *ContributorProfileHandlerIntegrationSuite) TestGetUserFollowing_VisibleList() {
	target := s.createUserWithUsername("followpublic")
	settings := contracts.DefaultPrivacySettings()
	settings.Following = contracts.PrivacyVisible
	s.setPrivacySettings(target, settings)

	a1 := s.createArtistEntity("Just Mustard", "just-mustard")
	a2 := s.createArtistEntity("Wisp", "wisp")
	v1 := s.createVenueEntity("Valley Bar", "valley-bar")
	s.bookmark(target.ID, engagementm.BookmarkEntityArtist, a1.ID, engagementm.BookmarkActionFollow)
	s.bookmark(target.ID, engagementm.BookmarkEntityArtist, a2.ID, engagementm.BookmarkActionFollow)
	s.bookmark(target.ID, engagementm.BookmarkEntityVenue, v1.ID, engagementm.BookmarkActionFollow)

	// Anonymous viewer, type=all: full enriched list.
	resp, err := s.handler.GetUserFollowingHandler(context.Background(), &GetUserFollowingRequest{
		Username: "followpublic", Type: "all", Limit: 20, Offset: 0,
	})
	s.Require().NoError(err)
	s.Equal(int64(3), resp.Body.Total)
	s.Require().Len(resp.Body.Following, 3)

	slugsByName := map[string]string{}
	for _, f := range resp.Body.Following {
		slugsByName[f.Name] = f.Slug
	}
	s.Equal("just-mustard", slugsByName["Just Mustard"])
	s.Equal("valley-bar", slugsByName["Valley Bar"])

	// Type filter narrows to artists only.
	resp, err = s.handler.GetUserFollowingHandler(context.Background(), &GetUserFollowingRequest{
		Username: "followpublic", Type: "artist", Limit: 20, Offset: 0,
	})
	s.Require().NoError(err)
	s.Equal(int64(2), resp.Body.Total)
	s.Require().Len(resp.Body.Following, 2)
	for _, f := range resp.Body.Following {
		s.Equal("artist", f.EntityType)
	}
}

func (s *ContributorProfileHandlerIntegrationSuite) TestGetUserFollowing_DefaultCountOnly() {
	// No stored privacy settings: the default for `following` is count_only,
	// so anonymous viewers get a total but no items. Guarding the default
	// matters — flipping it is an explicit open question on PSY-1045.
	target := s.createUserWithUsername("followdefault")
	a1 := s.createArtistEntity("Crows", "crows")
	s.bookmark(target.ID, engagementm.BookmarkEntityArtist, a1.ID, engagementm.BookmarkActionFollow)

	resp, err := s.handler.GetUserFollowingHandler(context.Background(), &GetUserFollowingRequest{
		Username: "followdefault", Type: "all", Limit: 20, Offset: 0,
	})
	s.Require().NoError(err)
	s.Equal(int64(1), resp.Body.Total)
	s.Empty(resp.Body.Following)
}

func (s *ContributorProfileHandlerIntegrationSuite) TestGetUserFollowing_Hidden() {
	target := s.createUserWithUsername("followhidden")
	settings := contracts.DefaultPrivacySettings()
	settings.Following = contracts.PrivacyHidden
	s.setPrivacySettings(target, settings)

	_, err := s.handler.GetUserFollowingHandler(context.Background(), &GetUserFollowingRequest{
		Username: "followhidden", Type: "all", Limit: 20, Offset: 0,
	})
	testhelpers.AssertHumaError(s.T(), err, 404)
}

func (s *ContributorProfileHandlerIntegrationSuite) TestGetUserFollowing_OwnerSeesFullDespiteHidden() {
	target := s.createUserWithUsername("followowner")
	settings := contracts.DefaultPrivacySettings()
	settings.Following = contracts.PrivacyHidden
	s.setPrivacySettings(target, settings)
	a1 := s.createArtistEntity("Squid", "squid")
	s.bookmark(target.ID, engagementm.BookmarkEntityArtist, a1.ID, engagementm.BookmarkActionFollow)

	resp, err := s.handler.GetUserFollowingHandler(testhelpers.CtxWithUser(target), &GetUserFollowingRequest{
		Username: "followowner", Type: "all", Limit: 20, Offset: 0,
	})
	s.Require().NoError(err)
	s.Equal(int64(1), resp.Body.Total)
	s.Require().Len(resp.Body.Following, 1)
	s.Equal("Squid", resp.Body.Following[0].Name)
}

func (s *ContributorProfileHandlerIntegrationSuite) TestGetUserFollowing_PrivateProfile() {
	target := s.createPrivateUser("followprivate")

	// Anonymous viewer: master gate hides the profile entirely.
	_, err := s.handler.GetUserFollowingHandler(context.Background(), &GetUserFollowingRequest{
		Username: "followprivate", Type: "all", Limit: 20, Offset: 0,
	})
	testhelpers.AssertHumaError(s.T(), err, 404)

	// Owner still passes the master gate.
	_, err = s.handler.GetUserFollowingHandler(testhelpers.CtxWithUser(target), &GetUserFollowingRequest{
		Username: "followprivate", Type: "all", Limit: 20, Offset: 0,
	})
	s.Require().NoError(err)
}

func (s *ContributorProfileHandlerIntegrationSuite) TestGetUserFollowing_InvalidType() {
	s.createUserWithUsername("followbadtype")
	_, err := s.handler.GetUserFollowingHandler(context.Background(), &GetUserFollowingRequest{
		Username: "followbadtype", Type: "banana", Limit: 20, Offset: 0,
	})
	testhelpers.AssertHumaError(s.T(), err, 400)
}

func (s *ContributorProfileHandlerIntegrationSuite) TestGetUserFollowing_UserNotFound() {
	_, err := s.handler.GetUserFollowingHandler(context.Background(), &GetUserFollowingRequest{
		Username: "ghost-user", Type: "all", Limit: 20, Offset: 0,
	})
	testhelpers.AssertHumaError(s.T(), err, 404)
}

// =============================================================================
// GetUserAttendedShowsHandler
// =============================================================================

func (s *ContributorProfileHandlerIntegrationSuite) TestGetUserAttendedShows_VisiblePastOnlyDesc() {
	target := s.createUserWithUsername("diaryuser")
	settings := contracts.DefaultPrivacySettings()
	settings.Attendance = contracts.PrivacyVisible
	s.setPrivacySettings(target, settings)

	venue := s.createVenueEntity("Rebel Lounge", "rebel-lounge")
	now := time.Now().UTC()
	older := s.createShowAt("Older Past Show", now.AddDate(0, -2, 0), venue)
	recent := s.createShowAt("Recent Past Show", now.AddDate(0, -1, 0), venue)
	future := s.createShowAt("Future Show", now.AddDate(0, 1, 0), venue)
	pastInterested := s.createShowAt("Interested Past Show", now.AddDate(0, -3, 0), venue)

	s.bookmark(target.ID, engagementm.BookmarkEntityShow, older.ID, engagementm.BookmarkActionGoing)
	s.bookmark(target.ID, engagementm.BookmarkEntityShow, recent.ID, engagementm.BookmarkActionGoing)
	s.bookmark(target.ID, engagementm.BookmarkEntityShow, future.ID, engagementm.BookmarkActionGoing)
	s.bookmark(target.ID, engagementm.BookmarkEntityShow, pastInterested.ID, engagementm.BookmarkActionInterested)

	resp, err := s.handler.GetUserAttendedShowsHandler(context.Background(), &GetUserAttendedShowsRequest{
		Username: "diaryuser", Limit: 20, Offset: 0,
	})
	s.Require().NoError(err)
	// Future "going" and past "interested" are both excluded from the diary.
	s.Equal(int64(2), resp.Body.Total)
	s.Require().Len(resp.Body.Shows, 2)
	// Most recent first.
	s.Equal("Recent Past Show", resp.Body.Shows[0].Title)
	s.Equal("Older Past Show", resp.Body.Shows[1].Title)
	s.Equal("going", resp.Body.Shows[0].Status)
	// Venue enrichment survives the join.
	s.Require().NotNil(resp.Body.Shows[0].VenueName)
	s.Equal("Rebel Lounge", *resp.Body.Shows[0].VenueName)
}

func (s *ContributorProfileHandlerIntegrationSuite) TestGetUserAttendedShows_DefaultHidden() {
	// No stored privacy settings: the default for `attendance` is hidden —
	// anonymous viewers must get a 404, while the owner still sees the diary.
	target := s.createUserWithUsername("diaryhidden")
	show := s.createShowAt("Past Hidden Show", time.Now().UTC().AddDate(0, -1, 0), nil)
	s.bookmark(target.ID, engagementm.BookmarkEntityShow, show.ID, engagementm.BookmarkActionGoing)

	_, err := s.handler.GetUserAttendedShowsHandler(context.Background(), &GetUserAttendedShowsRequest{
		Username: "diaryhidden", Limit: 20, Offset: 0,
	})
	testhelpers.AssertHumaError(s.T(), err, 404)

	resp, err := s.handler.GetUserAttendedShowsHandler(testhelpers.CtxWithUser(target), &GetUserAttendedShowsRequest{
		Username: "diaryhidden", Limit: 20, Offset: 0,
	})
	s.Require().NoError(err)
	s.Equal(int64(1), resp.Body.Total)
	s.Require().Len(resp.Body.Shows, 1)
}

func (s *ContributorProfileHandlerIntegrationSuite) TestGetUserAttendedShows_CountOnly() {
	target := s.createUserWithUsername("diarycount")
	settings := contracts.DefaultPrivacySettings()
	settings.Attendance = contracts.PrivacyCountOnly
	s.setPrivacySettings(target, settings)
	show := s.createShowAt("Past Counted Show", time.Now().UTC().AddDate(0, -1, 0), nil)
	s.bookmark(target.ID, engagementm.BookmarkEntityShow, show.ID, engagementm.BookmarkActionGoing)

	resp, err := s.handler.GetUserAttendedShowsHandler(context.Background(), &GetUserAttendedShowsRequest{
		Username: "diarycount", Limit: 20, Offset: 0,
	})
	s.Require().NoError(err)
	s.Equal(int64(1), resp.Body.Total)
	s.Empty(resp.Body.Shows)
}

// =============================================================================
// GetUserFieldNotesHandler
// =============================================================================

func (s *ContributorProfileHandlerIntegrationSuite) TestGetUserFieldNotes_List() {
	author := s.createUserWithUsername("noteauthor")
	now := time.Now().UTC()
	show1 := s.createShowAt("Wall of Sound Night", now.AddDate(0, -1, 0), nil)
	show2 := s.createShowAt("Reverent Room", now.AddDate(0, -2, 0), nil)

	s.createFieldNote(author.ID, show1.ID, "older note", engagementm.CommentVisibilityVisible, now.Add(-48*time.Hour))
	s.createFieldNote(author.ID, show2.ID, "newer note", engagementm.CommentVisibilityVisible, now.Add(-24*time.Hour))
	// Hidden notes and plain comments must never appear on the profile.
	s.createFieldNote(author.ID, show1.ID, "hidden note", engagementm.CommentVisibilityHiddenByMod, now.Add(-12*time.Hour))
	plain := &engagementm.Comment{
		EntityType: engagementm.CommentEntityShow,
		EntityID:   show1.ID,
		Kind:       engagementm.CommentKindComment,
		UserID:     author.ID,
		Body:       "plain comment",
		BodyHTML:   "<p>plain comment</p>",
		Visibility: engagementm.CommentVisibilityVisible,
	}
	s.deps.DB.Create(plain)

	resp, err := s.handler.GetUserFieldNotesHandler(context.Background(), &GetUserFieldNotesRequest{
		Username: "noteauthor", Limit: 20, Offset: 0,
	})
	s.Require().NoError(err)
	s.Equal(int64(2), resp.Body.Total)
	s.Require().Len(resp.Body.FieldNotes, 2)
	// Newest first, with show title/slug enrichment.
	s.Equal("newer note", resp.Body.FieldNotes[0].Body)
	s.Equal("Reverent Room", resp.Body.FieldNotes[0].ShowTitle)
	s.Equal(fmt.Sprintf("show-%d", show2.ID), resp.Body.FieldNotes[0].ShowSlug)
	s.Equal("older note", resp.Body.FieldNotes[1].Body)
	s.Equal("Wall of Sound Night", resp.Body.FieldNotes[1].ShowTitle)
	// Author attribution resolves (PSY-353 chain).
	s.Equal("noteauthor", resp.Body.FieldNotes[0].AuthorName)
}

func (s *ContributorProfileHandlerIntegrationSuite) TestGetUserFieldNotes_PrivateProfile() {
	target := s.createPrivateUser("noteprivate")

	_, err := s.handler.GetUserFieldNotesHandler(context.Background(), &GetUserFieldNotesRequest{
		Username: "noteprivate", Limit: 20, Offset: 0,
	})
	testhelpers.AssertHumaError(s.T(), err, 404)

	// Owner still passes the master gate.
	_, err = s.handler.GetUserFieldNotesHandler(testhelpers.CtxWithUser(target), &GetUserFieldNotesRequest{
		Username: "noteprivate", Limit: 20, Offset: 0,
	})
	s.Require().NoError(err)
}

func (s *ContributorProfileHandlerIntegrationSuite) TestGetUserFieldNotes_Empty() {
	s.createUserWithUsername("notenone")
	resp, err := s.handler.GetUserFieldNotesHandler(context.Background(), &GetUserFieldNotesRequest{
		Username: "notenone", Limit: 20, Offset: 0,
	})
	s.Require().NoError(err)
	s.Equal(int64(0), resp.Body.Total)
	s.Empty(resp.Body.FieldNotes)
}
