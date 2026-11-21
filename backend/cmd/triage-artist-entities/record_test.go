package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"strings"

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
	s.tag(junk)
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
	s.Require().Len(lines, 8, "a pending and a committed line per row")
	byAction := map[string]recordLine{}
	for i, l := range lines {
		s.NotEmpty(l.At)
		s.Equal(s.actor, l.ActorID)
		s.Equal("run-1", l.RunID)
		s.Equal("test-db", l.Database)
		if i%2 == 0 {
			s.Equal(recordPending, l.Outcome)
			byAction[l.Action] = l
			continue
		}
		s.Equal(recordCommitted, l.Outcome)
		s.Equal(lines[i-1].Line, l.Line)
		s.Equal(lines[i-1].ArtistID, l.ArtistID)
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
	// The copy taken before the write: the artist's row, its bill, its derived
	// edge and its tag.
	s.Contains(string(rm.Before["artists.id"]), `"name":"LIVE AT LAUNCHPAD"`)
	s.Contains(string(rm.Before["show_artists.artist_id"]), fmt.Sprintf(`"show_id":%d`, shRemove.ID))
	src, tgt := catalogm.CanonicalOrder(junk.ID, head.ID)
	edges := string(rm.Before["artist_relationships.source_artist_id"]) + string(rm.Before["artist_relationships.target_artist_id"])
	s.Contains(edges, fmt.Sprintf(`"source_artist_id":%d`, src))
	s.Contains(edges, fmt.Sprintf(`"target_artist_id":%d`, tgt))
	s.Contains(string(rm.Before["entity_tags.entity_id"]), fmt.Sprintf(`"added_by_user_id":%d`, s.actor))

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

	s.Empty(rn.Before, "a rename deletes nothing, so it copies nothing")

	mg := byAction[actionMerge]
	s.True(mg.ArtistDeleted)
	s.Equal(keep.ID, mg.KeptArtistID)
	s.Equal(*keep.Slug, mg.KeptArtistSlug)
	s.Contains(string(mg.Before["artists.id"]), `"name":"Harry Katz and the Pistachios"`)
}

// The copy covers every foreign key to artists in the live schema, so a
// migration that adds one is copied without a change here.
func (s *TriageSuite) TestTheRecordCopyCoversEveryForeignKeyToArtists() {
	refs, err := artistReferenceColumns(s.db)
	s.Require().NoError(err)
	covered := map[string]bool{}
	for _, r := range refs {
		covered[r.key] = true
	}
	var fks []string
	s.Require().NoError(s.db.Raw(`
		SELECT cl.relname || '.' || att.attname FROM pg_constraint con
		JOIN pg_class cl ON cl.oid = con.conrelid
		JOIN pg_attribute att ON att.attrelid = con.conrelid AND att.attnum = con.conkey[1]
		WHERE con.contype = 'f' AND con.confrelid = 'artists'::regclass`).Scan(&fks).Error)
	s.Require().NotEmpty(fks)
	for _, fk := range fks {
		s.True(covered[fk], "the record does not copy %s", fk)
	}
	for _, want := range []string{"user_bookmarks.entity_id", "entity_tags.entity_id", "collection_items.entity_id",
		"artist_releases.artist_id", "artist_aliases.artist_id", "notification_filters.artist_ids"} {
		s.True(covered[want], "the record does not copy %s", want)
	}
}

// A row whose artist is deleted between the plan and the write is refused, not
// reported applied, leaves no record line, and stops the run.
func (s *TriageSuite) TestARowWhoseArtistVanishesBeforeItsWriteIsRefused() {
	a := s.artist("Vanishing Night")
	other := s.show("", act{s.artist("Stays"), "headliner"}, act{a, "performer"})
	rows, err := parseWorklist(strings.NewReader(csvOf(*a.Slug + ",remove,")))
	s.Require().NoError(err)
	t := &triage{db: s.db, actor: s.actor}
	p := &rowPlan{Row: rows[0]}
	t.plan(p)
	s.Require().Equal(statusWouldApply, p.Status)
	s.Require().NoError(s.db.Exec("DELETE FROM show_artists WHERE artist_id = ?", a.ID).Error)
	s.Require().NoError(s.db.Exec("DELETE FROM artists WHERE id = ?", a.ID).Error)

	stop := t.apply(p, s.recorder())

	s.Require().Error(stop)
	s.Equal(statusRefused, p.Status)
	s.Contains(strings.Join(p.Reasons, "; "), "no longer exists")
	s.Empty(s.recordLines())
	s.Equal([]string{"Stays:headliner"}, s.bill(other.ID))
}

// A row whose artist was renamed after review is refused.
func (s *TriageSuite) TestARowWhoseArtistWasRenamedSinceReviewIsRefused() {
	a := s.artist("Renamed Since Review")
	s.show("", act{s.artist("Head"), "headliner"}, act{a, "performer"})
	csv := "artist_slug,action,target,name\n" + *a.Slug + ",remove,,Foo Presents\n"

	out, summary := s.run(csv, true)

	s.True(summary.Refused(), out)
	s.Contains(out, `is now named "Renamed Since Review", not "Foo Presents" as reviewed`)
	s.NotNil(s.artistByID(a.ID))

	out, summary = s.run("artist_slug,action,target,name\n"+*a.Slug+",remove,,Renamed Since Review\n", true)
	s.False(summary.Refused(), out)
	s.Nil(s.artistByID(a.ID))
}

// A merge into an artist that a split links on another bill is entangled: in
// either order one of them would fail at its write. Both are refused, in a
// confirmed run, whichever order the file lists them.
func (s *TriageSuite) TestAMergeIntoAnActASplitLinksIsRefusedInEitherOrder() {
	x := s.artist("Hoopla")
	joined := s.artist("Hoopla & Gnarwhal")
	z := s.artist("The Hoopla")
	sh := s.show("", act{joined, "headliner"}, act{s.artist("Opener"), "performer"})
	s.show("", act{z, "headliner"})
	split := *joined.Slug + ",split,Hoopla | Gnarwhal"
	merge := *z.Slug + ",merge," + *x.Slug

	for name, file := range map[string]string{"split first": csvOf(split, merge), "merge first": csvOf(merge, split)} {
		out, summary := s.run(file, true)
		s.Equal(2, summary[statusRefused], "%s: %s", name, out)
		s.Equal(0, summary[statusApplied], "%s: %s", name, out)
		s.Equal([]string{"Hoopla & Gnarwhal:headliner", "Opener:performer"}, s.bill(sh.ID), name)
		s.Contains(out, "receives the merge on line", name)
		s.NotContains(out, "each rewrite the bill of show", name)
	}
}

// The record file is refused when it is a symlink or others can read it.
func (s *TriageSuite) TestTheRecordFileMustBePrivateAndRegular() {
	dir := s.T().TempDir()
	open := dir + "/open.jsonl"
	s.Require().NoError(os.WriteFile(open, nil, 0o644))
	_, err := openRecord(open)
	s.ErrorContains(err, "readable by its owner only")

	target := dir + "/target.jsonl"
	s.Require().NoError(os.WriteFile(target, nil, 0o600))
	link := dir + "/link.jsonl"
	s.Require().NoError(os.Symlink(target, link))
	_, err = openRecord(link)
	s.ErrorContains(err, "cannot be opened for append")

	f, err := openRecord(dir + "/new.jsonl")
	s.Require().NoError(err)
	info, err := f.Stat()
	s.Require().NoError(err)
	s.Equal(os.FileMode(0o600), info.Mode().Perm())
	s.Require().NoError(f.Close())
}

// A record line that cannot be written rolls its row back and stops the run
// before the next row.
func (s *TriageSuite) TestARecordThatCannotBeWrittenRollsTheRowBackAndStops() {
	first := s.artist("First Night")
	second := s.artist("Second Night")
	rows, err := parseWorklist(strings.NewReader(csvOf(*first.Slug+",rename,First", *second.Slug+",rename,Second")))
	s.Require().NoError(err)
	var out bytes.Buffer

	s.record.failWrite, s.record.partial = true, 25
	outcome := runWorklist(s.db, rows, s.actor, s.recorder(), &out, s.reviewed(rows))
	s.Empty(s.record.buf.String(), "the partial line is cut back off the file")

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
	var out bytes.Buffer
	s.record.failSync = true

	outcome := runWorklist(s.db, rows, s.actor, s.recorder(), &out, s.reviewed(rows))

	s.Require().Error(outcome.Stopped)
	s.Equal("Synced Night", s.artistByID(a.ID).Name)
	lines := s.recordLines()
	s.Require().Len(lines, 2, "the unsynced pending line, then a not_applied line")
	s.Equal(recordPending, lines[0].Outcome)
	s.Equal(recordNotApplied, lines[1].Outcome)
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

	outcome := runWorklist(s.db, rows, s.actor, s.recorder(), &writeAfter{n: 10}, s.reviewed(rows))

	s.Require().Error(outcome.Stopped)
	s.Contains(outcome.Stopped.Error(), "the report could not be written")
	s.Equal("Report One", s.artistByID(first.ID).Name, "the first row was applied before the report failed")
	s.Equal("Report Second", s.artistByID(second.ID).Name, "no row runs after the report fails")
	s.Equal([]string{"line 2 rename report-first"}, outcome.Applied)
	s.Len(s.recordLines(), 2, "the applied row's pending and committed lines")
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

	stop := t.apply(p, s.recorder())

	s.Require().Error(stop)
	s.Equal(statusRefused, p.Status)
	s.Contains(strings.Join(p.Reasons, "; "), "rename failed and was rolled back")
	s.Equal("Race Night", s.artistByID(a.ID).Name)
	s.Empty(s.recordLines(), "a row refused before its record was written leaves no line")
}

// Every row's transaction carries the lock, statement and idle timeouts: read
// back inside a real row's transaction, through runRow.
func (s *TriageSuite) TestRowTransactionsCarryTheirTimeouts() {
	a := s.artist("Timed Night")
	rows, err := parseWorklist(strings.NewReader(csvOf(*a.Slug + ",rename,Timed")))
	s.Require().NoError(err)
	t := &triage{db: s.db, actor: s.actor}
	p := &rowPlan{Row: rows[0], SubjectID: a.ID}
	_, err = t.runRow(p, func(tx *gorm.DB) error {
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
	s.Equal("Timed Night", s.artistByID(a.ID).Name)
}
