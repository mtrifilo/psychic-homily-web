package indexnow

import (
	"context"
	"errors"
	"testing"
	"time"

	"psychic-homily-backend/internal/services/shared"
	"psychic-homily-backend/internal/testutil"
)

// The mark is last_success_at minus runLease, read from the row the loop's
// own run store writes: absent until a run succeeds, moved by a successful
// run, and left where it was by a failed one.
func TestRunRowMarksFollowTheLoopsLastSuccess(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	td := testutil.SetupTestPostgres(t)
	defer td.Cleanup()
	ctx := context.Background()
	marks := runRowMarks{db: td.DB, name: LoopName}
	runs := shared.NewGormRunStore(td.DB)

	lastSuccess := func() time.Time {
		t.Helper()
		var at *time.Time
		if err := td.DB.Raw("SELECT last_success_at FROM background_service_runs WHERE name = ?", LoopName).
			Scan(&at).Error; err != nil {
			t.Fatalf("read last_success_at: %v", err)
		}
		if at == nil {
			t.Fatal("last_success_at is NULL")
		}
		return *at
	}
	begin := func(label string) (time.Time, bool) {
		t.Helper()
		start, mark, ok, err := marks.Begin(ctx)
		if err != nil {
			t.Fatalf("%s: Begin: %v", label, err)
		}
		if start.IsZero() {
			t.Fatalf("%s: Begin returned no database time", label)
		}
		return mark, ok
	}
	cycle := func(err error) {
		t.Helper()
		token, claimed, claimErr := runs.Claim(ctx, LoopName, defaultInterval, runLease, true)
		if claimErr != nil || !claimed {
			t.Fatalf("claim: %v (claimed %v)", claimErr, claimed)
		}
		if err := runs.Complete(ctx, LoopName, token, shared.CycleOutcome{Err: err, Duration: time.Second}); err != nil {
			t.Fatalf("complete: %v", err)
		}
	}

	if _, ok := begin("no row"); ok {
		t.Fatal("a mark with no run row")
	}
	if err := runs.Register(ctx, LoopName, defaultInterval, runLease); err != nil {
		t.Fatalf("register: %v", err)
	}
	if _, ok := begin("registered, never run"); ok {
		t.Fatal("a mark before any successful run")
	}
	cycle(errors.New("indexnow answered HTTP 500"))
	if _, ok := begin("only a failed run"); ok {
		t.Fatal("a mark after only a failed run")
	}

	cycle(nil)
	succeededAt := lastSuccess()
	mark, ok := begin("after a success")
	if !ok || !mark.Equal(succeededAt.Add(-runLease)) {
		t.Fatalf("mark = %v (ok %v), want last_success_at %v minus the lease", mark, ok, succeededAt)
	}

	cycle(errors.New("indexnow rejected the submission: HTTP 403"))
	if again, _ := begin("after a later failure"); !again.Equal(mark) {
		t.Fatalf("a failed run moved the mark from %v to %v", mark, again)
	}
}
