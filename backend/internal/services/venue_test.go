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

func TestNewVenueService(t *testing.T) {
	venueService := NewVenueService(nil)
	assert.NotNil(t, venueService)
}

func TestVenueService_NilDatabase(t *testing.T) {
	svc := &VenueService{db: nil}

	t.Run("CreateVenue", func(t *testing.T) {
		resp, err := svc.CreateVenue(&CreateVenueRequest{Name: "Test", City: "Phoenix", State: "AZ"}, false)
		assert.Error(t, err)
		assert.Equal(t, "database not initialized", err.Error())
		assert.Nil(t, resp)
	})

	t.Run("GetVenue", func(t *testing.T) {
		resp, err := svc.GetVenue(1)
		assert.Error(t, err)
		assert.Equal(t, "database not initialized", err.Error())
		assert.Nil(t, resp)
	})

	t.Run("GetVenueBySlug", func(t *testing.T) {
		resp, err := svc.GetVenueBySlug("test-slug")
		assert.Error(t, err)
		assert.Equal(t, "database not initialized", err.Error())
		assert.Nil(t, resp)
	})

	t.Run("GetVenues", func(t *testing.T) {
		resp, err := svc.GetVenues(nil)
		assert.Error(t, err)
		assert.Equal(t, "database not initialized", err.Error())
		assert.Nil(t, resp)
	})

	t.Run("UpdateVenue", func(t *testing.T) {
		resp, err := svc.UpdateVenue(1, map[string]interface{}{"name": "x"})
		assert.Error(t, err)
		assert.Equal(t, "database not initialized", err.Error())
		assert.Nil(t, resp)
	})

	t.Run("DeleteVenue", func(t *testing.T) {
		err := svc.DeleteVenue(1)
		assert.Error(t, err)
		assert.Equal(t, "database not initialized", err.Error())
	})

	t.Run("SearchVenues", func(t *testing.T) {
		resp, err := svc.SearchVenues("test")
		assert.Error(t, err)
		assert.Equal(t, "database not initialized", err.Error())
		assert.Nil(t, resp)
	})

	t.Run("FindOrCreateVenue", func(t *testing.T) {
		venue, created, err := svc.FindOrCreateVenue("Test", "Phoenix", "AZ", nil, nil, nil, false)
		assert.Error(t, err)
		assert.Equal(t, "database not initialized", err.Error())
		assert.Nil(t, venue)
		assert.False(t, created)
	})

	t.Run("VerifyVenue", func(t *testing.T) {
		resp, err := svc.VerifyVenue(1)
		assert.Error(t, err)
		assert.Equal(t, "database not initialized", err.Error())
		assert.Nil(t, resp)
	})

	t.Run("GetVenuesWithShowCounts", func(t *testing.T) {
		resp, total, err := svc.GetVenuesWithShowCounts(VenueListFilters{}, 10, 0)
		assert.Error(t, err)
		assert.Equal(t, "database not initialized", err.Error())
		assert.Nil(t, resp)
		assert.Zero(t, total)
	})

	t.Run("GetShowsForVenue", func(t *testing.T) {
		resp, total, err := svc.GetShowsForVenue(1, "UTC", 10, "upcoming")
		assert.Error(t, err)
		assert.Equal(t, "database not initialized", err.Error())
		assert.Nil(t, resp)
		assert.Zero(t, total)
	})

	t.Run("GetUpcomingShowsForVenue", func(t *testing.T) {
		resp, total, err := svc.GetUpcomingShowsForVenue(1, "UTC", 10)
		assert.Error(t, err)
		assert.Equal(t, "database not initialized", err.Error())
		assert.Nil(t, resp)
		assert.Zero(t, total)
	})

	t.Run("GetVenueCities", func(t *testing.T) {
		resp, err := svc.GetVenueCities()
		assert.Error(t, err)
		assert.Equal(t, "database not initialized", err.Error())
		assert.Nil(t, resp)
	})

	t.Run("CreatePendingVenueEdit", func(t *testing.T) {
		resp, err := svc.CreatePendingVenueEdit(1, 1, &VenueEditRequest{})
		assert.Error(t, err)
		assert.Equal(t, "database not initialized", err.Error())
		assert.Nil(t, resp)
	})

	t.Run("GetPendingEditForVenue", func(t *testing.T) {
		resp, err := svc.GetPendingEditForVenue(1, 1)
		assert.Error(t, err)
		assert.Equal(t, "database not initialized", err.Error())
		assert.Nil(t, resp)
	})

	t.Run("GetPendingVenueEdits", func(t *testing.T) {
		resp, total, err := svc.GetPendingVenueEdits(10, 0)
		assert.Error(t, err)
		assert.Equal(t, "database not initialized", err.Error())
		assert.Nil(t, resp)
		assert.Zero(t, total)
	})

	t.Run("GetPendingVenueEdit", func(t *testing.T) {
		resp, err := svc.GetPendingVenueEdit(1)
		assert.Error(t, err)
		assert.Equal(t, "database not initialized", err.Error())
		assert.Nil(t, resp)
	})

	t.Run("ApproveVenueEdit", func(t *testing.T) {
		resp, err := svc.ApproveVenueEdit(1, 1)
		assert.Error(t, err)
		assert.Equal(t, "database not initialized", err.Error())
		assert.Nil(t, resp)
	})

	t.Run("RejectVenueEdit", func(t *testing.T) {
		resp, err := svc.RejectVenueEdit(1, 1, "reason")
		assert.Error(t, err)
		assert.Equal(t, "database not initialized", err.Error())
		assert.Nil(t, resp)
	})

	t.Run("CancelPendingVenueEdit", func(t *testing.T) {
		err := svc.CancelPendingVenueEdit(1, 1)
		assert.Error(t, err)
		assert.Equal(t, "database not initialized", err.Error())
	})

	t.Run("GetVenueModel", func(t *testing.T) {
		resp, err := svc.GetVenueModel(1)
		assert.Error(t, err)
		assert.Equal(t, "database not initialized", err.Error())
		assert.Nil(t, resp)
	})

	t.Run("GetUnverifiedVenues", func(t *testing.T) {
		resp, total, err := svc.GetUnverifiedVenues(10, 0)
		assert.Error(t, err)
		assert.Equal(t, "database not initialized", err.Error())
		assert.Nil(t, resp)
		assert.Zero(t, total)
	})
}

// =============================================================================
// INTEGRATION TESTS (With Real Database)
// =============================================================================

type VenueServiceIntegrationTestSuite struct {
	suite.Suite
	container    testcontainers.Container
	db           *gorm.DB
	venueService *VenueService
	ctx          context.Context
}

func (suite *VenueServiceIntegrationTestSuite) SetupSuite() {
	suite.ctx = context.Background()

	// Start PostgreSQL container
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

	// Run migrations
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
		"000032_add_favorite_cities.up.sql",
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

	suite.venueService = &VenueService{db: db}
}

func (suite *VenueServiceIntegrationTestSuite) TearDownSuite() {
	if suite.container != nil {
		if err := suite.container.Terminate(suite.ctx); err != nil {
			suite.T().Logf("failed to terminate container: %v", err)
		}
	}
}

// TearDownTest cleans up data between tests for isolation
func (suite *VenueServiceIntegrationTestSuite) TearDownTest() {
	sqlDB, err := suite.db.DB()
	suite.Require().NoError(err)
	// Delete in FK-safe order
	_, _ = sqlDB.Exec("DELETE FROM pending_venue_edits")
	_, _ = sqlDB.Exec("DELETE FROM show_artists")
	_, _ = sqlDB.Exec("DELETE FROM show_venues")
	_, _ = sqlDB.Exec("DELETE FROM shows")
	_, _ = sqlDB.Exec("DELETE FROM artists")
	_, _ = sqlDB.Exec("DELETE FROM venues")
	_, _ = sqlDB.Exec("DELETE FROM users")
}

func TestVenueServiceIntegrationTestSuite(t *testing.T) {
	suite.Run(t, new(VenueServiceIntegrationTestSuite))
}

// =============================================================================
// HELPERS
// =============================================================================

func (suite *VenueServiceIntegrationTestSuite) createTestUser() *models.User {
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

func (suite *VenueServiceIntegrationTestSuite) createTestVenue(name, city, state string, verified bool) *models.Venue {
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

func (suite *VenueServiceIntegrationTestSuite) createApprovedShow(venueID, userID uint) *models.Show {
	show := &models.Show{
		Title:       fmt.Sprintf("Show-%d", time.Now().UnixNano()),
		EventDate:   time.Now().UTC().AddDate(0, 0, 7),
		City:        stringPtr("Phoenix"),
		State:       stringPtr("AZ"),
		Status:      models.ShowStatusApproved,
		SubmittedBy: &userID,
	}
	err := suite.db.Create(show).Error
	suite.Require().NoError(err)

	// Link show to venue
	showVenue := &models.ShowVenue{
		ShowID:  show.ID,
		VenueID: venueID,
	}
	err = suite.db.Create(showVenue).Error
	suite.Require().NoError(err)

	return show
}

// =============================================================================
// Group 1: CreateVenue
// =============================================================================

func (suite *VenueServiceIntegrationTestSuite) TestCreateVenue_Success() {
	req := &CreateVenueRequest{
		Name:    "Valley Bar",
		City:    "Phoenix",
		State:   "AZ",
		Address: stringPtr("130 N Central Ave"),
		Zipcode: stringPtr("85004"),
	}

	resp, err := suite.venueService.CreateVenue(req, true)

	suite.Require().NoError(err)
	suite.Require().NotNil(resp)
	suite.NotZero(resp.ID)
	suite.Equal("Valley Bar", resp.Name)
	suite.Equal("Phoenix", resp.City)
	suite.Equal("AZ", resp.State)
	suite.NotEmpty(resp.Slug)
	suite.True(resp.Verified)
	suite.Equal("130 N Central Ave", *resp.Address)
	suite.Equal("85004", *resp.Zipcode)
}

func (suite *VenueServiceIntegrationTestSuite) TestCreateVenue_AdminAutoVerified() {
	req := &CreateVenueRequest{
		Name:  "Admin Venue",
		City:  "Tempe",
		State: "AZ",
	}

	resp, err := suite.venueService.CreateVenue(req, true)

	suite.Require().NoError(err)
	suite.True(resp.Verified)
}

func (suite *VenueServiceIntegrationTestSuite) TestCreateVenue_NonAdminUnverified() {
	req := &CreateVenueRequest{
		Name:  "User Venue",
		City:  "Mesa",
		State: "AZ",
	}

	resp, err := suite.venueService.CreateVenue(req, false)

	suite.Require().NoError(err)
	suite.False(resp.Verified)
}

func (suite *VenueServiceIntegrationTestSuite) TestCreateVenue_DuplicateNameCity_Fails() {
	req := &CreateVenueRequest{
		Name:  "The Rebel Lounge",
		City:  "Phoenix",
		State: "AZ",
	}
	_, err := suite.venueService.CreateVenue(req, true)
	suite.Require().NoError(err)

	// Same name, same city (case insensitive)
	req2 := &CreateVenueRequest{
		Name:  "the rebel lounge",
		City:  "phoenix",
		State: "AZ",
	}
	_, err = suite.venueService.CreateVenue(req2, true)

	suite.Require().Error(err)
	suite.Contains(err.Error(), "already exists")
}

func (suite *VenueServiceIntegrationTestSuite) TestCreateVenue_SameNameDifferentCity_OK() {
	req := &CreateVenueRequest{
		Name:  "The Local",
		City:  "Phoenix",
		State: "AZ",
	}
	_, err := suite.venueService.CreateVenue(req, true)
	suite.Require().NoError(err)

	req2 := &CreateVenueRequest{
		Name:  "The Local",
		City:  "Tucson",
		State: "AZ",
	}
	resp, err := suite.venueService.CreateVenue(req2, true)

	suite.Require().NoError(err)
	suite.Equal("Tucson", resp.City)
}

// =============================================================================
// Group 2: GetVenue / GetVenueBySlug
// =============================================================================

func (suite *VenueServiceIntegrationTestSuite) TestGetVenue_Success() {
	req := &CreateVenueRequest{
		Name:  "Get Test Venue",
		City:  "Phoenix",
		State: "AZ",
	}
	created, err := suite.venueService.CreateVenue(req, true)
	suite.Require().NoError(err)

	resp, err := suite.venueService.GetVenue(created.ID)

	suite.Require().NoError(err)
	suite.Equal(created.ID, resp.ID)
	suite.Equal("Get Test Venue", resp.Name)
	suite.Equal(created.Slug, resp.Slug)
}

func (suite *VenueServiceIntegrationTestSuite) TestGetVenue_NotFound() {
	resp, err := suite.venueService.GetVenue(99999)

	suite.Require().Error(err)
	suite.Nil(resp)
	var venueErr *apperrors.VenueError
	suite.ErrorAs(err, &venueErr)
	suite.Equal(apperrors.CodeVenueNotFound, venueErr.Code)
}

func (suite *VenueServiceIntegrationTestSuite) TestGetVenueBySlug_Success() {
	req := &CreateVenueRequest{
		Name:  "Slug Test Venue",
		City:  "Phoenix",
		State: "AZ",
	}
	created, err := suite.venueService.CreateVenue(req, true)
	suite.Require().NoError(err)

	resp, err := suite.venueService.GetVenueBySlug(created.Slug)

	suite.Require().NoError(err)
	suite.Equal(created.ID, resp.ID)
	suite.Equal(created.Slug, resp.Slug)
}

func (suite *VenueServiceIntegrationTestSuite) TestGetVenueBySlug_NotFound() {
	resp, err := suite.venueService.GetVenueBySlug("nonexistent-slug-xyz")

	suite.Require().Error(err)
	suite.Nil(resp)
	var venueErr *apperrors.VenueError
	suite.ErrorAs(err, &venueErr)
	suite.Equal(apperrors.CodeVenueNotFound, venueErr.Code)
}

// =============================================================================
// Group 3: GetVenues filtering
// =============================================================================

func (suite *VenueServiceIntegrationTestSuite) TestGetVenues_FilterByCity() {
	suite.venueService.CreateVenue(&CreateVenueRequest{Name: "PHX Venue", City: "Phoenix", State: "AZ"}, true)
	suite.venueService.CreateVenue(&CreateVenueRequest{Name: "TUC Venue", City: "Tucson", State: "AZ"}, true)

	resp, err := suite.venueService.GetVenues(map[string]interface{}{"city": "Phoenix"})

	suite.Require().NoError(err)
	suite.Require().Len(resp, 1)
	suite.Equal("PHX Venue", resp[0].Name)
}

func (suite *VenueServiceIntegrationTestSuite) TestGetVenues_FilterByState() {
	suite.venueService.CreateVenue(&CreateVenueRequest{Name: "AZ Venue", City: "Phoenix", State: "AZ"}, true)
	suite.venueService.CreateVenue(&CreateVenueRequest{Name: "CA Venue", City: "Los Angeles", State: "CA"}, true)

	resp, err := suite.venueService.GetVenues(map[string]interface{}{"state": "CA"})

	suite.Require().NoError(err)
	suite.Require().Len(resp, 1)
	suite.Equal("CA Venue", resp[0].Name)
}

func (suite *VenueServiceIntegrationTestSuite) TestGetVenues_FilterByName() {
	suite.venueService.CreateVenue(&CreateVenueRequest{Name: "Crescent Ballroom", City: "Phoenix", State: "AZ"}, true)
	suite.venueService.CreateVenue(&CreateVenueRequest{Name: "Valley Bar", City: "Phoenix", State: "AZ"}, true)

	resp, err := suite.venueService.GetVenues(map[string]interface{}{"name": "crescent"})

	suite.Require().NoError(err)
	suite.Require().Len(resp, 1)
	suite.Equal("Crescent Ballroom", resp[0].Name)
}

// =============================================================================
// Group 4: UpdateVenue
// =============================================================================

func (suite *VenueServiceIntegrationTestSuite) TestUpdateVenue_BasicFields() {
	created, err := suite.venueService.CreateVenue(&CreateVenueRequest{
		Name:  "Original Name",
		City:  "Phoenix",
		State: "AZ",
	}, true)
	suite.Require().NoError(err)

	resp, err := suite.venueService.UpdateVenue(created.ID, map[string]interface{}{
		"name":    "Updated Name",
		"address": "123 Main St",
	})

	suite.Require().NoError(err)
	suite.Equal("Updated Name", resp.Name)
	suite.Equal("123 Main St", *resp.Address)
}

func (suite *VenueServiceIntegrationTestSuite) TestUpdateVenue_NotFound() {
	resp, err := suite.venueService.UpdateVenue(99999, map[string]interface{}{"name": "x"})

	suite.Require().Error(err)
	suite.Nil(resp)
	var venueErr *apperrors.VenueError
	suite.ErrorAs(err, &venueErr)
	suite.Equal(apperrors.CodeVenueNotFound, venueErr.Code)
}

func (suite *VenueServiceIntegrationTestSuite) TestUpdateVenue_DuplicateNameCity_Fails() {
	suite.venueService.CreateVenue(&CreateVenueRequest{
		Name:  "Existing Venue",
		City:  "Phoenix",
		State: "AZ",
	}, true)
	other, err := suite.venueService.CreateVenue(&CreateVenueRequest{
		Name:  "Other Venue",
		City:  "Phoenix",
		State: "AZ",
	}, true)
	suite.Require().NoError(err)

	// Try to rename "Other Venue" to "Existing Venue" in same city
	_, err = suite.venueService.UpdateVenue(other.ID, map[string]interface{}{"name": "Existing Venue"})

	suite.Require().Error(err)
	suite.Contains(err.Error(), "already exists")
}

func (suite *VenueServiceIntegrationTestSuite) TestUpdateVenue_SameNameSameVenue_OK() {
	created, err := suite.venueService.CreateVenue(&CreateVenueRequest{
		Name:  "Keep My Name",
		City:  "Phoenix",
		State: "AZ",
	}, true)
	suite.Require().NoError(err)

	// Update address while keeping the same name — should not conflict with self
	resp, err := suite.venueService.UpdateVenue(created.ID, map[string]interface{}{
		"name":    "Keep My Name",
		"address": "456 New St",
	})

	suite.Require().NoError(err)
	suite.Equal("Keep My Name", resp.Name)
	suite.Equal("456 New St", *resp.Address)
}

// =============================================================================
// Group 5: DeleteVenue
// =============================================================================

func (suite *VenueServiceIntegrationTestSuite) TestDeleteVenue_Success() {
	created, err := suite.venueService.CreateVenue(&CreateVenueRequest{
		Name:  "Delete Me",
		City:  "Phoenix",
		State: "AZ",
	}, true)
	suite.Require().NoError(err)

	err = suite.venueService.DeleteVenue(created.ID)

	suite.Require().NoError(err)

	// Verify it's gone
	_, err = suite.venueService.GetVenue(created.ID)
	suite.Error(err)
}

func (suite *VenueServiceIntegrationTestSuite) TestDeleteVenue_NotFound() {
	err := suite.venueService.DeleteVenue(99999)

	suite.Require().Error(err)
	var venueErr *apperrors.VenueError
	suite.ErrorAs(err, &venueErr)
	suite.Equal(apperrors.CodeVenueNotFound, venueErr.Code)
}

func (suite *VenueServiceIntegrationTestSuite) TestDeleteVenue_HasShows_Fails() {
	venue := suite.createTestVenue("Show Venue", "Phoenix", "AZ", true)
	user := suite.createTestUser()
	suite.createApprovedShow(venue.ID, user.ID)

	err := suite.venueService.DeleteVenue(venue.ID)

	suite.Require().Error(err)
	var venueErr *apperrors.VenueError
	suite.ErrorAs(err, &venueErr)
	suite.Equal(apperrors.CodeVenueHasShows, venueErr.Code)
}

// =============================================================================
// Group 6: SearchVenues
// =============================================================================

func (suite *VenueServiceIntegrationTestSuite) TestSearchVenues_EmptyQuery() {
	suite.venueService.CreateVenue(&CreateVenueRequest{Name: "Some Venue", City: "Phoenix", State: "AZ"}, true)

	resp, err := suite.venueService.SearchVenues("")

	suite.Require().NoError(err)
	suite.Empty(resp)
}

func (suite *VenueServiceIntegrationTestSuite) TestSearchVenues_ShortQuery_PrefixMatch() {
	suite.venueService.CreateVenue(&CreateVenueRequest{Name: "Valley Bar", City: "Phoenix", State: "AZ"}, true)
	suite.venueService.CreateVenue(&CreateVenueRequest{Name: "Crescent Ballroom", City: "Phoenix", State: "AZ"}, true)

	resp, err := suite.venueService.SearchVenues("Va")

	suite.Require().NoError(err)
	suite.Require().Len(resp, 1)
	suite.Equal("Valley Bar", resp[0].Name)
}

func (suite *VenueServiceIntegrationTestSuite) TestSearchVenues_LongQuery_TrigramMatch() {
	suite.venueService.CreateVenue(&CreateVenueRequest{Name: "Crescent Ballroom", City: "Phoenix", State: "AZ"}, true)
	suite.venueService.CreateVenue(&CreateVenueRequest{Name: "The Rebel Lounge", City: "Phoenix", State: "AZ"}, true)

	resp, err := suite.venueService.SearchVenues("Crescent")

	suite.Require().NoError(err)
	suite.Require().NotEmpty(resp)
	suite.Equal("Crescent Ballroom", resp[0].Name)
}

func (suite *VenueServiceIntegrationTestSuite) TestSearchVenues_NoMatch() {
	suite.venueService.CreateVenue(&CreateVenueRequest{Name: "Real Venue", City: "Phoenix", State: "AZ"}, true)

	resp, err := suite.venueService.SearchVenues("zzzznonexistent")

	suite.Require().NoError(err)
	suite.Empty(resp)
}

// =============================================================================
// Group 7: FindOrCreateVenue
// =============================================================================

func (suite *VenueServiceIntegrationTestSuite) TestFindOrCreateVenue_CreatesNew() {
	venue, created, err := suite.venueService.FindOrCreateVenue("Brand New Place", "Phoenix", "AZ", nil, nil, nil, false)

	suite.Require().NoError(err)
	suite.True(created)
	suite.NotNil(venue)
	suite.Equal("Brand New Place", venue.Name)
	suite.NotNil(venue.Slug)
	suite.False(venue.Verified)
}

func (suite *VenueServiceIntegrationTestSuite) TestFindOrCreateVenue_FindsExisting() {
	// Create a venue first
	suite.venueService.CreateVenue(&CreateVenueRequest{
		Name:  "Existing Place",
		City:  "Phoenix",
		State: "AZ",
	}, true)

	venue, created, err := suite.venueService.FindOrCreateVenue("Existing Place", "Phoenix", "AZ", nil, nil, nil, false)

	suite.Require().NoError(err)
	suite.False(created)
	suite.Equal("Existing Place", venue.Name)
}

func (suite *VenueServiceIntegrationTestSuite) TestFindOrCreateVenue_CaseInsensitive() {
	suite.venueService.CreateVenue(&CreateVenueRequest{
		Name:  "The Venue",
		City:  "Phoenix",
		State: "AZ",
	}, true)

	venue, created, err := suite.venueService.FindOrCreateVenue("the venue", "phoenix", "AZ", nil, nil, nil, false)

	suite.Require().NoError(err)
	suite.False(created)
	suite.Equal("The Venue", venue.Name)
}

func (suite *VenueServiceIntegrationTestSuite) TestFindOrCreateVenue_AdminVerified() {
	venue, created, err := suite.venueService.FindOrCreateVenue("Admin New Place", "Phoenix", "AZ", nil, nil, nil, true)

	suite.Require().NoError(err)
	suite.True(created)
	suite.True(venue.Verified)
}

func (suite *VenueServiceIntegrationTestSuite) TestFindOrCreateVenue_BackfillsSlug() {
	// Create a venue directly in DB without a slug
	venue := &models.Venue{
		Name:     "No Slug Venue",
		City:     "Phoenix",
		State:    "AZ",
		Verified: true,
	}
	suite.db.Create(venue)

	found, created, err := suite.venueService.FindOrCreateVenue("No Slug Venue", "Phoenix", "AZ", nil, nil, nil, false)

	suite.Require().NoError(err)
	suite.False(created)
	suite.NotNil(found.Slug, "slug should be backfilled")
	suite.NotEmpty(*found.Slug)
}

func (suite *VenueServiceIntegrationTestSuite) TestFindOrCreateVenue_ValidationErrors() {
	_, _, err := suite.venueService.FindOrCreateVenue("", "Phoenix", "AZ", nil, nil, nil, false)
	suite.Require().Error(err)
	suite.Contains(err.Error(), "name is required")

	_, _, err = suite.venueService.FindOrCreateVenue("Venue", "", "AZ", nil, nil, nil, false)
	suite.Require().Error(err)
	suite.Contains(err.Error(), "city is required")

	_, _, err = suite.venueService.FindOrCreateVenue("Venue", "Phoenix", "", nil, nil, nil, false)
	suite.Require().Error(err)
	suite.Contains(err.Error(), "state is required")
}

// =============================================================================
// Group 8: VerifyVenue
// =============================================================================

func (suite *VenueServiceIntegrationTestSuite) TestVerifyVenue_Success() {
	created, err := suite.venueService.CreateVenue(&CreateVenueRequest{
		Name:  "Unverified Spot",
		City:  "Phoenix",
		State: "AZ",
	}, false) // non-admin = unverified
	suite.Require().NoError(err)
	suite.False(created.Verified)

	resp, err := suite.venueService.VerifyVenue(created.ID)

	suite.Require().NoError(err)
	suite.True(resp.Verified)
}

func (suite *VenueServiceIntegrationTestSuite) TestVerifyVenue_AlreadyVerified() {
	created, err := suite.venueService.CreateVenue(&CreateVenueRequest{
		Name:  "Already Verified",
		City:  "Phoenix",
		State: "AZ",
	}, true)
	suite.Require().NoError(err)

	resp, err := suite.venueService.VerifyVenue(created.ID)

	suite.Require().NoError(err)
	suite.True(resp.Verified)
}

func (suite *VenueServiceIntegrationTestSuite) TestVerifyVenue_NotFound() {
	resp, err := suite.venueService.VerifyVenue(99999)

	suite.Require().Error(err)
	suite.Nil(resp)
	var venueErr *apperrors.VenueError
	suite.ErrorAs(err, &venueErr)
	suite.Equal(apperrors.CodeVenueNotFound, venueErr.Code)
}

// =============================================================================
// Group 9: GetVenuesWithShowCounts
// =============================================================================

func (suite *VenueServiceIntegrationTestSuite) TestGetVenuesWithShowCounts_OnlyVerified() {
	suite.createTestVenue("Verified Venue", "Phoenix", "AZ", true)
	suite.createTestVenue("Unverified Venue", "Phoenix", "AZ", false)

	resp, total, err := suite.venueService.GetVenuesWithShowCounts(VenueListFilters{}, 10, 0)

	suite.Require().NoError(err)
	suite.Equal(int64(1), total)
	suite.Require().Len(resp, 1)
	suite.Equal("Verified Venue", resp[0].Name)
}

func (suite *VenueServiceIntegrationTestSuite) TestGetVenuesWithShowCounts_SortByCount() {
	venue1 := suite.createTestVenue("Few Shows", "Phoenix", "AZ", true)
	venue2 := suite.createTestVenue("Many Shows", "Phoenix", "AZ", true)
	user := suite.createTestUser()

	// Give venue2 more upcoming shows
	suite.createApprovedShow(venue2.ID, user.ID)
	suite.createApprovedShow(venue2.ID, user.ID)
	suite.createApprovedShow(venue1.ID, user.ID)

	resp, _, err := suite.venueService.GetVenuesWithShowCounts(VenueListFilters{}, 10, 0)

	suite.Require().NoError(err)
	suite.Require().Len(resp, 2)
	// venue2 should come first (more shows)
	suite.Equal("Many Shows", resp[0].Name)
	suite.Equal(2, resp[0].UpcomingShowCount)
	suite.Equal("Few Shows", resp[1].Name)
	suite.Equal(1, resp[1].UpcomingShowCount)
}

func (suite *VenueServiceIntegrationTestSuite) TestGetVenuesWithShowCounts_FilterByCity() {
	suite.createTestVenue("PHX Counted", "Phoenix", "AZ", true)
	suite.createTestVenue("TUC Counted", "Tucson", "AZ", true)

	resp, total, err := suite.venueService.GetVenuesWithShowCounts(VenueListFilters{City: "Phoenix"}, 10, 0)

	suite.Require().NoError(err)
	suite.Equal(int64(1), total)
	suite.Require().Len(resp, 1)
	suite.Equal("PHX Counted", resp[0].Name)
}

func (suite *VenueServiceIntegrationTestSuite) TestGetVenuesWithShowCounts_Pagination() {
	for i := 0; i < 5; i++ {
		suite.createTestVenue(fmt.Sprintf("Paginated Venue %d", i), "Phoenix", "AZ", true)
	}

	// Page 1
	resp1, total, err := suite.venueService.GetVenuesWithShowCounts(VenueListFilters{}, 2, 0)
	suite.Require().NoError(err)
	suite.Equal(int64(5), total)
	suite.Len(resp1, 2)

	// Page 2
	resp2, _, err := suite.venueService.GetVenuesWithShowCounts(VenueListFilters{}, 2, 2)
	suite.Require().NoError(err)
	suite.Len(resp2, 2)

	// Page 3 (last)
	resp3, _, err := suite.venueService.GetVenuesWithShowCounts(VenueListFilters{}, 2, 4)
	suite.Require().NoError(err)
	suite.Len(resp3, 1)

	// No overlap between pages
	suite.NotEqual(resp1[0].ID, resp2[0].ID)
	suite.NotEqual(resp2[0].ID, resp3[0].ID)
}

// =============================================================================
// Group 10: GetShowsForVenue
// =============================================================================

func (suite *VenueServiceIntegrationTestSuite) TestGetShowsForVenue_Upcoming() {
	venue := suite.createTestVenue("Upcoming Venue", "Phoenix", "AZ", true)
	user := suite.createTestUser()

	// Create a future show
	futureShow := &models.Show{
		Title:       "Future Show",
		EventDate:   time.Now().UTC().AddDate(0, 0, 7),
		City:        stringPtr("Phoenix"),
		State:       stringPtr("AZ"),
		Status:      models.ShowStatusApproved,
		SubmittedBy: &user.ID,
	}
	suite.db.Create(futureShow)
	suite.db.Create(&models.ShowVenue{ShowID: futureShow.ID, VenueID: venue.ID})

	// Create a past show
	pastShow := &models.Show{
		Title:       "Past Show",
		EventDate:   time.Now().UTC().AddDate(0, 0, -7),
		City:        stringPtr("Phoenix"),
		State:       stringPtr("AZ"),
		Status:      models.ShowStatusApproved,
		SubmittedBy: &user.ID,
	}
	suite.db.Create(pastShow)
	suite.db.Create(&models.ShowVenue{ShowID: pastShow.ID, VenueID: venue.ID})

	resp, total, err := suite.venueService.GetShowsForVenue(venue.ID, "UTC", 10, "upcoming")

	suite.Require().NoError(err)
	suite.Equal(int64(1), total)
	suite.Require().Len(resp, 1)
	suite.Equal("Future Show", resp[0].Title)
}

func (suite *VenueServiceIntegrationTestSuite) TestGetShowsForVenue_Past() {
	venue := suite.createTestVenue("Past Venue", "Phoenix", "AZ", true)
	user := suite.createTestUser()

	// Create a future show
	futureShow := &models.Show{
		Title:       "Future Show",
		EventDate:   time.Now().UTC().AddDate(0, 0, 7),
		City:        stringPtr("Phoenix"),
		State:       stringPtr("AZ"),
		Status:      models.ShowStatusApproved,
		SubmittedBy: &user.ID,
	}
	suite.db.Create(futureShow)
	suite.db.Create(&models.ShowVenue{ShowID: futureShow.ID, VenueID: venue.ID})

	// Create a past show
	pastShow := &models.Show{
		Title:       "Past Show",
		EventDate:   time.Now().UTC().AddDate(0, 0, -7),
		City:        stringPtr("Phoenix"),
		State:       stringPtr("AZ"),
		Status:      models.ShowStatusApproved,
		SubmittedBy: &user.ID,
	}
	suite.db.Create(pastShow)
	suite.db.Create(&models.ShowVenue{ShowID: pastShow.ID, VenueID: venue.ID})

	resp, total, err := suite.venueService.GetShowsForVenue(venue.ID, "UTC", 10, "past")

	suite.Require().NoError(err)
	suite.Equal(int64(1), total)
	suite.Require().Len(resp, 1)
	suite.Equal("Past Show", resp[0].Title)
}

func (suite *VenueServiceIntegrationTestSuite) TestGetShowsForVenue_All() {
	venue := suite.createTestVenue("All Shows Venue", "Phoenix", "AZ", true)
	user := suite.createTestUser()

	for _, offset := range []int{-7, 7} {
		show := &models.Show{
			Title:       fmt.Sprintf("Show %+d days", offset),
			EventDate:   time.Now().UTC().AddDate(0, 0, offset),
			City:        stringPtr("Phoenix"),
			State:       stringPtr("AZ"),
			Status:      models.ShowStatusApproved,
			SubmittedBy: &user.ID,
		}
		suite.db.Create(show)
		suite.db.Create(&models.ShowVenue{ShowID: show.ID, VenueID: venue.ID})
	}

	resp, total, err := suite.venueService.GetShowsForVenue(venue.ID, "UTC", 10, "all")

	suite.Require().NoError(err)
	suite.Equal(int64(2), total)
	suite.Len(resp, 2)
}

func (suite *VenueServiceIntegrationTestSuite) TestGetShowsForVenue_NotFound() {
	_, _, err := suite.venueService.GetShowsForVenue(99999, "UTC", 10, "upcoming")

	suite.Require().Error(err)
	var venueErr *apperrors.VenueError
	suite.ErrorAs(err, &venueErr)
	suite.Equal(apperrors.CodeVenueNotFound, venueErr.Code)
}

// =============================================================================
// Group 11: GetVenueCities
// =============================================================================

func (suite *VenueServiceIntegrationTestSuite) TestGetVenueCities_Success() {
	suite.createTestVenue("PHX Venue 1", "Phoenix", "AZ", true)
	suite.createTestVenue("PHX Venue 2", "Phoenix", "AZ", true)
	suite.createTestVenue("TUC Venue", "Tucson", "AZ", true)

	resp, err := suite.venueService.GetVenueCities()

	suite.Require().NoError(err)
	suite.Require().Len(resp, 2)
	// Sorted by count desc — Phoenix has 2, Tucson has 1
	suite.Equal("Phoenix", resp[0].City)
	suite.Equal(2, resp[0].VenueCount)
	suite.Equal("Tucson", resp[1].City)
	suite.Equal(1, resp[1].VenueCount)
}

func (suite *VenueServiceIntegrationTestSuite) TestGetVenueCities_OnlyVerified() {
	suite.createTestVenue("Verified City Venue", "Phoenix", "AZ", true)
	suite.createTestVenue("Unverified City Venue", "Tucson", "AZ", false)

	resp, err := suite.venueService.GetVenueCities()

	suite.Require().NoError(err)
	suite.Require().Len(resp, 1)
	suite.Equal("Phoenix", resp[0].City)
}

// =============================================================================
// Group 12: Pending Venue Edits
// =============================================================================

func (suite *VenueServiceIntegrationTestSuite) TestCreatePendingVenueEdit_Success() {
	venue, _ := suite.venueService.CreateVenue(&CreateVenueRequest{
		Name:  "Edit Target",
		City:  "Phoenix",
		State: "AZ",
	}, true)
	user := suite.createTestUser()

	resp, err := suite.venueService.CreatePendingVenueEdit(venue.ID, user.ID, &VenueEditRequest{
		Name:    stringPtr("New Name"),
		Address: stringPtr("789 Edit St"),
	})

	suite.Require().NoError(err)
	suite.Require().NotNil(resp)
	suite.Equal(venue.ID, resp.VenueID)
	suite.Equal(user.ID, resp.SubmittedBy)
	suite.Equal(models.VenueEditStatusPending, resp.Status)
	suite.Equal("New Name", *resp.Name)
	suite.Equal("789 Edit St", *resp.Address)
}

func (suite *VenueServiceIntegrationTestSuite) TestCreatePendingVenueEdit_DuplicatePending_Fails() {
	venue, _ := suite.venueService.CreateVenue(&CreateVenueRequest{
		Name:  "Dup Edit Target",
		City:  "Phoenix",
		State: "AZ",
	}, true)
	user := suite.createTestUser()

	_, err := suite.venueService.CreatePendingVenueEdit(venue.ID, user.ID, &VenueEditRequest{
		Name: stringPtr("First Edit"),
	})
	suite.Require().NoError(err)

	_, err = suite.venueService.CreatePendingVenueEdit(venue.ID, user.ID, &VenueEditRequest{
		Name: stringPtr("Second Edit"),
	})

	suite.Require().Error(err)
	var venueErr *apperrors.VenueError
	suite.ErrorAs(err, &venueErr)
	suite.Equal(apperrors.CodeVenuePendingEditExists, venueErr.Code)
}

func (suite *VenueServiceIntegrationTestSuite) TestCreatePendingVenueEdit_VenueNotFound() {
	user := suite.createTestUser()

	_, err := suite.venueService.CreatePendingVenueEdit(99999, user.ID, &VenueEditRequest{
		Name: stringPtr("Edit"),
	})

	suite.Require().Error(err)
	var venueErr *apperrors.VenueError
	suite.ErrorAs(err, &venueErr)
	suite.Equal(apperrors.CodeVenueNotFound, venueErr.Code)
}

func (suite *VenueServiceIntegrationTestSuite) TestGetPendingEditForVenue_Success() {
	venue, _ := suite.venueService.CreateVenue(&CreateVenueRequest{
		Name:  "Get Edit Target",
		City:  "Phoenix",
		State: "AZ",
	}, true)
	user := suite.createTestUser()

	suite.venueService.CreatePendingVenueEdit(venue.ID, user.ID, &VenueEditRequest{
		Name: stringPtr("Proposed Name"),
	})

	resp, err := suite.venueService.GetPendingEditForVenue(venue.ID, user.ID)

	suite.Require().NoError(err)
	suite.Require().NotNil(resp)
	suite.Equal("Proposed Name", *resp.Name)
}

func (suite *VenueServiceIntegrationTestSuite) TestGetPendingEditForVenue_NoPendingEdit() {
	venue, _ := suite.venueService.CreateVenue(&CreateVenueRequest{
		Name:  "No Edits Here",
		City:  "Phoenix",
		State: "AZ",
	}, true)
	user := suite.createTestUser()

	resp, err := suite.venueService.GetPendingEditForVenue(venue.ID, user.ID)

	suite.Require().NoError(err)
	suite.Nil(resp)
}

func (suite *VenueServiceIntegrationTestSuite) TestGetPendingVenueEdits_Pagination() {
	user := suite.createTestUser()

	// Create 3 venues with pending edits
	for i := 0; i < 3; i++ {
		venue, _ := suite.venueService.CreateVenue(&CreateVenueRequest{
			Name:  fmt.Sprintf("Edit Venue %d", i),
			City:  "Phoenix",
			State: "AZ",
		}, true)
		suite.venueService.CreatePendingVenueEdit(venue.ID, user.ID, &VenueEditRequest{
			Name: stringPtr(fmt.Sprintf("New Name %d", i)),
		})
	}

	// Page 1
	resp1, total, err := suite.venueService.GetPendingVenueEdits(2, 0)
	suite.Require().NoError(err)
	suite.Equal(int64(3), total)
	suite.Len(resp1, 2)

	// Page 2
	resp2, _, err := suite.venueService.GetPendingVenueEdits(2, 2)
	suite.Require().NoError(err)
	suite.Len(resp2, 1)
}

func (suite *VenueServiceIntegrationTestSuite) TestApproveVenueEdit_Success() {
	venue, _ := suite.venueService.CreateVenue(&CreateVenueRequest{
		Name:  "Approve Target",
		City:  "Phoenix",
		State: "AZ",
	}, true)
	user := suite.createTestUser()
	reviewer := suite.createTestUser()

	editResp, err := suite.venueService.CreatePendingVenueEdit(venue.ID, user.ID, &VenueEditRequest{
		Name:    stringPtr("Approved Name"),
		Address: stringPtr("111 Approved St"),
	})
	suite.Require().NoError(err)

	venueResp, err := suite.venueService.ApproveVenueEdit(editResp.ID, reviewer.ID)

	suite.Require().NoError(err)
	suite.Equal("Approved Name", venueResp.Name)
	suite.Equal("111 Approved St", *venueResp.Address)

	// Verify the edit status was updated
	editAfter, err := suite.venueService.GetPendingVenueEdit(editResp.ID)
	suite.Require().NoError(err)
	suite.Equal(models.VenueEditStatusApproved, editAfter.Status)
	suite.Equal(reviewer.ID, *editAfter.ReviewedBy)
	suite.NotNil(editAfter.ReviewedAt)
}

func (suite *VenueServiceIntegrationTestSuite) TestRejectVenueEdit_Success() {
	venue, _ := suite.venueService.CreateVenue(&CreateVenueRequest{
		Name:  "Reject Target",
		City:  "Phoenix",
		State: "AZ",
	}, true)
	user := suite.createTestUser()
	reviewer := suite.createTestUser()

	editResp, err := suite.venueService.CreatePendingVenueEdit(venue.ID, user.ID, &VenueEditRequest{
		Name: stringPtr("Bad Name"),
	})
	suite.Require().NoError(err)

	resp, err := suite.venueService.RejectVenueEdit(editResp.ID, reviewer.ID, "Inappropriate name")

	suite.Require().NoError(err)
	suite.Equal(models.VenueEditStatusRejected, resp.Status)
	suite.Equal("Inappropriate name", *resp.RejectionReason)
	suite.Equal(reviewer.ID, *resp.ReviewedBy)

	// Verify venue is unchanged
	venueAfter, err := suite.venueService.GetVenue(venue.ID)
	suite.Require().NoError(err)
	suite.Equal("Reject Target", venueAfter.Name)
}

// =============================================================================
// Group 13: Cancel / Reject edge cases
// =============================================================================

func (suite *VenueServiceIntegrationTestSuite) TestApproveVenueEdit_AlreadyReviewed_Fails() {
	venue, _ := suite.venueService.CreateVenue(&CreateVenueRequest{
		Name:  "Already Reviewed A",
		City:  "Phoenix",
		State: "AZ",
	}, true)
	user := suite.createTestUser()
	reviewer := suite.createTestUser()

	editResp, _ := suite.venueService.CreatePendingVenueEdit(venue.ID, user.ID, &VenueEditRequest{
		Name: stringPtr("X"),
	})
	// Approve it first
	suite.venueService.ApproveVenueEdit(editResp.ID, reviewer.ID)

	// Try to approve again
	_, err := suite.venueService.ApproveVenueEdit(editResp.ID, reviewer.ID)

	suite.Require().Error(err)
	suite.Contains(err.Error(), "already been")
}

func (suite *VenueServiceIntegrationTestSuite) TestRejectVenueEdit_AlreadyReviewed_Fails() {
	venue, _ := suite.venueService.CreateVenue(&CreateVenueRequest{
		Name:  "Already Reviewed R",
		City:  "Phoenix",
		State: "AZ",
	}, true)
	user := suite.createTestUser()
	reviewer := suite.createTestUser()

	editResp, _ := suite.venueService.CreatePendingVenueEdit(venue.ID, user.ID, &VenueEditRequest{
		Name: stringPtr("X"),
	})
	// Reject it first
	suite.venueService.RejectVenueEdit(editResp.ID, reviewer.ID, "bad")

	// Try to reject again
	_, err := suite.venueService.RejectVenueEdit(editResp.ID, reviewer.ID, "bad again")

	suite.Require().Error(err)
	suite.Contains(err.Error(), "already been")
}

func (suite *VenueServiceIntegrationTestSuite) TestCancelPendingVenueEdit_Success() {
	venue, _ := suite.venueService.CreateVenue(&CreateVenueRequest{
		Name:  "Cancel Target",
		City:  "Phoenix",
		State: "AZ",
	}, true)
	user := suite.createTestUser()

	editResp, _ := suite.venueService.CreatePendingVenueEdit(venue.ID, user.ID, &VenueEditRequest{
		Name: stringPtr("Cancel Me"),
	})

	err := suite.venueService.CancelPendingVenueEdit(editResp.ID, user.ID)

	suite.Require().NoError(err)

	// Verify edit is gone
	resp, err := suite.venueService.GetPendingEditForVenue(venue.ID, user.ID)
	suite.Require().NoError(err)
	suite.Nil(resp)
}

func (suite *VenueServiceIntegrationTestSuite) TestCancelPendingVenueEdit_WrongUser_Fails() {
	venue, _ := suite.venueService.CreateVenue(&CreateVenueRequest{
		Name:  "Wrong User Target",
		City:  "Phoenix",
		State: "AZ",
	}, true)
	owner := suite.createTestUser()
	otherUser := suite.createTestUser()

	editResp, _ := suite.venueService.CreatePendingVenueEdit(venue.ID, owner.ID, &VenueEditRequest{
		Name: stringPtr("Owner Edit"),
	})

	err := suite.venueService.CancelPendingVenueEdit(editResp.ID, otherUser.ID)

	suite.Require().Error(err)
	suite.Contains(err.Error(), "only cancel your own")
}

// =============================================================================
// Group 14: buildVenueResponse behavior
// =============================================================================

func (suite *VenueServiceIntegrationTestSuite) TestBuildVenueResponse_UnverifiedHidesAddress() {
	resp, err := suite.venueService.CreateVenue(&CreateVenueRequest{
		Name:    "Hidden Address",
		City:    "Phoenix",
		State:   "AZ",
		Address: stringPtr("Secret St"),
		Zipcode: stringPtr("85001"),
	}, false) // non-admin = unverified

	suite.Require().NoError(err)
	suite.Nil(resp.Address, "address should be hidden for unverified venues")
	suite.Nil(resp.Zipcode, "zipcode should be hidden for unverified venues")
}

func (suite *VenueServiceIntegrationTestSuite) TestBuildVenueResponse_VerifiedShowsAddress() {
	resp, err := suite.venueService.CreateVenue(&CreateVenueRequest{
		Name:    "Visible Address",
		City:    "Phoenix",
		State:   "AZ",
		Address: stringPtr("Public St"),
		Zipcode: stringPtr("85002"),
	}, true) // admin = verified

	suite.Require().NoError(err)
	suite.Require().NotNil(resp.Address)
	suite.Equal("Public St", *resp.Address)
	suite.Require().NotNil(resp.Zipcode)
	suite.Equal("85002", *resp.Zipcode)
}

// =============================================================================
// Group 15: GetUnverifiedVenues / GetVenueModel
// =============================================================================

func (suite *VenueServiceIntegrationTestSuite) TestGetUnverifiedVenues_Success() {
	suite.createTestVenue("Unverified 1", "Phoenix", "AZ", false)
	suite.createTestVenue("Unverified 2", "Tucson", "AZ", false)
	suite.createTestVenue("Verified One", "Phoenix", "AZ", true)

	resp, total, err := suite.venueService.GetUnverifiedVenues(10, 0)

	suite.Require().NoError(err)
	suite.Equal(int64(2), total)
	suite.Len(resp, 2)
	// All returned venues should be unverified — names should not include "Verified One"
	for _, v := range resp {
		suite.NotEqual("Verified One", v.Name)
	}
}

func (suite *VenueServiceIntegrationTestSuite) TestGetUnverifiedVenues_WithShowCounts() {
	venue := suite.createTestVenue("Unverified With Shows", "Phoenix", "AZ", false)
	user := suite.createTestUser()
	suite.createApprovedShow(venue.ID, user.ID)
	suite.createApprovedShow(venue.ID, user.ID)

	resp, _, err := suite.venueService.GetUnverifiedVenues(10, 0)

	suite.Require().NoError(err)
	suite.Require().Len(resp, 1)
	suite.Equal(2, resp[0].ShowCount)
}

func (suite *VenueServiceIntegrationTestSuite) TestGetUnverifiedVenues_Pagination() {
	for i := 0; i < 5; i++ {
		suite.createTestVenue(fmt.Sprintf("Unverified P%d", i), "Phoenix", "AZ", false)
	}

	resp, total, err := suite.venueService.GetUnverifiedVenues(2, 0)
	suite.Require().NoError(err)
	suite.Equal(int64(5), total)
	suite.Len(resp, 2)

	resp2, _, err := suite.venueService.GetUnverifiedVenues(2, 2)
	suite.Require().NoError(err)
	suite.Len(resp2, 2)

	// No overlap
	suite.NotEqual(resp[0].ID, resp2[0].ID)
}

func (suite *VenueServiceIntegrationTestSuite) TestGetVenueModel_Success() {
	created := suite.createTestVenue("Model Venue", "Phoenix", "AZ", true)

	model, err := suite.venueService.GetVenueModel(created.ID)

	suite.Require().NoError(err)
	suite.Equal(created.ID, model.ID)
	suite.Equal("Model Venue", model.Name)
}

func (suite *VenueServiceIntegrationTestSuite) TestGetVenueModel_NotFound() {
	model, err := suite.venueService.GetVenueModel(99999)

	suite.Require().Error(err)
	suite.Nil(model)
	var venueErr *apperrors.VenueError
	suite.ErrorAs(err, &venueErr)
	suite.Equal(apperrors.CodeVenueNotFound, venueErr.Code)
}
