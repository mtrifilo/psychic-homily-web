package catalog

import (
	"context"

	"github.com/danielgtaylor/huma/v2"

	"psychic-homily-backend/internal/services/contracts"
)

// RadioTracksLister is the read the internal top-tracks endpoint needs.
type RadioTracksLister interface {
	ListTopRadioTracks(ctx context.Context, sort contracts.RadioTrackSort, limit int) (*contracts.RadioTrackListing, error)
}

// RadioTracksHandler serves the internal radio_tracks listing.
type RadioTracksHandler struct {
	lister RadioTracksLister
}

// NewRadioTracksHandler creates a RadioTracksHandler.
func NewRadioTracksHandler(lister RadioTracksLister) *RadioTracksHandler {
	return &RadioTracksHandler{lister: lister}
}

// AdminListRadioTracksRequest selects the ranking and its length.
type AdminListRadioTracksRequest struct {
	Sort  string `query:"sort" required:"false" default:"plays" enum:"plays,station_rows,station_families" doc:"Rank by play_count (plays, default), station_row_count (station_rows) or station_family_count (station_families), descending; ties by play_count, then id."`
	Limit int    `query:"limit" required:"false" default:"50" minimum:"1" maximum:"500" doc:"Tracks to return (default 50)"`
}

// AdminListRadioTracksResponse is the listing.
type AdminListRadioTracksResponse struct {
	Body *contracts.RadioTrackListing
}

// AdminListRadioTracksHandler handles GET /admin/radio/tracks.
func (h *RadioTracksHandler) AdminListRadioTracksHandler(ctx context.Context, req *AdminListRadioTracksRequest) (*AdminListRadioTracksResponse, error) {
	sort := contracts.RadioTrackSort(req.Sort)
	if sort == "" {
		sort = contracts.RadioTrackSortPlays
	}
	limit := req.Limit
	if limit <= 0 {
		limit = 50
	}
	listing, err := h.lister.ListTopRadioTracks(ctx, sort, limit)
	if err != nil {
		return nil, huma.Error500InternalServerError("Failed to list radio tracks", err)
	}
	return &AdminListRadioTracksResponse{Body: listing}, nil
}
