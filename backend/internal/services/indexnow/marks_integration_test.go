package indexnow

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"psychic-homily-backend/internal/services/contracts"
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
	marks := runRowMarks{db: td.DB}
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

// The pinger's own cycles, run by the shared loop against the real run store,
// move the mark: a first run records one without posting, a rejected
// submission leaves it and puts the counts in last_error, and an accepted one
// moves it.
func TestPingerCyclesMoveTheMarkOnlyOnSuccess(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	td := testutil.SetupTestPostgres(t)
	defer td.Cleanup()
	shared.SetDefaultRunStore(shared.NewGormRunStore(td.DB))
	t.Cleanup(func() { shared.SetDefaultRunStore(nil) })

	s := newStub(t, http.StatusOK)
	source := &fixedSource{entries: &contracts.SitemapEntries{Shows: entriesOf("a-show")}}
	p := testPinger(t, s.server.URL, source, runRowMarks{db: td.DB})

	type runRow struct {
		RunCount      int64
		LastOutcome   *string
		LastSuccessAt *time.Time
		LastError     *string
	}
	readRow := func() runRow {
		t.Helper()
		var row runRow
		if err := td.DB.Raw(
			"SELECT run_count, last_outcome, last_success_at, last_error FROM background_service_runs WHERE name = ?",
			LoopName,
		).Scan(&row).Error; err != nil {
			t.Fatalf("read run row: %v", err)
		}
		return row
	}
	// oneCycle runs the loop until it completes one boot cycle, then stops it.
	oneCycle := func(status int) runRow {
		t.Helper()
		s.mu.Lock()
		s.statuses = []int{status}
		s.bodies = nil
		s.mu.Unlock()
		before := readRow().RunCount
		cfg := p.loopConfig()
		if cfg.Store == nil {
			t.Fatal("the loop config carries no run store, so the mark could never move")
		}
		cfg.RunAtBoot = true
		cfg.StopCh = nil
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan struct{})
		go func() {
			defer close(done)
			shared.RunScheduledLoop(ctx, cfg, p.runCycle)
		}()
		deadline := time.Now().Add(10 * time.Second)
		for readRow().RunCount == before {
			if time.Now().After(deadline) {
				cancel()
				<-done
				t.Fatal("the cycle did not complete")
			}
			time.Sleep(20 * time.Millisecond)
		}
		cancel()
		<-done
		return readRow()
	}

	first := oneCycle(http.StatusOK)
	if first.LastSuccessAt == nil || len(s.received()) != 0 {
		t.Fatalf("first run: last_success_at %v, %d posts; want a mark and no post", first.LastSuccessAt, len(s.received()))
	}

	rejected := oneCycle(http.StatusForbidden)
	if len(s.received()) != 1 {
		t.Fatalf("rejected run posted %d times, want 1", len(s.received()))
	}
	if rejected.LastOutcome == nil || *rejected.LastOutcome != shared.RunOutcomeError {
		t.Fatalf("rejected run outcome = %v, want error", rejected.LastOutcome)
	}
	if !rejected.LastSuccessAt.Equal(*first.LastSuccessAt) {
		t.Fatalf("a rejected run moved last_success_at from %v to %v", first.LastSuccessAt, rejected.LastSuccessAt)
	}
	wantError := "indexnow rejected the submission: HTTP 403 (selected 1, submitted 0, batches 1, last status 403)"
	if rejected.LastError == nil || *rejected.LastError != wantError {
		t.Fatalf("last_error = %v, want %q", rejected.LastError, wantError)
	}

	accepted := oneCycle(http.StatusAccepted)
	if len(s.received()) != 1 || !accepted.LastSuccessAt.After(*first.LastSuccessAt) {
		t.Fatalf("accepted run: %d posts, last_success_at %v (was %v)", len(s.received()), accepted.LastSuccessAt, first.LastSuccessAt)
	}
}
