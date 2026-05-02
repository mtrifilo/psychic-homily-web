package catalog

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/suite"

	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	catalogm "psychic-homily-backend/internal/models/catalog"
)

type VenueHandlerIntegrationSuite struct {
	suite.Suite
	deps    *testhelpers.IntegrationDeps
	handler *VenueHandler
}

func (s *VenueHandlerIntegrationSuite) SetupSuite() {
	s.deps = testhelpers.SetupIntegrationDeps(s.T())
	s.handler = NewVenueHandler(s.deps.VenueService, s.deps.DiscordService, s.deps.AuditLogService, nil)
}

func (s *VenueHandlerIntegrationSuite) TearDownTest() {
	testhelpers.CleanupTables(s.deps.DB)
}

func (s *VenueHandlerIntegrationSuite) TearDownSuite() {
	s.deps.TestDB.Cleanup()
}

func TestVenueHandlerIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	suite.Run(t, new(VenueHandlerIntegrationSuite))
}

// --- SearchVenuesHandler ---

func (s *VenueHandlerIntegrationSuite) TestSearchVenues_Success() {
	testhelpers.CreateVerifiedVenue(s.deps.DB, "Valley Bar", "Phoenix", "AZ")
	testhelpers.CreateVerifiedVenue(s.deps.DB, "Crescent Ballroom", "Phoenix", "AZ")

	req := &SearchVenuesRequest{Query: "Valley"}
	resp, err := s.handler.SearchVenuesHandler(s.deps.Ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.GreaterOrEqual(resp.Body.Count, 1)
}

func (s *VenueHandlerIntegrationSuite) TestSearchVenues_NoResults() {
	req := &SearchVenuesRequest{Query: "Nonexistent Venue XYZ"}
	resp, err := s.handler.SearchVenuesHandler(s.deps.Ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.Equal(0, resp.Body.Count)
}

// --- ListVenuesHandler ---

func (s *VenueHandlerIntegrationSuite) TestListVenues_Success() {
	testhelpers.CreateVerifiedVenue(s.deps.DB, "Valley Bar", "Phoenix", "AZ")
	testhelpers.CreateVerifiedVenue(s.deps.DB, "Crescent Ballroom", "Phoenix", "AZ")
	testhelpers.CreateVerifiedVenue(s.deps.DB, "The Rebel Lounge", "Phoenix", "AZ")

	req := &ListVenuesRequest{Limit: 50, Offset: 0}
	resp, err := s.handler.ListVenuesHandler(s.deps.Ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.GreaterOrEqual(resp.Body.Total, int64(3))
}

func (s *VenueHandlerIntegrationSuite) TestListVenues_Empty() {
	req := &ListVenuesRequest{Limit: 50, Offset: 0}
	resp, err := s.handler.ListVenuesHandler(s.deps.Ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.Equal(int64(0), resp.Body.Total)
}

func (s *VenueHandlerIntegrationSuite) TestListVenues_CityFilter() {
	testhelpers.CreateVerifiedVenue(s.deps.DB, "Valley Bar", "Phoenix", "AZ")
	testhelpers.CreateVerifiedVenue(s.deps.DB, "Club Congress", "Tucson", "AZ")

	req := &ListVenuesRequest{City: "Phoenix", Limit: 50, Offset: 0}
	resp, err := s.handler.ListVenuesHandler(s.deps.Ctx, req)
	s.NoError(err)
	s.Equal(int64(1), resp.Body.Total)
}

func (s *VenueHandlerIntegrationSuite) TestListVenues_MultiCityFilter() {
	testhelpers.CreateVerifiedVenue(s.deps.DB, "Valley Bar", "Phoenix", "AZ")
	testhelpers.CreateVerifiedVenue(s.deps.DB, "Club Congress", "Tucson", "AZ")
	testhelpers.CreateVerifiedVenue(s.deps.DB, "Empty Bottle", "Chicago", "IL")

	req := &ListVenuesRequest{Cities: "Phoenix,AZ|Chicago,IL", Limit: 50, Offset: 0}
	resp, err := s.handler.ListVenuesHandler(s.deps.Ctx, req)
	s.NoError(err)
	s.Equal(int64(2), resp.Body.Total)
	s.Len(resp.Body.Venues, 2)
}

// --- GetVenueHandler ---

func (s *VenueHandlerIntegrationSuite) TestGetVenue_ByID() {
	venue := testhelpers.CreateVerifiedVenue(s.deps.DB, "Valley Bar", "Phoenix", "AZ")

	req := &GetVenueRequest{VenueID: fmt.Sprintf("%d", venue.ID)}
	resp, err := s.handler.GetVenueHandler(s.deps.Ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.Equal(venue.ID, resp.Body.ID)
	s.Equal("Valley Bar", resp.Body.Name)
}

func (s *VenueHandlerIntegrationSuite) TestGetVenue_NotFound() {
	req := &GetVenueRequest{VenueID: "99999"}
	_, err := s.handler.GetVenueHandler(s.deps.Ctx, req)
	s.Error(err)
}

// --- GetVenueShowsHandler ---

func (s *VenueHandlerIntegrationSuite) TestGetVenueShows_Success() {
	user := testhelpers.CreateTestUser(s.deps.DB)
	venue := testhelpers.CreateVerifiedVenue(s.deps.DB, "Valley Bar", "Phoenix", "AZ")

	// Create a future show at this venue
	show := testhelpers.CreateFutureApprovedShow(s.deps.DB, user.ID, "Test Show", 7)
	s.deps.DB.Exec("INSERT INTO show_venues (show_id, venue_id) VALUES (?, ?)", show.ID, venue.ID)

	req := &GetVenueShowsRequest{
		VenueID:    fmt.Sprintf("%d", venue.ID),
		Timezone:   "UTC",
		Limit:      20,
		TimeFilter: "upcoming",
	}
	resp, err := s.handler.GetVenueShowsHandler(s.deps.Ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.Equal(venue.ID, resp.Body.VenueID)
}

// --- GetVenueCitiesHandler ---

func (s *VenueHandlerIntegrationSuite) TestGetVenueCities_Success() {
	testhelpers.CreateVerifiedVenue(s.deps.DB, "Valley Bar", "Phoenix", "AZ")
	testhelpers.CreateVerifiedVenue(s.deps.DB, "Club Congress", "Tucson", "AZ")

	req := &GetVenueCitiesRequest{}
	resp, err := s.handler.GetVenueCitiesHandler(s.deps.Ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.GreaterOrEqual(len(resp.Body.Cities), 2)
}

// --- UpdateVenueHandler ---

func (s *VenueHandlerIntegrationSuite) TestUpdateVenue_AdminDirectUpdate() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	venue := testhelpers.CreateVerifiedVenue(s.deps.DB, "Valley Bar", "Phoenix", "AZ")

	ctx := testhelpers.CtxWithUser(admin)
	newName := "Valley Bar Updated"
	req := &UpdateVenueRequest{VenueID: fmt.Sprintf("%d", venue.ID)}
	req.Body.Name = &newName

	resp, err := s.handler.UpdateVenueHandler(ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.NotNil(resp.Body)
	s.Equal("Valley Bar Updated", resp.Body.Name)
}

func (s *VenueHandlerIntegrationSuite) TestUpdateVenue_VenueNotFound() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	ctx := testhelpers.CtxWithUser(admin)

	newName := "Updated"
	req := &UpdateVenueRequest{VenueID: "99999"}
	req.Body.Name = &newName

	_, err := s.handler.UpdateVenueHandler(ctx, req)
	s.Error(err)
}

// --- DeleteVenueHandler ---

func (s *VenueHandlerIntegrationSuite) TestDeleteVenue_AdminSuccess() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	venue := testhelpers.CreateVerifiedVenue(s.deps.DB, "Delete Me", "Phoenix", "AZ")

	ctx := testhelpers.CtxWithUser(admin)
	req := &DeleteVenueRequest{VenueID: fmt.Sprintf("%d", venue.ID)}

	resp, err := s.handler.DeleteVenueHandler(ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.Contains(resp.Body.Message, "deleted")
}

func (s *VenueHandlerIntegrationSuite) TestDeleteVenue_OwnerSuccess() {
	user := testhelpers.CreateTestUser(s.deps.DB)
	venue := &catalogm.Venue{
		Name:        "My Venue",
		City:        "Phoenix",
		State:       "AZ",
		SubmittedBy: &user.ID,
	}
	s.deps.DB.Create(venue)

	ctx := testhelpers.CtxWithUser(user)
	req := &DeleteVenueRequest{VenueID: fmt.Sprintf("%d", venue.ID)}

	resp, err := s.handler.DeleteVenueHandler(ctx, req)
	s.NoError(err)
	s.NotNil(resp)
}

func (s *VenueHandlerIntegrationSuite) TestDeleteVenue_NonOwnerForbidden() {
	user := testhelpers.CreateTestUser(s.deps.DB)
	venue := testhelpers.CreateVerifiedVenue(s.deps.DB, "Valley Bar", "Phoenix", "AZ")

	ctx := testhelpers.CtxWithUser(user)
	req := &DeleteVenueRequest{VenueID: fmt.Sprintf("%d", venue.ID)}

	_, err := s.handler.DeleteVenueHandler(ctx, req)
	s.Error(err)
}

func (s *VenueHandlerIntegrationSuite) TestDeleteVenue_NotFound() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	ctx := testhelpers.CtxWithUser(admin)

	req := &DeleteVenueRequest{VenueID: "99999"}
	_, err := s.handler.DeleteVenueHandler(ctx, req)
	s.Error(err)
}
