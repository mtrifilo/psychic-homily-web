package community

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	"psychic-homily-backend/internal/api/handlers/shared"
	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	authm "psychic-homily-backend/internal/models/auth"
	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services"
	"psychic-homily-backend/internal/services/contracts"
)

// Local alias so the helper isn't tied to the `services.` qualifier on every
// reference. PSY-356.
const MinPublicCollectionItems = services.MinPublicCollectionItems

type CollectionHandlerIntegrationSuite struct {
	suite.Suite
	deps    *testhelpers.IntegrationDeps
	handler *CollectionHandler
}

func (s *CollectionHandlerIntegrationSuite) SetupSuite() {
	s.deps = testhelpers.SetupIntegrationDeps(s.T())
	s.handler = NewCollectionHandler(s.deps.CollectionService, s.deps.AuditLogService)
}

func (s *CollectionHandlerIntegrationSuite) TearDownTest() {
	testhelpers.CleanupTables(s.deps.DB)
}

func (s *CollectionHandlerIntegrationSuite) TearDownSuite() {
	s.deps.TestDB.Cleanup()
}

func TestCollectionHandlerIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	suite.Run(t, new(CollectionHandlerIntegrationSuite))
}

// --- Helpers ---

// createCollectionViaService creates a collection. PSY-356 disallows creating
// public-at-create-time, so the helper always creates private. Tests that
// pass isPublic=true get the gate-passing dance applied (seed 3 items + 50+
// char description, then flip is_public). Tests that pass false get a bare
// private collection — most tests that only need a slug should call with
// false to keep item counts predictable.
func (s *CollectionHandlerIntegrationSuite) createCollectionViaService(user *authm.User, title string, isPublic bool) *contracts.CollectionDetailResponse {
	priv, err := s.deps.CollectionService.CreateCollection(user.ID, &contracts.CreateCollectionRequest{
		Title:    title,
		IsPublic: false,
	})
	s.Require().NoError(err)

	if !isPublic {
		return priv
	}

	// PSY-356 publish-gate dance: private → seed items + description → flip public.
	for i := 0; i < MinPublicCollectionItems; i++ {
		artist := testhelpers.CreateArtist(s.deps.DB, fmt.Sprintf("%s seed %d-%d", title, i, time.Now().UnixNano()))
		_, err = s.deps.CollectionService.AddItem(priv.Slug, user.ID, &contracts.AddCollectionItemRequest{
			EntityType: "artist",
			EntityID:   artist.ID,
		})
		s.Require().NoError(err)
	}

	desc := fmt.Sprintf("Quality-gate description for %s — long enough to satisfy the 50-char minimum.", title)
	pub := true
	resp, err := s.deps.CollectionService.UpdateCollection(priv.Slug, user.ID, false, &contracts.UpdateCollectionRequest{
		Description: &desc,
		IsPublic:    &pub,
	})
	s.Require().NoError(err)
	return resp
}

// ============================================================================
// CreateCollectionHandler
// ============================================================================

func (s *CollectionHandlerIntegrationSuite) TestCreateCollection_Success() {
	user := testhelpers.CreateTestUser(s.deps.DB)
	ctx := testhelpers.CtxWithUser(user)

	// PSY-356: created private — public-at-create is rejected by the gate.
	req := &CreateCollectionHandlerRequest{}
	req.Body.Title = "My Favorite Artists"
	req.Body.IsPublic = false

	resp, err := s.handler.CreateCollectionHandler(ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.Equal("My Favorite Artists", resp.Body.Title)
	s.False(resp.Body.IsPublic)
	s.Equal(user.ID, resp.Body.CreatorID)
	s.NotEmpty(resp.Body.Slug)
}

func (s *CollectionHandlerIntegrationSuite) TestCreateCollection_WithDescription() {
	user := testhelpers.CreateTestUser(s.deps.DB)
	ctx := testhelpers.CtxWithUser(user)

	desc := "A curated list of favorites"
	req := &CreateCollectionHandlerRequest{}
	req.Body.Title = "Curated List"
	req.Body.Description = &desc
	req.Body.IsPublic = false // PSY-356: tests description persistence, not visibility.
	req.Body.Collaborative = true

	resp, err := s.handler.CreateCollectionHandler(ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.Equal("Curated List", resp.Body.Title)
	s.Equal("A curated list of favorites", resp.Body.Description)
	s.True(resp.Body.Collaborative)
}

func (s *CollectionHandlerIntegrationSuite) TestCreateCollection_NoAuth() {
	req := &CreateCollectionHandlerRequest{}
	req.Body.Title = "Unauthorized Collection"

	_, err := s.handler.CreateCollectionHandler(context.Background(), req)
	testhelpers.AssertHumaError(s.T(), err, 401)
}

func (s *CollectionHandlerIntegrationSuite) TestCreateCollection_EmptyTitle() {
	user := testhelpers.CreateTestUser(s.deps.DB)
	ctx := testhelpers.CtxWithUser(user)

	req := &CreateCollectionHandlerRequest{}
	req.Body.Title = ""

	_, err := s.handler.CreateCollectionHandler(ctx, req)
	testhelpers.AssertHumaError(s.T(), err, 422)
}

// ============================================================================
// GetCollectionHandler
// ============================================================================

func (s *CollectionHandlerIntegrationSuite) TestGetCollection_BySlug() {
	user := testhelpers.CreateTestUser(s.deps.DB)
	coll := s.createCollectionViaService(user, "Get By Slug", true)

	req := &GetCollectionHandlerRequest{Slug: coll.Slug}
	resp, err := s.handler.GetCollectionHandler(context.Background(), req)
	s.NoError(err)
	s.NotNil(resp)
	s.Equal(coll.ID, resp.Body.ID)
	s.Equal("Get By Slug", resp.Body.Title)
}

func (s *CollectionHandlerIntegrationSuite) TestGetCollection_NotFound() {
	req := &GetCollectionHandlerRequest{Slug: "nonexistent-slug"}
	_, err := s.handler.GetCollectionHandler(context.Background(), req)
	testhelpers.AssertHumaError(s.T(), err, 404)
}

func (s *CollectionHandlerIntegrationSuite) TestGetCollection_AuthenticatedViewerSeesSubscription() {
	owner := testhelpers.CreateTestUser(s.deps.DB)
	viewer := testhelpers.CreateTestUser(s.deps.DB)
	coll := s.createCollectionViaService(owner, "Sub Check", true)

	// Subscribe the viewer
	err := s.deps.CollectionService.Subscribe(coll.Slug, viewer.ID)
	s.Require().NoError(err)

	ctx := testhelpers.CtxWithUser(viewer)
	req := &GetCollectionHandlerRequest{Slug: coll.Slug}
	resp, err := s.handler.GetCollectionHandler(ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.True(resp.Body.IsSubscribed)
}

// ============================================================================
// GetCollectionStatsHandler
// ============================================================================

func (s *CollectionHandlerIntegrationSuite) TestGetCollectionStats_Success() {
	user := testhelpers.CreateTestUser(s.deps.DB)
	// Private — the test asserts a precise ItemCount=1 and the gate dance
	// would seed 3 extra items. Visibility is incidental here.
	coll := s.createCollectionViaService(user, "Stats Collection", false)

	// Add an artist item
	artist := testhelpers.CreateArtist(s.deps.DB, "Stats Artist")
	_, err := s.deps.CollectionService.AddItem(coll.Slug, user.ID, &contracts.AddCollectionItemRequest{
		EntityType: "artist",
		EntityID:   artist.ID,
	})
	s.Require().NoError(err)

	req := &GetCollectionStatsHandlerRequest{Slug: coll.Slug}
	resp, err := s.handler.GetCollectionStatsHandler(context.Background(), req)
	s.NoError(err)
	s.NotNil(resp)
	s.Equal(1, resp.Body.ItemCount)
}

func (s *CollectionHandlerIntegrationSuite) TestGetCollectionStats_NotFound() {
	req := &GetCollectionStatsHandlerRequest{Slug: "nonexistent"}
	_, err := s.handler.GetCollectionStatsHandler(context.Background(), req)
	testhelpers.AssertHumaError(s.T(), err, 404)
}

// ============================================================================
// ListCollectionsHandler
// ============================================================================

func (s *CollectionHandlerIntegrationSuite) TestListCollections_Success() {
	user := testhelpers.CreateTestUser(s.deps.DB)
	s.createCollectionViaService(user, "List A", true)
	s.createCollectionViaService(user, "List B", true)
	s.createCollectionViaService(user, "List C", true)

	req := &ListCollectionsHandlerRequest{}
	resp, err := s.handler.ListCollectionsHandler(context.Background(), req)
	s.NoError(err)
	s.NotNil(resp)
	s.GreaterOrEqual(resp.Body.Total, int64(3))
}

func (s *CollectionHandlerIntegrationSuite) TestListCollections_Empty() {
	req := &ListCollectionsHandlerRequest{}
	resp, err := s.handler.ListCollectionsHandler(context.Background(), req)
	s.NoError(err)
	s.NotNil(resp)
	s.Equal(int64(0), resp.Body.Total)
}

func (s *CollectionHandlerIntegrationSuite) TestListCollections_DefaultLimit() {
	user := testhelpers.CreateTestUser(s.deps.DB)
	// Create more than 0 collections so we see results
	s.createCollectionViaService(user, "Default Limit A", true)

	req := &ListCollectionsHandlerRequest{} // Limit defaults to 20
	resp, err := s.handler.ListCollectionsHandler(context.Background(), req)
	s.NoError(err)
	s.NotNil(resp)
	s.GreaterOrEqual(resp.Body.Total, int64(1))
}

func (s *CollectionHandlerIntegrationSuite) TestListCollections_WithLimit() {
	user := testhelpers.CreateTestUser(s.deps.DB)
	s.createCollectionViaService(user, "Limited A", true)
	s.createCollectionViaService(user, "Limited B", true)
	s.createCollectionViaService(user, "Limited C", true)

	req := &ListCollectionsHandlerRequest{Limit: 2}
	resp, err := s.handler.ListCollectionsHandler(context.Background(), req)
	s.NoError(err)
	s.NotNil(resp)
	s.LessOrEqual(len(resp.Body.Collections), 2)
	s.GreaterOrEqual(resp.Body.Total, int64(3))
}

func (s *CollectionHandlerIntegrationSuite) TestListCollections_OnlyPublic() {
	user := testhelpers.CreateTestUser(s.deps.DB)
	s.createCollectionViaService(user, "Public One", true)
	s.createCollectionViaService(user, "Private One", false)

	req := &ListCollectionsHandlerRequest{}
	resp, err := s.handler.ListCollectionsHandler(context.Background(), req)
	s.NoError(err)
	s.NotNil(resp)
	// Should only return public collections
	for _, c := range resp.Body.Collections {
		s.True(c.IsPublic, "expected only public collections in public listing")
	}
}

// PSY-352: sort=popular orders by HN gravity at the service layer; the
// handler's job is just to forward the value and reject unknowns.
func (s *CollectionHandlerIntegrationSuite) TestListCollections_PopularSort_Accepted() {
	user := testhelpers.CreateTestUser(s.deps.DB)
	s.createCollectionViaService(user, "Popular Sort A", true)

	req := &ListCollectionsHandlerRequest{Sort: "popular"}
	_, err := s.handler.ListCollectionsHandler(context.Background(), req)
	s.NoError(err)
}

func (s *CollectionHandlerIntegrationSuite) TestListCollections_UnknownSort_Rejected() {
	req := &ListCollectionsHandlerRequest{Sort: "bogus"}
	_, err := s.handler.ListCollectionsHandler(context.Background(), req)
	testhelpers.AssertHumaError(s.T(), err, 422)
}

func (s *CollectionHandlerIntegrationSuite) TestListCollections_FeaturedFilter() {
	user := testhelpers.CreateTestUser(s.deps.DB)
	coll := s.createCollectionViaService(user, "Featured Coll", true)
	s.createCollectionViaService(user, "Not Featured", true)

	// Set one as featured
	err := s.deps.CollectionService.SetFeatured(coll.Slug, true)
	s.Require().NoError(err)

	req := &ListCollectionsHandlerRequest{Featured: 1}
	resp, err := s.handler.ListCollectionsHandler(context.Background(), req)
	s.NoError(err)
	s.NotNil(resp)
	s.Equal(int64(1), resp.Body.Total)
	s.Equal("Featured Coll", resp.Body.Collections[0].Title)
}

// ============================================================================
// PSY-355: expanded browse search (description / notes / tag name+alias)
// ============================================================================

// seedSearchableCollection bundles the publish-gate dance with a description
// and (optionally) an item-with-notes payload so the search-tier tests below
// stay readable. Returns the public slug.
func (s *CollectionHandlerIntegrationSuite) seedSearchableCollection(
	user *authm.User,
	title, description, itemNotes string,
) string {
	priv, err := s.deps.CollectionService.CreateCollection(user.ID, &contracts.CreateCollectionRequest{
		Title:    title,
		IsPublic: false,
	})
	s.Require().NoError(err)

	// PSY-356 publish gate seeds. Reuse the supplied notes on the first
	// item so both the gate and notes-tier match share the same row.
	for i := 0; i < MinPublicCollectionItems; i++ {
		artist := testhelpers.CreateArtist(s.deps.DB, fmt.Sprintf("%s seed %d-%d", title, i, time.Now().UnixNano()))
		req := &contracts.AddCollectionItemRequest{
			EntityType: "artist",
			EntityID:   artist.ID,
		}
		if i == 0 && itemNotes != "" {
			notes := itemNotes
			req.Notes = &notes
		}
		_, err = s.deps.CollectionService.AddItem(priv.Slug, user.ID, req)
		s.Require().NoError(err)
	}

	// Description must be at least 50 chars to pass the publish gate; pad
	// when the test's intent description is shorter.
	desc := description
	if len(desc) < 50 {
		desc = description + " — " + strings.Repeat("seed", 16)
	}
	pub := true
	_, err = s.deps.CollectionService.UpdateCollection(priv.Slug, user.ID, false, &contracts.UpdateCollectionRequest{
		Description: &desc,
		IsPublic:    &pub,
	})
	s.Require().NoError(err)
	return priv.Slug
}

// TestListCollections_Search_DescriptionMatch surfaces a collection whose
// only matching field is its description.
func (s *CollectionHandlerIntegrationSuite) TestListCollections_Search_DescriptionMatch() {
	user := testhelpers.CreateTestUser(s.deps.DB)
	s.seedSearchableCollection(user, "Generic Title 1", "deep dive into shoegaze records", "")
	s.seedSearchableCollection(user, "Generic Title 2", "wide-ranging mix of new wave hits", "")

	req := &ListCollectionsHandlerRequest{Search: "shoegaze"}
	resp, err := s.handler.ListCollectionsHandler(context.Background(), req)
	s.Require().NoError(err)
	s.Require().Len(resp.Body.Collections, 1)
	s.Equal("Generic Title 1", resp.Body.Collections[0].Title)
}

// TestListCollections_Search_NotesMatch surfaces a collection whose only
// matching field is the notes on one of its items.
func (s *CollectionHandlerIntegrationSuite) TestListCollections_Search_NotesMatch() {
	user := testhelpers.CreateTestUser(s.deps.DB)
	s.seedSearchableCollection(user, "Plain Title One", "ordinary description text", "phenomenal myrkur set at the venue")
	s.seedSearchableCollection(user, "Plain Title Two", "ordinary description text", "")

	req := &ListCollectionsHandlerRequest{Search: "myrkur"}
	resp, err := s.handler.ListCollectionsHandler(context.Background(), req)
	s.Require().NoError(err)
	s.Require().Len(resp.Body.Collections, 1)
	s.Equal("Plain Title One", resp.Body.Collections[0].Title)
}

// TestListCollections_Search_TagNameMatch surfaces a collection whose only
// matching field is an applied tag's name.
func (s *CollectionHandlerIntegrationSuite) TestListCollections_Search_TagNameMatch() {
	user := testhelpers.CreateTestUser(s.deps.DB)
	s.promoteContributorForTags(user)

	taggedSlug := s.seedSearchableCollection(user, "Untitled Mix Alpha", "ordinary description text", "")
	s.seedSearchableCollection(user, "Untitled Mix Beta", "ordinary description text", "")

	ctx := testhelpers.CtxWithUser(user)
	tagReq := &AddCollectionTagHandlerRequest{Slug: taggedSlug}
	tagReq.Body.TagName = "darkwave"
	_, err := s.handler.AddCollectionTagHandler(ctx, tagReq)
	s.Require().NoError(err)

	req := &ListCollectionsHandlerRequest{Search: "darkwave"}
	resp, err := s.handler.ListCollectionsHandler(context.Background(), req)
	s.Require().NoError(err)
	s.Require().Len(resp.Body.Collections, 1)
	s.Equal("Untitled Mix Alpha", resp.Body.Collections[0].Title)
}

// TestListCollections_Search_TagAliasMatch surfaces a collection whose only
// matching field is an alias on an applied tag.
func (s *CollectionHandlerIntegrationSuite) TestListCollections_Search_TagAliasMatch() {
	user := testhelpers.CreateTestUser(s.deps.DB)
	s.promoteContributorForTags(user)

	taggedSlug := s.seedSearchableCollection(user, "Alpha Catalog", "ordinary description text", "")
	s.seedSearchableCollection(user, "Beta Catalog", "ordinary description text", "")

	tag, err := s.deps.TagService.CreateTag("post-rock", nil, nil, catalogm.TagCategoryGenre, false, &user.ID)
	s.Require().NoError(err)
	s.Require().NoError(s.deps.DB.Create(&catalogm.TagAlias{
		TagID: tag.ID,
		Alias: "postrock",
	}).Error)

	ctx := testhelpers.CtxWithUser(user)
	tagReq := &AddCollectionTagHandlerRequest{Slug: taggedSlug}
	tagReq.Body.TagID = tag.ID
	_, err = s.handler.AddCollectionTagHandler(ctx, tagReq)
	s.Require().NoError(err)

	req := &ListCollectionsHandlerRequest{Search: "postrock"}
	resp, err := s.handler.ListCollectionsHandler(context.Background(), req)
	s.Require().NoError(err)
	s.Require().Len(resp.Body.Collections, 1)
	s.Equal("Alpha Catalog", resp.Body.Collections[0].Title)
}

// TestListCollections_Search_RelevanceTierOrder seeds four collections each
// matched in a different tier and verifies title > description > notes > tag.
func (s *CollectionHandlerIntegrationSuite) TestListCollections_Search_RelevanceTierOrder() {
	user := testhelpers.CreateTestUser(s.deps.DB)
	s.promoteContributorForTags(user)

	tagOnlySlug := s.seedSearchableCollection(user, "Tag Only Title", "ordinary description text", "")
	notesOnlySlug := s.seedSearchableCollection(user, "Notes Only Title", "ordinary description text", "magnetar reverb pedal noted here")
	descOnlySlug := s.seedSearchableCollection(user, "Description Only Title", "a deep magnetar synth treatise", "")
	titleOnlySlug := s.seedSearchableCollection(user, "Magnetar Best Of", "ordinary description text", "")

	ctx := testhelpers.CtxWithUser(user)
	tagReq := &AddCollectionTagHandlerRequest{Slug: tagOnlySlug}
	tagReq.Body.TagName = "magnetar"
	_, err := s.handler.AddCollectionTagHandler(ctx, tagReq)
	s.Require().NoError(err)

	req := &ListCollectionsHandlerRequest{Search: "magnetar"}
	resp, err := s.handler.ListCollectionsHandler(context.Background(), req)
	s.Require().NoError(err)
	s.Require().Len(resp.Body.Collections, 4)

	// Verify the tier order. Resolve slugs back to titles so the assertion
	// failure messages are informative if the order regresses.
	s.Equal(titleOnlySlug, resp.Body.Collections[0].Slug, "tier 1 (title) leads")
	s.Equal(descOnlySlug, resp.Body.Collections[1].Slug, "tier 2 (description) second")
	s.Equal(notesOnlySlug, resp.Body.Collections[2].Slug, "tier 3 (notes) third")
	s.Equal(tagOnlySlug, resp.Body.Collections[3].Slug, "tier 4 (tag) last")
}

// TestListCollections_Search_DistinctMultiFieldMatch confirms a collection
// matched on multiple tiers appears exactly once. Guards against a JOIN-style
// fan-out regression.
func (s *CollectionHandlerIntegrationSuite) TestListCollections_Search_DistinctMultiFieldMatch() {
	user := testhelpers.CreateTestUser(s.deps.DB)
	s.promoteContributorForTags(user)

	taggedSlug := s.seedSearchableCollection(user, "Krautrock Essentials",
		"a tour through krautrock for the curious", "krautrock cornerstone")

	ctx := testhelpers.CtxWithUser(user)
	tagReq := &AddCollectionTagHandlerRequest{Slug: taggedSlug}
	tagReq.Body.TagName = "krautrock"
	_, err := s.handler.AddCollectionTagHandler(ctx, tagReq)
	s.Require().NoError(err)

	req := &ListCollectionsHandlerRequest{Search: "krautrock"}
	resp, err := s.handler.ListCollectionsHandler(context.Background(), req)
	s.Require().NoError(err)
	s.Equal(int64(1), resp.Body.Total)
	s.Require().Len(resp.Body.Collections, 1)
}

// TestListCollections_Search_EmptyShortCircuits verifies an empty/whitespace
// query disables the predicate at the handler boundary (mirrors PSY-520).
func (s *CollectionHandlerIntegrationSuite) TestListCollections_Search_EmptyShortCircuits() {
	user := testhelpers.CreateTestUser(s.deps.DB)
	s.createCollectionViaService(user, "Alpha Coll", true)
	s.createCollectionViaService(user, "Beta Coll", true)

	req := &ListCollectionsHandlerRequest{Search: ""}
	resp, err := s.handler.ListCollectionsHandler(context.Background(), req)
	s.Require().NoError(err)
	s.Equal(int64(2), resp.Body.Total)

	req = &ListCollectionsHandlerRequest{Search: "   \t  "}
	resp, err = s.handler.ListCollectionsHandler(context.Background(), req)
	s.Require().NoError(err)
	s.Equal(int64(2), resp.Body.Total)
}

// TestListCollections_Search_TitleStillMatches is regression coverage that
// the existing title-only path still works after the OR expansion.
func (s *CollectionHandlerIntegrationSuite) TestListCollections_Search_TitleStillMatches() {
	user := testhelpers.CreateTestUser(s.deps.DB)
	s.seedSearchableCollection(user, "Phoenix Punk Bands", "ordinary description text", "")
	s.seedSearchableCollection(user, "Chicago Jazz Venues", "ordinary description text", "")

	req := &ListCollectionsHandlerRequest{Search: "punk"}
	resp, err := s.handler.ListCollectionsHandler(context.Background(), req)
	s.Require().NoError(err)
	s.Equal(int64(1), resp.Body.Total)
	s.Equal("Phoenix Punk Bands", resp.Body.Collections[0].Title)
}

// ============================================================================
// UpdateCollectionHandler
// ============================================================================

func (s *CollectionHandlerIntegrationSuite) TestUpdateCollection_Success() {
	user := testhelpers.CreateTestUser(s.deps.DB)
	coll := s.createCollectionViaService(user, "Original Title", true)

	ctx := testhelpers.CtxWithUser(user)
	newTitle := "Updated Title"
	req := &UpdateCollectionHandlerRequest{Slug: coll.Slug}
	req.Body.Title = &newTitle

	resp, err := s.handler.UpdateCollectionHandler(ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.Equal("Updated Title", resp.Body.Title)
}

func (s *CollectionHandlerIntegrationSuite) TestUpdateCollection_ChangeVisibility() {
	user := testhelpers.CreateTestUser(s.deps.DB)
	coll := s.createCollectionViaService(user, "Visibility Test", true)

	ctx := testhelpers.CtxWithUser(user)
	isPublic := false
	req := &UpdateCollectionHandlerRequest{Slug: coll.Slug}
	req.Body.IsPublic = &isPublic

	resp, err := s.handler.UpdateCollectionHandler(ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.False(resp.Body.IsPublic)
}

func (s *CollectionHandlerIntegrationSuite) TestUpdateCollection_NoAuth() {
	user := testhelpers.CreateTestUser(s.deps.DB)
	coll := s.createCollectionViaService(user, "No Auth Update", true)

	newTitle := "Hacked"
	req := &UpdateCollectionHandlerRequest{Slug: coll.Slug}
	req.Body.Title = &newTitle

	_, err := s.handler.UpdateCollectionHandler(context.Background(), req)
	testhelpers.AssertHumaError(s.T(), err, 401)
}

func (s *CollectionHandlerIntegrationSuite) TestUpdateCollection_NotOwner() {
	owner := testhelpers.CreateTestUser(s.deps.DB)
	other := testhelpers.CreateTestUser(s.deps.DB)
	coll := s.createCollectionViaService(owner, "Not Mine", true)

	ctx := testhelpers.CtxWithUser(other)
	newTitle := "Hacked"
	req := &UpdateCollectionHandlerRequest{Slug: coll.Slug}
	req.Body.Title = &newTitle

	_, err := s.handler.UpdateCollectionHandler(ctx, req)
	testhelpers.AssertHumaError(s.T(), err, 403)
}

func (s *CollectionHandlerIntegrationSuite) TestUpdateCollection_AdminCanUpdate() {
	owner := testhelpers.CreateTestUser(s.deps.DB)
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	coll := s.createCollectionViaService(owner, "Admin Update", true)

	ctx := testhelpers.CtxWithUser(admin)
	newTitle := "Admin Updated"
	req := &UpdateCollectionHandlerRequest{Slug: coll.Slug}
	req.Body.Title = &newTitle

	resp, err := s.handler.UpdateCollectionHandler(ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.Equal("Admin Updated", resp.Body.Title)
}

func (s *CollectionHandlerIntegrationSuite) TestUpdateCollection_NotFound() {
	user := testhelpers.CreateTestUser(s.deps.DB)
	ctx := testhelpers.CtxWithUser(user)

	newTitle := "Ghost"
	req := &UpdateCollectionHandlerRequest{Slug: "nonexistent-slug"}
	req.Body.Title = &newTitle

	_, err := s.handler.UpdateCollectionHandler(ctx, req)
	testhelpers.AssertHumaError(s.T(), err, 404)
}

// ============================================================================
// DeleteCollectionHandler
// ============================================================================

func (s *CollectionHandlerIntegrationSuite) TestDeleteCollection_Success() {
	user := testhelpers.CreateTestUser(s.deps.DB)
	coll := s.createCollectionViaService(user, "Deletable", true)

	ctx := testhelpers.CtxWithUser(user)
	req := &DeleteCollectionHandlerRequest{Slug: coll.Slug}
	_, err := s.handler.DeleteCollectionHandler(ctx, req)
	s.NoError(err)

	// Verify deleted
	getReq := &GetCollectionHandlerRequest{Slug: coll.Slug}
	_, err = s.handler.GetCollectionHandler(context.Background(), getReq)
	testhelpers.AssertHumaError(s.T(), err, 404)
}

func (s *CollectionHandlerIntegrationSuite) TestDeleteCollection_NoAuth() {
	user := testhelpers.CreateTestUser(s.deps.DB)
	coll := s.createCollectionViaService(user, "NoAuth Delete", true)

	req := &DeleteCollectionHandlerRequest{Slug: coll.Slug}
	_, err := s.handler.DeleteCollectionHandler(context.Background(), req)
	testhelpers.AssertHumaError(s.T(), err, 401)
}

func (s *CollectionHandlerIntegrationSuite) TestDeleteCollection_NotOwner() {
	owner := testhelpers.CreateTestUser(s.deps.DB)
	other := testhelpers.CreateTestUser(s.deps.DB)
	coll := s.createCollectionViaService(owner, "Not My Delete", true)

	ctx := testhelpers.CtxWithUser(other)
	req := &DeleteCollectionHandlerRequest{Slug: coll.Slug}
	_, err := s.handler.DeleteCollectionHandler(ctx, req)
	testhelpers.AssertHumaError(s.T(), err, 403)
}

func (s *CollectionHandlerIntegrationSuite) TestDeleteCollection_AdminCanDelete() {
	owner := testhelpers.CreateTestUser(s.deps.DB)
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	coll := s.createCollectionViaService(owner, "Admin Deletable", true)

	ctx := testhelpers.CtxWithUser(admin)
	req := &DeleteCollectionHandlerRequest{Slug: coll.Slug}
	_, err := s.handler.DeleteCollectionHandler(ctx, req)
	s.NoError(err)

	// Verify deleted
	getReq := &GetCollectionHandlerRequest{Slug: coll.Slug}
	_, err = s.handler.GetCollectionHandler(context.Background(), getReq)
	testhelpers.AssertHumaError(s.T(), err, 404)
}

func (s *CollectionHandlerIntegrationSuite) TestDeleteCollection_NotFound() {
	user := testhelpers.CreateTestUser(s.deps.DB)
	ctx := testhelpers.CtxWithUser(user)

	req := &DeleteCollectionHandlerRequest{Slug: "nonexistent-slug"}
	_, err := s.handler.DeleteCollectionHandler(ctx, req)
	testhelpers.AssertHumaError(s.T(), err, 404)
}

// ============================================================================
// AddItemHandler
// ============================================================================

func (s *CollectionHandlerIntegrationSuite) TestAddItem_Success() {
	user := testhelpers.CreateTestUser(s.deps.DB)
	coll := s.createCollectionViaService(user, "Add Item Coll", true)
	artist := testhelpers.CreateArtist(s.deps.DB, "Item Artist")

	ctx := testhelpers.CtxWithUser(user)
	req := &AddItemHandlerRequest{Slug: coll.Slug}
	req.Body.EntityType = "artist"
	req.Body.EntityID = artist.ID

	resp, err := s.handler.AddItemHandler(ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.Equal("artist", resp.Body.EntityType)
	s.Equal(artist.ID, resp.Body.EntityID)
}

func (s *CollectionHandlerIntegrationSuite) TestAddItem_WithNotes() {
	user := testhelpers.CreateTestUser(s.deps.DB)
	coll := s.createCollectionViaService(user, "Notes Coll", true)
	artist := testhelpers.CreateArtist(s.deps.DB, "Notes Artist")

	ctx := testhelpers.CtxWithUser(user)
	notes := "Great live performances"
	req := &AddItemHandlerRequest{Slug: coll.Slug}
	req.Body.EntityType = "artist"
	req.Body.EntityID = artist.ID
	req.Body.Notes = &notes

	resp, err := s.handler.AddItemHandler(ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.NotNil(resp.Body.Notes)
	s.Equal("Great live performances", *resp.Body.Notes)
}

func (s *CollectionHandlerIntegrationSuite) TestAddItem_VenueEntity() {
	user := testhelpers.CreateTestUser(s.deps.DB)
	coll := s.createCollectionViaService(user, "Venue Coll", true)
	venue := testhelpers.CreateVerifiedVenue(s.deps.DB, "Item Venue", "Phoenix", "AZ")

	ctx := testhelpers.CtxWithUser(user)
	req := &AddItemHandlerRequest{Slug: coll.Slug}
	req.Body.EntityType = "venue"
	req.Body.EntityID = venue.ID

	resp, err := s.handler.AddItemHandler(ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.Equal("venue", resp.Body.EntityType)
	s.Equal(venue.ID, resp.Body.EntityID)
}

func (s *CollectionHandlerIntegrationSuite) TestAddItem_NoAuth() {
	user := testhelpers.CreateTestUser(s.deps.DB)
	coll := s.createCollectionViaService(user, "NoAuth Item", true)

	req := &AddItemHandlerRequest{Slug: coll.Slug}
	req.Body.EntityType = "artist"
	req.Body.EntityID = 1

	_, err := s.handler.AddItemHandler(context.Background(), req)
	testhelpers.AssertHumaError(s.T(), err, 401)
}

func (s *CollectionHandlerIntegrationSuite) TestAddItem_MissingEntityType() {
	user := testhelpers.CreateTestUser(s.deps.DB)
	coll := s.createCollectionViaService(user, "Missing Type", true)

	ctx := testhelpers.CtxWithUser(user)
	req := &AddItemHandlerRequest{Slug: coll.Slug}
	req.Body.EntityType = ""
	req.Body.EntityID = 1

	_, err := s.handler.AddItemHandler(ctx, req)
	testhelpers.AssertHumaError(s.T(), err, 422)
}

func (s *CollectionHandlerIntegrationSuite) TestAddItem_MissingEntityID() {
	user := testhelpers.CreateTestUser(s.deps.DB)
	coll := s.createCollectionViaService(user, "Missing ID", true)

	ctx := testhelpers.CtxWithUser(user)
	req := &AddItemHandlerRequest{Slug: coll.Slug}
	req.Body.EntityType = "artist"
	req.Body.EntityID = 0

	_, err := s.handler.AddItemHandler(ctx, req)
	testhelpers.AssertHumaError(s.T(), err, 422)
}

func (s *CollectionHandlerIntegrationSuite) TestAddItem_NotOwner() {
	owner := testhelpers.CreateTestUser(s.deps.DB)
	other := testhelpers.CreateTestUser(s.deps.DB)
	coll := s.createCollectionViaService(owner, "Not My Add", true)
	artist := testhelpers.CreateArtist(s.deps.DB, "Blocked Artist")

	ctx := testhelpers.CtxWithUser(other)
	req := &AddItemHandlerRequest{Slug: coll.Slug}
	req.Body.EntityType = "artist"
	req.Body.EntityID = artist.ID

	_, err := s.handler.AddItemHandler(ctx, req)
	testhelpers.AssertHumaError(s.T(), err, 403)
}

func (s *CollectionHandlerIntegrationSuite) TestAddItem_CollectionNotFound() {
	user := testhelpers.CreateTestUser(s.deps.DB)
	ctx := testhelpers.CtxWithUser(user)

	req := &AddItemHandlerRequest{Slug: "nonexistent"}
	req.Body.EntityType = "artist"
	req.Body.EntityID = 1

	_, err := s.handler.AddItemHandler(ctx, req)
	testhelpers.AssertHumaError(s.T(), err, 404)
}

func (s *CollectionHandlerIntegrationSuite) TestAddItem_DuplicateItem() {
	user := testhelpers.CreateTestUser(s.deps.DB)
	coll := s.createCollectionViaService(user, "Dup Item", true)
	artist := testhelpers.CreateArtist(s.deps.DB, "Dup Artist")

	ctx := testhelpers.CtxWithUser(user)

	// Add the item first
	req := &AddItemHandlerRequest{Slug: coll.Slug}
	req.Body.EntityType = "artist"
	req.Body.EntityID = artist.ID
	_, err := s.handler.AddItemHandler(ctx, req)
	s.Require().NoError(err)

	// Try to add it again
	req2 := &AddItemHandlerRequest{Slug: coll.Slug}
	req2.Body.EntityType = "artist"
	req2.Body.EntityID = artist.ID
	_, err = s.handler.AddItemHandler(ctx, req2)
	testhelpers.AssertHumaError(s.T(), err, 409)
}

// ============================================================================
// RemoveItemHandler
// ============================================================================

func (s *CollectionHandlerIntegrationSuite) TestRemoveItem_Success() {
	user := testhelpers.CreateTestUser(s.deps.DB)
	coll := s.createCollectionViaService(user, "Remove Item", true)
	artist := testhelpers.CreateArtist(s.deps.DB, "Removable Artist")

	// Add item via service
	item, err := s.deps.CollectionService.AddItem(coll.Slug, user.ID, &contracts.AddCollectionItemRequest{
		EntityType: "artist",
		EntityID:   artist.ID,
	})
	s.Require().NoError(err)

	ctx := testhelpers.CtxWithUser(user)
	req := &RemoveItemHandlerRequest{
		Slug:   coll.Slug,
		ItemID: fmt.Sprintf("%d", item.ID),
	}
	_, err = s.handler.RemoveItemHandler(ctx, req)
	s.NoError(err)
}

func (s *CollectionHandlerIntegrationSuite) TestRemoveItem_NoAuth() {
	req := &RemoveItemHandlerRequest{Slug: "some-slug", ItemID: "1"}
	_, err := s.handler.RemoveItemHandler(context.Background(), req)
	testhelpers.AssertHumaError(s.T(), err, 401)
}

func (s *CollectionHandlerIntegrationSuite) TestRemoveItem_InvalidItemID() {
	user := testhelpers.CreateTestUser(s.deps.DB)
	ctx := testhelpers.CtxWithUser(user)

	req := &RemoveItemHandlerRequest{Slug: "some-slug", ItemID: "not-a-number"}
	_, err := s.handler.RemoveItemHandler(ctx, req)
	testhelpers.AssertHumaError(s.T(), err, 400)
}

func (s *CollectionHandlerIntegrationSuite) TestRemoveItem_NotOwner() {
	owner := testhelpers.CreateTestUser(s.deps.DB)
	other := testhelpers.CreateTestUser(s.deps.DB)
	coll := s.createCollectionViaService(owner, "Not My Remove", true)
	artist := testhelpers.CreateArtist(s.deps.DB, "Not My Artist")

	item, err := s.deps.CollectionService.AddItem(coll.Slug, owner.ID, &contracts.AddCollectionItemRequest{
		EntityType: "artist",
		EntityID:   artist.ID,
	})
	s.Require().NoError(err)

	ctx := testhelpers.CtxWithUser(other)
	req := &RemoveItemHandlerRequest{
		Slug:   coll.Slug,
		ItemID: fmt.Sprintf("%d", item.ID),
	}
	_, err = s.handler.RemoveItemHandler(ctx, req)
	testhelpers.AssertHumaError(s.T(), err, 403)
}

func (s *CollectionHandlerIntegrationSuite) TestRemoveItem_CollectionNotFound() {
	user := testhelpers.CreateTestUser(s.deps.DB)
	ctx := testhelpers.CtxWithUser(user)

	req := &RemoveItemHandlerRequest{Slug: "nonexistent", ItemID: "1"}
	_, err := s.handler.RemoveItemHandler(ctx, req)
	testhelpers.AssertHumaError(s.T(), err, 404)
}

// ============================================================================
// ReorderItemsHandler
// ============================================================================

func (s *CollectionHandlerIntegrationSuite) TestReorderItems_Success() {
	user := testhelpers.CreateTestUser(s.deps.DB)
	coll := s.createCollectionViaService(user, "Reorder Coll", true)
	artist1 := testhelpers.CreateArtist(s.deps.DB, "Reorder Artist 1")
	artist2 := testhelpers.CreateArtist(s.deps.DB, "Reorder Artist 2")

	item1, err := s.deps.CollectionService.AddItem(coll.Slug, user.ID, &contracts.AddCollectionItemRequest{
		EntityType: "artist",
		EntityID:   artist1.ID,
	})
	s.Require().NoError(err)

	item2, err := s.deps.CollectionService.AddItem(coll.Slug, user.ID, &contracts.AddCollectionItemRequest{
		EntityType: "artist",
		EntityID:   artist2.ID,
	})
	s.Require().NoError(err)

	ctx := testhelpers.CtxWithUser(user)
	req := &ReorderItemsHandlerRequest{Slug: coll.Slug}
	req.Body.Items = []contracts.ReorderItem{
		{ItemID: item1.ID, Position: 2},
		{ItemID: item2.ID, Position: 1},
	}

	_, err = s.handler.ReorderItemsHandler(ctx, req)
	s.NoError(err)
}

func (s *CollectionHandlerIntegrationSuite) TestReorderItems_NoAuth() {
	req := &ReorderItemsHandlerRequest{Slug: "some-slug"}
	_, err := s.handler.ReorderItemsHandler(context.Background(), req)
	testhelpers.AssertHumaError(s.T(), err, 401)
}

func (s *CollectionHandlerIntegrationSuite) TestReorderItems_NotOwner() {
	owner := testhelpers.CreateTestUser(s.deps.DB)
	other := testhelpers.CreateTestUser(s.deps.DB)
	coll := s.createCollectionViaService(owner, "Not My Reorder", true)

	ctx := testhelpers.CtxWithUser(other)
	req := &ReorderItemsHandlerRequest{Slug: coll.Slug}
	req.Body.Items = []contracts.ReorderItem{
		{ItemID: 1, Position: 1},
	}

	_, err := s.handler.ReorderItemsHandler(ctx, req)
	testhelpers.AssertHumaError(s.T(), err, 403)
}

// ============================================================================
// SubscribeHandler
// ============================================================================

func (s *CollectionHandlerIntegrationSuite) TestSubscribe_Success() {
	owner := testhelpers.CreateTestUser(s.deps.DB)
	subscriber := testhelpers.CreateTestUser(s.deps.DB)
	coll := s.createCollectionViaService(owner, "Subscribable", true)

	ctx := testhelpers.CtxWithUser(subscriber)
	req := &SubscribeHandlerRequest{Slug: coll.Slug}
	_, err := s.handler.SubscribeHandler(ctx, req)
	s.NoError(err)

	// Verify subscription via get endpoint
	getReq := &GetCollectionHandlerRequest{Slug: coll.Slug}
	getResp, err := s.handler.GetCollectionHandler(ctx, getReq)
	s.NoError(err)
	s.True(getResp.Body.IsSubscribed)
}

func (s *CollectionHandlerIntegrationSuite) TestSubscribe_NoAuth() {
	req := &SubscribeHandlerRequest{Slug: "some-slug"}
	_, err := s.handler.SubscribeHandler(context.Background(), req)
	testhelpers.AssertHumaError(s.T(), err, 401)
}

func (s *CollectionHandlerIntegrationSuite) TestSubscribe_CollectionNotFound() {
	user := testhelpers.CreateTestUser(s.deps.DB)
	ctx := testhelpers.CtxWithUser(user)

	req := &SubscribeHandlerRequest{Slug: "nonexistent"}
	_, err := s.handler.SubscribeHandler(ctx, req)
	testhelpers.AssertHumaError(s.T(), err, 404)
}

// ============================================================================
// UnsubscribeHandler
// ============================================================================

func (s *CollectionHandlerIntegrationSuite) TestUnsubscribe_Success() {
	owner := testhelpers.CreateTestUser(s.deps.DB)
	subscriber := testhelpers.CreateTestUser(s.deps.DB)
	coll := s.createCollectionViaService(owner, "Unsubscribable", true)

	// Subscribe first
	err := s.deps.CollectionService.Subscribe(coll.Slug, subscriber.ID)
	s.Require().NoError(err)

	ctx := testhelpers.CtxWithUser(subscriber)
	req := &UnsubscribeHandlerRequest{Slug: coll.Slug}
	_, err = s.handler.UnsubscribeHandler(ctx, req)
	s.NoError(err)

	// Verify unsubscription
	getReq := &GetCollectionHandlerRequest{Slug: coll.Slug}
	getResp, err := s.handler.GetCollectionHandler(ctx, getReq)
	s.NoError(err)
	s.False(getResp.Body.IsSubscribed)
}

func (s *CollectionHandlerIntegrationSuite) TestUnsubscribe_NoAuth() {
	req := &UnsubscribeHandlerRequest{Slug: "some-slug"}
	_, err := s.handler.UnsubscribeHandler(context.Background(), req)
	testhelpers.AssertHumaError(s.T(), err, 401)
}

func (s *CollectionHandlerIntegrationSuite) TestUnsubscribe_CollectionNotFound() {
	user := testhelpers.CreateTestUser(s.deps.DB)
	ctx := testhelpers.CtxWithUser(user)

	req := &UnsubscribeHandlerRequest{Slug: "nonexistent"}
	_, err := s.handler.UnsubscribeHandler(ctx, req)
	testhelpers.AssertHumaError(s.T(), err, 404)
}

// ============================================================================
// SetFeaturedHandler
// ============================================================================

func (s *CollectionHandlerIntegrationSuite) TestSetFeatured_AdminSuccess() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	user := testhelpers.CreateTestUser(s.deps.DB)
	coll := s.createCollectionViaService(user, "Featureable", true)

	ctx := testhelpers.CtxWithUser(admin)
	req := &SetFeaturedHandlerRequest{Slug: coll.Slug}
	req.Body.Featured = true

	_, err := s.handler.SetFeaturedHandler(ctx, req)
	s.NoError(err)

	// Verify it's featured
	getReq := &GetCollectionHandlerRequest{Slug: coll.Slug}
	getResp, err := s.handler.GetCollectionHandler(context.Background(), getReq)
	s.NoError(err)
	s.True(getResp.Body.IsFeatured)
}

func (s *CollectionHandlerIntegrationSuite) TestSetFeatured_Unfeature() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	user := testhelpers.CreateTestUser(s.deps.DB)
	coll := s.createCollectionViaService(user, "Unfeature Me", true)

	// Feature first
	err := s.deps.CollectionService.SetFeatured(coll.Slug, true)
	s.Require().NoError(err)

	ctx := testhelpers.CtxWithUser(admin)
	req := &SetFeaturedHandlerRequest{Slug: coll.Slug}
	req.Body.Featured = false

	_, err = s.handler.SetFeaturedHandler(ctx, req)
	s.NoError(err)

	// Verify it's no longer featured
	getReq := &GetCollectionHandlerRequest{Slug: coll.Slug}
	getResp, err := s.handler.GetCollectionHandler(context.Background(), getReq)
	s.NoError(err)
	s.False(getResp.Body.IsFeatured)
}

func (s *CollectionHandlerIntegrationSuite) TestSetFeatured_NotFound() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	ctx := testhelpers.CtxWithUser(admin)

	req := &SetFeaturedHandlerRequest{Slug: "nonexistent"}
	req.Body.Featured = true

	_, err := s.handler.SetFeaturedHandler(ctx, req)
	testhelpers.AssertHumaError(s.T(), err, 404)
}

// ============================================================================
// GetUserCollectionsHandler
// ============================================================================

func (s *CollectionHandlerIntegrationSuite) TestGetUserCollections_Success() {
	user := testhelpers.CreateTestUser(s.deps.DB)
	s.createCollectionViaService(user, "My Coll A", true)
	s.createCollectionViaService(user, "My Coll B", false)

	ctx := testhelpers.CtxWithUser(user)
	req := &GetUserCollectionsHandlerRequest{}
	resp, err := s.handler.GetUserCollectionsHandler(ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.Equal(int64(2), resp.Body.Total)
}

func (s *CollectionHandlerIntegrationSuite) TestGetUserCollections_Empty() {
	user := testhelpers.CreateTestUser(s.deps.DB)
	ctx := testhelpers.CtxWithUser(user)

	req := &GetUserCollectionsHandlerRequest{}
	resp, err := s.handler.GetUserCollectionsHandler(ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.Equal(int64(0), resp.Body.Total)
}

func (s *CollectionHandlerIntegrationSuite) TestGetUserCollections_NoAuth() {
	req := &GetUserCollectionsHandlerRequest{}
	_, err := s.handler.GetUserCollectionsHandler(context.Background(), req)
	testhelpers.AssertHumaError(s.T(), err, 401)
}

func (s *CollectionHandlerIntegrationSuite) TestGetUserCollections_DoesNotIncludeOtherUsers() {
	user1 := testhelpers.CreateTestUser(s.deps.DB)
	user2 := testhelpers.CreateTestUser(s.deps.DB)
	s.createCollectionViaService(user1, "User1 Coll", true)
	s.createCollectionViaService(user2, "User2 Coll", true)

	ctx := testhelpers.CtxWithUser(user1)
	req := &GetUserCollectionsHandlerRequest{}
	resp, err := s.handler.GetUserCollectionsHandler(ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.Equal(int64(1), resp.Body.Total)
	s.Equal("User1 Coll", resp.Body.Collections[0].Title)
}

func (s *CollectionHandlerIntegrationSuite) TestGetUserCollections_WithLimit() {
	user := testhelpers.CreateTestUser(s.deps.DB)
	s.createCollectionViaService(user, "Limit A", true)
	s.createCollectionViaService(user, "Limit B", true)
	s.createCollectionViaService(user, "Limit C", true)

	ctx := testhelpers.CtxWithUser(user)
	req := &GetUserCollectionsHandlerRequest{Limit: 2}
	resp, err := s.handler.GetUserCollectionsHandler(ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.LessOrEqual(len(resp.Body.Collections), 2)
	s.Equal(int64(3), resp.Body.Total)
}

// ============================================================================
// CloneCollectionHandler (PSY-351)
// ============================================================================

// TestCloneCollection_CopiesItemsNotesAndPositions exercises the happy path:
// a public collection with items + notes + positions is copied faithfully
// into a new collection owned by the caller, with attribution back to the
// original. This is the primary acceptance criterion.
//
// PSY-356 note: createCollectionViaService(..., true) seeds 3 items as part
// of the publish-gate dance. Those land at positions 0..2, so the
// test-added items are at positions 3..5. The assertions index relative to
// the start of the test-added range.
func (s *CollectionHandlerIntegrationSuite) TestCloneCollection_CopiesItemsNotesAndPositions() {
	owner := testhelpers.CreateTestUser(s.deps.DB)
	cloner := testhelpers.CreateTestUser(s.deps.DB)
	src := s.createCollectionViaService(owner, "Source Collection", true)

	// Add three items with notes; reorder to confirm position is preserved.
	a1 := testhelpers.CreateArtist(s.deps.DB, "Artist One")
	a2 := testhelpers.CreateArtist(s.deps.DB, "Artist Two")
	a3 := testhelpers.CreateArtist(s.deps.DB, "Artist Three")
	notes1 := "first note"
	notes3 := "third note"
	_, err := s.deps.CollectionService.AddItem(src.Slug, owner.ID, &contracts.AddCollectionItemRequest{
		EntityType: "artist", EntityID: a1.ID, Notes: &notes1,
	})
	s.Require().NoError(err)
	_, err = s.deps.CollectionService.AddItem(src.Slug, owner.ID, &contracts.AddCollectionItemRequest{
		EntityType: "artist", EntityID: a2.ID,
	})
	s.Require().NoError(err)
	_, err = s.deps.CollectionService.AddItem(src.Slug, owner.ID, &contracts.AddCollectionItemRequest{
		EntityType: "artist", EntityID: a3.ID, Notes: &notes3,
	})
	s.Require().NoError(err)

	ctx := testhelpers.CtxWithUser(cloner)
	req := &CloneCollectionHandlerRequest{Slug: src.Slug}
	resp, err := s.handler.CloneCollectionHandler(ctx, req)
	s.Require().NoError(err)
	s.Require().NotNil(resp)
	s.Require().NotNil(resp.Body)

	// New collection is owned by the cloner, distinct from source.
	s.NotEqual(src.ID, resp.Body.ID)
	s.Equal(cloner.ID, resp.Body.CreatorID)
	s.Equal("Source Collection (fork)", resp.Body.Title)

	// Attribution back to original.
	s.Require().NotNil(resp.Body.ForkedFromCollectionID)
	s.Equal(src.ID, *resp.Body.ForkedFromCollectionID)
	s.Require().NotNil(resp.Body.ForkedFrom)
	s.Equal(src.ID, resp.Body.ForkedFrom.ID)
	s.Equal("Source Collection", resp.Body.ForkedFrom.Title)
	s.Equal(owner.ID, resp.Body.ForkedFrom.CreatorID)

	// Items copied: 3 gate-seeded + 3 explicit = 6 total. Index into the
	// test-added range (positions 3..5).
	s.Require().Len(resp.Body.Items, MinPublicCollectionItems+3)
	startIdx := MinPublicCollectionItems
	s.Equal(a1.ID, resp.Body.Items[startIdx].EntityID)
	s.Require().NotNil(resp.Body.Items[startIdx].Notes)
	s.Equal("first note", *resp.Body.Items[startIdx].Notes)
	s.Equal(a2.ID, resp.Body.Items[startIdx+1].EntityID)
	s.Nil(resp.Body.Items[startIdx+1].Notes)
	s.Equal(a3.ID, resp.Body.Items[startIdx+2].EntityID)
	s.Require().NotNil(resp.Body.Items[startIdx+2].Notes)
	s.Equal("third note", *resp.Body.Items[startIdx+2].Notes)

	// Positions are strictly increasing (matches source order).
	s.Less(resp.Body.Items[startIdx].Position, resp.Body.Items[startIdx+1].Position)
	s.Less(resp.Body.Items[startIdx+1].Position, resp.Body.Items[startIdx+2].Position)
}

// TestCloneCollection_NoAuth covers the authn boundary — the endpoint
// must reject anonymous callers.
func (s *CollectionHandlerIntegrationSuite) TestCloneCollection_NoAuth() {
	owner := testhelpers.CreateTestUser(s.deps.DB)
	src := s.createCollectionViaService(owner, "No Auth Clone", true)

	req := &CloneCollectionHandlerRequest{Slug: src.Slug}
	_, err := s.handler.CloneCollectionHandler(context.Background(), req)
	testhelpers.AssertHumaError(s.T(), err, 401)
}

// TestCloneCollection_PrivateSourceForbidden ensures the visibility check
// matches GetBySlug — non-owners cannot clone a private collection.
func (s *CollectionHandlerIntegrationSuite) TestCloneCollection_PrivateSourceForbidden() {
	owner := testhelpers.CreateTestUser(s.deps.DB)
	other := testhelpers.CreateTestUser(s.deps.DB)
	private := s.createCollectionViaService(owner, "Private Source", false)

	ctx := testhelpers.CtxWithUser(other)
	req := &CloneCollectionHandlerRequest{Slug: private.Slug}
	_, err := s.handler.CloneCollectionHandler(ctx, req)
	testhelpers.AssertHumaError(s.T(), err, 403)
}

// TestCloneCollection_SourceNotFound ensures unknown slugs return 404.
func (s *CollectionHandlerIntegrationSuite) TestCloneCollection_SourceNotFound() {
	user := testhelpers.CreateTestUser(s.deps.DB)
	ctx := testhelpers.CtxWithUser(user)
	req := &CloneCollectionHandlerRequest{Slug: "nope-not-real"}
	_, err := s.handler.CloneCollectionHandler(ctx, req)
	testhelpers.AssertHumaError(s.T(), err, 404)
}

// TestCloneCollection_OwnerCanCloneOwnPrivate ensures the visibility check
// allows owners to clone their own private collections (matching GetBySlug
// — public OR owner). UI can still hide the button per the ticket.
func (s *CollectionHandlerIntegrationSuite) TestCloneCollection_OwnerCanCloneOwnPrivate() {
	owner := testhelpers.CreateTestUser(s.deps.DB)
	src := s.createCollectionViaService(owner, "Mine Private", false)

	ctx := testhelpers.CtxWithUser(owner)
	req := &CloneCollectionHandlerRequest{Slug: src.Slug}
	resp, err := s.handler.CloneCollectionHandler(ctx, req)
	s.Require().NoError(err)
	s.Equal(owner.ID, resp.Body.CreatorID)
	s.Require().NotNil(resp.Body.ForkedFromCollectionID)
	s.Equal(src.ID, *resp.Body.ForkedFromCollectionID)
}

// TestCloneCollection_DeletingOriginalSetsForkedFromNull verifies the
// ON DELETE SET NULL behavior on the new FK. Deleting the source must
// not cascade-delete forks; the cloned page should still load with the
// FK reset and ForkedFrom = nil so the frontend renders fallback copy.
// This is the explicit user requirement for the FK semantics.
func (s *CollectionHandlerIntegrationSuite) TestCloneCollection_DeletingOriginalSetsForkedFromNull() {
	owner := testhelpers.CreateTestUser(s.deps.DB)
	cloner := testhelpers.CreateTestUser(s.deps.DB)
	src := s.createCollectionViaService(owner, "Doomed Source", true)

	// Clone first.
	ctx := testhelpers.CtxWithUser(cloner)
	cloneReq := &CloneCollectionHandlerRequest{Slug: src.Slug}
	cloneResp, err := s.handler.CloneCollectionHandler(ctx, cloneReq)
	s.Require().NoError(err)
	cloneSlug := cloneResp.Body.Slug

	// Delete the source.
	delErr := s.deps.CollectionService.DeleteCollection(src.Slug, owner.ID, false)
	s.Require().NoError(delErr)

	// Clone still exists; ForkedFromCollectionID must be NULL post-cascade.
	getReq := &GetCollectionHandlerRequest{Slug: cloneSlug}
	getResp, err := s.handler.GetCollectionHandler(context.Background(), getReq)
	s.Require().NoError(err)
	s.Require().NotNil(getResp)
	s.Nil(getResp.Body.ForkedFromCollectionID,
		"ON DELETE SET NULL should clear the FK when the source is deleted")
	s.Nil(getResp.Body.ForkedFrom,
		"ForkedFrom should be nil so the frontend renders fallback copy")
}

// TestCloneCollection_OriginalShowsForksCount verifies the public fork
// count on the original collection. After two clones, the source's
// `forks_count` should be 2.
func (s *CollectionHandlerIntegrationSuite) TestCloneCollection_OriginalShowsForksCount() {
	owner := testhelpers.CreateTestUser(s.deps.DB)
	cloner1 := testhelpers.CreateTestUser(s.deps.DB)
	cloner2 := testhelpers.CreateTestUser(s.deps.DB)
	src := s.createCollectionViaService(owner, "Forky Source", true)

	// Two clones from different users.
	for _, c := range []*authm.User{cloner1, cloner2} {
		ctx := testhelpers.CtxWithUser(c)
		req := &CloneCollectionHandlerRequest{Slug: src.Slug}
		_, err := s.handler.CloneCollectionHandler(ctx, req)
		s.Require().NoError(err)
	}

	// Reload the source via the public detail endpoint.
	getReq := &GetCollectionHandlerRequest{Slug: src.Slug}
	getResp, err := s.handler.GetCollectionHandler(context.Background(), getReq)
	s.Require().NoError(err)
	s.Equal(2, getResp.Body.ForksCount,
		"public forks_count should reflect clone count")
}

// ============================================================================
// PSY-356: publish-gate handler integration
// ============================================================================

func (s *CollectionHandlerIntegrationSuite) TestCreateCollection_PublicAtCreateRejectedAs422() {
	user := testhelpers.CreateTestUser(s.deps.DB)
	ctx := testhelpers.CtxWithUser(user)

	req := &CreateCollectionHandlerRequest{}
	req.Body.Title = "Public At Create"
	req.Body.IsPublic = true

	_, err := s.handler.CreateCollectionHandler(ctx, req)
	testhelpers.AssertHumaError(s.T(), err, 422)
}

func (s *CollectionHandlerIntegrationSuite) TestUpdateCollection_FlipPublicBelowGateRejectedAs422() {
	user := testhelpers.CreateTestUser(s.deps.DB)
	priv := s.createCollectionViaService(user, "Flip Below Gate", false)

	ctx := testhelpers.CtxWithUser(user)
	pub := true
	req := &UpdateCollectionHandlerRequest{Slug: priv.Slug}
	req.Body.IsPublic = &pub

	_, err := s.handler.UpdateCollectionHandler(ctx, req)
	testhelpers.AssertHumaError(s.T(), err, 422)
}

// ============================================================================
// shared.MapCollectionError
// ============================================================================

func (s *CollectionHandlerIntegrationSuite) TestMapCollectionError_NotFound() {
	err := shared.MapCollectionError(fmt.Errorf("generic error"))
	s.Nil(err, "non-CollectionError should return nil")
}

// ============================================================================
// PSY-354: collection tag endpoints
// ============================================================================

// promoteContributorForTags lifts a user above the new_user trust tier so the
// inline-tag-creation path passes (otherwise free-form tag names get a 403
// from the tag service's createTagInline gate). The trust-tier gate itself
// is covered in catalog/tag_service_test.go — these tests focus on the
// collection-side behavior.
func (s *CollectionHandlerIntegrationSuite) promoteContributorForTags(user *authm.User) {
	s.Require().NoError(s.deps.DB.Model(&authm.User{}).
		Where("id = ?", user.ID).
		Update("user_tier", "contributor").Error)
}

func (s *CollectionHandlerIntegrationSuite) TestAddCollectionTag_Success() {
	user := testhelpers.CreateTestUser(s.deps.DB)
	s.promoteContributorForTags(user)
	coll := s.createCollectionViaService(user, "Tagged Coll", false)

	ctx := testhelpers.CtxWithUser(user)
	req := &AddCollectionTagHandlerRequest{Slug: coll.Slug}
	req.Body.TagName = "first-tag"

	resp, err := s.handler.AddCollectionTagHandler(ctx, req)
	s.NoError(err)
	s.Require().NotNil(resp)
	s.Require().Len(resp.Body.Tags, 1)
	s.Equal("first-tag", resp.Body.Tags[0].Name)
}

func (s *CollectionHandlerIntegrationSuite) TestAddCollectionTag_NoAuth() {
	user := testhelpers.CreateTestUser(s.deps.DB)
	coll := s.createCollectionViaService(user, "Need Auth", false)

	req := &AddCollectionTagHandlerRequest{Slug: coll.Slug}
	req.Body.TagName = "no-auth"

	_, err := s.handler.AddCollectionTagHandler(context.Background(), req)
	testhelpers.AssertHumaError(s.T(), err, 401)
}

func (s *CollectionHandlerIntegrationSuite) TestAddCollectionTag_MissingArgs() {
	user := testhelpers.CreateTestUser(s.deps.DB)
	s.promoteContributorForTags(user)
	coll := s.createCollectionViaService(user, "Missing Args", false)

	ctx := testhelpers.CtxWithUser(user)
	req := &AddCollectionTagHandlerRequest{Slug: coll.Slug}
	// no tag_id, no tag_name

	_, err := s.handler.AddCollectionTagHandler(ctx, req)
	testhelpers.AssertHumaError(s.T(), err, 422)
}

func (s *CollectionHandlerIntegrationSuite) TestAddCollectionTag_NonOwner_Forbidden() {
	owner := testhelpers.CreateTestUser(s.deps.DB)
	stranger := testhelpers.CreateTestUser(s.deps.DB)
	s.promoteContributorForTags(stranger)
	coll := s.createCollectionViaService(owner, "Solo Owner", false)
	// createCollectionViaService leaves Collaborative=false (CreateCollection's
	// GORM-bool dance), so the stranger cannot tag the collection.

	ctx := testhelpers.CtxWithUser(stranger)
	req := &AddCollectionTagHandlerRequest{Slug: coll.Slug}
	req.Body.TagName = "stranger-tag"

	_, err := s.handler.AddCollectionTagHandler(ctx, req)
	testhelpers.AssertHumaError(s.T(), err, 403)
}

func (s *CollectionHandlerIntegrationSuite) TestAddCollectionTag_LimitExceeded() {
	user := testhelpers.CreateTestUser(s.deps.DB)
	s.promoteContributorForTags(user)
	coll := s.createCollectionViaService(user, "Capped Collection", false)
	ctx := testhelpers.CtxWithUser(user)

	for i := 0; i < contracts.MaxCollectionTags; i++ {
		r := &AddCollectionTagHandlerRequest{Slug: coll.Slug}
		r.Body.TagName = fmt.Sprintf("cap-%d", i)
		_, err := s.handler.AddCollectionTagHandler(ctx, r)
		s.Require().NoError(err)
	}

	r := &AddCollectionTagHandlerRequest{Slug: coll.Slug}
	r.Body.TagName = "one-too-many"
	_, err := s.handler.AddCollectionTagHandler(ctx, r)
	testhelpers.AssertHumaError(s.T(), err, 422)
}

func (s *CollectionHandlerIntegrationSuite) TestRemoveCollectionTag_Success() {
	user := testhelpers.CreateTestUser(s.deps.DB)
	s.promoteContributorForTags(user)
	coll := s.createCollectionViaService(user, "Remove Tag", false)
	ctx := testhelpers.CtxWithUser(user)

	addReq := &AddCollectionTagHandlerRequest{Slug: coll.Slug}
	addReq.Body.TagName = "to-remove"
	addResp, err := s.handler.AddCollectionTagHandler(ctx, addReq)
	s.Require().NoError(err)
	s.Require().Len(addResp.Body.Tags, 1)
	tagID := addResp.Body.Tags[0].TagID

	delReq := &RemoveCollectionTagHandlerRequest{
		Slug:  coll.Slug,
		TagID: fmt.Sprintf("%d", tagID),
	}
	_, err = s.handler.RemoveCollectionTagHandler(ctx, delReq)
	s.NoError(err)

	// Verify it's gone via the detail endpoint.
	getResp, err := s.handler.GetCollectionHandler(ctx, &GetCollectionHandlerRequest{Slug: coll.Slug})
	s.Require().NoError(err)
	s.Empty(getResp.Body.Tags)
}

func (s *CollectionHandlerIntegrationSuite) TestRemoveCollectionTag_NoAuth() {
	user := testhelpers.CreateTestUser(s.deps.DB)
	s.promoteContributorForTags(user)
	coll := s.createCollectionViaService(user, "Auth Needed Remove", false)
	ctx := testhelpers.CtxWithUser(user)

	addReq := &AddCollectionTagHandlerRequest{Slug: coll.Slug}
	addReq.Body.TagName = "tagged"
	addResp, err := s.handler.AddCollectionTagHandler(ctx, addReq)
	s.Require().NoError(err)
	tagID := addResp.Body.Tags[0].TagID

	delReq := &RemoveCollectionTagHandlerRequest{
		Slug:  coll.Slug,
		TagID: fmt.Sprintf("%d", tagID),
	}
	_, err = s.handler.RemoveCollectionTagHandler(context.Background(), delReq)
	testhelpers.AssertHumaError(s.T(), err, 401)
}

func (s *CollectionHandlerIntegrationSuite) TestRemoveCollectionTag_InvalidID() {
	user := testhelpers.CreateTestUser(s.deps.DB)
	coll := s.createCollectionViaService(user, "Invalid ID", false)

	delReq := &RemoveCollectionTagHandlerRequest{
		Slug:  coll.Slug,
		TagID: "not-a-number",
	}
	_, err := s.handler.RemoveCollectionTagHandler(testhelpers.CtxWithUser(user), delReq)
	testhelpers.AssertHumaError(s.T(), err, 400)
}

func (s *CollectionHandlerIntegrationSuite) TestGetCollection_SurfacesTags() {
	user := testhelpers.CreateTestUser(s.deps.DB)
	s.promoteContributorForTags(user)
	coll := s.createCollectionViaService(user, "Surface Tags", false)
	ctx := testhelpers.CtxWithUser(user)

	addReq := &AddCollectionTagHandlerRequest{Slug: coll.Slug}
	addReq.Body.TagName = "surfaced"
	_, err := s.handler.AddCollectionTagHandler(ctx, addReq)
	s.Require().NoError(err)

	getResp, err := s.handler.GetCollectionHandler(ctx, &GetCollectionHandlerRequest{Slug: coll.Slug})
	s.Require().NoError(err)
	s.Require().Len(getResp.Body.Tags, 1)
	s.Equal("surfaced", getResp.Body.Tags[0].Name)
}

func (s *CollectionHandlerIntegrationSuite) TestListCollections_TagFilter() {
	user := testhelpers.CreateTestUser(s.deps.DB)
	s.promoteContributorForTags(user)

	tagged := s.createCollectionViaService(user, "Tagged Browse", true)
	s.createCollectionViaService(user, "Untagged Browse", true)

	ctx := testhelpers.CtxWithUser(user)
	addReq := &AddCollectionTagHandlerRequest{Slug: tagged.Slug}
	addReq.Body.TagName = "browse-filter-tag"
	_, err := s.handler.AddCollectionTagHandler(ctx, addReq)
	s.Require().NoError(err)

	// Filter by the tag.
	listReq := &ListCollectionsHandlerRequest{Tag: "browse-filter-tag"}
	listResp, err := s.handler.ListCollectionsHandler(context.Background(), listReq)
	s.Require().NoError(err)
	s.Equal(int64(1), listResp.Body.Total)
	s.Require().Len(listResp.Body.Collections, 1)
	s.Equal(tagged.ID, listResp.Body.Collections[0].ID)

	// No filter — both surface.
	listReq = &ListCollectionsHandlerRequest{}
	listResp, err = s.handler.ListCollectionsHandler(context.Background(), listReq)
	s.Require().NoError(err)
	s.Equal(int64(2), listResp.Body.Total)
}

func (s *CollectionHandlerIntegrationSuite) TestListCollections_PopulatesTags() {
	user := testhelpers.CreateTestUser(s.deps.DB)
	s.promoteContributorForTags(user)
	tagged := s.createCollectionViaService(user, "Browse Tags", true)

	ctx := testhelpers.CtxWithUser(user)
	addReq := &AddCollectionTagHandlerRequest{Slug: tagged.Slug}
	addReq.Body.TagName = "browse-chip"
	_, err := s.handler.AddCollectionTagHandler(ctx, addReq)
	s.Require().NoError(err)

	listResp, err := s.handler.ListCollectionsHandler(context.Background(), &ListCollectionsHandlerRequest{})
	s.Require().NoError(err)
	s.Require().Len(listResp.Body.Collections, 1)
	s.Require().Len(listResp.Body.Collections[0].Tags, 1)
	s.Equal("browse-chip", listResp.Body.Collections[0].Tags[0].Name)
}
