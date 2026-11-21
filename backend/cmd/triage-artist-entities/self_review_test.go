package main

import (
	"bytes"
	"strings"
)

// A festival lineup marks a real act: the artist is not deleted.
func (s *TriageSuite) TestRemoveOfAnArtistOnAFestivalLineupIsRefused() {
	a := s.artist("Lineup Night")
	var festivalID uint
	s.Require().NoError(s.db.Raw(`INSERT INTO festivals (name, slug, series_slug, edition_year, start_date, end_date)
		VALUES ('Fest', 'fest-2027', 'fest', 2027, '2027-06-01', '2027-06-03') RETURNING id`).Scan(&festivalID).Error)
	s.Require().NoError(s.db.Exec(`INSERT INTO festival_artists (festival_id, artist_id) VALUES (?, ?)`, festivalID, a.ID).Error)

	out, summary := s.run(csvOf(*a.Slug+",remove,"), true)

	s.True(summary.Refused(), out)
	s.Contains(out, `"Lineup Night" has 1 festival lineup(s)`)
	s.NotNil(s.artistByID(a.ID))
}

// A remove that would write a title refuses an artist on no bill, and a name
// longer than a show title holds.
func (s *TriageSuite) TestRemoveWithTitleRefusesWhenThereIsNoTitleToWrite() {
	orphan := s.artist("No Bill Night")
	// Artist names hold 255 characters; the title check counts bytes, as the
	// show update handler does, so a long name of two-byte letters trips it.
	long := s.artist("Long " + strings.Repeat("\u00e9", 130))
	s.show("", act{long, "headliner"}, act{s.artist("Long Head"), "performer"})

	out, summary := s.run(csvOf(*orphan.Slug+",remove,title", *long.Slug+",remove,title"), true)

	s.Equal(2, summary[statusRefused], out)
	s.Contains(out, "is on no bill, so there is no show title to write")
	s.Contains(out, "longer than the 255 bytes the show update accepts for a title")
	s.NotNil(s.artistByID(orphan.ID))
	s.NotNil(s.artistByID(long.ID))
}

// A row whose commit returned but whose committed line cannot be written stops
// the run: the row is applied and its pending line is the record.
func (s *TriageSuite) TestACommittedLineThatCannotBeWrittenStopsTheRun() {
	first := s.artist("Committed First")
	second := s.artist("Committed Second")
	rows, err := parseWorklist(strings.NewReader(csvOf(*first.Slug+",rename,Committed One", *second.Slug+",rename,Committed Two")))
	s.Require().NoError(err)
	s.record.failAfterLines = 1
	var out bytes.Buffer

	outcome := runWorklist(s.db, rows, s.actor, s.recorder(), &out, s.reviewed(rows))

	s.Require().Error(outcome.Stopped, out.String())
	s.Contains(outcome.Stopped.Error(), "committed, but the record could not say so")
	s.Equal("Committed One", s.artistByID(first.ID).Name, "the row is applied")
	s.Equal("Committed Second", s.artistByID(second.ID).Name, "the next row is not run")
	lines := s.recordLines()
	s.Require().Len(lines, 1)
	s.Equal(recordPending, lines[0].Outcome)
}
