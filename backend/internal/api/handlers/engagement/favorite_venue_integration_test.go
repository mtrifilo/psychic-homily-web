package engagement

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	catalogm "psychic-homily-backend/internal/models/catalog"
)

type FavoriteVenueHandlerIntegrationSuite struct {
	suite.Suite
	deps    *testhelpers.IntegrationDeps
	handler *FavoriteVenueHandler
}

func (s *FavoriteVenueHandlerIntegrationSuite) SetupSuite() {
	s.deps = testhelpers.SetupIntegrationDeps(s.T())
	s.handler = NewFavoriteVenueHandler(s.deps.FavoriteVenueService)
}

func (s *FavoriteVenueHandlerIntegrationSuite) TearDownTest() {
	testhelpers.CleanupTables(s.deps.DB)
}

func (s *FavoriteVenueHandlerIntegrationSuite) TearDownSuite() {
	s.deps.TestDB.Cleanup()
}

func TestFavoriteVenueHandlerIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	suite.Run(t, new(FavoriteVenueHandlerIntegrationSuite))
}

// --- FavoriteVenueHandler (POST) ---

func (s *FavoriteVenueHandlerIntegrationSuite) TestFavoriteVenue_Success() {
	user := testhelpers.CreateTestUser(s.deps.DB)
	venue := testhelpers.CreateVerifiedVenue(s.deps.DB, "Valley Bar", "Phoenix", "AZ")

	ctx := testhelpers.CtxWithUser(user)
	req := &FavoriteVenueRequest{VenueID: fmt.Sprintf("%d", venue.ID)}

	resp, err := s.handler.FavoriteVenueHandler(ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.True(resp.Body.Success)
}

func (s *FavoriteVenueHandlerIntegrationSuite) TestFavoriteVenue_AlreadyFavorited_Idempotent() {
	user := testhelpers.CreateTestUser(s.deps.DB)
	venue := testhelpers.CreateVerifiedVenue(s.deps.DB, "Valley Bar", "Phoenix", "AZ")

	ctx := testhelpers.CtxWithUser(user)
	req := &FavoriteVenueRequest{VenueID: fmt.Sprintf("%d", venue.ID)}

	_, err := s.handler.FavoriteVenueHandler(ctx, req)
	s.NoError(err)

	// Favorite again — should succeed (service uses FirstOrCreate)
	resp, err := s.handler.FavoriteVenueHandler(ctx, req)
	s.NoError(err)
	s.True(resp.Body.Success)
}

func (s *FavoriteVenueHandlerIntegrationSuite) TestFavoriteVenue_VenueNotFound() {
	user := testhelpers.CreateTestUser(s.deps.DB)
	ctx := testhelpers.CtxWithUser(user)
	req := &FavoriteVenueRequest{VenueID: "99999"}

	_, err := s.handler.FavoriteVenueHandler(ctx, req)
	s.Error(err)
}

// --- UnfavoriteVenueHandler (DELETE) ---

func (s *FavoriteVenueHandlerIntegrationSuite) TestUnfavoriteVenue_Success() {
	user := testhelpers.CreateTestUser(s.deps.DB)
	venue := testhelpers.CreateVerifiedVenue(s.deps.DB, "Valley Bar", "Phoenix", "AZ")
	ctx := testhelpers.CtxWithUser(user)

	// Favorite first
	favReq := &FavoriteVenueRequest{VenueID: fmt.Sprintf("%d", venue.ID)}
	_, err := s.handler.FavoriteVenueHandler(ctx, favReq)
	s.NoError(err)

	// Unfavorite
	unfavReq := &UnfavoriteVenueRequest{VenueID: fmt.Sprintf("%d", venue.ID)}
	resp, err := s.handler.UnfavoriteVenueHandler(ctx, unfavReq)
	s.NoError(err)
	s.NotNil(resp)
	s.True(resp.Body.Success)
}

func (s *FavoriteVenueHandlerIntegrationSuite) TestUnfavoriteVenue_NotFavorited() {
	user := testhelpers.CreateTestUser(s.deps.DB)
	venue := testhelpers.CreateVerifiedVenue(s.deps.DB, "Valley Bar", "Phoenix", "AZ")
	ctx := testhelpers.CtxWithUser(user)

	req := &UnfavoriteVenueRequest{VenueID: fmt.Sprintf("%d", venue.ID)}
	_, err := s.handler.UnfavoriteVenueHandler(ctx, req)
	s.Error(err)
}

// --- GetFavoriteVenuesHandler ---

func (s *FavoriteVenueHandlerIntegrationSuite) TestGetFavoriteVenues_Empty() {
	user := testhelpers.CreateTestUser(s.deps.DB)
	ctx := testhelpers.CtxWithUser(user)
	req := &GetFavoriteVenuesRequest{Limit: 50, Offset: 0}

	resp, err := s.handler.GetFavoriteVenuesHandler(ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.Equal(int64(0), resp.Body.Total)
	s.Empty(resp.Body.Venues)
}

func (s *FavoriteVenueHandlerIntegrationSuite) TestGetFavoriteVenues_WithVenues() {
	user := testhelpers.CreateTestUser(s.deps.DB)
	ctx := testhelpers.CtxWithUser(user)

	venues := []string{"Valley Bar", "Crescent Ballroom", "The Rebel Lounge"}
	for _, name := range venues {
		venue := testhelpers.CreateVerifiedVenue(s.deps.DB, name, "Phoenix", "AZ")
		favReq := &FavoriteVenueRequest{VenueID: fmt.Sprintf("%d", venue.ID)}
		_, err := s.handler.FavoriteVenueHandler(ctx, favReq)
		s.NoError(err)
	}

	req := &GetFavoriteVenuesRequest{Limit: 50, Offset: 0}
	resp, err := s.handler.GetFavoriteVenuesHandler(ctx, req)
	s.NoError(err)
	s.Equal(int64(3), resp.Body.Total)
	s.Len(resp.Body.Venues, 3)
}

func (s *FavoriteVenueHandlerIntegrationSuite) TestGetFavoriteVenues_Pagination() {
	user := testhelpers.CreateTestUser(s.deps.DB)
	ctx := testhelpers.CtxWithUser(user)

	for i := 0; i < 3; i++ {
		venue := testhelpers.CreateVerifiedVenue(s.deps.DB, fmt.Sprintf("Venue %d", i), "Phoenix", "AZ")
		favReq := &FavoriteVenueRequest{VenueID: fmt.Sprintf("%d", venue.ID)}
		_, err := s.handler.FavoriteVenueHandler(ctx, favReq)
		s.NoError(err)
	}

	// First page
	req := &GetFavoriteVenuesRequest{Limit: 2, Offset: 0}
	resp, err := s.handler.GetFavoriteVenuesHandler(ctx, req)
	s.NoError(err)
	s.Len(resp.Body.Venues, 2)
	s.Equal(int64(3), resp.Body.Total)

	// Second page
	req2 := &GetFavoriteVenuesRequest{Limit: 2, Offset: 2}
	resp2, err := s.handler.GetFavoriteVenuesHandler(ctx, req2)
	s.NoError(err)
	s.Len(resp2.Body.Venues, 1)
}

// --- CheckFavoritedHandler ---

func (s *FavoriteVenueHandlerIntegrationSuite) TestCheckFavorited_True() {
	user := testhelpers.CreateTestUser(s.deps.DB)
	venue := testhelpers.CreateVerifiedVenue(s.deps.DB, "Valley Bar", "Phoenix", "AZ")
	ctx := testhelpers.CtxWithUser(user)

	// Favorite
	favReq := &FavoriteVenueRequest{VenueID: fmt.Sprintf("%d", venue.ID)}
	_, err := s.handler.FavoriteVenueHandler(ctx, favReq)
	s.NoError(err)

	// Check
	checkReq := &CheckFavoritedRequest{VenueID: fmt.Sprintf("%d", venue.ID)}
	resp, err := s.handler.CheckFavoritedHandler(ctx, checkReq)
	s.NoError(err)
	s.True(resp.Body.IsFavorited)
}

func (s *FavoriteVenueHandlerIntegrationSuite) TestCheckFavorited_False() {
	user := testhelpers.CreateTestUser(s.deps.DB)
	venue := testhelpers.CreateVerifiedVenue(s.deps.DB, "Valley Bar", "Phoenix", "AZ")
	ctx := testhelpers.CtxWithUser(user)

	checkReq := &CheckFavoritedRequest{VenueID: fmt.Sprintf("%d", venue.ID)}
	resp, err := s.handler.CheckFavoritedHandler(ctx, checkReq)
	s.NoError(err)
	s.False(resp.Body.IsFavorited)
}

// --- GetFavoriteVenueShowsHandler ---

func (s *FavoriteVenueHandlerIntegrationSuite) TestGetFavoriteVenueShows_Empty() {
	user := testhelpers.CreateTestUser(s.deps.DB)
	ctx := testhelpers.CtxWithUser(user)

	req := &GetFavoriteVenueShowsRequest{Timezone: "America/Phoenix", Limit: 50, Offset: 0}
	resp, err := s.handler.GetFavoriteVenueShowsHandler(ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.Equal(int64(0), resp.Body.Total)
}

func (s *FavoriteVenueHandlerIntegrationSuite) TestGetFavoriteVenueShows_WithShows() {
	user := testhelpers.CreateTestUser(s.deps.DB)
	ctx := testhelpers.CtxWithUser(user)

	// Create venue, favorite it, then create a future show at it
	venue := testhelpers.CreateVerifiedVenue(s.deps.DB, "Valley Bar", "Phoenix", "AZ")
	favReq := &FavoriteVenueRequest{VenueID: fmt.Sprintf("%d", venue.ID)}
	_, err := s.handler.FavoriteVenueHandler(ctx, favReq)
	s.NoError(err)

	// Create a future show at this venue
	show := &catalogm.Show{
		Title:       "Upcoming Show",
		EventDate:   time.Now().UTC().AddDate(0, 0, 7),
		City:        testhelpers.StringPtr("Phoenix"),
		State:       testhelpers.StringPtr("AZ"),
		Status:      catalogm.ShowStatusApproved,
		SubmittedBy: &user.ID,
	}
	s.deps.DB.Create(show)
	s.deps.DB.Exec("INSERT INTO show_venues (show_id, venue_id) VALUES (?, ?)", show.ID, venue.ID)

	artist := testhelpers.CreateArtist(s.deps.DB, "Test Artist")
	s.deps.DB.Exec("INSERT INTO show_artists (show_id, artist_id, position, set_type) VALUES (?, ?, 0, 'headliner')", show.ID, artist.ID)

	req := &GetFavoriteVenueShowsRequest{Timezone: "America/Phoenix", Limit: 50, Offset: 0}
	resp, err := s.handler.GetFavoriteVenueShowsHandler(ctx, req)
	s.NoError(err)
	s.GreaterOrEqual(resp.Body.Total, int64(1))
}
