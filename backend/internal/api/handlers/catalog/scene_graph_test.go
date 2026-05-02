package catalog

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	"psychic-homily-backend/internal/models"
)

// SceneGraphIntegrationSuite covers the PSY-367 scene-scale graph endpoint.
//
// The endpoint stitches together: scene artist set (derived from approved shows),
// per-artist primary-venue clustering (computed at query time), and stored
// `artist_relationships` rows scoped to the in-scene artist set. These tests
// pin down the cluster sizing thresholds, cross-scene leakage prevention, the
// `is_isolate` derivation, and the type filter.
type SceneGraphIntegrationSuite struct {
	suite.Suite
	deps *testhelpers.IntegrationDeps
}

func (s *SceneGraphIntegrationSuite) SetupSuite() {
	s.deps = testhelpers.SetupIntegrationDeps(s.T())
}

func (s *SceneGraphIntegrationSuite) TearDownTest() {
	testhelpers.CleanupTables(s.deps.DB)
}

func (s *SceneGraphIntegrationSuite) TearDownSuite() {
	s.deps.TestDB.Cleanup()
}

func TestSceneGraphIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	suite.Run(t, new(SceneGraphIntegrationSuite))
}

// --- Helpers ---

// seedSceneVenue creates a verified venue at (city, state). The scene service
// requires `sceneMinVenues` (= 2) verified venues to consider a scene valid;
// most tests below seed at least two even when only one carries activity.
func (s *SceneGraphIntegrationSuite) seedSceneVenue(name, city, state string) *models.Venue {
	return testhelpers.CreateVerifiedVenue(s.deps.DB, name, city, state)
}

// seedSceneArtist creates a bare artist row. Slug doesn't matter for graph
// computation but the response payload surfaces it.
func (s *SceneGraphIntegrationSuite) seedSceneArtist(name string) *models.Artist {
	a := &models.Artist{Name: name}
	s.deps.DB.Create(a)
	slug := fmt.Sprintf("artist-%d", a.ID)
	s.deps.DB.Model(a).Update("slug", slug)
	return a
}

// seedShowAtVenue creates one approved show on `eventDate` at `venue` with the
// given artists in lineup order (position 0 = headliner). Returns the show ID.
func (s *SceneGraphIntegrationSuite) seedShowAtVenue(eventDate time.Time, venue *models.Venue, artistIDs ...uint) uint {
	user := testhelpers.CreateTestUser(s.deps.DB)
	show := &models.Show{
		Title:       fmt.Sprintf("Show at %s on %s", venue.Name, eventDate.Format("2006-01-02")),
		EventDate:   eventDate,
		City:        testhelpers.StringPtr(venue.City),
		State:       testhelpers.StringPtr(venue.State),
		Status:      models.ShowStatusApproved,
		SubmittedBy: &user.ID,
	}
	s.deps.DB.Create(show)
	s.deps.DB.Exec("INSERT INTO show_venues (show_id, venue_id) VALUES (?, ?)", show.ID, venue.ID)
	for i, aid := range artistIDs {
		setType := "opener"
		if i == 0 {
			setType = "headliner"
		}
		s.deps.DB.Exec(
			"INSERT INTO show_artists (show_id, artist_id, position, set_type) VALUES (?, ?, ?, ?)",
			show.ID, aid, i, setType,
		)
	}
	return show.ID
}

// seedSharedBillsRel inserts a canonical shared_bills relationship between two
// artists with the given shared-show count carried in the `detail` JSONB. Score
// scaled the same way DeriveSharedBills does (count/10, capped at 1.0).
func (s *SceneGraphIntegrationSuite) seedSharedBillsRel(a, b uint, sharedCount int) {
	src, tgt := models.CanonicalOrder(a, b)
	score := float32(sharedCount) / 10.0
	if score > 1.0 {
		score = 1.0
	}
	detail, _ := json.Marshal(map[string]any{
		"shared_count": sharedCount,
		"last_shared":  time.Now().UTC().Format("2006-01-02"),
	})
	raw := json.RawMessage(detail)
	rel := models.ArtistRelationship{
		SourceArtistID:   src,
		TargetArtistID:   tgt,
		RelationshipType: models.RelationshipTypeSharedBills,
		Score:            score,
		AutoDerived:      true,
		Detail:           &raw,
	}
	s.deps.DB.Create(&rel)
}

// seedTypedRel inserts a relationship of the given type between two artists.
// Used to exercise the `types` query filter.
func (s *SceneGraphIntegrationSuite) seedTypedRel(a, b uint, relType string) {
	src, tgt := models.CanonicalOrder(a, b)
	rel := models.ArtistRelationship{
		SourceArtistID:   src,
		TargetArtistID:   tgt,
		RelationshipType: relType,
		Score:            0.5,
		AutoDerived:      true,
	}
	s.deps.DB.Create(&rel)
}

// --- Tests ---

// TestSceneGraph_NotFound: scene with no verified venues returns the same
// "scene not found" error path as the other scene endpoints.
func (s *SceneGraphIntegrationSuite) TestSceneGraph_NotFound() {
	_, err := s.deps.SceneService.GetSceneGraph("Nowhere", "ZZ", nil)
	s.Require().Error(err)
}

// TestSceneGraph_NoArtists: a scene that meets the venue threshold but has no
// approved shows yields an empty graph (artist_count = 0, edge_count = 0,
// nodes/links/clusters are non-nil empty arrays so the JSON contract is stable).
func (s *SceneGraphIntegrationSuite) TestSceneGraph_NoArtists() {
	s.seedSceneVenue("Empty Venue 1", "Phoenix", "AZ")
	s.seedSceneVenue("Empty Venue 2", "Phoenix", "AZ")

	graph, err := s.deps.SceneService.GetSceneGraph("Phoenix", "AZ", nil)
	s.Require().NoError(err)
	s.Equal(0, graph.Scene.ArtistCount)
	s.Equal(0, graph.Scene.EdgeCount)
	s.Equal("Phoenix", graph.Scene.City)
	s.Equal("AZ", graph.Scene.State)
	s.Equal("phoenix-az", graph.Scene.Slug)
	s.Empty(graph.Nodes)
	s.Empty(graph.Links)
	s.Empty(graph.Clusters)
	s.NotNil(graph.Nodes, "nodes should be empty array, not nil")
	s.NotNil(graph.Links, "links should be empty array, not nil")
	s.NotNil(graph.Clusters, "clusters should be empty array, not nil")
}

// TestSceneGraph_IsolatedNodes: every artist plays a Phoenix show but none have
// stored relationships. All nodes carry is_isolate=true; links is empty.
func (s *SceneGraphIntegrationSuite) TestSceneGraph_IsolatedNodes() {
	venue := s.seedSceneVenue("Valley Bar", "Phoenix", "AZ")
	s.seedSceneVenue("Crescent Ballroom", "Phoenix", "AZ") // second verified venue

	now := time.Now().UTC()
	a1 := s.seedSceneArtist("Iso-A")
	a2 := s.seedSceneArtist("Iso-B")
	a3 := s.seedSceneArtist("Iso-C")
	s.seedShowAtVenue(now.AddDate(0, -1, 0), venue, a1.ID)
	s.seedShowAtVenue(now.AddDate(0, -2, 0), venue, a2.ID)
	s.seedShowAtVenue(now.AddDate(0, -3, 0), venue, a3.ID)

	graph, err := s.deps.SceneService.GetSceneGraph("Phoenix", "AZ", nil)
	s.Require().NoError(err)
	s.Equal(3, graph.Scene.ArtistCount)
	s.Equal(0, graph.Scene.EdgeCount)
	s.Empty(graph.Links)
	s.Require().Len(graph.Nodes, 3)
	for _, n := range graph.Nodes {
		s.True(n.IsIsolate, "node %q should be marked isolate", n.Name)
	}
}

// TestSceneGraph_PrimaryVenueClusters: 6 artists at Valley Bar + 6 at Crescent
// produce two first-class clusters; cluster_id on each node points at the right
// bucket; cluster sizes match. Stored shared_bills relationships add edges,
// and intra-cluster vs cross-cluster edges are flagged consistently.
func (s *SceneGraphIntegrationSuite) TestSceneGraph_PrimaryVenueClusters() {
	valley := s.seedSceneVenue("Valley Bar", "Phoenix", "AZ")
	crescent := s.seedSceneVenue("Crescent Ballroom", "Phoenix", "AZ")

	now := time.Now().UTC()

	// 6 artists who play primarily at Valley Bar.
	valleyArtists := make([]*models.Artist, 0, 6)
	for i := 0; i < 6; i++ {
		a := s.seedSceneArtist(fmt.Sprintf("Valley-A%d", i))
		s.seedShowAtVenue(now.AddDate(0, 0, -i), valley, a.ID)
		s.seedShowAtVenue(now.AddDate(0, 0, -(i+10)), valley, a.ID)
		valleyArtists = append(valleyArtists, a)
	}

	// 6 artists who play primarily at Crescent Ballroom.
	crescentArtists := make([]*models.Artist, 0, 6)
	for i := 0; i < 6; i++ {
		a := s.seedSceneArtist(fmt.Sprintf("Crescent-A%d", i))
		s.seedShowAtVenue(now.AddDate(0, 0, -i), crescent, a.ID)
		s.seedShowAtVenue(now.AddDate(0, 0, -(i+10)), crescent, a.ID)
		crescentArtists = append(crescentArtists, a)
	}

	// Intra-Valley edge: should NOT be flagged is_cross_cluster.
	s.seedSharedBillsRel(valleyArtists[0].ID, valleyArtists[1].ID, 4)
	// Cross-cluster edge: Valley ↔ Crescent.
	s.seedSharedBillsRel(valleyArtists[0].ID, crescentArtists[0].ID, 3)

	graph, err := s.deps.SceneService.GetSceneGraph("Phoenix", "AZ", nil)
	s.Require().NoError(err)
	s.Equal(12, graph.Scene.ArtistCount)
	s.Equal(2, graph.Scene.EdgeCount)

	// Two first-class clusters; sizes 6 + 6.
	s.Require().Len(graph.Clusters, 2)
	s.Equal(6, graph.Clusters[0].Size)
	s.Equal(6, graph.Clusters[1].Size)
	s.NotEqual(graph.Clusters[0].ID, graph.Clusters[1].ID)
	s.True(graph.Clusters[0].ColorIndex >= 0 && graph.Clusters[0].ColorIndex < 8)
	s.True(graph.Clusters[1].ColorIndex >= 0 && graph.Clusters[1].ColorIndex < 8)

	// Cluster-id-by-artist sanity.
	idByArtist := make(map[uint]string, len(graph.Nodes))
	for _, n := range graph.Nodes {
		idByArtist[n.ID] = n.ClusterID
	}
	for _, a := range valleyArtists {
		s.Equalf(idByArtist[valleyArtists[0].ID], idByArtist[a.ID], "Valley artists must share a cluster id")
	}
	for _, a := range crescentArtists {
		s.Equalf(idByArtist[crescentArtists[0].ID], idByArtist[a.ID], "Crescent artists must share a cluster id")
	}
	s.NotEqual(idByArtist[valleyArtists[0].ID], idByArtist[crescentArtists[0].ID])

	// Edge cross-cluster flags.
	var intra, cross int
	for _, l := range graph.Links {
		if l.IsCrossCluster {
			cross++
		} else {
			intra++
		}
	}
	s.Equal(1, intra, "expected one intra-cluster edge")
	s.Equal(1, cross, "expected one cross-cluster edge")

	// At least the two artists with the cross-cluster edge are non-isolates.
	for _, n := range graph.Nodes {
		if n.ID == valleyArtists[0].ID || n.ID == valleyArtists[1].ID || n.ID == crescentArtists[0].ID {
			s.False(n.IsIsolate, "%s should not be isolate", n.Name)
		}
	}
}

// TestSceneGraph_OtherClusterRollup: 5 artists at one venue (below the
// sceneClusterMinSize=6 threshold) all roll up to the "other" bucket; the
// payload's clusters slice contains only one entry — "other".
func (s *SceneGraphIntegrationSuite) TestSceneGraph_OtherClusterRollup() {
	tiny := s.seedSceneVenue("Tiny Bar", "Phoenix", "AZ")
	s.seedSceneVenue("Spare Verified", "Phoenix", "AZ")
	now := time.Now().UTC()
	for i := 0; i < 5; i++ {
		a := s.seedSceneArtist(fmt.Sprintf("Tiny-A%d", i))
		s.seedShowAtVenue(now.AddDate(0, 0, -i), tiny, a.ID)
	}
	graph, err := s.deps.SceneService.GetSceneGraph("Phoenix", "AZ", nil)
	s.Require().NoError(err)
	s.Equal(5, graph.Scene.ArtistCount)
	s.Require().Len(graph.Clusters, 1)
	s.Equal("other", graph.Clusters[0].ID)
	s.Equal(5, graph.Clusters[0].Size)
	s.Equal(-1, graph.Clusters[0].ColorIndex)
	for _, n := range graph.Nodes {
		s.Equal("other", n.ClusterID)
	}
}

// TestSceneGraph_CrossSceneLeakagePrevented: a Phoenix artist + a Tucson artist
// share a stored relationship. Phoenix's scene graph must NOT include the
// Tucson artist as a node nor the cross-scene edge as a link.
func (s *SceneGraphIntegrationSuite) TestSceneGraph_CrossSceneLeakagePrevented() {
	phx := s.seedSceneVenue("Valley Bar", "Phoenix", "AZ")
	s.seedSceneVenue("Crescent Ballroom", "Phoenix", "AZ")
	tus := s.seedSceneVenue("Hotel Congress", "Tucson", "AZ")
	s.seedSceneVenue("191 Toole", "Tucson", "AZ")

	now := time.Now().UTC()
	phxArtist := s.seedSceneArtist("PHX-Only")
	tusArtist := s.seedSceneArtist("TUS-Only")
	s.seedShowAtVenue(now.AddDate(0, -1, 0), phx, phxArtist.ID)
	s.seedShowAtVenue(now.AddDate(0, -2, 0), tus, tusArtist.ID)

	// Cross-scene shared_bills — should not appear in either scene's response.
	s.seedSharedBillsRel(phxArtist.ID, tusArtist.ID, 3)

	phxGraph, err := s.deps.SceneService.GetSceneGraph("Phoenix", "AZ", nil)
	s.Require().NoError(err)
	s.Equal(1, phxGraph.Scene.ArtistCount)
	s.Equal(0, phxGraph.Scene.EdgeCount)
	s.Empty(phxGraph.Links)
	s.Require().Len(phxGraph.Nodes, 1)
	s.Equal(phxArtist.ID, phxGraph.Nodes[0].ID)
	s.True(phxGraph.Nodes[0].IsIsolate)

	tusGraph, err := s.deps.SceneService.GetSceneGraph("Tucson", "AZ", nil)
	s.Require().NoError(err)
	s.Equal(1, tusGraph.Scene.ArtistCount)
	s.Equal(0, tusGraph.Scene.EdgeCount)
}

// TestSceneGraph_TypeFilter: stored relationships of multiple types within the
// scene; passing types=[shared_label] returns only shared_label edges, even
// though shared_bills also exists between the same artists.
func (s *SceneGraphIntegrationSuite) TestSceneGraph_TypeFilter() {
	venue := s.seedSceneVenue("Valley Bar", "Phoenix", "AZ")
	s.seedSceneVenue("Crescent Ballroom", "Phoenix", "AZ")
	now := time.Now().UTC()

	a := s.seedSceneArtist("Filter-A")
	b := s.seedSceneArtist("Filter-B")
	s.seedShowAtVenue(now.AddDate(0, -1, 0), venue, a.ID)
	s.seedShowAtVenue(now.AddDate(0, -2, 0), venue, b.ID)

	s.seedTypedRel(a.ID, b.ID, models.RelationshipTypeSharedBills)
	s.seedTypedRel(a.ID, b.ID, models.RelationshipTypeSharedLabel)

	all, err := s.deps.SceneService.GetSceneGraph("Phoenix", "AZ", nil)
	s.Require().NoError(err)
	s.Equal(2, all.Scene.EdgeCount)

	onlyShared, err := s.deps.SceneService.GetSceneGraph("Phoenix", "AZ", []string{models.RelationshipTypeSharedLabel})
	s.Require().NoError(err)
	s.Equal(1, onlyShared.Scene.EdgeCount)
	s.Require().Len(onlyShared.Links, 1)
	s.Equal(models.RelationshipTypeSharedLabel, onlyShared.Links[0].Type)

	// Unknown type silently drops to allowlist; result is zero edges, not an error.
	bogus, err := s.deps.SceneService.GetSceneGraph("Phoenix", "AZ", []string{"definitely_not_a_type"})
	s.Require().NoError(err)
	s.Equal(0, bogus.Scene.EdgeCount)
}
