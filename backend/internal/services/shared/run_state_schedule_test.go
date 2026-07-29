package shared

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The bug this file guards is TEMPORAL: nothing errored, nothing was
// misconfigured, and a test asserting "a ticker was created" would have passed
// throughout. What was wrong was WHEN the first cycle happened relative to the
// process lifetime. So every test here manipulates the relationship between the
// loop's interval, the persisted completion time, and how long the process is
// allowed to live.
//
// Real durations are compressed but the ratios are the production ones: an
// "interval" is long relative to a "process lifetime", exactly as 24h is long
// relative to a Railway deploy window.

// memRunStore is an in-memory RunStore with the same decision rules as the SQL
// in GormRunStore: schedule from last completion, refuse a claim that is not due
// or that collides with a live one, release rather than complete an interrupted
// cycle. The SQL itself is covered separately against a real Postgres; this
// double exists so the SCHEDULER's behaviour can be tested at millisecond
// resolution without a container per case.
type memRunStore struct {
	mu   sync.Mutex
	rows map[string]*memRunRow

	registered atomic.Int32
	retired    atomic.Int32
	claimed    atomic.Int32
	refused    atomic.Int32
	finished   atomic.Int32
	released   atomic.Int32

	failDueIn    error
	failClaim    error
	failRegister error
}

type memRunRow struct {
	startedAt   time.Time
	completedAt time.Time
	successAt   time.Time
	outcome     string
	rows        *int
	// lastErrText mirrors the persisted last_error COLUMN, not the Go error the
	// cycle returned. Storing the error value instead would quietly diverge from
	// production, where a panic and a shutdown carry no error of their own and
	// the text is synthesized — a future assertion against this double would then
	// get a green result for behaviour the database does not have.
	lastErrText     *string
	intervalSeconds int64
	leaseSeconds    int64
	failures        int
	runCount        int
}

func newMemRunStore() *memRunStore {
	return &memRunStore{rows: map[string]*memRunRow{}}
}

func (s *memRunStore) row(name string) *memRunRow {
	r, ok := s.rows[name]
	if !ok {
		r = &memRunRow{}
		s.rows[name] = r
	}
	return r
}

// seedCompleted pretends the loop completed `ago` in the past.
func (s *memRunStore) seedCompleted(name string, ago time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r := s.row(name)
	r.completedAt = time.Now().Add(-ago)
	r.outcome = RunOutcomeSuccess
}

func (s *memRunStore) snapshot(name string) memRunRow {
	s.mu.Lock()
	defer s.mu.Unlock()
	return *s.row(name)
}

// Register mirrors the SQL: create-or-refresh identity and cadence, touching no
// run state. A double that reset completion here would hide the production bug
// where a redeploy wipes the schedule the table exists to preserve.
func (s *memRunStore) Register(_ context.Context, name string, interval, lease time.Duration) error {
	if s.failRegister != nil {
		return s.failRegister
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	r := s.row(name)
	r.intervalSeconds = int64(interval / time.Second)
	r.leaseSeconds = int64(lease / time.Second)
	s.registered.Add(1)
	return nil
}

// Retire mirrors the SQL sentinel: clear registration so the row reads as no
// longer expected to run.
func (s *memRunStore) Retire(_ context.Context, name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.row(name)
	s.retired.Add(1)
	return nil
}

func (s *memRunStore) DueIn(_ context.Context, name string, interval time.Duration) (time.Duration, error) {
	if s.failDueIn != nil {
		return 0, s.failDueIn
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	r := s.row(name)
	if r.completedAt.IsZero() {
		return 0, nil
	}
	due := time.Until(r.completedAt.Add(interval))
	if due < 0 {
		return 0, nil
	}
	if due > interval {
		due = interval
	}
	return due, nil
}

func (s *memRunStore) Claim(_ context.Context, name string, interval, lease time.Duration, force bool) (time.Time, bool, error) {
	if s.failClaim != nil {
		return time.Time{}, false, s.failClaim
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	r := s.row(name)
	now := time.Now()

	if !force && !r.completedAt.IsZero() && now.Before(r.completedAt.Add(interval-dueSlack(interval))) {
		s.refused.Add(1)
		return time.Time{}, false, nil
	}
	if r.outcome == RunOutcomeRunning && !r.startedAt.IsZero() && now.Before(r.startedAt.Add(lease)) {
		s.refused.Add(1)
		return time.Time{}, false, nil
	}

	r.startedAt = now
	r.outcome = RunOutcomeRunning
	r.intervalSeconds = int64(interval / time.Second)
	s.claimed.Add(1)
	return r.startedAt, true, nil
}

func (s *memRunStore) Complete(_ context.Context, name string, token time.Time, outcome CycleOutcome) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	r := s.row(name)
	if !r.startedAt.Equal(token) {
		return ErrClaimSuperseded
	}
	if outcome.Interrupted {
		r.startedAt = time.Time{}
		r.outcome = RunOutcomeError
		r.lastErrText = outcome.errorText()
		s.released.Add(1)
		return nil
	}
	r.completedAt = time.Now()
	r.outcome = outcome.outcomeLabel()
	r.rows = outcome.Rows
	r.lastErrText = outcome.errorText()
	r.runCount++
	if r.outcome == RunOutcomeSuccess {
		r.successAt = r.completedAt
		r.failures = 0
	} else {
		r.failures++
	}
	s.finished.Add(1)
	return nil
}

// compressCatchUp shrinks the boot stagger so a catch-up is observable inside a
// test, and restores the production values afterwards.
func compressCatchUp(t *testing.T, base, spacing time.Duration) {
	t.Helper()
	oldBase, oldSpacing, oldMax := catchUpBaseDelay, catchUpSpacing, catchUpMaxDelay
	catchUpBaseDelay, catchUpSpacing, catchUpMaxDelay = base, spacing, time.Second
	catchUpSlotsClaimed.Store(0)
	t.Cleanup(func() {
		catchUpBaseDelay, catchUpSpacing, catchUpMaxDelay = oldBase, oldSpacing, oldMax
		catchUpSlotsClaimed.Store(0)
	})
}

// runFor starts a loop and lets it live for `lifetime`, then shuts it down and
// waits — one simulated process.
func runFor(cfg LoopConfig, lifetime time.Duration, work func(context.Context)) {
	ctx, cancel := context.WithTimeout(context.Background(), lifetime)
	defer cancel()
	RunScheduledLoop(ctx, cfg, work)
}

// TestOverdueOnBootRunsImmediately: a loop whose persisted completion is older
// than its interval must not wait for the interval again. This is the entire
// point of persistence — the previous process already burned the wait.
func TestOverdueOnBootRunsImmediately(t *testing.T) {
	compressCatchUp(t, 5*time.Millisecond, time.Millisecond)

	store := newMemRunStore()
	store.seedCompleted("overdue-sweep", 2*time.Hour)

	var calls atomic.Int32
	start := time.Now()
	runFor(LoopConfig{
		Name:     "overdue-sweep",
		Interval: time.Hour,
		Store:    store,
	}, 120*time.Millisecond, func(_ context.Context) { calls.Add(1) })

	require.Equal(t, int32(1), calls.Load(),
		"an overdue loop must run on boot rather than waiting out another interval")
	assert.Less(t, time.Since(start), time.Hour, "sanity: it did not wait the interval")

	got := store.snapshot("overdue-sweep")
	assert.Equal(t, RunOutcomeSuccess, got.outcome)
	assert.Equal(t, 1, got.runCount, "the completed cycle must be recorded")
}

// TestNotOverdueOnBootWaitsAndSchedulesFromLastCompletion is the other half of
// the contract: a loop that completed recently must NOT re-run at boot, and its
// wait must be measured from that completion rather than from process start.
//
// The assertion is deliberately "did it fire inside the window", with no tight
// timing bound, because the three candidate behaviours are separated by orders
// of magnitude rather than milliseconds. The loop is given a 10s interval and a
// persisted completion that leaves it due in 150ms, and the catch-up stagger is
// stretched to 5s:
//
//	scheduled from last completion (correct) -> fires at ~150ms
//	treated as overdue at boot (wrong)       -> would wait 5s
//	anchored to the interval / start delay   -> would wait 10s
//
// Only the correct behaviour fires inside the 600ms process lifetime, so a
// single "it fired" assertion rules out both failure modes with 4x margin on the
// nearest alternative — nothing here degrades on a loaded CI box.
func TestNotOverdueOnBootWaitsAndSchedulesFromLastCompletion(t *testing.T) {
	compressCatchUp(t, 5*time.Second, time.Second)

	const interval = 10 * time.Second
	store := newMemRunStore()
	store.seedCompleted("recent-sweep", interval-150*time.Millisecond)

	var calls atomic.Int32
	runFor(LoopConfig{
		Name:     "recent-sweep",
		Interval: interval,
		Store:    store,
	}, 600*time.Millisecond, func(_ context.Context) { calls.Add(1) })

	require.Equal(t, int32(1), calls.Load(),
		"the cycle must fire at the time the PERSISTED COMPLETION makes it due; "+
			"waiting for the interval, the start delay, or a catch-up slot would all have missed this window")

	// And it consumed no catch-up slot, because it was never overdue.
	assert.Zero(t, catchUpSlotsClaimed.Load(),
		"a loop that is merely waiting must not be classified as overdue")
}

// TestRepeatedRestartsStillMakeProgress simulates the production failure
// directly: a process that never lives as long as the sweep's interval,
// restarted over and over. Under the old process-anchored ticker the answer was
// zero cycles forever, which is exactly what production did for days.
//
// Both halves run the SAME restart pattern. The persisted loop makes progress;
// the unpersisted one (the old behaviour, since a full interval is the only
// anchor it has) makes none. The contrast is the assertion.
//
// The interval is chosen so the due gate inside Claim stays live rather than
// degenerate: dueSlack is 5% of the interval bounded below by 10ms, so at 400ms
// the gate sits at 380ms — a real fraction of the interval, the same shape as
// the 24h/1m ratio in production. An interval small enough for slack to reach
// its floor would make `interval - slack` collapse and quietly retire the gate,
// leaving the cadence carried by DueIn alone and the test asserting less than it
// appears to.
func TestRepeatedRestartsStillMakeProgress(t *testing.T) {
	compressCatchUp(t, 2*time.Millisecond, time.Millisecond)

	const (
		interval  = 400 * time.Millisecond // the "24h sweep"
		lifetime  = 100 * time.Millisecond // each "deploy window", far shorter
		restarts  = 10
		cfgName   = "restart-sweep"
		unpersist = "restart-sweep-no-state"
	)

	// Guard the premise: if a future change to dueSlack made the gate vacuous,
	// this test would silently stop exercising it.
	require.Less(t, dueSlack(interval), interval/2+time.Nanosecond,
		"dueSlack must stay a genuine fraction of the interval or Claim's due gate is inert")

	store := newMemRunStore()
	var persistedCalls atomic.Int32
	persistedStart := time.Now()
	for i := 0; i < restarts; i++ {
		runFor(LoopConfig{
			Name:     cfgName,
			Interval: interval,
			Store:    store,
		}, lifetime, func(_ context.Context) { persistedCalls.Add(1) })
	}
	persistedElapsed := time.Since(persistedStart)

	var unpersistedCalls atomic.Int32
	for i := 0; i < restarts; i++ {
		runFor(LoopConfig{
			Name:       unpersist,
			Interval:   interval,
			StartDelay: interval, // no persisted state to shorten the wait
		}, lifetime, func(_ context.Context) { unpersistedCalls.Add(1) })
	}

	assert.Zero(t, unpersistedCalls.Load(),
		"control: a process-lifetime-anchored wait longer than the process lifetime never fires — the original bug")
	assert.GreaterOrEqual(t, int(persistedCalls.Load()), 2,
		"persisted state must carry the schedule across restarts; %d restarts x %v of uptime spans %v of a %v interval, got %d cycles",
		restarts, lifetime, restarts*lifetime, interval, persistedCalls.Load())

	// And progress is bounded by the INTERVAL, not by the restart count: a deploy
	// storm must not turn into a cycle storm against third-party APIs. The bound
	// is derived from elapsed wall time rather than hardcoded, so a slow or
	// -race-instrumented runner (which stretches elapsed time, and with it the
	// number of legitimately-due cycles) cannot make this fail spuriously.
	//
	// The pacing floor is `interval - dueSlack`, not the bare interval: that is
	// the earliest a second claim can succeed, so it is the honest divisor.
	// Allowance of +2: one for the catch-up cycle at the very first boot, which
	// is not paced at all, and one for a boundary landing mid-interval.
	minSpacing := interval - dueSlack(interval)
	maxCycles := int(persistedElapsed/minSpacing) + 2
	assert.LessOrEqual(t, int(persistedCalls.Load()), maxCycles,
		"restarts must not multiply cycles; %v elapsed at a %v minimum spacing permits at most %d, got %d",
		persistedElapsed, minSpacing, maxCycles, persistedCalls.Load())

	// The sharper statement of the same property: ten restarts did not buy ten
	// cycles. This is what would break if a restart ever reset the schedule.
	assert.Less(t, int(persistedCalls.Load()), restarts,
		"a restart must not earn a cycle; that would be the mirror image of the bug")
}

// TestCatchUpSlotsAreSpacedAndBounded covers the stampede question at its
// source. Seven sweeps discovering at the same boot that they are overdue must
// not fire together, because the MusicBrainz / Nominatim / Spotify rate limits
// they share are ToS obligations rather than best-effort.
//
// This asserts on the delays the scheduler HANDS OUT rather than on observed
// fire times: the spacing policy is a pure function, so testing it directly is
// both exact and immune to timer jitter. TestOverdueLoopsDoNotCoFire covers the
// end-to-end wiring separately.
func TestCatchUpSlotsAreSpacedAndBounded(t *testing.T) {
	compressCatchUp(t, 60*time.Second, 90*time.Second)
	catchUpMaxDelay = 30 * time.Minute // the production cap, not the compressed one

	var got []time.Duration
	for i := 0; i < 7; i++ {
		got = append(got, nextCatchUpDelay())
	}

	want := []time.Duration{
		60 * time.Second, 150 * time.Second, 240 * time.Second, 330 * time.Second,
		420 * time.Second, 510 * time.Second, 600 * time.Second,
	}
	assert.Equal(t, want, got,
		"seven overdue sweeps must be spread across ten minutes, not co-fired")

	// A pathological number of overdue loops must not push the tail out forever.
	for i := 0; i < 100; i++ {
		require.LessOrEqual(t, nextCatchUpDelay(), catchUpMaxDelay,
			"catch-up delay must stay bounded")
	}
}

// TestOverdueLoopsDoNotCoFire is the end-to-end half: three overdue loops
// booting together must actually observe distinct start times, proving the
// stagger is wired into firstCycleDelay and not merely computed.
func TestOverdueLoopsDoNotCoFire(t *testing.T) {
	compressCatchUp(t, 20*time.Millisecond, 80*time.Millisecond)

	store := newMemRunStore()
	names := []string{"sweep-a", "sweep-b", "sweep-c"}
	for _, n := range names {
		store.seedCompleted(n, time.Hour)
	}

	start := time.Now()
	var mu sync.Mutex
	firedAt := map[string]time.Duration{}

	var wg sync.WaitGroup
	for _, n := range names {
		wg.Add(1)
		go func(name string) {
			defer wg.Done()
			// Lifetime must clear the LAST stagger slot (20+2*80 = 180ms) with
			// room to spare, or the test fails for want of runway rather than
			// for want of staggering.
			runFor(LoopConfig{
				Name:     name,
				Interval: time.Hour,
				Store:    store,
			}, 500*time.Millisecond, func(_ context.Context) {
				mu.Lock()
				if _, seen := firedAt[name]; !seen {
					firedAt[name] = time.Since(start)
				}
				mu.Unlock()
			})
		}(n)
	}
	wg.Wait()

	require.Len(t, firedAt, len(names), "every overdue loop must eventually catch up")

	var times []time.Duration
	for _, d := range firedAt {
		times = append(times, d)
	}
	minAt, maxAt := times[0], times[0]
	for _, d := range times {
		if d < minAt {
			minAt = d
		}
		if d > maxAt {
			maxAt = d
		}
	}
	// Nominal spread is 160ms (slots at 20/100/180ms). Asserting >60ms leaves
	// well over half the nominal value as headroom for a loaded runner, while
	// still failing loudly if the loops co-fire.
	assert.Greater(t, maxAt-minAt, 60*time.Millisecond,
		"overdue loops must be staggered, not co-fired; got %v", times)
}

// TestClaimRefusedDoesNotCostAnInterval covers the crash-recovery path a naive
// implementation gets wrong: if a claim is refused because another instance
// holds one, retrying only after a full interval means one crashed process
// costs a daily sweep an entire day.
func TestClaimRefusedDoesNotCostAnInterval(t *testing.T) {
	compressCatchUp(t, time.Millisecond, time.Millisecond)
	oldRetry := claimRetryDelay
	claimRetryDelay = 20 * time.Millisecond
	t.Cleanup(func() { claimRetryDelay = oldRetry })

	store := newMemRunStore()
	// Someone else is mid-run and the lease has not expired.
	store.mu.Lock()
	r := store.row("contended-sweep")
	r.startedAt = time.Now()
	r.outcome = RunOutcomeRunning
	store.mu.Unlock()

	var calls atomic.Int32
	go func() {
		// Release the claim shortly after boot, as a finishing instance would.
		time.Sleep(30 * time.Millisecond)
		store.mu.Lock()
		store.row("contended-sweep").outcome = RunOutcomeSuccess
		store.mu.Unlock()
	}()

	runFor(LoopConfig{
		Name:     "contended-sweep",
		Interval: time.Hour,
		Store:    store,
	}, 200*time.Millisecond, func(_ context.Context) { calls.Add(1) })

	assert.Positive(t, store.refused.Load(), "the live claim must have been refused at least once")
	assert.Equal(t, int32(1), calls.Load(),
		"after the claim frees up the loop must retry within the retry delay, not a full interval")
}

// TestInterruptedCycleReleasesClaimWithoutCompleting: a deploy landing mid-cycle
// must leave the loop still overdue. Stamping a completion would tell the next
// boot the work is done for another interval, silently skipping it.
func TestInterruptedCycleReleasesClaimWithoutCompleting(t *testing.T) {
	compressCatchUp(t, time.Millisecond, time.Millisecond)

	store := newMemRunStore()
	store.seedCompleted("interrupted-sweep", time.Hour)

	ctx, cancel := context.WithCancel(context.Background())
	started := make(chan struct{})
	done := make(chan struct{})
	go func() {
		RunScheduledLoop(ctx, LoopConfig{
			Name:     "interrupted-sweep",
			Interval: time.Hour,
			Store:    store,
		}, func(cycleCtx context.Context) {
			close(started)
			<-cycleCtx.Done() // work that notices shutdown and returns
		})
		close(done)
	}()

	<-started
	cancel()
	<-done

	got := store.snapshot("interrupted-sweep")
	assert.Equal(t, int32(1), store.released.Load(), "an interrupted cycle must release its claim")
	assert.Zero(t, store.finished.Load(), "an interrupted cycle must not record a completion")
	assert.True(t, got.startedAt.IsZero(), "the claim must be cleared so the next boot can take it")

	due, err := store.DueIn(context.Background(), "interrupted-sweep", time.Hour)
	require.NoError(t, err)
	assert.LessOrEqual(t, due, time.Duration(0), "the loop must still read as overdue after an interruption")
}

// TestFailedCycleStillSchedulesNextAttempt: a cycle that errors must record the
// failure for health purposes but keep the normal cadence. Leaving it
// permanently overdue would make it fire on every single deploy — turning a
// broken sweep into a third-party rate-limit incident.
func TestFailedCycleStillSchedulesNextAttempt(t *testing.T) {
	compressCatchUp(t, time.Millisecond, time.Millisecond)

	store := newMemRunStore()
	store.seedCompleted("failing-sweep", time.Hour)

	var calls atomic.Int32
	runFor(LoopConfig{
		Name:     "failing-sweep",
		Interval: time.Hour,
		Store:    store,
	}, 120*time.Millisecond, func(cycleCtx context.Context) {
		calls.Add(1)
		ReportCycle(cycleCtx, 7, assert.AnError)
	})

	require.Equal(t, int32(1), calls.Load())
	got := store.snapshot("failing-sweep")
	assert.Equal(t, RunOutcomeError, got.outcome)
	assert.Equal(t, 1, got.failures, "a failure must be counted for alerting")
	assert.True(t, got.successAt.IsZero(), "a failure must not advance the last-success marker")
	require.NotNil(t, got.rows)
	assert.Equal(t, 7, *got.rows, "a partial cycle's row count is still worth recording")

	due, err := store.DueIn(context.Background(), "failing-sweep", time.Hour)
	require.NoError(t, err)
	assert.Greater(t, due, time.Duration(0),
		"a failed cycle must schedule the next attempt an interval out, not stay permanently overdue")
}

// TestPanickingCycleIsRecordedAndLoopSurvives ties the panic recovery to the
// run trace: a panicking sweep must be visible as a panic, not as silence.
func TestPanickingCycleIsRecordedAndLoopSurvives(t *testing.T) {
	_ = withCapturedSlog(t)
	compressCatchUp(t, time.Millisecond, time.Millisecond)

	store := newMemRunStore()
	store.seedCompleted("panicking-sweep", time.Hour)

	runFor(LoopConfig{
		Name:     "panicking-sweep",
		Interval: time.Hour,
		Store:    store,
	}, 100*time.Millisecond, func(_ context.Context) { panic("cycle blew up") })

	got := store.snapshot("panicking-sweep")
	assert.Equal(t, RunOutcomePanic, got.outcome, "a panic must be distinguishable from a plain error")
	assert.Equal(t, 1, got.failures)
}

// TestStoreFailureFailsOpen: an unreachable run-state table must not stop the
// sweeps. Losing the trace is strictly better than losing the work.
func TestStoreFailureFailsOpen(t *testing.T) {
	_ = withCapturedSlog(t)

	store := newMemRunStore()
	store.failDueIn = assert.AnError
	store.failClaim = assert.AnError

	var calls atomic.Int32
	runFor(LoopConfig{
		Name:       "degraded-sweep",
		Interval:   time.Hour,
		StartDelay: 10 * time.Millisecond,
		Store:      store,
	}, 100*time.Millisecond, func(_ context.Context) { calls.Add(1) })

	assert.Equal(t, int32(1), calls.Load(),
		"a store outage must fall back to the bounded start delay and still run the cycle")
}

// TestBootCycleRunsEvenWhenNotDue preserves the behaviour of the ten loops that
// deliberately fire at boot. Adding persistence must not silently take that
// away — an operator deploying to see fresh output would otherwise get nothing.
func TestBootCycleRunsEvenWhenNotDue(t *testing.T) {
	store := newMemRunStore()
	store.seedCompleted("boot-loop", time.Second) // nowhere near due

	var calls atomic.Int32
	runFor(LoopConfig{
		Name:      "boot-loop",
		Interval:  time.Hour,
		RunAtBoot: true,
		Store:     store,
	}, 60*time.Millisecond, func(_ context.Context) { calls.Add(1) })

	assert.Equal(t, int32(1), calls.Load(), "RunAtBoot must still fire a cycle regardless of due time")
}

// TestBootCycleStillRespectsALiveClaim: RunAtBoot skips the DUE check, not the
// CONCURRENCY check. During a zero-downtime deploy the new instance boots while
// the old one is still draining, so a forced boot cycle must not collide with a
// run already in flight.
func TestBootCycleStillRespectsALiveClaim(t *testing.T) {
	_ = withCapturedSlog(t)

	store := newMemRunStore()
	store.mu.Lock()
	r := store.row("draining-loop")
	r.startedAt = time.Now()
	r.outcome = RunOutcomeRunning
	store.mu.Unlock()

	var calls atomic.Int32
	runFor(LoopConfig{
		Name:      "draining-loop",
		Interval:  time.Hour,
		RunAtBoot: true,
		Store:     store,
	}, 60*time.Millisecond, func(_ context.Context) { calls.Add(1) })

	assert.Zero(t, calls.Load(), "a boot cycle must not run alongside another instance's live claim")
	assert.Positive(t, store.refused.Load())
}

// TestSubHourLoopGetsNoPersistedState pins the threshold decision: a 30s worker
// must not write a run-state row every cycle. It also documents why that is
// safe — a first cycle capped at a 30s interval cannot be starved by any deploy
// cadence.
func TestSubHourLoopGetsNoPersistedState(t *testing.T) {
	store := newMemRunStore()
	SetDefaultRunStore(store)
	t.Cleanup(func() { SetDefaultRunStore(nil) })

	var calls atomic.Int32
	runFor(LoopConfig{
		Name:     "fast-worker",
		Interval: 10 * time.Millisecond,
	}, 100*time.Millisecond, func(_ context.Context) { calls.Add(1) })

	assert.Greater(t, int(calls.Load()), 3, "the short loop must keep cycling")
	assert.Zero(t, store.claimed.Load(), "a sub-hour loop must not touch the run-state table")
}

// TestLongLoopPicksUpTheDefaultStore is the other side of the threshold: a
// service does not have to ask for persistence, so no future caller can
// reintroduce the bug by forgetting to.
func TestLongLoopPicksUpTheDefaultStore(t *testing.T) {
	compressCatchUp(t, time.Millisecond, time.Millisecond)

	store := newMemRunStore()
	SetDefaultRunStore(store)
	t.Cleanup(func() { SetDefaultRunStore(nil) })

	var calls atomic.Int32
	runFor(LoopConfig{
		Name:     "slow-sweep",
		Interval: time.Hour,
	}, 100*time.Millisecond, func(_ context.Context) { calls.Add(1) })

	assert.Equal(t, int32(1), calls.Load(), "a never-run loop is overdue and must catch up")
	assert.Positive(t, store.claimed.Load(), "an hour-or-longer loop must persist without being asked")
}
