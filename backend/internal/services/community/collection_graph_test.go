package community

import (
	"encoding/json"
	"fmt"
	"time"

	apperrors "psychic-homily-backend/internal/errors"
	catalogm "psychic-homily-backend/internal/models/catalog"
	communitym "psychic-homily-backend/internal/models/community"
	"psychic-homily-backend/internal/services/contracts"
)

// =============================================================================
// PSY-366: GetCollectionGraph
// =============================================================================
//
// Coverage matrix:
//   - public collection, anonymous viewer (viewerID=0)
//   - private collection, non-creator viewer  → ErrCollectionForbidden
//   - private collection, creator viewer      → graph returned
//   - missing slug                             → ErrCollectionNotFound
//   - mixed entity types in collection         → only artist items in graph
//   - empty artist set                         → 200 with empty nodes/links
//   - isolate flag set when artist has no in-set edges
//   - type filter: known types narrow the result
//   - type filter: all-unknown short-circuits to zero edges (no fallback)
//
// Helpers in collection_test.go (createTestUser, createTestArtist,
// createBasicCollection, createPublicCollection) are reused; tests below add
// graph-specific seeding (relationships + non-artist items).

// seedArtistRelationship creates a row in artist_relationships. The CHECK
// constraint enforces source_artist_id < target_artist_id; the helper
// canonicalizes the pair so callers don't have to.
func (suite *CollectionServiceIntegrationTestSuite) seedArtistRelationship(a, b *catalogm.Artist, relType string, score float32) {
	src, tgt := a.ID, b.ID
	if src > tgt {
		src, tgt = tgt, src
	}
	rel := &catalogm.ArtistRelationship{
		SourceArtistID:   src,
		TargetArtistID:   tgt,
		RelationshipType: relType,
		Score:            score,
		AutoDerived:      true,
	}
	suite.Require().NoError(suite.db.Create(rel).Error)
}

// addArtistItemToCollection bypasses CreateCollection's quality gates by
// inserting directly. The visibility gates only apply to UpdateCollection's
// false→true transition, so this is safe for graph tests that need a public
// collection with N artist items without satisfying the publish-gate.
func (suite *CollectionServiceIntegrationTestSuite) addArtistItemToCollection(collectionID, artistID, addedByUserID uint) {
	item := &communitym.CollectionItem{
		CollectionID:  collectionID,
		EntityType:    communitym.CollectionEntityArtist,
		EntityID:      artistID,
		AddedByUserID: addedByUserID,
	}
	suite.Require().NoError(suite.db.Create(item).Error)
}

// addNonArtistItemToCollection seeds a non-artist item (release/venue/etc.)
// to verify the graph filters correctly.
func (suite *CollectionServiceIntegrationTestSuite) addNonArtistItemToCollection(collectionID, entityID, addedByUserID uint, entityType string) {
	item := &communitym.CollectionItem{
		CollectionID:  collectionID,
		EntityType:    entityType,
		EntityID:      entityID,
		AddedByUserID: addedByUserID,
	}
	suite.Require().NoError(suite.db.Create(item).Error)
}

func (suite *CollectionServiceIntegrationTestSuite) TestGetCollectionGraph_PublicCollectionAnonymous() {
	user := suite.createTestUser("GraphCreator")
	coll := suite.createPublicCollection(user, "Graph Public") // already has 3 seed artists from gate
	a1 := suite.createTestArtist("Alpha")
	a2 := suite.createTestArtist("Bravo")
	a3 := suite.createTestArtist("Charlie")
	suite.addArtistItemToCollection(coll.ID, a1.ID, user.ID)
	suite.addArtistItemToCollection(coll.ID, a2.ID, user.ID)
	suite.addArtistItemToCollection(coll.ID, a3.ID, user.ID)
	suite.seedArtistRelationship(a1, a2, catalogm.RelationshipTypeSharedBills, 5.0)

	graph, err := suite.collectionService.GetCollectionGraph(coll.Slug, 0, nil)
	suite.Require().NoError(err)
	suite.Require().NotNil(graph)
	suite.Equal(coll.Slug, graph.Collection.Slug)
	suite.Equal("Graph Public", graph.Collection.Name)

	// 3 publish-gate seed artists + 3 named artists = 6 nodes total.
	suite.Equal(6, graph.Collection.ArtistCount)
	suite.Len(graph.Nodes, 6)
	suite.Equal(1, graph.Collection.EdgeCount)
	suite.Len(graph.Links, 1)

	// Connected artists should NOT be isolates; the rest should be.
	connected := 0
	isolated := 0
	for _, n := range graph.Nodes {
		if n.IsIsolate {
			isolated++
		} else {
			connected++
		}
	}
	suite.Equal(2, connected, "alpha + bravo are connected by shared_bills")
	suite.Equal(4, isolated, "charlie + 3 publish-gate artists have no edges")
}

func (suite *CollectionServiceIntegrationTestSuite) TestGetCollectionGraph_PrivateForbiddenForNonCreator() {
	creator := suite.createTestUser("PrivateGraphOwner")
	other := suite.createTestUser("OtherViewer")
	priv := suite.createBasicCollection(creator, "Private Graph") // private by default

	graph, err := suite.collectionService.GetCollectionGraph(priv.Slug, other.ID, nil)
	suite.Require().Error(err)
	suite.Nil(graph)
	var collErr *apperrors.CollectionError
	suite.Require().ErrorAs(err, &collErr)
	suite.Equal(apperrors.CodeCollectionForbidden, collErr.Code)
}

func (suite *CollectionServiceIntegrationTestSuite) TestGetCollectionGraph_PrivateAllowedForCreator() {
	creator := suite.createTestUser("PrivateGraphCreator")
	priv := suite.createBasicCollection(creator, "Private Graph Creator")
	a1 := suite.createTestArtist("PrivateA")
	a2 := suite.createTestArtist("PrivateB")
	suite.addArtistItemToCollection(priv.ID, a1.ID, creator.ID)
	suite.addArtistItemToCollection(priv.ID, a2.ID, creator.ID)
	suite.seedArtistRelationship(a1, a2, catalogm.RelationshipTypeSimilar, 3.0)

	graph, err := suite.collectionService.GetCollectionGraph(priv.Slug, creator.ID, nil)
	suite.Require().NoError(err)
	suite.Require().NotNil(graph)
	suite.Len(graph.Nodes, 2)
	suite.Len(graph.Links, 1)
	suite.Equal(catalogm.RelationshipTypeSimilar, graph.Links[0].Type)
}

func (suite *CollectionServiceIntegrationTestSuite) TestGetCollectionGraph_NotFound() {
	graph, err := suite.collectionService.GetCollectionGraph("does-not-exist-slug-xyz", 0, nil)
	suite.Require().Error(err)
	suite.Nil(graph)
	var collErr *apperrors.CollectionError
	suite.Require().ErrorAs(err, &collErr)
	suite.Equal(apperrors.CodeCollectionNotFound, collErr.Code)
}

func (suite *CollectionServiceIntegrationTestSuite) TestGetCollectionGraph_MixedEntityTypesEachBecomesNode() {
	// PSY-555 (Option B): every collection item becomes a node, regardless
	// of entity type. ArtistCount is preserved for backward compat (= count
	// of artist nodes specifically); total node count = items count.
	creator := suite.createTestUser("MixedCreator")
	priv := suite.createBasicCollection(creator, "Mixed Types")
	a1 := suite.createTestArtist("MixedArt1")
	a2 := suite.createTestArtist("MixedArt2")
	venue := suite.createTestVenueForCollection("Some Venue")

	suite.addArtistItemToCollection(priv.ID, a1.ID, creator.ID)
	suite.addArtistItemToCollection(priv.ID, a2.ID, creator.ID)
	suite.addNonArtistItemToCollection(priv.ID, venue.ID, creator.ID, communitym.CollectionEntityVenue)

	graph, err := suite.collectionService.GetCollectionGraph(priv.Slug, creator.ID, nil)
	suite.Require().NoError(err)
	suite.Len(graph.Nodes, 3, "every item — including the venue — should appear as a node")
	suite.Equal(2, graph.Collection.ArtistCount, "ArtistCount counts artist nodes only")
	suite.Equal(2, graph.Collection.EntityCounts[communitym.CollectionEntityArtist])
	suite.Equal(1, graph.Collection.EntityCounts[communitym.CollectionEntityVenue])
}

func (suite *CollectionServiceIntegrationTestSuite) TestGetCollectionGraph_VenueOnlyCollectionStillRendersNode() {
	// PSY-555: a single-venue collection used to return zero nodes (the
	// artist-only filter dropped everything). Now the venue is a node
	// itself; with no artists in the set, there are no edges.
	creator := suite.createTestUser("EmptyArtistCreator")
	priv := suite.createBasicCollection(creator, "No Artists")
	venue := suite.createTestVenueForCollection("Lonely Venue")
	suite.addNonArtistItemToCollection(priv.ID, venue.ID, creator.ID, communitym.CollectionEntityVenue)

	graph, err := suite.collectionService.GetCollectionGraph(priv.Slug, creator.ID, nil)
	suite.Require().NoError(err)
	suite.NotNil(graph)
	suite.Len(graph.Nodes, 1, "the venue is its own node")
	suite.Equal(communitym.CollectionEntityVenue, graph.Nodes[0].EntityType)
	suite.Empty(graph.Links, "no artists in set → no edges")
	suite.Equal(0, graph.Collection.ArtistCount)
	suite.Equal(0, graph.Collection.EdgeCount)
}

func (suite *CollectionServiceIntegrationTestSuite) TestGetCollectionGraph_TypeFilterNarrowsEdges() {
	creator := suite.createTestUser("FilterCreator")
	priv := suite.createBasicCollection(creator, "Filter Test")
	a1 := suite.createTestArtist("FilterA")
	a2 := suite.createTestArtist("FilterB")
	suite.addArtistItemToCollection(priv.ID, a1.ID, creator.ID)
	suite.addArtistItemToCollection(priv.ID, a2.ID, creator.ID)
	// Two edges between the same pair, different types.
	suite.seedArtistRelationship(a1, a2, catalogm.RelationshipTypeSharedBills, 4.0)
	suite.seedArtistRelationship(a1, a2, catalogm.RelationshipTypeSharedLabel, 2.0)

	// Empty types → both edges
	graphAll, err := suite.collectionService.GetCollectionGraph(priv.Slug, creator.ID, nil)
	suite.Require().NoError(err)
	suite.Len(graphAll.Links, 2)

	// Filter to shared_bills → only one edge
	graphFiltered, err := suite.collectionService.GetCollectionGraph(priv.Slug, creator.ID, []string{catalogm.RelationshipTypeSharedBills})
	suite.Require().NoError(err)
	suite.Len(graphFiltered.Links, 1)
	suite.Equal(catalogm.RelationshipTypeSharedBills, graphFiltered.Links[0].Type)
}

func (suite *CollectionServiceIntegrationTestSuite) TestGetCollectionGraph_AllUnknownTypesReturnsZeroEdges() {
	creator := suite.createTestUser("UnknownTypesCreator")
	priv := suite.createBasicCollection(creator, "Unknown Types")
	a1 := suite.createTestArtist("UnknownA")
	a2 := suite.createTestArtist("UnknownB")
	suite.addArtistItemToCollection(priv.ID, a1.ID, creator.ID)
	suite.addArtistItemToCollection(priv.ID, a2.ID, creator.ID)
	suite.seedArtistRelationship(a1, a2, catalogm.RelationshipTypeSharedBills, 4.0)

	// Caller asked for a type the allowlist rejects → must return zero edges
	// (must NOT silently fall back to "all allowed types").
	graph, err := suite.collectionService.GetCollectionGraph(priv.Slug, creator.ID, []string{"festival_cobill", "made_up_type"})
	suite.Require().NoError(err)
	suite.Len(graph.Nodes, 2, "nodes are still returned (collection has artists)")
	suite.Empty(graph.Links, "no edges because every requested type was rejected by the allowlist")
}

func (suite *CollectionServiceIntegrationTestSuite) TestGetCollectionGraph_RelationshipDetailRoundtrips() {
	creator := suite.createTestUser("DetailCreator")
	priv := suite.createBasicCollection(creator, "Detail RT")
	a1 := suite.createTestArtist("DetailA")
	a2 := suite.createTestArtist("DetailB")
	suite.addArtistItemToCollection(priv.ID, a1.ID, creator.ID)
	suite.addArtistItemToCollection(priv.ID, a2.ID, creator.ID)

	// Seed with a JSONB detail payload. Must round-trip through the response.
	src, tgt := a1.ID, a2.ID
	if src > tgt {
		src, tgt = tgt, src
	}
	detail := json.RawMessage(`{"shared_show_count": 7, "venue": "Trunk Space"}`)
	rel := &catalogm.ArtistRelationship{
		SourceArtistID:   src,
		TargetArtistID:   tgt,
		RelationshipType: catalogm.RelationshipTypeSharedBills,
		Score:            7.0,
		AutoDerived:      true,
		Detail:           &detail,
	}
	suite.Require().NoError(suite.db.Create(rel).Error)

	graph, err := suite.collectionService.GetCollectionGraph(priv.Slug, creator.ID, nil)
	suite.Require().NoError(err)
	suite.Require().Len(graph.Links, 1)

	// Detail is unmarshaled into `any`, which lands as map[string]any.
	detailMap, ok := graph.Links[0].Detail.(map[string]any)
	suite.Require().True(ok, "detail should round-trip as map[string]any")
	suite.Equal("Trunk Space", detailMap["venue"])
}

// =============================================================================
// PSY-555: multi-type collection graph (Option B)
// =============================================================================

// TestGetCollectionGraph_MultiType_VenueReleaseArtist exercises the ticket's
// canonical case: a collection with venue + release + 1 artist who played
// the venue and made the release. Expected: 3 nodes, 2 edges
// (artist↔venue via played_at, artist↔release via discography).
func (suite *CollectionServiceIntegrationTestSuite) TestGetCollectionGraph_MultiType_VenueReleaseArtist() {
	creator := suite.createTestUser("MultiTypeCreator")
	priv := suite.createBasicCollection(creator, "Multi Type Triangle")

	artist := suite.createTestArtist("MultiArtist")
	venue := suite.createTestVenueForCollection("MultiVenue")

	// Release made by the artist.
	releaseSlug := fmt.Sprintf("multi-release-%d", time.Now().UnixNano())
	release := &catalogm.Release{
		Title:       "Multi Release",
		Slug:        &releaseSlug,
		ReleaseType: catalogm.ReleaseTypeLP,
	}
	suite.Require().NoError(suite.db.Create(release).Error)
	ar := &catalogm.ArtistRelease{
		ArtistID:  artist.ID,
		ReleaseID: release.ID,
		Role:      catalogm.ArtistReleaseRoleMain,
	}
	suite.Require().NoError(suite.db.Create(ar).Error)

	// Show staged at the venue with the artist on the bill — this is what
	// makes the artist↔venue "played_at" edge resolvable.
	show := &catalogm.Show{
		Title:     "Multi Show",
		EventDate: time.Now().Add(-24 * time.Hour),
		Status:    catalogm.ShowStatusApproved,
	}
	suite.Require().NoError(suite.db.Create(show).Error)
	suite.Require().NoError(suite.db.Create(&catalogm.ShowArtist{
		ShowID: show.ID, ArtistID: artist.ID,
	}).Error)
	suite.Require().NoError(suite.db.Create(&catalogm.ShowVenue{
		ShowID: show.ID, VenueID: venue.ID,
	}).Error)

	// Add the three entities to the collection. The show is intentionally
	// NOT in the collection so its node doesn't appear (and so the
	// show_lineup/show_venue derived edges don't fire). The "played_at"
	// edge is artist↔venue regardless of which show source-of-truth'd it.
	suite.addArtistItemToCollection(priv.ID, artist.ID, creator.ID)
	suite.addNonArtistItemToCollection(priv.ID, venue.ID, creator.ID, communitym.CollectionEntityVenue)
	suite.addNonArtistItemToCollection(priv.ID, release.ID, creator.ID, communitym.CollectionEntityRelease)

	graph, err := suite.collectionService.GetCollectionGraph(priv.Slug, creator.ID, nil)
	suite.Require().NoError(err)
	suite.Require().Len(graph.Nodes, 3, "artist + venue + release each become a node")
	suite.Require().Len(graph.Links, 2, "exactly two edges: artist↔venue (played_at) and artist↔release (discography)")

	// Assert each node has the correct entity_type.
	gotTypes := make(map[string]int, 3)
	for _, n := range graph.Nodes {
		gotTypes[n.EntityType]++
	}
	suite.Equal(1, gotTypes[communitym.CollectionEntityArtist])
	suite.Equal(1, gotTypes[communitym.CollectionEntityVenue])
	suite.Equal(1, gotTypes[communitym.CollectionEntityRelease])

	// Assert the link types are exactly the two derived ones.
	gotLinkTypes := make(map[string]int, 2)
	for _, l := range graph.Links {
		gotLinkTypes[l.Type]++
	}
	suite.Equal(1, gotLinkTypes[CollectionEdgePlayedAt], "expected one played_at edge")
	suite.Equal(1, gotLinkTypes[CollectionEdgeDiscography], "expected one discography edge")

	// EntityCounts breakdown should match.
	suite.Equal(1, graph.Collection.EntityCounts[communitym.CollectionEntityArtist])
	suite.Equal(1, graph.Collection.EntityCounts[communitym.CollectionEntityVenue])
	suite.Equal(1, graph.Collection.EntityCounts[communitym.CollectionEntityRelease])
	suite.Equal(2, graph.Collection.EdgeCount)

	// No isolates — every node has at least one edge in this set.
	for _, n := range graph.Nodes {
		suite.False(n.IsIsolate, "node %s (%s) should not be isolated", n.Name, n.EntityType)
	}
}

// TestGetCollectionGraph_MultiType_PhantomEdgeNotEmitted documents the
// "edge to a node only if BOTH endpoints are in the collection" rule:
// putting a release alone (no artist) in a collection must NOT emit
// discography edges into a phantom artist node.
func (suite *CollectionServiceIntegrationTestSuite) TestGetCollectionGraph_MultiType_PhantomEdgeNotEmitted() {
	creator := suite.createTestUser("PhantomCreator")
	priv := suite.createBasicCollection(creator, "Phantom Edge Test")

	artist := suite.createTestArtist("PhantomArtist") // NOT in the collection
	releaseSlug := fmt.Sprintf("phantom-release-%d", time.Now().UnixNano())
	release := &catalogm.Release{
		Title:       "Phantom Release",
		Slug:        &releaseSlug,
		ReleaseType: catalogm.ReleaseTypeLP,
	}
	suite.Require().NoError(suite.db.Create(release).Error)
	suite.Require().NoError(suite.db.Create(&catalogm.ArtistRelease{
		ArtistID: artist.ID, ReleaseID: release.ID, Role: catalogm.ArtistReleaseRoleMain,
	}).Error)

	suite.addNonArtistItemToCollection(priv.ID, release.ID, creator.ID, communitym.CollectionEntityRelease)

	graph, err := suite.collectionService.GetCollectionGraph(priv.Slug, creator.ID, nil)
	suite.Require().NoError(err)
	suite.Len(graph.Nodes, 1, "only the release is in the collection")
	suite.Empty(graph.Links, "no edges — the artist endpoint isn't in the collection")
}

// TestGetCollectionGraph_MultiType_ShowEdges documents the show-as-node
// behaviour: a show in the collection edges to in-collection artists (its
// lineup) and venues (its location). PSY-555.
func (suite *CollectionServiceIntegrationTestSuite) TestGetCollectionGraph_MultiType_ShowEdges() {
	creator := suite.createTestUser("ShowEdgeCreator")
	priv := suite.createBasicCollection(creator, "Show Edge Test")

	artist := suite.createTestArtist("ShowEdgeArtist")
	venue := suite.createTestVenueForCollection("ShowEdgeVenue")
	show := &catalogm.Show{
		Title:     "ShowEdgeShow",
		EventDate: time.Now().Add(-24 * time.Hour),
		Status:    catalogm.ShowStatusApproved,
	}
	suite.Require().NoError(suite.db.Create(show).Error)
	suite.Require().NoError(suite.db.Create(&catalogm.ShowArtist{
		ShowID: show.ID, ArtistID: artist.ID,
	}).Error)
	suite.Require().NoError(suite.db.Create(&catalogm.ShowVenue{
		ShowID: show.ID, VenueID: venue.ID,
	}).Error)

	suite.addArtistItemToCollection(priv.ID, artist.ID, creator.ID)
	suite.addNonArtistItemToCollection(priv.ID, venue.ID, creator.ID, communitym.CollectionEntityVenue)
	suite.addNonArtistItemToCollection(priv.ID, show.ID, creator.ID, communitym.CollectionEntityShow)

	graph, err := suite.collectionService.GetCollectionGraph(priv.Slug, creator.ID, nil)
	suite.Require().NoError(err)
	suite.Len(graph.Nodes, 3)

	// Expected edges: show↔artist (show_lineup), show↔venue (show_venue),
	// artist↔venue (played_at — artist played the venue via this same show).
	gotLinkTypes := make(map[string]int)
	for _, l := range graph.Links {
		gotLinkTypes[l.Type]++
	}
	suite.Equal(1, gotLinkTypes[CollectionEdgeShowLineup])
	suite.Equal(1, gotLinkTypes[CollectionEdgeShowVenue])
	suite.Equal(1, gotLinkTypes[CollectionEdgePlayedAt])
	suite.Equal(3, graph.Collection.EdgeCount)
}

// TestGetCollectionGraph_MultiType_NodeIDsAreItemIDs verifies the node-ID
// invariant the frontend depends on: source_id/target_id reference node
// IDs (collection_item.id), not raw entity DB IDs. This matters because
// e.g. artist 5 and venue 5 collide on entity ID alone.
func (suite *CollectionServiceIntegrationTestSuite) TestGetCollectionGraph_MultiType_NodeIDsAreItemIDs() {
	creator := suite.createTestUser("NodeIDCreator")
	priv := suite.createBasicCollection(creator, "Node ID Test")
	artist := suite.createTestArtist("NodeIDArtist")
	venue := suite.createTestVenueForCollection("NodeIDVenue")
	show := &catalogm.Show{
		Title:     "NodeIDShow",
		EventDate: time.Now().Add(-24 * time.Hour),
		Status:    catalogm.ShowStatusApproved,
	}
	suite.Require().NoError(suite.db.Create(show).Error)
	suite.Require().NoError(suite.db.Create(&catalogm.ShowArtist{
		ShowID: show.ID, ArtistID: artist.ID,
	}).Error)
	suite.Require().NoError(suite.db.Create(&catalogm.ShowVenue{
		ShowID: show.ID, VenueID: venue.ID,
	}).Error)

	suite.addArtistItemToCollection(priv.ID, artist.ID, creator.ID)
	suite.addNonArtistItemToCollection(priv.ID, venue.ID, creator.ID, communitym.CollectionEntityVenue)

	graph, err := suite.collectionService.GetCollectionGraph(priv.Slug, creator.ID, nil)
	suite.Require().NoError(err)
	suite.Require().Len(graph.Nodes, 2)
	suite.Require().Len(graph.Links, 1)

	// Every link's source and target must resolve to a node in the response.
	nodeIDs := make(map[uint]string, len(graph.Nodes))
	for _, n := range graph.Nodes {
		nodeIDs[n.ID] = n.EntityType
	}
	for _, l := range graph.Links {
		_, srcOK := nodeIDs[l.SourceID]
		_, tgtOK := nodeIDs[l.TargetID]
		suite.True(srcOK, "link source_id %d must reference a node in the response", l.SourceID)
		suite.True(tgtOK, "link target_id %d must reference a node in the response", l.TargetID)
	}
}

// Verify the contract type signature aligns with the interface — this is a
// compile-time check; if GetCollectionGraph were missing or the signature
// drifted, the package would not build.
var _ = func() bool {
	var _ contracts.CollectionServiceInterface = (*CollectionService)(nil)
	return true
}()
