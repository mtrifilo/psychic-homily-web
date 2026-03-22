package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/suite"

	"psychic-homily-backend/internal/models"
	"psychic-homily-backend/internal/services/contracts"
	usersvc "psychic-homily-backend/internal/services/user"
)

type ContributorProfileHandlerIntegrationSuite struct {
	suite.Suite
	deps           *handlerIntegrationDeps
	handler        *ContributorProfileHandler
	profileService *usersvc.ContributorProfileService
}

func (s *ContributorProfileHandlerIntegrationSuite) SetupSuite() {
	s.deps = setupHandlerIntegrationDeps(s.T())
	s.profileService = usersvc.NewContributorProfileService(s.deps.db)
	s.handler = NewContributorProfileHandler(s.profileService, s.deps.userService)
}

func (s *ContributorProfileHandlerIntegrationSuite) TearDownTest() {
	sqlDB, _ := s.deps.db.DB()
	_, _ = sqlDB.Exec("DELETE FROM user_profile_sections")
	cleanupTables(s.deps.db)
}

func (s *ContributorProfileHandlerIntegrationSuite) TearDownSuite() {
	s.deps.testDB.Cleanup()
}

func TestContributorProfileHandlerIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	suite.Run(t, new(ContributorProfileHandlerIntegrationSuite))
}

// --- Helpers ---

func (s *ContributorProfileHandlerIntegrationSuite) createUserWithUsername(username string) *models.User {
	user := &models.User{
		Email:         stringPtr(fmt.Sprintf("%s@test.com", username)),
		Username:      stringPtr(username),
		FirstName:     stringPtr("Test"),
		LastName:      stringPtr("User"),
		IsActive:      true,
		EmailVerified: true,
	}
	s.deps.db.Create(user)
	return user
}

func (s *ContributorProfileHandlerIntegrationSuite) createPrivateUser(username string) *models.User {
	user := s.createUserWithUsername(username)
	s.deps.db.Model(user).Update("profile_visibility", "private")
	user.ProfileVisibility = "private"
	return user
}

func (s *ContributorProfileHandlerIntegrationSuite) setPrivacySettings(user *models.User, settings contracts.PrivacySettings) {
	raw, err := json.Marshal(settings)
	s.Require().NoError(err)
	rawMsg := json.RawMessage(raw)
	s.deps.db.Model(user).Update("privacy_settings", &rawMsg)
}

// =============================================================================
// GetPublicProfileHandler
// =============================================================================

func (s *ContributorProfileHandlerIntegrationSuite) TestGetPublicProfile_Success() {
	_ = s.createUserWithUsername("publicuser")

	req := &GetPublicProfileRequest{Username: "publicuser"}
	resp, err := s.handler.GetPublicProfileHandler(s.deps.ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.Equal("publicuser", resp.Body.Username)
	s.Equal("public", resp.Body.ProfileVisibility)
}

func (s *ContributorProfileHandlerIntegrationSuite) TestGetPublicProfile_NotFound() {
	req := &GetPublicProfileRequest{Username: "nonexistent"}
	_, err := s.handler.GetPublicProfileHandler(s.deps.ctx, req)
	assertHumaError(s.T(), err, 404)
}

func (s *ContributorProfileHandlerIntegrationSuite) TestGetPublicProfile_PrivateProfile_NotOwner() {
	_ = s.createPrivateUser("privateuser")

	req := &GetPublicProfileRequest{Username: "privateuser"}
	// View as anonymous — should get 404
	_, err := s.handler.GetPublicProfileHandler(s.deps.ctx, req)
	assertHumaError(s.T(), err, 404)
}

func (s *ContributorProfileHandlerIntegrationSuite) TestGetPublicProfile_PrivateProfile_AsOwner() {
	user := s.createPrivateUser("privateowner")
	ctx := ctxWithUser(user)

	req := &GetPublicProfileRequest{Username: "privateowner"}
	resp, err := s.handler.GetPublicProfileHandler(ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.Equal("privateowner", resp.Body.Username)
}

func (s *ContributorProfileHandlerIntegrationSuite) TestGetPublicProfile_OwnerSeesPrivacySettings() {
	user := s.createUserWithUsername("ownerview")
	ctx := ctxWithUser(user)

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
	ctx := ctxWithUser(viewer)

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
	ctx := ctxWithUser(user)

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
	assertHumaError(s.T(), err, 401)
}

// =============================================================================
// UpdateProfileVisibilityHandler
// =============================================================================

func (s *ContributorProfileHandlerIntegrationSuite) TestUpdateVisibility_SetPrivate() {
	user := s.createUserWithUsername("visuser")
	ctx := ctxWithUser(user)

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
	ctx := ctxWithUser(user)

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
	ctx := ctxWithUser(user)

	req := &UpdateProfileVisibilityRequest{}
	req.Body.Visibility = "invalid"

	_, err := s.handler.UpdateProfileVisibilityHandler(ctx, req)
	assertHumaError(s.T(), err, 400)
}

func (s *ContributorProfileHandlerIntegrationSuite) TestUpdateVisibility_Unauthenticated() {
	req := &UpdateProfileVisibilityRequest{}
	req.Body.Visibility = "private"

	_, err := s.handler.UpdateProfileVisibilityHandler(context.Background(), req)
	assertHumaError(s.T(), err, 401)
}

// =============================================================================
// UpdatePrivacySettingsHandler
// =============================================================================

func (s *ContributorProfileHandlerIntegrationSuite) TestUpdatePrivacySettings_Success() {
	user := s.createUserWithUsername("privacyuser")
	ctx := ctxWithUser(user)

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
	ctx := ctxWithUser(user)

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
	assertHumaError(s.T(), err, 400)
}

func (s *ContributorProfileHandlerIntegrationSuite) TestUpdatePrivacySettings_BinaryFieldCountOnly() {
	user := s.createUserWithUsername("privacyuser3")
	ctx := ctxWithUser(user)

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
	assertHumaError(s.T(), err, 400)
}

func (s *ContributorProfileHandlerIntegrationSuite) TestUpdatePrivacySettings_Unauthenticated() {
	req := &UpdatePrivacySettingsRequest{}
	req.Body = contracts.DefaultPrivacySettings()

	_, err := s.handler.UpdatePrivacySettingsHandler(context.Background(), req)
	assertHumaError(s.T(), err, 401)
}

// =============================================================================
// GetContributionHistoryHandler
// =============================================================================

func (s *ContributorProfileHandlerIntegrationSuite) TestGetContributionHistory_Success() {
	user := s.createUserWithUsername("contribuser")
	// Create a show submitted by this user to produce a contribution entry
	createApprovedShow(s.deps.db, user.ID, "Test Show")

	req := &GetContributionHistoryRequest{Username: "contribuser", Limit: 20, Offset: 0}
	resp, err := s.handler.GetContributionHistoryHandler(s.deps.ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.GreaterOrEqual(resp.Body.Total, int64(1))
}

func (s *ContributorProfileHandlerIntegrationSuite) TestGetContributionHistory_UserNotFound() {
	req := &GetContributionHistoryRequest{Username: "ghostuser", Limit: 20, Offset: 0}
	_, err := s.handler.GetContributionHistoryHandler(s.deps.ctx, req)
	assertHumaError(s.T(), err, 404)
}

func (s *ContributorProfileHandlerIntegrationSuite) TestGetContributionHistory_PrivateProfile_NotOwner() {
	s.createPrivateUser("privatecontrib")

	req := &GetContributionHistoryRequest{Username: "privatecontrib", Limit: 20, Offset: 0}
	_, err := s.handler.GetContributionHistoryHandler(s.deps.ctx, req)
	assertHumaError(s.T(), err, 404)
}

func (s *ContributorProfileHandlerIntegrationSuite) TestGetContributionHistory_PrivateProfile_AsOwner() {
	user := s.createPrivateUser("privatecontribowner")
	ctx := ctxWithUser(user)

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
	ctx := ctxWithUser(viewer)

	req := &GetContributionHistoryRequest{Username: "hiddencontrib", Limit: 20, Offset: 0}
	_, err := s.handler.GetContributionHistoryHandler(ctx, req)
	assertHumaError(s.T(), err, 404)
}

func (s *ContributorProfileHandlerIntegrationSuite) TestGetContributionHistory_PrivacyCountOnly() {
	user := s.createUserWithUsername("countcontrib")
	createApprovedShow(s.deps.db, user.ID, "Count Only Show")

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
	ctx := ctxWithUser(viewer)

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
	createApprovedShow(s.deps.db, user.ID, "Own Show")
	ctx := ctxWithUser(user)

	req := &GetOwnContributionsRequest{Limit: 20, Offset: 0}
	resp, err := s.handler.GetOwnContributionsHandler(ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.GreaterOrEqual(resp.Body.Total, int64(1))
}

func (s *ContributorProfileHandlerIntegrationSuite) TestGetOwnContributions_Unauthenticated() {
	req := &GetOwnContributionsRequest{Limit: 20, Offset: 0}
	_, err := s.handler.GetOwnContributionsHandler(context.Background(), req)
	assertHumaError(s.T(), err, 401)
}

// =============================================================================
// Section CRUD: CreateSectionHandler
// =============================================================================

func (s *ContributorProfileHandlerIntegrationSuite) TestCreateSection_Success() {
	user := s.createUserWithUsername("sectionuser")
	ctx := ctxWithUser(user)

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
	assertHumaError(s.T(), err, 401)
}

func (s *ContributorProfileHandlerIntegrationSuite) TestCreateSection_EmptyTitle() {
	user := s.createUserWithUsername("emptytitle")
	ctx := ctxWithUser(user)

	req := &CreateSectionRequest{}
	req.Body.Title = ""
	req.Body.Content = "Content"
	req.Body.Position = 0

	_, err := s.handler.CreateSectionHandler(ctx, req)
	assertHumaError(s.T(), err, 400)
}

func (s *ContributorProfileHandlerIntegrationSuite) TestCreateSection_InvalidPosition() {
	user := s.createUserWithUsername("badposition")
	ctx := ctxWithUser(user)

	req := &CreateSectionRequest{}
	req.Body.Title = "Valid Title"
	req.Body.Content = "Content"
	req.Body.Position = 5 // max is 2

	_, err := s.handler.CreateSectionHandler(ctx, req)
	assertHumaError(s.T(), err, 400)
}

func (s *ContributorProfileHandlerIntegrationSuite) TestCreateSection_MaxSectionsExceeded() {
	user := s.createUserWithUsername("maxsections")
	ctx := ctxWithUser(user)

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
	assertHumaError(s.T(), err, 400)
}

// =============================================================================
// Section CRUD: UpdateSectionHandler
// =============================================================================

func (s *ContributorProfileHandlerIntegrationSuite) TestUpdateSection_Success() {
	user := s.createUserWithUsername("updateuser")
	ctx := ctxWithUser(user)

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
	ctx := ctxWithUser(user)

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
	ctx := ctxWithUser(user)

	newTitle := "Ghost Update"
	updateReq := &UpdateSectionRequest{SectionID: "99999"}
	updateReq.Body.Title = &newTitle

	_, err := s.handler.UpdateSectionHandler(ctx, updateReq)
	assertHumaError(s.T(), err, 404)
}

func (s *ContributorProfileHandlerIntegrationSuite) TestUpdateSection_OtherUsersSection() {
	owner := s.createUserWithUsername("sectionowner")
	other := s.createUserWithUsername("sectionthief")

	ownerCtx := ctxWithUser(owner)
	createReq := &CreateSectionRequest{}
	createReq.Body.Title = "Owner Section"
	createReq.Body.Content = "Confidential"
	createReq.Body.Position = 0
	createResp, err := s.handler.CreateSectionHandler(ownerCtx, createReq)
	s.Require().NoError(err)

	// Try to update as the other user
	otherCtx := ctxWithUser(other)
	newTitle := "Hacked"
	updateReq := &UpdateSectionRequest{SectionID: fmt.Sprintf("%d", createResp.Body.ID)}
	updateReq.Body.Title = &newTitle

	_, err = s.handler.UpdateSectionHandler(otherCtx, updateReq)
	assertHumaError(s.T(), err, 404)
}

func (s *ContributorProfileHandlerIntegrationSuite) TestUpdateSection_InvalidSectionID() {
	user := s.createUserWithUsername("badsectionid")
	ctx := ctxWithUser(user)

	newTitle := "Broken"
	updateReq := &UpdateSectionRequest{SectionID: "not-a-number"}
	updateReq.Body.Title = &newTitle

	_, err := s.handler.UpdateSectionHandler(ctx, updateReq)
	assertHumaError(s.T(), err, 400)
}

func (s *ContributorProfileHandlerIntegrationSuite) TestUpdateSection_NoFields() {
	user := s.createUserWithUsername("nofielduser")
	ctx := ctxWithUser(user)

	createReq := &CreateSectionRequest{}
	createReq.Body.Title = "No Field Section"
	createReq.Body.Content = "Content"
	createReq.Body.Position = 0
	createResp, err := s.handler.CreateSectionHandler(ctx, createReq)
	s.Require().NoError(err)

	// Send update with no fields
	updateReq := &UpdateSectionRequest{SectionID: fmt.Sprintf("%d", createResp.Body.ID)}
	_, err = s.handler.UpdateSectionHandler(ctx, updateReq)
	assertHumaError(s.T(), err, 400)
}

func (s *ContributorProfileHandlerIntegrationSuite) TestUpdateSection_Unauthenticated() {
	newTitle := "No Auth"
	updateReq := &UpdateSectionRequest{SectionID: "1"}
	updateReq.Body.Title = &newTitle

	_, err := s.handler.UpdateSectionHandler(context.Background(), updateReq)
	assertHumaError(s.T(), err, 401)
}

// =============================================================================
// Section CRUD: DeleteSectionHandler
// =============================================================================

func (s *ContributorProfileHandlerIntegrationSuite) TestDeleteSection_Success() {
	user := s.createUserWithUsername("deleteuser")
	ctx := ctxWithUser(user)

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
	ctx := ctxWithUser(user)

	deleteReq := &DeleteSectionRequest{SectionID: "99999"}
	_, err := s.handler.DeleteSectionHandler(ctx, deleteReq)
	assertHumaError(s.T(), err, 404)
}

func (s *ContributorProfileHandlerIntegrationSuite) TestDeleteSection_OtherUsersSection() {
	owner := s.createUserWithUsername("delowner")
	other := s.createUserWithUsername("delthief")

	ownerCtx := ctxWithUser(owner)
	createReq := &CreateSectionRequest{}
	createReq.Body.Title = "Protected Section"
	createReq.Body.Content = "Content"
	createReq.Body.Position = 0
	createResp, err := s.handler.CreateSectionHandler(ownerCtx, createReq)
	s.Require().NoError(err)

	otherCtx := ctxWithUser(other)
	deleteReq := &DeleteSectionRequest{SectionID: fmt.Sprintf("%d", createResp.Body.ID)}
	_, err = s.handler.DeleteSectionHandler(otherCtx, deleteReq)
	assertHumaError(s.T(), err, 404)
}

func (s *ContributorProfileHandlerIntegrationSuite) TestDeleteSection_InvalidSectionID() {
	user := s.createUserWithUsername("delbadid")
	ctx := ctxWithUser(user)

	deleteReq := &DeleteSectionRequest{SectionID: "abc"}
	_, err := s.handler.DeleteSectionHandler(ctx, deleteReq)
	assertHumaError(s.T(), err, 400)
}

func (s *ContributorProfileHandlerIntegrationSuite) TestDeleteSection_Unauthenticated() {
	deleteReq := &DeleteSectionRequest{SectionID: "1"}
	_, err := s.handler.DeleteSectionHandler(context.Background(), deleteReq)
	assertHumaError(s.T(), err, 401)
}

// =============================================================================
// GetOwnSectionsHandler
// =============================================================================

func (s *ContributorProfileHandlerIntegrationSuite) TestGetOwnSections_Success() {
	user := s.createUserWithUsername("ownsections")
	ctx := ctxWithUser(user)

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
	ctx := ctxWithUser(user)

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
	ctx := ctxWithUser(user)

	resp, err := s.handler.GetOwnSectionsHandler(ctx, &struct{}{})
	s.NoError(err)
	s.NotNil(resp)
	s.Empty(resp.Body.Sections)
}

func (s *ContributorProfileHandlerIntegrationSuite) TestGetOwnSections_Unauthenticated() {
	_, err := s.handler.GetOwnSectionsHandler(context.Background(), &struct{}{})
	assertHumaError(s.T(), err, 401)
}

// =============================================================================
// GetUserSectionsHandler (public)
// =============================================================================

func (s *ContributorProfileHandlerIntegrationSuite) TestGetUserSections_Success() {
	user := s.createUserWithUsername("pubsections")
	ctx := ctxWithUser(user)

	req := &CreateSectionRequest{}
	req.Body.Title = "Public Section"
	req.Body.Content = "Content"
	req.Body.Position = 0
	_, err := s.handler.CreateSectionHandler(ctx, req)
	s.Require().NoError(err)

	// View as anonymous
	sectReq := &GetUserSectionsRequest{Username: "pubsections"}
	resp, err := s.handler.GetUserSectionsHandler(s.deps.ctx, sectReq)
	s.NoError(err)
	s.NotNil(resp)
	s.Len(resp.Body.Sections, 1)
	s.Equal("Public Section", resp.Body.Sections[0].Title)
}

func (s *ContributorProfileHandlerIntegrationSuite) TestGetUserSections_HiddenSectionNotVisible() {
	user := s.createUserWithUsername("hiddensecpub")
	ctx := ctxWithUser(user)

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
	viewerCtx := ctxWithUser(viewer)

	sectReq := &GetUserSectionsRequest{Username: "hiddensecpub"}
	resp, err := s.handler.GetUserSectionsHandler(viewerCtx, sectReq)
	s.NoError(err)
	s.Empty(resp.Body.Sections)
}

func (s *ContributorProfileHandlerIntegrationSuite) TestGetUserSections_OwnerSeesHiddenSections() {
	user := s.createUserWithUsername("ownerseeshidden")
	ctx := ctxWithUser(user)

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
	_, err := s.handler.GetUserSectionsHandler(s.deps.ctx, sectReq)
	assertHumaError(s.T(), err, 404)
}

func (s *ContributorProfileHandlerIntegrationSuite) TestGetUserSections_PrivateProfile() {
	s.createPrivateUser("privatesecuser")

	sectReq := &GetUserSectionsRequest{Username: "privatesecuser"}
	_, err := s.handler.GetUserSectionsHandler(s.deps.ctx, sectReq)
	assertHumaError(s.T(), err, 404)
}

func (s *ContributorProfileHandlerIntegrationSuite) TestGetUserSections_PrivacySectionsHidden() {
	user := s.createUserWithUsername("sechiddenpriv")
	ctx := ctxWithUser(user)

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
	viewerCtx := ctxWithUser(viewer)

	sectReq := &GetUserSectionsRequest{Username: "sechiddenpriv"}
	resp, err := s.handler.GetUserSectionsHandler(viewerCtx, sectReq)
	s.NoError(err)
	s.NotNil(resp)
	s.Empty(resp.Body.Sections, "sections should be empty when privacy is hidden")
}
