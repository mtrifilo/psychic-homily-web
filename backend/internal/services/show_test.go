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

func TestNewShowService(t *testing.T) {
	showService := NewShowService(nil)
	assert.NotNil(t, showService)
}

func TestShowService_NilDatabase(t *testing.T) {
	svc := &ShowService{db: nil}

	t.Run("CreateShow", func(t *testing.T) {
		resp, err := svc.CreateShow(&CreateShowRequest{})
		assert.Error(t, err)
		assert.Equal(t, "database not initialized", err.Error())
		assert.Nil(t, resp)
	})

	t.Run("GetShow", func(t *testing.T) {
		resp, err := svc.GetShow(1)
		assert.Error(t, err)
		assert.Equal(t, "database not initialized", err.Error())
		assert.Nil(t, resp)
	})

	t.Run("GetShowBySlug", func(t *testing.T) {
		resp, err := svc.GetShowBySlug("test-slug")
		assert.Error(t, err)
		assert.Equal(t, "database not initialized", err.Error())
		assert.Nil(t, resp)
	})

	t.Run("GetShows", func(t *testing.T) {
		resp, err := svc.GetShows(nil)
		assert.Error(t, err)
		assert.Equal(t, "database not initialized", err.Error())
		assert.Nil(t, resp)
	})

	t.Run("UpdateShow", func(t *testing.T) {
		resp, err := svc.UpdateShow(1, map[string]interface{}{"title": "x"})
		assert.Error(t, err)
		assert.Equal(t, "database not initialized", err.Error())
		assert.Nil(t, resp)
	})

	t.Run("UpdateShowWithRelations", func(t *testing.T) {
		resp, orphans, err := svc.UpdateShowWithRelations(1, nil, nil, nil, false)
		assert.Error(t, err)
		assert.Equal(t, "database not initialized", err.Error())
		assert.Nil(t, resp)
		assert.Nil(t, orphans)
	})

	t.Run("DeleteShow", func(t *testing.T) {
		err := svc.DeleteShow(1)
		assert.Error(t, err)
		assert.Equal(t, "database not initialized", err.Error())
	})

	t.Run("GetPendingShows", func(t *testing.T) {
		resp, count, err := svc.GetPendingShows(10, 0)
		assert.Error(t, err)
		assert.Equal(t, "database not initialized", err.Error())
		assert.Nil(t, resp)
		assert.Zero(t, count)
	})

	t.Run("GetRejectedShows", func(t *testing.T) {
		resp, count, err := svc.GetRejectedShows(10, 0, "")
		assert.Error(t, err)
		assert.Equal(t, "database not initialized", err.Error())
		assert.Nil(t, resp)
		assert.Zero(t, count)
	})

	t.Run("ApproveShow", func(t *testing.T) {
		resp, err := svc.ApproveShow(1, false)
		assert.Error(t, err)
		assert.Equal(t, "database not initialized", err.Error())
		assert.Nil(t, resp)
	})

	t.Run("RejectShow", func(t *testing.T) {
		resp, err := svc.RejectShow(1, "reason")
		assert.Error(t, err)
		assert.Equal(t, "database not initialized", err.Error())
		assert.Nil(t, resp)
	})

	t.Run("UnpublishShow", func(t *testing.T) {
		resp, err := svc.UnpublishShow(1, 1, false)
		assert.Error(t, err)
		assert.Equal(t, "database not initialized", err.Error())
		assert.Nil(t, resp)
	})

	t.Run("MakePrivateShow", func(t *testing.T) {
		resp, err := svc.MakePrivateShow(1, 1, false)
		assert.Error(t, err)
		assert.Equal(t, "database not initialized", err.Error())
		assert.Nil(t, resp)
	})

	t.Run("PublishShow", func(t *testing.T) {
		resp, err := svc.PublishShow(1, 1, false)
		assert.Error(t, err)
		assert.Equal(t, "database not initialized", err.Error())
		assert.Nil(t, resp)
	})

	t.Run("GetUserSubmissions", func(t *testing.T) {
		resp, count, err := svc.GetUserSubmissions(1, 10, 0)
		assert.Error(t, err)
		assert.Equal(t, "database not initialized", err.Error())
		assert.Nil(t, resp)
		assert.Zero(t, count)
	})

	t.Run("GetUpcomingShows", func(t *testing.T) {
		resp, cursor, err := svc.GetUpcomingShows("UTC", "", 10, false, nil)
		assert.Error(t, err)
		assert.Equal(t, "database not initialized", err.Error())
		assert.Nil(t, resp)
		assert.Nil(t, cursor)
	})

	t.Run("GetShowCities", func(t *testing.T) {
		resp, err := svc.GetShowCities("UTC")
		assert.Error(t, err)
		assert.Equal(t, "database not initialized", err.Error())
		assert.Nil(t, resp)
	})

	t.Run("SetShowSoldOut", func(t *testing.T) {
		resp, err := svc.SetShowSoldOut(1, true)
		assert.Error(t, err)
		assert.Equal(t, "database not initialized", err.Error())
		assert.Nil(t, resp)
	})

	t.Run("SetShowCancelled", func(t *testing.T) {
		resp, err := svc.SetShowCancelled(1, true)
		assert.Error(t, err)
		assert.Equal(t, "database not initialized", err.Error())
		assert.Nil(t, resp)
	})

	t.Run("ExportShowToMarkdown", func(t *testing.T) {
		data, filename, err := svc.ExportShowToMarkdown(1)
		assert.Error(t, err)
		assert.Equal(t, "database not initialized", err.Error())
		assert.Nil(t, data)
		assert.Empty(t, filename)
	})

	t.Run("PreviewShowImport", func(t *testing.T) {
		resp, err := svc.PreviewShowImport([]byte("---\n---"))
		assert.Error(t, err)
		assert.Equal(t, "database not initialized", err.Error())
		assert.Nil(t, resp)
	})

	t.Run("ConfirmShowImport", func(t *testing.T) {
		resp, err := svc.ConfirmShowImport([]byte("---\n---"), false)
		assert.Error(t, err)
		assert.Equal(t, "database not initialized", err.Error())
		assert.Nil(t, resp)
	})

	t.Run("GetAdminShows", func(t *testing.T) {
		resp, count, err := svc.GetAdminShows(10, 0, AdminShowFilters{})
		assert.Error(t, err)
		assert.Equal(t, "database not initialized", err.Error())
		assert.Nil(t, resp)
		assert.Zero(t, count)
	})
}

// =============================================================================
// INTEGRATION TESTS (With Real Database)
// =============================================================================

type ShowServiceIntegrationTestSuite struct {
	suite.Suite
	container   testcontainers.Container
	db          *gorm.DB
	showService *ShowService
	ctx         context.Context
}

func (suite *ShowServiceIntegrationTestSuite) SetupSuite() {
	suite.ctx = context.Background()

	// Start PostgreSQL container
	container, err := testcontainers.GenericContainer(suite.ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        "postgres:17.5",
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

	// Run migrations
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
		"000020_add_show_status_flags.up.sql",
		"000023_rename_scraper_to_discovery.up.sql",
		"000026_add_duplicate_of_show_id.up.sql",
		"000028_change_event_date_to_timestamptz.up.sql",
		// 000027 handled below (CONCURRENTLY not allowed in transaction)
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

	// Run migration 000027 with CONCURRENTLY stripped (not valid in test context)
	migration27, err := os.ReadFile(filepath.Join("..", "..", "db", "migrations", "000027_add_index_duplicate_of_show_id.up.sql"))
	if err != nil {
		suite.T().Fatalf("failed to read migration 000027: %v", err)
	}
	sql27 := strings.ReplaceAll(string(migration27), "CONCURRENTLY ", "")
	_, err = sqlDB.Exec(sql27)
	if err != nil {
		suite.T().Fatalf("failed to run migration 000027: %v", err)
	}

	suite.showService = &ShowService{db: db}
}

func (suite *ShowServiceIntegrationTestSuite) TearDownSuite() {
	if suite.container != nil {
		if err := suite.container.Terminate(suite.ctx); err != nil {
			suite.T().Logf("failed to terminate container: %v", err)
		}
	}
}

// TearDownTest cleans up data between tests for isolation
func (suite *ShowServiceIntegrationTestSuite) TearDownTest() {
	sqlDB, err := suite.db.DB()
	suite.Require().NoError(err)
	// Delete in FK-safe order
	_, _ = sqlDB.Exec("DELETE FROM show_artists")
	_, _ = sqlDB.Exec("DELETE FROM show_venues")
	_, _ = sqlDB.Exec("DELETE FROM shows")
	_, _ = sqlDB.Exec("DELETE FROM artists")
	_, _ = sqlDB.Exec("DELETE FROM venues")
	_, _ = sqlDB.Exec("DELETE FROM users")
}

func TestShowServiceIntegrationTestSuite(t *testing.T) {
	suite.Run(t, new(ShowServiceIntegrationTestSuite))
}

// =============================================================================
// HELPERS
// =============================================================================

func (suite *ShowServiceIntegrationTestSuite) createTestUser() *models.User {
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

func (suite *ShowServiceIntegrationTestSuite) createTestVenue(name, city, state string, verified bool) *models.Venue {
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

func (suite *ShowServiceIntegrationTestSuite) createTestShow(opts ...func(*CreateShowRequest)) *ShowResponse {
	user := suite.createTestUser()
	req := &CreateShowRequest{
		Title:     "Test Show",
		EventDate: time.Date(2026, 6, 15, 20, 0, 0, 0, time.UTC),
		City:      "Phoenix",
		State:     "AZ",
		Venues: []CreateShowVenue{
			{Name: "The Venue", City: "Phoenix", State: "AZ"},
		},
		Artists: []CreateShowArtist{
			{Name: "Test Artist", IsHeadliner: boolPtr(true)},
		},
		SubmittedByUserID: &user.ID,
		SubmitterIsAdmin:  true,
	}
	for _, opt := range opts {
		opt(req)
	}
	resp, err := suite.showService.CreateShow(req)
	suite.Require().NoError(err)
	suite.Require().NotNil(resp)
	return resp
}

func boolPtr(b bool) *bool {
	return &b
}

func uintPtr(u uint) *uint {
	return &u
}

// =============================================================================
// Group 1: CRUD Basics
// =============================================================================

func (suite *ShowServiceIntegrationTestSuite) TestCreateShow_Success() {
	user := suite.createTestUser()
	req := &CreateShowRequest{
		Title:     "Rock Night",
		EventDate: time.Date(2026, 7, 10, 21, 0, 0, 0, time.UTC),
		City:      "Phoenix",
		State:     "AZ",
		Venues: []CreateShowVenue{
			{Name: "Valley Bar", City: "Phoenix", State: "AZ"},
		},
		Artists: []CreateShowArtist{
			{Name: "The Rockers", IsHeadliner: boolPtr(true)},
			{Name: "Opening Act", IsHeadliner: boolPtr(false)},
		},
		SubmittedByUserID: &user.ID,
		SubmitterIsAdmin:  true,
	}

	resp, err := suite.showService.CreateShow(req)

	suite.Require().NoError(err)
	suite.Require().NotNil(resp)
	suite.NotZero(resp.ID)
	suite.Equal("Rock Night", resp.Title)
	suite.Equal("approved", resp.Status)
	suite.NotEmpty(resp.Slug)
	suite.Len(resp.Venues, 1)
	suite.Len(resp.Artists, 2)
	suite.Equal("The Rockers", resp.Artists[0].Name)
	suite.True(*resp.Artists[0].IsHeadliner)
	suite.Equal("Opening Act", resp.Artists[1].Name)
	suite.False(*resp.Artists[1].IsHeadliner)
}

func (suite *ShowServiceIntegrationTestSuite) TestCreateShow_Private() {
	user := suite.createTestUser()
	req := &CreateShowRequest{
		Title:     "Private Gig",
		EventDate: time.Date(2026, 8, 1, 19, 0, 0, 0, time.UTC),
		City:      "Tempe",
		State:     "AZ",
		Venues: []CreateShowVenue{
			{Name: "Small Club", City: "Tempe", State: "AZ"},
		},
		Artists: []CreateShowArtist{
			{Name: "Solo Artist", IsHeadliner: boolPtr(true)},
		},
		SubmittedByUserID: &user.ID,
		IsPrivate:         true,
	}

	resp, err := suite.showService.CreateShow(req)

	suite.Require().NoError(err)
	suite.Equal("private", resp.Status)
}

func (suite *ShowServiceIntegrationTestSuite) TestCreateShow_ExistingVenue() {
	existing := suite.createTestVenue("Existing Hall", "Phoenix", "AZ", true)
	user := suite.createTestUser()

	req := &CreateShowRequest{
		Title:     "Show at Existing",
		EventDate: time.Date(2026, 9, 5, 20, 0, 0, 0, time.UTC),
		City:      "Phoenix",
		State:     "AZ",
		Venues: []CreateShowVenue{
			{Name: "Existing Hall", City: "Phoenix", State: "AZ"},
		},
		Artists: []CreateShowArtist{
			{Name: "Band One", IsHeadliner: boolPtr(true)},
		},
		SubmittedByUserID: &user.ID,
		SubmitterIsAdmin:  true,
	}

	resp, err := suite.showService.CreateShow(req)

	suite.Require().NoError(err)
	suite.Require().Len(resp.Venues, 1)
	suite.Equal(existing.ID, resp.Venues[0].ID)

	// Verify no duplicate venue was created
	var venueCount int64
	suite.db.Model(&models.Venue{}).Where("LOWER(name) = LOWER(?) AND LOWER(city) = LOWER(?)", "Existing Hall", "Phoenix").Count(&venueCount)
	suite.Equal(int64(1), venueCount)
}

func (suite *ShowServiceIntegrationTestSuite) TestCreateShow_NewArtistAndVenue() {
	user := suite.createTestUser()
	req := &CreateShowRequest{
		Title:     "Brand New Show",
		EventDate: time.Date(2026, 10, 1, 20, 0, 0, 0, time.UTC),
		City:      "Tucson",
		State:     "AZ",
		Venues: []CreateShowVenue{
			{Name: "New Place", City: "Tucson", State: "AZ"},
		},
		Artists: []CreateShowArtist{
			{Name: "Brand New Band", IsHeadliner: boolPtr(true)},
		},
		SubmittedByUserID: &user.ID,
		SubmitterIsAdmin:  true,
	}

	resp, err := suite.showService.CreateShow(req)

	suite.Require().NoError(err)
	suite.Len(resp.Venues, 1)
	suite.True(*resp.Venues[0].IsNewVenue)
	suite.Len(resp.Artists, 1)
	suite.True(*resp.Artists[0].IsNewArtist)

	// Verify records exist in DB
	var artist models.Artist
	suite.NoError(suite.db.Where("name = ?", "Brand New Band").First(&artist).Error)
	suite.NotNil(artist.Slug)

	var venue models.Venue
	suite.NoError(suite.db.Where("name = ?", "New Place").First(&venue).Error)
}

func (suite *ShowServiceIntegrationTestSuite) TestGetShow_Success() {
	created := suite.createTestShow()

	resp, err := suite.showService.GetShow(created.ID)

	suite.Require().NoError(err)
	suite.Require().NotNil(resp)
	suite.Equal(created.ID, resp.ID)
	suite.Equal(created.Title, resp.Title)
	suite.Equal(created.Slug, resp.Slug)
	suite.Len(resp.Venues, 1)
	suite.Len(resp.Artists, 1)
}

func (suite *ShowServiceIntegrationTestSuite) TestGetShow_NotFound() {
	resp, err := suite.showService.GetShow(99999)

	suite.Require().Error(err)
	suite.Nil(resp)
	var showErr *apperrors.ShowError
	suite.ErrorAs(err, &showErr)
	suite.Equal(apperrors.CodeShowNotFound, showErr.Code)
}

func (suite *ShowServiceIntegrationTestSuite) TestGetShowBySlug_Success() {
	created := suite.createTestShow()

	resp, err := suite.showService.GetShowBySlug(created.Slug)

	suite.Require().NoError(err)
	suite.Require().NotNil(resp)
	suite.Equal(created.ID, resp.ID)
	suite.Equal(created.Slug, resp.Slug)
}

func (suite *ShowServiceIntegrationTestSuite) TestGetShowBySlug_NotFound() {
	resp, err := suite.showService.GetShowBySlug("nonexistent-slug-2026")

	suite.Require().Error(err)
	suite.Nil(resp)
	var showErr *apperrors.ShowError
	suite.ErrorAs(err, &showErr)
	suite.Equal(apperrors.CodeShowNotFound, showErr.Code)
}

func (suite *ShowServiceIntegrationTestSuite) TestDeleteShow_Success() {
	created := suite.createTestShow()

	err := suite.showService.DeleteShow(created.ID)

	suite.Require().NoError(err)

	// Verify show is gone
	_, err = suite.showService.GetShow(created.ID)
	suite.Error(err)
}

func (suite *ShowServiceIntegrationTestSuite) TestDeleteShow_AssociationsCleanedUp() {
	created := suite.createTestShow()
	showID := created.ID

	err := suite.showService.DeleteShow(showID)
	suite.Require().NoError(err)

	// Verify junction table rows are gone
	var svCount int64
	suite.db.Model(&models.ShowVenue{}).Where("show_id = ?", showID).Count(&svCount)
	suite.Zero(svCount)

	var saCount int64
	suite.db.Model(&models.ShowArtist{}).Where("show_id = ?", showID).Count(&saCount)
	suite.Zero(saCount)
}

// =============================================================================
// Group 2: Updates
// =============================================================================

func (suite *ShowServiceIntegrationTestSuite) TestUpdateShow_BasicFields() {
	created := suite.createTestShow()

	resp, err := suite.showService.UpdateShow(created.ID, map[string]interface{}{
		"title":       "Updated Title",
		"description": "New description",
	})

	suite.Require().NoError(err)
	suite.Equal("Updated Title", resp.Title)
	suite.Equal("New description", *resp.Description)
}

func (suite *ShowServiceIntegrationTestSuite) TestUpdateShow_EventDate_UTC() {
	created := suite.createTestShow()

	// Pass a non-UTC time
	eastern, _ := time.LoadLocation("America/New_York")
	newDate := time.Date(2026, 12, 25, 20, 0, 0, 0, eastern)

	resp, err := suite.showService.UpdateShow(created.ID, map[string]interface{}{
		"event_date": newDate,
	})

	suite.Require().NoError(err)
	// Verify the stored time represents the same instant (service converts to UTC before storing)
	suite.Equal(newDate.UTC().Unix(), resp.EventDate.Unix(),
		"event_date should represent the same instant after UTC conversion")
}

func (suite *ShowServiceIntegrationTestSuite) TestUpdateShowWithRelations_ReplaceArtists() {
	created := suite.createTestShow()

	newArtists := []CreateShowArtist{
		{Name: "Replacement Headliner", IsHeadliner: boolPtr(true)},
		{Name: "Replacement Opener", IsHeadliner: boolPtr(false)},
	}

	resp, _, err := suite.showService.UpdateShowWithRelations(created.ID, nil, nil, newArtists, true)

	suite.Require().NoError(err)
	suite.Require().Len(resp.Artists, 2)
	suite.Equal("Replacement Headliner", resp.Artists[0].Name)
	suite.True(*resp.Artists[0].IsHeadliner)
	suite.Equal("Replacement Opener", resp.Artists[1].Name)
}

func (suite *ShowServiceIntegrationTestSuite) TestUpdateShowWithRelations_ReplaceVenues() {
	created := suite.createTestShow()

	newVenues := []CreateShowVenue{
		{Name: "Replacement Venue", City: "Tempe", State: "AZ"},
	}

	resp, _, err := suite.showService.UpdateShowWithRelations(created.ID, nil, newVenues, nil, true)

	suite.Require().NoError(err)
	suite.Require().Len(resp.Venues, 1)
	suite.Equal("Replacement Venue", resp.Venues[0].Name)
}

func (suite *ShowServiceIntegrationTestSuite) TestUpdateShowWithRelations_OrphanedArtist() {
	// Create a show with a unique artist
	created := suite.createTestShow(func(req *CreateShowRequest) {
		req.Artists = []CreateShowArtist{
			{Name: "Soon Orphaned", IsHeadliner: boolPtr(true)},
		}
	})

	// Replace artists with a different one
	newArtists := []CreateShowArtist{
		{Name: "Brand New Star", IsHeadliner: boolPtr(true)},
	}

	_, orphans, err := suite.showService.UpdateShowWithRelations(created.ID, nil, nil, newArtists, true)

	suite.Require().NoError(err)
	suite.Require().Len(orphans, 1)
	suite.Equal("Soon Orphaned", orphans[0].Name)
}

func (suite *ShowServiceIntegrationTestSuite) TestUpdateShowWithRelations_NoOrphanIfStillAssociated() {
	// Create two shows sharing the same artist
	user := suite.createTestUser()
	req1 := &CreateShowRequest{
		Title:     "Show A",
		EventDate: time.Date(2026, 6, 10, 20, 0, 0, 0, time.UTC),
		City:      "Phoenix",
		State:     "AZ",
		Venues:    []CreateShowVenue{{Name: "Venue A", City: "Phoenix", State: "AZ"}},
		Artists: []CreateShowArtist{
			{Name: "Shared Artist", IsHeadliner: boolPtr(true)},
		},
		SubmittedByUserID: &user.ID,
		SubmitterIsAdmin:  true,
	}
	showA, err := suite.showService.CreateShow(req1)
	suite.Require().NoError(err)

	req2 := &CreateShowRequest{
		Title:     "Show B",
		EventDate: time.Date(2026, 6, 11, 20, 0, 0, 0, time.UTC),
		City:      "Tempe",
		State:     "AZ",
		Venues:    []CreateShowVenue{{Name: "Venue B", City: "Tempe", State: "AZ"}},
		Artists: []CreateShowArtist{
			{Name: "Shared Artist", IsHeadliner: boolPtr(true)},
		},
		SubmittedByUserID: &user.ID,
		SubmitterIsAdmin:  true,
	}
	_, err = suite.showService.CreateShow(req2)
	suite.Require().NoError(err)

	// Remove the shared artist from Show A
	newArtists := []CreateShowArtist{
		{Name: "Different Artist", IsHeadliner: boolPtr(true)},
	}

	_, orphans, err := suite.showService.UpdateShowWithRelations(showA.ID, nil, nil, newArtists, true)

	suite.Require().NoError(err)
	// "Shared Artist" is still on Show B, so it should NOT be orphaned
	for _, o := range orphans {
		suite.NotEqual("Shared Artist", o.Name)
	}
}

// =============================================================================
// Group 3: Status Transitions
// =============================================================================

func (suite *ShowServiceIntegrationTestSuite) TestApproveShow_FromPending() {
	// Create a pending show by setting status to pending directly
	created := suite.createTestShow()
	suite.db.Model(&models.Show{}).Where("id = ?", created.ID).Update("status", models.ShowStatusPending)

	resp, err := suite.showService.ApproveShow(created.ID, false)

	suite.Require().NoError(err)
	suite.Equal("approved", resp.Status)
}

func (suite *ShowServiceIntegrationTestSuite) TestApproveShow_FromRejected() {
	created := suite.createTestShow()
	suite.db.Model(&models.Show{}).Where("id = ?", created.ID).Updates(map[string]interface{}{
		"status":           models.ShowStatusRejected,
		"rejection_reason": "Bad info",
	})

	resp, err := suite.showService.ApproveShow(created.ID, false)

	suite.Require().NoError(err)
	suite.Equal("approved", resp.Status)
	// Rejection reason should be cleared
	suite.True(resp.RejectionReason == nil || *resp.RejectionReason == "")
}

func (suite *ShowServiceIntegrationTestSuite) TestApproveShow_WithVenueVerification() {
	// Create show with an unverified venue
	user := suite.createTestUser()
	req := &CreateShowRequest{
		Title:     "Verify Venue Show",
		EventDate: time.Date(2026, 7, 20, 20, 0, 0, 0, time.UTC),
		City:      "Phoenix",
		State:     "AZ",
		Venues: []CreateShowVenue{
			{Name: "Unverified Place", City: "Phoenix", State: "AZ"},
		},
		Artists: []CreateShowArtist{
			{Name: "Verify Artist", IsHeadliner: boolPtr(true)},
		},
		SubmittedByUserID: &user.ID,
		SubmitterIsAdmin:  false, // non-admin creates unverified venue
	}
	created, err := suite.showService.CreateShow(req)
	suite.Require().NoError(err)

	// Set show to pending so we can approve it
	suite.db.Model(&models.Show{}).Where("id = ?", created.ID).Update("status", models.ShowStatusPending)

	resp, err := suite.showService.ApproveShow(created.ID, true)

	suite.Require().NoError(err)
	suite.Equal("approved", resp.Status)

	// Venue should now be verified
	var venue models.Venue
	suite.db.First(&venue, resp.Venues[0].ID)
	suite.True(venue.Verified)
}

func (suite *ShowServiceIntegrationTestSuite) TestApproveShow_AlreadyApproved_Fails() {
	created := suite.createTestShow() // created as approved

	_, err := suite.showService.ApproveShow(created.ID, false)

	suite.Require().Error(err)
	suite.Contains(err.Error(), "cannot be approved")
}

func (suite *ShowServiceIntegrationTestSuite) TestRejectShow_Success() {
	created := suite.createTestShow()
	suite.db.Model(&models.Show{}).Where("id = ?", created.ID).Update("status", models.ShowStatusPending)

	resp, err := suite.showService.RejectShow(created.ID, "Duplicate listing")

	suite.Require().NoError(err)
	suite.Equal("rejected", resp.Status)
	suite.Require().NotNil(resp.RejectionReason)
	suite.Equal("Duplicate listing", *resp.RejectionReason)
}

func (suite *ShowServiceIntegrationTestSuite) TestRejectShow_NotPending_Fails() {
	created := suite.createTestShow() // approved

	_, err := suite.showService.RejectShow(created.ID, "reason")

	suite.Require().Error(err)
	suite.Contains(err.Error(), "not pending")
}

func (suite *ShowServiceIntegrationTestSuite) TestUnpublishShow_AsSubmitter() {
	user := suite.createTestUser()
	req := &CreateShowRequest{
		Title:     "Submitter Unpublish",
		EventDate: time.Date(2026, 8, 10, 20, 0, 0, 0, time.UTC),
		City:      "Phoenix",
		State:     "AZ",
		Venues:    []CreateShowVenue{{Name: "Unpub Venue", City: "Phoenix", State: "AZ"}},
		Artists:   []CreateShowArtist{{Name: "Unpub Artist", IsHeadliner: boolPtr(true)}},
		SubmittedByUserID: &user.ID,
		SubmitterIsAdmin:  true,
	}
	created, err := suite.showService.CreateShow(req)
	suite.Require().NoError(err)

	resp, err := suite.showService.UnpublishShow(created.ID, user.ID, false)

	suite.Require().NoError(err)
	suite.Equal("private", resp.Status)
}

func (suite *ShowServiceIntegrationTestSuite) TestUnpublishShow_AsAdmin() {
	created := suite.createTestShow()
	adminID := uint(9999)

	resp, err := suite.showService.UnpublishShow(created.ID, adminID, true)

	suite.Require().NoError(err)
	suite.Equal("private", resp.Status)
}

func (suite *ShowServiceIntegrationTestSuite) TestUnpublishShow_Unauthorized() {
	created := suite.createTestShow()
	differentUser := suite.createTestUser()

	_, err := suite.showService.UnpublishShow(created.ID, differentUser.ID, false)

	suite.Require().Error(err)
	var showErr *apperrors.ShowError
	suite.ErrorAs(err, &showErr)
	suite.Equal(apperrors.CodeShowUnpublishUnauthorized, showErr.Code)
}

func (suite *ShowServiceIntegrationTestSuite) TestMakePrivateShow_Success() {
	created := suite.createTestShow()
	suite.db.Model(&models.Show{}).Where("id = ?", created.ID).Update("status", models.ShowStatusPending)

	// Get the submitter ID
	var show models.Show
	suite.db.First(&show, created.ID)

	resp, err := suite.showService.MakePrivateShow(created.ID, *show.SubmittedBy, false)

	suite.Require().NoError(err)
	suite.Equal("private", resp.Status)
}

func (suite *ShowServiceIntegrationTestSuite) TestMakePrivateShow_NotPending_Fails() {
	created := suite.createTestShow() // approved
	var show models.Show
	suite.db.First(&show, created.ID)

	_, err := suite.showService.MakePrivateShow(created.ID, *show.SubmittedBy, false)

	suite.Require().Error(err)
	suite.Contains(err.Error(), "can only make pending shows private")
}

func (suite *ShowServiceIntegrationTestSuite) TestPublishShow_Success() {
	user := suite.createTestUser()
	req := &CreateShowRequest{
		Title:     "To Publish",
		EventDate: time.Date(2026, 9, 1, 20, 0, 0, 0, time.UTC),
		City:      "Phoenix",
		State:     "AZ",
		Venues:    []CreateShowVenue{{Name: "Pub Venue", City: "Phoenix", State: "AZ"}},
		Artists:   []CreateShowArtist{{Name: "Pub Artist", IsHeadliner: boolPtr(true)}},
		SubmittedByUserID: &user.ID,
		IsPrivate:         true,
	}
	created, err := suite.showService.CreateShow(req)
	suite.Require().NoError(err)
	suite.Equal("private", created.Status)

	resp, err := suite.showService.PublishShow(created.ID, user.ID, false)

	suite.Require().NoError(err)
	suite.Equal("approved", resp.Status)
}

func (suite *ShowServiceIntegrationTestSuite) TestPublishShow_Unauthorized() {
	user := suite.createTestUser()
	req := &CreateShowRequest{
		Title:     "Unauth Publish",
		EventDate: time.Date(2026, 9, 2, 20, 0, 0, 0, time.UTC),
		City:      "Phoenix",
		State:     "AZ",
		Venues:    []CreateShowVenue{{Name: "Unauth Pub Venue", City: "Phoenix", State: "AZ"}},
		Artists:   []CreateShowArtist{{Name: "Unauth Pub Artist", IsHeadliner: boolPtr(true)}},
		SubmittedByUserID: &user.ID,
		IsPrivate:         true,
	}
	created, err := suite.showService.CreateShow(req)
	suite.Require().NoError(err)

	differentUser := suite.createTestUser()
	_, err = suite.showService.PublishShow(created.ID, differentUser.ID, false)

	suite.Require().Error(err)
	var showErr *apperrors.ShowError
	suite.ErrorAs(err, &showErr)
	suite.Equal(apperrors.CodeShowPublishUnauthorized, showErr.Code)
}

// =============================================================================
// Group 4: Duplicate Detection
// =============================================================================

func (suite *ShowServiceIntegrationTestSuite) TestCreateShow_DuplicateHeadliner_SameVenueSameDay_Fails() {
	user := suite.createTestUser()
	eventDate := time.Date(2026, 11, 1, 20, 0, 0, 0, time.UTC)

	req := &CreateShowRequest{
		Title:     "First Show",
		EventDate: eventDate,
		City:      "Phoenix",
		State:     "AZ",
		Venues:    []CreateShowVenue{{Name: "Dup Venue", City: "Phoenix", State: "AZ"}},
		Artists:   []CreateShowArtist{{Name: "Dup Headliner", IsHeadliner: boolPtr(true)}},
		SubmittedByUserID: &user.ID,
		SubmitterIsAdmin:  true,
	}
	_, err := suite.showService.CreateShow(req)
	suite.Require().NoError(err)

	// Try creating duplicate
	req2 := &CreateShowRequest{
		Title:     "Second Show (duplicate)",
		EventDate: eventDate,
		City:      "Phoenix",
		State:     "AZ",
		Venues:    []CreateShowVenue{{Name: "Dup Venue", City: "Phoenix", State: "AZ"}},
		Artists:   []CreateShowArtist{{Name: "Dup Headliner", IsHeadliner: boolPtr(true)}},
		SubmittedByUserID: &user.ID,
		SubmitterIsAdmin:  true,
	}
	_, err = suite.showService.CreateShow(req2)

	suite.Require().Error(err)
	suite.Contains(err.Error(), "already performing")
}

func (suite *ShowServiceIntegrationTestSuite) TestCreateShow_DuplicateHeadliner_CaseInsensitive() {
	user := suite.createTestUser()
	eventDate := time.Date(2026, 11, 2, 20, 0, 0, 0, time.UTC)

	req := &CreateShowRequest{
		Title:     "Original Case Show",
		EventDate: eventDate,
		City:      "Phoenix",
		State:     "AZ",
		Venues:    []CreateShowVenue{{Name: "Case Venue", City: "Phoenix", State: "AZ"}},
		Artists:   []CreateShowArtist{{Name: "The Band", IsHeadliner: boolPtr(true)}},
		SubmittedByUserID: &user.ID,
		SubmitterIsAdmin:  true,
	}
	_, err := suite.showService.CreateShow(req)
	suite.Require().NoError(err)

	// Try with different case
	req2 := &CreateShowRequest{
		Title:     "Case Insensitive Dup",
		EventDate: eventDate,
		City:      "Phoenix",
		State:     "AZ",
		Venues:    []CreateShowVenue{{Name: "case venue", City: "Phoenix", State: "AZ"}},
		Artists:   []CreateShowArtist{{Name: "the band", IsHeadliner: boolPtr(true)}},
		SubmittedByUserID: &user.ID,
		SubmitterIsAdmin:  true,
	}
	_, err = suite.showService.CreateShow(req2)

	suite.Require().Error(err)
	suite.Contains(err.Error(), "already performing")
}

func (suite *ShowServiceIntegrationTestSuite) TestCreateShow_DuplicateHeadliner_DifferentDay_OK() {
	user := suite.createTestUser()

	req := &CreateShowRequest{
		Title:     "Day 1",
		EventDate: time.Date(2026, 11, 3, 20, 0, 0, 0, time.UTC),
		City:      "Phoenix",
		State:     "AZ",
		Venues:    []CreateShowVenue{{Name: "Multi Day Venue", City: "Phoenix", State: "AZ"}},
		Artists:   []CreateShowArtist{{Name: "Day Headliner", IsHeadliner: boolPtr(true)}},
		SubmittedByUserID: &user.ID,
		SubmitterIsAdmin:  true,
	}
	_, err := suite.showService.CreateShow(req)
	suite.Require().NoError(err)

	// Same headliner, same venue, DIFFERENT day
	req2 := &CreateShowRequest{
		Title:     "Day 2",
		EventDate: time.Date(2026, 11, 4, 20, 0, 0, 0, time.UTC),
		City:      "Phoenix",
		State:     "AZ",
		Venues:    []CreateShowVenue{{Name: "Multi Day Venue", City: "Phoenix", State: "AZ"}},
		Artists:   []CreateShowArtist{{Name: "Day Headliner", IsHeadliner: boolPtr(true)}},
		SubmittedByUserID: &user.ID,
		SubmitterIsAdmin:  true,
	}
	resp, err := suite.showService.CreateShow(req2)

	suite.Require().NoError(err)
	suite.NotNil(resp)
}

func (suite *ShowServiceIntegrationTestSuite) TestCreateShow_DuplicateHeadliner_DifferentVenue_OK() {
	user := suite.createTestUser()
	eventDate := time.Date(2026, 11, 5, 20, 0, 0, 0, time.UTC)

	req := &CreateShowRequest{
		Title:     "Venue 1",
		EventDate: eventDate,
		City:      "Phoenix",
		State:     "AZ",
		Venues:    []CreateShowVenue{{Name: "Venue Alpha", City: "Phoenix", State: "AZ"}},
		Artists:   []CreateShowArtist{{Name: "Venue Hopper", IsHeadliner: boolPtr(true)}},
		SubmittedByUserID: &user.ID,
		SubmitterIsAdmin:  true,
	}
	_, err := suite.showService.CreateShow(req)
	suite.Require().NoError(err)

	// Same headliner, same day, DIFFERENT venue
	req2 := &CreateShowRequest{
		Title:     "Venue 2",
		EventDate: eventDate,
		City:      "Phoenix",
		State:     "AZ",
		Venues:    []CreateShowVenue{{Name: "Venue Beta", City: "Phoenix", State: "AZ"}},
		Artists:   []CreateShowArtist{{Name: "Venue Hopper", IsHeadliner: boolPtr(true)}},
		SubmittedByUserID: &user.ID,
		SubmitterIsAdmin:  true,
	}
	resp, err := suite.showService.CreateShow(req2)

	suite.Require().NoError(err)
	suite.NotNil(resp)
}

func (suite *ShowServiceIntegrationTestSuite) TestCreateShow_NoHeadliner_NoDuplicateCheck() {
	user := suite.createTestUser()
	eventDate := time.Date(2026, 11, 6, 20, 0, 0, 0, time.UTC)

	// First show with no explicit headliner (first artist defaults to headliner)
	// We need both artists to be explicitly non-headliner for this test
	req := &CreateShowRequest{
		Title:     "No Headliner 1",
		EventDate: eventDate,
		City:      "Phoenix",
		State:     "AZ",
		Venues:    []CreateShowVenue{{Name: "NH Venue", City: "Phoenix", State: "AZ"}},
		Artists: []CreateShowArtist{
			{Name: "Opener Only", IsHeadliner: boolPtr(false)},
		},
		SubmittedByUserID: &user.ID,
		SubmitterIsAdmin:  true,
	}
	_, err := suite.showService.CreateShow(req)
	suite.Require().NoError(err)

	// Same details, should succeed (no headliner means no dup check)
	req2 := &CreateShowRequest{
		Title:     "No Headliner 2",
		EventDate: eventDate,
		City:      "Phoenix",
		State:     "AZ",
		Venues:    []CreateShowVenue{{Name: "NH Venue", City: "Phoenix", State: "AZ"}},
		Artists: []CreateShowArtist{
			{Name: "Opener Only 2", IsHeadliner: boolPtr(false)},
		},
		SubmittedByUserID: &user.ID,
		SubmitterIsAdmin:  true,
	}
	resp, err := suite.showService.CreateShow(req2)

	suite.Require().NoError(err)
	suite.NotNil(resp)
}

// =============================================================================
// Group 5: Listing & Filtering
// =============================================================================

func (suite *ShowServiceIntegrationTestSuite) TestGetShows_FilterByCity() {
	user := suite.createTestUser()

	// Phoenix show
	req1 := &CreateShowRequest{
		Title:     "Phoenix Show",
		EventDate: time.Date(2026, 12, 1, 20, 0, 0, 0, time.UTC),
		City:      "Phoenix",
		State:     "AZ",
		Venues:    []CreateShowVenue{{Name: "PHX Venue", City: "Phoenix", State: "AZ"}},
		Artists:   []CreateShowArtist{{Name: "PHX Artist", IsHeadliner: boolPtr(true)}},
		SubmittedByUserID: &user.ID,
		SubmitterIsAdmin:  true,
	}
	_, err := suite.showService.CreateShow(req1)
	suite.Require().NoError(err)

	// Tucson show
	req2 := &CreateShowRequest{
		Title:     "Tucson Show",
		EventDate: time.Date(2026, 12, 2, 20, 0, 0, 0, time.UTC),
		City:      "Tucson",
		State:     "AZ",
		Venues:    []CreateShowVenue{{Name: "TUC Venue", City: "Tucson", State: "AZ"}},
		Artists:   []CreateShowArtist{{Name: "TUC Artist", IsHeadliner: boolPtr(true)}},
		SubmittedByUserID: &user.ID,
		SubmitterIsAdmin:  true,
	}
	_, err = suite.showService.CreateShow(req2)
	suite.Require().NoError(err)

	resp, err := suite.showService.GetShows(map[string]interface{}{"city": "Phoenix"})

	suite.Require().NoError(err)
	suite.Require().Len(resp, 1)
	suite.Equal("Phoenix Show", resp[0].Title)
}

func (suite *ShowServiceIntegrationTestSuite) TestGetShows_FilterByDateRange() {
	user := suite.createTestUser()

	dates := []time.Time{
		time.Date(2026, 12, 5, 20, 0, 0, 0, time.UTC),
		time.Date(2026, 12, 10, 20, 0, 0, 0, time.UTC),
		time.Date(2026, 12, 20, 20, 0, 0, 0, time.UTC),
	}

	for i, d := range dates {
		req := &CreateShowRequest{
			Title:     fmt.Sprintf("Date Show %d", i),
			EventDate: d,
			City:      "Phoenix",
			State:     "AZ",
			Venues:    []CreateShowVenue{{Name: fmt.Sprintf("Date Venue %d", i), City: "Phoenix", State: "AZ"}},
			Artists:   []CreateShowArtist{{Name: fmt.Sprintf("Date Artist %d", i), IsHeadliner: boolPtr(true)}},
			SubmittedByUserID: &user.ID,
			SubmitterIsAdmin:  true,
		}
		_, err := suite.showService.CreateShow(req)
		suite.Require().NoError(err)
	}

	resp, err := suite.showService.GetShows(map[string]interface{}{
		"from_date": time.Date(2026, 12, 6, 0, 0, 0, 0, time.UTC),
		"to_date":   time.Date(2026, 12, 15, 0, 0, 0, 0, time.UTC),
	})

	suite.Require().NoError(err)
	suite.Require().Len(resp, 1)
	suite.Equal("Date Show 1", resp[0].Title)
}

func (suite *ShowServiceIntegrationTestSuite) TestGetUserSubmissions_Success() {
	user := suite.createTestUser()
	otherUser := suite.createTestUser()

	// Create 2 shows for user
	for i := 0; i < 2; i++ {
		req := &CreateShowRequest{
			Title:     fmt.Sprintf("User Show %d", i),
			EventDate: time.Date(2026, 12, 1+i, 20, 0, 0, 0, time.UTC),
			City:      "Phoenix",
			State:     "AZ",
			Venues:    []CreateShowVenue{{Name: fmt.Sprintf("Sub Venue %d", i), City: "Phoenix", State: "AZ"}},
			Artists:   []CreateShowArtist{{Name: fmt.Sprintf("Sub Artist %d", i), IsHeadliner: boolPtr(true)}},
			SubmittedByUserID: &user.ID,
			SubmitterIsAdmin:  true,
		}
		_, err := suite.showService.CreateShow(req)
		suite.Require().NoError(err)
	}

	// Create 1 show for other user
	req := &CreateShowRequest{
		Title:     "Other User Show",
		EventDate: time.Date(2026, 12, 3, 20, 0, 0, 0, time.UTC),
		City:      "Phoenix",
		State:     "AZ",
		Venues:    []CreateShowVenue{{Name: "Other Venue", City: "Phoenix", State: "AZ"}},
		Artists:   []CreateShowArtist{{Name: "Other Artist", IsHeadliner: boolPtr(true)}},
		SubmittedByUserID: &otherUser.ID,
		SubmitterIsAdmin:  true,
	}
	_, err := suite.showService.CreateShow(req)
	suite.Require().NoError(err)

	shows, total, err := suite.showService.GetUserSubmissions(user.ID, 10, 0)

	suite.Require().NoError(err)
	suite.Equal(2, total)
	suite.Len(shows, 2)
}

func (suite *ShowServiceIntegrationTestSuite) TestGetPendingShows_Success() {
	user := suite.createTestUser()

	// Create 2 shows, set one to pending
	req1 := &CreateShowRequest{
		Title:     "Pending Show",
		EventDate: time.Date(2026, 12, 1, 20, 0, 0, 0, time.UTC),
		City:      "Phoenix",
		State:     "AZ",
		Venues:    []CreateShowVenue{{Name: "Pending Venue 1", City: "Phoenix", State: "AZ"}},
		Artists:   []CreateShowArtist{{Name: "Pending Artist 1", IsHeadliner: boolPtr(true)}},
		SubmittedByUserID: &user.ID,
		SubmitterIsAdmin:  true,
	}
	show1, err := suite.showService.CreateShow(req1)
	suite.Require().NoError(err)
	suite.db.Model(&models.Show{}).Where("id = ?", show1.ID).Update("status", models.ShowStatusPending)

	req2 := &CreateShowRequest{
		Title:     "Approved Show",
		EventDate: time.Date(2026, 12, 2, 20, 0, 0, 0, time.UTC),
		City:      "Phoenix",
		State:     "AZ",
		Venues:    []CreateShowVenue{{Name: "Pending Venue 2", City: "Phoenix", State: "AZ"}},
		Artists:   []CreateShowArtist{{Name: "Pending Artist 2", IsHeadliner: boolPtr(true)}},
		SubmittedByUserID: &user.ID,
		SubmitterIsAdmin:  true,
	}
	_, err = suite.showService.CreateShow(req2)
	suite.Require().NoError(err)

	shows, total, err := suite.showService.GetPendingShows(10, 0)

	suite.Require().NoError(err)
	suite.Equal(int64(1), total)
	suite.Len(shows, 1)
	suite.Equal("Pending Show", shows[0].Title)
}

func (suite *ShowServiceIntegrationTestSuite) TestGetRejectedShows_Success() {
	user := suite.createTestUser()

	req := &CreateShowRequest{
		Title:     "Rejected Show",
		EventDate: time.Date(2026, 12, 1, 20, 0, 0, 0, time.UTC),
		City:      "Phoenix",
		State:     "AZ",
		Venues:    []CreateShowVenue{{Name: "Reject Venue", City: "Phoenix", State: "AZ"}},
		Artists:   []CreateShowArtist{{Name: "Reject Artist", IsHeadliner: boolPtr(true)}},
		SubmittedByUserID: &user.ID,
		SubmitterIsAdmin:  true,
	}
	show, err := suite.showService.CreateShow(req)
	suite.Require().NoError(err)
	suite.db.Model(&models.Show{}).Where("id = ?", show.ID).Updates(map[string]interface{}{
		"status":           models.ShowStatusRejected,
		"rejection_reason": "Spam",
	})

	shows, total, err := suite.showService.GetRejectedShows(10, 0, "")

	suite.Require().NoError(err)
	suite.Equal(int64(1), total)
	suite.Len(shows, 1)
	suite.Equal("Rejected Show", shows[0].Title)
}

func (suite *ShowServiceIntegrationTestSuite) TestGetRejectedShows_WithSearch() {
	user := suite.createTestUser()

	// Create 2 rejected shows
	for i, reason := range []string{"Duplicate entry", "Spam content"} {
		req := &CreateShowRequest{
			Title:     fmt.Sprintf("Rejected %d", i),
			EventDate: time.Date(2026, 12, 1+i, 20, 0, 0, 0, time.UTC),
			City:      "Phoenix",
			State:     "AZ",
			Venues:    []CreateShowVenue{{Name: fmt.Sprintf("Search Venue %d", i), City: "Phoenix", State: "AZ"}},
			Artists:   []CreateShowArtist{{Name: fmt.Sprintf("Search Artist %d", i), IsHeadliner: boolPtr(true)}},
			SubmittedByUserID: &user.ID,
			SubmitterIsAdmin:  true,
		}
		show, err := suite.showService.CreateShow(req)
		suite.Require().NoError(err)
		suite.db.Model(&models.Show{}).Where("id = ?", show.ID).Updates(map[string]interface{}{
			"status":           models.ShowStatusRejected,
			"rejection_reason": reason,
		})
	}

	shows, total, err := suite.showService.GetRejectedShows(10, 0, "Spam")

	suite.Require().NoError(err)
	suite.Equal(int64(1), total)
	suite.Len(shows, 1)
}

func (suite *ShowServiceIntegrationTestSuite) TestGetUpcomingShows_Pagination() {
	user := suite.createTestUser()

	// Create 5 future shows with distinct dates (no sub-second precision issues)
	baseDate := time.Date(2027, 6, 1, 20, 0, 0, 0, time.UTC)
	for i := 0; i < 5; i++ {
		req := &CreateShowRequest{
			Title:     fmt.Sprintf("Upcoming %d", i),
			EventDate: baseDate.AddDate(0, 0, i),
			City:      "Phoenix",
			State:     "AZ",
			Venues:    []CreateShowVenue{{Name: fmt.Sprintf("Up Venue %d", i), City: "Phoenix", State: "AZ"}},
			Artists:   []CreateShowArtist{{Name: fmt.Sprintf("Up Artist %d", i), IsHeadliner: boolPtr(true)}},
			SubmittedByUserID: &user.ID,
			SubmitterIsAdmin:  true,
		}
		_, err := suite.showService.CreateShow(req)
		suite.Require().NoError(err)
	}

	// Page 1: get 3
	page1, cursor1, err := suite.showService.GetUpcomingShows("UTC", "", 3, false, nil)
	suite.Require().NoError(err)
	suite.Require().Len(page1, 3)
	suite.Require().NotNil(cursor1, "should have a next cursor when more results exist")

	// Page 2: use cursor, expect remaining 2
	page2, cursor2, err := suite.showService.GetUpcomingShows("UTC", *cursor1, 3, false, nil)
	suite.Require().NoError(err)
	suite.Require().Len(page2, 2, "page 2 should have exactly the remaining 2 shows")
	suite.Nil(cursor2, "should be no more pages after page 2")

	// Verify no overlap: page 2 IDs must not appear in page 1
	page1IDs := map[uint]bool{}
	for _, s := range page1 {
		page1IDs[s.ID] = true
	}
	for _, s := range page2 {
		suite.False(page1IDs[s.ID], "show ID %d appeared on both pages", s.ID)
	}

	// Verify chronological ordering across pages
	suite.True(page2[0].EventDate.After(page1[len(page1)-1].EventDate) ||
		(page2[0].EventDate.Equal(page1[len(page1)-1].EventDate) && page2[0].ID > page1[len(page1)-1].ID),
		"page 2 first show should come strictly after page 1 last show")
}

func (suite *ShowServiceIntegrationTestSuite) TestGetShowCities_Success() {
	user := suite.createTestUser()

	// Create shows in different cities
	cities := []struct{ city, state string }{
		{"Phoenix", "AZ"},
		{"Phoenix", "AZ"},
		{"Tucson", "AZ"},
	}
	for i, c := range cities {
		req := &CreateShowRequest{
			Title:     fmt.Sprintf("City Show %d", i),
			EventDate: time.Now().UTC().AddDate(0, 0, i+1),
			City:      c.city,
			State:     c.state,
			Venues:    []CreateShowVenue{{Name: fmt.Sprintf("City Venue %d", i), City: c.city, State: c.state}},
			Artists:   []CreateShowArtist{{Name: fmt.Sprintf("City Artist %d", i), IsHeadliner: boolPtr(true)}},
			SubmittedByUserID: &user.ID,
			SubmitterIsAdmin:  true,
		}
		_, err := suite.showService.CreateShow(req)
		suite.Require().NoError(err)
	}

	results, err := suite.showService.GetShowCities("UTC")

	suite.Require().NoError(err)
	suite.GreaterOrEqual(len(results), 2)

	// Phoenix should have more shows
	cityMap := make(map[string]int)
	for _, r := range results {
		cityMap[r.City] = r.ShowCount
	}
	suite.Equal(2, cityMap["Phoenix"])
	suite.Equal(1, cityMap["Tucson"])
}

// =============================================================================
// Group 6: Status Flags
// =============================================================================

func (suite *ShowServiceIntegrationTestSuite) TestSetShowSoldOut() {
	created := suite.createTestShow()

	// Set sold out
	resp, err := suite.showService.SetShowSoldOut(created.ID, true)
	suite.Require().NoError(err)
	suite.True(resp.IsSoldOut)

	// Clear sold out
	resp, err = suite.showService.SetShowSoldOut(created.ID, false)
	suite.Require().NoError(err)
	suite.False(resp.IsSoldOut)
}

func (suite *ShowServiceIntegrationTestSuite) TestSetShowCancelled() {
	created := suite.createTestShow()

	// Set cancelled
	resp, err := suite.showService.SetShowCancelled(created.ID, true)
	suite.Require().NoError(err)
	suite.True(resp.IsCancelled)

	// Clear cancelled
	resp, err = suite.showService.SetShowCancelled(created.ID, false)
	suite.Require().NoError(err)
	suite.False(resp.IsCancelled)
}
