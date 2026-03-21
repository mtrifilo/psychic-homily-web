package pipeline

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"gorm.io/gorm"

	"psychic-homily-backend/db"
	"psychic-homily-backend/internal/models"
	"psychic-homily-backend/internal/services/contracts"
)

// Default thresholds for artist matching
const (
	// AutoMatchThreshold: similarity >= this value auto-links the artist
	AutoMatchThreshold = 0.8
	// SuggestThreshold: similarity >= this value stored as suggestion
	SuggestThreshold = 0.5
)

// EnrichmentService handles post-import enrichment of shows with artist matching,
// MusicBrainz lookups, and API cross-referencing.
type EnrichmentService struct {
	db              *gorm.DB
	artistService   contracts.ArtistServiceInterface
	mbClient        *MusicBrainzClient
	sgClient        *SeatGeekClient
	logger          *slog.Logger
	matchThreshold  float64
}

// NewEnrichmentService creates a new enrichment service.
func NewEnrichmentService(
	database *gorm.DB,
	artistService contracts.ArtistServiceInterface,
	seatgeekClientID string,
) *EnrichmentService {
	if database == nil {
		database = db.GetDB()
	}
	return &EnrichmentService{
		db:             database,
		artistService:  artistService,
		mbClient:       NewMusicBrainzClient(),
		sgClient:       NewSeatGeekClient(seatgeekClientID),
		logger:         slog.Default(),
		matchThreshold: AutoMatchThreshold,
	}
}

// QueueShowForEnrichment adds a show to the enrichment queue.
func (s *EnrichmentService) QueueShowForEnrichment(showID uint, enrichmentType string) error {
	if s.db == nil {
		return fmt.Errorf("database not initialized")
	}

	// Validate enrichment type
	switch enrichmentType {
	case models.EnrichmentTypeArtistMatch,
		models.EnrichmentTypeMusicBrainz,
		models.EnrichmentTypeAPICrossRef,
		models.EnrichmentTypeAll:
		// valid
	default:
		return fmt.Errorf("invalid enrichment type: %s", enrichmentType)
	}

	item := &models.EnrichmentQueueItem{
		ShowID:         showID,
		Status:         models.EnrichmentStatusPending,
		EnrichmentType: enrichmentType,
	}

	return s.db.Create(item).Error
}

// ProcessQueue processes pending enrichment items in batch.
// Returns the number of items processed.
func (s *EnrichmentService) ProcessQueue(ctx context.Context, batchSize int) (int, error) {
	if s.db == nil {
		return 0, fmt.Errorf("database not initialized")
	}

	if batchSize <= 0 {
		batchSize = 10
	}

	// Fetch pending items ordered by creation time
	var items []models.EnrichmentQueueItem
	err := s.db.Where("status = ? AND attempts < max_attempts", models.EnrichmentStatusPending).
		Order("created_at ASC").
		Limit(batchSize).
		Find(&items).Error
	if err != nil {
		return 0, fmt.Errorf("failed to fetch pending enrichment items: %w", err)
	}

	processed := 0
	for _, item := range items {
		select {
		case <-ctx.Done():
			return processed, ctx.Err()
		default:
		}

		// Mark as processing
		s.db.Model(&item).Updates(map[string]interface{}{
			"status":   models.EnrichmentStatusProcessing,
			"attempts": item.Attempts + 1,
		})

		// Run enrichment
		result, err := s.EnrichShow(ctx, item.ShowID)
		if err != nil {
			errStr := err.Error()
			if item.Attempts+1 >= item.MaxAttempts {
				// Max retries exceeded — mark as failed
				s.db.Model(&item).Updates(map[string]interface{}{
					"status":     models.EnrichmentStatusFailed,
					"last_error": errStr,
				})
			} else {
				// Retry later — reset to pending
				s.db.Model(&item).Updates(map[string]interface{}{
					"status":     models.EnrichmentStatusPending,
					"last_error": errStr,
				})
			}
			s.logger.Warn("enrichment failed",
				"show_id", item.ShowID,
				"attempt", item.Attempts+1,
				"error", err,
			)
		} else {
			// Success — store results
			resultJSON, _ := json.Marshal(result)
			raw := json.RawMessage(resultJSON)
			now := time.Now()
			s.db.Model(&item).Updates(map[string]interface{}{
				"status":       models.EnrichmentStatusCompleted,
				"results":      &raw,
				"completed_at": &now,
			})
			s.logger.Info("enrichment completed",
				"show_id", item.ShowID,
				"steps", strings.Join(result.CompletedSteps, ","),
			)
		}

		processed++
	}

	return processed, nil
}

// EnrichShow runs all applicable enrichment steps for a single show.
func (s *EnrichmentService) EnrichShow(ctx context.Context, showID uint) (*contracts.EnrichmentResult, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	// Load the show with its artists and venues
	var show models.Show
	if err := s.db.Preload("Artists").Preload("Venues").First(&show, showID).Error; err != nil {
		return nil, fmt.Errorf("show not found: %w", err)
	}

	// Load show_artists for detailed info (position, set_type)
	var showArtists []models.ShowArtist
	s.db.Where("show_id = ?", showID).Find(&showArtists)

	result := &contracts.EnrichmentResult{
		ShowID: showID,
	}

	// Step 1: Artist fuzzy matching
	artistResults := s.enrichArtistMatching(show.Artists, showArtists)
	result.ArtistMatches = artistResults
	result.CompletedSteps = append(result.CompletedSteps, "artist_match")

	// Step 2: MusicBrainz lookup (respect context cancellation)
	select {
	case <-ctx.Done():
		return result, nil
	default:
	}
	mbResults := s.enrichMusicBrainz(show.Artists)
	result.MusicBrainz = mbResults
	result.CompletedSteps = append(result.CompletedSteps, "musicbrainz")

	// Step 3: SeatGeek cross-reference
	select {
	case <-ctx.Done():
		return result, nil
	default:
	}
	sgResult := s.enrichSeatGeek(&show)
	result.SeatGeek = sgResult
	result.CompletedSteps = append(result.CompletedSteps, "api_crossref")

	return result, nil
}

// GetQueueStats returns summary statistics about the enrichment queue.
func (s *EnrichmentService) GetQueueStats() (*contracts.EnrichmentQueueStats, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	stats := &contracts.EnrichmentQueueStats{}

	s.db.Model(&models.EnrichmentQueueItem{}).
		Where("status = ?", models.EnrichmentStatusPending).
		Count(&stats.Pending)

	s.db.Model(&models.EnrichmentQueueItem{}).
		Where("status = ?", models.EnrichmentStatusProcessing).
		Count(&stats.Processing)

	today := time.Now().Truncate(24 * time.Hour)
	s.db.Model(&models.EnrichmentQueueItem{}).
		Where("status = ? AND completed_at >= ?", models.EnrichmentStatusCompleted, today).
		Count(&stats.CompletedToday)

	s.db.Model(&models.EnrichmentQueueItem{}).
		Where("status = ? AND updated_at >= ?", models.EnrichmentStatusFailed, today).
		Count(&stats.FailedToday)

	return stats, nil
}

// enrichArtistMatching performs fuzzy artist matching for each show artist.
func (s *EnrichmentService) enrichArtistMatching(artists []models.Artist, showArtists []models.ShowArtist) []contracts.ArtistMatchEnrichment {
	var results []contracts.ArtistMatchEnrichment

	for _, artist := range artists {
		// Skip if artist already has a well-established record (has slug, etc.)
		if artist.Slug != nil && *artist.Slug != "" && artist.DataSource != nil {
			continue
		}

		// Search for potential matches
		matches, err := s.artistService.SearchArtists(artist.Name)
		if err != nil {
			results = append(results, contracts.ArtistMatchEnrichment{
				ArtistName: artist.Name,
				Confidence: 0,
			})
			continue
		}

		matchResult := contracts.ArtistMatchEnrichment{
			ArtistName: artist.Name,
			Confidence: 0,
		}

		// Look for the best match that is NOT the same artist
		for _, match := range matches {
			if match.ID == artist.ID {
				continue // Skip self-match
			}
			// The search results are ordered by similarity, so the first non-self match
			// is the best candidate
			matchResult.MatchedID = &match.ID
			matchResult.MatchedName = &match.Name
			// Estimate confidence from position (SearchArtists uses pg_trgm similarity)
			// First result = highest confidence
			matchResult.Confidence = 0.9
			break
		}

		results = append(results, matchResult)
	}

	return results
}

// enrichMusicBrainz performs MusicBrainz lookups for unlinked artists.
func (s *EnrichmentService) enrichMusicBrainz(artists []models.Artist) []contracts.MBEnrichment {
	var results []contracts.MBEnrichment

	for _, artist := range artists {
		enrichment := contracts.MBEnrichment{
			ArtistName: artist.Name,
			ArtistID:   artist.ID,
		}

		// Skip if already has MusicBrainz data
		if artist.DataSource != nil && *artist.DataSource == models.DataSourceMusicBrainz {
			enrichment.AlreadyHadMBID = true
			enrichment.Found = true
			results = append(results, enrichment)
			continue
		}

		// Search MusicBrainz
		mbResult, err := s.mbClient.SearchArtist(artist.Name)
		if err != nil {
			s.logger.Warn("musicbrainz lookup failed",
				"artist", artist.Name,
				"error", err,
			)
			results = append(results, enrichment)
			continue
		}

		if mbResult == nil {
			results = append(results, enrichment)
			continue
		}

		enrichment.Found = true
		enrichment.MBID = mbResult.MBID
		enrichment.MBName = mbResult.Name
		enrichment.Score = mbResult.Score

		// Update artist's data provenance (fire-and-forget)
		mbSource := models.DataSourceMusicBrainz
		mbConfidence := float64(mbResult.Score) / 100.0
		now := time.Now()
		updateErr := s.db.Model(&models.Artist{}).Where("id = ?", artist.ID).Updates(map[string]interface{}{
			"data_source":       &mbSource,
			"source_confidence": &mbConfidence,
			"last_verified_at":  &now,
		}).Error
		if updateErr != nil {
			s.logger.Warn("failed to update artist provenance",
				"artist_id", artist.ID,
				"error", updateErr,
			)
		}

		results = append(results, enrichment)
	}

	return results
}

// enrichSeatGeek performs SeatGeek API cross-referencing for a show.
func (s *EnrichmentService) enrichSeatGeek(show *models.Show) *contracts.SeatGeekEnrichment {
	if !s.sgClient.IsConfigured() {
		return &contracts.SeatGeekEnrichment{Found: false}
	}

	// Get venue name for search
	venueName := ""
	if len(show.Venues) > 0 {
		venueName = show.Venues[0].Name
	}
	if venueName == "" {
		return &contracts.SeatGeekEnrichment{Found: false}
	}

	sgResult, err := s.sgClient.SearchEvent(venueName, show.EventDate)
	if err != nil {
		s.logger.Warn("seatgeek lookup failed",
			"show_id", show.ID,
			"venue", venueName,
			"error", err,
		)
		return &contracts.SeatGeekEnrichment{Found: false}
	}

	if sgResult == nil {
		return &contracts.SeatGeekEnrichment{Found: false}
	}

	enrichment := &contracts.SeatGeekEnrichment{
		Found:        true,
		EventID:      sgResult.EventID,
		LowestPrice:  sgResult.LowestPrice,
		HighestPrice: sgResult.HighestPrice,
		AveragePrice: sgResult.AveragePrice,
		Genres:       sgResult.Genres,
		EventType:    sgResult.EventType,
	}

	// If SeatGeek confirms the event, boost source confidence
	if show.SourceConfidence != nil {
		boosted := *show.SourceConfidence + 0.1
		if boosted > 1.0 {
			boosted = 1.0
		}
		s.db.Model(show).Update("source_confidence", boosted)
	}

	return enrichment
}
