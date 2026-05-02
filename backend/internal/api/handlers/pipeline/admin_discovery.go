package pipeline

import (
	"context"
	"fmt"

	"github.com/danielgtaylor/huma/v2"

	"psychic-homily-backend/internal/api/handlers/shared"
	"psychic-homily-backend/internal/logger"
	"psychic-homily-backend/internal/models"
	"psychic-homily-backend/internal/services/contracts"
)

// AdminDiscoveryHandler handles admin discovery import/check endpoints
type AdminDiscoveryHandler struct {
	discoveryService contracts.DiscoveryServiceInterface
}

// NewAdminDiscoveryHandler creates a new admin discovery handler
func NewAdminDiscoveryHandler(
	discoveryService contracts.DiscoveryServiceInterface,
) *AdminDiscoveryHandler {
	return &AdminDiscoveryHandler{
		discoveryService: discoveryService,
	}
}

// DiscoveryImportEventInput represents a single discovered event for import
type DiscoveryImportEventInput struct {
	ID             string   `json:"id" doc:"External event ID from the venue's system"`
	Title          string   `json:"title" doc:"Event title"`
	Date           string   `json:"date" doc:"Event date in ISO format (YYYY-MM-DD)"`
	Venue          string   `json:"venue" doc:"Venue name"`
	VenueSlug      string   `json:"venueSlug" doc:"Venue identifier (e.g., valley-bar)"`
	ImageURL       *string  `json:"imageUrl,omitempty" doc:"Event image URL"`
	DoorsTime      *string  `json:"doorsTime,omitempty" doc:"Doors time (e.g., 6:30 pm)"`
	ShowTime       *string  `json:"showTime,omitempty" doc:"Show time (e.g., 7:00 pm)"`
	TicketURL      *string  `json:"ticketUrl,omitempty" doc:"Ticket purchase URL"`
	Artists        []string `json:"artists" doc:"List of artist names"`
	ScrapedAt      string   `json:"scrapedAt" doc:"When the event was scraped (ISO timestamp)"`
	Price          *string  `json:"price,omitempty" doc:"Price string (e.g., $18, Free)"`
	AgeRestriction *string  `json:"ageRestriction,omitempty" doc:"Age restriction (e.g., 16+, All Ages)"`
	IsSoldOut      *bool    `json:"isSoldOut,omitempty" doc:"Whether the event is sold out"`
	IsCancelled    *bool    `json:"isCancelled,omitempty" doc:"Whether the event is cancelled"`
}

// DiscoveryImportRequest represents the HTTP request for importing discovered events
type DiscoveryImportRequest struct {
	Body struct {
		Events       []DiscoveryImportEventInput `json:"events" validate:"required,min=1" doc:"Array of discovered events to import"`
		DryRun       bool                        `json:"dryRun" doc:"If true, preview import without persisting"`
		AllowUpdates bool                        `json:"allowUpdates" doc:"If true, update existing shows with new data"`
	}
}

// DiscoveryImportResponse represents the HTTP response for importing discovered events
type DiscoveryImportResponse struct {
	Body contracts.ImportResult `json:"body"`
}

// DiscoveryImportHandler handles POST /admin/discovery/import
func (h *AdminDiscoveryHandler) DiscoveryImportHandler(ctx context.Context, req *DiscoveryImportRequest) (*DiscoveryImportResponse, error) {
	requestID := logger.GetRequestID(ctx)

	user, err := shared.RequireAdmin(ctx)
	if err != nil {
		return nil, err
	}

	if len(req.Body.Events) == 0 {
		return nil, huma.Error400BadRequest("At least one event is required")
	}

	if len(req.Body.Events) > 100 {
		return nil, huma.Error400BadRequest("Maximum 100 events can be imported at once")
	}

	logger.FromContext(ctx).Debug("admin_discovery_import_attempt",
		"event_count", len(req.Body.Events),
		"dry_run", req.Body.DryRun,
		"admin_id", user.ID,
	)

	// Convert input events to DiscoveredEvent format
	events := make([]contracts.DiscoveredEvent, len(req.Body.Events))
	for i, e := range req.Body.Events {
		events[i] = contracts.DiscoveredEvent{
			ID:             e.ID,
			Title:          e.Title,
			Date:           e.Date,
			Venue:          e.Venue,
			VenueSlug:      e.VenueSlug,
			ImageURL:       e.ImageURL,
			DoorsTime:      e.DoorsTime,
			ShowTime:       e.ShowTime,
			TicketURL:      e.TicketURL,
			Artists:        e.Artists,
			ScrapedAt:      e.ScrapedAt,
			Price:          e.Price,
			AgeRestriction: e.AgeRestriction,
			IsSoldOut:      e.IsSoldOut,
			IsCancelled:    e.IsCancelled,
		}
	}

	// Import events
	result, err := h.discoveryService.ImportEvents(events, req.Body.DryRun, req.Body.AllowUpdates, models.ShowStatusApproved)
	if err != nil {
		logger.FromContext(ctx).Error("admin_discovery_import_failed",
			"error", err.Error(),
			"request_id", requestID,
		)
		return nil, huma.Error500InternalServerError(
			fmt.Sprintf("Failed to import events (request_id: %s)", requestID),
		)
	}

	action := "imported"
	if req.Body.DryRun {
		action = "previewed"
	}

	logger.FromContext(ctx).Info("admin_discovery_import_success",
		"action", action,
		"total", result.Total,
		"imported", result.Imported,
		"duplicates", result.Duplicates,
		"rejected", result.Rejected,
		"pending_review", result.PendingReview,
		"errors", result.Errors,
		"admin_id", user.ID,
		"request_id", requestID,
	)

	return &DiscoveryImportResponse{Body: *result}, nil
}

// DiscoveryCheckEventInput represents a single event to check
type DiscoveryCheckEventInput struct {
	ID        string `json:"id" doc:"External event ID from the venue's system"`
	VenueSlug string `json:"venueSlug" doc:"Venue identifier (e.g., valley-bar)"`
	Date      string `json:"date,omitempty" doc:"Event date YYYY-MM-DD for venue+date fallback match"`
}

// DiscoveryCheckRequest represents the HTTP request for checking discovered events
type DiscoveryCheckRequest struct {
	Body struct {
		Events []DiscoveryCheckEventInput `json:"events" validate:"required,min=1" doc:"Array of events to check"`
	}
}

// DiscoveryCheckResponse represents the HTTP response for checking discovered events
type DiscoveryCheckResponse struct {
	Body contracts.CheckEventsResult `json:"body"`
}

// DiscoveryCheckHandler handles POST /admin/discovery/check
func (h *AdminDiscoveryHandler) DiscoveryCheckHandler(ctx context.Context, req *DiscoveryCheckRequest) (*DiscoveryCheckResponse, error) {
	requestID := logger.GetRequestID(ctx)

	user, err := shared.RequireAdmin(ctx)
	if err != nil {
		return nil, err
	}

	if len(req.Body.Events) == 0 {
		return nil, huma.Error400BadRequest("At least one event is required")
	}

	if len(req.Body.Events) > 200 {
		return nil, huma.Error400BadRequest("Maximum 200 events can be checked at once")
	}

	logger.FromContext(ctx).Debug("admin_discovery_check_attempt",
		"event_count", len(req.Body.Events),
		"admin_id", user.ID,
	)

	// Convert input to service types
	events := make([]contracts.CheckEventInput, len(req.Body.Events))
	for i, e := range req.Body.Events {
		events[i] = contracts.CheckEventInput{
			ID:        e.ID,
			VenueSlug: e.VenueSlug,
			Date:      e.Date,
		}
	}

	result, err := h.discoveryService.CheckEvents(events)
	if err != nil {
		logger.FromContext(ctx).Error("admin_discovery_check_failed",
			"error", err.Error(),
			"request_id", requestID,
		)
		return nil, huma.Error500InternalServerError(
			fmt.Sprintf("Failed to check events (request_id: %s)", requestID),
		)
	}

	logger.FromContext(ctx).Debug("admin_discovery_check_success",
		"checked", len(req.Body.Events),
		"found", len(result.Events),
		"admin_id", user.ID,
	)

	return &DiscoveryCheckResponse{Body: *result}, nil
}
