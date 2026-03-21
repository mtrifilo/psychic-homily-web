package handlers

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/suite"

	"psychic-homily-backend/internal/services"
)

type ReleaseHandlerIntegrationSuite struct {
	suite.Suite
	deps    *handlerIntegrationDeps
	handler *ReleaseHandler
}

func (s *ReleaseHandlerIntegrationSuite) SetupSuite() {
	s.deps = setupHandlerIntegrationDeps(s.T())
	s.handler = NewReleaseHandler(s.deps.releaseService, s.deps.artistService, s.deps.auditLogService)
}

func (s *ReleaseHandlerIntegrationSuite) TearDownTest() {
	cleanupTables(s.deps.db)
}

func (s *ReleaseHandlerIntegrationSuite) TearDownSuite() {
	s.deps.testDB.Cleanup()
}

func TestReleaseHandlerIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	suite.Run(t, new(ReleaseHandlerIntegrationSuite))
}

// --- Helpers ---

func (s *ReleaseHandlerIntegrationSuite) createReleaseViaService(title string) *services.ReleaseDetailResponse {
	resp, err := s.deps.releaseService.CreateRelease(&services.CreateReleaseRequest{Title: title})
	s.Require().NoError(err)
	return resp
}

func (s *ReleaseHandlerIntegrationSuite) createArtistViaService(name string) uint {
	resp, err := s.deps.artistService.CreateArtist(&services.CreateArtistRequest{Name: name})
	s.Require().NoError(err)
	return resp.ID
}

// --- ListReleasesHandler ---

func (s *ReleaseHandlerIntegrationSuite) TestListReleases_Success() {
	s.createReleaseViaService("Album A")
	s.createReleaseViaService("Album B")
	s.createReleaseViaService("Album C")

	req := &ListReleasesRequest{}
	resp, err := s.handler.ListReleasesHandler(s.deps.ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.GreaterOrEqual(resp.Body.Count, 3)
}

func (s *ReleaseHandlerIntegrationSuite) TestListReleases_Empty() {
	req := &ListReleasesRequest{}
	resp, err := s.handler.ListReleasesHandler(s.deps.ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.Equal(0, resp.Body.Count)
}

func (s *ReleaseHandlerIntegrationSuite) TestListReleases_FilterByType() {
	s.deps.releaseService.CreateRelease(&services.CreateReleaseRequest{Title: "LP Album", ReleaseType: "lp"})
	s.deps.releaseService.CreateRelease(&services.CreateReleaseRequest{Title: "EP Release", ReleaseType: "ep"})

	req := &ListReleasesRequest{ReleaseType: "ep"}
	resp, err := s.handler.ListReleasesHandler(s.deps.ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.Equal(1, resp.Body.Count)
	s.Equal("EP Release", resp.Body.Releases[0].Title)
}

// --- GetReleaseHandler ---

func (s *ReleaseHandlerIntegrationSuite) TestGetRelease_ByID() {
	release := s.createReleaseViaService("Test Album")

	req := &GetReleaseRequest{ReleaseID: fmt.Sprintf("%d", release.ID)}
	resp, err := s.handler.GetReleaseHandler(s.deps.ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.Equal("Test Album", resp.Body.Title)
}

func (s *ReleaseHandlerIntegrationSuite) TestGetRelease_BySlug() {
	s.createReleaseViaService("Slug Album")

	req := &GetReleaseRequest{ReleaseID: "slug-album"}
	resp, err := s.handler.GetReleaseHandler(s.deps.ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.Equal("Slug Album", resp.Body.Title)
}

func (s *ReleaseHandlerIntegrationSuite) TestGetRelease_NotFound() {
	req := &GetReleaseRequest{ReleaseID: "99999"}
	_, err := s.handler.GetReleaseHandler(s.deps.ctx, req)
	assertHumaError(s.T(), err, 404)
}

// --- CreateReleaseHandler ---

func (s *ReleaseHandlerIntegrationSuite) TestCreateRelease_AdminSuccess() {
	admin := createAdminUser(s.deps.db)
	ctx := ctxWithUser(admin)

	year := 2024
	req := &CreateReleaseRequest{}
	req.Body.Title = "New Album"
	req.Body.ReleaseType = "lp"
	req.Body.ReleaseYear = &year

	resp, err := s.handler.CreateReleaseHandler(ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.Equal("New Album", resp.Body.Title)
	s.Equal("lp", resp.Body.ReleaseType)
	s.Equal(2024, *resp.Body.ReleaseYear)
}

func (s *ReleaseHandlerIntegrationSuite) TestCreateRelease_WithArtists() {
	admin := createAdminUser(s.deps.db)
	ctx := ctxWithUser(admin)

	artistID := s.createArtistViaService("Test Artist")

	req := &CreateReleaseRequest{}
	req.Body.Title = "Artist Album"
	req.Body.Artists = []CreateReleaseArtistInput{
		{ArtistID: artistID, Role: "main"},
	}

	resp, err := s.handler.CreateReleaseHandler(ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.Len(resp.Body.Artists, 1)
	s.Equal("Test Artist", resp.Body.Artists[0].Name)
}

func (s *ReleaseHandlerIntegrationSuite) TestCreateRelease_NonAdminForbidden() {
	user := createTestUser(s.deps.db)
	ctx := ctxWithUser(user)

	req := &CreateReleaseRequest{}
	req.Body.Title = "Forbidden Album"

	_, err := s.handler.CreateReleaseHandler(ctx, req)
	assertHumaError(s.T(), err, 403)
}

func (s *ReleaseHandlerIntegrationSuite) TestCreateRelease_NoAuth() {
	req := &CreateReleaseRequest{}
	req.Body.Title = "No Auth Album"

	_, err := s.handler.CreateReleaseHandler(s.deps.ctx, req)
	assertHumaError(s.T(), err, 403)
}

func (s *ReleaseHandlerIntegrationSuite) TestCreateRelease_EmptyTitle() {
	admin := createAdminUser(s.deps.db)
	ctx := ctxWithUser(admin)

	req := &CreateReleaseRequest{}
	req.Body.Title = ""

	_, err := s.handler.CreateReleaseHandler(ctx, req)
	assertHumaError(s.T(), err, 400)
}

// --- UpdateReleaseHandler ---

func (s *ReleaseHandlerIntegrationSuite) TestUpdateRelease_Success() {
	admin := createAdminUser(s.deps.db)
	release := s.createReleaseViaService("Original Title")

	ctx := ctxWithUser(admin)
	newTitle := "Updated Title"
	req := &UpdateReleaseRequest{ReleaseID: fmt.Sprintf("%d", release.ID)}
	req.Body.Title = &newTitle

	resp, err := s.handler.UpdateReleaseHandler(ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.Equal("Updated Title", resp.Body.Title)
}

func (s *ReleaseHandlerIntegrationSuite) TestUpdateRelease_BySlug() {
	admin := createAdminUser(s.deps.db)
	s.createReleaseViaService("Slug Update Album")

	ctx := ctxWithUser(admin)
	newType := "ep"
	req := &UpdateReleaseRequest{ReleaseID: "slug-update-album"}
	req.Body.ReleaseType = &newType

	resp, err := s.handler.UpdateReleaseHandler(ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.Equal("ep", resp.Body.ReleaseType)
}

func (s *ReleaseHandlerIntegrationSuite) TestUpdateRelease_NotFound() {
	admin := createAdminUser(s.deps.db)
	ctx := ctxWithUser(admin)
	newTitle := "New Title"
	req := &UpdateReleaseRequest{ReleaseID: "99999"}
	req.Body.Title = &newTitle

	_, err := s.handler.UpdateReleaseHandler(ctx, req)
	assertHumaError(s.T(), err, 404)
}

func (s *ReleaseHandlerIntegrationSuite) TestUpdateRelease_NonAdminForbidden() {
	user := createTestUser(s.deps.db)
	release := s.createReleaseViaService("Forbidden Update")

	ctx := ctxWithUser(user)
	newTitle := "Hacked Title"
	req := &UpdateReleaseRequest{ReleaseID: fmt.Sprintf("%d", release.ID)}
	req.Body.Title = &newTitle

	_, err := s.handler.UpdateReleaseHandler(ctx, req)
	assertHumaError(s.T(), err, 403)
}

// --- DeleteReleaseHandler ---

func (s *ReleaseHandlerIntegrationSuite) TestDeleteRelease_Success() {
	admin := createAdminUser(s.deps.db)
	release := s.createReleaseViaService("Deletable Album")

	ctx := ctxWithUser(admin)
	req := &DeleteReleaseRequest{ReleaseID: fmt.Sprintf("%d", release.ID)}
	_, err := s.handler.DeleteReleaseHandler(ctx, req)
	s.NoError(err)

	// Verify release is gone
	getReq := &GetReleaseRequest{ReleaseID: fmt.Sprintf("%d", release.ID)}
	_, err = s.handler.GetReleaseHandler(s.deps.ctx, getReq)
	assertHumaError(s.T(), err, 404)
}

func (s *ReleaseHandlerIntegrationSuite) TestDeleteRelease_NotFound() {
	admin := createAdminUser(s.deps.db)
	ctx := ctxWithUser(admin)
	req := &DeleteReleaseRequest{ReleaseID: "99999"}

	_, err := s.handler.DeleteReleaseHandler(ctx, req)
	assertHumaError(s.T(), err, 404)
}

func (s *ReleaseHandlerIntegrationSuite) TestDeleteRelease_NonAdminForbidden() {
	user := createTestUser(s.deps.db)
	release := s.createReleaseViaService("Protected Album")

	ctx := ctxWithUser(user)
	req := &DeleteReleaseRequest{ReleaseID: fmt.Sprintf("%d", release.ID)}

	_, err := s.handler.DeleteReleaseHandler(ctx, req)
	assertHumaError(s.T(), err, 403)
}

// --- GetArtistReleasesHandler ---

func (s *ReleaseHandlerIntegrationSuite) TestGetArtistReleases_Success() {
	artistID := s.createArtistViaService("Discography Artist")

	s.deps.releaseService.CreateRelease(&services.CreateReleaseRequest{
		Title:   "Album One",
		Artists: []services.CreateReleaseArtistEntry{{ArtistID: artistID, Role: "main"}},
	})
	s.deps.releaseService.CreateRelease(&services.CreateReleaseRequest{
		Title:   "Album Two",
		Artists: []services.CreateReleaseArtistEntry{{ArtistID: artistID, Role: "featured"}},
	})

	req := &GetArtistReleasesRequest{ArtistID: fmt.Sprintf("%d", artistID)}
	resp, err := s.handler.GetArtistReleasesHandler(s.deps.ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.Equal(2, resp.Body.Count)

	// Verify roles are present in the response
	roleMap := make(map[string]string)
	for _, r := range resp.Body.Releases {
		roleMap[r.Title] = r.Role
	}
	s.Equal("main", roleMap["Album One"])
	s.Equal("featured", roleMap["Album Two"])
}

func (s *ReleaseHandlerIntegrationSuite) TestGetArtistReleases_BySlug() {
	artistID := s.createArtistViaService("Slug Discography Artist")

	s.deps.releaseService.CreateRelease(&services.CreateReleaseRequest{
		Title:   "Slug Album",
		Artists: []services.CreateReleaseArtistEntry{{ArtistID: artistID, Role: "main"}},
	})

	req := &GetArtistReleasesRequest{ArtistID: "slug-discography-artist"}
	resp, err := s.handler.GetArtistReleasesHandler(s.deps.ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.Equal(1, resp.Body.Count)
}

func (s *ReleaseHandlerIntegrationSuite) TestGetArtistReleases_ArtistNotFound() {
	req := &GetArtistReleasesRequest{ArtistID: "99999"}
	_, err := s.handler.GetArtistReleasesHandler(s.deps.ctx, req)
	assertHumaError(s.T(), err, 404)
}

func (s *ReleaseHandlerIntegrationSuite) TestGetArtistReleases_Empty() {
	artistID := s.createArtistViaService("Empty Discography")

	req := &GetArtistReleasesRequest{ArtistID: fmt.Sprintf("%d", artistID)}
	resp, err := s.handler.GetArtistReleasesHandler(s.deps.ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.Equal(0, resp.Body.Count)
}

// --- AddExternalLinkHandler ---

func (s *ReleaseHandlerIntegrationSuite) TestAddExternalLink_Success() {
	admin := createAdminUser(s.deps.db)
	release := s.createReleaseViaService("Link Album")

	ctx := ctxWithUser(admin)
	req := &AddExternalLinkRequest{ReleaseID: fmt.Sprintf("%d", release.ID)}
	req.Body.Platform = "bandcamp"
	req.Body.URL = "https://test.bandcamp.com/album/test"

	resp, err := s.handler.AddExternalLinkHandler(ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.Equal("bandcamp", resp.Body.Platform)
	s.Equal("https://test.bandcamp.com/album/test", resp.Body.URL)
}

func (s *ReleaseHandlerIntegrationSuite) TestAddExternalLink_ReleaseNotFound() {
	admin := createAdminUser(s.deps.db)
	ctx := ctxWithUser(admin)
	req := &AddExternalLinkRequest{ReleaseID: "99999"}
	req.Body.Platform = "bandcamp"
	req.Body.URL = "https://test.bandcamp.com"

	_, err := s.handler.AddExternalLinkHandler(ctx, req)
	assertHumaError(s.T(), err, 404)
}

func (s *ReleaseHandlerIntegrationSuite) TestAddExternalLink_NonAdminForbidden() {
	user := createTestUser(s.deps.db)
	release := s.createReleaseViaService("Protected Link Album")

	ctx := ctxWithUser(user)
	req := &AddExternalLinkRequest{ReleaseID: fmt.Sprintf("%d", release.ID)}
	req.Body.Platform = "bandcamp"
	req.Body.URL = "https://test.bandcamp.com"

	_, err := s.handler.AddExternalLinkHandler(ctx, req)
	assertHumaError(s.T(), err, 403)
}

// --- RemoveExternalLinkHandler ---

func (s *ReleaseHandlerIntegrationSuite) TestRemoveExternalLink_Success() {
	admin := createAdminUser(s.deps.db)
	release, err := s.deps.releaseService.CreateRelease(&services.CreateReleaseRequest{
		Title: "Remove Link Album",
		ExternalLinks: []services.CreateReleaseLinkEntry{
			{Platform: "spotify", URL: "https://open.spotify.com/album/abc"},
		},
	})
	s.Require().NoError(err)
	s.Require().Len(release.ExternalLinks, 1)

	ctx := ctxWithUser(admin)
	req := &RemoveExternalLinkRequest{
		ReleaseID: fmt.Sprintf("%d", release.ID),
		LinkID:    fmt.Sprintf("%d", release.ExternalLinks[0].ID),
	}

	_, err = s.handler.RemoveExternalLinkHandler(ctx, req)
	s.NoError(err)

	// Verify link is gone
	refreshed, err := s.deps.releaseService.GetRelease(release.ID)
	s.Require().NoError(err)
	s.Empty(refreshed.ExternalLinks)
}

func (s *ReleaseHandlerIntegrationSuite) TestRemoveExternalLink_NonAdminForbidden() {
	user := createTestUser(s.deps.db)
	release, err := s.deps.releaseService.CreateRelease(&services.CreateReleaseRequest{
		Title: "Protected Remove Link",
		ExternalLinks: []services.CreateReleaseLinkEntry{
			{Platform: "spotify", URL: "https://open.spotify.com/album/abc"},
		},
	})
	s.Require().NoError(err)

	ctx := ctxWithUser(user)
	req := &RemoveExternalLinkRequest{
		ReleaseID: fmt.Sprintf("%d", release.ID),
		LinkID:    fmt.Sprintf("%d", release.ExternalLinks[0].ID),
	}

	_, err = s.handler.RemoveExternalLinkHandler(ctx, req)
	assertHumaError(s.T(), err, 403)
}
