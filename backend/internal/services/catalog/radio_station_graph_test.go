package catalog

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"

	apperrors "psychic-homily-backend/internal/errors"
	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/testutil"
	"psychic-homily-backend/internal/utils"
)

// =============================================================================
// UNIT TESTS (No Database Required)
// =============================================================================

func TestNormalizeStationGraphWindow(t *testing.T) {
	tests := []struct {
		in, expected string
	}{
		{"12m", stationGraphWindow12M},
		{"all", stationGraphWindowAll},
		{"ALL", stationGraphWindowAll},
		{" all ", stationGraphWindowAll},
		{"", stationGraphWindow12M},
		{"bogus", stationGraphWindow12M},
	}
	for _, tc := range tests {
		t.Run(fmt.Sprintf("%q", tc.in), func(t *testing.T) {
			assert.Equal(t, tc.expected, normalizeStationGraphWindow(tc.in))
		})
	}
}

func TestGetStationGraph_NilDB(t *testing.T) {
	svc := &RadioService{}
	_, err := svc.GetStationGraph(1, "12m", 75)
	assert.EqualError(t, err, "database not initialized")
}

// =============================================================================
// INTEGRATION TESTS (With Real Database)
// =============================================================================

type RadioStationGraphTestSuite struct {
	suite.Suite
	testDB       *testutil.TestDatabase
	db           *gorm.DB
	radioService *RadioService
}

func (suite *RadioStationGraphTestSuite) SetupSuite() {
	suite.testDB = testutil.SetupTestPostgres(suite.T())
	suite.db = suite.testDB.DB
	suite.radioService = &RadioService{db: suite.testDB.DB}
}

func (suite *RadioStationGraphTestSuite) TearDownSuite() {
	suite.testDB.Cleanup()
}

// SetupTest wipes data so every test starts from a known empty state
// (migrations seed WFMU network + stations on the test container).
func (suite *RadioStationGraphTestSuite) SetupTest() {
	suite.cleanupTables()
}

func (suite *RadioStationGraphTestSuite) TearDownTest() {
	suite.cleanupTables()
}

func (suite *RadioStationGraphTestSuite) cleanupTables() {
	sqlDB, err := suite.db.DB()
	suite.Require().NoError(err)
	// Delete in FK-safe order.
	_, _ = sqlDB.Exec("DELETE FROM radio_plays")
	_, _ = sqlDB.Exec("DELETE FROM radio_episodes")
	_, _ = sqlDB.Exec("DELETE FROM radio_shows")
	_, _ = sqlDB.Exec("DELETE FROM radio_stations")
	_, _ = sqlDB.Exec("DELETE FROM radio_networks")
	_, _ = sqlDB.Exec("DELETE FROM show_artists")
	_, _ = sqlDB.Exec("DELETE FROM show_venues")
	_, _ = sqlDB.Exec("DELETE FROM shows")
	_, _ = sqlDB.Exec("DELETE FROM artists")
	_, _ = sqlDB.Exec("DELETE FROM venues")
}

func TestRadioStationGraphTestSuite(t *testing.T) {
	suite.Run(t, new(RadioStationGraphTestSuite))
}

// =============================================================================
// HELPERS
// =============================================================================

func (suite *RadioStationGraphTestSuite) createStation(name, slug string) *catalogm.RadioStation {
	station := &catalogm.RadioStation{Name: name, Slug: slug, BroadcastType: catalogm.BroadcastTypeBoth}
	suite.Require().NoError(suite.db.Create(station).Error)
	return station
}

func (suite *RadioStationGraphTestSuite) createShow(stationID uint, name, slug string) *catalogm.RadioShow {
	show := &catalogm.RadioShow{StationID: stationID, Name: name, Slug: slug}
	suite.Require().NoError(suite.db.Create(show).Error)
	return show
}

func (suite *RadioStationGraphTestSuite) createEpisode(showID uint, airDate string) *catalogm.RadioEpisode {
	ep := &catalogm.RadioEpisode{ShowID: showID, AirDate: airDate}
	suite.Require().NoError(suite.db.Create(ep).Error)
	return ep
}

func (suite *RadioStationGraphTestSuite) createArtist(name string) *catalogm.Artist {
	slug := utils.GenerateArtistSlug(name)
	artist := &catalogm.Artist{Name: name, Slug: &slug}
	suite.Require().NoError(suite.db.Create(artist).Error)
	return artist
}

// createMatchedPlay inserts a play linked to a knowledge-graph artist.
func (suite *RadioStationGraphTestSuite) createMatchedPlay(episodeID uint, position int, artist *catalogm.Artist) {
	play := &catalogm.RadioPlay{
		EpisodeID:  episodeID,
		Position:   position,
		ArtistName: artist.Name,
		ArtistID:   &artist.ID,
	}
	suite.Require().NoError(suite.db.Create(play).Error)
}

// createUnmatchedPlay inserts a play with no artist link (artist_id NULL).
func (suite *RadioStationGraphTestSuite) createUnmatchedPlay(episodeID uint, position int, artistName string) {
	play := &catalogm.RadioPlay{EpisodeID: episodeID, Position: position, ArtistName: artistName}
	suite.Require().NoError(suite.db.Create(play).Error)
}

func recentDate(monthsAgo int) string {
	return time.Now().UTC().AddDate(0, -monthsAgo, 0).Format("2006-01-02")
}

// =============================================================================
// TESTS
// =============================================================================

func (suite *RadioStationGraphTestSuite) TestGetStationGraph_NotFound() {
	_, err := suite.radioService.GetStationGraph(99999, "12m", 75)
	suite.Require().Error(err)
	var radioErr *apperrors.RadioError
	suite.Require().True(errors.As(err, &radioErr))
	suite.Equal(apperrors.CodeRadioStationNotFound, radioErr.Code)
}

func (suite *RadioStationGraphTestSuite) TestGetStationGraph_EmptyStation() {
	station := suite.createStation("KEXP", "kexp")

	graph, err := suite.radioService.GetStationGraph(station.ID, "12m", 75)
	suite.Require().NoError(err)

	suite.Equal(station.ID, graph.Station.ID)
	suite.Equal("kexp", graph.Station.Slug)
	suite.Equal("KEXP", graph.Station.Name)
	suite.Equal("last_12m", graph.Station.Window)
	suite.Equal(0, graph.Station.ArtistCount)
	suite.Equal(0, graph.Station.EdgeCount)
	suite.Empty(graph.Clusters)
	suite.Empty(graph.Nodes)
	suite.Empty(graph.Links)
}

// TestGetStationGraph_BasicGraph covers the core shape: top artists as nodes,
// episode co-occurrence as edges above the >=2 threshold, isolates flagged,
// unmatched plays excluded.
func (suite *RadioStationGraphTestSuite) TestGetStationGraph_BasicGraph() {
	station := suite.createStation("KEXP", "kexp")
	show := suite.createShow(station.ID, "Morning Show", "kexp-morning")

	artistA := suite.createArtist("Alpha")
	artistB := suite.createArtist("Beta")
	artistC := suite.createArtist("Gamma")

	// Episode 1 and 2: A + B co-occur (weight 2). Episode 2 also has C
	// (A–C and B–C co-occur once each — below the threshold of 2).
	ep1 := suite.createEpisode(show.ID, recentDate(2))
	suite.createMatchedPlay(ep1.ID, 1, artistA)
	suite.createMatchedPlay(ep1.ID, 2, artistB)
	// Duplicate play of A in ep1 — must NOT inflate the A–B weight
	// (weight is episodes, not play-pairs).
	suite.createMatchedPlay(ep1.ID, 3, artistA)
	// Unmatched play — must be ignored entirely.
	suite.createUnmatchedPlay(ep1.ID, 4, "Unmatched Mystery Band")

	ep2 := suite.createEpisode(show.ID, recentDate(1))
	suite.createMatchedPlay(ep2.ID, 1, artistA)
	suite.createMatchedPlay(ep2.ID, 2, artistB)
	suite.createMatchedPlay(ep2.ID, 3, artistC)

	graph, err := suite.radioService.GetStationGraph(station.ID, "12m", 75)
	suite.Require().NoError(err)

	// Nodes: 3 matched artists, ordered by name; play counts include the
	// duplicate play of A.
	suite.Equal(3, graph.Station.ArtistCount)
	suite.Require().Len(graph.Nodes, 3)
	suite.Equal("Alpha", graph.Nodes[0].Name)
	suite.Equal(3, graph.Nodes[0].PlayCount)
	suite.Equal("Beta", graph.Nodes[1].Name)
	suite.Equal(2, graph.Nodes[1].PlayCount)
	suite.Equal("Gamma", graph.Nodes[2].Name)
	suite.Equal(1, graph.Nodes[2].PlayCount)

	// Edges: only A–B clears the >=2 episode threshold.
	suite.Equal(1, graph.Station.EdgeCount)
	suite.Require().Len(graph.Links, 1)
	link := graph.Links[0]
	suite.Equal(minUint(artistA.ID, artistB.ID), link.SourceID)
	suite.Equal(maxUint(artistA.ID, artistB.ID), link.TargetID)
	suite.Equal(catalogm.RelationshipTypeRadioCooccurrence, link.Type)
	suite.InDelta(2.0/50.0, link.Score, 1e-9)
	detail, ok := link.Detail.(map[string]any)
	suite.Require().True(ok)
	suite.Equal(2, detail["co_occurrence_count"])
	suite.Equal(recentDate(1), detail["last_co_occurrence"])

	// Isolates: C has no surviving edges.
	suite.False(graph.Nodes[0].IsIsolate)
	suite.False(graph.Nodes[1].IsIsolate)
	suite.True(graph.Nodes[2].IsIsolate)

	// Small show rolls into the "other" cluster (below the size floor).
	suite.Require().Len(graph.Clusters, 1)
	suite.Equal(sceneClusterOtherID, graph.Clusters[0].ID)
	suite.Equal(3, graph.Clusters[0].Size)
	for _, n := range graph.Nodes {
		suite.Equal(sceneClusterOtherID, n.ClusterID)
	}
}

// TestGetStationGraph_StationScoped asserts that co-occurrence on another
// station never leaks into this station's graph — the whole point of the
// query-time derivation (the aggregate affinity table can't do this).
func (suite *RadioStationGraphTestSuite) TestGetStationGraph_StationScoped() {
	stationA := suite.createStation("KEXP", "kexp")
	showA := suite.createShow(stationA.ID, "Morning Show", "kexp-morning")
	stationB := suite.createStation("WFMU", "wfmu")
	showB := suite.createShow(stationB.ID, "Evening Show", "wfmu-evening")

	artist1 := suite.createArtist("Alpha")
	artist2 := suite.createArtist("Beta")

	// One shared episode on station A (below threshold alone)…
	epA := suite.createEpisode(showA.ID, recentDate(1))
	suite.createMatchedPlay(epA.ID, 1, artist1)
	suite.createMatchedPlay(epA.ID, 2, artist2)

	// …and three shared episodes on station B.
	for i := 0; i < 3; i++ {
		epB := suite.createEpisode(showB.ID, recentDate(i+1))
		suite.createMatchedPlay(epB.ID, 1, artist1)
		suite.createMatchedPlay(epB.ID, 2, artist2)
	}

	// Station A: the single shared episode is below the threshold → no edge.
	graphA, err := suite.radioService.GetStationGraph(stationA.ID, "12m", 75)
	suite.Require().NoError(err)
	suite.Equal(2, graphA.Station.ArtistCount)
	suite.Empty(graphA.Links)

	// Station B: weight 3 — station A's episode is not counted.
	graphB, err := suite.radioService.GetStationGraph(stationB.ID, "12m", 75)
	suite.Require().NoError(err)
	suite.Require().Len(graphB.Links, 1)
	detail := graphB.Links[0].Detail.(map[string]any)
	suite.Equal(3, detail["co_occurrence_count"])
}

// TestGetStationGraph_WindowFilter asserts the 12m default excludes old
// episodes and window=all includes them.
func (suite *RadioStationGraphTestSuite) TestGetStationGraph_WindowFilter() {
	station := suite.createStation("KEXP", "kexp")
	show := suite.createShow(station.ID, "Morning Show", "kexp-morning")

	artistA := suite.createArtist("Alpha")
	artistB := suite.createArtist("Beta")

	// Two co-occurring episodes, both ~2 years old.
	old1 := time.Now().UTC().AddDate(-2, 0, 0).Format("2006-01-02")
	old2 := time.Now().UTC().AddDate(-2, 1, 0).Format("2006-01-02")
	for _, d := range []string{old1, old2} {
		ep := suite.createEpisode(show.ID, d)
		suite.createMatchedPlay(ep.ID, 1, artistA)
		suite.createMatchedPlay(ep.ID, 2, artistB)
	}

	// Default (12m): everything is out of window.
	graph12m, err := suite.radioService.GetStationGraph(station.ID, "", 75)
	suite.Require().NoError(err)
	suite.Equal("last_12m", graph12m.Station.Window)
	suite.Equal(0, graph12m.Station.ArtistCount)
	suite.Empty(graph12m.Nodes)
	suite.Empty(graph12m.Links)

	// window=all: both nodes and the edge appear.
	graphAll, err := suite.radioService.GetStationGraph(station.ID, "all", 75)
	suite.Require().NoError(err)
	suite.Equal("all_time", graphAll.Station.Window)
	suite.Equal(2, graphAll.Station.ArtistCount)
	suite.Require().Len(graphAll.Links, 1)
	detail := graphAll.Links[0].Detail.(map[string]any)
	suite.Equal(2, detail["co_occurrence_count"])
	suite.Equal(old2, detail["last_co_occurrence"])
}

// TestGetStationGraph_LimitCapsNodes asserts the top-N cap keeps the
// most-played artists and drops edges to excluded artists.
func (suite *RadioStationGraphTestSuite) TestGetStationGraph_LimitCapsNodes() {
	station := suite.createStation("KEXP", "kexp")
	show := suite.createShow(station.ID, "Morning Show", "kexp-morning")

	artistA := suite.createArtist("Alpha") // 3 episodes
	artistB := suite.createArtist("Beta")  // 3 episodes
	artistC := suite.createArtist("Gamma") // 2 episodes

	// A + B in 3 episodes; C tags along in the first 2 (so A–C and B–C would
	// clear the threshold if C were included).
	for i := 0; i < 3; i++ {
		ep := suite.createEpisode(show.ID, recentDate(i+1))
		suite.createMatchedPlay(ep.ID, 1, artistA)
		suite.createMatchedPlay(ep.ID, 2, artistB)
		if i < 2 {
			suite.createMatchedPlay(ep.ID, 3, artistC)
		}
	}

	graph, err := suite.radioService.GetStationGraph(station.ID, "12m", 2)
	suite.Require().NoError(err)

	suite.Equal(2, graph.Station.ArtistCount)
	suite.Require().Len(graph.Nodes, 2)
	suite.Equal("Alpha", graph.Nodes[0].Name)
	suite.Equal("Beta", graph.Nodes[1].Name)

	// Only the A–B edge survives; edges touching the excluded C are gone.
	suite.Require().Len(graph.Links, 1)
	suite.Equal(minUint(artistA.ID, artistB.ID), graph.Links[0].SourceID)
	suite.Equal(maxUint(artistA.ID, artistB.ID), graph.Links[0].TargetID)
}

// TestGetStationGraph_ClustersByShow asserts the primary-show clustering:
// a show with >= sceneClusterMinSize primary artists becomes a first-class
// cluster, the tail rolls into "other", and an edge across the boundary is
// flagged cross-cluster.
func (suite *RadioStationGraphTestSuite) TestGetStationGraph_ClustersByShow() {
	station := suite.createStation("KEXP", "kexp")
	show1 := suite.createShow(station.ID, "Drone Zone", "kexp-drone-zone")
	show2 := suite.createShow(station.ID, "Pop Hour", "kexp-pop-hour")

	// 6 artists whose primary show is show1 (>= sceneClusterMinSize).
	show1Artists := make([]*catalogm.Artist, 0, sceneClusterMinSize)
	for i := 0; i < sceneClusterMinSize; i++ {
		show1Artists = append(show1Artists, suite.createArtist(fmt.Sprintf("Drone Artist %02d", i)))
	}
	// One artist whose primary show is show2 (rolls into "other").
	popArtist := suite.createArtist("Pop Artist")

	// Two show1 episodes with all six artists — pairwise co-occurrence + the
	// primary-show signal. popArtist guests on both (A–pop weight 2) but has
	// MORE plays on show2, keeping show2 primary.
	for i := 0; i < 2; i++ {
		ep := suite.createEpisode(show1.ID, recentDate(i+1))
		for pos, a := range show1Artists {
			suite.createMatchedPlay(ep.ID, pos+1, a)
		}
		suite.createMatchedPlay(ep.ID, len(show1Artists)+1, popArtist)
	}
	for i := 0; i < 3; i++ {
		ep := suite.createEpisode(show2.ID, recentDate(i+1))
		suite.createMatchedPlay(ep.ID, 1, popArtist)
	}

	graph, err := suite.radioService.GetStationGraph(station.ID, "12m", 75)
	suite.Require().NoError(err)

	// Clusters: show1 first-class, show2's lone artist in "other".
	suite.Require().Len(graph.Clusters, 2)
	show1ClusterID := fmt.Sprintf("rs_%d", show1.ID)
	suite.Equal(show1ClusterID, graph.Clusters[0].ID)
	suite.Equal("Drone Zone", graph.Clusters[0].Label)
	suite.Equal(sceneClusterMinSize, graph.Clusters[0].Size)
	suite.Equal(0, graph.Clusters[0].ColorIndex)
	suite.Equal(sceneClusterOtherID, graph.Clusters[1].ID)
	suite.Equal(1, graph.Clusters[1].Size)
	suite.Equal(-1, graph.Clusters[1].ColorIndex)

	// Node cluster assignment.
	clusterByName := make(map[string]string)
	for _, n := range graph.Nodes {
		clusterByName[n.Name] = n.ClusterID
	}
	for _, a := range show1Artists {
		suite.Equal(show1ClusterID, clusterByName[a.Name])
	}
	suite.Equal(sceneClusterOtherID, clusterByName["Pop Artist"])

	// Cross-cluster flags: edges among show1 artists are intra-cluster;
	// edges from popArtist into show1 artists cross the boundary.
	for _, l := range graph.Links {
		touchesPop := l.SourceID == popArtist.ID || l.TargetID == popArtist.ID
		suite.Equal(touchesPop, l.IsCrossCluster,
			"edge %d-%d cross-cluster flag", l.SourceID, l.TargetID)
	}
	// Sanity: all pairwise show1 edges (15) + pop edges (6) survived.
	suite.Len(graph.Links, 21)
}

// TestGetStationGraph_UpcomingShowCount asserts the green-dot signal is wired:
// an artist with an upcoming approved live show carries the count.
func (suite *RadioStationGraphTestSuite) TestGetStationGraph_UpcomingShowCount() {
	station := suite.createStation("KEXP", "kexp")
	show := suite.createShow(station.ID, "Morning Show", "kexp-morning")
	artistA := suite.createArtist("Alpha")
	artistB := suite.createArtist("Beta")

	ep := suite.createEpisode(show.ID, recentDate(1))
	suite.createMatchedPlay(ep.ID, 1, artistA)
	suite.createMatchedPlay(ep.ID, 2, artistB)

	liveShow := &catalogm.Show{
		EventDate: time.Now().UTC().AddDate(0, 1, 0),
		Status:    catalogm.ShowStatusApproved,
	}
	suite.Require().NoError(suite.db.Create(liveShow).Error)
	suite.Require().NoError(suite.db.Exec(
		"INSERT INTO show_artists (show_id, artist_id, position) VALUES (?, ?, 0)",
		liveShow.ID, artistA.ID).Error)

	graph, err := suite.radioService.GetStationGraph(station.ID, "12m", 75)
	suite.Require().NoError(err)
	suite.Require().Len(graph.Nodes, 2)
	suite.Equal(1, graph.Nodes[0].UpcomingShowCount) // Alpha
	suite.Equal(0, graph.Nodes[1].UpcomingShowCount) // Beta
}

// TestGetStationGraph_QueryCost seeds a deliberately dense synthetic dataset
// (5 shows, 120 artists, 240 episodes, 2,400 matched plays) and reports the
// end-to-end service timing. Logged, not asserted — CI machines vary; the
// number is recorded in the PSY-1081 PR body against the <250ms target.
func (suite *RadioStationGraphTestSuite) TestGetStationGraph_QueryCost() {
	station := suite.createStation("KEXP", "kexp")

	const (
		showCount       = 5
		artistCount     = 120
		episodesPerShow = 48
		playsPerEpisode = 10
	)

	shows := make([]*catalogm.RadioShow, 0, showCount)
	for i := 0; i < showCount; i++ {
		shows = append(shows, suite.createShow(station.ID,
			fmt.Sprintf("Show %02d", i), fmt.Sprintf("kexp-show-%02d", i)))
	}

	artists := make([]*catalogm.Artist, 0, artistCount)
	for i := 0; i < artistCount; i++ {
		artists = append(artists, suite.createArtist(fmt.Sprintf("Perf Artist %03d", i)))
	}

	// Round-robin artists across episodes so co-occurrence pairs repeat
	// across episodes (dense edges, like a real rotation-heavy station).
	plays := make([]catalogm.RadioPlay, 0, showCount*episodesPerShow*playsPerEpisode)
	for si, show := range shows {
		for e := 0; e < episodesPerShow; e++ {
			// Distinct air date per (show, episode) — radio_episodes has a
			// uniqueness constraint on the pair. ~7 days apart keeps all 48
			// episodes inside the 12m window.
			airDate := time.Now().UTC().AddDate(0, 0, -(e*7 + 1)).Format("2006-01-02")
			ep := suite.createEpisode(show.ID, airDate)
			for p := 0; p < playsPerEpisode; p++ {
				a := artists[(si*7+e*3+p)%artistCount]
				plays = append(plays, catalogm.RadioPlay{
					EpisodeID:  ep.ID,
					Position:   p + 1,
					ArtistName: a.Name,
					ArtistID:   &a.ID,
				})
			}
		}
	}
	suite.Require().NoError(suite.db.CreateInBatches(plays, 500).Error)

	start := time.Now()
	graph, err := suite.radioService.GetStationGraph(station.ID, "12m", 75)
	elapsed := time.Since(start)
	suite.Require().NoError(err)

	suite.Equal(75, graph.Station.ArtistCount)
	suite.NotEmpty(graph.Links)
	suite.T().Logf("GetStationGraph on %d plays / %d artists / %d episodes: %v (nodes=%d edges=%d clusters=%d)",
		len(plays), artistCount, showCount*episodesPerShow, elapsed,
		len(graph.Nodes), len(graph.Links), len(graph.Clusters))
}

func minUint(a, b uint) uint {
	if a < b {
		return a
	}
	return b
}

func maxUint(a, b uint) uint {
	if a > b {
		return a
	}
	return b
}
