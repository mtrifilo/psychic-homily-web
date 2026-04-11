package engagement

import (
	"fmt"
	"time"

	"gorm.io/gorm"

	"psychic-homily-backend/db"
	"psychic-homily-backend/internal/models"
	"psychic-homily-backend/internal/scoring"
)

// CommentVoteService handles comment voting operations.
type CommentVoteService struct {
	db *gorm.DB
}

// NewCommentVoteService creates a new comment vote service.
func NewCommentVoteService(database *gorm.DB) *CommentVoteService {
	if database == nil {
		database = db.GetDB()
	}
	return &CommentVoteService{
		db: database,
	}
}

// Vote casts or updates a vote on a comment.
// direction must be 1 (upvote) or -1 (downvote).
func (s *CommentVoteService) Vote(userID uint, commentID uint, direction int) error {
	if s.db == nil {
		return fmt.Errorf("database not initialized")
	}

	if direction != 1 && direction != -1 {
		return fmt.Errorf("invalid vote direction: must be 1 or -1")
	}

	// Verify comment exists
	var comment models.Comment
	if err := s.db.First(&comment, commentID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return fmt.Errorf("comment not found")
		}
		return fmt.Errorf("failed to get comment: %w", err)
	}

	return s.db.Transaction(func(tx *gorm.DB) error {
		// Upsert the vote
		var existingVote models.CommentVote
		err := tx.Where("comment_id = ? AND user_id = ?", commentID, userID).First(&existingVote).Error

		if err == gorm.ErrRecordNotFound {
			// New vote
			vote := models.CommentVote{
				CommentID: commentID,
				UserID:    userID,
				Direction: int16(direction),
				CreatedAt: time.Now().UTC(),
			}
			if err := tx.Create(&vote).Error; err != nil {
				return fmt.Errorf("failed to create vote: %w", err)
			}
		} else if err != nil {
			return fmt.Errorf("failed to check existing vote: %w", err)
		} else {
			// Update existing vote
			if err := tx.Model(&existingVote).Update("direction", int16(direction)).Error; err != nil {
				return fmt.Errorf("failed to update vote: %w", err)
			}
		}

		// Recompute aggregates
		return s.recomputeAggregates(tx, commentID)
	})
}

// Unvote removes a user's vote on a comment.
func (s *CommentVoteService) Unvote(userID uint, commentID uint) error {
	if s.db == nil {
		return fmt.Errorf("database not initialized")
	}

	// Verify comment exists
	var comment models.Comment
	if err := s.db.First(&comment, commentID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return fmt.Errorf("comment not found")
		}
		return fmt.Errorf("failed to get comment: %w", err)
	}

	return s.db.Transaction(func(tx *gorm.DB) error {
		result := tx.Where("comment_id = ? AND user_id = ?", commentID, userID).Delete(&models.CommentVote{})
		if result.Error != nil {
			return fmt.Errorf("failed to remove vote: %w", result.Error)
		}

		// Recompute aggregates
		return s.recomputeAggregates(tx, commentID)
	})
}

// GetUserVote returns the user's vote direction (1 or -1) or nil if not voted.
func (s *CommentVoteService) GetUserVote(userID uint, commentID uint) (*int, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	var vote models.CommentVote
	err := s.db.Where("comment_id = ? AND user_id = ?", commentID, userID).First(&vote).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get user vote: %w", err)
	}

	dir := int(vote.Direction)
	return &dir, nil
}

// GetUserVotesForComments returns a map of commentID→direction for batch lookups.
func (s *CommentVoteService) GetUserVotesForComments(userID uint, commentIDs []uint) (map[uint]int, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	if len(commentIDs) == 0 {
		return make(map[uint]int), nil
	}

	var votes []models.CommentVote
	err := s.db.Where("user_id = ? AND comment_id IN ?", userID, commentIDs).Find(&votes).Error
	if err != nil {
		return nil, fmt.Errorf("failed to get user votes: %w", err)
	}

	result := make(map[uint]int, len(votes))
	for _, v := range votes {
		result[v.CommentID] = int(v.Direction)
	}

	return result, nil
}

// GetCommentVoteCounts returns the current ups, downs, and Wilson score for a comment.
func (s *CommentVoteService) GetCommentVoteCounts(commentID uint) (int, int, float64, error) {
	if s.db == nil {
		return 0, 0, 0, fmt.Errorf("database not initialized")
	}

	var comment models.Comment
	err := s.db.Select("ups", "downs", "score").First(&comment, commentID).Error
	if err != nil {
		return 0, 0, 0, fmt.Errorf("failed to get comment: %w", err)
	}
	return comment.Ups, comment.Downs, comment.Score, nil
}

// recomputeAggregates counts ups and downs from comment_votes and updates
// the denormalized ups, downs, and score on the comments table.
func (s *CommentVoteService) recomputeAggregates(tx *gorm.DB, commentID uint) error {
	var ups, downs int64

	tx.Model(&models.CommentVote{}).
		Where("comment_id = ? AND direction = 1", commentID).
		Count(&ups)

	tx.Model(&models.CommentVote{}).
		Where("comment_id = ? AND direction = -1", commentID).
		Count(&downs)

	score := scoring.WilsonScore(int(ups), int(downs))

	return tx.Model(&models.Comment{}).
		Where("id = ?", commentID).
		Updates(map[string]interface{}{
			"ups":   int(ups),
			"downs": int(downs),
			"score": score,
		}).Error
}
