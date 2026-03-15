package admin

import (
	"context"
	"encoding/json"
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
// UNIT TESTS (No Database Required)
// =============================================================================

func TestNewAnalyticsService(t *testing.T) {
	t.Run("NilDB", func(t *testing.T) {
		svc := NewAnalyticsService(nil)
		assert.NotNil(t, svc)
	})

	t.Run("ExplicitDB", func(t *testing.T) {
		db := &gorm.DB{}
		svc := NewAnalyticsService(db)
		assert.NotNil(t, svc)
	})
}

func TestAnalyticsService_NilDB(t *testing.T) {
	svc := &AnalyticsService{db: nil}

	t.Run("GetGrowthMetrics", func(t *testing.T) {
		_, err := svc.GetGrowthMetrics(6)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "database not initialized")
	})

	t.Run("GetEngagementMetrics", func(t *testing.T) {
		_, err := svc.GetEngagementMetrics(6)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "database not initialized")
	})

	t.Run("GetCommunityHealth", func(t *testing.T) {
		_, err := svc.GetCommunityHealth()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "database not initialized")
	})

	t.Run("GetDataQualityTrends", func(t *testing.T) {
		_, err := svc.GetDataQualityTrends(6)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "database not initialized")
	})
}

func TestClampMonths(t *testing.T) {
	assert.Equal(t, 1, clampMonths(0))
	assert.Equal(t, 1, clampMonths(-5))
	assert.Equal(t, 1, clampMonths(1))
	assert.Equal(t, 6, clampMonths(6))
	assert.Equal(t, 24, clampMonths(24))
	assert.Equal(t, 24, clampMonths(100))
}

func TestGenerateMonthKeys(t *testing.T) {
	keys := generateMonthKeys(3)
	assert.Len(t, keys, 3)
	// Current month should be the last key
	now := time.Now().UTC()
	assert.Equal(t, now.Format("2006-01"), keys[2])
}

// =============================================================================
// INTEGRATION TESTS (With Real Database)
// =============================================================================

type AnalyticsServiceIntegrationTestSuite struct {
	suite.Suite
	container testcontainers.Container
	db        *gorm.DB
	service   *AnalyticsService
	ctx       context.Context
}

func (suite *AnalyticsServiceIntegrationTestSuite) SetupSuite() {
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
			WaitingFor: wait.ForLog("database system is ready to accept connections").WithOccurrence(2).WithStartupTimeout(120 * time.Second),
		},
		Started: true,
	})
	if err != nil {
		suite.T().Fatalf("failed to start postgres container: %v", err)
	}
	suite.container = container

	host, err := container.Host(suite.ctx)
	if err != nil {
		suite.T().Fatalf("failed to get host: %v", err)
	}
	port, err := container.MappedPort(suite.ctx, "5432")
	if err != nil {
		suite.T().Fatalf("failed to get port: %v", err)
	}

	dsn := fmt.Sprintf("host=%s port=%s user=test_user password=test_password dbname=test_db sslmode=disable",
		host, port.Port())

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		suite.T().Fatalf("failed to connect to test database: %v", err)
	}
	suite.db = db

	sqlDB, err := db.DB()
	if err != nil {
		suite.T().Fatalf("failed to get sql.DB: %v", err)
	}

	testutil.RunAllMigrations(suite.T(), sqlDB, filepath.Join("..", "..", "..", "db", "migrations"))

	suite.service = &AnalyticsService{db: db}
}

func (suite *AnalyticsServiceIntegrationTestSuite) TearDownSuite() {
	if suite.container != nil {
		if err := suite.container.Terminate(suite.ctx); err != nil {
			suite.T().Logf("failed to terminate container: %v", err)
		}
	}
}

func (suite *AnalyticsServiceIntegrationTestSuite) TearDownTest() {
	sqlDB, err := suite.db.DB()
	suite.Require().NoError(err)
	_, _ = sqlDB.Exec("DELETE FROM entity_tags")
	_, _ = sqlDB.Exec("DELETE FROM tag_votes")
	_, _ = sqlDB.Exec("DELETE FROM tag_aliases")
	_, _ = sqlDB.Exec("DELETE FROM tags")
	_, _ = sqlDB.Exec("DELETE FROM collection_items")
	_, _ = sqlDB.Exec("DELETE FROM collection_subscribers")
	_, _ = sqlDB.Exec("DELETE FROM collections")
	_, _ = sqlDB.Exec("DELETE FROM request_votes")
	_, _ = sqlDB.Exec("DELETE FROM requests")
	_, _ = sqlDB.Exec("DELETE FROM revisions")
	_, _ = sqlDB.Exec("DELETE FROM user_bookmarks")
	_, _ = sqlDB.Exec("DELETE FROM artist_releases")
	_, _ = sqlDB.Exec("DELETE FROM show_artists")
	_, _ = sqlDB.Exec("DELETE FROM show_venues")
	_, _ = sqlDB.Exec("DELETE FROM shows")
	_, _ = sqlDB.Exec("DELETE FROM artists")
	_, _ = sqlDB.Exec("DELETE FROM venues")
	_, _ = sqlDB.Exec("DELETE FROM releases")
	_, _ = sqlDB.Exec("DELETE FROM labels")
	_, _ = sqlDB.Exec("DELETE FROM users")
}

func TestAnalyticsServiceIntegrationTestSuite(t *testing.T) {
	suite.Run(t, new(AnalyticsServiceIntegrationTestSuite))
}

// =============================================================================
// HELPERS
// =============================================================================

func (suite *AnalyticsServiceIntegrationTestSuite) createUser(email, username string) *models.User {
	user := &models.User{
		Email:    &email,
		Username: &username,
	}
	err := suite.db.Create(user).Error
	suite.Require().NoError(err)
	return user
}

func (suite *AnalyticsServiceIntegrationTestSuite) createArtist(name string) *models.Artist {
	artist := &models.Artist{Name: name}
	err := suite.db.Create(artist).Error
	suite.Require().NoError(err)
	return artist
}

func (suite *AnalyticsServiceIntegrationTestSuite) createVenue(name, city, state string, verified bool) *models.Venue {
	venue := &models.Venue{
		Name:     name,
		City:     city,
		State:    state,
		Verified: verified,
	}
	err := suite.db.Create(venue).Error
	suite.Require().NoError(err)
	if !verified {
		suite.db.Exec("UPDATE venues SET verified = false WHERE id = ?", venue.ID)
	}
	return venue
}

func (suite *AnalyticsServiceIntegrationTestSuite) createShow(title string, status models.ShowStatus) *models.Show {
	show := &models.Show{
		Title:     title,
		EventDate: time.Now().Add(7 * 24 * time.Hour),
		Status:    status,
		Source:    models.ShowSourceUser,
	}
	err := suite.db.Create(show).Error
	suite.Require().NoError(err)
	return show
}

func (suite *AnalyticsServiceIntegrationTestSuite) createShowWithDate(title string, status models.ShowStatus, date time.Time) *models.Show {
	show := &models.Show{
		Title:     title,
		EventDate: date,
		Status:    status,
		Source:    models.ShowSourceUser,
	}
	err := suite.db.Create(show).Error
	suite.Require().NoError(err)
	return show
}

func (suite *AnalyticsServiceIntegrationTestSuite) createRelease(title string) *models.Release {
	release := &models.Release{Title: title, ReleaseType: models.ReleaseTypeLP}
	err := suite.db.Create(release).Error
	suite.Require().NoError(err)
	return release
}

func (suite *AnalyticsServiceIntegrationTestSuite) createLabel(name string) *models.Label {
	label := &models.Label{Name: name}
	err := suite.db.Create(label).Error
	suite.Require().NoError(err)
	return label
}

func (suite *AnalyticsServiceIntegrationTestSuite) createTag(name, category string) *models.Tag {
	tag := &models.Tag{Name: name, Category: category}
	err := suite.db.Create(tag).Error
	suite.Require().NoError(err)
	return tag
}

func (suite *AnalyticsServiceIntegrationTestSuite) createEntityTag(tagID, entityID, userID uint, entityType string) {
	et := &models.EntityTag{
		TagID:         tagID,
		EntityType:    entityType,
		EntityID:      entityID,
		AddedByUserID: userID,
	}
	err := suite.db.Create(et).Error
	suite.Require().NoError(err)
}

func (suite *AnalyticsServiceIntegrationTestSuite) createTagVote(tagID, entityID, userID uint, entityType string, vote int) {
	tv := &models.TagVote{
		TagID:      tagID,
		EntityType: entityType,
		EntityID:   entityID,
		UserID:     userID,
		Vote:       vote,
	}
	err := suite.db.Create(tv).Error
	suite.Require().NoError(err)
}

func (suite *AnalyticsServiceIntegrationTestSuite) createBookmark(userID uint, entityType models.BookmarkEntityType, entityID uint, action models.BookmarkAction) {
	bm := &models.UserBookmark{
		UserID:     userID,
		EntityType: entityType,
		EntityID:   entityID,
		Action:     action,
	}
	err := suite.db.Create(bm).Error
	suite.Require().NoError(err)
}

func (suite *AnalyticsServiceIntegrationTestSuite) createCollection(userID uint, title, slug string) *models.Collection {
	c := &models.Collection{
		Title:     title,
		Slug:      slug,
		CreatorID: userID,
		IsPublic:  true,
	}
	err := suite.db.Create(c).Error
	suite.Require().NoError(err)
	return c
}

func (suite *AnalyticsServiceIntegrationTestSuite) createCollectionItem(collectionID, userID uint, entityType string, entityID uint) {
	ci := &models.CollectionItem{
		CollectionID:  collectionID,
		EntityType:    entityType,
		EntityID:      entityID,
		AddedByUserID: userID,
	}
	err := suite.db.Create(ci).Error
	suite.Require().NoError(err)
}

func (suite *AnalyticsServiceIntegrationTestSuite) createRequest(userID uint, title string) *models.Request {
	r := &models.Request{
		RequesterID: userID,
		Title:       title,
		EntityType:  "artist",
		Status:      "pending",
	}
	err := suite.db.Create(r).Error
	suite.Require().NoError(err)
	return r
}

func (suite *AnalyticsServiceIntegrationTestSuite) createRequestVote(requestID, userID uint, vote int) {
	rv := &models.RequestVote{
		RequestID: requestID,
		UserID:    userID,
		Vote:      vote,
	}
	err := suite.db.Create(rv).Error
	suite.Require().NoError(err)
}

func (suite *AnalyticsServiceIntegrationTestSuite) createRevision(userID uint, entityType string, entityID uint) {
	summary := "test revision"
	changes := json.RawMessage(`[]`)
	r := &models.Revision{
		EntityType:   entityType,
		EntityID:     entityID,
		UserID:       userID,
		Summary:      &summary,
		FieldChanges: &changes,
	}
	err := suite.db.Create(r).Error
	suite.Require().NoError(err)
}

func (suite *AnalyticsServiceIntegrationTestSuite) linkShowVenue(showID, venueID uint) {
	sv := &models.ShowVenue{ShowID: showID, VenueID: venueID}
	err := suite.db.Create(sv).Error
	suite.Require().NoError(err)
}

func (suite *AnalyticsServiceIntegrationTestSuite) linkArtistRelease(artistID, releaseID uint) {
	ar := &models.ArtistRelease{ArtistID: artistID, ReleaseID: releaseID, Role: models.ArtistReleaseRoleMain}
	err := suite.db.Create(ar).Error
	suite.Require().NoError(err)
}

// =============================================================================
// TESTS: GetGrowthMetrics
// =============================================================================

func (suite *AnalyticsServiceIntegrationTestSuite) TestGetGrowthMetrics_Empty() {
	resp, err := suite.service.GetGrowthMetrics(6)
	suite.Require().NoError(err)
	suite.Len(resp.Shows, 6)
	suite.Len(resp.Artists, 6)
	suite.Len(resp.Venues, 6)
	suite.Len(resp.Releases, 6)
	suite.Len(resp.Labels, 6)
	suite.Len(resp.Users, 6)

	// All counts should be 0 in empty DB
	for _, s := range resp.Shows {
		suite.Equal(0, s.Count)
	}
}

func (suite *AnalyticsServiceIntegrationTestSuite) TestGetGrowthMetrics_WithData() {
	suite.createShow("Show 1", models.ShowStatusApproved)
	suite.createShow("Show 2", models.ShowStatusPending)
	suite.createArtist("Artist 1")
	suite.createVenue("Venue 1", "Phoenix", "AZ", true)
	suite.createRelease("Release 1")
	suite.createLabel("Label 1")
	suite.createUser("user1@test.com", "user1")

	resp, err := suite.service.GetGrowthMetrics(1)
	suite.Require().NoError(err)

	// Current month should have our entities
	suite.Len(resp.Shows, 1)
	suite.Equal(2, resp.Shows[0].Count) // both approved and pending counted
	suite.Equal(1, resp.Artists[0].Count)
	suite.Equal(1, resp.Venues[0].Count)
	suite.Equal(1, resp.Releases[0].Count)
	suite.Equal(1, resp.Labels[0].Count)
	suite.Equal(1, resp.Users[0].Count)
}

func (suite *AnalyticsServiceIntegrationTestSuite) TestGetGrowthMetrics_FillsMissingMonths() {
	// Create data only in the current month
	suite.createShow("Show 1", models.ShowStatusApproved)

	resp, err := suite.service.GetGrowthMetrics(3)
	suite.Require().NoError(err)
	suite.Len(resp.Shows, 3)

	// Last entry (current month) should have count 1
	suite.Equal(1, resp.Shows[2].Count)
	// Earlier months should have 0
	suite.Equal(0, resp.Shows[0].Count)
	suite.Equal(0, resp.Shows[1].Count)
}

func (suite *AnalyticsServiceIntegrationTestSuite) TestGetGrowthMetrics_ClampMonths() {
	resp, err := suite.service.GetGrowthMetrics(0)
	suite.Require().NoError(err)
	suite.Len(resp.Shows, 1) // clamped to 1

	resp2, err := suite.service.GetGrowthMetrics(100)
	suite.Require().NoError(err)
	suite.Len(resp2.Shows, 24) // clamped to 24
}

// =============================================================================
// TESTS: GetEngagementMetrics
// =============================================================================

func (suite *AnalyticsServiceIntegrationTestSuite) TestGetEngagementMetrics_Empty() {
	resp, err := suite.service.GetEngagementMetrics(6)
	suite.Require().NoError(err)
	suite.Len(resp.Bookmarks, 6)
	suite.Len(resp.TagsAdded, 6)
	suite.Len(resp.TagVotes, 6)
	suite.Len(resp.CollectionItems, 6)
	suite.Len(resp.Requests, 6)
	suite.Len(resp.RequestVotes, 6)
	suite.Len(resp.Revisions, 6)
	suite.Len(resp.Follows, 6)
	suite.Len(resp.Attendance, 6)

	// All should be 0
	for _, m := range resp.Bookmarks {
		suite.Equal(0, m.Count)
	}
}

func (suite *AnalyticsServiceIntegrationTestSuite) TestGetEngagementMetrics_WithData() {
	user := suite.createUser("engagement@test.com", "enguser")
	artist := suite.createArtist("Artist 1")
	tag := suite.createTag("rock", "genre")
	show := suite.createShow("Show 1", models.ShowStatusApproved)
	col := suite.createCollection(user.ID, "My Collection", "my-collection")

	// Create various engagement actions
	suite.createBookmark(user.ID, models.BookmarkEntityShow, show.ID, models.BookmarkActionSave)
	suite.createEntityTag(tag.ID, artist.ID, user.ID, "artist")
	suite.createTagVote(tag.ID, artist.ID, user.ID, "artist", 1)
	suite.createCollectionItem(col.ID, user.ID, "artist", artist.ID)
	req := suite.createRequest(user.ID, "Add this band")
	suite.createRequestVote(req.ID, user.ID, 1)
	suite.createRevision(user.ID, "artist", artist.ID)
	suite.createBookmark(user.ID, models.BookmarkEntityArtist, artist.ID, models.BookmarkActionFollow)
	suite.createBookmark(user.ID, models.BookmarkEntityShow, show.ID, models.BookmarkActionGoing)

	resp, err := suite.service.GetEngagementMetrics(1)
	suite.Require().NoError(err)

	suite.Equal(1, resp.Bookmarks[0].Count)
	suite.Equal(1, resp.TagsAdded[0].Count)
	suite.Equal(1, resp.TagVotes[0].Count)
	suite.Equal(1, resp.CollectionItems[0].Count)
	suite.Equal(1, resp.Requests[0].Count)
	suite.Equal(1, resp.RequestVotes[0].Count)
	suite.Equal(1, resp.Revisions[0].Count)
	suite.Equal(1, resp.Follows[0].Count)
	suite.Equal(1, resp.Attendance[0].Count)
}

// =============================================================================
// TESTS: GetCommunityHealth
// =============================================================================

func (suite *AnalyticsServiceIntegrationTestSuite) TestGetCommunityHealth_Empty() {
	resp, err := suite.service.GetCommunityHealth()
	suite.Require().NoError(err)
	suite.Equal(0, resp.ActiveContributors30d)
	suite.Equal(float64(0), resp.RequestFulfillmentRate)
	suite.Equal(0, resp.NewCollections30d)
	suite.Empty(resp.TopContributors)
}

func (suite *AnalyticsServiceIntegrationTestSuite) TestGetCommunityHealth_ActiveContributors() {
	user1 := suite.createUser("active1@test.com", "active1")
	user2 := suite.createUser("active2@test.com", "active2")
	artist := suite.createArtist("Band")
	tag := suite.createTag("indie", "genre")

	// User 1 adds a tag
	suite.createEntityTag(tag.ID, artist.ID, user1.ID, "artist")
	// User 2 creates a revision
	suite.createRevision(user2.ID, "artist", artist.ID)

	resp, err := suite.service.GetCommunityHealth()
	suite.Require().NoError(err)
	suite.Equal(2, resp.ActiveContributors30d)
}

func (suite *AnalyticsServiceIntegrationTestSuite) TestGetCommunityHealth_RequestFulfillmentRate() {
	user := suite.createUser("requser@test.com", "requser")

	// Create 4 requests: 2 fulfilled, 1 open, 1 canceled
	for i := 0; i < 2; i++ {
		r := suite.createRequest(user.ID, fmt.Sprintf("Fulfilled %d", i))
		suite.db.Model(r).Update("status", "fulfilled")
	}
	suite.createRequest(user.ID, "Open request")
	r := suite.createRequest(user.ID, "Canceled request")
	suite.db.Model(r).Update("status", "canceled")

	resp, err := suite.service.GetCommunityHealth()
	suite.Require().NoError(err)
	// 2 fulfilled / 3 non-canceled = 0.666...
	suite.InDelta(0.6667, resp.RequestFulfillmentRate, 0.01)
}

func (suite *AnalyticsServiceIntegrationTestSuite) TestGetCommunityHealth_TopContributors() {
	user1 := suite.createUser("top1@test.com", "topuser1")
	user2 := suite.createUser("top2@test.com", "topuser2")
	artist := suite.createArtist("Band")
	tag := suite.createTag("rock", "genre")

	// User 1: 3 contributions (2 tags + 1 revision)
	suite.createEntityTag(tag.ID, artist.ID, user1.ID, "artist")
	suite.createRevision(user1.ID, "artist", artist.ID)
	suite.createBookmark(user1.ID, models.BookmarkEntityArtist, artist.ID, models.BookmarkActionFollow)

	// User 2: 1 contribution
	suite.createTagVote(tag.ID, artist.ID, user2.ID, "artist", 1)

	resp, err := suite.service.GetCommunityHealth()
	suite.Require().NoError(err)
	suite.GreaterOrEqual(len(resp.TopContributors), 2)
	// User 1 should be first (more contributions)
	suite.Equal(user1.ID, resp.TopContributors[0].UserID)
	suite.Equal("topuser1", resp.TopContributors[0].Username)
	suite.Greater(resp.TopContributors[0].Count, resp.TopContributors[1].Count)
}

func (suite *AnalyticsServiceIntegrationTestSuite) TestGetCommunityHealth_NewCollections() {
	user := suite.createUser("coluser@test.com", "coluser")
	suite.createCollection(user.ID, "Collection A", "collection-a")
	suite.createCollection(user.ID, "Collection B", "collection-b")

	resp, err := suite.service.GetCommunityHealth()
	suite.Require().NoError(err)
	suite.Equal(2, resp.NewCollections30d)
}

// =============================================================================
// TESTS: GetDataQualityTrends
// =============================================================================

func (suite *AnalyticsServiceIntegrationTestSuite) TestGetDataQualityTrends_Empty() {
	resp, err := suite.service.GetDataQualityTrends(6)
	suite.Require().NoError(err)
	suite.Len(resp.ShowsApproved, 6)
	suite.Len(resp.ShowsRejected, 6)
	suite.Equal(0, resp.PendingReviewCount)
	suite.Equal(0, resp.ArtistsWithoutReleases)
	suite.Equal(0, resp.InactiveVenues90d)
}

func (suite *AnalyticsServiceIntegrationTestSuite) TestGetDataQualityTrends_ShowCounts() {
	suite.createShow("Approved 1", models.ShowStatusApproved)
	suite.createShow("Approved 2", models.ShowStatusApproved)
	suite.createShow("Rejected 1", models.ShowStatusRejected)
	suite.createShow("Pending 1", models.ShowStatusPending)
	suite.createShow("Pending 2", models.ShowStatusPending)

	resp, err := suite.service.GetDataQualityTrends(1)
	suite.Require().NoError(err)
	suite.Equal(2, resp.ShowsApproved[0].Count)
	suite.Equal(1, resp.ShowsRejected[0].Count)
	suite.Equal(2, resp.PendingReviewCount)
}

func (suite *AnalyticsServiceIntegrationTestSuite) TestGetDataQualityTrends_ArtistsWithoutReleases() {
	// Artist with no release
	suite.createArtist("No Release Band")

	// Artist with a release (should NOT be counted)
	artist := suite.createArtist("Has Release Band")
	release := suite.createRelease("Some Album")
	suite.linkArtistRelease(artist.ID, release.ID)

	resp, err := suite.service.GetDataQualityTrends(1)
	suite.Require().NoError(err)
	suite.Equal(1, resp.ArtistsWithoutReleases)
}

func (suite *AnalyticsServiceIntegrationTestSuite) TestGetDataQualityTrends_InactiveVenues() {
	// Verified venue with no recent shows
	suite.createVenue("Ghost Venue", "Phoenix", "AZ", true)

	// Verified venue with recent show (should NOT be counted)
	activeVenue := suite.createVenue("Active Venue", "Phoenix", "AZ", true)
	show := suite.createShowWithDate("Recent Show", models.ShowStatusApproved, time.Now().Add(-30*24*time.Hour))
	suite.linkShowVenue(show.ID, activeVenue.ID)

	// Unverified venue with no shows (should NOT be counted — only verified)
	suite.createVenue("Unverified Venue", "Tucson", "AZ", false)

	resp, err := suite.service.GetDataQualityTrends(1)
	suite.Require().NoError(err)
	suite.Equal(1, resp.InactiveVenues90d)
}
