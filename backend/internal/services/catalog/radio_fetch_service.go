package catalog

import (
	"context"
	"log/slog"
	"os"
	"strconv"
	"sync"
	"time"

	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/services/shared"
)

// Default radio fetch interval (6 hours)
const DefaultRadioFetchInterval = 6 * time.Hour

// Default affinity computation interval (24 hours)
const DefaultAffinityInterval = 24 * time.Hour

// Default re-matching interval (7 days)
const DefaultReMatchInterval = 7 * 24 * time.Hour

// Default show-discovery interval (24 hours). Provider rosters change slowly
// (WFMU adds shows monthly at most; KEXP/NTS slower), so once a day is plenty
// and the per-station DJ-index fetch cost is negligible.
const DefaultDiscoverInterval = 24 * time.Hour

// Default auto-backfill window (90 days). When the discover loop finds a new
// show, an import job is created + started for the last N days so the show
// arrives with some history instead of just the next ~7 days of episodes.
// Set RADIO_AUTO_BACKFILL_DAYS=0 to disable auto-backfill entirely.
const DefaultAutoBackfillDays = 90

// autoBackfillPollInterval is how often the per-station backfill goroutine
// polls a running import job for completion. The job updates its DB row every
// 10 episodes; 5s is comfortably finer-grained than typical job tick rates.
const autoBackfillPollInterval = 5 * time.Second

// radioCircuitBreakerThreshold is the number of consecutive failures before
// a station is temporarily skipped during fetch cycles.
const radioCircuitBreakerThreshold = 5

// RadioFetchService is a background service that periodically:
//  1. Fetches new episodes from all active stations with playlist sources (every 6h)
//  2. Computes artist affinity from co-occurrence data (daily)
//  3. Re-matches unmatched plays against newly added artists (weekly)
//  4. Discovers newly-added shows on every active station (daily)
//
// It follows the same Start/Stop pattern as SchedulerService and other background services.
type RadioFetchService struct {
	radioService   *RadioService
	discordService contracts.DiscordServiceInterface

	fetchInterval    time.Duration
	affinityInterval time.Duration
	rematchInterval  time.Duration
	discoverInterval time.Duration

	// autoBackfillDays: how far back to backfill when discovery finds a new show.
	// 0 disables auto-backfill (admins can still manually trigger via /admin/radio-shows/{id}/import-job).
	autoBackfillDays int

	stopCh chan struct{}
	wg     sync.WaitGroup
	logger *slog.Logger

	// consecutiveFailures tracks per-station failures within a fetch cycle
	// Reset on success, incremented on failure. Stations with >= threshold
	// failures are skipped until the counter resets. Shared across the fetch
	// and discover loops so a wedged provider gets one circuit breaker, not two.
	mu                  sync.Mutex
	consecutiveFailures map[uint]int
}

// NewRadioFetchService creates a new radio fetch background service.
// Env vars:
//   - RADIO_FETCH_INTERVAL_HOURS (default 6)
//   - RADIO_AFFINITY_INTERVAL_HOURS (default 24)
//   - RADIO_REMATCH_INTERVAL_HOURS (default 168, i.e. 7 days)
//   - RADIO_DISCOVER_INTERVAL_HOURS (default 24)
//   - RADIO_AUTO_BACKFILL_DAYS (default 90; 0 disables auto-backfill)
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

	discoverInterval := DefaultDiscoverInterval
	if envVal := os.Getenv("RADIO_DISCOVER_INTERVAL_HOURS"); envVal != "" {
		if hours, err := strconv.Atoi(envVal); err == nil && hours > 0 {
			discoverInterval = time.Duration(hours) * time.Hour
		}
	}

	// 0 explicitly disables auto-backfill. Negative values are silently treated
	// as 0 (defensive). Default 90 if env unset or invalid.
	autoBackfillDays := DefaultAutoBackfillDays
	if envVal := os.Getenv("RADIO_AUTO_BACKFILL_DAYS"); envVal != "" {
		if days, err := strconv.Atoi(envVal); err == nil && days >= 0 {
			autoBackfillDays = days
		}
	}

	return &RadioFetchService{
		radioService:        radioService,
		discordService:      discordService,
		fetchInterval:       fetchInterval,
		affinityInterval:    affinityInterval,
		rematchInterval:     rematchInterval,
		discoverInterval:    discoverInterval,
		autoBackfillDays:    autoBackfillDays,
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
	s.wg.Add(1)
	go s.runDiscoverLoop(ctx)

	s.logger.Info("radio fetch service started",
		"fetch_interval_hours", s.fetchInterval.Hours(),
		"affinity_interval_hours", s.affinityInterval.Hours(),
		"rematch_interval_hours", s.rematchInterval.Hours(),
		"discover_interval_hours", s.discoverInterval.Hours(),
	)
}

// Stop gracefully stops the radio fetch service.
func (s *RadioFetchService) Stop() {
	close(s.stopCh)
	s.wg.Wait()
	s.logger.Info("radio fetch service stopped")
}

// runFetchLoop runs the periodic station fetch cycle. Runs once on startup.
func (s *RadioFetchService) runFetchLoop(ctx context.Context) {
	defer s.wg.Done()
	shared.RunTickerLoop(ctx, "radio_fetch", s.fetchInterval, s.stopCh, true, func(_ context.Context) {
		s.runFetchCycle()
	})
}

// runAffinityLoop runs the periodic affinity computation.
// No startup cycle — the first fetch cycle is allowed to complete first.
func (s *RadioFetchService) runAffinityLoop(ctx context.Context) {
	defer s.wg.Done()
	shared.RunTickerLoop(ctx, "radio_affinity", s.affinityInterval, s.stopCh, false, func(_ context.Context) {
		s.runAffinityCycle()
	})
}

// runReMatchLoop runs the periodic re-matching of unmatched plays.
// No startup cycle — the first fetch cycle is allowed to complete first.
func (s *RadioFetchService) runReMatchLoop(ctx context.Context) {
	defer s.wg.Done()
	shared.RunTickerLoop(ctx, "radio_rematch", s.rematchInterval, s.stopCh, false, func(_ context.Context) {
		s.runReMatchCycle()
	})
}

// runDiscoverLoop runs the periodic show-discovery cycle. Runs immediately on
// startup so operators see output without waiting a full interval.
func (s *RadioFetchService) runDiscoverLoop(ctx context.Context) {
	defer s.wg.Done()
	shared.RunTickerLoop(ctx, "radio_discover", s.discoverInterval, s.stopCh, true, func(_ context.Context) {
		s.runDiscoverCycle()
	})
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

// runAffinityCycle computes the artist affinity table and syncs to artist relationships.
func (s *RadioFetchService) runAffinityCycle() {
	start := time.Now()
	s.logger.Info("starting affinity computation")

	if err := s.radioService.ComputeAffinity(); err != nil {
		s.logger.Error("affinity computation failed", "error", err)
		return
	}

	s.logger.Info("affinity computation complete", "duration", time.Since(start))

	// Sync affinity data to artist_relationships as radio_cooccurrence type
	syncStart := time.Now()
	s.logger.Info("starting affinity-to-relationship sync")

	syncResult, err := s.radioService.SyncAffinityToRelationships()
	if err != nil {
		s.logger.Error("affinity-to-relationship sync failed", "error", err)
		return
	}

	s.logger.Info("affinity-to-relationship sync complete",
		"created", syncResult.Created,
		"updated", syncResult.Updated,
		"deleted", syncResult.Deleted,
		"failed", syncResult.Failed,
		"duration", time.Since(syncStart),
	)
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

// runDiscoverCycle calls DiscoverStationShows for every active station with a
// playlist source. The cycle is idempotent — existing shows are no-ops; only
// brand-new rows fire the per-station Discord notification.
func (s *RadioFetchService) runDiscoverCycle() {
	cycleStart := time.Now()
	s.logger.Info("starting radio discover cycle")

	stations, err := s.radioService.GetActiveStationsWithPlaylistSource()
	if err != nil {
		s.logger.Error("failed to list active stations for discover", "error", err)
		return
	}

	if len(stations) == 0 {
		s.logger.Info("no active stations with playlist source found for discover")
		return
	}

	var (
		totalProcessed   int
		totalDiscovered  int
		totalNew         int
		totalFailed      int
		stationsNotified int
	)

	for _, station := range stations {
		s.mu.Lock()
		failures := s.consecutiveFailures[station.ID]
		s.mu.Unlock()

		if failures >= radioCircuitBreakerThreshold {
			s.logger.Warn("skipping station for discover (circuit breaker)",
				"station_id", station.ID,
				"station_name", station.Name,
				"consecutive_failures", failures,
			)
			continue
		}

		totalProcessed++
		s.logger.Info("discovering shows for station",
			"station_id", station.ID,
			"station_name", station.Name,
		)

		result, err := s.radioService.DiscoverStationShows(station.ID)
		if err != nil {
			totalFailed++
			s.logger.Error("station discover failed",
				"station_id", station.ID,
				"station_name", station.Name,
				"error", err,
			)

			s.mu.Lock()
			s.consecutiveFailures[station.ID]++
			s.mu.Unlock()
			continue
		}

		s.mu.Lock()
		s.consecutiveFailures[station.ID] = 0
		s.mu.Unlock()

		totalDiscovered += result.ShowsDiscovered
		totalNew += result.ShowsNew

		s.logger.Info("station discover complete",
			"station_id", station.ID,
			"station_name", station.Name,
			"shows_discovered", result.ShowsDiscovered,
			"shows_new", result.ShowsNew,
			"error_count", len(result.Errors),
		)

		if result.ShowsNew > 0 && s.discordService != nil {
			s.discordService.NotifyNewRadioShows(station.Name, result.NewShowNames)
			stationsNotified++
		}

		// Kick off the per-station auto-backfill drain in its own goroutine so
		// the discover cycle returns immediately. Serializes jobs PER STATION
		// (one at a time) so a 56-show provider burst doesn't fan out into
		// concurrent provider hits — the existing 1-rps per-instance rate
		// limiter on each provider handles per-episode pacing.
		if s.autoBackfillDays > 0 && len(result.NewShowIDs) > 0 {
			// GoSafe contains a panic here: this child goroutine escapes the
			// discover tick's RunTickerLoop guard, which only covers the tick
			// goroutine itself. autoBackfillStation defers s.wg.Done(), which
			// runs during unwind before GoSafe's recover, so the WaitGroup
			// stays balanced even on panic.
			s.wg.Add(1)
			showIDs := result.NewShowIDs
			showNames := result.NewShowNames
			stationName := station.Name
			shared.GoSafe(context.Background(), "radio_auto_backfill", func() {
				s.autoBackfillStation(stationName, showIDs, showNames)
			})
		}
	}

	s.logger.Info("radio discover cycle complete",
		"stations_processed", totalProcessed,
		"shows_discovered", totalDiscovered,
		"shows_new", totalNew,
		"failures", totalFailed,
		"stations_notified", stationsNotified,
		"duration", time.Since(cycleStart),
	)
}

// autoBackfillStation drains a per-station batch of newly-discovered shows
// SERIALLY — one import job at a time per station, polling each to completion
// before starting the next. Single batched Discord notification at the end.
//
// Why serial: provider HTTP clients are rate-limited at 1 req/sec PER INSTANCE
// (existing wfmuRateLimit / kexpRateLimit / ntsRateLimit). Each import job
// gets its own provider instance via getProvider, so parallel jobs would each
// have an independent rate limiter and could collectively hit a provider
// faster than the per-instance cap intends. Per-station serialization keeps
// effective egress at ~1 req/sec/provider.
//
// Process lifetime: the goroutine respects s.stopCh so a service Stop()
// abandons pending jobs cleanly (no orphan ticker goroutines). Already-started
// jobs continue in their own runImportJob goroutine until they finish or are
// cancelled.
func (s *RadioFetchService) autoBackfillStation(stationName string, showIDs []uint, showNames []string) {
	defer s.wg.Done()

	until := time.Now()
	since := until.AddDate(0, 0, -s.autoBackfillDays)
	sinceStr := since.Format("2006-01-02")
	untilStr := until.Format("2006-01-02")

	s.logger.Info("auto_backfill_started",
		"station", stationName,
		"shows", len(showIDs),
		"since", sinceStr,
		"until", untilStr,
	)

	var (
		completedShows []string
		totalEpisodes  int
		totalPlays     int
	)

	for i, showID := range showIDs {
		showName := showNames[i]

		job, err := s.radioService.CreateImportJob(showID, sinceStr, untilStr)
		if err != nil {
			s.logger.Warn("auto_backfill_create_job_failed",
				"show_id", showID,
				"show", showName,
				"error", err,
			)
			continue
		}

		if err := s.radioService.StartImportJob(job.ID); err != nil {
			s.logger.Warn("auto_backfill_start_job_failed",
				"show_id", showID,
				"job_id", job.ID,
				"error", err,
			)
			continue
		}

		finalJob, result := s.waitForJobCompletion(job.ID)
		switch result {
		case jobWaitShutdown:
			s.logger.Info("auto_backfill_abandoned_on_shutdown", "job_id", job.ID)
			return
		case jobWaitPollError:
			continue
		}

		if finalJob.Status == catalogm.RadioImportJobStatusCompleted {
			completedShows = append(completedShows, showName)
			totalEpisodes += finalJob.EpisodesImported
			totalPlays += finalJob.PlaysMatched
		} else {
			s.logger.Warn("auto_backfill_job_did_not_complete",
				"job_id", job.ID,
				"status", finalJob.Status,
			)
		}
	}

	s.logger.Info("auto_backfill_finished",
		"station", stationName,
		"completed", len(completedShows),
		"episodes_imported", totalEpisodes,
		"plays_matched", totalPlays,
	)

	if s.discordService != nil && len(completedShows) > 0 {
		s.discordService.NotifyBackfillCompleted(stationName, completedShows, totalEpisodes, totalPlays)
	}
}

// jobWaitResult distinguishes the three outcomes of waitForJobCompletion so the
// caller can route shutdown vs poll-error vs terminal without bool-overloading.
type jobWaitResult int

const (
	jobWaitTerminal  jobWaitResult = iota // job reached completed/failed/cancelled — *Job is non-nil
	jobWaitShutdown                       // service shutting down — caller should return
	jobWaitPollError                      // GetImportJob failed — caller should continue to next show
)

// waitForJobCompletion polls a running import job every autoBackfillPollInterval
// until it reaches a terminal status (catalogm.RadioImportJobStatus{Completed,
// Failed,Cancelled}), or returns earlier if s.stopCh closes (shutdown) or a
// GetImportJob call errors (poll error).
func (s *RadioFetchService) waitForJobCompletion(jobID uint) (*contracts.RadioImportJobResponse, jobWaitResult) {
	for {
		select {
		case <-s.stopCh:
			return nil, jobWaitShutdown
		case <-time.After(autoBackfillPollInterval):
		}

		job, err := s.radioService.GetImportJob(jobID)
		if err != nil {
			s.logger.Warn("auto_backfill_poll_job_load_failed", "job_id", jobID, "error", err)
			return nil, jobWaitPollError
		}
		switch job.Status {
		case catalogm.RadioImportJobStatusCompleted,
			catalogm.RadioImportJobStatusFailed,
			catalogm.RadioImportJobStatusCancelled:
			return job, jobWaitTerminal
		}
	}
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

// RunDiscoverCycleNow triggers an immediate discover cycle (useful for testing/admin).
func (s *RadioFetchService) RunDiscoverCycleNow() {
	s.runDiscoverCycle()
}
