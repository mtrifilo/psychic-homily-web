package catalog

import (
	"fmt"
	"testing"

	"github.com/danielgtaylor/huma/v2"
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

// --- Street-coordinate privacy gate (PSY-1536) ---

// seedStreetGeocode stamps a street-level geocode onto a venue directly in the
// DB, with geocoded_address matching the address so the geocode reads as
// FRESH — isolating the tests below to the verified/unverified privacy gate.
func (s *VenueHandlerIntegrationSuite) seedStreetGeocode(venueID uint, address, key string) {
	s.Require().NoError(s.deps.DB.Model(&catalogm.Venue{}).Where("id = ?", venueID).Updates(map[string]interface{}{
		"address":           address,
		"street_latitude":   33.448227,
		"street_longitude":  -112.073069,
		"geocode_precision": "rooftop",
		"geocoded_address":  key,
	}).Error)
}

func (s *VenueHandlerIntegrationSuite) TestGetVenue_UnverifiedNeverExposesStreetCoords() {
	venue := testhelpers.CreateUnverifiedVenue(s.deps.DB, "House Venue", "Phoenix", "AZ")
	s.seedStreetGeocode(venue.ID, "1234 Secret House St", "1234 Secret House St, Phoenix, AZ")

	req := &GetVenueRequest{VenueID: fmt.Sprintf("%d", venue.ID)}
	resp, err := s.handler.GetVenueHandler(s.deps.Ctx, req)
	s.Require().NoError(err)
	// Street-precise coordinates would map a DIY/house venue before human
	// review — they must be absent from the API response, exactly like the
	// existing address/zipcode redaction.
	s.Nil(resp.Body.StreetLatitude, "unverified venue must not expose street latitude")
	s.Nil(resp.Body.StreetLongitude, "unverified venue must not expose street longitude")
	s.Nil(resp.Body.GeocodePrecision, "unverified venue must not expose geocode precision")
	s.Nil(resp.Body.Address, "existing address redaction must still hold")
}

func (s *VenueHandlerIntegrationSuite) TestGetVenue_VerifiedExposesFreshStreetCoords() {
	venue := testhelpers.CreateVerifiedVenue(s.deps.DB, "Public Club", "Phoenix", "AZ")
	s.seedStreetGeocode(venue.ID, "130 N Central Ave", "130 N Central Ave, Phoenix, AZ")

	req := &GetVenueRequest{VenueID: fmt.Sprintf("%d", venue.ID)}
	resp, err := s.handler.GetVenueHandler(s.deps.Ctx, req)
	s.Require().NoError(err)
	s.Require().NotNil(resp.Body.StreetLatitude)
	s.InDelta(33.448227, *resp.Body.StreetLatitude, 1e-6)
	s.Require().NotNil(resp.Body.StreetLongitude)
	s.InDelta(-112.073069, *resp.Body.StreetLongitude, 1e-6)
	s.Require().NotNil(resp.Body.GeocodePrecision)
	s.Equal("rooftop", *resp.Body.GeocodePrecision)
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

// The envelope has to echo the window the server actually used, not the window
// the caller typed: a client computing "is there a next page" from its own
// request cannot see the handler's default-limit substitution (PSY-1750).
func (s *VenueHandlerIntegrationSuite) TestGetVenueShows_EchoesTheWindowActuallyUsed() {
	user := testhelpers.CreateTestUser(s.deps.DB)
	venue := testhelpers.CreateVerifiedVenue(s.deps.DB, "Echo Bar", "Phoenix", "AZ")
	for i := 0; i < 3; i++ {
		show := testhelpers.CreateFutureApprovedShow(s.deps.DB, user.ID, fmt.Sprintf("Echo Show %d", i), 7+i)
		s.deps.DB.Exec("INSERT INTO show_venues (show_id, venue_id) VALUES (?, ?)", show.ID, venue.ID)
	}

	req := &GetVenueShowsRequest{
		VenueID:    fmt.Sprintf("%d", venue.ID),
		Limit:      2,
		Offset:     2,
		TimeFilter: "upcoming",
	}
	resp, err := s.handler.GetVenueShowsHandler(s.deps.Ctx, req)
	s.Require().NoError(err)
	s.Equal(2, resp.Body.Limit)
	s.Equal(2, resp.Body.Offset)
	s.Equal(0, resp.Body.Year)
	s.Equal(int64(3), resp.Body.Total, "total spans every page, not just this one")
	s.Len(resp.Body.Shows, 1, "the third show is the whole last page")

	// An omitted limit is substituted server-side, and the echo must say 20
	// rather than the 0 the caller sent.
	defaulted, err := s.handler.GetVenueShowsHandler(s.deps.Ctx, &GetVenueShowsRequest{
		VenueID: fmt.Sprintf("%d", venue.ID), TimeFilter: "upcoming",
	})
	s.Require().NoError(err)
	s.Equal(20, defaulted.Body.Limit)
	s.Equal(0, defaulted.Body.Offset)
}

func (s *VenueHandlerIntegrationSuite) TestGetVenueShows_VenueNotFound() {
	_, err := s.handler.GetVenueShowsHandler(s.deps.Ctx, &GetVenueShowsRequest{
		VenueID: "no-such-venue-slug", TimeFilter: "upcoming",
	})
	s.Require().Error(err)
	var statusErr huma.StatusError
	s.Require().ErrorAs(err, &statusErr)
	s.Equal(404, statusErr.GetStatus())
}

// --- GetVenueShowYearsHandler ---

func (s *VenueHandlerIntegrationSuite) TestGetVenueShowYears_ResolvesBySlugAndEchoesFilter() {
	user := testhelpers.CreateTestUser(s.deps.DB)
	venue := testhelpers.CreateVerifiedVenue(s.deps.DB, "Years Bar", "Phoenix", "AZ")
	show := testhelpers.CreateFutureApprovedShow(s.deps.DB, user.ID, "Years Show", 14)
	s.deps.DB.Exec("INSERT INTO show_venues (show_id, venue_id) VALUES (?, ?)", show.ID, venue.ID)

	resp, err := s.handler.GetVenueShowYearsHandler(s.deps.Ctx, &GetVenueShowYearsRequest{
		VenueID: fmt.Sprintf("%d", venue.ID), TimeFilter: "upcoming",
	})
	s.Require().NoError(err)
	s.Equal(venue.ID, resp.Body.VenueID)
	s.Equal("upcoming", resp.Body.TimeFilter)
	s.Require().Len(resp.Body.Years, 1)
	s.Equal(int64(1), resp.Body.Years[0].Count)

	// An omitted time_filter must land on the same default the list uses, or the
	// picker and the list it drives would be counting different sets.
	defaulted, err := s.handler.GetVenueShowYearsHandler(s.deps.Ctx, &GetVenueShowYearsRequest{
		VenueID: fmt.Sprintf("%d", venue.ID),
	})
	s.Require().NoError(err)
	s.Equal("upcoming", defaulted.Body.TimeFilter)
	s.Equal(resp.Body.Years, defaulted.Body.Years)
}

func (s *VenueHandlerIntegrationSuite) TestGetVenueShowYears_VenueNotFound() {
	_, err := s.handler.GetVenueShowYearsHandler(s.deps.Ctx, &GetVenueShowYearsRequest{
		VenueID: "no-such-venue-slug", TimeFilter: "all",
	})
	s.Require().Error(err)
	var statusErr huma.StatusError
	s.Require().ErrorAs(err, &statusErr)
	s.Equal(404, statusErr.GetStatus())
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
