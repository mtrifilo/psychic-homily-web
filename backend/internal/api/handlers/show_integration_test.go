package handlers

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	"psychic-homily-backend/internal/models"
)

type ShowHandlerIntegrationSuite struct {
	suite.Suite
	deps    *handlerIntegrationDeps
	handler *ShowHandler
}

func (s *ShowHandlerIntegrationSuite) SetupSuite() {
	s.deps = setupHandlerIntegrationDeps(s.T())
	s.handler = NewShowHandler(
		s.deps.showService,
		s.deps.savedShowService,
		s.deps.discordService,
		s.deps.musicDiscoveryService,
		s.deps.extractionService,
	)
}

func (s *ShowHandlerIntegrationSuite) TearDownTest() {
	cleanupTables(s.deps.db)
}

func (s *ShowHandlerIntegrationSuite) TearDownSuite() {
	if s.deps.container != nil {
		s.deps.container.Terminate(s.deps.ctx)
	}
}

func TestShowHandlerIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	suite.Run(t, new(ShowHandlerIntegrationSuite))
}

// --- CreateShowHandler ---

func (s *ShowHandlerIntegrationSuite) TestCreateShow_Success() {
	user := createTestUser(s.deps.db)
	venue := createVerifiedVenue(s.deps.db, "Valley Bar", "Phoenix", "AZ")

	ctx := ctxWithUser(user)
	title := "New Show"
	req := &CreateShowRequest{}
	req.Body.Title = &title
	req.Body.EventDate = time.Now().UTC().AddDate(0, 0, 14)
	req.Body.City = "Phoenix"
	req.Body.State = "AZ"
	req.Body.Venues = []Venue{{ID: &venue.ID}}
	req.Body.Artists = []Artist{{Name: stringPtr("Test Artist")}}

	resp, err := s.handler.CreateShowHandler(ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.Equal("New Show", resp.Body.Title)
	// Shows with verified venues are auto-approved
	s.Equal("approved", resp.Body.Status)
}

func (s *ShowHandlerIntegrationSuite) TestCreateShow_AdminAutoApproved() {
	admin := createAdminUser(s.deps.db)
	venue := createVerifiedVenue(s.deps.db, "Valley Bar", "Phoenix", "AZ")

	ctx := ctxWithUser(admin)
	title := "Admin Show"
	req := &CreateShowRequest{}
	req.Body.Title = &title
	req.Body.EventDate = time.Now().UTC().AddDate(0, 0, 14)
	req.Body.City = "Phoenix"
	req.Body.State = "AZ"
	req.Body.Venues = []Venue{{ID: &venue.ID}}
	req.Body.Artists = []Artist{{Name: stringPtr("Test Artist")}}

	resp, err := s.handler.CreateShowHandler(ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.Equal("approved", resp.Body.Status)
}

func (s *ShowHandlerIntegrationSuite) TestCreateShow_UnverifiedEmailBlocked() {
	user := &models.User{
		Email:         stringPtr("unverified@test.com"),
		FirstName:     stringPtr("Test"),
		LastName:      stringPtr("User"),
		IsActive:      true,
		EmailVerified: false,
	}
	s.deps.db.Create(user)

	ctx := ctxWithUser(user)
	title := "Blocked Show"
	req := &CreateShowRequest{}
	req.Body.Title = &title
	req.Body.EventDate = time.Now().UTC().AddDate(0, 0, 14)
	req.Body.City = "Phoenix"
	req.Body.State = "AZ"
	req.Body.Venues = []Venue{{Name: stringPtr("Some Venue")}}
	req.Body.Artists = []Artist{{Name: stringPtr("Some Artist")}}

	_, err := s.handler.CreateShowHandler(ctx, req)
	s.Error(err)
}

// --- GetShowHandler ---

func (s *ShowHandlerIntegrationSuite) TestGetShow_ByID() {
	user := createTestUser(s.deps.db)
	show := createApprovedShow(s.deps.db, user.ID, "Test Show")

	req := &GetShowRequest{ShowID: fmt.Sprintf("%d", show.ID)}
	resp, err := s.handler.GetShowHandler(context.Background(), req)
	s.NoError(err)
	s.NotNil(resp)
	s.Equal(show.ID, resp.Body.ID)
}

func (s *ShowHandlerIntegrationSuite) TestGetShow_NotFound() {
	req := &GetShowRequest{ShowID: "99999"}
	_, err := s.handler.GetShowHandler(context.Background(), req)
	s.Error(err)
}

func (s *ShowHandlerIntegrationSuite) TestGetShow_PendingShowSubmitterCanView() {
	user := createTestUser(s.deps.db)
	show := createPendingShow(s.deps.db, user.ID, "Pending Show")

	ctx := ctxWithUser(user)
	req := &GetShowRequest{ShowID: fmt.Sprintf("%d", show.ID)}
	resp, err := s.handler.GetShowHandler(ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.Equal("pending", resp.Body.Status)
}

func (s *ShowHandlerIntegrationSuite) TestGetShow_PendingShowOtherUserDenied() {
	submitter := createTestUser(s.deps.db)
	other := createTestUser(s.deps.db)
	show := createPendingShow(s.deps.db, submitter.ID, "Pending Show")

	ctx := ctxWithUser(other)
	req := &GetShowRequest{ShowID: fmt.Sprintf("%d", show.ID)}
	_, err := s.handler.GetShowHandler(ctx, req)
	s.Error(err)
}

// --- GetShowsHandler ---

func (s *ShowHandlerIntegrationSuite) TestGetShows_Success() {
	user := createTestUser(s.deps.db)
	createApprovedShow(s.deps.db, user.ID, "Show 1")
	createApprovedShow(s.deps.db, user.ID, "Show 2")

	req := &GetShowsRequest{}
	resp, err := s.handler.GetShowsHandler(context.Background(), req)
	s.NoError(err)
	s.NotNil(resp)
	s.GreaterOrEqual(len(resp.Body), 2)
}

func (s *ShowHandlerIntegrationSuite) TestGetShows_Empty() {
	req := &GetShowsRequest{}
	resp, err := s.handler.GetShowsHandler(context.Background(), req)
	s.NoError(err)
	s.NotNil(resp)
	s.Empty(resp.Body)
}

// --- GetUpcomingShowsHandler ---

func (s *ShowHandlerIntegrationSuite) TestGetUpcomingShows_Success() {
	user := createTestUser(s.deps.db)
	createFutureApprovedShow(s.deps.db, user.ID, "Future Show", 7)

	req := &GetUpcomingShowsRequest{Timezone: "UTC", Limit: 50}
	resp, err := s.handler.GetUpcomingShowsHandler(context.Background(), req)
	s.NoError(err)
	s.NotNil(resp)
	s.GreaterOrEqual(len(resp.Body.Shows), 1)
}

func (s *ShowHandlerIntegrationSuite) TestGetUpcomingShows_ExcludesPast() {
	user := createTestUser(s.deps.db)
	createPastApprovedShow(s.deps.db, user.ID, "Past Show", 30)

	req := &GetUpcomingShowsRequest{Timezone: "UTC", Limit: 50}
	resp, err := s.handler.GetUpcomingShowsHandler(context.Background(), req)
	s.NoError(err)
	// Past shows should not appear
	for _, show := range resp.Body.Shows {
		s.NotEqual("Past Show", show.Title)
	}
}

func (s *ShowHandlerIntegrationSuite) TestGetUpcomingShows_Empty() {
	req := &GetUpcomingShowsRequest{Timezone: "UTC", Limit: 50}
	resp, err := s.handler.GetUpcomingShowsHandler(context.Background(), req)
	s.NoError(err)
	s.Empty(resp.Body.Shows)
}

// --- UpdateShowHandler ---

func (s *ShowHandlerIntegrationSuite) TestUpdateShow_OwnerSuccess() {
	user := createTestUser(s.deps.db)
	show := createApprovedShow(s.deps.db, user.ID, "Original Title")

	ctx := ctxWithUser(user)
	newTitle := "Updated Title"
	req := &UpdateShowRequest{ShowID: fmt.Sprintf("%d", show.ID)}
	req.Body.Title = &newTitle

	resp, err := s.handler.UpdateShowHandler(ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.Equal("Updated Title", resp.Body.Title)
}

func (s *ShowHandlerIntegrationSuite) TestUpdateShow_AdminSuccess() {
	user := createTestUser(s.deps.db)
	admin := createAdminUser(s.deps.db)
	show := createApprovedShow(s.deps.db, user.ID, "Original Title")

	ctx := ctxWithUser(admin)
	newTitle := "Admin Updated"
	req := &UpdateShowRequest{ShowID: fmt.Sprintf("%d", show.ID)}
	req.Body.Title = &newTitle

	resp, err := s.handler.UpdateShowHandler(ctx, req)
	s.NoError(err)
	s.Equal("Admin Updated", resp.Body.Title)
}

func (s *ShowHandlerIntegrationSuite) TestUpdateShow_NotOwnerForbidden() {
	submitter := createTestUser(s.deps.db)
	other := createTestUser(s.deps.db)
	show := createApprovedShow(s.deps.db, submitter.ID, "Test Show")

	ctx := ctxWithUser(other)
	newTitle := "Hacked Title"
	req := &UpdateShowRequest{ShowID: fmt.Sprintf("%d", show.ID)}
	req.Body.Title = &newTitle

	_, err := s.handler.UpdateShowHandler(ctx, req)
	s.Error(err)
}

func (s *ShowHandlerIntegrationSuite) TestUpdateShow_NotFound() {
	user := createTestUser(s.deps.db)
	ctx := ctxWithUser(user)

	newTitle := "Updated"
	req := &UpdateShowRequest{ShowID: "99999"}
	req.Body.Title = &newTitle

	_, err := s.handler.UpdateShowHandler(ctx, req)
	s.Error(err)
}

// --- DeleteShowHandler ---

func (s *ShowHandlerIntegrationSuite) TestDeleteShow_OwnerSuccess() {
	user := createTestUser(s.deps.db)
	show := createApprovedShow(s.deps.db, user.ID, "Delete Me")

	ctx := ctxWithUser(user)
	req := &DeleteShowRequest{ShowID: fmt.Sprintf("%d", show.ID)}

	_, err := s.handler.DeleteShowHandler(ctx, req)
	s.NoError(err)
}

func (s *ShowHandlerIntegrationSuite) TestDeleteShow_NotOwnerForbidden() {
	submitter := createTestUser(s.deps.db)
	other := createTestUser(s.deps.db)
	show := createApprovedShow(s.deps.db, submitter.ID, "Test Show")

	ctx := ctxWithUser(other)
	req := &DeleteShowRequest{ShowID: fmt.Sprintf("%d", show.ID)}

	_, err := s.handler.DeleteShowHandler(ctx, req)
	s.Error(err)
}

func (s *ShowHandlerIntegrationSuite) TestDeleteShow_NotFound() {
	user := createTestUser(s.deps.db)
	ctx := ctxWithUser(user)

	req := &DeleteShowRequest{ShowID: "99999"}
	_, err := s.handler.DeleteShowHandler(ctx, req)
	s.Error(err)
}

// --- CreateShowHandler with InstagramHandle ---

func (s *ShowHandlerIntegrationSuite) TestCreateShow_WithInstagramHandle() {
	user := createTestUser(s.deps.db)
	venue := createVerifiedVenue(s.deps.db, "IG Test Venue", "Phoenix", "AZ")

	ctx := ctxWithUser(user)
	title := "IG Show"
	igHandle := "@new_ig"
	req := &CreateShowRequest{}
	req.Body.Title = &title
	req.Body.EventDate = time.Now().UTC().AddDate(0, 0, 14)
	req.Body.City = "Phoenix"
	req.Body.State = "AZ"
	req.Body.Venues = []Venue{{ID: &venue.ID}}
	req.Body.Artists = []Artist{{Name: stringPtr("New IG Artist"), InstagramHandle: &igHandle}}

	resp, err := s.handler.CreateShowHandler(ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.Require().Len(resp.Body.Artists, 1)
	s.Require().NotNil(resp.Body.Artists[0].Socials.Instagram)
	s.Equal("@new_ig", *resp.Body.Artists[0].Socials.Instagram)

	// Verify in DB
	var artist models.Artist
	s.NoError(s.deps.db.Where("name = ?", "New IG Artist").First(&artist).Error)
	s.Require().NotNil(artist.Social.Instagram)
	s.Equal("@new_ig", *artist.Social.Instagram)
}

func (s *ShowHandlerIntegrationSuite) TestUpdateShow_WithInstagramOnNewArtist() {
	user := createTestUser(s.deps.db)
	show := createApprovedShow(s.deps.db, user.ID, "Update IG Show")

	ctx := ctxWithUser(user)
	igHandle := "@updated_ig"
	req := &UpdateShowRequest{ShowID: fmt.Sprintf("%d", show.ID)}
	req.Body.Artists = []Artist{
		{Name: stringPtr("Updated IG Artist"), InstagramHandle: &igHandle},
	}

	resp, err := s.handler.UpdateShowHandler(ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.Require().Len(resp.Body.Artists, 1)
	s.Require().NotNil(resp.Body.Artists[0].Socials.Instagram)
	s.Equal("@updated_ig", *resp.Body.Artists[0].Socials.Instagram)

	// Verify in DB
	var artist models.Artist
	s.NoError(s.deps.db.Where("name = ?", "Updated IG Artist").First(&artist).Error)
	s.Require().NotNil(artist.Social.Instagram)
	s.Equal("@updated_ig", *artist.Social.Instagram)
}

// --- GetMySubmissionsHandler ---

func (s *ShowHandlerIntegrationSuite) TestGetMySubmissions_Success() {
	user := createTestUser(s.deps.db)
	createApprovedShow(s.deps.db, user.ID, "My Show 1")
	createPendingShow(s.deps.db, user.ID, "My Show 2")

	ctx := ctxWithUser(user)
	req := &GetMySubmissionsRequest{Limit: 50, Offset: 0}
	resp, err := s.handler.GetMySubmissionsHandler(ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.Equal(2, resp.Body.Total)
	s.Len(resp.Body.Shows, 2)
}

func (s *ShowHandlerIntegrationSuite) TestGetMySubmissions_Empty() {
	user := createTestUser(s.deps.db)
	ctx := ctxWithUser(user)

	req := &GetMySubmissionsRequest{Limit: 50, Offset: 0}
	resp, err := s.handler.GetMySubmissionsHandler(ctx, req)
	s.NoError(err)
	s.Equal(0, resp.Body.Total)
}

// --- GetShowCitiesHandler ---

func (s *ShowHandlerIntegrationSuite) TestGetShowCities_Success() {
	user := createTestUser(s.deps.db)
	createFutureApprovedShow(s.deps.db, user.ID, "Phoenix Show", 7)

	req := &GetShowCitiesRequest{Timezone: "UTC"}
	resp, err := s.handler.GetShowCitiesHandler(context.Background(), req)
	s.NoError(err)
	s.NotNil(resp)
}

// --- UnpublishShowHandler ---

func (s *ShowHandlerIntegrationSuite) TestUnpublishShow_OwnerSuccess() {
	user := createTestUser(s.deps.db)
	show := createApprovedShow(s.deps.db, user.ID, "Approved Show")

	ctx := ctxWithUser(user)
	req := &UnpublishShowRequest{ShowID: fmt.Sprintf("%d", show.ID)}
	resp, err := s.handler.UnpublishShowHandler(ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.Equal("private", resp.Body.Status)
}

func (s *ShowHandlerIntegrationSuite) TestUnpublishShow_NotFound() {
	user := createTestUser(s.deps.db)
	ctx := ctxWithUser(user)

	req := &UnpublishShowRequest{ShowID: "99999"}
	_, err := s.handler.UnpublishShowHandler(ctx, req)
	s.Error(err)
}

// --- SetShowSoldOutHandler ---

func (s *ShowHandlerIntegrationSuite) TestSetShowSoldOut_OwnerSuccess() {
	user := createTestUser(s.deps.db)
	show := createApprovedShow(s.deps.db, user.ID, "Test Show")

	ctx := ctxWithUser(user)
	req := &SetShowSoldOutRequest{ShowID: fmt.Sprintf("%d", show.ID)}
	req.Body.Value = true

	resp, err := s.handler.SetShowSoldOutHandler(ctx, req)
	s.NoError(err)
	s.NotNil(resp)
}

func (s *ShowHandlerIntegrationSuite) TestSetShowSoldOut_NonOwnerForbidden() {
	submitter := createTestUser(s.deps.db)
	other := createTestUser(s.deps.db)
	show := createApprovedShow(s.deps.db, submitter.ID, "Test Show")

	ctx := ctxWithUser(other)
	req := &SetShowSoldOutRequest{ShowID: fmt.Sprintf("%d", show.ID)}
	req.Body.Value = true

	_, err := s.handler.SetShowSoldOutHandler(ctx, req)
	s.Error(err)
}

// --- SetShowCancelledHandler ---

func (s *ShowHandlerIntegrationSuite) TestSetShowCancelled_OwnerSuccess() {
	user := createTestUser(s.deps.db)
	show := createApprovedShow(s.deps.db, user.ID, "Test Show")

	ctx := ctxWithUser(user)
	req := &SetShowCancelledRequest{ShowID: fmt.Sprintf("%d", show.ID)}
	req.Body.Value = true

	resp, err := s.handler.SetShowCancelledHandler(ctx, req)
	s.NoError(err)
	s.NotNil(resp)
}
