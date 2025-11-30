package handlers

import (
	"context"
	"psychic-homily-backend/internal/services"
)

type VenueHandler struct {
	venueService *services.VenueService
}

func NewVenueHandler() *VenueHandler {
	return &VenueHandler{
		venueService: services.NewVenueService(),
	}
}

type SearchVenuesRequest struct {
	Query string `query:"q" doc:"Search query for venue autocomplete" example:"empty bottle"`
}

type SearchVenuesResponse struct {
	Body struct {
		Venues []*services.VenueDetailResponse `json:"venues" doc:"Matching venues"`
		Count  int                             `json:"count" doc:"Number of results"`
	}
}

func (h *VenueHandler) SearchVenuesHandler(ctx context.Context, req *SearchVenuesRequest) (*SearchVenuesResponse, error) {
	venues, err := h.venueService.SearchVenues(req.Query)
	if err != nil {
		return nil, err
	}

	resp := &SearchVenuesResponse{}
	resp.Body.Venues = venues
	resp.Body.Count = len(venues)

	return resp, nil
}
