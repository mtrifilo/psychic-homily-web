package catalog

// Catalog-wide label hubs (PSY-1722), on the SceneService Postgres suite: the
// same builder GetSceneGraph uses, driven over every artist in the catalog
// instead of one metro's roster.
//
// The rules themselves are pinned by the pure fixtures in label_hubs_test.go.
// What only a database can show is here: that the same rows produce a hub at
// catalog scope and no hub at scene scope, that the label columns survive the
// round trip into the hub node, and that spokes land on real seeded artists.
// The three headline outcomes are re-asserted against real rows because they
// are what the ticket's acceptance criteria ask to see at global scope.

import (
	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/contracts"
)

// seedLabelMemberships creates a label and puts every artist on it. It is the
// membership primitive seedLabelWithRoster builds on: hubs are derived from the
// artist_labels fact table, so a test that only needs memberships should not
// have to write the `shared_label` clique too.
func (suite *SceneServiceIntegrationTestSuite) seedLabelMemberships(
	label *catalogm.Label, artists []*catalogm.Artist,
) {
	suite.Require().NoError(suite.db.Create(label).Error)
	for _, a := range artists {
		suite.Require().NoError(suite.db.Create(&catalogm.ArtistLabel{
			ArtistID: a.ID, LabelID: label.ID,
		}).Error)
	}
}

// catalogArtistIDs is the catalog-wide artist set — every artist a global
// consumer would draw, and so the bound on its roster read.
func (suite *SceneServiceIntegrationTestSuite) catalogArtistIDs() []uint {
	var ids []uint
	suite.Require().NoError(suite.db.Table("artists").Order("id ASC").Pluck("id", &ids).Error)
	return ids
}

// A roster no single scene can see whole: 4 artists across 3 metros. At global
// scope it is one hub plus 4 spokes; at scene scope the same label is only a
// 2-artist overlap and keeps its pairwise edge. The 2-artist and slug-less
// labels in the same payload keep theirs at BOTH scopes.
func (suite *SceneServiceIntegrationTestSuite) TestGlobalLabelHubs_CollapseRostersAcrossMetros() {
	phx1 := suite.createArtistIn("Global Band PHX 1", "Phoenix", "AZ")
	phx2 := suite.createArtistIn("Global Band PHX 2", "Phoenix", "AZ")
	atx := suite.createArtistIn("Global Band ATX", "Austin", "TX")
	nyc := suite.createArtistIn("Global Band NYC", "New York", "NY")
	roster := []*catalogm.Artist{phx1, phx2, atx, nyc}
	suite.seedLabelMemberships(&catalogm.Label{
		Name:    "12XU",
		Slug:    stringPtr("12xu"),
		City:    stringPtr("Austin"),
		State:   stringPtr("TX"),
		Country: stringPtr("US"),
	}, roster)

	// 2-artist control, also split across metros so the scene scope can't see
	// it whole either.
	duoA := suite.createArtistIn("Duo Band A", "Phoenix", "AZ")
	duoB := suite.createArtistIn("Duo Band B", "Austin", "TX")
	suite.seedLabelMemberships(&catalogm.Label{
		Name: "Duo Records", Slug: stringPtr("duo-records"),
	}, []*catalogm.Artist{duoA, duoB})

	// Over the threshold but unlinkable: a hub here would promise a click it
	// cannot honor, so the roster keeps its pairwise edges.
	un1 := suite.createArtistIn("Unlinkable Band 1", "Phoenix", "AZ")
	un2 := suite.createArtistIn("Unlinkable Band 2", "Austin", "TX")
	un3 := suite.createArtistIn("Unlinkable Band 3", "New York", "NY")
	suite.seedLabelMemberships(&catalogm.Label{
		Name: "Unlinkable Records", Slug: nil,
	}, []*catalogm.Artist{un1, un2, un3})

	catalogSet := suite.catalogArtistIDs()
	rows, err := queryLabelRosters(suite.db, catalogSet)
	suite.Require().NoError(err)
	suite.Len(rows, 9, "the catalog-wide set sees every membership row, unbounded by metro")

	hubs, err := buildLabelHubs(rows, catalogSet)
	suite.Require().NoError(err)

	// Only the linkable 3+ roster hubs.
	suite.Require().Len(hubs.Nodes, 1,
		"only 12XU is both over-threshold and linkable")
	hub := hubs.Nodes[0]
	suite.Equal("12XU", hub.Name)
	suite.Equal(contracts.SceneNodeKindLabel, hub.EntityType)
	suite.Equal("12xu", hub.Slug)
	suite.Equal("Austin", hub.City)
	suite.Equal("TX", hub.State)
	suite.Equal("US", hub.Country)
	suite.False(hub.IsIsolate)
	suite.Empty(hub.ClusterID)

	// One spoke per roster artist, each resolving to a real artist node.
	suite.Require().Len(hubs.Spokes, len(roster))
	spokeTargets := make(map[uint]bool, len(hubs.Spokes))
	for _, spoke := range hubs.Spokes {
		suite.Equal(hub.ID, spoke.SourceID)
		suite.Equal(contracts.SceneEdgeTypeOnLabel, spoke.Type)
		spokeTargets[spoke.TargetID] = true
	}
	for _, a := range roster {
		suite.True(spokeTargets[a.ID], "artist %d must have a membership spoke", a.ID)
	}

	// (a) All C(4,2) = 6 pairwise `shared_label` edges are replaced.
	replaced := 0
	for i := 0; i < len(roster); i++ {
		for j := i + 1; j < len(roster); j++ {
			if hubs.replacesSharedLabelEdge(roster[i].ID, roster[j].ID) {
				replaced++
			}
		}
	}
	suite.Equal(6, replaced, "every pair in the hubbed roster loses its pairwise edge")

	// (b) The 2-artist control keeps its pairwise edge.
	suite.False(hubs.replacesSharedLabelEdge(duoA.ID, duoB.ID),
		"a 2-artist label has no hub, so its pairwise edge must survive")

	// (c) The slug-less roster keeps its pairwise edges.
	suite.False(hubs.replacesSharedLabelEdge(un1.ID, un2.ID),
		"a slug-less label has no hub, so its pairwise edges must survive")

	// Scope contrast: read the SAME data bounded to Phoenix and 12XU is a
	// 2-artist overlap, below the threshold — the roster size that matters is
	// the one inside the payload.
	phoenixSet := []uint{phx1.ID, phx2.ID}
	phoenixRows, err := queryLabelRosters(suite.db, phoenixSet)
	suite.Require().NoError(err)
	suite.Len(phoenixRows, 2, "the scoped read sees only the in-set memberships")

	phoenixHubs, err := buildLabelHubs(phoenixRows, phoenixSet)
	suite.Require().NoError(err)
	suite.Empty(phoenixHubs.Nodes,
		"12XU is a 2-artist overlap inside one metro, so it stays pairwise there")
	suite.False(phoenixHubs.replacesSharedLabelEdge(phx1.ID, phx2.ID))
}
