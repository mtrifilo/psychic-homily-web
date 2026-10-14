package catalog

import (
	"context"

	"github.com/danielgtaylor/huma/v2"

	"psychic-homily-backend/internal/services/contracts"
)

// RadioLabelCandidatesLister is the read the label candidates endpoint needs.
type RadioLabelCandidatesLister interface {
	ListLabelCandidates(ctx context.Context, q contracts.RadioLabelCandidatesQuery) (*contracts.RadioLabelCandidateListing, error)
}

// RadioLabelCandidatesHandler serves the internal label candidate ranking.
type RadioLabelCandidatesHandler struct {
	lister RadioLabelCandidatesLister
}

// NewRadioLabelCandidatesHandler creates a RadioLabelCandidatesHandler.
func NewRadioLabelCandidatesHandler(lister RadioLabelCandidatesLister) *RadioLabelCandidatesHandler {
	return &RadioLabelCandidatesHandler{lister: lister}
}

// AdminListRadioLabelCandidatesRequest selects the lanes, the order and the
// length of the ranking.
type AdminListRadioLabelCandidatesRequest struct {
	Limit int    `query:"limit" required:"false" default:"50" minimum:"1" maximum:"500" doc:"Candidates to return (default 50)"`
	Lane  string `query:"lane" required:"false" default:"all" enum:"auto,human,all" doc:"Return the auto lane, the human lane, or both (all, default). The auto lane needs exactly one MusicBrainz label id across a candidate's plays; plays carry no MusicBrainz label id today, so the auto lane is empty."`
	Sort  string `query:"sort" required:"false" default:"score" enum:"score,plays,stations,velocity" doc:"Rank by score (default), plays in the window, distinct station rows, or velocity, descending; ties by score, then the most recent play, then label key."`
}

// AdminListRadioLabelCandidatesResponse is the ranking.
type AdminListRadioLabelCandidatesResponse struct {
	Body *contracts.RadioLabelCandidateListing
}

// AdminListRadioLabelCandidatesHandler handles GET /admin/radio/candidates/labels.
func (h *RadioLabelCandidatesHandler) AdminListRadioLabelCandidatesHandler(ctx context.Context, req *AdminListRadioLabelCandidatesRequest) (*AdminListRadioLabelCandidatesResponse, error) {
	q := contracts.RadioLabelCandidatesQuery{
		Lane:  contracts.RadioCandidateLaneFilter(req.Lane),
		Sort:  contracts.RadioLabelCandidateSort(req.Sort),
		Limit: req.Limit,
	}
	if q.Lane == "" {
		q.Lane = contracts.RadioCandidateLaneFilterAll
	}
	if q.Sort == "" {
		q.Sort = contracts.RadioLabelCandidateSortScore
	}
	if q.Limit <= 0 {
		q.Limit = 50
	}
	listing, err := h.lister.ListLabelCandidates(ctx, q)
	if err != nil {
		return nil, huma.Error500InternalServerError("Failed to list radio label candidates", err)
	}
	return &AdminListRadioLabelCandidatesResponse{Body: listing}, nil
}
