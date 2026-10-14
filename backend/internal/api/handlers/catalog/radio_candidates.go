package catalog

import (
	"context"

	"github.com/danielgtaylor/huma/v2"

	"psychic-homily-backend/internal/services/contracts"
)

// RadioCandidatesLister is the read the candidate endpoints need.
type RadioCandidatesLister interface {
	ListArtistCandidates(ctx context.Context, q contracts.RadioArtistCandidatesQuery) (*contracts.RadioArtistCandidateListing, error)
	ListTrackCandidates(ctx context.Context, q contracts.RadioTrackCandidatesQuery) (*contracts.RadioTrackCandidateListing, error)
}

// RadioCandidatesHandler serves the internal enrichment candidate rankings.
type RadioCandidatesHandler struct {
	lister RadioCandidatesLister
}

// NewRadioCandidatesHandler creates a RadioCandidatesHandler.
func NewRadioCandidatesHandler(lister RadioCandidatesLister) *RadioCandidatesHandler {
	return &RadioCandidatesHandler{lister: lister}
}

// radioCandidateDefaultLimit is the listing length when the request names none.
const radioCandidateDefaultLimit = 50

// radioCandidateListParams returns the lane filter, sort and limit a request
// names, with the defaults (all lanes, score, radioCandidateDefaultLimit) for
// the ones it leaves empty or zero.
func radioCandidateListParams(lane, sort string, limit int) (contracts.RadioCandidateLaneFilter, contracts.RadioCandidateSort, int) {
	if lane == "" {
		lane = string(contracts.RadioCandidateLaneFilterAll)
	}
	if sort == "" {
		sort = string(contracts.RadioCandidateSortScore)
	}
	if limit <= 0 {
		limit = radioCandidateDefaultLimit
	}
	return contracts.RadioCandidateLaneFilter(lane), contracts.RadioCandidateSort(sort), limit
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
	var q contracts.RadioArtistCandidatesQuery
	q.Lane, q.Sort, q.Limit = radioCandidateListParams(req.Lane, req.Sort, req.Limit)
	listing, err := h.lister.ListArtistCandidates(ctx, q)
	if err != nil {
		return nil, huma.Error500InternalServerError("Failed to list radio artist candidates", err)
	}
	return &AdminListRadioArtistCandidatesResponse{Body: listing}, nil
}

// AdminListRadioTrackCandidatesRequest selects the view, the lanes, the order
// and the length of the ranking. Its query fields mirror
// AdminListRadioArtistCandidatesRequest's; Huma reads query tags only from a
// request struct's own fields, so they are not shared by embedding.
type AdminListRadioTrackCandidatesRequest struct {
	View  string `query:"view" required:"true" enum:"unlinked,unmatched" doc:"unlinked: tracks of catalog artists with no release link; unmatched: tracks of artists the catalog does not have, each of whose artist keys is an artist candidate"`
	Limit int    `query:"limit" required:"false" default:"50" minimum:"1" maximum:"500" doc:"Candidates to return (default 50)"`
	Lane  string `query:"lane" required:"false" default:"all" enum:"auto,human,all" doc:"Return the auto lane, the human lane, or both (all, default)"`
	Sort  string `query:"sort" required:"false" default:"score" enum:"score,plays,stations" doc:"Rank by score (default), plays in the window, or distinct station rows, descending; ties by score, then the most recent play, then track id."`
}

// AdminListRadioTrackCandidatesResponse is the ranking.
type AdminListRadioTrackCandidatesResponse struct {
	Body *contracts.RadioTrackCandidateListing
}

// AdminListRadioTrackCandidatesHandler handles GET /admin/radio/candidates/tracks.
func (h *RadioCandidatesHandler) AdminListRadioTrackCandidatesHandler(ctx context.Context, req *AdminListRadioTrackCandidatesRequest) (*AdminListRadioTrackCandidatesResponse, error) {
	q := contracts.RadioTrackCandidatesQuery{View: contracts.RadioTrackCandidateView(req.View)}
	q.Lane, q.Sort, q.Limit = radioCandidateListParams(req.Lane, req.Sort, req.Limit)
	listing, err := h.lister.ListTrackCandidates(ctx, q)
	if err != nil {
		return nil, huma.Error500InternalServerError("Failed to list radio track candidates", err)
	}
	return &AdminListRadioTrackCandidatesResponse{Body: listing}, nil
}
