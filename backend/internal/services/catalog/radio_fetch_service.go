package catalog

import (
	"context"
	"log/slog"
	"os"
	"strconv"
	"sync"
	"time"

	"psychic-homily-backend/internal/services/contracts"
)

// Default radio fetch interval (6 hours)
const DefaultRadioFetchInterval = 6 * time.Hour

// Default affinity computation interval (24 hours)
const DefaultAffinityInterval = 24 * time.Hour

// Default re-matching interval (7 days)
const DefaultReMatchInterval = 7 * 24 * time.Hour

// radioCircuitBreakerThreshold is the number of consecutive failures before
// a station is temporarily skipped during fetch cycles.
const radioCircuitBreakerThreshold = 5

// RadioFetchService is a background service that periodically:
//  1. Fetches new episodes from all active stations with playlist sources (every 6h)
//  2. Computes artist affinity from co-occurrence data (daily)
//  3. Re-matches unmatched plays against newly added artists (weekly)
//
// It follows the same Start/Stop pattern as SchedulerService and other background services.
type RadioFetchService struct {
	radioService   *RadioService
	discordService contracts.DiscordServiceInterface

	fetchInterval    time.Duration
	affinityInterval time.Duration
	rematchInterval  time.Duration

	stopCh chan struct{}
	wg     sync.WaitGroup
	logger *slog.Logger

	// consecutiveFailures tracks per-station failures within a fetch cycle
	// Reset on success, incremented on failure. Stations with >= threshold
	// failures are skipped until the counter resets.
	mu                  sync.Mutex
	consecutiveFailures map[uint]int
}

// NewRadioFetchService creates a new radio fetch background service.
// Env vars:
//   - RADIO_FETCH_INTERVAL_HOURS (default 6)
//   - RADIO_AFFINITY_INTERVAL_HOURS (default 24)
//   - RADIO_REMATCH_INTERVAL_HOURS (default 168, i.e. 7 days)
func NewRadioFetchService(
	radioService *RadioService,
	discordService contracts.DiscordServiceInterface,
) *RadioFetchService {
	fetchInterval := DefaultRadioFetchInterval
	if envVal := os.Getenv("RADIO_FETCH_INTERVAL_HOURS"); envVal != "" {
		if hours, err := strconv.Atoi(envVal); err == nil && hours > 0 {
			fetchInterval = time.Duration(hours) * time.Hour
		}
	}

	affinityInterval := DefaultAffinityInterval
	if envVal := os.Getenv("RADIO_AFFINITY_INTERVAL_HOURS"); envVal != "" {
		if hours, err := strconv.Atoi(envVal); err == nil && hours > 0 {
			affinityInterval = time.Duration(hours) * time.Hour
		}
	}

	rematchInterval := DefaultReMatchInterval
	if envVal := os.Getenv("RADIO_REMATCH_INTERVAL_HOURS"); envVal != "" {
		if hours, err := strconv.Atoi(envVal); err == nil && hours > 0 {
			rematchInterval = time.Duration(hours) * time.Hour
		}
	}

	return &RadioFetchService{
		radioService:        radioService,
		discordService:      discordService,
		fetchInterval:       fetchInterval,
		affinityInterval:    affinityInterval,
		rematchInterval:     rematchInterval,
		stopCh:              make(chan struct{}),
		logger:              slog.Default(),
		consecutiveFailures: make(map[uint]int),
	}
}

// Start begins the background radio fetch service.
func (s *RadioFetchService) Start(ctx context.Context) {
	s.wg.Add(1)
	go s.runFetchLoop(ctx)
	s.wg.Add(1)
	go s.runAffinityLoop(ctx)
	s.wg.Add(1)
	go s.runReMatchLoop(ctx)

	s.logger.Info("radio fetch service started",
		"fetch_interval_hours", s.fetchInterval.Hours(),
		"affinity_interval_hours", s.affinityInterval.Hours(),
		"rematch_interval_hours", s.rematchInterval.Hours(),
	)
}

// Stop gracefully stops the radio fetch service.
func (s *RadioFetchService) Stop() {
	close(s.stopCh)
	s.wg.Wait()
	s.logger.Info("radio fetch service stopped")
}

// runFetchLoop runs the periodic station fetch cycle.
func (s *RadioFetchService) runFetchLoop(ctx context.Context) {
	defer s.wg.Done()

	// Run immediately on startup
	s.runFetchCycle()

	ticker := time.NewTicker(s.fetchInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-s.stopCh:
			return
		case <-ticker.C:
			s.runFetchCycle()
		}
	}
}

// runAffinityLoop runs the periodic affinity computation.
func (s *RadioFetchService) runAffinityLoop(ctx context.Context) {
	defer s.wg.Done()

	// Don't run immediately — let the first fetch cycle complete
	ticker := time.NewTicker(s.affinityInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-s.stopCh:
			return
		case <-ticker.C:
			s.runAffinityCycle()
		}
	}
}

// runReMatchLoop runs the periodic re-matching of unmatched plays.
func (s *RadioFetchService) runReMatchLoop(ctx context.Context) {
	defer s.wg.Done()

	// Don't run immediately — let the first fetch cycle complete
	ticker := time.NewTicker(s.rematchInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-s.stopCh:
			return
		case <-ticker.C:
			s.runReMatchCycle()
		}
	}
}

// runFetchCycle fetches new episodes from all active stations sequentially.
func (s *RadioFetchService) runFetchCycle() {
	cycleStart := time.Now()
	s.logger.Info("starting radio fetch cycle")

	stations, err := s.radioService.GetActiveStationsWithPlaylistSource()
	if err != nil {
		s.logger.Error("failed to list active stations", "error", err)
		return
	}

	if len(stations) == 0 {
		s.logger.Info("no active stations with playlist source found")
		return
	}

	var (
		totalProcessed int
		totalEpisodes  int
		totalPlays     int
		totalMatched   int
		totalFailed    int
	)

	// Process stations sequentially to respect per-provider rate limits
	for _, station := range stations {
		// Check circuit breaker
		s.mu.Lock()
		failures := s.consecutiveFailures[station.ID]
		s.mu.Unlock()

		if failures >= radioCircuitBreakerThreshold {
			s.logger.Warn("skipping station (circuit breaker)",
				"station_id", station.ID,
				"station_name", station.Name,
				"consecutive_failures", failures,
			)
			continue
		}

		totalProcessed++
		s.logger.Info("fetching station",
			"station_id", station.ID,
			"station_name", station.Name,
		)

		result, err := s.radioService.FetchNewEpisodes(station.ID)
		if err != nil {
			totalFailed++
			s.logger.Error("station fetch failed",
				"station_id", station.ID,
				"station_name", station.Name,
				"error", err,
			)

			s.mu.Lock()
			s.consecutiveFailures[station.ID]++
			s.mu.Unlock()

			continue
		}

		// Reset circuit breaker on success
		s.mu.Lock()
		s.consecutiveFailures[station.ID] = 0
		s.mu.Unlock()

		totalEpisodes += result.EpisodesImported
		totalPlays += result.PlaysImported
		totalMatched += result.PlaysMatched

		s.logger.Info("station fetch complete",
			"station_id", station.ID,
			"station_name", station.Name,
			"episodes_imported", result.EpisodesImported,
			"plays_imported", result.PlaysImported,
			"plays_matched", result.PlaysMatched,
		)

		if len(result.Errors) > 0 {
			s.logger.Warn("station fetch had errors",
				"station_id", station.ID,
				"station_name", station.Name,
				"error_count", len(result.Errors),
			)
		}
	}

	cycleDuration := time.Since(cycleStart)
	s.logger.Info("radio fetch cycle complete",
		"stations_processed", totalProcessed,
		"episodes_imported", totalEpisodes,
		"plays_imported", totalPlays,
		"plays_matched", totalMatched,
		"failures", totalFailed,
		"duration", cycleDuration,
	)
}

// runAffinityCycle computes the artist affinity table.
func (s *RadioFetchService) runAffinityCycle() {
	start := time.Now()
	s.logger.Info("starting affinity computation")

	if err := s.radioService.ComputeAffinity(); err != nil {
		s.logger.Error("affinity computation failed", "error", err)
		return
	}

	s.logger.Info("affinity computation complete", "duration", time.Since(start))
}

// runReMatchCycle re-matches unmatched plays against current artists.
func (s *RadioFetchService) runReMatchCycle() {
	start := time.Now()
	s.logger.Info("starting re-match of unmatched plays")

	result, err := s.radioService.ReMatchUnmatched()
	if err != nil {
		s.logger.Error("re-match failed", "error", err)
		return
	}

	s.logger.Info("re-match complete",
		"total", result.Total,
		"matched", result.Matched,
		"unmatched", result.Unmatched,
		"duration", time.Since(start),
	)
}

// GetConsecutiveFailures returns the failure count for a station. Exported for testing.
func (s *RadioFetchService) GetConsecutiveFailures(stationID uint) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.consecutiveFailures[stationID]
}

// SetConsecutiveFailures sets the failure count for a station. Exported for testing.
func (s *RadioFetchService) SetConsecutiveFailures(stationID uint, count int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.consecutiveFailures[stationID] = count
}

// RunFetchCycleNow triggers an immediate fetch cycle (useful for testing/admin).
func (s *RadioFetchService) RunFetchCycleNow() {
	s.runFetchCycle()
}

// RunAffinityCycleNow triggers an immediate affinity computation (useful for testing/admin).
func (s *RadioFetchService) RunAffinityCycleNow() {
	s.runAffinityCycle()
}

// RunReMatchCycleNow triggers an immediate re-match cycle (useful for testing/admin).
func (s *RadioFetchService) RunReMatchCycleNow() {
	s.runReMatchCycle()
}
