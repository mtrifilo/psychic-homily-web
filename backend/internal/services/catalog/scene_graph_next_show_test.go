package catalog

// Scene-graph next-show summary (PSY-1449): nodes with upcoming shows carry a
// next-show summary (graph-card shape) sourced from ONE batched query, so the
// homepage teaser can render date/venue chips without N graph-card fetches.

import (
	"time"

	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"psychic-homily-backend/internal/services/contracts"
)

// findSceneGraphNode returns the node for the given artist ID, or nil.
func findSceneGraphNode(graph *contracts.SceneGraphResponse, artistID uint) *contracts.SceneGraphNode {
	for i := range graph.Nodes {
		if graph.Nodes[i].ID == artistID {
			return &graph.Nodes[i]
		}
	}
	return nil
}

// TestGetSceneGraph_NextShowSummary: nodes with upcoming shows carry the
// soonest show's summary (date + venue + timezone, graph-card shape); nodes
// without upcoming shows omit it; presence tracks upcoming_show_count.
func (suite *SceneServiceIntegrationTestSuite) TestGetSceneGraph_NextShowSummary() {
	venues, artists := suite.seedSceneData()
	// seedSceneData: Band A plays Show 1 (day+7, Crescent Ballroom) and
	// Show 3 (day+9, Valley Bar) — the summary must pick Show 1 (soonest).
	tz := "America/Phoenix"
	suite.Require().NoError(suite.db.Model(venues[0]).Update("timezone", &tz).Error)

	// A roster artist with only a PAST show — node present, no next_show.
	user := suite.createUser()
	pastOnly := suite.createArtist("Past Only Band")
	suite.createApprovedShow("Old Show", venues[2].ID, pastOnly.ID, user.ID,
		time.Now().UTC().AddDate(0, 0, -30))

	graph, err := suite.sceneService.GetSceneGraph("Phoenix", "AZ", nil, "")
	suite.Require().NoError(err)

	bandA := findSceneGraphNode(graph, artists[0].ID)
	suite.Require().NotNil(bandA)
	suite.Require().NotNil(bandA.NextShow, "node with upcoming shows must carry next_show")
	suite.Equal("Crescent Ballroom", bandA.NextShow.VenueName, "must pick the SOONEST upcoming show")
	suite.Equal("Phoenix", bandA.NextShow.VenueCity)
	suite.Equal("AZ", bandA.NextShow.VenueState)
	suite.Require().NotNil(bandA.NextShow.VenueTimezone, "venue timezone must propagate for tz-safe rendering")
	suite.Equal(tz, *bandA.NextShow.VenueTimezone)
	suite.False(bandA.NextShow.EventDate.IsZero())

	noUpcoming := findSceneGraphNode(graph, pastOnly.ID)
	suite.Require().NotNil(noUpcoming, "past-show artist still counts toward the roster")
	suite.Nil(noUpcoming.NextShow, "node without upcoming shows must omit next_show")

	// Payload invariant: next_show presence ⟺ upcoming_show_count > 0.
	for _, n := range graph.Nodes {
		suite.Equal(n.UpcomingShowCount > 0, n.NextShow != nil,
			"node %d: next_show presence must track upcoming_show_count", n.ID)
	}
}

// TestGetSceneGraph_NextShowSingleBatchedQuery pins the PSY-1449 AC: the
// next-show summaries come from ONE batched query, not N per-node lookups.
// Uses the TestGraphCardQuerySlimming counting-logger pattern (PSY-1352).
func (suite *SceneServiceIntegrationTestSuite) TestGetSceneGraph_NextShowSingleBatchedQuery() {
	_, artists := suite.seedSceneData()

	var n int
	countingDB := suite.db.Session(&gorm.Session{
		Logger: queryCounter{Interface: gormlogger.Default.LogMode(gormlogger.Silent), n: &n},
	})

	// The helper itself: exactly one statement for the whole artist set.
	ids := []uint{artists[0].ID, artists[1].ID, artists[2].ID}
	n = 0
	got := batchArtistNextShows(countingDB, ids)
	suite.Equal(1, n, "batchArtistNextShows must issue exactly ONE query for %d artists", len(ids))
	suite.Len(got, 3, "all three seeded artists have upcoming shows")

	// End-to-end: GetSceneGraph's query count must not scale with roster size.
	svc := NewSceneService(countingDB)
	n = 0
	_, err := svc.GetSceneGraph("Phoenix", "AZ", nil, "")
	suite.Require().NoError(err)
	baseline := n

	// Double the roster with upcoming shows; the count must stay flat.
	user := suite.createUser()
	future := time.Now().UTC().AddDate(0, 0, 5)
	extraVenue := suite.createVerifiedVenue("The Extra Room", "Phoenix", "AZ")
	for _, name := range []string{"Band D", "Band E", "Band F"} {
		a := suite.createArtist(name)
		suite.createApprovedShow("Show for "+name, extraVenue.ID, a.ID, user.ID, future)
	}
	n = 0
	graph, err := svc.GetSceneGraph("Phoenix", "AZ", nil, "")
	suite.Require().NoError(err)
	suite.Require().GreaterOrEqual(len(graph.Nodes), 6)
	suite.Equal(baseline, n,
		"GetSceneGraph query count must be independent of node count (no per-node next-show lookups)")
}
