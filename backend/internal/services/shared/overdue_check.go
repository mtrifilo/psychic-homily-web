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

	// overdueRecoveryJitter is scheduling slop on top of the catch-up stagger.
	overdueRecoveryJitter = 15 * time.Minute

	// retireAfter is how long a row survives without any process re-registering
	// it before it stops being treated as something that should be running.
	//
	// Sized against the HEALTH-CHECK cadence, not deploy cadence. A live process
	// re-stamps every loop it owns on each pass (see refreshRegistrations), so a
	// wired loop's registration is at most one pass old no matter how long the
	// process has been up — 15 minutes against a 7-day window, four orders of
	// magnitude of headroom.
	//
	// Sizing this against deploys instead was a real bug, caught in review: with
	// registration written only at boot, a process that stayed up longer than this
	// window aged out every row and silently switched off alerting for the entire
	// fleet — PSY-1606's own shape, reproduced by the mechanism meant to reduce
	// noise. If you ever make registration depend on deploys again, this constant
	// becomes a time bomb.
	//
	// What the window now measures is how long a row outlives the last process
	// that wired it: long enough that a loop accidentally dropped from the wiring
	// still alerts for days first, short enough that a stage-to-prod restore stops
	// paging within a week rather than forever.
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
// It reports whether the handler ran to completion. That return value is what
// makes the claim recoverable: the stamp is committed BEFORE the handler runs
// (it has to be, or two replicas would both report), so a handler that dies
// leaves a row claiming an alert nobody received. Telling the caller lets it put
// the claim back rather than sit silent for the whole re-assert window.
func invokeOverdueHandler(loop OverdueLoop) (delivered bool) {
	overdueHandlerMu.RLock()
	h := overdueHandler
	overdueHandlerMu.RUnlock()
	if h == nil {
		// No sink installed (local dev, tests). The log line in runCycle is the
		// delivery, so the claim stands rather than retrying every pass forever.
		return true
	}
	defer func() {
		if r := recover(); r != nil {
			delivered = false
			slog.Default().Error("overdue handler itself panicked",
				"service", loop.Name,
				"panic", r,
			)
		}
	}()
	h(loop)
	return true
}

// OverdueClaimer reports loops that have stopped running, claiming each report so
// it is made once.
//
// Split from RunStore rather than added to it: RunStore is the SCHEDULING
// contract that every loop depends on, and widening it would force a health query
// onto in-memory doubles that have no interest in one. Only the health check
// needs this.
// LoopRegistrar re-asserts that the loops this process owns are still expected to
// run. Split from RunStore because the health check needs only this one method,
// and from OverdueClaimer because they answer different questions.
type LoopRegistrar interface {
	Register(ctx context.Context, name string, interval, lease time.Duration) error
}

type OverdueClaimer interface {
	// ClaimOverdueAlerts atomically finds every loop that is overdue and not
	// currently throttled, marks each as reported, and returns them. Claiming and
	// reporting in one statement is what makes the once-per-occurrence guarantee
	// hold across replicas.
	ClaimOverdueAlerts(ctx context.Context, reAlertAfter time.Duration) ([]OverdueLoop, error)

	// ReleaseOverdueAlert undoes a claim whose report was never delivered, so the
	// next pass reports it again rather than leaving a row that asserts an alert
	// nobody received.
	ReleaseOverdueAlert(ctx context.Context, name string) error
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
		overdueRecoveryMargin().Seconds(),
		reAlertAfter.Seconds(),
	).Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("claim overdue alerts: %w", err)
	}
	return rows, nil
}

// ReleaseOverdueAlert implements OverdueClaimer.
//
// Guarded on the stamp still being the one this pass wrote is deliberately NOT
// done: if another replica has since re-claimed and genuinely reported, clearing
// costs at most one duplicate alert, whereas leaving a stale stamp costs a missed
// one. Duplicates are noticed; silence is not.
func (s *GormRunStore) ReleaseOverdueAlert(ctx context.Context, name string) error {
	err := s.db.WithContext(ctx).Exec(`
		UPDATE background_service_runs
		SET last_overdue_alert_at = NULL, updated_at = NOW()
		WHERE name = ?
	`, name).Error
	if err != nil {
		return fmt.Errorf("release overdue alert for %q: %w", name, err)
	}
	return nil
}

// SweepHealthCheck reports background loops that have stopped running.
//
// It rides RunScheduledLoop like everything else it watches, with two properties
// that keep the bug class it detects from silencing it: a short interval, which
// the PSY-1606 audit showed empirically is never starved (the ten loops that ran
// continuously were exactly the ones that ran often), and RunAtBoot, which makes
// every deploy re-run the check rather than reset a timer toward it.
type SweepHealthCheck struct {
	store OverdueClaimer
	// registrar re-stamps this process's loops each pass. Without it,
	// registration recency would decay with process UPTIME rather than with
	// whether anything still wires the loop up, and a process that simply stayed
	// alive past the retirement window would silence the entire fleet.
	registrar    LoopRegistrar
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
		registrar:    store,
		interval:     healthCheckInterval(),
		reAlertAfter: overdueReAlertAfter(),
		stopCh:       make(chan struct{}),
		logger:       slog.Default(),
	}
}

// overdueRecoveryMargin is how much slack the overdue threshold allows on top of
// interval + lease.
//
// DERIVED from catchUpMaxDelay rather than hardcoded, because that delay is the
// real quantity it must cover and it is independently env-tunable
// (SWEEP_CATCHUP_MAX_SECONDS). A boot catch-up hands successive overdue loops
// successive slots — 60s + n*90s, capped — so a loop recovering perfectly
// normally can legitimately wait out that whole cap before its cycle completes.
// A fixed margin smaller than the cap would report that loop as stalled while it
// was doing exactly what it was told, which is the false positive the floor
// exists to prevent. An operator widening the stagger to spread third-party load
// would otherwise silently eat into this margin from another file.
func overdueRecoveryMargin() time.Duration {
	return catchUpMaxDelay + overdueRecoveryJitter
}

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
	// Both ends. A non-positive window would invert the SQL predicate into
	// always-true and re-alert every pass — the flood, produced by the one action
	// an operator takes to reduce noise. EnvPositiveDuration now rejects the
	// overflow that could produce this, but the guard is kept because the cost of
	// being wrong here is a Sentry storm during an incident.
	if window <= 0 {
		slog.Default().Warn("SWEEP_OVERDUE_REALERT_HOURS resolved to a non-positive window — using default",
			"resolved", window, "using", defaultOverdueReAlertAfter)
		return defaultOverdueReAlertAfter
	}
	if window > maxOverdueReAlertAfter {
		slog.Default().Warn("SWEEP_OVERDUE_REALERT_HOURS is long enough to read as report-once — clamping so a dead sweep keeps re-asserting",
			"configured", window,
			"using", maxOverdueReAlertAfter,
		)
		return maxOverdueReAlertAfter
	}
	return window
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
func healthCheckInterval() time.Duration {
	interval := EnvPositiveDuration("SWEEP_HEALTH_CHECK_INTERVAL_MINUTES", time.Minute, defaultHealthCheckInterval)
	// A non-positive interval slips past the >= guard below and is then rewritten
	// to one hour by newLoopRunner — landing exactly ON the persistence threshold,
	// which hands the checker a run-state row and puts it in the set it scans.
	if interval <= 0 {
		slog.Default().Warn("SWEEP_HEALTH_CHECK_INTERVAL_MINUTES resolved to a non-positive interval — using default",
			"resolved", interval, "using", defaultHealthCheckInterval)
		return defaultHealthCheckInterval
	}
	if interval >= runStatePersistenceThreshold {
		slog.Default().Warn("SWEEP_HEALTH_CHECK_INTERVAL_MINUTES is at or above the run-state persistence threshold — clamping so the health check cannot monitor itself",
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

// refreshRegistrations re-asserts every loop this process wired up, so a row's
// registration recency tracks "something still runs this" rather than "a deploy
// happened recently".
//
// Runs BEFORE the overdue query in the same pass, so a long-lived process can
// never age its own loops out from under the retirement gate. Best-effort per
// loop: a failure to re-stamp one must not stop the others being re-stamped or
// the overdue check running, since the consequence of skipping is at worst a
// delayed alert, while aborting the pass would be a missed one.
func (c *SweepHealthCheck) refreshRegistrations(ctx context.Context) {
	if c.registrar == nil {
		return
	}
	for _, loop := range RegisteredLoops() {
		if err := c.registrar.Register(ctx, loop.Name, loop.Interval, loop.Lease); err != nil {
			c.logger.Error("sweep health check: could not refresh loop registration",
				"service", loop.Name,
				"error", err,
			)
		}
	}
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
	c.refreshRegistrations(ctx)

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
		if delivered := invokeOverdueHandler(loop); !delivered {
			// Put the claim back. Leaving it stamped would mean the row asserts a
			// report that never reached anyone, and the loop then stays silent for
			// the whole re-assert window — a silent failure inside the thing built
			// to catch silent failures.
			if err := c.store.ReleaseOverdueAlert(ctx, loop.Name); err != nil {
				c.logger.Error("sweep health check: could not release an undelivered alert claim",
					"service", loop.Name,
					"error", err,
				)
			}
		}
	}
}
