package catalog

import (
	"context"
	"fmt"
	"strings"
	"time"

	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/contracts"
)

// Track candidate cases on RadioCandidatesIntegrationSuite. Every case
// rebuilds radio_tracks after seeding, as the rebuild loop would.

const (
	radioTrackTestReleaseA = "aaaaaaaa-1111-4111-8111-111111111111"
	radioTrackTestReleaseB = "bbbbbbbb-1111-4111-8111-111111111111"
	radioTrackTestRecord   = "dddddddd-1111-4111-8111-111111111111"
)

func (s *RadioCandidatesIntegrationSuite) rebuildTracks() {
	report, err := RebuildRadioTracks(context.Background(), s.db)
	s.Require().NoError(err)
	s.Require().False(report.Skipped)
}

func (s *RadioCandidatesIntegrationSuite) listTracks(view contracts.RadioTrackCandidateView, lane contracts.RadioCandidateLaneFilter, sort contracts.RadioCandidateSort, limit int) *contracts.RadioTrackCandidateListing {
	listing, err := s.reader.ListTrackCandidates(context.Background(), contracts.RadioTrackCandidatesQuery{View: view, Lane: lane, Sort: sort, Limit: limit})
	s.Require().NoError(err)
	return listing
}

// tracks returns a view's candidates, every lane, keyed by display title.
func (s *RadioCandidatesIntegrationSuite) tracks(view contracts.RadioTrackCandidateView) map[string]contracts.RadioTrackCandidate {
	listing := s.listTracks(view, contracts.RadioCandidateLaneFilterAll, contracts.RadioCandidateSortScore, 500)
	byTitle := map[string]contracts.RadioTrackCandidate{}
	for _, c := range listing.Candidates {
		byTitle[c.DisplayTitle] = c
	}
	return byTitle
}

func (s *RadioCandidatesIntegrationSuite) createRelease(title string) uint {
	slug := fmt.Sprintf("t-release-%d", time.Now().UnixNano())
	r := &catalogm.Release{Title: title, Slug: &slug}
	s.Require().NoError(s.db.Create(r).Error)
	return r.ID
}

func (s *RadioCandidatesIntegrationSuite) TestTrackListingReportsWindowAndFloors() {
	listing := s.listTracks(contracts.RadioTrackCandidateViewUnlinked, contracts.RadioCandidateLaneFilterAll, contracts.RadioCandidateSortScore, 10)
	s.Equal(contracts.RadioTrackCandidateViewUnlinked, listing.View)
	s.Equal("2026-07-31", listing.AsOf)
	s.Equal(radioCandidateTestWindowStart, listing.WindowStart)
	s.Equal(180, listing.WindowDays)
	s.Equal(2, listing.MinStationRows)
	s.Equal(3, listing.MinPlays)
	s.NotNil(listing.Candidates)
	s.Empty(listing.Candidates)
	s.Zero(listing.Total)
	s.Zero(listing.QualifyingTracks)
	s.Nil(listing.TracksRebuiltAt)
}

func (s *RadioCandidatesIntegrationSuite) TestTrackFloorsAndWindowEdge() {
	band := candidatePlay{artist: "T Band"}
	with := func(title, station, date string) candidatePlay {
		p := band
		p.title, p.station, p.date = title, station, date
		return p
	}
	// Two station rows, three plays: in.
	s.add(with("Pair Song", "t-kexp", ""), 2)
	s.add(with("Pair Song", "t-wfmu", ""), 1)
	// Ten plays on one station row: out.
	s.add(with("Lonely Song", "t-kexp", ""), 10)
	// Two station rows, two plays: out.
	s.add(with("Short Song", "t-kexp", ""), 1)
	s.add(with("Short Song", "t-wfmu", ""), 1)
	// Two WFMU streams are two station rows of one family.
	s.add(with("Network Song", "t-wfmu", ""), 2)
	s.add(with("Network Song", "t-rocknsoul", ""), 1)
	// The window's first day counts; the day before does not.
	s.add(with("Edge In", "t-kexp", ""), 1)
	s.add(with("Edge In", "t-wfmu", ""), 1)
	s.add(with("Edge In", "t-kexp", radioCandidateTestWindowStart), 1)
	s.add(with("Edge Out", "t-kexp", ""), 1)
	s.add(with("Edge Out", "t-wfmu", ""), 1)
	s.add(with("Edge Out", "t-kexp", radioCandidateTestBeforeStart), 1)
	s.rebuildTracks()

	got := s.tracks(contracts.RadioTrackCandidateViewUnmatched)
	s.Contains(got, "Pair Song")
	s.NotContains(got, "Lonely Song", "ten plays on one station row never qualify")
	s.NotContains(got, "Short Song", "two plays are under the play floor")
	s.Require().Contains(got, "Network Song")
	s.Equal(2, got["Network Song"].StationRowCount)
	s.Equal(1, got["Network Song"].StationFamilyCount)
	s.Require().Contains(got, "Edge In")
	s.Equal(3, got["Edge In"].PlayCount)
	s.Equal(radioCandidateTestWindowStart, got["Edge In"].FirstAiredOn)
	s.NotContains(got, "Edge Out", "a play the day before the window does not count")
	s.Len(got, 3)
}

func (s *RadioCandidatesIntegrationSuite) TestTrackExcludedAndAmbiguousPlaysAreOut() {
	s.addAcross(candidatePlay{artist: "T Band", title: "Real Song"}, 2)
	// Placeholder and talk titles never form a track.
	s.addAcross(candidatePlay{artist: "T Band", title: "Unknown"}, 3)
	s.addAcross(candidatePlay{artist: "T Band", title: "Your DJ speaks"}, 3)
	// A placeholder artist name excludes the play for the track ranking too.
	s.addAcross(candidatePlay{artist: "Various Artists", title: "Compiled Song"}, 3)
	// Ambiguous plays do not count, so this track has one play per row.
	s.addAcross(candidatePlay{artist: "T Band", title: "Ambiguous Song", matchState: catalogm.RadioPlayMatchStateAmbiguous}, 2)
	s.addAcross(candidatePlay{artist: "T Band", title: "Ambiguous Song"}, 1)
	// An unkeyable artist name maps to no track, even when the title is
	// unkeyable too and the play's key is the unkeyed marker's; a blank title
	// is excluded.
	s.addAcross(candidatePlay{artist: "東京", title: "Song"}, 2)
	s.addAcross(candidatePlay{artist: "東京", title: "大阪"}, 2)
	s.addAcross(candidatePlay{artist: "T Band", title: ""}, 2)

	// A station-scoped rule removes the WFMU plays of this track, leaving
	// one station row.
	s.addAcross(candidatePlay{artist: "T Band", title: "Scoped Song"}, 3)
	s.Require().NoError(s.db.Exec(
		`INSERT INTO radio_play_exclusions (match_kind, pattern_type, pattern, category, reason, station_id)
		 VALUES ('track_title', 'exact', 'scoped song', 'talk', 'test', ?)`, s.stations["t-wfmu"]).Error)
	defer func() {
		s.Require().NoError(s.db.Exec(`DELETE FROM radio_play_exclusions WHERE reason = 'test'`).Error)
	}()
	s.rebuildTracks()

	listing := s.listTracks(contracts.RadioTrackCandidateViewUnmatched, contracts.RadioCandidateLaneFilterAll, contracts.RadioCandidateSortScore, 50)
	titles := []string{}
	for _, c := range listing.Candidates {
		titles = append(titles, c.DisplayTitle)
	}
	s.Equal([]string{"Real Song"}, titles)
	s.Equal(1, listing.QualifyingTracks)
	s.Equal(int64(8), listing.UntrackedPlays, "the unkeyable artist's plays count toward no track")

	// A play newer than the last rebuild has no radio_track_keys row yet.
	s.add(candidatePlay{station: "t-kexp", artist: "T Band", title: "Brand New Song"}, 1)
	listing = s.listTracks(contracts.RadioTrackCandidateViewUnmatched, contracts.RadioCandidateLaneFilterAll, contracts.RadioCandidateSortScore, 50)
	s.Equal(int64(9), listing.UntrackedPlays)
}

func (s *RadioCandidatesIntegrationSuite) TestTrackViewSplit() {
	known := s.createArtist("T Known", nil)
	s.createArtist("T Lagging", nil)
	release := s.createRelease("T Known Album")

	// A catalog artist's track with no release link: the unlinked view.
	s.addAcross(candidatePlay{artist: "T Known", title: "Known Song", artistID: &known}, 2)
	// One counted play linked to a release removes the track.
	s.addAcross(candidatePlay{artist: "T Known", title: "Released Song", artistID: &known}, 2)
	s.add(candidatePlay{station: "t-kexp", artist: "T Known", title: "Released Song", artistID: &known, releaseID: &release}, 1)
	// A release link on a play before the window is not a counted play.
	s.addAcross(candidatePlay{artist: "T Known", title: "Old Release Song", artistID: &known}, 2)
	s.add(candidatePlay{station: "t-kexp", artist: "T Known", title: "Old Release Song", artistID: &known, releaseID: &release, date: radioCandidateTestBeforeStart}, 1)
	// Some counted plays linked, some not: still the catalog artist's track.
	s.addAcross(candidatePlay{artist: "T Known", title: "Mixed Song", artistID: &known}, 1)
	s.add(candidatePlay{station: "t-kexp", artist: "T Known", title: "Mixed Song"}, 1)
	// An artist the catalog does not have: the unmatched view.
	s.addAcross(candidatePlay{artist: "T Newcomer", title: "New Song"}, 2)
	// Unlinked plays of an artist the catalog has by name: neither view.
	s.addAcross(candidatePlay{artist: "T Lagging", title: "Lagging Song"}, 2)
	s.rebuildTracks()

	unlinked := s.listTracks(contracts.RadioTrackCandidateViewUnlinked, contracts.RadioCandidateLaneFilterAll, contracts.RadioCandidateSortScore, 50)
	unmatched := s.listTracks(contracts.RadioTrackCandidateViewUnmatched, contracts.RadioCandidateLaneFilterAll, contracts.RadioCandidateSortScore, 50)
	titles := func(l *contracts.RadioTrackCandidateListing) []string {
		out := []string{}
		for _, c := range l.Candidates {
			out = append(out, c.DisplayTitle)
		}
		return out
	}
	s.ElementsMatch([]string{"Known Song", "Old Release Song", "Mixed Song"}, titles(unlinked))
	s.Equal([]string{"New Song"}, titles(unmatched))
	s.Equal(6, unlinked.QualifyingTracks)
	s.Equal(6, unmatched.QualifyingTracks)
	s.Equal(3, unlinked.Total)
	s.Equal(1, unmatched.Total)

	for _, c := range unlinked.Candidates {
		s.Require().Len(c.Artists, 1, c.DisplayTitle)
		s.Equal(known, c.Artists[0].ArtistID)
		s.Equal("T Known", c.Artists[0].Name)
		s.True(strings.HasPrefix(c.Artists[0].Slug, "t-artist-"))
	}
	byTitle := map[string]contracts.RadioTrackCandidate{}
	for _, c := range unlinked.Candidates {
		byTitle[c.DisplayTitle] = c
	}
	s.Equal(2, byTitle["Mixed Song"].Artists[0].PlayCount, "only the linked plays count toward the artist")
	s.Equal(3, byTitle["Mixed Song"].PlayCount)

	newSong := unmatched.Candidates[0]
	s.Empty(newSong.Artists)
	s.Equal([]contracts.RadioCandidateName{{Name: "t newcomer", PlayCount: 4}}, newSong.ArtistKeys)
	s.Equal("T Newcomer", newSong.DisplayArtist)
}

func (s *RadioCandidatesIntegrationSuite) TestTrackUnmatchedViewNeedsEveryArtistKeyToBeACandidate() {
	// A MusicBrainz recording id joins two spellings into one track; both
	// keys clear the artist floors.
	s.addAcross(candidatePlay{artist: "T Joined", title: "Joined Song", mbRecording: radioTrackTestRecord}, 2)
	s.addAcross(candidatePlay{artist: "T Joined Band", title: "Joined Song", mbRecording: radioTrackTestRecord}, 2)
	// Here one spelling has a single play, so its key is no artist candidate.
	const other = "eeeeeeee-1111-4111-8111-111111111111"
	s.addAcross(candidatePlay{artist: "T Half", title: "Half Song", mbRecording: other}, 2)
	s.add(candidatePlay{station: "t-kexp", artist: "T Half And Friend", title: "Half Song", mbRecording: other}, 1)
	s.rebuildTracks()

	listing := s.listTracks(contracts.RadioTrackCandidateViewUnmatched, contracts.RadioCandidateLaneFilterAll, contracts.RadioCandidateSortScore, 50)
	s.Require().Len(listing.Candidates, 1)
	c := listing.Candidates[0]
	s.Equal("Joined Song", c.DisplayTitle)
	s.Equal(8, c.PlayCount)
	s.ElementsMatch([]contracts.RadioCandidateName{{Name: "t joined", PlayCount: 4}, {Name: "t joined band", PlayCount: 4}}, c.ArtistKeys)
	s.Equal(2, listing.QualifyingTracks, "the half-candidate track qualifies but is in neither view")
}

func (s *RadioCandidatesIntegrationSuite) TestTrackLanes() {
	upperA := strings.ToUpper(radioTrackTestReleaseA)
	// Every play carries the same release id: auto.
	s.addAcross(candidatePlay{artist: "T Band", title: "One Release", mbRelease: radioTrackTestReleaseA}, 2)
	// One play carries the id in capitals, the rest none: auto, lower-cased.
	s.addAcross(candidatePlay{artist: "T Band", title: "Some Release"}, 2)
	s.add(candidatePlay{station: "t-kexp", artist: "T Band", title: "Some Release", mbRelease: upperA}, 1)
	// Two release ids: human, both listed.
	s.addAcross(candidatePlay{artist: "T Band", title: "Two Releases", mbRelease: radioTrackTestReleaseA}, 1)
	s.add(candidatePlay{station: "t-kexp", artist: "T Band", title: "Two Releases", mbRelease: radioTrackTestReleaseB}, 1)
	// No id, or an id that is not shaped like a UUID: human.
	s.addAcross(candidatePlay{artist: "T Band", title: "No Release"}, 2)
	s.addAcross(candidatePlay{artist: "T Band", title: "Bad Release", mbRelease: "not-a-uuid"}, 2)
	// A title in a script normalization may not fold: human, flagged.
	s.addAcross(candidatePlay{artist: "T Band", title: "Song 東京", mbRelease: radioTrackTestReleaseA}, 2)
	s.rebuildTracks()

	got := s.tracks(contracts.RadioTrackCandidateViewUnmatched)
	auto, human := contracts.RadioCandidateLaneAuto, contracts.RadioCandidateLaneHuman
	for title, want := range map[string]contracts.RadioCandidateLane{
		"One Release": auto, "Some Release": auto, "Two Releases": human,
		"No Release": human, "Bad Release": human, "Song 東京": human,
	} {
		s.Require().Contains(got, title)
		s.Equal(want, got[title].Lane, title)
	}
	s.Require().NotNil(got["Some Release"].MBReleaseID)
	s.Equal(radioTrackTestReleaseA, *got["Some Release"].MBReleaseID)
	s.Equal([]string{radioTrackTestReleaseA, radioTrackTestReleaseB}, got["Two Releases"].MBReleaseIDs)
	s.Nil(got["Two Releases"].MBReleaseID)
	s.Empty(got["Bad Release"].MBReleaseIDs)
	s.True(got["Song 東京"].NonLatinSpelling)
	s.False(got["One Release"].NonLatinSpelling)

	autoOnly := s.listTracks(contracts.RadioTrackCandidateViewUnmatched, contracts.RadioCandidateLaneFilterAuto, contracts.RadioCandidateSortScore, 50)
	s.Equal(2, autoOnly.Total)
	s.Equal(2, autoOnly.AutoTotal)
	s.Equal(4, autoOnly.HumanTotal)
	for _, c := range autoOnly.Candidates {
		s.Equal(auto, c.Lane)
	}
	limited := s.listTracks(contracts.RadioTrackCandidateViewUnmatched, contracts.RadioCandidateLaneFilterHuman, contracts.RadioCandidateSortPlays, 1)
	s.Len(limited.Candidates, 1)
	s.Equal(4, limited.Total)
}

func (s *RadioCandidatesIntegrationSuite) TestTrackEvidence() {
	known := s.createArtist("T Evidence", nil)
	add := func(station, show, title, album, mbRelease string, linked bool, n int) {
		p := candidatePlay{station: station, show: show, artist: "T Evidence", title: title, album: album, mbRelease: mbRelease}
		if linked {
			p.artistID = &known
		}
		s.add(p, n)
	}
	add("t-kexp", "a", "Song One", "Album One", radioTrackTestReleaseB, true, 4)
	add("t-kexp", "b", "Song One (Live)", "album one", radioTrackTestReleaseA, true, 1)
	add("t-wfmu", "a", "song one", "Other Album", "", true, 2)
	add("t-rocknsoul", "a", "Song One", "", "", true, 1)
	s.add(candidatePlay{station: "t-rocknsoul", show: "a", artist: "T EVIDENCE", title: "Song One", artistID: &known}, 1)
	s.rebuildTracks()

	got := s.tracks(contracts.RadioTrackCandidateViewUnlinked)
	s.Require().Contains(got, "Song One")
	c := got["Song One"]
	s.Equal(9, c.PlayCount)
	s.Equal(3, c.StationRowCount)
	s.Equal(2, c.StationFamilyCount)
	s.Equal(4, c.ShowCount)
	s.InDelta(RadioTrackCandidateScore(3, 9), c.Score, 1e-9)
	s.Equal(3, c.TitleSpellingCount)
	s.Equal([]contracts.RadioCandidateName{
		{Name: "Song One", PlayCount: 6}, {Name: "song one", PlayCount: 2}, {Name: "Song One (Live)", PlayCount: 1},
	}, c.TitleSpellings)
	s.Equal("T Evidence", c.DisplayArtist)
	s.Equal([]contracts.RadioCandidateName{{Name: "t evidence", PlayCount: 9}}, c.ArtistKeys)
	s.Require().Len(c.Artists, 1)
	s.Equal(known, c.Artists[0].ArtistID)
	s.Equal(9, c.Artists[0].PlayCount)
	s.Equal([]contracts.RadioCandidateStation{
		{StationID: s.stations["t-kexp"], Slug: "t-kexp", Name: "t-kexp name", PlayCount: 5},
		{StationID: s.stations["t-wfmu"], Slug: "t-wfmu", Name: "t-wfmu name", PlayCount: 2},
		{StationID: s.stations["t-rocknsoul"], Slug: "t-rocknsoul", Name: "t-rocknsoul name", PlayCount: 2},
	}, c.Stations, "equal play counts order by station id")
	s.Equal(2, c.AlbumNameCount, "album names that normalize alike are one entry")
	s.Equal([]contracts.RadioCandidateName{{Name: "Album One", PlayCount: 5}, {Name: "Other Album", PlayCount: 2}}, c.AlbumNames)
	s.Equal([]string{radioTrackTestReleaseA, radioTrackTestReleaseB}, c.MBReleaseIDs)
	s.Equal(contracts.RadioCandidateLaneHuman, c.Lane)
}
