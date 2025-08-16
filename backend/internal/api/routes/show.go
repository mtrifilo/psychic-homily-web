package routes

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/go-chi/chi/v5"

	"psychic-homily-backend/internal/api/middleware"
	"psychic-homily-backend/internal/services"
)

// ShowHandler handles show-related HTTP requests
type ShowHandler struct {
	showService *services.ShowService
}

// NewShowHandler creates a new show handler
func NewShowHandler() *ShowHandler {
	return &ShowHandler{
		showService: services.NewShowService(),
	}
}

// Artist represents an artist in a show request
type Artist struct {
	ID          *uint   `json:"id,omitempty"`
	Name        *string `json:"name,omitempty"`
	IsHeadliner *bool   `json:"is_headliner,omitempty"`
}

// Venue represents a venue in a show request
type Venue struct {
	ID   *uint   `json:"id,omitempty"`
	Name *string `json:"name,omitempty"`
}

// initializeArtist provides sensible defaults for Artist fields
func initializeArtist(a *Artist) {
	// Set default for IsHeadliner if not provided
	if a.IsHeadliner == nil {
		// Default to false for non-headliners
		defaultValue := false
		a.IsHeadliner = &defaultValue
	}

	// Note: ID and Name are left as-is since validation will check
	// that at least one is provided. No need to "initialize" nil values.
}

// CreateShowRequestBody represents the request body with preprocessing
type CreateShowRequestBody struct {
	Title          string    `json:"title" validate:"required" doc:"Show title"`
	EventDate      time.Time `json:"event_date" validate:"required" doc:"Event date and time"`
	City           string    `json:"city" doc:"City where the show takes place"`
	State          string    `json:"state" doc:"State where the show takes place"`
	Price          *float64  `json:"price" doc:"Ticket price"`
	AgeRequirement string    `json:"age_requirement" doc:"Age requirement (e.g., '21+', 'All Ages')"`
	Description    string    `json:"description" doc:"Show description"`
	Venues         []Venue   `json:"venues" validate:"required,min=1" doc:"List of venues for the show"`
	Artists        []Artist  `json:"artists" validate:"required,min=1" doc:"List of artists in the show"`
}

// Resolve implements preprocessing and validation for the request body
func (r *CreateShowRequestBody) Resolve(ctx huma.Context) []error {
	var errors []error

	// Validate venues - no preprocessing needed currently
	for i := range r.Venues {
		// Validate that either ID or Name is provided
		venue := &r.Venues[i]
		if (venue.ID == nil || *venue.ID == 0) && (venue.Name == nil || *venue.Name == "") {
			errors = append(errors, &huma.ErrorDetail{
				Location: fmt.Sprintf("body.venues[%d]", i),
				Message:  "Either 'id' or 'name' must be provided",
				Value:    venue,
			})
		}
	}

	// Preprocess and validate artists
	for i := range r.Artists {
		initializeArtist(&r.Artists[i])

		// Validate that either ID or Name is provided
		artist := &r.Artists[i]
		if (artist.ID == nil || *artist.ID == 0) && (artist.Name == nil || *artist.Name == "") {
			errors = append(errors, &huma.ErrorDetail{
				Location: fmt.Sprintf("body.artists[%d]", i),
				Message:  "Either 'id' or 'name' must be provided",
				Value:    artist,
			})
		}
	}

	return errors
}

// CreateShowRequest represents the HTTP request for creating a show
type CreateShowRequest struct {
	Body CreateShowRequestBody `json:"body"`
}

// CreateShowResponse represents the HTTP response for creating a show
type CreateShowResponse struct {
	Body services.ShowResponse `json:"body"`
}

// GetShowRequest represents the HTTP request for getting a show
type GetShowRequest struct {
	ShowID string `path:"show_id" validate:"required" doc:"Show ID"`
}

// GetShowResponse represents the HTTP response for getting a show
type GetShowResponse struct {
	Body services.ShowResponse `json:"body"`
}

// GetShowsRequest represents the HTTP request for listing shows
type GetShowsRequest struct {
	City     string    `query:"city" doc:"Filter by city"`
	State    string    `query:"state" doc:"Filter by state"`
	FromDate time.Time `query:"from_date" doc:"Filter shows from this date"`
	ToDate   time.Time `query:"to_date" doc:"Filter shows until this date"`
}

// GetShowsResponse represents the HTTP response for listing shows
type GetShowsResponse struct {
	Body []*services.ShowResponse `json:"body"`
}

// UpdateShowRequest represents the HTTP request for updating a show
type UpdateShowRequest struct {
	ShowID string `path:"show_id" validate:"required" doc:"Show ID"`
	Body   struct {
		Title          *string    `json:"title" doc:"Show title"`
		EventDate      *time.Time `json:"event_date" doc:"Event date and time"`
		City           *string    `json:"city" doc:"City where the show takes place"`
		State          *string    `json:"state" doc:"State where the show takes place"`
		Price          *float64   `json:"price" doc:"Ticket price"`
		AgeRequirement *string    `json:"age_requirement" doc:"Age requirement"`
		Description    *string    `json:"description" doc:"Show description"`
		Venues         []Venue    `json:"venues" doc:"List of venues for the show"`
		Artists        []Artist   `json:"artists" doc:"List of artists in the show"`
	}
}

// UpdateShowResponse represents the HTTP response for updating a show
type UpdateShowResponse struct {
	Body services.ShowResponse `json:"body"`
}

// DeleteShowRequest represents the HTTP request for deleting a show
type DeleteShowRequest struct {
	ShowID string `path:"show_id" validate:"required" doc:"Show ID"`
}

// AIProcessShowRequest represents the HTTP request for AI show processing (future)
type AIProcessShowRequest struct {
	Body struct {
		Text string `json:"text" validate:"required" doc:"Unstructured text to process"`
	}
}

// AIProcessShowResponse represents the HTTP response for AI show processing
type AIProcessShowResponse struct {
	Body struct {
		Message string `json:"message" doc:"Response message"`
		Status  string `json:"status" doc:"Processing status"`
	}
}

// CreateShowHandler handles POST /shows
func (h *ShowHandler) CreateShowHandler(ctx context.Context, req *CreateShowRequest) (*CreateShowResponse, error) {
	// Validation is now handled by Huma's custom resolvers

	// Convert Venues to service format
	serviceVenues := make([]services.CreateShowVenue, len(req.Body.Venues))
	for i, venue := range req.Body.Venues {
		var name string
		if venue.Name != nil {
			name = *venue.Name
		}
		serviceVenues[i] = services.CreateShowVenue{
			ID:   venue.ID,
			Name: name,
		}
	}

	// Convert Artists to service format
	serviceArtists := make([]services.CreateShowArtist, len(req.Body.Artists))
	for i, artist := range req.Body.Artists {
		var name string
		if artist.Name != nil {
			name = *artist.Name
		}
		serviceArtists[i] = services.CreateShowArtist{
			ID:          artist.ID,
			Name:        name,
			IsHeadliner: artist.IsHeadliner,
		}
	}

	// Convert request to service request
	serviceReq := &services.CreateShowRequest{
		Title:          req.Body.Title,
		EventDate:      req.Body.EventDate,
		City:           req.Body.City,
		State:          req.Body.State,
		Price:          req.Body.Price,
		AgeRequirement: req.Body.AgeRequirement,
		Description:    req.Body.Description,
		Venues:         serviceVenues,
		Artists:        serviceArtists,
	}

	// Create show using service
	show, err := h.showService.CreateShow(serviceReq)
	if err != nil {
		return nil, huma.Error422UnprocessableEntity(fmt.Sprintf("Failed to create show: %v", err))
	}

	return &CreateShowResponse{Body: *show}, nil
}

// GetShowHandler handles GET /shows/{show_id}
func (h *ShowHandler) GetShowHandler(ctx context.Context, req *GetShowRequest) (*GetShowResponse, error) {
	// Parse show ID
	showID, err := strconv.ParseUint(req.ShowID, 10, 32)
	if err != nil {
		return nil, huma.Error400BadRequest("Invalid show ID")
	}

	// Get show using service
	show, err := h.showService.GetShow(uint(showID))
	if err != nil {
		return nil, huma.Error404NotFound(fmt.Sprintf("Show not found: %v", err))
	}

	return &GetShowResponse{Body: *show}, nil
}

// GetShowsHandler handles GET /shows
func (h *ShowHandler) GetShowsHandler(ctx context.Context, req *GetShowsRequest) (*GetShowsResponse, error) {
	// Build filters
	filters := make(map[string]interface{})
	if req.City != "" {
		filters["city"] = req.City
	}
	if req.State != "" {
		filters["state"] = req.State
	}
	if !req.FromDate.IsZero() {
		filters["from_date"] = req.FromDate
	}
	if !req.ToDate.IsZero() {
		filters["to_date"] = req.ToDate
	}

	// Get shows using service
	shows, err := h.showService.GetShows(filters)
	if err != nil {
		return nil, huma.Error500InternalServerError(fmt.Sprintf("Failed to get shows: %v", err))
	}

	return &GetShowsResponse{Body: shows}, nil
}

// UpdateShowHandler handles PUT /shows/{show_id}
func (h *ShowHandler) UpdateShowHandler(ctx context.Context, req *UpdateShowRequest) (*UpdateShowResponse, error) {
	// Parse show ID
	showID, err := strconv.ParseUint(req.ShowID, 10, 32)
	if err != nil {
		return nil, huma.Error400BadRequest("Invalid show ID")
	}

	// Build updates map
	updates := make(map[string]interface{})
	if req.Body.Title != nil {
		updates["title"] = *req.Body.Title
	}
	if req.Body.EventDate != nil {
		updates["event_date"] = *req.Body.EventDate
	}
	if req.Body.City != nil {
		updates["city"] = *req.Body.City
	}
	if req.Body.State != nil {
		updates["state"] = *req.Body.State
	}
	if req.Body.Price != nil {
		updates["price"] = *req.Body.Price
	}
	if req.Body.AgeRequirement != nil {
		updates["age_requirement"] = *req.Body.AgeRequirement
	}
	if req.Body.Description != nil {
		updates["description"] = *req.Body.Description
	}

	// Update show using service
	show, err := h.showService.UpdateShow(uint(showID), updates)
	if err != nil {
		return nil, huma.Error422UnprocessableEntity(fmt.Sprintf("Failed to update show: %v", err))
	}

	return &UpdateShowResponse{Body: *show}, nil
}

// DeleteShowHandler handles DELETE /shows/{show_id}
func (h *ShowHandler) DeleteShowHandler(ctx context.Context, req *DeleteShowRequest) (*huma.Response, error) {
	// Parse show ID
	showID, err := strconv.ParseUint(req.ShowID, 10, 32)
	if err != nil {
		return nil, huma.Error400BadRequest("Invalid show ID")
	}

	// Delete show using service
	err = h.showService.DeleteShow(uint(showID))
	if err != nil {
		return nil, huma.Error422UnprocessableEntity(fmt.Sprintf("Failed to delete show: %v", err))
	}

	// Return 204 No Content
	return &huma.Response{}, nil
}

// AIProcessShowHandler handles POST /shows/ai-process (future implementation)
func (h *ShowHandler) AIProcessShowHandler(ctx context.Context, req *AIProcessShowRequest) (*AIProcessShowResponse, error) {
	// TODO: Implement AI processing logic
	// For now, return "Not Implemented" response
	return &AIProcessShowResponse{
		Body: struct {
			Message string `json:"message" doc:"Response message"`
			Status  string `json:"status" doc:"Processing status"`
		}{
			Message: "AI show processing is not yet implemented",
			Status:  "not_implemented",
		},
	}, nil
}

// SetupShowRoutes configures all show-related endpoints
func SetupShowRoutes(router *chi.Mux, api huma.API, jwtService *services.JWTService) {
	showHandler := NewShowHandler()

	// Public show endpoints
	huma.Get(api, "/shows", showHandler.GetShowsHandler)
	huma.Get(api, "/shows/{show_id}", showHandler.GetShowHandler)

	// Protected show endpoints (require authentication)
	router.Group(func(r chi.Router) {
		r.Use(middleware.JWTMiddleware(jwtService))

		// Create show
		huma.Post(api, "/shows", showHandler.CreateShowHandler)

		// Update show
		huma.Put(api, "/shows/{show_id}", showHandler.UpdateShowHandler)

		// Delete show
		huma.Delete(api, "/shows/{show_id}", showHandler.DeleteShowHandler)

		// AI processing endpoint (future)
		huma.Post(api, "/shows/ai-process", showHandler.AIProcessShowHandler)
	})
}
