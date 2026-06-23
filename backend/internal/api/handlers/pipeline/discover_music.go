package pipeline

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/danielgtaylor/huma/v2"

	apperrors "psychic-homily-backend/internal/errors"
	"psychic-homily-backend/internal/logger"
	"psychic-homily-backend/internal/services/contracts"
)

// DiscoverMusicHandler owns the admin discover-music surface: given an artist,
// it returns candidate Bandcamp/Spotify links discovered via MusicBrainz for the
// admin to review. It is DISCOVERY-ONLY — it persists nothing.
type DiscoverMusicHandler struct {
	discoverService contracts.DiscoverMusicServiceInterface
	artistService   contracts.ArtistServiceInterface
}

// NewDiscoverMusicHandler wires the discovery service and the artist service
// (used to resolve the artist name + confirm the artist exists).
func NewDiscoverMusicHandler(
	discoverService contracts.DiscoverMusicServiceInterface,
	artistService contracts.ArtistServiceInterface,
) *DiscoverMusicHandler {
	return &DiscoverMusicHandler{
		discoverService: discoverService,
		artistService:   artistService,
	}
}

// DiscoverMusicRequest is the Huma request shape for
// POST /admin/artists/{artist_id}/discover-music. The artist is identified by
// numeric ID in the path; there is no request body.
type DiscoverMusicRequest struct {
	ArtistID string `path:"artist_id" validate:"required" doc:"Artist ID"`
}

// DiscoverMusicResponse wraps the service result for OpenAPI.
type DiscoverMusicResponse struct {
	Body contracts.DiscoverMusicResult `json:"body"`
}

// DiscoverMusicHandler handles POST /admin/artists/{artist_id}/discover-music.
// Admin-gated via the rc.Admin middleware (PSY-423); the handler carries no
// inline admin check. Discovery hits MusicBrainz (rate-limited) and probes
// candidate URL liveness, so it can take several seconds.
func (h *DiscoverMusicHandler) DiscoverMusicHandler(ctx context.Context, req *DiscoverMusicRequest) (*DiscoverMusicResponse, error) {
	requestID := logger.GetRequestID(ctx)

	artistID, err := strconv.ParseUint(req.ArtistID, 10, 32)
	if err != nil {
		return nil, huma.Error400BadRequest("Invalid artist ID")
	}

	// Resolve the artist to get its canonical name for the exact-name gate and
	// to return 404 for a non-existent artist before doing any network work.
	artist, err := h.artistService.GetArtist(uint(artistID))
	if err != nil {
		var artistErr *apperrors.ArtistError
		if errors.As(err, &artistErr) && artistErr.Code == apperrors.CodeArtistNotFound {
			return nil, huma.Error404NotFound("Artist not found")
		}
		logger.FromContext(ctx).Error("discover_music_artist_lookup_failed",
			"artist_id", artistID,
			"error", err.Error(),
			"request_id", requestID,
		)
		return nil, huma.Error500InternalServerError(
			fmt.Sprintf("Failed to load artist (request_id: %s)", requestID),
		)
	}

	logger.FromContext(ctx).Info("discover_music_attempt",
		"artist_id", artistID,
		"artist_name", artist.Name,
		"request_id", requestID,
	)

	result, err := h.discoverService.DiscoverMusic(uint(artistID), artist.Name)
	if err != nil {
		logger.FromContext(ctx).Error("discover_music_failed",
			"artist_id", artistID,
			"error", err.Error(),
			"request_id", requestID,
		)
		// MusicBrainz being unreachable/rate-limited is an upstream dependency
		// failure, not a client error — surface 502 so the FE can show a
		// "try again / use manual entry" message.
		return nil, huma.Error502BadGateway(
			fmt.Sprintf("Music discovery is temporarily unavailable (request_id: %s)", requestID),
		)
	}

	logger.FromContext(ctx).Info("discover_music_success",
		"artist_id", artistID,
		"candidate_count", len(result.Candidates),
		"request_id", requestID,
	)

	return &DiscoverMusicResponse{Body: *result}, nil
}
