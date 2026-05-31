package catalog

import (
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"
	"unicode/utf8"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/utils"
)

// PSY-885: VARCHAR(500) limit on radio_plays text columns (artist_name,
// track_title, album_title, label_name). Counted in runes (not bytes) so
// truncation respects multi-byte boundaries — matches the Postgres semantics
// for varchar length, which is character-count, not byte-count.
const radioPlayVarcharMaxRunes = 500

// getProvider returns the appropriate RadioPlaylistProvider for a station's playlist_source.
func (s *RadioService) getProvider(source string) (RadioPlaylistProvider, error) {
	switch source {
	case catalogm.PlaylistSourceKEXP:
		return NewKEXPProvider(), nil
	case catalogm.PlaylistSourceWFMU:
		return NewWFMUProvider(), nil
	case catalogm.PlaylistSourceNTS:
		return NewNTSProvider(), nil
	case catalogm.PlaylistSourceManual:
		// "manual" is a valid, intentional source: playlists are curated by hand,
		// so there is no automated provider. The scheduled cycle never reaches
		// here for manual stations — GetActiveStationsWithPlaylistSource excludes
		// them — so this guards the manual import-job trigger path, returning a
		// clear error without the default branch's misconfiguration alert.
		return nil, fmt.Errorf("playlist source %q is manual; no automated provider", source)
	default:
		// A truly unrecognized playlist_source silently breaks ALL playlist
		// import for the station — every show imports 0 tracks with no obvious
		// cause. (PSY-927: the value "wfmu_html", which no provider handles, had
		// been set on the WFMU station and zeroed out every show's tracks.) Log
		// loudly with the offending value so a misconfigured station is visible
		// rather than disappearing into a per-cycle error count.
		slog.Default().Error("radio import: unsupported playlist source",
			"playlist_source", source,
			"valid", catalogm.PlaylistSources,
		)
		return nil, fmt.Errorf("unsupported playlist source: %s", source)
	}
}

// parseImportDate parses an import-window bound (since/until). The value may
// arrive date-only ("2026-03-02") from the API, or as a Postgres DATE-column
// round-trip ("2026-03-02T00:00:00Z") when read back from a persisted import
// job — normalizeDateString trims the time suffix so both forms parse. Without
// it the auto-backfill job-execution path failed on every job, since
// radio_import_job.go feeds job.Since/job.Until straight from the DB. (PSY-927)
func parseImportDate(s string) (time.Time, error) {
	return time.Parse("2006-01-02", normalizeDateString(s))
}

// ImportStation runs a full import: discover shows + fetch episodes for the last N days.
func (s *RadioService) ImportStation(stationID uint, backfillDays int) (*contracts.RadioImportResult, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	var station catalogm.RadioStation
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
		showID, _, err := s.upsertRadioShow(stationID, importShow)
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
		episodes, err := provider.FetchNewEpisodes(extID, since, time.Time{})
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
			if epResult.DropSummary != "" {
				result.Errors = append(result.Errors, fmt.Sprintf("episode %s: %s", ep.ExternalID, epResult.DropSummary))
			}
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

	var station catalogm.RadioStation
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

	// Get active shows for this station
	var shows []catalogm.RadioShow
	if err := s.db.Where("station_id = ? AND is_active = ?", stationID, true).Find(&shows).Error; err != nil {
		return nil, fmt.Errorf("loading shows: %w", err)
	}

	for _, show := range shows {
		if show.ExternalID == nil || *show.ExternalID == "" {
			continue
		}

		episodes, err := provider.FetchNewEpisodes(*show.ExternalID, since, time.Time{})
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
			if epResult.DropSummary != "" {
				result.Errors = append(result.Errors, fmt.Sprintf("episode %s: %s", ep.ExternalID, epResult.DropSummary))
			}
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
	var show catalogm.RadioShow
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
	var episode catalogm.RadioEpisode
	err = s.db.Where("show_id = ? AND external_id = ?", showID, episodeExternalID).First(&episode).Error
	if err != nil {
		return nil, fmt.Errorf("episode not found: %w", err)
	}

	// Fetch and import the playlist
	plays, err := provider.FetchPlaylist(episodeExternalID)
	if err != nil {
		return nil, fmt.Errorf("fetching playlist: %w", err)
	}

	imported, dropSummary, err := s.importPlays(episode.ID, plays)
	if err != nil {
		return nil, fmt.Errorf("importing plays: %w", err)
	}

	// Run matching
	matcher := NewRadioMatchingEngine(s.db)
	matchResult, err := matcher.MatchPlaysForEpisode(episode.ID)
	if err != nil {
		return &contracts.EpisodeImportResult{PlaysImported: imported, DropSummary: dropSummary}, nil
	}

	return &contracts.EpisodeImportResult{
		PlaysImported: imported,
		PlaysMatched:  matchResult.Matched,
		DropSummary:   dropSummary,
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

// DiscoverStationShows discovers all shows for a station without importing episodes.
func (s *RadioService) DiscoverStationShows(stationID uint) (*contracts.RadioDiscoverResult, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	var station catalogm.RadioStation
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

	result := &contracts.RadioDiscoverResult{}

	importedShows, err := provider.DiscoverShows()
	if err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("discover shows: %v", err))
		return result, nil
	}

	for _, importShow := range importedShows {
		showID, created, err := s.upsertRadioShow(stationID, importShow)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("upsert show %s: %v", importShow.Name, err))
			continue
		}
		result.ShowsDiscovered++
		result.ShowNames = append(result.ShowNames, importShow.Name)
		if created {
			result.ShowsNew++
			result.NewShowNames = append(result.NewShowNames, importShow.Name)
			result.NewShowIDs = append(result.NewShowIDs, showID)
		}
	}

	return result, nil
}

// importProgressCallback is called periodically during episode import to report
// cumulative progress. Returning cancel=true stops the import early.
type importProgressCallback func(episodesImported, playsImported, playsMatched int, currentDate string, errors []string) (cancel bool)

// importShowEpisodesWithProgress is the shared implementation for importing
// episodes of a single show within a date range. It handles date parsing,
// provider setup, episode fetching/filtering, and per-episode import.
//
// If progressFn is non-nil it is called after every episode with cumulative
// stats; a true return value stops the import early. The episodesFound callback
// (if non-nil) is called once after filtering with the total episode count.
func (s *RadioService) importShowEpisodesWithProgress(
	showID uint,
	since, until string,
	episodesFoundFn func(int),
	progressFn importProgressCallback,
) (*contracts.RadioImportResult, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	sinceTime, err := parseImportDate(since)
	if err != nil {
		return nil, fmt.Errorf("invalid since date %q: %w", since, err)
	}
	untilTime, err := parseImportDate(until)
	if err != nil {
		return nil, fmt.Errorf("invalid until date %q: %w", until, err)
	}

	var show catalogm.RadioShow
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

	if show.ExternalID == nil || *show.ExternalID == "" {
		return nil, fmt.Errorf("show %d has no external ID", showID)
	}

	episodes, err := provider.FetchNewEpisodes(*show.ExternalID, sinceTime, untilTime)
	if err != nil {
		return nil, fmt.Errorf("fetching episodes: %w", err)
	}

	// Filter episodes by air_date within [since, until] (inclusive both ends)
	// Providers apply coarse date filtering, but we still apply precise bounds here.
	var filtered []RadioEpisodeImport
	for _, ep := range episodes {
		epDate, parseErr := time.Parse("2006-01-02", ep.AirDate)
		if parseErr != nil {
			continue
		}
		if !epDate.Before(sinceTime) && !epDate.After(untilTime) {
			filtered = append(filtered, ep)
		}
	}

	if episodesFoundFn != nil {
		episodesFoundFn(len(filtered))
	}

	result := &contracts.RadioImportResult{}

	for _, ep := range filtered {
		epResult, importErr := s.importEpisode(show.ID, ep, provider)
		if importErr != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("import episode %s: %v", ep.ExternalID, importErr))
		} else {
			result.EpisodesImported++
			result.PlaysImported += epResult.PlaysImported
			result.PlaysMatched += epResult.PlaysMatched
			if epResult.DropSummary != "" {
				result.Errors = append(result.Errors, fmt.Sprintf("episode %s: %s", ep.ExternalID, epResult.DropSummary))
			}
		}

		if progressFn != nil {
			if cancel := progressFn(
				result.EpisodesImported,
				result.PlaysImported,
				result.PlaysMatched,
				ep.AirDate,
				result.Errors,
			); cancel {
				return result, nil
			}
		}
	}

	return result, nil
}

// ImportShowEpisodes imports episodes for a single show within a date range.
func (s *RadioService) ImportShowEpisodes(showID uint, since string, until string) (*contracts.RadioImportResult, error) {
	return s.importShowEpisodesWithProgress(showID, since, until, nil, nil)
}

// =============================================================================
// Internal import helpers
// =============================================================================

// upsertRadioShow creates or updates a radio show from import data.
// Returns the internal show ID.
//
// Matching priority:
//  1. (station_id, external_id) — canonical match
//  2. (station_id, slug) — fallback for seeded shows whose external_id
//     may have been incorrect; also updates external_id to the correct value
//  3. Create new show
//
// When a show already exists, only fields that are currently empty/NULL in
// the database are populated from the import data. This preserves
// admin-curated or migration-seeded values.
// upsertRadioShow returns (showID, created, error). created is true ONLY when
// a brand-new row was inserted; both the match-by-external-id and the
// match-by-slug fallback paths return false because they update an existing
// row. Callers use the bool to distinguish "new arrival" from "idempotent
// re-run" — e.g. to fire a notification only on actually-new shows.
func (s *RadioService) upsertRadioShow(stationID uint, importShow RadioShowImport) (uint, bool, error) {
	// Try matching by external_id first (canonical path)
	var existing catalogm.RadioShow
	err := s.db.Where("station_id = ? AND external_id = ?", stationID, importShow.ExternalID).First(&existing).Error
	if err == nil {
		// Only fill in fields that are currently empty — never overwrite curated data.
		updates := s.buildNullSafeShowUpdates(&existing, importShow)
		if len(updates) > 0 {
			s.db.Model(&existing).Updates(updates)
		}
		return existing.ID, false, nil
	}

	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, false, fmt.Errorf("checking existing show by external_id: %w", err)
	}

	// Fallback: match by slug within the same station.
	// This handles seeded shows that had incorrect external_ids — the slug
	// derived from the name will still match, so we adopt the API's
	// external_id instead of creating a duplicate.
	baseSlug := utils.GenerateArtistSlug(importShow.Name)
	err = s.db.Where("station_id = ? AND slug = ?", stationID, baseSlug).First(&existing).Error
	if err == nil {
		// Found by slug — update external_id to the correct API value and
		// fill in any empty fields.
		updates := s.buildNullSafeShowUpdates(&existing, importShow)
		updates["external_id"] = importShow.ExternalID
		s.db.Model(&existing).Updates(updates)
		return existing.ID, false, nil
	}

	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, false, fmt.Errorf("checking existing show by slug: %w", err)
	}

	// Create new show
	slug := utils.GenerateUniqueSlug(baseSlug, func(candidate string) bool {
		var count int64
		s.db.Model(&catalogm.RadioShow{}).Where("slug = ?", candidate).Count(&count)
		return count > 0
	})

	show := &catalogm.RadioShow{
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
		return 0, false, fmt.Errorf("creating show: %w", err)
	}

	return show.ID, true, nil
}

// buildNullSafeShowUpdates returns a map of fields to update, only including
// fields that are currently empty/NULL in the existing record. This preserves
// admin-curated or migration-seeded values.
func (s *RadioService) buildNullSafeShowUpdates(existing *catalogm.RadioShow, importShow RadioShowImport) map[string]interface{} {
	updates := map[string]interface{}{}

	if existing.Name == "" && importShow.Name != "" {
		updates["name"] = importShow.Name
	}
	if existing.HostName == nil && importShow.HostName != nil {
		updates["host_name"] = *importShow.HostName
	}
	if existing.Description == nil && importShow.Description != nil {
		updates["description"] = *importShow.Description
	}
	if existing.ImageURL == nil && importShow.ImageURL != nil {
		updates["image_url"] = *importShow.ImageURL
	}
	if existing.ArchiveURL == nil && importShow.ArchiveURL != nil {
		updates["archive_url"] = *importShow.ArchiveURL
	}
	return updates
}

// importEpisode imports a single episode and its playlist.
func (s *RadioService) importEpisode(showID uint, ep RadioEpisodeImport, provider RadioPlaylistProvider) (*contracts.EpisodeImportResult, error) {
	// Check for existing episode (dedup by show_id + external_id)
	var existing catalogm.RadioEpisode
	err := s.db.Where("show_id = ? AND external_id = ?", showID, ep.ExternalID).First(&existing).Error
	if err == nil {
		// Episode already exists — skip to avoid duplicates
		return &contracts.EpisodeImportResult{}, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("checking existing episode: %w", err)
	}

	// air_date is NOT NULL; an episode with no recoverable date (no broadcast
	// and no parseable alias date) can't be stored. Skip it rather than fail the
	// whole import batch on a date NOT NULL violation — same posture as the
	// dedup skip above.
	if ep.AirDate == "" {
		return &contracts.EpisodeImportResult{}, nil
	}

	// Create episode
	episode := &catalogm.RadioEpisode{
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

	imported, dropSummary, err := s.importPlays(episode.ID, plays)
	if err != nil {
		return &contracts.EpisodeImportResult{PlaysImported: imported, DropSummary: dropSummary}, nil
	}

	// Update play count on episode
	s.db.Model(episode).Update("play_count", imported)

	// Run matching
	matcher := NewRadioMatchingEngine(s.db)
	matchResult, err := matcher.MatchPlaysForEpisode(episode.ID)
	if err != nil {
		return &contracts.EpisodeImportResult{PlaysImported: imported, DropSummary: dropSummary}, nil
	}

	return &contracts.EpisodeImportResult{
		PlaysImported: imported,
		PlaysMatched:  matchResult.Matched,
		DropSummary:   dropSummary,
	}, nil
}

// importPlays batch-creates play records for an episode.
//
// PSY-885: validate-at-the-boundary semantics. Each provider-returned play is
// passed through sanitizePlay BEFORE the batch insert so a single malformed
// row (NULL artist_name, over-length VARCHAR field) can no longer poison its
// 100-row CreateInBatches batch and silently drop all 99 sibling rows. CLAUDE.md:
// "defensive programming at boundaries, trust internally".
//
// Returns (rowsCommitted, summary, err). rowsCommitted is the count of rows
// actually written to the database — rejected rows are excluded; truncated
// rows ARE included (they were salvaged with shortened text). summary is a
// human-readable per-episode aggregate of "dropped N plays: ..." or "" when
// no intervention was needed; callers append it to RadioImportResult.Errors
// so the outcome is visible in admin job logs without per-row noise.
func (s *RadioService) importPlays(episodeID uint, plays []RadioPlayImport) (int, string, error) {
	if len(plays) == 0 {
		return 0, "", nil
	}

	records := make([]catalogm.RadioPlay, 0, len(plays))
	truncatedRows := 0
	missingArtistRows := 0

	for _, p := range plays {
		record, err := sanitizePlay(episodeID, p)
		if err != nil {
			// Per-row diagnostic at WARN — surfaces the actual culprit in logs
			// while the per-episode summary stays compact.
			slog.Warn("radio import: dropping play row",
				"episode_id", episodeID,
				"position", p.Position,
				"reason", err.Error(),
			)
			if errors.Is(err, errMissingArtistName) {
				missingArtistRows++
			}
			continue
		}
		if playNeededTruncation(p) {
			truncatedRows++
		}
		records = append(records, record)
	}

	// droppedRows is computed from the actual delta so new sanitize-drop
	// reasons (added later without a matching per-class counter) still get
	// reflected in the N total — only the per-class breakdown will lag.
	droppedRows := len(plays) - len(records)
	summary := summarizeDrops(droppedRows, truncatedRows, missingArtistRows)

	if len(records) == 0 {
		return 0, summary, nil
	}

	// Batch insert with ON CONFLICT DO NOTHING so duplicate rows (re-imports
	// of the same playlist; chronic during dev / scheduled re-fetches) are
	// silently skipped rather than rolling back the entire 100-row batch.
	// Dedup is enforced by the idx_radio_plays_unique UNIQUE index on
	// (episode_id, position, air_timestamp, artist_name, track_title) NULLS
	// NOT DISTINCT (PSY-888 migration). Records are pre-validated (PSY-885), so
	// a non-UNIQUE constraint violation here (FK gone, NOT NULL) is a hard
	// infrastructural error — bubble it up.
	result := s.db.Clauses(clause.OnConflict{DoNothing: true}).CreateInBatches(records, 100)
	if err := result.Error; err != nil {
		return 0, summary, fmt.Errorf("batch inserting plays: %w", err)
	}

	if skipped := len(records) - int(result.RowsAffected); skipped > 0 {
		slog.Info("radio import: skipped duplicate plays",
			"episode_id", episodeID,
			"skipped", skipped,
			"total", len(records),
		)
	}

	// Return len(records) (attempted) rather than RowsAffected (newly
	// inserted) so callers like importEpisode keep using it to set
	// play_count on the episode without regressing on re-imports where
	// most rows are duplicates. summary carries the PSY-885 drop aggregate.
	return len(records), summary, nil
}

// errMissingArtistName flags a play with no artist_name. radio_plays.artist_name
// is NOT NULL in the schema — we can't make up an artist, so the row is dropped.
var errMissingArtistName = errors.New("missing artist_name")

// sanitizePlay validates and normalizes a provider-returned play DTO for safe
// insertion into radio_plays. It is the single boundary checkpoint where bad
// provider data is rejected (NULL artist_name) or salvaged (truncate
// over-length VARCHAR fields to the column's 500-rune width). All downstream
// code can then trust that any RadioPlay it sees is schema-valid.
//
// Returns the sanitized record on success, or an error explaining why the row
// must be dropped. Truncation does NOT produce an error — it's a non-fatal
// repair, surfaced via playNeededTruncation for the caller's per-episode
// summary counter.
func sanitizePlay(episodeID uint, p RadioPlayImport) (catalogm.RadioPlay, error) {
	// artist_name is NOT NULL in the schema. Trimmed empty / whitespace-only
	// names can't be salvaged — drop the row.
	if strings.TrimSpace(p.ArtistName) == "" {
		return catalogm.RadioPlay{}, errMissingArtistName
	}

	return catalogm.RadioPlay{
		EpisodeID:              episodeID,
		Position:               p.Position,
		ArtistName:             truncateRunes(p.ArtistName, radioPlayVarcharMaxRunes),
		TrackTitle:             truncateOptionalRunes(p.TrackTitle, radioPlayVarcharMaxRunes),
		AlbumTitle:             truncateOptionalRunes(p.AlbumTitle, radioPlayVarcharMaxRunes),
		LabelName:              truncateOptionalRunes(p.LabelName, radioPlayVarcharMaxRunes),
		ReleaseYear:            p.ReleaseYear,
		IsNew:                  p.IsNew,
		RotationStatus:         p.RotationStatus,
		DJComment:              p.DJComment, // TEXT column, no length cap
		IsLivePerformance:      p.IsLivePerformance,
		IsRequest:              p.IsRequest,
		MusicBrainzArtistID:    p.MusicBrainzArtistID,
		MusicBrainzRecordingID: p.MusicBrainzRecordingID,
		MusicBrainzReleaseID:   p.MusicBrainzReleaseID,
		AirTimestamp:           p.AirTimestamp,
	}, nil
}

// playNeededTruncation reports whether sanitizePlay had to shorten any of the
// four VARCHAR(500) text fields on p. Used by importPlays to count
// truncated-row count for the per-episode summary without re-running the
// sanitize logic.
func playNeededTruncation(p RadioPlayImport) bool {
	if utf8.RuneCountInString(p.ArtistName) > radioPlayVarcharMaxRunes {
		return true
	}
	if p.TrackTitle != nil && utf8.RuneCountInString(*p.TrackTitle) > radioPlayVarcharMaxRunes {
		return true
	}
	if p.AlbumTitle != nil && utf8.RuneCountInString(*p.AlbumTitle) > radioPlayVarcharMaxRunes {
		return true
	}
	if p.LabelName != nil && utf8.RuneCountInString(*p.LabelName) > radioPlayVarcharMaxRunes {
		return true
	}
	return false
}

// truncateRunes shortens s to at most max runes, respecting rune boundaries
// (no split multi-byte sequences). Returns s unchanged when within budget.
func truncateRunes(s string, max int) string {
	if utf8.RuneCountInString(s) <= max {
		return s
	}
	runes := []rune(s)
	return string(runes[:max])
}

// truncateOptionalRunes is the *string variant of truncateRunes, preserving
// nil semantics for Huma-style optional pointers.
func truncateOptionalRunes(s *string, max int) *string {
	if s == nil {
		return nil
	}
	if utf8.RuneCountInString(*s) <= max {
		return s
	}
	truncated := truncateRunes(*s, max)
	return &truncated
}

// summarizeDrops formats a single-line per-episode aggregate of plays that
// required boundary intervention (PSY-885). Returns "" when nothing needed
// intervention. Format is stable for log scraping:
//
//	dropped N plays: X over-length titles truncated, Y missing artist_name
//
// N is droppedCount + truncatedCount — the total count of rows the sanitize
// step touched. "Dropped" reads loosely here: truncated rows are kept (just
// with shortened text), but grouping under one number gives admins a single
// "needed attention" magnitude. The per-class breakdown clauses distinguish
// salvage (truncated) from data loss (missing artist_name).
//
// droppedCount is taken as the authoritative drop count from the caller (not
// re-derived from missingArtistCount) so future sanitize-drop classes added
// without a matching counter still appear in N — the breakdown will lag the
// total in that case, surfacing the omission rather than hiding it.
func summarizeDrops(droppedCount, truncatedCount, missingArtistCount int) string {
	total := droppedCount + truncatedCount
	if total == 0 {
		return ""
	}
	var parts []string
	if truncatedCount > 0 {
		parts = append(parts, fmt.Sprintf("%d over-length titles truncated", truncatedCount))
	}
	if missingArtistCount > 0 {
		parts = append(parts, fmt.Sprintf("%d missing artist_name", missingArtistCount))
	}
	return fmt.Sprintf("dropped %d plays: %s", total, strings.Join(parts, ", "))
}

// closeProvider closes a provider if it implements a Close method.
func closeProvider(provider RadioPlaylistProvider) {
	if closer, ok := provider.(interface{ Close() }); ok {
		closer.Close()
	}
}
