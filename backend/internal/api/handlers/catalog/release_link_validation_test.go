package catalog

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

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

// A registered platform the pickers do not offer is still accepted here: the
// API's accepted set is wider than what a curator is invited to type, and the
// seed already writes youtube_music rows.
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

// TestExternalLinkAuditRecordsThePair pins the audit entries as an evidence
// trail rather than a counter. release_external_links carries no created_by and
// a removal hard-deletes the row, so the add entry is the only record of who put
// a URL under a platform label, and the remove entry's link_id is the only thing
// that ties a deletion back to it.
func (s *ReleaseHandlerIntegrationSuite) TestExternalLinkAuditRecordsThePair() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	release := s.createReleaseViaService("Audited Link Album")
	ctx := testhelpers.CtxWithUser(admin)

	// The handler logs from a goroutine, so the assertions read from a channel.
	entries := make(chan map[string]interface{}, 4)
	audit := &testhelpers.MockAuditLogService{
		LogActionFn: func(_ uint, action, _ string, _ uint, metadata map[string]interface{}) {
			if action == "add_release_link" || action == "remove_release_link" {
				entries <- metadata
			}
		},
	}
	handler := NewReleaseHandler(s.deps.ReleaseService, s.deps.ArtistService, audit, nil)

	addReq := &AddExternalLinkRequest{ReleaseID: fmt.Sprintf("%d", release.ID)}
	addReq.Body.Platform = "bandcamp"
	addReq.Body.URL = "https://kingbuffalo.bandcamp.com/album/regenerator"
	addResp, err := handler.AddExternalLinkHandler(ctx, addReq)
	s.Require().NoError(err)

	added := s.nextAuditEntry(entries)
	s.Equal("bandcamp", added["platform"])
	s.Equal("https://kingbuffalo.bandcamp.com/album/regenerator", added["url"])
	s.EqualValues(addResp.Body.ID, added["link_id"])

	_, err = handler.RemoveExternalLinkHandler(ctx, &RemoveExternalLinkRequest{
		ReleaseID: fmt.Sprintf("%d", release.ID),
		LinkID:    fmt.Sprintf("%d", addResp.Body.ID),
	})
	s.Require().NoError(err)

	removed := s.nextAuditEntry(entries)
	s.EqualValues(addResp.Body.ID, removed["link_id"])
}

func (s *ReleaseHandlerIntegrationSuite) nextAuditEntry(entries chan map[string]interface{}) map[string]interface{} {
	s.T().Helper()
	select {
	case metadata := <-entries:
		return metadata
	case <-time.After(10 * time.Second):
		s.Require().Fail("no audit entry was logged")
		return nil
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

// platformDocPrefix is the fixed half of the two `doc:` tags below. The list
// after it is parsed back out and compared as a SET, so a stale name is caught
// as well as a missing one, and a new key that happens to be a substring of an
// existing one ("youtube" inside "youtube_music") cannot pass by accident.
const platformDocPrefix = "Platform key. One of: "

// TestPlatformDocListsEveryRegisteredPlatform pins the two `doc:` tags that
// publish the accepted platforms in the OpenAPI document (and, through
// bun run api:types, in the frontend's generated types).
//
// The tag is a string literal because huma reads struct tags statically, so it
// is the one copy of the registry that nothing else forces to stay current.
// This is that force: changing the registry without editing both tags fails
// here rather than shipping a document that lies about the contract.
func TestPlatformDocListsEveryRegisteredPlatform(t *testing.T) {
	// The field lookups are positional, so they are asserted by name first:
	// inserting a field ahead of these would otherwise retarget the test
	// silently.
	linkInput := reflect.TypeOf(CreateReleaseLinkInput{}).Field(0)
	require.Equal(t, "Platform", linkInput.Name)
	addBody := reflect.TypeOf(AddExternalLinkRequest{}).Field(1)
	require.Equal(t, "Body", addBody.Name)
	addPlatform := addBody.Type.Field(0)
	require.Equal(t, "Platform", addPlatform.Name)

	docs := map[string]string{
		"CreateReleaseLinkInput":      linkInput.Tag.Get("doc"),
		"AddExternalLinkRequest.Body": addPlatform.Tag.Get("doc"),
	}

	for name, doc := range docs {
		require.True(t, strings.HasPrefix(doc, platformDocPrefix),
			"%s doc must start with %q, got %q", name, platformDocPrefix, doc)
		listed := strings.Split(strings.TrimPrefix(doc, platformDocPrefix), ", ")
		assert.ElementsMatch(t, utils.ReleaseLinkPlatforms(), listed,
			"%s doc and the registry name different platforms", name)
	}
}
