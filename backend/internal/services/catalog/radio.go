package catalog

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"

	"psychic-homily-backend/db"
	apperrors "psychic-homily-backend/internal/errors"
	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/services/shared"
	"psychic-homily-backend/internal/utils"
)

// RadioService handles radio station, show, episode, and play operations
type RadioService struct {
	db *gorm.DB

	// Per-station now-playing TTL cache (PSY-1022), lazily initialized via
	// nowPlayingCacheInstance so tests building &RadioService{db: ...}
	// directly still work.
	npCacheOnce sync.Once
	npCache     *nowPlayingCache

	// liveProviderFactory overrides live-provider resolution in tests;
	// nil → the real providers (see resolveLiveProvider).
	liveProviderFactory func(source string) (RadioLiveProvider, func(), bool)

	// playlistProviderFactory overrides playlist-provider resolution in tests
	// (the RunStationSync / import paths); nil → the real providers (see getProvider).
	playlistProviderFactory func(source string) (RadioPlaylistProvider, error)

	// onPermanentFailure, when non-nil, replaces the Sentry escalation of a permanent
	// scheduled/auto sync failure — tests inject a recorder to assert escalation fired
	// (or didn't). nil → escalatePermanentFailure fires the real sentry.CaptureException
	// (PSY-1141).
	onPermanentFailure func(err error, stationID uint, category string)
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

// normalizeStationTimezone validates a station timezone against the SAME catalog
// the aired-only feed resolves it through — Postgres' pg_timezone_names
// (PSY-1204) — and returns the catalog's canonical spelling to persist. This
// keeps the write boundary and the feed's AT TIME ZONE in agreement so an
// accepted value can never silently resolve to UTC in the feed. time.LoadLocation
// is intentionally NOT used: Go's tz catalog differs from Postgres' (it accepts
// abbreviations like "EST" and the alias "Local", which pg_timezone_names lacks),
// so validating with it would accept zones the feed then quietly treats as UTC.
// nil or blank → nil (store NULL; the feed falls back to UTC). A value not in
// pg_timezone_names is rejected.
func (s *RadioService) normalizeStationTimezone(tz *string) (*string, error) {
	if tz == nil {
		return nil, nil
	}
	trimmed := strings.TrimSpace(*tz)
	if trimmed == "" {
		return nil, nil
	}
	var canonical string
	if err := s.db.Raw(
		"SELECT name FROM pg_timezone_names WHERE lower(name) = lower(?) LIMIT 1", trimmed,
	).Scan(&canonical).Error; err != nil {
		return nil, fmt.Errorf("validate station timezone: %w", err)
	}
	if canonical == "" {
		return nil, fmt.Errorf("invalid timezone %q: not a recognized IANA zone name", *tz)
	}
	return &canonical, nil
}

// stationLocalToday returns the current calendar date ("YYYY-MM-DD") in a
// station's timezone, resolving the zone through pg_timezone_names exactly as the
// aired-only feed does (PSY-1204/1205) — an empty or unrecognized value falls
// back to UTC, so a legacy/garbage timezone can never error. It bounds the
// single-station aired-only surfaces (the shows-directory "latest playlist"
// badge and the per-show "upcoming" flag); the dial-wide feed resolves per row
// in SQL instead because it spans stations in different zones.
func (s *RadioService) stationLocalToday(timezone *string) (string, error) {
	tz := ""
	if timezone != nil {
		tz = *timezone
	}
	var today string
	if err := s.db.Raw(
		`SELECT (now() AT TIME ZONE COALESCE((SELECT name FROM pg_timezone_names WHERE lower(name) = lower(btrim(?, E' \t\n\r'))), 'UTC'))::date::text`,
		tz,
	).Scan(&today).Error; err != nil {
		return "", fmt.Errorf("resolve station-local today: %w", err)
	}
	return today, nil
}

// stationLocalTodayForShow resolves stationLocalToday for the show's station
// (PSY-1205), looking the timezone up by show id. A missing show/station/zone
// falls back to UTC. Used by the per-show aired-only surfaces (the archive's
// upcoming flag and the now-playing archive fallback's latest-aired episode).
func (s *RadioService) stationLocalTodayForShow(showID uint) (string, error) {
	var stationRow struct{ Timezone *string }
	if err := s.db.Model(&catalogm.RadioShow{}).
		Select("radio_stations.timezone").
		Joins("JOIN radio_stations ON radio_stations.id = radio_shows.station_id").
		Where("radio_shows.id = ?", showID).
		Scan(&stationRow).Error; err != nil {
		return "", fmt.Errorf("resolve show station timezone: %w", err)
	}
	return s.stationLocalToday(stationRow.Timezone)
}

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

	timezone, err := s.normalizeStationTimezone(req.Timezone)
	if err != nil {
		return nil, err
	}

	if req.PlaylistSource != nil && !catalogm.IsValidPlaylistSource(*req.PlaylistSource) {
		return nil, fmt.Errorf("invalid playlist source: %s", *req.PlaylistSource)
	}

	// Station names are unique (case-insensitive). Reject a duplicate up front
	// with a clean conflict error; the DB unique index is the race backstop.
	var nameCount int64
	if err := s.db.Model(&catalogm.RadioStation{}).Where("lower(name) = lower(?)", req.Name).Count(&nameCount).Error; err != nil {
		return nil, fmt.Errorf("failed to check radio station name uniqueness: %w", err)
	}
	if nameCount > 0 {
		return nil, apperrors.ErrRadioStationNameConflict(req.Name)
	}

	station := &catalogm.RadioStation{
		Name:             req.Name,
		Slug:             slug,
		Description:      req.Description,
		City:             req.City,
		State:            req.State,
		Country:          req.Country,
		Timezone:         timezone,
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
		// The name pre-check above returns a clean 409 for the common (sequential)
		// duplicate-name case. A duplicate-key error here is the rare concurrent
		// race: gorm's TranslateError collapses it to the bare ErrDuplicatedKey
		// sentinel (no constraint name survives), and a create can collide on the
		// name OR the slug index, so it can't be attributed — return the generic
		// error. The DB unique indexes still guarantee integrity either way.
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
		if errors.Is(err, gorm.ErrRecordNotFound) {
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
		if errors.Is(err, gorm.ErrRecordNotFound) {
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

	// Batch-load siblings per network so list responses can render the
	// network tab bar without N+1 (one extra query total, regardless of
	// how many networked stations are in the response).
	networkIDSet := make(map[uint]struct{})
	for _, st := range stations {
		if st.NetworkID != nil {
			networkIDSet[*st.NetworkID] = struct{}{}
		}
	}
	networkIDs := make([]uint, 0, len(networkIDSet))
	for id := range networkIDSet {
		networkIDs = append(networkIDs, id)
	}
	stationsByNetwork := s.batchFetchSiblings(networkIDs)

	responses := make([]*contracts.RadioStationListResponse, len(stations))
	for i, st := range stations {
		var networkSlug *string
		var network *contracts.RadioNetworkInfo
		if st.Network != nil {
			slug := st.Network.Slug
			networkSlug = &slug
			network = &contracts.RadioNetworkInfo{
				Slug:       st.Network.Slug,
				Name:       st.Network.Name,
				IsFlagship: st.IsFlagship,
			}
		}

		var networkScoped []contracts.RadioSiblingStationResponse
		if st.NetworkID != nil {
			networkScoped = stationsByNetwork[*st.NetworkID]
		}
		siblings := excludeSibling(networkScoped, st.ID)

		responses[i] = &contracts.RadioStationListResponse{
			ID:              st.ID,
			Name:            st.Name,
			Slug:            st.Slug,
			City:            st.City,
			State:           st.State,
			Country:         st.Country,
			BroadcastType:   st.BroadcastType,
			FrequencyMHz:    st.FrequencyMHz,
			LogoURL:         st.LogoURL,
			IsActive:        st.IsActive,
			NetworkID:       st.NetworkID,
			NetworkSlug:     networkSlug,
			Network:         network,
			SiblingStations: siblings,
			ShowCount:       showCounts[st.ID],
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
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperrors.ErrRadioStationNotFound(stationID)
		}
		return nil, fmt.Errorf("failed to get radio station: %w", err)
	}

	// Reject a rename to an existing (case-insensitive) name with a clean 409,
	// excluding this station itself; the DB unique index is the race backstop.
	if req.Name != nil && !strings.EqualFold(*req.Name, station.Name) {
		var nameCount int64
		if err := s.db.Model(&catalogm.RadioStation{}).
			Where("lower(name) = lower(?) AND id <> ?", *req.Name, stationID).
			Count(&nameCount).Error; err != nil {
			return nil, fmt.Errorf("failed to check radio station name uniqueness: %w", err)
		}
		if nameCount > 0 {
			return nil, apperrors.ErrRadioStationNameConflict(*req.Name)
		}
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
		timezone, err := s.normalizeStationTimezone(req.Timezone)
		if err != nil {
			return nil, err
		}
		if timezone != nil {
			updates["timezone"] = *timezone
		} else {
			updates["timezone"] = nil // blank clears the column back to NULL
		}
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
		if !catalogm.IsValidPlaylistSource(*req.PlaylistSource) {
			return nil, fmt.Errorf("invalid playlist source: %s", *req.PlaylistSource)
		}
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
			// UpdateStation never changes the slug, so a duplicate-key violation
			// here can only be the name unique index — attribute it to the name
			// conflict. Covers the rare rename race that slips past the pre-check
			// (gorm collapses the error to ErrDuplicatedKey, but for an update the
			// violated constraint is unambiguous).
			if req.Name != nil && shared.IsDuplicateKey(err) {
				return nil, apperrors.ErrRadioStationNameConflict(*req.Name)
			}
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
		if errors.Is(err, gorm.ErrRecordNotFound) {
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

	if req.Schedule != nil {
		if _, err := catalogm.ParseRadioSchedule(req.Schedule); err != nil {
			return nil, apperrors.ErrRadioScheduleInvalid(err.Error())
		}
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
		if errors.Is(err, gorm.ErrRecordNotFound) {
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
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperrors.ErrRadioShowNotFound(0)
		}
		return nil, fmt.Errorf("failed to get radio show: %w", err)
	}

	return s.buildShowDetailResponse(&show)
}

// RadioShowSortLatest orders shows active-first, most-recent playlist first
// (PSY-1048, for the station-page shows directory). Any other sortBy value
// (the handler's enum allows "name" or empty) keeps the original
// alphabetical order.
const RadioShowSortLatest = "latest"

// ListShows retrieves all shows for a station
func (s *RadioService) ListShows(stationID uint, sortBy string) ([]*contracts.RadioShowListResponse, error) {
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

	// Batch-load episode counts + latest air dates (one query each)
	showIDs := make([]uint, len(shows))
	for i, sh := range shows {
		showIDs[i] = sh.ID
	}

	episodeCounts := make(map[uint]int64)
	latestAirDates := make(map[uint]string)
	if len(showIDs) > 0 {
		// "latest playlist" is aired-only (PSY-1205): a future-dated placeholder
		// (an upcoming WFMU broadcast, or a corrupt date) must not become a show's
		// latest date and sort it to the top of the "sorted by latest playlist"
		// directory. Bound MAX to the station's local today (all shows here belong
		// to one station, so the zone is shared); COUNT stays the full episode
		// count. A show with only future episodes gets a NULL latest → sorts as
		// "no playlists yet", which is correct.
		//
		// PSY-1285: the day-granular date bound still admits a TODAY-dated row that
		// hasn't aired yet. Bound the latest-date to episodes that count as a visible
		// aired playlist — the SAME gate the feed uses (airedEpisodeVisibleSQL) — so the
		// directory's "latest playlist: <date>" can never point at an episode the feed
		// won't list: a not-yet-aired (future-windowed) row AND a windowless 0-track
		// placeholder are excluded. COUNT stays the full episode count.
		today, err := s.stationLocalToday(shows[0].Station.Timezone)
		if err != nil {
			return nil, err
		}
		now := time.Now()
		type countResult struct {
			ShowID uint
			Count  int64
			Latest *string
		}
		var counts []countResult
		if err := s.db.Model(&catalogm.RadioEpisode{}).
			Select(fmt.Sprintf(`show_id, COUNT(*) as count,
				MAX(air_date) FILTER (WHERE air_date <= ? AND %s) as latest`, airedEpisodeVisibleSQL("")), today, now).
			Where("show_id IN ?", showIDs).
			Group("show_id").
			Find(&counts).Error; err != nil {
			return nil, fmt.Errorf("failed to load episode counts: %w", err)
		}

		for _, c := range counts {
			episodeCounts[c.ShowID] = c.Count
			if c.Latest != nil {
				latestAirDates[c.ShowID] = normalizeDate(*c.Latest)
			}
		}
	}

	responses := make([]*contracts.RadioShowListResponse, len(shows))
	for i, sh := range shows {
		var latest *string
		if d, ok := latestAirDates[sh.ID]; ok {
			latest = &d
		}
		responses[i] = &contracts.RadioShowListResponse{
			ID:              sh.ID,
			StationID:       sh.StationID,
			StationName:     sh.Station.Name,
			Name:            sh.Name,
			Slug:            sh.Slug,
			HostName:        sh.HostName,
			ScheduleDisplay: sh.ScheduleDisplay,
			GenreTags:       sh.GenreTags,
			ImageURL:        sh.ImageURL,
			IsActive:        sh.IsActive,
			LifecycleState:  sh.LifecycleState,
			ScheduleLocked:  sh.ScheduleLocked,
			EpisodeCount:    episodeCounts[sh.ID],
			LatestAirDate:   latest,
		}
	}

	// In-memory sort is safe because this endpoint always returns every show
	// for the station (no pagination) and LatestAirDate is already computed.
	if sortBy == RadioShowSortLatest {
		sort.SliceStable(responses, func(i, j int) bool {
			a, b := responses[i], responses[j]
			// NOTE: this active-first sort intentionally still keys off is_active, not
			// the new lifecycle_state (PSY-1155). The janitor maintains lifecycle_state
			// (active↔dormant) but deliberately leaves is_active untouched (so dormant
			// shows keep polling); the two can diverge. lifecycle_state is the future
			// authoritative signal for the active-vs-historical split — switching this
			// sort (and any new active/historical filter) to it is the frontend
			// follow-up, not done here (backend-only scope).
			if a.IsActive != b.IsActive {
				return a.IsActive
			}
			// Shows with episodes before shows without; later dates first.
			// YYYY-MM-DD strings compare correctly lexicographically.
			ad, bd := "", ""
			if a.LatestAirDate != nil {
				ad = *a.LatestAirDate
			}
			if b.LatestAirDate != nil {
				bd = *b.LatestAirDate
			}
			if ad != bd {
				return ad > bd
			}
			return a.Name < b.Name
		})
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
		if errors.Is(err, gorm.ErrRecordNotFound) {
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
		if _, err := catalogm.ParseRadioSchedule(req.Schedule); err != nil {
			return nil, apperrors.ErrRadioScheduleInvalid(err.Error())
		}
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
	// LifecycleState (PSY-1172): the only write path for the operational state. Validate
	// against the enum before writing — an invalid value must not reach the DB. Setting
	// 'retired' is sticky (the janitor excludes it from reconcile); active/dormant are
	// advisory (the next janitor run may re-reconcile them by episode recency).
	if req.LifecycleState != nil {
		if !catalogm.IsValidRadioLifecycleState(*req.LifecycleState) {
			return nil, apperrors.ErrRadioLifecycleInvalid(*req.LifecycleState)
		}
		updates["lifecycle_state"] = *req.LifecycleState
	}
	// Schedule provenance (PSY-1186): an explicit schedule_locked wins; otherwise a manual
	// schedule edit auto-locks it (the admin curated it by hand, so the weekly WFMU scrape
	// must not clobber it). To edit the schedule but KEEP it scrape-managed, send the schedule
	// together with schedule_locked=false (the explicit value wins over the auto-lock).
	switch {
	case req.ScheduleLocked != nil:
		updates["schedule_locked"] = *req.ScheduleLocked
	case req.Schedule != nil:
		updates["schedule_locked"] = true
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
	if err := s.db.Model(&catalogm.RadioEpisode{}).Where("show_id = ?", showID).Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to count episodes: %w", err)
	}

	var episodes []catalogm.RadioEpisode
	// Shared feed ordering so "latest" (episodes[0], drives the show-page ON
	// AIR strip's live derivation) is deterministic and agrees with the
	// dial-wide feed when two episodes share an air_date (PSY-1152/PSY-1297).
	err := s.db.Where("show_id = ?", showID).
		Order(episodeLatestFirstOrderSQL("")).
		Limit(limit).
		Offset(offset).
		Find(&episodes).Error
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get episodes: %w", err)
	}

	episodeIDs := make([]uint, len(episodes))
	for i, ep := range episodes {
		episodeIDs[i] = ep.ID
	}
	previews, err := s.episodeArtistPreviews(episodeIDs)
	if err != nil {
		return nil, 0, err
	}

	// Flag upcoming (not-yet-aired) episodes against the show's station-local
	// today (PSY-1205). Windowless providers (WFMU) can't express "upcoming"
	// through Status — a null air window settles to "aired" — so the per-show
	// archive labels them from air_date instead.
	today, err := s.stationLocalTodayForShow(showID)
	if err != nil {
		return nil, 0, err
	}

	now := time.Now()
	responses := make([]*contracts.RadioEpisodeResponse, len(episodes))
	for i, ep := range episodes {
		airDate := normalizeDate(ep.AirDate)
		responses[i] = &contracts.RadioEpisodeResponse{
			ID:              ep.ID,
			ShowID:          ep.ShowID,
			Title:           ep.Title,
			AirDate:         airDate,
			AirTime:         ep.AirTime,
			DurationMinutes: ep.DurationMinutes,
			ArchiveURL:      ep.ArchiveURL,
			StartsAt:        ep.StartsAt,
			EndsAt:          ep.EndsAt,
			// Status is computed on read — "live" is a function of now, so a stored
			// value would go stale the instant the window ends (the PSY-1128 bug).
			Status: catalogm.ComputeEpisodeStatus(ep.StartsAt, ep.EndsAt, ep.PlaylistState, now),
			// air_date is a DATE; YYYY-MM-DD strings compare lexicographically.
			IsUpcoming:    airDate > today,
			PlayCount:     ep.PlayCount,
			CreatedAt:     ep.CreatedAt,
			ArtistPreview: previewOrEmpty(previews, ep.ID),
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
	// Same-day siblings are legal ((show_id, air_date, external_id) index);
	// day-keyed URLs can't address them individually, so resolve to the same
	// same-day winner the list surfaces rank first (PSY-1297) rather than
	// First's arbitrary pick. Time-varying by design: with an aired sibling
	// and a pre-published later-today one, the same URL resolves to the aired
	// row until the later window passes, then to the newly-aired row.
	err := s.db.Preload("Show.Station").
		Where("show_id = ? AND air_date = ?", showID, airDate).
		Order(episodeLatestFirstOrderSQL("")).
		First(&episode).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
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
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperrors.ErrRadioEpisodeNotFound(episodeID)
		}
		return nil, fmt.Errorf("failed to get episode: %w", err)
	}

	return s.buildEpisodeDetailResponse(&episode)
}

// =============================================================================
// Aggregation queries
// =============================================================================

// WFMU playlists log background/segment music as plays whose artist_name
// carries the "Music behind DJ:" prefix (casing varies; the colon is always
// present in observed data). These are not artists — left in, they dominate
// top-artists boxes and artist previews. Aggregated surfaces exclude them;
// raw playlist surfaces (episode detail, now-playing current track) keep the
// rows so the playlist stays an honest record. Import-time flagging plus a
// backfill is the durable follow-up; this is the query-time filter (PSY-1078).
const pseudoArtistNamePrefix = "Music behind DJ:"

// pseudoArtistExclusionSQL is the shared predicate for aggregation queries
// over radio_plays aliased as rp. Compile-time constant (no interpolated
// input); the prefix contains no LIKE wildcards.
const pseudoArtistExclusionSQL = "rp.artist_name NOT ILIKE '" + pseudoArtistNamePrefix + "%'"

// isPseudoArtistName is the Go-side mirror of pseudoArtistExclusionSQL, for
// derivations that aggregate play rows in memory (now-playing recent artists).
func isPseudoArtistName(name string) bool {
	return len(name) >= len(pseudoArtistNamePrefix) &&
		strings.EqualFold(name[:len(pseudoArtistNamePrefix)], pseudoArtistNamePrefix)
}

// playsScope narrows a top-artists/labels aggregation query (radio_plays rp
// joined to radio_episodes re) to a show or a station, applied with
// .Scopes(). The episode feeds use the separate episodeFeedScope type — see
// its comment for why the two are not interchangeable.
type playsScope func(*gorm.DB) *gorm.DB

func showScope(showID uint) playsScope {
	return func(q *gorm.DB) *gorm.DB {
		return q.Where("re.show_id = ?", showID)
	}
}

func stationScope(stationID uint) playsScope {
	return func(q *gorm.DB) *gorm.DB {
		return q.Joins("JOIN radio_shows rsh ON rsh.id = re.show_id").
			Where("rsh.station_id = ?", stationID)
	}
}

// GetTopArtistsForShow returns the most-played artists for a show over a time period
func (s *RadioService) GetTopArtistsForShow(showID uint, periodDays, limit int) ([]*contracts.RadioTopArtistResponse, error) {
	return s.topArtists(showScope(showID), periodDays, limit)
}

// GetTopArtistsForStation returns the most-played artists across the
// requested station's shows — strictly that station, never its network
// siblings (PSY-1074).
func (s *RadioService) GetTopArtistsForStation(stationID uint, periodDays, limit int) ([]*contracts.RadioTopArtistResponse, error) {
	if err := s.verifyStationExists(stationID); err != nil {
		return nil, err
	}
	return s.topArtists(stationScope(stationID), periodDays, limit)
}

func (s *RadioService) topArtists(scope playsScope, periodDays, limit int) ([]*contracts.RadioTopArtistResponse, error) {
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
		Scopes(scope).
		Where(pseudoArtistExclusionSQL).
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
	return s.topLabels(showScope(showID), periodDays, limit)
}

// GetTopLabelsForStation returns the most-featured labels across the
// requested station's shows — strictly that station, never its network
// siblings (PSY-1074).
func (s *RadioService) GetTopLabelsForStation(stationID uint, periodDays, limit int) ([]*contracts.RadioTopLabelResponse, error) {
	if err := s.verifyStationExists(stationID); err != nil {
		return nil, err
	}
	return s.topLabels(stationScope(stationID), periodDays, limit)
}

func (s *RadioService) topLabels(scope playsScope, periodDays, limit int) ([]*contracts.RadioTopLabelResponse, error) {
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
		Scopes(scope).
		Where("rp.label_name IS NOT NULL AND rp.label_name != ''").
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

// =============================================================================
// Latest-playlists feeds (PSY-1048)
// =============================================================================

// episodePreviewArtistCount is how many distinct artists an episode row's
// preview carries, in play order.
const episodePreviewArtistCount = 3

// previewOrEmpty guarantees a JSON [] (never null) for episodes whose
// playlist has no usable artists yet.
func previewOrEmpty(previews map[uint][]contracts.RadioEpisodePreviewArtist, episodeID uint) []contracts.RadioEpisodePreviewArtist {
	if p, ok := previews[episodeID]; ok {
		return p
	}
	return []contracts.RadioEpisodePreviewArtist{}
}

// ResolveStationIDBySlug returns just the station ID for a slug — a cheap
// resolver for station-scoped reads that don't need the full detail build
// (network preload, show count, siblings) that GetStationBySlug performs.
func (s *RadioService) ResolveStationIDBySlug(slug string) (uint, error) {
	if s.db == nil {
		return 0, fmt.Errorf("database not initialized")
	}
	var station catalogm.RadioStation
	if err := s.db.Select("id").Where("slug = ?", slug).First(&station).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return 0, apperrors.ErrRadioStationNotFound(0)
		}
		return 0, fmt.Errorf("failed to resolve radio station: %w", err)
	}
	return station.ID, nil
}

// verifyStationExists returns ErrRadioStationNotFound for an unknown station
// ID — the station-scoped reads (episodes feed, top artists/labels) call it
// so an unknown numeric ID surfaces as a 404 rather than an empty 200.
func (s *RadioService) verifyStationExists(stationID uint) error {
	if s.db == nil {
		return fmt.Errorf("database not initialized")
	}
	var station catalogm.RadioStation
	if err := s.db.Select("id").First(&station, stationID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return apperrors.ErrRadioStationNotFound(stationID)
		}
		return fmt.Errorf("failed to get radio station: %w", err)
	}
	return nil
}

// episodeArtistPreviews returns up to episodePreviewArtistCount distinct
// artists per episode, in play order, with knowledge-graph links where
// matched — one query for the whole page of episodes, never per-row.
//
// The inner query groups by name (so a name that is matched on some plays
// and unmatched on others appears once, and MAX(artist_id) keeps its
// knowledge-graph link) and caps each episode's rows in SQL via
// ROW_NUMBER, so the transfer scales with the preview size rather than
// with playlist length.
func (s *RadioService) episodeArtistPreviews(episodeIDs []uint) (map[uint][]contracts.RadioEpisodePreviewArtist, error) {
	previews := make(map[uint][]contracts.RadioEpisodePreviewArtist, len(episodeIDs))
	if len(episodeIDs) == 0 {
		return previews, nil
	}

	type row struct {
		EpisodeID  uint    `gorm:"column:episode_id"`
		ArtistName string  `gorm:"column:artist_name"`
		ArtistID   *uint   `gorm:"column:artist_id"`
		ArtistSlug *string `gorm:"column:artist_slug"`
	}
	var rows []row
	err := s.db.Raw(`
		SELECT g.episode_id, g.artist_name, g.artist_id, a.slug AS artist_slug
		FROM (
			SELECT rp.episode_id, rp.artist_name, MAX(rp.artist_id) AS artist_id,
			       MIN(rp.position) AS first_pos,
			       ROW_NUMBER() OVER (PARTITION BY rp.episode_id ORDER BY MIN(rp.position)) AS rn
			FROM radio_plays rp
			WHERE rp.episode_id IN ? AND rp.artist_name != '' AND `+pseudoArtistExclusionSQL+`
			GROUP BY rp.episode_id, rp.artist_name
		) g
		LEFT JOIN artists a ON a.id = g.artist_id
		WHERE g.rn <= ?
		ORDER BY g.episode_id, g.first_pos`,
		episodeIDs, episodePreviewArtistCount).
		Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("failed to load episode artist previews: %w", err)
	}

	for _, r := range rows {
		previews[r.EpisodeID] = append(previews[r.EpisodeID], contracts.RadioEpisodePreviewArtist{
			ArtistName: r.ArtistName,
			ArtistID:   r.ArtistID,
			ArtistSlug: r.ArtistSlug,
		})
	}
	return previews, nil
}

// GetStationEpisodes returns the station's latest playlists across all of
// its shows, newest first — strictly the requested station, never its
// network siblings (PSY-1074); channel shows live under their own tabs.
// Aired-only: future-dated episodes are excluded (day-granular, station-local;
// PSY-1204).
func (s *RadioService) GetStationEpisodes(stationID uint, limit, offset int) ([]*contracts.RadioStationEpisodeRow, int64, error) {
	if err := s.verifyStationExists(stationID); err != nil {
		return nil, 0, err
	}
	return s.episodeRows(func(q *gorm.DB) *gorm.DB {
		return q.Where("rsh.station_id = ?", stationID)
	}, limit, offset)
}

// GetRecentEpisodes returns the newest playlists across every active
// station — the dial-wide "latest playlists" feed. (Active-filtering is hub
// policy; station pages serve inactive stations' archives directly.)
// Aired-only: future-dated episodes are excluded (day-granular, station-local;
// PSY-1204).
func (s *RadioService) GetRecentEpisodes(limit, offset int) ([]*contracts.RadioStationEpisodeRow, int64, error) {
	return s.episodeRows(func(q *gorm.DB) *gorm.DB {
		return q.Where("rst.is_active = ?", true)
	}, limit, offset)
}

// episodeFeedScope narrows the episode-feed base query (radio_episodes re
// joined to radio_shows rsh and radio_stations rst). Distinct from
// playsScope: stationScope embeds its own rsh join and would produce a
// duplicate alias here.
type episodeFeedScope func(*gorm.DB) *gorm.DB

// airedEpisodeVisibleSQL is the single definition of "this episode counts as a
// visible aired playlist" (PSY-1285): a WINDOWED episode is visible iff its frozen
// window has passed (starts_at <= now) — so a future-windowed (not-yet-aired) episode
// is hidden even if it already captured an early play snapshot; a WINDOWLESS episode
// (no derivable window) falls back to "has plays". So a not-yet-aired episode and a
// windowless 0-track placeholder are both excluded, while a windowed-aired episode
// (even 0-track) and a windowless episode carrying a playlist stay. `prefix` qualifies
// the columns ("re." in the joined feed query, "" in a single-table query); the one ?
// binds the "now" instant. Every aired-only surface — the "Latest playlists" feed
// (episodeRows), the shows-directory latest date (ListShows), the now-playing "Latest"
// selector (latestEpisodeForShow), and the most-active-show pick (mostActiveShow) —
// shares this single air-window definition. The first three also pair it with an
// `air_date <= today` bound; mostActiveShow gates on the window alone, so the four
// agree except for the practically-unreachable case of a windowless future-dated
// episode that already carries plays.
func airedEpisodeVisibleSQL(prefix string) string {
	return fmt.Sprintf(
		"((%[1]sstarts_at IS NOT NULL AND %[1]sstarts_at <= ?) OR (%[1]sstarts_at IS NULL AND %[1]splay_count > 0))",
		prefix,
	)
}

// episodeLatestFirstOrderSQL is the single definition of "latest first" for
// episode lists (PSY-1297): newest calendar day; within a day, aired rows
// before not-yet-aired ones, then the latest frozen air window (PSY-1238) — so
// same-day rows read latest-aired-first instead of import-order. The
// future-window sink (the CASE key) exists for the UNGATED per-show archive:
// GetEpisodes shows upcoming rows by design (PSY-1205), and without the sink a
// pre-published later-today row would deterministically beat the already-aired
// one for episodes[0] (which drives the show page's "latest" pick — its
// is_upcoming check is day-granular and can't skip a today-dated future row).
// On the gated surfaces (episodeRows, latestEpisodeForShow) the sink is a
// no-op (modulo app/DB clock skew): airedEpisodeVisibleSQL already excludes
// future-windowed rows. Windowless rows (pop-ups/off-schedule airings the
// window stamper deliberately skips, plus providers without times) fail the
// CASE (NULL > now() is not true), so they group WITH aired rows — above
// future-windowed ones — and NULLS LAST then sinks them within that group,
// below the windowed-aired rows, rather than scrambling the top; id DESC stays
// as the final deterministic tiebreaker. NOTE the ordering is a function of DB
// now(), not just stored data: a same-day pre-published row jumps from the
// sink to its aired slot the instant its window passes, so by-date resolution
// and offset pagination can shift across that instant (accepted: once per
// airing, today's rows only). Used by the "Latest playlists" feeds
// (episodeRows), the per-show archive (GetEpisodes), the now-playing
// latest-episode selector (latestEpisodeForShow), and the by-date detail
// lookup (GetEpisodeByShowAndDate) so they all pick the same same-day winner.
// `prefix` qualifies the columns ("re." in the joined feed query, "" in
// single-table queries) and MUST be a compile-time literal — the result is
// interpolated into ORDER BY unparameterized.
func episodeLatestFirstOrderSQL(prefix string) string {
	return fmt.Sprintf(
		"%[1]sair_date DESC, CASE WHEN %[1]sstarts_at > now() THEN 1 ELSE 0 END, %[1]sstarts_at DESC NULLS LAST, %[1]sid DESC",
		prefix,
	)
}

// episodeRows is the shared core of the station-scoped and dial-wide feeds.
func (s *RadioService) episodeRows(scope episodeFeedScope, limit, offset int) ([]*contracts.RadioStationEpisodeRow, int64, error) {
	if s.db == nil {
		return nil, 0, fmt.Errorf("database not initialized")
	}
	// Pin one instant so the COUNT and the FIND (two separate statements) bound
	// air_date against the same "now" — otherwise a station-local midnight tick
	// between them could make total and the returned page disagree by a row.
	// (The ORDER BY's future-window sink deliberately uses DB now() outside
	// this pinned instant — ordering never affects row membership, so COUNT
	// and FIND cannot disagree through it.)
	now := time.Now().UTC()
	base := func() *gorm.DB {
		return s.db.Table("radio_episodes re").
			Joins("JOIN radio_shows rsh ON rsh.id = re.show_id").
			Joins("JOIN radio_stations rst ON rst.id = rsh.station_id").
			// Resolve the station's timezone through pg_timezone_names — the catalog
			// AT TIME ZONE uses — via a LEFT JOIN so it's looked up once per station
			// (not once per episode row), case-insensitively. The write boundary
			// (normalizeStationTimezone) keeps new rows canonical against this same
			// catalog; the COALESCE-to-UTC backstop means a legacy/out-of-band or
			// unrecognized value leaves tzn.name NULL and falls back to UTC rather
			// than erroring out of AT TIME ZONE and 500-ing this public,
			// all-stations feed.
			// btrim strips space/tab/newline/CR to match the write side's
			// strings.TrimSpace, so a legacy whitespace-padded value still resolves.
			Joins(`LEFT JOIN pg_timezone_names tzn ON lower(tzn.name) = lower(btrim(rst.timezone, E' \t\n\r'))`).
			// Aired-only: WFMU (and other providers) publish playlist pages for
			// UPCOMING broadcasts ahead of airtime, which the importer ingests as
			// future-dated, 0-track placeholder episodes (PSY-1204). The day-granular
			// air_date bound below is a coarse pre-filter (it drops tomorrow-onward and
			// lets the per-station zone bound use no air_date index — fine at this scale);
			// the precise aired-only filter is the air-window gate after it (PSY-1285).
			// air-time-precise "is it live right now" lives in ComputeEpisodeStatus.
			Where("re.air_date <= (?::timestamptz AT TIME ZONE COALESCE(tzn.name, 'UTC'))::date", now).
			// PSY-1285: air-window gate — keep this "Latest playlists" feed to episodes
			// that have actually aired (the day-granular date bound above still admits a
			// today-dated page WFMU pre-published for a broadcast airing LATER TODAY; the
			// frozen air window from PSY-1238/1283 can exclude it). Shared with the
			// directory + now-playing surfaces via airedEpisodeVisibleSQL.
			Where(airedEpisodeVisibleSQL("re."), now).
			Scopes(scope)
	}

	var total int64
	if err := base().Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to count episodes: %w", err)
	}

	type row struct {
		ID          uint       `gorm:"column:id"`
		Title       *string    `gorm:"column:title"`
		AirDate     string     `gorm:"column:air_date"`
		StartsAt    *time.Time `gorm:"column:starts_at"`
		EndsAt      *time.Time `gorm:"column:ends_at"`
		PlayCount   int        `gorm:"column:play_count"`
		ArchiveURL  *string    `gorm:"column:archive_url"`
		ShowID      uint       `gorm:"column:show_id"`
		ShowName    string     `gorm:"column:show_name"`
		ShowSlug    string     `gorm:"column:show_slug"`
		StationID   uint       `gorm:"column:station_id"`
		StationName string     `gorm:"column:station_name"`
		StationSlug string     `gorm:"column:station_slug"`
	}
	var rows []row
	err := base().
		Select(`re.id, re.title, re.air_date, re.starts_at, re.ends_at, re.play_count, re.archive_url,
			rsh.id as show_id, rsh.name as show_name, rsh.slug as show_slug,
			rst.id as station_id, rst.name as station_name, rst.slug as station_slug`).
		Order(episodeLatestFirstOrderSQL("re.")).
		Limit(limit).
		Offset(offset).
		Find(&rows).Error
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list episodes: %w", err)
	}

	episodeIDs := make([]uint, len(rows))
	for i, r := range rows {
		episodeIDs[i] = r.ID
	}
	previews, err := s.episodeArtistPreviews(episodeIDs)
	if err != nil {
		return nil, 0, err
	}

	out := make([]*contracts.RadioStationEpisodeRow, len(rows))
	for i, r := range rows {
		out[i] = &contracts.RadioStationEpisodeRow{
			ID:            r.ID,
			Title:         r.Title,
			AirDate:       normalizeDate(r.AirDate),
			StartsAt:      r.StartsAt,
			EndsAt:        r.EndsAt,
			PlayCount:     r.PlayCount,
			ArchiveURL:    r.ArchiveURL,
			ShowID:        r.ShowID,
			ShowName:      r.ShowName,
			ShowSlug:      r.ShowSlug,
			StationID:     r.StationID,
			StationName:   r.StationName,
			StationSlug:   r.StationSlug,
			ArtistPreview: previewOrEmpty(previews, r.ID),
		}
	}
	return out, total, nil
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
	// resolve a slug; the nested Network object below carries the same
	// info plus name + is_flagship.
	var networkSlug *string
	var network *contracts.RadioNetworkInfo
	if station.Network != nil {
		slug := station.Network.Slug
		networkSlug = &slug
		network = &contracts.RadioNetworkInfo{
			Slug:       station.Network.Slug,
			Name:       station.Network.Name,
			IsFlagship: station.IsFlagship,
		}
	}

	siblings := s.fetchSiblings(station.NetworkID, station.ID)

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
		Network:             network,
		SiblingStations:     siblings,
		ShowCount:           int(showCount),
		CreatedAt:           station.CreatedAt,
		UpdatedAt:           station.UpdatedAt,
	}, nil
}

// fetchSiblings is the single-station-response convenience wrapper around
// batchFetchSiblings. Used by buildStationDetailResponse so single-station
// fetch paths and the list path share one query shape.
func (s *RadioService) fetchSiblings(networkID *uint, excludeStationID uint) []contracts.RadioSiblingStationResponse {
	if networkID == nil {
		return []contracts.RadioSiblingStationResponse{}
	}
	byNetwork := s.batchFetchSiblings([]uint{*networkID})
	return excludeSibling(byNetwork[*networkID], excludeStationID)
}

// batchFetchSiblings returns network_id → all stations in that network
// (caller-side filters self via excludeSibling). One query for any number
// of network ids; no N+1. Select() narrows the row read so we skip the
// JSONB blobs (stream_urls/social/playlist_config) the sibling response
// doesn't render — ~25-column hydration shrinks to 7 scalar columns.
func (s *RadioService) batchFetchSiblings(networkIDs []uint) map[uint][]contracts.RadioSiblingStationResponse {
	out := make(map[uint][]contracts.RadioSiblingStationResponse, len(networkIDs))
	if len(networkIDs) == 0 {
		return out
	}
	var stations []catalogm.RadioStation
	s.db.
		Select("id, slug, name, broadcast_type, frequency_mhz, is_flagship, network_id").
		Where("network_id IN ?", networkIDs).
		Order("is_flagship DESC, name ASC").
		Find(&stations)
	for _, st := range stations {
		if st.NetworkID == nil {
			continue
		}
		out[*st.NetworkID] = append(out[*st.NetworkID], contracts.RadioSiblingStationResponse{
			ID:            st.ID,
			Slug:          st.Slug,
			Name:          st.Name,
			BroadcastType: st.BroadcastType,
			FrequencyMHz:  st.FrequencyMHz,
			IsFlagship:    st.IsFlagship,
		})
	}
	return out
}

// excludeSibling drops the entry with the given id and returns the remaining
// stations as a non-nil slice — empty `[]`, never `nil`, so the JSON shape
// stays stable for frontend iterators.
func excludeSibling(siblings []contracts.RadioSiblingStationResponse, excludeID uint) []contracts.RadioSiblingStationResponse {
	out := make([]contracts.RadioSiblingStationResponse, 0, len(siblings))
	for _, sib := range siblings {
		if sib.ID == excludeID {
			continue
		}
		out = append(out, sib)
	}
	return out
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
		ScheduleLocked:  show.ScheduleLocked,
		GenreTags:       show.GenreTags,
		ArchiveURL:      show.ArchiveURL,
		ImageURL:        show.ImageURL,
		IsActive:        show.IsActive,
		LifecycleState:  show.LifecycleState,
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

	// Flag a not-yet-aired episode (PSY-1205) so the detail page labels it
	// "upcoming" instead of "aired {future date}". Resolved against the show's
	// station-local today; the station is already preloaded (callers
	// Preload("Show.Station")), so read its timezone directly — no extra query.
	today, err := s.stationLocalToday(episode.Show.Station.Timezone)
	if err != nil {
		return nil, err
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
		IsUpcoming:      normalizeDate(episode.AirDate) > today,
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
