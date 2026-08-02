package catalog

// Scene-graph label hubs (PSY-1530), end-to-end on the SceneService Postgres
// suite: a label roster arrives from DeriveSharedLabels as a complete C(n,2)
// clique, and the graph must serve it as one hub node plus n membership spokes.
//
// The unit-level rules live in label_hubs_test.go (pure builder). These
// tests cover the wiring the builder can't see: that the pairwise rows really
// are dropped from the response, that spokes clear the isolate flag, that the
// `types` filter takes the hubs with it, and that hub node IDs can't collide
// with artist IDs in the shared node-ID space.

import (
	"encoding/json"
	"time"

	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/contracts"
)

// seedLabelWithRoster creates a label and puts every artist on it, then writes
// the pairwise `shared_label` rows DeriveSharedLabels would derive — the clique
// this feature exists to collapse.
func (suite *SceneServiceIntegrationTestSuite) seedLabelWithRoster(
	labelName, slug, city, state, country string,
	artists []*catalogm.Artist,
) *catalogm.Label {
	label := &catalogm.Label{
		Name:    labelName,
		Slug:    stringPtr(slug),
		City:    stringPtr(city),
		State:   stringPtr(state),
		Country: stringPtr(country),
	}
	suite.seedLabelMemberships(label, artists)

	// Pairwise clique, canonical endpoint order (source < target) like the
	// deriver's CHECK constraint requires.
	detail, err := json.Marshal(map[string]any{
		"shared_count":      1,
		"normalized_weight": 0.0172,
		"label_names":       labelName,
	})
	suite.Require().NoError(err)
	for i := 0; i < len(artists); i++ {
		for j := i + 1; j < len(artists); j++ {
			src, tgt := artists[i].ID, artists[j].ID
			if src > tgt {
				src, tgt = tgt, src
			}
			raw := json.RawMessage(detail)
			suite.Require().NoError(suite.db.Create(&catalogm.ArtistRelationship{
				SourceArtistID:   src,
				TargetArtistID:   tgt,
				RelationshipType: catalogm.RelationshipTypeSharedLabel,
				Score:            0.0172,
				Detail:           &raw,
				AutoDerived:      true,
			}).Error)
		}
	}
	return label
}

func findLabelHub(graph *contracts.SceneGraphResponse, name string) *contracts.SceneGraphNode {
	for i := range graph.Nodes {
		if graph.Nodes[i].EntityType == contracts.SceneNodeKindLabel && graph.Nodes[i].Name == name {
			return &graph.Nodes[i]
		}
	}
	return nil
}

func countLinksOfType(graph *contracts.SceneGraphResponse, edgeType string) int {
	n := 0
	for _, l := range graph.Links {
		if l.Type == edgeType {
			n++
		}
	}
	return n
}

// The headline behavior: a 4-artist roster's 6 pairwise edges become 1 hub + 4
// spokes, and the pairwise rows are gone from the payload.
func (suite *SceneServiceIntegrationTestSuite) TestGetSceneGraph_LabelRosterCollapsesToHub() {
	venues, artists := suite.seedSceneData()
	user := suite.createUser()
	extra := suite.createArtist("Band D")
	suite.createApprovedShow("Show 6", venues[0].ID, extra.ID, user.ID,
		time.Now().UTC().AddDate(0, 0, 7))
	roster := []*catalogm.Artist{artists[0], artists[1], artists[2], extra}

	suite.seedLabelWithRoster("12XU", "12xu", "Austin", "TX", "US", roster)

	graph, err := suite.sceneService.GetSceneGraph("Phoenix", "AZ", nil, "")
	suite.Require().NoError(err)

	// C(4,2) = 6 pairwise rows were seeded; none may survive.
	suite.Equal(0, countLinksOfType(graph, catalogm.RelationshipTypeSharedLabel),
		"pairwise shared_label edges must be replaced by the hub's spokes")
	suite.Equal(len(roster), countLinksOfType(graph, contracts.SceneEdgeTypeOnLabel),
		"one membership spoke per in-scene roster artist")

	hub := findLabelHub(graph, "12XU")
	suite.Require().NotNil(hub, "label with a 3+ in-scene roster must emit a hub node")
	suite.Equal(1, graph.Scene.LabelCount)
	suite.Equal("12xu", hub.Slug)
	suite.Equal("Austin", hub.City, "hub carries its home for the canvas caption")
	suite.Equal("TX", hub.State)
	suite.Equal("US", hub.Country)
	suite.Empty(hub.ClusterID, "a hub joins no cluster (and so no hull)")
	suite.False(hub.IsIsolate, "a hub always has its roster's spokes")

	// The hub must not be counted as an artist — the truncation phrasing is
	// defined against artist_count.
	suite.Equal(len(graph.Nodes)-1, graph.Scene.ArtistCount,
		"artist_count counts artists only; hubs are counted in label_count")

	// Node IDs must stay unique across the two populations.
	seen := make(map[uint]bool, len(graph.Nodes))
	for _, n := range graph.Nodes {
		suite.False(seen[n.ID], "duplicate node id %d across artists and hubs", n.ID)
		seen[n.ID] = true
		suite.NotEmpty(n.EntityType, "every node must declare its kind")
	}

	// Every spoke must resolve to a real node on both ends.
	for _, l := range graph.Links {
		if l.Type != contracts.SceneEdgeTypeOnLabel {
			continue
		}
		suite.Equal(hub.ID, l.SourceID, "spokes originate at the hub")
		suite.True(seen[l.TargetID], "spoke target %d must be a node in the payload", l.TargetID)
		suite.False(l.IsCrossCluster, "spokes must not take the weak cross-cluster strength")
	}
}

// A 2-artist overlap keeps its pairwise edge: a hub there would add a node to
// say strictly less.
func (suite *SceneServiceIntegrationTestSuite) TestGetSceneGraph_BelowThresholdRosterStaysPairwise() {
	_, artists := suite.seedSceneData()

	suite.seedLabelWithRoster("Duo Records", "duo-records", "Phoenix", "AZ", "US",
		[]*catalogm.Artist{artists[0], artists[1]})

	graph, err := suite.sceneService.GetSceneGraph("Phoenix", "AZ", nil, "")
	suite.Require().NoError(err)

	suite.Equal(1, countLinksOfType(graph, catalogm.RelationshipTypeSharedLabel),
		"a 2-artist label keeps its single pairwise edge")
	suite.Equal(0, countLinksOfType(graph, contracts.SceneEdgeTypeOnLabel))
	suite.Nil(findLabelHub(graph, "Duo Records"))
	suite.Equal(0, graph.Scene.LabelCount)
}

// An artist whose ONLY connection is its label must not be shelved as
// not-yet-connected: its spoke anchors it to the hub.
func (suite *SceneServiceIntegrationTestSuite) TestGetSceneGraph_SpokeClearsIsolateFlag() {
	venues, artists := suite.seedSceneData()
	user := suite.createUser()

	// This artist has a show (so it is in the roster) but no relationship rows.
	lonely := suite.createArtist("Label Only Band")
	suite.createApprovedShow("Show 7", venues[0].ID, lonely.ID, user.ID,
		time.Now().UTC().AddDate(0, 0, 7))

	suite.seedLabelWithRoster("Solo Signing", "solo-signing", "Phoenix", "AZ", "US",
		[]*catalogm.Artist{artists[0], artists[1], lonely})

	graph, err := suite.sceneService.GetSceneGraph("Phoenix", "AZ", nil, "")
	suite.Require().NoError(err)

	node := findSceneGraphNode(graph, lonely.ID)
	suite.Require().NotNil(node)
	suite.False(node.IsIsolate,
		"an artist reached only by a label spoke is connected, not shelved")
}

// Hubs are a projection of shared_label, so filtering that type out must take
// the hubs with it — otherwise a caller who excluded label edges still gets
// hubs whose spokes they asked not to see.
func (suite *SceneServiceIntegrationTestSuite) TestGetSceneGraph_TypeFilterExcludesHubs() {
	_, artists := suite.seedSceneData()
	suite.seedLabelWithRoster("Filtered Records", "filtered-records", "Phoenix", "AZ", "US",
		[]*catalogm.Artist{artists[0], artists[1], artists[2]})

	graph, err := suite.sceneService.GetSceneGraph("Phoenix", "AZ",
		[]string{catalogm.RelationshipTypeSharedBills}, "")
	suite.Require().NoError(err)

	suite.Nil(findLabelHub(graph, "Filtered Records"),
		"excluding shared_label must exclude its hub projection")
	suite.Equal(0, countLinksOfType(graph, contracts.SceneEdgeTypeOnLabel))
	suite.Equal(0, graph.Scene.LabelCount)

	// Requesting shared_label explicitly brings the hub back.
	withLabels, err := suite.sceneService.GetSceneGraph("Phoenix", "AZ",
		[]string{catalogm.RelationshipTypeSharedLabel}, "")
	suite.Require().NoError(err)
	suite.NotNil(findLabelHub(withLabels, "Filtered Records"))
}

// A slug-less label cannot be opened, so it must not occupy the canvas as a hub
// promising a click it can't honor — its roster keeps the pairwise edges.
func (suite *SceneServiceIntegrationTestSuite) TestGetSceneGraph_SlugLessLabelIsNotHubbed() {
	_, artists := suite.seedSceneData()
	label := &catalogm.Label{Name: "Unlinkable Records"}
	suite.Require().NoError(suite.db.Create(label).Error)
	for _, a := range artists {
		suite.Require().NoError(suite.db.Create(&catalogm.ArtistLabel{
			ArtistID: a.ID, LabelID: label.ID,
		}).Error)
	}

	graph, err := suite.sceneService.GetSceneGraph("Phoenix", "AZ", nil, "")
	suite.Require().NoError(err)

	suite.Nil(findLabelHub(graph, "Unlinkable Records"))
	suite.Equal(0, graph.Scene.LabelCount)
}
