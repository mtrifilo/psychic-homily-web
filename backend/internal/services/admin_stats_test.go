package services

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"psychic-homily-backend/internal/models"
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

// =============================================================================
// INTEGRATION TESTS (With Real Database)
// =============================================================================

type AdminStatsServiceIntegrationTestSuite struct {
	suite.Suite
	container testcontainers.Container
	db        *gorm.DB
	service   *AdminStatsService
	ctx       context.Context
}

func (suite *AdminStatsServiceIntegrationTestSuite) SetupSuite() {
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

	migrations := []string{
		"000001_create_initial_schema.up.sql",
		"000004_update_venue_constraints.up.sql",
		"000005_add_show_status.up.sql",
		"000007_add_private_show_status.up.sql",
		"000008_add_pending_venue_edits.up.sql",
		"000009_add_bandcamp_embed_url.up.sql",
		"000010_add_scraper_source_fields.up.sql",
		"000012_add_user_deletion_fields.up.sql",
		"000013_add_slugs.up.sql",
		"000014_add_account_lockout.up.sql",
		"000018_add_show_reports.up.sql",
		"000020_add_show_status_flags.up.sql",
		"000023_rename_scraper_to_discovery.up.sql",
		"000026_add_duplicate_of_show_id.up.sql",
		"000028_change_event_date_to_timestamptz.up.sql",
		"000030_add_artist_reports.up.sql",
		"000031_add_user_terms_acceptance.up.sql",
	}
	for _, m := range migrations {
		migrationSQL, err := os.ReadFile(filepath.Join("..", "..", "db", "migrations", m))
		if err != nil {
			suite.T().Fatalf("failed to read migration file %s: %v", m, err)
		}
		_, err = sqlDB.Exec(string(migrationSQL))
		if err != nil {
			suite.T().Fatalf("failed to run migration %s: %v", m, err)
		}
	}

	// Migration 000027 with CONCURRENTLY stripped
	migration27, err := os.ReadFile(filepath.Join("..", "..", "db", "migrations", "000027_add_index_duplicate_of_show_id.up.sql"))
	if err != nil {
		suite.T().Fatalf("failed to read migration 000027: %v", err)
	}
	sql27 := strings.ReplaceAll(string(migration27), "CONCURRENTLY ", "")
	_, err = sqlDB.Exec(sql27)
	if err != nil {
		suite.T().Fatalf("failed to run migration 000027: %v", err)
	}

	suite.service = &AdminStatsService{db: db}
}

func (suite *AdminStatsServiceIntegrationTestSuite) TearDownSuite() {
	if suite.container != nil {
		if err := suite.container.Terminate(suite.ctx); err != nil {
			suite.T().Logf("failed to terminate container: %v", err)
		}
	}
}

func (suite *AdminStatsServiceIntegrationTestSuite) TearDownTest() {
	sqlDB, err := suite.db.DB()
	suite.Require().NoError(err)
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
