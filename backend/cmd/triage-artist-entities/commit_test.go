package main

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
)

// plannedRow plans one row of csv and returns it, accepted.
func (s *TriageSuite) plannedRow(csv string) (*triage, *rowPlan) {
	rows, err := parseWorklist(strings.NewReader(csv))
	s.Require().NoError(err)
	t := &triage{db: s.db, actor: s.actor}
	p := &rowPlan{Row: rows[0]}
	t.plan(p)
	s.Require().Equal(statusWouldApply, p.Status, strings.Join(p.Reasons, "; "))
	return t, p
}

// xidOf runs a transaction that ends by commit or rollback and returns its id.
func (s *TriageSuite) xidOf(commit bool) string {
	var xid string
	err := s.db.Transaction(func(tx *gorm.DB) error {
		s.Require().NoError(tx.Raw("SELECT txid_current()::text").Scan(&xid).Error)
		if commit {
			return nil
		}
		return errDryRunRollback
	})
	if commit {
		s.Require().NoError(err)
	}
	return xid
}

// A commit that errored is settled by the transaction's status: committed is
// applied, aborted is rolled back, and a status the database cannot give is
// OUTCOME UNKNOWN. Each answer is written to the record after the pending line.
func (s *TriageSuite) TestACommitThatErroredIsSettledByItsTransactionStatus() {
	commitStatusWait = 50 * time.Millisecond
	defer func() { commitStatusWait = 10 * time.Second }()
	commitErr := errors.New("connection reset by peer")

	for name, tc := range map[string]struct {
		xid         string
		wantStatus  rowStatus
		wantOutcome string
		wantStop    bool
	}{
		"committed": {xid: s.xidOf(true), wantStatus: statusApplied, wantOutcome: recordCommitted},
		"aborted":   {xid: s.xidOf(false), wantStatus: statusRefused, wantOutcome: recordNotApplied, wantStop: true},
		"in the future": {xid: fmt.Sprintf("%d", 1<<40), wantStatus: statusUnknown, wantOutcome: recordUnknown,
			wantStop: true},
	} {
		s.record = &memRecord{}
		a := s.artist("Commit " + name)
		t, p := s.plannedRow(csvOf(*a.Slug + ",rename,Renamed " + name))
		p.Record.Xid = tc.xid

		stop := t.resolveCommitError(p, s.recorder(), nil, commitErr)

		s.Equal(tc.wantStatus, p.Status, name)
		s.Equal(tc.wantStop, stop != nil, "%s: %v", name, stop)
		lines := s.recordLines()
		s.Require().Len(lines, 1, name)
		s.Equal(tc.wantOutcome, lines[0].Outcome, name)
		s.Equal(p.Row.Line, lines[0].Line, name)
	}
}

// A run with an OUTCOME UNKNOWN row exits non-zero and names the row as one to
// check, not as one where nothing was written.
func (s *TriageSuite) TestAnUnknownOutcomeIsReportedAsUnknown() {
	o := runOutcome{Summary: runSummary{statusApplied: 1, statusUnknown: 1},
		Applied: []string{"line 2 rename a"}, Unknown: []string{"line 3 merge b"}}
	s.True(o.Failed())
	s.Equal("Applied, and only these: line 2 rename a. Outcome unknown, so read the database for: line 3 merge b.",
		o.appliedSentence())
	var out bytes.Buffer
	printSummary(&report{w: &out}, o, true)
	s.Contains(out.String(), "OUTCOME UNKNOWN: 1")
}

// A merge counts every bill the merged artist is on, so a merge and a remove
// on the same show are entangled.
func (s *TriageSuite) TestAMergeAndARemoveOnOneBillAreRefused() {
	keep := s.artist("Bill Keep")
	dup := s.artist("Bill Keep Copy")
	junk := s.artist("Bill Junk Night")
	sh := s.show("", act{s.artist("Bill Head"), "headliner"}, act{dup, "performer"}, act{junk, "performer"})

	out, summary := s.run(csvOf(*dup.Slug+",merge,"+*keep.Slug, *junk.Slug+",remove,"), true)

	s.Equal(2, summary[statusRefused], out)
	s.Contains(out, "each rewrite the bill of show "+*sh.Slug)
	s.Len(s.bill(sh.ID), 3)
}

// A slug that now names a different artist from the one reviewed is refused.
func (s *TriageSuite) TestASlugThatNowNamesAnotherArtistIsRefused() {
	a := s.artist("Reused Slug Night")
	s.show("", act{s.artist("Head"), "headliner"}, act{a, "performer"})
	csv := fmt.Sprintf("artist_slug,action,target,artist_id\n%s,remove,,%d\n", *a.Slug, a.ID+1000)

	out, summary := s.run(csv, true)

	s.True(summary.Refused(), out)
	s.Contains(out, fmt.Sprintf("now names artist %d, not %d as reviewed", a.ID, a.ID+1000))
	s.NotNil(s.artistByID(a.ID))

	out, summary = s.run(fmt.Sprintf("artist_slug,action,target,artist_id\n%s,remove,,%d\n", *a.Slug, a.ID), true)
	s.False(summary.Refused(), out)
	s.Nil(s.artistByID(a.ID))
}

// An interrupt while planning stops the run before anything is written.
func (s *TriageSuite) TestAnInterruptWhilePlanningWritesNothing() {
	a := s.artist("Interrupted Night")
	interrupted.Store(true)
	defer interrupted.Store(false)

	out, outcome := s.runOutcome(csvOf(*a.Slug+",rename,Interrupted"), true)

	s.Require().Error(outcome.Stopped)
	s.Contains(out, "STOPPED while planning line 2: interrupted; nothing was written")
	s.True(outcome.Failed())
	s.Equal("Interrupted Night", s.artistByID(a.ID).Name)
	s.Empty(s.recordLines())
}

// The dry run takes the same record copy the confirmed run takes, so a copy
// that would fail fails in the preview.
func (s *TriageSuite) TestTheDryRunTakesTheRecordCopy() {
	a := s.artist("Copied Night")
	s.show("", act{s.artist("Head"), "headliner"}, act{a, "performer"})
	t, p := s.plannedRow(csvOf(*a.Slug + ",remove,"))
	s.NotEmpty(p.Record.Before["artists.id"], "the plan carries the copy")
	s.NotNil(t.refs, "the columns are read once and kept")
}
