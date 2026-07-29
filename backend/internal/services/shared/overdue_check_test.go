package shared

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeClaimer stands in for the store so the health check's own behaviour — what
// it reports, and what it does when the store misbehaves — can be tested without
// a container.
type fakeClaimer struct {
	mu       sync.Mutex
	loops    []OverdueLoop
	err      error
	window   time.Duration
	released []string
}

// ReleaseOverdueAlert records which claims were handed back because their report
// never reached anyone.
func (f *fakeClaimer) ReleaseOverdueAlert(_ context.Context, name string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.released = append(f.released, name)
	return nil
}

func (f *fakeClaimer) ClaimOverdueAlerts(_ context.Context, reAlertAfter time.Duration) ([]OverdueLoop, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.window = reAlertAfter
	if f.err != nil {
		return nil, f.err
	}
	return f.loops, nil
}

// newTestHealthCheck wires the registrar only when the claimer can register, so
// unit tests can pass a claim-only fake while integration tests get the real
// refresh path.
func newTestHealthCheck(claimer OverdueClaimer) *SweepHealthCheck {
	c := &SweepHealthCheck{
		store:        claimer,
		interval:     defaultHealthCheckInterval,
		reAlertAfter: defaultOverdueReAlertAfter,
		stopCh:       make(chan struct{}),
		logger:       slog.Default(),
	}
	if r, ok := claimer.(LoopRegistrar); ok {
		c.registrar = r
	}
	return c
}

// TestUndeliveredAlertIsReleasedForRetry: the throttle stamp is committed BEFORE
// the handler runs, because two replicas must not both report one occurrence. The
// cost of that ordering is that a handler which dies leaves a row asserting an
// alert nobody received — silence for the whole re-assert window, produced inside
// the machinery built to prevent silence. The claim must be handed back.
func TestUndeliveredAlertIsReleasedForRetry(t *testing.T) {
	withCapturedSlog(t)

	SetOverdueHandler(func(OverdueLoop) { panic("sentry exploded") })
	t.Cleanup(func() { SetOverdueHandler(nil) })

	claimer := &fakeClaimer{loops: []OverdueLoop{
		{Name: "undelivered", IntervalSeconds: 3600, OverdueSeconds: 99999},
		{Name: "also-undelivered", IntervalSeconds: 3600, OverdueSeconds: 99999},
	}}
	newTestHealthCheck(claimer).RunCheckNow(context.Background())

	claimer.mu.Lock()
	defer claimer.mu.Unlock()
	assert.ElementsMatch(t, []string{"undelivered", "also-undelivered"}, claimer.released,
		"a report that never reached its sink must release the claim so the next pass retries")
}

// TestDeliveredAlertIsNotReleased: the retry path must not undo a successful
// report, or the throttle stops throttling and every pass re-alerts.
func TestDeliveredAlertIsNotReleased(t *testing.T) {
	SetOverdueHandler(func(OverdueLoop) {})
	t.Cleanup(func() { SetOverdueHandler(nil) })

	claimer := &fakeClaimer{loops: []OverdueLoop{
		{Name: "delivered", IntervalSeconds: 3600, OverdueSeconds: 99999},
	}}
	newTestHealthCheck(claimer).RunCheckNow(context.Background())

	claimer.mu.Lock()
	defer claimer.mu.Unlock()
	assert.Empty(t, claimer.released, "a delivered alert must stay claimed")
}

// TestEnvDurationRejectsOverflow guards a trap that fails toward the WORST
// outcome. time.Duration is int64 nanoseconds, so a large hour count wraps
// negative; a bare positivity check passes it through, and a negative duration
// inverts every predicate it is bound into — including the SQL that throttles
// alerting, which would then re-alert on every pass. The realistic input is an
// operator typing a huge number to mean "effectively never".
func TestEnvDurationRejectsOverflow(t *testing.T) {
	logs := withCapturedSlog(t)

	t.Setenv("PSY_TEST_OVERFLOW_HOURS", "99999999")
	got := EnvPositiveDuration("PSY_TEST_OVERFLOW_HOURS", time.Hour, 24*time.Hour)

	assert.Positive(t, got, "an unrepresentable value must never yield a negative duration")
	assert.Equal(t, 24*time.Hour, got, "it must fall back to the default")
	assert.Contains(t, logs.String(), "too large to represent")

	// And the clamps downstream must also refuse a non-positive result.
	t.Setenv("SWEEP_OVERDUE_REALERT_HOURS", "99999999")
	assert.Positive(t, overdueReAlertAfter())
	t.Setenv("SWEEP_HEALTH_CHECK_INTERVAL_MINUTES", "99999999999")
	assert.Positive(t, healthCheckInterval())
	assert.Less(t, healthCheckInterval(), runStatePersistenceThreshold)
}

func strptr(s string) *string        { return &s }
func ptrTime(t time.Time) *time.Time { return &t }

// TestHealthCheckerCannotMonitorItself pins the answer to "what watches the
// watcher?". The checker rides the same scheduling mechanism it audits, so the one
// property that keeps the bug class from silencing it is running OFTEN — short
// intervals were the empirical control group in PSY-1606 that never starved.
//
// Raising the interval past the persistence threshold would also give the checker
// a row of its own, at which point it starts reporting on itself. This test fails
// the moment someone makes that change.
func TestHealthCheckerCannotMonitorItself(t *testing.T) {
	assert.Less(t, defaultHealthCheckInterval, runStatePersistenceThreshold,
		"the health check must stay below the persistence threshold: at or above it, "+
			"it takes a run-state row and begins monitoring itself")
	assert.LessOrEqual(t, defaultHealthCheckInterval, 15*time.Minute,
		"short intervals are the property that makes the checker immune to deploy starvation")
}

// TestHealthCheckIntervalIsClampedBelowPersistence closes the hole the constant
// alone leaves open: the effective cadence comes from an env var, so asserting on
// the const proves nothing about a deployed process. An operator setting 60
// minutes would push the checker over runStatePersistenceThreshold, hand it a
// run-state row, and put it in the candidate set it scans — at which point a
// checker that stopped running is the thing responsible for noticing.
func TestHealthCheckIntervalIsClampedBelowPersistence(t *testing.T) {
	logs := withCapturedSlog(t)

	t.Setenv("SWEEP_HEALTH_CHECK_INTERVAL_MINUTES", "60")
	got := healthCheckInterval()

	assert.Less(t, got, runStatePersistenceThreshold,
		"an over-threshold interval must be clamped, not honoured")
	assert.Equal(t, defaultHealthCheckInterval, got)
	assert.Contains(t, logs.String(), "clamping",
		"silently ignoring an operator's setting is worse than saying so")
}

// TestHealthCheckIntervalHonoursUsableOverrides: the clamp must not swallow the
// legitimate reason the knob exists — dropping the cadence during an incident.
func TestHealthCheckIntervalHonoursUsableOverrides(t *testing.T) {
	t.Setenv("SWEEP_HEALTH_CHECK_INTERVAL_MINUTES", "2")
	assert.Equal(t, 2*time.Minute, healthCheckInterval())
}

// TestReAlertWindowCannotBecomeReportOnce: overshooting this knob fails toward
// SILENCE, which is indistinguishable from health — the same shape as the incident
// this whole subsystem exists to catch.
func TestReAlertWindowCannotBecomeReportOnce(t *testing.T) {
	logs := withCapturedSlog(t)

	t.Setenv("SWEEP_OVERDUE_REALERT_HOURS", "8760") // a year
	assert.Equal(t, maxOverdueReAlertAfter, overdueReAlertAfter())
	assert.Contains(t, logs.String(), "clamping")

	t.Setenv("SWEEP_OVERDUE_REALERT_HOURS", "48") // a usable widening survives
	assert.Equal(t, 48*time.Hour, overdueReAlertAfter())
}

// TestOverdueReportsNameTheActionableFacts covers the acceptance criterion that
// the alert names the sweep, its interval, and how long it has actually been. An
// alert that says only "something is wrong" costs an operator the same
// investigation the alert was supposed to save.
func TestOverdueReportsNameTheActionableFacts(t *testing.T) {
	stalled := OverdueLoop{
		Name:            "artist_image_sweep",
		IntervalSeconds: 3600,
		OverdueSeconds:  18000, // 5h
		LastCompletedAt: ptrTime(time.Now().Add(-5 * time.Hour)),
		LastOutcome:     strptr(RunOutcomeSuccess),
	}
	summary := stalled.Summary()
	assert.Contains(t, summary, "artist_image_sweep", "the alert must name the sweep")
	assert.Contains(t, summary, "1h0m0s", "the alert must name the configured interval")
	assert.Contains(t, summary, "5h0m0s", "the alert must name how long it has actually been")
	assert.NotContains(t, summary, "NEVER", "a stalled sweep must not read as never-run")

	neverRan := OverdueLoop{
		Name:            "radio_janitor",
		IntervalSeconds: 86400,
		OverdueSeconds:  259200, // 3d
	}
	assert.True(t, neverRan.NeverRan())
	assert.Equal(t, FailureModeStalled, stalled.FailureMode())
	assert.Equal(t, FailureModeNeverRan, neverRan.FailureMode(),
		"failure mode feeds the Sentry fingerprint; the two causes must group separately "+
			"so an ongoing outage is not buried under a newly-broken deploy")
	assert.Contains(t, neverRan.Summary(), "NEVER",
		"a never-run sweep points at a wiring problem, not a stuck job — the two must not read alike")
	assert.Equal(t, "none", neverRan.OutcomeLabel(),
		"a loop with no outcome must say so rather than report an empty string")
}

// TestOverdueHandlerReceivesEveryOverdueLoop: one report per loop, not one per
// batch. A single "3 sweeps are down" event collapses in Sentry into one issue
// whose title stops matching once the set changes.
func TestOverdueHandlerReceivesEveryOverdueLoop(t *testing.T) {
	var (
		mu   sync.Mutex
		seen []string
	)
	SetOverdueHandler(func(loop OverdueLoop) {
		mu.Lock()
		defer mu.Unlock()
		seen = append(seen, loop.Name)
	})
	t.Cleanup(func() { SetOverdueHandler(nil) })

	claimer := &fakeClaimer{loops: []OverdueLoop{
		{Name: "sweep_a", IntervalSeconds: 3600, OverdueSeconds: 7300},
		{Name: "sweep_b", IntervalSeconds: 3600, OverdueSeconds: 7300},
		{Name: "sweep_c", IntervalSeconds: 3600, OverdueSeconds: 7300},
	}}
	newTestHealthCheck(claimer).RunCheckNow(context.Background())

	assert.ElementsMatch(t, []string{"sweep_a", "sweep_b", "sweep_c"}, seen)
	assert.Equal(t, defaultOverdueReAlertAfter, claimer.window,
		"the configured re-alert window must reach the store, or the throttle is whatever the SQL defaults to")
}

// TestHealthyPassInvokesNoHandler is the no-noise criterion at the handler
// boundary: a healthy system must not merely send fewer events, it must send none.
func TestHealthyPassInvokesNoHandler(t *testing.T) {
	var called int
	SetOverdueHandler(func(OverdueLoop) { called++ })
	t.Cleanup(func() { SetOverdueHandler(nil) })

	newTestHealthCheck(&fakeClaimer{}).RunCheckNow(context.Background())

	assert.Zero(t, called, "a healthy pass must produce no alert at all")
}

// TestPanickingHandlerCannotDisturbTheCheck is the acceptance criterion that a
// failure inside the health check cannot disturb the sweeps. The whole subsystem
// exists because a background failure went unnoticed — monitoring that can itself
// take down a loop would leave the system worse off than with no monitoring.
func TestPanickingHandlerCannotDisturbTheCheck(t *testing.T) {
	logs := withCapturedSlog(t)

	var reached []string
	SetOverdueHandler(func(loop OverdueLoop) {
		reached = append(reached, loop.Name)
		panic("sentry exploded")
	})
	t.Cleanup(func() { SetOverdueHandler(nil) })

	claimer := &fakeClaimer{loops: []OverdueLoop{
		{Name: "first", IntervalSeconds: 3600, OverdueSeconds: 7300},
		{Name: "second", IntervalSeconds: 3600, OverdueSeconds: 7300},
	}}

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("a panicking handler must not escape the health check; got: %v", r)
		}
	}()
	newTestHealthCheck(claimer).RunCheckNow(context.Background())

	assert.Equal(t, []string{"first", "second"}, reached,
		"a handler that panics on the first loop must not prevent the second from being reported")
	assert.Contains(t, logs.String(), "overdue handler itself panicked")
}

// TestStoreFailureIsContained: the checker observes, it does not repair. A
// database blip must leave the sweeps exactly as it found them and wait for the
// next pass rather than escalating.
func TestStoreFailureIsContained(t *testing.T) {
	logs := withCapturedSlog(t)

	var called int
	SetOverdueHandler(func(OverdueLoop) { called++ })
	t.Cleanup(func() { SetOverdueHandler(nil) })

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("a store failure must not panic the check; got: %v", r)
		}
	}()
	newTestHealthCheck(&fakeClaimer{err: errors.New("connection refused")}).RunCheckNow(context.Background())

	assert.Zero(t, called, "a failed query must not be reported as an overdue sweep")
	assert.Contains(t, logs.String(), "sweep health check: overdue query failed")
}

// TestNilHandlerIsSafe covers local development and tests, where no Sentry handler
// is installed. The check must still run and log rather than nil-panic.
func TestNilHandlerIsSafe(t *testing.T) {
	SetOverdueHandler(nil)
	logs := withCapturedSlog(t)

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("no handler installed must be a supported mode; got: %v", r)
		}
	}()
	newTestHealthCheck(&fakeClaimer{loops: []OverdueLoop{
		{Name: "unwatched", IntervalSeconds: 3600, OverdueSeconds: 7300},
	}}).RunCheckNow(context.Background())

	assert.Contains(t, logs.String(), "background sweep overdue",
		"even with no alert sink, an overdue sweep must be logged")
}

// TestLoopRegistersItselfBeforeScheduling: registration must happen at loop START,
// not at first cycle. A loop whose first cycle is a day away would otherwise be
// invisible for that whole day, which is exactly when a wiring failure most needs
// to be visible.
func TestLoopRegistersItselfBeforeScheduling(t *testing.T) {
	store := newMemRunStore()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	RunScheduledLoop(ctx, LoopConfig{
		Name:       "announces-itself",
		Interval:   time.Hour,
		StartDelay: time.Hour, // first cycle far beyond this test's lifetime
		Store:      store,
	}, func(context.Context) { t.Error("no cycle should run in this window") })

	require.Equal(t, int32(1), store.registered.Load(),
		"a loop must announce itself at start, before any cycle")
	assert.Equal(t, int64(3600), store.snapshot("announces-itself").intervalSeconds,
		"registration must record the cadence the overdue threshold is computed from")
}

// TestRegistrationFailureDoesNotStopTheLoop: losing the ability to OBSERVE a sweep
// must never cost the sweep itself. Refusing to run because the state table is
// unreachable would convert a monitoring outage into a data outage.
func TestRegistrationFailureDoesNotStopTheLoop(t *testing.T) {
	logs := withCapturedSlog(t)

	store := newMemRunStore()
	store.failRegister = errors.New("table missing")

	var ran int32
	var mu sync.Mutex
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Millisecond)
	defer cancel()
	RunScheduledLoop(ctx, LoopConfig{
		Name:      "unregisterable",
		Interval:  time.Hour,
		RunAtBoot: true,
		Store:     store,
	}, func(context.Context) {
		mu.Lock()
		ran++
		mu.Unlock()
	})

	mu.Lock()
	defer mu.Unlock()
	assert.Positive(t, ran, "a loop must still run when registration fails")
	assert.Contains(t, logs.String(), "register")
	assert.True(t, strings.Contains(logs.String(), "unregisterable"),
		"the failure must name the service so it can be diagnosed")
}

// TestRegistrationPanicDoesNotStopTheLoop is the harder version. RunScheduledLoop's
// outer recover deliberately STOPS the loop on a scheduling panic — correct there,
// catastrophic here, because registration is pure observability and this code path
// runs for all twenty-odd sweeps.
func TestRegistrationPanicDoesNotStopTheLoop(t *testing.T) {
	logs := withCapturedSlog(t)

	var ran int32
	var mu sync.Mutex
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Millisecond)
	defer cancel()
	RunScheduledLoop(ctx, LoopConfig{
		Name:      "panic-on-register",
		Interval:  time.Hour,
		RunAtBoot: true,
		Store:     panicOnRegisterStore{},
	}, func(context.Context) {
		mu.Lock()
		ran++
		mu.Unlock()
	})

	mu.Lock()
	defer mu.Unlock()
	assert.Positive(t, ran,
		"a panic in the monitoring write must not take down the sweep it was added to watch")
	assert.Contains(t, logs.String(), "register panicked")
	assert.NotContains(t, logs.String(), "background service panic — service stopping",
		"a registration panic must be contained locally, not escalated to the loop-stopping recover")
}

// panicOnRegisterStore panics only on Register; everything else behaves, so the
// test isolates the registration path.
type panicOnRegisterStore struct{}

func (panicOnRegisterStore) Register(context.Context, string, time.Duration, time.Duration) error {
	panic("register exploded")
}

func (panicOnRegisterStore) Retire(context.Context, string) error { return nil }

func (panicOnRegisterStore) DueIn(context.Context, string, time.Duration) (time.Duration, error) {
	return 0, nil
}

func (panicOnRegisterStore) Claim(context.Context, string, time.Duration, time.Duration, bool) (time.Time, bool, error) {
	return time.Now(), true, nil
}

func (panicOnRegisterStore) Complete(context.Context, string, time.Time, CycleOutcome) error {
	return nil
}
