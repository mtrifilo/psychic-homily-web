package main

import (
	"bytes"
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"

	catalogm "psychic-homily-backend/internal/models/catalog"
)

// Each applied row leaves one record line naming, by id and slug, what it
// changed and deleted: the bill before and after, the title written, the acts
// a split linked or created, the derived edges and the follows the delete
// dropped. The dry run writes none.
func (s *TriageSuite) TestEveryAppliedRowLeavesARecordLine() {
	head := s.artist("Ouija Macc")
	junk := s.artist("LIVE AT LAUNCHPAD")
	joined := s.artist("Darby O'Trill and Mikey Rotten")
	existing := s.artist("Darby O'Trill")
	renamed := s.artist("Riot Act (Pearl Jam Tribute)")
	keep := s.artist("Harry Katz & The Pistachios")
	dup := s.artist("Harry Katz and the Pistachios")
	shRemove := s.show("", act{junk, "headliner"}, act{head, "performer"})
	shSplit := s.show("", act{s.artist("Closer"), "headliner"}, act{joined, "opener"})
	s.show("", act{renamed, "headliner"})
	fan := s.follow(junk)
	s.relationship(junk, head, "shared_bills", true)
	csv := csvOf(
		*junk.Slug+",remove,title",
		*joined.Slug+",split,Darby O'Trill | Mikey Rotten",
		*renamed.Slug+",rename,Riot Act",
		*dup.Slug+",merge,"+*keep.Slug,
	)

	_, summary := s.run(csv, false)
	s.Require().False(summary.Refused())
	s.Empty(s.recordLines(), "a dry run records nothing")

	out, summary := s.run(csv, true)
	s.Require().Equal(4, summary[statusApplied], out)

	lines := s.recordLines()
	s.Require().Len(lines, 4)
	byAction := map[string]recordLine{}
	for _, l := range lines {
		s.Equal(recordApplied, l.Outcome)
		s.NotEmpty(l.At)
		byAction[l.Action] = l
	}

	rm := byAction[actionRemove]
	s.Equal(junk.ID, rm.ArtistID)
	s.Equal("live-at-launchpad", rm.ArtistSlug)
	s.True(rm.ArtistDeleted)
	s.Require().Len(rm.Shows, 1)
	s.Equal(shRemove.ID, rm.Shows[0].ShowID)
	s.Equal("LIVE AT LAUNCHPAD", rm.Shows[0].TitleSet)
	s.Equal([]recordAct{
		{ArtistID: junk.ID, Name: "LIVE AT LAUNCHPAD", Position: 0, SetType: "headliner"},
		{ArtistID: head.ID, Name: "Ouija Macc", Position: 1, SetType: "performer"},
	}, rm.Shows[0].BillBefore)
	s.Equal([]recordAct{{ArtistID: head.ID, Name: "Ouija Macc", Position: 0, SetType: "performer"}}, rm.Shows[0].BillAfter)
	src, tgt := catalogm.CanonicalOrder(junk.ID, head.ID)
	s.Equal([]recordEdge{{SourceArtistID: src, TargetArtistID: tgt, RelationshipType: "shared_bills"}}, rm.EdgesDeleted)
	s.Equal([]recordFollow{{UserID: fan, Action: "follow"}}, rm.FollowsDropped)

	sp := byAction[actionSplit]
	s.True(sp.ArtistDeleted)
	s.Require().Len(sp.Shows, 1)
	s.Equal(shSplit.ID, sp.Shows[0].ShowID)
	s.Equal([]recordArtist{{ID: existing.ID, Slug: *existing.Slug, Name: "Darby O'Trill"}}, sp.ActsLinked)
	s.Require().Len(sp.ActsCreated, 1)
	s.Equal("Mikey Rotten", sp.ActsCreated[0].Name)
	s.Equal("mikey-rotten", sp.ActsCreated[0].Slug)

	rn := byAction[actionRename]
	s.Equal("Riot Act", rn.NewName)
	s.False(rn.ArtistDeleted)

	mg := byAction[actionMerge]
	s.True(mg.ArtistDeleted)
	s.Equal(keep.ID, mg.KeptArtistID)
	s.Equal(*keep.Slug, mg.KeptArtistSlug)
}

// failingWriter fails every write.
type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("disk full") }

func noSync() error { return nil }

// A record line that cannot be written rolls its row back and stops the run
// before the next row.
func (s *TriageSuite) TestARecordThatCannotBeWrittenRollsTheRowBackAndStops() {
	first := s.artist("First Night")
	second := s.artist("Second Night")
	rows, err := parseWorklist(strings.NewReader(csvOf(*first.Slug+",rename,First", *second.Slug+",rename,Second")))
	s.Require().NoError(err)
	var out bytes.Buffer

	outcome := runWorklist(s.db, rows, s.actor, &recorder{w: failingWriter{}, sync: noSync, now: time.Now}, &out)

	s.Require().Error(outcome.Stopped, out.String())
	s.Contains(outcome.Stopped.Error(), "the record file could not be written")
	s.Contains(out.String(), "the record line could not be written, so the row was rolled back")
	s.Contains(out.String(), "STOPPED at line 2")
	s.Contains(out.String(), "1 row(s) after it were not run")
	s.Equal("First Night", s.artistByID(first.ID).Name, "the row is rolled back")
	s.Equal("Second Night", s.artistByID(second.ID).Name, "the next row is not run")
	s.True(outcome.Failed())
}

// A sync that fails counts as a record that was not written.
func (s *TriageSuite) TestARecordThatCannotBeSyncedRollsTheRowBack() {
	a := s.artist("Synced Night")
	rows, err := parseWorklist(strings.NewReader(csvOf(*a.Slug + ",rename,Synced")))
	s.Require().NoError(err)
	var buf, out bytes.Buffer

	outcome := runWorklist(s.db, rows, s.actor,
		&recorder{w: &buf, sync: func() error { return errors.New("fsync failed") }, now: time.Now}, &out)

	s.Require().Error(outcome.Stopped)
	s.Equal("Synced Night", s.artistByID(a.ID).Name)
}

// writeAfter accepts the first n bytes and fails every write after them.
type writeAfter struct {
	n   int
	buf bytes.Buffer
}

func (w *writeAfter) Write(p []byte) (int, error) {
	if w.buf.Len()+len(p) > w.n {
		return 0, errors.New("broken pipe")
	}
	return w.buf.Write(p)
}

// A report that cannot be written stops a confirmed run before the next row.
func (s *TriageSuite) TestAReportThatCannotBeWrittenStopsTheRun() {
	first := s.artist("Report First")
	second := s.artist("Report Second")
	rows, err := parseWorklist(strings.NewReader(csvOf(*first.Slug+",rename,Report One", *second.Slug+",rename,Report Two")))
	s.Require().NoError(err)

	outcome := runWorklist(s.db, rows, s.actor, &recorder{w: &s.record, sync: noSync, now: time.Now}, &writeAfter{n: 10})

	s.Require().Error(outcome.Stopped)
	s.Contains(outcome.Stopped.Error(), "the report could not be written")
	s.Equal("Report One", s.artistByID(first.ID).Name, "the first row was applied before the report failed")
	s.Equal("Report Second", s.artistByID(second.ID).Name, "no row runs after the report fails")
	s.Equal([]string{"line 2 rename report-first"}, outcome.Applied)
	s.Len(s.recordLines(), 1)
}

// A row the database refuses at its write (a name taken between the plan and
// the write) is rolled back, leaves no record line, and stops the run.
func (s *TriageSuite) TestARowThatFailsAtItsWriteStopsTheRun() {
	a := s.artist("Race Night")
	rows, err := parseWorklist(strings.NewReader(csvOf(*a.Slug + ",rename,Race")))
	s.Require().NoError(err)
	t := &triage{db: s.db, actor: s.actor}
	p := &rowPlan{Row: rows[0]}
	t.plan(p)
	s.Require().Equal(statusWouldApply, p.Status)
	s.artist("Race")

	stop := t.apply(p, &recorder{w: &s.record, sync: noSync, now: time.Now})

	s.Require().Error(stop)
	s.Equal(statusRefused, p.Status)
	s.Contains(strings.Join(p.Reasons, "; "), "rename failed and was rolled back")
	s.Equal("Race Night", s.artistByID(a.ID).Name)
	s.Empty(s.recordLines(), "a row refused before its record was written leaves no line")
}

// Every row's transaction carries the lock, statement and idle timeouts.
func (s *TriageSuite) TestRowTransactionsCarryTheirTimeouts() {
	err := s.db.Transaction(func(tx *gorm.DB) error {
		s.Require().NoError(setRowTimeouts(tx))
		for setting, want := range map[string]string{
			"lock_timeout":                        "5s",
			"statement_timeout":                   "1min",
			"idle_in_transaction_session_timeout": "1min",
		} {
			var got string
			s.Require().NoError(tx.Raw("SELECT current_setting(?)", setting).Scan(&got).Error)
			s.Equal(want, got, setting)
		}
		return errDryRunRollback
	})
	s.ErrorIs(err, errDryRunRollback)
}
