package catalog

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"

	apperrors "psychic-homily-backend/internal/errors"
	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/testutil"
)

// FestivalGraphIntegrationSuite covers the PSY-1080 festival-scoped co-bill
// graph endpoint.
//
// The endpoint stitches together: the festival's lineup (festival_artists),
// billing-tier clustering, stored artist_relationships rows scoped to the
// lineup set, and the query-time festival_cobill derivation that EXCLUDES the
// festival being viewed (every lineup pair trivially shares this festival —
// counting it would produce a structureless complete graph). These tests pin
// down the tier clusters, the current-festival exclusion, the type allowlist
// + filter semantics, is_isolate / is_cross_cluster derivation, and the
// 150-node cap dropping from the bottom of the bill.
type FestivalGraphIntegrationSuite struct {
	suite.Suite
	testDB *testutil.TestDatabase
	db     *gorm.DB
	svc    *FestivalService
}

func (s *FestivalGraphIntegrationSuite) SetupSuite() {
	s.testDB = testutil.SetupTestPostgres(s.T())
	s.db = s.testDB.DB
	s.svc = &FestivalService{db: s.testDB.DB}
}

func (s *FestivalGraphIntegrationSuite) TearDownSuite() {
	s.testDB.Cleanup()
}

func (s *FestivalGraphIntegrationSuite) TearDownTest() {
	sqlDB, err := s.db.DB()
	s.Require().NoError(err)
	// Delete in FK-safe order.
	_, _ = sqlDB.Exec("DELETE FROM festival_artists")
	_, _ = sqlDB.Exec("DELETE FROM festival_venues")
	_, _ = sqlDB.Exec("DELETE FROM festivals")
	_, _ = sqlDB.Exec("DELETE FROM artist_relationships")
	_, _ = sqlDB.Exec("DELETE FROM artists")
}

func TestFestivalGraphIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	suite.Run(t, new(FestivalGraphIntegrationSuite))
}

// --- Helpers ---

func (s *FestivalGraphIntegrationSuite) createArtist(name string) *catalogm.Artist {
	a := &catalogm.Artist{Name: name}
	s.Require().NoError(s.db.Create(a).Error)
	slug := fmt.Sprintf("artist-%d", a.ID)
	s.db.Model(a).Update("slug", slug)
	return a
}

func (s *FestivalGraphIntegrationSuite) createFestival(name, seriesSlug string, year int, startDate string) *contracts.FestivalDetailResponse {
	req := &contracts.CreateFestivalRequest{
		Name:        name,
		SeriesSlug:  seriesSlug,
		EditionYear: year,
		StartDate:   startDate,
		EndDate:     startDate,
		Status:      "confirmed",
	}
	resp, err := s.svc.CreateFestival(req)
	s.Require().NoError(err)
	return resp
}

func (s *FestivalGraphIntegrationSuite) addToLineup(festivalID, artistID uint, tier string, position int) {
	_, err := s.svc.AddFestivalArtist(festivalID, &contracts.AddFestivalArtistRequest{
		ArtistID:    artistID,
		BillingTier: tier,
		Position:    position,
	})
	s.Require().NoError(err)
}

// seedTypedRelationship inserts a stored relationship of the given type.
func (s *FestivalGraphIntegrationSuite) seedTypedRelationship(a, b uint, relType string, score float32) {
	src, tgt := catalogm.CanonicalOrder(a, b)
	rel := catalogm.ArtistRelationship{
		SourceArtistID:   src,
		TargetArtistID:   tgt,
		RelationshipType: relType,
		Score:            score,
		AutoDerived:      true,
	}
	s.Require().NoError(s.db.Create(&rel).Error)
}

func linksByType(links []contracts.FestivalGraphLink) map[string][]contracts.FestivalGraphLink {
	out := make(map[string][]contracts.FestivalGraphLink)
	for _, l := range links {
		out[l.Type] = append(out[l.Type], l)
	}
	return out
}

// --- Tests ---

// TestGetFestivalGraph_NotFound: unknown festival id surfaces the typed
// festival-not-found error (mapped to 404 by the handler).
func (s *FestivalGraphIntegrationSuite) TestGetFestivalGraph_NotFound() {
	_, err := s.svc.GetFestivalGraph(999999, nil)
	s.Require().Error(err)
	var festErr *apperrors.FestivalError
	s.Require().ErrorAs(err, &festErr)
	s.Equal(apperrors.CodeFestivalNotFound, festErr.Code)
}

// TestGetFestivalGraph_EmptyLineup: a festival with no artists yields an
// empty graph with the festival info block populated and non-nil empty
// arrays so the JSON contract is stable.
func (s *FestivalGraphIntegrationSuite) TestGetFestivalGraph_EmptyLineup() {
	fest := s.createFestival("Empty Fest 2026", "empty-fest", 2026, "2026-10-01")

	graph, err := s.svc.GetFestivalGraph(fest.ID, nil)
	s.Require().NoError(err)
	s.Equal(fest.ID, graph.Festival.ID)
	s.Equal("Empty Fest 2026", graph.Festival.Name)
	s.Equal(fest.Slug, graph.Festival.Slug)
	s.Equal(2026, graph.Festival.Year)
	s.Equal(0, graph.Festival.ArtistCount)
	s.Equal(0, graph.Festival.EdgeCount)
	s.Empty(graph.Nodes)
	s.Empty(graph.Links)
	s.Empty(graph.Clusters)
	s.NotNil(graph.Nodes, "nodes should be empty array, not nil")
	s.NotNil(graph.Links, "links should be empty array, not nil")
	s.NotNil(graph.Clusters, "clusters should be empty array, not nil")
}

// TestGetFestivalGraph_TierClustersAndCrossClusterFlags: lineup spread across
// three tiers produces one cluster per tier (headliner-first, contiguous
// color indices). Stored shared_bills edges are flagged is_cross_cluster only
// when the endpoints sit on different tiers; lineup members without any edge
// are isolates.
func (s *FestivalGraphIntegrationSuite) TestGetFestivalGraph_TierClustersAndCrossClusterFlags() {
	fest := s.createFestival("Tier Fest 2026", "tier-fest", 2026, "2026-10-01")

	head1 := s.createArtist("Head One")
	head2 := s.createArtist("Head Two")
	mid1 := s.createArtist("Mid One")
	mid2 := s.createArtist("Mid Two")
	local1 := s.createArtist("Local One")

	s.addToLineup(fest.ID, head1.ID, "headliner", 0)
	s.addToLineup(fest.ID, head2.ID, "headliner", 1)
	s.addToLineup(fest.ID, mid1.ID, "mid_card", 0)
	s.addToLineup(fest.ID, mid2.ID, "mid_card", 1)
	s.addToLineup(fest.ID, local1.ID, "local", 0)

	// Intra-tier edge (headliner ↔ headliner) and cross-tier edge
	// (headliner ↔ mid_card).
	s.seedTypedRelationship(head1.ID, head2.ID, catalogm.RelationshipTypeSharedBills, 0.4)
	s.seedTypedRelationship(head1.ID, mid1.ID, catalogm.RelationshipTypeSharedBills, 0.3)

	graph, err := s.svc.GetFestivalGraph(fest.ID, nil)
	s.Require().NoError(err)
	s.Equal(5, graph.Festival.ArtistCount)
	s.Equal(2, graph.Festival.EdgeCount)

	// Clusters: headliner (2), mid_card (2), local (1) — in that order with
	// contiguous color indices.
	s.Require().Len(graph.Clusters, 3)
	s.Equal("tier_headliner", graph.Clusters[0].ID)
	s.Equal("Headliner", graph.Clusters[0].Label)
	s.Equal(2, graph.Clusters[0].Size)
	s.Equal(0, graph.Clusters[0].ColorIndex)
	s.Equal("tier_mid_card", graph.Clusters[1].ID)
	s.Equal(2, graph.Clusters[1].Size)
	s.Equal(1, graph.Clusters[1].ColorIndex)
	s.Equal("tier_local", graph.Clusters[2].ID)
	s.Equal(1, graph.Clusters[2].Size)
	s.Equal(2, graph.Clusters[2].ColorIndex)

	// Nodes are in bill order (tier rank, then position) and carry tier
	// cluster ids.
	s.Require().Len(graph.Nodes, 5)
	s.Equal(head1.ID, graph.Nodes[0].ID)
	s.Equal(head2.ID, graph.Nodes[1].ID)
	clusterByArtist := make(map[uint]string, 5)
	isolateByArtist := make(map[uint]bool, 5)
	for _, n := range graph.Nodes {
		clusterByArtist[n.ID] = n.ClusterID
		isolateByArtist[n.ID] = n.IsIsolate
	}
	s.Equal("tier_headliner", clusterByArtist[head1.ID])
	s.Equal("tier_mid_card", clusterByArtist[mid1.ID])
	s.Equal("tier_local", clusterByArtist[local1.ID])

	// Edge cross-cluster flags: intra-tier edge false, cross-tier edge true.
	var intra, cross int
	for _, l := range graph.Links {
		s.Equal(catalogm.RelationshipTypeSharedBills, l.Type)
		if l.IsCrossCluster {
			cross++
		} else {
			intra++
		}
	}
	s.Equal(1, intra)
	s.Equal(1, cross)

	// Isolates: mid2 and local1 have no edges.
	s.False(isolateByArtist[head1.ID])
	s.False(isolateByArtist[head2.ID])
	s.False(isolateByArtist[mid1.ID])
	s.True(isolateByArtist[mid2.ID])
	s.True(isolateByArtist[local1.ID])
}

// TestGetFestivalGraph_CobillExcludesCurrentFestival: every pair on the
// lineup shares THIS festival by definition, so cobill edges must count only
// OTHER shared festivals. A+B also share "Other Fest" → exactly one
// festival_cobill edge with count=1 and the other festival's name in detail;
// A+C and B+C (who share nothing but this festival) get no edge.
func (s *FestivalGraphIntegrationSuite) TestGetFestivalGraph_CobillExcludesCurrentFestival() {
	fest := s.createFestival("Main Fest 2026", "main-fest", 2026, "2026-10-01")
	other := s.createFestival("Other Fest 2026", "other-fest", 2026, "2026-03-01")

	a := s.createArtist("Artist A")
	b := s.createArtist("Artist B")
	c := s.createArtist("Artist C")

	s.addToLineup(fest.ID, a.ID, "headliner", 0)
	s.addToLineup(fest.ID, b.ID, "mid_card", 0)
	s.addToLineup(fest.ID, c.ID, "mid_card", 1)

	s.addToLineup(other.ID, a.ID, "mid_card", 0)
	s.addToLineup(other.ID, b.ID, "mid_card", 1)

	graph, err := s.svc.GetFestivalGraph(fest.ID, nil)
	s.Require().NoError(err)
	s.Equal(3, graph.Festival.ArtistCount)

	byType := linksByType(graph.Links)
	cobill := byType["festival_cobill"]
	s.Require().Len(cobill, 1, "only the A-B pair shares a festival other than the one being viewed")
	s.Equal(1, graph.Festival.EdgeCount)

	edge := cobill[0]
	srcA, tgtB := catalogm.CanonicalOrder(a.ID, b.ID)
	s.Equal(srcA, edge.SourceID)
	s.Equal(tgtB, edge.TargetID)
	// 1 shared (other) festival, started 2026-03-01 → within the 2-year
	// recency window: min(1/3, 1.0) * 1.2 = 0.4.
	s.InDelta(0.4, edge.Score, 0.0001)
	// Headliner ↔ mid_card on this bill → cross-cluster.
	s.True(edge.IsCrossCluster)

	detail, ok := edge.Detail.(map[string]interface{})
	s.Require().True(ok, "cobill detail should be the PSY-363 map shape")
	s.Equal(1, detail["count"])
	s.Equal("Other Fest 2026", detail["festival_names"])
	s.Equal(2026, detail["most_recent_year"])

	// C shares only the viewed festival with A/B → isolate.
	for _, n := range graph.Nodes {
		if n.ID == c.ID {
			s.True(n.IsIsolate, "artist C must be an isolate")
		} else {
			s.False(n.IsIsolate)
		}
	}
}

// TestGetFestivalGraph_StoredTypesAllowlist: stored edges between lineup
// members come through with their type field intact for every allowlisted
// type; member_of is filtered out; edges touching a non-lineup artist are
// excluded.
func (s *FestivalGraphIntegrationSuite) TestGetFestivalGraph_StoredTypesAllowlist() {
	fest := s.createFestival("Allow Fest 2026", "allow-fest", 2026, "2026-10-01")

	a := s.createArtist("Allow A")
	b := s.createArtist("Allow B")
	c := s.createArtist("Allow C")
	outsider := s.createArtist("Outsider")

	s.addToLineup(fest.ID, a.ID, "headliner", 0)
	s.addToLineup(fest.ID, b.ID, "mid_card", 0)
	s.addToLineup(fest.ID, c.ID, "mid_card", 1)

	s.seedTypedRelationship(a.ID, b.ID, catalogm.RelationshipTypeSharedBills, 0.5)
	s.seedTypedRelationship(a.ID, c.ID, catalogm.RelationshipTypeSharedLabel, 0.5)
	s.seedTypedRelationship(b.ID, c.ID, catalogm.RelationshipTypeSimilar, 0.5)
	s.seedTypedRelationship(a.ID, b.ID, catalogm.RelationshipTypeRadioCooccurrence, 0.5)
	// Not allowlisted at festival scope.
	s.seedTypedRelationship(b.ID, c.ID, catalogm.RelationshipTypeMemberOf, 0.5)
	// Allowlisted type, but one endpoint is off the lineup.
	s.seedTypedRelationship(a.ID, outsider.ID, catalogm.RelationshipTypeSharedBills, 0.5)

	graph, err := s.svc.GetFestivalGraph(fest.ID, nil)
	s.Require().NoError(err)

	byType := linksByType(graph.Links)
	s.Len(byType[catalogm.RelationshipTypeSharedBills], 1)
	s.Len(byType[catalogm.RelationshipTypeSharedLabel], 1)
	s.Len(byType[catalogm.RelationshipTypeSimilar], 1)
	s.Len(byType[catalogm.RelationshipTypeRadioCooccurrence], 1)
	s.Empty(byType[catalogm.RelationshipTypeMemberOf], "member_of is not allowlisted at festival scope")
	s.Equal(4, graph.Festival.EdgeCount)

	// The outsider edge is gone and the outsider is not a node.
	for _, n := range graph.Nodes {
		s.NotEqual(outsider.ID, n.ID)
	}
}

// TestGetFestivalGraph_TypeFilter: a types filter restricts the edge set; a
// non-empty filter that resolves to nothing yields zero edges (not "all
// types"), leaving every node an isolate.
func (s *FestivalGraphIntegrationSuite) TestGetFestivalGraph_TypeFilter() {
	fest := s.createFestival("Filter Fest 2026", "filter-fest", 2026, "2026-10-01")
	other := s.createFestival("Filter Other 2026", "filter-other", 2026, "2026-03-01")

	a := s.createArtist("Filter A")
	b := s.createArtist("Filter B")
	s.addToLineup(fest.ID, a.ID, "headliner", 0)
	s.addToLineup(fest.ID, b.ID, "mid_card", 0)
	s.addToLineup(other.ID, a.ID, "mid_card", 0)
	s.addToLineup(other.ID, b.ID, "mid_card", 1)

	s.seedTypedRelationship(a.ID, b.ID, catalogm.RelationshipTypeSharedBills, 0.5)

	// shared_bills only: the cobill edge (A+B share "Filter Other") is excluded.
	graph, err := s.svc.GetFestivalGraph(fest.ID, []string{"shared_bills"})
	s.Require().NoError(err)
	s.Require().Len(graph.Links, 1)
	s.Equal(catalogm.RelationshipTypeSharedBills, graph.Links[0].Type)

	// festival_cobill only: the stored edge is excluded.
	graph, err = s.svc.GetFestivalGraph(fest.ID, []string{"festival_cobill"})
	s.Require().NoError(err)
	s.Require().Len(graph.Links, 1)
	s.Equal("festival_cobill", graph.Links[0].Type)

	// Unknown-only filter short-circuits to zero edges.
	graph, err = s.svc.GetFestivalGraph(fest.ID, []string{"bogus_type"})
	s.Require().NoError(err)
	s.Empty(graph.Links)
	s.Equal(0, graph.Festival.EdgeCount)
	for _, n := range graph.Nodes {
		s.True(n.IsIsolate)
	}
}

// TestGetFestivalGraph_NodeCapKeepsTopOfBill: a lineup over
// festivalGraphMaxNodes is capped at the ceiling, dropping from the bottom
// of the bill (tier rank, then position, then name) — headliners always stay.
func (s *FestivalGraphIntegrationSuite) TestGetFestivalGraph_NodeCapKeepsTopOfBill() {
	fest := s.createFestival("Cap Fest 2026", "cap-fest", 2026, "2026-10-01")

	headliners := make([]*catalogm.Artist, 0, 3)
	for i := 0; i < 3; i++ {
		a := s.createArtist(fmt.Sprintf("Cap Head %d", i))
		s.addToLineup(fest.ID, a.ID, "headliner", i)
		headliners = append(headliners, a)
	}

	// 150 undercard artists → 153 total, 3 over the cap. Insert the lineup
	// rows in one batch to keep the test fast.
	undercards := make([]catalogm.Artist, 150)
	for i := range undercards {
		undercards[i] = catalogm.Artist{Name: fmt.Sprintf("Cap Under %03d", i)}
	}
	s.Require().NoError(s.db.CreateInBatches(&undercards, 100).Error)
	lineupRows := make([]catalogm.FestivalArtist, 0, len(undercards))
	for i := range undercards {
		lineupRows = append(lineupRows, catalogm.FestivalArtist{
			FestivalID:  fest.ID,
			ArtistID:    undercards[i].ID,
			BillingTier: catalogm.BillingTierUndercard,
		})
	}
	s.Require().NoError(s.db.CreateInBatches(&lineupRows, 100).Error)

	graph, err := s.svc.GetFestivalGraph(fest.ID, nil)
	s.Require().NoError(err)
	s.Equal(festivalGraphMaxNodes, graph.Festival.ArtistCount)
	s.Require().Len(graph.Nodes, festivalGraphMaxNodes)

	present := make(map[uint]bool, len(graph.Nodes))
	for _, n := range graph.Nodes {
		present[n.ID] = true
	}
	for _, h := range headliners {
		s.Truef(present[h.ID], "headliner %q must survive the node cap", h.Name)
	}
	// Undercards sort by name within the tier (position all 0); the cap
	// leaves room for 147 of 150, so the last three by name are dropped.
	for i := 0; i < 147; i++ {
		s.Truef(present[undercards[i].ID], "undercard %03d should be inside the cap", i)
	}
	for i := 147; i < 150; i++ {
		s.Falsef(present[undercards[i].ID], "undercard %03d should be dropped by the cap", i)
	}

	// Cluster sizes reflect the capped node set.
	s.Require().Len(graph.Clusters, 2)
	s.Equal("tier_headliner", graph.Clusters[0].ID)
	s.Equal(3, graph.Clusters[0].Size)
	s.Equal("tier_undercard", graph.Clusters[1].ID)
	s.Equal(147, graph.Clusters[1].Size)
}
