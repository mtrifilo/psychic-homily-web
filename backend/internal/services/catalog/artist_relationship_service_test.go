package catalog

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"

	"psychic-homily-backend/internal/models"
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
	_, _ = sqlDB.Exec("DELETE FROM artists")
	_, _ = sqlDB.Exec("DELETE FROM users")
}

func (suite *ArtistRelationshipServiceIntegrationTestSuite) createArtist(name string) uint {
	slug := fmt.Sprintf("%s-%d", name, time.Now().UnixNano())
	artist := &models.Artist{Name: name, Slug: &slug}
	err := suite.db.Create(artist).Error
	suite.Require().NoError(err)
	return artist.ID
}

func (suite *ArtistRelationshipServiceIntegrationTestSuite) createUser(name string) *models.User {
	email := fmt.Sprintf("%s-%d@test.com", name, time.Now().UnixNano())
	user := &models.User{Email: &email, FirstName: &name, IsActive: true, EmailVerified: true}
	err := suite.db.Create(user).Error
	suite.Require().NoError(err)
	return user
}

func (suite *ArtistRelationshipServiceIntegrationTestSuite) createShow(title string) uint {
	slug := fmt.Sprintf("show-%d", time.Now().UnixNano())
	show := &models.Show{
		Title:     title,
		Slug:      &slug,
		EventDate: time.Now().Add(24 * time.Hour),
		Status:    models.ShowStatusApproved,
	}
	err := suite.db.Create(show).Error
	suite.Require().NoError(err)
	return show.ID
}

func (suite *ArtistRelationshipServiceIntegrationTestSuite) addArtistToShow(showID, artistID uint) {
	suite.db.Exec("INSERT INTO show_artists (show_id, artist_id) VALUES (?, ?)", showID, artistID)
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

	src, tgt := models.CanonicalOrder(a1, a2)
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
	suite.svc.CreateRelationship(a1, a2, "similar", false)

	rel, err := suite.svc.GetRelationship(a1, a2, "similar")
	suite.Require().NoError(err)
	suite.Require().NotNil(rel)
}

func (suite *ArtistRelationshipServiceIntegrationTestSuite) TestGetRelationship_ReversedOrder() {
	a1 := suite.createArtist("Band A")
	a2 := suite.createArtist("Band B")
	suite.svc.CreateRelationship(a1, a2, "similar", false)

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

	suite.svc.CreateRelationship(a1, a2, "similar", false)
	suite.svc.CreateRelationship(a1, a3, "shared_bills", true)

	related, err := suite.svc.GetRelatedArtists(a1, "", 30)
	suite.Require().NoError(err)
	suite.Assert().Len(related, 2)
}

func (suite *ArtistRelationshipServiceIntegrationTestSuite) TestGetRelatedArtists_FilterByType() {
	a1 := suite.createArtist("Center")
	a2 := suite.createArtist("Similar")
	a3 := suite.createArtist("Shared")

	suite.svc.CreateRelationship(a1, a2, "similar", false)
	suite.svc.CreateRelationship(a1, a3, "shared_bills", true)

	related, err := suite.svc.GetRelatedArtists(a1, "similar", 30)
	suite.Require().NoError(err)
	suite.Assert().Len(related, 1)
	suite.Assert().Equal("Similar", related[0].Name)
}

func (suite *ArtistRelationshipServiceIntegrationTestSuite) TestDeleteRelationship() {
	a1 := suite.createArtist("Band A")
	a2 := suite.createArtist("Band B")
	suite.svc.CreateRelationship(a1, a2, "similar", false)

	err := suite.svc.DeleteRelationship(a1, a2, "similar")
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
	suite.svc.CreateRelationship(a1, a2, "similar", false)

	err := suite.svc.Vote(a1, a2, "similar", user.ID, true)
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
	suite.svc.CreateRelationship(a1, a2, "similar", false)

	err := suite.svc.Vote(a1, a2, "similar", user.ID, false)
	suite.Assert().NoError(err)

	vote, _ := suite.svc.GetUserVote(a1, a2, "similar", user.ID)
	suite.Assert().Equal(int16(-1), vote.Direction)
}

func (suite *ArtistRelationshipServiceIntegrationTestSuite) TestVote_ChangeDirection() {
	a1 := suite.createArtist("Band A")
	a2 := suite.createArtist("Band B")
	user := suite.createUser("voter")
	suite.svc.CreateRelationship(a1, a2, "similar", false)

	suite.svc.Vote(a1, a2, "similar", user.ID, true)
	suite.svc.Vote(a1, a2, "similar", user.ID, false) // Change to downvote

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
	suite.svc.CreateRelationship(a1, a2, "similar", false)

	// Two upvotes
	suite.svc.Vote(a1, a2, "similar", user1.ID, true)
	suite.svc.Vote(a1, a2, "similar", user2.ID, true)

	rel, _ := suite.svc.GetRelationship(a1, a2, "similar")
	suite.Assert().Greater(rel.Score, float32(0))
}

func (suite *ArtistRelationshipServiceIntegrationTestSuite) TestRemoveVote() {
	a1 := suite.createArtist("Band A")
	a2 := suite.createArtist("Band B")
	user := suite.createUser("voter")
	suite.svc.CreateRelationship(a1, a2, "similar", false)

	suite.svc.Vote(a1, a2, "similar", user.ID, true)
	err := suite.svc.RemoveVote(a1, a2, "similar", user.ID)
	suite.Assert().NoError(err)

	vote, _ := suite.svc.GetUserVote(a1, a2, "similar", user.ID)
	suite.Assert().Nil(vote)
}

func (suite *ArtistRelationshipServiceIntegrationTestSuite) TestGetUserVote_NotVoted() {
	a1 := suite.createArtist("Band A")
	a2 := suite.createArtist("Band B")
	user := suite.createUser("voter")
	suite.svc.CreateRelationship(a1, a2, "similar", false)

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

	suite.svc.DeriveSharedBills(2)

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

// ──────────────────────────────────────────────
// Graph Tests
// ──────────────────────────────────────────────

func (suite *ArtistRelationshipServiceIntegrationTestSuite) TestGetArtistGraph_Basic() {
	a1 := suite.createArtist("Center Band")
	a2 := suite.createArtist("Related Band 1")
	a3 := suite.createArtist("Related Band 2")

	suite.svc.CreateRelationship(a1, a2, "similar", false)
	suite.svc.CreateRelationship(a1, a3, "shared_bills", true)

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

	suite.svc.CreateRelationship(a1, a2, "similar", false)
	suite.svc.CreateRelationship(a1, a3, "shared_bills", true)

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
	suite.svc.CreateRelationship(a1, a2, "similar", false)
	suite.svc.CreateRelationship(a1, a3, "similar", false)
	// Cross-connection between related artists
	suite.svc.CreateRelationship(a2, a3, "shared_bills", true)

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

	suite.svc.CreateRelationship(a1, a2, "similar", false)
	suite.svc.Vote(a1, a2, "similar", user.ID, true)

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
	suite.Assert().Contains(err.Error(), "artist not found")
}

func (suite *ArtistRelationshipServiceIntegrationTestSuite) TestGetArtistGraph_UpcomingShowCounts() {
	a1 := suite.createArtist("Center")
	a2 := suite.createArtist("Related")

	// Create future show with related artist
	slug := fmt.Sprintf("future-show-%d", time.Now().UnixNano())
	futureShow := &models.Show{
		Title:     "Future Show",
		Slug:      &slug,
		EventDate: time.Now().Add(48 * time.Hour),
		Status:    models.ShowStatusApproved,
	}
	suite.db.Create(futureShow)
	suite.addArtistToShow(futureShow.ID, a2)

	suite.svc.CreateRelationship(a1, a2, "similar", false)

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
	f := &models.Festival{
		Name:        name,
		Slug:        slug,
		SeriesSlug:  seriesSlug,
		EditionYear: editionYear,
		StartDate:   startDate,
		EndDate:     endDate,
		Status:      models.FestivalStatusCompleted,
	}
	err := suite.db.Create(f).Error
	suite.Require().NoError(err)
	return f.ID
}

// addArtistToFestival inserts a festival_artists row. Position is auto.
func (suite *ArtistRelationshipServiceIntegrationTestSuite) addArtistToFestival(festivalID, artistID uint) {
	fa := &models.FestivalArtist{
		FestivalID:  festivalID,
		ArtistID:    artistID,
		BillingTier: models.BillingTierMidCard,
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

func (suite *ArtistRelationshipServiceIntegrationTestSuite) TestGetArtistGraph_FestivalCobill_IncludedInAllTypes() {
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

	// Empty types = include all (stored + query-time).
	graph, err := suite.svc.GetArtistGraph(a1, nil, 0)
	suite.Require().NoError(err)

	hasFestivalCobill := false
	hasSimilar := false
	for _, l := range graph.Links {
		if l.Type == "festival_cobill" {
			hasFestivalCobill = true
		}
		if l.Type == "similar" {
			hasSimilar = true
		}
	}
	suite.Assert().True(hasFestivalCobill, "festival_cobill edge should be present when types is empty")
	suite.Assert().True(hasSimilar, "similar edge should be present when types is empty")
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
	suite.svc.CreateRelationship(a1, a2, "similar", false)
	suite.svc.CreateRelationship(a1, a3, "similar", false)

	graph, err := suite.svc.GetArtistGraph(a1, nil, 0)
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
// Run all integration tests
// ──────────────────────────────────────────────

func TestArtistRelationshipServiceIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	suite.Run(t, new(ArtistRelationshipServiceIntegrationTestSuite))
}
