package catalog

import (
	"fmt"
	"time"

	"gorm.io/gorm"

	"psychic-homily-backend/db"
	apperrors "psychic-homily-backend/internal/errors"
	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/utils"
)

// RadioService handles radio station, show, episode, and play operations
type RadioService struct {
	db *gorm.DB
}

// NewRadioService creates a new radio service
func NewRadioService(database *gorm.DB) *RadioService {
	if database == nil {
		database = db.GetDB()
	}
	return &RadioService{db: database}
}

// =============================================================================
// Station CRUD
// =============================================================================

// CreateStation creates a new radio station
func (s *RadioService) CreateStation(req *contracts.CreateRadioStationRequest) (*contracts.RadioStationDetailResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	slug := req.Slug
	if slug == "" {
		baseSlug := utils.GenerateArtistSlug(req.Name)
		slug = utils.GenerateUniqueSlug(baseSlug, func(candidate string) bool {
			var count int64
			s.db.Model(&catalogm.RadioStation{}).Where("slug = ?", candidate).Count(&count)
			return count > 0
		})
	}

	if !catalogm.IsValidBroadcastType(req.BroadcastType) {
		return nil, fmt.Errorf("invalid broadcast type: %s", req.BroadcastType)
	}

	station := &catalogm.RadioStation{
		Name:             req.Name,
		Slug:             slug,
		Description:      req.Description,
		City:             req.City,
		State:            req.State,
		Country:          req.Country,
		Timezone:         req.Timezone,
		StreamURL:        req.StreamURL,
		StreamURLs:       req.StreamURLs,
		Website:          req.Website,
		DonationURL:      req.DonationURL,
		DonationEmbedURL: req.DonationEmbedURL,
		LogoURL:          req.LogoURL,
		Social:           req.Social,
		BroadcastType:    req.BroadcastType,
		FrequencyMHz:     req.FrequencyMHz,
		PlaylistSource:   req.PlaylistSource,
		PlaylistConfig:   req.PlaylistConfig,
	}

	if err := s.db.Create(station).Error; err != nil {
		return nil, fmt.Errorf("failed to create radio station: %w", err)
	}

	return s.GetStation(station.ID)
}

// GetStation retrieves a radio station by ID
func (s *RadioService) GetStation(stationID uint) (*contracts.RadioStationDetailResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	var station catalogm.RadioStation
	err := s.db.Preload("Network").First(&station, stationID).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, apperrors.ErrRadioStationNotFound(stationID)
		}
		return nil, fmt.Errorf("failed to get radio station: %w", err)
	}

	return s.buildStationDetailResponse(&station)
}

// GetStationBySlug retrieves a radio station by slug
func (s *RadioService) GetStationBySlug(slug string) (*contracts.RadioStationDetailResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	var station catalogm.RadioStation
	err := s.db.Preload("Network").Where("slug = ?", slug).First(&station).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, apperrors.ErrRadioStationNotFound(0)
		}
		return nil, fmt.Errorf("failed to get radio station: %w", err)
	}

	return s.buildStationDetailResponse(&station)
}

// ListStations retrieves radio stations with optional filtering
func (s *RadioService) ListStations(filters map[string]interface{}) ([]*contracts.RadioStationListResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	query := s.db.Model(&catalogm.RadioStation{})

	if isActive, ok := filters["is_active"].(bool); ok {
		query = query.Where("is_active = ?", isActive)
	}
	if city, ok := filters["city"].(string); ok && city != "" {
		query = query.Where("city = ?", city)
	}

	query = query.Order("name ASC")

	var stations []catalogm.RadioStation
	if err := query.Preload("Network").Find(&stations).Error; err != nil {
		return nil, fmt.Errorf("failed to list radio stations: %w", err)
	}

	// Batch-load show counts
	stationIDs := make([]uint, len(stations))
	for i, st := range stations {
		stationIDs[i] = st.ID
	}

	showCounts := make(map[uint]int)
	if len(stationIDs) > 0 {
		type countResult struct {
			StationID uint
			Count     int
		}
		var counts []countResult
		s.db.Model(&catalogm.RadioShow{}).
			Select("station_id, COUNT(*) as count").
			Where("station_id IN ?", stationIDs).
			Group("station_id").
			Find(&counts)

		for _, c := range counts {
			showCounts[c.StationID] = c.Count
		}
	}

	responses := make([]*contracts.RadioStationListResponse, len(stations))
	for i, st := range stations {
		var networkSlug *string
		if st.Network != nil {
			slug := st.Network.Slug
			networkSlug = &slug
		}
		responses[i] = &contracts.RadioStationListResponse{
			ID:            st.ID,
			Name:          st.Name,
			Slug:          st.Slug,
			City:          st.City,
			State:         st.State,
			Country:       st.Country,
			BroadcastType: st.BroadcastType,
			FrequencyMHz:  st.FrequencyMHz,
			LogoURL:       st.LogoURL,
			IsActive:      st.IsActive,
			NetworkID:     st.NetworkID,
			NetworkSlug:   networkSlug,
			ShowCount:     showCounts[st.ID],
		}
	}

	return responses, nil
}

// UpdateStation updates a radio station
func (s *RadioService) UpdateStation(stationID uint, req *contracts.UpdateRadioStationRequest) (*contracts.RadioStationDetailResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	var station catalogm.RadioStation
	if err := s.db.First(&station, stationID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, apperrors.ErrRadioStationNotFound(stationID)
		}
		return nil, fmt.Errorf("failed to get radio station: %w", err)
	}

	updates := make(map[string]interface{})
	if req.Name != nil {
		updates["name"] = *req.Name
	}
	if req.Description != nil {
		updates["description"] = *req.Description
	}
	if req.City != nil {
		updates["city"] = *req.City
	}
	if req.State != nil {
		updates["state"] = *req.State
	}
	if req.Country != nil {
		updates["country"] = *req.Country
	}
	if req.Timezone != nil {
		updates["timezone"] = *req.Timezone
	}
	if req.StreamURL != nil {
		updates["stream_url"] = *req.StreamURL
	}
	if req.StreamURLs != nil {
		updates["stream_urls"] = req.StreamURLs
	}
	if req.Website != nil {
		updates["website"] = *req.Website
	}
	if req.DonationURL != nil {
		updates["donation_url"] = *req.DonationURL
	}
	if req.DonationEmbedURL != nil {
		updates["donation_embed_url"] = *req.DonationEmbedURL
	}
	if req.LogoURL != nil {
		updates["logo_url"] = *req.LogoURL
	}
	if req.Social != nil {
		updates["social"] = req.Social
	}
	if req.BroadcastType != nil {
		if !catalogm.IsValidBroadcastType(*req.BroadcastType) {
			return nil, fmt.Errorf("invalid broadcast type: %s", *req.BroadcastType)
		}
		updates["broadcast_type"] = *req.BroadcastType
	}
	if req.FrequencyMHz != nil {
		updates["frequency_mhz"] = *req.FrequencyMHz
	}
	if req.PlaylistSource != nil {
		updates["playlist_source"] = *req.PlaylistSource
	}
	if req.PlaylistConfig != nil {
		updates["playlist_config"] = req.PlaylistConfig
	}
	if req.IsActive != nil {
		updates["is_active"] = *req.IsActive
	}

	if len(updates) > 0 {
		if err := s.db.Model(&station).Updates(updates).Error; err != nil {
			return nil, fmt.Errorf("failed to update radio station: %w", err)
		}
	}

	return s.GetStation(stationID)
}

// DeleteStation deletes a radio station
func (s *RadioService) DeleteStation(stationID uint) error {
	if s.db == nil {
		return fmt.Errorf("database not initialized")
	}

	result := s.db.Delete(&catalogm.RadioStation{}, stationID)
	if result.Error != nil {
		return fmt.Errorf("failed to delete radio station: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return apperrors.ErrRadioStationNotFound(stationID)
	}
	return nil
}

// =============================================================================
// Show CRUD
// =============================================================================

// CreateShow creates a new radio show for a station
func (s *RadioService) CreateShow(stationID uint, req *contracts.CreateRadioShowRequest) (*contracts.RadioShowDetailResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	// Verify station exists
	var station catalogm.RadioStation
	if err := s.db.First(&station, stationID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, apperrors.ErrRadioStationNotFound(stationID)
		}
		return nil, fmt.Errorf("failed to get radio station: %w", err)
	}

	slug := req.Slug
	if slug == "" {
		baseSlug := utils.GenerateArtistSlug(req.Name)
		slug = utils.GenerateUniqueSlug(baseSlug, func(candidate string) bool {
			var count int64
			s.db.Model(&catalogm.RadioShow{}).Where("slug = ?", candidate).Count(&count)
			return count > 0
		})
	}

	show := &catalogm.RadioShow{
		StationID:       stationID,
		Name:            req.Name,
		Slug:            slug,
		HostName:        req.HostName,
		Description:     req.Description,
		ScheduleDisplay: req.ScheduleDisplay,
		Schedule:        req.Schedule,
		GenreTags:       req.GenreTags,
		ArchiveURL:      req.ArchiveURL,
		ImageURL:        req.ImageURL,
		ExternalID:      req.ExternalID,
	}

	if err := s.db.Create(show).Error; err != nil {
		return nil, fmt.Errorf("failed to create radio show: %w", err)
	}

	return s.GetShow(show.ID)
}

// GetShow retrieves a radio show by ID
func (s *RadioService) GetShow(showID uint) (*contracts.RadioShowDetailResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	var show catalogm.RadioShow
	err := s.db.Preload("Station").First(&show, showID).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, apperrors.ErrRadioShowNotFound(showID)
		}
		return nil, fmt.Errorf("failed to get radio show: %w", err)
	}

	return s.buildShowDetailResponse(&show)
}

// GetShowBySlug retrieves a radio show by slug
func (s *RadioService) GetShowBySlug(slug string) (*contracts.RadioShowDetailResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	var show catalogm.RadioShow
	err := s.db.Preload("Station").Where("slug = ?", slug).First(&show).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, apperrors.ErrRadioShowNotFound(0)
		}
		return nil, fmt.Errorf("failed to get radio show: %w", err)
	}

	return s.buildShowDetailResponse(&show)
}

// ListShows retrieves all shows for a station
func (s *RadioService) ListShows(stationID uint) ([]*contracts.RadioShowListResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	var shows []catalogm.RadioShow
	err := s.db.Preload("Station").
		Where("station_id = ?", stationID).
		Order("name ASC").
		Find(&shows).Error
	if err != nil {
		return nil, fmt.Errorf("failed to list radio shows: %w", err)
	}

	// Batch-load episode counts
	showIDs := make([]uint, len(shows))
	for i, sh := range shows {
		showIDs[i] = sh.ID
	}

	episodeCounts := make(map[uint]int64)
	if len(showIDs) > 0 {
		type countResult struct {
			ShowID uint
			Count  int64
		}
		var counts []countResult
		s.db.Model(&catalogm.RadioEpisode{}).
			Select("show_id, COUNT(*) as count").
			Where("show_id IN ?", showIDs).
			Group("show_id").
			Find(&counts)

		for _, c := range counts {
			episodeCounts[c.ShowID] = c.Count
		}
	}

	responses := make([]*contracts.RadioShowListResponse, len(shows))
	for i, sh := range shows {
		responses[i] = &contracts.RadioShowListResponse{
			ID:           sh.ID,
			StationID:    sh.StationID,
			StationName:  sh.Station.Name,
			Name:         sh.Name,
			Slug:         sh.Slug,
			HostName:     sh.HostName,
			GenreTags:    sh.GenreTags,
			ImageURL:     sh.ImageURL,
			IsActive:     sh.IsActive,
			EpisodeCount: episodeCounts[sh.ID],
		}
	}

	return responses, nil
}

// UpdateShow updates a radio show
func (s *RadioService) UpdateShow(showID uint, req *contracts.UpdateRadioShowRequest) (*contracts.RadioShowDetailResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	var show catalogm.RadioShow
	if err := s.db.First(&show, showID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, apperrors.ErrRadioShowNotFound(showID)
		}
		return nil, fmt.Errorf("failed to get radio show: %w", err)
	}

	updates := make(map[string]interface{})
	if req.Name != nil {
		updates["name"] = *req.Name
	}
	if req.HostName != nil {
		updates["host_name"] = *req.HostName
	}
	if req.Description != nil {
		updates["description"] = *req.Description
	}
	if req.ScheduleDisplay != nil {
		updates["schedule_display"] = *req.ScheduleDisplay
	}
	if req.Schedule != nil {
		updates["schedule"] = req.Schedule
	}
	if req.GenreTags != nil {
		updates["genre_tags"] = req.GenreTags
	}
	if req.ArchiveURL != nil {
		updates["archive_url"] = *req.ArchiveURL
	}
	if req.ImageURL != nil {
		updates["image_url"] = *req.ImageURL
	}
	if req.IsActive != nil {
		updates["is_active"] = *req.IsActive
	}

	if len(updates) > 0 {
		if err := s.db.Model(&show).Updates(updates).Error; err != nil {
			return nil, fmt.Errorf("failed to update radio show: %w", err)
		}
	}

	return s.GetShow(showID)
}

// DeleteShow deletes a radio show
func (s *RadioService) DeleteShow(showID uint) error {
	if s.db == nil {
		return fmt.Errorf("database not initialized")
	}

	result := s.db.Delete(&catalogm.RadioShow{}, showID)
	if result.Error != nil {
		return fmt.Errorf("failed to delete radio show: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return apperrors.ErrRadioShowNotFound(showID)
	}
	return nil
}

// =============================================================================
// Episodes
// =============================================================================

// GetEpisodes retrieves paginated episodes for a show
func (s *RadioService) GetEpisodes(showID uint, limit, offset int) ([]*contracts.RadioEpisodeResponse, int64, error) {
	if s.db == nil {
		return nil, 0, fmt.Errorf("database not initialized")
	}

	var total int64
	s.db.Model(&catalogm.RadioEpisode{}).Where("show_id = ?", showID).Count(&total)

	var episodes []catalogm.RadioEpisode
	err := s.db.Where("show_id = ?", showID).
		Order("air_date DESC").
		Limit(limit).
		Offset(offset).
		Find(&episodes).Error
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get episodes: %w", err)
	}

	responses := make([]*contracts.RadioEpisodeResponse, len(episodes))
	for i, ep := range episodes {
		responses[i] = &contracts.RadioEpisodeResponse{
			ID:              ep.ID,
			ShowID:          ep.ShowID,
			Title:           ep.Title,
			AirDate:         normalizeDate(ep.AirDate),
			AirTime:         ep.AirTime,
			DurationMinutes: ep.DurationMinutes,
			ArchiveURL:      ep.ArchiveURL,
			PlayCount:       ep.PlayCount,
			CreatedAt:       ep.CreatedAt,
		}
	}

	return responses, total, nil
}

// GetEpisodeByShowAndDate retrieves an episode by show ID and air date, with full playlist
func (s *RadioService) GetEpisodeByShowAndDate(showID uint, airDate string) (*contracts.RadioEpisodeDetailResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	var episode catalogm.RadioEpisode
	err := s.db.Preload("Show.Station").
		Where("show_id = ? AND air_date = ?", showID, airDate).
		First(&episode).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, apperrors.ErrRadioEpisodeNotFound(0)
		}
		return nil, fmt.Errorf("failed to get episode: %w", err)
	}

	return s.buildEpisodeDetailResponse(&episode)
}

// GetEpisodeDetail retrieves a full episode detail by ID, with playlist
func (s *RadioService) GetEpisodeDetail(episodeID uint) (*contracts.RadioEpisodeDetailResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	var episode catalogm.RadioEpisode
	err := s.db.Preload("Show.Station").First(&episode, episodeID).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, apperrors.ErrRadioEpisodeNotFound(episodeID)
		}
		return nil, fmt.Errorf("failed to get episode: %w", err)
	}

	return s.buildEpisodeDetailResponse(&episode)
}

// =============================================================================
// Aggregation queries
// =============================================================================

// GetTopArtistsForShow returns the most-played artists for a show over a time period
func (s *RadioService) GetTopArtistsForShow(showID uint, periodDays, limit int) ([]*contracts.RadioTopArtistResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	query := s.db.Table("radio_plays rp").
		Select(`
			rp.artist_name,
			rp.artist_id,
			a.slug as artist_slug,
			COUNT(*) as play_count,
			COUNT(DISTINCT rp.episode_id) as episode_count
		`).
		Joins("JOIN radio_episodes re ON re.id = rp.episode_id").
		Joins("LEFT JOIN artists a ON a.id = rp.artist_id").
		Where("re.show_id = ?", showID).
		Group("rp.artist_name, rp.artist_id, a.slug").
		Order("play_count DESC").
		Limit(limit)

	if periodDays > 0 {
		cutoff := time.Now().AddDate(0, 0, -periodDays)
		query = query.Where("re.air_date >= ?", cutoff.Format("2006-01-02"))
	}

	type result struct {
		ArtistName   string  `gorm:"column:artist_name"`
		ArtistID     *uint   `gorm:"column:artist_id"`
		ArtistSlug   *string `gorm:"column:artist_slug"`
		PlayCount    int     `gorm:"column:play_count"`
		EpisodeCount int     `gorm:"column:episode_count"`
	}

	var results []result
	if err := query.Find(&results).Error; err != nil {
		return nil, fmt.Errorf("failed to get top artists: %w", err)
	}

	responses := make([]*contracts.RadioTopArtistResponse, len(results))
	for i, r := range results {
		responses[i] = &contracts.RadioTopArtistResponse{
			ArtistName:   r.ArtistName,
			ArtistID:     r.ArtistID,
			ArtistSlug:   r.ArtistSlug,
			PlayCount:    r.PlayCount,
			EpisodeCount: r.EpisodeCount,
		}
	}

	return responses, nil
}

// GetTopLabelsForShow returns the most-featured labels for a show over a time period
func (s *RadioService) GetTopLabelsForShow(showID uint, periodDays, limit int) ([]*contracts.RadioTopLabelResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	query := s.db.Table("radio_plays rp").
		Select(`
			rp.label_name,
			rp.label_id,
			l.slug as label_slug,
			COUNT(*) as play_count
		`).
		Joins("JOIN radio_episodes re ON re.id = rp.episode_id").
		Joins("LEFT JOIN labels l ON l.id = rp.label_id").
		Where("re.show_id = ? AND rp.label_name IS NOT NULL AND rp.label_name != ''", showID).
		Group("rp.label_name, rp.label_id, l.slug").
		Order("play_count DESC").
		Limit(limit)

	if periodDays > 0 {
		cutoff := time.Now().AddDate(0, 0, -periodDays)
		query = query.Where("re.air_date >= ?", cutoff.Format("2006-01-02"))
	}

	type result struct {
		LabelName string  `gorm:"column:label_name"`
		LabelID   *uint   `gorm:"column:label_id"`
		LabelSlug *string `gorm:"column:label_slug"`
		PlayCount int     `gorm:"column:play_count"`
	}

	var results []result
	if err := query.Find(&results).Error; err != nil {
		return nil, fmt.Errorf("failed to get top labels: %w", err)
	}

	responses := make([]*contracts.RadioTopLabelResponse, len(results))
	for i, r := range results {
		responses[i] = &contracts.RadioTopLabelResponse{
			LabelName: r.LabelName,
			LabelID:   r.LabelID,
			LabelSlug: r.LabelSlug,
			PlayCount: r.PlayCount,
		}
	}

	return responses, nil
}

// GetAsHeardOnForArtist returns stations/shows where an artist has been played
func (s *RadioService) GetAsHeardOnForArtist(artistID uint) ([]*contracts.RadioAsHeardOnResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	type result struct {
		StationID   uint   `gorm:"column:station_id"`
		StationName string `gorm:"column:station_name"`
		StationSlug string `gorm:"column:station_slug"`
		ShowID      uint   `gorm:"column:show_id"`
		ShowName    string `gorm:"column:show_name"`
		ShowSlug    string `gorm:"column:show_slug"`
		PlayCount   int    `gorm:"column:play_count"`
		LastPlayed  string `gorm:"column:last_played"`
	}

	var results []result
	err := s.db.Table("radio_plays rp").
		Select(`
			rs.id as station_id,
			rs.name as station_name,
			rs.slug as station_slug,
			rsh.id as show_id,
			rsh.name as show_name,
			rsh.slug as show_slug,
			COUNT(*) as play_count,
			MAX(re.air_date) as last_played
		`).
		Joins("JOIN radio_episodes re ON re.id = rp.episode_id").
		Joins("JOIN radio_shows rsh ON rsh.id = re.show_id").
		Joins("JOIN radio_stations rs ON rs.id = rsh.station_id").
		Where("rp.artist_id = ?", artistID).
		Group("rs.id, rs.name, rs.slug, rsh.id, rsh.name, rsh.slug").
		Order("play_count DESC").
		Find(&results).Error
	if err != nil {
		return nil, fmt.Errorf("failed to get as-heard-on for artist: %w", err)
	}

	responses := make([]*contracts.RadioAsHeardOnResponse, len(results))
	for i, r := range results {
		responses[i] = &contracts.RadioAsHeardOnResponse{
			StationID:   r.StationID,
			StationName: r.StationName,
			StationSlug: r.StationSlug,
			ShowID:      r.ShowID,
			ShowName:    r.ShowName,
			ShowSlug:    r.ShowSlug,
			PlayCount:   r.PlayCount,
			LastPlayed:  r.LastPlayed,
		}
	}

	return responses, nil
}

// GetAsHeardOnForRelease returns stations/shows where a release has been played
func (s *RadioService) GetAsHeardOnForRelease(releaseID uint) ([]*contracts.RadioAsHeardOnResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	type result struct {
		StationID   uint   `gorm:"column:station_id"`
		StationName string `gorm:"column:station_name"`
		StationSlug string `gorm:"column:station_slug"`
		ShowID      uint   `gorm:"column:show_id"`
		ShowName    string `gorm:"column:show_name"`
		ShowSlug    string `gorm:"column:show_slug"`
		PlayCount   int    `gorm:"column:play_count"`
		LastPlayed  string `gorm:"column:last_played"`
	}

	var results []result
	err := s.db.Table("radio_plays rp").
		Select(`
			rs.id as station_id,
			rs.name as station_name,
			rs.slug as station_slug,
			rsh.id as show_id,
			rsh.name as show_name,
			rsh.slug as show_slug,
			COUNT(*) as play_count,
			MAX(re.air_date) as last_played
		`).
		Joins("JOIN radio_episodes re ON re.id = rp.episode_id").
		Joins("JOIN radio_shows rsh ON rsh.id = re.show_id").
		Joins("JOIN radio_stations rs ON rs.id = rsh.station_id").
		Where("rp.release_id = ?", releaseID).
		Group("rs.id, rs.name, rs.slug, rsh.id, rsh.name, rsh.slug").
		Order("play_count DESC").
		Find(&results).Error
	if err != nil {
		return nil, fmt.Errorf("failed to get as-heard-on for release: %w", err)
	}

	responses := make([]*contracts.RadioAsHeardOnResponse, len(results))
	for i, r := range results {
		responses[i] = &contracts.RadioAsHeardOnResponse{
			StationID:   r.StationID,
			StationName: r.StationName,
			StationSlug: r.StationSlug,
			ShowID:      r.ShowID,
			ShowName:    r.ShowName,
			ShowSlug:    r.ShowSlug,
			PlayCount:   r.PlayCount,
			LastPlayed:  r.LastPlayed,
		}
	}

	return responses, nil
}

// GetNewReleaseRadar returns aggregated new releases played across radio stations.
// If stationID is 0, aggregates across all stations. Minimum 2+ station threshold for
// cross-station aggregation; no threshold for single-station filter.
func (s *RadioService) GetNewReleaseRadar(stationID uint, limit int) ([]*contracts.RadioNewReleaseRadarEntry, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	query := s.db.Table("radio_plays rp").
		Select(`
			rp.artist_name,
			rp.artist_id,
			a.slug as artist_slug,
			rp.album_title,
			rp.label_name,
			rp.release_id,
			r.slug as release_slug,
			rp.label_id,
			l.slug as label_slug,
			COUNT(*) as play_count,
			COUNT(DISTINCT rs.id) as station_count
		`).
		Joins("JOIN radio_episodes re ON re.id = rp.episode_id").
		Joins("JOIN radio_shows rsh ON rsh.id = re.show_id").
		Joins("JOIN radio_stations rs ON rs.id = rsh.station_id").
		Joins("LEFT JOIN artists a ON a.id = rp.artist_id").
		Joins("LEFT JOIN releases r ON r.id = rp.release_id").
		Joins("LEFT JOIN labels l ON l.id = rp.label_id").
		Where("rp.is_new = TRUE").
		Group("rp.artist_name, rp.artist_id, a.slug, rp.album_title, rp.label_name, rp.release_id, r.slug, rp.label_id, l.slug").
		Order("station_count DESC, play_count DESC").
		Limit(limit)

	if stationID > 0 {
		query = query.Where("rs.id = ?", stationID)
	} else {
		// Cross-station: require 2+ stations
		query = query.Having("COUNT(DISTINCT rs.id) >= 2")
	}

	type result struct {
		ArtistName   string  `gorm:"column:artist_name"`
		ArtistID     *uint   `gorm:"column:artist_id"`
		ArtistSlug   *string `gorm:"column:artist_slug"`
		AlbumTitle   *string `gorm:"column:album_title"`
		LabelName    *string `gorm:"column:label_name"`
		ReleaseID    *uint   `gorm:"column:release_id"`
		ReleaseSlug  *string `gorm:"column:release_slug"`
		LabelID      *uint   `gorm:"column:label_id"`
		LabelSlug    *string `gorm:"column:label_slug"`
		PlayCount    int     `gorm:"column:play_count"`
		StationCount int     `gorm:"column:station_count"`
	}

	var results []result
	if err := query.Find(&results).Error; err != nil {
		return nil, fmt.Errorf("failed to get new release radar: %w", err)
	}

	responses := make([]*contracts.RadioNewReleaseRadarEntry, len(results))
	for i, r := range results {
		responses[i] = &contracts.RadioNewReleaseRadarEntry{
			ArtistName:   r.ArtistName,
			ArtistID:     r.ArtistID,
			ArtistSlug:   r.ArtistSlug,
			AlbumTitle:   r.AlbumTitle,
			LabelName:    r.LabelName,
			ReleaseID:    r.ReleaseID,
			ReleaseSlug:  r.ReleaseSlug,
			LabelID:      r.LabelID,
			LabelSlug:    r.LabelSlug,
			PlayCount:    r.PlayCount,
			StationCount: r.StationCount,
		}
	}

	return responses, nil
}

// =============================================================================
// Stats
// =============================================================================

// GetRadioStats returns overall radio stats
func (s *RadioService) GetRadioStats() (*contracts.RadioStatsResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	var stats contracts.RadioStatsResponse

	var stationCount int64
	s.db.Model(&catalogm.RadioStation{}).Where("is_active = TRUE").Count(&stationCount)
	stats.TotalStations = int(stationCount)

	var showCount int64
	s.db.Model(&catalogm.RadioShow{}).Where("is_active = TRUE").Count(&showCount)
	stats.TotalShows = int(showCount)

	var episodeCount int64
	s.db.Model(&catalogm.RadioEpisode{}).Count(&episodeCount)
	stats.TotalEpisodes = int(episodeCount)

	s.db.Model(&catalogm.RadioPlay{}).Count(&stats.TotalPlays)

	var matchedPlays int64
	s.db.Model(&catalogm.RadioPlay{}).Where("artist_id IS NOT NULL").Count(&matchedPlays)
	stats.MatchedPlays = matchedPlays

	var uniqueArtists int64
	s.db.Model(&catalogm.RadioPlay{}).Where("artist_id IS NOT NULL").Distinct("artist_id").Count(&uniqueArtists)
	stats.UniqueArtists = int(uniqueArtists)

	return &stats, nil
}

// =============================================================================
// Response builders
// =============================================================================

func (s *RadioService) buildStationDetailResponse(station *catalogm.RadioStation) (*contracts.RadioStationDetailResponse, error) {
	var showCount int64
	s.db.Model(&catalogm.RadioShow{}).Where("station_id = ?", station.ID).Count(&showCount)

	// network_slug is convenience for clients that already know how to
	// resolve a slug; full network objects are not embedded here.
	var networkSlug *string
	if station.Network != nil {
		slug := station.Network.Slug
		networkSlug = &slug
	}

	return &contracts.RadioStationDetailResponse{
		ID:                  station.ID,
		Name:                station.Name,
		Slug:                station.Slug,
		Description:         station.Description,
		City:                station.City,
		State:               station.State,
		Country:             station.Country,
		Timezone:            station.Timezone,
		StreamURL:           station.StreamURL,
		StreamURLs:          station.StreamURLs,
		Website:             station.Website,
		DonationURL:         station.DonationURL,
		DonationEmbedURL:    station.DonationEmbedURL,
		LogoURL:             station.LogoURL,
		Social:              station.Social,
		BroadcastType:       station.BroadcastType,
		FrequencyMHz:        station.FrequencyMHz,
		PlaylistSource:      station.PlaylistSource,
		PlaylistConfig:      station.PlaylistConfig,
		LastPlaylistFetchAt: station.LastPlaylistFetchAt,
		IsActive:            station.IsActive,
		NetworkID:           station.NetworkID,
		NetworkSlug:         networkSlug,
		ShowCount:           int(showCount),
		CreatedAt:           station.CreatedAt,
		UpdatedAt:           station.UpdatedAt,
	}, nil
}

func (s *RadioService) buildShowDetailResponse(show *catalogm.RadioShow) (*contracts.RadioShowDetailResponse, error) {
	var episodeCount int64
	s.db.Model(&catalogm.RadioEpisode{}).Where("show_id = ?", show.ID).Count(&episodeCount)

	return &contracts.RadioShowDetailResponse{
		ID:              show.ID,
		StationID:       show.StationID,
		StationName:     show.Station.Name,
		StationSlug:     show.Station.Slug,
		Name:            show.Name,
		Slug:            show.Slug,
		HostName:        show.HostName,
		Description:     show.Description,
		ScheduleDisplay: show.ScheduleDisplay,
		Schedule:        show.Schedule,
		GenreTags:       show.GenreTags,
		ArchiveURL:      show.ArchiveURL,
		ImageURL:        show.ImageURL,
		IsActive:        show.IsActive,
		EpisodeCount:    episodeCount,
		CreatedAt:       show.CreatedAt,
		UpdatedAt:       show.UpdatedAt,
	}, nil
}

// normalizeDate strips any time component from a date string (e.g. "2026-01-01T00:00:00Z" → "2026-01-01")
func normalizeDate(d string) string {
	if t, err := time.Parse(time.RFC3339, d); err == nil {
		return t.Format("2006-01-02")
	}
	if len(d) >= 10 {
		return d[:10]
	}
	return d
}

func (s *RadioService) buildEpisodeDetailResponse(episode *catalogm.RadioEpisode) (*contracts.RadioEpisodeDetailResponse, error) {
	// Load plays with linked entity data
	var plays []catalogm.RadioPlay
	err := s.db.Where("episode_id = ?", episode.ID).
		Preload("Artist").
		Preload("Release").
		Preload("Label").
		Order("position ASC").
		Find(&plays).Error
	if err != nil {
		return nil, fmt.Errorf("failed to load plays: %w", err)
	}

	playResponses := make([]contracts.RadioPlayResponse, len(plays))
	for i, p := range plays {
		playResponses[i] = contracts.RadioPlayResponse{
			ID:                     p.ID,
			EpisodeID:              p.EpisodeID,
			Position:               p.Position,
			ArtistName:             p.ArtistName,
			TrackTitle:             p.TrackTitle,
			AlbumTitle:             p.AlbumTitle,
			LabelName:              p.LabelName,
			ReleaseYear:            p.ReleaseYear,
			IsNew:                  p.IsNew,
			RotationStatus:         p.RotationStatus,
			DJComment:              p.DJComment,
			IsLivePerformance:      p.IsLivePerformance,
			IsRequest:              p.IsRequest,
			ArtistID:               p.ArtistID,
			ReleaseID:              p.ReleaseID,
			LabelID:                p.LabelID,
			MusicBrainzArtistID:    p.MusicBrainzArtistID,
			MusicBrainzRecordingID: p.MusicBrainzRecordingID,
			MusicBrainzReleaseID:   p.MusicBrainzReleaseID,
			AirTimestamp:           p.AirTimestamp,
		}
		if p.Artist != nil {
			playResponses[i].ArtistSlug = p.Artist.Slug
		}
		if p.Release != nil {
			playResponses[i].ReleaseSlug = p.Release.Slug
		}
		if p.Label != nil {
			playResponses[i].LabelSlug = p.Label.Slug
		}
	}

	return &contracts.RadioEpisodeDetailResponse{
		ID:              episode.ID,
		ShowID:          episode.ShowID,
		ShowName:        episode.Show.Name,
		ShowSlug:        episode.Show.Slug,
		StationName:     episode.Show.Station.Name,
		StationSlug:     episode.Show.Station.Slug,
		Title:           episode.Title,
		AirDate:         normalizeDate(episode.AirDate),
		AirTime:         episode.AirTime,
		DurationMinutes: episode.DurationMinutes,
		Description:     episode.Description,
		ArchiveURL:      episode.ArchiveURL,
		MixcloudURL:     episode.MixcloudURL,
		GenreTags:       episode.GenreTags,
		MoodTags:        episode.MoodTags,
		PlayCount:       episode.PlayCount,
		Plays:           playResponses,
		CreatedAt:       episode.CreatedAt,
	}, nil
}
