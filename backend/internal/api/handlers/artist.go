package handlers

import (
	"context"

	"github.com/danielgtaylor/huma/v2"

	"psychic-homily-backend/internal/services"
)

type ArtistHandler struct {
	artistService *services.ArtistService
}

func NewArtistHandler() *ArtistHandler {
	return &ArtistHandler{
		artistService: services.NewArtistService(),
	}
}

// SearchArtistsRequest represents the autocomplete search request
type SearchArtistsRequest struct {
	Query string `query:"q" doc:"Search query for artist autocomplete" example:"radio"`
}

// SearchArtistsResponse represents the autocomplete search response
type SearchArtistsResponse struct {
	Body struct {
		Artists []*services.ArtistDetailResponse `json:"artists" doc:"Matching artists"`
		Count   int                              `json:"count" doc:"Number of results"`
	}
}

// SearchArtistsHandler handles GET /artists/search?q=query
func (h *ArtistHandler) SearchArtistsHandler(ctx context.Context, req *SearchArtistsRequest) (*SearchArtistsResponse, error) {
	artists, err := h.artistService.SearchArtists(req.Query)
	if err != nil {
		return nil, err
	}

	resp := &SearchArtistsResponse{}
	resp.Body.Artists = artists
	resp.Body.Count = len(artists)

	return resp, nil
}

// GetArtistRequest represents the request for getting a single artist
type GetArtistRequest struct {
	ArtistID uint `path:"artist_id" doc:"Artist ID" example:"1"`
}

// GetArtistResponse represents the response for the get artist endpoint
type GetArtistResponse struct {
	Body *services.ArtistDetailResponse
}

// GetArtistHandler handles GET /artists/{artist_id} - returns a single artist by ID
func (h *ArtistHandler) GetArtistHandler(ctx context.Context, req *GetArtistRequest) (*GetArtistResponse, error) {
	artist, err := h.artistService.GetArtist(req.ArtistID)
	if err != nil {
		if err.Error() == "artist not found" {
			return nil, huma.Error404NotFound("Artist not found")
		}
		return nil, huma.Error500InternalServerError("Failed to fetch artist", err)
	}

	return &GetArtistResponse{Body: artist}, nil
}

// GetArtistShowsRequest represents the request for getting shows for an artist
type GetArtistShowsRequest struct {
	ArtistID   uint   `path:"artist_id" doc:"Artist ID" example:"1"`
	Timezone   string `query:"timezone" doc:"Timezone for date filtering" example:"America/Phoenix"`
	Limit      int    `query:"limit" default:"20" minimum:"1" maximum:"50" doc:"Maximum number of shows to return"`
	TimeFilter string `query:"time_filter" doc:"Filter shows by time: upcoming, past, or all" example:"upcoming" enum:"upcoming,past,all"`
}

// GetArtistShowsResponse represents the response for the artist shows endpoint
type GetArtistShowsResponse struct {
	Body struct {
		Shows    []*services.ArtistShowResponse `json:"shows" doc:"List of shows"`
		ArtistID uint                           `json:"artist_id" doc:"Artist ID"`
		Total    int64                          `json:"total" doc:"Total number of shows matching filter"`
	}
}

// GetArtistShowsHandler handles GET /artists/{artist_id}/shows - returns shows for an artist
func (h *ArtistHandler) GetArtistShowsHandler(ctx context.Context, req *GetArtistShowsRequest) (*GetArtistShowsResponse, error) {
	limit := req.Limit
	if limit == 0 {
		limit = 20
	}

	timezone := req.Timezone
	if timezone == "" {
		timezone = "UTC"
	}

	timeFilter := req.TimeFilter
	if timeFilter == "" {
		timeFilter = "upcoming"
	}

	shows, total, err := h.artistService.GetShowsForArtist(req.ArtistID, timezone, limit, timeFilter)
	if err != nil {
		if err.Error() == "artist not found" {
			return nil, huma.Error404NotFound("Artist not found")
		}
		return nil, huma.Error500InternalServerError("Failed to fetch shows", err)
	}

	resp := &GetArtistShowsResponse{}
	resp.Body.Shows = shows
	resp.Body.ArtistID = req.ArtistID
	resp.Body.Total = total

	return resp, nil
}
