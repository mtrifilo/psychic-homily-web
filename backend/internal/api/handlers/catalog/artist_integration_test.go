package catalog

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	"psychic-homily-backend/internal/models"
	"psychic-homily-backend/internal/services/contracts"
)

type ArtistHandlerIntegrationSuite struct {
	suite.Suite
	deps    *testhelpers.IntegrationDeps
	handler *ArtistHandler
}

func (s *ArtistHandlerIntegrationSuite) SetupSuite() {
	s.deps = testhelpers.SetupIntegrationDeps(s.T())
	s.handler = NewArtistHandler(s.deps.ArtistService, s.deps.AuditLogService, nil)
}

func (s *ArtistHandlerIntegrationSuite) TearDownTest() {
	testhelpers.CleanupTables(s.deps.DB)
}

func (s *ArtistHandlerIntegrationSuite) TearDownSuite() {
	s.deps.TestDB.Cleanup()
}

func TestArtistHandlerIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	suite.Run(t, new(ArtistHandlerIntegrationSuite))
}

// createArtistWithSlug creates an artist using the service (which generates a slug)
func (s *ArtistHandlerIntegrationSuite) createArtistViaService(name string) uint {
	resp, err := s.deps.ArtistService.CreateArtist(&contracts.CreateArtistRequest{Name: name})
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
	s.deps.DB.Create(artist)
	// Set a slug so slug-based lookups work
	slug := fmt.Sprintf("%s-slug", name)
	s.deps.DB.Model(artist).Update("slug", slug)
	return artist
}

// --- SearchArtistsHandler ---

func (s *ArtistHandlerIntegrationSuite) TestSearchArtists_Success() {
	s.createArtistViaService("Radiohead")
	s.createArtistViaService("Radio Moscow")
	s.createArtistViaService("Unrelated Band")

	req := &SearchArtistsRequest{Query: "radio"}
	resp, err := s.handler.SearchArtistsHandler(s.deps.Ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.GreaterOrEqual(resp.Body.Count, 2)
}

func (s *ArtistHandlerIntegrationSuite) TestSearchArtists_NoResults() {
	s.createArtistViaService("Radiohead")

	req := &SearchArtistsRequest{Query: "zzzznonexistent"}
	resp, err := s.handler.SearchArtistsHandler(s.deps.Ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.Equal(0, resp.Body.Count)
}

// createArtistWithUpcomingShow creates an artist and gives it an upcoming approved show
func (s *ArtistHandlerIntegrationSuite) createArtistWithUpcomingShow(name string) uint {
	artistID := s.createArtistViaService(name)
	user := testhelpers.CreateTestUser(s.deps.DB)
	venue := testhelpers.CreateVerifiedVenue(s.deps.DB, fmt.Sprintf("Venue for %s", name), "Phoenix", "AZ")

	show := &models.Show{
		Title:       fmt.Sprintf("Show for %s", name),
		EventDate:   time.Now().UTC().AddDate(0, 0, 30),
		City:        testhelpers.StringPtr("Phoenix"),
		State:       testhelpers.StringPtr("AZ"),
		Status:      models.ShowStatusApproved,
		SubmittedBy: &user.ID,
	}
	s.deps.DB.Create(show)
	s.deps.DB.Exec("INSERT INTO show_venues (show_id, venue_id) VALUES (?, ?)", show.ID, venue.ID)
	s.deps.DB.Exec("INSERT INTO show_artists (show_id, artist_id, position, set_type) VALUES (?, ?, 0, 'headliner')", show.ID, artistID)
	return artistID
}

// --- ListArtistsHandler ---

func (s *ArtistHandlerIntegrationSuite) TestListArtists_Success() {
	s.createArtistWithUpcomingShow("Artist A")
	s.createArtistWithUpcomingShow("Artist B")
	s.createArtistWithUpcomingShow("Artist C")

	req := &ListArtistsRequest{}
	resp, err := s.handler.ListArtistsHandler(s.deps.Ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.GreaterOrEqual(resp.Body.Count, 3)
	// Should include upcoming show counts
	s.GreaterOrEqual(resp.Body.Artists[0].UpcomingShowCount, 1)
}

func (s *ArtistHandlerIntegrationSuite) TestListArtists_Empty() {
	// Artists without upcoming shows should not appear
	s.createArtistViaService("No Shows Artist")

	req := &ListArtistsRequest{}
	resp, err := s.handler.ListArtistsHandler(s.deps.Ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.Equal(0, resp.Body.Count)
}

func (s *ArtistHandlerIntegrationSuite) TestListArtists_CityFilter() {
	// Create artist with city + upcoming show
	artist := s.createArtistWithCity("Phoenix Band", "Phoenix", "AZ")
	s.createArtistWithCity("Tucson Band", "Tucson", "AZ")

	// Give Phoenix Band an upcoming show
	user := testhelpers.CreateTestUser(s.deps.DB)
	venue := testhelpers.CreateVerifiedVenue(s.deps.DB, "PHX Venue", "Phoenix", "AZ")
	show := &models.Show{
		Title:       "PHX Show",
		EventDate:   time.Now().UTC().AddDate(0, 0, 30),
		City:        testhelpers.StringPtr("Phoenix"),
		State:       testhelpers.StringPtr("AZ"),
		Status:      models.ShowStatusApproved,
		SubmittedBy: &user.ID,
	}
	s.deps.DB.Create(show)
	s.deps.DB.Exec("INSERT INTO show_venues (show_id, venue_id) VALUES (?, ?)", show.ID, venue.ID)
	s.deps.DB.Exec("INSERT INTO show_artists (show_id, artist_id, position, set_type) VALUES (?, ?, 0, 'headliner')", show.ID, artist.ID)

	req := &ListArtistsRequest{City: "Phoenix"}
	resp, err := s.handler.ListArtistsHandler(s.deps.Ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.Equal(1, resp.Body.Count)
	s.Equal("Phoenix Band", resp.Body.Artists[0].Name)
}

// --- ListArtists with Tag Filter (PSY-495) ---

// tagArtist is a low-level helper that tags an existing artist with the given
// tag slug, creating the tag row and entity_tags row if needed. It does NOT
// bump tags.usage_count — PSY-495 tests only exercise the tag-filter query
// path, not the count-rollup trigger.
func (s *ArtistHandlerIntegrationSuite) tagArtist(artistID uint, tagSlug string) {
	// upsert tag
	var tag models.Tag
	if err := s.deps.DB.Where("slug = ?", tagSlug).First(&tag).Error; err != nil {
		tag = models.Tag{
			Name:     tagSlug,
			Slug:     tagSlug,
			Category: models.TagCategoryGenre,
		}
		s.Require().NoError(s.deps.DB.Create(&tag).Error)
	}

	// need an AddedByUserID: grab or create a throwaway user
	adder := testhelpers.CreateTestUser(s.deps.DB)

	entityTag := models.EntityTag{
		TagID:         tag.ID,
		EntityType:    models.TagEntityArtist,
		EntityID:      artistID,
		AddedByUserID: adder.ID,
	}
	s.Require().NoError(s.deps.DB.Create(&entityTag).Error)
}

// TestListArtists_TagFilter_DropsActivityGate is the PSY-495 contract.
// Without a tag filter, /artists gates on "has upcoming shows". With a tag
// filter engaged, we follow the Bandcamp model — every artist tagged `X`
// surfaces, active or not. Facet-chip count parity is what the fix is for.
func (s *ArtistHandlerIntegrationSuite) TestListArtists_TagFilter_DropsActivityGate() {
	// Three artists tagged `punk`: one with an upcoming show, two without
	activeID := s.createArtistWithUpcomingShow("Punk Active Band")
	inactiveID1 := s.createArtistViaService("Punk Dormant One")
	inactiveID2 := s.createArtistViaService("Punk Dormant Two")

	s.tagArtist(activeID, "punk")
	s.tagArtist(inactiveID1, "punk")
	s.tagArtist(inactiveID2, "punk")

	// One artist tagged `rock` to make sure the filter actually narrows
	s.createArtistWithUpcomingShow("Rock Noise")

	// With no tag filter: default activity gate applies → only the active
	// punk band and the rock band show up (2 total)
	unfiltered, err := s.handler.ListArtistsHandler(s.deps.Ctx, &ListArtistsRequest{})
	s.Require().NoError(err)
	s.Equal(2, unfiltered.Body.Count, "unfiltered list should exclude dormant artists")

	// With tags=punk: activity gate drops → all 3 punk-tagged artists
	// surface regardless of upcoming-show status (facet count parity)
	req := &ListArtistsRequest{Tags: "punk"}
	resp, err := s.handler.ListArtistsHandler(s.deps.Ctx, req)
	s.Require().NoError(err)
	s.Equal(3, resp.Body.Count, "tag-filtered list must return all 3 punk artists")

	// Sort order: upcoming_show_count DESC, then name ASC. The active band
	// sits first; the two dormant ones follow in alphabetical order.
	s.Equal("Punk Active Band", resp.Body.Artists[0].Name)
	s.Equal(1, resp.Body.Artists[0].UpcomingShowCount)
	s.Equal("Punk Dormant One", resp.Body.Artists[1].Name)
	s.Equal(0, resp.Body.Artists[1].UpcomingShowCount)
	s.Equal("Punk Dormant Two", resp.Body.Artists[2].Name)
	s.Equal(0, resp.Body.Artists[2].UpcomingShowCount)
}

// TestListArtists_TagFilter_SurfacesLastShowDate verifies that evergreen mode
// populates `last_show_date` for dormant artists so cards can render a
// "no upcoming shows · last show <Mon Year>" affordance.
func (s *ArtistHandlerIntegrationSuite) TestListArtists_TagFilter_SurfacesLastShowDate() {
	dormantID := s.createArtistViaService("Old Shoegaze Band")
	// Give the dormant artist a past approved show 400 days ago
	user := testhelpers.CreateTestUser(s.deps.DB)
	venue := testhelpers.CreateVerifiedVenue(s.deps.DB, "Past Venue", "Phoenix", "AZ")
	pastShow := &models.Show{
		Title:       "Past Gig",
		EventDate:   time.Now().UTC().AddDate(0, 0, -400),
		City:        testhelpers.StringPtr("Phoenix"),
		State:       testhelpers.StringPtr("AZ"),
		Status:      models.ShowStatusApproved,
		SubmittedBy: &user.ID,
	}
	s.deps.DB.Create(pastShow)
	s.deps.DB.Exec("INSERT INTO show_venues (show_id, venue_id) VALUES (?, ?)", pastShow.ID, venue.ID)
	s.deps.DB.Exec("INSERT INTO show_artists (show_id, artist_id, position, set_type) VALUES (?, ?, 0, 'headliner')", pastShow.ID, dormantID)

	s.tagArtist(dormantID, "shoegaze")

	resp, err := s.handler.ListArtistsHandler(s.deps.Ctx, &ListArtistsRequest{Tags: "shoegaze"})
	s.Require().NoError(err)
	s.Equal(1, resp.Body.Count)
	s.Equal("Old Shoegaze Band", resp.Body.Artists[0].Name)
	s.Equal(0, resp.Body.Artists[0].UpcomingShowCount)
	s.Require().NotNil(resp.Body.Artists[0].LastShowDate, "last_show_date should populate for dormant artists in evergreen mode")
}

// --- GetArtistHandler ---

func (s *ArtistHandlerIntegrationSuite) TestGetArtist_ByID() {
	id := s.createArtistViaService("The National")

	req := &GetArtistRequest{ArtistID: fmt.Sprintf("%d", id)}
	resp, err := s.handler.GetArtistHandler(s.deps.Ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.Equal("The National", resp.Body.Name)
}

func (s *ArtistHandlerIntegrationSuite) TestGetArtist_BySlug() {
	s.createArtistViaService("The National")

	req := &GetArtistRequest{ArtistID: "the-national"}
	resp, err := s.handler.GetArtistHandler(s.deps.Ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.Equal("The National", resp.Body.Name)
}

func (s *ArtistHandlerIntegrationSuite) TestGetArtist_NotFound() {
	req := &GetArtistRequest{ArtistID: "99999"}
	_, err := s.handler.GetArtistHandler(s.deps.Ctx, req)
	testhelpers.AssertHumaError(s.T(), err, 404)
}

// --- GetArtistShowsHandler ---

func (s *ArtistHandlerIntegrationSuite) TestGetArtistShows_Success() {
	user := testhelpers.CreateTestUser(s.deps.DB)
	venue := testhelpers.CreateVerifiedVenue(s.deps.DB, "Test Venue", "Phoenix", "AZ")

	artistID := s.createArtistViaService("Show Artist")

	// Create an approved show and associate the artist
	show := &models.Show{
		Title:       "Future Gig",
		EventDate:   time.Now().UTC().AddDate(0, 0, 30),
		City:        testhelpers.StringPtr("Phoenix"),
		State:       testhelpers.StringPtr("AZ"),
		Status:      models.ShowStatusApproved,
		SubmittedBy: &user.ID,
	}
	s.deps.DB.Create(show)
	s.deps.DB.Exec("INSERT INTO show_venues (show_id, venue_id) VALUES (?, ?)", show.ID, venue.ID)
	s.deps.DB.Exec("INSERT INTO show_artists (show_id, artist_id, position, set_type) VALUES (?, ?, 0, 'headliner')", show.ID, artistID)

	req := &GetArtistShowsRequest{ArtistID: fmt.Sprintf("%d", artistID), TimeFilter: "upcoming"}
	resp, err := s.handler.GetArtistShowsHandler(s.deps.Ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.Equal(int64(1), resp.Body.Total)
	s.Equal("Future Gig", resp.Body.Shows[0].Title)
}

func (s *ArtistHandlerIntegrationSuite) TestGetArtistShows_Empty() {
	artistID := s.createArtistViaService("Lonely Artist")

	req := &GetArtistShowsRequest{ArtistID: fmt.Sprintf("%d", artistID), TimeFilter: "upcoming"}
	resp, err := s.handler.GetArtistShowsHandler(s.deps.Ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.Equal(int64(0), resp.Body.Total)
}

func (s *ArtistHandlerIntegrationSuite) TestGetArtistShows_BySlug() {
	user := testhelpers.CreateTestUser(s.deps.DB)
	venue := testhelpers.CreateVerifiedVenue(s.deps.DB, "Slug Venue", "Phoenix", "AZ")

	artistID := s.createArtistViaService("Slug Artist")

	show := &models.Show{
		Title:       "Slug Show",
		EventDate:   time.Now().UTC().AddDate(0, 0, 30),
		City:        testhelpers.StringPtr("Phoenix"),
		State:       testhelpers.StringPtr("AZ"),
		Status:      models.ShowStatusApproved,
		SubmittedBy: &user.ID,
	}
	s.deps.DB.Create(show)
	s.deps.DB.Exec("INSERT INTO show_venues (show_id, venue_id) VALUES (?, ?)", show.ID, venue.ID)
	s.deps.DB.Exec("INSERT INTO show_artists (show_id, artist_id, position, set_type) VALUES (?, ?, 0, 'headliner')", show.ID, artistID)

	req := &GetArtistShowsRequest{ArtistID: "slug-artist", TimeFilter: "upcoming"}
	resp, err := s.handler.GetArtistShowsHandler(s.deps.Ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.Equal(int64(1), resp.Body.Total)
}

func (s *ArtistHandlerIntegrationSuite) TestGetArtistShows_NotFound() {
	req := &GetArtistShowsRequest{ArtistID: "99999", TimeFilter: "upcoming"}
	_, err := s.handler.GetArtistShowsHandler(s.deps.Ctx, req)
	testhelpers.AssertHumaError(s.T(), err, 404)
}

// --- DeleteArtistHandler ---

func (s *ArtistHandlerIntegrationSuite) TestDeleteArtist_Success() {
	user := testhelpers.CreateTestUser(s.deps.DB)
	artistID := s.createArtistViaService("Deletable Artist")

	ctx := testhelpers.CtxWithUser(user)
	req := &DeleteArtistRequest{ArtistID: fmt.Sprintf("%d", artistID)}
	_, err := s.handler.DeleteArtistHandler(ctx, req)
	s.NoError(err)

	// Verify artist is gone
	getReq := &GetArtistRequest{ArtistID: fmt.Sprintf("%d", artistID)}
	_, err = s.handler.GetArtistHandler(s.deps.Ctx, getReq)
	testhelpers.AssertHumaError(s.T(), err, 404)
}

func (s *ArtistHandlerIntegrationSuite) TestDeleteArtist_HasShows() {
	user := testhelpers.CreateTestUser(s.deps.DB)
	venue := testhelpers.CreateVerifiedVenue(s.deps.DB, "Show Venue", "Phoenix", "AZ")
	artistID := s.createArtistViaService("Busy Artist")

	show := &models.Show{
		Title:       "Active Show",
		EventDate:   time.Now().UTC().AddDate(0, 0, 30),
		City:        testhelpers.StringPtr("Phoenix"),
		State:       testhelpers.StringPtr("AZ"),
		Status:      models.ShowStatusApproved,
		SubmittedBy: &user.ID,
	}
	s.deps.DB.Create(show)
	s.deps.DB.Exec("INSERT INTO show_venues (show_id, venue_id) VALUES (?, ?)", show.ID, venue.ID)
	s.deps.DB.Exec("INSERT INTO show_artists (show_id, artist_id, position, set_type) VALUES (?, ?, 0, 'headliner')", show.ID, artistID)

	ctx := testhelpers.CtxWithUser(user)
	req := &DeleteArtistRequest{ArtistID: fmt.Sprintf("%d", artistID)}
	_, err := s.handler.DeleteArtistHandler(ctx, req)
	testhelpers.AssertHumaError(s.T(), err, 409)
}

func (s *ArtistHandlerIntegrationSuite) TestDeleteArtist_NotFound() {
	user := testhelpers.CreateTestUser(s.deps.DB)
	ctx := testhelpers.CtxWithUser(user)
	req := &DeleteArtistRequest{ArtistID: "99999"}

	_, err := s.handler.DeleteArtistHandler(ctx, req)
	testhelpers.AssertHumaError(s.T(), err, 404)
}

// --- AdminUpdateArtistHandler ---

func (s *ArtistHandlerIntegrationSuite) TestAdminUpdateArtist_Success() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	artistID := s.createArtistViaService("Original Name")

	ctx := testhelpers.CtxWithUser(admin)
	newName := "Updated Name"
	req := &AdminUpdateArtistRequest{ArtistID: fmt.Sprintf("%d", artistID)}
	req.Body.Name = &newName

	resp, err := s.handler.AdminUpdateArtistHandler(ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.Equal("Updated Name", resp.Body.Name)
}

func (s *ArtistHandlerIntegrationSuite) TestAdminUpdateArtist_SocialLinks() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	artistID := s.createArtistViaService("Social Artist")

	ctx := testhelpers.CtxWithUser(admin)
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
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	ctx := testhelpers.CtxWithUser(admin)
	newName := "New Name"
	req := &AdminUpdateArtistRequest{ArtistID: "99999"}
	req.Body.Name = &newName

	_, err := s.handler.AdminUpdateArtistHandler(ctx, req)
	testhelpers.AssertHumaError(s.T(), err, 404)
}

// --- UpdateArtistBandcampHandler ---

func (s *ArtistHandlerIntegrationSuite) TestUpdateBandcamp_AdminSuccess() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	artistID := s.createArtistViaService("Bandcamp Artist")

	ctx := testhelpers.CtxWithUser(admin)
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
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	artistID := s.createArtistViaService("Clearable Artist")

	// First set a bandcamp URL
	ctx := testhelpers.CtxWithUser(admin)
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
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	ctx := testhelpers.CtxWithUser(admin)
	url := "https://artist.bandcamp.com/album/cool-album"
	req := &UpdateArtistBandcampRequest{ArtistID: "99999"}
	req.Body.BandcampEmbedURL = &url

	_, err := s.handler.UpdateArtistBandcampHandler(ctx, req)
	testhelpers.AssertHumaError(s.T(), err, 404)
}

// --- UpdateArtistSpotifyHandler ---

func (s *ArtistHandlerIntegrationSuite) TestUpdateSpotify_AdminSuccess() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	artistID := s.createArtistViaService("Spotify Artist")

	ctx := testhelpers.CtxWithUser(admin)
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
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	artistID := s.createArtistViaService("Spotify Clear Artist")

	ctx := testhelpers.CtxWithUser(admin)
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
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	ctx := testhelpers.CtxWithUser(admin)
	url := "https://open.spotify.com/artist/abc123"
	req := &UpdateArtistSpotifyRequest{ArtistID: "99999"}
	req.Body.SpotifyURL = &url

	_, err := s.handler.UpdateArtistSpotifyHandler(ctx, req)
	testhelpers.AssertHumaError(s.T(), err, 404)
}
