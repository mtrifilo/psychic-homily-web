// Package explore exposes the public read endpoints that back the
// /explore landing page. The three endpoints — upcoming-shows,
// featured, and shuffle-target — register on the public API group;
// anonymous and authenticated callers see identical content (locked
// product decision).
//
// Handler logic stays thin: validate the query envelope, delegate to
// the service, surface 500s with a request-id breadcrumb on failure.
// All the read shape + privacy logic lives in services/explore.
package explore

import (
	"context"
	"fmt"

	"github.com/danielgtaylor/huma/v2"

	"psychic-homily-backend/internal/logger"
	"psychic-homily-backend/internal/services/contracts"
)

// ExploreHandler holds the service dependencies for the /explore
// public endpoints. Constructed once at route-setup time.
type ExploreHandler struct {
	exploreService contracts.ExploreServiceInterface
}

// NewExploreHandler wires the explore service. The featured-slot
// service is reached transitively through the explore service — the
// handler does not call it directly.
func NewExploreHandler(exploreService contracts.ExploreServiceInterface) *ExploreHandler {
	return &ExploreHandler{exploreService: exploreService}
}

// ──────────────────────────────────────────────
// GET /explore/upcoming-shows
// ──────────────────────────────────────────────

// GetUpcomingShowsRequest is the query envelope for the upcoming-shows
// endpoint. Limit defaults to 20, capped at 50; offset is non-negative.
// City filter is intentionally out of scope (PSY-840 follow-up) — adding
// it later is additive and doesn't change the existing wire shape.
type GetUpcomingShowsRequest struct {
	Limit  int `query:"limit" minimum:"1" maximum:"50" default:"20" doc:"Number of shows per page (1-50)"`
	Offset int `query:"offset" minimum:"0" default:"0" doc:"Offset for pagination"`
}

// GetUpcomingShowsResponse wraps the explore-shaped page response.
type GetUpcomingShowsResponse struct {
	Body contracts.ExploreUpcomingShowsResponse
}

// GetUpcomingShowsHandler handles GET /explore/upcoming-shows. Returns
// approved shows with event_date >= NOW(), ordered chronologically by
// (event_date ASC, id ASC). Deterministic across pages — two shows
// sharing the same event_date sort by id, never randomly reshuffle.
func (h *ExploreHandler) GetUpcomingShowsHandler(ctx context.Context, req *GetUpcomingShowsRequest) (*GetUpcomingShowsResponse, error) {
	requestID := logger.GetRequestID(ctx)

	result, err := h.exploreService.GetUpcomingShows(req.Limit, req.Offset)
	if err != nil {
		logger.FromContext(ctx).Error("explore_upcoming_shows_failed",
			"error", err.Error(),
			"request_id", requestID,
		)
		return nil, huma.Error500InternalServerError(
			fmt.Sprintf("Failed to load upcoming shows (request_id: %s)", requestID),
		)
	}

	return &GetUpcomingShowsResponse{Body: *result}, nil
}

// ──────────────────────────────────────────────
// GET /explore/featured
// ──────────────────────────────────────────────

// GetFeaturedRequest has no inputs — the endpoint is a pure read.
// Huma requires the request struct to exist; an empty struct works.
type GetFeaturedRequest struct{}

// GetFeaturedResponse wraps the explore-shaped featured response.
type GetFeaturedResponse struct {
	Body contracts.ExploreFeaturedResponse
}

// GetFeaturedHandler handles GET /explore/featured. Returns the
// currently-active bill + collection from the admin-curated
// featured_slots table (PSY-835). Both fields are nullable: the
// frontend collapses the section when both are nil. A stale referent
// (deleted / privacy-flipped / status-flipped) resolves to nil for
// that field — never a 500.
func (h *ExploreHandler) GetFeaturedHandler(ctx context.Context, _ *GetFeaturedRequest) (*GetFeaturedResponse, error) {
	requestID := logger.GetRequestID(ctx)

	result, err := h.exploreService.GetFeatured()
	if err != nil {
		logger.FromContext(ctx).Error("explore_featured_failed",
			"error", err.Error(),
			"request_id", requestID,
		)
		return nil, huma.Error500InternalServerError(
			fmt.Sprintf("Failed to load featured picks (request_id: %s)", requestID),
		)
	}

	return &GetFeaturedResponse{Body: *result}, nil
}

// ──────────────────────────────────────────────
// GET /explore/shuffle-target
// ──────────────────────────────────────────────

// GetShuffleTargetRequest has no inputs — random pick.
type GetShuffleTargetRequest struct{}

// GetShuffleTargetResponse wraps the explore-shaped shuffle response.
type GetShuffleTargetResponse struct {
	Body contracts.ExploreShuffleTargetResponse
}

// GetShuffleTargetHandler handles GET /explore/shuffle-target. Returns
// one random artist drawn from the pool of artists with a show in the
// ±90-day window. When the pool is empty, returns all-nil fields with
// HTTP 200 — the frontend renders a graceful empty state.
func (h *ExploreHandler) GetShuffleTargetHandler(ctx context.Context, _ *GetShuffleTargetRequest) (*GetShuffleTargetResponse, error) {
	requestID := logger.GetRequestID(ctx)

	result, err := h.exploreService.GetShuffleTarget()
	if err != nil {
		logger.FromContext(ctx).Error("explore_shuffle_target_failed",
			"error", err.Error(),
			"request_id", requestID,
		)
		return nil, huma.Error500InternalServerError(
			fmt.Sprintf("Failed to pick shuffle target (request_id: %s)", requestID),
		)
	}

	return &GetShuffleTargetResponse{Body: *result}, nil
}
