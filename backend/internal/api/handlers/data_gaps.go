package handlers

import (
	"context"
	"errors"
	"strconv"

	"github.com/danielgtaylor/huma/v2"

	"psychic-homily-backend/internal/api/middleware"
	apperrors "psychic-homily-backend/internal/errors"
	"psychic-homily-backend/internal/logger"
	"psychic-homily-backend/internal/services/contracts"
)

// DataGap represents a missing field on an entity that a user could fill in.
type DataGap struct {
	Field    string `json:"field" doc:"The field name that is missing"`
	Label    string `json:"label" doc:"Human-readable label for the missing field"`
	Priority int    `json:"priority" doc:"Priority (1=highest) for display ordering"`
}

// DataGapsHandler handles entity data gap detection endpoints.
type DataGapsHandler struct {
	artistService   contracts.ArtistServiceInterface
	venueService    contracts.VenueServiceInterface
	festivalService contracts.FestivalServiceInterface
	releaseService  contracts.ReleaseServiceInterface
	labelService    contracts.LabelServiceInterface
}

// NewDataGapsHandler creates a new data gaps handler.
func NewDataGapsHandler(
	artistService contracts.ArtistServiceInterface,
	venueService contracts.VenueServiceInterface,
	festivalService contracts.FestivalServiceInterface,
	releaseService contracts.ReleaseServiceInterface,
	labelService contracts.LabelServiceInterface,
) *DataGapsHandler {
	return &DataGapsHandler{
		artistService:   artistService,
		venueService:    venueService,
		festivalService: festivalService,
		releaseService:  releaseService,
		labelService:    labelService,
	}
}

// GetDataGapsRequest is the Huma request for GET /entities/{entity_type}/{id_or_slug}/data-gaps
type GetDataGapsRequest struct {
	EntityType string `path:"entity_type" doc:"Entity type: artist, venue, festival, release, or label" example:"artist"`
	IDOrSlug   string `path:"id_or_slug" doc:"Entity ID or slug" example:"the-national"`
}

// GetDataGapsResponse is the Huma response for GET /entities/{entity_type}/{id_or_slug}/data-gaps
type GetDataGapsResponse struct {
	Body struct {
		Gaps []DataGap `json:"gaps" doc:"List of missing data fields sorted by priority"`
	}
}

// GetDataGapsHandler handles GET /entities/{entity_type}/{id_or_slug}/data-gaps
func (h *DataGapsHandler) GetDataGapsHandler(ctx context.Context, req *GetDataGapsRequest) (*GetDataGapsResponse, error) {
	// Require authentication
	user := middleware.GetUserFromContext(ctx)
	if user == nil {
		return nil, huma.Error401Unauthorized("Authentication required")
	}

	var gaps []DataGap
	var err error

	switch req.EntityType {
	case "artist":
		gaps, err = h.getArtistGaps(req.IDOrSlug)
	case "venue":
		gaps, err = h.getVenueGaps(req.IDOrSlug)
	case "festival":
		gaps, err = h.getFestivalGaps(req.IDOrSlug)
	case "release":
		gaps, err = h.getReleaseGaps(req.IDOrSlug)
	case "label":
		gaps, err = h.getLabelGaps(req.IDOrSlug)
	default:
		return nil, huma.Error400BadRequest("Invalid entity type: must be artist, venue, festival, release, or label")
	}

	if err != nil {
		// Check for not-found errors
		var artistErr *apperrors.ArtistError
		var venueErr *apperrors.VenueError
		var festivalErr *apperrors.FestivalError
		var releaseErr *apperrors.ReleaseError
		var labelErr *apperrors.LabelError
		if errors.As(err, &artistErr) && artistErr.Code == apperrors.CodeArtistNotFound {
			return nil, huma.Error404NotFound("Artist not found")
		}
		if errors.As(err, &venueErr) && venueErr.Code == apperrors.CodeVenueNotFound {
			return nil, huma.Error404NotFound("Venue not found")
		}
		if errors.As(err, &festivalErr) && festivalErr.Code == apperrors.CodeFestivalNotFound {
			return nil, huma.Error404NotFound("Festival not found")
		}
		if errors.As(err, &releaseErr) && releaseErr.Code == apperrors.CodeReleaseNotFound {
			return nil, huma.Error404NotFound("Release not found")
		}
		if errors.As(err, &labelErr) && labelErr.Code == apperrors.CodeLabelNotFound {
			return nil, huma.Error404NotFound("Label not found")
		}
		logger.FromContext(ctx).Error("data_gaps_fetch_failed",
			"entity_type", req.EntityType,
			"id_or_slug", req.IDOrSlug,
			"error", err.Error(),
		)
		return nil, huma.Error500InternalServerError("Failed to fetch entity data gaps")
	}

	resp := &GetDataGapsResponse{}
	resp.Body.Gaps = gaps
	return resp, nil
}

// getArtistGaps fetches an artist and returns missing fields as data gaps.
func (h *DataGapsHandler) getArtistGaps(idOrSlug string) ([]DataGap, error) {
	var artist *contracts.ArtistDetailResponse
	var err error

	if id, parseErr := strconv.ParseUint(idOrSlug, 10, 32); parseErr == nil {
		artist, err = h.artistService.GetArtist(uint(id))
	} else {
		artist, err = h.artistService.GetArtistBySlug(idOrSlug)
	}
	if err != nil {
		return nil, err
	}

	var gaps []DataGap

	// Check social/URL fields (highest priority — most impactful for users)
	if isEmptyPtr(artist.Social.Bandcamp) {
		gaps = append(gaps, DataGap{Field: "bandcamp", Label: "Bandcamp URL", Priority: 1})
	}
	if isEmptyPtr(artist.Social.Spotify) {
		gaps = append(gaps, DataGap{Field: "spotify", Label: "Spotify URL", Priority: 2})
	}
	if isEmptyPtr(artist.Social.Website) {
		gaps = append(gaps, DataGap{Field: "website", Label: "Website", Priority: 3})
	}
	if isEmptyPtr(artist.Social.Instagram) {
		gaps = append(gaps, DataGap{Field: "instagram", Label: "Instagram", Priority: 4})
	}

	// Location info
	if isEmptyPtr(artist.City) {
		gaps = append(gaps, DataGap{Field: "city", Label: "City", Priority: 5})
	}
	if isEmptyPtr(artist.State) {
		gaps = append(gaps, DataGap{Field: "state", Label: "State", Priority: 6})
	}

	// Description
	if isEmptyPtr(artist.Description) {
		gaps = append(gaps, DataGap{Field: "description", Label: "Description", Priority: 7})
	}

	return gaps, nil
}

// getVenueGaps fetches a venue and returns missing fields as data gaps.
func (h *DataGapsHandler) getVenueGaps(idOrSlug string) ([]DataGap, error) {
	var venue *contracts.VenueDetailResponse
	var err error

	if id, parseErr := strconv.ParseUint(idOrSlug, 10, 32); parseErr == nil {
		venue, err = h.venueService.GetVenue(uint(id))
	} else {
		venue, err = h.venueService.GetVenueBySlug(idOrSlug)
	}
	if err != nil {
		return nil, err
	}

	var gaps []DataGap

	if isEmptyPtr(venue.Social.Website) {
		gaps = append(gaps, DataGap{Field: "website", Label: "Website", Priority: 1})
	}
	if isEmptyPtr(venue.Social.Instagram) {
		gaps = append(gaps, DataGap{Field: "instagram", Label: "Instagram", Priority: 2})
	}
	if isEmptyPtr(venue.Description) {
		gaps = append(gaps, DataGap{Field: "description", Label: "Description", Priority: 3})
	}

	return gaps, nil
}

// getFestivalGaps fetches a festival and returns missing fields as data gaps.
func (h *DataGapsHandler) getFestivalGaps(idOrSlug string) ([]DataGap, error) {
	var festival *contracts.FestivalDetailResponse
	var err error

	if id, parseErr := strconv.ParseUint(idOrSlug, 10, 32); parseErr == nil {
		festival, err = h.festivalService.GetFestival(uint(id))
	} else {
		festival, err = h.festivalService.GetFestivalBySlug(idOrSlug)
	}
	if err != nil {
		return nil, err
	}

	var gaps []DataGap

	if isEmptyPtr(festival.Website) {
		gaps = append(gaps, DataGap{Field: "website", Label: "Website", Priority: 1})
	}
	if isEmptyPtr(festival.FlyerURL) {
		gaps = append(gaps, DataGap{Field: "flyer_url", Label: "Flyer", Priority: 2})
	}
	if isEmptyPtr(festival.Description) {
		gaps = append(gaps, DataGap{Field: "description", Label: "Description", Priority: 3})
	}

	return gaps, nil
}

// getReleaseGaps fetches a release and returns missing fields as data gaps.
func (h *DataGapsHandler) getReleaseGaps(idOrSlug string) ([]DataGap, error) {
	var release *contracts.ReleaseDetailResponse
	var err error

	if id, parseErr := strconv.ParseUint(idOrSlug, 10, 32); parseErr == nil {
		release, err = h.releaseService.GetRelease(uint(id))
	} else {
		release, err = h.releaseService.GetReleaseBySlug(idOrSlug)
	}
	if err != nil {
		return nil, err
	}

	var gaps []DataGap

	// Cover art — most visible gap
	if isEmptyPtr(release.CoverArtURL) {
		gaps = append(gaps, DataGap{Field: "cover_art_url", Label: "Cover Art", Priority: 1})
	}
	if release.ReleaseYear == nil {
		gaps = append(gaps, DataGap{Field: "release_year", Label: "Release Year", Priority: 2})
	}
	if isEmptyPtr(release.ReleaseDate) {
		gaps = append(gaps, DataGap{Field: "release_date", Label: "Release Date", Priority: 3})
	}
	if isEmptyPtr(release.Description) {
		gaps = append(gaps, DataGap{Field: "description", Label: "Description", Priority: 4})
	}

	return gaps, nil
}

// getLabelGaps fetches a label and returns missing fields as data gaps.
func (h *DataGapsHandler) getLabelGaps(idOrSlug string) ([]DataGap, error) {
	var label *contracts.LabelDetailResponse
	var err error

	if id, parseErr := strconv.ParseUint(idOrSlug, 10, 32); parseErr == nil {
		label, err = h.labelService.GetLabel(uint(id))
	} else {
		label, err = h.labelService.GetLabelBySlug(idOrSlug)
	}
	if err != nil {
		return nil, err
	}

	var gaps []DataGap

	if isEmptyPtr(label.Social.Website) {
		gaps = append(gaps, DataGap{Field: "website", Label: "Website", Priority: 1})
	}
	if isEmptyPtr(label.Social.Bandcamp) {
		gaps = append(gaps, DataGap{Field: "bandcamp", Label: "Bandcamp URL", Priority: 2})
	}
	if isEmptyPtr(label.Social.Instagram) {
		gaps = append(gaps, DataGap{Field: "instagram", Label: "Instagram", Priority: 3})
	}
	if isEmptyPtr(label.Description) {
		gaps = append(gaps, DataGap{Field: "description", Label: "Description", Priority: 4})
	}
	if label.FoundedYear == nil {
		gaps = append(gaps, DataGap{Field: "founded_year", Label: "Founded Year", Priority: 5})
	}

	return gaps, nil
}

// isEmptyPtr returns true if a string pointer is nil or points to an empty string.
func isEmptyPtr(s *string) bool {
	return s == nil || *s == ""
}
