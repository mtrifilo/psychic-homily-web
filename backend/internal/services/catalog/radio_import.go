package catalog

import (
	"fmt"
	"time"

	"gorm.io/gorm"

	"psychic-homily-backend/internal/models"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/utils"
)

// getProvider returns the appropriate RadioPlaylistProvider for a station's playlist_source.
func (s *RadioService) getProvider(source string) (RadioPlaylistProvider, error) {
	switch source {
	case models.PlaylistSourceKEXP:
		return NewKEXPProvider(), nil
	case models.PlaylistSourceWFMU:
		return NewWFMUProvider(), nil
	case models.PlaylistSourceNTS:
		return NewNTSProvider(), nil
	default:
		return nil, fmt.Errorf("unsupported playlist source: %s", source)
	}
}

// ImportStation runs a full import: discover shows + fetch episodes for the last N days.
func (s *RadioService) ImportStation(stationID uint, backfillDays int) (*contracts.RadioImportResult, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	var station models.RadioStation
	if err := s.db.First(&station, stationID).Error; err != nil {
		return nil, fmt.Errorf("station not found: %w", err)
	}

	if station.PlaylistSource == nil || *station.PlaylistSource == "" {
		return nil, fmt.Errorf("station %d has no playlist source configured", stationID)
	}

	provider, err := s.getProvider(*station.PlaylistSource)
	if err != nil {
		return nil, err
	}
	defer closeProvider(provider)

	result := &contracts.RadioImportResult{}

	// 1. Discover shows
	importedShows, err := provider.DiscoverShows()
	if err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("discover shows: %v", err))
		return result, nil
	}

	showMap := make(map[string]uint) // external_id → our show ID
	for _, importShow := range importedShows {
		showID, err := s.upsertRadioShow(stationID, importShow)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("upsert show %s: %v", importShow.Name, err))
			continue
		}
		showMap[importShow.ExternalID] = showID
		result.ShowsDiscovered++
	}

	// 2. Fetch episodes for each show
	since := time.Now().AddDate(0, 0, -backfillDays)
	for extID, showID := range showMap {
		episodes, err := provider.FetchNewEpisodes(extID, since)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("fetch episodes for show %s: %v", extID, err))
			continue
		}

		for _, ep := range episodes {
			epResult, err := s.importEpisode(showID, ep, provider)
			if err != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("import episode %s: %v", ep.ExternalID, err))
				continue
			}
			result.EpisodesImported++
			result.PlaysImported += epResult.PlaysImported
			result.PlaysMatched += epResult.PlaysMatched
		}
	}

	// Update last fetch timestamp
	now := time.Now()
	s.db.Model(&station).Update("last_playlist_fetch_at", now)

	return result, nil
}

// FetchNewEpisodes does an incremental fetch since last_playlist_fetch_at.
func (s *RadioService) FetchNewEpisodes(stationID uint) (*contracts.RadioImportResult, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	var station models.RadioStation
	if err := s.db.First(&station, stationID).Error; err != nil {
		return nil, fmt.Errorf("station not found: %w", err)
	}

	if station.PlaylistSource == nil || *station.PlaylistSource == "" {
		return nil, fmt.Errorf("station %d has no playlist source configured", stationID)
	}

	provider, err := s.getProvider(*station.PlaylistSource)
	if err != nil {
		return nil, err
	}
	defer closeProvider(provider)

	// Determine since time: last fetch or 7 days ago
	since := time.Now().AddDate(0, 0, -7)
	if station.LastPlaylistFetchAt != nil {
		since = *station.LastPlaylistFetchAt
	}

	result := &contracts.RadioImportResult{}

	// Get all shows for this station
	var shows []models.RadioShow
	if err := s.db.Where("station_id = ?", stationID).Find(&shows).Error; err != nil {
		return nil, fmt.Errorf("loading shows: %w", err)
	}

	for _, show := range shows {
		if show.ExternalID == nil || *show.ExternalID == "" {
			continue
		}

		episodes, err := provider.FetchNewEpisodes(*show.ExternalID, since)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("fetch episodes for show %s: %v", show.Name, err))
			continue
		}

		for _, ep := range episodes {
			epResult, err := s.importEpisode(show.ID, ep, provider)
			if err != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("import episode %s: %v", ep.ExternalID, err))
				continue
			}
			result.EpisodesImported++
			result.PlaysImported += epResult.PlaysImported
			result.PlaysMatched += epResult.PlaysMatched
		}
	}

	// Update last fetch timestamp
	now := time.Now()
	s.db.Model(&station).Update("last_playlist_fetch_at", now)

	return result, nil
}

// ImportEpisodePlaylist fetches and imports a single episode's playlist by external ID.
func (s *RadioService) ImportEpisodePlaylist(showID uint, episodeExternalID string) (*contracts.EpisodeImportResult, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	// Look up show and station to get the provider
	var show models.RadioShow
	if err := s.db.Preload("Station").First(&show, showID).Error; err != nil {
		return nil, fmt.Errorf("show not found: %w", err)
	}

	if show.Station.PlaylistSource == nil || *show.Station.PlaylistSource == "" {
		return nil, fmt.Errorf("station has no playlist source configured")
	}

	provider, err := s.getProvider(*show.Station.PlaylistSource)
	if err != nil {
		return nil, err
	}
	defer closeProvider(provider)

	// Find the episode by external_id
	var episode models.RadioEpisode
	err = s.db.Where("show_id = ? AND external_id = ?", showID, episodeExternalID).First(&episode).Error
	if err != nil {
		return nil, fmt.Errorf("episode not found: %w", err)
	}

	// Fetch and import the playlist
	plays, err := provider.FetchPlaylist(episodeExternalID)
	if err != nil {
		return nil, fmt.Errorf("fetching playlist: %w", err)
	}

	imported, err := s.importPlays(episode.ID, plays)
	if err != nil {
		return nil, fmt.Errorf("importing plays: %w", err)
	}

	// Run matching
	matcher := NewRadioMatchingEngine(s.db)
	matchResult, err := matcher.MatchPlaysForEpisode(episode.ID)
	if err != nil {
		return &contracts.EpisodeImportResult{PlaysImported: imported}, nil
	}

	return &contracts.EpisodeImportResult{
		PlaysImported: imported,
		PlaysMatched:  matchResult.Matched,
	}, nil
}

// MatchPlays runs the matching engine on unmatched plays for an episode.
func (s *RadioService) MatchPlays(episodeID uint) (*contracts.MatchResult, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	matcher := NewRadioMatchingEngine(s.db)
	return matcher.MatchPlaysForEpisode(episodeID)
}

// =============================================================================
// Internal import helpers
// =============================================================================

// upsertRadioShow creates or updates a radio show from import data.
// Returns the internal show ID.
func (s *RadioService) upsertRadioShow(stationID uint, importShow RadioShowImport) (uint, error) {
	var existing models.RadioShow
	err := s.db.Where("station_id = ? AND external_id = ?", stationID, importShow.ExternalID).First(&existing).Error
	if err == nil {
		// Update existing show
		updates := map[string]interface{}{
			"name": importShow.Name,
		}
		if importShow.HostName != nil {
			updates["host_name"] = *importShow.HostName
		}
		if importShow.Description != nil {
			updates["description"] = *importShow.Description
		}
		if importShow.ImageURL != nil {
			updates["image_url"] = *importShow.ImageURL
		}
		if importShow.ArchiveURL != nil {
			updates["archive_url"] = *importShow.ArchiveURL
		}

		s.db.Model(&existing).Updates(updates)
		return existing.ID, nil
	}

	if err != gorm.ErrRecordNotFound {
		return 0, fmt.Errorf("checking existing show: %w", err)
	}

	// Create new show
	baseSlug := utils.GenerateArtistSlug(importShow.Name)
	slug := utils.GenerateUniqueSlug(baseSlug, func(candidate string) bool {
		var count int64
		s.db.Model(&models.RadioShow{}).Where("slug = ?", candidate).Count(&count)
		return count > 0
	})

	show := &models.RadioShow{
		StationID:   stationID,
		Name:        importShow.Name,
		Slug:        slug,
		HostName:    importShow.HostName,
		Description: importShow.Description,
		ImageURL:    importShow.ImageURL,
		ArchiveURL:  importShow.ArchiveURL,
		ExternalID:  &importShow.ExternalID,
	}

	if err := s.db.Create(show).Error; err != nil {
		return 0, fmt.Errorf("creating show: %w", err)
	}

	return show.ID, nil
}

// importEpisode imports a single episode and its playlist.
func (s *RadioService) importEpisode(showID uint, ep RadioEpisodeImport, provider RadioPlaylistProvider) (*contracts.EpisodeImportResult, error) {
	// Check for existing episode (dedup by show_id + external_id)
	var existing models.RadioEpisode
	err := s.db.Where("show_id = ? AND external_id = ?", showID, ep.ExternalID).First(&existing).Error
	if err == nil {
		// Episode already exists — skip to avoid duplicates
		return &contracts.EpisodeImportResult{}, nil
	}
	if err != gorm.ErrRecordNotFound {
		return nil, fmt.Errorf("checking existing episode: %w", err)
	}

	// Create episode
	episode := &models.RadioEpisode{
		ShowID:          showID,
		Title:           ep.Title,
		AirDate:         ep.AirDate,
		AirTime:         ep.AirTime,
		DurationMinutes: ep.DurationMinutes,
		ArchiveURL:      ep.ArchiveURL,
		ExternalID:      &ep.ExternalID,
	}

	if err := s.db.Create(episode).Error; err != nil {
		return nil, fmt.Errorf("creating episode: %w", err)
	}

	// Fetch and import playlist
	plays, err := provider.FetchPlaylist(ep.ExternalID)
	if err != nil {
		return &contracts.EpisodeImportResult{}, nil // non-fatal: episode created but no plays
	}

	imported, err := s.importPlays(episode.ID, plays)
	if err != nil {
		return &contracts.EpisodeImportResult{PlaysImported: imported}, nil
	}

	// Update play count on episode
	s.db.Model(episode).Update("play_count", imported)

	// Run matching
	matcher := NewRadioMatchingEngine(s.db)
	matchResult, err := matcher.MatchPlaysForEpisode(episode.ID)
	if err != nil {
		return &contracts.EpisodeImportResult{PlaysImported: imported}, nil
	}

	return &contracts.EpisodeImportResult{
		PlaysImported: imported,
		PlaysMatched:  matchResult.Matched,
	}, nil
}

// importPlays batch-creates play records for an episode.
func (s *RadioService) importPlays(episodeID uint, plays []RadioPlayImport) (int, error) {
	if len(plays) == 0 {
		return 0, nil
	}

	records := make([]models.RadioPlay, 0, len(plays))
	for _, p := range plays {
		record := models.RadioPlay{
			EpisodeID:              episodeID,
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
			MusicBrainzArtistID:    p.MusicBrainzArtistID,
			MusicBrainzRecordingID: p.MusicBrainzRecordingID,
			MusicBrainzReleaseID:   p.MusicBrainzReleaseID,
			AirTimestamp:           p.AirTimestamp,
		}
		records = append(records, record)
	}

	// Batch insert
	if err := s.db.CreateInBatches(records, 100).Error; err != nil {
		return 0, fmt.Errorf("batch inserting plays: %w", err)
	}

	return len(records), nil
}

// closeProvider closes a provider if it implements a Close method.
func closeProvider(provider RadioPlaylistProvider) {
	if closer, ok := provider.(interface{ Close() }); ok {
		closer.Close()
	}
}

