package catalog

import (
	"fmt"
	"time"

	"gorm.io/gorm"

	"psychic-homily-backend/db"
	"psychic-homily-backend/internal/models"
	"psychic-homily-backend/internal/services/contracts"
)

// ChartsService computes top charts / trending content from engagement signals.
// No new tables — all data is derived from existing bookmark, show, artist, venue,
// and release tables.
type ChartsService struct {
	db *gorm.DB
}

// NewChartsService creates a new charts service.
func NewChartsService(database *gorm.DB) *ChartsService {
	if database == nil {
		database = db.GetDB()
	}
	return &ChartsService{db: database}
}

// GetTrendingShows returns upcoming shows ranked by total attendance (going + interested).
// Only includes future shows with approved status. Shows without engagement data are
// included and ranked by soonest date, so the chart is never empty when shows exist.
func (s *ChartsService) GetTrendingShows(limit int) ([]contracts.TrendingShow, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	now := time.Now().UTC()

	type trendingRow struct {
		ShowID          uint      `gorm:"column:show_id"`
		Title           string    `gorm:"column:title"`
		Slug            string    `gorm:"column:slug"`
		Date            time.Time `gorm:"column:event_date"`
		VenueName       string    `gorm:"column:venue_name"`
		VenueSlug       string    `gorm:"column:venue_slug"`
		City            string    `gorm:"column:city"`
		GoingCount      int       `gorm:"column:going_count"`
		InterestedCount int       `gorm:"column:interested_count"`
		TotalAttendance int       `gorm:"column:total_attendance"`
	}

	var rows []trendingRow
	err := s.db.Raw(`
		SELECT
			s.id AS show_id,
			s.title,
			COALESCE(s.slug, '') AS slug,
			s.event_date,
			COALESCE(v.name, '') AS venue_name,
			COALESCE(v.slug, '') AS venue_slug,
			COALESCE(v.city, '') AS city,
			COALESCE(SUM(CASE WHEN ub.action = 'going' THEN 1 ELSE 0 END), 0) AS going_count,
			COALESCE(SUM(CASE WHEN ub.action = 'interested' THEN 1 ELSE 0 END), 0) AS interested_count,
			COALESCE(COUNT(ub.id), 0) AS total_attendance
		FROM shows s
		LEFT JOIN show_venues sv ON sv.show_id = s.id
		LEFT JOIN venues v ON v.id = sv.venue_id
		LEFT JOIN user_bookmarks ub ON ub.entity_id = s.id
			AND ub.entity_type = ?
			AND ub.action IN (?, ?)
		WHERE s.status = ?
			AND s.event_date >= ?
		GROUP BY s.id, s.title, s.slug, s.event_date, v.name, v.slug, v.city
		ORDER BY total_attendance DESC, s.event_date ASC
		LIMIT ?
	`, models.BookmarkEntityShow, models.BookmarkActionGoing, models.BookmarkActionInterested,
		models.ShowStatusApproved, now, limit).Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("failed to get trending shows: %w", err)
	}

	results := make([]contracts.TrendingShow, len(rows))
	for i, r := range rows {
		results[i] = contracts.TrendingShow{
			ShowID:          r.ShowID,
			Title:           r.Title,
			Slug:            r.Slug,
			Date:            r.Date,
			VenueName:       r.VenueName,
			VenueSlug:       r.VenueSlug,
			City:            r.City,
			GoingCount:      r.GoingCount,
			InterestedCount: r.InterestedCount,
			TotalAttendance: r.TotalAttendance,
		}
	}

	return results, nil
}

// GetPopularArtists returns artists ranked by a composite score of followers and upcoming shows.
// Score = follow_count * 2 + upcoming_show_count.
func (s *ChartsService) GetPopularArtists(limit int) ([]contracts.PopularArtist, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	now := time.Now().UTC()

	type artistRow struct {
		ArtistID          uint   `gorm:"column:artist_id"`
		Name              string `gorm:"column:name"`
		Slug              string `gorm:"column:slug"`
		ImageURL          string `gorm:"column:image_url"`
		FollowCount       int    `gorm:"column:follow_count"`
		UpcomingShowCount int    `gorm:"column:upcoming_show_count"`
		Score             int    `gorm:"column:score"`
	}

	var rows []artistRow
	err := s.db.Raw(`
		SELECT
			a.id AS artist_id,
			a.name,
			COALESCE(a.slug, '') AS slug,
			COALESCE(a.bandcamp_embed_url, '') AS image_url,
			COALESCE(follow_counts.cnt, 0) AS follow_count,
			COALESCE(show_counts.cnt, 0) AS upcoming_show_count,
			(COALESCE(follow_counts.cnt, 0) * 2 + COALESCE(show_counts.cnt, 0)) AS score
		FROM artists a
		LEFT JOIN (
			SELECT entity_id, COUNT(*) AS cnt
			FROM user_bookmarks
			WHERE entity_type = ? AND action = ?
			GROUP BY entity_id
		) follow_counts ON follow_counts.entity_id = a.id
		LEFT JOIN (
			SELECT sa.artist_id, COUNT(DISTINCT s.id) AS cnt
			FROM show_artists sa
			JOIN shows s ON s.id = sa.show_id
			WHERE s.status = ? AND s.event_date >= ?
			GROUP BY sa.artist_id
		) show_counts ON show_counts.artist_id = a.id
		WHERE (COALESCE(follow_counts.cnt, 0) > 0 OR COALESCE(show_counts.cnt, 0) > 0)
		ORDER BY score DESC, a.name ASC
		LIMIT ?
	`, models.BookmarkEntityArtist, models.BookmarkActionFollow,
		models.ShowStatusApproved, now, limit).Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("failed to get popular artists: %w", err)
	}

	results := make([]contracts.PopularArtist, len(rows))
	for i, r := range rows {
		results[i] = contracts.PopularArtist{
			ArtistID:          r.ArtistID,
			Name:              r.Name,
			Slug:              r.Slug,
			ImageURL:          r.ImageURL,
			FollowCount:       r.FollowCount,
			UpcomingShowCount: r.UpcomingShowCount,
			Score:             r.Score,
		}
	}

	return results, nil
}

// GetActiveVenues returns venues ranked by a composite score of upcoming shows and followers.
// Score = upcoming_show_count * 2 + follow_count.
func (s *ChartsService) GetActiveVenues(limit int) ([]contracts.ActiveVenue, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	now := time.Now().UTC()

	type venueRow struct {
		VenueID           uint   `gorm:"column:venue_id"`
		Name              string `gorm:"column:name"`
		Slug              string `gorm:"column:slug"`
		City              string `gorm:"column:city"`
		State             string `gorm:"column:state"`
		UpcomingShowCount int    `gorm:"column:upcoming_show_count"`
		FollowCount       int    `gorm:"column:follow_count"`
		Score             int    `gorm:"column:score"`
	}

	var rows []venueRow
	err := s.db.Raw(`
		SELECT
			v.id AS venue_id,
			v.name,
			COALESCE(v.slug, '') AS slug,
			COALESCE(v.city, '') AS city,
			COALESCE(v.state, '') AS state,
			COALESCE(show_counts.cnt, 0) AS upcoming_show_count,
			COALESCE(follow_counts.cnt, 0) AS follow_count,
			(COALESCE(show_counts.cnt, 0) * 2 + COALESCE(follow_counts.cnt, 0)) AS score
		FROM venues v
		LEFT JOIN (
			SELECT sv.venue_id, COUNT(DISTINCT s.id) AS cnt
			FROM show_venues sv
			JOIN shows s ON s.id = sv.show_id
			WHERE s.status = ? AND s.event_date >= ?
			GROUP BY sv.venue_id
		) show_counts ON show_counts.venue_id = v.id
		LEFT JOIN (
			SELECT entity_id, COUNT(*) AS cnt
			FROM user_bookmarks
			WHERE entity_type = ? AND action = ?
			GROUP BY entity_id
		) follow_counts ON follow_counts.entity_id = v.id
		WHERE (COALESCE(show_counts.cnt, 0) > 0 OR COALESCE(follow_counts.cnt, 0) > 0)
		ORDER BY score DESC, v.name ASC
		LIMIT ?
	`, models.ShowStatusApproved, now,
		models.BookmarkEntityVenue, models.BookmarkActionFollow, limit).Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("failed to get active venues: %w", err)
	}

	results := make([]contracts.ActiveVenue, len(rows))
	for i, r := range rows {
		results[i] = contracts.ActiveVenue{
			VenueID:           r.VenueID,
			Name:              r.Name,
			Slug:              r.Slug,
			City:              r.City,
			State:             r.State,
			UpcomingShowCount: r.UpcomingShowCount,
			FollowCount:       r.FollowCount,
			Score:             r.Score,
		}
	}

	return results, nil
}

// GetHotReleases returns releases ranked by recent bookmark count, falling back to
// recently added releases when no bookmarks exist so the chart is never empty.
func (s *ChartsService) GetHotReleases(limit int) ([]contracts.HotRelease, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	thirtyDaysAgo := time.Now().UTC().AddDate(0, 0, -30)

	type releaseRow struct {
		ReleaseID     uint    `gorm:"column:release_id"`
		Title         string  `gorm:"column:title"`
		Slug          string  `gorm:"column:slug"`
		ReleaseDate   *string `gorm:"column:release_date"`
		BookmarkCount int     `gorm:"column:bookmark_count"`
	}

	var rows []releaseRow
	err := s.db.Raw(`
		SELECT
			r.id AS release_id,
			r.title,
			COALESCE(r.slug, '') AS slug,
			r.release_date,
			COALESCE(COUNT(ub.id), 0) AS bookmark_count
		FROM releases r
		LEFT JOIN user_bookmarks ub ON ub.entity_id = r.id
			AND ub.entity_type = ?
			AND ub.action = ?
			AND ub.created_at >= ?
		GROUP BY r.id, r.title, r.slug, r.release_date
		ORDER BY bookmark_count DESC, r.created_at DESC
		LIMIT ?
	`, models.BookmarkEntityRelease, models.BookmarkActionBookmark, thirtyDaysAgo, limit).Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("failed to get hot releases: %w", err)
	}

	// Build result list, then enrich with artist names
	results := make([]contracts.HotRelease, len(rows))
	releaseIDs := make([]uint, len(rows))
	for i, r := range rows {
		var releaseDate *time.Time
		if r.ReleaseDate != nil {
			if t, err := time.Parse("2006-01-02", *r.ReleaseDate); err == nil {
				releaseDate = &t
			}
		}
		results[i] = contracts.HotRelease{
			ReleaseID:     r.ReleaseID,
			Title:         r.Title,
			Slug:          r.Slug,
			ReleaseDate:   releaseDate,
			ArtistNames:   []string{},
			BookmarkCount: r.BookmarkCount,
		}
		releaseIDs[i] = r.ReleaseID
	}

	// Fetch artist names for all releases in one query
	if len(releaseIDs) > 0 {
		type artistNameRow struct {
			ReleaseID uint   `gorm:"column:release_id"`
			Name      string `gorm:"column:name"`
		}
		var artistRows []artistNameRow
		err := s.db.Raw(`
			SELECT ar.release_id, a.name
			FROM artist_releases ar
			JOIN artists a ON a.id = ar.artist_id
			WHERE ar.release_id IN ?
			ORDER BY ar.release_id, ar.position
		`, releaseIDs).Scan(&artistRows).Error
		if err != nil {
			return nil, fmt.Errorf("failed to get release artists: %w", err)
		}

		// Build map of release_id -> artist names
		artistMap := make(map[uint][]string)
		for _, ar := range artistRows {
			artistMap[ar.ReleaseID] = append(artistMap[ar.ReleaseID], ar.Name)
		}

		// Assign to results
		for i := range results {
			if names, ok := artistMap[results[i].ReleaseID]; ok {
				results[i].ArtistNames = names
			}
		}
	}

	return results, nil
}

// GetChartsOverview returns a condensed summary with top 5 of each chart.
func (s *ChartsService) GetChartsOverview() (*contracts.ChartsOverview, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	const overviewLimit = 5

	shows, err := s.GetTrendingShows(overviewLimit)
	if err != nil {
		return nil, fmt.Errorf("failed to get trending shows for overview: %w", err)
	}

	artists, err := s.GetPopularArtists(overviewLimit)
	if err != nil {
		return nil, fmt.Errorf("failed to get popular artists for overview: %w", err)
	}

	venues, err := s.GetActiveVenues(overviewLimit)
	if err != nil {
		return nil, fmt.Errorf("failed to get active venues for overview: %w", err)
	}

	releases, err := s.GetHotReleases(overviewLimit)
	if err != nil {
		return nil, fmt.Errorf("failed to get hot releases for overview: %w", err)
	}

	return &contracts.ChartsOverview{
		TrendingShows:  shows,
		PopularArtists: artists,
		ActiveVenues:   venues,
		HotReleases:    releases,
	}, nil
}
