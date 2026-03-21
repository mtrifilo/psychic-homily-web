package admin

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"

	"psychic-homily-backend/internal/models"
	"psychic-homily-backend/internal/testutil"
)

// =============================================================================
// UNIT TESTS (No Database Required)
// =============================================================================

func TestNewAdminStatsService(t *testing.T) {
	t.Run("NilDB", func(t *testing.T) {
		svc := NewAdminStatsService(nil)
		assert.NotNil(t, svc)
	})

	t.Run("ExplicitDB", func(t *testing.T) {
		db := &gorm.DB{}
		svc := NewAdminStatsService(db)
		assert.NotNil(t, svc)
	})
}

func TestAdminStatsService_NilDB(t *testing.T) {
	svc := &AdminStatsService{db: nil}
	assert.Panics(t, func() {
		svc.GetDashboardStats()
	})
}

func TestAdminStatsService_NilDB_GetRecentActivity(t *testing.T) {
	svc := &AdminStatsService{db: nil}
	assert.Panics(t, func() {
		svc.GetRecentActivity()
	})
}

// =============================================================================
// INTEGRATION TESTS (With Real Database)
// =============================================================================

type AdminStatsServiceIntegrationTestSuite struct {
	suite.Suite
	testDB  *testutil.TestDatabase
	db      *gorm.DB
	service *AdminStatsService
}

func (suite *AdminStatsServiceIntegrationTestSuite) SetupSuite() {
	suite.testDB = testutil.SetupTestPostgres(suite.T())
	suite.db = suite.testDB.DB

	suite.service = &AdminStatsService{db: suite.testDB.DB}
}

func (suite *AdminStatsServiceIntegrationTestSuite) TearDownSuite() {
	suite.testDB.Cleanup()
}

func (suite *AdminStatsServiceIntegrationTestSuite) TearDownTest() {
	sqlDB, err := suite.db.DB()
	suite.Require().NoError(err)
	_, _ = sqlDB.Exec("DELETE FROM audit_logs")
	_, _ = sqlDB.Exec("DELETE FROM artist_reports")
	_, _ = sqlDB.Exec("DELETE FROM show_reports")
	_, _ = sqlDB.Exec("DELETE FROM pending_venue_edits")
	_, _ = sqlDB.Exec("DELETE FROM show_artists")
	_, _ = sqlDB.Exec("DELETE FROM show_venues")
	_, _ = sqlDB.Exec("DELETE FROM shows")
	_, _ = sqlDB.Exec("DELETE FROM artists")
	_, _ = sqlDB.Exec("DELETE FROM venues")
	_, _ = sqlDB.Exec("DELETE FROM users")
}

func TestAdminStatsServiceIntegrationTestSuite(t *testing.T) {
	suite.Run(t, new(AdminStatsServiceIntegrationTestSuite))
}

// =============================================================================
// HELPERS
// =============================================================================

func (suite *AdminStatsServiceIntegrationTestSuite) createUser(email string) *models.User {
	user := &models.User{
		Email:         &email,
		IsActive:      true,
		EmailVerified: true,
	}
	err := suite.db.Create(user).Error
	suite.Require().NoError(err)
	return user
}

func (suite *AdminStatsServiceIntegrationTestSuite) createVenue(name, city, state string, verified bool) *models.Venue {
	venue := &models.Venue{
		Name:     name,
		City:     city,
		State:    state,
		Verified: verified,
	}
	err := suite.db.Create(venue).Error
	suite.Require().NoError(err)
	return venue
}

func (suite *AdminStatsServiceIntegrationTestSuite) createArtist(name string) *models.Artist {
	artist := &models.Artist{Name: name}
	err := suite.db.Create(artist).Error
	suite.Require().NoError(err)
	return artist
}

func (suite *AdminStatsServiceIntegrationTestSuite) createShow(title string, status models.ShowStatus) *models.Show {
	show := &models.Show{
		Title:     title,
		EventDate: time.Now().Add(24 * time.Hour),
		Status:    status,
		Source:    models.ShowSourceUser,
	}
	err := suite.db.Create(show).Error
	suite.Require().NoError(err)
	return show
}

func (suite *AdminStatsServiceIntegrationTestSuite) createShowWithTime(title string, status models.ShowStatus, createdAt time.Time) *models.Show {
	show := &models.Show{
		Title:     title,
		EventDate: time.Now().Add(24 * time.Hour),
		Status:    status,
		Source:    models.ShowSourceUser,
	}
	err := suite.db.Create(show).Error
	suite.Require().NoError(err)
	// Update created_at with raw SQL to bypass GORM auto-update
	suite.db.Exec("UPDATE shows SET created_at = ? WHERE id = ?", createdAt, show.ID)
	return show
}

func (suite *AdminStatsServiceIntegrationTestSuite) createUserWithTime(email string, createdAt time.Time) *models.User {
	user := &models.User{
		Email:         &email,
		IsActive:      true,
		EmailVerified: true,
	}
	err := suite.db.Create(user).Error
	suite.Require().NoError(err)
	suite.db.Exec("UPDATE users SET created_at = ? WHERE id = ?", createdAt, user.ID)
	return user
}

func (suite *AdminStatsServiceIntegrationTestSuite) createVenueWithTime(name, city, state string, verified bool, createdAt time.Time) *models.Venue {
	venue := &models.Venue{
		Name:     name,
		City:     city,
		State:    state,
		Verified: verified,
	}
	err := suite.db.Create(venue).Error
	suite.Require().NoError(err)
	suite.db.Exec("UPDATE venues SET created_at = ? WHERE id = ?", createdAt, venue.ID)
	return venue
}

func (suite *AdminStatsServiceIntegrationTestSuite) createArtistWithTime(name string, createdAt time.Time) *models.Artist {
	artist := &models.Artist{Name: name}
	err := suite.db.Create(artist).Error
	suite.Require().NoError(err)
	suite.db.Exec("UPDATE artists SET created_at = ? WHERE id = ?", createdAt, artist.ID)
	return artist
}

// =============================================================================
// TESTS
// =============================================================================

func (suite *AdminStatsServiceIntegrationTestSuite) TestGetDashboardStats_Empty() {
	stats, err := suite.service.GetDashboardStats()
	suite.Require().NoError(err)
	suite.Equal(int64(0), stats.PendingShows)
	suite.Equal(int64(0), stats.PendingVenueEdits)
	suite.Equal(int64(0), stats.PendingReports)
	suite.Equal(int64(0), stats.UnverifiedVenues)
	suite.Equal(int64(0), stats.TotalShows)
	suite.Equal(int64(0), stats.TotalVenues)
	suite.Equal(int64(0), stats.TotalArtists)
	suite.Equal(int64(0), stats.TotalUsers)
	suite.Equal(int64(0), stats.ShowsSubmittedLast7Days)
	suite.Equal(int64(0), stats.UsersRegisteredLast7Days)
}

func (suite *AdminStatsServiceIntegrationTestSuite) TestGetDashboardStats_PendingShows() {
	suite.createShow("Pending Show 1", models.ShowStatusPending)
	suite.createShow("Pending Show 2", models.ShowStatusPending)
	suite.createShow("Approved Show", models.ShowStatusApproved)

	stats, err := suite.service.GetDashboardStats()
	suite.Require().NoError(err)
	suite.Equal(int64(2), stats.PendingShows)
}

func (suite *AdminStatsServiceIntegrationTestSuite) TestGetDashboardStats_PendingVenueEdits() {
	user := suite.createUser("user@test.com")
	venue := suite.createVenue("Test Venue", "NYC", "NY", true)

	edit := &models.PendingVenueEdit{
		VenueID:     venue.ID,
		SubmittedBy: user.ID,
		Status:      models.VenueEditStatusPending,
	}
	err := suite.db.Create(edit).Error
	suite.Require().NoError(err)

	stats, err := suite.service.GetDashboardStats()
	suite.Require().NoError(err)
	suite.Equal(int64(1), stats.PendingVenueEdits)
}

func (suite *AdminStatsServiceIntegrationTestSuite) TestGetDashboardStats_PendingReports() {
	user := suite.createUser("user@test.com")
	show := suite.createShow("Show", models.ShowStatusApproved)

	// Create 2 pending reports
	for i := 0; i < 2; i++ {
		reporter := suite.createUser(fmt.Sprintf("reporter%d@test.com", i))
		sqlDB, _ := suite.db.DB()
		_, err := sqlDB.Exec(
			"INSERT INTO show_reports (show_id, reported_by, report_type, status) VALUES ($1, $2, $3, $4)",
			show.ID, reporter.ID, "cancelled", "pending",
		)
		suite.Require().NoError(err)
	}
	// Dismissed report — should not count
	sqlDB, _ := suite.db.DB()
	_, err := sqlDB.Exec(
		"INSERT INTO show_reports (show_id, reported_by, report_type, status) VALUES ($1, $2, $3, $4)",
		show.ID, user.ID, "inaccurate", "dismissed",
	)
	suite.Require().NoError(err)

	stats, err := suite.service.GetDashboardStats()
	suite.Require().NoError(err)
	suite.Equal(int64(2), stats.PendingReports)
}

func (suite *AdminStatsServiceIntegrationTestSuite) TestGetDashboardStats_UnverifiedVenues() {
	suite.createVenue("Verified 1", "NYC", "NY", true)
	suite.createVenue("Unverified 1", "LA", "CA", false)
	suite.createVenue("Unverified 2", "CHI", "IL", false)

	stats, err := suite.service.GetDashboardStats()
	suite.Require().NoError(err)
	suite.Equal(int64(2), stats.UnverifiedVenues)
}

func (suite *AdminStatsServiceIntegrationTestSuite) TestGetDashboardStats_TotalCounts() {
	suite.createShow("Approved 1", models.ShowStatusApproved)
	suite.createShow("Approved 2", models.ShowStatusApproved)
	suite.createShow("Approved 3", models.ShowStatusApproved)
	suite.createShow("Pending", models.ShowStatusPending) // Should NOT count as TotalShows

	suite.createVenue("Verified 1", "NYC", "NY", true)
	suite.createVenue("Verified 2", "LA", "CA", true)
	suite.createVenue("Unverified", "CHI", "IL", false) // Should NOT count as TotalVenues

	suite.createArtist("Artist 1")
	suite.createArtist("Artist 2")
	suite.createArtist("Artist 3")
	suite.createArtist("Artist 4")

	stats, err := suite.service.GetDashboardStats()
	suite.Require().NoError(err)
	suite.Equal(int64(3), stats.TotalShows)
	suite.Equal(int64(2), stats.TotalVenues)
	suite.Equal(int64(4), stats.TotalArtists)
}

func (suite *AdminStatsServiceIntegrationTestSuite) TestGetDashboardStats_TotalUsers() {
	suite.createUser("user1@test.com")
	suite.createUser("user2@test.com")
	suite.createUser("user3@test.com")

	stats, err := suite.service.GetDashboardStats()
	suite.Require().NoError(err)
	suite.Equal(int64(3), stats.TotalUsers)
}

func (suite *AdminStatsServiceIntegrationTestSuite) TestGetDashboardStats_RecentActivity() {
	// Recent shows (within 7 days)
	suite.createShow("Recent Show", models.ShowStatusPending)
	// Old show (10 days ago)
	suite.createShowWithTime("Old Show", models.ShowStatusPending, time.Now().AddDate(0, 0, -10))

	// Recent users
	suite.createUser("recent1@test.com")
	suite.createUser("recent2@test.com")
	// Old user
	suite.createUserWithTime("old@test.com", time.Now().AddDate(0, 0, -10))

	stats, err := suite.service.GetDashboardStats()
	suite.Require().NoError(err)
	suite.Equal(int64(1), stats.ShowsSubmittedLast7Days)
	suite.Equal(int64(2), stats.UsersRegisteredLast7Days)
}

// =============================================================================
// GetRecentActivity TESTS
// =============================================================================

func (suite *AdminStatsServiceIntegrationTestSuite) TestGetRecentActivity_Empty() {
	feed, err := suite.service.GetRecentActivity()
	suite.Require().NoError(err)
	suite.NotNil(feed)
	suite.Len(feed.Events, 0)
}

func (suite *AdminStatsServiceIntegrationTestSuite) TestGetRecentActivity_BasicEvent() {
	user := suite.createUser("admin@test.com")
	suite.createAuditLog(user.ID, "approve_show", "show", 1)

	feed, err := suite.service.GetRecentActivity()
	suite.Require().NoError(err)
	suite.Require().Len(feed.Events, 1)

	event := feed.Events[0]
	suite.Equal("show_approved", event.EventType)
	suite.Contains(event.Description, "Show #1")
	suite.Contains(event.Description, "approved")
	suite.Equal("show", event.EntityType)
	suite.NotEmpty(event.ActorName)
}

func (suite *AdminStatsServiceIntegrationTestSuite) TestGetRecentActivity_WithSlugResolution() {
	user := suite.createUser("admin@test.com")
	slug := "test-slug"
	artist := &models.Artist{Name: "Test Artist", Slug: &slug}
	err := suite.db.Create(artist).Error
	suite.Require().NoError(err)

	suite.createAuditLog(user.ID, "edit_artist", "artist", artist.ID)

	feed, err := suite.service.GetRecentActivity()
	suite.Require().NoError(err)
	suite.Require().Len(feed.Events, 1)

	event := feed.Events[0]
	suite.Equal("artist_edited", event.EventType)
	suite.Equal("artist", event.EntityType)
	suite.Equal("test-slug", event.EntitySlug)
}

func (suite *AdminStatsServiceIntegrationTestSuite) TestGetRecentActivity_OrderedByRecent() {
	user := suite.createUser("admin@test.com")

	// Create events with different timestamps
	suite.createAuditLogWithTime(user.ID, "approve_show", "show", 1, time.Now().Add(-2*time.Hour))
	suite.createAuditLogWithTime(user.ID, "verify_venue", "venue", 1, time.Now().Add(-1*time.Hour))
	suite.createAuditLogWithTime(user.ID, "create_artist", "artist", 1, time.Now())

	feed, err := suite.service.GetRecentActivity()
	suite.Require().NoError(err)
	suite.Require().Len(feed.Events, 3)

	// Most recent first
	suite.Equal("artist_created", feed.Events[0].EventType)
	suite.Equal("venue_verified", feed.Events[1].EventType)
	suite.Equal("show_approved", feed.Events[2].EventType)
}

func (suite *AdminStatsServiceIntegrationTestSuite) TestGetRecentActivity_Limit20() {
	user := suite.createUser("admin@test.com")

	// Create 25 events
	for i := 0; i < 25; i++ {
		suite.createAuditLog(user.ID, "approve_show", "show", uint(i+1))
	}

	feed, err := suite.service.GetRecentActivity()
	suite.Require().NoError(err)
	suite.Len(feed.Events, 20) // Should be capped at 20
}

func (suite *AdminStatsServiceIntegrationTestSuite) TestGetRecentActivity_ActorNameResolution() {
	firstName := "Jane"
	lastName := "Doe"
	email := "jane@test.com"
	user := &models.User{
		Email:     &email,
		FirstName: &firstName,
		LastName:  &lastName,
		IsActive:  true,
	}
	err := suite.db.Create(user).Error
	suite.Require().NoError(err)

	suite.createAuditLog(user.ID, "approve_show", "show", 1)

	feed, err := suite.service.GetRecentActivity()
	suite.Require().NoError(err)
	suite.Require().Len(feed.Events, 1)
	suite.Equal("Jane Doe", feed.Events[0].ActorName)
}

func (suite *AdminStatsServiceIntegrationTestSuite) TestGetRecentActivity_ActorNameFallbackToUsername() {
	email := "user@test.com"
	username := "cooluser"
	user := &models.User{
		Email:    &email,
		Username: &username,
		IsActive: true,
	}
	err := suite.db.Create(user).Error
	suite.Require().NoError(err)

	suite.createAuditLog(user.ID, "verify_venue", "venue", 1)

	feed, err := suite.service.GetRecentActivity()
	suite.Require().NoError(err)
	suite.Require().Len(feed.Events, 1)
	suite.Equal("cooluser", feed.Events[0].ActorName)
}

func (suite *AdminStatsServiceIntegrationTestSuite) TestGetRecentActivity_VenueEditSlugResolution() {
	user := suite.createUser("admin@test.com")
	slug := "test-venue"
	venue := &models.Venue{Name: "Test Venue", City: "NYC", State: "NY", Slug: &slug}
	err := suite.db.Create(venue).Error
	suite.Require().NoError(err)

	edit := &models.PendingVenueEdit{
		VenueID:     venue.ID,
		SubmittedBy: user.ID,
		Status:      models.VenueEditStatusPending,
	}
	err = suite.db.Create(edit).Error
	suite.Require().NoError(err)

	suite.createAuditLog(user.ID, "approve_venue_edit", "venue_edit", edit.ID)

	feed, err := suite.service.GetRecentActivity()
	suite.Require().NoError(err)
	suite.Require().Len(feed.Events, 1)

	event := feed.Events[0]
	suite.Equal("venue_edit_approved", event.EventType)
	suite.Equal("venue", event.EntityType)
	suite.Equal("test-venue", event.EntitySlug)
}

func (suite *AdminStatsServiceIntegrationTestSuite) TestGetRecentActivity_UnknownActionFallback() {
	user := suite.createUser("admin@test.com")
	suite.createAuditLog(user.ID, "some_new_action", "widget", 42)

	feed, err := suite.service.GetRecentActivity()
	suite.Require().NoError(err)
	suite.Require().Len(feed.Events, 1)

	event := feed.Events[0]
	suite.Equal("some_new_action", event.EventType) // Falls through unchanged
	suite.Contains(event.Description, "#42")
}

// =============================================================================
// GetRecentActivity HELPERS
// =============================================================================

func (suite *AdminStatsServiceIntegrationTestSuite) createAuditLog(actorID uint, action, entityType string, entityID uint) {
	log := &models.AuditLog{
		ActorID:    &actorID,
		Action:     action,
		EntityType: entityType,
		EntityID:   entityID,
	}
	err := suite.db.Create(log).Error
	suite.Require().NoError(err)
}

func (suite *AdminStatsServiceIntegrationTestSuite) createAuditLogWithTime(actorID uint, action, entityType string, entityID uint, createdAt time.Time) {
	log := &models.AuditLog{
		ActorID:    &actorID,
		Action:     action,
		EntityType: entityType,
		EntityID:   entityID,
	}
	err := suite.db.Create(log).Error
	suite.Require().NoError(err)
	suite.db.Exec("UPDATE audit_logs SET created_at = ? WHERE id = ?", createdAt, log.ID)
}

func (suite *AdminStatsServiceIntegrationTestSuite) TestGetDashboardStats_FullScenario() {
	// Users
	user := suite.createUser("user@test.com")
	suite.createUser("user2@test.com")
	suite.createUserWithTime("old-user@test.com", time.Now().AddDate(0, 0, -30))

	// Venues
	venue := suite.createVenue("Verified Venue", "NYC", "NY", true)
	suite.createVenue("Unverified Venue", "LA", "CA", false)

	// Artists
	suite.createArtist("Band A")
	suite.createArtist("Band B")

	// Shows
	show := suite.createShow("Approved Show", models.ShowStatusApproved)
	suite.createShow("Pending Show", models.ShowStatusPending)
	suite.createShowWithTime("Old Show", models.ShowStatusApproved, time.Now().AddDate(0, 0, -10))

	// Pending venue edit
	edit := &models.PendingVenueEdit{
		VenueID:     venue.ID,
		SubmittedBy: user.ID,
		Status:      models.VenueEditStatusPending,
	}
	suite.db.Create(edit)

	// Pending report
	sqlDB, _ := suite.db.DB()
	_, _ = sqlDB.Exec(
		"INSERT INTO show_reports (show_id, reported_by, report_type, status) VALUES ($1, $2, $3, $4)",
		show.ID, user.ID, "cancelled", "pending",
	)

	stats, err := suite.service.GetDashboardStats()
	suite.Require().NoError(err)

	suite.Equal(int64(1), stats.PendingShows)
	suite.Equal(int64(1), stats.PendingVenueEdits)
	suite.Equal(int64(1), stats.PendingReports)
	suite.Equal(int64(1), stats.UnverifiedVenues)
	suite.Equal(int64(2), stats.TotalShows)  // 2 approved
	suite.Equal(int64(1), stats.TotalVenues) // 1 verified
	suite.Equal(int64(2), stats.TotalArtists)
	suite.Equal(int64(3), stats.TotalUsers)
	suite.Equal(int64(2), stats.ShowsSubmittedLast7Days) // Recent approved + pending
	suite.Equal(int64(2), stats.UsersRegisteredLast7Days)
}

func (suite *AdminStatsServiceIntegrationTestSuite) TestGetDashboardStats_Trends() {
	now := time.Now()
	threeDaysAgo := now.AddDate(0, 0, -3)   // within current 7 days
	tenDaysAgo := now.AddDate(0, 0, -10)    // within previous 7 days (14-7 days ago)
	twentyDaysAgo := now.AddDate(0, 0, -20) // older than 14 days (should not count)

	// Shows: 2 approved in current week, 1 approved in previous week => trend = +1
	suite.createShowWithTime("Current Show 1", models.ShowStatusApproved, threeDaysAgo)
	suite.createShowWithTime("Current Show 2", models.ShowStatusApproved, now)
	suite.createShowWithTime("Previous Show", models.ShowStatusApproved, tenDaysAgo)
	suite.createShowWithTime("Old Show", models.ShowStatusApproved, twentyDaysAgo)
	// Pending shows should not count for trend
	suite.createShowWithTime("Pending Current", models.ShowStatusPending, threeDaysAgo)

	// Venues: 1 verified in current week, 2 verified in previous week => trend = -1
	suite.createVenueWithTime("Current Venue", "NYC", "NY", true, threeDaysAgo)
	suite.createVenueWithTime("Previous Venue 1", "LA", "CA", true, tenDaysAgo)
	suite.createVenueWithTime("Previous Venue 2", "CHI", "IL", true, tenDaysAgo)
	// Unverified venue should not count
	suite.createVenueWithTime("Unverified Current", "SF", "CA", false, threeDaysAgo)

	// Artists: 3 in current week, 1 in previous week => trend = +2
	suite.createArtistWithTime("Current Artist 1", threeDaysAgo)
	suite.createArtistWithTime("Current Artist 2", now)
	suite.createArtistWithTime("Current Artist 3", now)
	suite.createArtistWithTime("Previous Artist", tenDaysAgo)
	suite.createArtistWithTime("Old Artist", twentyDaysAgo)

	// Users: 1 in current week, 1 in previous week => trend = 0
	suite.createUserWithTime("current@test.com", threeDaysAgo)
	suite.createUserWithTime("previous@test.com", tenDaysAgo)
	suite.createUserWithTime("old@test.com", twentyDaysAgo)

	stats, err := suite.service.GetDashboardStats()
	suite.Require().NoError(err)

	suite.Equal(int64(1), stats.TotalShowsTrend, "shows trend: 2 current - 1 previous = +1")
	suite.Equal(int64(-1), stats.TotalVenuesTrend, "venues trend: 1 current - 2 previous = -1")
	suite.Equal(int64(2), stats.TotalArtistsTrend, "artists trend: 3 current - 1 previous = +2")
	suite.Equal(int64(0), stats.TotalUsersTrend, "users trend: 1 current - 1 previous = 0")
}

func (suite *AdminStatsServiceIntegrationTestSuite) TestGetDashboardStats_TrendsEmpty() {
	// With no data, all trends should be 0
	stats, err := suite.service.GetDashboardStats()
	suite.Require().NoError(err)

	suite.Equal(int64(0), stats.TotalShowsTrend)
	suite.Equal(int64(0), stats.TotalVenuesTrend)
	suite.Equal(int64(0), stats.TotalArtistsTrend)
	suite.Equal(int64(0), stats.TotalUsersTrend)
}
