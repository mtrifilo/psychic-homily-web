package indexnow

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"gorm.io/gorm"

	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/services/shared"
	"psychic-homily-backend/internal/testutil"
)

// The mark is read from the row the loop's own run store writes: absent until
// a run succeeds; last_success_at minus that run's duration and the skew
// margin while the success is the row's latest completion, so a long run
// widens the overlap; and last_success_at minus the lease and the margin once
// a later failure has overwritten the duration, or when the row has none. A
// failure never moves the mark later.
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
	cycle := func(err error, duration time.Duration) {
		t.Helper()
		token, claimed, claimErr := runs.Claim(ctx, LoopName, defaultInterval, runLease, true)
		if claimErr != nil || !claimed {
			t.Fatalf("claim: %v (claimed %v)", claimErr, claimed)
		}
		if err := runs.Complete(ctx, LoopName, token, shared.CycleOutcome{Err: err, Duration: duration}); err != nil {
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
	cycle(errors.New("indexnow answered HTTP 500"), time.Second)
	if _, ok := begin("only a failed run"); ok {
		t.Fatal("a mark after only a failed run")
	}

	cycle(nil, 2*time.Second)
	succeededAt := lastSuccess()
	mark, ok := begin("after a short success")
	if want := succeededAt.Add(-(2*time.Second + clockSkewMargin)); !ok || !mark.Equal(want) {
		t.Fatalf("mark = %v (ok %v), want %v: last_success_at minus 2s and the skew margin", mark, ok, want)
	}

	cycle(nil, 10*time.Minute)
	succeededAt = lastSuccess()
	mark, _ = begin("after a long success")
	if want := succeededAt.Add(-(10*time.Minute + clockSkewMargin)); !mark.Equal(want) {
		t.Fatalf("mark = %v, want %v: a 10-minute run widens the overlap to 11 minutes", mark, want)
	}

	cycle(errors.New("indexnow rejected the submission: HTTP 403"), time.Second)
	afterFailure, _ := begin("after a later failure")
	if want := succeededAt.Add(-(runLease + clockSkewMargin)); !afterFailure.Equal(want) {
		t.Fatalf("mark = %v, want %v: the failure's duration is not the success's, so the lease applies", afterFailure, want)
	}
	if afterFailure.After(mark) {
		t.Fatalf("a failed run moved the mark later, from %v to %v", mark, afterFailure)
	}

	cycle(nil, time.Second)
	if err := td.DB.Exec("UPDATE background_service_runs SET last_duration_ms = NULL WHERE name = ?", LoopName).Error; err != nil {
		t.Fatalf("clear duration: %v", err)
	}
	succeededAt = lastSuccess()
	if noDuration, _ := begin("success with no recorded duration"); !noDuration.Equal(succeededAt.Add(-(runLease + clockSkewMargin))) {
		t.Fatalf("mark = %v, want last_success_at %v minus the lease and the margin", noDuration, succeededAt)
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
	source := &timedSource{db: td.DB, entries: &contracts.SitemapEntries{Shows: entriesOf("a-show")}}
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

	// The accepted run selects, then takes a measurable time to finish. The
	// duration the loop records must cover the span from that selection to
	// last_success_at, or the next mark would land after the selection.
	source.delay = 1500 * time.Millisecond
	accepted := oneCycle(http.StatusAccepted)
	if len(s.received()) != 1 || !accepted.LastSuccessAt.After(*first.LastSuccessAt) {
		t.Fatalf("accepted run: %d posts, last_success_at %v (was %v)", len(s.received()), accepted.LastSuccessAt, first.LastSuccessAt)
	}
	var durationMs int64
	if err := td.DB.Raw("SELECT last_duration_ms FROM background_service_runs WHERE name = ?", LoopName).
		Scan(&durationMs).Error; err != nil {
		t.Fatalf("read last_duration_ms: %v", err)
	}
	span := accepted.LastSuccessAt.Sub(source.selectedAt)
	if recorded := time.Duration(durationMs) * time.Millisecond; recorded < span-500*time.Millisecond {
		t.Fatalf("recorded duration %v is shorter than selection-to-success %v", recorded, span)
	}
	_, mark, _, err := runRowMarks{db: td.DB}.Begin(context.Background())
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	if mark.After(source.selectedAt) {
		t.Fatalf("next mark %v is after the accepted run's selection at %v", mark, source.selectedAt)
	}
}

// timedSource returns fixed entries, records the database time each selection
// ran at, and then waits delay before returning.
type timedSource struct {
	db         *gorm.DB
	entries    *contracts.SitemapEntries
	delay      time.Duration
	selectedAt time.Time
}

func (s *timedSource) EntriesChangedSince(ctx context.Context, _ time.Time) (*contracts.SitemapEntries, error) {
	if err := s.db.WithContext(ctx).Raw("SELECT NOW()").Scan(&s.selectedAt).Error; err != nil {
		return nil, err
	}
	time.Sleep(s.delay)
	return s.entries, nil
}
