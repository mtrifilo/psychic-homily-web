package services

import (
	"encoding/json"

	apperrors "psychic-homily-backend/internal/errors"
	"psychic-homily-backend/internal/models"
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
func (suite *CollectionServiceIntegrationTestSuite) seedArtistRelationship(a, b *models.Artist, relType string, score float32) {
	src, tgt := a.ID, b.ID
	if src > tgt {
		src, tgt = tgt, src
	}
	rel := &models.ArtistRelationship{
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
	item := &models.CollectionItem{
		CollectionID:  collectionID,
		EntityType:    models.CollectionEntityArtist,
		EntityID:      artistID,
		AddedByUserID: addedByUserID,
	}
	suite.Require().NoError(suite.db.Create(item).Error)
}

// addNonArtistItemToCollection seeds a non-artist item (release/venue/etc.)
// to verify the graph filters correctly.
func (suite *CollectionServiceIntegrationTestSuite) addNonArtistItemToCollection(collectionID, entityID, addedByUserID uint, entityType string) {
	item := &models.CollectionItem{
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
	suite.seedArtistRelationship(a1, a2, models.RelationshipTypeSharedBills, 5.0)

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
	suite.seedArtistRelationship(a1, a2, models.RelationshipTypeSimilar, 3.0)

	graph, err := suite.collectionService.GetCollectionGraph(priv.Slug, creator.ID, nil)
	suite.Require().NoError(err)
	suite.Require().NotNil(graph)
	suite.Len(graph.Nodes, 2)
	suite.Len(graph.Links, 1)
	suite.Equal(models.RelationshipTypeSimilar, graph.Links[0].Type)
}

func (suite *CollectionServiceIntegrationTestSuite) TestGetCollectionGraph_NotFound() {
	graph, err := suite.collectionService.GetCollectionGraph("does-not-exist-slug-xyz", 0, nil)
	suite.Require().Error(err)
	suite.Nil(graph)
	var collErr *apperrors.CollectionError
	suite.Require().ErrorAs(err, &collErr)
	suite.Equal(apperrors.CodeCollectionNotFound, collErr.Code)
}

func (suite *CollectionServiceIntegrationTestSuite) TestGetCollectionGraph_MixedEntityTypesFiltersToArtistsOnly() {
	creator := suite.createTestUser("MixedCreator")
	priv := suite.createBasicCollection(creator, "Mixed Types")
	a1 := suite.createTestArtist("MixedArt1")
	a2 := suite.createTestArtist("MixedArt2")
	venue := suite.createTestVenueForCollection("Some Venue")

	suite.addArtistItemToCollection(priv.ID, a1.ID, creator.ID)
	suite.addArtistItemToCollection(priv.ID, a2.ID, creator.ID)
	// Add a venue item — should NOT appear in graph.
	suite.addNonArtistItemToCollection(priv.ID, venue.ID, creator.ID, models.CollectionEntityVenue)

	graph, err := suite.collectionService.GetCollectionGraph(priv.Slug, creator.ID, nil)
	suite.Require().NoError(err)
	suite.Len(graph.Nodes, 2, "only the two artist items should appear; the venue item is filtered out")
	suite.Equal(2, graph.Collection.ArtistCount)
}

func (suite *CollectionServiceIntegrationTestSuite) TestGetCollectionGraph_EmptyArtistSetReturnsEmptyGraph() {
	creator := suite.createTestUser("EmptyArtistCreator")
	priv := suite.createBasicCollection(creator, "No Artists")
	venue := suite.createTestVenueForCollection("Lonely Venue")
	// Only a venue item — no artist items at all.
	suite.addNonArtistItemToCollection(priv.ID, venue.ID, creator.ID, models.CollectionEntityVenue)

	graph, err := suite.collectionService.GetCollectionGraph(priv.Slug, creator.ID, nil)
	suite.Require().NoError(err)
	suite.NotNil(graph)
	suite.Empty(graph.Nodes)
	suite.Empty(graph.Links)
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
	suite.seedArtistRelationship(a1, a2, models.RelationshipTypeSharedBills, 4.0)
	suite.seedArtistRelationship(a1, a2, models.RelationshipTypeSharedLabel, 2.0)

	// Empty types → both edges
	graphAll, err := suite.collectionService.GetCollectionGraph(priv.Slug, creator.ID, nil)
	suite.Require().NoError(err)
	suite.Len(graphAll.Links, 2)

	// Filter to shared_bills → only one edge
	graphFiltered, err := suite.collectionService.GetCollectionGraph(priv.Slug, creator.ID, []string{models.RelationshipTypeSharedBills})
	suite.Require().NoError(err)
	suite.Len(graphFiltered.Links, 1)
	suite.Equal(models.RelationshipTypeSharedBills, graphFiltered.Links[0].Type)
}

func (suite *CollectionServiceIntegrationTestSuite) TestGetCollectionGraph_AllUnknownTypesReturnsZeroEdges() {
	creator := suite.createTestUser("UnknownTypesCreator")
	priv := suite.createBasicCollection(creator, "Unknown Types")
	a1 := suite.createTestArtist("UnknownA")
	a2 := suite.createTestArtist("UnknownB")
	suite.addArtistItemToCollection(priv.ID, a1.ID, creator.ID)
	suite.addArtistItemToCollection(priv.ID, a2.ID, creator.ID)
	suite.seedArtistRelationship(a1, a2, models.RelationshipTypeSharedBills, 4.0)

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
	rel := &models.ArtistRelationship{
		SourceArtistID:   src,
		TargetArtistID:   tgt,
		RelationshipType: models.RelationshipTypeSharedBills,
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

// Verify the contract type signature aligns with the interface — this is a
// compile-time check; if GetCollectionGraph were missing or the signature
// drifted, the package would not build.
var _ = func() bool {
	var _ contracts.CollectionServiceInterface = (*CollectionService)(nil)
	return true
}()
