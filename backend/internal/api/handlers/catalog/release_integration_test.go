package catalog

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/suite"

	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	"psychic-homily-backend/internal/services/contracts"
)

type ReleaseHandlerIntegrationSuite struct {
	suite.Suite
	deps    *testhelpers.IntegrationDeps
	handler *ReleaseHandler
}

func (s *ReleaseHandlerIntegrationSuite) SetupSuite() {
	s.deps = testhelpers.SetupIntegrationDeps(s.T())
	s.handler = NewReleaseHandler(s.deps.ReleaseService, s.deps.ArtistService, s.deps.AuditLogService, nil)
}

func (s *ReleaseHandlerIntegrationSuite) TearDownTest() {
	testhelpers.CleanupTables(s.deps.DB)
}

func (s *ReleaseHandlerIntegrationSuite) TearDownSuite() {
	s.deps.TestDB.Cleanup()
}

func TestReleaseHandlerIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	suite.Run(t, new(ReleaseHandlerIntegrationSuite))
}

// --- Helpers ---

func (s *ReleaseHandlerIntegrationSuite) createReleaseViaService(title string) *contracts.ReleaseDetailResponse {
	resp, err := s.deps.ReleaseService.CreateRelease(&contracts.CreateReleaseRequest{Title: title})
	s.Require().NoError(err)
	return resp
}

func (s *ReleaseHandlerIntegrationSuite) createArtistViaService(name string) uint {
	resp, err := s.deps.ArtistService.CreateArtist(&contracts.CreateArtistRequest{Name: name})
	s.Require().NoError(err)
	return resp.ID
}

// --- ListReleasesHandler ---

func (s *ReleaseHandlerIntegrationSuite) TestListReleases_Success() {
	s.createReleaseViaService("Album A")
	s.createReleaseViaService("Album B")
	s.createReleaseViaService("Album C")

	req := &ListReleasesRequest{}
	resp, err := s.handler.ListReleasesHandler(s.deps.Ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.GreaterOrEqual(resp.Body.Total, int64(3))
}

func (s *ReleaseHandlerIntegrationSuite) TestListReleases_Empty() {
	req := &ListReleasesRequest{}
	resp, err := s.handler.ListReleasesHandler(s.deps.Ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.Equal(int64(0), resp.Body.Total)
}

func (s *ReleaseHandlerIntegrationSuite) TestListReleases_FilterByType() {
	_, err := s.deps.ReleaseService.CreateRelease(&contracts.CreateReleaseRequest{Title: "LP Album", ReleaseType: "lp"})
	s.Require().NoError(err)
	_, err = s.deps.ReleaseService.CreateRelease(&contracts.CreateReleaseRequest{Title: "EP Release", ReleaseType: "ep"})
	s.Require().NoError(err)

	req := &ListReleasesRequest{ReleaseType: "ep"}
	resp, err := s.handler.ListReleasesHandler(s.deps.Ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.Equal(int64(1), resp.Body.Total)
	s.Equal("EP Release", resp.Body.Releases[0].Title)
}

// --- GetReleaseHandler ---

func (s *ReleaseHandlerIntegrationSuite) TestGetRelease_ByID() {
	release := s.createReleaseViaService("Test Album")

	req := &GetReleaseRequest{ReleaseID: fmt.Sprintf("%d", release.ID)}
	resp, err := s.handler.GetReleaseHandler(s.deps.Ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.Equal("Test Album", resp.Body.Title)
}

func (s *ReleaseHandlerIntegrationSuite) TestGetRelease_BySlug() {
	s.createReleaseViaService("Slug Album")

	req := &GetReleaseRequest{ReleaseID: "slug-album"}
	resp, err := s.handler.GetReleaseHandler(s.deps.Ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.Equal("Slug Album", resp.Body.Title)
}

func (s *ReleaseHandlerIntegrationSuite) TestGetRelease_NotFound() {
	req := &GetReleaseRequest{ReleaseID: "99999"}
	_, err := s.handler.GetReleaseHandler(s.deps.Ctx, req)
	testhelpers.AssertHumaError(s.T(), err, 404)
}

// --- CreateReleaseHandler ---

func (s *ReleaseHandlerIntegrationSuite) TestCreateRelease_AdminSuccess() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	ctx := testhelpers.CtxWithUser(admin)

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
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	ctx := testhelpers.CtxWithUser(admin)

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

func (s *ReleaseHandlerIntegrationSuite) TestCreateRelease_EmptyTitle() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	ctx := testhelpers.CtxWithUser(admin)

	req := &CreateReleaseRequest{}
	req.Body.Title = ""

	_, err := s.handler.CreateReleaseHandler(ctx, req)
	testhelpers.AssertHumaError(s.T(), err, 422)
}

// --- UpdateReleaseHandler ---

func (s *ReleaseHandlerIntegrationSuite) TestUpdateRelease_Success() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	release := s.createReleaseViaService("Original Title")

	ctx := testhelpers.CtxWithUser(admin)
	newTitle := "Updated Title"
	req := &UpdateReleaseRequest{ReleaseID: fmt.Sprintf("%d", release.ID)}
	req.Body.Title = &newTitle

	resp, err := s.handler.UpdateReleaseHandler(ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.Equal("Updated Title", resp.Body.Title)
}

func (s *ReleaseHandlerIntegrationSuite) TestUpdateRelease_BySlug() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	s.createReleaseViaService("Slug Update Album")

	ctx := testhelpers.CtxWithUser(admin)
	newType := "ep"
	req := &UpdateReleaseRequest{ReleaseID: "slug-update-album"}
	req.Body.ReleaseType = &newType

	resp, err := s.handler.UpdateReleaseHandler(ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.Equal("ep", resp.Body.ReleaseType)
}

func (s *ReleaseHandlerIntegrationSuite) TestUpdateRelease_NotFound() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	ctx := testhelpers.CtxWithUser(admin)
	newTitle := "New Title"
	req := &UpdateReleaseRequest{ReleaseID: "99999"}
	req.Body.Title = &newTitle

	_, err := s.handler.UpdateReleaseHandler(ctx, req)
	testhelpers.AssertHumaError(s.T(), err, 404)
}

// --- DeleteReleaseHandler ---

func (s *ReleaseHandlerIntegrationSuite) TestDeleteRelease_Success() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	release := s.createReleaseViaService("Deletable Album")

	ctx := testhelpers.CtxWithUser(admin)
	req := &DeleteReleaseRequest{ReleaseID: fmt.Sprintf("%d", release.ID)}
	_, err := s.handler.DeleteReleaseHandler(ctx, req)
	s.NoError(err)

	// Verify release is gone
	getReq := &GetReleaseRequest{ReleaseID: fmt.Sprintf("%d", release.ID)}
	_, err = s.handler.GetReleaseHandler(s.deps.Ctx, getReq)
	testhelpers.AssertHumaError(s.T(), err, 404)
}

func (s *ReleaseHandlerIntegrationSuite) TestDeleteRelease_NotFound() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	ctx := testhelpers.CtxWithUser(admin)
	req := &DeleteReleaseRequest{ReleaseID: "99999"}

	_, err := s.handler.DeleteReleaseHandler(ctx, req)
	testhelpers.AssertHumaError(s.T(), err, 404)
}

// --- GetArtistReleasesHandler ---

func (s *ReleaseHandlerIntegrationSuite) TestGetArtistReleases_Success() {
	artistID := s.createArtistViaService("Discography Artist")

	_, err := s.deps.ReleaseService.CreateRelease(&contracts.CreateReleaseRequest{
		Title:   "Album One",
		Artists: []contracts.CreateReleaseArtistEntry{{ArtistID: artistID, Role: "main"}},
	})
	s.Require().NoError(err)
	_, err = s.deps.ReleaseService.CreateRelease(&contracts.CreateReleaseRequest{
		Title:   "Album Two",
		Artists: []contracts.CreateReleaseArtistEntry{{ArtistID: artistID, Role: "featured"}},
	})
	s.Require().NoError(err)

	req := &GetArtistReleasesRequest{ArtistID: fmt.Sprintf("%d", artistID)}
	resp, err := s.handler.GetArtistReleasesHandler(s.deps.Ctx, req)
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

	_, err := s.deps.ReleaseService.CreateRelease(&contracts.CreateReleaseRequest{
		Title:   "Slug Album",
		Artists: []contracts.CreateReleaseArtistEntry{{ArtistID: artistID, Role: "main"}},
	})
	s.Require().NoError(err)

	req := &GetArtistReleasesRequest{ArtistID: "slug-discography-artist"}
	resp, err := s.handler.GetArtistReleasesHandler(s.deps.Ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.Equal(1, resp.Body.Count)
}

func (s *ReleaseHandlerIntegrationSuite) TestGetArtistReleases_ArtistNotFound() {
	req := &GetArtistReleasesRequest{ArtistID: "99999"}
	_, err := s.handler.GetArtistReleasesHandler(s.deps.Ctx, req)
	testhelpers.AssertHumaError(s.T(), err, 404)
}

func (s *ReleaseHandlerIntegrationSuite) TestGetArtistReleases_Empty() {
	artistID := s.createArtistViaService("Empty Discography")

	req := &GetArtistReleasesRequest{ArtistID: fmt.Sprintf("%d", artistID)}
	resp, err := s.handler.GetArtistReleasesHandler(s.deps.Ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.Equal(0, resp.Body.Count)
}

// --- AddExternalLinkHandler ---

func (s *ReleaseHandlerIntegrationSuite) TestAddExternalLink_Success() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	release := s.createReleaseViaService("Link Album")

	ctx := testhelpers.CtxWithUser(admin)
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
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	ctx := testhelpers.CtxWithUser(admin)
	req := &AddExternalLinkRequest{ReleaseID: "99999"}
	req.Body.Platform = "bandcamp"
	req.Body.URL = "https://test.bandcamp.com"

	_, err := s.handler.AddExternalLinkHandler(ctx, req)
	testhelpers.AssertHumaError(s.T(), err, 404)
}

// --- AddExternalLinkHandler authorization matrix (PSY-660) ---
//
// The POST /releases/{id}/links route lives on rc.Protected (JWT only), so the
// handler authorizes the tier itself. Admin, trusted_contributor, and
// local_ambassador may add links; new_user and contributor (and any
// unauthenticated request) must be rejected. TestAddExternalLink_Success above
// covers the admin case.

func (s *ReleaseHandlerIntegrationSuite) TestAddExternalLink_TrustedContributorSuccess() {
	user := testhelpers.CreateUserWithTier(s.deps.DB, "trusted_contributor")
	release := s.createReleaseViaService("Trusted Link Album")

	ctx := testhelpers.CtxWithUser(user)
	req := &AddExternalLinkRequest{ReleaseID: fmt.Sprintf("%d", release.ID)}
	req.Body.Platform = "bandcamp"
	req.Body.URL = "https://trusted.bandcamp.com/album/test"

	resp, err := s.handler.AddExternalLinkHandler(ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.Equal("bandcamp", resp.Body.Platform)
}

func (s *ReleaseHandlerIntegrationSuite) TestAddExternalLink_LocalAmbassadorSuccess() {
	user := testhelpers.CreateUserWithTier(s.deps.DB, "local_ambassador")
	release := s.createReleaseViaService("Ambassador Link Album")

	ctx := testhelpers.CtxWithUser(user)
	req := &AddExternalLinkRequest{ReleaseID: fmt.Sprintf("%d", release.ID)}
	req.Body.Platform = "spotify"
	req.Body.URL = "https://open.spotify.com/album/abc"

	resp, err := s.handler.AddExternalLinkHandler(ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.Equal("spotify", resp.Body.Platform)
}

func (s *ReleaseHandlerIntegrationSuite) TestAddExternalLink_NewUserForbidden() {
	user := testhelpers.CreateUserWithTier(s.deps.DB, "new_user")
	release := s.createReleaseViaService("Forbidden Link Album")

	ctx := testhelpers.CtxWithUser(user)
	req := &AddExternalLinkRequest{ReleaseID: fmt.Sprintf("%d", release.ID)}
	req.Body.Platform = "bandcamp"
	req.Body.URL = "https://new.bandcamp.com/album/test"

	_, err := s.handler.AddExternalLinkHandler(ctx, req)
	testhelpers.AssertHumaError(s.T(), err, 403)
}

// TestAddExternalLink_ContributorForbidden guards the lower-trust boundary: a
// plain "contributor" (one tier below trusted_contributor) must NOT be able to
// add links. Without this, accidentally widening the gate to include
// contributor would slip through unnoticed.
func (s *ReleaseHandlerIntegrationSuite) TestAddExternalLink_ContributorForbidden() {
	user := testhelpers.CreateUserWithTier(s.deps.DB, "contributor")
	release := s.createReleaseViaService("Contributor Link Album")

	ctx := testhelpers.CtxWithUser(user)
	req := &AddExternalLinkRequest{ReleaseID: fmt.Sprintf("%d", release.ID)}
	req.Body.Platform = "bandcamp"
	req.Body.URL = "https://contributor.bandcamp.com/album/test"

	_, err := s.handler.AddExternalLinkHandler(ctx, req)
	testhelpers.AssertHumaError(s.T(), err, 403)
}

// TestAddExternalLink_UnauthenticatedForbidden documents the handler's
// fail-closed behavior when no user is in context. In production the
// rc.Protected JWT middleware returns 401 before the handler runs, so the
// handler never sees a nil user on a real request; this asserts the
// defense-in-depth 403 if that invariant is ever violated.
func (s *ReleaseHandlerIntegrationSuite) TestAddExternalLink_UnauthenticatedForbidden() {
	release := s.createReleaseViaService("Anon Link Album")

	// s.deps.Ctx carries no user.
	req := &AddExternalLinkRequest{ReleaseID: fmt.Sprintf("%d", release.ID)}
	req.Body.Platform = "bandcamp"
	req.Body.URL = "https://anon.bandcamp.com/album/test"

	_, err := s.handler.AddExternalLinkHandler(s.deps.Ctx, req)
	testhelpers.AssertHumaError(s.T(), err, 403)
}

// --- RemoveExternalLinkHandler ---

func (s *ReleaseHandlerIntegrationSuite) TestRemoveExternalLink_Success() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	release, err := s.deps.ReleaseService.CreateRelease(&contracts.CreateReleaseRequest{
		Title: "Remove Link Album",
		ExternalLinks: []contracts.CreateReleaseLinkEntry{
			{Platform: "spotify", URL: "https://open.spotify.com/album/abc"},
		},
	})
	s.Require().NoError(err)
	s.Require().Len(release.ExternalLinks, 1)

	ctx := testhelpers.CtxWithUser(admin)
	req := &RemoveExternalLinkRequest{
		ReleaseID: fmt.Sprintf("%d", release.ID),
		LinkID:    fmt.Sprintf("%d", release.ExternalLinks[0].ID),
	}

	_, err = s.handler.RemoveExternalLinkHandler(ctx, req)
	s.NoError(err)

	// Verify link is gone
	refreshed, err := s.deps.ReleaseService.GetRelease(release.ID)
	s.Require().NoError(err)
	s.Empty(refreshed.ExternalLinks)
}
