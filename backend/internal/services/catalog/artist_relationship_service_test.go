package catalog

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"psychic-homily-backend/internal/models"
	"psychic-homily-backend/internal/testutil"
)

// =============================================================================
// UNIT TESTS
// =============================================================================

func TestNewArtistRelationshipService(t *testing.T) {
	svc := NewArtistRelationshipService(nil)
	assert.NotNil(t, svc)
}

func TestArtistRelationshipService_NilDatabase(t *testing.T) {
	svc := &ArtistRelationshipService{db: nil}

	t.Run("CreateRelationship", func(t *testing.T) {
		_, err := svc.CreateRelationship(1, 2, "similar", false)
		assert.Error(t, err)
		assert.Equal(t, "database not initialized", err.Error())
	})

	t.Run("GetRelationship", func(t *testing.T) {
		_, err := svc.GetRelationship(1, 2, "similar")
		assert.Error(t, err)
		assert.Equal(t, "database not initialized", err.Error())
	})

	t.Run("GetRelatedArtists", func(t *testing.T) {
		_, err := svc.GetRelatedArtists(1, "", 10)
		assert.Error(t, err)
		assert.Equal(t, "database not initialized", err.Error())
	})

	t.Run("DeleteRelationship", func(t *testing.T) {
		err := svc.DeleteRelationship(1, 2, "similar")
		assert.Error(t, err)
		assert.Equal(t, "database not initialized", err.Error())
	})

	t.Run("Vote", func(t *testing.T) {
		err := svc.Vote(1, 2, "similar", 1, true)
		assert.Error(t, err)
		assert.Equal(t, "database not initialized", err.Error())
	})

	t.Run("RemoveVote", func(t *testing.T) {
		err := svc.RemoveVote(1, 2, "similar", 1)
		assert.Error(t, err)
		assert.Equal(t, "database not initialized", err.Error())
	})

	t.Run("GetUserVote", func(t *testing.T) {
		_, err := svc.GetUserVote(1, 2, "similar", 1)
		assert.Error(t, err)
		assert.Equal(t, "database not initialized", err.Error())
	})

	t.Run("DeriveSharedBills", func(t *testing.T) {
		_, err := svc.DeriveSharedBills(2)
		assert.Error(t, err)
		assert.Equal(t, "database not initialized", err.Error())
	})
}

// =============================================================================
// INTEGRATION TESTS
// =============================================================================

type ArtistRelationshipServiceIntegrationTestSuite struct {
	suite.Suite
	container testcontainers.Container
	db        *gorm.DB
	svc       *ArtistRelationshipService
	ctx       context.Context
}

func (suite *ArtistRelationshipServiceIntegrationTestSuite) SetupSuite() {
	suite.ctx = context.Background()

	container, err := testcontainers.GenericContainer(suite.ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        "postgres:18",
			ExposedPorts: []string{"5432/tcp"},
			Env: map[string]string{
				"POSTGRES_DB":       "test_db",
				"POSTGRES_USER":     "test_user",
				"POSTGRES_PASSWORD": "test_password",
			},
			WaitingFor: wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(120 * time.Second),
		},
		Started: true,
	})
	suite.Require().NoError(err)
	suite.container = container

	host, err := container.Host(suite.ctx)
	suite.Require().NoError(err)
	port, err := container.MappedPort(suite.ctx, "5432")
	suite.Require().NoError(err)

	dsn := fmt.Sprintf("host=%s port=%s user=test_user password=test_password dbname=test_db sslmode=disable",
		host, port.Port())

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	suite.Require().NoError(err)

	sqlDB, err := db.DB()
	suite.Require().NoError(err)
	testutil.RunAllMigrations(suite.T(), sqlDB, filepath.Join("..", "..", "..", "db", "migrations"))

	suite.db = db
	suite.svc = NewArtistRelationshipService(db)
}

func (suite *ArtistRelationshipServiceIntegrationTestSuite) TearDownSuite() {
	if suite.container != nil {
		_ = suite.container.Terminate(suite.ctx)
	}
}

func (suite *ArtistRelationshipServiceIntegrationTestSuite) SetupTest() {
	sqlDB, _ := suite.db.DB()
	_, _ = sqlDB.Exec("DELETE FROM artist_relationship_votes")
	_, _ = sqlDB.Exec("DELETE FROM artist_relationships")
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
// Run all integration tests
// ──────────────────────────────────────────────

func TestArtistRelationshipServiceIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	suite.Run(t, new(ArtistRelationshipServiceIntegrationTestSuite))
}
