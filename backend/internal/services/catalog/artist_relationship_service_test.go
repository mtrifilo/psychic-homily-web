package catalog

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"

	apperrors "psychic-homily-backend/internal/errors"
	authm "psychic-homily-backend/internal/models/auth"
	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/testutil"
)

// =============================================================================
// INTEGRATION TESTS
// =============================================================================

type ArtistRelationshipServiceIntegrationTestSuite struct {
	suite.Suite
	testDB *testutil.TestDatabase
	db     *gorm.DB
	svc    *ArtistRelationshipService
}

func (suite *ArtistRelationshipServiceIntegrationTestSuite) SetupSuite() {
	suite.testDB = testutil.SetupTestPostgres(suite.T())
	suite.db = suite.testDB.DB

	suite.db = suite.testDB.DB
	suite.svc = NewArtistRelationshipService(suite.testDB.DB)
}

func (suite *ArtistRelationshipServiceIntegrationTestSuite) TearDownSuite() {
	suite.testDB.Cleanup()
}

func (suite *ArtistRelationshipServiceIntegrationTestSuite) SetupTest() {
	sqlDB, _ := suite.db.DB()
	_, _ = sqlDB.Exec("DELETE FROM artist_relationship_votes")
	_, _ = sqlDB.Exec("DELETE FROM artist_relationships")
	_, _ = sqlDB.Exec("DELETE FROM festival_artists")
	_, _ = sqlDB.Exec("DELETE FROM festival_venues")
	_, _ = sqlDB.Exec("DELETE FROM festivals")
	_, _ = sqlDB.Exec("DELETE FROM show_artists")
	_, _ = sqlDB.Exec("DELETE FROM shows")
	_, _ = sqlDB.Exec("DELETE FROM artist_labels")
	_, _ = sqlDB.Exec("DELETE FROM labels")
	_, _ = sqlDB.Exec("DELETE FROM artists")
	_, _ = sqlDB.Exec("DELETE FROM users")
}

func (suite *ArtistRelationshipServiceIntegrationTestSuite) createArtist(name string) uint {
	slug := fmt.Sprintf("%s-%d", name, time.Now().UnixNano())
	artist := &catalogm.Artist{Name: name, Slug: &slug}
	err := suite.db.Create(artist).Error
	suite.Require().NoError(err)
	return artist.ID
}

func (suite *ArtistRelationshipServiceIntegrationTestSuite) createUser(name string) *authm.User {
	email := fmt.Sprintf("%s-%d@test.com", name, time.Now().UnixNano())
	user := &authm.User{Email: &email, FirstName: &name, IsActive: true, EmailVerified: true}
	err := suite.db.Create(user).Error
	suite.Require().NoError(err)
	return user
}

func (suite *ArtistRelationshipServiceIntegrationTestSuite) createShow(title string) uint {
	slug := fmt.Sprintf("show-%d", time.Now().UnixNano())
	show := &catalogm.Show{
		Title:     title,
		Slug:      &slug,
		EventDate: time.Now().Add(24 * time.Hour),
		Status:    catalogm.ShowStatusApproved,
	}
	err := suite.db.Create(show).Error
	suite.Require().NoError(err)
	return show.ID
}

func (suite *ArtistRelationshipServiceIntegrationTestSuite) addArtistToShow(showID, artistID uint) {
	suite.db.Exec("INSERT INTO show_artists (show_id, artist_id) VALUES (?, ?)", showID, artistID)
}

func (suite *ArtistRelationshipServiceIntegrationTestSuite) createLabel(name string) uint {
	slug := fmt.Sprintf("%s-%d", name, time.Now().UnixNano())
	label := &catalogm.Label{Name: name, Slug: &slug}
	err := suite.db.Create(label).Error
	suite.Require().NoError(err)
	return label.ID
}

func (suite *ArtistRelationshipServiceIntegrationTestSuite) addArtistToLabel(labelID, artistID uint) {
	err := suite.db.Exec("INSERT INTO artist_labels (artist_id, label_id) VALUES (?, ?)", artistID, labelID).Error
	suite.Require().NoError(err)
}

// ──────────────────────────────────────────────
// CRUD Tests
// ──────────────────────────────────────────────

func (suite *ArtistRelationshipServiceIntegrationTestSuite) TestCreateRelationship_Success() {
	a1 := suite.createArtist("Band A")
	a2 := suite.createArtist("Band B")

	rel, err := suite.svc.CreateRelationship(a1, a2, "similar", false)
	suite.Require().NoError(err)
	suite.Require().NotNil(rel)
	suite.Assert().Equal("similar", rel.RelationshipType)
	suite.Assert().False(rel.AutoDerived)
}

func (suite *ArtistRelationshipServiceIntegrationTestSuite) TestCreateRelationship_CanonicalOrdering() {
	a1 := suite.createArtist("Band A")
	a2 := suite.createArtist("Band B")

	// Create with reversed order — should still use canonical
	rel, err := suite.svc.CreateRelationship(a2, a1, "similar", false)
	suite.Require().NoError(err)

	src, tgt := catalogm.CanonicalOrder(a1, a2)
	suite.Assert().Equal(src, rel.SourceArtistID)
	suite.Assert().Equal(tgt, rel.TargetArtistID)
}

func (suite *ArtistRelationshipServiceIntegrationTestSuite) TestCreateRelationship_SelfRelationship() {
	a1 := suite.createArtist("Band A")

	_, err := suite.svc.CreateRelationship(a1, a1, "similar", false)
	suite.Assert().Error(err)
	suite.Assert().Contains(err.Error(), "cannot create relationship between an artist and itself")
}

func (suite *ArtistRelationshipServiceIntegrationTestSuite) TestCreateRelationship_Duplicate() {
	a1 := suite.createArtist("Band A")
	a2 := suite.createArtist("Band B")

	_, err := suite.svc.CreateRelationship(a1, a2, "similar", false)
	suite.Require().NoError(err)

	_, err = suite.svc.CreateRelationship(a1, a2, "similar", false)
	suite.Assert().Error(err)
	suite.Assert().Contains(err.Error(), "relationship already exists")
}

func (suite *ArtistRelationshipServiceIntegrationTestSuite) TestCreateRelationship_DifferentTypes() {
	a1 := suite.createArtist("Band A")
	a2 := suite.createArtist("Band B")

	_, err := suite.svc.CreateRelationship(a1, a2, "similar", false)
	suite.Require().NoError(err)

	_, err = suite.svc.CreateRelationship(a1, a2, "side_project", false)
	suite.Require().NoError(err) // Different type = OK
}

func (suite *ArtistRelationshipServiceIntegrationTestSuite) TestGetRelationship_Found() {
	a1 := suite.createArtist("Band A")
	a2 := suite.createArtist("Band B")
	_, err := suite.svc.CreateRelationship(a1, a2, "similar", false)
	suite.Require().NoError(err)

	rel, err := suite.svc.GetRelationship(a1, a2, "similar")
	suite.Require().NoError(err)
	suite.Require().NotNil(rel)
}

func (suite *ArtistRelationshipServiceIntegrationTestSuite) TestGetRelationship_ReversedOrder() {
	a1 := suite.createArtist("Band A")
	a2 := suite.createArtist("Band B")
	_, err := suite.svc.CreateRelationship(a1, a2, "similar", false)
	suite.Require().NoError(err)

	// Query with reversed order — should still find it
	rel, err := suite.svc.GetRelationship(a2, a1, "similar")
	suite.Require().NoError(err)
	suite.Require().NotNil(rel)
}

func (suite *ArtistRelationshipServiceIntegrationTestSuite) TestGetRelationship_NotFound() {
	rel, err := suite.svc.GetRelationship(99999, 99998, "similar")
	suite.Assert().NoError(err)
	suite.Assert().Nil(rel)
}

func (suite *ArtistRelationshipServiceIntegrationTestSuite) TestGetRelatedArtists() {
	a1 := suite.createArtist("Center Band")
	a2 := suite.createArtist("Related Band 1")
	a3 := suite.createArtist("Related Band 2")

	_, err := suite.svc.CreateRelationship(a1, a2, "similar", false)
	suite.Require().NoError(err)
	_, err = suite.svc.CreateRelationship(a1, a3, "shared_bills", true)
	suite.Require().NoError(err)

	related, err := suite.svc.GetRelatedArtists(a1, "", 30)
	suite.Require().NoError(err)
	suite.Assert().Len(related, 2)
}

func (suite *ArtistRelationshipServiceIntegrationTestSuite) TestGetRelatedArtists_FilterByType() {
	a1 := suite.createArtist("Center")
	a2 := suite.createArtist("Similar")
	a3 := suite.createArtist("Shared")

	_, err := suite.svc.CreateRelationship(a1, a2, "similar", false)
	suite.Require().NoError(err)
	_, err = suite.svc.CreateRelationship(a1, a3, "shared_bills", true)
	suite.Require().NoError(err)

	related, err := suite.svc.GetRelatedArtists(a1, "similar", 30)
	suite.Require().NoError(err)
	suite.Assert().Len(related, 1)
	suite.Assert().Equal("Similar", related[0].Name)
}

func (suite *ArtistRelationshipServiceIntegrationTestSuite) TestDeleteRelationship() {
	a1 := suite.createArtist("Band A")
	a2 := suite.createArtist("Band B")
	_, err := suite.svc.CreateRelationship(a1, a2, "similar", false)
	suite.Require().NoError(err)

	err = suite.svc.DeleteRelationship(a1, a2, "similar")
	suite.Assert().NoError(err)

	rel, _ := suite.svc.GetRelationship(a1, a2, "similar")
	suite.Assert().Nil(rel)
}

func (suite *ArtistRelationshipServiceIntegrationTestSuite) TestDeleteRelationship_NotFound() {
	err := suite.svc.DeleteRelationship(99999, 99998, "similar")
	suite.Assert().Error(err)
}

// ──────────────────────────────────────────────
// Voting Tests
// ──────────────────────────────────────────────

func (suite *ArtistRelationshipServiceIntegrationTestSuite) TestVote_Upvote() {
	a1 := suite.createArtist("Band A")
	a2 := suite.createArtist("Band B")
	user := suite.createUser("voter")
	_, err := suite.svc.CreateRelationship(a1, a2, "similar", false)
	suite.Require().NoError(err)

	err = suite.svc.Vote(a1, a2, "similar", user.ID, true)
	suite.Assert().NoError(err)

	vote, err := suite.svc.GetUserVote(a1, a2, "similar", user.ID)
	suite.Require().NoError(err)
	suite.Require().NotNil(vote)
	suite.Assert().Equal(int16(1), vote.Direction)
}

func (suite *ArtistRelationshipServiceIntegrationTestSuite) TestVote_Downvote() {
	a1 := suite.createArtist("Band A")
	a2 := suite.createArtist("Band B")
	user := suite.createUser("voter")
	_, err := suite.svc.CreateRelationship(a1, a2, "similar", false)
	suite.Require().NoError(err)

	err = suite.svc.Vote(a1, a2, "similar", user.ID, false)
	suite.Assert().NoError(err)

	vote, _ := suite.svc.GetUserVote(a1, a2, "similar", user.ID)
	suite.Assert().Equal(int16(-1), vote.Direction)
}

func (suite *ArtistRelationshipServiceIntegrationTestSuite) TestVote_ChangeDirection() {
	a1 := suite.createArtist("Band A")
	a2 := suite.createArtist("Band B")
	user := suite.createUser("voter")
	_, err := suite.svc.CreateRelationship(a1, a2, "similar", false)
	suite.Require().NoError(err)

	suite.Require().NoError(suite.svc.Vote(a1, a2, "similar", user.ID, true))
	suite.Require().NoError(suite.svc.Vote(a1, a2, "similar", user.ID, false)) // Change to downvote

	vote, _ := suite.svc.GetUserVote(a1, a2, "similar", user.ID)
	suite.Assert().Equal(int16(-1), vote.Direction)
}

func (suite *ArtistRelationshipServiceIntegrationTestSuite) TestVote_RelationshipNotFound() {
	user := suite.createUser("voter")
	err := suite.svc.Vote(99999, 99998, "similar", user.ID, true)
	suite.Assert().Error(err)
	suite.Assert().Contains(err.Error(), "relationship not found")
}

func (suite *ArtistRelationshipServiceIntegrationTestSuite) TestVote_ScoreUpdates() {
	a1 := suite.createArtist("Band A")
	a2 := suite.createArtist("Band B")
	user1 := suite.createUser("voter1")
	user2 := suite.createUser("voter2")
	_, err := suite.svc.CreateRelationship(a1, a2, "similar", false)
	suite.Require().NoError(err)

	// Two upvotes
	suite.Require().NoError(suite.svc.Vote(a1, a2, "similar", user1.ID, true))
	suite.Require().NoError(suite.svc.Vote(a1, a2, "similar", user2.ID, true))

	rel, _ := suite.svc.GetRelationship(a1, a2, "similar")
	suite.Assert().Greater(rel.Score, float32(0))
}

func (suite *ArtistRelationshipServiceIntegrationTestSuite) TestRemoveVote() {
	a1 := suite.createArtist("Band A")
	a2 := suite.createArtist("Band B")
	user := suite.createUser("voter")
	_, err := suite.svc.CreateRelationship(a1, a2, "similar", false)
	suite.Require().NoError(err)

	suite.Require().NoError(suite.svc.Vote(a1, a2, "similar", user.ID, true))
	err = suite.svc.RemoveVote(a1, a2, "similar", user.ID)
	suite.Assert().NoError(err)

	vote, _ := suite.svc.GetUserVote(a1, a2, "similar", user.ID)
	suite.Assert().Nil(vote)
}

func (suite *ArtistRelationshipServiceIntegrationTestSuite) TestGetUserVote_NotVoted() {
	a1 := suite.createArtist("Band A")
	a2 := suite.createArtist("Band B")
	user := suite.createUser("voter")
	_, err := suite.svc.CreateRelationship(a1, a2, "similar", false)
	suite.Require().NoError(err)

	vote, err := suite.svc.GetUserVote(a1, a2, "similar", user.ID)
	suite.Assert().NoError(err)
	suite.Assert().Nil(vote)
}

// ──────────────────────────────────────────────
// Auto-derivation Tests
// ──────────────────────────────────────────────

func (suite *ArtistRelationshipServiceIntegrationTestSuite) TestDeriveSharedBills_TwoShows() {
	a1 := suite.createArtist("Band A")
	a2 := suite.createArtist("Band B")
	show1 := suite.createShow("Show 1")
	show2 := suite.createShow("Show 2")

	suite.addArtistToShow(show1, a1)
	suite.addArtistToShow(show1, a2)
	suite.addArtistToShow(show2, a1)
	suite.addArtistToShow(show2, a2)

	count, err := suite.svc.DeriveSharedBills(2)
	suite.Require().NoError(err)
	suite.Assert().Equal(int64(1), count)

	// Verify relationship created
	rel, err := suite.svc.GetRelationship(a1, a2, "shared_bills")
	suite.Require().NoError(err)
	suite.Require().NotNil(rel)
	suite.Assert().True(rel.AutoDerived)
	suite.Assert().Greater(rel.Score, float32(0))
}

func (suite *ArtistRelationshipServiceIntegrationTestSuite) TestDeriveSharedBills_BelowThreshold() {
	a1 := suite.createArtist("Band A")
	a2 := suite.createArtist("Band B")
	show1 := suite.createShow("Show 1")

	suite.addArtistToShow(show1, a1)
	suite.addArtistToShow(show1, a2)

	// Only 1 shared show, threshold is 2
	count, err := suite.svc.DeriveSharedBills(2)
	suite.Require().NoError(err)
	suite.Assert().Equal(int64(0), count)
}

func (suite *ArtistRelationshipServiceIntegrationTestSuite) TestDeriveSharedBills_UpdatesExisting() {
	a1 := suite.createArtist("Band A")
	a2 := suite.createArtist("Band B")
	show1 := suite.createShow("Show 1")
	show2 := suite.createShow("Show 2")

	suite.addArtistToShow(show1, a1)
	suite.addArtistToShow(show1, a2)
	suite.addArtistToShow(show2, a1)
	suite.addArtistToShow(show2, a2)

	_, err := suite.svc.DeriveSharedBills(2)
	suite.Require().NoError(err)

	// Add another show and re-derive
	show3 := suite.createShow("Show 3")
	suite.addArtistToShow(show3, a1)
	suite.addArtistToShow(show3, a2)

	count, err := suite.svc.DeriveSharedBills(2)
	suite.Require().NoError(err)
	suite.Assert().Equal(int64(1), count) // Updated existing

	rel, _ := suite.svc.GetRelationship(a1, a2, "shared_bills")
	suite.Assert().NotNil(rel.Detail) // Has detail JSON
}

// PSY-1323: minShows=1 keeps one-off co-bills with a low score instead of
// dropping them — the count/10 formula bounds the noise by weight.
func (suite *ArtistRelationshipServiceIntegrationTestSuite) TestDeriveSharedBills_OneOffMinShowsOne() {
	a1 := suite.createArtist("Band A")
	a2 := suite.createArtist("Band B")
	show1 := suite.createShow("Show 1")

	suite.addArtistToShow(show1, a1)
	suite.addArtistToShow(show1, a2)

	count, err := suite.svc.DeriveSharedBills(1)
	suite.Require().NoError(err)
	suite.Assert().Equal(int64(1), count)

	rel, err := suite.svc.GetRelationship(a1, a2, "shared_bills")
	suite.Require().NoError(err)
	suite.Require().NotNil(rel)
	// One shared show = 0.1, ×1.2 recency boost (show is within 3 months).
	suite.Assert().InDelta(0.12, float64(rel.Score), 0.001)
}

// ──────────────────────────────────────────────
// Shared-label derivation (PSY-1323 roster normalization)
// ──────────────────────────────────────────────

func (suite *ArtistRelationshipServiceIntegrationTestSuite) TestDeriveSharedLabels_TwoArtistLabel() {
	a1 := suite.createArtist("Band A")
	a2 := suite.createArtist("Band B")
	label := suite.createLabel("Tiny Label")
	suite.addArtistToLabel(label, a1)
	suite.addArtistToLabel(label, a2)

	count, err := suite.svc.DeriveSharedLabels(1)
	suite.Require().NoError(err)
	suite.Assert().Equal(int64(1), count)

	rel, err := suite.svc.GetRelationship(a1, a2, "shared_label")
	suite.Require().NoError(err)
	suite.Require().NotNil(rel)
	suite.Assert().True(rel.AutoDerived)
	// Roster of 2: the pair carries the label's full weight, 1/(2-1) = 1.0.
	suite.Assert().InDelta(1.0, float64(rel.Score), 0.001)
}

func (suite *ArtistRelationshipServiceIntegrationTestSuite) TestDeriveSharedLabels_LargeRosterNormalized() {
	label := suite.createLabel("Big Label")
	ids := make([]uint, 5)
	for i := range ids {
		ids[i] = suite.createArtist(fmt.Sprintf("Roster Band %d", i))
		suite.addArtistToLabel(label, ids[i])
	}

	count, err := suite.svc.DeriveSharedLabels(1)
	suite.Require().NoError(err)
	suite.Assert().Equal(int64(10), count) // C(5,2) pairs

	rel, err := suite.svc.GetRelationship(ids[0], ids[1], "shared_label")
	suite.Require().NoError(err)
	suite.Require().NotNil(rel)
	// Roster of 5: each pair gets 1/(5-1) = 0.25, not the flat 0.2 of the
	// old shared_count/5 formula — bigger rosters dilute per-pair weight.
	suite.Assert().InDelta(0.25, float64(rel.Score), 0.001)
}

func (suite *ArtistRelationshipServiceIntegrationTestSuite) TestDeriveSharedLabels_MultiLabelWeightsSum() {
	a1 := suite.createArtist("Band A")
	a2 := suite.createArtist("Band B")
	// Two labels, each with roster 4 (the pair + 2 fillers): the pair's score
	// sums per-label contributions, 1/3 + 1/3.
	for _, name := range []string{"Label One", "Label Two"} {
		label := suite.createLabel(name)
		suite.addArtistToLabel(label, a1)
		suite.addArtistToLabel(label, a2)
		suite.addArtistToLabel(label, suite.createArtist("Filler for "+name))
		suite.addArtistToLabel(label, suite.createArtist("Filler 2 for "+name))
	}

	_, err := suite.svc.DeriveSharedLabels(1)
	suite.Require().NoError(err)

	rel, err := suite.svc.GetRelationship(a1, a2, "shared_label")
	suite.Require().NoError(err)
	suite.Require().NotNil(rel)
	suite.Assert().InDelta(2.0/3.0, float64(rel.Score), 0.001)
}

func (suite *ArtistRelationshipServiceIntegrationTestSuite) TestDeriveSharedLabels_ScoreCappedAtOne() {
	a1 := suite.createArtist("Band A")
	a2 := suite.createArtist("Band B")
	// Three 2-artist labels: raw normalized weight 3.0, capped to 1.0.
	for _, name := range []string{"Cap One", "Cap Two", "Cap Three"} {
		label := suite.createLabel(name)
		suite.addArtistToLabel(label, a1)
		suite.addArtistToLabel(label, a2)
	}

	_, err := suite.svc.DeriveSharedLabels(1)
	suite.Require().NoError(err)

	rel, err := suite.svc.GetRelationship(a1, a2, "shared_label")
	suite.Require().NoError(err)
	suite.Require().NotNil(rel)
	suite.Assert().InDelta(1.0, float64(rel.Score), 0.001)
}

func (suite *ArtistRelationshipServiceIntegrationTestSuite) TestDeriveSharedLabels_RederiveRescoresGrownRoster() {
	a1 := suite.createArtist("Band A")
	a2 := suite.createArtist("Band B")
	label := suite.createLabel("Growing Label")
	suite.addArtistToLabel(label, a1)
	suite.addArtistToLabel(label, a2)

	_, err := suite.svc.DeriveSharedLabels(1)
	suite.Require().NoError(err)
	rel, _ := suite.svc.GetRelationship(a1, a2, "shared_label")
	suite.Require().NotNil(rel)
	suite.Assert().InDelta(1.0, float64(rel.Score), 0.001)

	// Roster grows to 3 (e.g. discography enrichment): re-derive updates the
	// existing row's score to 1/(3-1) = 0.5 in place — no wipe needed.
	suite.addArtistToLabel(label, suite.createArtist("Band C"))
	_, err = suite.svc.DeriveSharedLabels(1)
	suite.Require().NoError(err)

	rel, err = suite.svc.GetRelationship(a1, a2, "shared_label")
	suite.Require().NoError(err)
	suite.Require().NotNil(rel)
	suite.Assert().InDelta(0.5, float64(rel.Score), 0.001)
}

// ──────────────────────────────────────────────
// Graph Tests
// ──────────────────────────────────────────────

func (suite *ArtistRelationshipServiceIntegrationTestSuite) TestGetArtistGraph_Basic() {
	a1 := suite.createArtist("Center Band")
	a2 := suite.createArtist("Related Band 1")
	a3 := suite.createArtist("Related Band 2")

	_, err := suite.svc.CreateRelationship(a1, a2, "similar", false)
	suite.Require().NoError(err)
	_, err = suite.svc.CreateRelationship(a1, a3, "shared_bills", true)
	suite.Require().NoError(err)

	graph, err := suite.svc.GetArtistGraph(a1, nil, 0)
	suite.Require().NoError(err)
	suite.Require().NotNil(graph)

	suite.Assert().Equal(a1, graph.Center.ID)
	suite.Assert().Equal("Center Band", graph.Center.Name)
	suite.Assert().Len(graph.Nodes, 2)
	suite.Assert().Len(graph.Links, 2)
	suite.Assert().Nil(graph.UserVotes)
}

func (suite *ArtistRelationshipServiceIntegrationTestSuite) TestGetArtistGraph_FilterByTypes() {
	a1 := suite.createArtist("Center")
	a2 := suite.createArtist("Similar")
	a3 := suite.createArtist("Shared")

	_, err := suite.svc.CreateRelationship(a1, a2, "similar", false)
	suite.Require().NoError(err)
	_, err = suite.svc.CreateRelationship(a1, a3, "shared_bills", true)
	suite.Require().NoError(err)

	// Only similar
	graph, err := suite.svc.GetArtistGraph(a1, []string{"similar"}, 0)
	suite.Require().NoError(err)
	suite.Assert().Len(graph.Nodes, 1)
	suite.Assert().Len(graph.Links, 1)
	suite.Assert().Equal("similar", graph.Links[0].Type)
}

func (suite *ArtistRelationshipServiceIntegrationTestSuite) TestGetArtistGraph_CrossConnections() {
	a1 := suite.createArtist("Center")
	a2 := suite.createArtist("Related A")
	a3 := suite.createArtist("Related B")

	// Center connects to both
	_, err := suite.svc.CreateRelationship(a1, a2, "similar", false)
	suite.Require().NoError(err)
	_, err = suite.svc.CreateRelationship(a1, a3, "similar", false)
	suite.Require().NoError(err)
	// Cross-connection between related artists
	_, err = suite.svc.CreateRelationship(a2, a3, "shared_bills", true)
	suite.Require().NoError(err)

	graph, err := suite.svc.GetArtistGraph(a1, nil, 0)
	suite.Require().NoError(err)
	suite.Assert().Len(graph.Nodes, 2)
	// 2 center relationships + 1 cross-connection = 3 links
	suite.Assert().Len(graph.Links, 3)
}

func (suite *ArtistRelationshipServiceIntegrationTestSuite) TestGetArtistGraph_WithUserVotes() {
	a1 := suite.createArtist("Center")
	a2 := suite.createArtist("Related")
	user := suite.createUser("voter")

	_, err := suite.svc.CreateRelationship(a1, a2, "similar", false)
	suite.Require().NoError(err)
	suite.Require().NoError(suite.svc.Vote(a1, a2, "similar", user.ID, true))

	graph, err := suite.svc.GetArtistGraph(a1, nil, user.ID)
	suite.Require().NoError(err)
	suite.Require().NotNil(graph.UserVotes)
	suite.Assert().Len(graph.UserVotes, 1)

	// Find the vote key
	for _, v := range graph.UserVotes {
		suite.Assert().Equal("up", v)
	}
}

func (suite *ArtistRelationshipServiceIntegrationTestSuite) TestGetArtistGraph_EmptyGraph() {
	a1 := suite.createArtist("Lonely Artist")

	graph, err := suite.svc.GetArtistGraph(a1, nil, 0)
	suite.Require().NoError(err)
	suite.Assert().Equal(a1, graph.Center.ID)
	suite.Assert().Empty(graph.Nodes)
	suite.Assert().Empty(graph.Links)
}

func (suite *ArtistRelationshipServiceIntegrationTestSuite) TestGetArtistGraph_ArtistNotFound() {
	_, err := suite.svc.GetArtistGraph(99999, nil, 0)
	suite.Assert().Error(err)
	var artistErr *apperrors.ArtistError
	suite.Require().True(errors.As(err, &artistErr), "expected *apperrors.ArtistError, got %T", err)
	suite.Assert().Equal(apperrors.CodeArtistNotFound, artistErr.Code)
}

func (suite *ArtistRelationshipServiceIntegrationTestSuite) TestGetArtistGraph_UpcomingShowCounts() {
	a1 := suite.createArtist("Center")
	a2 := suite.createArtist("Related")

	// Create future show with related artist
	slug := fmt.Sprintf("future-show-%d", time.Now().UnixNano())
	futureShow := &catalogm.Show{
		Title:     "Future Show",
		Slug:      &slug,
		EventDate: time.Now().Add(48 * time.Hour),
		Status:    catalogm.ShowStatusApproved,
	}
	suite.db.Create(futureShow)
	suite.addArtistToShow(futureShow.ID, a2)

	_, err := suite.svc.CreateRelationship(a1, a2, "similar", false)
	suite.Require().NoError(err)

	graph, err := suite.svc.GetArtistGraph(a1, nil, 0)
	suite.Require().NoError(err)
	suite.Require().Len(graph.Nodes, 1)
	suite.Assert().Equal(1, graph.Nodes[0].UpcomingShowCount)
}

// ──────────────────────────────────────────────
// PSY-363 — Festival co-lineup (query-time) tests
// ──────────────────────────────────────────────

// createFestival inserts a festivals row using year-month-day strings
// for start/end. The `editionYear` matches the start_date's year.
func (suite *ArtistRelationshipServiceIntegrationTestSuite) createFestival(name string, startDate, endDate string, editionYear int) uint {
	slug := fmt.Sprintf("fest-%s-%d", name, time.Now().UnixNano())
	seriesSlug := fmt.Sprintf("fest-series-%s-%d", name, time.Now().UnixNano())
	f := &catalogm.Festival{
		Name:        name,
		Slug:        slug,
		SeriesSlug:  seriesSlug,
		EditionYear: editionYear,
		StartDate:   startDate,
		EndDate:     endDate,
		Status:      catalogm.FestivalStatusCompleted,
	}
	err := suite.db.Create(f).Error
	suite.Require().NoError(err)
	return f.ID
}

// addArtistToFestival inserts a festival_artists row. Position is auto.
func (suite *ArtistRelationshipServiceIntegrationTestSuite) addArtistToFestival(festivalID, artistID uint) {
	fa := &catalogm.FestivalArtist{
		FestivalID:  festivalID,
		ArtistID:    artistID,
		BillingTier: catalogm.BillingTierMidCard,
	}
	err := suite.db.Create(fa).Error
	suite.Require().NoError(err)
}

func (suite *ArtistRelationshipServiceIntegrationTestSuite) TestGetArtistGraph_FestivalCobill_OneSharedFestival() {
	a1 := suite.createArtist("Center")
	a2 := suite.createArtist("Co-billed")
	thisYear := time.Now().Year()
	startDate := fmt.Sprintf("%d-06-15", thisYear)
	endDate := fmt.Sprintf("%d-06-17", thisYear)
	f1 := suite.createFestival("Coachella", startDate, endDate, thisYear)
	suite.addArtistToFestival(f1, a1)
	suite.addArtistToFestival(f1, a2)

	graph, err := suite.svc.GetArtistGraph(a1, []string{"festival_cobill"}, 0)
	suite.Require().NoError(err)
	suite.Require().NotNil(graph)
	suite.Require().Len(graph.Links, 1, "expected 1 festival_cobill edge")
	suite.Assert().Equal("festival_cobill", graph.Links[0].Type)

	// Score: count=1 → base=min(1/3,1)=0.333..., recency boost active
	// (this year), so 0.333 * 1.2 = 0.4.
	suite.Assert().InDelta(0.4, graph.Links[0].Score, 0.0001)

	// Detail JSONB shape
	detail, ok := graph.Links[0].Detail.(map[string]interface{})
	suite.Require().True(ok, "detail should be map")
	suite.Assert().Equal("Coachella", detail["festival_names"])
	suite.Assert().Equal(1, asInt(detail["count"]))
	suite.Assert().Equal(thisYear, asInt(detail["most_recent_year"]))
}

func (suite *ArtistRelationshipServiceIntegrationTestSuite) TestGetArtistGraph_FestivalCobill_MultipleSharedFestivals() {
	a1 := suite.createArtist("Center")
	a2 := suite.createArtist("Co-billed")
	thisYear := time.Now().Year()

	// 4 shared festivals — exceeds the festivalCobillTopN cap (3), so
	// base score is min(4/3,1)=1.0; recency boost is also 1.0 capped.
	for i := 0; i < 4; i++ {
		startDate := fmt.Sprintf("%d-06-1%d", thisYear-i, i+1)
		endDate := fmt.Sprintf("%d-06-1%d", thisYear-i, i+2)
		fid := suite.createFestival(fmt.Sprintf("Festival-%d", i), startDate, endDate, thisYear-i)
		suite.addArtistToFestival(fid, a1)
		suite.addArtistToFestival(fid, a2)
	}

	graph, err := suite.svc.GetArtistGraph(a1, []string{"festival_cobill"}, 0)
	suite.Require().NoError(err)
	suite.Require().Len(graph.Links, 1)
	suite.Assert().Equal("festival_cobill", graph.Links[0].Type)
	suite.Assert().InDelta(1.0, graph.Links[0].Score, 0.0001)

	detail, ok := graph.Links[0].Detail.(map[string]interface{})
	suite.Require().True(ok)
	suite.Assert().Equal(4, asInt(detail["count"]))
	suite.Assert().Equal(thisYear, asInt(detail["most_recent_year"]))
	// Top 3 festival names by recency. Strict ordering by start_date DESC.
	names, _ := detail["festival_names"].(string)
	suite.Assert().Contains(names, "Festival-0")
	suite.Assert().Contains(names, "Festival-1")
	suite.Assert().Contains(names, "Festival-2")
}

func (suite *ArtistRelationshipServiceIntegrationTestSuite) TestGetArtistGraph_FestivalCobill_OldFestivalNoRecencyBoost() {
	a1 := suite.createArtist("Center")
	a2 := suite.createArtist("Co-billed")
	// Festival 5 years ago — outside the 2-year recency window, no boost.
	oldYear := time.Now().Year() - 5
	startDate := fmt.Sprintf("%d-06-15", oldYear)
	endDate := fmt.Sprintf("%d-06-17", oldYear)
	fid := suite.createFestival("Old Fest", startDate, endDate, oldYear)
	suite.addArtistToFestival(fid, a1)
	suite.addArtistToFestival(fid, a2)

	graph, err := suite.svc.GetArtistGraph(a1, []string{"festival_cobill"}, 0)
	suite.Require().NoError(err)
	suite.Require().Len(graph.Links, 1)
	// count=1, no boost: score = 1/3 ≈ 0.3333
	suite.Assert().InDelta(1.0/3.0, graph.Links[0].Score, 0.0001)
}

func (suite *ArtistRelationshipServiceIntegrationTestSuite) TestGetArtistGraph_FestivalCobill_FilterExcludesOtherTypes() {
	a1 := suite.createArtist("Center")
	a2 := suite.createArtist("Festival peer")
	a3 := suite.createArtist("Stored peer")

	// Festival co-lineup with a2 only.
	thisYear := time.Now().Year()
	startDate := fmt.Sprintf("%d-06-15", thisYear)
	endDate := fmt.Sprintf("%d-06-17", thisYear)
	fid := suite.createFestival("Lollapalooza", startDate, endDate, thisYear)
	suite.addArtistToFestival(fid, a1)
	suite.addArtistToFestival(fid, a2)

	// Stored 'similar' relationship with a3 — should NOT appear when
	// filter is festival_cobill only.
	_, err := suite.svc.CreateRelationship(a1, a3, "similar", false)
	suite.Require().NoError(err)

	graph, err := suite.svc.GetArtistGraph(a1, []string{"festival_cobill"}, 0)
	suite.Require().NoError(err)
	suite.Require().Len(graph.Links, 1)
	suite.Assert().Equal("festival_cobill", graph.Links[0].Type)
	// The 'similar' edge should be absent.
	for _, l := range graph.Links {
		suite.Assert().NotEqual("similar", l.Type)
	}
}

// TestGetArtistGraph_FestivalCobill_ExcludedFromDefault asserts the PSY-954
// opt-in semantics: an empty types filter returns STORED types only and must
// NOT auto-include the query-time festival_cobill signal. Requesting it
// explicitly (alone or alongside stored types) still returns it.
func (suite *ArtistRelationshipServiceIntegrationTestSuite) TestGetArtistGraph_FestivalCobill_ExcludedFromDefault() {
	a1 := suite.createArtist("Center")
	a2 := suite.createArtist("Festival peer")

	// One shared festival.
	thisYear := time.Now().Year()
	startDate := fmt.Sprintf("%d-06-15", thisYear)
	endDate := fmt.Sprintf("%d-06-17", thisYear)
	fid := suite.createFestival("ACL", startDate, endDate, thisYear)
	suite.addArtistToFestival(fid, a1)
	suite.addArtistToFestival(fid, a2)

	// Stored 'similar' relationship.
	_, err := suite.svc.CreateRelationship(a1, a2, "similar", false)
	suite.Require().NoError(err)

	// Empty types = STORED types only (PSY-954). festival_cobill is opt-in
	// and must be absent; the stored 'similar' edge must still be present.
	defaultGraph, err := suite.svc.GetArtistGraph(a1, nil, 0)
	suite.Require().NoError(err)

	hasFestivalCobill := false
	hasSimilar := false
	for _, l := range defaultGraph.Links {
		if l.Type == "festival_cobill" {
			hasFestivalCobill = true
		}
		if l.Type == "similar" {
			hasSimilar = true
		}
	}
	suite.Assert().False(hasFestivalCobill, "festival_cobill must be absent on a default (empty-types) graph — it is opt-in (PSY-954)")
	suite.Assert().True(hasSimilar, "stored 'similar' edge must still be present on a default graph")

	// Explicit opt-in: festival_cobill alongside the stored type returns both.
	optInGraph, err := suite.svc.GetArtistGraph(a1, []string{"similar", "festival_cobill"}, 0)
	suite.Require().NoError(err)

	hasFestivalCobill = false
	hasSimilar = false
	for _, l := range optInGraph.Links {
		if l.Type == "festival_cobill" {
			hasFestivalCobill = true
		}
		if l.Type == "similar" {
			hasSimilar = true
		}
	}
	suite.Assert().True(hasFestivalCobill, "festival_cobill must be present when explicitly requested")
	suite.Assert().True(hasSimilar, "stored 'similar' edge must be present alongside an explicit festival_cobill request")
}

func (suite *ArtistRelationshipServiceIntegrationTestSuite) TestGetArtistGraph_FestivalCobill_NoSharedFestivals() {
	a1 := suite.createArtist("Center")
	a2 := suite.createArtist("Other")
	thisYear := time.Now().Year()
	startDate := fmt.Sprintf("%d-06-15", thisYear)
	endDate := fmt.Sprintf("%d-06-17", thisYear)

	// Each artist has their own festival, but no shared festival.
	f1 := suite.createFestival("Solo Fest 1", startDate, endDate, thisYear)
	f2 := suite.createFestival("Solo Fest 2", startDate, endDate, thisYear)
	suite.addArtistToFestival(f1, a1)
	suite.addArtistToFestival(f2, a2)

	graph, err := suite.svc.GetArtistGraph(a1, []string{"festival_cobill"}, 0)
	suite.Require().NoError(err)
	suite.Assert().Empty(graph.Links)
}

func (suite *ArtistRelationshipServiceIntegrationTestSuite) TestGetArtistGraph_FestivalCobill_CrossEdges() {
	// Center connects to a2 via stored 'similar'; a2 connects to a3 via
	// query-time festival_cobill. The cross edge should appear.
	a1 := suite.createArtist("Center")
	a2 := suite.createArtist("Peer A")
	a3 := suite.createArtist("Peer B")

	thisYear := time.Now().Year()
	startDate := fmt.Sprintf("%d-06-15", thisYear)
	endDate := fmt.Sprintf("%d-06-17", thisYear)

	// a2 ↔ a3 share a festival.
	f1 := suite.createFestival("CrossFest", startDate, endDate, thisYear)
	suite.addArtistToFestival(f1, a2)
	suite.addArtistToFestival(f1, a3)

	// Center ↔ a2 and Center ↔ a3 stored similar relationships so they
	// surface as related artists.
	_, err := suite.svc.CreateRelationship(a1, a2, "similar", false)
	suite.Require().NoError(err)
	_, err = suite.svc.CreateRelationship(a1, a3, "similar", false)
	suite.Require().NoError(err)

	// festival_cobill is opt-in (PSY-954), so request it explicitly to get
	// the cross-edge between the two related peers.
	graph, err := suite.svc.GetArtistGraph(a1, []string{"similar", "festival_cobill"}, 0)
	suite.Require().NoError(err)

	// Find the festival_cobill cross edge between a2 and a3.
	hasCrossFestivalCobill := false
	for _, l := range graph.Links {
		if l.Type == "festival_cobill" &&
			((l.SourceID == a2 && l.TargetID == a3) || (l.SourceID == a3 && l.TargetID == a2)) {
			hasCrossFestivalCobill = true
		}
	}
	suite.Assert().True(hasCrossFestivalCobill, "expected a festival_cobill cross edge between peer artists")
}

// asInt coerces a JSON-decoded numeric value to int. JSON numbers
// round-trip through float64 in interface{}.
func asInt(v interface{}) int {
	switch n := v.(type) {
	case int:
		return n
	case int64:
		return int(n)
	case float64:
		return int(n)
	default:
		return 0
	}
}

// ──────────────────────────────────────────────
// PSY-1293: ego-graph backbone UNION
// ──────────────────────────────────────────────

// insertRel inserts an artist_relationships row honoring the source<target CHECK constraint.
func (suite *ArtistRelationshipServiceIntegrationTestSuite) insertRel(aID, bID uint, typ string, score float32, sig *float64) {
	lo, hi := aID, bID
	if lo > hi {
		lo, hi = hi, lo
	}
	suite.Require().NoError(suite.db.Create(&catalogm.ArtistRelationship{
		SourceArtistID:       lo,
		TargetArtistID:       hi,
		RelationshipType:     typ,
		Score:                score,
		AutoDerived:          true,
		BackboneSignificance: sig,
	}).Error)
}

// A mid-degree ego whose backbone-significant radio link ranks below the top-k score cut still keeps
// that link (UNION), while a non-significant edge below the cut stays dropped.
func (suite *ArtistRelationshipServiceIntegrationTestSuite) TestGetArtistGraph_BackboneUnion_SurfacesNicheEdgeBeyondTopK() {
	center := suite.createArtist("Center")

	// Fill the top-k (30) with high-score, NON-significant radio edges.
	for i := 0; i < 30; i++ {
		n := suite.createArtist(fmt.Sprintf("Top %d", i))
		suite.insertRel(center, n, catalogm.RelationshipTypeRadioCooccurrence, 0.9, nil)
	}
	// Niche edge: low score (cut by top-k) but backbone-significant (< 0.10 default alpha) → UNIONed.
	niche := suite.createArtist("Niche")
	nicheSig := 0.02
	suite.insertRel(center, niche, catalogm.RelationshipTypeRadioCooccurrence, 0.01, &nicheSig)
	// Noise edge: low score AND not backbone-significant (>= alpha) → stays cut.
	noise := suite.createArtist("Noise")
	noiseSig := 0.50
	suite.insertRel(center, noise, catalogm.RelationshipTypeRadioCooccurrence, 0.02, &noiseSig)

	graph, err := suite.svc.GetArtistGraph(center, nil, 0)
	suite.Require().NoError(err)

	// 30 top-k + 1 unioned niche = 31; noise excluded.
	suite.Assert().Len(graph.Nodes, 31)
	ids := map[uint]bool{}
	for _, n := range graph.Nodes {
		ids[n.ID] = true
	}
	suite.Assert().True(ids[niche], "backbone-significant niche edge is unioned in beyond the top-k cap")
	suite.Assert().False(ids[noise], "non-significant edge stays cut by the top-k cap")
}

// The backbone UNION must not fire when radio_cooccurrence is filtered out of the requested types.
func (suite *ArtistRelationshipServiceIntegrationTestSuite) TestGetArtistGraph_BackboneUnion_SkippedWhenRadioFilteredOut() {
	center := suite.createArtist("Center")

	// A backbone-significant radio edge that WOULD be unioned if radio were in scope.
	radioN := suite.createArtist("Radio N")
	sig := 0.02
	suite.insertRel(center, radioN, catalogm.RelationshipTypeRadioCooccurrence, 0.01, &sig)
	// A similar edge — the only requested type.
	similarN := suite.createArtist("Similar N")
	suite.insertRel(center, similarN, catalogm.RelationshipTypeSimilar, 0.5, nil)

	graph, err := suite.svc.GetArtistGraph(center, []string{"similar"}, 0)
	suite.Require().NoError(err)

	suite.Require().Len(graph.Nodes, 1)
	suite.Assert().Equal("similar", graph.Links[0].Type)
}

// crossEdgeSet collects a graph's links as canonical (lo, hi, type) tuples.
func crossEdgeSet(links []contracts.ArtistGraphLink) map[[2]uint]map[string]bool {
	got := map[[2]uint]map[string]bool{}
	for _, l := range links {
		k := [2]uint{min(l.SourceID, l.TargetID), max(l.SourceID, l.TargetID)}
		if got[k] == nil {
			got[k] = map[string]bool{}
		}
		got[k][l.Type] = true
	}
	return got
}

// Cross edges (step 7, PSY-1301): NULL backbone significance is NOT a drop condition — the
// backbone alpha deliberately does not apply to ego cross edges (see egoCrossRadioPerNodeCap) —
// and non-radio cross edges pass through unbounded.
func (suite *ArtistRelationshipServiceIntegrationTestSuite) TestGetArtistGraph_CrossEdges_NullSigKept_NonRadioUnbounded() {
	center := suite.createArtist("Center")
	n1 := suite.createArtist("N1")
	n2 := suite.createArtist("N2")

	suite.insertRel(center, n1, catalogm.RelationshipTypeSharedBills, 0.9, nil)
	suite.insertRel(center, n2, catalogm.RelationshipTypeSharedBills, 0.8, nil)
	// Radio cross edge with NO backbone stamp (the pre-sync / degenerate-edge state).
	suite.insertRel(n1, n2, catalogm.RelationshipTypeRadioCooccurrence, 0.5, nil)
	suite.insertRel(n1, n2, catalogm.RelationshipTypeSharedBills, 0.4, nil)

	graph, err := suite.svc.GetArtistGraph(center, nil, 0)
	suite.Require().NoError(err)

	got := crossEdgeSet(graph.Links)
	pair := [2]uint{min(n1, n2), max(n1, n2)}
	suite.Assert().True(got[pair][catalogm.RelationshipTypeRadioCooccurrence],
		"NULL-significance radio cross edge is kept (no backbone alpha on ego cross edges)")
	suite.Assert().True(got[pair][catalogm.RelationshipTypeSharedBills],
		"non-radio cross edge always kept")
}

// Radio cross edges are bounded to per-node top-K with EITHER-endpoint semantics (PSY-1301):
// an edge drops only when it ranks beyond K for BOTH endpoints; an edge that is a niche node's
// best link survives a saturated partner.
func (suite *ArtistRelationshipServiceIntegrationTestSuite) TestGetArtistGraph_CrossEdges_RadioPerNodeCap() {
	center := suite.createArtist("Center")

	// A complete radio graph among K+2 neighbors: every member has K+1 radio
	// cross edges, so each member's weakest edge exceeds its top-K. The edge
	// with the globally lowest score is the weakest for BOTH endpoints → the
	// only guaranteed drop.
	n := egoCrossRadioPerNodeCap + 2
	members := make([]uint, 0, n)
	for i := 0; i < n; i++ {
		m := suite.createArtist(fmt.Sprintf("Member %02d", i))
		suite.insertRel(center, m, catalogm.RelationshipTypeSharedBills, 0.9, nil)
		members = append(members, m)
	}
	// Unique, strictly decreasing scores; the LAST inserted pair is the global minimum.
	score := float32(0.99)
	var weakestPair [2]uint
	for i := 0; i < n; i++ {
		for j := i + 1; j < n; j++ {
			suite.insertRel(members[i], members[j], catalogm.RelationshipTypeRadioCooccurrence, score, nil)
			weakestPair = [2]uint{min(members[i], members[j]), max(members[i], members[j])}
			score -= 0.001
		}
	}

	// Niche case: one extra neighbor whose ONLY radio cross edge scores below
	// everything above — rank 1 for the niche node, far beyond K for the member
	// it attaches to → either-endpoint keeps it.
	niche := suite.createArtist("Niche")
	suite.insertRel(center, niche, catalogm.RelationshipTypeSharedBills, 0.7, nil)
	suite.insertRel(niche, members[0], catalogm.RelationshipTypeRadioCooccurrence, 0.01, nil)

	graph, err := suite.svc.GetArtistGraph(center, nil, 0)
	suite.Require().NoError(err)

	got := crossEdgeSet(graph.Links)
	suite.Assert().False(got[weakestPair][catalogm.RelationshipTypeRadioCooccurrence],
		"the globally weakest clique edge is beyond top-K for both endpoints and drops")
	strongestPair := [2]uint{min(members[0], members[1]), max(members[0], members[1])}
	suite.Assert().True(got[strongestPair][catalogm.RelationshipTypeRadioCooccurrence],
		"the strongest clique edge is kept")
	nichePair := [2]uint{min(niche, members[0]), max(niche, members[0])}
	suite.Assert().True(got[nichePair][catalogm.RelationshipTypeRadioCooccurrence],
		"a niche node's only cross edge survives via the either-endpoint rule")
}

// GetRelatedArtists shares the batched vote path (PSY-1301) — assert its vote fields survive the
// migration (a key-orientation regression here would otherwise ship green as all-zero tallies).
func (suite *ArtistRelationshipServiceIntegrationTestSuite) TestGetRelatedArtists_VoteCounts_Batched() {
	center := suite.createArtist("Center")
	other := suite.createArtist("Other")
	_, err := suite.svc.CreateRelationship(center, other, "similar", false)
	suite.Require().NoError(err)

	u1 := suite.createUser("rvoter1")
	u2 := suite.createUser("rvoter2")
	lo, hi := min(center, other), max(center, other)
	for _, v := range []struct {
		user *authm.User
		dir  int16
	}{{u1, 1}, {u2, -1}} {
		suite.Require().NoError(suite.db.Create(&catalogm.ArtistRelationshipVote{
			SourceArtistID:   lo,
			TargetArtistID:   hi,
			RelationshipType: "similar",
			UserID:           v.user.ID,
			Direction:        v.dir,
		}).Error)
	}

	related, err := suite.svc.GetRelatedArtists(center, "", 30)
	suite.Require().NoError(err)
	suite.Require().Len(related, 1)
	suite.Assert().Equal(1, related[0].Upvotes)
	suite.Assert().Equal(1, related[0].Downvotes)
	suite.Assert().Greater(related[0].WilsonScore, 0.0)
}

// The batched vote-count path (PSY-1301) populates VotesUp/VotesDown on both center and cross edges.
func (suite *ArtistRelationshipServiceIntegrationTestSuite) TestGetArtistGraph_VoteCounts_Batched() {
	center := suite.createArtist("Center")
	n1 := suite.createArtist("N1")
	n2 := suite.createArtist("N2")

	suite.insertRel(center, n1, catalogm.RelationshipTypeSharedBills, 0.9, nil)
	suite.insertRel(center, n2, catalogm.RelationshipTypeSharedBills, 0.8, nil)
	suite.insertRel(n1, n2, catalogm.RelationshipTypeSharedBills, 0.5, nil)

	vote := func(a, b uint, dir int16, user *authm.User) {
		lo, hi := a, b
		if lo > hi {
			lo, hi = hi, lo
		}
		suite.Require().NoError(suite.db.Create(&catalogm.ArtistRelationshipVote{
			SourceArtistID:   lo,
			TargetArtistID:   hi,
			RelationshipType: catalogm.RelationshipTypeSharedBills,
			UserID:           user.ID,
			Direction:        dir,
		}).Error)
	}
	u1 := suite.createUser("voter1")
	u2 := suite.createUser("voter2")
	u3 := suite.createUser("voter3")
	vote(center, n1, 1, u1)
	vote(center, n1, 1, u2)
	vote(center, n1, -1, u3)
	vote(n1, n2, -1, u1) // cross edge downvote

	graph, err := suite.svc.GetArtistGraph(center, nil, 0)
	suite.Require().NoError(err)

	byPair := map[[2]uint]contracts.ArtistGraphLink{}
	for _, l := range graph.Links {
		lo, hi := l.SourceID, l.TargetID
		if lo > hi {
			lo, hi = hi, lo
		}
		byPair[[2]uint{lo, hi}] = l
	}

	centerEdge := byPair[[2]uint{min(center, n1), max(center, n1)}]
	suite.Assert().Equal(2, centerEdge.VotesUp, "center edge upvotes")
	suite.Assert().Equal(1, centerEdge.VotesDown, "center edge downvotes")

	crossEdge := byPair[[2]uint{min(n1, n2), max(n1, n2)}]
	suite.Assert().Equal(0, crossEdge.VotesUp, "cross edge upvotes")
	suite.Assert().Equal(1, crossEdge.VotesDown, "cross edge downvotes")

	unvoted := byPair[[2]uint{min(center, n2), max(center, n2)}]
	suite.Assert().Equal(0, unvoted.VotesUp)
	suite.Assert().Equal(0, unvoted.VotesDown)
}

// ──────────────────────────────────────────────
// Run all integration tests
// ──────────────────────────────────────────────

func TestArtistRelationshipServiceIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	suite.Run(t, new(ArtistRelationshipServiceIntegrationTestSuite))
}
