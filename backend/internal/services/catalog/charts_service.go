package catalog

import (
	"fmt"
	"time"

	"gorm.io/gorm"

	"psychic-homily-backend/db"
	catalogm "psychic-homily-backend/internal/models/catalog"
	engagementm "psychic-homily-backend/internal/models/engagement"
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

// GetTrendingShows returns upcoming shows ranked by save count.
// Only includes future shows with approved status. Shows without engagement data are
// included and ranked by soonest date, so the chart is never empty when shows exist.
func (s *ChartsService) GetTrendingShows(limit int) ([]contracts.TrendingShow, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	now := time.Now().UTC()

	type trendingRow struct {
		ShowID    uint      `gorm:"column:show_id"`
		Title     string    `gorm:"column:title"`
		Slug      string    `gorm:"column:slug"`
		Date      time.Time `gorm:"column:event_date"`
		VenueName string    `gorm:"column:venue_name"`
		VenueSlug string    `gorm:"column:venue_slug"`
		City      string    `gorm:"column:city"`
		SaveCount int       `gorm:"column:save_count"`
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
			COALESCE(COUNT(ub.id), 0) AS save_count
		FROM shows s
		LEFT JOIN show_venues sv ON sv.show_id = s.id
		LEFT JOIN venues v ON v.id = sv.venue_id
		LEFT JOIN user_bookmarks ub ON ub.entity_id = s.id
			AND ub.entity_type = ?
			AND ub.action = ?
		WHERE s.status = ?
			AND s.event_date >= ?
		GROUP BY s.id, s.title, s.slug, s.event_date, v.name, v.slug, v.city
		ORDER BY save_count DESC, s.event_date ASC
		LIMIT ?
	`, engagementm.BookmarkEntityShow, engagementm.BookmarkActionSave,
		catalogm.ShowStatusApproved, now, limit).Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("failed to get trending shows: %w", err)
	}

	results := make([]contracts.TrendingShow, len(rows))
	showIDs := make([]uint, len(rows))
	for i, r := range rows {
		results[i] = contracts.TrendingShow{
			ShowID:      r.ShowID,
			Title:       r.Title,
			Slug:        r.Slug,
			Date:        r.Date,
			VenueName:   r.VenueName,
			VenueSlug:   r.VenueSlug,
			City:        r.City,
			ArtistNames: []string{},
			SaveCount:   r.SaveCount,
		}
		showIDs[i] = r.ShowID
	}

	// Fetch artist names for all shows in one query
	if len(showIDs) > 0 {
		type artistNameRow struct {
			ShowID uint   `gorm:"column:show_id"`
			Name   string `gorm:"column:name"`
		}
		var artistRows []artistNameRow
		err := s.db.Raw(`
			SELECT sa.show_id, a.name
			FROM show_artists sa
			JOIN artists a ON a.id = sa.artist_id
			WHERE sa.show_id IN ?
			ORDER BY sa.show_id, sa.position
		`, showIDs).Scan(&artistRows).Error
		if err != nil {
			return nil, fmt.Errorf("failed to get show artists: %w", err)
		}

		// Build map of show_id -> artist names
		artistMap := make(map[uint][]string)
		for _, ar := range artistRows {
			artistMap[ar.ShowID] = append(artistMap[ar.ShowID], ar.Name)
		}

		// Assign to results
		for i := range results {
			if names, ok := artistMap[results[i].ShowID]; ok {
				results[i].ArtistNames = names
			}
		}
	}

	return results, nil
}

// chartWindowStart returns the inclusive lower bound for a chart window, or
// nil for all-time. Defaulting for empty/unknown values is owned by
// ChartWindow.OrDefault so handler and service can't drift apart.
func chartWindowStart(window contracts.ChartWindow, now time.Time) *time.Time {
	days := 0
	switch window.OrDefault() {
	case contracts.ChartWindowMonth:
		days = 30
	case contracts.ChartWindowAllTime:
		return nil
	case contracts.ChartWindowQuarter:
		days = 90
	default:
		// Unreachable while OrDefault enumerates every window; keeps quarter
		// semantics if a new ChartWindow value lands there without a case here.
		days = 90
	}
	// Truncate to midnight UTC: event dates are midnight timestamps, so a
	// time-of-day lower bound would exclude the show exactly N days ago.
	t := now.AddDate(0, 0, -days).Truncate(24 * time.Hour)
	return &t
}

// GetMostActiveArtists returns artists ranked by approved, non-cancelled
// shows played within the window. Shows dated after now are excluded — but
// event dates are midnight timestamps, so a show later today already counts
// as played. Headline share uses the same predicate as the discovery
// pipeline: set_type = 'headliner' OR position = 0. Artists with zero shows
// in the window are never returned.
func (s *ChartsService) GetMostActiveArtists(window contracts.ChartWindow, limit int) ([]contracts.MostActiveArtist, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	now := time.Now().UTC()
	start := chartWindowStart(window, now)

	type artistRow struct {
		ArtistID      uint   `gorm:"column:artist_id"`
		Name          string `gorm:"column:name"`
		Slug          string `gorm:"column:slug"`
		City          string `gorm:"column:city"`
		State         string `gorm:"column:state"`
		ShowCount     int    `gorm:"column:show_count"`
		HeadlineCount int    `gorm:"column:headline_count"`
	}

	query := `
		SELECT
			a.id AS artist_id,
			a.name,
			COALESCE(a.slug, '') AS slug,
			COALESCE(a.city, '') AS city,
			COALESCE(a.state, '') AS state,
			COUNT(*) AS show_count,
			COALESCE(SUM(CASE WHEN sa.set_type = 'headliner' OR sa.position = 0 THEN 1 ELSE 0 END), 0) AS headline_count
		FROM show_artists sa
		JOIN artists a ON a.id = sa.artist_id
		JOIN shows s ON s.id = sa.show_id
		WHERE s.status = ?
			AND s.is_cancelled = FALSE
			AND s.event_date <= ?`
	args := []any{catalogm.ShowStatusApproved, now}
	if start != nil {
		query += `
			AND s.event_date >= ?`
		args = append(args, *start)
	}
	query += `
		GROUP BY a.id, a.name, a.slug, a.city, a.state
		ORDER BY show_count DESC, a.name ASC, a.id ASC
		LIMIT ?`
	args = append(args, limit)

	var rows []artistRow
	if err := s.db.Raw(query, args...).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("failed to get most-active artists: %w", err)
	}

	results := make([]contracts.MostActiveArtist, len(rows))
	artistIDs := make([]uint, len(rows))
	for i, r := range rows {
		headlinePct := 0
		if r.ShowCount > 0 {
			headlinePct = int(float64(r.HeadlineCount)/float64(r.ShowCount)*100 + 0.5)
		}
		results[i] = contracts.MostActiveArtist{
			ArtistID:    r.ArtistID,
			Name:        r.Name,
			Slug:        r.Slug,
			City:        r.City,
			State:       r.State,
			ShowCount:   r.ShowCount,
			HeadlinePct: headlinePct,
		}
		artistIDs[i] = r.ArtistID
	}

	// Enrich with each artist's most recent show in the window (one query).
	if len(artistIDs) > 0 {
		type lastShowRow struct {
			ArtistID  uint      `gorm:"column:artist_id"`
			EventDate time.Time `gorm:"column:event_date"`
			ShowSlug  string    `gorm:"column:show_slug"`
			VenueName string    `gorm:"column:venue_name"`
		}
		lastQuery := `
			SELECT DISTINCT ON (sa.artist_id)
				sa.artist_id,
				s.event_date,
				COALESCE(s.slug, '') AS show_slug,
				COALESCE(v.name, '') AS venue_name
			FROM show_artists sa
			JOIN shows s ON s.id = sa.show_id
			LEFT JOIN show_venues sv ON sv.show_id = s.id
			LEFT JOIN venues v ON v.id = sv.venue_id
			WHERE sa.artist_id IN ?
				AND s.status = ?
				AND s.is_cancelled = FALSE
				AND s.event_date <= ?`
		lastArgs := []any{artistIDs, catalogm.ShowStatusApproved, now}
		if start != nil {
			lastQuery += `
				AND s.event_date >= ?`
			lastArgs = append(lastArgs, *start)
		}
		// s.id and v.name tiebreaks keep the picked row deterministic when an
		// artist plays two shows on one date or a show has multiple venue links.
		lastQuery += `
			ORDER BY sa.artist_id, s.event_date DESC, s.id DESC, v.name ASC`

		var lastRows []lastShowRow
		if err := s.db.Raw(lastQuery, lastArgs...).Scan(&lastRows).Error; err != nil {
			return nil, fmt.Errorf("failed to get last shows for most-active artists: %w", err)
		}

		lastByArtist := make(map[uint]lastShowRow, len(lastRows))
		for _, lr := range lastRows {
			lastByArtist[lr.ArtistID] = lr
		}
		for i := range results {
			if lr, ok := lastByArtist[results[i].ArtistID]; ok {
				date := lr.EventDate
				results[i].LastShowDate = &date
				results[i].LastShowSlug = lr.ShowSlug
				results[i].LastShowVenue = lr.VenueName
			}
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
	`, engagementm.BookmarkEntityArtist, engagementm.BookmarkActionFollow,
		catalogm.ShowStatusApproved, now, limit).Scan(&rows).Error
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
	`, catalogm.ShowStatusApproved, now,
		engagementm.BookmarkEntityVenue, engagementm.BookmarkActionFollow, limit).Scan(&rows).Error
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
	`, engagementm.BookmarkEntityRelease, engagementm.BookmarkActionBookmark, thirtyDaysAgo, limit).Scan(&rows).Error
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
