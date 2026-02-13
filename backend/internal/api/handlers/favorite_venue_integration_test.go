package handlers

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	"psychic-homily-backend/internal/models"
)

type FavoriteVenueHandlerIntegrationSuite struct {
	suite.Suite
	deps    *handlerIntegrationDeps
	handler *FavoriteVenueHandler
}

func (s *FavoriteVenueHandlerIntegrationSuite) SetupSuite() {
	s.deps = setupHandlerIntegrationDeps(s.T())
	s.handler = NewFavoriteVenueHandler(s.deps.favoriteVenueService)
}

func (s *FavoriteVenueHandlerIntegrationSuite) TearDownTest() {
	cleanupTables(s.deps.db)
}

func (s *FavoriteVenueHandlerIntegrationSuite) TearDownSuite() {
	if s.deps.container != nil {
		s.deps.container.Terminate(s.deps.ctx)
	}
}

func TestFavoriteVenueHandlerIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	suite.Run(t, new(FavoriteVenueHandlerIntegrationSuite))
}

// --- FavoriteVenueHandler (POST) ---

func (s *FavoriteVenueHandlerIntegrationSuite) TestFavoriteVenue_Success() {
	user := createTestUser(s.deps.db)
	venue := createVerifiedVenue(s.deps.db, "Valley Bar", "Phoenix", "AZ")

	ctx := ctxWithUser(user)
	req := &FavoriteVenueRequest{VenueID: fmt.Sprintf("%d", venue.ID)}

	resp, err := s.handler.FavoriteVenueHandler(ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.True(resp.Body.Success)
}

func (s *FavoriteVenueHandlerIntegrationSuite) TestFavoriteVenue_AlreadyFavorited_Idempotent() {
	user := createTestUser(s.deps.db)
	venue := createVerifiedVenue(s.deps.db, "Valley Bar", "Phoenix", "AZ")

	ctx := ctxWithUser(user)
	req := &FavoriteVenueRequest{VenueID: fmt.Sprintf("%d", venue.ID)}

	_, err := s.handler.FavoriteVenueHandler(ctx, req)
	s.NoError(err)

	// Favorite again — should succeed (service uses FirstOrCreate)
	resp, err := s.handler.FavoriteVenueHandler(ctx, req)
	s.NoError(err)
	s.True(resp.Body.Success)
}

func (s *FavoriteVenueHandlerIntegrationSuite) TestFavoriteVenue_VenueNotFound() {
	user := createTestUser(s.deps.db)
	ctx := ctxWithUser(user)
	req := &FavoriteVenueRequest{VenueID: "99999"}

	_, err := s.handler.FavoriteVenueHandler(ctx, req)
	s.Error(err)
}

// --- UnfavoriteVenueHandler (DELETE) ---

func (s *FavoriteVenueHandlerIntegrationSuite) TestUnfavoriteVenue_Success() {
	user := createTestUser(s.deps.db)
	venue := createVerifiedVenue(s.deps.db, "Valley Bar", "Phoenix", "AZ")
	ctx := ctxWithUser(user)

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
	user := createTestUser(s.deps.db)
	venue := createVerifiedVenue(s.deps.db, "Valley Bar", "Phoenix", "AZ")
	ctx := ctxWithUser(user)

	req := &UnfavoriteVenueRequest{VenueID: fmt.Sprintf("%d", venue.ID)}
	_, err := s.handler.UnfavoriteVenueHandler(ctx, req)
	s.Error(err)
}

// --- GetFavoriteVenuesHandler ---

func (s *FavoriteVenueHandlerIntegrationSuite) TestGetFavoriteVenues_Empty() {
	user := createTestUser(s.deps.db)
	ctx := ctxWithUser(user)
	req := &GetFavoriteVenuesRequest{Limit: 50, Offset: 0}

	resp, err := s.handler.GetFavoriteVenuesHandler(ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.Equal(int64(0), resp.Body.Total)
	s.Empty(resp.Body.Venues)
}

func (s *FavoriteVenueHandlerIntegrationSuite) TestGetFavoriteVenues_WithVenues() {
	user := createTestUser(s.deps.db)
	ctx := ctxWithUser(user)

	venues := []string{"Valley Bar", "Crescent Ballroom", "The Rebel Lounge"}
	for _, name := range venues {
		venue := createVerifiedVenue(s.deps.db, name, "Phoenix", "AZ")
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
	user := createTestUser(s.deps.db)
	ctx := ctxWithUser(user)

	for i := 0; i < 3; i++ {
		venue := createVerifiedVenue(s.deps.db, fmt.Sprintf("Venue %d", i), "Phoenix", "AZ")
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
	user := createTestUser(s.deps.db)
	venue := createVerifiedVenue(s.deps.db, "Valley Bar", "Phoenix", "AZ")
	ctx := ctxWithUser(user)

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
	user := createTestUser(s.deps.db)
	venue := createVerifiedVenue(s.deps.db, "Valley Bar", "Phoenix", "AZ")
	ctx := ctxWithUser(user)

	checkReq := &CheckFavoritedRequest{VenueID: fmt.Sprintf("%d", venue.ID)}
	resp, err := s.handler.CheckFavoritedHandler(ctx, checkReq)
	s.NoError(err)
	s.False(resp.Body.IsFavorited)
}

// --- GetFavoriteVenueShowsHandler ---

func (s *FavoriteVenueHandlerIntegrationSuite) TestGetFavoriteVenueShows_Empty() {
	user := createTestUser(s.deps.db)
	ctx := ctxWithUser(user)

	req := &GetFavoriteVenueShowsRequest{Timezone: "America/Phoenix", Limit: 50, Offset: 0}
	resp, err := s.handler.GetFavoriteVenueShowsHandler(ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.Equal(int64(0), resp.Body.Total)
}

func (s *FavoriteVenueHandlerIntegrationSuite) TestGetFavoriteVenueShows_WithShows() {
	user := createTestUser(s.deps.db)
	ctx := ctxWithUser(user)

	// Create venue, favorite it, then create a future show at it
	venue := createVerifiedVenue(s.deps.db, "Valley Bar", "Phoenix", "AZ")
	favReq := &FavoriteVenueRequest{VenueID: fmt.Sprintf("%d", venue.ID)}
	_, err := s.handler.FavoriteVenueHandler(ctx, favReq)
	s.NoError(err)

	// Create a future show at this venue
	show := &models.Show{
		Title:       "Upcoming Show",
		EventDate:   time.Now().UTC().AddDate(0, 0, 7),
		City:        stringPtr("Phoenix"),
		State:       stringPtr("AZ"),
		Status:      models.ShowStatusApproved,
		SubmittedBy: &user.ID,
	}
	s.deps.db.Create(show)
	s.deps.db.Exec("INSERT INTO show_venues (show_id, venue_id) VALUES (?, ?)", show.ID, venue.ID)

	artist := createArtist(s.deps.db, "Test Artist")
	s.deps.db.Exec("INSERT INTO show_artists (show_id, artist_id, position, set_type) VALUES (?, ?, 0, 'headliner')", show.ID, artist.ID)

	req := &GetFavoriteVenueShowsRequest{Timezone: "America/Phoenix", Limit: 50, Offset: 0}
	resp, err := s.handler.GetFavoriteVenueShowsHandler(ctx, req)
	s.NoError(err)
	s.GreaterOrEqual(resp.Body.Total, int64(1))
}
