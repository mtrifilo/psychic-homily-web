package catalog

import (
	"testing"
	"time"

	catalogm "psychic-homily-backend/internal/models/catalog"
)

// These tests cover the PURE breaker decision functions (no DB). The end-to-end
// DB wiring — gate → run → updateStationHealth write-back, restart survival, the
// manual-probe policy through RunStationSync — is covered by RadioSyncSuite in
// radio_sync_integration_test.go.

func ptrTime(t time.Time) *time.Time { return &t }

// breakerGateFor: a scheduled/auto run is blocked only while the breaker is open
// AND within cooldown; past cooldown (or already half_open) it gets one trial.
func TestBreakerGateFor(t *testing.T) {
	now := time.Date(2026, 6, 20, 12, 0, 0, 0, time.UTC)
	withinCooldown := now.Add(-radioBreakerCooldown + time.Minute)
	pastCooldown := now.Add(-radioBreakerCooldown - time.Minute)

	cases := []struct {
		name string
		snap breakerSnapshot
		want breakerGate
	}{
		{"closed → allow", breakerSnapshot{state: catalogm.RadioBreakerStateClosed}, gateAllow},
		{"never-synced zero value → allow", breakerSnapshot{}, gateAllow},
		{"open, no trip time → blocked (defensive)", breakerSnapshot{state: catalogm.RadioBreakerStateOpen}, gateBlocked},
		{"open, within cooldown → blocked", breakerSnapshot{state: catalogm.RadioBreakerStateOpen, trippedAt: ptrTime(withinCooldown)}, gateBlocked},
		{"open, past cooldown → trial", breakerSnapshot{state: catalogm.RadioBreakerStateOpen, trippedAt: ptrTime(pastCooldown)}, gateTrial},
		{"open, exactly at cooldown → trial", breakerSnapshot{state: catalogm.RadioBreakerStateOpen, trippedAt: ptrTime(now.Add(-radioBreakerCooldown))}, gateTrial},
		{"half_open → trial", breakerSnapshot{state: catalogm.RadioBreakerStateHalfOpen}, gateTrial},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := breakerGateFor(tc.snap, now); got != tc.want {
				t.Fatalf("breakerGateFor(%+v) = %d, want %d", tc.snap, got, tc.want)
			}
		})
	}
}

// breakerTransition: the full closed → open → half_open → closed/open machine,
// plus the trigger/errKind policy (manual never trips; only permanent counts).
func TestBreakerTransition(t *testing.T) {
	now := time.Date(2026, 6, 20, 12, 0, 0, 0, time.UTC)
	closed := func(failures int) breakerSnapshot {
		return breakerSnapshot{state: catalogm.RadioBreakerStateClosed, failures: failures}
	}
	const sched = catalogm.RadioSyncRunTriggerScheduled
	const auto = catalogm.RadioSyncRunTriggerAutoBackfill
	const manual = catalogm.RadioSyncRunTriggerManual

	cases := []struct {
		name         string
		cur          breakerSnapshot
		status       string
		trigger      string
		kind         errorKind
		wantState    string
		wantFailures int
		wantTripped  bool // expect breaker_tripped_at == now
	}{
		// success / partial reset to closed from any state.
		{"success resets from climbing", closed(3), catalogm.RadioSyncRunStatusSuccess, sched, kindPermanent, catalogm.RadioBreakerStateClosed, 0, false},
		{"success closes a half-open trial", breakerSnapshot{state: catalogm.RadioBreakerStateHalfOpen, failures: 5, trippedAt: ptrTime(now)}, catalogm.RadioSyncRunStatusSuccess, sched, kindPermanent, catalogm.RadioBreakerStateClosed, 0, false},
		{"partial also resets+closes", breakerSnapshot{state: catalogm.RadioBreakerStateHalfOpen, failures: 5}, catalogm.RadioSyncRunStatusPartial, sched, kindPermanent, catalogm.RadioBreakerStateClosed, 0, false},

		// skipped / non-terminal leave everything untouched.
		{"skipped leaves open breaker untouched", breakerSnapshot{state: catalogm.RadioBreakerStateOpen, failures: 5, trippedAt: ptrTime(now.Add(-time.Hour))}, catalogm.RadioSyncRunStatusSkipped, sched, kindPermanent, catalogm.RadioBreakerStateOpen, 5, false},

		// permanent failures climb, then trip at threshold.
		{"permanent below threshold climbs, stays closed", closed(radioCircuitBreakerThreshold - 2), catalogm.RadioSyncRunStatusFailed, sched, kindPermanent, catalogm.RadioBreakerStateClosed, radioCircuitBreakerThreshold - 1, false},
		{"permanent reaching threshold opens", closed(radioCircuitBreakerThreshold - 1), catalogm.RadioSyncRunStatusFailed, sched, kindPermanent, catalogm.RadioBreakerStateOpen, radioCircuitBreakerThreshold, true},

		// transient never increments, never trips (PSY-887).
		{"transient failure does not climb", closed(radioCircuitBreakerThreshold - 1), catalogm.RadioSyncRunStatusFailed, sched, kindTransient, catalogm.RadioBreakerStateClosed, radioCircuitBreakerThreshold - 1, false},

		// half-open trial failures re-open with a fresh trip time.
		{"half-open + permanent fail re-opens (counter++)", breakerSnapshot{state: catalogm.RadioBreakerStateHalfOpen, failures: radioCircuitBreakerThreshold}, catalogm.RadioSyncRunStatusFailed, sched, kindPermanent, catalogm.RadioBreakerStateOpen, radioCircuitBreakerThreshold + 1, true},
		{"half-open + transient fail re-opens (counter unchanged)", breakerSnapshot{state: catalogm.RadioBreakerStateHalfOpen, failures: radioCircuitBreakerThreshold}, catalogm.RadioSyncRunStatusFailed, sched, kindTransient, catalogm.RadioBreakerStateOpen, radioCircuitBreakerThreshold, true},

		// auto_backfill behaves like scheduled.
		{"auto_backfill permanent trips like scheduled", closed(radioCircuitBreakerThreshold - 1), catalogm.RadioSyncRunStatusFailed, auto, kindPermanent, catalogm.RadioBreakerStateOpen, radioCircuitBreakerThreshold, true},

		// MANUAL probe: success closes (covered above via success arm); failure never trips.
		{"manual permanent failure never trips (closed stays closed)", closed(radioCircuitBreakerThreshold - 1), catalogm.RadioSyncRunStatusFailed, manual, kindPermanent, catalogm.RadioBreakerStateClosed, radioCircuitBreakerThreshold - 1, false},
		{"manual failure leaves an open breaker exactly as-is", breakerSnapshot{state: catalogm.RadioBreakerStateOpen, failures: 5, trippedAt: ptrTime(now.Add(-time.Hour))}, catalogm.RadioSyncRunStatusFailed, manual, kindPermanent, catalogm.RadioBreakerStateOpen, 5, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := breakerTransition(tc.cur, tc.status, tc.trigger, tc.kind, now)
			if got.state != tc.wantState {
				t.Errorf("state = %q, want %q", got.state, tc.wantState)
			}
			if got.failures != tc.wantFailures {
				t.Errorf("failures = %d, want %d", got.failures, tc.wantFailures)
			}
			if tc.wantTripped {
				if got.trippedAt == nil || !got.trippedAt.Equal(now) {
					t.Errorf("trippedAt = %v, want %v", got.trippedAt, now)
				}
			}
		})
	}
}

// A clean closed → open → (cooldown) → half_open trial → closed recovery cycle,
// driven purely through the decision functions, mirrors the lifecycle the DB
// wiring persists.
func TestBreakerLifecycle_CloseOpenHalfOpenClose(t *testing.T) {
	now := time.Date(2026, 6, 20, 12, 0, 0, 0, time.UTC)
	const sched = catalogm.RadioSyncRunTriggerScheduled

	// Climb to the threshold with permanent failures → open.
	snap := breakerSnapshot{state: catalogm.RadioBreakerStateClosed}
	for i := 0; i < radioCircuitBreakerThreshold; i++ {
		snap = breakerTransition(snap, catalogm.RadioSyncRunStatusFailed, sched, kindPermanent, now)
	}
	if snap.state != catalogm.RadioBreakerStateOpen {
		t.Fatalf("after %d permanent failures, want open; got %q", radioCircuitBreakerThreshold, snap.state)
	}

	// Within cooldown → blocked.
	if g := breakerGateFor(snap, now.Add(time.Minute)); g != gateBlocked {
		t.Fatalf("within cooldown want gateBlocked; got %d", g)
	}
	// Past cooldown → trial.
	past := now.Add(radioBreakerCooldown + time.Minute)
	if g := breakerGateFor(snap, past); g != gateTrial {
		t.Fatalf("past cooldown want gateTrial; got %d", g)
	}

	// Mark half_open (what RunStationSync does on a trial), then a successful trial closes it.
	snap.state = catalogm.RadioBreakerStateHalfOpen
	snap = breakerTransition(snap, catalogm.RadioSyncRunStatusSuccess, sched, kindPermanent, past)
	if snap.state != catalogm.RadioBreakerStateClosed || snap.failures != 0 {
		t.Fatalf("a successful trial must close+reset; got state=%q failures=%d", snap.state, snap.failures)
	}
}
