package catalog

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/danielgtaylor/huma/v2"

	"psychic-homily-backend/internal/api/handlers/shared"
	apperrors "psychic-homily-backend/internal/errors"
	"psychic-homily-backend/internal/logger"
	adminm "psychic-homily-backend/internal/models/admin"
	"psychic-homily-backend/internal/services/contracts"
)

type FestivalHandler struct {
	festivalService contracts.FestivalServiceInterface
	artistService   contracts.ArtistServiceInterface
	auditLogService contracts.AuditLogServiceInterface
	revisionService contracts.RevisionServiceInterface
}

func NewFestivalHandler(festivalService contracts.FestivalServiceInterface, artistService contracts.ArtistServiceInterface, auditLogService contracts.AuditLogServiceInterface, revisionService contracts.RevisionServiceInterface) *FestivalHandler {
	return &FestivalHandler{
		festivalService: festivalService,
		artistService:   artistService,
		auditLogService: auditLogService,
		revisionService: revisionService,
	}
}

// ============================================================================
// Search Festivals
// ============================================================================

// SearchFestivalsRequest represents the autocomplete search request
type SearchFestivalsRequest struct {
	Query string `query:"q" doc:"Search query for festival autocomplete" example:"m3f"`
}

// SearchFestivalsResponse represents the autocomplete search response
type SearchFestivalsResponse struct {
	Body struct {
		Festivals []*contracts.FestivalListResponse `json:"festivals" doc:"Matching festivals"`
		Count     int                               `json:"count" doc:"Number of results"`
	}
}

// SearchFestivalsHandler handles GET /festivals/search?q=query
func (h *FestivalHandler) SearchFestivalsHandler(ctx context.Context, req *SearchFestivalsRequest) (*SearchFestivalsResponse, error) {
	festivals, err := h.festivalService.SearchFestivals(req.Query)
	if err != nil {
		return nil, err
	}

	resp := &SearchFestivalsResponse{}
	resp.Body.Festivals = festivals
	resp.Body.Count = len(festivals)

	return resp, nil
}

// ============================================================================
// List Festivals
// ============================================================================

// ListFestivalsRequest represents the request for listing festivals
type ListFestivalsRequest struct {
	City       string `query:"city" required:"false" doc:"Filter by city" example:"Phoenix"`
	State      string `query:"state" required:"false" doc:"Filter by state" example:"AZ"`
	Year       int    `query:"year" required:"false" doc:"Filter by edition year" example:"2026"`
	Status     string `query:"status" required:"false" doc:"Filter by status (announced, confirmed, cancelled, completed)" example:"confirmed"`
	SeriesSlug string `query:"series_slug" required:"false" doc:"Filter by festival series slug" example:"m3f"`
	Tags       string `query:"tags" required:"false" doc:"Comma-separated tag slugs. Multi-tag filter (PSY-309): AND by default; set tag_match=any for OR." example:"electronic,festival"`
	TagMatch   string `query:"tag_match" required:"false" doc:"Tag matching mode: 'all' (default, AND) or 'any' (OR)" example:"all" enum:"all,any"`
}

// ListFestivalsResponse represents the response for listing festivals
type ListFestivalsResponse struct {
	Body struct {
		Festivals []*contracts.FestivalListResponse `json:"festivals" doc:"List of festivals"`
		Count     int                               `json:"count" doc:"Number of festivals"`
	}
}

// ListFestivalsHandler handles GET /festivals
func (h *FestivalHandler) ListFestivalsHandler(ctx context.Context, req *ListFestivalsRequest) (*ListFestivalsResponse, error) {
	filters := make(map[string]interface{})

	if req.City != "" {
		filters["city"] = req.City
	}
	if req.State != "" {
		filters["state"] = req.State
	}
	if req.Year > 0 {
		filters["year"] = req.Year
	}
	if req.Status != "" {
		filters["status"] = req.Status
	}
	if req.SeriesSlug != "" {
		filters["series_slug"] = req.SeriesSlug
	}
	if tf := parseTagFilter(req.Tags, req.TagMatch); tf.HasTags() {
		filters["tag_filter"] = tf
	}

	festivals, err := h.festivalService.ListFestivals(filters)
	if err != nil {
		return nil, huma.Error500InternalServerError("Failed to fetch festivals", err)
	}

	resp := &ListFestivalsResponse{}
	resp.Body.Festivals = festivals
	resp.Body.Count = len(festivals)

	return resp, nil
}

// ============================================================================
// Get Festival
// ============================================================================

// GetFestivalRequest represents the request for getting a single festival
type GetFestivalRequest struct {
	FestivalID string `path:"festival_id" doc:"Festival ID or slug" example:"m3f-2026"`
}

// GetFestivalResponse represents the response for the get festival endpoint
type GetFestivalResponse struct {
	Body *contracts.FestivalDetailResponse
}

// GetFestivalHandler handles GET /festivals/{festival_id}
func (h *FestivalHandler) GetFestivalHandler(ctx context.Context, req *GetFestivalRequest) (*GetFestivalResponse, error) {
	var festival *contracts.FestivalDetailResponse
	var err error

	// Try to parse as numeric ID first
	if id, parseErr := strconv.ParseUint(req.FestivalID, 10, 32); parseErr == nil {
		festival, err = h.festivalService.GetFestival(uint(id))
	} else {
		// Fall back to slug lookup
		festival, err = h.festivalService.GetFestivalBySlug(req.FestivalID)
	}

	if err != nil {
		var festivalErr *apperrors.FestivalError
		if errors.As(err, &festivalErr) && festivalErr.Code == apperrors.CodeFestivalNotFound {
			return nil, huma.Error404NotFound("Festival not found")
		}
		return nil, huma.Error500InternalServerError("Failed to fetch festival", err)
	}

	return &GetFestivalResponse{Body: festival}, nil
}

// ============================================================================
// Create Festival
// ============================================================================

// CreateFestivalRequest represents the request for creating a festival
type CreateFestivalRequest struct {
	Body struct {
		Name         string  `json:"name" doc:"Festival name" example:"M3F Festival"`
		SeriesSlug   string  `json:"series_slug" doc:"Festival series slug (for recurring festivals)" example:"m3f"`
		EditionYear  int     `json:"edition_year" doc:"Festival edition year" example:"2026"`
		Description  *string `json:"description,omitempty" required:"false" doc:"Description"`
		LocationName *string `json:"location_name,omitempty" required:"false" doc:"Location name" example:"Margaret T. Hance Park"`
		City         *string `json:"city,omitempty" required:"false" doc:"City" example:"Phoenix"`
		State        *string `json:"state,omitempty" required:"false" doc:"State" example:"AZ"`
		Country      *string `json:"country,omitempty" required:"false" doc:"Country" example:"US"`
		StartDate    string  `json:"start_date" doc:"Start date (YYYY-MM-DD)" example:"2026-03-06"`
		EndDate      string  `json:"end_date" doc:"End date (YYYY-MM-DD)" example:"2026-03-08"`
		Website      *string `json:"website,omitempty" required:"false" doc:"Website URL"`
		TicketURL    *string `json:"ticket_url,omitempty" required:"false" doc:"Ticket URL"`
		FlyerURL     *string `json:"flyer_url,omitempty" required:"false" doc:"Flyer URL"`
		Status       string  `json:"status,omitempty" required:"false" doc:"Status (announced, confirmed, cancelled, completed)" example:"announced"`
	}
}

// CreateFestivalResponse represents the response for creating a festival
type CreateFestivalResponse struct {
	Body *contracts.FestivalDetailResponse
}

// CreateFestivalHandler handles POST /festivals
func (h *FestivalHandler) CreateFestivalHandler(ctx context.Context, req *CreateFestivalRequest) (*CreateFestivalResponse, error) {
	requestID := logger.GetRequestID(ctx)

	user, err := shared.RequireAdmin(ctx)
	if err != nil {
		return nil, err
	}

	if req.Body.Name == "" {
		return nil, huma.Error400BadRequest("Name is required")
	}
	if req.Body.SeriesSlug == "" {
		return nil, huma.Error400BadRequest("Series slug is required")
	}
	if req.Body.EditionYear == 0 {
		return nil, huma.Error400BadRequest("Edition year is required")
	}
	if req.Body.StartDate == "" {
		return nil, huma.Error400BadRequest("Start date is required")
	}
	if req.Body.EndDate == "" {
		return nil, huma.Error400BadRequest("End date is required")
	}

	serviceReq := &contracts.CreateFestivalRequest{
		Name:         req.Body.Name,
		SeriesSlug:   req.Body.SeriesSlug,
		EditionYear:  req.Body.EditionYear,
		Description:  req.Body.Description,
		LocationName: req.Body.LocationName,
		City:         req.Body.City,
		State:        req.Body.State,
		Country:      req.Body.Country,
		StartDate:    req.Body.StartDate,
		EndDate:      req.Body.EndDate,
		Website:      req.Body.Website,
		TicketURL:    req.Body.TicketURL,
		FlyerURL:     req.Body.FlyerURL,
		Status:       req.Body.Status,
	}

	festival, err := h.festivalService.CreateFestival(serviceReq)
	if err != nil {
		logger.FromContext(ctx).Error("create_festival_failed",
			"error", err.Error(),
			"request_id", requestID,
		)
		return nil, huma.Error500InternalServerError(
			fmt.Sprintf("Failed to create festival (request_id: %s)", requestID),
		)
	}

	// Audit log (fire and forget)
	if h.auditLogService != nil {
		go func() {
			h.auditLogService.LogAction(user.ID, "create_festival", "festival", festival.ID, nil)
		}()
	}

	logger.FromContext(ctx).Info("festival_created",
		"festival_id", festival.ID,
		"admin_id", user.ID,
		"request_id", requestID,
	)

	return &CreateFestivalResponse{Body: festival}, nil
}

// ============================================================================
// Update Festival
// ============================================================================

// UpdateFestivalRequest represents the request for updating a festival
type UpdateFestivalRequest struct {
	FestivalID string `path:"festival_id" doc:"Festival ID" example:"1"`
	Body       struct {
		Name         *string `json:"name,omitempty" required:"false" doc:"Festival name"`
		SeriesSlug   *string `json:"series_slug,omitempty" required:"false" doc:"Series slug"`
		EditionYear  *int    `json:"edition_year,omitempty" required:"false" doc:"Edition year"`
		Description  *string `json:"description,omitempty" required:"false" doc:"Description"`
		LocationName *string `json:"location_name,omitempty" required:"false" doc:"Location name"`
		City         *string `json:"city,omitempty" required:"false" doc:"City"`
		State        *string `json:"state,omitempty" required:"false" doc:"State"`
		Country      *string `json:"country,omitempty" required:"false" doc:"Country"`
		StartDate    *string `json:"start_date,omitempty" required:"false" doc:"Start date (YYYY-MM-DD)"`
		EndDate      *string `json:"end_date,omitempty" required:"false" doc:"End date (YYYY-MM-DD)"`
		Website      *string `json:"website,omitempty" required:"false" doc:"Website URL"`
		TicketURL    *string `json:"ticket_url,omitempty" required:"false" doc:"Ticket URL"`
		FlyerURL     *string `json:"flyer_url,omitempty" required:"false" doc:"Flyer URL"`
		Status       *string `json:"status,omitempty" required:"false" doc:"Status"`
		Summary      *string `json:"summary,omitempty" required:"false" doc:"Revision summary describing the change"`
	}
}

// UpdateFestivalResponse represents the response for updating a festival
type UpdateFestivalResponse struct {
	Body *contracts.FestivalDetailResponse
}

// UpdateFestivalHandler handles PUT /festivals/{festival_id}
func (h *FestivalHandler) UpdateFestivalHandler(ctx context.Context, req *UpdateFestivalRequest) (*UpdateFestivalResponse, error) {
	requestID := logger.GetRequestID(ctx)

	user, err := shared.RequireAdmin(ctx)
	if err != nil {
		return nil, err
	}

	festivalID, err := h.resolveFestivalID(req.FestivalID)
	if err != nil {
		return nil, err
	}

	// Capture old values for revision diff (fire-and-forget safe)
	var oldFestival *contracts.FestivalDetailResponse
	if h.revisionService != nil {
		oldFestival, _ = h.festivalService.GetFestival(festivalID)
	}

	serviceReq := &contracts.UpdateFestivalRequest{
		Name:         req.Body.Name,
		SeriesSlug:   req.Body.SeriesSlug,
		EditionYear:  req.Body.EditionYear,
		Description:  req.Body.Description,
		LocationName: req.Body.LocationName,
		City:         req.Body.City,
		State:        req.Body.State,
		Country:      req.Body.Country,
		StartDate:    req.Body.StartDate,
		EndDate:      req.Body.EndDate,
		Website:      req.Body.Website,
		TicketURL:    req.Body.TicketURL,
		FlyerURL:     req.Body.FlyerURL,
		Status:       req.Body.Status,
	}

	festival, err := h.festivalService.UpdateFestival(festivalID, serviceReq)
	if err != nil {
		var festivalErr *apperrors.FestivalError
		if errors.As(err, &festivalErr) && festivalErr.Code == apperrors.CodeFestivalNotFound {
			return nil, huma.Error404NotFound("Festival not found")
		}
		logger.FromContext(ctx).Error("update_festival_failed",
			"festival_id", festivalID,
			"error", err.Error(),
			"request_id", requestID,
		)
		return nil, huma.Error500InternalServerError(
			fmt.Sprintf("Failed to update festival (request_id: %s)", requestID),
		)
	}

	// Audit log (fire and forget)
	if h.auditLogService != nil {
		go func() {
			h.auditLogService.LogAction(user.ID, "edit_festival", "festival", festivalID, nil)
		}()
	}

	// Record revision (fire and forget)
	if h.revisionService != nil && oldFestival != nil {
		go func() {
			changes := computeFestivalChanges(oldFestival, festival)
			if len(changes) > 0 {
				summary := ""
				if req.Body.Summary != nil {
					summary = *req.Body.Summary
				}
				if err := h.revisionService.RecordRevision("festival", festivalID, user.ID, changes, summary); err != nil {
					logger.Default().Error("record_festival_revision_failed",
						"festival_id", festivalID,
						"error", err.Error(),
					)
				}
			}
		}()
	}

	logger.FromContext(ctx).Info("festival_updated",
		"festival_id", festivalID,
		"admin_id", user.ID,
		"request_id", requestID,
	)

	return &UpdateFestivalResponse{Body: festival}, nil
}

// ============================================================================
// Delete Festival
// ============================================================================

// DeleteFestivalRequest represents the request for deleting a festival
type DeleteFestivalRequest struct {
	FestivalID string `path:"festival_id" doc:"Festival ID" example:"1"`
}

// DeleteFestivalHandler handles DELETE /festivals/{festival_id}
func (h *FestivalHandler) DeleteFestivalHandler(ctx context.Context, req *DeleteFestivalRequest) (*struct{}, error) {
	requestID := logger.GetRequestID(ctx)

	user, err := shared.RequireAdmin(ctx)
	if err != nil {
		return nil, err
	}

	festivalID, err := h.resolveFestivalID(req.FestivalID)
	if err != nil {
		return nil, err
	}

	err = h.festivalService.DeleteFestival(festivalID)
	if err != nil {
		var festivalErr *apperrors.FestivalError
		if errors.As(err, &festivalErr) && festivalErr.Code == apperrors.CodeFestivalNotFound {
			return nil, huma.Error404NotFound("Festival not found")
		}
		logger.FromContext(ctx).Error("delete_festival_failed",
			"festival_id", festivalID,
			"error", err.Error(),
			"request_id", requestID,
		)
		return nil, huma.Error500InternalServerError(
			fmt.Sprintf("Failed to delete festival (request_id: %s)", requestID),
		)
	}

	// Audit log (fire and forget)
	if h.auditLogService != nil {
		go func() {
			h.auditLogService.LogAction(user.ID, "delete_festival", "festival", festivalID, nil)
		}()
	}

	logger.FromContext(ctx).Info("festival_deleted",
		"festival_id", festivalID,
		"admin_id", user.ID,
		"request_id", requestID,
	)

	return nil, nil
}

// ============================================================================
// Festival Lineup (Artists)
// ============================================================================

// GetFestivalArtistsRequest represents the request for getting a festival's artists
type GetFestivalArtistsRequest struct {
	FestivalID string `path:"festival_id" doc:"Festival ID or slug" example:"m3f-2026"`
	DayDate    string `query:"day_date" required:"false" doc:"Filter by day (YYYY-MM-DD)" example:"2026-03-07"`
}

// GetFestivalArtistsResponse represents the response for the festival lineup endpoint
type GetFestivalArtistsResponse struct {
	Body struct {
		Artists []*contracts.FestivalArtistResponse `json:"artists" doc:"List of artists in lineup"`
		Count   int                                 `json:"count" doc:"Number of artists"`
	}
}

// GetFestivalArtistsHandler handles GET /festivals/{festival_id}/artists
func (h *FestivalHandler) GetFestivalArtistsHandler(ctx context.Context, req *GetFestivalArtistsRequest) (*GetFestivalArtistsResponse, error) {
	festivalID, err := h.resolveFestivalID(req.FestivalID)
	if err != nil {
		return nil, err
	}

	var dayDate *string
	if req.DayDate != "" {
		dayDate = &req.DayDate
	}

	artists, err := h.festivalService.GetFestivalArtists(festivalID, dayDate)
	if err != nil {
		var festivalErr *apperrors.FestivalError
		if errors.As(err, &festivalErr) && festivalErr.Code == apperrors.CodeFestivalNotFound {
			return nil, huma.Error404NotFound("Festival not found")
		}
		return nil, huma.Error500InternalServerError("Failed to fetch festival lineup", err)
	}

	resp := &GetFestivalArtistsResponse{}
	resp.Body.Artists = artists
	resp.Body.Count = len(artists)

	return resp, nil
}

// AddFestivalArtistRequest represents the request for adding an artist to a festival
type AddFestivalArtistHandlerRequest struct {
	FestivalID string `path:"festival_id" doc:"Festival ID" example:"1"`
	Body       struct {
		ArtistID    uint    `json:"artist_id" doc:"Artist ID"`
		BillingTier string  `json:"billing_tier,omitempty" required:"false" doc:"Billing tier (headliner, sub_headliner, mid_card, undercard, local, dj, host)" example:"headliner"`
		Position    int     `json:"position,omitempty" required:"false" doc:"Position within tier (0-based)" example:"0"`
		DayDate     *string `json:"day_date,omitempty" required:"false" doc:"Day date (YYYY-MM-DD)" example:"2026-03-07"`
		Stage       *string `json:"stage,omitempty" required:"false" doc:"Stage name" example:"Main Stage"`
		SetTime     *string `json:"set_time,omitempty" required:"false" doc:"Set time (HH:MM:SS)" example:"21:00:00"`
		VenueID     *uint   `json:"venue_id,omitempty" required:"false" doc:"Venue ID (for multi-venue festivals)"`
	}
}

// AddFestivalArtistResponse represents the response for adding an artist to a festival
type AddFestivalArtistHandlerResponse struct {
	Body *contracts.FestivalArtistResponse
}

// AddFestivalArtistHandler handles POST /festivals/{festival_id}/artists
func (h *FestivalHandler) AddFestivalArtistHandler(ctx context.Context, req *AddFestivalArtistHandlerRequest) (*AddFestivalArtistHandlerResponse, error) {
	requestID := logger.GetRequestID(ctx)

	user, err := shared.RequireAdmin(ctx)
	if err != nil {
		return nil, err
	}

	festivalID, err := strconv.ParseUint(req.FestivalID, 10, 32)
	if err != nil {
		return nil, huma.Error400BadRequest("Invalid festival ID")
	}

	if req.Body.ArtistID == 0 {
		return nil, huma.Error400BadRequest("Artist ID is required")
	}

	serviceReq := &contracts.AddFestivalArtistRequest{
		ArtistID:    req.Body.ArtistID,
		BillingTier: req.Body.BillingTier,
		Position:    req.Body.Position,
		DayDate:     req.Body.DayDate,
		Stage:       req.Body.Stage,
		SetTime:     req.Body.SetTime,
		VenueID:     req.Body.VenueID,
	}

	artist, err := h.festivalService.AddFestivalArtist(uint(festivalID), serviceReq)
	if err != nil {
		var festivalErr *apperrors.FestivalError
		if errors.As(err, &festivalErr) && festivalErr.Code == apperrors.CodeFestivalNotFound {
			return nil, huma.Error404NotFound("Festival not found")
		}
		if err.Error() == "artist not found" {
			return nil, huma.Error404NotFound("Artist not found")
		}
		logger.FromContext(ctx).Error("add_festival_artist_failed",
			"festival_id", festivalID,
			"error", err.Error(),
			"request_id", requestID,
		)
		return nil, huma.Error500InternalServerError(
			fmt.Sprintf("Failed to add artist to festival (request_id: %s)", requestID),
		)
	}

	// Audit log (fire and forget)
	if h.auditLogService != nil {
		go func() {
			h.auditLogService.LogAction(user.ID, "add_festival_artist", "festival", uint(festivalID), nil)
		}()
	}

	return &AddFestivalArtistHandlerResponse{Body: artist}, nil
}

// UpdateFestivalArtistHandlerRequest represents the request for updating a festival artist
type UpdateFestivalArtistHandlerRequest struct {
	FestivalID string `path:"festival_id" doc:"Festival ID" example:"1"`
	ArtistID   string `path:"artist_id" doc:"Artist ID" example:"1"`
	Body       struct {
		BillingTier *string `json:"billing_tier,omitempty" required:"false" doc:"Billing tier"`
		Position    *int    `json:"position,omitempty" required:"false" doc:"Position within tier"`
		DayDate     *string `json:"day_date,omitempty" required:"false" doc:"Day date (YYYY-MM-DD)"`
		Stage       *string `json:"stage,omitempty" required:"false" doc:"Stage name"`
		SetTime     *string `json:"set_time,omitempty" required:"false" doc:"Set time (HH:MM:SS)"`
		VenueID     *uint   `json:"venue_id,omitempty" required:"false" doc:"Venue ID"`
	}
}

// UpdateFestivalArtistHandlerResponse represents the response for updating a festival artist
type UpdateFestivalArtistHandlerResponse struct {
	Body *contracts.FestivalArtistResponse
}

// UpdateFestivalArtistHandler handles PUT /festivals/{festival_id}/artists/{artist_id}
func (h *FestivalHandler) UpdateFestivalArtistHandler(ctx context.Context, req *UpdateFestivalArtistHandlerRequest) (*UpdateFestivalArtistHandlerResponse, error) {
	requestID := logger.GetRequestID(ctx)

	user, err := shared.RequireAdmin(ctx)
	if err != nil {
		return nil, err
	}

	festivalID, err := strconv.ParseUint(req.FestivalID, 10, 32)
	if err != nil {
		return nil, huma.Error400BadRequest("Invalid festival ID")
	}

	artistID, err := strconv.ParseUint(req.ArtistID, 10, 32)
	if err != nil {
		return nil, huma.Error400BadRequest("Invalid artist ID")
	}

	serviceReq := &contracts.UpdateFestivalArtistRequest{
		BillingTier: req.Body.BillingTier,
		Position:    req.Body.Position,
		DayDate:     req.Body.DayDate,
		Stage:       req.Body.Stage,
		SetTime:     req.Body.SetTime,
		VenueID:     req.Body.VenueID,
	}

	artist, err := h.festivalService.UpdateFestivalArtist(uint(festivalID), uint(artistID), serviceReq)
	if err != nil {
		if err.Error() == "artist not found in festival lineup" {
			return nil, huma.Error404NotFound("Artist not found in festival lineup")
		}
		logger.FromContext(ctx).Error("update_festival_artist_failed",
			"festival_id", festivalID,
			"artist_id", artistID,
			"error", err.Error(),
			"request_id", requestID,
		)
		return nil, huma.Error500InternalServerError(
			fmt.Sprintf("Failed to update festival artist (request_id: %s)", requestID),
		)
	}

	// Audit log (fire and forget)
	if h.auditLogService != nil {
		go func() {
			h.auditLogService.LogAction(user.ID, "update_festival_artist", "festival", uint(festivalID), nil)
		}()
	}

	return &UpdateFestivalArtistHandlerResponse{Body: artist}, nil
}

// RemoveFestivalArtistRequest represents the request for removing an artist from a festival
type RemoveFestivalArtistRequest struct {
	FestivalID string `path:"festival_id" doc:"Festival ID" example:"1"`
	ArtistID   string `path:"artist_id" doc:"Artist ID" example:"1"`
}

// RemoveFestivalArtistHandler handles DELETE /festivals/{festival_id}/artists/{artist_id}
func (h *FestivalHandler) RemoveFestivalArtistHandler(ctx context.Context, req *RemoveFestivalArtistRequest) (*struct{}, error) {
	requestID := logger.GetRequestID(ctx)

	user, err := shared.RequireAdmin(ctx)
	if err != nil {
		return nil, err
	}

	festivalID, err := strconv.ParseUint(req.FestivalID, 10, 32)
	if err != nil {
		return nil, huma.Error400BadRequest("Invalid festival ID")
	}

	artistID, err := strconv.ParseUint(req.ArtistID, 10, 32)
	if err != nil {
		return nil, huma.Error400BadRequest("Invalid artist ID")
	}

	err = h.festivalService.RemoveFestivalArtist(uint(festivalID), uint(artistID))
	if err != nil {
		if err.Error() == "artist not found in festival lineup" {
			return nil, huma.Error404NotFound("Artist not found in festival lineup")
		}
		logger.FromContext(ctx).Error("remove_festival_artist_failed",
			"festival_id", festivalID,
			"artist_id", artistID,
			"error", err.Error(),
			"request_id", requestID,
		)
		return nil, huma.Error500InternalServerError(
			fmt.Sprintf("Failed to remove artist from festival (request_id: %s)", requestID),
		)
	}

	// Audit log (fire and forget)
	if h.auditLogService != nil {
		go func() {
			h.auditLogService.LogAction(user.ID, "remove_festival_artist", "festival", uint(festivalID), nil)
		}()
	}

	return nil, nil
}

// ============================================================================
// Festival Venues
// ============================================================================

// GetFestivalVenuesRequest represents the request for getting a festival's venues
type GetFestivalVenuesRequest struct {
	FestivalID string `path:"festival_id" doc:"Festival ID or slug" example:"m3f-2026"`
}

// GetFestivalVenuesResponse represents the response for the festival venues endpoint
type GetFestivalVenuesResponse struct {
	Body struct {
		Venues []*contracts.FestivalVenueResponse `json:"venues" doc:"List of venues"`
		Count  int                                `json:"count" doc:"Number of venues"`
	}
}

// GetFestivalVenuesHandler handles GET /festivals/{festival_id}/venues
func (h *FestivalHandler) GetFestivalVenuesHandler(ctx context.Context, req *GetFestivalVenuesRequest) (*GetFestivalVenuesResponse, error) {
	festivalID, err := h.resolveFestivalID(req.FestivalID)
	if err != nil {
		return nil, err
	}

	venues, err := h.festivalService.GetFestivalVenues(festivalID)
	if err != nil {
		var festivalErr *apperrors.FestivalError
		if errors.As(err, &festivalErr) && festivalErr.Code == apperrors.CodeFestivalNotFound {
			return nil, huma.Error404NotFound("Festival not found")
		}
		return nil, huma.Error500InternalServerError("Failed to fetch festival venues", err)
	}

	resp := &GetFestivalVenuesResponse{}
	resp.Body.Venues = venues
	resp.Body.Count = len(venues)

	return resp, nil
}

// AddFestivalVenueHandlerRequest represents the request for adding a venue to a festival
type AddFestivalVenueHandlerRequest struct {
	FestivalID string `path:"festival_id" doc:"Festival ID" example:"1"`
	Body       struct {
		VenueID   uint `json:"venue_id" doc:"Venue ID"`
		IsPrimary bool `json:"is_primary,omitempty" required:"false" doc:"Whether this is the primary venue"`
	}
}

// AddFestivalVenueHandlerResponse represents the response for adding a venue to a festival
type AddFestivalVenueHandlerResponse struct {
	Body *contracts.FestivalVenueResponse
}

// AddFestivalVenueHandler handles POST /festivals/{festival_id}/venues
func (h *FestivalHandler) AddFestivalVenueHandler(ctx context.Context, req *AddFestivalVenueHandlerRequest) (*AddFestivalVenueHandlerResponse, error) {
	requestID := logger.GetRequestID(ctx)

	user, err := shared.RequireAdmin(ctx)
	if err != nil {
		return nil, err
	}

	festivalID, err := strconv.ParseUint(req.FestivalID, 10, 32)
	if err != nil {
		return nil, huma.Error400BadRequest("Invalid festival ID")
	}

	if req.Body.VenueID == 0 {
		return nil, huma.Error400BadRequest("Venue ID is required")
	}

	serviceReq := &contracts.AddFestivalVenueRequest{
		VenueID:   req.Body.VenueID,
		IsPrimary: req.Body.IsPrimary,
	}

	venue, err := h.festivalService.AddFestivalVenue(uint(festivalID), serviceReq)
	if err != nil {
		var festivalErr *apperrors.FestivalError
		if errors.As(err, &festivalErr) && festivalErr.Code == apperrors.CodeFestivalNotFound {
			return nil, huma.Error404NotFound("Festival not found")
		}
		if err.Error() == "venue not found" {
			return nil, huma.Error404NotFound("Venue not found")
		}
		logger.FromContext(ctx).Error("add_festival_venue_failed",
			"festival_id", festivalID,
			"error", err.Error(),
			"request_id", requestID,
		)
		return nil, huma.Error500InternalServerError(
			fmt.Sprintf("Failed to add venue to festival (request_id: %s)", requestID),
		)
	}

	// Audit log (fire and forget)
	if h.auditLogService != nil {
		go func() {
			h.auditLogService.LogAction(user.ID, "add_festival_venue", "festival", uint(festivalID), nil)
		}()
	}

	return &AddFestivalVenueHandlerResponse{Body: venue}, nil
}

// RemoveFestivalVenueRequest represents the request for removing a venue from a festival
type RemoveFestivalVenueRequest struct {
	FestivalID string `path:"festival_id" doc:"Festival ID" example:"1"`
	VenueID    string `path:"venue_id" doc:"Venue ID" example:"1"`
}

// RemoveFestivalVenueHandler handles DELETE /festivals/{festival_id}/venues/{venue_id}
func (h *FestivalHandler) RemoveFestivalVenueHandler(ctx context.Context, req *RemoveFestivalVenueRequest) (*struct{}, error) {
	requestID := logger.GetRequestID(ctx)

	user, err := shared.RequireAdmin(ctx)
	if err != nil {
		return nil, err
	}

	festivalID, err := strconv.ParseUint(req.FestivalID, 10, 32)
	if err != nil {
		return nil, huma.Error400BadRequest("Invalid festival ID")
	}

	venueID, err := strconv.ParseUint(req.VenueID, 10, 32)
	if err != nil {
		return nil, huma.Error400BadRequest("Invalid venue ID")
	}

	err = h.festivalService.RemoveFestivalVenue(uint(festivalID), uint(venueID))
	if err != nil {
		if err.Error() == "venue not found in festival" {
			return nil, huma.Error404NotFound("Venue not found in festival")
		}
		logger.FromContext(ctx).Error("remove_festival_venue_failed",
			"festival_id", festivalID,
			"venue_id", venueID,
			"error", err.Error(),
			"request_id", requestID,
		)
		return nil, huma.Error500InternalServerError(
			fmt.Sprintf("Failed to remove venue from festival (request_id: %s)", requestID),
		)
	}

	// Audit log (fire and forget)
	if h.auditLogService != nil {
		go func() {
			h.auditLogService.LogAction(user.ID, "remove_festival_venue", "festival", uint(festivalID), nil)
		}()
	}

	return nil, nil
}

// ============================================================================
// Artist Festivals (cross-entity query)
// ============================================================================

// GetArtistFestivalsRequest represents the request for getting an artist's festivals
type GetArtistFestivalsRequest struct {
	ArtistID string `path:"artist_id" doc:"Artist ID or slug" example:"the-national"`
}

// GetArtistFestivalsResponse represents the response for the artist festivals endpoint
type GetArtistFestivalsResponse struct {
	Body struct {
		Festivals []*contracts.ArtistFestivalListResponse `json:"festivals" doc:"List of festivals"`
		Count     int                                     `json:"count" doc:"Number of festivals"`
	}
}

// GetArtistFestivalsHandler handles GET /artists/{artist_id}/festivals
func (h *FestivalHandler) GetArtistFestivalsHandler(ctx context.Context, req *GetArtistFestivalsRequest) (*GetArtistFestivalsResponse, error) {
	// Resolve artist ID from numeric ID or slug
	var artistID uint
	if id, parseErr := strconv.ParseUint(req.ArtistID, 10, 32); parseErr == nil {
		artistID = uint(id)
	} else {
		// Look up by slug to get the ID
		artist, err := h.artistService.GetArtistBySlug(req.ArtistID)
		if err != nil {
			var artistErr *apperrors.ArtistError
			if errors.As(err, &artistErr) && artistErr.Code == apperrors.CodeArtistNotFound {
				return nil, huma.Error404NotFound("Artist not found")
			}
			return nil, huma.Error500InternalServerError("Failed to fetch artist", err)
		}
		artistID = artist.ID
	}

	festivals, err := h.festivalService.GetFestivalsForArtist(artistID)
	if err != nil {
		var artistErr *apperrors.ArtistError
		if errors.As(err, &artistErr) && artistErr.Code == apperrors.CodeArtistNotFound {
			return nil, huma.Error404NotFound("Artist not found")
		}
		return nil, huma.Error500InternalServerError("Failed to fetch festivals", err)
	}

	resp := &GetArtistFestivalsResponse{}
	resp.Body.Festivals = festivals
	resp.Body.Count = len(festivals)

	return resp, nil
}

// ============================================================================
// Helpers
// ============================================================================

// resolveFestivalID tries to parse the ID as a number first, then falls back to slug lookup
func (h *FestivalHandler) resolveFestivalID(idOrSlug string) (uint, error) {
	if id, parseErr := strconv.ParseUint(idOrSlug, 10, 32); parseErr == nil {
		return uint(id), nil
	}

	// Fall back to slug lookup
	festival, err := h.festivalService.GetFestivalBySlug(idOrSlug)
	if err != nil {
		var festivalErr *apperrors.FestivalError
		if errors.As(err, &festivalErr) && festivalErr.Code == apperrors.CodeFestivalNotFound {
			return 0, huma.Error404NotFound("Festival not found")
		}
		return 0, huma.Error500InternalServerError("Failed to fetch festival", err)
	}

	return festival.ID, nil
}

// computeFestivalChanges compares old and new festival detail responses and returns field-level diffs.
func computeFestivalChanges(old, new *contracts.FestivalDetailResponse) []adminm.FieldChange {
	var changes []adminm.FieldChange

	if old.Name != new.Name {
		changes = append(changes, adminm.FieldChange{Field: "name", OldValue: old.Name, NewValue: new.Name})
	}
	if old.SeriesSlug != new.SeriesSlug {
		changes = append(changes, adminm.FieldChange{Field: "series_slug", OldValue: old.SeriesSlug, NewValue: new.SeriesSlug})
	}
	if old.EditionYear != new.EditionYear {
		changes = append(changes, adminm.FieldChange{Field: "edition_year", OldValue: old.EditionYear, NewValue: new.EditionYear})
	}
	if ptrToStr(old.Description) != ptrToStr(new.Description) {
		changes = append(changes, adminm.FieldChange{Field: "description", OldValue: ptrToStr(old.Description), NewValue: ptrToStr(new.Description)})
	}
	if ptrToStr(old.LocationName) != ptrToStr(new.LocationName) {
		changes = append(changes, adminm.FieldChange{Field: "location_name", OldValue: ptrToStr(old.LocationName), NewValue: ptrToStr(new.LocationName)})
	}
	if ptrToStr(old.City) != ptrToStr(new.City) {
		changes = append(changes, adminm.FieldChange{Field: "city", OldValue: ptrToStr(old.City), NewValue: ptrToStr(new.City)})
	}
	if ptrToStr(old.State) != ptrToStr(new.State) {
		changes = append(changes, adminm.FieldChange{Field: "state", OldValue: ptrToStr(old.State), NewValue: ptrToStr(new.State)})
	}
	if ptrToStr(old.Country) != ptrToStr(new.Country) {
		changes = append(changes, adminm.FieldChange{Field: "country", OldValue: ptrToStr(old.Country), NewValue: ptrToStr(new.Country)})
	}
	if old.StartDate != new.StartDate {
		changes = append(changes, adminm.FieldChange{Field: "start_date", OldValue: old.StartDate, NewValue: new.StartDate})
	}
	if old.EndDate != new.EndDate {
		changes = append(changes, adminm.FieldChange{Field: "end_date", OldValue: old.EndDate, NewValue: new.EndDate})
	}
	if ptrToStr(old.Website) != ptrToStr(new.Website) {
		changes = append(changes, adminm.FieldChange{Field: "website", OldValue: ptrToStr(old.Website), NewValue: ptrToStr(new.Website)})
	}
	if ptrToStr(old.TicketURL) != ptrToStr(new.TicketURL) {
		changes = append(changes, adminm.FieldChange{Field: "ticket_url", OldValue: ptrToStr(old.TicketURL), NewValue: ptrToStr(new.TicketURL)})
	}
	if ptrToStr(old.FlyerURL) != ptrToStr(new.FlyerURL) {
		changes = append(changes, adminm.FieldChange{Field: "flyer_url", OldValue: ptrToStr(old.FlyerURL), NewValue: ptrToStr(new.FlyerURL)})
	}
	if old.Status != new.Status {
		changes = append(changes, adminm.FieldChange{Field: "status", OldValue: old.Status, NewValue: new.Status})
	}

	return changes
}
