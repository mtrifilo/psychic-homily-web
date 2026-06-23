package pipeline

import (
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	"psychic-homily-backend/db"
	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/contracts"
)

// LinkSuggestionService owns the admin music-link suggestion review queue
// (PSY-1199). The list query returns pending rows joined to their artist,
// high-confidence first. Accept reuses the EXISTING artist update path
// (artistService.UpdateArtist) to write the link — Spotify sets social.spotify;
// Bandcamp sets social.bandcamp, which triggers the PSY-1190 profile→embed
// resolver inside UpdateArtist — so the write/resolve logic is NOT duplicated
// here. Reject just marks the row. Both stamp reviewer + reviewed_at and are
// idempotent on replay of the same verdict.
type LinkSuggestionService struct {
	db *gorm.DB
	// artistService is the existing artist write path. AcceptSuggestion calls
	// UpdateArtist so the link write (and, for Bandcamp, the profile→embed
	// resolver) goes through the SAME validated, provenance-stamping code the
	// per-artist accept endpoint uses — never a parallel writer.
	artistService contracts.ArtistServiceInterface
	// now is injectable so tests can assert the reviewed_at stamp deterministically.
	now func() time.Time
}

// NewLinkSuggestionService creates the service. A nil database resolves to the
// process default. artistService is the existing artist write path the accept
// flow reuses.
func NewLinkSuggestionService(database *gorm.DB, artistService contracts.ArtistServiceInterface) *LinkSuggestionService {
	if database == nil {
		database = db.GetDB()
	}
	return &LinkSuggestionService{
		db:            database,
		artistService: artistService,
		now:           time.Now,
	}
}

// ListPendingSuggestions returns pending suggestions joined to their artist,
// ordered high-confidence first (then by id ASC for stable pagination).
//
// limit is clamped to [1, 200]; offset is clamped to >= 0.
func (s *LinkSuggestionService) ListPendingSuggestions(limit, offset int) (*contracts.LinkSuggestionListResult, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	if limit < 1 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	if offset < 0 {
		offset = 0
	}

	// 'high' must sort before 'review', which a plain DESC/ASC on the string
	// won't give ('high' < 'review' alphabetically — ASC would already put high
	// first, but make the intent explicit so a future confidence tier doesn't
	// silently reorder). The id tiebreak is deterministic for pagination.
	query := `
		SELECT
			als.id,
			als.artist_id,
			a.name  AS artist_name,
			a.slug  AS artist_slug,
			als.platform,
			als.url,
			als.source,
			als.mb_artist_id,
			als.mb_artist_name,
			als.confidence,
			als.region_match,
			als.live,
			als.notes,
			als.status,
			als.created_at
		FROM artist_link_suggestions als
		JOIN artists a ON a.id = als.artist_id
		WHERE als.status = ?
		ORDER BY (CASE WHEN als.confidence = 'high' THEN 0 ELSE 1 END) ASC, als.id ASC
		LIMIT ? OFFSET ?
	`

	entries := make([]contracts.LinkSuggestionEntry, 0)
	if err := s.db.Raw(query, catalogm.LinkSuggestionStatusPending, limit, offset).Scan(&entries).Error; err != nil {
		return nil, fmt.Errorf("list link suggestions: %w", err)
	}

	var total int64
	if err := s.db.Model(&catalogm.ArtistLinkSuggestion{}).
		Where("status = ?", catalogm.LinkSuggestionStatusPending).
		Count(&total).Error; err != nil {
		return nil, fmt.Errorf("count link suggestions: %w", err)
	}

	return &contracts.LinkSuggestionListResult{
		Suggestions: entries,
		Total:       total,
	}, nil
}

// AcceptSuggestion writes the suggested link to the artist via the existing
// artist update path, then marks the row accepted and stamps the reviewer.
//
// Idempotency + replay safety:
//   - A row already 'accepted' returns the existing stamp WITHOUT re-writing the
//     link (the write is gated on the pending→accepted transition), so a double
//     POST never double-writes.
//   - A row already 'rejected' returns ErrLinkSuggestionAlreadyReviewed (a
//     conflicting verdict — the handler maps it to 409).
//
// The link write reuses artistService.UpdateArtist: Spotify sets the spotify
// social URL; Bandcamp sets the bandcamp social URL, which triggers the PSY-1190
// profile→embed resolver inside UpdateArtist (fill-when-empty). No write/resolve
// logic is duplicated here.
func (s *LinkSuggestionService) AcceptSuggestion(suggestionID, reviewerUserID uint) (*contracts.LinkSuggestionReviewResult, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	if s.artistService == nil {
		return nil, fmt.Errorf("artist service not configured")
	}

	suggestion, err := s.loadSuggestion(suggestionID)
	if err != nil {
		return nil, err
	}

	// Idempotent replay: already accepted → return the stored stamp, no re-write.
	if suggestion.Status == catalogm.LinkSuggestionStatusAccepted {
		return reviewResultFromModel(suggestion), nil
	}
	// Conflicting verdict: already rejected → 409.
	if suggestion.Status == catalogm.LinkSuggestionStatusRejected {
		return nil, contracts.ErrLinkSuggestionAlreadyReviewed
	}

	// Write the link via the existing artist update path. UpdateArtist surfaces
	// ErrArtistNotFound when the artist is gone; the FK ON DELETE CASCADE means a
	// pending suggestion for a deleted artist shouldn't exist, but treat a missing
	// artist as a hard failure rather than silently marking accepted.
	if err := s.applyLink(suggestion); err != nil {
		return nil, fmt.Errorf("apply suggested link: %w", err)
	}

	// Mark accepted + stamp the reviewer. The WHERE re-asserts status='pending'
	// so two concurrent accepts can't both write the link AND both stamp — the
	// second sees RowsAffected==0 and re-reads the already-accepted row below.
	reviewedAt := s.now().UTC()
	res := s.db.Model(&catalogm.ArtistLinkSuggestion{}).
		Where("id = ? AND status = ?", suggestionID, catalogm.LinkSuggestionStatusPending).
		Updates(map[string]interface{}{
			"status":              catalogm.LinkSuggestionStatusAccepted,
			"reviewed_at":         reviewedAt,
			"reviewed_by_user_id": reviewerUserID,
		})
	if res.Error != nil {
		return nil, fmt.Errorf("mark suggestion accepted: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		// Lost the race — another accept already stamped it. Re-read and return.
		reloaded, err := s.loadSuggestion(suggestionID)
		if err != nil {
			return nil, err
		}
		return reviewResultFromModel(reloaded), nil
	}

	return &contracts.LinkSuggestionReviewResult{
		ID:               suggestionID,
		ArtistID:         suggestion.ArtistID,
		Status:           catalogm.LinkSuggestionStatusAccepted,
		ReviewedAt:       &reviewedAt,
		ReviewedByUserID: &reviewerUserID,
	}, nil
}

// RejectSuggestion marks the row rejected and stamps the reviewer.
//
// Idempotency + replay safety:
//   - A row already 'rejected' returns the existing stamp (no-op).
//   - A row already 'accepted' returns ErrLinkSuggestionAlreadyReviewed (409).
func (s *LinkSuggestionService) RejectSuggestion(suggestionID, reviewerUserID uint) (*contracts.LinkSuggestionReviewResult, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	suggestion, err := s.loadSuggestion(suggestionID)
	if err != nil {
		return nil, err
	}

	if suggestion.Status == catalogm.LinkSuggestionStatusRejected {
		return reviewResultFromModel(suggestion), nil
	}
	if suggestion.Status == catalogm.LinkSuggestionStatusAccepted {
		return nil, contracts.ErrLinkSuggestionAlreadyReviewed
	}

	reviewedAt := s.now().UTC()
	res := s.db.Model(&catalogm.ArtistLinkSuggestion{}).
		Where("id = ? AND status = ?", suggestionID, catalogm.LinkSuggestionStatusPending).
		Updates(map[string]interface{}{
			"status":              catalogm.LinkSuggestionStatusRejected,
			"reviewed_at":         reviewedAt,
			"reviewed_by_user_id": reviewerUserID,
		})
	if res.Error != nil {
		return nil, fmt.Errorf("mark suggestion rejected: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		reloaded, err := s.loadSuggestion(suggestionID)
		if err != nil {
			return nil, err
		}
		return reviewResultFromModel(reloaded), nil
	}

	return &contracts.LinkSuggestionReviewResult{
		ID:               suggestionID,
		ArtistID:         suggestion.ArtistID,
		Status:           catalogm.LinkSuggestionStatusRejected,
		ReviewedAt:       &reviewedAt,
		ReviewedByUserID: &reviewerUserID,
	}, nil
}

// applyLink writes the suggestion's URL to the artist via the existing artist
// update path, routing on platform. The artist update path validates the URL
// shape, stamps provenance, and (for Bandcamp) triggers the profile→embed
// resolver — none of which is re-implemented here.
func (s *LinkSuggestionService) applyLink(suggestion *catalogm.ArtistLinkSuggestion) error {
	url := suggestion.URL
	var req contracts.UpdateArtistRequest
	switch suggestion.Platform {
	case contracts.MusicPlatformSpotify:
		req.Spotify = &url
	case contracts.MusicPlatformBandcamp:
		// Setting the bandcamp social URL triggers the PSY-1190 resolver inside
		// UpdateArtist, which fills bandcamp_embed_url from the profile root.
		req.Bandcamp = &url
	default:
		return fmt.Errorf("unsupported suggestion platform %q", suggestion.Platform)
	}

	_, err := s.artistService.UpdateArtist(suggestion.ArtistID, &req)
	return err
}

// loadSuggestion fetches the suggestion row, translating a missing row into the
// typed ErrLinkSuggestionNotFound the handler maps to a 404.
func (s *LinkSuggestionService) loadSuggestion(suggestionID uint) (*catalogm.ArtistLinkSuggestion, error) {
	var suggestion catalogm.ArtistLinkSuggestion
	if err := s.db.First(&suggestion, suggestionID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, contracts.ErrLinkSuggestionNotFound
		}
		return nil, fmt.Errorf("load link suggestion: %w", err)
	}
	return &suggestion, nil
}

// reviewResultFromModel builds the review response from a stored row, used on an
// idempotent replay (the row already carries the stamp).
func reviewResultFromModel(suggestion *catalogm.ArtistLinkSuggestion) *contracts.LinkSuggestionReviewResult {
	return &contracts.LinkSuggestionReviewResult{
		ID:               suggestion.ID,
		ArtistID:         suggestion.ArtistID,
		Status:           suggestion.Status,
		ReviewedAt:       suggestion.ReviewedAt,
		ReviewedByUserID: suggestion.ReviewedByUserID,
	}
}
