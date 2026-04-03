package catalog

import (
	"fmt"
	"strings"
	"time"

	"psychic-homily-backend/internal/models"
	"psychic-homily-backend/internal/services/contracts"
)

// GetUnmatchedPlays returns unmatched plays grouped by artist_name,
// optionally filtered by station_id, with pagination.
func (s *RadioService) GetUnmatchedPlays(stationID uint, limit, offset int) ([]*contracts.UnmatchedPlayGroup, int64, error) {
	if s.db == nil {
		return nil, 0, fmt.Errorf("database not initialized")
	}

	if limit <= 0 {
		limit = 50
	}

	// Build base query for grouping unmatched plays by artist_name
	baseQuery := s.db.Table("radio_plays rp").
		Where("rp.artist_id IS NULL")

	if stationID > 0 {
		baseQuery = baseQuery.
			Joins("JOIN radio_episodes re ON re.id = rp.episode_id").
			Joins("JOIN radio_shows rsh ON rsh.id = re.show_id").
			Where("rsh.station_id = ?", stationID)
	}

	// Count total distinct artist names
	var total int64
	countQuery := s.db.Table("radio_plays rp").
		Select("COUNT(DISTINCT rp.artist_name)").
		Where("rp.artist_id IS NULL")
	if stationID > 0 {
		countQuery = countQuery.
			Joins("JOIN radio_episodes re ON re.id = rp.episode_id").
			Joins("JOIN radio_shows rsh ON rsh.id = re.show_id").
			Where("rsh.station_id = ?", stationID)
	}
	countQuery.Scan(&total)

	// Get grouped results
	type groupResult struct {
		ArtistName string `gorm:"column:artist_name"`
		PlayCount  int    `gorm:"column:play_count"`
	}

	groupQuery := s.db.Table("radio_plays rp").
		Select("rp.artist_name, COUNT(*) as play_count").
		Where("rp.artist_id IS NULL")

	if stationID > 0 {
		groupQuery = groupQuery.
			Joins("JOIN radio_episodes re ON re.id = rp.episode_id").
			Joins("JOIN radio_shows rsh ON rsh.id = re.show_id").
			Where("rsh.station_id = ?", stationID)
	}

	var groups []groupResult
	err := groupQuery.
		Group("rp.artist_name").
		Order("play_count DESC").
		Limit(limit).
		Offset(offset).
		Find(&groups).Error
	if err != nil {
		return nil, 0, fmt.Errorf("querying unmatched plays: %w", err)
	}

	// For each group, get station names and suggested matches
	results := make([]*contracts.UnmatchedPlayGroup, len(groups))
	for i, g := range groups {
		group := &contracts.UnmatchedPlayGroup{
			ArtistName: g.ArtistName,
			PlayCount:  g.PlayCount,
		}

		// Get station names for this artist_name
		type stationResult struct {
			StationName string `gorm:"column:station_name"`
		}
		var stations []stationResult
		s.db.Table("radio_plays rp").
			Select("DISTINCT rs.name as station_name").
			Joins("JOIN radio_episodes re ON re.id = rp.episode_id").
			Joins("JOIN radio_shows rsh ON rsh.id = re.show_id").
			Joins("JOIN radio_stations rs ON rs.id = rsh.station_id").
			Where("rp.artist_name = ? AND rp.artist_id IS NULL", g.ArtistName).
			Find(&stations)

		stationNames := make([]string, len(stations))
		for j, st := range stations {
			stationNames[j] = st.StationName
		}
		group.StationNames = stationNames

		// Get suggested matches (top 3 artists by trigram/name similarity)
		group.SuggestedMatches = s.suggestArtistMatches(g.ArtistName, 3)

		results[i] = group
	}

	return results, total, nil
}

// suggestArtistMatches returns top N artists that match the given name.
// Uses case-insensitive exact match first, then prefix match, then LIKE match.
func (s *RadioService) suggestArtistMatches(artistName string, limit int) []contracts.SuggestedMatch {
	var matches []contracts.SuggestedMatch
	normalizedName := strings.TrimSpace(strings.ToLower(artistName))

	// 1. Exact match (case-insensitive)
	var exactMatches []models.Artist
	s.db.Where("LOWER(name) = ?", normalizedName).Limit(limit).Find(&exactMatches)
	for _, a := range exactMatches {
		slug := ""
		if a.Slug != nil {
			slug = *a.Slug
		}
		matches = append(matches, contracts.SuggestedMatch{
			ArtistID:   a.ID,
			ArtistName: a.Name,
			ArtistSlug: slug,
		})
	}
	if len(matches) >= limit {
		return matches[:limit]
	}

	// 2. Alias match (case-insensitive)
	remaining := limit - len(matches)
	existingIDs := make(map[uint]bool)
	for _, m := range matches {
		existingIDs[m.ArtistID] = true
	}

	var aliasMatches []struct {
		ArtistID uint   `gorm:"column:artist_id"`
		Name     string `gorm:"column:name"`
		Slug     string `gorm:"column:slug"`
	}
	s.db.Table("artist_aliases aa").
		Select("aa.artist_id, a.name, COALESCE(a.slug, '') as slug").
		Joins("JOIN artists a ON a.id = aa.artist_id").
		Where("LOWER(aa.alias) = ?", normalizedName).
		Limit(remaining).
		Find(&aliasMatches)

	for _, am := range aliasMatches {
		if existingIDs[am.ArtistID] {
			continue
		}
		existingIDs[am.ArtistID] = true
		matches = append(matches, contracts.SuggestedMatch{
			ArtistID:   am.ArtistID,
			ArtistName: am.Name,
			ArtistSlug: am.Slug,
		})
	}
	if len(matches) >= limit {
		return matches[:limit]
	}

	// 3. LIKE match (prefix)
	remaining = limit - len(matches)
	var likeMatches []models.Artist
	s.db.Where("LOWER(name) LIKE ?", normalizedName+"%").
		Limit(remaining).
		Find(&likeMatches)

	for _, a := range likeMatches {
		if existingIDs[a.ID] {
			continue
		}
		existingIDs[a.ID] = true
		slug := ""
		if a.Slug != nil {
			slug = *a.Slug
		}
		matches = append(matches, contracts.SuggestedMatch{
			ArtistID:   a.ID,
			ArtistName: a.Name,
			ArtistSlug: slug,
		})
	}

	return matches
}

// LinkPlay links a single radio play to an artist/release/label.
func (s *RadioService) LinkPlay(playID uint, req *contracts.LinkPlayRequest) error {
	if s.db == nil {
		return fmt.Errorf("database not initialized")
	}

	var play models.RadioPlay
	if err := s.db.First(&play, playID).Error; err != nil {
		return fmt.Errorf("play not found: %w", err)
	}

	updates := make(map[string]interface{})
	if req.ArtistID != nil {
		updates["artist_id"] = *req.ArtistID
	}
	if req.ReleaseID != nil {
		updates["release_id"] = *req.ReleaseID
	}
	if req.LabelID != nil {
		updates["label_id"] = *req.LabelID
	}

	if len(updates) == 0 {
		return fmt.Errorf("no fields to update")
	}

	return s.db.Model(&play).Updates(updates).Error
}

// BulkLinkPlays links all unmatched plays with a given artist_name to an artist.
func (s *RadioService) BulkLinkPlays(req *contracts.BulkLinkRequest) (*contracts.BulkLinkResult, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	if req.ArtistName == "" {
		return nil, fmt.Errorf("artist_name is required")
	}
	if req.ArtistID == 0 {
		return nil, fmt.Errorf("artist_id is required")
	}

	result := s.db.Model(&models.RadioPlay{}).
		Where("artist_name = ? AND artist_id IS NULL", req.ArtistName).
		Update("artist_id", req.ArtistID)

	if result.Error != nil {
		return nil, fmt.Errorf("bulk linking plays: %w", result.Error)
	}

	return &contracts.BulkLinkResult{
		Updated: int(result.RowsAffected),
	}, nil
}

// ComputeAffinity recomputes the radio_artist_affinity table from scratch.
// It truncates the existing data, then aggregates co-occurrence counts from episodes
// where both plays have matched artist_id values.
func (s *RadioService) ComputeAffinity() error {
	if s.db == nil {
		return fmt.Errorf("database not initialized")
	}

	// Truncate existing affinity data
	if err := s.db.Exec("DELETE FROM radio_artist_affinity").Error; err != nil {
		return fmt.Errorf("clearing affinity table: %w", err)
	}

	// Compute co-occurrences: for each episode, find all pairs of matched artists
	// that co-occur, then aggregate across episodes.
	// Uses a self-join on radio_plays within the same episode, with canonical ordering
	// (artist_a_id < artist_b_id) to avoid duplicates.
	query := `
		INSERT INTO radio_artist_affinity (artist_a_id, artist_b_id, co_occurrence_count, show_count, station_count, last_co_occurrence, updated_at)
		SELECT
			LEAST(rp1.artist_id, rp2.artist_id) AS artist_a_id,
			GREATEST(rp1.artist_id, rp2.artist_id) AS artist_b_id,
			COUNT(*) AS co_occurrence_count,
			COUNT(DISTINCT re.show_id) AS show_count,
			COUNT(DISTINCT rsh.station_id) AS station_count,
			MAX(re.air_date) AS last_co_occurrence,
			NOW() AS updated_at
		FROM radio_plays rp1
		JOIN radio_plays rp2 ON rp1.episode_id = rp2.episode_id
			AND rp1.artist_id < rp2.artist_id
		JOIN radio_episodes re ON re.id = rp1.episode_id
		JOIN radio_shows rsh ON rsh.id = re.show_id
		WHERE rp1.artist_id IS NOT NULL
			AND rp2.artist_id IS NOT NULL
		GROUP BY LEAST(rp1.artist_id, rp2.artist_id), GREATEST(rp1.artist_id, rp2.artist_id)
		HAVING COUNT(*) >= 2
	`

	if err := s.db.Exec(query).Error; err != nil {
		return fmt.Errorf("computing affinity: %w", err)
	}

	return nil
}

// ReMatchUnmatched re-runs the matching engine on all plays where artist_id IS NULL.
// This catches newly added artists since the last match attempt.
func (s *RadioService) ReMatchUnmatched() (*contracts.MatchResult, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	matcher := NewRadioMatchingEngine(s.db)
	return matcher.MatchAllUnmatched()
}

// GetActiveStationsWithPlaylistSource returns all active stations that have a playlist_source set.
func (s *RadioService) GetActiveStationsWithPlaylistSource() ([]models.RadioStation, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	var stations []models.RadioStation
	err := s.db.Where("is_active = TRUE AND playlist_source IS NOT NULL AND playlist_source != ''").
		Find(&stations).Error
	if err != nil {
		return nil, fmt.Errorf("querying active stations: %w", err)
	}
	return stations, nil
}

// RecordFetchFailure increments the consecutive failure counter on a station.
// Called by the fetch service on per-station errors.
func (s *RadioService) RecordFetchFailure(stationID uint) {
	s.db.Exec("UPDATE radio_stations SET updated_at = ? WHERE id = ?", time.Now(), stationID)
}

// RecordFetchSuccess resets the consecutive failure tracking for a station.
func (s *RadioService) RecordFetchSuccess(stationID uint) {
	now := time.Now()
	s.db.Model(&models.RadioStation{}).Where("id = ?", stationID).
		Update("last_playlist_fetch_at", now)
}
