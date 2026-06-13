package catalog

// PSY-1078: WFMU logs background/segment music as plays named
// "Music behind DJ: <name>". Aggregated surfaces (top artists, episode
// artist previews, now-playing recent artists) must exclude them; raw
// playlist surfaces must keep them. Methods here run as part of
// RadioServiceIntegrationTestSuite (radio_test.go).

import (
	"testing"

	"github.com/stretchr/testify/assert"

	catalogm "psychic-homily-backend/internal/models/catalog"
)

// =============================================================================
// UNIT TESTS (No Database Required)
// =============================================================================

func TestIsPseudoArtistName(t *testing.T) {
	pseudo := []string{
		"Music behind DJ: The Shadows",
		"Music Behind DJ: Living Guitars", // casing varies in real WFMU data
		"MUSIC BEHIND DJ: LOUD",
		"Music behind DJ:", // bare prefix (424 rows observed)
	}
	for _, name := range pseudo {
		assert.True(t, isPseudoArtistName(name), "expected pseudo-artist: %q", name)
	}

	realArtists := []string{
		"The Shadows",
		"Music",
		"Music behind DJ", // no colon — never observed; don't over-match
		"DJ Shadow",
		"Behind the DJ",
		"",
	}
	for _, name := range realArtists {
		assert.False(t, isPseudoArtistName(name), "expected real artist: %q", name)
	}
}

func TestRecentArtistsFromPlayRows_SkipsPseudoArtists(t *testing.T) {
	row := func(name string, pos int) nowPlayingPlayRow {
		return nowPlayingPlayRow{RadioPlay: catalogm.RadioPlay{ArtistName: name, Position: pos}}
	}
	rows := []nowPlayingPlayRow{
		row("A", 1),
		row("Music behind DJ: Mr. Weather", 2),
		row("B", 3),
		row("Music Behind DJ: Living Guitars", 4),
		row("C", 5),
	}

	got := recentArtistsFromPlayRows(rows, true, "")
	names := make([]string, len(got))
	for i, p := range got {
		names[i] = p.ArtistName
	}
	assert.Equal(t, []string{"B", "A"}, names)
}

// =============================================================================
// INTEGRATION TESTS (run via RadioServiceIntegrationTestSuite)
// =============================================================================

// seedPseudoArtistEpisode seeds one show with an episode mixing real artists
// and pseudo-artist segment rows, with the pseudo rows outnumbering the real
// plays (the failure mode on the live WFMU station page).
func (suite *RadioServiceIntegrationTestSuite) seedPseudoArtistEpisode() (stationID, showID uint, episode *catalogm.RadioEpisode) {
	station := suite.createStation("WFMU")
	show := suite.createShow(station.ID, "Garbage Time")
	ep := suite.createEpisode(show.ID, "2026-06-09")

	suite.createPlay(ep.ID, 1, "Music behind DJ: The Shadows")
	suite.createPlay(ep.ID, 2, "The Shadows")
	suite.createPlay(ep.ID, 3, "Music behind DJ: The Shadows")
	suite.createPlay(ep.ID, 4, "Music Behind DJ: Living Guitars") // casing variant
	suite.createPlay(ep.ID, 5, "Music behind DJ:")                // bare prefix
	suite.createPlay(ep.ID, 6, "Deerhunter")

	return station.ID, show.ID, ep
}

func (suite *RadioServiceIntegrationTestSuite) TestTopArtists_ExcludePseudoArtists() {
	stationID, showID, _ := suite.seedPseudoArtistEpisode()

	// Show-scoped and station-scoped paths share topArtists; assert both.
	forShow, err := suite.radioService.GetTopArtistsForShow(showID, 0, 10)
	suite.Require().NoError(err)
	forStation, err := suite.radioService.GetTopArtistsForStation(stationID, 0, 10)
	suite.Require().NoError(err)

	// Both real artists have 1 play each, so ordering within the tie is not
	// asserted — only membership and the absence of pseudo-artists.
	suite.Require().Len(forShow, 2)
	suite.Require().Len(forStation, 2)

	gotNames := []string{forShow[0].ArtistName, forShow[1].ArtistName}
	suite.ElementsMatch([]string{"The Shadows", "Deerhunter"}, gotNames)
	gotNames = []string{forStation[0].ArtistName, forStation[1].ArtistName}
	suite.ElementsMatch([]string{"The Shadows", "Deerhunter"}, gotNames)
}

func (suite *RadioServiceIntegrationTestSuite) TestEpisodeArtistPreview_ExcludesPseudoArtists() {
	_, showID, _ := suite.seedPseudoArtistEpisode()

	episodes, total, err := suite.radioService.GetEpisodes(showID, 10, 0)
	suite.Require().NoError(err)
	suite.Equal(int64(1), total)
	suite.Require().Len(episodes, 1)

	preview := episodes[0].ArtistPreview
	suite.Require().Len(preview, 2)
	suite.Equal("The Shadows", preview[0].ArtistName)
	suite.Equal("Deerhunter", preview[1].ArtistName)
}

func (suite *RadioServiceIntegrationTestSuite) TestEpisodeDetailPlaylist_KeepsPseudoArtistRows() {
	_, _, ep := suite.seedPseudoArtistEpisode()

	detail, err := suite.radioService.GetEpisodeDetail(ep.ID)
	suite.Require().NoError(err)

	// The playlist is the honest raw record: all 6 rows, segments included.
	suite.Require().Len(detail.Plays, 6)
	suite.Equal("Music behind DJ: The Shadows", detail.Plays[0].ArtistName)
	suite.Equal("Music Behind DJ: Living Guitars", detail.Plays[3].ArtistName)
}
