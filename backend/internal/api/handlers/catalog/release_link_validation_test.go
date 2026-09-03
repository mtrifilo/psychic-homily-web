package catalog

import (
	"fmt"
	"strings"

	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	catalogm "psychic-homily-backend/internal/models/catalog"
)

// Release external-link validation (PSY-1996).
//
// The stored platform/url pair renders as an <a href> headed by the platform
// label, so a row whose URL is not anchored to the platform it names is an
// arbitrary host wearing a name readers trust. Each case below fails if its
// half of the gate is removed.
//
// Methods hang off ReleaseHandlerIntegrationSuite (release_integration_test.go),
// which owns the DB fixture and the per-test cleanup.

func (s *ReleaseHandlerIntegrationSuite) TestAddExternalLink_RefusesForeignHost() {
	user := testhelpers.CreateUserWithTier(s.deps.DB, "trusted_contributor")
	release := s.createReleaseViaService("Foreign Host Album")

	ctx := testhelpers.CtxWithUser(user)
	req := &AddExternalLinkRequest{ReleaseID: fmt.Sprintf("%d", release.ID)}
	req.Body.Platform = "bandcamp"
	req.Body.URL = "https://bandcamp-checkout.evil.test/album/x"

	_, err := s.handler.AddExternalLinkHandler(ctx, req)
	testhelpers.AssertHumaError(s.T(), err, 422)

	var count int64
	s.deps.DB.Model(&catalogm.ReleaseExternalLink{}).Where("release_id = ?", release.ID).Count(&count)
	s.EqualValues(0, count, "a refused link must not be stored")
}

// A lookalike suffix is the case a substring host check would pass.
func (s *ReleaseHandlerIntegrationSuite) TestAddExternalLink_RefusesLookalikeSuffix() {
	user := testhelpers.CreateUserWithTier(s.deps.DB, "trusted_contributor")
	release := s.createReleaseViaService("Lookalike Album")

	ctx := testhelpers.CtxWithUser(user)
	req := &AddExternalLinkRequest{ReleaseID: fmt.Sprintf("%d", release.ID)}
	req.Body.Platform = "bandcamp"
	req.Body.URL = "https://bandcamp.com.evil.test/album/x"

	_, err := s.handler.AddExternalLinkHandler(ctx, req)
	testhelpers.AssertHumaError(s.T(), err, 422)
}

func (s *ReleaseHandlerIntegrationSuite) TestAddExternalLink_RefusesUnknownPlatform() {
	user := testhelpers.CreateUserWithTier(s.deps.DB, "trusted_contributor")
	release := s.createReleaseViaService("Unknown Platform Album")

	ctx := testhelpers.CtxWithUser(user)
	req := &AddExternalLinkRequest{ReleaseID: fmt.Sprintf("%d", release.ID)}
	req.Body.Platform = "napster"
	req.Body.URL = "https://us.napster.com/album/x"

	_, err := s.handler.AddExternalLinkHandler(ctx, req)
	testhelpers.AssertHumaError(s.T(), err, 422)
}

// A platform longer than the column's VARCHAR(50) reaches Postgres as a 22001
// without this check, which surfaces as a 500 the submitter cannot act on.
func (s *ReleaseHandlerIntegrationSuite) TestAddExternalLink_RefusesOversizePlatform() {
	user := testhelpers.CreateUserWithTier(s.deps.DB, "trusted_contributor")
	release := s.createReleaseViaService("Oversize Platform Album")

	ctx := testhelpers.CtxWithUser(user)
	req := &AddExternalLinkRequest{ReleaseID: fmt.Sprintf("%d", release.ID)}
	req.Body.Platform = strings.Repeat("b", 200)
	req.Body.URL = "https://test.bandcamp.com/album/x"

	_, err := s.handler.AddExternalLinkHandler(ctx, req)
	testhelpers.AssertHumaError(s.T(), err, 422)
}

func (s *ReleaseHandlerIntegrationSuite) TestAddExternalLink_RefusesNonHTTPScheme() {
	user := testhelpers.CreateUserWithTier(s.deps.DB, "trusted_contributor")
	release := s.createReleaseViaService("Scheme Album")

	ctx := testhelpers.CtxWithUser(user)
	req := &AddExternalLinkRequest{ReleaseID: fmt.Sprintf("%d", release.ID)}
	req.Body.Platform = "spotify"
	req.Body.URL = "javascript:alert(document.domain)"

	_, err := s.handler.AddExternalLinkHandler(ctx, req)
	testhelpers.AssertHumaError(s.T(), err, 422)
}

// Admins get the same refusal: the gate is about what the column may hold, not
// about who is trusted to type it.
func (s *ReleaseHandlerIntegrationSuite) TestAddExternalLink_AdminIsNotExempt() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	release := s.createReleaseViaService("Admin Hostile Album")

	ctx := testhelpers.CtxWithUser(admin)
	req := &AddExternalLinkRequest{ReleaseID: fmt.Sprintf("%d", release.ID)}
	req.Body.Platform = "spotify"
	req.Body.URL = "https://spotify-account-verify.evil.test/album/x"

	_, err := s.handler.AddExternalLinkHandler(ctx, req)
	testhelpers.AssertHumaError(s.T(), err, 422)
}

// The refusal has to name the hosts that work, or a curator cannot fix it.
func (s *ReleaseHandlerIntegrationSuite) TestAddExternalLink_RefusalNamesTheAcceptedHosts() {
	user := testhelpers.CreateUserWithTier(s.deps.DB, "trusted_contributor")
	release := s.createReleaseViaService("Refusal Copy Album")

	ctx := testhelpers.CtxWithUser(user)
	req := &AddExternalLinkRequest{ReleaseID: fmt.Sprintf("%d", release.ID)}
	req.Body.Platform = "spotify"
	req.Body.URL = "https://evil.test/album/x"

	_, err := s.handler.AddExternalLinkHandler(ctx, req)
	s.Require().Error(err)
	s.Contains(err.Error(), "spotify.com")
}

// "Required" is a different problem from "not on that platform", and keeps its
// own wording.
func (s *ReleaseHandlerIntegrationSuite) TestAddExternalLink_RefusesEmptyValues() {
	user := testhelpers.CreateUserWithTier(s.deps.DB, "trusted_contributor")
	release := s.createReleaseViaService("Empty Values Album")

	ctx := testhelpers.CtxWithUser(user)
	req := &AddExternalLinkRequest{ReleaseID: fmt.Sprintf("%d", release.ID)}
	req.Body.Platform = "bandcamp"
	req.Body.URL = ""
	_, err := s.handler.AddExternalLinkHandler(ctx, req)
	testhelpers.AssertHumaError(s.T(), err, 422)

	req.Body.Platform = ""
	req.Body.URL = "https://test.bandcamp.com/album/x"
	_, err = s.handler.AddExternalLinkHandler(ctx, req)
	testhelpers.AssertHumaError(s.T(), err, 422)
}

// A release link on a platform the picker did not previously offer is accepted:
// the registry is one list, and the seed already writes youtube_music rows.
func (s *ReleaseHandlerIntegrationSuite) TestAddExternalLink_AcceptsEveryRegisteredPlatform() {
	user := testhelpers.CreateUserWithTier(s.deps.DB, "trusted_contributor")
	release := s.createReleaseViaService("Every Platform Album")
	ctx := testhelpers.CtxWithUser(user)

	for _, tc := range []struct{ platform, url string }{
		{"youtube_music", "https://music.youtube.com/playlist?list=EXEMPLAR"},
		{"deezer", "https://www.deezer.com/album/123456"},
		{"amazon_music", "https://music.amazon.com/albums/B00EXAMPLE"},
	} {
		req := &AddExternalLinkRequest{ReleaseID: fmt.Sprintf("%d", release.ID)}
		req.Body.Platform = tc.platform
		req.Body.URL = tc.url

		resp, err := s.handler.AddExternalLinkHandler(ctx, req)
		s.Require().NoError(err, "platform %s", tc.platform)
		s.Equal(tc.platform, resp.Body.Platform)
	}
}

// --- CreateReleaseHandler external-link validation ---

func (s *ReleaseHandlerIntegrationSuite) TestCreateRelease_RefusesHostileExternalLink() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	ctx := testhelpers.CtxWithUser(admin)

	req := &CreateReleaseRequest{}
	req.Body.Title = "Hostile Link Release"
	req.Body.ExternalLinks = []CreateReleaseLinkInput{
		{Platform: "bandcamp", URL: "https://kingbuffalo.bandcamp.com/album/regenerator"},
		{Platform: "spotify", URL: "https://evil.test/album/x"},
	}

	_, err := s.handler.CreateReleaseHandler(ctx, req)
	testhelpers.AssertHumaError(s.T(), err, 422)
	// The body carries a list, so the refusal has to say which entry to fix.
	s.Require().Error(err)
	s.Contains(err.Error(), "external_links[1]")

	var count int64
	s.deps.DB.Model(&catalogm.Release{}).Where("title = ?", "Hostile Link Release").Count(&count)
	s.EqualValues(0, count, "a refused link must not leave a half-created release")
}

func (s *ReleaseHandlerIntegrationSuite) TestCreateRelease_AcceptsAnchoredExternalLinks() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	ctx := testhelpers.CtxWithUser(admin)

	req := &CreateReleaseRequest{}
	req.Body.Title = "Anchored Link Release"
	req.Body.ExternalLinks = []CreateReleaseLinkInput{
		{Platform: "bandcamp", URL: "https://kingbuffalo.bandcamp.com/album/regenerator"},
		{Platform: "apple_music", URL: "https://music.apple.com/us/album/x/1"},
	}

	resp, err := s.handler.CreateReleaseHandler(ctx, req)
	s.Require().NoError(err)
	s.Len(resp.Body.ExternalLinks, 2)
}
