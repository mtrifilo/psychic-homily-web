package catalog

import (
	"errors"
	"fmt"

	"gorm.io/gorm"

	apperrors "psychic-homily-backend/internal/errors"
	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/contracts"
)

// radio_observability.go — admin observability READ surfaces over the radio_sync_runs
// and radio_station_health tables (PSY-1129, P5). All read-only: the rows are written
// by RunStationSync (PSY-1134) and updateStationHealth (PSY-1140). These back the admin
// sync-run feed (PSY-1130) and station health cards (PSY-1200).

const (
	// defaultSyncRunPageSize / maxSyncRunPageSize bound a single feed page so an
	// unbounded limit can't pull the whole table — each run preloads its error list.
	defaultSyncRunPageSize = 50
	maxSyncRunPageSize     = 100
)

// Sync-run feed scope values (PSY-1343). The PSY-1333 slot fetch writes a
// show-scoped fetch row per slot boundary — tens per day on a schedule-bearing
// station — so the feed needs a way to separate them from the handful of daily
// station sweeps the operator usually cares about.
//
// KEEP IN SYNC (string-typed across three layers, nothing fails at compile
// time): the huma `enum:"all,sweep,scoped"` tags on both AdminList*SyncRuns
// requests (api/handlers/catalog/radio.go) and the FE `SyncRunScope` union
// (frontend/lib/hooks/admin/useAdminRadio.ts).
const (
	// SyncRunScopeAll — no scope filter (every run type, scoped or not).
	SyncRunScopeAll = "all"
	// SyncRunScopeSweep — hide show-scoped FETCH rows (the slot-fetch flood).
	// Discover/backfill/janitor rows all still show; backfills carry a show_id
	// too but are operator-initiated and rare, so they stay visible.
	SyncRunScopeSweep = "sweep"
	// SyncRunScopeScoped — ONLY show-scoped fetch rows (inspect the slot fetcher).
	SyncRunScopeScoped = "scoped"
)

// ListSyncRuns returns recent sync runs newest-first for the admin feed. stationID nil
// = global (across all stations); non-nil scopes to one station (404 if it doesn't
// exist). status, when non-empty, filters to that exact run status (an unknown status
// simply yields no rows — a forgiving filter for an internal admin tool). scope is one
// of the SyncRunScope* values; empty or unknown behaves as SyncRunScopeAll. NOTE: the
// unknown-value leniency is defense-in-depth for direct callers only — through the API
// an unknown scope never reaches here (the huma enum tag 422s it). Returns the page
// plus the total
// count of the matched set (for pagination). Station/Show/Errors are preloaded so the
// feed renders names + an error summary in one round trip.
func (s *RadioService) ListSyncRuns(stationID *uint, status, scope string, limit, offset int) ([]*contracts.RadioSyncRunResponse, int64, error) {
	if s.db == nil {
		return nil, 0, fmt.Errorf("database not initialized")
	}

	if stationID != nil {
		if err := s.assertStationExists(*stationID); err != nil {
			return nil, 0, err
		}
	}

	if limit <= 0 {
		limit = defaultSyncRunPageSize
	}
	if limit > maxSyncRunPageSize {
		limit = maxSyncRunPageSize
	}
	if offset < 0 {
		offset = 0
	}

	// Apply the same filters to two independent queries (count + page) so they can't
	// drift and we avoid reusing one mutated *gorm.DB across executions.
	applyFilters := func(db *gorm.DB) *gorm.DB {
		if stationID != nil {
			db = db.Where("station_id = ?", *stationID)
		}
		if status != "" {
			db = db.Where("status = ?", status)
		}
		switch scope {
		case SyncRunScopeSweep:
			db = db.Where("NOT (run_type = ? AND show_id IS NOT NULL)", catalogm.RadioSyncRunTypeFetch)
		case SyncRunScopeScoped:
			db = db.Where("run_type = ? AND show_id IS NOT NULL", catalogm.RadioSyncRunTypeFetch)
		}
		return db
	}

	var total int64
	if err := applyFilters(s.db.Model(&catalogm.RadioSyncRun{})).Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("counting sync runs: %w", err)
	}

	var runs []catalogm.RadioSyncRun
	if err := applyFilters(s.db.Model(&catalogm.RadioSyncRun{})).
		Preload("Station").
		Preload("Show").
		Preload("Errors").
		Order("started_at DESC, id DESC").
		Limit(limit).
		Offset(offset).
		Find(&runs).Error; err != nil {
		return nil, 0, fmt.Errorf("listing sync runs: %w", err)
	}

	responses := make([]*contracts.RadioSyncRunResponse, len(runs))
	for i := range runs {
		responses[i] = syncRunToResponse(&runs[i])
	}
	return responses, total, nil
}

// GetStationHealth reads one station's radio_station_health rollup for the health-card
// UI. 404s if the station doesn't exist; a station that exists but has never run has no
// health row, so a zero-value response (rates nil, consecutive_failures 0, breaker
// closed) is synthesized so the card still renders ("never run").
func (s *RadioService) GetStationHealth(stationID uint) (*contracts.RadioStationHealthResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	var station catalogm.RadioStation
	if err := s.db.Select("id", "name", "slug").First(&station, stationID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperrors.ErrRadioStationNotFound(stationID)
		}
		return nil, fmt.Errorf("loading station: %w", err)
	}

	var health catalogm.RadioStationHealth
	err := s.db.First(&health, "station_id = ?", stationID).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return zeroStationHealthResponse(station.ID, station.Name, station.Slug), nil
		}
		return nil, fmt.Errorf("loading station health: %w", err)
	}
	return stationHealthToResponse(&health, station.Name, station.Slug), nil
}

// ListStationHealth returns one health card per station (name ASC) for the admin
// dashboard. Stations without a health row yet still appear with a zero-value
// ("never run") response, so the dashboard surfaces every station.
func (s *RadioService) ListStationHealth() ([]*contracts.RadioStationHealthResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	var stations []catalogm.RadioStation
	if err := s.db.Select("id", "name", "slug").Order("name ASC").Find(&stations).Error; err != nil {
		return nil, fmt.Errorf("listing stations: %w", err)
	}

	var healthRows []catalogm.RadioStationHealth
	if err := s.db.Find(&healthRows).Error; err != nil {
		return nil, fmt.Errorf("listing station health: %w", err)
	}
	byStation := make(map[uint]*catalogm.RadioStationHealth, len(healthRows))
	for i := range healthRows {
		byStation[healthRows[i].StationID] = &healthRows[i]
	}

	out := make([]*contracts.RadioStationHealthResponse, len(stations))
	for i := range stations {
		st := &stations[i]
		if h, ok := byStation[st.ID]; ok {
			out[i] = stationHealthToResponse(h, st.Name, st.Slug)
		} else {
			out[i] = zeroStationHealthResponse(st.ID, st.Name, st.Slug)
		}
	}
	return out, nil
}

// stationHealthToResponse maps a radio_station_health row onto its DTO, carrying the
// station's name/slug (joined by the caller).
func stationHealthToResponse(h *catalogm.RadioStationHealth, name, slug string) *contracts.RadioStationHealthResponse {
	updatedAt := h.UpdatedAt
	return &contracts.RadioStationHealthResponse{
		StationID:           h.StationID,
		StationName:         name,
		StationSlug:         slug,
		LastSuccessAt:       h.LastSuccessAt,
		LastRunAt:           h.LastRunAt,
		ConsecutiveFailures: h.ConsecutiveFailures,
		BreakerState:        h.BreakerState,
		BreakerTrippedAt:    h.BreakerTrippedAt,
		RecentSuccessRate:   h.RecentSuccessRate,
		PlayMatchRate:       h.PlayMatchRate,
		ZeroPlayEpisodeRate: h.ZeroPlayEpisodeRate,
		UpdatedAt:           &updatedAt,
	}
}

// zeroStationHealthResponse is the "never run" card for a station with no health row:
// nil rates (distinct from 0.0), zero consecutive failures, closed breaker.
func zeroStationHealthResponse(stationID uint, name, slug string) *contracts.RadioStationHealthResponse {
	return &contracts.RadioStationHealthResponse{
		StationID:    stationID,
		StationName:  name,
		StationSlug:  slug,
		BreakerState: catalogm.RadioBreakerStateClosed,
	}
}
