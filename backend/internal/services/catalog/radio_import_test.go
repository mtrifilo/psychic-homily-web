package catalog

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

// TestRetryOnSerialization exercises the bounded retry-on-serialization loop in
// isolation (no DB): it must retry only on a Postgres 40001, cap at
// playUpsertMaxAttempts, and pass any other error (or success) straight through.
// There is no integration test for the real upsert's 40001 path because a 40001
// cannot arise on INSERT … ON CONFLICT DO NOTHING under the production READ
// COMMITTED isolation (the retry is defense-in-depth for a future higher
// isolation level — see retryOnSerialization). The loop's behavior is therefore
// verified here via a stub op, and the detection predicate it relies on is
// covered by shared.TestIsSerializationFailure (incl. the TranslateError-survival
// case).
func TestRetryOnSerialization(t *testing.T) {
	serErr := &pgconn.PgError{Code: "40001"}

	t.Run("succeeds on first attempt", func(t *testing.T) {
		calls := 0
		err := retryOnSerialization(func() error { calls++; return nil })
		if err != nil {
			t.Fatalf("want nil err, got %v", err)
		}
		if calls != 1 {
			t.Errorf("want 1 call, got %d", calls)
		}
	})

	t.Run("retries past transient 40001 then succeeds", func(t *testing.T) {
		calls := 0
		err := retryOnSerialization(func() error {
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

	t.Run("surfaces persistent 40001 after max attempts", func(t *testing.T) {
		calls := 0
		err := retryOnSerialization(func() error { calls++; return serErr })
		if !errors.Is(err, serErr) {
			t.Fatalf("want the 40001 surfaced, got %v", err)
		}
		if calls != playUpsertMaxAttempts {
			t.Errorf("want %d attempts before surfacing, got %d", playUpsertMaxAttempts, calls)
		}
	})

	t.Run("non-serialization error returns immediately, unchanged", func(t *testing.T) {
		boom := errors.New("FK violation")
		calls := 0
		err := retryOnSerialization(func() error { calls++; return boom })
		if !errors.Is(err, boom) {
			t.Fatalf("want the original error unchanged, got %v", err)
		}
		if calls != 1 {
			t.Errorf("non-serialization error must not retry; got %d calls", calls)
		}
	})
}
