package handlers

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/danielgtaylor/huma/v2"

	"psychic-homily-backend/internal/api/middleware"
	apperrors "psychic-homily-backend/internal/errors"
	"psychic-homily-backend/internal/logger"
	"psychic-homily-backend/internal/models"
	"psychic-homily-backend/internal/services/contracts"
)

// TagHandler handles tag-related API requests.
type TagHandler struct {
	tagService contracts.TagServiceInterface
	auditLog   contracts.AuditLogServiceInterface
}

// NewTagHandler creates a new TagHandler.
func NewTagHandler(tagService contracts.TagServiceInterface, auditLog contracts.AuditLogServiceInterface) *TagHandler {
	return &TagHandler{
		tagService: tagService,
		auditLog:   auditLog,
	}
}

// ============================================================================
// List Tags (public)
// ============================================================================

type ListTagsRequest struct {
	Category string `query:"category" required:"false" doc:"Filter by category (genre, locale, other)"`
	Search   string `query:"search" required:"false" doc:"Search tags by name"`
	ParentID uint   `query:"parent_id" required:"false" doc:"Filter by parent tag ID"`
	Sort     string `query:"sort" required:"false" doc:"Sort by: usage, name, created (default: usage)"`
	Limit    int    `query:"limit" required:"false" doc:"Max results (default 50)" example:"50"`
	Offset   int    `query:"offset" required:"false" doc:"Offset for pagination" example:"0"`
	// EntityType scopes the per-tag usage_count in the response to a single
	// entity type (PSY-484). Used by the browse-page tag facet so the count
	// next to each chip reflects "tags applied to <this entity type>" rather
	// than the global cross-entity total. The /tags browse page omits this
	// param to keep the global count.
	EntityType string `query:"entity_type" required:"false" doc:"Scope usage_count to a single entity type (artist, release, label, show, venue, festival)"`
}

type ListTagsResponse struct {
	Body struct {
		Tags  []contracts.TagListItem `json:"tags"`
		Total int64                   `json:"total"`
	}
}

func (h *TagHandler) ListTagsHandler(ctx context.Context, req *ListTagsRequest) (*ListTagsResponse, error) {
	var parentID *uint
	if req.ParentID > 0 {
		parentID = &req.ParentID
	}

	if req.EntityType != "" && !models.IsValidTagEntityType(req.EntityType) {
		return nil, huma.Error400BadRequest("Invalid entity_type")
	}

	tags, total, err := h.tagService.ListTags(req.Category, req.Search, parentID, req.Sort, req.Limit, req.Offset, req.EntityType)
	if err != nil {
		return nil, huma.Error500InternalServerError("Failed to list tags")
	}

	items := make([]contracts.TagListItem, len(tags))
	for i, t := range tags {
		items[i] = contracts.TagListItem{
			ID:         t.ID,
			Name:       t.Name,
			Slug:       t.Slug,
			Category:   t.Category,
			IsOfficial: t.IsOfficial,
			UsageCount: t.UsageCount,
			CreatedAt:  t.CreatedAt,
		}
	}

	resp := &ListTagsResponse{}
	resp.Body.Tags = items
	resp.Body.Total = total
	return resp, nil
}

// ============================================================================
// Get Tag (public)
// ============================================================================

type GetTagRequest struct {
	TagID string `path:"tag_id" doc:"Tag ID or slug" example:"post-punk"`
}

type GetTagResponse struct {
	Body *contracts.TagResponse
}

func (h *TagHandler) GetTagHandler(ctx context.Context, req *GetTagRequest) (*GetTagResponse, error) {
	tag := h.resolveTag(req.TagID)
	if tag == nil {
		return nil, huma.Error404NotFound("Tag not found")
	}

	resp := buildTagResponse(tag)
	return &GetTagResponse{Body: resp}, nil
}

// ============================================================================
// Get Tag Detail (public) — enriched response for the tag detail page
// ============================================================================

type GetTagDetailRequest struct {
	TagID string `path:"tag_id" doc:"Tag ID or slug" example:"post-punk"`
}

type GetTagDetailResponse struct {
	Body *contracts.TagDetailResponse
}

func (h *TagHandler) GetTagDetailHandler(ctx context.Context, req *GetTagDetailRequest) (*GetTagDetailResponse, error) {
	tag := h.resolveTag(req.TagID)
	if tag == nil {
		return nil, huma.Error404NotFound("Tag not found")
	}

	detail, err := h.tagService.GetTagDetail(tag.ID)
	if err != nil {
		return nil, huma.Error500InternalServerError("Failed to load tag detail")
	}
	if detail == nil {
		return nil, huma.Error404NotFound("Tag not found")
	}

	return &GetTagDetailResponse{Body: detail}, nil
}

// ============================================================================
// List Tagged Entities (public)
// ============================================================================

type ListTagEntitiesRequest struct {
	TagID      string `path:"tag_id" doc:"Tag ID or slug" example:"post-punk"`
	EntityType string `query:"entity_type" required:"false" doc:"Filter by entity type (artist, release, label, show, venue, festival)"`
	Limit      int    `query:"limit" required:"false" doc:"Max results (default 50)" example:"50"`
	Offset     int    `query:"offset" required:"false" doc:"Offset for pagination" example:"0"`
}

type ListTagEntitiesResponse struct {
	Body struct {
		Entities []contracts.TaggedEntityItem `json:"entities"`
		Total    int64                        `json:"total"`
	}
}

func (h *TagHandler) ListTagEntitiesHandler(ctx context.Context, req *ListTagEntitiesRequest) (*ListTagEntitiesResponse, error) {
	tag := h.resolveTag(req.TagID)
	if tag == nil {
		return nil, huma.Error404NotFound("Tag not found")
	}

	entities, total, err := h.tagService.GetTagEntities(tag.ID, req.EntityType, req.Limit, req.Offset)
	if err != nil {
		return nil, huma.Error500InternalServerError("Failed to list tagged entities")
	}

	resp := &ListTagEntitiesResponse{}
	resp.Body.Entities = entities
	resp.Body.Total = total
	return resp, nil
}

// ============================================================================
// Search Tags (public, autocomplete)
// ============================================================================

type SearchTagsRequest struct {
	Query    string `query:"q" doc:"Search query" example:"post"`
	Limit    int    `query:"limit" required:"false" doc:"Max results (default 10)" example:"10"`
	Category string `query:"category" required:"false" doc:"Filter by category (genre, locale, descriptor, era, mood, instrument, technique, origin, status, other)" example:"genre"`
}

type SearchTagsResponse struct {
	Body struct {
		Tags []contracts.TagListItem `json:"tags"`
	}
}

func (h *TagHandler) SearchTagsHandler(ctx context.Context, req *SearchTagsRequest) (*SearchTagsResponse, error) {
	if req.Query == "" {
		return nil, huma.Error400BadRequest("Query parameter 'q' is required")
	}

	results, err := h.tagService.SearchTags(req.Query, req.Limit, req.Category)
	if err != nil {
		return nil, huma.Error500InternalServerError("Failed to search tags")
	}

	items := make([]contracts.TagListItem, len(results))
	for i, r := range results {
		items[i] = contracts.TagListItem{
			ID:              r.Tag.ID,
			Name:            r.Tag.Name,
			Slug:            r.Tag.Slug,
			Category:        r.Tag.Category,
			IsOfficial:      r.Tag.IsOfficial,
			UsageCount:      r.Tag.UsageCount,
			CreatedAt:       r.Tag.CreatedAt,
			MatchedViaAlias: r.MatchedAlias,
		}
	}

	resp := &SearchTagsResponse{}
	resp.Body.Tags = items
	return resp, nil
}

// ============================================================================
// List Entity Tags (optional auth)
// ============================================================================

type ListEntityTagsRequest struct {
	EntityType string `path:"entity_type" doc:"Entity type (artist, release, label, show, venue, festival)" example:"artist"`
	EntityID   string `path:"entity_id" doc:"Entity ID" example:"1"`
}

type ListEntityTagsResponse struct {
	Body struct {
		Tags []contracts.EntityTagResponse `json:"tags"`
	}
}

func (h *TagHandler) ListEntityTagsHandler(ctx context.Context, req *ListEntityTagsRequest) (*ListEntityTagsResponse, error) {
	entityID, err := strconv.ParseUint(req.EntityID, 10, 32)
	if err != nil {
		return nil, huma.Error400BadRequest("Invalid entity ID")
	}

	var userID uint
	user := middleware.GetUserFromContext(ctx)
	if user != nil {
		userID = user.ID
	}

	tags, err := h.tagService.ListEntityTags(req.EntityType, uint(entityID), userID)
	if err != nil {
		return nil, huma.Error500InternalServerError("Failed to list entity tags")
	}

	resp := &ListEntityTagsResponse{}
	resp.Body.Tags = tags
	return resp, nil
}

// ============================================================================
// Add Tag to Entity (protected)
// ============================================================================

type AddTagToEntityRequest struct {
	EntityType string `path:"entity_type" doc:"Entity type" example:"artist"`
	EntityID   string `path:"entity_id" doc:"Entity ID" example:"1"`
	Body       struct {
		TagID    uint   `json:"tag_id" required:"false" doc:"Tag ID (provide tag_id or tag_name)"`
		TagName  string `json:"tag_name" required:"false" doc:"Tag name (with alias resolution; creates tag if not found)"`
		Category string `json:"category" required:"false" doc:"Tag category for new tags (genre, locale, other; default: other)"`
	}
}

func (h *TagHandler) AddTagToEntityHandler(ctx context.Context, req *AddTagToEntityRequest) (*struct{}, error) {
	user := middleware.GetUserFromContext(ctx)
	if user == nil {
		return nil, huma.Error401Unauthorized("Authentication required")
	}

	entityID, err := strconv.ParseUint(req.EntityID, 10, 32)
	if err != nil {
		return nil, huma.Error400BadRequest("Invalid entity ID")
	}

	if req.Body.TagID == 0 && req.Body.TagName == "" {
		return nil, huma.Error400BadRequest("Either tag_id or tag_name is required")
	}

	_, err = h.tagService.AddTagToEntity(req.Body.TagID, req.Body.TagName, req.EntityType, uint(entityID), user.ID, req.Body.Category)
	if err != nil {
		mapped := mapTagError(err)
		if mapped != nil {
			return nil, mapped
		}
		requestID := logger.GetRequestID(ctx)
		return nil, huma.Error500InternalServerError(
			fmt.Sprintf("Failed to add tag (request_id: %s)", requestID),
		)
	}

	return nil, nil
}

// ============================================================================
// Remove Tag from Entity (protected)
// ============================================================================

type RemoveTagFromEntityRequest struct {
	EntityType string `path:"entity_type" doc:"Entity type" example:"artist"`
	EntityID   string `path:"entity_id" doc:"Entity ID" example:"1"`
	TagID      string `path:"tag_id" doc:"Tag ID" example:"1"`
}

func (h *TagHandler) RemoveTagFromEntityHandler(ctx context.Context, req *RemoveTagFromEntityRequest) (*struct{}, error) {
	user := middleware.GetUserFromContext(ctx)
	if user == nil {
		return nil, huma.Error401Unauthorized("Authentication required")
	}

	entityID, err := strconv.ParseUint(req.EntityID, 10, 32)
	if err != nil {
		return nil, huma.Error400BadRequest("Invalid entity ID")
	}
	tagID, err := strconv.ParseUint(req.TagID, 10, 32)
	if err != nil {
		return nil, huma.Error400BadRequest("Invalid tag ID")
	}

	err = h.tagService.RemoveTagFromEntity(uint(tagID), req.EntityType, uint(entityID))
	if err != nil {
		mapped := mapTagError(err)
		if mapped != nil {
			return nil, mapped
		}
		return nil, huma.Error500InternalServerError("Failed to remove tag")
	}

	return nil, nil
}

// ============================================================================
// Vote on Tag (protected)
// ============================================================================

type VoteTagRequest struct {
	TagID      string `path:"tag_id" doc:"Tag ID" example:"1"`
	EntityType string `path:"entity_type" doc:"Entity type" example:"artist"`
	EntityID   string `path:"entity_id" doc:"Entity ID" example:"1"`
	Body       struct {
		IsUpvote bool `json:"is_upvote" doc:"True for upvote, false for downvote"`
	}
}

func (h *TagHandler) VoteTagHandler(ctx context.Context, req *VoteTagRequest) (*struct{}, error) {
	user := middleware.GetUserFromContext(ctx)
	if user == nil {
		return nil, huma.Error401Unauthorized("Authentication required")
	}

	tagID, err := strconv.ParseUint(req.TagID, 10, 32)
	if err != nil {
		return nil, huma.Error400BadRequest("Invalid tag ID")
	}
	entityID, err := strconv.ParseUint(req.EntityID, 10, 32)
	if err != nil {
		return nil, huma.Error400BadRequest("Invalid entity ID")
	}

	err = h.tagService.VoteOnTag(uint(tagID), req.EntityType, uint(entityID), user.ID, req.Body.IsUpvote)
	if err != nil {
		mapped := mapTagError(err)
		if mapped != nil {
			return nil, mapped
		}
		return nil, huma.Error500InternalServerError("Failed to vote on tag")
	}

	return nil, nil
}

// ============================================================================
// Remove Tag Vote (protected)
// ============================================================================

type RemoveTagVoteRequest struct {
	TagID      string `path:"tag_id" doc:"Tag ID" example:"1"`
	EntityType string `path:"entity_type" doc:"Entity type" example:"artist"`
	EntityID   string `path:"entity_id" doc:"Entity ID" example:"1"`
}

func (h *TagHandler) RemoveTagVoteHandler(ctx context.Context, req *RemoveTagVoteRequest) (*struct{}, error) {
	user := middleware.GetUserFromContext(ctx)
	if user == nil {
		return nil, huma.Error401Unauthorized("Authentication required")
	}

	tagID, err := strconv.ParseUint(req.TagID, 10, 32)
	if err != nil {
		return nil, huma.Error400BadRequest("Invalid tag ID")
	}
	entityID, err := strconv.ParseUint(req.EntityID, 10, 32)
	if err != nil {
		return nil, huma.Error400BadRequest("Invalid entity ID")
	}

	err = h.tagService.RemoveTagVote(uint(tagID), req.EntityType, uint(entityID), user.ID)
	if err != nil {
		return nil, huma.Error500InternalServerError("Failed to remove vote")
	}

	return nil, nil
}

// ============================================================================
// Create Tag (admin)
// ============================================================================

type CreateTagRequest struct {
	Body struct {
		Name        string  `json:"name" doc:"Tag name" example:"post-punk"`
		Description *string `json:"description" required:"false" doc:"Tag description"`
		ParentID    *uint   `json:"parent_id" required:"false" doc:"Parent tag ID for hierarchy"`
		Category    string  `json:"category" doc:"Tag category (genre, locale, other)" example:"genre"`
		IsOfficial  bool    `json:"is_official" required:"false" doc:"Whether this is an official/canonical tag"`
	}
}

type CreateTagResponse struct {
	Body *contracts.TagResponse
}

func (h *TagHandler) CreateTagHandler(ctx context.Context, req *CreateTagRequest) (*CreateTagResponse, error) {
	user := middleware.GetUserFromContext(ctx)
	if user == nil {
		return nil, huma.Error401Unauthorized("Authentication required")
	}
	if !user.IsAdmin {
		return nil, huma.Error403Forbidden("Admin access required")
	}

	if req.Body.Name == "" {
		return nil, huma.Error400BadRequest("Name is required")
	}
	if req.Body.Category == "" {
		return nil, huma.Error400BadRequest("Category is required")
	}

	tag, err := h.tagService.CreateTag(req.Body.Name, req.Body.Description, req.Body.ParentID, req.Body.Category, req.Body.IsOfficial, &user.ID)
	if err != nil {
		mapped := mapTagError(err)
		if mapped != nil {
			return nil, mapped
		}
		requestID := logger.GetRequestID(ctx)
		return nil, huma.Error500InternalServerError(
			fmt.Sprintf("Failed to create tag (request_id: %s)", requestID),
		)
	}

	// Audit log (fire and forget)
	if h.auditLog != nil {
		go func() {
			h.auditLog.LogAction(user.ID, "create_tag", "tag", tag.ID, map[string]interface{}{
				"name":     tag.Name,
				"category": tag.Category,
			})
		}()
	}

	// Re-fetch with preloads
	fullTag, _ := h.tagService.GetTag(tag.ID)
	if fullTag == nil {
		fullTag = tag
	}

	resp := buildTagResponse(fullTag)
	return &CreateTagResponse{Body: resp}, nil
}

// ============================================================================
// Update Tag (admin)
// ============================================================================

type UpdateTagRequest struct {
	TagID string `path:"tag_id" doc:"Tag ID" example:"1"`
	Body  struct {
		Name        *string `json:"name" required:"false" doc:"Tag name"`
		Description *string `json:"description" required:"false" doc:"Tag description"`
		ParentID    *uint   `json:"parent_id" required:"false" doc:"Parent tag ID"`
		Category    *string `json:"category" required:"false" doc:"Tag category"`
		IsOfficial  *bool   `json:"is_official" required:"false" doc:"Whether this is official"`
	}
}

type UpdateTagResponse struct {
	Body *contracts.TagResponse
}

func (h *TagHandler) UpdateTagHandler(ctx context.Context, req *UpdateTagRequest) (*UpdateTagResponse, error) {
	user := middleware.GetUserFromContext(ctx)
	if user == nil {
		return nil, huma.Error401Unauthorized("Authentication required")
	}
	if !user.IsAdmin {
		return nil, huma.Error403Forbidden("Admin access required")
	}

	id, err := strconv.ParseUint(req.TagID, 10, 32)
	if err != nil {
		return nil, huma.Error400BadRequest("Invalid tag ID")
	}

	tag, err := h.tagService.UpdateTag(uint(id), req.Body.Name, req.Body.Description, req.Body.ParentID, req.Body.Category, req.Body.IsOfficial)
	if err != nil {
		mapped := mapTagError(err)
		if mapped != nil {
			return nil, mapped
		}
		return nil, huma.Error500InternalServerError("Failed to update tag")
	}

	// Audit log (fire and forget)
	if h.auditLog != nil {
		go func() {
			h.auditLog.LogAction(user.ID, "update_tag", "tag", uint(id), nil)
		}()
	}

	resp := buildTagResponse(tag)
	return &UpdateTagResponse{Body: resp}, nil
}

// ============================================================================
// Delete Tag (admin)
// ============================================================================

type DeleteTagRequest struct {
	TagID string `path:"tag_id" doc:"Tag ID" example:"1"`
}

func (h *TagHandler) DeleteTagHandler(ctx context.Context, req *DeleteTagRequest) (*struct{}, error) {
	user := middleware.GetUserFromContext(ctx)
	if user == nil {
		return nil, huma.Error401Unauthorized("Authentication required")
	}
	if !user.IsAdmin {
		return nil, huma.Error403Forbidden("Admin access required")
	}

	id, err := strconv.ParseUint(req.TagID, 10, 32)
	if err != nil {
		return nil, huma.Error400BadRequest("Invalid tag ID")
	}

	err = h.tagService.DeleteTag(uint(id))
	if err != nil {
		mapped := mapTagError(err)
		if mapped != nil {
			return nil, mapped
		}
		return nil, huma.Error500InternalServerError("Failed to delete tag")
	}

	// Audit log (fire and forget)
	if h.auditLog != nil {
		go func() {
			h.auditLog.LogAction(user.ID, "delete_tag", "tag", uint(id), nil)
		}()
	}

	return nil, nil
}

// ============================================================================
// List Aliases (public)
// ============================================================================

type ListAliasesRequest struct {
	TagID string `path:"tag_id" doc:"Tag ID or slug" example:"post-punk"`
}

type ListAliasesResponse struct {
	Body struct {
		Aliases []contracts.TagAliasResponse `json:"aliases"`
	}
}

func (h *TagHandler) ListAliasesHandler(ctx context.Context, req *ListAliasesRequest) (*ListAliasesResponse, error) {
	tag := h.resolveTag(req.TagID)
	if tag == nil {
		return nil, huma.Error404NotFound("Tag not found")
	}

	aliases, err := h.tagService.ListAliases(tag.ID)
	if err != nil {
		return nil, huma.Error500InternalServerError("Failed to list aliases")
	}

	items := make([]contracts.TagAliasResponse, len(aliases))
	for i, a := range aliases {
		items[i] = contracts.TagAliasResponse{
			ID:        a.ID,
			Alias:     a.Alias,
			CreatedAt: a.CreatedAt,
		}
	}

	resp := &ListAliasesResponse{}
	resp.Body.Aliases = items
	return resp, nil
}

// ============================================================================
// Create Alias (admin)
// ============================================================================

type CreateAliasRequest struct {
	TagID string `path:"tag_id" doc:"Tag ID" example:"1"`
	Body  struct {
		Alias string `json:"alias" doc:"Alias name" example:"post punk"`
	}
}

type CreateAliasResponse struct {
	Body *contracts.TagAliasResponse
}

func (h *TagHandler) CreateAliasHandler(ctx context.Context, req *CreateAliasRequest) (*CreateAliasResponse, error) {
	user := middleware.GetUserFromContext(ctx)
	if user == nil {
		return nil, huma.Error401Unauthorized("Authentication required")
	}
	if !user.IsAdmin {
		return nil, huma.Error403Forbidden("Admin access required")
	}

	id, err := strconv.ParseUint(req.TagID, 10, 32)
	if err != nil {
		return nil, huma.Error400BadRequest("Invalid tag ID")
	}

	if req.Body.Alias == "" {
		return nil, huma.Error400BadRequest("Alias is required")
	}

	alias, err := h.tagService.CreateAlias(uint(id), req.Body.Alias)
	if err != nil {
		mapped := mapTagError(err)
		if mapped != nil {
			return nil, mapped
		}
		return nil, huma.Error500InternalServerError("Failed to create alias")
	}

	// Audit log (fire and forget)
	if h.auditLog != nil {
		go func() {
			h.auditLog.LogAction(user.ID, "create_tag_alias", "tag", uint(id), map[string]interface{}{
				"alias": req.Body.Alias,
			})
		}()
	}

	resp := &contracts.TagAliasResponse{
		ID:        alias.ID,
		Alias:     alias.Alias,
		CreatedAt: alias.CreatedAt,
	}
	return &CreateAliasResponse{Body: resp}, nil
}

// ============================================================================
// Delete Alias (admin)
// ============================================================================

type DeleteAliasRequest struct {
	TagID   string `path:"tag_id" doc:"Tag ID" example:"1"`
	AliasID string `path:"alias_id" doc:"Alias ID" example:"1"`
}

func (h *TagHandler) DeleteAliasHandler(ctx context.Context, req *DeleteAliasRequest) (*struct{}, error) {
	user := middleware.GetUserFromContext(ctx)
	if user == nil {
		return nil, huma.Error401Unauthorized("Authentication required")
	}
	if !user.IsAdmin {
		return nil, huma.Error403Forbidden("Admin access required")
	}

	aliasID, err := strconv.ParseUint(req.AliasID, 10, 32)
	if err != nil {
		return nil, huma.Error400BadRequest("Invalid alias ID")
	}

	err = h.tagService.DeleteAlias(uint(aliasID))
	if err != nil {
		return nil, huma.Error500InternalServerError("Failed to delete alias")
	}

	// Audit log (fire and forget)
	if h.auditLog != nil {
		tagID, _ := strconv.ParseUint(req.TagID, 10, 32)
		go func() {
			h.auditLog.LogAction(user.ID, "delete_tag_alias", "tag", uint(tagID), map[string]interface{}{
				"alias_id": uint(aliasID),
			})
		}()
	}

	return nil, nil
}

// ============================================================================
// List All Aliases (admin)
// ============================================================================

type ListAllAliasesRequest struct {
	Search string `query:"search" required:"false" doc:"Search by alias text or canonical tag name"`
	Limit  int    `query:"limit" required:"false" doc:"Max results (default 50, max 500)" example:"50"`
	Offset int    `query:"offset" required:"false" doc:"Offset for pagination" example:"0"`
}

type ListAllAliasesResponse struct {
	Body struct {
		Aliases []contracts.TagAliasListing `json:"aliases"`
		Total   int64                       `json:"total"`
	}
}

func (h *TagHandler) ListAllAliasesHandler(ctx context.Context, req *ListAllAliasesRequest) (*ListAllAliasesResponse, error) {
	user := middleware.GetUserFromContext(ctx)
	if user == nil {
		return nil, huma.Error401Unauthorized("Authentication required")
	}
	if !user.IsAdmin {
		return nil, huma.Error403Forbidden("Admin access required")
	}

	items, total, err := h.tagService.ListAllAliases(req.Search, req.Limit, req.Offset)
	if err != nil {
		return nil, huma.Error500InternalServerError("Failed to list aliases")
	}

	resp := &ListAllAliasesResponse{}
	resp.Body.Aliases = items
	resp.Body.Total = total
	return resp, nil
}

// ============================================================================
// Merge Tags Preview (admin)
// ============================================================================

type MergeTagsPreviewRequest struct {
	SourceID string `path:"source_id" doc:"Source tag ID (will be merged away)" example:"1"`
	TargetID uint   `query:"target_id" doc:"Target tag ID (survives the merge)" example:"2"`
}

type MergeTagsPreviewResponse struct {
	Body *contracts.MergeTagsPreview
}

func (h *TagHandler) MergeTagsPreviewHandler(ctx context.Context, req *MergeTagsPreviewRequest) (*MergeTagsPreviewResponse, error) {
	user := middleware.GetUserFromContext(ctx)
	if user == nil {
		return nil, huma.Error401Unauthorized("Authentication required")
	}
	if !user.IsAdmin {
		return nil, huma.Error403Forbidden("Admin access required")
	}

	sourceID, err := strconv.ParseUint(req.SourceID, 10, 32)
	if err != nil {
		return nil, huma.Error400BadRequest("Invalid source tag ID")
	}
	if req.TargetID == 0 {
		return nil, huma.Error400BadRequest("target_id is required")
	}

	preview, err := h.tagService.PreviewMergeTags(uint(sourceID), req.TargetID)
	if err != nil {
		mapped := mapTagError(err)
		if mapped != nil {
			return nil, mapped
		}
		return nil, huma.Error500InternalServerError("Failed to preview merge")
	}

	return &MergeTagsPreviewResponse{Body: preview}, nil
}

// ============================================================================
// Merge Tags (admin)
// ============================================================================

type MergeTagsRequest struct {
	SourceID string `path:"source_id" doc:"Source tag ID (will be merged away)" example:"1"`
	Body     struct {
		TargetID uint `json:"target_id" doc:"Target tag ID (survives the merge)"`
	}
}

type MergeTagsResponse struct {
	Body *contracts.MergeTagsResult
}

func (h *TagHandler) MergeTagsHandler(ctx context.Context, req *MergeTagsRequest) (*MergeTagsResponse, error) {
	user := middleware.GetUserFromContext(ctx)
	if user == nil {
		return nil, huma.Error401Unauthorized("Authentication required")
	}
	if !user.IsAdmin {
		return nil, huma.Error403Forbidden("Admin access required")
	}

	sourceID, err := strconv.ParseUint(req.SourceID, 10, 32)
	if err != nil {
		return nil, huma.Error400BadRequest("Invalid source tag ID")
	}
	if req.Body.TargetID == 0 {
		return nil, huma.Error400BadRequest("target_id is required")
	}

	result, err := h.tagService.MergeTags(uint(sourceID), req.Body.TargetID, user.ID)
	if err != nil {
		mapped := mapTagError(err)
		if mapped != nil {
			return nil, mapped
		}
		requestID := logger.GetRequestID(ctx)
		return nil, huma.Error500InternalServerError(
			fmt.Sprintf("Failed to merge tags (request_id: %s)", requestID),
		)
	}

	return &MergeTagsResponse{Body: result}, nil
}

// ============================================================================
// Bulk Import Aliases (admin)
// ============================================================================

// Cap the bulk import payload to keep admin mistakes (e.g. pasting a huge
// CSV) from becoming a DoS. 2000 rows comfortably covers realistic seeding
// batches while bounding work per request.
const maxBulkAliasImportRows = 2000

type BulkImportAliasesRequest struct {
	Body struct {
		Items []contracts.BulkAliasImportItem `json:"items" doc:"List of alias,canonical pairs to import"`
	}
}

type BulkImportAliasesResponse struct {
	Body *contracts.BulkAliasImportResult
}

func (h *TagHandler) BulkImportAliasesHandler(ctx context.Context, req *BulkImportAliasesRequest) (*BulkImportAliasesResponse, error) {
	user := middleware.GetUserFromContext(ctx)
	if user == nil {
		return nil, huma.Error401Unauthorized("Authentication required")
	}
	if !user.IsAdmin {
		return nil, huma.Error403Forbidden("Admin access required")
	}

	if len(req.Body.Items) == 0 {
		return nil, huma.Error400BadRequest("items is required and must not be empty")
	}
	if len(req.Body.Items) > maxBulkAliasImportRows {
		return nil, huma.Error400BadRequest(
			fmt.Sprintf("items exceeds max of %d rows", maxBulkAliasImportRows),
		)
	}

	result, err := h.tagService.BulkImportAliases(req.Body.Items)
	if err != nil {
		return nil, huma.Error500InternalServerError("Failed to bulk import aliases")
	}

	if h.auditLog != nil {
		imported := result.Imported
		skipped := len(result.Skipped)
		go func() {
			h.auditLog.LogAction(user.ID, "bulk_import_tag_aliases", "tag", 0, map[string]interface{}{
				"imported": imported,
				"skipped":  skipped,
			})
		}()
	}

	return &BulkImportAliasesResponse{Body: result}, nil
}

// ============================================================================
// Low-Quality Tag Queue (admin, PSY-310)
// ============================================================================

type ListLowQualityTagsRequest struct {
	Limit  int `query:"limit" required:"false" doc:"Max results (default 20, max 100)" example:"20"`
	Offset int `query:"offset" required:"false" doc:"Offset for pagination" example:"0"`
}

type ListLowQualityTagsResponse struct {
	Body *contracts.LowQualityTagQueueResponse
}

func (h *TagHandler) ListLowQualityTagsHandler(ctx context.Context, req *ListLowQualityTagsRequest) (*ListLowQualityTagsResponse, error) {
	user := middleware.GetUserFromContext(ctx)
	if user == nil {
		return nil, huma.Error401Unauthorized("Authentication required")
	}
	if !user.IsAdmin {
		return nil, huma.Error403Forbidden("Admin access required")
	}

	queue, err := h.tagService.GetLowQualityTagQueue(req.Limit, req.Offset)
	if err != nil {
		return nil, huma.Error500InternalServerError("Failed to load low-quality tag queue")
	}

	return &ListLowQualityTagsResponse{Body: queue}, nil
}

type SnoozeTagRequest struct {
	TagID string `path:"tag_id" doc:"Tag ID" example:"1"`
}

func (h *TagHandler) SnoozeTagHandler(ctx context.Context, req *SnoozeTagRequest) (*struct{}, error) {
	user := middleware.GetUserFromContext(ctx)
	if user == nil {
		return nil, huma.Error401Unauthorized("Authentication required")
	}
	if !user.IsAdmin {
		return nil, huma.Error403Forbidden("Admin access required")
	}

	id, err := strconv.ParseUint(req.TagID, 10, 32)
	if err != nil {
		return nil, huma.Error400BadRequest("Invalid tag ID")
	}

	if err := h.tagService.SnoozeLowQualityTag(uint(id), user.ID); err != nil {
		mapped := mapTagError(err)
		if mapped != nil {
			return nil, mapped
		}
		return nil, huma.Error500InternalServerError("Failed to snooze tag")
	}

	if h.auditLog != nil {
		go func() {
			h.auditLog.LogAction(user.ID, "snooze_low_quality_tag", "tag", uint(id), nil)
		}()
	}

	return nil, nil
}

// ============================================================================
// Genre hierarchy (admin, PSY-311)
// ============================================================================

// GenreHierarchyTag is the minimal shape returned by the hierarchy endpoint.
// The frontend builds the tree client-side from parent_id, so we don't need
// the heavier TagResponse shape with relationships and creator attribution.
type GenreHierarchyTag struct {
	ID         uint   `json:"id"`
	Name       string `json:"name"`
	Slug       string `json:"slug"`
	ParentID   *uint  `json:"parent_id,omitempty"`
	UsageCount int    `json:"usage_count"`
	IsOfficial bool   `json:"is_official"`
}

type GetGenreHierarchyResponse struct {
	Body struct {
		Tags []GenreHierarchyTag `json:"tags"`
	}
}

// GetGenreHierarchyHandler returns all genre tags as a flat list with
// parent_id populated. The frontend assembles the tree — this keeps the
// backend query trivial (one indexed scan) and avoids a recursive CTE.
func (h *TagHandler) GetGenreHierarchyHandler(ctx context.Context, _ *struct{}) (*GetGenreHierarchyResponse, error) {
	user := middleware.GetUserFromContext(ctx)
	if user == nil {
		return nil, huma.Error401Unauthorized("Authentication required")
	}
	if !user.IsAdmin {
		return nil, huma.Error403Forbidden("Admin access required")
	}

	tags, err := h.tagService.GetGenreHierarchy()
	if err != nil {
		return nil, huma.Error500InternalServerError("Failed to load genre hierarchy")
	}

	items := make([]GenreHierarchyTag, len(tags))
	for i, t := range tags {
		items[i] = GenreHierarchyTag{
			ID:         t.ID,
			Name:       t.Name,
			Slug:       t.Slug,
			ParentID:   t.ParentID,
			UsageCount: t.UsageCount,
			IsOfficial: t.IsOfficial,
		}
	}

	resp := &GetGenreHierarchyResponse{}
	resp.Body.Tags = items
	return resp, nil
}

// SetTagParentRequest has ParentID as a pointer so the request can
// explicitly send `null` to clear the parent. Huma treats pointer body
// fields as required by default; we mark it optional so callers can omit
// it and default to "clear parent" — but in practice the frontend always
// sends the field explicitly.
type SetTagParentRequest struct {
	TagID string `path:"tag_id" doc:"Tag ID" example:"1"`
	Body  struct {
		ParentID *uint `json:"parent_id" required:"false" doc:"New parent tag ID, or null to clear parent"`
	}
}

// SetTagParentHandler sets or clears the parent of a genre tag. Cycle
// detection, category enforcement, and audit logging live in the service.
// The handler's job is path-id parsing + error mapping.
func (h *TagHandler) SetTagParentHandler(ctx context.Context, req *SetTagParentRequest) (*struct{}, error) {
	user := middleware.GetUserFromContext(ctx)
	if user == nil {
		return nil, huma.Error401Unauthorized("Authentication required")
	}
	if !user.IsAdmin {
		return nil, huma.Error403Forbidden("Admin access required")
	}

	id, err := strconv.ParseUint(req.TagID, 10, 32)
	if err != nil {
		return nil, huma.Error400BadRequest("Invalid tag ID")
	}

	if err := h.tagService.SetTagParent(uint(id), req.Body.ParentID, user.ID); err != nil {
		if mapped := mapTagError(err); mapped != nil {
			return nil, mapped
		}
		return nil, huma.Error500InternalServerError("Failed to set tag parent")
	}

	return nil, nil
}

// ============================================================================
// Helpers
// ============================================================================

// resolveTag resolves a tag by numeric ID or slug.
func (h *TagHandler) resolveTag(idOrSlug string) *models.Tag {
	// Try numeric ID first
	if id, err := strconv.ParseUint(idOrSlug, 10, 32); err == nil {
		tag, err := h.tagService.GetTag(uint(id))
		if err == nil && tag != nil {
			return tag
		}
	}
	// Fall back to slug
	tag, err := h.tagService.GetTagBySlug(idOrSlug)
	if err == nil && tag != nil {
		return tag
	}
	return nil
}

// buildTagResponse converts a models.Tag to a TagResponse.
func buildTagResponse(tag *models.Tag) *contracts.TagResponse {
	resp := &contracts.TagResponse{
		ID:              tag.ID,
		Name:            tag.Name,
		Slug:            tag.Slug,
		Description:     tag.Description,
		ParentID:        tag.ParentID,
		Category:        tag.Category,
		IsOfficial:      tag.IsOfficial,
		UsageCount:      tag.UsageCount,
		ChildCount:      len(tag.Children),
		CreatedByUserID: tag.CreatedByUserID,
		CreatedAt:       tag.CreatedAt,
		UpdatedAt:       tag.UpdatedAt,
	}

	if tag.Parent != nil {
		resp.ParentName = tag.Parent.Name
	}

	if tag.CreatedBy != nil && tag.CreatedBy.Username != nil && *tag.CreatedBy.Username != "" {
		resp.CreatedByUsername = tag.CreatedBy.Username
	}

	if len(tag.Aliases) > 0 {
		resp.Aliases = make([]string, len(tag.Aliases))
		for i, a := range tag.Aliases {
			resp.Aliases[i] = a.Alias
		}
	}

	return resp
}

// mapTagError converts a TagError to an appropriate Huma HTTP error.
func mapTagError(err error) error {
	var tagErr *apperrors.TagError
	if errors.As(err, &tagErr) {
		switch tagErr.Code {
		case apperrors.CodeTagNotFound:
			return huma.Error404NotFound(tagErr.Message)
		case apperrors.CodeTagExists, apperrors.CodeTagAliasExists, apperrors.CodeEntityTagExists:
			return huma.Error409Conflict(tagErr.Message)
		case apperrors.CodeEntityTagNotFound:
			return huma.Error404NotFound(tagErr.Message)
		case apperrors.CodeTagCreationForbidden:
			return huma.Error403Forbidden(tagErr.Message)
		case apperrors.CodeTagNameInvalid:
			return huma.Error400BadRequest(tagErr.Message)
		case apperrors.CodeTagMergeInvalid:
			return huma.Error400BadRequest(tagErr.Message)
		case apperrors.CodeTagMergeAliasConflict:
			return huma.Error409Conflict(tagErr.Message)
		case apperrors.CodeTagHierarchyCycle:
			return huma.Error400BadRequest(tagErr.Message)
		case apperrors.CodeTagHierarchyNotGenre:
			return huma.Error400BadRequest(tagErr.Message)
		}
	}
	return nil
}
