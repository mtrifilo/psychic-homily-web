package catalog

import (
	"context"
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

// RunStationSync is the unified ingestion orchestrator (PSY-1134, P2 of the Radio
// Ingestion Redesign). Every ingestion path will eventually flow through here so
// each run leaves ONE durable, queryable trace in radio_sync_runs (with
// categorized radio_sync_run_errors and a radio_station_health rollup). In PR1
// ONLY the scheduled fetch/discover tickers route through it; the manual admin
// triggers and auto-backfill still use the legacy executors / import-jobs and
// move onto RunStationSync in PR2 (PSY-1135). It wraps the existing mode executors
// (DiscoverStationShows / FetchNewEpisodes / importShowEpisodesWithProgress)
// rather than re-implementing them.
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
// SCOPE (PR1 / PSY-1134): additive. The breaker is only READ here; the logic that
// OPENS it (thresholds, half-open) is P3 (PSY-1135+ milestone P3). Modes are
// discover|fetch|backfill; rematch stays on its global ticker. plays_dropped /
// plays_truncated are left 0 (P4 plumbs real per-play counters).
type RunStationSyncOpts struct {
	Mode    string // catalogm.RadioSyncRunType{Discover,Fetch,Backfill}
	Trigger string // catalogm.RadioSyncRunTrigger{Scheduled,Manual,AutoBackfill}

	// ShowID + Window* are required for mode=backfill (the historic re-ingestion
	// of one show over a date range; replaces RadioImportJob.Since/Until).
	ShowID      *uint
	WindowStart *time.Time
	WindowEnd   *time.Time
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

	// Exactly one of Import / Discover is set on a non-skipped run, per mode.
	Import   *contracts.RadioImportResult   // fetch / backfill
	Discover *contracts.RadioDiscoverResult // discover
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
	now := time.Now()
	if err := s.db.Model(&catalogm.RadioSyncRun{}).
		Where("id = ?", run.ID).
		Updates(map[string]any{
			"status":            out.status,
			"finished_at":       now,
			"episodes_found":    out.episodesFound,
			"episodes_imported": out.episodesImported,
			"plays_imported":    out.playsImported,
			"plays_matched":     out.playsMatched,
			"plays_unmatched":   out.playsUnmatched,
			"updated_at":        now,
		}).Error; err != nil {
		return &RunStationSyncResult{RunID: run.ID, Status: out.status, Import: out.importResult, Discover: out.discoverResult},
			fmt.Errorf("close sync run %d: %w", run.ID, err)
	}
	closed = true

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
// terminal ⇒ finished_at set).
func (s *RadioService) failRun(runID uint) {
	now := time.Now()
	_ = s.db.Model(&catalogm.RadioSyncRun{}).Where("id = ?", runID).Updates(map[string]any{
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
		// progressFn streams progress onto the run row (throttled) so the async
		// poll (PR2) can observe an in-flight backfill. Cancellation wiring (the
		// cancel endpoint) is PR2; this never returns cancel=true in PR1.
		var sinceLastWrite int
		progressFn := func(epImported, plImported, plMatched int, currentDate string, _ []string) bool {
			sinceLastWrite++
			if sinceLastWrite < 10 {
				return false
			}
			sinceLastWrite = 0
			s.db.Model(&catalogm.RadioSyncRun{}).Where("id = ?", runID).Updates(map[string]any{
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

// terminalStatus resolves the non-skipped, non-cancelled terminal status.
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
// P3/P4. A clean success resets the failure counter; failed/partial increment it;
// skipped touches only last_run_at.
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
	case catalogm.RadioSyncRunStatusFailed, catalogm.RadioSyncRunStatusPartial:
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
		// A real read error fails OPEN (treated as closed → run proceeds) so the
		// observability layer's own hiccup can't halt ingestion — but log it; this
		// is the line to revisit once P3 makes the persistent breaker load-bearing.
		slog.Warn("radio: breaker state read failed; treating as closed", "station_id", stationID, "error", err)
		return false
	}
	return health.BreakerState == catalogm.RadioBreakerStateOpen
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
		// Unlock on a fresh context so a cancelled run ctx still releases; closing
		// the session would release it anyway, but unlock explicitly to be clean.
		_, _ = conn.ExecContext(context.Background(), "SELECT pg_advisory_unlock($1)", key)
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
	case strings.Contains(ls, "truncat"):
		return catalogm.RadioSyncRunErrorTruncation
	case strings.Contains(ls, "missing artist") || strings.Contains(ls, "dropped"):
		return catalogm.RadioSyncRunErrorValidationDrop
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
