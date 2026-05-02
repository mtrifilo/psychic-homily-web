package catalog

import (
	"context"
	"strconv"
	"strings"

	"github.com/danielgtaylor/huma/v2"

	"psychic-homily-backend/internal/services/contracts"
)

// FestivalIntelligenceHandler handles festival intelligence endpoints.
type FestivalIntelligenceHandler struct {
	intelligenceService contracts.FestivalIntelligenceServiceInterface
	festivalService     contracts.FestivalServiceInterface
	artistService       contracts.ArtistServiceInterface
}

// NewFestivalIntelligenceHandler creates a new festival intelligence handler.
func NewFestivalIntelligenceHandler(
	intelligenceService contracts.FestivalIntelligenceServiceInterface,
	festivalService contracts.FestivalServiceInterface,
	artistService contracts.ArtistServiceInterface,
) *FestivalIntelligenceHandler {
	return &FestivalIntelligenceHandler{
		intelligenceService: intelligenceService,
		festivalService:     festivalService,
		artistService:       artistService,
	}
}

// ============================================================================
// Similar Festivals
// ============================================================================

type GetSimilarFestivalsRequest struct {
	FestivalID string `path:"festival_id" doc:"Festival ID or slug" example:"m3f-2026"`
	Limit      int    `query:"limit" required:"false" doc:"Maximum number of similar festivals to return" example:"10"`
}

type GetSimilarFestivalsResponse struct {
	Body struct {
		Similar []contracts.SimilarFestival `json:"similar" doc:"List of similar festivals ranked by overlap"`
	}
}

func (h *FestivalIntelligenceHandler) GetSimilarFestivalsHandler(ctx context.Context, req *GetSimilarFestivalsRequest) (*GetSimilarFestivalsResponse, error) {
	festivalID, err := h.resolveFestivalID(req.FestivalID)
	if err != nil {
		return nil, err
	}

	limit := req.Limit
	if limit <= 0 {
		limit = 10
	}

	similar, err := h.intelligenceService.GetSimilarFestivals(festivalID, limit)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return nil, huma.Error404NotFound("Festival not found")
		}
		return nil, huma.Error500InternalServerError("Failed to compute similar festivals", err)
	}

	resp := &GetSimilarFestivalsResponse{}
	resp.Body.Similar = similar
	return resp, nil
}

// ============================================================================
// Festival Overlap
// ============================================================================

type GetFestivalOverlapRequest struct {
	FestivalAID string `path:"festival_a_id" doc:"First festival ID or slug" example:"m3f-2026"`
	FestivalBID string `path:"festival_b_id" doc:"Second festival ID or slug" example:"levitation-2026"`
}

type GetFestivalOverlapResponse struct {
	Body *contracts.FestivalOverlap
}

func (h *FestivalIntelligenceHandler) GetFestivalOverlapHandler(ctx context.Context, req *GetFestivalOverlapRequest) (*GetFestivalOverlapResponse, error) {
	festivalAID, err := h.resolveFestivalID(req.FestivalAID)
	if err != nil {
		return nil, err
	}

	festivalBID, err := h.resolveFestivalID(req.FestivalBID)
	if err != nil {
		return nil, err
	}

	overlap, err := h.intelligenceService.GetFestivalOverlap(festivalAID, festivalBID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return nil, huma.Error404NotFound("Festival not found")
		}
		return nil, huma.Error500InternalServerError("Failed to compute festival overlap", err)
	}

	return &GetFestivalOverlapResponse{Body: overlap}, nil
}

// ============================================================================
// Festival Breakouts
// ============================================================================

type GetFestivalBreakoutsRequest struct {
	FestivalID string `path:"festival_id" doc:"Festival ID or slug" example:"m3f-2026"`
}

type GetFestivalBreakoutsResponse struct {
	Body *contracts.FestivalBreakouts
}

func (h *FestivalIntelligenceHandler) GetFestivalBreakoutsHandler(ctx context.Context, req *GetFestivalBreakoutsRequest) (*GetFestivalBreakoutsResponse, error) {
	festivalID, err := h.resolveFestivalID(req.FestivalID)
	if err != nil {
		return nil, err
	}

	breakouts, err := h.intelligenceService.GetFestivalBreakouts(festivalID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return nil, huma.Error404NotFound("Festival not found")
		}
		return nil, huma.Error500InternalServerError("Failed to compute breakouts", err)
	}

	return &GetFestivalBreakoutsResponse{Body: breakouts}, nil
}

// ============================================================================
// Artist Festival Trajectory
// ============================================================================

type GetArtistFestivalTrajectoryRequest struct {
	ArtistID string `path:"artist_id" doc:"Artist ID or slug" example:"frozen-soul"`
}

type GetArtistFestivalTrajectoryResponse struct {
	Body *contracts.ArtistTrajectory
}

func (h *FestivalIntelligenceHandler) GetArtistFestivalTrajectoryHandler(ctx context.Context, req *GetArtistFestivalTrajectoryRequest) (*GetArtistFestivalTrajectoryResponse, error) {
	artistID, err := h.resolveArtistID(req.ArtistID)
	if err != nil {
		return nil, err
	}

	trajectory, err := h.intelligenceService.GetArtistFestivalTrajectory(artistID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return nil, huma.Error404NotFound("Artist not found")
		}
		return nil, huma.Error500InternalServerError("Failed to compute trajectory", err)
	}

	return &GetArtistFestivalTrajectoryResponse{Body: trajectory}, nil
}

// ============================================================================
// Series Comparison
// ============================================================================

type GetSeriesComparisonRequest struct {
	SeriesSlug string `path:"series_slug" doc:"Festival series slug" example:"m3f"`
	Years      string `query:"years" doc:"Comma-separated list of years to compare" example:"2024,2025,2026"`
}

type GetSeriesComparisonResponse struct {
	Body *contracts.SeriesComparison
}

func (h *FestivalIntelligenceHandler) GetSeriesComparisonHandler(ctx context.Context, req *GetSeriesComparisonRequest) (*GetSeriesComparisonResponse, error) {
	if req.SeriesSlug == "" {
		return nil, huma.Error400BadRequest("Series slug is required")
	}

	if req.Years == "" {
		return nil, huma.Error400BadRequest("Years parameter is required (comma-separated)")
	}

	yearStrs := strings.Split(req.Years, ",")
	var years []int
	for _, ys := range yearStrs {
		ys = strings.TrimSpace(ys)
		y, err := strconv.Atoi(ys)
		if err != nil {
			return nil, huma.Error400BadRequest("Invalid year: " + ys)
		}
		years = append(years, y)
	}

	if len(years) < 2 {
		return nil, huma.Error400BadRequest("At least 2 years required for comparison")
	}

	comparison, err := h.intelligenceService.GetSeriesComparison(req.SeriesSlug, years)
	if err != nil {
		if strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "no festivals found") {
			return nil, huma.Error404NotFound(err.Error())
		}
		if strings.Contains(err.Error(), "at least 2 years") {
			return nil, huma.Error400BadRequest(err.Error())
		}
		return nil, huma.Error500InternalServerError("Failed to compute series comparison", err)
	}

	return &GetSeriesComparisonResponse{Body: comparison}, nil
}

// ============================================================================
// Helpers
// ============================================================================

func (h *FestivalIntelligenceHandler) resolveFestivalID(idOrSlug string) (uint, error) {
	if id, parseErr := strconv.ParseUint(idOrSlug, 10, 32); parseErr == nil {
		return uint(id), nil
	}

	festival, err := h.festivalService.GetFestivalBySlug(idOrSlug)
	if err != nil {
		return 0, huma.Error404NotFound("Festival not found")
	}
	return festival.ID, nil
}

func (h *FestivalIntelligenceHandler) resolveArtistID(idOrSlug string) (uint, error) {
	if id, parseErr := strconv.ParseUint(idOrSlug, 10, 32); parseErr == nil {
		return uint(id), nil
	}

	artist, err := h.artistService.GetArtistBySlug(idOrSlug)
	if err != nil {
		return 0, huma.Error404NotFound("Artist not found")
	}
	return artist.ID, nil
}
