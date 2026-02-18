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

	apperrors "psychic-homily-backend/internal/errors"
	"psychic-homily-backend/internal/models"
)

// =============================================================================
// UNIT TESTS (No Database Required)
// =============================================================================

func TestNewSavedShowService(t *testing.T) {
	svc := NewSavedShowService(nil)
	assert.NotNil(t, svc)
}

func TestSavedShowService_NilDatabase(t *testing.T) {
	svc := &SavedShowService{db: nil}

	t.Run("SaveShow", func(t *testing.T) {
		err := svc.SaveShow(1, 1)
		assert.Error(t, err)
		assert.Equal(t, "database not initialized", err.Error())
	})

	t.Run("UnsaveShow", func(t *testing.T) {
		err := svc.UnsaveShow(1, 1)
		assert.Error(t, err)
		assert.Equal(t, "database not initialized", err.Error())
	})

	t.Run("GetUserSavedShows", func(t *testing.T) {
		resp, total, err := svc.GetUserSavedShows(1, 10, 0)
		assert.Error(t, err)
		assert.Equal(t, "database not initialized", err.Error())
		assert.Nil(t, resp)
		assert.Zero(t, total)
	})

	t.Run("IsShowSaved", func(t *testing.T) {
		saved, err := svc.IsShowSaved(1, 1)
		assert.Error(t, err)
		assert.Equal(t, "database not initialized", err.Error())
		assert.False(t, saved)
	})

	t.Run("GetSavedShowIDs", func(t *testing.T) {
		result, err := svc.GetSavedShowIDs(1, []uint{1, 2})
		assert.Error(t, err)
		assert.Equal(t, "database not initialized", err.Error())
		assert.Nil(t, result)
	})
}

// =============================================================================
// INTEGRATION TESTS (With Real Database)
// =============================================================================

type SavedShowServiceIntegrationTestSuite struct {
	suite.Suite
	container        testcontainers.Container
	db               *gorm.DB
	savedShowService *SavedShowService
	ctx              context.Context
}

func (suite *SavedShowServiceIntegrationTestSuite) SetupSuite() {
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
		"000002_add_artist_search_indexes.up.sql",
		"000003_add_venue_search_indexes.up.sql",
		"000004_update_venue_constraints.up.sql",
		"000005_add_show_status.up.sql",
		"000006_add_user_saved_shows.up.sql",
		"000007_add_private_show_status.up.sql",
		"000008_add_pending_venue_edits.up.sql",
		"000009_add_bandcamp_embed_url.up.sql",
		"000010_add_scraper_source_fields.up.sql",
		"000012_add_user_deletion_fields.up.sql",
		"000013_add_slugs.up.sql",
		"000014_add_account_lockout.up.sql",
		"000020_add_show_status_flags.up.sql",
		"000023_rename_scraper_to_discovery.up.sql",
		"000026_add_duplicate_of_show_id.up.sql",
		"000028_change_event_date_to_timestamptz.up.sql",
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

	// Run migration 000027 with CONCURRENTLY stripped
	migration27, err := os.ReadFile(filepath.Join("..", "..", "db", "migrations", "000027_add_index_duplicate_of_show_id.up.sql"))
	if err != nil {
		suite.T().Fatalf("failed to read migration 000027: %v", err)
	}
	sql27 := strings.ReplaceAll(string(migration27), "CONCURRENTLY ", "")
	_, err = sqlDB.Exec(sql27)
	if err != nil {
		suite.T().Fatalf("failed to run migration 000027: %v", err)
	}

	suite.savedShowService = &SavedShowService{db: db}
}

func (suite *SavedShowServiceIntegrationTestSuite) TearDownSuite() {
	if suite.container != nil {
		if err := suite.container.Terminate(suite.ctx); err != nil {
			suite.T().Logf("failed to terminate container: %v", err)
		}
	}
}

func (suite *SavedShowServiceIntegrationTestSuite) TearDownTest() {
	sqlDB, err := suite.db.DB()
	suite.Require().NoError(err)
	_, _ = sqlDB.Exec("DELETE FROM user_saved_shows")
	_, _ = sqlDB.Exec("DELETE FROM show_artists")
	_, _ = sqlDB.Exec("DELETE FROM show_venues")
	_, _ = sqlDB.Exec("DELETE FROM shows")
	_, _ = sqlDB.Exec("DELETE FROM artists")
	_, _ = sqlDB.Exec("DELETE FROM venues")
	_, _ = sqlDB.Exec("DELETE FROM users")
}

func TestSavedShowServiceIntegrationTestSuite(t *testing.T) {
	suite.Run(t, new(SavedShowServiceIntegrationTestSuite))
}

// =============================================================================
// HELPERS
// =============================================================================

func (suite *SavedShowServiceIntegrationTestSuite) createTestUser() *models.User {
	user := &models.User{
		Email:         stringPtr(fmt.Sprintf("user-%d@test.com", time.Now().UnixNano())),
		FirstName:     stringPtr("Test"),
		LastName:      stringPtr("User"),
		IsActive:      true,
		EmailVerified: true,
	}
	err := suite.db.Create(user).Error
	suite.Require().NoError(err)
	return user
}

func (suite *SavedShowServiceIntegrationTestSuite) createApprovedShow(title string, userID uint) *models.Show {
	show := &models.Show{
		Title:       title,
		EventDate:   time.Now().UTC().AddDate(0, 0, 7),
		City:        stringPtr("Phoenix"),
		State:       stringPtr("AZ"),
		Status:      models.ShowStatusApproved,
		SubmittedBy: &userID,
	}
	err := suite.db.Create(show).Error
	suite.Require().NoError(err)
	return show
}

func (suite *SavedShowServiceIntegrationTestSuite) createShowWithVenueAndArtist(title string, userID uint) (*models.Show, *models.Venue, *models.Artist) {
	venue := &models.Venue{
		Name:  fmt.Sprintf("Venue for %s", title),
		City:  "Phoenix",
		State: "AZ",
	}
	suite.db.Create(venue)

	artist := &models.Artist{Name: fmt.Sprintf("Artist for %s", title)}
	suite.db.Create(artist)

	show := suite.createApprovedShow(title, userID)

	suite.db.Create(&models.ShowVenue{ShowID: show.ID, VenueID: venue.ID})
	suite.db.Create(&models.ShowArtist{ShowID: show.ID, ArtistID: artist.ID, Position: 0})

	return show, venue, artist
}

// =============================================================================
// Group 1: SaveShow
// =============================================================================

func (suite *SavedShowServiceIntegrationTestSuite) TestSaveShow_Success() {
	user := suite.createTestUser()
	show := suite.createApprovedShow("Save Me", user.ID)

	err := suite.savedShowService.SaveShow(user.ID, show.ID)

	suite.Require().NoError(err)

	// Verify in DB
	var count int64
	suite.db.Model(&models.UserSavedShow{}).
		Where("user_id = ? AND show_id = ?", user.ID, show.ID).
		Count(&count)
	suite.Equal(int64(1), count)
}

func (suite *SavedShowServiceIntegrationTestSuite) TestSaveShow_ShowNotFound() {
	user := suite.createTestUser()

	err := suite.savedShowService.SaveShow(user.ID, 99999)

	suite.Require().Error(err)
	var showErr *apperrors.ShowError
	suite.ErrorAs(err, &showErr)
	suite.Equal(apperrors.CodeShowNotFound, showErr.Code)
}

func (suite *SavedShowServiceIntegrationTestSuite) TestSaveShow_Idempotent() {
	user := suite.createTestUser()
	show := suite.createApprovedShow("Idempotent Save", user.ID)

	err := suite.savedShowService.SaveShow(user.ID, show.ID)
	suite.Require().NoError(err)

	// Save again — should not error (FirstOrCreate)
	err = suite.savedShowService.SaveShow(user.ID, show.ID)
	suite.Require().NoError(err)

	// Should still only be one record
	var count int64
	suite.db.Model(&models.UserSavedShow{}).
		Where("user_id = ? AND show_id = ?", user.ID, show.ID).
		Count(&count)
	suite.Equal(int64(1), count)
}

// =============================================================================
// Group 2: UnsaveShow
// =============================================================================

func (suite *SavedShowServiceIntegrationTestSuite) TestUnsaveShow_Success() {
	user := suite.createTestUser()
	show := suite.createApprovedShow("Unsave Me", user.ID)

	suite.savedShowService.SaveShow(user.ID, show.ID)

	err := suite.savedShowService.UnsaveShow(user.ID, show.ID)

	suite.Require().NoError(err)

	// Verify removed
	var count int64
	suite.db.Model(&models.UserSavedShow{}).
		Where("user_id = ? AND show_id = ?", user.ID, show.ID).
		Count(&count)
	suite.Equal(int64(0), count)
}

func (suite *SavedShowServiceIntegrationTestSuite) TestUnsaveShow_NotSaved() {
	user := suite.createTestUser()
	show := suite.createApprovedShow("Never Saved", user.ID)

	err := suite.savedShowService.UnsaveShow(user.ID, show.ID)

	suite.Require().Error(err)
	suite.Contains(err.Error(), "show was not saved")
}

// =============================================================================
// Group 3: GetUserSavedShows
// =============================================================================

func (suite *SavedShowServiceIntegrationTestSuite) TestGetUserSavedShows_Success() {
	user := suite.createTestUser()
	show1, _, _ := suite.createShowWithVenueAndArtist("Saved Show 1", user.ID)
	show2, _, _ := suite.createShowWithVenueAndArtist("Saved Show 2", user.ID)

	suite.savedShowService.SaveShow(user.ID, show1.ID)
	time.Sleep(10 * time.Millisecond) // ensure different saved_at
	suite.savedShowService.SaveShow(user.ID, show2.ID)

	resp, total, err := suite.savedShowService.GetUserSavedShows(user.ID, 10, 0)

	suite.Require().NoError(err)
	suite.Equal(int64(2), total)
	suite.Require().Len(resp, 2)
	// Most recently saved first
	suite.Equal(show2.ID, resp[0].ID)
	suite.Equal(show1.ID, resp[1].ID)
	suite.NotZero(resp[0].SavedAt)
}

func (suite *SavedShowServiceIntegrationTestSuite) TestGetUserSavedShows_Empty() {
	user := suite.createTestUser()

	resp, total, err := suite.savedShowService.GetUserSavedShows(user.ID, 10, 0)

	suite.Require().NoError(err)
	suite.Equal(int64(0), total)
	suite.Empty(resp)
}

func (suite *SavedShowServiceIntegrationTestSuite) TestGetUserSavedShows_IncludesVenueAndArtist() {
	user := suite.createTestUser()
	show, venue, artist := suite.createShowWithVenueAndArtist("Full Show", user.ID)

	suite.savedShowService.SaveShow(user.ID, show.ID)

	resp, _, err := suite.savedShowService.GetUserSavedShows(user.ID, 10, 0)

	suite.Require().NoError(err)
	suite.Require().Len(resp, 1)
	suite.Require().Len(resp[0].Venues, 1)
	suite.Equal(venue.ID, resp[0].Venues[0].ID)
	suite.Require().Len(resp[0].Artists, 1)
	suite.Equal(artist.ID, resp[0].Artists[0].ID)
}

func (suite *SavedShowServiceIntegrationTestSuite) TestGetUserSavedShows_Pagination() {
	user := suite.createTestUser()
	for i := 0; i < 5; i++ {
		show := suite.createApprovedShow(fmt.Sprintf("Paginated Show %d", i), user.ID)
		suite.savedShowService.SaveShow(user.ID, show.ID)
		time.Sleep(5 * time.Millisecond)
	}

	// Page 1
	resp1, total, err := suite.savedShowService.GetUserSavedShows(user.ID, 2, 0)
	suite.Require().NoError(err)
	suite.Equal(int64(5), total)
	suite.Len(resp1, 2)

	// Page 2
	resp2, _, err := suite.savedShowService.GetUserSavedShows(user.ID, 2, 2)
	suite.Require().NoError(err)
	suite.Len(resp2, 2)

	// Page 3
	resp3, _, err := suite.savedShowService.GetUserSavedShows(user.ID, 2, 4)
	suite.Require().NoError(err)
	suite.Len(resp3, 1)

	// No overlap
	suite.NotEqual(resp1[0].ID, resp2[0].ID)
}

func (suite *SavedShowServiceIntegrationTestSuite) TestGetUserSavedShows_OnlyOwnShows() {
	user1 := suite.createTestUser()
	user2 := suite.createTestUser()
	show1 := suite.createApprovedShow("User1 Show", user1.ID)
	show2 := suite.createApprovedShow("User2 Show", user2.ID)

	suite.savedShowService.SaveShow(user1.ID, show1.ID)
	suite.savedShowService.SaveShow(user2.ID, show2.ID)

	resp, total, err := suite.savedShowService.GetUserSavedShows(user1.ID, 10, 0)

	suite.Require().NoError(err)
	suite.Equal(int64(1), total)
	suite.Require().Len(resp, 1)
	suite.Equal(show1.ID, resp[0].ID)
}

// =============================================================================
// Group 4: IsShowSaved
// =============================================================================

func (suite *SavedShowServiceIntegrationTestSuite) TestIsShowSaved_True() {
	user := suite.createTestUser()
	show := suite.createApprovedShow("Saved Check", user.ID)

	suite.savedShowService.SaveShow(user.ID, show.ID)

	saved, err := suite.savedShowService.IsShowSaved(user.ID, show.ID)

	suite.Require().NoError(err)
	suite.True(saved)
}

func (suite *SavedShowServiceIntegrationTestSuite) TestIsShowSaved_False() {
	user := suite.createTestUser()
	show := suite.createApprovedShow("Not Saved Check", user.ID)

	saved, err := suite.savedShowService.IsShowSaved(user.ID, show.ID)

	suite.Require().NoError(err)
	suite.False(saved)
}

// =============================================================================
// Group 5: GetSavedShowIDs
// =============================================================================

func (suite *SavedShowServiceIntegrationTestSuite) TestGetSavedShowIDs_Success() {
	user := suite.createTestUser()
	show1 := suite.createApprovedShow("Batch Show 1", user.ID)
	show2 := suite.createApprovedShow("Batch Show 2", user.ID)
	show3 := suite.createApprovedShow("Batch Show 3", user.ID)

	suite.savedShowService.SaveShow(user.ID, show1.ID)
	suite.savedShowService.SaveShow(user.ID, show3.ID)

	result, err := suite.savedShowService.GetSavedShowIDs(user.ID, []uint{show1.ID, show2.ID, show3.ID})

	suite.Require().NoError(err)
	suite.True(result[show1.ID])
	suite.False(result[show2.ID])
	suite.True(result[show3.ID])
}

func (suite *SavedShowServiceIntegrationTestSuite) TestGetSavedShowIDs_EmptyInput() {
	user := suite.createTestUser()

	result, err := suite.savedShowService.GetSavedShowIDs(user.ID, []uint{})

	suite.Require().NoError(err)
	suite.Empty(result)
}

func (suite *SavedShowServiceIntegrationTestSuite) TestGetSavedShowIDs_NoneMatched() {
	user := suite.createTestUser()
	show := suite.createApprovedShow("Unmatched Show", user.ID)

	result, err := suite.savedShowService.GetSavedShowIDs(user.ID, []uint{show.ID})

	suite.Require().NoError(err)
	suite.False(result[show.ID])
}
