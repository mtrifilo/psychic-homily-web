package catalog

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/utils"
)

// Release external-link validation at the HTTP layer (PSY-1996).
//
// The rules themselves are pinned in internal/utils by the shared corpus; these
// cases are about what this layer adds: that the service's refusal arrives as a
// 422 carrying the validator's sentence rather than the generic 500, that no
// row is written, and that the tier gate does not exempt anyone.
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
	// The refusal has to name the hosts that work, or a curator cannot fix it.
	s.Require().Error(err)
	s.Contains(err.Error(), "bandcamp.com")

	var count int64
	s.deps.DB.Model(&catalogm.ReleaseExternalLink{}).Where("release_id = ?", release.ID).Count(&count)
	s.EqualValues(0, count, "a refused link must not be stored")
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

// This route used to answer both empty halves with one message of its own; the
// refusal is now the validator's, and still a 422.
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

// A platform the picker did not previously offer is accepted: the registry is
// one list, and the seed already writes youtube_music rows.
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

// TestPlatformDocListsEveryRegisteredPlatform pins the two `doc:` tags that
// publish the accepted platforms in the OpenAPI document (and, through
// bun run api:types, in the frontend's generated types).
//
// The tag is a string literal because huma reads struct tags statically, so it
// is the one copy of the registry that nothing else forces to stay current.
// This is that force: adding a platform without editing both tags fails here
// rather than shipping a document that lies about the contract.
func TestPlatformDocListsEveryRegisteredPlatform(t *testing.T) {
	docs := map[string]string{
		"CreateReleaseLinkInput": reflect.TypeOf(CreateReleaseLinkInput{}).
			Field(0).Tag.Get("doc"),
		"AddExternalLinkRequest.Body": reflect.TypeOf(AddExternalLinkRequest{}).
			Field(1).Type.Field(0).Tag.Get("doc"),
	}

	for name, doc := range docs {
		require.NotEmpty(t, doc, "%s has no doc tag", name)
		for _, platform := range utils.ReleaseLinkPlatforms() {
			assert.Contains(t, doc, platform, "%s doc omits %q", name, platform)
		}
	}
}
