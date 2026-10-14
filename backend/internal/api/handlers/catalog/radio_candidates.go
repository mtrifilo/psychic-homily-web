package catalog

import (
	"context"

	"github.com/danielgtaylor/huma/v2"

	"psychic-homily-backend/internal/services/contracts"
)

// RadioArtistCandidatesLister is the read the artist candidates endpoint needs.
type RadioArtistCandidatesLister interface {
	ListArtistCandidates(ctx context.Context, q contracts.RadioArtistCandidatesQuery) (*contracts.RadioArtistCandidateListing, error)
}

// RadioCandidatesHandler serves the internal enrichment candidate rankings.
type RadioCandidatesHandler struct {
	lister RadioArtistCandidatesLister
}

// NewRadioCandidatesHandler creates a RadioCandidatesHandler.
func NewRadioCandidatesHandler(lister RadioArtistCandidatesLister) *RadioCandidatesHandler {
	return &RadioCandidatesHandler{lister: lister}
}

// AdminListRadioArtistCandidatesRequest selects the lanes, the order and the
// length of the ranking.
type AdminListRadioArtistCandidatesRequest struct {
	Limit int    `query:"limit" required:"false" default:"50" minimum:"1" maximum:"500" doc:"Candidates to return (default 50)"`
	Lane  string `query:"lane" required:"false" default:"all" enum:"auto,human,all" doc:"Return the auto lane, the human lane, or both (all, default)"`
	Sort  string `query:"sort" required:"false" default:"score" enum:"score,plays,stations" doc:"Rank by score (default), plays in the window, or distinct station rows, descending; ties by score, then the most recent play, then artist key."`
}

// AdminListRadioArtistCandidatesResponse is the ranking.
type AdminListRadioArtistCandidatesResponse struct {
	Body *contracts.RadioArtistCandidateListing
}

// AdminListRadioArtistCandidatesHandler handles GET /admin/radio/candidates/artists.
func (h *RadioCandidatesHandler) AdminListRadioArtistCandidatesHandler(ctx context.Context, req *AdminListRadioArtistCandidatesRequest) (*AdminListRadioArtistCandidatesResponse, error) {
	q := contracts.RadioArtistCandidatesQuery{
		Lane:  contracts.RadioCandidateLaneFilter(req.Lane),
		Sort:  contracts.RadioArtistCandidateSort(req.Sort),
		Limit: req.Limit,
	}
	if q.Lane == "" {
		q.Lane = contracts.RadioCandidateLaneFilterAll
	}
	if q.Sort == "" {
		q.Sort = contracts.RadioArtistCandidateSortScore
	}
	if q.Limit <= 0 {
		q.Limit = 50
	}
	listing, err := h.lister.ListArtistCandidates(ctx, q)
	if err != nil {
		return nil, huma.Error500InternalServerError("Failed to list radio artist candidates", err)
	}
	return &AdminListRadioArtistCandidatesResponse{Body: listing}, nil
}
