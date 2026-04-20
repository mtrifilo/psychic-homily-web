package handlers

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	"psychic-homily-backend/internal/models"
	"psychic-homily-backend/internal/services/contracts"
)

type TagHandlerIntegrationSuite struct {
	suite.Suite
	deps    *handlerIntegrationDeps
	handler *TagHandler
}

func (s *TagHandlerIntegrationSuite) SetupSuite() {
	s.deps = setupHandlerIntegrationDeps(s.T())
	s.handler = NewTagHandler(s.deps.tagService, s.deps.auditLogService)
}

func (s *TagHandlerIntegrationSuite) TearDownTest() {
	cleanupTables(s.deps.db)
	sqlDB, _ := s.deps.db.DB()
	_, _ = sqlDB.Exec("DELETE FROM tag_votes")
	_, _ = sqlDB.Exec("DELETE FROM entity_tags")
	_, _ = sqlDB.Exec("DELETE FROM tag_aliases")
	_, _ = sqlDB.Exec("DELETE FROM tags")
}

func (s *TagHandlerIntegrationSuite) TearDownSuite() {
	s.deps.testDB.Cleanup()
}

func TestTagHandlerIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	suite.Run(t, new(TagHandlerIntegrationSuite))
}

// --- Helpers ---

func (s *TagHandlerIntegrationSuite) createTagViaHandler(admin *models.User, name, category string) *CreateTagResponse {
	ctx := ctxWithUser(admin)
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
	admin := createAdminUser(s.deps.db)
	ctx := ctxWithUser(admin)

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
	admin := createAdminUser(s.deps.db)
	created := s.createTagViaHandler(admin, "math-rock", models.TagCategoryGenre)

	// Fetch via GetTag and verify attribution persists
	ctx := ctxWithUser(admin)
	getReq := &GetTagRequest{TagID: fmt.Sprintf("%d", created.Body.ID)}
	getResp, err := s.handler.GetTagHandler(ctx, getReq)
	s.NoError(err)
	s.NotNil(getResp)
	s.NotNil(getResp.Body.CreatedByUserID)
	s.Equal(admin.ID, *getResp.Body.CreatedByUserID)
	// CreatedByUsername may be nil if test user has no username set
}

func (s *TagHandlerIntegrationSuite) TestCreateTag_WithDescription() {
	admin := createAdminUser(s.deps.db)
	ctx := ctxWithUser(admin)

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
	admin := createAdminUser(s.deps.db)

	// Create parent tag
	parent := s.createTagViaHandler(admin, "rock", models.TagCategoryGenre)

	// Create child tag with parent
	ctx := ctxWithUser(admin)
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
	admin := createAdminUser(s.deps.db)
	s.createTagViaHandler(admin, "post-punk", models.TagCategoryGenre)

	ctx := ctxWithUser(admin)
	req := &CreateTagRequest{}
	req.Body.Name = "post-punk"
	req.Body.Category = models.TagCategoryGenre

	_, err := s.handler.CreateTagHandler(ctx, req)
	assertHumaError(s.T(), err, 409)
}

func (s *TagHandlerIntegrationSuite) TestCreateTag_MissingName() {
	admin := createAdminUser(s.deps.db)
	ctx := ctxWithUser(admin)

	req := &CreateTagRequest{}
	req.Body.Name = ""
	req.Body.Category = models.TagCategoryGenre

	_, err := s.handler.CreateTagHandler(ctx, req)
	assertHumaError(s.T(), err, 400)
}

func (s *TagHandlerIntegrationSuite) TestCreateTag_MissingCategory() {
	admin := createAdminUser(s.deps.db)
	ctx := ctxWithUser(admin)

	req := &CreateTagRequest{}
	req.Body.Name = "post-punk"
	req.Body.Category = ""

	_, err := s.handler.CreateTagHandler(ctx, req)
	assertHumaError(s.T(), err, 400)
}

func (s *TagHandlerIntegrationSuite) TestCreateTag_NonAdmin() {
	user := createTestUser(s.deps.db)
	ctx := ctxWithUser(user)

	req := &CreateTagRequest{}
	req.Body.Name = "post-punk"
	req.Body.Category = models.TagCategoryGenre

	_, err := s.handler.CreateTagHandler(ctx, req)
	assertHumaError(s.T(), err, 403)
}

func (s *TagHandlerIntegrationSuite) TestCreateTag_NoAuth() {
	req := &CreateTagRequest{}
	req.Body.Name = "post-punk"
	req.Body.Category = models.TagCategoryGenre

	_, err := s.handler.CreateTagHandler(s.deps.ctx, req)
	assertHumaError(s.T(), err, 401)
}

// ============================================================================
// GetTagHandler
// ============================================================================

func (s *TagHandlerIntegrationSuite) TestGetTag_ByID() {
	admin := createAdminUser(s.deps.db)
	created := s.createTagViaHandler(admin, "shoegaze", models.TagCategoryGenre)

	req := &GetTagRequest{TagID: fmt.Sprintf("%d", created.Body.ID)}
	resp, err := s.handler.GetTagHandler(s.deps.ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.Equal("shoegaze", resp.Body.Name)
	s.Equal(created.Body.ID, resp.Body.ID)
}

func (s *TagHandlerIntegrationSuite) TestGetTag_BySlug() {
	admin := createAdminUser(s.deps.db)
	created := s.createTagViaHandler(admin, "shoegaze", models.TagCategoryGenre)

	req := &GetTagRequest{TagID: created.Body.Slug}
	resp, err := s.handler.GetTagHandler(s.deps.ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.Equal("shoegaze", resp.Body.Name)
}

func (s *TagHandlerIntegrationSuite) TestGetTag_NotFound() {
	req := &GetTagRequest{TagID: "99999"}
	_, err := s.handler.GetTagHandler(s.deps.ctx, req)
	assertHumaError(s.T(), err, 404)
}

func (s *TagHandlerIntegrationSuite) TestGetTag_NotFoundBySlug() {
	req := &GetTagRequest{TagID: "nonexistent-tag"}
	_, err := s.handler.GetTagHandler(s.deps.ctx, req)
	assertHumaError(s.T(), err, 404)
}

// ============================================================================
// GetTagDetailHandler (enriched detail endpoint)
// ============================================================================

func (s *TagHandlerIntegrationSuite) TestGetTagDetail_NotFound() {
	req := &GetTagDetailRequest{TagID: "does-not-exist"}
	_, err := s.handler.GetTagDetailHandler(s.deps.ctx, req)
	assertHumaError(s.T(), err, 404)
}

func (s *TagHandlerIntegrationSuite) TestGetTagDetail_Minimal_BySlug() {
	admin := createAdminUser(s.deps.db)
	created := s.createTagViaHandler(admin, "detail-tag", models.TagCategoryGenre)

	req := &GetTagDetailRequest{TagID: created.Body.Slug}
	resp, err := s.handler.GetTagDetailHandler(s.deps.ctx, req)
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
	admin := createAdminUser(s.deps.db)
	ctx := ctxWithUser(admin)

	desc := "Rendered **markdown** body."
	req := &CreateTagRequest{}
	req.Body.Name = "desc-tag"
	req.Body.Category = models.TagCategoryGenre
	req.Body.Description = &desc

	created, err := s.handler.CreateTagHandler(ctx, req)
	s.Require().NoError(err)

	detailReq := &GetTagDetailRequest{TagID: fmt.Sprintf("%d", created.Body.ID)}
	resp, err := s.handler.GetTagDetailHandler(s.deps.ctx, detailReq)
	s.NoError(err)
	s.Require().NotNil(resp)

	s.Contains(resp.Body.DescriptionHTML, "<strong>markdown</strong>")
}

func (s *TagHandlerIntegrationSuite) TestGetTagDetail_ParentAndChildren() {
	admin := createAdminUser(s.deps.db)
	ctx := ctxWithUser(admin)

	parent := s.createTagViaHandler(admin, "post-punk", models.TagCategoryGenre)

	childReq := &CreateTagRequest{}
	childReq.Body.Name = "shoegaze"
	childReq.Body.Category = models.TagCategoryGenre
	parentID := parent.Body.ID
	childReq.Body.ParentID = &parentID
	childCreated, err := s.handler.CreateTagHandler(ctx, childReq)
	s.Require().NoError(err)

	// Parent detail has the child.
	parentResp, err := s.handler.GetTagDetailHandler(s.deps.ctx, &GetTagDetailRequest{TagID: parent.Body.Slug})
	s.NoError(err)
	s.Require().NotNil(parentResp)
	s.Len(parentResp.Body.Children, 1)
	s.Equal(childCreated.Body.ID, parentResp.Body.Children[0].ID)

	// Child detail has the parent.
	childResp, err := s.handler.GetTagDetailHandler(s.deps.ctx, &GetTagDetailRequest{TagID: childCreated.Body.Slug})
	s.NoError(err)
	s.Require().NotNil(childResp)
	s.Require().NotNil(childResp.Body.Parent)
	s.Equal(parent.Body.ID, childResp.Body.Parent.ID)
	s.Equal("post-punk", childResp.Body.Parent.Name)
}

func (s *TagHandlerIntegrationSuite) TestGetTagDetail_UsageBreakdownAcrossTypes() {
	admin := createAdminUser(s.deps.db)
	ctx := ctxWithUser(admin)
	tag := s.createTagViaHandler(admin, "multi-type", models.TagCategoryGenre)

	// Tag 1 artist and 1 venue.
	artist := createArtist(s.deps.db, "Handler Test Band")
	venue := createVerifiedVenue(s.deps.db, "Handler Test Venue", "Phoenix", "AZ")

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

	resp, err := s.handler.GetTagDetailHandler(s.deps.ctx, &GetTagDetailRequest{TagID: tag.Body.Slug})
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
	admin := createAdminUser(s.deps.db)
	// Give admin a username so CreatedBy has a usable handle.
	aliceUsername := "alice-admin"
	s.deps.db.Model(admin).Update("username", aliceUsername)
	admin.Username = &aliceUsername
	ctx := ctxWithUser(admin)

	tag := s.createTagViaHandler(admin, "contrib-detail", models.TagCategoryGenre)

	artist := createArtist(s.deps.db, "Contrib Artist")
	applyReq := &AddTagToEntityRequest{
		EntityType: models.TagEntityArtist,
		EntityID:   fmt.Sprintf("%d", artist.ID),
	}
	applyReq.Body.TagID = tag.Body.ID
	_, err := s.handler.AddTagToEntityHandler(ctx, applyReq)
	s.Require().NoError(err)

	resp, err := s.handler.GetTagDetailHandler(s.deps.ctx, &GetTagDetailRequest{TagID: tag.Body.Slug})
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
	admin := createAdminUser(s.deps.db)
	// Ensure contributor tier so AddTagToEntity passes the trust gate if it ever
	// attempts inline creation. IsAdmin bypasses the gate, but be explicit.
	s.deps.db.Model(admin).Update("user_tier", "contributor")
	ctx := ctxWithUser(admin)

	focus := s.createTagViaHandler(admin, "focus-tag", models.TagCategoryGenre)
	related := s.createTagViaHandler(admin, "related-tag", models.TagCategoryGenre)

	artist := createArtist(s.deps.db, "Related Artist")
	for _, tagID := range []uint{focus.Body.ID, related.Body.ID} {
		r := &AddTagToEntityRequest{
			EntityType: models.TagEntityArtist,
			EntityID:   fmt.Sprintf("%d", artist.ID),
		}
		r.Body.TagID = tagID
		_, err := s.handler.AddTagToEntityHandler(ctx, r)
		s.Require().NoError(err)
	}

	resp, err := s.handler.GetTagDetailHandler(s.deps.ctx, &GetTagDetailRequest{TagID: focus.Body.Slug})
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
	admin := createAdminUser(s.deps.db)
	tag := s.createTagViaHandler(admin, "alias-detail", models.TagCategoryGenre)

	ctx := ctxWithUser(admin)
	aliasReq := &CreateAliasRequest{TagID: fmt.Sprintf("%d", tag.Body.ID)}
	aliasReq.Body.Alias = "detail-aka"
	_, err := s.handler.CreateAliasHandler(ctx, aliasReq)
	s.Require().NoError(err)

	resp, err := s.handler.GetTagDetailHandler(s.deps.ctx, &GetTagDetailRequest{TagID: tag.Body.Slug})
	s.NoError(err)
	s.Require().NotNil(resp)
	s.Contains(resp.Body.Aliases, "detail-aka")
}

// ============================================================================
// ListTagsHandler
// ============================================================================

func (s *TagHandlerIntegrationSuite) TestListTags_Success() {
	admin := createAdminUser(s.deps.db)
	s.createTagViaHandler(admin, "post-punk", models.TagCategoryGenre)
	s.createTagViaHandler(admin, "shoegaze", models.TagCategoryGenre)
	s.createTagViaHandler(admin, "melancholy", models.TagCategoryOther)

	req := &ListTagsRequest{}
	resp, err := s.handler.ListTagsHandler(s.deps.ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.Equal(int64(3), resp.Body.Total)
	s.Len(resp.Body.Tags, 3)
}

func (s *TagHandlerIntegrationSuite) TestListTags_FilterByCategory() {
	admin := createAdminUser(s.deps.db)
	s.createTagViaHandler(admin, "post-punk", models.TagCategoryGenre)
	s.createTagViaHandler(admin, "shoegaze", models.TagCategoryGenre)
	s.createTagViaHandler(admin, "melancholy", models.TagCategoryOther)

	req := &ListTagsRequest{Category: models.TagCategoryGenre}
	resp, err := s.handler.ListTagsHandler(s.deps.ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.Equal(int64(2), resp.Body.Total)
	for _, tag := range resp.Body.Tags {
		s.Equal("genre", tag.Category)
	}
}

func (s *TagHandlerIntegrationSuite) TestListTags_Search() {
	admin := createAdminUser(s.deps.db)
	s.createTagViaHandler(admin, "post-punk", models.TagCategoryGenre)
	s.createTagViaHandler(admin, "post-rock", models.TagCategoryGenre)
	s.createTagViaHandler(admin, "shoegaze", models.TagCategoryGenre)

	req := &ListTagsRequest{Search: "post"}
	resp, err := s.handler.ListTagsHandler(s.deps.ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.Equal(int64(2), resp.Body.Total)
}

func (s *TagHandlerIntegrationSuite) TestListTags_Pagination() {
	admin := createAdminUser(s.deps.db)
	for i := 0; i < 5; i++ {
		s.createTagViaHandler(admin, fmt.Sprintf("tag-%d", i), models.TagCategoryGenre)
	}

	req := &ListTagsRequest{Limit: 2, Offset: 0}
	resp, err := s.handler.ListTagsHandler(s.deps.ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.Equal(int64(5), resp.Body.Total)
	s.Len(resp.Body.Tags, 2)

	// Second page
	req2 := &ListTagsRequest{Limit: 2, Offset: 2}
	resp2, err := s.handler.ListTagsHandler(s.deps.ctx, req2)
	s.NoError(err)
	s.Len(resp2.Body.Tags, 2)
}

func (s *TagHandlerIntegrationSuite) TestListTags_Empty() {
	req := &ListTagsRequest{}
	resp, err := s.handler.ListTagsHandler(s.deps.ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.Equal(int64(0), resp.Body.Total)
	s.Empty(resp.Body.Tags)
}

// ============================================================================
// SearchTagsHandler
// ============================================================================

func (s *TagHandlerIntegrationSuite) TestSearchTags_Success() {
	admin := createAdminUser(s.deps.db)
	s.createTagViaHandler(admin, "post-punk", models.TagCategoryGenre)
	s.createTagViaHandler(admin, "post-rock", models.TagCategoryGenre)
	s.createTagViaHandler(admin, "shoegaze", models.TagCategoryGenre)

	req := &SearchTagsRequest{Query: "post"}
	resp, err := s.handler.SearchTagsHandler(s.deps.ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.GreaterOrEqual(len(resp.Body.Tags), 2)
}

func (s *TagHandlerIntegrationSuite) TestSearchTags_NoResults() {
	admin := createAdminUser(s.deps.db)
	s.createTagViaHandler(admin, "post-punk", models.TagCategoryGenre)

	req := &SearchTagsRequest{Query: "zzzznonexistent"}
	resp, err := s.handler.SearchTagsHandler(s.deps.ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.Empty(resp.Body.Tags)
}

func (s *TagHandlerIntegrationSuite) TestSearchTags_EmptyQuery() {
	req := &SearchTagsRequest{Query: ""}
	_, err := s.handler.SearchTagsHandler(s.deps.ctx, req)
	assertHumaError(s.T(), err, 400)
}

func (s *TagHandlerIntegrationSuite) TestSearchTags_WithLimit() {
	admin := createAdminUser(s.deps.db)
	for i := 0; i < 10; i++ {
		s.createTagViaHandler(admin, fmt.Sprintf("rock-%d", i), models.TagCategoryGenre)
	}

	req := &SearchTagsRequest{Query: "rock", Limit: 3}
	resp, err := s.handler.SearchTagsHandler(s.deps.ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.LessOrEqual(len(resp.Body.Tags), 3)
}

// TestSearchTags_MatchedViaAlias covers PSY-442 — the autocomplete endpoint
// surfaces the specific alias that matched so the add-tag dialog can render
// a "matched `punk-rock`" caption under the canonical row.
func (s *TagHandlerIntegrationSuite) TestSearchTags_MatchedViaAlias() {
	admin := createAdminUser(s.deps.db)
	tag := s.createTagViaHandler(admin, "punk", models.TagCategoryGenre)

	// Seed an alias via the service layer (alias creation is admin-only via
	// the handler, but we don't need to exercise that path here).
	_, err := s.deps.tagService.CreateAlias(tag.Body.ID, "punk-rock")
	s.Require().NoError(err)

	req := &SearchTagsRequest{Query: "punk-rock"}
	resp, err := s.handler.SearchTagsHandler(s.deps.ctx, req)
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
	admin := createAdminUser(s.deps.db)
	tag := s.createTagViaHandler(admin, "punk", models.TagCategoryGenre)
	// An alias exists on the tag, but the query hits the canonical name directly.
	_, err := s.deps.tagService.CreateAlias(tag.Body.ID, "punk-rock")
	s.Require().NoError(err)

	req := &SearchTagsRequest{Query: "punk"}
	resp, err := s.handler.SearchTagsHandler(s.deps.ctx, req)
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
	admin := createAdminUser(s.deps.db)
	created := s.createTagViaHandler(admin, "post-punk", models.TagCategoryGenre)

	ctx := ctxWithUser(admin)
	newName := "Post-Punk Revival"
	req := &UpdateTagRequest{TagID: fmt.Sprintf("%d", created.Body.ID)}
	req.Body.Name = &newName

	resp, err := s.handler.UpdateTagHandler(ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.Equal("Post-Punk Revival", resp.Body.Name)
}

func (s *TagHandlerIntegrationSuite) TestUpdateTag_ChangeCategory() {
	admin := createAdminUser(s.deps.db)
	created := s.createTagViaHandler(admin, "dark", models.TagCategoryOther)

	ctx := ctxWithUser(admin)
	newCat := models.TagCategoryLocale
	req := &UpdateTagRequest{TagID: fmt.Sprintf("%d", created.Body.ID)}
	req.Body.Category = &newCat

	resp, err := s.handler.UpdateTagHandler(ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.Equal("locale", resp.Body.Category)
}

func (s *TagHandlerIntegrationSuite) TestUpdateTag_NonAdmin() {
	admin := createAdminUser(s.deps.db)
	created := s.createTagViaHandler(admin, "post-punk", models.TagCategoryGenre)

	user := createTestUser(s.deps.db)
	ctx := ctxWithUser(user)
	newName := "Updated"
	req := &UpdateTagRequest{TagID: fmt.Sprintf("%d", created.Body.ID)}
	req.Body.Name = &newName

	_, err := s.handler.UpdateTagHandler(ctx, req)
	assertHumaError(s.T(), err, 403)
}

func (s *TagHandlerIntegrationSuite) TestUpdateTag_NoAuth() {
	admin := createAdminUser(s.deps.db)
	created := s.createTagViaHandler(admin, "post-punk", models.TagCategoryGenre)

	newName := "Updated"
	req := &UpdateTagRequest{TagID: fmt.Sprintf("%d", created.Body.ID)}
	req.Body.Name = &newName

	_, err := s.handler.UpdateTagHandler(s.deps.ctx, req)
	assertHumaError(s.T(), err, 401)
}

func (s *TagHandlerIntegrationSuite) TestUpdateTag_NotFound() {
	admin := createAdminUser(s.deps.db)
	ctx := ctxWithUser(admin)
	newName := "Updated"
	req := &UpdateTagRequest{TagID: "99999"}
	req.Body.Name = &newName

	_, err := s.handler.UpdateTagHandler(ctx, req)
	assertHumaError(s.T(), err, 404)
}

// ============================================================================
// DeleteTagHandler
// ============================================================================

func (s *TagHandlerIntegrationSuite) TestDeleteTag_Success() {
	admin := createAdminUser(s.deps.db)
	created := s.createTagViaHandler(admin, "to-delete", models.TagCategoryGenre)

	ctx := ctxWithUser(admin)
	req := &DeleteTagRequest{TagID: fmt.Sprintf("%d", created.Body.ID)}
	_, err := s.handler.DeleteTagHandler(ctx, req)
	s.NoError(err)

	// Verify it's gone
	getReq := &GetTagRequest{TagID: fmt.Sprintf("%d", created.Body.ID)}
	_, err = s.handler.GetTagHandler(s.deps.ctx, getReq)
	assertHumaError(s.T(), err, 404)
}

func (s *TagHandlerIntegrationSuite) TestDeleteTag_NonAdmin() {
	admin := createAdminUser(s.deps.db)
	created := s.createTagViaHandler(admin, "protected-tag", models.TagCategoryGenre)

	user := createTestUser(s.deps.db)
	ctx := ctxWithUser(user)
	req := &DeleteTagRequest{TagID: fmt.Sprintf("%d", created.Body.ID)}

	_, err := s.handler.DeleteTagHandler(ctx, req)
	assertHumaError(s.T(), err, 403)
}

func (s *TagHandlerIntegrationSuite) TestDeleteTag_NoAuth() {
	admin := createAdminUser(s.deps.db)
	created := s.createTagViaHandler(admin, "protected-tag", models.TagCategoryGenre)

	req := &DeleteTagRequest{TagID: fmt.Sprintf("%d", created.Body.ID)}
	_, err := s.handler.DeleteTagHandler(s.deps.ctx, req)
	assertHumaError(s.T(), err, 401)
}

func (s *TagHandlerIntegrationSuite) TestDeleteTag_NotFound() {
	admin := createAdminUser(s.deps.db)
	ctx := ctxWithUser(admin)
	req := &DeleteTagRequest{TagID: "99999"}

	_, err := s.handler.DeleteTagHandler(ctx, req)
	assertHumaError(s.T(), err, 404)
}

// ============================================================================
// AddTagToEntityHandler
// ============================================================================

func (s *TagHandlerIntegrationSuite) TestAddTagToEntity_ByTagID() {
	admin := createAdminUser(s.deps.db)
	tag := s.createTagViaHandler(admin, "post-punk", models.TagCategoryGenre)
	artist := createArtist(s.deps.db, "Joy Division")

	user := createTestUser(s.deps.db)
	ctx := ctxWithUser(user)
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
	listResp, err := s.handler.ListEntityTagsHandler(s.deps.ctx, listReq)
	s.NoError(err)
	s.Len(listResp.Body.Tags, 1)
	s.Equal("post-punk", listResp.Body.Tags[0].Name)
}

func (s *TagHandlerIntegrationSuite) TestAddTagToEntity_ByTagName() {
	admin := createAdminUser(s.deps.db)
	s.createTagViaHandler(admin, "shoegaze", models.TagCategoryGenre)
	artist := createArtist(s.deps.db, "My Bloody Valentine")

	user := createTestUser(s.deps.db)
	ctx := ctxWithUser(user)
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
	listResp, err := s.handler.ListEntityTagsHandler(s.deps.ctx, listReq)
	s.NoError(err)
	s.Len(listResp.Body.Tags, 1)
	s.Equal("shoegaze", listResp.Body.Tags[0].Name)
}

func (s *TagHandlerIntegrationSuite) TestAddTagToEntity_Duplicate() {
	admin := createAdminUser(s.deps.db)
	tag := s.createTagViaHandler(admin, "post-punk", models.TagCategoryGenre)
	artist := createArtist(s.deps.db, "Siouxsie")

	user := createTestUser(s.deps.db)
	ctx := ctxWithUser(user)
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
	assertHumaError(s.T(), err, 409)
}

func (s *TagHandlerIntegrationSuite) TestAddTagToEntity_MissingFields() {
	user := createTestUser(s.deps.db)
	ctx := ctxWithUser(user)
	artist := createArtist(s.deps.db, "Test Artist")

	req := &AddTagToEntityRequest{
		EntityType: models.TagEntityArtist,
		EntityID:   fmt.Sprintf("%d", artist.ID),
	}
	// Both TagID and TagName are zero/empty

	_, err := s.handler.AddTagToEntityHandler(ctx, req)
	assertHumaError(s.T(), err, 400)
}

func (s *TagHandlerIntegrationSuite) TestAddTagToEntity_NoAuth() {
	admin := createAdminUser(s.deps.db)
	tag := s.createTagViaHandler(admin, "post-punk", models.TagCategoryGenre)
	artist := createArtist(s.deps.db, "Test Artist")

	req := &AddTagToEntityRequest{
		EntityType: models.TagEntityArtist,
		EntityID:   fmt.Sprintf("%d", artist.ID),
	}
	req.Body.TagID = tag.Body.ID

	_, err := s.handler.AddTagToEntityHandler(s.deps.ctx, req)
	assertHumaError(s.T(), err, 401)
}

// ============================================================================
// ListEntityTagsHandler
// ============================================================================

func (s *TagHandlerIntegrationSuite) TestListEntityTags_Success() {
	admin := createAdminUser(s.deps.db)
	tag := s.createTagViaHandler(admin, "post-punk", models.TagCategoryGenre)
	artist := createArtist(s.deps.db, "Joy Division")

	user := createTestUser(s.deps.db)
	ctx := ctxWithUser(user)
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
	resp, err := s.handler.ListEntityTagsHandler(s.deps.ctx, listReq)
	s.NoError(err)
	s.NotNil(resp)
	s.Len(resp.Body.Tags, 1)
	s.Equal("post-punk", resp.Body.Tags[0].Name)
}

func (s *TagHandlerIntegrationSuite) TestListEntityTags_Empty() {
	artist := createArtist(s.deps.db, "No Tags Artist")

	req := &ListEntityTagsRequest{
		EntityType: models.TagEntityArtist,
		EntityID:   fmt.Sprintf("%d", artist.ID),
	}
	resp, err := s.handler.ListEntityTagsHandler(s.deps.ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.Empty(resp.Body.Tags)
}

func (s *TagHandlerIntegrationSuite) TestListEntityTags_MultipleTags() {
	admin := createAdminUser(s.deps.db)
	tag1 := s.createTagViaHandler(admin, "post-punk", models.TagCategoryGenre)
	tag2 := s.createTagViaHandler(admin, "dark", models.TagCategoryOther)
	tag3 := s.createTagViaHandler(admin, "80s", models.TagCategoryLocale)
	artist := createArtist(s.deps.db, "Bauhaus")

	user := createTestUser(s.deps.db)
	ctx := ctxWithUser(user)

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
	resp, err := s.handler.ListEntityTagsHandler(s.deps.ctx, listReq)
	s.NoError(err)
	s.Len(resp.Body.Tags, 3)
}

// ============================================================================
// RemoveTagFromEntityHandler
// ============================================================================

func (s *TagHandlerIntegrationSuite) TestRemoveTagFromEntity_Success() {
	admin := createAdminUser(s.deps.db)
	tag := s.createTagViaHandler(admin, "post-punk", models.TagCategoryGenre)
	artist := createArtist(s.deps.db, "Wire")

	user := createTestUser(s.deps.db)
	ctx := ctxWithUser(user)

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
	listResp, err := s.handler.ListEntityTagsHandler(s.deps.ctx, listReq)
	s.NoError(err)
	s.Empty(listResp.Body.Tags)
}

func (s *TagHandlerIntegrationSuite) TestRemoveTagFromEntity_NotFound() {
	user := createTestUser(s.deps.db)
	ctx := ctxWithUser(user)
	artist := createArtist(s.deps.db, "Test Artist")

	req := &RemoveTagFromEntityRequest{
		EntityType: models.TagEntityArtist,
		EntityID:   fmt.Sprintf("%d", artist.ID),
		TagID:      "99999",
	}
	_, err := s.handler.RemoveTagFromEntityHandler(ctx, req)
	assertHumaError(s.T(), err, 404)
}

func (s *TagHandlerIntegrationSuite) TestRemoveTagFromEntity_NoAuth() {
	admin := createAdminUser(s.deps.db)
	tag := s.createTagViaHandler(admin, "post-punk", models.TagCategoryGenre)
	artist := createArtist(s.deps.db, "Test Artist")

	req := &RemoveTagFromEntityRequest{
		EntityType: models.TagEntityArtist,
		EntityID:   fmt.Sprintf("%d", artist.ID),
		TagID:      fmt.Sprintf("%d", tag.Body.ID),
	}
	_, err := s.handler.RemoveTagFromEntityHandler(s.deps.ctx, req)
	assertHumaError(s.T(), err, 401)
}

// ============================================================================
// VoteTagHandler
// ============================================================================

func (s *TagHandlerIntegrationSuite) TestVoteTag_Upvote() {
	admin := createAdminUser(s.deps.db)
	tag := s.createTagViaHandler(admin, "post-punk", models.TagCategoryGenre)
	artist := createArtist(s.deps.db, "Joy Division")

	// First add the tag to the entity
	user := createTestUser(s.deps.db)
	ctx := ctxWithUser(user)
	addReq := &AddTagToEntityRequest{
		EntityType: models.TagEntityArtist,
		EntityID:   fmt.Sprintf("%d", artist.ID),
	}
	addReq.Body.TagID = tag.Body.ID
	_, err := s.handler.AddTagToEntityHandler(ctx, addReq)
	s.Require().NoError(err)

	// Vote
	voter := createTestUser(s.deps.db)
	voteCtx := ctxWithUser(voter)
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
	admin := createAdminUser(s.deps.db)
	tag := s.createTagViaHandler(admin, "post-punk", models.TagCategoryGenre)
	artist := createArtist(s.deps.db, "Joy Division")

	// Add the tag
	user := createTestUser(s.deps.db)
	ctx := ctxWithUser(user)
	addReq := &AddTagToEntityRequest{
		EntityType: models.TagEntityArtist,
		EntityID:   fmt.Sprintf("%d", artist.ID),
	}
	addReq.Body.TagID = tag.Body.ID
	_, err := s.handler.AddTagToEntityHandler(ctx, addReq)
	s.Require().NoError(err)

	// Downvote
	voter := createTestUser(s.deps.db)
	voteCtx := ctxWithUser(voter)
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

	_, err := s.handler.VoteTagHandler(s.deps.ctx, req)
	assertHumaError(s.T(), err, 401)
}

// ============================================================================
// RemoveTagVoteHandler
// ============================================================================

func (s *TagHandlerIntegrationSuite) TestRemoveTagVote_Success() {
	admin := createAdminUser(s.deps.db)
	tag := s.createTagViaHandler(admin, "post-punk", models.TagCategoryGenre)
	artist := createArtist(s.deps.db, "Joy Division")

	// Add the tag
	user := createTestUser(s.deps.db)
	ctx := ctxWithUser(user)
	addReq := &AddTagToEntityRequest{
		EntityType: models.TagEntityArtist,
		EntityID:   fmt.Sprintf("%d", artist.ID),
	}
	addReq.Body.TagID = tag.Body.ID
	_, err := s.handler.AddTagToEntityHandler(ctx, addReq)
	s.Require().NoError(err)

	// Vote first
	voter := createTestUser(s.deps.db)
	voteCtx := ctxWithUser(voter)
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
	_, err := s.handler.RemoveTagVoteHandler(s.deps.ctx, req)
	assertHumaError(s.T(), err, 401)
}

// ============================================================================
// CreateAliasHandler
// ============================================================================

func (s *TagHandlerIntegrationSuite) TestCreateAlias_Success() {
	admin := createAdminUser(s.deps.db)
	tag := s.createTagViaHandler(admin, "post-punk", models.TagCategoryGenre)

	ctx := ctxWithUser(admin)
	req := &CreateAliasRequest{TagID: fmt.Sprintf("%d", tag.Body.ID)}
	req.Body.Alias = "post punk"

	resp, err := s.handler.CreateAliasHandler(ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.Equal("post punk", resp.Body.Alias)
	s.NotZero(resp.Body.ID)
}

func (s *TagHandlerIntegrationSuite) TestCreateAlias_NonAdmin() {
	admin := createAdminUser(s.deps.db)
	tag := s.createTagViaHandler(admin, "post-punk", models.TagCategoryGenre)

	user := createTestUser(s.deps.db)
	ctx := ctxWithUser(user)
	req := &CreateAliasRequest{TagID: fmt.Sprintf("%d", tag.Body.ID)}
	req.Body.Alias = "post punk"

	_, err := s.handler.CreateAliasHandler(ctx, req)
	assertHumaError(s.T(), err, 403)
}

func (s *TagHandlerIntegrationSuite) TestCreateAlias_NoAuth() {
	admin := createAdminUser(s.deps.db)
	tag := s.createTagViaHandler(admin, "post-punk", models.TagCategoryGenre)

	req := &CreateAliasRequest{TagID: fmt.Sprintf("%d", tag.Body.ID)}
	req.Body.Alias = "post punk"

	_, err := s.handler.CreateAliasHandler(s.deps.ctx, req)
	assertHumaError(s.T(), err, 401)
}

func (s *TagHandlerIntegrationSuite) TestCreateAlias_EmptyAlias() {
	admin := createAdminUser(s.deps.db)
	tag := s.createTagViaHandler(admin, "post-punk", models.TagCategoryGenre)

	ctx := ctxWithUser(admin)
	req := &CreateAliasRequest{TagID: fmt.Sprintf("%d", tag.Body.ID)}
	req.Body.Alias = ""

	_, err := s.handler.CreateAliasHandler(ctx, req)
	assertHumaError(s.T(), err, 400)
}

func (s *TagHandlerIntegrationSuite) TestCreateAlias_DuplicateAlias() {
	admin := createAdminUser(s.deps.db)
	tag := s.createTagViaHandler(admin, "post-punk", models.TagCategoryGenre)

	ctx := ctxWithUser(admin)

	// First alias
	req1 := &CreateAliasRequest{TagID: fmt.Sprintf("%d", tag.Body.ID)}
	req1.Body.Alias = "post punk"
	_, err := s.handler.CreateAliasHandler(ctx, req1)
	s.Require().NoError(err)

	// Duplicate alias
	req2 := &CreateAliasRequest{TagID: fmt.Sprintf("%d", tag.Body.ID)}
	req2.Body.Alias = "post punk"
	_, err = s.handler.CreateAliasHandler(ctx, req2)
	assertHumaError(s.T(), err, 409)
}

// ============================================================================
// ListAliasesHandler
// ============================================================================

func (s *TagHandlerIntegrationSuite) TestListAliases_Success() {
	admin := createAdminUser(s.deps.db)
	tag := s.createTagViaHandler(admin, "post-punk", models.TagCategoryGenre)

	ctx := ctxWithUser(admin)
	for _, alias := range []string{"post punk", "postpunk", "pp"} {
		req := &CreateAliasRequest{TagID: fmt.Sprintf("%d", tag.Body.ID)}
		req.Body.Alias = alias
		_, err := s.handler.CreateAliasHandler(ctx, req)
		s.Require().NoError(err)
	}

	listReq := &ListAliasesRequest{TagID: fmt.Sprintf("%d", tag.Body.ID)}
	resp, err := s.handler.ListAliasesHandler(s.deps.ctx, listReq)
	s.NoError(err)
	s.NotNil(resp)
	s.Len(resp.Body.Aliases, 3)
}

func (s *TagHandlerIntegrationSuite) TestListAliases_Empty() {
	admin := createAdminUser(s.deps.db)
	tag := s.createTagViaHandler(admin, "post-punk", models.TagCategoryGenre)

	req := &ListAliasesRequest{TagID: fmt.Sprintf("%d", tag.Body.ID)}
	resp, err := s.handler.ListAliasesHandler(s.deps.ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.Empty(resp.Body.Aliases)
}

func (s *TagHandlerIntegrationSuite) TestListAliases_BySlug() {
	admin := createAdminUser(s.deps.db)
	tag := s.createTagViaHandler(admin, "post-punk", models.TagCategoryGenre)

	ctx := ctxWithUser(admin)
	aliasReq := &CreateAliasRequest{TagID: fmt.Sprintf("%d", tag.Body.ID)}
	aliasReq.Body.Alias = "postpunk"
	_, err := s.handler.CreateAliasHandler(ctx, aliasReq)
	s.Require().NoError(err)

	// List by slug instead of ID
	listReq := &ListAliasesRequest{TagID: tag.Body.Slug}
	resp, err := s.handler.ListAliasesHandler(s.deps.ctx, listReq)
	s.NoError(err)
	s.NotNil(resp)
	s.Len(resp.Body.Aliases, 1)
}

func (s *TagHandlerIntegrationSuite) TestListAliases_TagNotFound() {
	req := &ListAliasesRequest{TagID: "99999"}
	_, err := s.handler.ListAliasesHandler(s.deps.ctx, req)
	assertHumaError(s.T(), err, 404)
}

// ============================================================================
// DeleteAliasHandler
// ============================================================================

func (s *TagHandlerIntegrationSuite) TestDeleteAlias_Success() {
	admin := createAdminUser(s.deps.db)
	tag := s.createTagViaHandler(admin, "post-punk", models.TagCategoryGenre)

	ctx := ctxWithUser(admin)
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
	listResp, err := s.handler.ListAliasesHandler(s.deps.ctx, listReq)
	s.NoError(err)
	s.Empty(listResp.Body.Aliases)
}

func (s *TagHandlerIntegrationSuite) TestDeleteAlias_NonAdmin() {
	admin := createAdminUser(s.deps.db)
	tag := s.createTagViaHandler(admin, "post-punk", models.TagCategoryGenre)

	ctx := ctxWithUser(admin)
	createReq := &CreateAliasRequest{TagID: fmt.Sprintf("%d", tag.Body.ID)}
	createReq.Body.Alias = "post punk"
	aliasResp, err := s.handler.CreateAliasHandler(ctx, createReq)
	s.Require().NoError(err)

	user := createTestUser(s.deps.db)
	userCtx := ctxWithUser(user)
	delReq := &DeleteAliasRequest{
		TagID:   fmt.Sprintf("%d", tag.Body.ID),
		AliasID: fmt.Sprintf("%d", aliasResp.Body.ID),
	}
	_, err = s.handler.DeleteAliasHandler(userCtx, delReq)
	assertHumaError(s.T(), err, 403)
}

func (s *TagHandlerIntegrationSuite) TestDeleteAlias_NoAuth() {
	admin := createAdminUser(s.deps.db)
	tag := s.createTagViaHandler(admin, "post-punk", models.TagCategoryGenre)

	ctx := ctxWithUser(admin)
	createReq := &CreateAliasRequest{TagID: fmt.Sprintf("%d", tag.Body.ID)}
	createReq.Body.Alias = "post punk"
	aliasResp, err := s.handler.CreateAliasHandler(ctx, createReq)
	s.Require().NoError(err)

	delReq := &DeleteAliasRequest{
		TagID:   fmt.Sprintf("%d", tag.Body.ID),
		AliasID: fmt.Sprintf("%d", aliasResp.Body.ID),
	}
	_, err = s.handler.DeleteAliasHandler(s.deps.ctx, delReq)
	assertHumaError(s.T(), err, 401)
}

// ============================================================================
// AddTagToEntity with alias resolution
// ============================================================================

func (s *TagHandlerIntegrationSuite) TestAddTagToEntity_ByAliasName() {
	admin := createAdminUser(s.deps.db)
	tag := s.createTagViaHandler(admin, "post-punk", models.TagCategoryGenre)

	// Create alias
	ctx := ctxWithUser(admin)
	aliasReq := &CreateAliasRequest{TagID: fmt.Sprintf("%d", tag.Body.ID)}
	aliasReq.Body.Alias = "postpunk"
	_, err := s.handler.CreateAliasHandler(ctx, aliasReq)
	s.Require().NoError(err)

	// Add tag using alias name
	artist := createArtist(s.deps.db, "Gang of Four")
	user := createTestUser(s.deps.db)
	userCtx := ctxWithUser(user)
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
	listResp, err := s.handler.ListEntityTagsHandler(s.deps.ctx, listReq)
	s.NoError(err)
	s.Len(listResp.Body.Tags, 1)
	s.Equal("post-punk", listResp.Body.Tags[0].Name)
}

// ============================================================================
// CreateTag with IsOfficial flag
// ============================================================================

func (s *TagHandlerIntegrationSuite) TestCreateTag_Official() {
	admin := createAdminUser(s.deps.db)
	ctx := ctxWithUser(admin)

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
	admin := createAdminUser(s.deps.db)
	created := s.createTagViaHandler(admin, "ambient", models.TagCategoryGenre)

	ctx := ctxWithUser(admin)
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
	admin := createAdminUser(s.deps.db)
	tag := s.createTagViaHandler(admin, "intimate", models.TagCategoryOther)
	venue := createVerifiedVenue(s.deps.db, "The Rebel Lounge", "Phoenix", "AZ")

	user := createTestUser(s.deps.db)
	ctx := ctxWithUser(user)
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
	listResp, err := s.handler.ListEntityTagsHandler(s.deps.ctx, listReq)
	s.NoError(err)
	s.Len(listResp.Body.Tags, 1)
	s.Equal("intimate", listResp.Body.Tags[0].Name)
}

// ============================================================================
// ListAllAliasesHandler + BulkImportAliasesHandler (PSY-307)
// ============================================================================

func (s *TagHandlerIntegrationSuite) TestListAllAliases_AdminOnly() {
	user := createTestUser(s.deps.db)
	ctx := ctxWithUser(user)

	_, err := s.handler.ListAllAliasesHandler(ctx, &ListAllAliasesRequest{})
	s.Error(err)
	s.Contains(err.Error(), "Admin")
}

func (s *TagHandlerIntegrationSuite) TestListAllAliases_Unauthenticated() {
	_, err := s.handler.ListAllAliasesHandler(s.deps.ctx, &ListAllAliasesRequest{})
	s.Error(err)
}

func (s *TagHandlerIntegrationSuite) TestListAllAliases_ReturnsAllWithCanonicalInfo() {
	admin := createAdminUser(s.deps.db)
	ctx := ctxWithUser(admin)
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
	admin := createAdminUser(s.deps.db)
	ctx := ctxWithUser(admin)
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
	user := createTestUser(s.deps.db)
	ctx := ctxWithUser(user)

	req := &BulkImportAliasesRequest{}
	req.Body.Items = []contracts.BulkAliasImportItem{{Alias: "x", Canonical: "y"}}
	_, err := s.handler.BulkImportAliasesHandler(ctx, req)
	s.Error(err)
	s.Contains(err.Error(), "Admin")
}

func (s *TagHandlerIntegrationSuite) TestBulkImportAliases_EmptyRejected() {
	admin := createAdminUser(s.deps.db)
	ctx := ctxWithUser(admin)

	req := &BulkImportAliasesRequest{}
	_, err := s.handler.BulkImportAliasesHandler(ctx, req)
	s.Error(err)
}

func (s *TagHandlerIntegrationSuite) TestBulkImportAliases_TooLargeRejected() {
	admin := createAdminUser(s.deps.db)
	ctx := ctxWithUser(admin)

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
	admin := createAdminUser(s.deps.db)
	ctx := ctxWithUser(admin)
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
	admin := createAdminUser(s.deps.db)
	source := s.createTagViaHandler(admin, "shoe-gaze", models.TagCategoryGenre)
	target := s.createTagViaHandler(admin, "shoegaze", models.TagCategoryGenre)

	ctx := ctxWithUser(admin)
	req := &MergeTagsRequest{SourceID: fmt.Sprintf("%d", source.Body.ID)}
	req.Body.TargetID = target.Body.ID

	resp, err := s.handler.MergeTagsHandler(ctx, req)
	s.Require().NoError(err)
	s.Require().NotNil(resp.Body)
	s.True(resp.Body.AliasCreated)

	// Source is gone.
	getReq := &GetTagRequest{TagID: fmt.Sprintf("%d", source.Body.ID)}
	_, err = s.handler.GetTagHandler(ctx, getReq)
	assertHumaError(s.T(), err, 404)
}

func (s *TagHandlerIntegrationSuite) TestMergeTags_NonAdmin() {
	admin := createAdminUser(s.deps.db)
	source := s.createTagViaHandler(admin, "shoe-gaze", models.TagCategoryGenre)
	target := s.createTagViaHandler(admin, "shoegaze", models.TagCategoryGenre)

	user := createTestUser(s.deps.db)
	ctx := ctxWithUser(user)
	req := &MergeTagsRequest{SourceID: fmt.Sprintf("%d", source.Body.ID)}
	req.Body.TargetID = target.Body.ID

	_, err := s.handler.MergeTagsHandler(ctx, req)
	assertHumaError(s.T(), err, 403)
}

func (s *TagHandlerIntegrationSuite) TestMergeTags_NoAuth() {
	req := &MergeTagsRequest{SourceID: "1"}
	req.Body.TargetID = 2
	_, err := s.handler.MergeTagsHandler(s.deps.ctx, req)
	assertHumaError(s.T(), err, 401)
}

func (s *TagHandlerIntegrationSuite) TestMergeTags_MissingTarget() {
	admin := createAdminUser(s.deps.db)
	source := s.createTagViaHandler(admin, "shoe-gaze", models.TagCategoryGenre)

	ctx := ctxWithUser(admin)
	req := &MergeTagsRequest{SourceID: fmt.Sprintf("%d", source.Body.ID)}
	req.Body.TargetID = 0

	_, err := s.handler.MergeTagsHandler(ctx, req)
	assertHumaError(s.T(), err, 400)
}

func (s *TagHandlerIntegrationSuite) TestMergeTags_SelfMergeRejected() {
	admin := createAdminUser(s.deps.db)
	tag := s.createTagViaHandler(admin, "shoegaze", models.TagCategoryGenre)

	ctx := ctxWithUser(admin)
	req := &MergeTagsRequest{SourceID: fmt.Sprintf("%d", tag.Body.ID)}
	req.Body.TargetID = tag.Body.ID

	_, err := s.handler.MergeTagsHandler(ctx, req)
	assertHumaError(s.T(), err, 400)
}

func (s *TagHandlerIntegrationSuite) TestMergeTags_InvalidSource() {
	admin := createAdminUser(s.deps.db)
	target := s.createTagViaHandler(admin, "shoegaze", models.TagCategoryGenre)

	ctx := ctxWithUser(admin)
	req := &MergeTagsRequest{SourceID: "notanumber"}
	req.Body.TargetID = target.Body.ID

	_, err := s.handler.MergeTagsHandler(ctx, req)
	assertHumaError(s.T(), err, 400)
}

func (s *TagHandlerIntegrationSuite) TestMergePreview_Success() {
	admin := createAdminUser(s.deps.db)
	source := s.createTagViaHandler(admin, "shoe-gaze", models.TagCategoryGenre)
	target := s.createTagViaHandler(admin, "shoegaze", models.TagCategoryGenre)

	ctx := ctxWithUser(admin)
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
	user := createTestUser(s.deps.db)
	ctx := ctxWithUser(user)
	req := &MergeTagsPreviewRequest{SourceID: "1", TargetID: 2}
	_, err := s.handler.MergeTagsPreviewHandler(ctx, req)
	assertHumaError(s.T(), err, 403)
}

// ============================================================================
// Low-Quality Tag Queue (PSY-310)
// ============================================================================

func (s *TagHandlerIntegrationSuite) TestListLowQualityTags_Admin() {
	admin := createAdminUser(s.deps.db)
	// Seed an orphaned tag so the queue has content.
	orphan := &models.Tag{
		Name:     "orphan",
		Slug:     "orphan-lq-admin",
		Category: "other",
	}
	s.Require().NoError(s.deps.db.Create(orphan).Error)

	ctx := ctxWithUser(admin)
	resp, err := s.handler.ListLowQualityTagsHandler(ctx, &ListLowQualityTagsRequest{Limit: 20, Offset: 0})
	s.Require().NoError(err)
	s.Require().NotNil(resp.Body)
	s.Require().Len(resp.Body.Tags, 1)
	s.Assert().Equal(orphan.ID, resp.Body.Tags[0].ID)
	s.Assert().Contains(resp.Body.Tags[0].Reasons, "orphaned")
}

func (s *TagHandlerIntegrationSuite) TestListLowQualityTags_NonAdmin() {
	user := createTestUser(s.deps.db)
	ctx := ctxWithUser(user)
	_, err := s.handler.ListLowQualityTagsHandler(ctx, &ListLowQualityTagsRequest{})
	assertHumaError(s.T(), err, 403)
}

func (s *TagHandlerIntegrationSuite) TestListLowQualityTags_Unauthenticated() {
	_, err := s.handler.ListLowQualityTagsHandler(context.Background(), &ListLowQualityTagsRequest{})
	assertHumaError(s.T(), err, 401)
}

func (s *TagHandlerIntegrationSuite) TestSnoozeTag_Admin_WritesAuditLog() {
	admin := createAdminUser(s.deps.db)
	tag := &models.Tag{
		Name:     "to-snooze",
		Slug:     "to-snooze-lq",
		Category: "other",
	}
	s.Require().NoError(s.deps.db.Create(tag).Error)

	ctx := ctxWithUser(admin)
	_, err := s.handler.SnoozeTagHandler(ctx, &SnoozeTagRequest{TagID: fmt.Sprintf("%d", tag.ID)})
	s.Require().NoError(err)

	// reviewed_at should now be set.
	var refreshed models.Tag
	s.Require().NoError(s.deps.db.First(&refreshed, tag.ID).Error)
	s.Require().NotNil(refreshed.ReviewedAt)

	// Audit log fires via goroutine — poll briefly so the goroutine wins.
	var log models.AuditLog
	for i := 0; i < 40; i++ {
		if err := s.deps.db.Where("action = ? AND entity_id = ?", "snooze_low_quality_tag", tag.ID).First(&log).Error; err == nil {
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
	admin := createAdminUser(s.deps.db)
	ctx := ctxWithUser(admin)
	_, err := s.handler.SnoozeTagHandler(ctx, &SnoozeTagRequest{TagID: "99999"})
	assertHumaError(s.T(), err, 404)
}

func (s *TagHandlerIntegrationSuite) TestSnoozeTag_NonAdmin() {
	user := createTestUser(s.deps.db)
	ctx := ctxWithUser(user)
	_, err := s.handler.SnoozeTagHandler(ctx, &SnoozeTagRequest{TagID: "1"})
	assertHumaError(s.T(), err, 403)
}

func (s *TagHandlerIntegrationSuite) TestSnoozeTag_InvalidID() {
	admin := createAdminUser(s.deps.db)
	ctx := ctxWithUser(admin)
	_, err := s.handler.SnoozeTagHandler(ctx, &SnoozeTagRequest{TagID: "abc"})
	assertHumaError(s.T(), err, 400)
}
