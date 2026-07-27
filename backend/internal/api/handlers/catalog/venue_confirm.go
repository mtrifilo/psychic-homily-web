package catalog

import (
	"context"
	"fmt"
	"strconv"

	"github.com/danielgtaylor/huma/v2"

	"psychic-homily-backend/internal/api/handlers/shared"
	"psychic-homily-backend/internal/api/middleware"
	"psychic-homily-backend/internal/logger"
	"psychic-homily-backend/internal/services/contracts"
)

// Venue confirm-current endpoint (PSY-1542).
//
// One tap that says "this listing is still accurate" and edits nothing. It is
// the cheapest contribution the app offers, so it is open to any authenticated
// user at any trust tier — the whole point is to be an on-ramp. Throttling is
// handled upstream by the shared engagement-mutation limiter (see
// routes/engagement_mutation_rate_limit.go), which answers 429 with a
// Retry-After header; nothing here needs its own budget.

// VenueConfirmHandler owns POST /venues/{venue_id}/confirm.
//
// Its own handler with its own one-method service, not a method on
// VenueHandler: that struct carries admin venue CRUD, the bill network and the
// genre profile, and this is the house pattern for one-tap engagement toggles
// (CollectionLikeHandler, FollowHandler, SavedShowHandler are all separate from
// their domain's main handler for the same reason).
type VenueConfirmHandler struct {
	confirmService contracts.VenueConfirmServiceInterface
}

// NewVenueConfirmHandler creates a new venue confirm handler.
func NewVenueConfirmHandler(confirmService contracts.VenueConfirmServiceInterface) *VenueConfirmHandler {
	return &VenueConfirmHandler{confirmService: confirmService}
}

// ConfirmVenueRequest is the request shape for POST /venues/{venue_id}/confirm.
//
// Unlike the read endpoints, this one accepts a NUMERIC id only. A confirmation
// is a write against a specific row, and slug lookup would give the same venue
// two addressable identities on a rate-limited path.
type ConfirmVenueRequest struct {
	VenueID string `path:"venue_id" doc:"Numeric venue ID" example:"42"`
}

// ConfirmVenueResponse carries the post-mutation aggregate so the client can
// re-render the provenance stamp without a follow-up read.
type ConfirmVenueResponse struct {
	Body contracts.VenueConfirmationResponse
}

// ConfirmVenueHandler handles POST /venues/{venue_id}/confirm.
//
// Idempotent: confirming a venue you already confirmed returns 200 with the
// unchanged aggregate, not a conflict. The client treats both the same.
func (h *VenueConfirmHandler) ConfirmVenueHandler(ctx context.Context, req *ConfirmVenueRequest) (*ConfirmVenueResponse, error) {
	requestID := logger.GetRequestID(ctx)

	user := middleware.GetUserFromContext(ctx)
	if user == nil {
		return nil, huma.Error401Unauthorized("Authentication required")
	}

	venueID, err := strconv.ParseUint(req.VenueID, 10, 32)
	if err != nil {
		return nil, huma.Error400BadRequest("Invalid venue ID")
	}

	resp, err := h.confirmService.ConfirmVenue(uint(venueID), user.ID)
	if err != nil {
		if mapped := shared.MapVenueError(err); mapped != nil {
			return nil, mapped
		}
		logger.FromContext(ctx).Error("confirm_venue_failed",
			"venue_id", venueID,
			"user_id", user.ID,
			"error", err.Error(),
			"request_id", requestID,
		)
		return nil, huma.Error500InternalServerError(
			fmt.Sprintf("Failed to confirm venue (request_id: %s)", requestID),
		)
	}

	return &ConfirmVenueResponse{Body: *resp}, nil
}
