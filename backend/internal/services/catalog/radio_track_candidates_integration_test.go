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

// createRelease creates a release credited to the given artists.
func (s *RadioCandidatesIntegrationSuite) createRelease(title string, artistIDs ...uint) uint {
	slug := fmt.Sprintf("t-release-%d", time.Now().UnixNano())
	r := &catalogm.Release{Title: title, Slug: &slug}
	s.Require().NoError(s.db.Create(r).Error)
	for i, id := range artistIDs {
		s.Require().NoError(s.db.Create(&catalogm.ArtistRelease{ArtistID: id, ReleaseID: r.ID, Position: i}).Error)
	}
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
	other := s.createArtist("T Other", nil)
	s.createArtist("T Lagging", nil)
	release := s.createRelease("T Known Album", known)
	othersRelease := s.createRelease("T Other Album", other)
	compilation := s.createRelease("T Split", other, known)

	// A catalog artist's track with no release link: the unlinked view.
	s.addAcross(candidatePlay{artist: "T Known", title: "Known Song", artistID: &known}, 2)
	// One counted play linked to a release credited to the artist removes
	// the track.
	s.addAcross(candidatePlay{artist: "T Known", title: "Released Song", artistID: &known}, 2)
	s.add(candidatePlay{station: "t-kexp", artist: "T Known", title: "Released Song", artistID: &known, releaseID: &release}, 1)
	// So does such a link on any play at any time, here before the window.
	s.addAcross(candidatePlay{artist: "T Known", title: "Old Release Song", artistID: &known}, 2)
	s.add(candidatePlay{station: "t-kexp", artist: "T Known", title: "Old Release Song", artistID: &known, releaseID: &release, date: radioCandidateTestBeforeStart}, 1)
	// A release credited to several artists counts for each of them.
	s.addAcross(candidatePlay{artist: "T Known", title: "Split Song", artistID: &known}, 2)
	s.add(candidatePlay{station: "t-kexp", artist: "T Known", title: "Split Song", artistID: &known, releaseID: &compilation}, 1)
	// A link to a release the play's artist is not credited on does not count.
	s.addAcross(candidatePlay{artist: "T Known", title: "Mislinked Song", artistID: &known}, 2)
	s.add(candidatePlay{station: "t-kexp", artist: "T Known", title: "Mislinked Song", artistID: &known, releaseID: &othersRelease}, 1)
	// A credited link on an ambiguous play, which the counts leave out, still
	// counts.
	s.addAcross(candidatePlay{artist: "T Known", title: "Ambiguous Link Song", artistID: &known}, 2)
	s.add(candidatePlay{station: "t-kexp", artist: "T Known", title: "Ambiguous Link Song", artistID: &known, releaseID: &release,
		matchState: catalogm.RadioPlayMatchStateAmbiguous}, 1)
	// So does one on a play the track ranking excludes (a talk title on
	// WFMU, by a station-scoped rule set below).
	s.addAcross(candidatePlay{artist: "T Known", title: "Excluded Link Song", artistID: &known}, 2)
	s.add(candidatePlay{station: "t-wfmu", show: "talk", artist: "T Known", title: "Excluded Link Song feat Talker", artistID: &known, releaseID: &release}, 1)
	// A credited release on a play not linked to an artist does not count.
	s.addAcross(candidatePlay{artist: "T Known", title: "Unlinked Link Song", artistID: &known}, 2)
	s.add(candidatePlay{station: "t-kexp", artist: "T Known", title: "Unlinked Link Song", releaseID: &release}, 1)
	// So does one on another spelling a recording id joined into the track.
	const joined = "abababab-1111-4111-8111-111111111111"
	s.addAcross(candidatePlay{artist: "T Known", title: "Joined Link Song", artistID: &known, mbRecording: joined}, 2)
	s.add(candidatePlay{station: "t-kexp", artist: "T Known Band", title: "Joined Link Song", artistID: &known, releaseID: &release,
		mbRecording: joined, date: radioCandidateTestBeforeStart}, 1)
	// Some counted plays linked, some not: still the catalog artist's track.
	s.addAcross(candidatePlay{artist: "T Known", title: "Mixed Song", artistID: &known}, 1)
	s.add(candidatePlay{station: "t-kexp", artist: "T Known", title: "Mixed Song"}, 1)
	// An artist the catalog does not have: the unmatched view.
	s.addAcross(candidatePlay{artist: "T Newcomer", title: "New Song"}, 2)
	// Unlinked plays of an artist the catalog has by name: neither view.
	s.addAcross(candidatePlay{artist: "T Lagging", title: "Lagging Song"}, 2)
	s.Require().NoError(s.db.Exec(
		`INSERT INTO radio_play_exclusions (match_kind, pattern_type, pattern, category, reason, station_id)
		 VALUES ('track_title', 'exact', 'excluded link song feat talker', 'talk', 'test', ?)`, s.stations["t-wfmu"]).Error)
	defer func() {
		s.Require().NoError(s.db.Exec(`DELETE FROM radio_play_exclusions WHERE reason = 'test'`).Error)
	}()
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
	s.ElementsMatch([]string{"Known Song", "Mislinked Song", "Mixed Song", "Unlinked Link Song"}, titles(unlinked))
	s.Equal([]string{"New Song"}, titles(unmatched))
	s.Equal(12, unlinked.QualifyingTracks)
	s.Equal(12, unmatched.QualifyingTracks)
	s.Equal(4, unlinked.Total)
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
	s.Equal([]string{"t newcomer"}, newSong.ArtistCandidateKeys)
	s.Equal("T Newcomer", newSong.DisplayArtist)
	for _, c := range unlinked.Candidates {
		s.NotNil(c.ArtistCandidateKeys)
		s.Empty(c.ArtistCandidateKeys, "the unlinked view does not compute artist candidates")
	}
}

func (s *RadioCandidatesIntegrationSuite) TestTrackUnmatchedViewNeedsAnArtistCandidateKey() {
	// A MusicBrainz recording id joins two spellings into one track; both
	// keys clear the artist floors.
	s.addAcross(candidatePlay{artist: "T Joined", title: "Joined Song", mbRecording: radioTrackTestRecord}, 2)
	s.addAcross(candidatePlay{artist: "T Joined Band", title: "Joined Song", mbRecording: radioTrackTestRecord}, 3)
	// Here one spelling has a single play, so its key is no artist candidate;
	// the other key is, so the track stays.
	const half = "eeeeeeee-1111-4111-8111-111111111111"
	s.addAcross(candidatePlay{artist: "T Half", title: "Half Song", mbRecording: half}, 2)
	s.add(candidatePlay{station: "t-kexp", artist: "T Half And Friend", title: "Half Song", mbRecording: half}, 1)
	// No key of this track is an artist candidate: each spelling is under
	// the artist floors, though the joined track clears the track floors.
	const none = "ffffffff-1111-4111-8111-111111111111"
	s.add(candidatePlay{station: "t-kexp", artist: "T Lone A", title: "Lone Song", mbRecording: none}, 2)
	s.add(candidatePlay{station: "t-wfmu", artist: "T Lone B", title: "Lone Song", mbRecording: none}, 2)
	s.rebuildTracks()

	listing := s.listTracks(contracts.RadioTrackCandidateViewUnmatched, contracts.RadioCandidateLaneFilterAll, contracts.RadioCandidateSortScore, 50)
	byTitle := map[string]contracts.RadioTrackCandidate{}
	for _, c := range listing.Candidates {
		byTitle[c.DisplayTitle] = c
	}
	s.Require().Len(byTitle, 2)
	joined := byTitle["Joined Song"]
	s.Equal(10, joined.PlayCount)
	s.Equal([]contracts.RadioCandidateName{{Name: "t joined band", PlayCount: 6}, {Name: "t joined", PlayCount: 4}}, joined.ArtistKeys)
	s.Equal([]string{"t joined band", "t joined"}, joined.ArtistCandidateKeys, "candidate keys follow artist_keys, by play count")
	halfSong := byTitle["Half Song"]
	s.Equal(5, halfSong.PlayCount)
	s.Equal([]string{"t half"}, halfSong.ArtistCandidateKeys)
	s.Len(halfSong.ArtistKeys, 2)
	s.Equal(3, listing.QualifyingTracks, "the track with no artist candidate key qualifies but is in neither view")
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
	// An artist name in a script normalization may not fold: human, flagged.
	s.addAcross(candidatePlay{artist: "T Band 東京", title: "Script Artist Song", mbRelease: radioTrackTestReleaseA}, 2)
	// The only id is on a live version the track key strips: human, flagged.
	s.addAcross(candidatePlay{artist: "T Band", title: "Variant Song"}, 1)
	s.add(candidatePlay{station: "t-kexp", artist: "T Band", title: "Variant Song (Live on KEXP)", mbRelease: radioTrackTestReleaseB}, 1)
	// The same id also on plain titles: not a variant, auto.
	s.addAcross(candidatePlay{artist: "T Band", title: "Backed Song", mbRelease: radioTrackTestReleaseB}, 2)
	s.add(candidatePlay{station: "t-kexp", artist: "T Band", title: "Backed Song (feat. Guest)", mbRelease: radioTrackTestReleaseB}, 1)
	s.rebuildTracks()

	got := s.tracks(contracts.RadioTrackCandidateViewUnmatched)
	auto, human := contracts.RadioCandidateLaneAuto, contracts.RadioCandidateLaneHuman
	for title, want := range map[string]contracts.RadioCandidateLane{
		"One Release": auto, "Some Release": auto, "Two Releases": human,
		"No Release": human, "Bad Release": human, "Song 東京": human, "Variant Song": human,
		"Script Artist Song": human, "Backed Song": auto,
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
	s.True(got["Script Artist Song"].NonLatinSpelling, "a non-Latin artist name flags the track too")
	s.False(got["One Release"].NonLatinSpelling)
	s.True(got["Variant Song"].VariantReleaseID)
	s.Equal(radioTrackTestReleaseB, *got["Variant Song"].MBReleaseID)
	s.False(got["One Release"].VariantReleaseID)
	s.False(got["Some Release"].VariantReleaseID, "a capitalized id on an unstripped title is not a variant")
	s.False(got["Backed Song"].VariantReleaseID, "the id is also on plain titles")

	autoOnly := s.listTracks(contracts.RadioTrackCandidateViewUnmatched, contracts.RadioCandidateLaneFilterAuto, contracts.RadioCandidateSortScore, 50)
	s.Equal(3, autoOnly.Total)
	s.Equal(3, autoOnly.AutoTotal)
	s.Equal(6, autoOnly.HumanTotal)
	for _, c := range autoOnly.Candidates {
		s.Equal(auto, c.Lane)
	}
	limited := s.listTracks(contracts.RadioTrackCandidateViewUnmatched, contracts.RadioCandidateLaneFilterHuman, contracts.RadioCandidateSortPlays, 1)
	s.Len(limited.Candidates, 1)
	s.Equal(6, limited.Total)
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
	s.False(c.VariantReleaseID, "plain spellings carry a release id too")
	s.Equal(contracts.RadioCandidateLaneHuman, c.Lane)
}

// A placeholder album (radio_play_exclusions, album_title) is not album
// evidence, yet its plays still count toward the track.
func (s *RadioCandidatesIntegrationSuite) TestTrackEvidenceSkipsPlaceholderAlbums() {
	known := s.createArtist("T Album Band", nil)
	add := func(station, album string, n int) {
		s.add(candidatePlay{station: station, artist: "T Album Band", title: "Album Song", album: album, artistID: &known}, n)
	}
	add("t-kexp", "Single", 2)
	add("t-wfmu", "Real Album", 1)
	add("t-wfmu", "S/T", 1)
	s.rebuildTracks()

	got := s.tracks(contracts.RadioTrackCandidateViewUnlinked)
	s.Require().Contains(got, "Album Song")
	c := got["Album Song"]
	s.Equal(4, c.PlayCount, "plays with a placeholder album still count for the track")
	s.Equal(1, c.AlbumNameCount)
	s.Equal([]contracts.RadioCandidateName{{Name: "Real Album", PlayCount: 1}}, c.AlbumNames)
}
