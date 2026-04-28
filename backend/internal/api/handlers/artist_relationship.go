package handlers

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/danielgtaylor/huma/v2"

	"psychic-homily-backend/internal/api/middleware"
	"psychic-homily-backend/internal/logger"
	"psychic-homily-backend/internal/services/contracts"
)

// ArtistRelationshipHandler handles artist relationship API requests.
type ArtistRelationshipHandler struct {
	relService contracts.ArtistRelationshipServiceInterface
	auditLog   contracts.AuditLogServiceInterface
}

// NewArtistRelationshipHandler creates a new handler.
func NewArtistRelationshipHandler(
	relService contracts.ArtistRelationshipServiceInterface,
	auditLog contracts.AuditLogServiceInterface,
) *ArtistRelationshipHandler {
	return &ArtistRelationshipHandler{
		relService: relService,
		auditLog:   auditLog,
	}
}

// ============================================================================
// Get Artist Graph (optional auth)
// ============================================================================

type GetArtistGraphRequest struct {
	ArtistID string `path:"artist_id" doc:"Artist ID" example:"1"`
	Types    string `query:"types" required:"false" doc:"Comma-separated relationship types to include (similar,shared_bills,shared_label,side_project,member_of,radio_cooccurrence,festival_cobill)"`
}

type GetArtistGraphResponse struct {
	Body contracts.ArtistGraph
}

func (h *ArtistRelationshipHandler) GetArtistGraphHandler(ctx context.Context, req *GetArtistGraphRequest) (*GetArtistGraphResponse, error) {
	id, err := strconv.ParseUint(req.ArtistID, 10, 32)
	if err != nil {
		return nil, huma.Error400BadRequest("Invalid artist ID")
	}

	// Parse types filter
	var types []string
	if req.Types != "" {
		for _, t := range splitAndTrim(req.Types) {
			if t != "" {
				types = append(types, t)
			}
		}
	}

	// Get user ID for vote context
	var userID uint
	user := middleware.GetUserFromContext(ctx)
	if user != nil {
		userID = user.ID
	}

	graph, err := h.relService.GetArtistGraph(uint(id), types, userID)
	if err != nil {
		if err.Error() == "artist not found" || (len(err.Error()) > 16 && err.Error()[:16] == "artist not found") {
			return nil, huma.Error404NotFound("Artist not found")
		}
		return nil, huma.Error500InternalServerError("Failed to get artist graph")
	}

	resp := &GetArtistGraphResponse{}
	resp.Body = *graph
	return resp, nil
}

// ============================================================================
// Get Artist Bill Composition (PSY-364, public)
// ============================================================================

type GetArtistBillCompositionRequest struct {
	ArtistID string `path:"artist_id" doc:"Artist ID" example:"1"`
	Months   int    `query:"months" required:"false" doc:"Time window in months (0 = all-time, max 24)"`
}

type GetArtistBillCompositionResponse struct {
	Body contracts.ArtistBillComposition
}

func (h *ArtistRelationshipHandler) GetArtistBillCompositionHandler(ctx context.Context, req *GetArtistBillCompositionRequest) (*GetArtistBillCompositionResponse, error) {
	id, err := strconv.ParseUint(req.ArtistID, 10, 32)
	if err != nil {
		return nil, huma.Error400BadRequest("Invalid artist ID")
	}

	if req.Months < 0 {
		return nil, huma.Error400BadRequest("months must be >= 0")
	}

	bc, err := h.relService.GetArtistBillComposition(uint(id), req.Months)
	if err != nil {
		if strings.HasPrefix(err.Error(), "artist not found") {
			return nil, huma.Error404NotFound("Artist not found")
		}
		return nil, huma.Error500InternalServerError("Failed to get bill composition")
	}

	return &GetArtistBillCompositionResponse{Body: *bc}, nil
}

// splitAndTrim splits a comma-separated string and trims whitespace from each element.
func splitAndTrim(s string) []string {
	parts := make([]string, 0)
	for _, p := range strings.Split(s, ",") {
		trimmed := strings.TrimSpace(p)
		if trimmed != "" {
			parts = append(parts, trimmed)
		}
	}
	return parts
}

// ============================================================================
// Get Related Artists (optional auth)
// ============================================================================

type GetRelatedArtistsRequest struct {
	ArtistID string `path:"artist_id" doc:"Artist ID" example:"1"`
	Type     string `query:"type" required:"false" doc:"Filter by relationship type (similar, shared_bills, shared_label, side_project, member_of)"`
	Limit    int    `query:"limit" required:"false" doc:"Max results (default 30)" example:"30"`
}

type GetRelatedArtistsResponse struct {
	Body struct {
		Related []contracts.RelatedArtistResponse `json:"related"`
	}
}

func (h *ArtistRelationshipHandler) GetRelatedArtistsHandler(ctx context.Context, req *GetRelatedArtistsRequest) (*GetRelatedArtistsResponse, error) {
	id, err := strconv.ParseUint(req.ArtistID, 10, 32)
	if err != nil {
		return nil, huma.Error400BadRequest("Invalid artist ID")
	}

	related, err := h.relService.GetRelatedArtists(uint(id), req.Type, req.Limit)
	if err != nil {
		return nil, huma.Error500InternalServerError("Failed to get related artists")
	}

	// Include user's votes if authenticated
	user := middleware.GetUserFromContext(ctx)
	if user != nil {
		for i := range related {
			r := &related[i]
			vote, err := h.relService.GetUserVote(uint(id), r.ArtistID, r.RelationshipType, user.ID)
			if err == nil && vote != nil {
				dir := int(vote.Direction)
				r.UserVote = &dir
			}
		}
	}

	resp := &GetRelatedArtistsResponse{}
	resp.Body.Related = related
	return resp, nil
}

// ============================================================================
// Create Relationship (protected)
// ============================================================================

type CreateRelationshipRequest struct {
	Body struct {
		SourceArtistID uint   `json:"source_artist_id" doc:"Source artist ID" example:"1"`
		TargetArtistID uint   `json:"target_artist_id" doc:"Target artist ID" example:"2"`
		Type           string `json:"type" doc:"Relationship type (similar, side_project, member_of)" example:"similar"`
	}
}

type CreateRelationshipResponse struct {
	Body struct {
		Success bool `json:"success"`
	}
}

func (h *ArtistRelationshipHandler) CreateRelationshipHandler(ctx context.Context, req *CreateRelationshipRequest) (*CreateRelationshipResponse, error) {
	user := middleware.GetUserFromContext(ctx)
	if user == nil {
		return nil, huma.Error401Unauthorized("Authentication required")
	}

	if req.Body.SourceArtistID == 0 || req.Body.TargetArtistID == 0 {
		return nil, huma.Error400BadRequest("Both source_artist_id and target_artist_id are required")
	}
	if req.Body.Type == "" {
		return nil, huma.Error400BadRequest("Relationship type is required")
	}

	_, err := h.relService.CreateRelationship(req.Body.SourceArtistID, req.Body.TargetArtistID, req.Body.Type, false)
	if err != nil {
		requestID := logger.GetRequestID(ctx)
		return nil, huma.Error500InternalServerError(
			fmt.Sprintf("Failed to create relationship (request_id: %s): %v", requestID, err),
		)
	}

	// Cast initial upvote from the creator
	_ = h.relService.Vote(req.Body.SourceArtistID, req.Body.TargetArtistID, req.Body.Type, user.ID, true)

	// Audit log (fire and forget)
	if h.auditLog != nil {
		go func() {
			h.auditLog.LogAction(user.ID, "create_artist_relationship", "artist", req.Body.SourceArtistID, map[string]interface{}{
				"target_artist_id": req.Body.TargetArtistID,
				"type":             req.Body.Type,
			})
		}()
	}

	resp := &CreateRelationshipResponse{}
	resp.Body.Success = true
	return resp, nil
}

// ============================================================================
// Vote on Relationship (protected)
// ============================================================================

type VoteRelationshipRequest struct {
	SourceID string `path:"source_id" doc:"Source artist ID" example:"1"`
	TargetID string `path:"target_id" doc:"Target artist ID" example:"2"`
	Body     struct {
		Type     string `json:"type" doc:"Relationship type" example:"similar"`
		IsUpvote bool   `json:"is_upvote" doc:"True for upvote, false for downvote"`
	}
}

func (h *ArtistRelationshipHandler) VoteHandler(ctx context.Context, req *VoteRelationshipRequest) (*struct{}, error) {
	user := middleware.GetUserFromContext(ctx)
	if user == nil {
		return nil, huma.Error401Unauthorized("Authentication required")
	}

	sourceID, err := strconv.ParseUint(req.SourceID, 10, 32)
	if err != nil {
		return nil, huma.Error400BadRequest("Invalid source artist ID")
	}
	targetID, err := strconv.ParseUint(req.TargetID, 10, 32)
	if err != nil {
		return nil, huma.Error400BadRequest("Invalid target artist ID")
	}

	if req.Body.Type == "" {
		return nil, huma.Error400BadRequest("Relationship type is required")
	}

	err = h.relService.Vote(uint(sourceID), uint(targetID), req.Body.Type, user.ID, req.Body.IsUpvote)
	if err != nil {
		return nil, huma.Error500InternalServerError(fmt.Sprintf("Failed to vote: %v", err))
	}

	return nil, nil
}

// ============================================================================
// Remove Vote (protected)
// ============================================================================

type RemoveRelationshipVoteRequest struct {
	SourceID string `path:"source_id" doc:"Source artist ID" example:"1"`
	TargetID string `path:"target_id" doc:"Target artist ID" example:"2"`
	Type     string `query:"type" doc:"Relationship type" example:"similar"`
}

func (h *ArtistRelationshipHandler) RemoveVoteHandler(ctx context.Context, req *RemoveRelationshipVoteRequest) (*struct{}, error) {
	user := middleware.GetUserFromContext(ctx)
	if user == nil {
		return nil, huma.Error401Unauthorized("Authentication required")
	}

	sourceID, err := strconv.ParseUint(req.SourceID, 10, 32)
	if err != nil {
		return nil, huma.Error400BadRequest("Invalid source artist ID")
	}
	targetID, err := strconv.ParseUint(req.TargetID, 10, 32)
	if err != nil {
		return nil, huma.Error400BadRequest("Invalid target artist ID")
	}

	if req.Type == "" {
		return nil, huma.Error400BadRequest("Relationship type is required")
	}

	err = h.relService.RemoveVote(uint(sourceID), uint(targetID), req.Type, user.ID)
	if err != nil {
		return nil, huma.Error500InternalServerError("Failed to remove vote")
	}

	return nil, nil
}

// ============================================================================
// Delete Relationship (admin)
// ============================================================================

type DeleteRelationshipRequest struct {
	SourceID string `path:"source_id" doc:"Source artist ID" example:"1"`
	TargetID string `path:"target_id" doc:"Target artist ID" example:"2"`
	Type     string `query:"type" doc:"Relationship type" example:"similar"`
}

func (h *ArtistRelationshipHandler) DeleteRelationshipHandler(ctx context.Context, req *DeleteRelationshipRequest) (*struct{}, error) {
	user := middleware.GetUserFromContext(ctx)
	if user == nil {
		return nil, huma.Error401Unauthorized("Authentication required")
	}
	if !user.IsAdmin {
		return nil, huma.Error403Forbidden("Admin access required")
	}

	sourceID, _ := strconv.ParseUint(req.SourceID, 10, 32)
	targetID, _ := strconv.ParseUint(req.TargetID, 10, 32)

	err := h.relService.DeleteRelationship(uint(sourceID), uint(targetID), req.Type)
	if err != nil {
		return nil, huma.Error500InternalServerError("Failed to delete relationship")
	}

	if h.auditLog != nil {
		go func() {
			h.auditLog.LogAction(user.ID, "delete_artist_relationship", "artist", uint(sourceID), map[string]interface{}{
				"target_artist_id": uint(targetID),
				"type":             req.Type,
			})
		}()
	}

	return nil, nil
}

// ============================================================================
// Derive Relationships (admin)
// ============================================================================

type DeriveRelationshipsRequest struct{}

type DeriveRelationshipsResponse struct {
	Body struct {
		Success            bool  `json:"success"`
		SharedBillsUpserted int64 `json:"shared_bills_upserted"`
		SharedLabelsUpserted int64 `json:"shared_labels_upserted"`
	}
}

func (h *ArtistRelationshipHandler) DeriveRelationshipsHandler(ctx context.Context, req *DeriveRelationshipsRequest) (*DeriveRelationshipsResponse, error) {
	user := middleware.GetUserFromContext(ctx)
	if user == nil {
		return nil, huma.Error401Unauthorized("Authentication required")
	}
	if !user.IsAdmin {
		return nil, huma.Error403Forbidden("Admin access required")
	}

	billsCount, err := h.relService.DeriveSharedBills(2)
	if err != nil {
		requestID := logger.GetRequestID(ctx)
		return nil, huma.Error500InternalServerError(
			fmt.Sprintf("Failed to derive shared bills (request_id: %s): %v", requestID, err),
		)
	}

	labelsCount, err := h.relService.DeriveSharedLabels(1)
	if err != nil {
		requestID := logger.GetRequestID(ctx)
		return nil, huma.Error500InternalServerError(
			fmt.Sprintf("Failed to derive shared labels (request_id: %s): %v", requestID, err),
		)
	}

	// Audit log (fire and forget)
	if h.auditLog != nil {
		go func() {
			h.auditLog.LogAction(user.ID, "derive_artist_relationships", "system", 0, map[string]interface{}{
				"shared_bills_upserted":  billsCount,
				"shared_labels_upserted": labelsCount,
			})
		}()
	}

	resp := &DeriveRelationshipsResponse{}
	resp.Body.Success = true
	resp.Body.SharedBillsUpserted = billsCount
	resp.Body.SharedLabelsUpserted = labelsCount
	return resp, nil
}
