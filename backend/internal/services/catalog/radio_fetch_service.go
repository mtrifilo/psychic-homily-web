package catalog

import (
	"context"
	"errors"
	"log/slog"
	"net"
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

// radioCircuitBreakerThreshold is the number of consecutive PERMANENT failures
// before a station is temporarily skipped during fetch cycles. Transient errors
// (timeout, connection refused, 429) bump a separate counter and trigger a
// single in-cycle retry instead of incrementing this one (PSY-887).
const radioCircuitBreakerThreshold = 5

// radioTransientRetryBackoff is the brief delay before retrying a station once
// after a transient error. Single retry per station per cycle; we don't carry
// transient-retry state across cycles (no exponential backoff) — that's an
// explicit non-goal of PSY-887 to keep the fetch loop's failure modes obvious.
const radioTransientRetryBackoff = 500 * time.Millisecond

// errorKind classifies a radio provider error for circuit-breaker routing.
// kindTransient → bump transientFailures + retry once, do NOT trip breaker.
// kindPermanent → bump consecutiveFailures, trip breaker at threshold.
type errorKind int

const (
	kindPermanent errorKind = iota // default — string-only errors land here too
	kindTransient
)

// classifyError routes a radio provider error to transient vs permanent per
// PSY-887. Type-assertion-based, not string-matching:
//   - context.DeadlineExceeded                  → transient
//   - any error implementing net.Error.Timeout() → transient
//   - *net.OpError                              → transient (covers connection refused,
//     network unreachable, EOF on idle socket — all worth one retry)
//   - *RadioHTTPError                           → 429 transient, other non-OK permanent
//   - errors.Is(err, ErrTransient)              → transient (manual provider tag)
//   - errors.Is(err, ErrPermanent)              → permanent (manual provider tag)
//   - anything else (parse, schema, db, setup)  → permanent
//
// Operates on the WHOLE error chain via errors.As / errors.Is so wrapped
// errors (fmt.Errorf("...: %w", err)) classify correctly. A non-2xx HTTP
// response wrapped in five layers of fmt.Errorf still routes by status code.
func classifyError(err error) errorKind {
	if err == nil {
		return kindPermanent // caller shouldn't pass nil; default safely
	}

	// Context-level timeout — the surrounding ctx hit its deadline mid-request.
	if errors.Is(err, context.DeadlineExceeded) {
		return kindTransient
	}

	// HTTP-level classification via RadioHTTPError.Unwrap() chain. Has to come
	// before the net.Error check because http.Client errors can satisfy
	// net.Error too (via *url.Error.Timeout()) — but we want 429 routed
	// specifically, not lumped in with generic "network timeout".
	var httpErr *RadioHTTPError
	if errors.As(err, &httpErr) {
		// Unwrap() returns ErrTransient for 429, ErrPermanent otherwise.
		if errors.Is(httpErr, ErrTransient) {
			return kindTransient
		}
		return kindPermanent
	}

	// Generic net.Error.Timeout() catches both *url.Error (http.Client.Timeout)
	// and any other net-package error chain. The interface is the idiomatic
	// classifier; *url.Error implements it directly.
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return kindTransient
	}

	// *net.OpError catches connection refused, network unreachable, EOF on
	// an idle socket, etc. — anything dial-level worth one retry.
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		return kindTransient
	}

	// Last resort: manual provider tags. If a provider wraps an error with
	// errors.Join(ErrTransient, ...) without RadioHTTPError, honor it.
	if errors.Is(err, ErrTransient) {
		return kindTransient
	}
	if errors.Is(err, ErrPermanent) {
		return kindPermanent
	}

	// Parse failures, schema mismatches, db setup errors — all permanent.
	return kindPermanent
}

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

	// consecutiveFailures tracks per-station PERMANENT failures within a fetch
	// cycle. Reset on success. Stations with >= threshold permanent failures are
	// skipped until the counter resets. Shared across the fetch and discover
	// loops so a wedged provider gets one circuit breaker, not two (per the
	// shared-failures-map design intent from PSY-671).
	//
	// transientFailures (PSY-887): tracks transient errors (timeout, conn
	// refused, 429) separately. These do NOT trip the breaker; they trigger a
	// single in-cycle retry with brief backoff. Reset on success alongside
	// consecutiveFailures. Exposed via GetTransientFailures for testing /
	// observability. Kept as a separate map so a station with intermittent
	// network blips on a healthy provider doesn't get wedged for the rest of
	// the 6h cycle (the original PSY-887 bug).
	mu                  sync.Mutex
	consecutiveFailures map[uint]int
	transientFailures   map[uint]int
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
		transientFailures:   make(map[uint]int),
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

// stationBreakerSkip reports whether the station should be skipped this cycle
// because its PERMANENT failure count is at threshold. Transient failures are
// tracked separately and do NOT cause skip (PSY-887).
//
// Locks/unlocks internally — caller MUST NOT hold s.mu.
func (s *RadioFetchService) stationBreakerSkip(stationID uint) (skip bool, failures int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	failures = s.consecutiveFailures[stationID]
	return failures >= radioCircuitBreakerThreshold, failures
}

// recordStationSuccess resets BOTH counters for a station after a successful
// fetch. Reset transientFailures too so a station that recovered from a
// transient blip doesn't carry the count into the next blip (PSY-887).
func (s *RadioFetchService) recordStationSuccess(stationID uint) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.consecutiveFailures[stationID] = 0
	s.transientFailures[stationID] = 0
}

// recordStationFailure routes a fetch error to the right counter per PSY-887.
// Returns the classification so callers can branch on retry-or-skip without a
// second classifyError call.
func (s *RadioFetchService) recordStationFailure(stationID uint, err error) errorKind {
	kind := classifyError(err)
	s.mu.Lock()
	defer s.mu.Unlock()
	if kind == kindTransient {
		s.transientFailures[stationID]++
	} else {
		s.consecutiveFailures[stationID]++
	}
	return kind
}

// fetchStationWithRetry calls FetchNewEpisodes. On a transient error, sleeps
// radioTransientRetryBackoff and tries ONCE more. Returns the final result/err
// pair. Counter updates are the caller's responsibility — this helper only
// owns the retry decision so the two loops (fetch + discover) can share it
// even though they call different RadioService methods (PSY-887).
//
// Single retry, not exponential — explicit non-goal of the PSY-887 design.
// Cross-cycle backoff would require persisting transientFailures state, which
// the ticket explicitly defers ("DO NOT add persistent state").
func (s *RadioFetchService) fetchStationWithRetry(
	stationID uint,
	stationName string,
	op string, // "fetch" or "discover" — for log clarity
	call func() (any, error),
) (any, error) {
	result, err := call()
	if err == nil {
		return result, nil
	}

	if classifyError(err) != kindTransient {
		return nil, err
	}

	// Transient — one retry after brief backoff. Use a stopCh-aware sleep so
	// a service shutdown mid-backoff doesn't waste 500ms before returning.
	s.logger.Warn("transient station error, retrying after backoff",
		"station_id", stationID,
		"station_name", stationName,
		"op", op,
		"backoff", radioTransientRetryBackoff,
		"error", err,
	)
	select {
	case <-time.After(radioTransientRetryBackoff):
	case <-s.stopCh:
		return nil, err // caller will record the original transient failure
	}

	result, retryErr := call()
	if retryErr == nil {
		s.logger.Info("station recovered after transient retry",
			"station_id", stationID,
			"station_name", stationName,
			"op", op,
		)
		return result, nil
	}
	// Return the retry error so the caller's log/counter reflects the
	// post-retry state.
	return nil, retryErr
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
		totalTransient int
	)

	// Process stations sequentially to respect per-provider rate limits
	for _, station := range stations {
		// PSY-887: breaker only trips on PERMANENT failures. A station with
		// transient blips can still attempt this cycle.
		if skip, failures := s.stationBreakerSkip(station.ID); skip {
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

		// Route through the unified orchestrator (PSY-1134): the run is recorded in
		// radio_sync_runs and the returned hard error still drives the in-memory
		// PSY-887 breaker/retry below (that in-memory breaker remains authoritative
		// until P3 migrates it to radio_station_health; RunStationSync only READS
		// the persistent breaker, which stays closed in P2).
		raw, err := s.fetchStationWithRetry(station.ID, station.Name, "fetch",
			func() (any, error) {
				return s.radioService.RunStationSync(context.Background(), station.ID, RunStationSyncOpts{
					Mode:    catalogm.RadioSyncRunTypeFetch,
					Trigger: catalogm.RadioSyncRunTriggerScheduled,
				})
			},
		)
		if err != nil {
			totalFailed++
			kind := s.recordStationFailure(station.ID, err)
			if kind == kindTransient {
				totalTransient++
			}
			s.logger.Error("station fetch failed",
				"station_id", station.ID,
				"station_name", station.Name,
				"error_kind", errorKindName(kind),
				"error", err,
			)
			continue
		}

		result := raw.(*RunStationSyncResult)
		// Lock contention (another run for this station is in flight) is a benign
		// no-op — not a success or a failure for the breaker.
		if result.LockContended {
			s.logger.Info("station fetch skipped: a sync is already running",
				"station_id", station.ID, "station_name", station.Name)
			continue
		}
		s.recordStationSuccess(station.ID)

		if imp := result.Import; imp != nil {
			totalEpisodes += imp.EpisodesImported
			totalPlays += imp.PlaysImported
			totalMatched += imp.PlaysMatched

			s.logger.Info("station fetch complete",
				"station_id", station.ID,
				"station_name", station.Name,
				"run_id", result.RunID,
				"episodes_imported", imp.EpisodesImported,
				"plays_imported", imp.PlaysImported,
				"plays_matched", imp.PlaysMatched,
			)

			if len(imp.Errors) > 0 {
				s.logger.Warn("station fetch had errors",
					"station_id", station.ID,
					"station_name", station.Name,
					"error_count", len(imp.Errors),
				)
			}
		}
	}

	cycleDuration := time.Since(cycleStart)
	s.logger.Info("radio fetch cycle complete",
		"stations_processed", totalProcessed,
		"episodes_imported", totalEpisodes,
		"plays_imported", totalPlays,
		"plays_matched", totalMatched,
		"failures", totalFailed,
		// PSY-887: counts stations whose error STILL classified as transient
		// after the in-cycle retry — NOT total retries fired. A station that
		// recovered on retry is not counted (success path resets the counter).
		"transient_failures_after_retry", totalTransient,
		"duration", cycleDuration,
	)
}

// errorKindName returns a stable log key for an errorKind classification.
func errorKindName(k errorKind) string {
	if k == kindTransient {
		return "transient"
	}
	return "permanent"
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
		totalTransient   int
		stationsNotified int
	)

	for _, station := range stations {
		// PSY-887: same shared-counter policy as runFetchCycle — breaker only
		// trips on PERMANENT failures.
		if skip, failures := s.stationBreakerSkip(station.ID); skip {
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

		// Route through the unified orchestrator (PSY-1134) — see runFetchCycle.
		raw, err := s.fetchStationWithRetry(station.ID, station.Name, "discover",
			func() (any, error) {
				return s.radioService.RunStationSync(context.Background(), station.ID, RunStationSyncOpts{
					Mode:    catalogm.RadioSyncRunTypeDiscover,
					Trigger: catalogm.RadioSyncRunTriggerScheduled,
				})
			},
		)
		if err != nil {
			totalFailed++
			kind := s.recordStationFailure(station.ID, err)
			if kind == kindTransient {
				totalTransient++
			}
			s.logger.Error("station discover failed",
				"station_id", station.ID,
				"station_name", station.Name,
				"error_kind", errorKindName(kind),
				"error", err,
			)
			continue
		}

		result := raw.(*RunStationSyncResult)
		if result.LockContended {
			s.logger.Info("station discover skipped: a sync is already running",
				"station_id", station.ID, "station_name", station.Name)
			continue
		}
		s.recordStationSuccess(station.ID)

		disc := result.Discover
		if disc == nil {
			continue
		}

		totalDiscovered += disc.ShowsDiscovered
		totalNew += disc.ShowsNew

		s.logger.Info("station discover complete",
			"station_id", station.ID,
			"station_name", station.Name,
			"run_id", result.RunID,
			"shows_discovered", disc.ShowsDiscovered,
			"shows_new", disc.ShowsNew,
			"error_count", len(disc.Errors),
		)

		if disc.ShowsNew > 0 && s.discordService != nil {
			s.discordService.NotifyNewRadioShows(station.Name, disc.NewShowNames)
			stationsNotified++
		}

		// Kick off the per-station auto-backfill drain in its own goroutine so
		// the discover cycle returns immediately. Serializes jobs PER STATION
		// (one at a time) so a 56-show provider burst doesn't fan out into
		// concurrent provider hits — the existing 1-rps per-instance rate
		// limiter on each provider handles per-episode pacing.
		if s.autoBackfillDays > 0 && len(disc.NewShowIDs) > 0 {
			// GoSafe contains a panic here: this child goroutine escapes the
			// discover tick's RunTickerLoop guard, which only covers the tick
			// goroutine itself. autoBackfillStation defers s.wg.Done(), which
			// runs during unwind before GoSafe's recover, so the WaitGroup
			// stays balanced even on panic.
			//
			// NOTE (PSY-1134 / P2): auto-backfill still drives the import-job
			// machinery (CreateImportJob/StartImportJob), so its runs are traced in
			// radio_import_jobs, NOT yet radio_sync_runs. It moves onto
			// RunStationSync(backfill, auto_backfill) in PR2 when import-jobs retire.
			s.wg.Add(1)
			showIDs := disc.NewShowIDs
			showNames := disc.NewShowNames
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
		// PSY-887: same semantics as runFetchCycle — see fetch-cycle log note.
		"transient_failures_after_retry", totalTransient,
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

// GetConsecutiveFailures returns the PERMANENT failure count for a station.
// Exported for testing.
func (s *RadioFetchService) GetConsecutiveFailures(stationID uint) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.consecutiveFailures[stationID]
}

// SetConsecutiveFailures sets the PERMANENT failure count for a station.
// Exported for testing.
func (s *RadioFetchService) SetConsecutiveFailures(stationID uint, count int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.consecutiveFailures[stationID] = count
}

// GetTransientFailures returns the TRANSIENT failure count for a station
// (PSY-887). Exported for testing.
func (s *RadioFetchService) GetTransientFailures(stationID uint) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.transientFailures[stationID]
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
