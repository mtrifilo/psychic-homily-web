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

// registerLoop announces a loop the way RunScheduledLoop does at startup, with
// the production lease.
func registerLoop(t *testing.T, store *GormRunStore, name string, interval time.Duration) {
	t.Helper()
	if err := store.Register(context.Background(), name, interval, defaultRunLease); err != nil {
		t.Fatalf("register %s: %v", name, err)
	}
}

// retireRow ages a row's registration past the retirement window, simulating a
// loop nothing re-registers any more.
func retireRow(t *testing.T, db *gorm.DB, name string) {
	t.Helper()
	backdateColumn(t, db, name, "last_registered_at", retireAfter+time.Hour)
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

	// A 6h interval, where 2x (12h) dominates the interval+lease+margin floor
	// (7h15m) — so this case isolates the multiplier rule itself.
	registerLoop(t, store, "just-inside", 6*time.Hour)
	backdateCompletion(t, db, "just-inside", 11*time.Hour) // < 12h
	registerLoop(t, store, "just-outside", 6*time.Hour)
	backdateCompletion(t, db, "just-outside", 13*time.Hour) // > 12h

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

// TestThresholdFloorCoversCrashRecovery pins the case a bare 2x multiplier gets
// wrong, and it comes from ordinary platform behaviour rather than a fault.
//
// An hourly loop carries an hourly claim lease. Kill the process mid-cycle and
// the stranded claim blocks re-claiming for a full lease, so the earliest a
// healthy recovery can COMPLETE is interval + lease = 2h — the exact moment a 2x
// threshold goes true. Every SIGKILL would then be a coin-flip page for a loop
// that recovered perfectly.
//
// The floor (interval + lease + margin) is what separates "recovering normally"
// from "stopped".
func TestThresholdFloorCoversCrashRecovery(t *testing.T) {
	db, store := setupRunStore(t)
	ctx := context.Background()

	// 2h05m: past 2x the interval, but inside interval + lease + margin.
	registerLoop(t, store, "crash-recovering", time.Hour)
	backdateCompletion(t, db, "crash-recovering", 125*time.Minute)

	// Comfortably past the floor — genuinely stopped.
	registerLoop(t, store, "really-stopped", time.Hour)
	backdateCompletion(t, db, "really-stopped", 4*time.Hour)

	loops, err := store.ClaimOverdueAlerts(ctx, testAlertWindow)
	if err != nil {
		t.Fatalf("ClaimOverdueAlerts: %v", err)
	}
	if _, found := findLoop(loops, "crash-recovering"); found {
		t.Fatal("a loop still inside interval+lease+margin is recovering from a crash, not stalled — " +
			"alerting here turns every SIGKILL into a false page")
	}
	if _, found := findLoop(loops, "really-stopped"); !found {
		t.Fatal("the floor must not swallow a genuinely stopped loop")
	}
}

// TestRetiredLoopStopsAlerting covers the operational failure that would
// otherwise make this feature self-defeating.
//
// A row whose loop no longer runs — renamed, switched off, or carried in by
// `psy-deploy-prod --with-db-restore`, which copies stage (where extra sweeps are
// enabled) into prod — is indistinguishable from a stalled sweep and would alert
// every single day forever, with nothing in the application able to clear it.
// That is precisely the fatigue that gets alerting muted, at which point it looks
// like coverage while providing none.
//
// A loop that still exists re-registers on every boot, so recency of REGISTRATION
// separates "should be running and isn't" from "nothing expects this any more".
func TestRetiredLoopStopsAlerting(t *testing.T) {
	db, store := setupRunStore(t)
	ctx := context.Background()

	registerLoop(t, store, "retired", time.Hour)
	backdateCompletion(t, db, "retired", 30*24*time.Hour)
	registerLoop(t, store, "still-wired", time.Hour)
	backdateCompletion(t, db, "still-wired", 30*24*time.Hour)

	// Only the first stops being re-registered.
	retireRow(t, db, "retired")

	loops, err := store.ClaimOverdueAlerts(ctx, testAlertWindow)
	if err != nil {
		t.Fatalf("ClaimOverdueAlerts: %v", err)
	}
	if _, found := findLoop(loops, "retired"); found {
		t.Fatal("a loop no process has registered for longer than the retirement window is retired, " +
			"not stalled — it must stop alerting on its own")
	}
	if _, found := findLoop(loops, "still-wired"); !found {
		t.Fatal("retirement must not silence a loop that is still registered on every boot")
	}
}

// TestLongLivedProcessDoesNotRetireItsOwnLoops is the regression test for the
// worst defect found in review, and it is worth stating plainly because the
// mechanism was subtle.
//
// Registration was originally written ONLY at loop start, so a row's
// last_registered_at was effectively process start time, frozen. Any process that
// stayed up longer than retireAfter — a quiet week, a code freeze — aged every row
// out of the retirement gate, and the overdue query then returned nothing for the
// entire fleet, forever, with no error and no log above Debug. Monitoring that
// silently switches itself off, coupled to deploy cadence: PSY-1606's exact shape,
// re-created by the feature built to prevent it.
//
// The fix separates CONFIGURATION liveness from GOROUTINE liveness. The health
// check re-stamps the loops this process owns on its own cadence, so a wired loop
// stays eligible for as long as something runs it — while a loop whose goroutine
// has died still alerts, because the process still declares it.
func TestLongLivedProcessDoesNotRetireItsOwnLoops(t *testing.T) {
	db, store := setupRunStore(t)

	resetRegisteredLoops()
	t.Cleanup(resetRegisteredLoops)

	// A loop this process owns, whose row was last stamped long before the
	// retirement window — i.e. a process that has simply been up a long time.
	rememberLoop("long-uptime-sweep", time.Hour, defaultRunLease)
	registerLoop(t, store, "long-uptime-sweep", time.Hour)
	backdateCompletion(t, db, "long-uptime-sweep", 30*24*time.Hour)
	retireRow(t, db, "long-uptime-sweep")

	var received []OverdueLoop
	SetOverdueHandler(func(loop OverdueLoop) { received = append(received, loop) })
	t.Cleanup(func() { SetOverdueHandler(nil) })

	newTestHealthCheck(store).RunCheckNow(context.Background())

	if _, found := findLoop(received, "long-uptime-sweep"); !found {
		t.Fatal("a loop this process still owns must stay alertable no matter how long the process has been up — " +
			"tying registration to deploy cadence silently disables the whole subsystem")
	}
}

// TestRetirementSurvivesWhenNoProcessOwnsTheLoop is the other half: the refresh
// must not make retirement unreachable. A row for a loop NOT in this process's
// registry is never re-stamped, so it still ages out.
func TestRetirementSurvivesWhenNoProcessOwnsTheLoop(t *testing.T) {
	db, store := setupRunStore(t)

	resetRegisteredLoops()
	t.Cleanup(resetRegisteredLoops)

	// Deliberately NOT remembered — nothing in this process wires it up.
	registerLoop(t, store, "orphan-sweep", time.Hour)
	backdateCompletion(t, db, "orphan-sweep", 30*24*time.Hour)
	retireRow(t, db, "orphan-sweep")

	var received []OverdueLoop
	SetOverdueHandler(func(loop OverdueLoop) { received = append(received, loop) })
	t.Cleanup(func() { SetOverdueHandler(nil) })

	newTestHealthCheck(store).RunCheckNow(context.Background())

	if _, found := findLoop(received, "orphan-sweep"); found {
		t.Fatal("the registration refresh must only cover loops this process owns, " +
			"or retirement can never happen and a restored/renamed row alerts forever")
	}
}

// TestRetireStopsAlertingImmediately covers the operational escape hatch. The
// passive path (letting registration go stale) bounds noise to a week but does not
// prevent it, and the case it was introduced for is exactly where that is not good
// enough: `psy-deploy-prod --with-db-restore` copies stage's rows INCLUDING their
// registration timestamps, and stage re-stamps its own every pass — so restored
// rows arrive in prod looking freshly registered and page daily for the full
// window. Retiring explicitly is what makes "a disabled sweep is not reported"
// true now rather than in a week.
func TestRetireStopsAlertingImmediately(t *testing.T) {
	db, store := setupRunStore(t)
	ctx := context.Background()

	registerLoop(t, store, "switched-off", time.Hour)

	// Run a REAL cycle so the row carries genuine history — backdateCompletion
	// only fabricates a timestamp and would leave run_count at 0, which would make
	// the "history survives retirement" assertion below vacuous.
	token, ok, err := store.Claim(ctx, "switched-off", time.Hour, time.Hour, true)
	if err != nil || !ok {
		t.Fatalf("claim: ok=%v err=%v", ok, err)
	}
	if err := store.Complete(ctx, "switched-off", token, CycleOutcome{Duration: time.Second}); err != nil {
		t.Fatalf("complete: %v", err)
	}
	backdateCompletion(t, db, "switched-off", 30*24*time.Hour)

	first, err := store.ClaimOverdueAlerts(ctx, testAlertWindow)
	if err != nil {
		t.Fatalf("precondition pass: %v", err)
	}
	if _, found := findLoop(first, "switched-off"); !found {
		t.Fatal("precondition: a 30-day-stale loop must alert before it is retired")
	}

	if err := store.Retire(ctx, "switched-off"); err != nil {
		t.Fatalf("retire: %v", err)
	}

	// Clear the throttle so the next pass is not merely rate-limited — the
	// assertion must prove retirement, not the 24h window.
	backdateAlert(t, db, "switched-off", 48*time.Hour)

	again, err := store.ClaimOverdueAlerts(ctx, testAlertWindow)
	if err != nil {
		t.Fatalf("post-retire pass: %v", err)
	}
	if _, found := findLoop(again, "switched-off"); found {
		t.Fatal("a retired loop must stop alerting immediately, not after the retirement window")
	}

	// Retiring must not destroy the run trace — the row is still the evidence for
	// when the loop last did anything.
	var runCount int64
	if err := db.Raw(`SELECT run_count FROM background_service_runs WHERE name = ?`, "switched-off").
		Scan(&runCount).Error; err != nil {
		t.Fatalf("read run_count: %v", err)
	}
	if runCount == 0 {
		t.Fatal("retire must keep the row and its history; DELETE is the destructive alternative it replaces")
	}
}

// TestUnregisteredRowIsNotAlerted: rows created before registration existed (or by
// Claim alone) carry NULL. Treating NULL as retired is what stops the first deploy
// of this feature from paging for every historical row at once; any loop that is
// genuinely wired re-registers within one boot and becomes eligible immediately.
func TestUnregisteredRowIsNotAlerted(t *testing.T) {
	db, store := setupRunStore(t)
	ctx := context.Background()

	// A row the way PSY-1611 made them: created by Claim, never registered.
	token, ok, err := store.Claim(ctx, "legacy-row", time.Hour, time.Hour, false)
	if err != nil || !ok {
		t.Fatalf("claim: ok=%v err=%v", ok, err)
	}
	if err := store.Complete(ctx, "legacy-row", token, CycleOutcome{Duration: time.Second}); err != nil {
		t.Fatalf("complete: %v", err)
	}
	backdateCompletion(t, db, "legacy-row", 30*24*time.Hour)

	loops, err := store.ClaimOverdueAlerts(ctx, testAlertWindow)
	if err != nil {
		t.Fatalf("ClaimOverdueAlerts: %v", err)
	}
	if _, found := findLoop(loops, "legacy-row"); found {
		t.Fatal("a never-registered row must not alert; otherwise the first deploy pages for every legacy row at once")
	}

	// One boot of the owning process makes it eligible again.
	registerLoop(t, store, "legacy-row", time.Hour)
	again, err := store.ClaimOverdueAlerts(ctx, testAlertWindow)
	if err != nil {
		t.Fatalf("second pass: %v", err)
	}
	if _, found := findLoop(again, "legacy-row"); !found {
		t.Fatal("once a live process registers the loop, a genuine stall must alert")
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

// TestFailedCycleAlsoClearsThrottle pins the counterpart, and it is deliberately
// the OPPOSITE of what looks intuitive.
//
// The tempting rule is "only a successful cycle clears the throttle, because a
// failing loop hasn't recovered". That rule is wrong twice. It cannot prevent the
// flood it appears to guard against — Complete writes last_completed_at for
// failures too, so a crash-looping loop never reads as overdue in the first place
// — and it leaves a stale stamp behind that suppresses the NEXT genuine outage for
// the remainder of the window.
//
// "Is it running" and "is it succeeding" are different questions. This column only
// answers the first.
func TestFailedCycleAlsoClearsThrottle(t *testing.T) {
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
	if stamp := alertStamp(t, db, "failing"); stamp != nil {
		t.Fatalf("a completed cycle ends the overdue episode even when it failed, got stamp %v", stamp)
	}
}

// TestStaleStampCannotSuppressALaterOutage is the blind window that the
// clear-on-success-only rule opened, written as the operator-visible symptom
// rather than as an implementation detail.
//
// Sequence: a sweep goes overdue and alerts; it comes back but only produces
// FAILING cycles, so it stops being overdue without ever succeeding; then it dies
// for real. Under clear-on-success the stamp from the first outage was still
// sitting there, and the second — genuine, ongoing — outage stayed silent until
// the window expired.
func TestStaleStampCannotSuppressALaterOutage(t *testing.T) {
	db, store := setupRunStore(t)
	ctx := context.Background()

	registerLoop(t, store, "twice-dead", time.Hour)

	// Outage one: reported.
	backdateCompletion(t, db, "twice-dead", 5*time.Hour)
	first, err := store.ClaimOverdueAlerts(ctx, testAlertWindow)
	if err != nil {
		t.Fatalf("first outage: %v", err)
	}
	if _, found := findLoop(first, "twice-dead"); !found {
		t.Fatal("precondition: first outage must alert")
	}

	// It comes back, but every cycle fails. It is running again, so it is no
	// longer overdue — and it never succeeds, so a success-only rule never clears.
	token, ok, err := store.Claim(ctx, "twice-dead", time.Hour, time.Hour, true)
	if err != nil || !ok {
		t.Fatalf("claim: ok=%v err=%v", ok, err)
	}
	if err := store.Complete(ctx, "twice-dead", token, CycleOutcome{
		Err:      context.DeadlineExceeded,
		Duration: time.Second,
	}); err != nil {
		t.Fatalf("complete: %v", err)
	}

	// Outage two, well inside the re-alert window.
	backdateCompletion(t, db, "twice-dead", 5*time.Hour)
	second, err := store.ClaimOverdueAlerts(ctx, testAlertWindow)
	if err != nil {
		t.Fatalf("second outage: %v", err)
	}
	if _, found := findLoop(second, "twice-dead"); !found {
		t.Fatal("a second, genuine outage must not be suppressed by a stamp left over from the first")
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

	// RunScheduledLoop populates the process-wide registry; without this the name
	// leaks into every later test's refreshRegistrations pass.
	t.Cleanup(resetRegisteredLoops)

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

	// The checker must hold NO run-state row of its own. Asserted here rather than
	// only against the constants, because the property actually depends on
	// newLoopRunner's store-selection branch in another file — both constants could
	// still satisfy a unit assertion while a stray explicit Store put the checker
	// into the very set of loops it scans.
	var selfRows int64
	if err := db.Raw(`SELECT COUNT(*) FROM background_service_runs WHERE name = ?`, "sweep_health_check").
		Scan(&selfRows).Error; err != nil {
		t.Fatalf("count checker rows: %v", err)
	}
	if selfRows != 0 {
		t.Fatal("the health check must not persist run state — with a row it enters its own candidate set, " +
			"and a checker that stops running becomes the thing responsible for noticing")
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
