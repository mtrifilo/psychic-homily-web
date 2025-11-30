package handlers

import (
	"context"
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
