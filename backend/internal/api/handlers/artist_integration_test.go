package handlers

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	"psychic-homily-backend/internal/models"
	"psychic-homily-backend/internal/services"
)

type ArtistHandlerIntegrationSuite struct {
	suite.Suite
	deps    *handlerIntegrationDeps
	handler *ArtistHandler
}

func (s *ArtistHandlerIntegrationSuite) SetupSuite() {
	s.deps = setupHandlerIntegrationDeps(s.T())
	s.handler = NewArtistHandler(s.deps.artistService, s.deps.auditLogService)
}

func (s *ArtistHandlerIntegrationSuite) TearDownTest() {
	cleanupTables(s.deps.db)
}

func (s *ArtistHandlerIntegrationSuite) TearDownSuite() {
	if s.deps.container != nil {
		s.deps.container.Terminate(s.deps.ctx)
	}
}

func TestArtistHandlerIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	suite.Run(t, new(ArtistHandlerIntegrationSuite))
}

// createArtistWithSlug creates an artist using the service (which generates a slug)
func (s *ArtistHandlerIntegrationSuite) createArtistViaService(name string) uint {
	resp, err := s.deps.artistService.CreateArtist(&services.CreateArtistRequest{Name: name})
	s.Require().NoError(err)
	return resp.ID
}

// createArtistWithCity creates an artist with city and state via raw insert + slug
func (s *ArtistHandlerIntegrationSuite) createArtistWithCity(name, city, state string) *models.Artist {
	artist := &models.Artist{
		Name:  name,
		City:  &city,
		State: &state,
	}
	s.deps.db.Create(artist)
	// Set a slug so slug-based lookups work
	slug := fmt.Sprintf("%s-slug", name)
	s.deps.db.Model(artist).Update("slug", slug)
	return artist
}

// --- SearchArtistsHandler ---

func (s *ArtistHandlerIntegrationSuite) TestSearchArtists_Success() {
	s.createArtistViaService("Radiohead")
	s.createArtistViaService("Radio Moscow")
	s.createArtistViaService("Unrelated Band")

	req := &SearchArtistsRequest{Query: "radio"}
	resp, err := s.handler.SearchArtistsHandler(s.deps.ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.GreaterOrEqual(resp.Body.Count, 2)
}

func (s *ArtistHandlerIntegrationSuite) TestSearchArtists_NoResults() {
	s.createArtistViaService("Radiohead")

	req := &SearchArtistsRequest{Query: "zzzznonexistent"}
	resp, err := s.handler.SearchArtistsHandler(s.deps.ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.Equal(0, resp.Body.Count)
}

// --- ListArtistsHandler ---

func (s *ArtistHandlerIntegrationSuite) TestListArtists_Success() {
	s.createArtistViaService("Artist A")
	s.createArtistViaService("Artist B")
	s.createArtistViaService("Artist C")

	req := &ListArtistsRequest{}
	resp, err := s.handler.ListArtistsHandler(s.deps.ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.GreaterOrEqual(resp.Body.Count, 3)
}

func (s *ArtistHandlerIntegrationSuite) TestListArtists_Empty() {
	req := &ListArtistsRequest{}
	resp, err := s.handler.ListArtistsHandler(s.deps.ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.Equal(0, resp.Body.Count)
}

func (s *ArtistHandlerIntegrationSuite) TestListArtists_CityFilter() {
	s.createArtistWithCity("Phoenix Band", "Phoenix", "AZ")
	s.createArtistWithCity("Tucson Band", "Tucson", "AZ")

	req := &ListArtistsRequest{City: "Phoenix"}
	resp, err := s.handler.ListArtistsHandler(s.deps.ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.Equal(1, resp.Body.Count)
	s.Equal("Phoenix Band", resp.Body.Artists[0].Name)
}

// --- GetArtistHandler ---

func (s *ArtistHandlerIntegrationSuite) TestGetArtist_ByID() {
	id := s.createArtistViaService("The National")

	req := &GetArtistRequest{ArtistID: fmt.Sprintf("%d", id)}
	resp, err := s.handler.GetArtistHandler(s.deps.ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.Equal("The National", resp.Body.Name)
}

func (s *ArtistHandlerIntegrationSuite) TestGetArtist_BySlug() {
	s.createArtistViaService("The National")

	req := &GetArtistRequest{ArtistID: "the-national"}
	resp, err := s.handler.GetArtistHandler(s.deps.ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.Equal("The National", resp.Body.Name)
}

func (s *ArtistHandlerIntegrationSuite) TestGetArtist_NotFound() {
	req := &GetArtistRequest{ArtistID: "99999"}
	_, err := s.handler.GetArtistHandler(s.deps.ctx, req)
	assertHumaError(s.T(), err, 404)
}

// --- GetArtistShowsHandler ---

func (s *ArtistHandlerIntegrationSuite) TestGetArtistShows_Success() {
	user := createTestUser(s.deps.db)
	venue := createVerifiedVenue(s.deps.db, "Test Venue", "Phoenix", "AZ")

	artistID := s.createArtistViaService("Show Artist")

	// Create an approved show and associate the artist
	show := &models.Show{
		Title:       "Future Gig",
		EventDate:   time.Now().UTC().AddDate(0, 0, 30),
		City:        stringPtr("Phoenix"),
		State:       stringPtr("AZ"),
		Status:      models.ShowStatusApproved,
		SubmittedBy: &user.ID,
	}
	s.deps.db.Create(show)
	s.deps.db.Exec("INSERT INTO show_venues (show_id, venue_id) VALUES (?, ?)", show.ID, venue.ID)
	s.deps.db.Exec("INSERT INTO show_artists (show_id, artist_id, position, set_type) VALUES (?, ?, 0, 'headliner')", show.ID, artistID)

	req := &GetArtistShowsRequest{ArtistID: fmt.Sprintf("%d", artistID), TimeFilter: "upcoming"}
	resp, err := s.handler.GetArtistShowsHandler(s.deps.ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.Equal(int64(1), resp.Body.Total)
	s.Equal("Future Gig", resp.Body.Shows[0].Title)
}

func (s *ArtistHandlerIntegrationSuite) TestGetArtistShows_Empty() {
	artistID := s.createArtistViaService("Lonely Artist")

	req := &GetArtistShowsRequest{ArtistID: fmt.Sprintf("%d", artistID), TimeFilter: "upcoming"}
	resp, err := s.handler.GetArtistShowsHandler(s.deps.ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.Equal(int64(0), resp.Body.Total)
}

func (s *ArtistHandlerIntegrationSuite) TestGetArtistShows_BySlug() {
	user := createTestUser(s.deps.db)
	venue := createVerifiedVenue(s.deps.db, "Slug Venue", "Phoenix", "AZ")

	artistID := s.createArtistViaService("Slug Artist")

	show := &models.Show{
		Title:       "Slug Show",
		EventDate:   time.Now().UTC().AddDate(0, 0, 30),
		City:        stringPtr("Phoenix"),
		State:       stringPtr("AZ"),
		Status:      models.ShowStatusApproved,
		SubmittedBy: &user.ID,
	}
	s.deps.db.Create(show)
	s.deps.db.Exec("INSERT INTO show_venues (show_id, venue_id) VALUES (?, ?)", show.ID, venue.ID)
	s.deps.db.Exec("INSERT INTO show_artists (show_id, artist_id, position, set_type) VALUES (?, ?, 0, 'headliner')", show.ID, artistID)

	req := &GetArtistShowsRequest{ArtistID: "slug-artist", TimeFilter: "upcoming"}
	resp, err := s.handler.GetArtistShowsHandler(s.deps.ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.Equal(int64(1), resp.Body.Total)
}

func (s *ArtistHandlerIntegrationSuite) TestGetArtistShows_NotFound() {
	req := &GetArtistShowsRequest{ArtistID: "99999", TimeFilter: "upcoming"}
	_, err := s.handler.GetArtistShowsHandler(s.deps.ctx, req)
	assertHumaError(s.T(), err, 404)
}

// --- DeleteArtistHandler ---

func (s *ArtistHandlerIntegrationSuite) TestDeleteArtist_Success() {
	user := createTestUser(s.deps.db)
	artistID := s.createArtistViaService("Deletable Artist")

	ctx := ctxWithUser(user)
	req := &DeleteArtistRequest{ArtistID: fmt.Sprintf("%d", artistID)}
	_, err := s.handler.DeleteArtistHandler(ctx, req)
	s.NoError(err)

	// Verify artist is gone
	getReq := &GetArtistRequest{ArtistID: fmt.Sprintf("%d", artistID)}
	_, err = s.handler.GetArtistHandler(s.deps.ctx, getReq)
	assertHumaError(s.T(), err, 404)
}

func (s *ArtistHandlerIntegrationSuite) TestDeleteArtist_HasShows() {
	user := createTestUser(s.deps.db)
	venue := createVerifiedVenue(s.deps.db, "Show Venue", "Phoenix", "AZ")
	artistID := s.createArtistViaService("Busy Artist")

	show := &models.Show{
		Title:       "Active Show",
		EventDate:   time.Now().UTC().AddDate(0, 0, 30),
		City:        stringPtr("Phoenix"),
		State:       stringPtr("AZ"),
		Status:      models.ShowStatusApproved,
		SubmittedBy: &user.ID,
	}
	s.deps.db.Create(show)
	s.deps.db.Exec("INSERT INTO show_venues (show_id, venue_id) VALUES (?, ?)", show.ID, venue.ID)
	s.deps.db.Exec("INSERT INTO show_artists (show_id, artist_id, position, set_type) VALUES (?, ?, 0, 'headliner')", show.ID, artistID)

	ctx := ctxWithUser(user)
	req := &DeleteArtistRequest{ArtistID: fmt.Sprintf("%d", artistID)}
	_, err := s.handler.DeleteArtistHandler(ctx, req)
	assertHumaError(s.T(), err, 409)
}

func (s *ArtistHandlerIntegrationSuite) TestDeleteArtist_NotFound() {
	user := createTestUser(s.deps.db)
	ctx := ctxWithUser(user)
	req := &DeleteArtistRequest{ArtistID: "99999"}

	_, err := s.handler.DeleteArtistHandler(ctx, req)
	assertHumaError(s.T(), err, 404)
}

// --- AdminUpdateArtistHandler ---

func (s *ArtistHandlerIntegrationSuite) TestAdminUpdateArtist_Success() {
	admin := createAdminUser(s.deps.db)
	artistID := s.createArtistViaService("Original Name")

	ctx := ctxWithUser(admin)
	newName := "Updated Name"
	req := &AdminUpdateArtistRequest{ArtistID: fmt.Sprintf("%d", artistID)}
	req.Body.Name = &newName

	resp, err := s.handler.AdminUpdateArtistHandler(ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.Equal("Updated Name", resp.Body.Name)
}

func (s *ArtistHandlerIntegrationSuite) TestAdminUpdateArtist_SocialLinks() {
	admin := createAdminUser(s.deps.db)
	artistID := s.createArtistViaService("Social Artist")

	ctx := ctxWithUser(admin)
	instagram := "https://instagram.com/artist"
	website := "https://artist.com"
	req := &AdminUpdateArtistRequest{ArtistID: fmt.Sprintf("%d", artistID)}
	req.Body.Instagram = &instagram
	req.Body.Website = &website

	resp, err := s.handler.AdminUpdateArtistHandler(ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.NotNil(resp.Body.Social.Instagram)
	s.Equal("https://instagram.com/artist", *resp.Body.Social.Instagram)
	s.NotNil(resp.Body.Social.Website)
	s.Equal("https://artist.com", *resp.Body.Social.Website)
}

func (s *ArtistHandlerIntegrationSuite) TestAdminUpdateArtist_NotFound() {
	admin := createAdminUser(s.deps.db)
	ctx := ctxWithUser(admin)
	newName := "New Name"
	req := &AdminUpdateArtistRequest{ArtistID: "99999"}
	req.Body.Name = &newName

	_, err := s.handler.AdminUpdateArtistHandler(ctx, req)
	assertHumaError(s.T(), err, 404)
}

// --- UpdateArtistBandcampHandler ---

func (s *ArtistHandlerIntegrationSuite) TestUpdateBandcamp_AdminSuccess() {
	admin := createAdminUser(s.deps.db)
	artistID := s.createArtistViaService("Bandcamp Artist")

	ctx := ctxWithUser(admin)
	url := "https://artist.bandcamp.com/album/cool-album"
	req := &UpdateArtistBandcampRequest{ArtistID: fmt.Sprintf("%d", artistID)}
	req.Body.BandcampEmbedURL = &url

	resp, err := s.handler.UpdateArtistBandcampHandler(ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.NotNil(resp.Body.BandcampEmbedURL)
	s.Equal(url, *resp.Body.BandcampEmbedURL)
	// Profile URL should also be set
	s.NotNil(resp.Body.Social.Bandcamp)
	s.Equal("https://artist.bandcamp.com", *resp.Body.Social.Bandcamp)
}

func (s *ArtistHandlerIntegrationSuite) TestUpdateBandcamp_ClearURL() {
	admin := createAdminUser(s.deps.db)
	artistID := s.createArtistViaService("Clearable Artist")

	// First set a bandcamp URL
	ctx := ctxWithUser(admin)
	url := "https://artist.bandcamp.com/album/cool-album"
	setReq := &UpdateArtistBandcampRequest{ArtistID: fmt.Sprintf("%d", artistID)}
	setReq.Body.BandcampEmbedURL = &url
	_, err := s.handler.UpdateArtistBandcampHandler(ctx, setReq)
	s.NoError(err)

	// Now clear it
	empty := ""
	clearReq := &UpdateArtistBandcampRequest{ArtistID: fmt.Sprintf("%d", artistID)}
	clearReq.Body.BandcampEmbedURL = &empty

	resp, err := s.handler.UpdateArtistBandcampHandler(ctx, clearReq)
	s.NoError(err)
	s.NotNil(resp)
	s.Nil(resp.Body.BandcampEmbedURL)
}

func (s *ArtistHandlerIntegrationSuite) TestUpdateBandcamp_NotFound() {
	admin := createAdminUser(s.deps.db)
	ctx := ctxWithUser(admin)
	url := "https://artist.bandcamp.com/album/cool-album"
	req := &UpdateArtistBandcampRequest{ArtistID: "99999"}
	req.Body.BandcampEmbedURL = &url

	_, err := s.handler.UpdateArtistBandcampHandler(ctx, req)
	assertHumaError(s.T(), err, 404)
}

// --- UpdateArtistSpotifyHandler ---

func (s *ArtistHandlerIntegrationSuite) TestUpdateSpotify_AdminSuccess() {
	admin := createAdminUser(s.deps.db)
	artistID := s.createArtistViaService("Spotify Artist")

	ctx := ctxWithUser(admin)
	url := "https://open.spotify.com/artist/abc123"
	req := &UpdateArtistSpotifyRequest{ArtistID: fmt.Sprintf("%d", artistID)}
	req.Body.SpotifyURL = &url

	resp, err := s.handler.UpdateArtistSpotifyHandler(ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.NotNil(resp.Body.Social.Spotify)
	s.Equal(url, *resp.Body.Social.Spotify)
}

func (s *ArtistHandlerIntegrationSuite) TestUpdateSpotify_ClearURL() {
	admin := createAdminUser(s.deps.db)
	artistID := s.createArtistViaService("Spotify Clear Artist")

	ctx := ctxWithUser(admin)
	url := "https://open.spotify.com/artist/abc123"
	setReq := &UpdateArtistSpotifyRequest{ArtistID: fmt.Sprintf("%d", artistID)}
	setReq.Body.SpotifyURL = &url
	_, err := s.handler.UpdateArtistSpotifyHandler(ctx, setReq)
	s.NoError(err)

	// Clear
	empty := ""
	clearReq := &UpdateArtistSpotifyRequest{ArtistID: fmt.Sprintf("%d", artistID)}
	clearReq.Body.SpotifyURL = &empty

	resp, err := s.handler.UpdateArtistSpotifyHandler(ctx, clearReq)
	s.NoError(err)
	s.NotNil(resp)
	s.Nil(resp.Body.Social.Spotify)
}

func (s *ArtistHandlerIntegrationSuite) TestUpdateSpotify_NotFound() {
	admin := createAdminUser(s.deps.db)
	ctx := ctxWithUser(admin)
	url := "https://open.spotify.com/artist/abc123"
	req := &UpdateArtistSpotifyRequest{ArtistID: "99999"}
	req.Body.SpotifyURL = &url

	_, err := s.handler.UpdateArtistSpotifyHandler(ctx, req)
	assertHumaError(s.T(), err, 404)
}
