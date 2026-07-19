package catalog

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"

	"psychic-homily-backend/db"
	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/services/shared"
)

// RadioPlayMatchSuggestionService owns community submit + admin review for
// radio play match suggestions (PSY-1494). Accept reuses RadioService.LinkPlay
// / BulkLinkPlays — never duplicates the update logic. Community never mutates
// radio_plays; only admin accept does (via LinkPlay). Approval email is sent
// by the handler (mirrors edit-approved prefs) so this package does not import
// engagement (avoids catalog↔engagement import cycle).
type RadioPlayMatchSuggestionService struct {
	db           *gorm.DB
	radioService contracts.RadioServiceInterface
	now          func() time.Time
}

// NewRadioPlayMatchSuggestionService wires the service. radioService is required
// for accept (LinkPlay / BulkLinkPlays).
func NewRadioPlayMatchSuggestionService(
	database *gorm.DB,
	radioService contracts.RadioServiceInterface,
) *RadioPlayMatchSuggestionService {
	if database == nil {
		database = db.GetDB()
	}
	return &RadioPlayMatchSuggestionService{
		db:           database,
		radioService: radioService,
		now:          time.Now,
	}
}

// suggestableMatchStates are the match_state values that allow a community
// suggestion when artist_id IS NULL (PSY-1494 / PSY-1052).
var suggestableMatchStates = map[string]bool{
	catalogm.RadioPlayMatchStateUnmatched: true,
	catalogm.RadioPlayMatchStateAmbiguous: true,
	catalogm.RadioPlayMatchStateNoMatch:   true,
}

// CreateSuggestion inserts a pending suggestion without mutating radio_plays.
//
// Resubmit rules (enforced here + by the partial unique index):
//   - One pending suggestion per (submitted_by, play_id). A second pending
//     insert returns ErrRadioPlayMatchSuggestionDuplicatePending (409).
//   - After reject, the user MAY resubmit (rejected rows are outside the
//     partial unique index).
//   - After accept, the play is linked (artist_id set) so CreateSuggestion
//     returns ErrRadioPlayMatchSuggestionPlayNotSuggestable.
func (s *RadioPlayMatchSuggestionService) CreateSuggestion(
	playID, submitterID uint,
	req *contracts.CreateRadioPlayMatchSuggestionRequest,
) (*contracts.RadioPlayMatchSuggestionEntry, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	if req == nil || req.ArtistID == 0 {
		return nil, contracts.ErrRadioPlayMatchSuggestionArtistNotFound
	}

	var play catalogm.RadioPlay
	if err := s.db.First(&play, playID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, contracts.ErrRadioPlayMatchSuggestionPlayNotSuggestable
		}
		return nil, fmt.Errorf("load play: %w", err)
	}
	if play.ArtistID != nil || !suggestableMatchStates[play.MatchState] {
		return nil, contracts.ErrRadioPlayMatchSuggestionPlayNotSuggestable
	}

	var artist catalogm.Artist
	if err := s.db.Select("id", "name", "slug").First(&artist, req.ArtistID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, contracts.ErrRadioPlayMatchSuggestionArtistNotFound
		}
		return nil, fmt.Errorf("load artist: %w", err)
	}

	now := s.now().UTC()
	row := catalogm.RadioPlayMatchSuggestion{
		PlayID:            playID,
		SuggestedArtistID: req.ArtistID,
		SubmittedBy:       submitterID,
		Note:              trimNote(req.Note),
		Status:            catalogm.RadioPlayMatchSuggestionStatusPending,
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	if err := s.db.Create(&row).Error; err != nil {
		if shared.IsDuplicateKey(err) {
			return nil, contracts.ErrRadioPlayMatchSuggestionDuplicatePending
		}
		return nil, fmt.Errorf("insert match suggestion: %w", err)
	}

	return s.entryFromParts(&row, &play, &artist, nil), nil
}

// GetOwnPendingSuggestion returns the caller's pending suggestion for a play,
// or (nil, nil) when none exists.
func (s *RadioPlayMatchSuggestionService) GetOwnPendingSuggestion(
	playID, submitterID uint,
) (*contracts.RadioPlayMatchSuggestionEntry, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	var row catalogm.RadioPlayMatchSuggestion
	err := s.db.Where(
		"play_id = ? AND submitted_by = ? AND status = ?",
		playID, submitterID, catalogm.RadioPlayMatchSuggestionStatusPending,
	).First(&row).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("load own pending suggestion: %w", err)
	}
	return s.loadEntry(row.ID)
}

// ListPendingSuggestions returns pending suggestions oldest-first for the
// admin review queue.
func (s *RadioPlayMatchSuggestionService) ListPendingSuggestions(
	limit, offset int,
) (*contracts.RadioPlayMatchSuggestionListResult, error) {
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

	query := `
		SELECT
			s.id,
			s.play_id,
			rp.artist_name AS play_artist_name,
			rp.match_state AS play_match_state,
			s.suggested_artist_id,
			a.name AS suggested_artist_name,
			a.slug AS suggested_artist_slug,
			s.submitted_by,
			u.username AS submitter_username,
			s.note,
			s.status,
			s.reviewed_by,
			s.reviewed_at,
			s.rejection_reason,
			s.created_at
		FROM radio_play_match_suggestions s
		JOIN radio_plays rp ON rp.id = s.play_id
		JOIN artists a ON a.id = s.suggested_artist_id
		LEFT JOIN users u ON u.id = s.submitted_by
		WHERE s.status = ?
		ORDER BY s.created_at ASC, s.id ASC
		LIMIT ? OFFSET ?
	`

	entries := make([]contracts.RadioPlayMatchSuggestionEntry, 0)
	if err := s.db.Raw(query, catalogm.RadioPlayMatchSuggestionStatusPending, limit, offset).
		Scan(&entries).Error; err != nil {
		return nil, fmt.Errorf("list pending match suggestions: %w", err)
	}

	var total int64
	if err := s.db.Model(&catalogm.RadioPlayMatchSuggestion{}).
		Where("status = ?", catalogm.RadioPlayMatchSuggestionStatusPending).
		Count(&total).Error; err != nil {
		return nil, fmt.Errorf("count pending match suggestions: %w", err)
	}

	return &contracts.RadioPlayMatchSuggestionListResult{
		Suggestions: entries,
		Total:       total,
	}, nil
}

// AcceptSuggestion links the play via RadioService.LinkPlay, optionally
// BulkLinkPlays, stamps the row accepted, and emails the submitter (edit-
// approved prefs). Idempotent on replay of the same verdict.
func (s *RadioPlayMatchSuggestionService) AcceptSuggestion(
	suggestionID, reviewerUserID uint,
	req *contracts.AcceptRadioPlayMatchSuggestionRequest,
) (*contracts.RadioPlayMatchSuggestionReviewResult, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	if s.radioService == nil {
		return nil, fmt.Errorf("radio service not configured")
	}
	if req == nil {
		req = &contracts.AcceptRadioPlayMatchSuggestionRequest{}
	}

	suggestion, err := s.loadSuggestion(suggestionID)
	if err != nil {
		return nil, err
	}
	if suggestion.Status == catalogm.RadioPlayMatchSuggestionStatusAccepted {
		return reviewResultFromModel(suggestion, nil), nil
	}
	if suggestion.Status == catalogm.RadioPlayMatchSuggestionStatusRejected {
		return nil, contracts.ErrRadioPlayMatchSuggestionAlreadyReviewed
	}

	artistID := suggestion.SuggestedArtistID
	if err := s.radioService.LinkPlay(suggestion.PlayID, &contracts.LinkPlayRequest{
		ArtistID: &artistID,
	}); err != nil {
		return nil, fmt.Errorf("link play on accept: %w", err)
	}

	var bulkUpdated *int
	if req.AlsoBulkLinkName {
		var play catalogm.RadioPlay
		if err := s.db.Select("id", "artist_name").First(&play, suggestion.PlayID).Error; err != nil {
			return nil, fmt.Errorf("load play for bulk link: %w", err)
		}
		bulkResult, err := s.radioService.BulkLinkPlays(&contracts.BulkLinkRequest{
			ArtistName: play.ArtistName,
			ArtistID:   suggestion.SuggestedArtistID,
		})
		if err != nil {
			return nil, fmt.Errorf("bulk link on accept: %w", err)
		}
		n := bulkResult.Updated
		bulkUpdated = &n
	}

	reviewedAt := s.now().UTC()
	res := s.db.Model(&catalogm.RadioPlayMatchSuggestion{}).
		Where("id = ? AND status = ?", suggestionID, catalogm.RadioPlayMatchSuggestionStatusPending).
		Updates(map[string]interface{}{
			"status":      catalogm.RadioPlayMatchSuggestionStatusAccepted,
			"reviewed_at": reviewedAt,
			"reviewed_by": reviewerUserID,
			"updated_at":  reviewedAt,
		})
	if res.Error != nil {
		return nil, fmt.Errorf("mark suggestion accepted: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		reloaded, err := s.loadSuggestion(suggestionID)
		if err != nil {
			return nil, err
		}
		return reviewResultFromModel(reloaded, bulkUpdated), nil
	}

	return &contracts.RadioPlayMatchSuggestionReviewResult{
		ID:                suggestionID,
		PlayID:            suggestion.PlayID,
		SuggestedArtistID: suggestion.SuggestedArtistID,
		SubmittedBy:       suggestion.SubmittedBy,
		Status:            catalogm.RadioPlayMatchSuggestionStatusAccepted,
		ReviewedBy:        &reviewerUserID,
		ReviewedAt:        &reviewedAt,
		BulkUpdated:       bulkUpdated,
		NewlyReviewed:     true,
	}, nil
}

// RejectSuggestion stamps rejection_reason + reviewer. Idempotent on replay of
// reject; conflicting accept→reject returns AlreadyReviewed.
//
// Resubmit: after reject the submitter may CreateSuggestion again for the same
// play (partial unique only covers pending rows).
func (s *RadioPlayMatchSuggestionService) RejectSuggestion(
	suggestionID, reviewerUserID uint,
	req *contracts.RejectRadioPlayMatchSuggestionRequest,
) (*contracts.RadioPlayMatchSuggestionReviewResult, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	if req == nil || strings.TrimSpace(req.Reason) == "" {
		return nil, contracts.ErrRadioPlayMatchSuggestionRejectReasonRequired
	}
	reason := strings.TrimSpace(req.Reason)

	suggestion, err := s.loadSuggestion(suggestionID)
	if err != nil {
		return nil, err
	}
	if suggestion.Status == catalogm.RadioPlayMatchSuggestionStatusRejected {
		return reviewResultFromModel(suggestion, nil), nil
	}
	if suggestion.Status == catalogm.RadioPlayMatchSuggestionStatusAccepted {
		return nil, contracts.ErrRadioPlayMatchSuggestionAlreadyReviewed
	}

	reviewedAt := s.now().UTC()
	res := s.db.Model(&catalogm.RadioPlayMatchSuggestion{}).
		Where("id = ? AND status = ?", suggestionID, catalogm.RadioPlayMatchSuggestionStatusPending).
		Updates(map[string]interface{}{
			"status":           catalogm.RadioPlayMatchSuggestionStatusRejected,
			"reviewed_at":      reviewedAt,
			"reviewed_by":      reviewerUserID,
			"rejection_reason": reason,
			"updated_at":       reviewedAt,
		})
	if res.Error != nil {
		return nil, fmt.Errorf("mark suggestion rejected: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		reloaded, err := s.loadSuggestion(suggestionID)
		if err != nil {
			return nil, err
		}
		return reviewResultFromModel(reloaded, nil), nil
	}

	return &contracts.RadioPlayMatchSuggestionReviewResult{
		ID:                suggestionID,
		PlayID:            suggestion.PlayID,
		SuggestedArtistID: suggestion.SuggestedArtistID,
		SubmittedBy:       suggestion.SubmittedBy,
		Status:            catalogm.RadioPlayMatchSuggestionStatusRejected,
		ReviewedBy:        &reviewerUserID,
		ReviewedAt:        &reviewedAt,
		RejectionReason:   &reason,
		NewlyReviewed:     true,
	}, nil
}

func (s *RadioPlayMatchSuggestionService) loadSuggestion(id uint) (*catalogm.RadioPlayMatchSuggestion, error) {
	var row catalogm.RadioPlayMatchSuggestion
	if err := s.db.First(&row, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, contracts.ErrRadioPlayMatchSuggestionNotFound
		}
		return nil, fmt.Errorf("load match suggestion: %w", err)
	}
	return &row, nil
}

func (s *RadioPlayMatchSuggestionService) loadEntry(id uint) (*contracts.RadioPlayMatchSuggestionEntry, error) {
	query := `
		SELECT
			s.id,
			s.play_id,
			rp.artist_name AS play_artist_name,
			rp.match_state AS play_match_state,
			s.suggested_artist_id,
			a.name AS suggested_artist_name,
			a.slug AS suggested_artist_slug,
			s.submitted_by,
			u.username AS submitter_username,
			s.note,
			s.status,
			s.reviewed_by,
			s.reviewed_at,
			s.rejection_reason,
			s.created_at
		FROM radio_play_match_suggestions s
		JOIN radio_plays rp ON rp.id = s.play_id
		JOIN artists a ON a.id = s.suggested_artist_id
		LEFT JOIN users u ON u.id = s.submitted_by
		WHERE s.id = ?
	`
	var entry contracts.RadioPlayMatchSuggestionEntry
	if err := s.db.Raw(query, id).Scan(&entry).Error; err != nil {
		return nil, fmt.Errorf("load match suggestion entry: %w", err)
	}
	if entry.ID == 0 {
		return nil, contracts.ErrRadioPlayMatchSuggestionNotFound
	}
	return &entry, nil
}

func (s *RadioPlayMatchSuggestionService) entryFromParts(
	row *catalogm.RadioPlayMatchSuggestion,
	play *catalogm.RadioPlay,
	artist *catalogm.Artist,
	submitterUsername *string,
) *contracts.RadioPlayMatchSuggestionEntry {
	return &contracts.RadioPlayMatchSuggestionEntry{
		ID:                  row.ID,
		PlayID:              row.PlayID,
		PlayArtistName:      play.ArtistName,
		PlayMatchState:      play.MatchState,
		SuggestedArtistID:   row.SuggestedArtistID,
		SuggestedArtistName: artist.Name,
		SuggestedArtistSlug: artist.Slug,
		SubmittedBy:         row.SubmittedBy,
		SubmitterUsername:   submitterUsername,
		Note:                row.Note,
		Status:              row.Status,
		ReviewedBy:          row.ReviewedBy,
		ReviewedAt:          row.ReviewedAt,
		RejectionReason:     row.RejectionReason,
		CreatedAt:           row.CreatedAt,
	}
}

func reviewResultFromModel(
	row *catalogm.RadioPlayMatchSuggestion,
	bulkUpdated *int,
) *contracts.RadioPlayMatchSuggestionReviewResult {
	return &contracts.RadioPlayMatchSuggestionReviewResult{
		ID:                row.ID,
		PlayID:            row.PlayID,
		SuggestedArtistID: row.SuggestedArtistID,
		SubmittedBy:       row.SubmittedBy,
		Status:            row.Status,
		ReviewedBy:        row.ReviewedBy,
		ReviewedAt:        row.ReviewedAt,
		RejectionReason:   row.RejectionReason,
		BulkUpdated:       bulkUpdated,
	}
}

func trimNote(note *string) *string {
	if note == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*note)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}
