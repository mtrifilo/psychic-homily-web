package user

import (
	"fmt"

	"gorm.io/gorm"

	"psychic-homily-backend/db"
	"psychic-homily-backend/internal/services/contracts"
)

// LeaderboardService computes contributor leaderboards across multiple dimensions.
type LeaderboardService struct {
	db *gorm.DB
}

// NewLeaderboardService creates a new leaderboard service.
func NewLeaderboardService(database *gorm.DB) *LeaderboardService {
	if database == nil {
		database = db.GetDB()
	}
	return &LeaderboardService{
		db: database,
	}
}

// validDimensions lists all supported leaderboard dimensions.
var validDimensions = map[string]bool{
	"overall":  true,
	"shows":    true,
	"venues":   true,
	"tags":     true,
	"edits":    true,
	"requests": true,
}

// validPeriods lists all supported time periods.
var validPeriods = map[string]bool{
	"all_time": true,
	"month":    true,
	"week":     true,
}

// dimensionWeights are used for the "overall" dimension weighted sum.
var dimensionWeights = map[string]int{
	"shows":    25,
	"venues":   15,
	"tags":     10,
	"edits":    25,
	"requests": 10,
}

// GetLeaderboard returns ranked contributor entries for a given dimension and period.
func (s *LeaderboardService) GetLeaderboard(dimension string, period string, limit int) ([]contracts.LeaderboardEntry, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	if !validDimensions[dimension] {
		return nil, fmt.Errorf("invalid dimension: %s", dimension)
	}
	if !validPeriods[period] {
		return nil, fmt.Errorf("invalid period: %s", period)
	}

	if limit <= 0 {
		limit = 25
	}
	if limit > 100 {
		limit = 100
	}

	query, args := s.buildLeaderboardQuery(dimension, period, limit)

	type leaderboardRow struct {
		UserID    uint    `gorm:"column:user_id"`
		Username  string  `gorm:"column:username"`
		AvatarURL *string `gorm:"column:avatar_url"`
		UserTier  string  `gorm:"column:user_tier"`
		Count     int64   `gorm:"column:count"`
	}

	var rows []leaderboardRow
	if err := s.db.Raw(query, args...).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("failed to get leaderboard: %w", err)
	}

	entries := make([]contracts.LeaderboardEntry, len(rows))
	for i, row := range rows {
		entries[i] = contracts.LeaderboardEntry{
			Rank:      i + 1,
			UserID:    row.UserID,
			Username:  row.Username,
			AvatarURL: row.AvatarURL,
			UserTier:  row.UserTier,
			Count:     row.Count,
		}
	}

	return entries, nil
}

// GetUserRank returns the requesting user's rank for a given dimension and period.
// Returns nil if the user has no contributions or their contributions are hidden.
func (s *LeaderboardService) GetUserRank(userID uint, dimension string, period string) (*int, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	if !validDimensions[dimension] {
		return nil, fmt.Errorf("invalid dimension: %s", dimension)
	}
	if !validPeriods[period] {
		return nil, fmt.Errorf("invalid period: %s", period)
	}

	// Get the full leaderboard (uncapped) to find the user's position
	query, args := s.buildLeaderboardQuery(dimension, period, 10000)

	type rankRow struct {
		UserID uint  `gorm:"column:user_id"`
		Count  int64 `gorm:"column:count"`
	}

	var rows []rankRow
	if err := s.db.Raw(query, args...).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("failed to compute user rank: %w", err)
	}

	for i, row := range rows {
		if row.UserID == userID {
			rank := i + 1
			return &rank, nil
		}
	}

	return nil, nil
}

// buildLeaderboardQuery constructs the SQL query for a given dimension and period.
func (s *LeaderboardService) buildLeaderboardQuery(dimension string, period string, limit int) (string, []interface{}) {
	periodFilter := buildPeriodFilter(period)

	if dimension == "overall" {
		return s.buildOverallQuery(periodFilter, limit)
	}

	countSubquery := buildCountSubquery(dimension, periodFilter)

	query := fmt.Sprintf(`
		SELECT sub.user_id, u.username, u.avatar_url, u.user_tier, sub.count
		FROM (%s) sub
		JOIN users u ON u.id = sub.user_id
		WHERE u.is_active = true
		  AND u.username IS NOT NULL
		  AND (u.privacy_settings IS NULL OR u.privacy_settings->>'contributions' != 'hidden')
		  AND sub.count > 0
		ORDER BY sub.count DESC, u.username ASC
		LIMIT ?
	`, countSubquery)

	return query, []interface{}{limit}
}

// buildOverallQuery constructs the weighted overall leaderboard query.
func (s *LeaderboardService) buildOverallQuery(periodFilter string, limit int) (string, []interface{}) {
	showsSubquery := buildCountSubquery("shows", periodFilter)
	venuesSubquery := buildCountSubquery("venues", periodFilter)
	tagsSubquery := buildCountSubquery("tags", periodFilter)
	editsSubquery := buildCountSubquery("edits", periodFilter)
	requestsSubquery := buildCountSubquery("requests", periodFilter)

	query := fmt.Sprintf(`
		SELECT
			u.id AS user_id,
			u.username,
			u.avatar_url,
			u.user_tier,
			COALESCE(shows.count, 0) * %d +
			COALESCE(venues.count, 0) * %d +
			COALESCE(tags.count, 0) * %d +
			COALESCE(edits.count, 0) * %d +
			COALESCE(requests.count, 0) * %d AS count
		FROM users u
		LEFT JOIN (%s) shows ON shows.user_id = u.id
		LEFT JOIN (%s) venues ON venues.user_id = u.id
		LEFT JOIN (%s) tags ON tags.user_id = u.id
		LEFT JOIN (%s) edits ON edits.user_id = u.id
		LEFT JOIN (%s) requests ON requests.user_id = u.id
		WHERE u.is_active = true
		  AND u.username IS NOT NULL
		  AND (u.privacy_settings IS NULL OR u.privacy_settings->>'contributions' != 'hidden')
		  AND (
		    COALESCE(shows.count, 0) +
		    COALESCE(venues.count, 0) +
		    COALESCE(tags.count, 0) +
		    COALESCE(edits.count, 0) +
		    COALESCE(requests.count, 0)
		  ) > 0
		ORDER BY count DESC, u.username ASC
		LIMIT ?
	`,
		dimensionWeights["shows"],
		dimensionWeights["venues"],
		dimensionWeights["tags"],
		dimensionWeights["edits"],
		dimensionWeights["requests"],
		showsSubquery,
		venuesSubquery,
		tagsSubquery,
		editsSubquery,
		requestsSubquery,
	)

	return query, []interface{}{limit}
}

// buildCountSubquery returns a SQL subquery that counts contributions for a dimension.
func buildCountSubquery(dimension string, periodFilter string) string {
	switch dimension {
	case "shows":
		return fmt.Sprintf(`
			SELECT submitted_by AS user_id, COUNT(*) AS count
			FROM shows
			WHERE submitted_by IS NOT NULL %s
			GROUP BY submitted_by
		`, periodFilter)
	case "venues":
		return fmt.Sprintf(`
			SELECT submitted_by AS user_id, COUNT(*) AS count
			FROM venues
			WHERE submitted_by IS NOT NULL %s
			GROUP BY submitted_by
		`, periodFilter)
	case "tags":
		return fmt.Sprintf(`
			SELECT added_by_user_id AS user_id, COUNT(*) AS count
			FROM entity_tags
			WHERE 1=1 %s
			GROUP BY added_by_user_id
		`, periodFilter)
	case "edits":
		// Combine approved pending edits + revisions
		return fmt.Sprintf(`
			SELECT user_id, SUM(count) AS count FROM (
				SELECT submitted_by AS user_id, COUNT(*) AS count
				FROM pending_entity_edits
				WHERE status = 'approved' %s
				GROUP BY submitted_by
				UNION ALL
				SELECT user_id, COUNT(*) AS count
				FROM revisions
				WHERE 1=1 %s
				GROUP BY user_id
			) combined
			GROUP BY user_id
		`, periodFilter, periodFilter)
	case "requests":
		return fmt.Sprintf(`
			SELECT fulfiller_id AS user_id, COUNT(*) AS count
			FROM requests
			WHERE fulfiller_id IS NOT NULL %s
			GROUP BY fulfiller_id
		`, periodFilter)
	default:
		// Should never happen — validated before call
		return "SELECT 0 AS user_id, 0 AS count WHERE 1=0"
	}
}

// buildPeriodFilter returns a SQL AND clause for date filtering.
func buildPeriodFilter(period string) string {
	switch period {
	case "month":
		return "AND created_at >= date_trunc('month', NOW())"
	case "week":
		return "AND created_at >= date_trunc('week', NOW())"
	default:
		return ""
	}
}
