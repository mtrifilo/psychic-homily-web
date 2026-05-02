package user

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"

	adminm "psychic-homily-backend/internal/models/admin"
	authm "psychic-homily-backend/internal/models/auth"
	catalogm "psychic-homily-backend/internal/models/catalog"
	communitym "psychic-homily-backend/internal/models/community"
	"psychic-homily-backend/internal/testutil"
)

// =============================================================================
// UNIT TESTS (No Database Required)
// =============================================================================

func TestLeaderboardService_NilDB(t *testing.T) {
	svc := &LeaderboardService{db: nil}

	t.Run("GetLeaderboard", func(t *testing.T) {
		_, err := svc.GetLeaderboard("overall", "all_time", 25)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "database not initialized")
	})

	t.Run("GetUserRank", func(t *testing.T) {
		_, err := svc.GetUserRank(1, "overall", "all_time")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "database not initialized")
	})
}

func TestLeaderboardService_ValidDimensions(t *testing.T) {
	svc := &LeaderboardService{db: nil} // nil db, validation happens first

	for _, dim := range []string{"overall", "shows", "venues", "tags", "edits", "requests"} {
		t.Run(dim, func(t *testing.T) {
			assert.True(t, validDimensions[dim], "dimension should be valid: %s", dim)
		})
	}

	t.Run("invalid_dimension", func(t *testing.T) {
		_, err := svc.GetLeaderboard("invalid", "all_time", 25)
		// With nil db, validation returns before db access
		assert.Error(t, err)
	})
}

func TestLeaderboardService_ValidPeriods(t *testing.T) {
	for _, p := range []string{"all_time", "month", "week"} {
		t.Run(p, func(t *testing.T) {
			assert.True(t, validPeriods[p], "period should be valid: %s", p)
		})
	}
}

func TestBuildPeriodFilter(t *testing.T) {
	t.Run("all_time", func(t *testing.T) {
		assert.Equal(t, "", buildPeriodFilter("all_time"))
	})
	t.Run("month", func(t *testing.T) {
		assert.Contains(t, buildPeriodFilter("month"), "date_trunc('month'")
	})
	t.Run("week", func(t *testing.T) {
		assert.Contains(t, buildPeriodFilter("week"), "date_trunc('week'")
	})
}

// =============================================================================
// INTEGRATION TESTS (With Real Database)
// =============================================================================

type LeaderboardServiceIntegrationTestSuite struct {
	suite.Suite
	testDB             *testutil.TestDatabase
	db                 *gorm.DB
	leaderboardService *LeaderboardService
}

func (suite *LeaderboardServiceIntegrationTestSuite) SetupSuite() {
	suite.testDB = testutil.SetupTestPostgres(suite.T())
	suite.db = suite.testDB.DB
	suite.leaderboardService = &LeaderboardService{db: suite.testDB.DB}
}

func (suite *LeaderboardServiceIntegrationTestSuite) TearDownSuite() {
	suite.testDB.Cleanup()
}

func (suite *LeaderboardServiceIntegrationTestSuite) TearDownTest() {
	sqlDB, err := suite.db.DB()
	suite.Require().NoError(err)
	_, _ = sqlDB.Exec("DELETE FROM pending_entity_edits")
	_, _ = sqlDB.Exec("DELETE FROM revisions")
	_, _ = sqlDB.Exec("DELETE FROM entity_tags")
	_, _ = sqlDB.Exec("DELETE FROM tags")
	_, _ = sqlDB.Exec("DELETE FROM requests")
	_, _ = sqlDB.Exec("DELETE FROM show_artists")
	_, _ = sqlDB.Exec("DELETE FROM show_venues")
	_, _ = sqlDB.Exec("DELETE FROM shows")
	_, _ = sqlDB.Exec("DELETE FROM venues")
	_, _ = sqlDB.Exec("DELETE FROM users")
}

func TestLeaderboardServiceIntegrationTestSuite(t *testing.T) {
	suite.Run(t, new(LeaderboardServiceIntegrationTestSuite))
}

// =============================================================================
// HELPERS
// =============================================================================

func (suite *LeaderboardServiceIntegrationTestSuite) createTestUser(username string) *authm.User {
	user := &authm.User{
		Email:             stringPtr(fmt.Sprintf("%s-%d@test.com", username, time.Now().UnixNano())),
		Username:          stringPtr(username),
		ProfileVisibility: "public",
		IsActive:          true,
		UserTier:          "contributor",
	}
	err := suite.db.Create(user).Error
	suite.Require().NoError(err)
	return user
}

func (suite *LeaderboardServiceIntegrationTestSuite) createUserWithHiddenContributions(username string) *authm.User {
	privJSON := `{"contributions":"hidden","saved_shows":"hidden","attendance":"hidden","following":"count_only","collections":"visible","last_active":"visible","profile_sections":"visible"}`
	raw := json.RawMessage(privJSON)
	user := &authm.User{
		Email:             stringPtr(fmt.Sprintf("%s-%d@test.com", username, time.Now().UnixNano())),
		Username:          stringPtr(username),
		ProfileVisibility: "public",
		IsActive:          true,
		UserTier:          "contributor",
		PrivacySettings:   &raw,
	}
	err := suite.db.Create(user).Error
	suite.Require().NoError(err)
	return user
}

func (suite *LeaderboardServiceIntegrationTestSuite) createShow(submittedBy uint, title string) {
	show := &catalogm.Show{
		Title:       title,
		SubmittedBy: &submittedBy,
		Status:      "approved",
		EventDate:   time.Now(),
	}
	err := suite.db.Create(show).Error
	suite.Require().NoError(err)
}

func (suite *LeaderboardServiceIntegrationTestSuite) createVenue(submittedBy uint, name string) {
	slug := fmt.Sprintf("%s-%d", name, time.Now().UnixNano())
	venue := &catalogm.Venue{
		Name:        name,
		SubmittedBy: &submittedBy,
		City:        "Phoenix",
		State:       "AZ",
		Slug:        &slug,
	}
	err := suite.db.Create(venue).Error
	suite.Require().NoError(err)
}

func (suite *LeaderboardServiceIntegrationTestSuite) createTag(addedByUserID uint) {
	// Create the tag first
	tag := &catalogm.Tag{
		Name:     fmt.Sprintf("tag-%d", time.Now().UnixNano()),
		Slug:     fmt.Sprintf("tag-%d", time.Now().UnixNano()),
		Category: "genre",
	}
	err := suite.db.Create(tag).Error
	suite.Require().NoError(err)

	// Create the entity tag
	entityTag := &catalogm.EntityTag{
		TagID:         tag.ID,
		EntityType:    "artist",
		EntityID:      1, // doesn't need to exist for count
		AddedByUserID: addedByUserID,
	}
	err = suite.db.Create(entityTag).Error
	suite.Require().NoError(err)
}

func (suite *LeaderboardServiceIntegrationTestSuite) createRevision(userID uint) {
	rev := &adminm.Revision{
		EntityType: "artist",
		EntityID:   1,
		UserID:     userID,
		FieldChanges: func() *json.RawMessage {
			raw := json.RawMessage(`[{"field":"name","old_value":"old","new_value":"new"}]`)
			return &raw
		}(),
	}
	err := suite.db.Create(rev).Error
	suite.Require().NoError(err)
}

// =============================================================================
// TESTS
// =============================================================================

func (suite *LeaderboardServiceIntegrationTestSuite) TestGetLeaderboard_ReturnsRankedUsers() {
	alice := suite.createTestUser("alice")
	bob := suite.createTestUser("bob")

	// Alice submits 3 shows, Bob submits 1
	suite.createShow(alice.ID, "Show 1")
	suite.createShow(alice.ID, "Show 2")
	suite.createShow(alice.ID, "Show 3")
	suite.createShow(bob.ID, "Show 4")

	entries, err := suite.leaderboardService.GetLeaderboard("shows", "all_time", 25)
	suite.Require().NoError(err)
	suite.Require().Len(entries, 2)

	// Alice should be rank 1
	suite.Equal(1, entries[0].Rank)
	suite.Equal("alice", entries[0].Username)
	suite.Equal(int64(3), entries[0].Count)

	// Bob should be rank 2
	suite.Equal(2, entries[1].Rank)
	suite.Equal("bob", entries[1].Username)
	suite.Equal(int64(1), entries[1].Count)
}

func (suite *LeaderboardServiceIntegrationTestSuite) TestGetLeaderboard_VenueDimension() {
	alice := suite.createTestUser("alice-venues")

	suite.createVenue(alice.ID, "Venue A")
	suite.createVenue(alice.ID, "Venue B")

	entries, err := suite.leaderboardService.GetLeaderboard("venues", "all_time", 25)
	suite.Require().NoError(err)
	suite.Require().Len(entries, 1)
	suite.Equal("alice-venues", entries[0].Username)
	suite.Equal(int64(2), entries[0].Count)
}

func (suite *LeaderboardServiceIntegrationTestSuite) TestGetLeaderboard_TagsDimension() {
	alice := suite.createTestUser("alice-tags")

	suite.createTag(alice.ID)
	suite.createTag(alice.ID)

	entries, err := suite.leaderboardService.GetLeaderboard("tags", "all_time", 25)
	suite.Require().NoError(err)
	suite.Require().Len(entries, 1)
	suite.Equal("alice-tags", entries[0].Username)
	suite.Equal(int64(2), entries[0].Count)
}

func (suite *LeaderboardServiceIntegrationTestSuite) TestGetLeaderboard_EditsDimension() {
	alice := suite.createTestUser("alice-edits")

	suite.createRevision(alice.ID)
	suite.createRevision(alice.ID)
	suite.createRevision(alice.ID)

	entries, err := suite.leaderboardService.GetLeaderboard("edits", "all_time", 25)
	suite.Require().NoError(err)
	suite.Require().Len(entries, 1)
	suite.Equal("alice-edits", entries[0].Username)
	suite.Equal(int64(3), entries[0].Count)
}

func (suite *LeaderboardServiceIntegrationTestSuite) TestGetLeaderboard_RequestsDimension() {
	alice := suite.createTestUser("alice-requests")

	// Create a fulfilled request
	desc := "Looking for setlist"
	req := &communitym.Request{
		Title:       "Find setlist",
		Description: &desc,
		EntityType:  "show",
		Status:      "fulfilled",
		RequesterID: alice.ID,
		FulfillerID: &alice.ID,
	}
	err := suite.db.Create(req).Error
	suite.Require().NoError(err)

	entries, err := suite.leaderboardService.GetLeaderboard("requests", "all_time", 25)
	suite.Require().NoError(err)
	suite.Require().Len(entries, 1)
	suite.Equal("alice-requests", entries[0].Username)
	suite.Equal(int64(1), entries[0].Count)
}

func (suite *LeaderboardServiceIntegrationTestSuite) TestGetLeaderboard_OverallDimension() {
	alice := suite.createTestUser("alice-overall")

	// Alice submits shows and venues
	suite.createShow(alice.ID, "Show Overall")
	suite.createVenue(alice.ID, "Venue Overall")

	entries, err := suite.leaderboardService.GetLeaderboard("overall", "all_time", 25)
	suite.Require().NoError(err)
	suite.Require().Len(entries, 1)

	// Expected: 1 show * 25 + 1 venue * 15 = 40
	suite.Equal(int64(40), entries[0].Count)
	suite.Equal("alice-overall", entries[0].Username)
}

func (suite *LeaderboardServiceIntegrationTestSuite) TestGetLeaderboard_PeriodFiltering() {
	alice := suite.createTestUser("alice-period")

	// Create a show right now (should appear in week/month)
	suite.createShow(alice.ID, "Recent Show")

	// Week period
	entries, err := suite.leaderboardService.GetLeaderboard("shows", "week", 25)
	suite.Require().NoError(err)
	suite.Require().Len(entries, 1)
	suite.Equal("alice-period", entries[0].Username)

	// Month period
	entries, err = suite.leaderboardService.GetLeaderboard("shows", "month", 25)
	suite.Require().NoError(err)
	suite.Require().Len(entries, 1)
	suite.Equal("alice-period", entries[0].Username)
}

func (suite *LeaderboardServiceIntegrationTestSuite) TestGetLeaderboard_PrivacyGating() {
	visible := suite.createTestUser("visible-user")
	hidden := suite.createUserWithHiddenContributions("hidden-user")

	suite.createShow(visible.ID, "Visible Show")
	suite.createShow(hidden.ID, "Hidden Show")

	entries, err := suite.leaderboardService.GetLeaderboard("shows", "all_time", 25)
	suite.Require().NoError(err)

	// Only the visible user should appear
	suite.Require().Len(entries, 1)
	suite.Equal("visible-user", entries[0].Username)
}

func (suite *LeaderboardServiceIntegrationTestSuite) TestGetLeaderboard_DefaultLimit() {
	// Should not error with default limit
	entries, err := suite.leaderboardService.GetLeaderboard("overall", "all_time", 0)
	suite.Require().NoError(err)
	suite.NotNil(entries)
	suite.Len(entries, 0) // empty database
}

func (suite *LeaderboardServiceIntegrationTestSuite) TestGetLeaderboard_EmptyReturnsEmptyArray() {
	entries, err := suite.leaderboardService.GetLeaderboard("shows", "all_time", 25)
	suite.Require().NoError(err)
	suite.NotNil(entries)
	suite.Len(entries, 0)
}

func (suite *LeaderboardServiceIntegrationTestSuite) TestGetLeaderboard_InvalidDimension() {
	_, err := suite.leaderboardService.GetLeaderboard("invalid", "all_time", 25)
	suite.Error(err)
	suite.Contains(err.Error(), "invalid dimension")
}

func (suite *LeaderboardServiceIntegrationTestSuite) TestGetLeaderboard_InvalidPeriod() {
	_, err := suite.leaderboardService.GetLeaderboard("overall", "invalid", 25)
	suite.Error(err)
	suite.Contains(err.Error(), "invalid period")
}

func (suite *LeaderboardServiceIntegrationTestSuite) TestGetUserRank() {
	alice := suite.createTestUser("alice-rank")
	bob := suite.createTestUser("bob-rank")

	// Alice has more shows than Bob
	suite.createShow(alice.ID, "Alice Show 1")
	suite.createShow(alice.ID, "Alice Show 2")
	suite.createShow(bob.ID, "Bob Show 1")

	// Alice should be rank 1
	rank, err := suite.leaderboardService.GetUserRank(alice.ID, "shows", "all_time")
	suite.Require().NoError(err)
	suite.Require().NotNil(rank)
	suite.Equal(1, *rank)

	// Bob should be rank 2
	rank, err = suite.leaderboardService.GetUserRank(bob.ID, "shows", "all_time")
	suite.Require().NoError(err)
	suite.Require().NotNil(rank)
	suite.Equal(2, *rank)
}

func (suite *LeaderboardServiceIntegrationTestSuite) TestGetUserRank_NotOnLeaderboard() {
	alice := suite.createTestUser("alice-norank")

	// Alice has no contributions
	rank, err := suite.leaderboardService.GetUserRank(alice.ID, "shows", "all_time")
	suite.Require().NoError(err)
	suite.Nil(rank)
}

func (suite *LeaderboardServiceIntegrationTestSuite) TestGetLeaderboard_ExcludesInactiveUsers() {
	active := suite.createTestUser("active-user")
	inactive := suite.createTestUser("inactive-user")

	suite.createShow(active.ID, "Active Show")
	suite.createShow(inactive.ID, "Inactive Show")

	// Deactivate user
	suite.db.Model(inactive).Update("is_active", false)

	entries, err := suite.leaderboardService.GetLeaderboard("shows", "all_time", 25)
	suite.Require().NoError(err)
	suite.Require().Len(entries, 1)
	suite.Equal("active-user", entries[0].Username)
}

func (suite *LeaderboardServiceIntegrationTestSuite) TestGetLeaderboard_LimitClamped() {
	alice := suite.createTestUser("alice-limit")
	suite.createShow(alice.ID, "Show Limit")

	// Request limit over max (100) — should be clamped
	entries, err := suite.leaderboardService.GetLeaderboard("shows", "all_time", 200)
	suite.Require().NoError(err)
	suite.Require().Len(entries, 1) // Only one user in the db
}
