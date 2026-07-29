package shared

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"gorm.io/gorm"
)

// The throttle is the whole difference between a useful alert and one everyone
// mutes, and it lives entirely in one SQL statement — the claim predicate plus
// the RETURNING. An in-memory double could not prove any of it: the
// once-per-occurrence guarantee depends on PostgreSQL locking each candidate row
// before re-evaluating the predicate, and every time comparison is made against
// the database clock precisely so a skewed app clock cannot manufacture or mask
// an alert. So these run the real statement against real Postgres.
//
// Durations are compressed but the RATIOS are production's: an interval is long
// relative to the window a test can observe, exactly as 24h is long relative to a
// deploy.

const testAlertWindow = 24 * time.Hour

// registerLoop announces a loop the way RunScheduledLoop does at startup.
func registerLoop(t *testing.T, store *GormRunStore, name string, interval time.Duration) {
	t.Helper()
	if err := store.Register(context.Background(), name, interval); err != nil {
		t.Fatalf("register %s: %v", name, err)
	}
}

// backdateColumn ages one timestamp column on the DATABASE clock, so no test ever
// depends on the test process and Postgres agreeing about what time it is — the
// same reason the store itself never accepts a caller-supplied timestamp.
//
// The column name is interpolated rather than bound because identifiers cannot be
// parameters in SQL; every caller passes a compile-time constant from this file.
func backdateColumn(t *testing.T, db *gorm.DB, name, column string, ago time.Duration) {
	t.Helper()
	err := db.Exec(
		`UPDATE background_service_runs SET `+column+` = NOW() - make_interval(secs => ?) WHERE name = ?`,
		ago.Seconds(), name,
	).Error
	if err != nil {
		t.Fatalf("backdate %s.%s: %v", name, column, err)
	}
}

// backdateCreation makes a never-run loop overdue: there is no completion to move,
// so its grace window is measured from when it announced itself.
func backdateCreation(t *testing.T, db *gorm.DB, name string, ago time.Duration) {
	t.Helper()
	backdateColumn(t, db, name, "created_at", ago)
}

// backdateAlert ages the throttle stamp, simulating the passage of the re-alert
// window without a test that sleeps for it.
func backdateAlert(t *testing.T, db *gorm.DB, name string, ago time.Duration) {
	t.Helper()
	backdateColumn(t, db, name, "last_overdue_alert_at", ago)
}

func alertStamp(t *testing.T, db *gorm.DB, name string) *time.Time {
	t.Helper()
	var got BackgroundServiceRun
	if err := db.Table("background_service_runs").Where("name = ?", name).Take(&got).Error; err != nil {
		t.Fatalf("read back %s: %v", name, err)
	}
	return got.LastOverdueAlertAt
}

// findLoop locates a named loop in a report batch. The table is shared between
// cases within a run, so a test must assert on ITS loop rather than on the batch
// size.
func findLoop(loops []OverdueLoop, name string) (OverdueLoop, bool) {
	for _, l := range loops {
		if l.Name == name {
			return l, true
		}
	}
	return OverdueLoop{}, false
}

// TestClaimOverdueAlerts_HealthyLoopIsSilent is the acceptance criterion that
// matters most for the alert surviving contact with humans: a working system must
// produce NO recurring noise. An alerter that cries wolf gets muted, and a muted
// alert is worse than none because it still looks like coverage.
func TestClaimOverdueAlerts_HealthyLoopIsSilent(t *testing.T) {
	db, store := setupRunStore(t)
	ctx := context.Background()

	registerLoop(t, store, "healthy", time.Hour)
	backdateCompletion(t, db, "healthy", 10*time.Minute) // well inside 2x interval

	loops, err := store.ClaimOverdueAlerts(ctx, testAlertWindow)
	if err != nil {
		t.Fatalf("ClaimOverdueAlerts: %v", err)
	}
	if _, found := findLoop(loops, "healthy"); found {
		t.Fatal("a loop that completed 10m ago on an hourly cadence must not alert")
	}
	if stamp := alertStamp(t, db, "healthy"); stamp != nil {
		t.Fatalf("a healthy loop must not be stamped as alerted, got %v", stamp)
	}
}

// TestClaimOverdueAlerts_ThresholdBoundary pins the 2x rule from both sides.
// Without the negative case a threshold bug that alerted at 1x would still pass a
// test that only checked "an old loop alerts".
func TestClaimOverdueAlerts_ThresholdBoundary(t *testing.T) {
	db, store := setupRunStore(t)
	ctx := context.Background()

	registerLoop(t, store, "just-inside", time.Hour)
	backdateCompletion(t, db, "just-inside", 110*time.Minute) // < 2h
	registerLoop(t, store, "just-outside", time.Hour)
	backdateCompletion(t, db, "just-outside", 125*time.Minute) // > 2h

	loops, err := store.ClaimOverdueAlerts(ctx, testAlertWindow)
	if err != nil {
		t.Fatalf("ClaimOverdueAlerts: %v", err)
	}
	if _, found := findLoop(loops, "just-inside"); found {
		t.Fatal("a loop inside 2x its interval must not alert — one slow cycle must not flap")
	}
	if _, found := findLoop(loops, "just-outside"); !found {
		t.Fatal("a loop past 2x its interval must alert")
	}
}

// TestClaimOverdueAlerts_ReportsOnceThenThrottles is the core of the decision: an
// overdue sweep STAYS overdue, so the naive check re-reports on every 15m pass.
func TestClaimOverdueAlerts_ReportsOnceThenThrottles(t *testing.T) {
	db, store := setupRunStore(t)
	ctx := context.Background()

	registerLoop(t, store, "stalled", time.Hour)
	backdateCompletion(t, db, "stalled", 5*time.Hour)

	first, err := store.ClaimOverdueAlerts(ctx, testAlertWindow)
	if err != nil {
		t.Fatalf("first pass: %v", err)
	}
	loop, found := findLoop(first, "stalled")
	if !found {
		t.Fatal("a loop 5h past an hourly cadence must alert on the transition")
	}
	if loop.NeverRan() {
		t.Fatal("a loop with a completion must report as stalled, not never-run")
	}
	if elapsed := loop.Overdue(); elapsed < 4*time.Hour || elapsed > 6*time.Hour {
		t.Fatalf("overdue_by = %v, want ~5h — the alert must name how long it has actually been", elapsed)
	}
	if loop.Interval() != time.Hour {
		t.Fatalf("interval = %v, want 1h — the alert must name the configured cadence", loop.Interval())
	}

	// Three more passes, as the 15m checker would make within the window.
	for i := range 3 {
		again, err := store.ClaimOverdueAlerts(ctx, testAlertWindow)
		if err != nil {
			t.Fatalf("pass %d: %v", i+2, err)
		}
		if _, found := findLoop(again, "stalled"); found {
			t.Fatalf("pass %d re-reported inside the throttle window — this is the flood that gets alerts muted", i+2)
		}
	}
}

// TestClaimOverdueAlerts_ReAssertsAfterWindow guards the other direction. A
// report-once-and-never-again policy reproduces the PSY-1606 failure one level up:
// if that single Sentry issue is resolved or missed, the sweep is silently dead
// again with nothing left to say so.
func TestClaimOverdueAlerts_ReAssertsAfterWindow(t *testing.T) {
	db, store := setupRunStore(t)
	ctx := context.Background()

	registerLoop(t, store, "long-dead", time.Hour)
	backdateCompletion(t, db, "long-dead", 72*time.Hour)

	if _, err := store.ClaimOverdueAlerts(ctx, testAlertWindow); err != nil {
		t.Fatalf("first pass: %v", err)
	}
	backdateAlert(t, db, "long-dead", 25*time.Hour) // window elapsed

	again, err := store.ClaimOverdueAlerts(ctx, testAlertWindow)
	if err != nil {
		t.Fatalf("second pass: %v", err)
	}
	if _, found := findLoop(again, "long-dead"); !found {
		t.Fatal("a still-dead sweep must re-assert once the window elapses, or a missed alert is silence forever")
	}
}

// TestClaimOverdueAlerts_NeverRunIsDistinguishable covers the case the ticket
// asked about and PSY-1611 could not answer: rows were created by Claim, so a loop
// that never started had no row and was invisible. Registration makes it visible,
// and the two failures must stay separable — a never-run loop is usually a wiring
// or deploy problem, a stalled one was working and broke.
func TestClaimOverdueAlerts_NeverRunIsDistinguishable(t *testing.T) {
	db, store := setupRunStore(t)
	ctx := context.Background()

	registerLoop(t, store, "never-started", time.Hour)
	backdateCreation(t, db, "never-started", 5*time.Hour)

	loops, err := store.ClaimOverdueAlerts(ctx, testAlertWindow)
	if err != nil {
		t.Fatalf("ClaimOverdueAlerts: %v", err)
	}
	loop, found := findLoop(loops, "never-started")
	if !found {
		t.Fatal("a registered loop that never completed a cycle must alert")
	}
	if !loop.NeverRan() {
		t.Fatal("a loop with no completion must report as never-run, not stalled")
	}
	if loop.RunCount != 0 {
		t.Fatalf("run_count = %d, want 0", loop.RunCount)
	}
	if loop.OutcomeLabel() != "none" {
		t.Fatalf("outcome = %q; a never-run loop has no outcome and must not report an empty string", loop.OutcomeLabel())
	}
}

// TestClaimOverdueAlerts_FreshlyRegisteredLoopGetsGrace: a loop that has only just
// been deployed has no completion either, and must not be reported as dead before
// it has had a chance to run. Otherwise every deploy of a new daily sweep pages
// someone.
func TestClaimOverdueAlerts_FreshlyRegisteredLoopGetsGrace(t *testing.T) {
	db, store := setupRunStore(t)
	ctx := context.Background()

	registerLoop(t, store, "just-deployed", 24*time.Hour)

	loops, err := store.ClaimOverdueAlerts(ctx, testAlertWindow)
	if err != nil {
		t.Fatalf("ClaimOverdueAlerts: %v", err)
	}
	if _, found := findLoop(loops, "just-deployed"); found {
		t.Fatal("a loop registered moments ago must get its grace window before being called dead")
	}
	if stamp := alertStamp(t, db, "just-deployed"); stamp != nil {
		t.Fatalf("no alert means no stamp, got %v", stamp)
	}
}

// TestSuccessfulCycleClearsThrottle proves the policy is a TRANSITION and not a
// level. A loop that recovers and later stalls again must report immediately
// rather than inheriting a throttle window it was already partway through —
// otherwise a sweep can be dead for most of a day with the alert suppressed by its
// own recovery.
func TestSuccessfulCycleClearsThrottle(t *testing.T) {
	db, store := setupRunStore(t)
	ctx := context.Background()

	registerLoop(t, store, "flapper", time.Hour)
	backdateCompletion(t, db, "flapper", 5*time.Hour)

	if _, err := store.ClaimOverdueAlerts(ctx, testAlertWindow); err != nil {
		t.Fatalf("first alert: %v", err)
	}
	if alertStamp(t, db, "flapper") == nil {
		t.Fatal("an alerted loop must be stamped")
	}

	// It recovers.
	token, ok, err := store.Claim(ctx, "flapper", time.Hour, time.Hour, true)
	if err != nil || !ok {
		t.Fatalf("claim for recovery cycle: ok=%v err=%v", ok, err)
	}
	if err := store.Complete(ctx, "flapper", token, CycleOutcome{Duration: time.Second}); err != nil {
		t.Fatalf("complete recovery cycle: %v", err)
	}
	if stamp := alertStamp(t, db, "flapper"); stamp != nil {
		t.Fatalf("a successful cycle must clear the throttle stamp, got %v", stamp)
	}

	// It stalls again, well inside the 24h re-alert window.
	backdateCompletion(t, db, "flapper", 5*time.Hour)
	again, err := store.ClaimOverdueAlerts(ctx, testAlertWindow)
	if err != nil {
		t.Fatalf("post-recovery pass: %v", err)
	}
	if _, found := findLoop(again, "flapper"); !found {
		t.Fatal("a loop that recovered and stalled again must alert immediately — recovery resets the window")
	}
}

// TestFailedCycleDoesNotClearThrottle is the counterpart. A loop failing every
// cycle has NOT recovered; if a failure cleared the stamp, a crash-looping sweep
// would re-alert on every health-check pass — the exact flood the throttle exists
// to prevent.
func TestFailedCycleDoesNotClearThrottle(t *testing.T) {
	db, store := setupRunStore(t)
	ctx := context.Background()

	registerLoop(t, store, "failing", time.Hour)
	backdateCompletion(t, db, "failing", 5*time.Hour)
	if _, err := store.ClaimOverdueAlerts(ctx, testAlertWindow); err != nil {
		t.Fatalf("first alert: %v", err)
	}

	token, ok, err := store.Claim(ctx, "failing", time.Hour, time.Hour, true)
	if err != nil || !ok {
		t.Fatalf("claim: ok=%v err=%v", ok, err)
	}
	if err := store.Complete(ctx, "failing", token, CycleOutcome{
		Err:      context.DeadlineExceeded,
		Duration: time.Second,
	}); err != nil {
		t.Fatalf("complete failing cycle: %v", err)
	}
	if alertStamp(t, db, "failing") == nil {
		t.Fatal("a FAILED cycle must not clear the throttle — a failing loop has not recovered")
	}
}

// TestClaimOverdueAlerts_ConcurrentCheckersReportOnce is why claiming and
// reporting are one statement. Production runs more than one instance; a
// read-then-write pair here would alert once per replica, turning a
// once-per-occurrence policy into once-per-occurrence-per-instance.
func TestClaimOverdueAlerts_ConcurrentCheckersReportOnce(t *testing.T) {
	db, store := setupRunStore(t)
	ctx := context.Background()

	registerLoop(t, store, "contended", time.Hour)
	backdateCompletion(t, db, "contended", 5*time.Hour)

	const checkers = 4
	var (
		wg     sync.WaitGroup
		mu     sync.Mutex
		total  int
		errSet []error
	)
	start := make(chan struct{})
	for range checkers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			loops, err := store.ClaimOverdueAlerts(ctx, testAlertWindow)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				errSet = append(errSet, err)
				return
			}
			if _, found := findLoop(loops, "contended"); found {
				total++
			}
		}()
	}
	close(start)
	wg.Wait()

	if len(errSet) > 0 {
		t.Fatalf("concurrent checkers errored: %v", errSet)
	}
	if total != 1 {
		t.Fatalf("%d checkers reported the same overdue loop, want exactly 1 — the claim must be atomic", total)
	}
}

// TestRegister_PreservesRunState is the guard on the riskiest part of this change.
// Register runs on EVERY loop start, i.e. on every deploy. If it touched
// last_completed_at it would reset the schedule that PSY-1611 exists to preserve,
// re-creating the original starvation bug through the very mechanism added to
// detect it — and every test above would still pass.
func TestRegister_PreservesRunState(t *testing.T) {
	db, store := setupRunStore(t)
	ctx := context.Background()

	token, ok, err := store.Claim(ctx, "restarting", time.Hour, time.Hour, false)
	if err != nil || !ok {
		t.Fatalf("claim: ok=%v err=%v", ok, err)
	}
	rows := 7
	if err := store.Complete(ctx, "restarting", token, CycleOutcome{Rows: &rows, Duration: time.Second}); err != nil {
		t.Fatalf("complete: %v", err)
	}

	var before BackgroundServiceRun
	if err := db.Table("background_service_runs").Where("name = ?", "restarting").Take(&before).Error; err != nil {
		t.Fatalf("read before: %v", err)
	}

	// The process restarts, with the interval reconfigured.
	registerLoop(t, store, "restarting", 6*time.Hour)

	var after BackgroundServiceRun
	if err := db.Table("background_service_runs").Where("name = ?", "restarting").Take(&after).Error; err != nil {
		t.Fatalf("read after: %v", err)
	}

	if after.LastCompletedAt == nil || !after.LastCompletedAt.Equal(*before.LastCompletedAt) {
		t.Fatal("register must not move last_completed_at — a redeploy would reset every schedule")
	}
	if after.LastSuccessAt == nil || !after.LastSuccessAt.Equal(*before.LastSuccessAt) {
		t.Fatal("register must not move last_success_at")
	}
	if after.RunCount != before.RunCount {
		t.Fatalf("run_count %d -> %d; register must not touch the run trace", before.RunCount, after.RunCount)
	}
	if after.LastRowsProcessed == nil || *after.LastRowsProcessed != 7 {
		t.Fatal("register must not clear the last cycle's row count")
	}
	if !after.CreatedAt.Equal(before.CreatedAt) {
		t.Fatal("register must not move created_at — it is the grace anchor for a never-run loop")
	}
	if after.IntervalSeconds != int64((6 * time.Hour).Seconds()) {
		t.Fatalf("interval_seconds = %d, want 21600 — the overdue threshold must track current config", after.IntervalSeconds)
	}
}

// TestStalledSweepEndToEnd is the acceptance criterion that the behaviour is
// "verified by simulating a stalled sweep, not only by unit test".
//
// Every other test here exercises one link of the chain. This one wires the real
// ones together — a real loop registering itself through RunScheduledLoop, a real
// GormRunStore, the real SweepHealthCheck on its boot cycle, and a real installed
// handler — and asserts on what an operator would actually receive. The links can
// each be correct while the assembly is not: PSY-1606 was a wiring-shaped bug, and
// PSY-1538 shipped a defect that was invisible locally because a fixture stood in
// for the real thing.
func TestStalledSweepEndToEnd(t *testing.T) {
	db, store := setupRunStore(t)

	// A sweep runs, completes, and is then starved — exactly PSY-1606's shape.
	SetDefaultRunStore(store)
	t.Cleanup(func() { SetDefaultRunStore(nil) })

	loopCtx, loopCancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer loopCancel()
	RunScheduledLoop(loopCtx, LoopConfig{
		Name:       "starved_sweep",
		Interval:   24 * time.Hour,
		StartDelay: 24 * time.Hour, // never reaches a cycle in this window
	}, func(context.Context) {})

	// It registered itself at start even though it never ran a cycle — this is what
	// PSY-1611 alone could not see.
	var registered int64
	if err := db.Raw(`SELECT COUNT(*) FROM background_service_runs WHERE name = ?`, "starved_sweep").
		Scan(&registered).Error; err != nil {
		t.Fatalf("count registration: %v", err)
	}
	if registered != 1 {
		t.Fatal("a loop must register at start, or a never-started sweep stays invisible")
	}

	backdateCompletion(t, db, "starved_sweep", 72*time.Hour)

	var (
		mu       sync.Mutex
		received []OverdueLoop
	)
	SetOverdueHandler(func(loop OverdueLoop) {
		mu.Lock()
		defer mu.Unlock()
		received = append(received, loop)
	})
	t.Cleanup(func() { SetOverdueHandler(nil) })

	check := newTestHealthCheck(store)

	// The boot cycle, as a deploy would trigger it.
	checkCtx, checkCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer checkCancel()
	check.Start(checkCtx)
	t.Cleanup(check.Stop)

	deadline := time.Now().Add(2 * time.Second)
	for {
		mu.Lock()
		_, found := findLoop(received, "starved_sweep")
		mu.Unlock()
		if found {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the health check's boot cycle must report a starved sweep — this is the whole feature")
		}
		time.Sleep(10 * time.Millisecond)
	}

	mu.Lock()
	got, _ := findLoop(received, "starved_sweep")
	count := len(received)
	mu.Unlock()

	if got.Interval() != 24*time.Hour {
		t.Fatalf("reported interval = %v, want 24h", got.Interval())
	}
	if got.Overdue() < 71*time.Hour {
		t.Fatalf("reported overdue_by = %v, want ~72h", got.Overdue())
	}
	if !strings.Contains(got.Summary(), "starved_sweep") {
		t.Fatalf("summary must name the sweep, got %q", got.Summary())
	}

	// And the throttle holds against a second pass, so a redeploy loop cannot flood.
	check.RunCheckNow(context.Background())
	mu.Lock()
	after := len(received)
	mu.Unlock()
	if after != count {
		t.Fatalf("a second pass re-reported (%d -> %d); the throttle must hold across cycles", count, after)
	}
}

// TestRegister_DoesNotSilenceAnActiveAlert: a crash-looping process re-registers
// on every restart. If that cleared the throttle stamp, the loop would re-alert on
// every restart; if it cleared the alert STATE, a fast crash loop could suppress
// its own report entirely. Neither may happen.
func TestRegister_DoesNotSilenceAnActiveAlert(t *testing.T) {
	db, store := setupRunStore(t)
	ctx := context.Background()

	registerLoop(t, store, "crash-looper", time.Hour)
	backdateCompletion(t, db, "crash-looper", 5*time.Hour)
	if _, err := store.ClaimOverdueAlerts(ctx, testAlertWindow); err != nil {
		t.Fatalf("first alert: %v", err)
	}
	stampBefore := alertStamp(t, db, "crash-looper")
	if stampBefore == nil {
		t.Fatal("precondition: loop must be alerted")
	}

	registerLoop(t, store, "crash-looper", time.Hour) // process restarts

	stampAfter := alertStamp(t, db, "crash-looper")
	if stampAfter == nil || !stampAfter.Equal(*stampBefore) {
		t.Fatal("register must not touch the throttle stamp — a crash loop would otherwise flood or silence itself")
	}
}
