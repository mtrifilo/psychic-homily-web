package shared

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"gorm.io/gorm"
)

// Overdue detection for background loops.
//
// PSY-1606 found seven production sweeps that had run exactly one cycle in the
// life of production. Nothing errored and nothing was misconfigured — the failure
// was an ABSENCE of events, which conventional monitoring cannot see. It surfaced
// only because a human looked at a map and recognised that a venue pin was in the
// wrong place. That detection path does not scale.
//
// PSY-1611 made the schedule durable, which is what makes the absence
// observable at all: a loop now records when it last completed, so "has this
// stopped running?" became a question the database can answer. This file asks it
// on a schedule and reports the answer.
//
// The hard part is not detection, it is restraint. An overdue sweep STAYS
// overdue, so the naive check re-reports on every pass, floods Sentry, and gets
// muted — leaving a system that looks monitored and is not. Hence the throttle:
// report on the healthy -> overdue transition, re-assert at most daily, clear on
// recovery.

const (
	// overdueIntervalMultiplier is how many of a loop's own intervals may pass
	// before it is considered stopped. Generous enough that one slow cycle cannot
	// flap the alert, tight enough that a daily sweep is caught in days rather
	// than the weeks PSY-1606 took.
	//
	// Uniform rather than per-loop on purpose: a per-loop tolerance is a knob
	// nobody tunes, and a wrong one reintroduces the config-inspection problem
	// that made the original incident expensive to diagnose. Only loops at or
	// above runStatePersistenceThreshold have rows, so the tightest window this
	// produces in practice is two hours.
	overdueIntervalMultiplier = 2

	// overdueRecoveryMargin is slack on top of interval + lease, covering the
	// boot catch-up stagger and ordinary scheduling jitter so a loop that is
	// recovering exactly on time is not reported the instant it succeeds.
	overdueRecoveryMargin = 15 * time.Minute

	// retireAfter is how long a row survives without any process re-registering
	// it before it stops being treated as something that should be running.
	//
	// Sized against DEPLOY cadence, not sweep cadence: production redeploys many
	// times a week, so a week of silence means no process has claimed this loop
	// across many boots. Long enough that a loop accidentally dropped from wiring
	// still alerts for days first — the case worth catching — and short enough
	// that a stage-to-prod database restore stops paging within a week instead of
	// forever.
	retireAfter = 7 * 24 * time.Hour

	// defaultOverdueReAlertAfter bounds re-reporting of a loop that is still
	// overdue. Seven simultaneously-dead sweeps cost seven events a day, which is
	// signal; the same seven on a 15m check would cost 672, which is noise.
	defaultOverdueReAlertAfter = 24 * time.Hour

	// defaultHealthCheckInterval is how often the check runs. Deliberately far
	// below runStatePersistenceThreshold so the checker takes no persisted row of
	// its own and cannot end up monitoring itself.
	defaultHealthCheckInterval = 15 * time.Minute
)

// OverdueLoop is one loop that has stopped running, as reported to the handler.
//
// Every field exists to answer a question an operator would otherwise have to go
// digging for: which loop, how bad, and did it fail loudly or just stop.
//
// It is a projection of background_service_runs rather than a reuse of
// BackgroundServiceRun because it carries a column that does not exist on the
// table — OverdueSeconds, computed on the database clock — and deliberately omits
// the scheduling fields an alert has no business consulting. The two structs must
// agree about nullability of the columns they share; they describe one table.
type OverdueLoop struct {
	Name            string `gorm:"column:name"`
	IntervalSeconds int64  `gorm:"column:interval_seconds"`

	// OverdueSeconds is how long it has actually been, computed on the DATABASE
	// clock like every other time comparison in this package — an app/DB skew
	// must not be able to manufacture or mask an alert.
	OverdueSeconds float64 `gorm:"column:overdue_seconds"`

	LastCompletedAt *time.Time `gorm:"column:last_completed_at"`
	LastSuccessAt   *time.Time `gorm:"column:last_success_at"`

	// LastOutcome is a pointer because a registered loop that has never run a
	// cycle has no outcome at all — distinct from one that ran and failed.
	LastOutcome         *string `gorm:"column:last_outcome"`
	LastError           *string `gorm:"column:last_error"`
	LastRowsProcessed   *int64  `gorm:"column:last_rows_processed"`
	ConsecutiveFailures int     `gorm:"column:consecutive_failures"`
	RunCount            int64   `gorm:"column:run_count"`
}

// Interval is the loop's configured cadence.
func (o OverdueLoop) Interval() time.Duration {
	return time.Duration(o.IntervalSeconds) * time.Second
}

// Overdue is how long it has been since the loop last completed (or since it was
// registered, if it never has).
func (o OverdueLoop) Overdue() time.Duration {
	return secondsToDuration(o.OverdueSeconds)
}

// OutcomeLabel renders last_outcome for logs and alert context, naming the
// never-run case explicitly rather than reporting it as an empty string.
func (o OverdueLoop) OutcomeLabel() string {
	if o.LastOutcome == nil {
		return "none"
	}
	return *o.LastOutcome
}

// NeverRan distinguishes a loop that was registered and never completed a cycle
// from one that was running and stopped.
//
// The two have different causes and different fixes: a never-run loop usually
// means a registration, wiring, or env-gating problem introduced at deploy time,
// whereas a stalled one was working and broke. Reporting them identically would
// send an operator looking in the wrong place.
func (o OverdueLoop) NeverRan() bool { return o.LastCompletedAt == nil }

// Failure modes. These are alert POLICY, not cosmetics: they tag the Sentry
// event and form part of its fingerprint, so changing a string re-groups every
// existing issue and can bury an ongoing outage under a new one.
const (
	FailureModeStalled  = "stalled"
	FailureModeNeverRan = "never_ran"
)

// FailureMode classifies the failure for tagging and fingerprinting.
//
// It lives here rather than in the Sentry handler so the classification is
// reachable from a test — nothing can call into cmd/server's main(), and an
// untestable branch that decides how alerts group is exactly the kind of thing
// that is discovered to be wrong during an incident.
func (o OverdueLoop) FailureMode() string {
	if o.NeverRan() {
		return FailureModeNeverRan
	}
	return FailureModeStalled
}

// Summary renders the one-line description used as the alert title.
func (o OverdueLoop) Summary() string {
	if o.NeverRan() {
		return fmt.Sprintf(
			"background sweep %q has NEVER completed a cycle (registered %s ago, interval %s)",
			o.Name, o.Overdue().Round(time.Minute), o.Interval(),
		)
	}
	return fmt.Sprintf(
		"background sweep %q is overdue: last completed %s ago, interval %s",
		o.Name, o.Overdue().Round(time.Minute), o.Interval(),
	)
}

// OverdueHandler receives one report per overdue loop per alert occurrence.
//
// A hook rather than a direct Sentry call for the same reason SetPanicHandler is
// one: this package must not depend on the observability stack, and tests need to
// observe reports without one. cmd/server/main.go installs the Sentry-capturing
// handler at startup.
type OverdueHandler func(loop OverdueLoop)

var (
	overdueHandlerMu sync.RWMutex
	overdueHandler   OverdueHandler
)

// SetOverdueHandler installs the process-wide handler. Pass nil to clear (tests
// via t.Cleanup).
func SetOverdueHandler(h OverdueHandler) {
	overdueHandlerMu.Lock()
	overdueHandler = h
	overdueHandlerMu.Unlock()
}

// invokeOverdueHandler runs the handler under the read lock, recovering any panic
// it raises.
//
// The recover is not defensive tidying. This whole subsystem exists because a
// background failure went unnoticed; a monitoring hook that could itself take
// down a loop would make the system less reliable than having no monitoring at
// all, which is the opposite of the point.
func invokeOverdueHandler(loop OverdueLoop) {
	overdueHandlerMu.RLock()
	h := overdueHandler
	overdueHandlerMu.RUnlock()
	if h == nil {
		return
	}
	defer recoverAndLog("overdue handler itself panicked", loop.Name)
	h(loop)
}

// OverdueClaimer reports loops that have stopped running, claiming each report so
// it is made once.
//
// Split from RunStore rather than added to it: RunStore is the SCHEDULING
// contract that every loop depends on, and widening it would force a health query
// onto in-memory doubles that have no interest in one. Only the health check
// needs this.
type OverdueClaimer interface {
	// ClaimOverdueAlerts atomically finds every loop that is overdue and not
	// currently throttled, marks each as reported, and returns them. Claiming and
	// reporting in one statement is what makes the once-per-occurrence guarantee
	// hold across replicas.
	ClaimOverdueAlerts(ctx context.Context, reAlertAfter time.Duration) ([]OverdueLoop, error)
}

var _ OverdueClaimer = (*GormRunStore)(nil)

// ClaimOverdueAlerts implements OverdueClaimer.
//
// One statement, for the same reason Claim is one: the decision and the write
// must not be separable by a concurrent instance. PostgreSQL locks each candidate
// row before re-evaluating the predicate, so of two replicas checking at the same
// moment exactly one gets the row back and the other sees the freshly-written
// last_overdue_alert_at and skips it. A read-then-write pair here would alert once
// per replica.
//
// COALESCE(last_completed_at, created_at) covers both failure shapes in one
// predicate: a loop that ran and stopped is measured from its last completion, and
// one that never ran from when it was registered — which also gives a
// newly-deployed loop a full grace window before it can be called dead.
//
// Note this reports a loop that was deliberately turned off via its env flag and
// left with a stale row. That is intended — from the outside, "switched off on
// purpose" and "silently stopped" are the same observation, and the alert is what
// forces the distinction to be made explicitly. Deleting the row retires the loop.
func (s *GormRunStore) ClaimOverdueAlerts(ctx context.Context, reAlertAfter time.Duration) ([]OverdueLoop, error) {
	var rows []OverdueLoop
	err := s.db.WithContext(ctx).Raw(`
		UPDATE background_service_runs SET
			last_overdue_alert_at = NOW(),
			updated_at            = NOW()
		WHERE interval_seconds > 0
		  -- Retirement gate. A loop announces itself on every boot, so a row
		  -- nobody has re-registered lately is not running because nothing
		  -- expects it to. Excluding it is what keeps a renamed, disabled, or
		  -- restored-from-stage row from alerting daily forever with no way to
		  -- clear it. NULL is treated as retired: rows predating registration
		  -- get stamped by the first boot of any process that owns the loop.
		  AND last_registered_at IS NOT NULL
		  AND last_registered_at > NOW() - make_interval(secs => ?)
		  -- Overdue threshold. GREATEST because 2x the interval alone is too
		  -- tight when the lease is comparable to the interval: a process killed
		  -- mid-cycle holds its claim for a full lease, so the earliest a healthy
		  -- recovery can complete is interval + lease. Without the floor an
		  -- hourly loop with an hourly lease crosses the threshold at the exact
		  -- moment it recovers, and ordinary platform behaviour reads as a stall.
		  AND COALESCE(last_completed_at, created_at) <= NOW() - make_interval(secs => GREATEST(
		        interval_seconds * ?,
		        interval_seconds + COALESCE(lease_seconds, ?) + ?
		      ))
		  AND (
		        last_overdue_alert_at IS NULL
		        OR last_overdue_alert_at <= NOW() - make_interval(secs => ?)
		      )
		RETURNING
			name,
			interval_seconds,
			EXTRACT(EPOCH FROM (NOW() - COALESCE(last_completed_at, created_at))) AS overdue_seconds,
			last_completed_at,
			last_success_at,
			last_outcome,
			last_error,
			last_rows_processed,
			consecutive_failures,
			run_count
	`,
		retireAfter.Seconds(),
		overdueIntervalMultiplier,
		defaultRunLease.Seconds(),
		overdueRecoveryMargin.Seconds(),
		reAlertAfter.Seconds(),
	).Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("claim overdue alerts: %w", err)
	}
	return rows, nil
}

// SweepHealthCheck reports background loops that have stopped running.
//
// It rides RunScheduledLoop like everything else it watches, with two properties
// that keep the bug class it detects from silencing it: a short interval, which
// the PSY-1606 audit showed empirically is never starved (the ten loops that ran
// continuously were exactly the ones that ran often), and RunAtBoot, which makes
// every deploy re-run the check rather than reset a timer toward it.
type SweepHealthCheck struct {
	store        OverdueClaimer
	interval     time.Duration
	reAlertAfter time.Duration
	stopCh       chan struct{}
	wg           sync.WaitGroup
	logger       *slog.Logger
}

// NewSweepHealthCheck builds the check over an existing connection. Returns nil
// for a nil database so a caller can start it unconditionally.
func NewSweepHealthCheck(database *gorm.DB) *SweepHealthCheck {
	store := NewGormRunStore(database)
	if store == nil {
		return nil
	}
	return &SweepHealthCheck{
		store:        store,
		interval:     healthCheckInterval(),
		reAlertAfter: overdueReAlertAfter(),
		stopCh:       make(chan struct{}),
		logger:       slog.Default(),
	}
}

// healthCheckInterval reads the configured cadence and CLAMPS it below the
// persistence threshold.
//
// The clamp is load-bearing, not defensive tidying. At or above the threshold
// the checker is handed a run-state row of its own, which puts it into the very
// set of loops it scans — it would start reporting on itself, and worse, a
// checker that stopped running would be the thing responsible for noticing. The
// env knob exists for operability (dropping the cadence during an incident), so
// it stays; it just cannot be used to disable the property the design rests on.
// maxOverdueReAlertAfter caps the re-assert window. Beyond about a week the
// policy stops being "throttled" and becomes "report once, then never again",
// which is the failure this design explicitly rejected: a single alert that gets
// resolved or missed leaves a dead sweep silent, i.e. PSY-1606 one level up.
const maxOverdueReAlertAfter = 7 * 24 * time.Hour

// overdueReAlertAfter reads the re-assert window and clamps it to something that
// still re-asserts. Clamped for the same reason the interval is: an operator
// reaching for this knob is trying to reduce noise, and the failure mode of
// overshooting is silence, which looks identical to health.
func overdueReAlertAfter() time.Duration {
	window := EnvPositiveDuration("SWEEP_OVERDUE_REALERT_HOURS", time.Hour, defaultOverdueReAlertAfter)
	if window > maxOverdueReAlertAfter {
		slog.Default().Warn("SWEEP_OVERDUE_REALERT_HOURS is long enough to read as report-once — clamping so a dead sweep keeps re-asserting",
			"configured", window,
			"using", maxOverdueReAlertAfter,
		)
		return maxOverdueReAlertAfter
	}
	return window
}

func healthCheckInterval() time.Duration {
	interval := EnvPositiveDuration("SWEEP_HEALTH_CHECK_INTERVAL_MINUTES", time.Minute, defaultHealthCheckInterval)
	if interval >= runStatePersistenceThreshold {
		slog.Default().Warn("SWEEP_HEALTH_CHECK_INTERVAL is at or above the run-state persistence threshold — clamping so the health check cannot monitor itself",
			"configured", interval,
			"threshold", runStatePersistenceThreshold,
			"using", defaultHealthCheckInterval,
		)
		return defaultHealthCheckInterval
	}
	return interval
}

// Start begins the health check.
func (c *SweepHealthCheck) Start(ctx context.Context) {
	if c == nil {
		return
	}
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		RunScheduledLoop(ctx, LoopConfig{
			Name:     "sweep_health_check",
			Interval: c.interval,
			StopCh:   c.stopCh,
			// Every deploy re-checks. This is the property that keeps the checker
			// immune to the starvation it looks for.
			RunAtBoot: true,
		}, c.runCycle)
	}()
	c.logger.Info("sweep health check started",
		"interval", c.interval,
		"re_alert_after", c.reAlertAfter,
		"overdue_multiplier", overdueIntervalMultiplier,
	)
}

// Stop gracefully stops the health check.
func (c *SweepHealthCheck) Stop() {
	if c == nil {
		return
	}
	close(c.stopCh)
	c.wg.Wait()
	c.logger.Info("sweep health check stopped")
}

// RunCheckNow runs one pass immediately (tests / manual trigger). Nil-safe like
// Start and Stop: NewSweepHealthCheck yields nil without a database, and a caller
// that starts the check unconditionally should be able to poke it the same way.
func (c *SweepHealthCheck) RunCheckNow(ctx context.Context) {
	if c == nil {
		return
	}
	c.runCycle(ctx)
}

// runCycle claims and reports one batch of overdue loops.
//
// A store failure is logged and dropped rather than returned: this loop's only
// job is observation, so a database blip must leave the sweeps it watches exactly
// as it found them. The next pass is fifteen minutes away.
func (c *SweepHealthCheck) runCycle(ctx context.Context) {
	loops, err := c.store.ClaimOverdueAlerts(ctx, c.reAlertAfter)
	if err != nil {
		c.logger.Error("sweep health check: overdue query failed", "error", err)
		return
	}

	if len(loops) == 0 {
		c.logger.Debug("sweep health check: all background loops within schedule")
		return
	}

	for _, loop := range loops {
		c.logger.Error("background sweep overdue",
			"service", loop.Name,
			"interval", loop.Interval(),
			"overdue_by", loop.Overdue(),
			"never_ran", loop.NeverRan(),
			"last_outcome", loop.OutcomeLabel(),
			"consecutive_failures", loop.ConsecutiveFailures,
			"run_count", loop.RunCount,
		)
		invokeOverdueHandler(loop)
	}
}
