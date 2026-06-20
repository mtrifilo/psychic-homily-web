package catalog

import (
	"context"
	"database/sql/driver"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"strings"
	"time"
	"unicode/utf8"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/contracts"
)

// RunStationSync is the unified ingestion orchestrator (PSY-1134/PSY-1135, phase P2
// of the Radio Ingestion Redesign). Every ingestion path flows through here so
// each run leaves ONE durable, queryable trace in radio_sync_runs (with
// categorized radio_sync_run_errors and a radio_station_health rollup). As of PR2
// (PSY-1135) all four paths route through it: the scheduled fetch/discover
// tickers, the manual admin triggers (radio_sync_manual.go: station discover/fetch
// + show backfill, plus the poll/cancel surface), and the discover loop's
// auto-backfill. It wraps the existing mode executors (DiscoverStationShows /
// FetchNewEpisodes / importShowEpisodesWithProgress) rather than re-implementing
// them.
//
// Lifecycle (design doc §4): acquire a per-station advisory lock → open a
// 'running' row → check the breaker → execute the mode → record counts/errors →
// update station health → close the run to a terminal status.
//
// Return contract: a *RunStationSyncResult, ALWAYS accompanied by the executor's
// hard error (nil on success/partial). The run is recorded regardless — the trace
// is the product — but the error is also surfaced so a caller that needs the
// signal (e.g. the scheduled ticker's PSY-887 in-memory breaker + retry) keeps
// working unchanged. A nil result with a non-nil error means the run could not
// even be opened (bad opts, lock failure, station not found). Lock contention
// returns a result with LockContended=true and a nil error: another run holds the
// per-station lock, so this call is a no-op that intentionally leaves no row
// (there is no schema column to distinguish lock-skip from breaker-skip, and
// contention is benign).
//
// SCOPE: phase P2 — all four entry points route through here (PR2/PSY-1135). The
// breaker is only READ; the logic that OPENS it (thresholds, half-open) is phase
// P3 (a later ticket). Modes are discover|fetch|backfill; rematch stays on its own
// global ticker. Count plumbing not yet done: fetch leaves episodes_found=0
// (FetchNewEpisodes returns no found-count) and discover persists no count
// columns; plays_dropped / plays_truncated are left 0 — all P4.
type RunStationSyncOpts struct {
	Mode    string // catalogm.RadioSyncRunType{Discover,Fetch,Backfill}
	Trigger string // catalogm.RadioSyncRunTrigger{Scheduled,Manual,AutoBackfill}

	// ShowID + Window* are required for mode=backfill (the historic re-ingestion
	// of one show over a date range; replaces RadioImportJob.Since/Until).
	ShowID      *uint
	WindowStart *time.Time
	WindowEnd   *time.Time

	// OnRunOpened, if non-nil, is invoked synchronously with the new run id the
	// instant the radio_sync_runs row is created (after the lock + breaker checks,
	// before the mode executes). The async manual-trigger wrapper (PSY-1135) uses
	// it to learn the run id and return a 202 to the operator while the run
	// continues in the background. It fires only on the row-opened path — never on
	// lock contention or a pre-open error (those produce no row).
	OnRunOpened func(runID uint)
}

// RunStationSyncResult is what RunStationSync returns: the run-row metadata plus
// the raw executor result, so callers that need the executor's domain output
// (e.g. the discover ticker's new-show notifications + auto-backfill enqueue, or
// the fetch ticker's cycle totals) get exactly what the executors returned.
type RunStationSyncResult struct {
	RunID         uint   // 0 when LockContended
	Status        string // terminal status (or "" when contended)
	Skipped       bool   // breaker-open skip (a skipped row was written)
	LockContended bool   // another run held the per-station lock; no row written

	// On a SUCCESS or PARTIAL run, exactly one of Import/Discover is set (per mode);
	// on a FAILED run BOTH are nil — check Status (or the returned error) before
	// dereferencing either.
	Import   *contracts.RadioImportResult   // fetch / backfill (success/partial)
	Discover *contracts.RadioDiscoverResult // discover (success/partial)
}

// RunStationSync executes one ingestion run for a station. See the type doc above
// for the full contract.
func (s *RadioService) RunStationSync(ctx context.Context, stationID uint, opts RunStationSyncOpts) (*RunStationSyncResult, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	if !catalogm.IsValidRadioSyncRunType(opts.Mode) {
		return nil, fmt.Errorf("invalid sync run mode %q", opts.Mode)
	}
	if opts.Mode == catalogm.RadioSyncRunTypeRematch {
		// rematch is global, not per-station — deferred from the orchestrator (P2
		// decision); it stays on its own ticker until a per-station rematch is
		// justified (P3/P4).
		return nil, fmt.Errorf("rematch is not a per-station sync mode")
	}
	if !catalogm.IsValidRadioSyncRunTrigger(opts.Trigger) {
		return nil, fmt.Errorf("invalid sync run trigger %q", opts.Trigger)
	}
	if opts.Mode == catalogm.RadioSyncRunTypeBackfill {
		if opts.ShowID == nil || opts.WindowStart == nil || opts.WindowEnd == nil {
			return nil, fmt.Errorf("backfill requires showID + window start/end")
		}
	}

	// 1. Per-station advisory lock (single-runner). Held on a pinned connection for
	//    the whole run; contention → no-op with no row.
	release, acquired, err := s.acquireStationSyncLock(ctx, stationID)
	if err != nil {
		return nil, fmt.Errorf("acquire station sync lock: %w", err)
	}
	if !acquired {
		return &RunStationSyncResult{LockContended: true}, nil
	}
	defer release()

	// The station must exist before we can open a run (it's the run's FK target).
	var station catalogm.RadioStation
	if err := s.db.First(&station, stationID).Error; err != nil {
		return nil, fmt.Errorf("station not found: %w", err)
	}

	// 2/3. Breaker check. Scheduled + auto-backfill honor an open breaker (skip with
	//      a trace); a manual trigger BYPASSES it (operator override). The breaker
	//      SET/CLEAR logic (open after N failures, half-open trial, close on
	//      recovery) is all P3 — PR1 only READS breaker_state, and nothing here
	//      opens OR clears it, so a manual success does not yet reset an open breaker.
	if opts.Trigger != catalogm.RadioSyncRunTriggerManual && s.breakerOpen(stationID) {
		return &RunStationSyncResult{
			RunID:   s.recordSkippedRun(stationID, opts),
			Status:  catalogm.RadioSyncRunStatusSkipped,
			Skipped: true,
		}, nil
	}

	// Open the run row (status=running, started_at set explicitly — do NOT lean on
	// the DB default; GORM's skip-zero-value-with-default makes that subtle, and
	// the lifecycle CHECK requires running ⟺ finished_at NULL).
	run := catalogm.RadioSyncRun{
		StationID:   stationID,
		ShowID:      opts.ShowID,
		RunType:     opts.Mode,
		Trigger:     opts.Trigger,
		Status:      catalogm.RadioSyncRunStatusRunning,
		WindowStart: opts.WindowStart,
		WindowEnd:   opts.WindowEnd,
		StartedAt:   time.Now(),
	}
	if err := s.db.Create(&run).Error; err != nil {
		return nil, fmt.Errorf("open sync run: %w", err)
	}

	// The run row exists now — hand the id to an async caller (PSY-1135) so it can
	// return a poll handle while the rest of this call runs in the background.
	if opts.OnRunOpened != nil {
		opts.OnRunOpened(run.ID)
	}

	// A panic in the executor (provider/import code) must still terminate the run's
	// trace — otherwise the row is orphaned at status=running forever (the ticker's
	// RunTickerLoop recovers + continues, so the process survives). The lock's own
	// defer (above) still releases; `closed` guards against a double-close.
	closed := false
	defer func() {
		if r := recover(); r != nil {
			if !closed {
				s.failRun(run.ID)
			}
			panic(r)
		}
	}()

	// 4/5. Execute the mode and record its errors.
	out := s.executeSyncMode(stationID, run.ID, opts)
	s.recordRunErrors(run.ID, out.errs)

	// 6. Close the run to its terminal status (finished_at set → lifecycle CHECK)
	//    BEFORE the health rollup, so a failed close never leaves radio_station_health
	//    reporting an outcome the run row itself doesn't reflect.
	//
	//    The WHERE status='running' guard is load-bearing for cancellation (PSY-1135):
	//    a mid-run cancel sets status='cancelled' (+ finished_at) out-of-band via
	//    CancelSyncRun, and the backfill executor returns a partial result with NO
	//    error (progressFn cancel=true). Without the guard this close would overwrite
	//    'cancelled' with success/partial. RowsAffected==0 ⟺ the run is already
	//    terminal (cancelled) — leave it, and skip the health rollup (a manual cancel
	//    is not a station-health signal).
	now := time.Now()
	res := s.db.Model(&catalogm.RadioSyncRun{}).
		Where("id = ? AND status = ?", run.ID, catalogm.RadioSyncRunStatusRunning).
		Updates(map[string]any{
			"status":            out.status,
			"finished_at":       now,
			"episodes_found":    out.episodesFound,
			"episodes_imported": out.episodesImported,
			"plays_imported":    out.playsImported,
			"plays_matched":     out.playsMatched,
			"plays_unmatched":   out.playsUnmatched,
			"updated_at":        now,
		})
	if res.Error != nil {
		return &RunStationSyncResult{RunID: run.ID, Status: out.status, Import: out.importResult, Discover: out.discoverResult},
			fmt.Errorf("close sync run %d: %w", run.ID, res.Error)
	}
	closed = true

	if res.RowsAffected == 0 {
		// Cancelled mid-run: the row is already terminal (cancelled, finished_at set
		// by CancelSyncRun). Report the cancelled status and do NOT roll up health.
		return &RunStationSyncResult{
			RunID:    run.ID,
			Status:   catalogm.RadioSyncRunStatusCancelled,
			Import:   out.importResult,
			Discover: out.discoverResult,
		}, out.hardErr
	}

	// 7. Roll up station health (last_run/last_success, consecutive_failures).
	s.updateStationHealth(stationID, out.status)

	// Surface the executor's hard error (nil on success/partial) so callers that
	// need it keep their signal; the run is recorded regardless.
	return &RunStationSyncResult{
		RunID:    run.ID,
		Status:   out.status,
		Import:   out.importResult,
		Discover: out.discoverResult,
	}, out.hardErr
}

// failRun best-effort closes an orphaned run as failed — used by the panic
// recovery path so a panicked run still terminates its trace (lifecycle CHECK:
// terminal ⇒ finished_at set). The WHERE status='running' guard is symmetric with
// the close path: a run that was cancelled out-of-band (CancelSyncRun) before the
// panic is already terminal, and must NOT be rewritten to 'failed' — the operator's
// cancel wins.
func (s *RadioService) failRun(runID uint) {
	now := time.Now()
	_ = s.db.Model(&catalogm.RadioSyncRun{}).
		Where("id = ? AND status = ?", runID, catalogm.RadioSyncRunStatusRunning).
		Updates(map[string]any{
			"status":      catalogm.RadioSyncRunStatusFailed,
			"finished_at": now,
			"updated_at":  now,
		}).Error
}

// syncOutcome is the unified result of executing one mode, mapped onto the
// radio_sync_runs columns + the categorized error list.
type syncOutcome struct {
	status           string
	hardErr          error // the executor's setup/provider error (nil on success/partial)
	episodesFound    int
	episodesImported int
	playsImported    int
	playsMatched     int
	playsUnmatched   int
	errs             []runError

	// Raw executor results, surfaced to the caller via RunStationSyncResult.
	importResult   *contracts.RadioImportResult
	discoverResult *contracts.RadioDiscoverResult
}

// runError is one categorized error destined for radio_sync_run_errors.
type runError struct {
	category   string
	detail     string
	episodeRef *string
}

// executeSyncMode dispatches to the existing executor for the mode and maps its
// result into a syncOutcome. The executors are called directly (no cycle: they do
// not call RunStationSync).
func (s *RadioService) executeSyncMode(stationID, runID uint, opts RunStationSyncOpts) syncOutcome {
	switch opts.Mode {
	case catalogm.RadioSyncRunTypeDiscover:
		res, err := s.DiscoverStationShows(stationID)
		if err != nil {
			return syncOutcome{status: catalogm.RadioSyncRunStatusFailed, hardErr: err, errs: topLevelErr(err)}
		}
		return syncOutcome{
			status:         terminalStatus(false, len(res.Errors)),
			errs:           stringErrs(res.Errors),
			discoverResult: res,
		}

	case catalogm.RadioSyncRunTypeFetch:
		res, err := s.FetchNewEpisodes(stationID)
		if err != nil {
			return syncOutcome{status: catalogm.RadioSyncRunStatusFailed, hardErr: err, errs: topLevelErr(err)}
		}
		return importResultOutcome(res, 0)

	case catalogm.RadioSyncRunTypeBackfill:
		since := opts.WindowStart.Format("2006-01-02")
		until := opts.WindowEnd.Format("2006-01-02")
		var found int
		// progressFn streams progress onto the run row (throttled) so the async poll
		// can observe an in-flight backfill, and honors a mid-run cancel (PSY-1135):
		// when CancelSyncRun has flipped this run to 'cancelled', it returns true so
		// the importer stops early. The cancel check runs every episode (a cheap
		// single-column SELECT); the progress write stays throttled to every 10.
		var sinceLastWrite int
		progressFn := func(epImported, plImported, plMatched int, currentDate string, _ []string) bool {
			if s.isSyncRunCancelled(runID) {
				return true
			}
			sinceLastWrite++
			if sinceLastWrite < 10 {
				return false
			}
			sinceLastWrite = 0
			// status='running' guard: if a cancel landed between the cancel-poll
			// above and this write (the poll's best-effort read can miss it), don't
			// rewrite counters/updated_at onto an already-terminal (cancelled) row.
			s.db.Model(&catalogm.RadioSyncRun{}).
				Where("id = ? AND status = ?", runID, catalogm.RadioSyncRunStatusRunning).
				Updates(map[string]any{
					"episodes_imported":    epImported,
					"plays_imported":       plImported,
					"plays_matched":        plMatched,
					"current_episode_date": currentDate,
					"updated_at":           time.Now(),
				})
			return false
		}
		res, err := s.importShowEpisodesWithProgress(*opts.ShowID, since, until, func(n int) { found = n }, progressFn)
		if err != nil {
			return syncOutcome{status: catalogm.RadioSyncRunStatusFailed, hardErr: err, errs: topLevelErr(err)}
		}
		return importResultOutcome(res, found)

	default:
		return syncOutcome{status: catalogm.RadioSyncRunStatusFailed,
			errs: []runError{{category: catalogm.RadioSyncRunErrorParseError, detail: "unsupported sync mode " + opts.Mode}}}
	}
}

// importResultOutcome maps a RadioImportResult (fetch/backfill) onto a syncOutcome.
// plays_unmatched is derived (imported − matched); plays_dropped/truncated stay 0
// (P4). A run with any per-episode error, fetch error, or match-persist error is
// 'partial', else 'success'.
func importResultOutcome(res *contracts.RadioImportResult, episodesFound int) syncOutcome {
	unmatched := res.PlaysImported - res.PlaysMatched
	if unmatched < 0 {
		unmatched = 0
	}
	// res.Errors is the superset: EpisodeFetchErrors/MatchPersistErrors are each
	// also appended to Errors (contract), so counting len(Errors) alone avoids
	// double-counting while still flipping the status to partial when any occurred.
	errCount := len(res.Errors)
	return syncOutcome{
		status:           terminalStatus(false, errCount),
		episodesFound:    episodesFound,
		episodesImported: res.EpisodesImported,
		playsImported:    res.PlaysImported,
		playsMatched:     res.PlaysMatched,
		playsUnmatched:   unmatched,
		errs:             stringErrs(res.Errors),
		importResult:     res,
	}
}

// terminalStatus resolves the non-skipped, non-cancelled terminal status. The
// hardErr arm is exercised only by unit tests: in production executeSyncMode
// short-circuits a hard executor error to the failed literal upstream, so every
// production call passes hardErr=false (success-vs-partial on errCount alone).
func terminalStatus(hardErr bool, errCount int) string {
	switch {
	case hardErr:
		return catalogm.RadioSyncRunStatusFailed
	case errCount > 0:
		return catalogm.RadioSyncRunStatusPartial
	default:
		return catalogm.RadioSyncRunStatusSuccess
	}
}

// recordSkippedRun writes a single terminal skipped row for a breaker-open skip.
// breaker_skipped=true requires status=skipped (the breaker_skipped_check CHECK);
// both started_at and finished_at are set (lifecycle CHECK: terminal ⇒ finished).
func (s *RadioService) recordSkippedRun(stationID uint, opts RunStationSyncOpts) uint {
	now := time.Now()
	run := catalogm.RadioSyncRun{
		StationID:      stationID,
		ShowID:         opts.ShowID,
		RunType:        opts.Mode,
		Trigger:        opts.Trigger,
		Status:         catalogm.RadioSyncRunStatusSkipped,
		BreakerSkipped: true,
		WindowStart:    opts.WindowStart,
		WindowEnd:      opts.WindowEnd,
		StartedAt:      now,
		FinishedAt:     &now,
	}
	if err := s.db.Create(&run).Error; err != nil {
		slog.Warn("radio: failed to record skipped run", "station_id", stationID, "error", err)
		return 0
	}
	s.updateStationHealth(stationID, catalogm.RadioSyncRunStatusSkipped)
	return run.ID
}

// recordRunErrors inserts the categorized errors for a run. detail is truncated so
// a flapping provider can't bloat the admin-readable table.
func (s *RadioService) recordRunErrors(runID uint, errs []runError) {
	if len(errs) == 0 {
		return
	}
	rows := make([]catalogm.RadioSyncRunError, 0, len(errs))
	for _, e := range errs {
		detail := truncateForDetail(e.detail)
		rows = append(rows, catalogm.RadioSyncRunError{
			SyncRunID:  runID,
			Category:   e.category,
			Detail:     &detail,
			EpisodeRef: e.episodeRef,
		})
	}
	// Best-effort: a failure to persist the error log must not mask the run's
	// own terminal status.
	_ = s.db.Create(&rows).Error
}

// updateStationHealth upserts the per-station health rollup by station_id (ON
// CONFLICT column-set inference). PR1 maintains last_run/last_success +
// consecutive_failures only; the breaker-open decision + rate computations are
// P3/P4. consecutive_failures matches the in-memory PSY-887 breaker's posture so
// the two don't diverge: only a 'failed' run (station/provider unreachable)
// increments it; 'success' AND 'partial' both reset it — a partial imported data
// (just some per-episode noise) and is NOT a breaker failure, else a chronically-
// noisy-but-healthy station would climb the counter forever and trip the P3
// breaker. last_success_at is set ONLY on a clean success; 'skipped' touches only
// last_run_at.
func (s *RadioService) updateStationHealth(stationID uint, status string) {
	now := time.Now()
	assignments := map[string]any{
		"last_run_at": now,
		"updated_at":  now,
	}
	health := catalogm.RadioStationHealth{
		StationID:    stationID,
		LastRunAt:    &now,
		BreakerState: catalogm.RadioBreakerStateClosed,
	}
	switch status {
	case catalogm.RadioSyncRunStatusSuccess:
		health.LastSuccessAt = &now
		assignments["last_success_at"] = now
		assignments["consecutive_failures"] = 0
	case catalogm.RadioSyncRunStatusPartial:
		assignments["consecutive_failures"] = 0 // imported data; not a breaker failure
	case catalogm.RadioSyncRunStatusFailed:
		// INSERT path (first-ever run for this station) starts the counter at 1;
		// the ON CONFLICT path increments the stored value.
		health.ConsecutiveFailures = 1
		assignments["consecutive_failures"] = gorm.Expr("radio_station_health.consecutive_failures + 1")
	}
	_ = s.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "station_id"}},
		DoUpdates: clause.Assignments(assignments),
	}).Create(&health).Error
}

// breakerOpen reports whether the persisted breaker for a station is open. A
// missing health row means "never synced" → treated as closed (not open), so a
// brand-new station is never born tripped.
func (s *RadioService) breakerOpen(stationID uint) bool {
	var health catalogm.RadioStationHealth
	err := s.db.Select("breaker_state").First(&health, "station_id = ?", stationID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false // never synced → breaker closed (never born tripped)
	}
	if err != nil {
		// A real read error fails PERMISSIVE (treated as breaker-closed → run
		// proceeds; note "open" is the BLOCK state in this domain, so don't call
		// this "fail-open") so the observability layer's own hiccup can't halt
		// ingestion — but log it; revisit once P3 makes the breaker load-bearing.
		slog.Warn("radio: breaker state read failed; treating as closed", "station_id", stationID, "error", err)
		return false
	}
	return health.BreakerState == catalogm.RadioBreakerStateOpen
}

// isSyncRunCancelled reports whether the run has been flipped to 'cancelled'
// out-of-band (CancelSyncRun). Polled by the backfill executor's progressFn so a
// long historic re-ingestion can be stopped mid-flight (PSY-1135). A read error
// is treated as not-cancelled (best-effort) so an observability hiccup never
// aborts an otherwise-healthy import.
func (s *RadioService) isSyncRunCancelled(runID uint) bool {
	var run catalogm.RadioSyncRun
	if err := s.db.Select("status").First(&run, runID).Error; err != nil {
		return false
	}
	return run.Status == catalogm.RadioSyncRunStatusCancelled
}

// acquireStationSyncLock pins a connection and takes a non-blocking per-station
// advisory lock. pg_try_advisory_lock returns immediately (false if held), giving
// the "skip if another run holds it" semantic. The lock is SESSION-scoped (not
// xact-scoped like show.go's pg_advisory_xact_lock) because a sync run spans many
// transactions; it is held on the pinned conn until release() unlocks + returns
// the conn to the pool. release is nil when acquired is false.
func (s *RadioService) acquireStationSyncLock(ctx context.Context, stationID uint) (release func(), acquired bool, err error) {
	sqlDB, err := s.db.DB()
	if err != nil {
		return nil, false, fmt.Errorf("resolve sql.DB: %w", err)
	}
	conn, err := sqlDB.Conn(ctx)
	if err != nil {
		return nil, false, fmt.Errorf("pin connection: %w", err)
	}
	key := fnvHash(fmt.Sprintf("radio_sync:station:%d", stationID))
	var ok bool
	if err := conn.QueryRowContext(ctx, "SELECT pg_try_advisory_lock($1)", key).Scan(&ok); err != nil {
		_ = conn.Close()
		return nil, false, fmt.Errorf("pg_try_advisory_lock: %w", err)
	}
	if !ok {
		_ = conn.Close()
		return nil, false, nil
	}
	release = func() {
		// Unlock on a fresh context so a cancelled run ctx still releases. IMPORTANT:
		// conn.Close() returns the physical connection to the POOL without closing the
		// Postgres session, and a session-scoped advisory lock is released ONLY by
		// pg_advisory_unlock — NOT by Close. So if the unlock fails, the lock would
		// stay held on a pooled conn and silently no-op (LockContended) every future
		// run for this station. Discard the connection in that case so it can never
		// re-enter the pool still holding the lock.
		if _, unlockErr := conn.ExecContext(context.Background(), "SELECT pg_advisory_unlock($1)", key); unlockErr != nil {
			slog.Warn("radio: advisory unlock failed; discarding connection to avoid a leaked lock",
				"station_id", stationID, "error", unlockErr)
			_ = conn.Raw(func(any) error { return driver.ErrBadConn })
			return
		}
		_ = conn.Close()
	}
	return release, true, nil
}

// ───────────────────────────── error categorization ─────────────────────────────

// topLevelErr categorizes a single mode-level (hard) error into one run-error row.
func topLevelErr(err error) []runError {
	return []runError{{category: categorizeRunError(err), detail: err.Error()}}
}

// stringErrs maps the executors' free-text per-episode error strings into
// categorized run-error rows. Fine-grained categorization from typed errors
// threaded out of importEpisode/importPlays is a follow-up (the executors collapse
// errors to strings today); the heuristic below covers the common shapes.
func stringErrs(msgs []string) []runError {
	if len(msgs) == 0 {
		return nil
	}
	out := make([]runError, 0, len(msgs))
	for _, m := range msgs {
		out = append(out, runError{category: categorizeErrorString(m), detail: m})
	}
	return out
}

// categorizeRunError maps a typed error to a radio_sync_run_errors category.
func categorizeRunError(err error) string {
	if err == nil {
		return catalogm.RadioSyncRunErrorProviderUnreachable
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return catalogm.RadioSyncRunErrorTimeout
	}
	var httpErr *RadioHTTPError
	if errors.As(err, &httpErr) {
		if errors.Is(httpErr, ErrTransient) { // 429
			return catalogm.RadioSyncRunErrorRateLimited
		}
		return catalogm.RadioSyncRunErrorProviderUnreachable
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return catalogm.RadioSyncRunErrorTimeout
	}
	return catalogm.RadioSyncRunErrorProviderUnreachable
}

// categorizeErrorString is the best-effort heuristic for already-stringified
// per-episode errors.
func categorizeErrorString(s string) string {
	ls := strings.ToLower(s)
	switch {
	case strings.Contains(ls, "timeout") || strings.Contains(ls, "deadline"):
		return catalogm.RadioSyncRunErrorTimeout
	case strings.Contains(ls, "429") || strings.Contains(ls, "rate limit") || strings.Contains(ls, "too many"):
		return catalogm.RadioSyncRunErrorRateLimited
	case strings.Contains(ls, "parse") || strings.Contains(ls, "unmarshal") || strings.Contains(ls, "decode") || strings.Contains(ls, "invalid character"):
		return catalogm.RadioSyncRunErrorParseError
	// validation_drop before truncation: every provider drop-summary starts
	// "dropped N plays: ..." (summarizeDrops), so a summary that also mentions
	// truncated titles correctly buckets as a drop. NOTE: this makes 'truncation'
	// effectively unreachable from today's stringified drop-summaries — precise
	// truncation categorization needs structured (typed) errors threaded out of
	// importPlays, which is P3/P4. The 'truncat' arm stays for any future
	// non-summary error string that mentions truncation without "dropped".
	case strings.Contains(ls, "missing artist") || strings.Contains(ls, "dropped"):
		return catalogm.RadioSyncRunErrorValidationDrop
	case strings.Contains(ls, "truncat"):
		return catalogm.RadioSyncRunErrorTruncation
	case strings.Contains(ls, "match") || strings.Contains(ls, "persist"):
		return catalogm.RadioSyncRunErrorMatchPersistError
	default:
		return catalogm.RadioSyncRunErrorProviderUnreachable
	}
}

// runErrorDetailLimit caps a radio_sync_run_errors.detail so a flapping provider
// can't bloat the admin-readable table (migration note on the column).
const runErrorDetailLimit = 2000

func truncateForDetail(s string) string {
	const marker = "…[truncated]"
	if utf8.RuneCountInString(s) <= runErrorDetailLimit {
		return s
	}
	// Rune-safe (reuse truncateRunes): a byte-slice could split a multi-byte
	// sequence mid-rune → invalid UTF-8 → Postgres rejects the TEXT insert, and
	// recordRunErrors swallows that error, silently losing the whole run's error
	// batch — exactly the garbled-provider case this table exists to capture.
	return truncateRunes(s, runErrorDetailLimit) + marker
}
