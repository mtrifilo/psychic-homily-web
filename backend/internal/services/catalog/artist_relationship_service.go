package catalog

import (
	"encoding/json"
	"fmt"
	"math"
	"time"

	"gorm.io/gorm"

	"psychic-homily-backend/db"
	"psychic-homily-backend/internal/models"
	"psychic-homily-backend/internal/services/contracts"
)

// ArtistRelationshipService handles artist relationship business logic.
type ArtistRelationshipService struct {
	db *gorm.DB
}

// NewArtistRelationshipService creates a new artist relationship service.
func NewArtistRelationshipService(database *gorm.DB) *ArtistRelationshipService {
	if database == nil {
		database = db.GetDB()
	}
	return &ArtistRelationshipService{db: database}
}

// ──────────────────────────────────────────────
// CRUD
// ──────────────────────────────────────────────

// CreateRelationship creates a new artist relationship with canonical ordering.
func (s *ArtistRelationshipService) CreateRelationship(sourceID, targetID uint, relType string, autoDerived bool) (*models.ArtistRelationship, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	if sourceID == targetID {
		return nil, fmt.Errorf("cannot create relationship between an artist and itself")
	}

	src, tgt := models.CanonicalOrder(sourceID, targetID)

	// Check for existing
	var existing models.ArtistRelationship
	err := s.db.Where("source_artist_id = ? AND target_artist_id = ? AND relationship_type = ?",
		src, tgt, relType).First(&existing).Error
	if err == nil {
		return nil, fmt.Errorf("relationship already exists between artists %d and %d (type: %s)", src, tgt, relType)
	}

	rel := &models.ArtistRelationship{
		SourceArtistID:   src,
		TargetArtistID:   tgt,
		RelationshipType: relType,
		AutoDerived:      autoDerived,
	}

	if err := s.db.Create(rel).Error; err != nil {
		return nil, fmt.Errorf("failed to create relationship: %w", err)
	}

	return rel, nil
}

// GetRelationship retrieves a relationship between two artists (order-independent).
func (s *ArtistRelationshipService) GetRelationship(artistA, artistB uint, relType string) (*models.ArtistRelationship, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	src, tgt := models.CanonicalOrder(artistA, artistB)

	var rel models.ArtistRelationship
	err := s.db.Where("source_artist_id = ? AND target_artist_id = ? AND relationship_type = ?",
		src, tgt, relType).First(&rel).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get relationship: %w", err)
	}

	return &rel, nil
}

// GetRelatedArtists returns artists related to the given artist with vote counts.
// Pass relType="" to get all types. Results sorted by score descending.
func (s *ArtistRelationshipService) GetRelatedArtists(artistID uint, relType string, limit int) ([]contracts.RelatedArtistResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	if limit <= 0 {
		limit = 30
	}

	// Query both directions
	query := s.db.Model(&models.ArtistRelationship{}).
		Where("source_artist_id = ? OR target_artist_id = ?", artistID, artistID)

	if relType != "" {
		query = query.Where("relationship_type = ?", relType)
	}

	query = query.Order("score DESC").Limit(limit)

	var rels []models.ArtistRelationship
	if err := query.Find(&rels).Error; err != nil {
		return nil, fmt.Errorf("failed to get related artists: %w", err)
	}

	responses := make([]contracts.RelatedArtistResponse, 0, len(rels))
	for _, rel := range rels {
		// Determine the "other" artist
		otherID := rel.TargetArtistID
		if otherID == artistID {
			otherID = rel.SourceArtistID
		}

		// Fetch artist info
		var artist models.Artist
		if err := s.db.First(&artist, otherID).Error; err != nil {
			continue
		}

		slug := ""
		if artist.Slug != nil {
			slug = *artist.Slug
		}

		// Get vote counts
		upvotes, downvotes := s.getVoteCounts(rel.SourceArtistID, rel.TargetArtistID, rel.RelationshipType)

		resp := contracts.RelatedArtistResponse{
			ArtistID:         otherID,
			Name:             artist.Name,
			Slug:             slug,
			RelationshipType: rel.RelationshipType,
			Score:            rel.Score,
			Upvotes:          upvotes,
			Downvotes:        downvotes,
			WilsonScore:      rel.WilsonScore(upvotes, downvotes),
			AutoDerived:      rel.AutoDerived,
		}

		responses = append(responses, resp)
	}

	return responses, nil
}

// DeleteRelationship deletes a relationship and its votes.
func (s *ArtistRelationshipService) DeleteRelationship(artistA, artistB uint, relType string) error {
	if s.db == nil {
		return fmt.Errorf("database not initialized")
	}

	src, tgt := models.CanonicalOrder(artistA, artistB)

	return s.db.Transaction(func(tx *gorm.DB) error {
		// Delete votes first
		tx.Where("source_artist_id = ? AND target_artist_id = ? AND relationship_type = ?",
			src, tgt, relType).Delete(&models.ArtistRelationshipVote{})

		result := tx.Where("source_artist_id = ? AND target_artist_id = ? AND relationship_type = ?",
			src, tgt, relType).Delete(&models.ArtistRelationship{})
		if result.Error != nil {
			return fmt.Errorf("failed to delete relationship: %w", result.Error)
		}
		if result.RowsAffected == 0 {
			return fmt.Errorf("relationship not found")
		}
		return nil
	})
}

// ──────────────────────────────────────────────
// Voting
// ──────────────────────────────────────────────

// Vote adds or updates a vote on an artist relationship.
func (s *ArtistRelationshipService) Vote(artistA, artistB uint, relType string, userID uint, isUpvote bool) error {
	if s.db == nil {
		return fmt.Errorf("database not initialized")
	}

	src, tgt := models.CanonicalOrder(artistA, artistB)

	// Verify relationship exists
	var rel models.ArtistRelationship
	err := s.db.Where("source_artist_id = ? AND target_artist_id = ? AND relationship_type = ?",
		src, tgt, relType).First(&rel).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return fmt.Errorf("relationship not found between artists %d and %d (type: %s)", src, tgt, relType)
		}
		return fmt.Errorf("failed to verify relationship: %w", err)
	}

	direction := int16(-1)
	if isUpvote {
		direction = 1
	}

	return s.db.Transaction(func(tx *gorm.DB) error {
		// Upsert vote
		var existing models.ArtistRelationshipVote
		err := tx.Where("source_artist_id = ? AND target_artist_id = ? AND relationship_type = ? AND user_id = ?",
			src, tgt, relType, userID).First(&existing).Error

		if err == gorm.ErrRecordNotFound {
			vote := models.ArtistRelationshipVote{
				SourceArtistID:   src,
				TargetArtistID:   tgt,
				RelationshipType: relType,
				UserID:           userID,
				Direction:        direction,
			}
			if err := tx.Create(&vote).Error; err != nil {
				return fmt.Errorf("failed to create vote: %w", err)
			}
		} else if err != nil {
			return fmt.Errorf("failed to check existing vote: %w", err)
		} else {
			if err := tx.Model(&existing).Update("direction", direction).Error; err != nil {
				return fmt.Errorf("failed to update vote: %w", err)
			}
		}

		// Recalculate score
		return s.recalculateScore(tx, src, tgt, relType)
	})
}

// RemoveVote removes a user's vote on an artist relationship.
func (s *ArtistRelationshipService) RemoveVote(artistA, artistB uint, relType string, userID uint) error {
	if s.db == nil {
		return fmt.Errorf("database not initialized")
	}

	src, tgt := models.CanonicalOrder(artistA, artistB)

	return s.db.Transaction(func(tx *gorm.DB) error {
		tx.Where("source_artist_id = ? AND target_artist_id = ? AND relationship_type = ? AND user_id = ?",
			src, tgt, relType, userID).Delete(&models.ArtistRelationshipVote{})

		return s.recalculateScore(tx, src, tgt, relType)
	})
}

// GetUserVote returns a user's vote on a relationship, or nil if not voted.
func (s *ArtistRelationshipService) GetUserVote(artistA, artistB uint, relType string, userID uint) (*models.ArtistRelationshipVote, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	src, tgt := models.CanonicalOrder(artistA, artistB)

	var vote models.ArtistRelationshipVote
	err := s.db.Where("source_artist_id = ? AND target_artist_id = ? AND relationship_type = ? AND user_id = ?",
		src, tgt, relType, userID).First(&vote).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get user vote: %w", err)
	}

	return &vote, nil
}

// ──────────────────────────────────────────────
// Auto-derivation
// ──────────────────────────────────────────────

// sharedBillRow represents the result of the co-occurrence query.
type sharedBillRow struct {
	ArtistA     uint
	ArtistB     uint
	SharedCount int
	LastShared  time.Time
}

// DeriveSharedBills computes shared_bills relationships from show_artists co-occurrences.
// Creates or updates relationships where artists share minShows or more approved shows.
func (s *ArtistRelationshipService) DeriveSharedBills(minShows int) (int64, error) {
	if s.db == nil {
		return 0, fmt.Errorf("database not initialized")
	}

	if minShows <= 0 {
		minShows = 2
	}

	var rows []sharedBillRow
	err := s.db.Raw(`
		SELECT
			sa1.artist_id AS artist_a,
			sa2.artist_id AS artist_b,
			COUNT(DISTINCT sa1.show_id) AS shared_count,
			MAX(s.event_date) AS last_shared
		FROM show_artists sa1
		JOIN show_artists sa2 ON sa1.show_id = sa2.show_id
			AND sa1.artist_id < sa2.artist_id
		JOIN shows s ON s.id = sa1.show_id
		WHERE s.status = 'approved'
		GROUP BY sa1.artist_id, sa2.artist_id
		HAVING COUNT(DISTINCT sa1.show_id) >= ?
	`, minShows).Scan(&rows).Error

	if err != nil {
		return 0, fmt.Errorf("failed to query shared bills: %w", err)
	}

	var upserted int64
	now := time.Now()

	for _, row := range rows {
		// Compute recency-weighted score
		monthsSince := now.Sub(row.LastShared).Hours() / (24 * 30)
		score := float32(math.Min(float64(row.SharedCount)/10.0, 1.0))
		// Boost for recency
		if monthsSince < 3 {
			score = float32(math.Min(float64(score)*1.2, 1.0))
		}

		detail, _ := json.Marshal(map[string]interface{}{
			"shared_count": row.SharedCount,
			"last_shared":  row.LastShared.Format("2006-01-02"),
		})
		detailRaw := json.RawMessage(detail)

		// Upsert
		var existing models.ArtistRelationship
		err := s.db.Where("source_artist_id = ? AND target_artist_id = ? AND relationship_type = ?",
			row.ArtistA, row.ArtistB, models.RelationshipTypeSharedBills).First(&existing).Error

		if err == gorm.ErrRecordNotFound {
			rel := &models.ArtistRelationship{
				SourceArtistID:   row.ArtistA,
				TargetArtistID:   row.ArtistB,
				RelationshipType: models.RelationshipTypeSharedBills,
				Score:            score,
				AutoDerived:      true,
				Detail:           &detailRaw,
			}
			if err := s.db.Create(rel).Error; err == nil {
				upserted++
			}
		} else if err == nil {
			s.db.Model(&existing).Updates(map[string]interface{}{
				"score":  score,
				"detail": &detailRaw,
			})
			upserted++
		}
	}

	return upserted, nil
}

// ──────────────────────────────────────────────
// Helpers
// ──────────────────────────────────────────────

// getVoteCounts returns upvote and downvote counts for a relationship.
func (s *ArtistRelationshipService) getVoteCounts(sourceID, targetID uint, relType string) (int, int) {
	var upvotes, downvotes int64
	s.db.Model(&models.ArtistRelationshipVote{}).
		Where("source_artist_id = ? AND target_artist_id = ? AND relationship_type = ? AND direction = 1",
			sourceID, targetID, relType).Count(&upvotes)
	s.db.Model(&models.ArtistRelationshipVote{}).
		Where("source_artist_id = ? AND target_artist_id = ? AND relationship_type = ? AND direction = -1",
			sourceID, targetID, relType).Count(&downvotes)
	return int(upvotes), int(downvotes)
}

// recalculateScore recalculates the score for a relationship from vote counts.
func (s *ArtistRelationshipService) recalculateScore(tx *gorm.DB, sourceID, targetID uint, relType string) error {
	var upvotes, downvotes int64
	tx.Model(&models.ArtistRelationshipVote{}).
		Where("source_artist_id = ? AND target_artist_id = ? AND relationship_type = ? AND direction = 1",
			sourceID, targetID, relType).Count(&upvotes)
	tx.Model(&models.ArtistRelationshipVote{}).
		Where("source_artist_id = ? AND target_artist_id = ? AND relationship_type = ? AND direction = -1",
			sourceID, targetID, relType).Count(&downvotes)

	var rel models.ArtistRelationship
	score := float32(rel.WilsonScore(int(upvotes), int(downvotes)))

	return tx.Model(&models.ArtistRelationship{}).
		Where("source_artist_id = ? AND target_artist_id = ? AND relationship_type = ?",
			sourceID, targetID, relType).
		Update("score", score).Error
}
