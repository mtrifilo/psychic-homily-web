package catalog

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	"psychic-homily-backend/internal/models"
	"psychic-homily-backend/internal/services/contracts"
)

type TagHandlerIntegrationSuite struct {
	suite.Suite
	deps    *testhelpers.IntegrationDeps
	handler *TagHandler
}

func (s *TagHandlerIntegrationSuite) SetupSuite() {
	s.deps = testhelpers.SetupIntegrationDeps(s.T())
	s.handler = NewTagHandler(s.deps.TagService, s.deps.AuditLogService)
}

func (s *TagHandlerIntegrationSuite) TearDownTest() {
	testhelpers.CleanupTables(s.deps.DB)
	sqlDB, _ := s.deps.DB.DB()
	_, _ = sqlDB.Exec("DELETE FROM tag_votes")
	_, _ = sqlDB.Exec("DELETE FROM entity_tags")
	_, _ = sqlDB.Exec("DELETE FROM tag_aliases")
	_, _ = sqlDB.Exec("DELETE FROM tags")
}

func (s *TagHandlerIntegrationSuite) TearDownSuite() {
	s.deps.TestDB.Cleanup()
}

func TestTagHandlerIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	suite.Run(t, new(TagHandlerIntegrationSuite))
}

// --- Helpers ---

func (s *TagHandlerIntegrationSuite) createTagViaHandler(admin *models.User, name, category string) *CreateTagResponse {
	ctx := testhelpers.CtxWithUser(admin)
	req := &CreateTagRequest{}
	req.Body.Name = name
	req.Body.Category = category
	resp, err := s.handler.CreateTagHandler(ctx, req)
	s.Require().NoError(err)
	s.Require().NotNil(resp)
	return resp
}

// ============================================================================
// CreateTagHandler
// ============================================================================

func (s *TagHandlerIntegrationSuite) TestCreateTag_Success() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	ctx := testhelpers.CtxWithUser(admin)

	req := &CreateTagRequest{}
	req.Body.Name = "post-punk"
	req.Body.Category = models.TagCategoryGenre

	resp, err := s.handler.CreateTagHandler(ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.Equal("post-punk", resp.Body.Name)
	s.Equal("genre", resp.Body.Category)
	s.NotEmpty(resp.Body.Slug)
	s.NotZero(resp.Body.ID)

	// Verify created_by attribution
	s.NotNil(resp.Body.CreatedByUserID)
	s.Equal(admin.ID, *resp.Body.CreatedByUserID)
	// CreatedByUsername may be nil if test user has no username set
}

func (s *TagHandlerIntegrationSuite) TestCreateTag_CreatedByIncludedInGetResponse() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	created := s.createTagViaHandler(admin, "math-rock", models.TagCategoryGenre)

	// Fetch via GetTag and verify attribution persists
	ctx := testhelpers.CtxWithUser(admin)
	getReq := &GetTagRequest{TagID: fmt.Sprintf("%d", created.Body.ID)}
	getResp, err := s.handler.GetTagHandler(ctx, getReq)
	s.NoError(err)
	s.NotNil(getResp)
	s.NotNil(getResp.Body.CreatedByUserID)
	s.Equal(admin.ID, *getResp.Body.CreatedByUserID)
	// CreatedByUsername may be nil if test user has no username set
}

func (s *TagHandlerIntegrationSuite) TestCreateTag_WithDescription() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	ctx := testhelpers.CtxWithUser(admin)

	desc := "Music influenced by the post-punk movement"
	req := &CreateTagRequest{}
	req.Body.Name = "post-punk"
	req.Body.Category = models.TagCategoryGenre
	req.Body.Description = &desc

	resp, err := s.handler.CreateTagHandler(ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.NotNil(resp.Body.Description)
	s.Equal(desc, *resp.Body.Description)
}

func (s *TagHandlerIntegrationSuite) TestCreateTag_WithParent() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)

	// Create parent tag
	parent := s.createTagViaHandler(admin, "rock", models.TagCategoryGenre)

	// Create child tag with parent
	ctx := testhelpers.CtxWithUser(admin)
	req := &CreateTagRequest{}
	req.Body.Name = "post-punk"
	req.Body.Category = models.TagCategoryGenre
	req.Body.ParentID = &parent.Body.ID

	resp, err := s.handler.CreateTagHandler(ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.NotNil(resp.Body.ParentID)
	s.Equal(parent.Body.ID, *resp.Body.ParentID)
}

func (s *TagHandlerIntegrationSuite) TestCreateTag_Duplicate() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	s.createTagViaHandler(admin, "post-punk", models.TagCategoryGenre)

	ctx := testhelpers.CtxWithUser(admin)
	req := &CreateTagRequest{}
	req.Body.Name = "post-punk"
	req.Body.Category = models.TagCategoryGenre

	_, err := s.handler.CreateTagHandler(ctx, req)
	testhelpers.AssertHumaError(s.T(), err, 409)
}

func (s *TagHandlerIntegrationSuite) TestCreateTag_MissingName() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	ctx := testhelpers.CtxWithUser(admin)

	req := &CreateTagRequest{}
	req.Body.Name = ""
	req.Body.Category = models.TagCategoryGenre

	_, err := s.handler.CreateTagHandler(ctx, req)
	testhelpers.AssertHumaError(s.T(), err, 400)
}

func (s *TagHandlerIntegrationSuite) TestCreateTag_MissingCategory() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	ctx := testhelpers.CtxWithUser(admin)

	req := &CreateTagRequest{}
	req.Body.Name = "post-punk"
	req.Body.Category = ""

	_, err := s.handler.CreateTagHandler(ctx, req)
	testhelpers.AssertHumaError(s.T(), err, 400)
}

func (s *TagHandlerIntegrationSuite) TestCreateTag_NonAdmin() {
	user := testhelpers.CreateTestUser(s.deps.DB)
	ctx := testhelpers.CtxWithUser(user)

	req := &CreateTagRequest{}
	req.Body.Name = "post-punk"
	req.Body.Category = models.TagCategoryGenre

	_, err := s.handler.CreateTagHandler(ctx, req)
	testhelpers.AssertHumaError(s.T(), err, 403)
}

func (s *TagHandlerIntegrationSuite) TestCreateTag_NoAuth() {
	req := &CreateTagRequest{}
	req.Body.Name = "post-punk"
	req.Body.Category = models.TagCategoryGenre

	_, err := s.handler.CreateTagHandler(s.deps.Ctx, req)
	testhelpers.AssertHumaError(s.T(), err, 401)
}

// ============================================================================
// GetTagHandler
// ============================================================================

func (s *TagHandlerIntegrationSuite) TestGetTag_ByID() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	created := s.createTagViaHandler(admin, "shoegaze", models.TagCategoryGenre)

	req := &GetTagRequest{TagID: fmt.Sprintf("%d", created.Body.ID)}
	resp, err := s.handler.GetTagHandler(s.deps.Ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.Equal("shoegaze", resp.Body.Name)
	s.Equal(created.Body.ID, resp.Body.ID)
}

func (s *TagHandlerIntegrationSuite) TestGetTag_BySlug() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	created := s.createTagViaHandler(admin, "shoegaze", models.TagCategoryGenre)

	req := &GetTagRequest{TagID: created.Body.Slug}
	resp, err := s.handler.GetTagHandler(s.deps.Ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.Equal("shoegaze", resp.Body.Name)
}

func (s *TagHandlerIntegrationSuite) TestGetTag_NotFound() {
	req := &GetTagRequest{TagID: "99999"}
	_, err := s.handler.GetTagHandler(s.deps.Ctx, req)
	testhelpers.AssertHumaError(s.T(), err, 404)
}

func (s *TagHandlerIntegrationSuite) TestGetTag_NotFoundBySlug() {
	req := &GetTagRequest{TagID: "nonexistent-tag"}
	_, err := s.handler.GetTagHandler(s.deps.Ctx, req)
	testhelpers.AssertHumaError(s.T(), err, 404)
}

// ============================================================================
// GetTagDetailHandler (enriched detail endpoint)
// ============================================================================

func (s *TagHandlerIntegrationSuite) TestGetTagDetail_NotFound() {
	req := &GetTagDetailRequest{TagID: "does-not-exist"}
	_, err := s.handler.GetTagDetailHandler(s.deps.Ctx, req)
	testhelpers.AssertHumaError(s.T(), err, 404)
}

func (s *TagHandlerIntegrationSuite) TestGetTagDetail_Minimal_BySlug() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	created := s.createTagViaHandler(admin, "detail-tag", models.TagCategoryGenre)

	req := &GetTagDetailRequest{TagID: created.Body.Slug}
	resp, err := s.handler.GetTagDetailHandler(s.deps.Ctx, req)
	s.NoError(err)
	s.Require().NotNil(resp)
	s.Require().NotNil(resp.Body)

	s.Equal(created.Body.ID, resp.Body.ID)
	s.Equal("detail-tag", resp.Body.Name)
	// Zero-state sanity: breakdown initialized, collections non-nil (empty slices, not nil).
	s.Len(resp.Body.UsageBreakdown, len(models.TagEntityTypes))
	s.NotNil(resp.Body.Children)
	s.NotNil(resp.Body.TopContributors)
	s.NotNil(resp.Body.RelatedTags)
}

func (s *TagHandlerIntegrationSuite) TestGetTagDetail_DescriptionRendered() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	ctx := testhelpers.CtxWithUser(admin)

	desc := "Rendered **markdown** body."
	req := &CreateTagRequest{}
	req.Body.Name = "desc-tag"
	req.Body.Category = models.TagCategoryGenre
	req.Body.Description = &desc

	created, err := s.handler.CreateTagHandler(ctx, req)
	s.Require().NoError(err)

	detailReq := &GetTagDetailRequest{TagID: fmt.Sprintf("%d", created.Body.ID)}
	resp, err := s.handler.GetTagDetailHandler(s.deps.Ctx, detailReq)
	s.NoError(err)
	s.Require().NotNil(resp)

	s.Contains(resp.Body.DescriptionHTML, "<strong>markdown</strong>")
}

func (s *TagHandlerIntegrationSuite) TestGetTagDetail_ParentAndChildren() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	ctx := testhelpers.CtxWithUser(admin)

	parent := s.createTagViaHandler(admin, "post-punk", models.TagCategoryGenre)

	childReq := &CreateTagRequest{}
	childReq.Body.Name = "shoegaze"
	childReq.Body.Category = models.TagCategoryGenre
	parentID := parent.Body.ID
	childReq.Body.ParentID = &parentID
	childCreated, err := s.handler.CreateTagHandler(ctx, childReq)
	s.Require().NoError(err)

	// Parent detail has the child.
	parentResp, err := s.handler.GetTagDetailHandler(s.deps.Ctx, &GetTagDetailRequest{TagID: parent.Body.Slug})
	s.NoError(err)
	s.Require().NotNil(parentResp)
	s.Len(parentResp.Body.Children, 1)
	s.Equal(childCreated.Body.ID, parentResp.Body.Children[0].ID)

	// Child detail has the parent.
	childResp, err := s.handler.GetTagDetailHandler(s.deps.Ctx, &GetTagDetailRequest{TagID: childCreated.Body.Slug})
	s.NoError(err)
	s.Require().NotNil(childResp)
	s.Require().NotNil(childResp.Body.Parent)
	s.Equal(parent.Body.ID, childResp.Body.Parent.ID)
	s.Equal("post-punk", childResp.Body.Parent.Name)
}

func (s *TagHandlerIntegrationSuite) TestGetTagDetail_UsageBreakdownAcrossTypes() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	ctx := testhelpers.CtxWithUser(admin)
	tag := s.createTagViaHandler(admin, "multi-type", models.TagCategoryGenre)

	// Tag 1 artist and 1 venue.
	artist := testhelpers.CreateArtist(s.deps.DB, "Handler Test Band")
	venue := testhelpers.CreateVerifiedVenue(s.deps.DB, "Handler Test Venue", "Phoenix", "AZ")

	applyReq := func(entityType string, entityID uint) {
		req := &AddTagToEntityRequest{
			EntityType: entityType,
			EntityID:   fmt.Sprintf("%d", entityID),
		}
		req.Body.TagID = tag.Body.ID
		_, err := s.handler.AddTagToEntityHandler(ctx, req)
		s.Require().NoError(err)
	}
	applyReq(models.TagEntityArtist, artist.ID)
	applyReq(models.TagEntityVenue, venue.ID)

	resp, err := s.handler.GetTagDetailHandler(s.deps.Ctx, &GetTagDetailRequest{TagID: tag.Body.Slug})
	s.NoError(err)
	s.Require().NotNil(resp)

	s.Equal(int64(1), resp.Body.UsageBreakdown["artist"])
	s.Equal(int64(1), resp.Body.UsageBreakdown["venue"])
	s.Equal(int64(0), resp.Body.UsageBreakdown["show"])
	s.Equal(int64(0), resp.Body.UsageBreakdown["release"])
	s.Equal(int64(0), resp.Body.UsageBreakdown["label"])
	s.Equal(int64(0), resp.Body.UsageBreakdown["festival"])
}

func (s *TagHandlerIntegrationSuite) TestGetTagDetail_TopContributorsAndCreatedBy() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	// Give admin a username so CreatedBy has a usable handle.
	aliceUsername := "alice-admin"
	s.deps.DB.Model(admin).Update("username", aliceUsername)
	admin.Username = &aliceUsername
	ctx := testhelpers.CtxWithUser(admin)

	tag := s.createTagViaHandler(admin, "contrib-detail", models.TagCategoryGenre)

	artist := testhelpers.CreateArtist(s.deps.DB, "Contrib Artist")
	applyReq := &AddTagToEntityRequest{
		EntityType: models.TagEntityArtist,
		EntityID:   fmt.Sprintf("%d", artist.ID),
	}
	applyReq.Body.TagID = tag.Body.ID
	_, err := s.handler.AddTagToEntityHandler(ctx, applyReq)
	s.Require().NoError(err)

	resp, err := s.handler.GetTagDetailHandler(s.deps.Ctx, &GetTagDetailRequest{TagID: tag.Body.Slug})
	s.NoError(err)
	s.Require().NotNil(resp)

	s.Require().Len(resp.Body.TopContributors, 1)
	s.Equal(admin.ID, resp.Body.TopContributors[0].User.ID)
	s.Equal(aliceUsername, resp.Body.TopContributors[0].User.Username)
	s.Equal(int64(1), resp.Body.TopContributors[0].Count)

	s.Require().NotNil(resp.Body.CreatedBy)
	s.Equal(admin.ID, resp.Body.CreatedBy.ID)
	s.Equal(aliceUsername, resp.Body.CreatedBy.Username)
}

func (s *TagHandlerIntegrationSuite) TestGetTagDetail_RelatedTags() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	// Ensure contributor tier so AddTagToEntity passes the trust gate if it ever
	// attempts inline creation. IsAdmin bypasses the gate, but be explicit.
	s.deps.DB.Model(admin).Update("user_tier", "contributor")
	ctx := testhelpers.CtxWithUser(admin)

	focus := s.createTagViaHandler(admin, "focus-tag", models.TagCategoryGenre)
	related := s.createTagViaHandler(admin, "related-tag", models.TagCategoryGenre)

	artist := testhelpers.CreateArtist(s.deps.DB, "Related Artist")
	for _, tagID := range []uint{focus.Body.ID, related.Body.ID} {
		r := &AddTagToEntityRequest{
			EntityType: models.TagEntityArtist,
			EntityID:   fmt.Sprintf("%d", artist.ID),
		}
		r.Body.TagID = tagID
		_, err := s.handler.AddTagToEntityHandler(ctx, r)
		s.Require().NoError(err)
	}

	resp, err := s.handler.GetTagDetailHandler(s.deps.Ctx, &GetTagDetailRequest{TagID: focus.Body.Slug})
	s.NoError(err)
	s.Require().NotNil(resp)

	s.Require().Len(resp.Body.RelatedTags, 1)
	s.Equal(related.Body.ID, resp.Body.RelatedTags[0].ID)
	// Self must never appear.
	for _, rt := range resp.Body.RelatedTags {
		s.NotEqual(focus.Body.ID, rt.ID)
	}
}

func (s *TagHandlerIntegrationSuite) TestGetTagDetail_AliasesPreserved() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	tag := s.createTagViaHandler(admin, "alias-detail", models.TagCategoryGenre)

	ctx := testhelpers.CtxWithUser(admin)
	aliasReq := &CreateAliasRequest{TagID: fmt.Sprintf("%d", tag.Body.ID)}
	aliasReq.Body.Alias = "detail-aka"
	_, err := s.handler.CreateAliasHandler(ctx, aliasReq)
	s.Require().NoError(err)

	resp, err := s.handler.GetTagDetailHandler(s.deps.Ctx, &GetTagDetailRequest{TagID: tag.Body.Slug})
	s.NoError(err)
	s.Require().NotNil(resp)
	s.Contains(resp.Body.Aliases, "detail-aka")
}

// ============================================================================
// ListTagsHandler
// ============================================================================

func (s *TagHandlerIntegrationSuite) TestListTags_Success() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	s.createTagViaHandler(admin, "post-punk", models.TagCategoryGenre)
	s.createTagViaHandler(admin, "shoegaze", models.TagCategoryGenre)
	s.createTagViaHandler(admin, "melancholy", models.TagCategoryOther)

	req := &ListTagsRequest{}
	resp, err := s.handler.ListTagsHandler(s.deps.Ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.Equal(int64(3), resp.Body.Total)
	s.Len(resp.Body.Tags, 3)
}

func (s *TagHandlerIntegrationSuite) TestListTags_FilterByCategory() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	s.createTagViaHandler(admin, "post-punk", models.TagCategoryGenre)
	s.createTagViaHandler(admin, "shoegaze", models.TagCategoryGenre)
	s.createTagViaHandler(admin, "melancholy", models.TagCategoryOther)

	req := &ListTagsRequest{Category: models.TagCategoryGenre}
	resp, err := s.handler.ListTagsHandler(s.deps.Ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.Equal(int64(2), resp.Body.Total)
	for _, tag := range resp.Body.Tags {
		s.Equal("genre", tag.Category)
	}
}

func (s *TagHandlerIntegrationSuite) TestListTags_Search() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	s.createTagViaHandler(admin, "post-punk", models.TagCategoryGenre)
	s.createTagViaHandler(admin, "post-rock", models.TagCategoryGenre)
	s.createTagViaHandler(admin, "shoegaze", models.TagCategoryGenre)

	req := &ListTagsRequest{Search: "post"}
	resp, err := s.handler.ListTagsHandler(s.deps.Ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.Equal(int64(2), resp.Body.Total)
}

func (s *TagHandlerIntegrationSuite) TestListTags_Pagination() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	for i := 0; i < 5; i++ {
		s.createTagViaHandler(admin, fmt.Sprintf("tag-%d", i), models.TagCategoryGenre)
	}

	req := &ListTagsRequest{Limit: 2, Offset: 0}
	resp, err := s.handler.ListTagsHandler(s.deps.Ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.Equal(int64(5), resp.Body.Total)
	s.Len(resp.Body.Tags, 2)

	// Second page
	req2 := &ListTagsRequest{Limit: 2, Offset: 2}
	resp2, err := s.handler.ListTagsHandler(s.deps.Ctx, req2)
	s.NoError(err)
	s.Len(resp2.Body.Tags, 2)
}

func (s *TagHandlerIntegrationSuite) TestListTags_Empty() {
	req := &ListTagsRequest{}
	resp, err := s.handler.ListTagsHandler(s.deps.Ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.Equal(int64(0), resp.Body.Total)
	s.Empty(resp.Body.Tags)
}

// TestListTags_EntityTypeScopedFacet verifies PSY-484: callers can scope the
// usage_count to a single entity type via ?entity_type=. The same tag returns
// different counts depending on which browse page asked, so the chip on
// /artists shows "punk N (artists)" and the chip on /venues shows "punk M
// (venues)" instead of both showing the global N+M+….
func (s *TagHandlerIntegrationSuite) TestListTags_EntityTypeScopedFacet() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	createdPunk := s.createTagViaHandler(admin, "punk", models.TagCategoryGenre)
	createdRock := s.createTagViaHandler(admin, "rock", models.TagCategoryGenre)

	// Apply punk to one artist; apply rock to a venue. After this:
	//   global: punk=1, rock=1
	//   artist scope: punk=1, rock=0
	//   venue scope:  punk=0, rock=1
	artist := testhelpers.CreateArtist(s.deps.DB, "Black Flag")
	venue := testhelpers.CreateVerifiedVenue(s.deps.DB, "The Smell", "Los Angeles", "CA")

	addReq := &AddTagToEntityRequest{EntityType: "artist", EntityID: fmt.Sprintf("%d", artist.ID)}
	addReq.Body.TagID = createdPunk.Body.ID
	_, err := s.handler.AddTagToEntityHandler(testhelpers.CtxWithUser(admin), addReq)
	s.Require().NoError(err)

	addReq = &AddTagToEntityRequest{EntityType: "venue", EntityID: fmt.Sprintf("%d", venue.ID)}
	addReq.Body.TagID = createdRock.Body.ID
	_, err = s.handler.AddTagToEntityHandler(testhelpers.CtxWithUser(admin), addReq)
	s.Require().NoError(err)

	// /artists facet — punk should be 1, rock 0.
	resp, err := s.handler.ListTagsHandler(s.deps.Ctx, &ListTagsRequest{EntityType: "artist"})
	s.Require().NoError(err)
	s.Require().NotNil(resp)
	got := map[string]int{}
	for _, t := range resp.Body.Tags {
		got[t.Name] = t.UsageCount
	}
	s.Equal(1, got["punk"], "/artists facet: punk should reflect 1 artist application")
	s.Equal(0, got["rock"], "/artists facet: rock should be 0 (no artists tagged rock)")

	// /venues facet — punk should be 0, rock should be 1.
	resp, err = s.handler.ListTagsHandler(s.deps.Ctx, &ListTagsRequest{EntityType: "venue"})
	s.Require().NoError(err)
	s.Require().NotNil(resp)
	got = map[string]int{}
	for _, t := range resp.Body.Tags {
		got[t.Name] = t.UsageCount
	}
	s.Equal(0, got["punk"], "/venues facet: punk should be 0 (the dogfood-output bug)")
	s.Equal(1, got["rock"], "/venues facet: rock should reflect 1 venue application")

	// /festivals facet — both should be 0; ensure the count never falls back
	// to the global persisted value.
	resp, err = s.handler.ListTagsHandler(s.deps.Ctx, &ListTagsRequest{EntityType: "festival"})
	s.Require().NoError(err)
	for _, t := range resp.Body.Tags {
		s.Equal(0, t.UsageCount, "/festivals facet: %q should be 0 (no festivals tagged)", t.Name)
	}

	// Without entity_type, we get the persisted global counts — same shape
	// as before PSY-484 so the /tags browse page is unchanged.
	resp, err = s.handler.ListTagsHandler(s.deps.Ctx, &ListTagsRequest{})
	s.Require().NoError(err)
	got = map[string]int{}
	for _, t := range resp.Body.Tags {
		got[t.Name] = t.UsageCount
	}
	s.Equal(1, got["punk"])
	s.Equal(1, got["rock"])
}

func (s *TagHandlerIntegrationSuite) TestListTags_InvalidEntityType() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	s.createTagViaHandler(admin, "punk", models.TagCategoryGenre)

	resp, err := s.handler.ListTagsHandler(s.deps.Ctx, &ListTagsRequest{EntityType: "user"})
	s.Nil(resp)
	testhelpers.AssertHumaError(s.T(), err, 400)
}

// ============================================================================
// SearchTagsHandler
// ============================================================================

func (s *TagHandlerIntegrationSuite) TestSearchTags_Success() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	s.createTagViaHandler(admin, "post-punk", models.TagCategoryGenre)
	s.createTagViaHandler(admin, "post-rock", models.TagCategoryGenre)
	s.createTagViaHandler(admin, "shoegaze", models.TagCategoryGenre)

	req := &SearchTagsRequest{Query: "post"}
	resp, err := s.handler.SearchTagsHandler(s.deps.Ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.GreaterOrEqual(len(resp.Body.Tags), 2)
}

func (s *TagHandlerIntegrationSuite) TestSearchTags_NoResults() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	s.createTagViaHandler(admin, "post-punk", models.TagCategoryGenre)

	req := &SearchTagsRequest{Query: "zzzznonexistent"}
	resp, err := s.handler.SearchTagsHandler(s.deps.Ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.Empty(resp.Body.Tags)
}

func (s *TagHandlerIntegrationSuite) TestSearchTags_EmptyQuery() {
	req := &SearchTagsRequest{Query: ""}
	_, err := s.handler.SearchTagsHandler(s.deps.Ctx, req)
	testhelpers.AssertHumaError(s.T(), err, 400)
}

func (s *TagHandlerIntegrationSuite) TestSearchTags_WithLimit() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	for i := 0; i < 10; i++ {
		s.createTagViaHandler(admin, fmt.Sprintf("rock-%d", i), models.TagCategoryGenre)
	}

	req := &SearchTagsRequest{Query: "rock", Limit: 3}
	resp, err := s.handler.SearchTagsHandler(s.deps.Ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.LessOrEqual(len(resp.Body.Tags), 3)
}

// TestSearchTags_MatchedViaAlias covers PSY-442 — the autocomplete endpoint
// surfaces the specific alias that matched so the add-tag dialog can render
// a "matched `punk-rock`" caption under the canonical row.
func (s *TagHandlerIntegrationSuite) TestSearchTags_MatchedViaAlias() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	tag := s.createTagViaHandler(admin, "punk", models.TagCategoryGenre)

	// Seed an alias via the service layer (alias creation is admin-only via
	// the handler, but we don't need to exercise that path here).
	_, err := s.deps.TagService.CreateAlias(tag.Body.ID, "punk-rock")
	s.Require().NoError(err)

	req := &SearchTagsRequest{Query: "punk-rock"}
	resp, err := s.handler.SearchTagsHandler(s.deps.Ctx, req)
	s.NoError(err)
	s.Require().NotNil(resp)
	s.Require().Len(resp.Body.Tags, 1)
	s.Equal(tag.Body.ID, resp.Body.Tags[0].ID)
	s.Equal("punk", resp.Body.Tags[0].Name)
	s.Equal("punk-rock", resp.Body.Tags[0].MatchedViaAlias,
		"alias-match rows must expose MatchedViaAlias for the frontend caption")
}

// Name-match rows keep MatchedViaAlias empty so existing autocomplete
// consumers (Cmd+K, admin browse) render unchanged.
func (s *TagHandlerIntegrationSuite) TestSearchTags_NameMatchHasNoAliasCaption() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	tag := s.createTagViaHandler(admin, "punk", models.TagCategoryGenre)
	// An alias exists on the tag, but the query hits the canonical name directly.
	_, err := s.deps.TagService.CreateAlias(tag.Body.ID, "punk-rock")
	s.Require().NoError(err)

	req := &SearchTagsRequest{Query: "punk"}
	resp, err := s.handler.SearchTagsHandler(s.deps.Ctx, req)
	s.NoError(err)
	s.Require().NotNil(resp)
	s.Require().Len(resp.Body.Tags, 1)
	s.Equal(tag.Body.ID, resp.Body.Tags[0].ID)
	s.Empty(resp.Body.Tags[0].MatchedViaAlias,
		"name matches should leave MatchedViaAlias empty so the caption stays hidden")
}

// ============================================================================
// UpdateTagHandler
// ============================================================================

func (s *TagHandlerIntegrationSuite) TestUpdateTag_Success() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	created := s.createTagViaHandler(admin, "post-punk", models.TagCategoryGenre)

	ctx := testhelpers.CtxWithUser(admin)
	newName := "Post-Punk Revival"
	req := &UpdateTagRequest{TagID: fmt.Sprintf("%d", created.Body.ID)}
	req.Body.Name = &newName

	resp, err := s.handler.UpdateTagHandler(ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.Equal("Post-Punk Revival", resp.Body.Name)
}

func (s *TagHandlerIntegrationSuite) TestUpdateTag_ChangeCategory() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	created := s.createTagViaHandler(admin, "dark", models.TagCategoryOther)

	ctx := testhelpers.CtxWithUser(admin)
	newCat := models.TagCategoryLocale
	req := &UpdateTagRequest{TagID: fmt.Sprintf("%d", created.Body.ID)}
	req.Body.Category = &newCat

	resp, err := s.handler.UpdateTagHandler(ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.Equal("locale", resp.Body.Category)
}

func (s *TagHandlerIntegrationSuite) TestUpdateTag_NonAdmin() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	created := s.createTagViaHandler(admin, "post-punk", models.TagCategoryGenre)

	user := testhelpers.CreateTestUser(s.deps.DB)
	ctx := testhelpers.CtxWithUser(user)
	newName := "Updated"
	req := &UpdateTagRequest{TagID: fmt.Sprintf("%d", created.Body.ID)}
	req.Body.Name = &newName

	_, err := s.handler.UpdateTagHandler(ctx, req)
	testhelpers.AssertHumaError(s.T(), err, 403)
}

func (s *TagHandlerIntegrationSuite) TestUpdateTag_NoAuth() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	created := s.createTagViaHandler(admin, "post-punk", models.TagCategoryGenre)

	newName := "Updated"
	req := &UpdateTagRequest{TagID: fmt.Sprintf("%d", created.Body.ID)}
	req.Body.Name = &newName

	_, err := s.handler.UpdateTagHandler(s.deps.Ctx, req)
	testhelpers.AssertHumaError(s.T(), err, 401)
}

func (s *TagHandlerIntegrationSuite) TestUpdateTag_NotFound() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	ctx := testhelpers.CtxWithUser(admin)
	newName := "Updated"
	req := &UpdateTagRequest{TagID: "99999"}
	req.Body.Name = &newName

	_, err := s.handler.UpdateTagHandler(ctx, req)
	testhelpers.AssertHumaError(s.T(), err, 404)
}

// ============================================================================
// DeleteTagHandler
// ============================================================================

func (s *TagHandlerIntegrationSuite) TestDeleteTag_Success() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	created := s.createTagViaHandler(admin, "to-delete", models.TagCategoryGenre)

	ctx := testhelpers.CtxWithUser(admin)
	req := &DeleteTagRequest{TagID: fmt.Sprintf("%d", created.Body.ID)}
	_, err := s.handler.DeleteTagHandler(ctx, req)
	s.NoError(err)

	// Verify it's gone
	getReq := &GetTagRequest{TagID: fmt.Sprintf("%d", created.Body.ID)}
	_, err = s.handler.GetTagHandler(s.deps.Ctx, getReq)
	testhelpers.AssertHumaError(s.T(), err, 404)
}

func (s *TagHandlerIntegrationSuite) TestDeleteTag_NonAdmin() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	created := s.createTagViaHandler(admin, "protected-tag", models.TagCategoryGenre)

	user := testhelpers.CreateTestUser(s.deps.DB)
	ctx := testhelpers.CtxWithUser(user)
	req := &DeleteTagRequest{TagID: fmt.Sprintf("%d", created.Body.ID)}

	_, err := s.handler.DeleteTagHandler(ctx, req)
	testhelpers.AssertHumaError(s.T(), err, 403)
}

func (s *TagHandlerIntegrationSuite) TestDeleteTag_NoAuth() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	created := s.createTagViaHandler(admin, "protected-tag", models.TagCategoryGenre)

	req := &DeleteTagRequest{TagID: fmt.Sprintf("%d", created.Body.ID)}
	_, err := s.handler.DeleteTagHandler(s.deps.Ctx, req)
	testhelpers.AssertHumaError(s.T(), err, 401)
}

func (s *TagHandlerIntegrationSuite) TestDeleteTag_NotFound() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	ctx := testhelpers.CtxWithUser(admin)
	req := &DeleteTagRequest{TagID: "99999"}

	_, err := s.handler.DeleteTagHandler(ctx, req)
	testhelpers.AssertHumaError(s.T(), err, 404)
}

// ============================================================================
// AddTagToEntityHandler
// ============================================================================

func (s *TagHandlerIntegrationSuite) TestAddTagToEntity_ByTagID() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	tag := s.createTagViaHandler(admin, "post-punk", models.TagCategoryGenre)
	artist := testhelpers.CreateArtist(s.deps.DB, "Joy Division")

	user := testhelpers.CreateTestUser(s.deps.DB)
	ctx := testhelpers.CtxWithUser(user)
	req := &AddTagToEntityRequest{
		EntityType: models.TagEntityArtist,
		EntityID:   fmt.Sprintf("%d", artist.ID),
	}
	req.Body.TagID = tag.Body.ID

	_, err := s.handler.AddTagToEntityHandler(ctx, req)
	s.NoError(err)

	// Verify via list
	listReq := &ListEntityTagsRequest{
		EntityType: models.TagEntityArtist,
		EntityID:   fmt.Sprintf("%d", artist.ID),
	}
	listResp, err := s.handler.ListEntityTagsHandler(s.deps.Ctx, listReq)
	s.NoError(err)
	s.Len(listResp.Body.Tags, 1)
	s.Equal("post-punk", listResp.Body.Tags[0].Name)
}

func (s *TagHandlerIntegrationSuite) TestAddTagToEntity_ByTagName() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	s.createTagViaHandler(admin, "shoegaze", models.TagCategoryGenre)
	artist := testhelpers.CreateArtist(s.deps.DB, "My Bloody Valentine")

	user := testhelpers.CreateTestUser(s.deps.DB)
	ctx := testhelpers.CtxWithUser(user)
	req := &AddTagToEntityRequest{
		EntityType: models.TagEntityArtist,
		EntityID:   fmt.Sprintf("%d", artist.ID),
	}
	req.Body.TagName = "shoegaze"

	_, err := s.handler.AddTagToEntityHandler(ctx, req)
	s.NoError(err)

	// Verify
	listReq := &ListEntityTagsRequest{
		EntityType: models.TagEntityArtist,
		EntityID:   fmt.Sprintf("%d", artist.ID),
	}
	listResp, err := s.handler.ListEntityTagsHandler(s.deps.Ctx, listReq)
	s.NoError(err)
	s.Len(listResp.Body.Tags, 1)
	s.Equal("shoegaze", listResp.Body.Tags[0].Name)
}

func (s *TagHandlerIntegrationSuite) TestAddTagToEntity_Duplicate() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	tag := s.createTagViaHandler(admin, "post-punk", models.TagCategoryGenre)
	artist := testhelpers.CreateArtist(s.deps.DB, "Siouxsie")

	user := testhelpers.CreateTestUser(s.deps.DB)
	ctx := testhelpers.CtxWithUser(user)
	req := &AddTagToEntityRequest{
		EntityType: models.TagEntityArtist,
		EntityID:   fmt.Sprintf("%d", artist.ID),
	}
	req.Body.TagID = tag.Body.ID

	// First add succeeds
	_, err := s.handler.AddTagToEntityHandler(ctx, req)
	s.NoError(err)

	// Second add should conflict
	_, err = s.handler.AddTagToEntityHandler(ctx, req)
	testhelpers.AssertHumaError(s.T(), err, 409)
}

func (s *TagHandlerIntegrationSuite) TestAddTagToEntity_MissingFields() {
	user := testhelpers.CreateTestUser(s.deps.DB)
	ctx := testhelpers.CtxWithUser(user)
	artist := testhelpers.CreateArtist(s.deps.DB, "Test Artist")

	req := &AddTagToEntityRequest{
		EntityType: models.TagEntityArtist,
		EntityID:   fmt.Sprintf("%d", artist.ID),
	}
	// Both TagID and TagName are zero/empty

	_, err := s.handler.AddTagToEntityHandler(ctx, req)
	testhelpers.AssertHumaError(s.T(), err, 400)
}

func (s *TagHandlerIntegrationSuite) TestAddTagToEntity_NoAuth() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	tag := s.createTagViaHandler(admin, "post-punk", models.TagCategoryGenre)
	artist := testhelpers.CreateArtist(s.deps.DB, "Test Artist")

	req := &AddTagToEntityRequest{
		EntityType: models.TagEntityArtist,
		EntityID:   fmt.Sprintf("%d", artist.ID),
	}
	req.Body.TagID = tag.Body.ID

	_, err := s.handler.AddTagToEntityHandler(s.deps.Ctx, req)
	testhelpers.AssertHumaError(s.T(), err, 401)
}

// ============================================================================
// ListEntityTagsHandler
// ============================================================================

func (s *TagHandlerIntegrationSuite) TestListEntityTags_Success() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	tag := s.createTagViaHandler(admin, "post-punk", models.TagCategoryGenre)
	artist := testhelpers.CreateArtist(s.deps.DB, "Joy Division")

	user := testhelpers.CreateTestUser(s.deps.DB)
	ctx := testhelpers.CtxWithUser(user)
	addReq := &AddTagToEntityRequest{
		EntityType: models.TagEntityArtist,
		EntityID:   fmt.Sprintf("%d", artist.ID),
	}
	addReq.Body.TagID = tag.Body.ID
	_, err := s.handler.AddTagToEntityHandler(ctx, addReq)
	s.Require().NoError(err)

	listReq := &ListEntityTagsRequest{
		EntityType: models.TagEntityArtist,
		EntityID:   fmt.Sprintf("%d", artist.ID),
	}
	resp, err := s.handler.ListEntityTagsHandler(s.deps.Ctx, listReq)
	s.NoError(err)
	s.NotNil(resp)
	s.Len(resp.Body.Tags, 1)
	s.Equal("post-punk", resp.Body.Tags[0].Name)
}

func (s *TagHandlerIntegrationSuite) TestListEntityTags_Empty() {
	artist := testhelpers.CreateArtist(s.deps.DB, "No Tags Artist")

	req := &ListEntityTagsRequest{
		EntityType: models.TagEntityArtist,
		EntityID:   fmt.Sprintf("%d", artist.ID),
	}
	resp, err := s.handler.ListEntityTagsHandler(s.deps.Ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.Empty(resp.Body.Tags)
}

// PSY-479: ListEntityTags response surfaces attribution fields so the
// frontend hover card can render "Added by @user" and a relative timestamp.
// Verifies the handler-level wiring (response body shape) end-to-end.
func (s *TagHandlerIntegrationSuite) TestListEntityTags_SurfacesAttribution() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	tag := s.createTagViaHandler(admin, "shoegaze-revival", models.TagCategoryGenre)
	artist := testhelpers.CreateArtist(s.deps.DB, "Faetooth")

	// Create a user with a username so AddedByUsername is non-nil.
	user := testhelpers.CreateTestUser(s.deps.DB)
	username := "testuser2"
	s.Require().NoError(s.deps.DB.Model(user).Update("username", username).Error)

	ctx := testhelpers.CtxWithUser(user)
	addReq := &AddTagToEntityRequest{
		EntityType: models.TagEntityArtist,
		EntityID:   fmt.Sprintf("%d", artist.ID),
	}
	addReq.Body.TagID = tag.Body.ID
	_, err := s.handler.AddTagToEntityHandler(ctx, addReq)
	s.Require().NoError(err)

	listReq := &ListEntityTagsRequest{
		EntityType: models.TagEntityArtist,
		EntityID:   fmt.Sprintf("%d", artist.ID),
	}
	resp, err := s.handler.ListEntityTagsHandler(s.deps.Ctx, listReq)
	s.Require().NoError(err)
	s.Require().Len(resp.Body.Tags, 1)

	got := resp.Body.Tags[0]
	s.Require().NotNil(got.AddedByUserID)
	s.Equal(user.ID, *got.AddedByUserID)

	s.Require().NotNil(got.AddedByUsername)
	s.Equal(username, *got.AddedByUsername)

	s.Require().NotNil(got.AddedAt)
	s.False(got.AddedAt.IsZero())
}

// PSY-479: when the user who applied a tag has a null username (older seed
// rows, accounts that never set one), the response still includes
// added_by_user_id + added_at. added_by_username comes back as JSON null so
// the frontend can render "Source: system seed" instead of suppressing the
// attribution line entirely.
func (s *TagHandlerIntegrationSuite) TestListEntityTags_AttributionNullUsername() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	tag := s.createTagViaHandler(admin, "dream-pop", models.TagCategoryGenre)
	artist := testhelpers.CreateArtist(s.deps.DB, "Cocteau Twins")

	// createTestUser does NOT set Username — mirrors the seed/dogfood case.
	user := testhelpers.CreateTestUser(s.deps.DB)
	ctx := testhelpers.CtxWithUser(user)
	addReq := &AddTagToEntityRequest{
		EntityType: models.TagEntityArtist,
		EntityID:   fmt.Sprintf("%d", artist.ID),
	}
	addReq.Body.TagID = tag.Body.ID
	_, err := s.handler.AddTagToEntityHandler(ctx, addReq)
	s.Require().NoError(err)

	listReq := &ListEntityTagsRequest{
		EntityType: models.TagEntityArtist,
		EntityID:   fmt.Sprintf("%d", artist.ID),
	}
	resp, err := s.handler.ListEntityTagsHandler(s.deps.Ctx, listReq)
	s.Require().NoError(err)
	s.Require().Len(resp.Body.Tags, 1)

	got := resp.Body.Tags[0]
	s.Require().NotNil(got.AddedByUserID)
	s.Equal(user.ID, *got.AddedByUserID)
	s.Require().NotNil(got.AddedAt)
	s.Nil(got.AddedByUsername, "username must be nil so the frontend renders 'Source: system seed'")
}

func (s *TagHandlerIntegrationSuite) TestListEntityTags_MultipleTags() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	tag1 := s.createTagViaHandler(admin, "post-punk", models.TagCategoryGenre)
	tag2 := s.createTagViaHandler(admin, "dark", models.TagCategoryOther)
	tag3 := s.createTagViaHandler(admin, "80s", models.TagCategoryLocale)
	artist := testhelpers.CreateArtist(s.deps.DB, "Bauhaus")

	user := testhelpers.CreateTestUser(s.deps.DB)
	ctx := testhelpers.CtxWithUser(user)

	for _, tagID := range []uint{tag1.Body.ID, tag2.Body.ID, tag3.Body.ID} {
		addReq := &AddTagToEntityRequest{
			EntityType: models.TagEntityArtist,
			EntityID:   fmt.Sprintf("%d", artist.ID),
		}
		addReq.Body.TagID = tagID
		_, err := s.handler.AddTagToEntityHandler(ctx, addReq)
		s.Require().NoError(err)
	}

	listReq := &ListEntityTagsRequest{
		EntityType: models.TagEntityArtist,
		EntityID:   fmt.Sprintf("%d", artist.ID),
	}
	resp, err := s.handler.ListEntityTagsHandler(s.deps.Ctx, listReq)
	s.NoError(err)
	s.Len(resp.Body.Tags, 3)
}

// ============================================================================
// RemoveTagFromEntityHandler
// ============================================================================

func (s *TagHandlerIntegrationSuite) TestRemoveTagFromEntity_Success() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	tag := s.createTagViaHandler(admin, "post-punk", models.TagCategoryGenre)
	artist := testhelpers.CreateArtist(s.deps.DB, "Wire")

	user := testhelpers.CreateTestUser(s.deps.DB)
	ctx := testhelpers.CtxWithUser(user)

	// Add tag
	addReq := &AddTagToEntityRequest{
		EntityType: models.TagEntityArtist,
		EntityID:   fmt.Sprintf("%d", artist.ID),
	}
	addReq.Body.TagID = tag.Body.ID
	_, err := s.handler.AddTagToEntityHandler(ctx, addReq)
	s.Require().NoError(err)

	// Remove tag
	removeReq := &RemoveTagFromEntityRequest{
		EntityType: models.TagEntityArtist,
		EntityID:   fmt.Sprintf("%d", artist.ID),
		TagID:      fmt.Sprintf("%d", tag.Body.ID),
	}
	_, err = s.handler.RemoveTagFromEntityHandler(ctx, removeReq)
	s.NoError(err)

	// Verify removed
	listReq := &ListEntityTagsRequest{
		EntityType: models.TagEntityArtist,
		EntityID:   fmt.Sprintf("%d", artist.ID),
	}
	listResp, err := s.handler.ListEntityTagsHandler(s.deps.Ctx, listReq)
	s.NoError(err)
	s.Empty(listResp.Body.Tags)
}

func (s *TagHandlerIntegrationSuite) TestRemoveTagFromEntity_NotFound() {
	user := testhelpers.CreateTestUser(s.deps.DB)
	ctx := testhelpers.CtxWithUser(user)
	artist := testhelpers.CreateArtist(s.deps.DB, "Test Artist")

	req := &RemoveTagFromEntityRequest{
		EntityType: models.TagEntityArtist,
		EntityID:   fmt.Sprintf("%d", artist.ID),
		TagID:      "99999",
	}
	_, err := s.handler.RemoveTagFromEntityHandler(ctx, req)
	testhelpers.AssertHumaError(s.T(), err, 404)
}

func (s *TagHandlerIntegrationSuite) TestRemoveTagFromEntity_NoAuth() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	tag := s.createTagViaHandler(admin, "post-punk", models.TagCategoryGenre)
	artist := testhelpers.CreateArtist(s.deps.DB, "Test Artist")

	req := &RemoveTagFromEntityRequest{
		EntityType: models.TagEntityArtist,
		EntityID:   fmt.Sprintf("%d", artist.ID),
		TagID:      fmt.Sprintf("%d", tag.Body.ID),
	}
	_, err := s.handler.RemoveTagFromEntityHandler(s.deps.Ctx, req)
	testhelpers.AssertHumaError(s.T(), err, 401)
}

// ============================================================================
// VoteTagHandler
// ============================================================================

func (s *TagHandlerIntegrationSuite) TestVoteTag_Upvote() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	tag := s.createTagViaHandler(admin, "post-punk", models.TagCategoryGenre)
	artist := testhelpers.CreateArtist(s.deps.DB, "Joy Division")

	// First add the tag to the entity
	user := testhelpers.CreateTestUser(s.deps.DB)
	ctx := testhelpers.CtxWithUser(user)
	addReq := &AddTagToEntityRequest{
		EntityType: models.TagEntityArtist,
		EntityID:   fmt.Sprintf("%d", artist.ID),
	}
	addReq.Body.TagID = tag.Body.ID
	_, err := s.handler.AddTagToEntityHandler(ctx, addReq)
	s.Require().NoError(err)

	// Vote
	voter := testhelpers.CreateTestUser(s.deps.DB)
	voteCtx := testhelpers.CtxWithUser(voter)
	req := &VoteTagRequest{
		TagID:      fmt.Sprintf("%d", tag.Body.ID),
		EntityType: models.TagEntityArtist,
		EntityID:   fmt.Sprintf("%d", artist.ID),
	}
	req.Body.IsUpvote = true

	_, err = s.handler.VoteTagHandler(voteCtx, req)
	s.NoError(err)
}

func (s *TagHandlerIntegrationSuite) TestVoteTag_Downvote() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	tag := s.createTagViaHandler(admin, "post-punk", models.TagCategoryGenre)
	artist := testhelpers.CreateArtist(s.deps.DB, "Joy Division")

	// Add the tag
	user := testhelpers.CreateTestUser(s.deps.DB)
	ctx := testhelpers.CtxWithUser(user)
	addReq := &AddTagToEntityRequest{
		EntityType: models.TagEntityArtist,
		EntityID:   fmt.Sprintf("%d", artist.ID),
	}
	addReq.Body.TagID = tag.Body.ID
	_, err := s.handler.AddTagToEntityHandler(ctx, addReq)
	s.Require().NoError(err)

	// Downvote
	voter := testhelpers.CreateTestUser(s.deps.DB)
	voteCtx := testhelpers.CtxWithUser(voter)
	req := &VoteTagRequest{
		TagID:      fmt.Sprintf("%d", tag.Body.ID),
		EntityType: models.TagEntityArtist,
		EntityID:   fmt.Sprintf("%d", artist.ID),
	}
	req.Body.IsUpvote = false

	_, err = s.handler.VoteTagHandler(voteCtx, req)
	s.NoError(err)
}

func (s *TagHandlerIntegrationSuite) TestVoteTag_NoAuth() {
	req := &VoteTagRequest{
		TagID:      "1",
		EntityType: models.TagEntityArtist,
		EntityID:   "1",
	}
	req.Body.IsUpvote = true

	_, err := s.handler.VoteTagHandler(s.deps.Ctx, req)
	testhelpers.AssertHumaError(s.T(), err, 401)
}

// ============================================================================
// RemoveTagVoteHandler
// ============================================================================

func (s *TagHandlerIntegrationSuite) TestRemoveTagVote_Success() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	tag := s.createTagViaHandler(admin, "post-punk", models.TagCategoryGenre)
	artist := testhelpers.CreateArtist(s.deps.DB, "Joy Division")

	// Add the tag
	user := testhelpers.CreateTestUser(s.deps.DB)
	ctx := testhelpers.CtxWithUser(user)
	addReq := &AddTagToEntityRequest{
		EntityType: models.TagEntityArtist,
		EntityID:   fmt.Sprintf("%d", artist.ID),
	}
	addReq.Body.TagID = tag.Body.ID
	_, err := s.handler.AddTagToEntityHandler(ctx, addReq)
	s.Require().NoError(err)

	// Vote first
	voter := testhelpers.CreateTestUser(s.deps.DB)
	voteCtx := testhelpers.CtxWithUser(voter)
	voteReq := &VoteTagRequest{
		TagID:      fmt.Sprintf("%d", tag.Body.ID),
		EntityType: models.TagEntityArtist,
		EntityID:   fmt.Sprintf("%d", artist.ID),
	}
	voteReq.Body.IsUpvote = true
	_, err = s.handler.VoteTagHandler(voteCtx, voteReq)
	s.Require().NoError(err)

	// Remove vote
	removeReq := &RemoveTagVoteRequest{
		TagID:      fmt.Sprintf("%d", tag.Body.ID),
		EntityType: models.TagEntityArtist,
		EntityID:   fmt.Sprintf("%d", artist.ID),
	}
	_, err = s.handler.RemoveTagVoteHandler(voteCtx, removeReq)
	s.NoError(err)
}

func (s *TagHandlerIntegrationSuite) TestRemoveTagVote_NoAuth() {
	req := &RemoveTagVoteRequest{
		TagID:      "1",
		EntityType: models.TagEntityArtist,
		EntityID:   "1",
	}
	_, err := s.handler.RemoveTagVoteHandler(s.deps.Ctx, req)
	testhelpers.AssertHumaError(s.T(), err, 401)
}

// ============================================================================
// CreateAliasHandler
// ============================================================================

func (s *TagHandlerIntegrationSuite) TestCreateAlias_Success() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	tag := s.createTagViaHandler(admin, "post-punk", models.TagCategoryGenre)

	ctx := testhelpers.CtxWithUser(admin)
	req := &CreateAliasRequest{TagID: fmt.Sprintf("%d", tag.Body.ID)}
	req.Body.Alias = "post punk"

	resp, err := s.handler.CreateAliasHandler(ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.Equal("post punk", resp.Body.Alias)
	s.NotZero(resp.Body.ID)
}

func (s *TagHandlerIntegrationSuite) TestCreateAlias_NonAdmin() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	tag := s.createTagViaHandler(admin, "post-punk", models.TagCategoryGenre)

	user := testhelpers.CreateTestUser(s.deps.DB)
	ctx := testhelpers.CtxWithUser(user)
	req := &CreateAliasRequest{TagID: fmt.Sprintf("%d", tag.Body.ID)}
	req.Body.Alias = "post punk"

	_, err := s.handler.CreateAliasHandler(ctx, req)
	testhelpers.AssertHumaError(s.T(), err, 403)
}

func (s *TagHandlerIntegrationSuite) TestCreateAlias_NoAuth() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	tag := s.createTagViaHandler(admin, "post-punk", models.TagCategoryGenre)

	req := &CreateAliasRequest{TagID: fmt.Sprintf("%d", tag.Body.ID)}
	req.Body.Alias = "post punk"

	_, err := s.handler.CreateAliasHandler(s.deps.Ctx, req)
	testhelpers.AssertHumaError(s.T(), err, 401)
}

func (s *TagHandlerIntegrationSuite) TestCreateAlias_EmptyAlias() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	tag := s.createTagViaHandler(admin, "post-punk", models.TagCategoryGenre)

	ctx := testhelpers.CtxWithUser(admin)
	req := &CreateAliasRequest{TagID: fmt.Sprintf("%d", tag.Body.ID)}
	req.Body.Alias = ""

	_, err := s.handler.CreateAliasHandler(ctx, req)
	testhelpers.AssertHumaError(s.T(), err, 400)
}

func (s *TagHandlerIntegrationSuite) TestCreateAlias_DuplicateAlias() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	tag := s.createTagViaHandler(admin, "post-punk", models.TagCategoryGenre)

	ctx := testhelpers.CtxWithUser(admin)

	// First alias
	req1 := &CreateAliasRequest{TagID: fmt.Sprintf("%d", tag.Body.ID)}
	req1.Body.Alias = "post punk"
	_, err := s.handler.CreateAliasHandler(ctx, req1)
	s.Require().NoError(err)

	// Duplicate alias
	req2 := &CreateAliasRequest{TagID: fmt.Sprintf("%d", tag.Body.ID)}
	req2.Body.Alias = "post punk"
	_, err = s.handler.CreateAliasHandler(ctx, req2)
	testhelpers.AssertHumaError(s.T(), err, 409)
}

// ============================================================================
// ListAliasesHandler
// ============================================================================

func (s *TagHandlerIntegrationSuite) TestListAliases_Success() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	tag := s.createTagViaHandler(admin, "post-punk", models.TagCategoryGenre)

	ctx := testhelpers.CtxWithUser(admin)
	for _, alias := range []string{"post punk", "postpunk", "pp"} {
		req := &CreateAliasRequest{TagID: fmt.Sprintf("%d", tag.Body.ID)}
		req.Body.Alias = alias
		_, err := s.handler.CreateAliasHandler(ctx, req)
		s.Require().NoError(err)
	}

	listReq := &ListAliasesRequest{TagID: fmt.Sprintf("%d", tag.Body.ID)}
	resp, err := s.handler.ListAliasesHandler(s.deps.Ctx, listReq)
	s.NoError(err)
	s.NotNil(resp)
	s.Len(resp.Body.Aliases, 3)
}

func (s *TagHandlerIntegrationSuite) TestListAliases_Empty() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	tag := s.createTagViaHandler(admin, "post-punk", models.TagCategoryGenre)

	req := &ListAliasesRequest{TagID: fmt.Sprintf("%d", tag.Body.ID)}
	resp, err := s.handler.ListAliasesHandler(s.deps.Ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.Empty(resp.Body.Aliases)
}

func (s *TagHandlerIntegrationSuite) TestListAliases_BySlug() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	tag := s.createTagViaHandler(admin, "post-punk", models.TagCategoryGenre)

	ctx := testhelpers.CtxWithUser(admin)
	aliasReq := &CreateAliasRequest{TagID: fmt.Sprintf("%d", tag.Body.ID)}
	aliasReq.Body.Alias = "postpunk"
	_, err := s.handler.CreateAliasHandler(ctx, aliasReq)
	s.Require().NoError(err)

	// List by slug instead of ID
	listReq := &ListAliasesRequest{TagID: tag.Body.Slug}
	resp, err := s.handler.ListAliasesHandler(s.deps.Ctx, listReq)
	s.NoError(err)
	s.NotNil(resp)
	s.Len(resp.Body.Aliases, 1)
}

func (s *TagHandlerIntegrationSuite) TestListAliases_TagNotFound() {
	req := &ListAliasesRequest{TagID: "99999"}
	_, err := s.handler.ListAliasesHandler(s.deps.Ctx, req)
	testhelpers.AssertHumaError(s.T(), err, 404)
}

// ============================================================================
// DeleteAliasHandler
// ============================================================================

func (s *TagHandlerIntegrationSuite) TestDeleteAlias_Success() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	tag := s.createTagViaHandler(admin, "post-punk", models.TagCategoryGenre)

	ctx := testhelpers.CtxWithUser(admin)
	createReq := &CreateAliasRequest{TagID: fmt.Sprintf("%d", tag.Body.ID)}
	createReq.Body.Alias = "post punk"
	aliasResp, err := s.handler.CreateAliasHandler(ctx, createReq)
	s.Require().NoError(err)

	// Delete alias
	delReq := &DeleteAliasRequest{
		TagID:   fmt.Sprintf("%d", tag.Body.ID),
		AliasID: fmt.Sprintf("%d", aliasResp.Body.ID),
	}
	_, err = s.handler.DeleteAliasHandler(ctx, delReq)
	s.NoError(err)

	// Verify it's gone
	listReq := &ListAliasesRequest{TagID: fmt.Sprintf("%d", tag.Body.ID)}
	listResp, err := s.handler.ListAliasesHandler(s.deps.Ctx, listReq)
	s.NoError(err)
	s.Empty(listResp.Body.Aliases)
}

func (s *TagHandlerIntegrationSuite) TestDeleteAlias_NonAdmin() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	tag := s.createTagViaHandler(admin, "post-punk", models.TagCategoryGenre)

	ctx := testhelpers.CtxWithUser(admin)
	createReq := &CreateAliasRequest{TagID: fmt.Sprintf("%d", tag.Body.ID)}
	createReq.Body.Alias = "post punk"
	aliasResp, err := s.handler.CreateAliasHandler(ctx, createReq)
	s.Require().NoError(err)

	user := testhelpers.CreateTestUser(s.deps.DB)
	userCtx := testhelpers.CtxWithUser(user)
	delReq := &DeleteAliasRequest{
		TagID:   fmt.Sprintf("%d", tag.Body.ID),
		AliasID: fmt.Sprintf("%d", aliasResp.Body.ID),
	}
	_, err = s.handler.DeleteAliasHandler(userCtx, delReq)
	testhelpers.AssertHumaError(s.T(), err, 403)
}

func (s *TagHandlerIntegrationSuite) TestDeleteAlias_NoAuth() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	tag := s.createTagViaHandler(admin, "post-punk", models.TagCategoryGenre)

	ctx := testhelpers.CtxWithUser(admin)
	createReq := &CreateAliasRequest{TagID: fmt.Sprintf("%d", tag.Body.ID)}
	createReq.Body.Alias = "post punk"
	aliasResp, err := s.handler.CreateAliasHandler(ctx, createReq)
	s.Require().NoError(err)

	delReq := &DeleteAliasRequest{
		TagID:   fmt.Sprintf("%d", tag.Body.ID),
		AliasID: fmt.Sprintf("%d", aliasResp.Body.ID),
	}
	_, err = s.handler.DeleteAliasHandler(s.deps.Ctx, delReq)
	testhelpers.AssertHumaError(s.T(), err, 401)
}

// ============================================================================
// AddTagToEntity with alias resolution
// ============================================================================

func (s *TagHandlerIntegrationSuite) TestAddTagToEntity_ByAliasName() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	tag := s.createTagViaHandler(admin, "post-punk", models.TagCategoryGenre)

	// Create alias
	ctx := testhelpers.CtxWithUser(admin)
	aliasReq := &CreateAliasRequest{TagID: fmt.Sprintf("%d", tag.Body.ID)}
	aliasReq.Body.Alias = "postpunk"
	_, err := s.handler.CreateAliasHandler(ctx, aliasReq)
	s.Require().NoError(err)

	// Add tag using alias name
	artist := testhelpers.CreateArtist(s.deps.DB, "Gang of Four")
	user := testhelpers.CreateTestUser(s.deps.DB)
	userCtx := testhelpers.CtxWithUser(user)
	addReq := &AddTagToEntityRequest{
		EntityType: models.TagEntityArtist,
		EntityID:   fmt.Sprintf("%d", artist.ID),
	}
	addReq.Body.TagName = "postpunk"

	_, err = s.handler.AddTagToEntityHandler(userCtx, addReq)
	s.NoError(err)

	// Verify the canonical tag was applied
	listReq := &ListEntityTagsRequest{
		EntityType: models.TagEntityArtist,
		EntityID:   fmt.Sprintf("%d", artist.ID),
	}
	listResp, err := s.handler.ListEntityTagsHandler(s.deps.Ctx, listReq)
	s.NoError(err)
	s.Len(listResp.Body.Tags, 1)
	s.Equal("post-punk", listResp.Body.Tags[0].Name)
}

// ============================================================================
// CreateTag with IsOfficial flag
// ============================================================================

func (s *TagHandlerIntegrationSuite) TestCreateTag_Official() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	ctx := testhelpers.CtxWithUser(admin)

	req := &CreateTagRequest{}
	req.Body.Name = "official-genre"
	req.Body.Category = models.TagCategoryGenre
	req.Body.IsOfficial = true

	resp, err := s.handler.CreateTagHandler(ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.True(resp.Body.IsOfficial)
}

// ============================================================================
// UpdateTag with description
// ============================================================================

func (s *TagHandlerIntegrationSuite) TestUpdateTag_SetDescription() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	created := s.createTagViaHandler(admin, "ambient", models.TagCategoryGenre)

	ctx := testhelpers.CtxWithUser(admin)
	desc := "Electronic music focused on atmosphere"
	req := &UpdateTagRequest{TagID: fmt.Sprintf("%d", created.Body.ID)}
	req.Body.Description = &desc

	resp, err := s.handler.UpdateTagHandler(ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.NotNil(resp.Body.Description)
	s.Equal(desc, *resp.Body.Description)
}

// ============================================================================
// Venue entity tagging (test different entity type)
// ============================================================================

func (s *TagHandlerIntegrationSuite) TestAddTagToEntity_VenueType() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	tag := s.createTagViaHandler(admin, "intimate", models.TagCategoryOther)
	venue := testhelpers.CreateVerifiedVenue(s.deps.DB, "The Rebel Lounge", "Phoenix", "AZ")

	user := testhelpers.CreateTestUser(s.deps.DB)
	ctx := testhelpers.CtxWithUser(user)
	req := &AddTagToEntityRequest{
		EntityType: models.TagEntityVenue,
		EntityID:   fmt.Sprintf("%d", venue.ID),
	}
	req.Body.TagID = tag.Body.ID

	_, err := s.handler.AddTagToEntityHandler(ctx, req)
	s.NoError(err)

	listReq := &ListEntityTagsRequest{
		EntityType: models.TagEntityVenue,
		EntityID:   fmt.Sprintf("%d", venue.ID),
	}
	listResp, err := s.handler.ListEntityTagsHandler(s.deps.Ctx, listReq)
	s.NoError(err)
	s.Len(listResp.Body.Tags, 1)
	s.Equal("intimate", listResp.Body.Tags[0].Name)
}

// ============================================================================
// ListAllAliasesHandler + BulkImportAliasesHandler (PSY-307)
// ============================================================================

func (s *TagHandlerIntegrationSuite) TestListAllAliases_AdminOnly() {
	user := testhelpers.CreateTestUser(s.deps.DB)
	ctx := testhelpers.CtxWithUser(user)

	_, err := s.handler.ListAllAliasesHandler(ctx, &ListAllAliasesRequest{})
	s.Error(err)
	s.Contains(err.Error(), "Admin")
}

func (s *TagHandlerIntegrationSuite) TestListAllAliases_Unauthenticated() {
	_, err := s.handler.ListAllAliasesHandler(s.deps.Ctx, &ListAllAliasesRequest{})
	s.Error(err)
}

func (s *TagHandlerIntegrationSuite) TestListAllAliases_ReturnsAllWithCanonicalInfo() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	ctx := testhelpers.CtxWithUser(admin)
	tag := s.createTagViaHandler(admin, "post-punk", models.TagCategoryGenre)

	aliasReq := &CreateAliasRequest{TagID: fmt.Sprintf("%d", tag.Body.ID)}
	aliasReq.Body.Alias = "postpunk"
	_, err := s.handler.CreateAliasHandler(ctx, aliasReq)
	s.Require().NoError(err)

	resp, err := s.handler.ListAllAliasesHandler(ctx, &ListAllAliasesRequest{})
	s.NoError(err)
	s.Equal(int64(1), resp.Body.Total)
	s.Require().Len(resp.Body.Aliases, 1)
	s.Equal("postpunk", resp.Body.Aliases[0].Alias)
	s.Equal("post-punk", resp.Body.Aliases[0].TagName)
	s.Equal(tag.Body.ID, resp.Body.Aliases[0].TagID)
}

func (s *TagHandlerIntegrationSuite) TestListAllAliases_SearchFilter() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	ctx := testhelpers.CtxWithUser(admin)
	tagA := s.createTagViaHandler(admin, "post-punk", models.TagCategoryGenre)
	tagB := s.createTagViaHandler(admin, "hip-hop", models.TagCategoryGenre)

	aliasReqA := &CreateAliasRequest{TagID: fmt.Sprintf("%d", tagA.Body.ID)}
	aliasReqA.Body.Alias = "postpunk"
	_, err := s.handler.CreateAliasHandler(ctx, aliasReqA)
	s.Require().NoError(err)

	aliasReqB := &CreateAliasRequest{TagID: fmt.Sprintf("%d", tagB.Body.ID)}
	aliasReqB.Body.Alias = "hiphop"
	_, err = s.handler.CreateAliasHandler(ctx, aliasReqB)
	s.Require().NoError(err)

	resp, err := s.handler.ListAllAliasesHandler(ctx, &ListAllAliasesRequest{Search: "hip"})
	s.NoError(err)
	s.Equal(int64(1), resp.Body.Total)
	s.Require().Len(resp.Body.Aliases, 1)
	s.Equal("hiphop", resp.Body.Aliases[0].Alias)
}

func (s *TagHandlerIntegrationSuite) TestBulkImportAliases_AdminOnly() {
	user := testhelpers.CreateTestUser(s.deps.DB)
	ctx := testhelpers.CtxWithUser(user)

	req := &BulkImportAliasesRequest{}
	req.Body.Items = []contracts.BulkAliasImportItem{{Alias: "x", Canonical: "y"}}
	_, err := s.handler.BulkImportAliasesHandler(ctx, req)
	s.Error(err)
	s.Contains(err.Error(), "Admin")
}

func (s *TagHandlerIntegrationSuite) TestBulkImportAliases_EmptyRejected() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	ctx := testhelpers.CtxWithUser(admin)

	req := &BulkImportAliasesRequest{}
	_, err := s.handler.BulkImportAliasesHandler(ctx, req)
	s.Error(err)
}

func (s *TagHandlerIntegrationSuite) TestBulkImportAliases_TooLargeRejected() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	ctx := testhelpers.CtxWithUser(admin)

	req := &BulkImportAliasesRequest{}
	req.Body.Items = make([]contracts.BulkAliasImportItem, maxBulkAliasImportRows+1)
	for i := range req.Body.Items {
		req.Body.Items[i] = contracts.BulkAliasImportItem{Alias: fmt.Sprintf("a%d", i), Canonical: "x"}
	}
	_, err := s.handler.BulkImportAliasesHandler(ctx, req)
	s.Error(err)
	s.Contains(err.Error(), "max")
}

func (s *TagHandlerIntegrationSuite) TestBulkImportAliases_MixedResultSummary() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	ctx := testhelpers.CtxWithUser(admin)
	s.createTagViaHandler(admin, "drum-and-bass", models.TagCategoryGenre)

	req := &BulkImportAliasesRequest{}
	req.Body.Items = []contracts.BulkAliasImportItem{
		{Alias: "dnb", Canonical: "drum-and-bass"},
		{Alias: "foo", Canonical: "nonexistent"},
	}
	resp, err := s.handler.BulkImportAliasesHandler(ctx, req)
	s.NoError(err)
	s.Equal(1, resp.Body.Imported)
	s.Require().Len(resp.Body.Skipped, 1)
	s.Equal(2, resp.Body.Skipped[0].Row)
	s.Equal("foo", resp.Body.Skipped[0].Alias)
}

// ============================================================================
// MergeTagsHandler / MergeTagsPreviewHandler (PSY-306)
// ============================================================================

func (s *TagHandlerIntegrationSuite) TestMergeTags_Success() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	source := s.createTagViaHandler(admin, "shoe-gaze", models.TagCategoryGenre)
	target := s.createTagViaHandler(admin, "shoegaze", models.TagCategoryGenre)

	ctx := testhelpers.CtxWithUser(admin)
	req := &MergeTagsRequest{SourceID: fmt.Sprintf("%d", source.Body.ID)}
	req.Body.TargetID = target.Body.ID

	resp, err := s.handler.MergeTagsHandler(ctx, req)
	s.Require().NoError(err)
	s.Require().NotNil(resp.Body)
	s.True(resp.Body.AliasCreated)

	// Source is gone.
	getReq := &GetTagRequest{TagID: fmt.Sprintf("%d", source.Body.ID)}
	_, err = s.handler.GetTagHandler(ctx, getReq)
	testhelpers.AssertHumaError(s.T(), err, 404)
}

func (s *TagHandlerIntegrationSuite) TestMergeTags_NonAdmin() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	source := s.createTagViaHandler(admin, "shoe-gaze", models.TagCategoryGenre)
	target := s.createTagViaHandler(admin, "shoegaze", models.TagCategoryGenre)

	user := testhelpers.CreateTestUser(s.deps.DB)
	ctx := testhelpers.CtxWithUser(user)
	req := &MergeTagsRequest{SourceID: fmt.Sprintf("%d", source.Body.ID)}
	req.Body.TargetID = target.Body.ID

	_, err := s.handler.MergeTagsHandler(ctx, req)
	testhelpers.AssertHumaError(s.T(), err, 403)
}

func (s *TagHandlerIntegrationSuite) TestMergeTags_NoAuth() {
	req := &MergeTagsRequest{SourceID: "1"}
	req.Body.TargetID = 2
	_, err := s.handler.MergeTagsHandler(s.deps.Ctx, req)
	testhelpers.AssertHumaError(s.T(), err, 401)
}

func (s *TagHandlerIntegrationSuite) TestMergeTags_MissingTarget() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	source := s.createTagViaHandler(admin, "shoe-gaze", models.TagCategoryGenre)

	ctx := testhelpers.CtxWithUser(admin)
	req := &MergeTagsRequest{SourceID: fmt.Sprintf("%d", source.Body.ID)}
	req.Body.TargetID = 0

	_, err := s.handler.MergeTagsHandler(ctx, req)
	testhelpers.AssertHumaError(s.T(), err, 400)
}

func (s *TagHandlerIntegrationSuite) TestMergeTags_SelfMergeRejected() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	tag := s.createTagViaHandler(admin, "shoegaze", models.TagCategoryGenre)

	ctx := testhelpers.CtxWithUser(admin)
	req := &MergeTagsRequest{SourceID: fmt.Sprintf("%d", tag.Body.ID)}
	req.Body.TargetID = tag.Body.ID

	_, err := s.handler.MergeTagsHandler(ctx, req)
	testhelpers.AssertHumaError(s.T(), err, 400)
}

func (s *TagHandlerIntegrationSuite) TestMergeTags_InvalidSource() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	target := s.createTagViaHandler(admin, "shoegaze", models.TagCategoryGenre)

	ctx := testhelpers.CtxWithUser(admin)
	req := &MergeTagsRequest{SourceID: "notanumber"}
	req.Body.TargetID = target.Body.ID

	_, err := s.handler.MergeTagsHandler(ctx, req)
	testhelpers.AssertHumaError(s.T(), err, 400)
}

func (s *TagHandlerIntegrationSuite) TestMergePreview_Success() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	source := s.createTagViaHandler(admin, "shoe-gaze", models.TagCategoryGenre)
	target := s.createTagViaHandler(admin, "shoegaze", models.TagCategoryGenre)

	ctx := testhelpers.CtxWithUser(admin)
	req := &MergeTagsPreviewRequest{
		SourceID: fmt.Sprintf("%d", source.Body.ID),
		TargetID: target.Body.ID,
	}

	resp, err := s.handler.MergeTagsPreviewHandler(ctx, req)
	s.Require().NoError(err)
	s.Require().NotNil(resp.Body)
	s.Equal("shoe-gaze", resp.Body.SourceName)
	s.Equal("shoegaze", resp.Body.TargetName)
}

func (s *TagHandlerIntegrationSuite) TestMergePreview_NonAdmin() {
	user := testhelpers.CreateTestUser(s.deps.DB)
	ctx := testhelpers.CtxWithUser(user)
	req := &MergeTagsPreviewRequest{SourceID: "1", TargetID: 2}
	_, err := s.handler.MergeTagsPreviewHandler(ctx, req)
	testhelpers.AssertHumaError(s.T(), err, 403)
}

// ============================================================================
// Low-Quality Tag Queue (PSY-310)
// ============================================================================

func (s *TagHandlerIntegrationSuite) TestListLowQualityTags_Admin() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	// Seed an orphaned tag so the queue has content.
	orphan := &models.Tag{
		Name:     "orphan",
		Slug:     "orphan-lq-admin",
		Category: "other",
	}
	s.Require().NoError(s.deps.DB.Create(orphan).Error)

	ctx := testhelpers.CtxWithUser(admin)
	resp, err := s.handler.ListLowQualityTagsHandler(ctx, &ListLowQualityTagsRequest{Limit: 20, Offset: 0})
	s.Require().NoError(err)
	s.Require().NotNil(resp.Body)
	s.Require().Len(resp.Body.Tags, 1)
	s.Assert().Equal(orphan.ID, resp.Body.Tags[0].ID)
	s.Assert().Contains(resp.Body.Tags[0].Reasons, "orphaned")
}

func (s *TagHandlerIntegrationSuite) TestListLowQualityTags_NonAdmin() {
	user := testhelpers.CreateTestUser(s.deps.DB)
	ctx := testhelpers.CtxWithUser(user)
	_, err := s.handler.ListLowQualityTagsHandler(ctx, &ListLowQualityTagsRequest{})
	testhelpers.AssertHumaError(s.T(), err, 403)
}

func (s *TagHandlerIntegrationSuite) TestListLowQualityTags_Unauthenticated() {
	_, err := s.handler.ListLowQualityTagsHandler(context.Background(), &ListLowQualityTagsRequest{})
	testhelpers.AssertHumaError(s.T(), err, 401)
}

func (s *TagHandlerIntegrationSuite) TestSnoozeTag_Admin_WritesAuditLog() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	tag := &models.Tag{
		Name:     "to-snooze",
		Slug:     "to-snooze-lq",
		Category: "other",
	}
	s.Require().NoError(s.deps.DB.Create(tag).Error)

	ctx := testhelpers.CtxWithUser(admin)
	_, err := s.handler.SnoozeTagHandler(ctx, &SnoozeTagRequest{TagID: fmt.Sprintf("%d", tag.ID)})
	s.Require().NoError(err)

	// reviewed_at should now be set.
	var refreshed models.Tag
	s.Require().NoError(s.deps.DB.First(&refreshed, tag.ID).Error)
	s.Require().NotNil(refreshed.ReviewedAt)

	// Audit log fires via goroutine — poll briefly so the goroutine wins.
	var log models.AuditLog
	for i := 0; i < 40; i++ {
		if err := s.deps.DB.Where("action = ? AND entity_id = ?", "snooze_low_quality_tag", tag.ID).First(&log).Error; err == nil {
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	s.Require().NotZero(log.ID, "audit log was not written in time")
	s.Equal("tag", log.EntityType)
	s.Require().NotNil(log.ActorID)
	s.Equal(admin.ID, *log.ActorID)
}

func (s *TagHandlerIntegrationSuite) TestSnoozeTag_NotFound() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	ctx := testhelpers.CtxWithUser(admin)
	_, err := s.handler.SnoozeTagHandler(ctx, &SnoozeTagRequest{TagID: "99999"})
	testhelpers.AssertHumaError(s.T(), err, 404)
}

func (s *TagHandlerIntegrationSuite) TestSnoozeTag_NonAdmin() {
	user := testhelpers.CreateTestUser(s.deps.DB)
	ctx := testhelpers.CtxWithUser(user)
	_, err := s.handler.SnoozeTagHandler(ctx, &SnoozeTagRequest{TagID: "1"})
	testhelpers.AssertHumaError(s.T(), err, 403)
}

func (s *TagHandlerIntegrationSuite) TestSnoozeTag_InvalidID() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	ctx := testhelpers.CtxWithUser(admin)
	_, err := s.handler.SnoozeTagHandler(ctx, &SnoozeTagRequest{TagID: "abc"})
	testhelpers.AssertHumaError(s.T(), err, 400)
}

// ============================================================================
// BulkLowQualityTagsHandler (PSY-487)
// ============================================================================

func (s *TagHandlerIntegrationSuite) TestBulkLowQualityTags_Snooze_Success() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	t1 := &models.Tag{Name: "bulk-snooze-1", Slug: "bulk-snooze-1", Category: "other"}
	t2 := &models.Tag{Name: "bulk-snooze-2", Slug: "bulk-snooze-2", Category: "other"}
	s.Require().NoError(s.deps.DB.Create(t1).Error)
	s.Require().NoError(s.deps.DB.Create(t2).Error)

	req := &BulkLowQualityTagsRequest{}
	req.Body.Action = "snooze"
	req.Body.TagIDs = []uint{t1.ID, t2.ID}

	ctx := testhelpers.CtxWithUser(admin)
	resp, err := s.handler.BulkLowQualityTagsHandler(ctx, req)
	s.Require().NoError(err)
	s.Require().NotNil(resp)
	s.Equal("snooze", resp.Body.Action)
	s.EqualValues(2, resp.Body.Affected)
	s.EqualValues(0, resp.Body.NotFound)

	var refreshed models.Tag
	s.Require().NoError(s.deps.DB.First(&refreshed, t1.ID).Error)
	s.Require().NotNil(refreshed.ReviewedAt)
}

func (s *TagHandlerIntegrationSuite) TestBulkLowQualityTags_Delete_Success() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	t1 := &models.Tag{Name: "bulk-del-1", Slug: "bulk-del-1", Category: "other"}
	s.Require().NoError(s.deps.DB.Create(t1).Error)

	req := &BulkLowQualityTagsRequest{}
	req.Body.Action = "delete"
	req.Body.TagIDs = []uint{t1.ID}

	ctx := testhelpers.CtxWithUser(admin)
	resp, err := s.handler.BulkLowQualityTagsHandler(ctx, req)
	s.Require().NoError(err)
	s.EqualValues(1, resp.Body.Affected)

	var count int64
	s.Require().NoError(s.deps.DB.Model(&models.Tag{}).Where("id = ?", t1.ID).Count(&count).Error)
	s.EqualValues(0, count)
}

func (s *TagHandlerIntegrationSuite) TestBulkLowQualityTags_MarkOfficial_Success() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	t1 := &models.Tag{Name: "bulk-promo-1", Slug: "bulk-promo-1", Category: "other"}
	s.Require().NoError(s.deps.DB.Create(t1).Error)

	req := &BulkLowQualityTagsRequest{}
	req.Body.Action = "mark_official"
	req.Body.TagIDs = []uint{t1.ID}

	ctx := testhelpers.CtxWithUser(admin)
	resp, err := s.handler.BulkLowQualityTagsHandler(ctx, req)
	s.Require().NoError(err)
	s.EqualValues(1, resp.Body.Affected)

	var refreshed models.Tag
	s.Require().NoError(s.deps.DB.First(&refreshed, t1.ID).Error)
	s.True(refreshed.IsOfficial)
}

func (s *TagHandlerIntegrationSuite) TestBulkLowQualityTags_MissingAction() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	req := &BulkLowQualityTagsRequest{}
	req.Body.TagIDs = []uint{1}

	ctx := testhelpers.CtxWithUser(admin)
	_, err := s.handler.BulkLowQualityTagsHandler(ctx, req)
	testhelpers.AssertHumaError(s.T(), err, 400)
}

func (s *TagHandlerIntegrationSuite) TestBulkLowQualityTags_EmptyIDs() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	req := &BulkLowQualityTagsRequest{}
	req.Body.Action = "snooze"
	req.Body.TagIDs = []uint{}

	ctx := testhelpers.CtxWithUser(admin)
	_, err := s.handler.BulkLowQualityTagsHandler(ctx, req)
	testhelpers.AssertHumaError(s.T(), err, 400)
}

func (s *TagHandlerIntegrationSuite) TestBulkLowQualityTags_UnknownAction() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	t1 := &models.Tag{Name: "tag-x", Slug: "tag-x-bulk", Category: "other"}
	s.Require().NoError(s.deps.DB.Create(t1).Error)

	req := &BulkLowQualityTagsRequest{}
	req.Body.Action = "explode"
	req.Body.TagIDs = []uint{t1.ID}

	ctx := testhelpers.CtxWithUser(admin)
	_, err := s.handler.BulkLowQualityTagsHandler(ctx, req)
	testhelpers.AssertHumaError(s.T(), err, 400)
}

func (s *TagHandlerIntegrationSuite) TestBulkLowQualityTags_NonAdmin() {
	user := testhelpers.CreateTestUser(s.deps.DB)
	req := &BulkLowQualityTagsRequest{}
	req.Body.Action = "snooze"
	req.Body.TagIDs = []uint{1}

	ctx := testhelpers.CtxWithUser(user)
	_, err := s.handler.BulkLowQualityTagsHandler(ctx, req)
	testhelpers.AssertHumaError(s.T(), err, 403)
}

func (s *TagHandlerIntegrationSuite) TestBulkLowQualityTags_Unauthenticated() {
	req := &BulkLowQualityTagsRequest{}
	req.Body.Action = "snooze"
	req.Body.TagIDs = []uint{1}

	_, err := s.handler.BulkLowQualityTagsHandler(context.Background(), req)
	testhelpers.AssertHumaError(s.T(), err, 401)
}

func (s *TagHandlerIntegrationSuite) TestBulkLowQualityTags_NotFoundCounted() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	t1 := &models.Tag{Name: "real", Slug: "real-bulk", Category: "other"}
	s.Require().NoError(s.deps.DB.Create(t1).Error)

	req := &BulkLowQualityTagsRequest{}
	req.Body.Action = "snooze"
	req.Body.TagIDs = []uint{t1.ID, 999998}

	ctx := testhelpers.CtxWithUser(admin)
	resp, err := s.handler.BulkLowQualityTagsHandler(ctx, req)
	s.Require().NoError(err)
	s.EqualValues(2, resp.Body.Requested)
	s.EqualValues(1, resp.Body.Affected)
	s.EqualValues(1, resp.Body.NotFound)
}

func (s *TagHandlerIntegrationSuite) TestBulkLowQualityTags_WritesAuditLog() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	t1 := &models.Tag{Name: "audit-target", Slug: "audit-target-bulk", Category: "other"}
	s.Require().NoError(s.deps.DB.Create(t1).Error)

	req := &BulkLowQualityTagsRequest{}
	req.Body.Action = "snooze"
	req.Body.TagIDs = []uint{t1.ID}

	ctx := testhelpers.CtxWithUser(admin)
	_, err := s.handler.BulkLowQualityTagsHandler(ctx, req)
	s.Require().NoError(err)

	// Audit log fires via goroutine — poll briefly so the goroutine wins.
	var log models.AuditLog
	for i := 0; i < 40; i++ {
		if err := s.deps.DB.Where("action = ?", "bulk_low_quality_tags").First(&log).Error; err == nil {
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	s.Require().NotZero(log.ID, "audit log was not written in time")
	s.Equal("tag", log.EntityType)
	s.Require().NotNil(log.ActorID)
	s.Equal(admin.ID, *log.ActorID)
}
