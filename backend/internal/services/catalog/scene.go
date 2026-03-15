package catalog

import (
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"

	"psychic-homily-backend/db"
	"psychic-homily-backend/internal/models"
	"psychic-homily-backend/internal/services/contracts"
)

// SceneService handles computed city-level aggregations for "scene" pages.
// No new tables — all data is derived from existing venue, show, and artist tables.
type SceneService struct {
	db *gorm.DB
}

// NewSceneService creates a new scene service.
func NewSceneService(database *gorm.DB) *SceneService {
	if database == nil {
		database = db.GetDB()
	}
	return &SceneService{db: database}
}

// Thresholds for a city to qualify as a "scene".
const (
	sceneMinVenues = 3
	sceneMinShows  = 5
)

// ListScenes returns cities that meet scene thresholds:
// 3+ verified venues AND 5+ upcoming approved shows.
func (s *SceneService) ListScenes() ([]*contracts.SceneListResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	now := time.Now().UTC()

	type cityRow struct {
		City              string `gorm:"column:city"`
		State             string `gorm:"column:state"`
		VenueCount        int    `gorm:"column:venue_count"`
		UpcomingShowCount int    `gorm:"column:upcoming_show_count"`
	}

	// Step 1: Find cities with 3+ verified venues.
	var cities []cityRow
	err := s.db.Raw(`
		SELECT v.city, v.state, COUNT(DISTINCT v.id) AS venue_count
		FROM venues v
		WHERE v.verified = true
		  AND v.city IS NOT NULL AND v.city != ''
		  AND v.state IS NOT NULL AND v.state != ''
		GROUP BY v.city, v.state
		HAVING COUNT(DISTINCT v.id) >= ?
	`, sceneMinVenues).Scan(&cities).Error
	if err != nil {
		return nil, fmt.Errorf("failed to list scenes: %w", err)
	}

	// Step 2: For each qualifying city, count upcoming approved shows.
	var results []*contracts.SceneListResponse
	for i := range cities {
		c := &cities[i]
		var showCount int64
		err := s.db.Raw(`
			SELECT COUNT(DISTINCT s.id)
			FROM shows s
			JOIN show_venues sv ON sv.show_id = s.id
			JOIN venues v ON v.id = sv.venue_id
			WHERE v.city = ? AND v.state = ?
			  AND s.status = ?
			  AND s.event_date >= ?
		`, c.City, c.State, models.ShowStatusApproved, now).Scan(&showCount).Error
		if err != nil {
			return nil, fmt.Errorf("failed to count shows for %s, %s: %w", c.City, c.State, err)
		}

		if int(showCount) < sceneMinShows {
			continue
		}

		results = append(results, &contracts.SceneListResponse{
			City:              c.City,
			State:             c.State,
			Slug:              buildSceneSlug(c.City, c.State),
			VenueCount:        c.VenueCount,
			UpcomingShowCount: int(showCount),
		})
	}

	// Sort by upcoming show count descending (do in Go since we already filtered).
	// Simple insertion sort is fine for a small number of cities.
	for i := 1; i < len(results); i++ {
		for j := i; j > 0 && results[j].UpcomingShowCount > results[j-1].UpcomingShowCount; j-- {
			results[j], results[j-1] = results[j-1], results[j]
		}
	}

	return results, nil
}

// GetSceneDetail returns computed aggregation stats and pulse for a city.
func (s *SceneService) GetSceneDetail(city, state string) (*contracts.SceneDetailResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	now := time.Now().UTC()

	// Venue count (verified only)
	var venueCount int64
	if err := s.db.Model(&models.Venue{}).
		Where("city = ? AND state = ? AND verified = true", city, state).
		Count(&venueCount).Error; err != nil {
		return nil, fmt.Errorf("failed to count venues: %w", err)
	}
	if venueCount < sceneMinVenues {
		return nil, fmt.Errorf("scene not found: %s, %s", city, state)
	}

	// Upcoming show count
	var upcomingShowCount int64
	if err := s.db.Raw(`
		SELECT COUNT(DISTINCT s.id)
		FROM shows s
		JOIN show_venues sv ON sv.show_id = s.id
		JOIN venues v ON v.id = sv.venue_id
		WHERE v.city = ? AND v.state = ?
		  AND s.status = ?
		  AND s.event_date >= ?
	`, city, state, models.ShowStatusApproved, now).Scan(&upcomingShowCount).Error; err != nil {
		return nil, fmt.Errorf("failed to count upcoming shows: %w", err)
	}

	// Artist count: distinct artists with approved shows at venues in this city
	var artistCount int64
	if err := s.db.Raw(`
		SELECT COUNT(DISTINCT sa.artist_id)
		FROM show_artists sa
		JOIN shows s ON s.id = sa.show_id
		JOIN show_venues sv ON sv.show_id = s.id
		JOIN venues v ON v.id = sv.venue_id
		WHERE v.city = ? AND v.state = ?
		  AND s.status = ?
	`, city, state, models.ShowStatusApproved).Scan(&artistCount).Error; err != nil {
		return nil, fmt.Errorf("failed to count artists: %w", err)
	}

	// Festival count: festivals with matching city
	var festivalCount int64
	if err := s.db.Model(&models.Festival{}).
		Where("city = ? AND state = ?", city, state).
		Count(&festivalCount).Error; err != nil {
		return nil, fmt.Errorf("failed to count festivals: %w", err)
	}

	// ── Pulse computations ──

	// Current month boundaries
	thisMonthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	nextMonthStart := thisMonthStart.AddDate(0, 1, 0)
	prevMonthStart := thisMonthStart.AddDate(0, -1, 0)

	// Shows this month
	var showsThisMonth int64
	s.db.Raw(`
		SELECT COUNT(DISTINCT s.id)
		FROM shows s
		JOIN show_venues sv ON sv.show_id = s.id
		JOIN venues v ON v.id = sv.venue_id
		WHERE v.city = ? AND v.state = ?
		  AND s.status = ?
		  AND s.event_date >= ? AND s.event_date < ?
	`, city, state, models.ShowStatusApproved, thisMonthStart, nextMonthStart).Scan(&showsThisMonth)

	// Shows previous month
	var showsPrevMonth int64
	s.db.Raw(`
		SELECT COUNT(DISTINCT s.id)
		FROM shows s
		JOIN show_venues sv ON sv.show_id = s.id
		JOIN venues v ON v.id = sv.venue_id
		WHERE v.city = ? AND v.state = ?
		  AND s.status = ?
		  AND s.event_date >= ? AND s.event_date < ?
	`, city, state, models.ShowStatusApproved, prevMonthStart, thisMonthStart).Scan(&showsPrevMonth)

	// Trend string
	diff := int(showsThisMonth) - int(showsPrevMonth)
	showsTrend := "0"
	if diff > 0 {
		showsTrend = fmt.Sprintf("+%d", diff)
	} else if diff < 0 {
		showsTrend = fmt.Sprintf("%d", diff)
	}

	// New artists in last 30 days: artists whose first show in this city was in last 30 days
	thirtyDaysAgo := now.AddDate(0, 0, -30)
	var newArtists30d int64
	s.db.Raw(`
		SELECT COUNT(*)
		FROM (
			SELECT sa.artist_id, MIN(s.event_date) AS first_show
			FROM show_artists sa
			JOIN shows s ON s.id = sa.show_id
			JOIN show_venues sv ON sv.show_id = s.id
			JOIN venues v ON v.id = sv.venue_id
			WHERE v.city = ? AND v.state = ?
			  AND s.status = ?
			GROUP BY sa.artist_id
			HAVING MIN(s.event_date) >= ?
		) AS new_artists
	`, city, state, models.ShowStatusApproved, thirtyDaysAgo).Scan(&newArtists30d)

	// Active venues this month: venues with at least 1 approved show this month
	var activeVenuesThisMonth int64
	s.db.Raw(`
		SELECT COUNT(DISTINCT v.id)
		FROM venues v
		JOIN show_venues sv ON sv.venue_id = v.id
		JOIN shows s ON s.id = sv.show_id
		WHERE v.city = ? AND v.state = ?
		  AND s.status = ?
		  AND s.event_date >= ? AND s.event_date < ?
	`, city, state, models.ShowStatusApproved, thisMonthStart, nextMonthStart).Scan(&activeVenuesThisMonth)

	// Shows by month: last 6 months (from 5 months ago through current month)
	showsByMonth := make([]int, 6)
	for i := 5; i >= 0; i-- {
		monthStart := thisMonthStart.AddDate(0, -i, 0)
		monthEnd := monthStart.AddDate(0, 1, 0)
		var count int64
		s.db.Raw(`
			SELECT COUNT(DISTINCT s.id)
			FROM shows s
			JOIN show_venues sv ON sv.show_id = s.id
			JOIN venues v ON v.id = sv.venue_id
			WHERE v.city = ? AND v.state = ?
			  AND s.status = ?
			  AND s.event_date >= ? AND s.event_date < ?
		`, city, state, models.ShowStatusApproved, monthStart, monthEnd).Scan(&count)
		showsByMonth[5-i] = int(count)
	}

	return &contracts.SceneDetailResponse{
		City:        city,
		State:       state,
		Slug:        buildSceneSlug(city, state),
		Description: nil, // nil until scenes table exists
		Stats: contracts.SceneStats{
			VenueCount:        int(venueCount),
			ArtistCount:       int(artistCount),
			UpcomingShowCount: int(upcomingShowCount),
			FestivalCount:     int(festivalCount),
		},
		Pulse: contracts.ScenePulse{
			ShowsThisMonth:        int(showsThisMonth),
			ShowsPrevMonth:        int(showsPrevMonth),
			ShowsTrend:            showsTrend,
			NewArtists30d:         int(newArtists30d),
			ActiveVenuesThisMonth: int(activeVenuesThisMonth),
			ShowsByMonth:          showsByMonth,
		},
	}, nil
}

// GetActiveArtists returns artists ranked by show count in a city within the given period.
func (s *SceneService) GetActiveArtists(city, state string, periodDays, limit, offset int) ([]*contracts.SceneArtistResponse, int64, error) {
	if s.db == nil {
		return nil, 0, fmt.Errorf("database not initialized")
	}

	// Verify city qualifies as scene
	var venueCount int64
	if err := s.db.Model(&models.Venue{}).
		Where("city = ? AND state = ? AND verified = true", city, state).
		Count(&venueCount).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to count venues: %w", err)
	}
	if venueCount < sceneMinVenues {
		return nil, 0, fmt.Errorf("scene not found: %s, %s", city, state)
	}

	cutoff := time.Now().UTC().AddDate(0, 0, -periodDays)

	// Count total distinct artists
	var total int64
	if err := s.db.Raw(`
		SELECT COUNT(DISTINCT sa.artist_id)
		FROM show_artists sa
		JOIN shows s ON s.id = sa.show_id
		JOIN show_venues sv ON sv.show_id = s.id
		JOIN venues v ON v.id = sv.venue_id
		WHERE v.city = ? AND v.state = ?
		  AND s.status = ?
		  AND s.event_date >= ?
	`, city, state, models.ShowStatusApproved, cutoff).Scan(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to count active artists: %w", err)
	}

	type artistRow struct {
		ID        uint    `gorm:"column:id"`
		Slug      *string `gorm:"column:slug"`
		Name      string  `gorm:"column:name"`
		City      *string `gorm:"column:city"`
		State     *string `gorm:"column:state"`
		ShowCount int     `gorm:"column:show_count"`
	}

	var rows []artistRow
	if err := s.db.Raw(`
		SELECT a.id, a.slug, a.name, a.city, a.state, COUNT(DISTINCT s.id) AS show_count
		FROM artists a
		JOIN show_artists sa ON sa.artist_id = a.id
		JOIN shows s ON s.id = sa.show_id
		JOIN show_venues sv ON sv.show_id = s.id
		JOIN venues v ON v.id = sv.venue_id
		WHERE v.city = ? AND v.state = ?
		  AND s.status = ?
		  AND s.event_date >= ?
		GROUP BY a.id
		ORDER BY show_count DESC, a.name ASC
		LIMIT ? OFFSET ?
	`, city, state, models.ShowStatusApproved, cutoff, limit, offset).Scan(&rows).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to get active artists: %w", err)
	}

	results := make([]*contracts.SceneArtistResponse, len(rows))
	for i, r := range rows {
		slug := ""
		if r.Slug != nil {
			slug = *r.Slug
		}
		results[i] = &contracts.SceneArtistResponse{
			ID:        r.ID,
			Slug:      slug,
			Name:      r.Name,
			City:      r.City,
			State:     r.State,
			ShowCount: r.ShowCount,
		}
	}

	return results, total, nil
}

// ParseSceneSlug resolves a slug like "phoenix-az" to actual city and state
// by matching against verified venues in the database.
func (s *SceneService) ParseSceneSlug(slug string) (string, string, error) {
	if s.db == nil {
		return "", "", fmt.Errorf("database not initialized")
	}

	type cityState struct {
		City  string
		State string
	}

	var result cityState
	err := s.db.Raw(`
		SELECT DISTINCT city, state
		FROM venues
		WHERE verified = true
		  AND LOWER(REPLACE(city, ' ', '-')) || '-' || LOWER(state) = ?
		LIMIT 1
	`, strings.ToLower(slug)).Scan(&result).Error
	if err != nil {
		return "", "", fmt.Errorf("failed to resolve scene slug: %w", err)
	}
	if result.City == "" {
		return "", "", fmt.Errorf("scene not found for slug: %s", slug)
	}

	return result.City, result.State, nil
}

// buildSceneSlug generates a URL-safe slug from city and state.
// Example: "Phoenix", "AZ" → "phoenix-az"
func buildSceneSlug(city, state string) string {
	slug := strings.ToLower(city)
	slug = strings.ReplaceAll(slug, " ", "-")
	slug = slug + "-" + strings.ToLower(state)
	return slug
}
