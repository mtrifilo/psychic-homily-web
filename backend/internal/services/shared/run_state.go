package shared

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"gorm.io/gorm"
)

// Durable run state for periodic background loops.
//
// The defect this replaces: RunTickerLoop's schedule lived entirely in a
// process-local time.Ticker anchored at process start. On a continuously-deployed
// platform the process restarts far more often than a daily sweep's interval, so
// the first tick was never reached and seven production sweeps silently did
// nothing for the life of production.
//
// The fix is to move the schedule OUT of process memory: a loop records when it
// last completed, and the next boot asks the database "am I overdue?" rather than
// restarting a timer. Nothing about deploy cadence can starve a loop once the
// answer lives in a table.
//
// The same row doubles as the run-trace (outcome / error / duration / rows), which
// is what makes a loop's health answerable at all: whether the radio janitor had
// ever run could not be determined even in retrospect, because every table it
// writes is also written by a 6h loop.

// Run outcomes recorded in background_service_runs.last_outcome. Kept as
// constants so the CHECK constraint in the migration and the writer cannot drift.
const (
	RunOutcomeRunning = "running"
	RunOutcomeSuccess = "success"
	RunOutcomeError   = "error"
	RunOutcomePanic   = "panic"
)

// maxRunErrorLength bounds what we persist from a cycle's error. A provider that
// echoes a whole response body into an error must not turn a status row into a
// megabyte.
const maxRunErrorLength = 2000

// BackgroundServiceRun is the persisted state of one background loop.
//
// It is deliberately a "latest state" row rather than a journal: the questions it
// exists to answer — when is this due, did the last cycle work — are about now,
// and a journal would need retention management to answer them no better.
type BackgroundServiceRun struct {
	Name            string `gorm:"column:name;primaryKey"`
	IntervalSeconds int64  `gorm:"column:interval_seconds"`

	// LastStartedAt is the claim timestamp; it stays set for the duration of a
	// cycle so a concurrent instance can distinguish a live run from a crashed one.
	LastStartedAt *time.Time `gorm:"column:last_started_at"`
	// LastCompletedAt is written for BOTH success and failure and is what
	// SCHEDULING keys on — see the migration comment for why.
	LastCompletedAt *time.Time `gorm:"column:last_completed_at"`
	// LastSuccessAt carries health, separate from schedule.
	LastSuccessAt *time.Time `gorm:"column:last_success_at"`

	LastOutcome       string  `gorm:"column:last_outcome"`
	LastError         *string `gorm:"column:last_error"`
	LastDurationMs    *int64  `gorm:"column:last_duration_ms"`
	LastRowsProcessed *int64  `gorm:"column:last_rows_processed"`

	ConsecutiveFailures int   `gorm:"column:consecutive_failures"`
	RunCount            int64 `gorm:"column:run_count"`

	CreatedAt time.Time `gorm:"column:created_at"`
	UpdatedAt time.Time `gorm:"column:updated_at"`
}

// TableName pins the table so a GORM naming-strategy change can't silently
// repoint the scheduler at a table that doesn't exist.
func (BackgroundServiceRun) TableName() string { return "background_service_runs" }

// CycleOutcome is what a finished cycle reports back to the store.
type CycleOutcome struct {
	// Rows is nil when the cycle did not report a count. Distinct from a reported
	// zero, which means "ran, nothing to do".
	Rows     *int
	Err      error
	Panicked bool
	Duration time.Duration

	// Interrupted marks a cycle cut short by shutdown rather than finished. Such
	// a cycle must NOT stamp a completion: doing so would tell the next boot the
	// work is done for another whole interval, so a deploy landing mid-cycle
	// would quietly skip a day of it. Instead the claim is released and the loop
	// reads as still overdue.
	Interrupted bool
}

// outcomeLabel maps a finished cycle onto the persisted vocabulary.
func (o CycleOutcome) outcomeLabel() string {
	switch {
	case o.Panicked:
		return RunOutcomePanic
	case o.Err != nil:
		return RunOutcomeError
	default:
		return RunOutcomeSuccess
	}
}

// RunStore persists background-loop run state.
//
// Every time comparison it makes uses the DATABASE clock. That is not an
// implementation detail: with the schedule shared between a process and a table,
// an app/DB clock skew would otherwise translate directly into a loop running
// early, late, or twice. The store never accepts a timestamp from the caller.
type RunStore interface {
	// DueIn reports how long until `name` is next due, measured against the
	// database clock. A value <= 0 means overdue — including the never-run case,
	// where there is no row at all.
	DueIn(ctx context.Context, name string, interval time.Duration) (time.Duration, error)

	// Claim atomically records the start of a cycle, returning false when the
	// cycle must NOT run: either another instance holds a live claim, or (unless
	// force is set) the loop is not yet due.
	//
	// The returned token identifies this specific claim; pass it to Complete so a
	// cycle that overran its lease and was superseded cannot clobber the state of
	// whichever run replaced it.
	Claim(ctx context.Context, name string, interval, lease time.Duration, force bool) (token time.Time, claimed bool, err error)

	// Complete records a finished cycle against the claim identified by token.
	Complete(ctx context.Context, name string, token time.Time, outcome CycleOutcome) error
}

// ErrClaimSuperseded reports that Complete found no row matching the claim token:
// something else re-claimed the loop while this cycle was running. The cycle's
// result is dropped rather than overwriting the newer run's state.
var ErrClaimSuperseded = errors.New("background run claim superseded by another run")

// GormRunStore is the Postgres-backed RunStore.
type GormRunStore struct {
	db *gorm.DB
}

// NewGormRunStore builds a store over an existing connection. Returns nil for a
// nil database so a caller can pass the result straight through to LoopConfig —
// a nil store degrades the loop to non-persistent scheduling rather than panicking.
func NewGormRunStore(database *gorm.DB) *GormRunStore {
	if database == nil {
		return nil
	}
	return &GormRunStore{db: database}
}

var _ RunStore = (*GormRunStore)(nil)

// dueSlack is how early a scheduled tick may claim without being rejected as
// "not due". A process ticker and the database clock will never agree to the
// millisecond; without slack, a tick arriving a hair early would be refused and
// the loop would wait another whole interval — turning a rounding error into a
// day of lost work for a 24h sweep.
func dueSlack(interval time.Duration) time.Duration {
	slack := interval / 20
	if slack > time.Minute {
		slack = time.Minute
	}
	if slack < time.Second {
		slack = time.Second
	}
	return slack
}

// DueIn implements RunStore.
func (s *GormRunStore) DueIn(ctx context.Context, name string, interval time.Duration) (time.Duration, error) {
	var secs []float64
	err := s.db.WithContext(ctx).Raw(`
		SELECT EXTRACT(EPOCH FROM (last_completed_at + make_interval(secs => ?) - NOW()))
		FROM background_service_runs
		WHERE name = ? AND last_completed_at IS NOT NULL
	`, interval.Seconds(), name).Scan(&secs).Error
	if err != nil {
		return 0, fmt.Errorf("read run state for %q: %w", name, err)
	}
	// No row, or a row that has never completed a cycle (first boot, or a process
	// that died mid-cycle): overdue. Keying on last_completed_at rather than
	// last_started_at is what stops a crash from wedging a loop forever.
	if len(secs) == 0 {
		return 0, nil
	}
	due := time.Duration(secs[0] * float64(time.Second))
	if due < 0 {
		return 0, nil
	}
	// A last_completed_at in the FUTURE (a clock that jumped, or a restore from a
	// newer environment) would otherwise park the loop for an unbounded time.
	// One interval is the longest a healthy loop should ever wait.
	if due > interval {
		due = interval
	}
	return due, nil
}

// Claim implements RunStore.
//
// One statement, so the decision and the write cannot be split by a concurrent
// instance. The ON CONFLICT ... WHERE predicate is the lock: PostgreSQL takes a
// row lock for the conflicting tuple before evaluating it, so exactly one of two
// racing claims can satisfy it.
func (s *GormRunStore) Claim(
	ctx context.Context,
	name string,
	interval, lease time.Duration,
	force bool,
) (time.Time, bool, error) {
	type claimRow struct {
		LastStartedAt time.Time
	}
	var rows []claimRow

	err := s.db.WithContext(ctx).Raw(`
		INSERT INTO background_service_runs AS b
			(name, interval_seconds, last_started_at, last_outcome, created_at, updated_at)
		VALUES (?, ?, NOW(), ?, NOW(), NOW())
		ON CONFLICT (name) DO UPDATE
		SET last_started_at  = NOW(),
		    last_outcome     = EXCLUDED.last_outcome,
		    interval_seconds = EXCLUDED.interval_seconds,
		    updated_at       = NOW()
		WHERE (
		        ?::boolean
		        OR b.last_completed_at IS NULL
		        OR b.last_completed_at <= NOW() - make_interval(secs => ?)
		      )
		  AND (
		        b.last_outcome IS DISTINCT FROM 'running'
		        OR b.last_started_at IS NULL
		        OR b.last_started_at <= NOW() - make_interval(secs => ?)
		      )
		RETURNING last_started_at
	`,
		name,
		int64(interval/time.Second),
		RunOutcomeRunning,
		force,
		(interval - dueSlack(interval)).Seconds(),
		lease.Seconds(),
	).Scan(&rows).Error
	if err != nil {
		return time.Time{}, false, fmt.Errorf("claim run for %q: %w", name, err)
	}
	if len(rows) == 0 {
		return time.Time{}, false, nil
	}
	return rows[0].LastStartedAt, true, nil
}

// Complete implements RunStore.
func (s *GormRunStore) Complete(ctx context.Context, name string, token time.Time, outcome CycleOutcome) error {
	label := outcome.outcomeLabel()
	ok := label == RunOutcomeSuccess

	var errText *string
	if outcome.Err != nil {
		msg := outcome.Err.Error()
		if len(msg) > maxRunErrorLength {
			msg = msg[:maxRunErrorLength]
		}
		errText = &msg
	} else if outcome.Panicked {
		msg := "cycle panicked"
		errText = &msg
	} else if outcome.Interrupted {
		msg := "cycle interrupted by shutdown"
		errText = &msg
	}

	var rowsProcessed *int64
	if outcome.Rows != nil {
		n := int64(*outcome.Rows)
		rowsProcessed = &n
	}

	if outcome.Interrupted {
		// Release the claim without stamping a completion, and without counting a
		// failure: a deploy landing mid-cycle is normal operation, not a fault.
		// The loop stays overdue, so the next boot picks the work straight back up.
		res := s.db.WithContext(ctx).Exec(`
			UPDATE background_service_runs SET
				last_started_at = NULL,
				last_outcome    = ?,
				last_error      = ?,
				updated_at      = NOW()
			WHERE name = ? AND last_started_at = ?
		`, RunOutcomeError, errText, name, token)
		if res.Error != nil {
			return fmt.Errorf("release interrupted run for %q: %w", name, res.Error)
		}
		if res.RowsAffected == 0 {
			return ErrClaimSuperseded
		}
		return nil
	}

	res := s.db.WithContext(ctx).Exec(`
		UPDATE background_service_runs SET
			last_completed_at    = NOW(),
			last_success_at      = CASE WHEN ?::boolean THEN NOW() ELSE last_success_at END,
			last_outcome         = ?,
			last_error           = ?,
			last_duration_ms     = ?,
			last_rows_processed  = ?,
			consecutive_failures = CASE WHEN ?::boolean THEN 0 ELSE consecutive_failures + 1 END,
			run_count            = run_count + 1,
			updated_at           = NOW()
		WHERE name = ? AND last_started_at = ?
	`,
		ok, label, errText, outcome.Duration.Milliseconds(), rowsProcessed, ok, name, token,
	)
	if res.Error != nil {
		return fmt.Errorf("complete run for %q: %w", name, res.Error)
	}
	if res.RowsAffected == 0 {
		return ErrClaimSuperseded
	}
	return nil
}

var (
	defaultRunStoreMu sync.RWMutex
	defaultRunStore   RunStore
)

// SetDefaultRunStore installs the process-wide store that RunTickerLoop uses when
// a LoopConfig does not supply one.
//
// A process-wide default rather than a constructor argument on all 20-odd
// background services: making persistence opt-in per call site would recreate
// exactly the failure this ticket fixes, where the safe behaviour depended on
// every future caller remembering to ask for it. Wired once from the service
// container; tests install their own via t.Cleanup.
func SetDefaultRunStore(store RunStore) {
	defaultRunStoreMu.Lock()
	defaultRunStore = store
	defaultRunStoreMu.Unlock()
}

// DefaultRunStore returns the process-wide store, or nil when none is installed
// (unit tests, CLIs). A nil store is a supported mode: loops fall back to a
// bounded start delay, which is still immune to the deploy-cadence starvation —
// it only loses the durable trace.
func DefaultRunStore() RunStore {
	defaultRunStoreMu.RLock()
	defer defaultRunStoreMu.RUnlock()
	return defaultRunStore
}

// typedNilStore reports whether a RunStore interface holds a typed nil pointer,
// which NewGormRunStore can produce from a nil *gorm.DB. Without this an
// `if store != nil` guard would pass and every call would panic.
func typedNilStore(store RunStore) bool {
	if store == nil {
		return true
	}
	if gs, ok := store.(*GormRunStore); ok {
		return gs == nil
	}
	return false
}

// cycleRecorderKey types the context key so nothing else can collide with it.
type cycleRecorderKey struct{}

// cycleRecorder collects what a running cycle chooses to report about itself.
type cycleRecorder struct {
	mu   sync.Mutex
	rows *int
	err  error
}

// ReportCycle lets the work function of a background loop attach a row count and
// an error to the persisted run trace. Calling it is optional: a cycle that never
// reports still records started/completed/duration, which is enough to answer
// "did this run?".
//
// It exists as a context call rather than a return value so that adding a trace to
// one sweep does not force a signature change on the twenty loops that have
// nothing to report. Safe to call from any goroutine, and a no-op outside a
// managed cycle (so a manual/admin invocation of the same cycle function works
// unchanged).
//
// The LAST call wins; a cycle that reports repeatedly reports its final state.
func ReportCycle(ctx context.Context, rows int, err error) {
	rec, ok := ctx.Value(cycleRecorderKey{}).(*cycleRecorder)
	if !ok || rec == nil {
		return
	}
	rec.mu.Lock()
	rec.rows = &rows
	rec.err = err
	rec.mu.Unlock()
}

// snapshot reads the reported values under the lock.
func (r *cycleRecorder) snapshot() (*int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.rows, r.err
}

// withCycleRecorder returns a context a cycle can report into.
func withCycleRecorder(ctx context.Context, rec *cycleRecorder) context.Context {
	return context.WithValue(ctx, cycleRecorderKey{}, rec)
}

// logStoreError keeps store failures loud but non-fatal. A database blip must
// never stop a background loop from doing its work — losing the trace is strictly
// better than losing the sweep.
func logStoreError(name, op string, err error) {
	slog.Default().Error("background run state: "+op+" failed",
		"service", name,
		"error", err,
	)
}
