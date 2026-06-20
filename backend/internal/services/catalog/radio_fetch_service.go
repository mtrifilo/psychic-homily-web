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

// radioTransientRetryBackoff is the brief delay before retrying a station once
// after a transient error. Single retry per station per cycle; we don't carry
// transient-retry state across cycles (no exponential backoff) — that's an
// explicit non-goal of PSY-887 to keep the fetch loop's failure modes obvious.
const radioTransientRetryBackoff = 500 * time.Millisecond

// errorKind classifies a radio provider error for retry + breaker routing.
// kindTransient → retry once (fetchStationWithRetry); never trips the breaker.
// kindPermanent → no retry; increments the persistent breaker counter and trips
// it at radioCircuitBreakerThreshold (see breakerTransition in radio_sync.go).
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
	// 0 disables auto-backfill (admins can still manually trigger a backfill via
	// POST /admin/radio-shows/{id}/backfill).
	autoBackfillDays int

	stopCh chan struct{}
	wg     sync.WaitGroup
	logger *slog.Logger

	// The circuit breaker is no longer in-memory (PSY-1140): it lives in
	// radio_station_health.breaker_state and is owned end-to-end by RunStationSync
	// (read at the gate, written on the run's outcome via updateStationHealth), so
	// it survives restarts. The loops below just consult RunStationSyncResult.Skipped
	// for a breaker-open station and keep the transient-error single-retry
	// (fetchStationWithRetry); the retry budget rework is PSY-1142.
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
		radioService:     radioService,
		discordService:   discordService,
		fetchInterval:    fetchInterval,
		affinityInterval: affinityInterval,
		rematchInterval:  rematchInterval,
		discoverInterval: discoverInterval,
		autoBackfillDays: autoBackfillDays,
		stopCh:           make(chan struct{}),
		logger:           slog.Default(),
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

// fetchStationWithRetry calls the run op. On a transient error, sleeps
// radioTransientRetryBackoff and tries ONCE more. Returns the final result/err
// pair. The breaker counter is the caller's responsibility (now persisted by
// RunStationSync's updateStationHealth); this helper only owns the retry decision
// so the two loops (fetch + discover) can share it even though they call different
// RadioService methods (PSY-887).
//
// Single fixed-delay retry, not exponential — kept as-is in PSY-1140; the Full
// Jitter backoff + two-tier retry budget rework is PSY-1142.
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
		totalSkipped   int // breaker-open or lock-contended no-ops (no fetch happened)
	)

	// Process stations sequentially to respect per-provider rate limits. The
	// breaker is no longer pre-checked here (PSY-1140): RunStationSync consults the
	// persistent breaker itself and returns Skipped for an open station (writing a
	// skipped run row — better observability than the old silent in-memory skip).
	for _, station := range stations {
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
			// The persistent breaker counter is owned by RunStationSync's
			// updateStationHealth; here we classify only to label the log + tally
			// post-retry transients (the count drives no breaker decision now).
			kind := classifyError(err)
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
		// A no-op run — the per-station lock was held by another in-flight sync, or
		// the persistent breaker is open (a skipped run row was written) — produced
		// no Import payload, so skip the result handling below. The persistent
		// breaker counter is reset by RunStationSync's own success path, not here.
		if result.LockContended || result.Skipped {
			totalSkipped++
			reason := "breaker_open"
			if result.LockContended {
				reason = "sync_already_running"
			}
			s.logger.Info("station fetch skipped",
				"station_id", station.ID, "station_name", station.Name, "reason", reason)
			continue
		}

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
		// PSY-1140: stations the breaker skipped (or that another run held the lock
		// for) — counted in stations_processed but did NOT fetch. Each also wrote a
		// skipped run row.
		"stations_skipped", totalSkipped,
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
		totalSkipped     int // breaker-open or lock-contended no-ops (no discover happened)
		stationsNotified int
	)

	for _, station := range stations {
		// Breaker pre-check removed (PSY-1140) — RunStationSync handles the
		// persistent breaker and returns Skipped for an open station, same as
		// runFetchCycle.
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
			kind := classifyError(err) // log label + transient tally only (see runFetchCycle)
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
		// See runFetchCycle: a no-op (lock contended or breaker open) carries no
		// Discover payload, so skip the result handling below.
		if result.LockContended || result.Skipped {
			totalSkipped++
			reason := "breaker_open"
			if result.LockContended {
				reason = "sync_already_running"
			}
			s.logger.Info("station discover skipped",
				"station_id", station.ID, "station_name", station.Name, "reason", reason)
			continue
		}

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
			// PSY-1135: auto-backfill now runs through RunStationSync(backfill,
			// auto_backfill), so each newly-discovered show's history is traced in
			// radio_sync_runs alongside every other ingestion path.
			s.wg.Add(1)
			stationID := station.ID
			showIDs := disc.NewShowIDs
			showNames := disc.NewShowNames
			stationName := station.Name
			shared.GoSafe(context.Background(), "radio_auto_backfill", func() {
				s.autoBackfillStation(stationID, stationName, showIDs, showNames)
			})
		}
	}

	s.logger.Info("radio discover cycle complete",
		"stations_processed", totalProcessed,
		"stations_skipped", totalSkipped, // breaker/lock no-ops, counted in processed (PSY-1140)
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
// SERIALLY — one RunStationSync(backfill) at a time per station, blocking on each
// before starting the next. Single batched Discord notification at the end.
//
// Why serial: provider HTTP clients are rate-limited at 1 req/sec PER INSTANCE
// (existing wfmuRateLimit / kexpRateLimit / ntsRateLimit). Each backfill run gets
// its own provider instance via getProvider, so parallel runs would each have an
// independent rate limiter and could collectively hit a provider faster than the
// per-instance cap intends. Per-station serialization keeps effective egress at
// ~1 req/sec/provider. The per-station advisory lock inside RunStationSync is the
// other guard: if a scheduled fetch/discover for this station is mid-flight, the
// backfill no-ops (lock contended) and is retried on the next discover cycle.
//
// Process lifetime: runAutoBackfillShow cancels the in-flight run on s.stopCh, so
// a service Stop() unwinds within ~one episode instead of blocking on a full
// historic import; the loop also checks s.stopCh between shows.
func (s *RadioFetchService) autoBackfillStation(stationID uint, stationName string, showIDs []uint, showNames []string) {
	defer s.wg.Done()

	until := time.Now()
	since := until.AddDate(0, 0, -s.autoBackfillDays)

	s.logger.Info("auto_backfill_started",
		"station", stationName,
		"shows", len(showIDs),
		"since", since.Format("2006-01-02"),
		"until", until.Format("2006-01-02"),
	)

	var (
		completedShows []string
		totalEpisodes  int
		totalPlays     int
	)

	for i, showID := range showIDs {
		// Stop cleanly between shows on shutdown.
		select {
		case <-s.stopCh:
			s.logger.Info("auto_backfill_abandoned_on_shutdown", "station", stationName)
			return
		default:
		}

		showName := showNames[i]

		res := s.runAutoBackfillShow(stationID, showID, since, until)
		if res == nil {
			continue // pre-open failure, already logged
		}
		if res.LockContended || res.Skipped {
			reason := "breaker_open"
			if res.LockContended {
				reason = "sync_already_running"
			}
			s.logger.Info("auto_backfill_show_skipped", "show_id", showID, "show", showName, "reason", reason)
			continue
		}
		if res.Status == catalogm.RadioSyncRunStatusCancelled {
			// Cancelled by the shutdown watcher — stop draining the batch.
			s.logger.Info("auto_backfill_abandoned_on_shutdown", "station", stationName)
			return
		}

		// Import is non-nil on success AND partial (partial imported data with some
		// per-episode noise); nil on a failed run.
		if imp := res.Import; imp != nil {
			completedShows = append(completedShows, showName)
			totalEpisodes += imp.EpisodesImported
			totalPlays += imp.PlaysMatched
		} else {
			s.logger.Warn("auto_backfill_show_did_not_complete",
				"show_id", showID, "show", showName, "status", res.Status)
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

// runAutoBackfillShow runs one show's auto-backfill through RunStationSync and
// cancels it on service shutdown so Stop() never blocks on a long historic
// import. A watcher goroutine learns the run id via OnRunOpened and, if s.stopCh
// closes before the run finishes, flips the run to cancelled — the backfill
// executor's progressFn observes the cancel and returns within one episode.
// Returns the run result, or nil on a pre-open failure (logged).
func (s *RadioFetchService) runAutoBackfillShow(stationID, showID uint, since, until time.Time) *RunStationSyncResult {
	runIDCh := make(chan uint, 1)
	done := make(chan struct{})
	watcherExited := make(chan struct{})
	shared.GoSafe(context.Background(), "radio_auto_backfill_cancel", func() {
		defer close(watcherExited)
		select {
		case runID := <-runIDCh:
			select {
			case <-s.stopCh:
				_ = s.radioService.CancelSyncRun(runID)
			case <-done:
			}
		case <-done:
		}
	})

	res, err := s.radioService.RunStationSync(context.Background(), stationID, RunStationSyncOpts{
		Mode:        catalogm.RadioSyncRunTypeBackfill,
		Trigger:     catalogm.RadioSyncRunTriggerAutoBackfill,
		ShowID:      &showID,
		WindowStart: &since,
		WindowEnd:   &until,
		OnRunOpened: func(id uint) { runIDCh <- id },
	})
	close(done)
	// Join the watcher: it may still be issuing CancelSyncRun on the shutdown
	// path, so don't return (and let autoBackfillStation's wg.Done fire) until the
	// watcher has fully exited — no goroutine doing DB work outlives the WaitGroup
	// barrier in Stop().
	<-watcherExited

	if res == nil {
		s.logger.Warn("auto_backfill_open_failed", "show_id", showID, "error", err)
		return nil
	}
	if err != nil && res.Status == catalogm.RadioSyncRunStatusFailed {
		s.logger.Warn("auto_backfill_show_failed", "show_id", showID, "error", err)
	}
	return res
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
