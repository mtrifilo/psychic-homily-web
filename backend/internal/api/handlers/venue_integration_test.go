package handlers

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/suite"

	"psychic-homily-backend/internal/models"
)

type VenueHandlerIntegrationSuite struct {
	suite.Suite
	deps    *handlerIntegrationDeps
	handler *VenueHandler
}

func (s *VenueHandlerIntegrationSuite) SetupSuite() {
	s.deps = setupHandlerIntegrationDeps(s.T())
	s.handler = NewVenueHandler(s.deps.venueService, s.deps.discordService)
}

func (s *VenueHandlerIntegrationSuite) TearDownTest() {
	cleanupTables(s.deps.db)
}

func (s *VenueHandlerIntegrationSuite) TearDownSuite() {
	if s.deps.container != nil {
		s.deps.container.Terminate(s.deps.ctx)
	}
}

func TestVenueHandlerIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	suite.Run(t, new(VenueHandlerIntegrationSuite))
}

// --- SearchVenuesHandler ---

func (s *VenueHandlerIntegrationSuite) TestSearchVenues_Success() {
	createVerifiedVenue(s.deps.db, "Valley Bar", "Phoenix", "AZ")
	createVerifiedVenue(s.deps.db, "Crescent Ballroom", "Phoenix", "AZ")

	req := &SearchVenuesRequest{Query: "Valley"}
	resp, err := s.handler.SearchVenuesHandler(s.deps.ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.GreaterOrEqual(resp.Body.Count, 1)
}

func (s *VenueHandlerIntegrationSuite) TestSearchVenues_NoResults() {
	req := &SearchVenuesRequest{Query: "Nonexistent Venue XYZ"}
	resp, err := s.handler.SearchVenuesHandler(s.deps.ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.Equal(0, resp.Body.Count)
}

// --- ListVenuesHandler ---

func (s *VenueHandlerIntegrationSuite) TestListVenues_Success() {
	createVerifiedVenue(s.deps.db, "Valley Bar", "Phoenix", "AZ")
	createVerifiedVenue(s.deps.db, "Crescent Ballroom", "Phoenix", "AZ")
	createVerifiedVenue(s.deps.db, "The Rebel Lounge", "Phoenix", "AZ")

	req := &ListVenuesRequest{Limit: 50, Offset: 0}
	resp, err := s.handler.ListVenuesHandler(s.deps.ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.GreaterOrEqual(resp.Body.Total, int64(3))
}

func (s *VenueHandlerIntegrationSuite) TestListVenues_Empty() {
	req := &ListVenuesRequest{Limit: 50, Offset: 0}
	resp, err := s.handler.ListVenuesHandler(s.deps.ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.Equal(int64(0), resp.Body.Total)
}

func (s *VenueHandlerIntegrationSuite) TestListVenues_CityFilter() {
	createVerifiedVenue(s.deps.db, "Valley Bar", "Phoenix", "AZ")
	createVerifiedVenue(s.deps.db, "Club Congress", "Tucson", "AZ")

	req := &ListVenuesRequest{City: "Phoenix", Limit: 50, Offset: 0}
	resp, err := s.handler.ListVenuesHandler(s.deps.ctx, req)
	s.NoError(err)
	s.Equal(int64(1), resp.Body.Total)
}

// --- GetVenueHandler ---

func (s *VenueHandlerIntegrationSuite) TestGetVenue_ByID() {
	venue := createVerifiedVenue(s.deps.db, "Valley Bar", "Phoenix", "AZ")

	req := &GetVenueRequest{VenueID: fmt.Sprintf("%d", venue.ID)}
	resp, err := s.handler.GetVenueHandler(s.deps.ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.Equal(venue.ID, resp.Body.ID)
	s.Equal("Valley Bar", resp.Body.Name)
}

func (s *VenueHandlerIntegrationSuite) TestGetVenue_NotFound() {
	req := &GetVenueRequest{VenueID: "99999"}
	_, err := s.handler.GetVenueHandler(s.deps.ctx, req)
	s.Error(err)
}

// --- GetVenueShowsHandler ---

func (s *VenueHandlerIntegrationSuite) TestGetVenueShows_Success() {
	user := createTestUser(s.deps.db)
	venue := createVerifiedVenue(s.deps.db, "Valley Bar", "Phoenix", "AZ")

	// Create a future show at this venue
	show := createFutureApprovedShow(s.deps.db, user.ID, "Test Show", 7)
	s.deps.db.Exec("INSERT INTO show_venues (show_id, venue_id) VALUES (?, ?)", show.ID, venue.ID)

	req := &GetVenueShowsRequest{
		VenueID:    fmt.Sprintf("%d", venue.ID),
		Timezone:   "UTC",
		Limit:      20,
		TimeFilter: "upcoming",
	}
	resp, err := s.handler.GetVenueShowsHandler(s.deps.ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.Equal(venue.ID, resp.Body.VenueID)
}

// --- GetVenueCitiesHandler ---

func (s *VenueHandlerIntegrationSuite) TestGetVenueCities_Success() {
	createVerifiedVenue(s.deps.db, "Valley Bar", "Phoenix", "AZ")
	createVerifiedVenue(s.deps.db, "Club Congress", "Tucson", "AZ")

	req := &GetVenueCitiesRequest{}
	resp, err := s.handler.GetVenueCitiesHandler(s.deps.ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.GreaterOrEqual(len(resp.Body.Cities), 2)
}

// --- UpdateVenueHandler ---

func (s *VenueHandlerIntegrationSuite) TestUpdateVenue_AdminDirectUpdate() {
	admin := createAdminUser(s.deps.db)
	venue := createVerifiedVenue(s.deps.db, "Valley Bar", "Phoenix", "AZ")

	ctx := ctxWithUser(admin)
	newName := "Valley Bar Updated"
	req := &UpdateVenueRequest{VenueID: fmt.Sprintf("%d", venue.ID)}
	req.Body.Name = &newName

	resp, err := s.handler.UpdateVenueHandler(ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.Equal("updated", resp.Body.Status)
	s.NotNil(resp.Body.Venue)
	s.Equal("Valley Bar Updated", resp.Body.Venue.Name)
}

func (s *VenueHandlerIntegrationSuite) TestUpdateVenue_NonAdminCreatesPendingEdit() {
	user := createTestUser(s.deps.db)

	// Create venue submitted by user
	venue := &models.Venue{
		Name:        "My Venue",
		City:        "Phoenix",
		State:       "AZ",
		Verified:    true,
		SubmittedBy: &user.ID,
	}
	s.deps.db.Create(venue)

	ctx := ctxWithUser(user)
	newName := "My Venue Updated"
	req := &UpdateVenueRequest{VenueID: fmt.Sprintf("%d", venue.ID)}
	req.Body.Name = &newName

	resp, err := s.handler.UpdateVenueHandler(ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.Equal("pending", resp.Body.Status)
	s.NotNil(resp.Body.PendingEdit)
}

func (s *VenueHandlerIntegrationSuite) TestUpdateVenue_NonOwnerForbidden() {
	user := createTestUser(s.deps.db)
	venue := createVerifiedVenue(s.deps.db, "Valley Bar", "Phoenix", "AZ") // no SubmittedBy

	ctx := ctxWithUser(user)
	newName := "Changed Name"
	req := &UpdateVenueRequest{VenueID: fmt.Sprintf("%d", venue.ID)}
	req.Body.Name = &newName

	_, err := s.handler.UpdateVenueHandler(ctx, req)
	s.Error(err)
}

func (s *VenueHandlerIntegrationSuite) TestUpdateVenue_VenueNotFound() {
	admin := createAdminUser(s.deps.db)
	ctx := ctxWithUser(admin)

	newName := "Updated"
	req := &UpdateVenueRequest{VenueID: "99999"}
	req.Body.Name = &newName

	_, err := s.handler.UpdateVenueHandler(ctx, req)
	s.Error(err)
}

// --- DeleteVenueHandler ---

func (s *VenueHandlerIntegrationSuite) TestDeleteVenue_AdminSuccess() {
	admin := createAdminUser(s.deps.db)
	venue := createVerifiedVenue(s.deps.db, "Delete Me", "Phoenix", "AZ")

	ctx := ctxWithUser(admin)
	req := &DeleteVenueRequest{VenueID: fmt.Sprintf("%d", venue.ID)}

	resp, err := s.handler.DeleteVenueHandler(ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.Contains(resp.Body.Message, "deleted")
}

func (s *VenueHandlerIntegrationSuite) TestDeleteVenue_OwnerSuccess() {
	user := createTestUser(s.deps.db)
	venue := &models.Venue{
		Name:        "My Venue",
		City:        "Phoenix",
		State:       "AZ",
		SubmittedBy: &user.ID,
	}
	s.deps.db.Create(venue)

	ctx := ctxWithUser(user)
	req := &DeleteVenueRequest{VenueID: fmt.Sprintf("%d", venue.ID)}

	resp, err := s.handler.DeleteVenueHandler(ctx, req)
	s.NoError(err)
	s.NotNil(resp)
}

func (s *VenueHandlerIntegrationSuite) TestDeleteVenue_NonOwnerForbidden() {
	user := createTestUser(s.deps.db)
	venue := createVerifiedVenue(s.deps.db, "Valley Bar", "Phoenix", "AZ")

	ctx := ctxWithUser(user)
	req := &DeleteVenueRequest{VenueID: fmt.Sprintf("%d", venue.ID)}

	_, err := s.handler.DeleteVenueHandler(ctx, req)
	s.Error(err)
}

func (s *VenueHandlerIntegrationSuite) TestDeleteVenue_NotFound() {
	admin := createAdminUser(s.deps.db)
	ctx := ctxWithUser(admin)

	req := &DeleteVenueRequest{VenueID: "99999"}
	_, err := s.handler.DeleteVenueHandler(ctx, req)
	s.Error(err)
}

// --- GetMyPendingEditHandler ---

func (s *VenueHandlerIntegrationSuite) TestGetMyPendingEdit_None() {
	user := createTestUser(s.deps.db)
	venue := &models.Venue{
		Name:        "My Venue",
		City:        "Phoenix",
		State:       "AZ",
		SubmittedBy: &user.ID,
	}
	s.deps.db.Create(venue)

	ctx := ctxWithUser(user)
	req := &GetMyPendingEditRequest{VenueID: fmt.Sprintf("%d", venue.ID)}
	resp, err := s.handler.GetMyPendingEditHandler(ctx, req)
	s.NoError(err)
	s.Nil(resp.Body.PendingEdit)
}

func (s *VenueHandlerIntegrationSuite) TestGetMyPendingEdit_Exists() {
	user := createTestUser(s.deps.db)
	venue := &models.Venue{
		Name:        "My Venue",
		City:        "Phoenix",
		State:       "AZ",
		Verified:    true,
		SubmittedBy: &user.ID,
	}
	s.deps.db.Create(venue)

	// Create pending edit
	ctx := ctxWithUser(user)
	newName := "Updated Name"
	updateReq := &UpdateVenueRequest{VenueID: fmt.Sprintf("%d", venue.ID)}
	updateReq.Body.Name = &newName
	_, err := s.handler.UpdateVenueHandler(ctx, updateReq)
	s.NoError(err)

	// Check pending edit exists
	getReq := &GetMyPendingEditRequest{VenueID: fmt.Sprintf("%d", venue.ID)}
	resp, err := s.handler.GetMyPendingEditHandler(ctx, getReq)
	s.NoError(err)
	s.NotNil(resp.Body.PendingEdit)
}
