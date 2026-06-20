package catalog

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

// TestRetryTransientConflict exercises the bounded retry-on-transient-conflict
// loop in isolation (no DB): it must retry on a Postgres 40001 (serialization
// failure) OR a 40P01 (deadlock), cap at playUpsertMaxAttempts, and pass any
// other error (or success) straight through.
//
// There is no integration test for the real upsert's retry path because a 40001
// cannot arise on INSERT … ON CONFLICT DO NOTHING under the production READ
// COMMITTED isolation, and a 40P01 deadlock — while possible at READ COMMITTED —
// can't be induced deterministically here (the per-station advisory lock
// serializes same-station writes; the retry is mostly defense-in-depth for a
// future higher isolation level — see retryTransientConflict). The loop's
// behavior is therefore verified via a stub op, and the detection predicates it
// relies on are covered by shared.TestIsSerializationFailure / TestIsDeadlock
// (incl. the TranslateError-survival cases).
func TestRetryTransientConflict(t *testing.T) {
	serErr := &pgconn.PgError{Code: "40001"}      // serialization_failure
	deadlockErr := &pgconn.PgError{Code: "40P01"} // deadlock_detected

	t.Run("succeeds on first attempt", func(t *testing.T) {
		calls := 0
		err := retryTransientConflict(func() error { calls++; return nil })
		if err != nil {
			t.Fatalf("want nil err, got %v", err)
		}
		if calls != 1 {
			t.Errorf("want 1 call, got %d", calls)
		}
	})

	t.Run("retries past transient 40001 then succeeds", func(t *testing.T) {
		calls := 0
		err := retryTransientConflict(func() error {
			calls++
			if calls < playUpsertMaxAttempts {
				return serErr
			}
			return nil
		})
		if err != nil {
			t.Fatalf("want nil err after transient 40001s, got %v", err)
		}
		if calls != playUpsertMaxAttempts {
			t.Errorf("want %d calls, got %d", playUpsertMaxAttempts, calls)
		}
	})

	t.Run("retries on a 40P01 deadlock too", func(t *testing.T) {
		calls := 0
		err := retryTransientConflict(func() error {
			calls++
			if calls < playUpsertMaxAttempts {
				return deadlockErr
			}
			return nil
		})
		if err != nil {
			t.Fatalf("want nil err after transient deadlocks, got %v", err)
		}
		if calls != playUpsertMaxAttempts {
			t.Errorf("deadlock must be retried; want %d calls, got %d", playUpsertMaxAttempts, calls)
		}
	})

	t.Run("surfaces persistent 40001 after max attempts", func(t *testing.T) {
		calls := 0
		err := retryTransientConflict(func() error { calls++; return serErr })
		if !errors.Is(err, serErr) {
			t.Fatalf("want the 40001 surfaced, got %v", err)
		}
		if calls != playUpsertMaxAttempts {
			t.Errorf("want %d attempts before surfacing, got %d", playUpsertMaxAttempts, calls)
		}
	})

	t.Run("non-conflict error returns immediately, unchanged", func(t *testing.T) {
		boom := errors.New("FK violation")
		calls := 0
		err := retryTransientConflict(func() error { calls++; return boom })
		if !errors.Is(err, boom) {
			t.Fatalf("want the original error unchanged, got %v", err)
		}
		if calls != 1 {
			t.Errorf("non-conflict error must not retry; got %d calls", calls)
		}
	})
}
