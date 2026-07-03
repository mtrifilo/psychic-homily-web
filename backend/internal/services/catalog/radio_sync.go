package catalog

import (
	"context"
	"database/sql/driver"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/getsentry/sentry-go"
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
	// For mode=fetch, ShowID optionally SCOPES the incremental fetch to that one
	// show — the PSY-1333 slot-fetch path (targeted fetch on a schedule-slot
	// boundary). Scoped fetch runs carry show_id on their run row and are
	// excluded from the volume-anomaly guard + its baseline (single-show volumes
	// are not station-scale).
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

	// 2/3. Breaker gate. Scheduled + auto-backfill honor the persistent breaker; a
	//      manual trigger BYPASSES it (operator override — the manual run is itself a
	//      deliberate half-open probe, see updateStationHealth's trigger handling).
	//      An open breaker that is past its cooldown promotes to half_open for a
	//      single trial run; the breaker write-back (open / half_open / close) happens
	//      in updateStationHealth on the run's outcome (PSY-1140).
	if opts.Trigger != catalogm.RadioSyncRunTriggerManual {
		switch breakerGateFor(s.readBreakerSnapshot(stationID), time.Now()) {
		case gateBlocked:
			return &RunStationSyncResult{
				RunID:   s.recordSkippedRun(stationID, opts),
				Status:  catalogm.RadioSyncRunStatusSkipped,
				Skipped: true,
			}, nil
		case gateTrial:
			// Open past cooldown → this run is the half-open trial. Mark it so the
			// state is observable; the outcome closes (success) or re-opens (failure).
			s.markBreakerHalfOpen(stationID)
		case gateAllow:
			// closed (or never-synced) → run normally.
		}
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

	// PSY-1156 volume-anomaly guard: a FETCH that imported far fewer plays than the
	// station's trailing baseline is flagged (partial + empty_unexpected) rather than
	// passing as a silent success (the PSY-1126 KEXP-0-vs-~50 failure). Fetch only —
	// the steady-state cadence — and only on a non-failed run: a hard executor failure
	// is already recorded, and discover/backfill volumes are too variable for a
	// baseline. Observational: it does not page Sentry (empty_unexpected is not in
	// escalationError's escalate set) and only downgrades success → partial, never a failure.
	// Show-SCOPED fetches (PSY-1333 slot fetch) are exempt: a single show's volume is
	// nowhere near the station-sweep baseline, so every scoped run would false-flag.
	if opts.Mode == catalogm.RadioSyncRunTypeFetch && opts.ShowID == nil && out.hardErr == nil {
		if anomaly, detail := s.detectVolumeAnomaly(stationID, run.ID, out.playsImported); anomaly {
			out.errs = append(out.errs, runError{category: catalogm.RadioSyncRunErrorEmptyUnexpected, detail: detail})
			if out.status == catalogm.RadioSyncRunStatusSuccess {
				out.status = catalogm.RadioSyncRunStatusPartial
			}
		}
	}

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

	// 7. Roll up station health (last_run/last_success, breaker state). The error
	//    kind drives the breaker: only a PERMANENT failure increments the counter /
	//    trips it (transient errors retry, never trip — PSY-887). classifyError(nil)
	//    is harmless on success/partial (breakerTransition ignores errKind there).
	s.updateStationHealth(stationID, out.status, opts.Trigger, classifyError(out.hardErr))

	// Escalate a PERMANENT scheduled/auto failure to Sentry (the scraper-drift /
	// format-change signal). Manual failures stay log-only (the operator already sees
	// the result); transient failures retry and stay log-only (PSY-1141).
	if category, escErr := escalationError(out, opts.Trigger); escErr != nil {
		s.escalatePermanentFailure(escErr, stationID, category)
	}

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
		// PSY-1153 create-on-first-episode: materialize the newly-discovered roster
		// shows that aired in the window — create the row + import its episodes — HERE,
		// inside this discover run (under its per-station lock + breaker gate). Both the
		// scheduled discover cycle and the manual admin "discover" trigger flow through
		// here, so both create aired shows; an episode-less roster DJ never becomes a row.
		// NOTE: the per-station advisory lock (RunStationSync) is now held across this
		// whole import, so on a greenfield first run it can span the full sequential
		// roster (paced by the per-provider limiter, shutdown-cancellable). This is safe
		// only because the lock is pg_try_advisory_lock (non-blocking — contenders skip,
		// they don't queue); do NOT switch to a blocking acquire here or elsewhere.
		since, until := discoverCreateWindow(opts)
		imp, createdNames := s.createOnFirstForRoster(stationID, runID, res.NewRosterShows, since, until)
		res.CreatedShowNames = createdNames
		return discoverOutcome(res, imp)

	case catalogm.RadioSyncRunTypeFetch:
		res, err := s.fetchNewEpisodes(stationID, opts.Trigger, opts.ShowID)
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
	// CategorizedErrors carries the category decided AT THE SOURCE (PSY-1141), so the
	// import path records the real radio_sync_run_errors category instead of the
	// substring heuristic — it is parallel to res.Errors (same order, same length).
	// The fetch/match counters are each also recorded as a categorized error, so the
	// length flips the status to 'partial' when any occurred without double-counting.
	errs := categorizedToRunErrors(res.CategorizedErrors)
	return syncOutcome{
		status:           terminalStatus(false, len(errs)),
		episodesFound:    episodesFound,
		episodesImported: res.EpisodesImported,
		playsImported:    res.PlaysImported,
		playsMatched:     res.PlaysMatched,
		playsUnmatched:   unmatched,
		errs:             errs,
		importResult:     res,
	}
}

// defaultDiscoverCreateLookbackDays bounds create-on-first when the discover run
// carries no explicit window (e.g. the manual admin trigger). A roster show with an
// episode within this many days is created; older-only shows stay invisible (§9 dec 1).
// The scheduled discover cycle overrides this with RADIO_AUTO_BACKFILL_DAYS so history
// depth stays operator-tunable. 90 matches the prior auto-backfill default.
const defaultDiscoverCreateLookbackDays = 90

// discoverCreateWindow resolves the [since, until] create-on-first window for a discover
// run from opts, defaulting to the last defaultDiscoverCreateLookbackDays.
func discoverCreateWindow(opts RunStationSyncOpts) (since, until time.Time) {
	until = time.Now()
	if opts.WindowEnd != nil {
		until = *opts.WindowEnd
	}
	since = until.AddDate(0, 0, -defaultDiscoverCreateLookbackDays)
	if opts.WindowStart != nil {
		since = *opts.WindowStart
	}
	return since, until
}

// discoverOutcome merges a discover result with the create-on-first import result into
// a single syncOutcome (PSY-1153). The discover run's counts now reflect the episodes
// imported while materializing new shows; its errors combine discovery errors with
// per-show/per-episode import errors, so any import noise flips the run to 'partial'.
func discoverOutcome(disc *contracts.RadioDiscoverResult, imp *contracts.RadioImportResult) syncOutcome {
	unmatched := imp.PlaysImported - imp.PlaysMatched
	if unmatched < 0 {
		unmatched = 0
	}
	errs := append(stringErrs(disc.Errors), categorizedToRunErrors(imp.CategorizedErrors)...)
	return syncOutcome{
		status:           terminalStatus(false, len(errs)),
		episodesImported: imp.EpisodesImported,
		playsImported:    imp.PlaysImported,
		playsMatched:     imp.PlaysMatched,
		playsUnmatched:   unmatched,
		errs:             errs,
		discoverResult:   disc,
		importResult:     imp,
	}
}

// categorizedToRunErrors maps the structured import errors (category decided at the
// source) straight onto run-error rows — no re-categorization. PSY-1141.
func categorizedToRunErrors(cats []contracts.RadioRunError) []runError {
	if len(cats) == 0 {
		return nil
	}
	out := make([]runError, 0, len(cats))
	for _, c := range cats {
		out = append(out, runError{category: c.Category, detail: c.Detail, episodeRef: c.EpisodeRef})
	}
	return out
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
	// A skipped run touches only last_run_at; breakerTransition's default arm leaves
	// the breaker state + counter unchanged for 'skipped', so trigger/errKind are
	// inert here (kindPermanent is a placeholder — if you ever make the skipped/
	// default arm read errKind, revisit this call site).
	s.updateStationHealth(stationID, catalogm.RadioSyncRunStatusSkipped, opts.Trigger, kindPermanent)
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

// ───────────────────────────── persistent circuit breaker ─────────────────────────────
//
// PSY-1140 migrates the radio breaker from the in-memory PSY-887 map (which reset
// on every deploy) onto radio_station_health.breaker_state, so a tripped station
// stays tripped across restarts. The state machine — closed → open (at threshold)
// → half_open (after cooldown) → closed (successful trial) / → open (failed trial)
// — is split into PURE decision functions (breakerGateFor, breakerTransition; no
// I/O, exhaustively unit-tested) and thin DB wiring (readBreakerSnapshot,
// markBreakerHalfOpen, updateStationHealth). Every write path runs under the
// per-station advisory lock RunStationSync holds, so updateStationHealth's
// read-modify-write is race-free per station.

// radioCircuitBreakerThreshold is the number of consecutive PERMANENT failures
// before the persistent breaker opens and the station is skipped on scheduled/auto
// cycles. Transient errors (timeout, connection refused, 429) retry instead of
// counting toward this — they never trip the breaker (PSY-887, preserved by
// PSY-1140).
const radioCircuitBreakerThreshold = 5

// radioBreakerCooldown is how long an open breaker waits before it allows a single
// half-open trial. A successful trial closes the breaker; a failed trial re-opens
// it with a fresh cooldown. 30 min balances "recover quickly once the provider is
// back" against "don't hammer a still-broken provider every cycle."
const radioBreakerCooldown = 30 * time.Minute

// breakerSnapshot is the persisted breaker state read from radio_station_health. A
// never-synced station (no row) is the zero value — closed, 0 failures, no trip
// time — so a brand-new station is never born tripped.
type breakerSnapshot struct {
	state     string
	failures  int
	trippedAt *time.Time
}

// breakerGate is the gate decision for a scheduled/auto run (manual bypasses).
type breakerGate int

const (
	gateAllow   breakerGate = iota // closed / never-synced → run normally
	gateBlocked                    // open and still within cooldown → skip
	gateTrial                      // open past cooldown (or half_open) → run one trial
)

// breakerGateFor decides whether a scheduled/auto run may proceed. Pure (no I/O) so
// it is unit-tested directly. Both open AND half_open are cooldown-gated against
// breaker_tripped_at: past the cooldown → one trial; within it → blocked. half_open
// is gated identically to open (NOT allowed unconditionally) so a trial that never
// resolved cannot re-trial every cycle: a run cancelled on shutdown or a panic
// leaves the row at half_open (updateStationHealth never ran), and markBreakerHalfOpen
// stamped breaker_tripped_at at trial start — so the stranded breaker still waits a
// full cooldown before the next trial instead of defeating the cooldown. An
// open/half_open row with no trip time is treated as freshly tripped (blocked) —
// defensive against a row written without breaker_tripped_at.
func breakerGateFor(snap breakerSnapshot, now time.Time) breakerGate {
	switch snap.state {
	case catalogm.RadioBreakerStateOpen, catalogm.RadioBreakerStateHalfOpen:
		if snap.trippedAt == nil {
			return gateBlocked
		}
		if now.Sub(*snap.trippedAt) >= radioBreakerCooldown {
			return gateTrial
		}
		return gateBlocked
	default: // closed (or never-synced) → allow
		return gateAllow
	}
}

// breakerTransition is the PURE next-state function for the persistent breaker.
// Given the current snapshot and a run outcome (status, trigger, errKind), it
// returns the next snapshot. No I/O — exhaustively unit-tested; updateStationHealth
// applies the result. Policy (design §5.2/§5.3 + the PSY-1140 locked decision):
//   - success / partial → reset to closed (a partial imported data; it is recovery,
//     not a breaker failure — keeps a chronically-noisy-but-healthy station from
//     ever climbing the counter).
//   - skipped (and any non-terminal) → unchanged (a breaker skip is not a signal).
//   - failed + MANUAL → unchanged: a manual run is a half-open probe; the operator
//     chose to poke a known-bad station, so a manual failure never trips (a manual
//     SUCCESS still closes via the success arm above — the asymmetric policy).
//   - failed + scheduled/auto → only a PERMANENT error increments the counter
//     (transient retries, never trips — PSY-887). A failed half-open trial re-opens
//     with a fresh cooldown regardless of kind; a permanent failure reaching the
//     threshold opens the breaker.
func breakerTransition(cur breakerSnapshot, status, trigger string, errKind errorKind, now time.Time) breakerSnapshot {
	switch status {
	case catalogm.RadioSyncRunStatusSuccess, catalogm.RadioSyncRunStatusPartial:
		return breakerSnapshot{state: catalogm.RadioBreakerStateClosed, failures: 0, trippedAt: nil}
	case catalogm.RadioSyncRunStatusFailed:
		if trigger == catalogm.RadioSyncRunTriggerManual {
			return cur // half-open probe: a manual failure never trips
		}
		next := cur
		if errKind == kindPermanent {
			next.failures = cur.failures + 1
		}
		// transient: counter unchanged (PSY-887 — transient never trips)
		switch {
		case cur.state == catalogm.RadioBreakerStateHalfOpen:
			// The trial failed (permanent OR transient) → re-open with fresh cooldown.
			t := now
			next.state, next.trippedAt = catalogm.RadioBreakerStateOpen, &t
		case errKind == kindPermanent && next.failures >= radioCircuitBreakerThreshold:
			t := now
			next.state, next.trippedAt = catalogm.RadioBreakerStateOpen, &t
		}
		return next
	default: // skipped / running / cancelled → leave the breaker untouched
		return cur
	}
}

// readBreakerSnapshot loads the persisted breaker state for a station. A missing
// row (never synced) or a read error returns the closed zero value — a station is
// never born tripped, and an observability-layer hiccup must not halt ingestion
// (the breaker's BLOCK state is 'open', so defaulting to closed/allow is the
// permissive choice).
func (s *RadioService) readBreakerSnapshot(stationID uint) breakerSnapshot {
	var health catalogm.RadioStationHealth
	err := s.db.Select("breaker_state", "consecutive_failures", "breaker_tripped_at").
		First(&health, "station_id = ?", stationID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return breakerSnapshot{state: catalogm.RadioBreakerStateClosed}
	}
	if err != nil {
		slog.Warn("radio: breaker snapshot read failed; treating as closed", "station_id", stationID, "error", err)
		return breakerSnapshot{state: catalogm.RadioBreakerStateClosed}
	}
	return breakerSnapshot{
		state:     health.BreakerState,
		failures:  health.ConsecutiveFailures,
		trippedAt: health.BreakerTrippedAt,
	}
}

// markBreakerHalfOpen flips a tripped breaker (open, or a still-stranded half_open
// from a prior unresolved trial) to half_open AND stamps breaker_tripped_at=now —
// the trial-start time. That timestamp refresh is what bounds a stranded trial: if
// this run never resolves the state (cancelled on shutdown, or a panic — both skip
// updateStationHealth), the row stays half_open with a recent trip time, so
// breakerGateFor keeps it blocked for a full cooldown before the next trial instead
// of re-trialing every cycle.
//
// Best-effort: if this write fails (a DB error), the row stays as the gate read it
// (open/half_open with its prior trip time). The breaker still works — it stays
// cooldown-gated via that prior timestamp — losing only the half_open label and this
// trial's cooldown refresh; on a DB flaky enough to drop this write the run's own
// writes would be failing too.
func (s *RadioService) markBreakerHalfOpen(stationID uint) {
	now := time.Now()
	_ = s.db.Model(&catalogm.RadioStationHealth{}).
		Where("station_id = ? AND breaker_state IN ?", stationID,
			[]string{catalogm.RadioBreakerStateOpen, catalogm.RadioBreakerStateHalfOpen}).
		Updates(map[string]any{
			"breaker_state":      catalogm.RadioBreakerStateHalfOpen,
			"breaker_tripped_at": now,
			"updated_at":         now,
		}).Error
}

// updateStationHealth upserts the per-station health rollup by station_id (ON
// CONFLICT column-set inference) and applies the breaker state machine. It reads the
// current snapshot, computes the next state via the pure breakerTransition, and
// writes it back — safe because RunStationSync holds the per-station advisory lock
// for the whole run, so no concurrent run mutates this station's health. last_run_at
// is always bumped; last_success_at only on a clean success ('partial' resets the
// breaker but is not a "last success").
func (s *RadioService) updateStationHealth(stationID uint, status, trigger string, errKind errorKind) {
	now := time.Now()
	next := breakerTransition(s.readBreakerSnapshot(stationID), status, trigger, errKind, now)

	assignments := map[string]any{
		"last_run_at":          now,
		"consecutive_failures": next.failures,
		"breaker_state":        next.state,
		"updated_at":           now,
	}
	if next.trippedAt != nil {
		assignments["breaker_tripped_at"] = *next.trippedAt
	} else {
		assignments["breaker_tripped_at"] = gorm.Expr("NULL")
	}
	health := catalogm.RadioStationHealth{
		StationID:           stationID,
		LastRunAt:           &now,
		ConsecutiveFailures: next.failures,
		BreakerState:        next.state,
		BreakerTrippedAt:    next.trippedAt,
	}
	if status == catalogm.RadioSyncRunStatusSuccess {
		health.LastSuccessAt = &now
		assignments["last_success_at"] = now
	}

	// Recompute the observability rates over the trailing window (PSY-1201). On-write:
	// the run row was already closed (status + play counts committed) above, so this
	// includes the current run. Best-effort — on a query error we leave the rate columns
	// untouched (omit from both paths) rather than clobber a prior value or fail the
	// health write.
	if successRate, playMatchRate, zeroPlayRate, ok := s.computeStationRates(stationID, now); ok {
		health.RecentSuccessRate = successRate
		health.PlayMatchRate = playMatchRate
		health.ZeroPlayEpisodeRate = zeroPlayRate
		setNullableFloatAssignment(assignments, "recent_success_rate", successRate)
		setNullableFloatAssignment(assignments, "play_match_rate", playMatchRate)
		setNullableFloatAssignment(assignments, "zero_play_episode_rate", zeroPlayRate)
	}

	_ = s.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "station_id"}},
		DoUpdates: clause.Assignments(assignments),
	}).Create(&health).Error
}

// stationHealthRateWindow is the trailing window over which the observability rates are
// computed (PSY-1201). 30 days balances "recent enough to flag a regression" against
// "enough runs to be meaningful" at the current scheduled cadence.
const stationHealthRateWindow = 30 * 24 * time.Hour

// setNullableFloatAssignment writes a *float64 into a GORM assignments map as a value or
// an explicit NULL (mirrors the breaker_tripped_at handling), so a nil rate clears the
// column rather than being silently skipped.
func setNullableFloatAssignment(m map[string]any, col string, v *float64) {
	if v != nil {
		m[col] = *v
	} else {
		m[col] = gorm.Expr("NULL")
	}
}

// computeStationRates derives the three radio_station_health rates over the trailing
// window for a station (PSY-1201): recent_success_rate (success / terminal runs),
// play_match_rate (matched / imported plays), and zero_play_episode_rate (zero-play /
// total aired episodes). Each is nil when its denominator is zero (no data → "never
// computed", distinct from 0.0). ok=false on a query error so the caller leaves the
// existing values untouched. Best-effort: errors are logged, never surfaced.
func (s *RadioService) computeStationRates(stationID uint, now time.Time) (successRate, playMatchRate, zeroPlayRate *float64, ok bool) {
	windowStart := now.Add(-stationHealthRateWindow)

	// Runs: success rate + play-match rate in one pass over the window's ATTEMPTED runs.
	// We exclude running (incomplete), cancelled (operator action), and skipped (breaker
	// no-op — the run never attempted a sync); none is a health signal, and counting
	// skipped would make a breaker-protected station's success rate decay toward 0 while
	// the breaker is protecting it. So terminal = COUNT(*) of attempts, and the play SUMs
	// likewise ignore cancelled partial mid-flight plays. The just-closed current run is
	// committed and (unless skipped/cancelled) counts.
	var runAgg struct {
		Success  int64
		Terminal int64
		Matched  int64
		Imported int64
	}
	if err := s.db.Raw(`
		SELECT
			COUNT(*) FILTER (WHERE status = ?) AS success,
			COUNT(*) AS terminal,
			COALESCE(SUM(plays_matched), 0) AS matched,
			COALESCE(SUM(plays_imported), 0) AS imported
		FROM radio_sync_runs
		WHERE station_id = ? AND started_at >= ?
			AND status NOT IN (?, ?, ?)
	`, catalogm.RadioSyncRunStatusSuccess,
		stationID, windowStart,
		catalogm.RadioSyncRunStatusRunning, catalogm.RadioSyncRunStatusCancelled, catalogm.RadioSyncRunStatusSkipped).Scan(&runAgg).Error; err != nil {
		slog.Warn("radio: computing station run-rates failed", "station_id", stationID, "error", err)
		return nil, nil, nil, false
	}

	// Episodes aired in the window for this station's shows: zero-play rate. air_date is
	// a DATE column; the YYYY-MM-DD cutoff compares correctly.
	var epAgg struct {
		Zero  int64
		Total int64
	}
	if err := s.db.Raw(`
		SELECT
			COUNT(*) FILTER (WHERE e.play_count = 0) AS zero,
			COUNT(*) AS total
		FROM radio_episodes e
		JOIN radio_shows sh ON sh.id = e.show_id
		WHERE sh.station_id = ? AND e.air_date >= ?
	`, stationID, windowStart.Format("2006-01-02")).Scan(&epAgg).Error; err != nil {
		slog.Warn("radio: computing station episode-rate failed", "station_id", stationID, "error", err)
		return nil, nil, nil, false
	}

	return ratio(runAgg.Success, runAgg.Terminal),
		ratio(runAgg.Matched, runAgg.Imported),
		ratio(epAgg.Zero, epAgg.Total),
		true
}

// ratio returns numerator/denominator as a *float64, or nil when denominator is zero
// (no data to measure — distinct from a genuine 0.0).
func ratio(numerator, denominator int64) *float64 {
	if denominator == 0 {
		return nil
	}
	r := float64(numerator) / float64(denominator)
	return &r
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

// stringErrs maps free-text error strings into categorized run-error rows via the
// substring heuristic. As of PSY-1141 this is the fallback for the DISCOVER path
// only (RadioDiscoverResult.Errors) — the import path threads structured categories
// out at the source (see categorizedToRunErrors / recordImportError), so its
// categories are exact, not guessed.
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

// categorizeRunError maps an error to a radio_sync_run_errors category. Typed checks
// (deadline, RadioHTTPError, net.Error timeout) come first; a remaining non-HTTP,
// non-timeout error is then parse-detected by message before defaulting to
// provider_unreachable.
//
// The parse arm is load-bearing for escalation (PSY-1141): a provider format/scraper
// change surfaces as a wrapped parse failure (every provider wraps it
// "parsing X response: %w"), reaching here via importEpisode's FetchErrorCategory.
// Without this arm a parse failure defaulted to provider_unreachable and the
// scraper-drift Sentry escalation (escalationError's parse_error arm) was unreachable
// on the import path. Only parse is string-matched here (not drop/truncation — those
// are import-path-only concepts handled structurally, and matching "dropped" risks
// false positives on network-error strings).
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
	// Reached only after the typed transient checks above (deadline / RadioHTTPError
	// 429 / net timeout), so a parse-classified error here is permanent by
	// construction. Invariant: providers signal transience via RadioHTTPError/net
	// (caught above), NOT by joining ErrTransient onto a "parsing …" message — a
	// hypothetical transient-tagged parse string would mis-route here. None do today.
	// "parsing" (not just "parse") matters: every provider wraps parse failures as
	// "parsing X response: %w" — strings.Contains("parsing","parse") is false.
	ls := strings.ToLower(err.Error())
	if strings.Contains(ls, "parsing") || strings.Contains(ls, "parse") ||
		strings.Contains(ls, "unmarshal") || strings.Contains(ls, "decode") ||
		strings.Contains(ls, "invalid character") {
		return catalogm.RadioSyncRunErrorParseError
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
	case strings.Contains(ls, "parsing") || strings.Contains(ls, "parse") || strings.Contains(ls, "unmarshal") || strings.Contains(ls, "decode") || strings.Contains(ls, "invalid character"):
		return catalogm.RadioSyncRunErrorParseError
	// NOTE: as of PSY-1141 this heuristic is the DISCOVER-path fallback only (the
	// import path categorizes structurally — see categorizedToRunErrors). Drop/
	// truncation summaries arise ONLY on the import path, so the two arms below are
	// unreachable from both current callers; they are retained purely as a defensive
	// default for arbitrary future free-text error strings. validation_drop is checked
	// before truncation so a combined "dropped N plays: ... truncated" summary buckets
	// as a drop.
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

// ───────────────────────────── permanent-failure escalation ─────────────────────────────

// escalationError decides whether a resolved run carries a failure worth paging on,
// returning its category + the representative error (nil error → do not escalate).
// Pure (no I/O) so it is unit-tested directly. Policy (PSY-1141 + the locked decision — "every
// permanent failure on a scheduled/auto run"):
//   - manual trigger → never (the operator already sees the result);
//   - a hard FAILED run whose error classifies PERMANENT → escalate. NOTE this is
//     deliberately BROAD per the locked decision: it includes config/setup/DB hard
//     errors (a misconfigured station IS a permanent failure worth knowing about), not
//     only provider drift. The error_category tag distinguishes them downstream
//     (parse_error = format change vs provider_unreachable = config/infra/4xx-5xx);
//   - a run carrying any per-episode parse_error → escalate once: a scraper format
//     change surfaces as a PARTIAL run of per-episode parse failures, not a hard
//     failure (the episode loop continues), so escalating only on FAILED would miss
//     the headline signal. parse_error is reachable on the import path now that
//     categorizeRunError parse-detects (it was dead before — the PSY-1141 review fix);
//   - everything else (transient, data-quality drops, success/partial-without-parse) →
//     no escalation.
func escalationError(out syncOutcome, trigger string) (string, error) {
	if trigger == catalogm.RadioSyncRunTriggerManual {
		return "", nil
	}
	if out.status == catalogm.RadioSyncRunStatusFailed && out.hardErr != nil &&
		classifyError(out.hardErr) == kindPermanent {
		return categorizeRunError(out.hardErr), out.hardErr
	}
	for _, e := range out.errs {
		if e.category == catalogm.RadioSyncRunErrorParseError {
			return catalogm.RadioSyncRunErrorParseError, errors.New(e.detail)
		}
	}
	return "", nil
}

// escalatePermanentFailure escalates a permanent radio sync failure to Sentry, tagged
// with the station + error category (the canonical sentry.WithScope + CaptureException
// pattern used across the services). The onPermanentFailure seam lets tests observe
// escalation without a Sentry transport; nil → the real call (a no-op when Sentry is
// not initialised, e.g. in tests that don't set the seam).
func (s *RadioService) escalatePermanentFailure(err error, stationID uint, category string) {
	if s.onPermanentFailure != nil {
		s.onPermanentFailure(err, stationID, category)
		return
	}
	sentry.WithScope(func(scope *sentry.Scope) {
		stationTag := strconv.FormatUint(uint64(stationID), 10)
		scope.SetTag("service", "radio_sync")
		scope.SetTag("radio_station_id", stationTag)
		scope.SetTag("error_category", category)
		// Group ALL escalations for a (station, category) into ONE Sentry issue with a
		// rising occurrence count, instead of fragmenting per-episode (the captured
		// detail varies by episode, so the default fingerprint would open a new issue
		// per episode per run). This is what makes the locked "rely on Sentry's native
		// grouping" decision actually hold for the parse-drift case, which recurs every
		// fetch cycle — a parse-drift run is PARTIAL (some episodes created), so it does
		// not trip the breaker and is not otherwise damped (PSY-1141 review).
		scope.SetFingerprint([]string{"radio_sync", category, stationTag})
		// Cap the captured message to the same bound the run-errors table enforces
		// (truncateForDetail), so a provider's raw response body — RadioHTTPError
		// embeds up to 512B of it — can't be exfiltrated to Sentry unbounded. The DB
		// path truncates; escalation must too (PSY-1141 review).
		sentry.CaptureException(errors.New(truncateForDetail(err.Error())))
	})
}

// radioFetchOutageEscalationThreshold is the FLOOR for how stale a station's
// last_playlist_fetch_at must be before the janitor escalates it as a sustained
// total-fetch outage. The janitor raises it to 3× the configured fetch interval when
// that is larger (see runJanitorCycle) — RADIO_FETCH_INTERVAL_HOURS is operator-tunable,
// so a const alone would false-escalate healthy stations once the interval is widened.
// PSY-1241 holds the watermark stale across a wholly-failed run and a healthy run
// always advances it (success, empty, or no fetchable shows), so staleness beyond ~3
// fetch cycles means the station has imported nothing for that long. 18h ≈ 3 missed
// cycles at the default 6h interval. (PSY-1269)
const radioFetchOutageEscalationThreshold = 18 * time.Hour

// radioFetchOutageCategory is the escalation category — the Sentry error_category tag
// and a fingerprint component (escalatePermanentFailure) — for a sustained total-fetch
// outage. A named const (vs a bare literal) keeps the production call and its test
// assertion from drifting and silently fragmenting Sentry grouping. (PSY-1269)
const radioFetchOutageCategory = "fetch_outage"

// EscalateStaleFetchOutages escalates active stations whose last successful playlist
// fetch (last_playlist_fetch_at) is older than `threshold` — a sustained total-fetch
// outage (PSY-1269). It reuses the PSY-1241 held-watermark as the outage signal: a
// healthy run always advances the watermark, so a stale one means every fetchable
// show has been failing. Excluded: manual-source stations (no automated fetch) and
// never-fetched stations (NULL watermark) — a provider broken from day one surfaces
// only via the per-cycle no-progress Warn log (advanceLastFetch), not this Sentry
// escalation. Escalations group per (station, radioFetchOutageCategory) in Sentry, so
// a persistent outage is one issue re-triggered each janitor cycle, not nightly spam.
// Returns the number escalated.
func (s *RadioService) EscalateStaleFetchOutages(threshold time.Duration, now time.Time) (int, error) {
	if s.db == nil {
		return 0, fmt.Errorf("database not initialized")
	}
	cutoff := now.Add(-threshold)
	var stations []catalogm.RadioStation
	err := s.db.
		Where("is_active = TRUE AND playlist_source IS NOT NULL AND playlist_source != '' AND playlist_source != ?", catalogm.PlaylistSourceManual).
		Where("last_playlist_fetch_at IS NOT NULL AND last_playlist_fetch_at < ?", cutoff).
		Find(&stations).Error
	if err != nil {
		return 0, fmt.Errorf("querying stale-fetch stations: %w", err)
	}
	for i := range stations {
		st := stations[i]
		staleFor := now.Sub(*st.LastPlaylistFetchAt).Round(time.Hour)
		s.escalatePermanentFailure(
			fmt.Errorf("radio station %q has imported no new playlists for ~%s (last successful fetch %s)",
				st.Slug, staleFor, st.LastPlaylistFetchAt.UTC().Format(time.RFC3339)),
			st.ID, radioFetchOutageCategory)
	}
	return len(stations), nil
}

// escalateShowPermanentFailure is the per-SHOW sibling of escalatePermanentFailure
// (PSY-1274): same Sentry path, fingerprinted per (show, category). To be precise
// about volume: Sentry receives one EVENT per janitor cycle while the streak
// persists; the fingerprint collapses them into ONE issue with a rising occurrence
// count (and re-opens it if resolved while still broken). That per-cycle re-fire is
// the same deliberate convention as the station escalation — the population is
// bounded (broken shows on otherwise-healthy, non-retired stations), and a resolved-
// but-recurring issue is the desired "still broken" signal. The station id rides
// along as a tag (not a fingerprint component) so the issue is filterable by station.
func (s *RadioService) escalateShowPermanentFailure(err error, showID, stationID uint, category string) {
	if s.onShowPermanentFailure != nil {
		s.onShowPermanentFailure(err, showID, category)
		return
	}
	sentry.WithScope(func(scope *sentry.Scope) {
		showTag := strconv.FormatUint(uint64(showID), 10)
		scope.SetTag("service", "radio_sync")
		scope.SetTag("radio_show_id", showTag)
		scope.SetTag("radio_station_id", strconv.FormatUint(uint64(stationID), 10))
		scope.SetTag("error_category", category)
		scope.SetFingerprint([]string{"radio_sync", category, "show", showTag})
		// Same detail cap as escalatePermanentFailure: the escalation must not carry
		// more of a provider's raw response than the run-errors table would.
		sentry.CaptureException(errors.New(truncateForDetail(err.Error())))
	})
}

// radioShowFetchFailureEscalationThreshold is how many CONSECUTIVE scheduled-fetch
// failures a single show accumulates before the janitor escalates it (PSY-1274).
// The counter is cadence-independent (a successful fetch returning zero episodes
// resets it — see RadioShow.ConsecutiveFetchFailures), and counting cycles rather
// than wall-clock hours means the threshold scales with RADIO_FETCH_INTERVAL_HOURS
// for free. 3 mirrors the station janitor's ~3-missed-cycles bar
// (radioFetchOutageEscalationThreshold = 3 × the default 6h interval): one or two
// failures are provider flakiness; three back-to-back cycles (~18h at the default
// interval) is a show that needs an admin (typically a renamed/removed external_id).
const radioShowFetchFailureEscalationThreshold = 3

// radioShowFetchOutageCategory is the escalation category (Sentry error_category tag
// + fingerprint component) for a per-show sustained fetch outage. Distinct from the
// station-level radioFetchOutageCategory so the two never share a Sentry issue.
const radioShowFetchOutageCategory = "show_fetch_outage"

// EscalateShowFetchFailureStreaks escalates active, fetchable shows whose
// consecutive-fetch-failure streak has reached `streakThreshold` (PSY-1274). This
// closes the alerting gap PSY-1272 left deliberately open: the per-show watermark
// holds a broken show's frontier for auto-recovery, but the station watermark
// advances when any sibling succeeds, so a single chronically-broken show never
// trips the PSY-1269 station escalation.
//
// healthyWindow is the healthy-station guard: shows on a station whose watermark is
// staler than it are skipped — every sibling streaking at once IS a station-level
// outage, the PSY-1269 escalation's job, and escalating each sibling alongside it
// would be pure Sentry noise. Callers MUST derive healthyWindow from the SAME clock
// as the streak (streakThreshold × the scheduled fetch interval): if it were looser
// (e.g. the 18h-floored station threshold while cycles run hourly), a short total-
// station outage would trip every sibling's streak before the station counts as
// unhealthy, producing exactly the per-show storm this guard exists to prevent.
//
// retired shows are excluded: retiring IS the documented remediation for a
// permanently-gone external_id (PSY-1152), so it must quiesce this alert (the
// fetch loop still selects on the legacy is_active flag and would otherwise keep
// failing/escalating a retired show forever). Escalations group per
// (show, radioShowFetchOutageCategory). Returns the number escalated.
func (s *RadioService) EscalateShowFetchFailureStreaks(streakThreshold int, healthyWindow time.Duration, now time.Time) (int, error) {
	if s.db == nil {
		return 0, fmt.Errorf("database not initialized")
	}
	stationOutageCutoff := now.Add(-healthyWindow)
	var shows []catalogm.RadioShow
	err := s.db.
		Joins("JOIN radio_stations ON radio_stations.id = radio_shows.station_id").
		Where("radio_shows.is_active = TRUE AND radio_shows.external_id IS NOT NULL AND radio_shows.external_id != ''").
		Where("radio_shows.lifecycle_state != ?", catalogm.RadioLifecycleRetired).
		Where("radio_shows.consecutive_fetch_failures >= ?", streakThreshold).
		Where("radio_stations.is_active = TRUE AND radio_stations.playlist_source IS NOT NULL AND radio_stations.playlist_source != '' AND radio_stations.playlist_source != ?", catalogm.PlaylistSourceManual).
		// Healthy-station guard: NULL station watermark (never fetched) is excluded by
		// the comparison too — same never-fetched carve-out as the station janitor.
		Where("radio_stations.last_playlist_fetch_at >= ?", stationOutageCutoff).
		Find(&shows).Error
	if err != nil {
		return 0, fmt.Errorf("querying show fetch-failure streaks: %w", err)
	}
	for i := range shows {
		sh := shows[i]
		s.escalateShowPermanentFailure(
			fmt.Errorf("radio show %q has failed %d consecutive scheduled fetches (station otherwise healthy — likely a stale external_id or persistent provider errors for this show)",
				sh.Slug, sh.ConsecutiveFetchFailures),
			sh.ID, sh.StationID, radioShowFetchOutageCategory)
	}
	return len(shows), nil
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
