package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	catalogm "psychic-homily-backend/internal/models/catalog"
)

// showOn creates an approved show at the suite's room at an exact instant.
func (s *TriageSuite) showOn(at time.Time, acts ...act) *catalogm.Show {
	slug := fmt.Sprintf("show-on-%d", s.seq.Add(1))
	sh := &catalogm.Show{Slug: &slug, EventDate: at}
	s.Require().NoError(s.db.Create(sh).Error)
	s.Require().NoError(s.db.Create(&catalogm.ShowVenue{ShowID: sh.ID, VenueID: s.venue}).Error)
	for i, a := range acts {
		s.Require().NoError(s.db.Create(&catalogm.ShowArtist{ShowID: sh.ID, ArtistID: a.artist.ID, Position: i, SetType: a.setType}).Error)
	}
	return sh
}

// The merge service drops, rather than moves, a billing that collides with the
// target's at the same room and time. A merge that would leave a show with no
// acts that way is refused, and nothing changes.
func (s *TriageSuite) TestAMergeThatWouldEmptyAShowIsRefused() {
	keep := s.artist("Collide Keep")
	dup := s.artist("Collide Keep Copy")
	at := time.Date(2027, 1, 9, 3, 0, 0, 0, time.UTC)
	lonely := s.showOn(at, act{dup, "headliner"})
	s.showOn(at, act{keep, "headliner"})

	out, summary := s.run(csvOf(*dup.Slug+",merge,"+*keep.Slug), true)

	s.True(summary.Refused(), out)
	s.Contains(out, "the merge would leave show "+*lonely.Slug+" with no acts")
	s.NotNil(s.artistByID(dup.ID))
	s.Equal([]string{"Collide Keep Copy:headliner"}, s.bill(lonely.ID))
}

// A merge whose headliner billing is dropped (the target is on the same bill
// in another role) warns that the bill loses its headliner, and the record
// carries the bill before and after.
func (s *TriageSuite) TestAMergeThatDropsTheHeadlinerWarns() {
	keep := s.artist("Headline Keep")
	dup := s.artist("Headline Keep Copy")
	sh := s.show("", act{dup, "headliner"}, act{keep, "opener"})

	out, summary := s.run(csvOf(*dup.Slug+",merge,"+*keep.Slug), true)

	s.Equal(1, summary[statusApplied], out)
	s.Contains(out, `bill ["Headline Keep Copy" [headliner], "Headline Keep" [opener]] -> ["Headline Keep" [opener]]`)
	s.Contains(out, "WARNING: the artist held the headline slot; the bill is left with no headliner")
	pending := s.recordLines()[0]
	s.Require().Len(pending.Shows, 1)
	s.Equal(sh.ID, pending.Shows[0].ShowID)
	s.Len(pending.Shows[0].BillBefore, 2)
	s.Len(pending.Shows[0].BillAfter, 1)
}

// Two splits that put the same existing act on a bill are refused: on a
// duplicate show pair the second would collide with the first at its write.
func (s *TriageSuite) TestTwoSplitsLinkingOneActAreRefused() {
	s.artist("Shared Act")
	one := s.artist("Shared Act and Other One")
	two := s.artist("Shared Act & Other Two")
	s.show("", act{s.artist("Head One"), "headliner"}, act{one, "performer"})
	s.show("", act{s.artist("Head Two"), "headliner"}, act{two, "performer"})

	rowOne := *one.Slug + ",split,Shared Act | Other One"
	rowTwo := *two.Slug + ",split,Shared Act | Other Two"
	for name, file := range map[string]string{"file order": csvOf(rowOne, rowTwo), "reversed": csvOf(rowTwo, rowOne)} {
		out, summary := s.run(file, false)
		s.Equal(2, summary[statusRefused], "%s: %s", name, out)
		s.Contains(out, "lines 2, 3 each put the same existing act on a bill", name)
	}
}

// An artist MusicBrainz identifies, by id or by a band-membership edge it
// imported, is a real act and is not deleted.
func (s *TriageSuite) TestMusicBrainzEvidenceMakesAnArtistARealAct() {
	withMBID := s.artist("MB Identified Night")
	s.Require().NoError(s.db.Model(&catalogm.Artist{}).Where("id = ?", withMBID.ID).
		Update("musicbrainz_artist_id", "5b11f4ce-a62d-471e-81fc-a69a8278c7da").Error)
	withEdge := s.artist("MB Member Night")
	s.relationship(withEdge, s.artist("MB Band"), catalogm.RelationshipTypeMemberOf, true)

	out, summary := s.run(csvOf(*withMBID.Slug+",remove,", *withEdge.Slug+",remove,"), true)

	s.Equal(2, summary[statusRefused], out)
	s.Contains(out, `"MB Identified Night" has 1 MusicBrainz id`)
	s.Contains(out, `"MB Member Night" has 1 band-membership edge(s)`)
	s.NotNil(s.artistByID(withMBID.ID))
	s.NotNil(s.artistByID(withEdge.ID))
}

// The preview warns about references a delete leaves wrong: an alert filter
// that still lists the artist.
func (s *TriageSuite) TestTheDeletePreviewWarnsAboutAlertFilters() {
	junk := s.artist("Filtered Night")
	s.show("", act{s.artist("Head"), "headliner"}, act{junk, "performer"})
	fan := s.user()
	s.Require().NoError(s.db.Exec(`INSERT INTO notification_filters (user_id, name, artist_ids) VALUES (?, 'mine', ARRAY[?]::bigint[])`,
		fan, junk.ID).Error)

	out, summary := s.run(csvOf(*junk.Slug+",remove,"), false)

	s.False(summary.Refused(), out)
	s.Contains(out, "WARNING: 1 alert filter(s) list it and will match nothing")
}

// A record file left without a final newline gets one, so the next line is
// not glued onto the torn one.
func (s *TriageSuite) TestOpenRecordEndsATornFileWithANewline() {
	path := filepath.Join(s.T().TempDir(), "torn.jsonl")
	s.Require().NoError(os.WriteFile(path, []byte(`{"a":1}`), 0o600))

	f, err := openRecord(path)
	s.Require().NoError(err)
	_, err = f.WriteString("{\"b\":2}\n")
	s.Require().NoError(err)
	s.Require().NoError(f.Close())

	got, err := os.ReadFile(path)
	s.Require().NoError(err)
	s.Equal("{\"a\":1}\n{\"b\":2}\n", string(got))
}
