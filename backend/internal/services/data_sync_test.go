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

func TestNewDataSyncService(t *testing.T) {
	t.Run("NilDB", func(t *testing.T) {
		svc := NewDataSyncService(nil)
		assert.NotNil(t, svc)
	})

	t.Run("ExplicitDB", func(t *testing.T) {
		db := &gorm.DB{}
		svc := NewDataSyncService(db)
		assert.NotNil(t, svc)
	})
}

func TestDataSyncService_NilDB(t *testing.T) {
	svc := &DataSyncService{db: nil}

	t.Run("ExportShows", func(t *testing.T) {
		result, err := svc.ExportShows(ExportShowsParams{})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "database not initialized")
		assert.Nil(t, result)
	})

	t.Run("ExportArtists", func(t *testing.T) {
		result, err := svc.ExportArtists(ExportArtistsParams{})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "database not initialized")
		assert.Nil(t, result)
	})

	t.Run("ExportVenues", func(t *testing.T) {
		result, err := svc.ExportVenues(ExportVenuesParams{})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "database not initialized")
		assert.Nil(t, result)
	})

	t.Run("ImportData", func(t *testing.T) {
		result, err := svc.ImportData(DataImportRequest{})
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "database not initialized")
		assert.Nil(t, result)
	})
}

// =============================================================================
// INTEGRATION TESTS (With Real Database)
// =============================================================================

type DataSyncServiceIntegrationTestSuite struct {
	suite.Suite
	container testcontainers.Container
	db        *gorm.DB
	service   *DataSyncService
	ctx       context.Context
}

func (suite *DataSyncServiceIntegrationTestSuite) SetupSuite() {
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

	suite.service = NewDataSyncService(db)
}

func (suite *DataSyncServiceIntegrationTestSuite) TearDownSuite() {
	if suite.container != nil {
		if err := suite.container.Terminate(suite.ctx); err != nil {
			suite.T().Logf("failed to terminate container: %v", err)
		}
	}
}

func (suite *DataSyncServiceIntegrationTestSuite) TearDownTest() {
	sqlDB, err := suite.db.DB()
	suite.Require().NoError(err)
	_, _ = sqlDB.Exec("DELETE FROM show_artists")
	_, _ = sqlDB.Exec("DELETE FROM show_venues")
	_, _ = sqlDB.Exec("DELETE FROM shows")
	_, _ = sqlDB.Exec("DELETE FROM artists")
	_, _ = sqlDB.Exec("DELETE FROM venues")
	_, _ = sqlDB.Exec("DELETE FROM users")
}

func TestDataSyncServiceIntegrationTestSuite(t *testing.T) {
	suite.Run(t, new(DataSyncServiceIntegrationTestSuite))
}

// =============================================================================
// HELPERS
// =============================================================================

func (suite *DataSyncServiceIntegrationTestSuite) createVenue(name, city, state string, verified bool) *models.Venue {
	slug := fmt.Sprintf("%s-%s", strings.ToLower(strings.ReplaceAll(name, " ", "-")), strings.ToLower(city))
	venue := &models.Venue{
		Name:     name,
		Slug:     &slug,
		City:     city,
		State:    state,
		Verified: verified,
	}
	err := suite.db.Create(venue).Error
	suite.Require().NoError(err)
	return venue
}

func (suite *DataSyncServiceIntegrationTestSuite) createArtist(name string) *models.Artist {
	slug := strings.ToLower(strings.ReplaceAll(name, " ", "-"))
	artist := &models.Artist{
		Name: name,
		Slug: &slug,
	}
	err := suite.db.Create(artist).Error
	suite.Require().NoError(err)
	return artist
}

func (suite *DataSyncServiceIntegrationTestSuite) createArtistWithSocial(name string, instagram *string) *models.Artist {
	slug := strings.ToLower(strings.ReplaceAll(name, " ", "-"))
	artist := &models.Artist{
		Name: name,
		Slug: &slug,
		Social: models.Social{
			Instagram: instagram,
		},
	}
	err := suite.db.Create(artist).Error
	suite.Require().NoError(err)
	return artist
}

func (suite *DataSyncServiceIntegrationTestSuite) createVenueWithSocial(name, city, state string, instagram *string) *models.Venue {
	slug := fmt.Sprintf("%s-%s", strings.ToLower(strings.ReplaceAll(name, " ", "-")), strings.ToLower(city))
	venue := &models.Venue{
		Name:     name,
		Slug:     &slug,
		City:     city,
		State:    state,
		Verified: true,
		Social: models.Social{
			Instagram: instagram,
		},
	}
	err := suite.db.Create(venue).Error
	suite.Require().NoError(err)
	return venue
}

func (suite *DataSyncServiceIntegrationTestSuite) createShow(title string, eventDate time.Time, status models.ShowStatus, venue *models.Venue, artists ...*models.Artist) *models.Show {
	slug := strings.ToLower(strings.ReplaceAll(title, " ", "-"))
	city := "NYC"
	state := "NY"
	show := &models.Show{
		Title:     title,
		Slug:      &slug,
		EventDate: eventDate.UTC(),
		City:      &city,
		State:     &state,
		Status:    status,
		Source:    models.ShowSourceUser,
	}
	err := suite.db.Create(show).Error
	suite.Require().NoError(err)

	if venue != nil {
		sv := models.ShowVenue{ShowID: show.ID, VenueID: venue.ID}
		suite.db.Create(&sv)
	}

	for i, artist := range artists {
		sa := models.ShowArtist{
			ShowID:   show.ID,
			ArtistID: artist.ID,
			Position: i,
			SetType:  "performer",
		}
		suite.db.Create(&sa)
	}

	return show
}

func dssBoolPtr(b bool) *bool {
	return &b
}

// =============================================================================
// ExportShows Tests
// =============================================================================

func (suite *DataSyncServiceIntegrationTestSuite) TestExportShows_Empty() {
	result, err := suite.service.ExportShows(ExportShowsParams{})
	suite.Require().NoError(err)
	suite.Equal(int64(0), result.Total)
	suite.Empty(result.Shows)
}

func (suite *DataSyncServiceIntegrationTestSuite) TestExportShows_DefaultLimit() {
	venue := suite.createVenue("Venue", "NYC", "NY", true)
	// Create 60 shows
	for i := 0; i < 60; i++ {
		suite.createShow(fmt.Sprintf("Show %d", i), time.Now().Add(time.Duration(i)*time.Hour), models.ShowStatusApproved, venue)
	}

	result, err := suite.service.ExportShows(ExportShowsParams{Status: "all"})
	suite.Require().NoError(err)
	suite.Equal(int64(60), result.Total)
	suite.Len(result.Shows, 50) // Default limit
}

func (suite *DataSyncServiceIntegrationTestSuite) TestExportShows_MaxLimit() {
	result, err := suite.service.ExportShows(ExportShowsParams{Limit: 500, Status: "all"})
	suite.Require().NoError(err)
	// Limit capped to 200 (no data, but verifies no error)
	suite.Equal(int64(0), result.Total)
}

func (suite *DataSyncServiceIntegrationTestSuite) TestExportShows_StatusFilter_Approved() {
	venue := suite.createVenue("Venue", "NYC", "NY", true)
	suite.createShow("Approved 1", time.Now(), models.ShowStatusApproved, venue)
	suite.createShow("Approved 2", time.Now(), models.ShowStatusApproved, venue)
	suite.createShow("Pending 1", time.Now(), models.ShowStatusPending, venue)

	result, err := suite.service.ExportShows(ExportShowsParams{Status: "approved"})
	suite.Require().NoError(err)
	suite.Equal(int64(2), result.Total)
	suite.Len(result.Shows, 2)
}

func (suite *DataSyncServiceIntegrationTestSuite) TestExportShows_StatusFilter_Pending() {
	venue := suite.createVenue("Venue", "NYC", "NY", true)
	suite.createShow("Approved 1", time.Now(), models.ShowStatusApproved, venue)
	suite.createShow("Pending 1", time.Now(), models.ShowStatusPending, venue)

	result, err := suite.service.ExportShows(ExportShowsParams{Status: "pending"})
	suite.Require().NoError(err)
	suite.Equal(int64(1), result.Total)
}

func (suite *DataSyncServiceIntegrationTestSuite) TestExportShows_StatusFilter_All() {
	venue := suite.createVenue("Venue", "NYC", "NY", true)
	suite.createShow("Approved", time.Now(), models.ShowStatusApproved, venue)
	suite.createShow("Pending", time.Now().Add(time.Hour), models.ShowStatusPending, venue)
	suite.createShow("Rejected", time.Now().Add(2*time.Hour), models.ShowStatusRejected, venue)

	result, err := suite.service.ExportShows(ExportShowsParams{Status: "all"})
	suite.Require().NoError(err)
	suite.Equal(int64(3), result.Total)
	suite.Len(result.Shows, 3)
}

func (suite *DataSyncServiceIntegrationTestSuite) TestExportShows_DateFilter() {
	venue := suite.createVenue("Venue", "NYC", "NY", true)
	suite.createShow("Old Show", time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), models.ShowStatusApproved, venue)
	suite.createShow("New Show", time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC), models.ShowStatusApproved, venue)

	fromDate := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	result, err := suite.service.ExportShows(ExportShowsParams{
		Status:   "approved",
		FromDate: &fromDate,
	})
	suite.Require().NoError(err)
	suite.Equal(int64(1), result.Total)
	suite.Equal("New Show", result.Shows[0].Title)
}

func (suite *DataSyncServiceIntegrationTestSuite) TestExportShows_LocationFilter() {
	venueNYC := suite.createVenue("NYC Venue", "New York", "NY", true)
	venueLA := suite.createVenue("LA Venue", "Los Angeles", "CA", true)

	showNYC := suite.createShow("NYC Show", time.Now(), models.ShowStatusApproved, venueNYC)
	suite.createShow("LA Show", time.Now(), models.ShowStatusApproved, venueLA)
	// Shows have city set from the createShow helper
	_ = showNYC

	result, err := suite.service.ExportShows(ExportShowsParams{
		Status: "approved",
		City:   "NYC",
	})
	suite.Require().NoError(err)
	suite.Equal(int64(2), result.Total)
}

func (suite *DataSyncServiceIntegrationTestSuite) TestExportShows_WithArtistsAndVenues() {
	venue := suite.createVenue("The Hall", "NYC", "NY", true)
	artist1 := suite.createArtist("Band One")
	artist2 := suite.createArtist("Band Two")
	suite.createShow("Big Show", time.Now(), models.ShowStatusApproved, venue, artist1, artist2)

	result, err := suite.service.ExportShows(ExportShowsParams{Status: "all"})
	suite.Require().NoError(err)
	suite.Require().Len(result.Shows, 1)

	show := result.Shows[0]
	suite.Equal("Big Show", show.Title)
	suite.Require().Len(show.Venues, 1)
	suite.Equal("The Hall", show.Venues[0].Name)
	suite.Require().Len(show.Artists, 2)
	// Check artist names exist (order from DB join may vary)
	artistNames := []string{show.Artists[0].Name, show.Artists[1].Name}
	suite.Contains(artistNames, "Band One")
	suite.Contains(artistNames, "Band Two")
}

func (suite *DataSyncServiceIntegrationTestSuite) TestExportShows_Pagination() {
	venue := suite.createVenue("Venue", "NYC", "NY", true)
	for i := 0; i < 5; i++ {
		suite.createShow(fmt.Sprintf("Show %d", i), time.Now().Add(time.Duration(i)*time.Hour), models.ShowStatusApproved, venue)
	}

	result, err := suite.service.ExportShows(ExportShowsParams{
		Status: "all",
		Limit:  2,
		Offset: 2,
	})
	suite.Require().NoError(err)
	suite.Equal(int64(5), result.Total)
	suite.Len(result.Shows, 2)
}

// =============================================================================
// ExportArtists Tests
// =============================================================================

func (suite *DataSyncServiceIntegrationTestSuite) TestExportArtists_Empty() {
	result, err := suite.service.ExportArtists(ExportArtistsParams{})
	suite.Require().NoError(err)
	suite.Equal(int64(0), result.Total)
	suite.Empty(result.Artists)
}

func (suite *DataSyncServiceIntegrationTestSuite) TestExportArtists_DefaultLimit() {
	for i := 0; i < 60; i++ {
		suite.createArtist(fmt.Sprintf("Artist %03d", i))
	}

	result, err := suite.service.ExportArtists(ExportArtistsParams{})
	suite.Require().NoError(err)
	suite.Equal(int64(60), result.Total)
	suite.Len(result.Artists, 50) // Default limit
}

func (suite *DataSyncServiceIntegrationTestSuite) TestExportArtists_Search() {
	suite.createArtist("The Band")
	suite.createArtist("Another Band")
	suite.createArtist("Solo Singer")

	result, err := suite.service.ExportArtists(ExportArtistsParams{Search: "band"})
	suite.Require().NoError(err)
	suite.Equal(int64(2), result.Total)
}

func (suite *DataSyncServiceIntegrationTestSuite) TestExportArtists_Pagination() {
	for i := 0; i < 5; i++ {
		suite.createArtist(fmt.Sprintf("Artist %d", i))
	}

	result, err := suite.service.ExportArtists(ExportArtistsParams{Limit: 2, Offset: 2})
	suite.Require().NoError(err)
	suite.Equal(int64(5), result.Total)
	suite.Len(result.Artists, 2)
}

func (suite *DataSyncServiceIntegrationTestSuite) TestExportArtists_WithSocial() {
	insta := "@theband"
	suite.createArtistWithSocial("The Band", &insta)

	result, err := suite.service.ExportArtists(ExportArtistsParams{})
	suite.Require().NoError(err)
	suite.Require().Len(result.Artists, 1)
	suite.Equal("The Band", result.Artists[0].Name)
	suite.NotNil(result.Artists[0].Instagram)
	suite.Equal("@theband", *result.Artists[0].Instagram)
}

// =============================================================================
// ExportVenues Tests
// =============================================================================

func (suite *DataSyncServiceIntegrationTestSuite) TestExportVenues_Empty() {
	result, err := suite.service.ExportVenues(ExportVenuesParams{})
	suite.Require().NoError(err)
	suite.Equal(int64(0), result.Total)
	suite.Empty(result.Venues)
}

func (suite *DataSyncServiceIntegrationTestSuite) TestExportVenues_DefaultLimit() {
	for i := 0; i < 60; i++ {
		suite.createVenue(fmt.Sprintf("Venue %03d", i), "NYC", "NY", true)
	}

	result, err := suite.service.ExportVenues(ExportVenuesParams{})
	suite.Require().NoError(err)
	suite.Equal(int64(60), result.Total)
	suite.Len(result.Venues, 50) // Default limit
}

func (suite *DataSyncServiceIntegrationTestSuite) TestExportVenues_Search() {
	suite.createVenue("Music Hall", "NYC", "NY", true)
	suite.createVenue("Concert Hall", "LA", "CA", true)
	suite.createVenue("The Dive Bar", "CHI", "IL", true)

	result, err := suite.service.ExportVenues(ExportVenuesParams{Search: "hall"})
	suite.Require().NoError(err)
	suite.Equal(int64(2), result.Total)
}

func (suite *DataSyncServiceIntegrationTestSuite) TestExportVenues_FilterVerified() {
	suite.createVenue("Verified 1", "NYC", "NY", true)
	suite.createVenue("Verified 2", "LA", "CA", true)
	suite.createVenue("Unverified", "CHI", "IL", false)

	result, err := suite.service.ExportVenues(ExportVenuesParams{Verified: dssBoolPtr(true)})
	suite.Require().NoError(err)
	suite.Equal(int64(2), result.Total)
}

func (suite *DataSyncServiceIntegrationTestSuite) TestExportVenues_FilterCity() {
	suite.createVenue("Venue NYC", "New York", "NY", true)
	suite.createVenue("Venue LA", "Los Angeles", "CA", true)

	result, err := suite.service.ExportVenues(ExportVenuesParams{City: "New York"})
	suite.Require().NoError(err)
	suite.Equal(int64(1), result.Total)
	suite.Equal("Venue NYC", result.Venues[0].Name)
}

func (suite *DataSyncServiceIntegrationTestSuite) TestExportVenues_WithSocial() {
	insta := "@thevenue"
	suite.createVenueWithSocial("The Venue", "NYC", "NY", &insta)

	result, err := suite.service.ExportVenues(ExportVenuesParams{})
	suite.Require().NoError(err)
	suite.Require().Len(result.Venues, 1)
	suite.NotNil(result.Venues[0].Instagram)
	suite.Equal("@thevenue", *result.Venues[0].Instagram)
}

// =============================================================================
// ImportData Tests — Artists
// =============================================================================

func (suite *DataSyncServiceIntegrationTestSuite) TestImportData_EmptyRequest() {
	result, err := suite.service.ImportData(DataImportRequest{})
	suite.Require().NoError(err)
	suite.Equal(0, result.Shows.Total)
	suite.Equal(0, result.Artists.Total)
	suite.Equal(0, result.Venues.Total)
}

func (suite *DataSyncServiceIntegrationTestSuite) TestImportArtist_Success() {
	result, err := suite.service.ImportData(DataImportRequest{
		Artists: []ExportedArtist{
			{Name: "New Band"},
		},
	})
	suite.Require().NoError(err)
	suite.Equal(1, result.Artists.Imported)
	suite.Contains(result.Artists.Messages[0], "IMPORTED")

	// Verify created with slug
	var artist models.Artist
	err = suite.db.Where("name = ?", "New Band").First(&artist).Error
	suite.Require().NoError(err)
	suite.NotNil(artist.Slug)
}

func (suite *DataSyncServiceIntegrationTestSuite) TestImportArtist_Duplicate() {
	suite.createArtist("Existing Band")

	result, err := suite.service.ImportData(DataImportRequest{
		Artists: []ExportedArtist{
			{Name: "Existing Band"},
		},
	})
	suite.Require().NoError(err)
	suite.Equal(1, result.Artists.Duplicates)
	suite.Contains(result.Artists.Messages[0], "DUPLICATE")
}

func (suite *DataSyncServiceIntegrationTestSuite) TestImportArtist_DuplicateBackfillSlug() {
	// Create artist WITHOUT a slug
	artist := &models.Artist{Name: "No Slug Band"}
	suite.db.Create(artist)
	suite.Nil(artist.Slug)

	result, err := suite.service.ImportData(DataImportRequest{
		Artists: []ExportedArtist{
			{Name: "No Slug Band"},
		},
	})
	suite.Require().NoError(err)
	suite.Equal(1, result.Artists.Duplicates)

	// Verify slug was backfilled
	var updated models.Artist
	suite.db.First(&updated, artist.ID)
	suite.NotNil(updated.Slug)
}

func (suite *DataSyncServiceIntegrationTestSuite) TestImportArtist_EmptyName() {
	result, err := suite.service.ImportData(DataImportRequest{
		Artists: []ExportedArtist{
			{Name: ""},
		},
	})
	suite.Require().NoError(err)
	suite.Equal(1, result.Artists.Errors)
	suite.Contains(result.Artists.Messages[0], "SKIP")
}

func (suite *DataSyncServiceIntegrationTestSuite) TestImportArtist_DryRun() {
	result, err := suite.service.ImportData(DataImportRequest{
		Artists: []ExportedArtist{
			{Name: "Dry Run Band"},
		},
		DryRun: true,
	})
	suite.Require().NoError(err)
	suite.Equal(1, result.Artists.Imported)
	suite.Contains(result.Artists.Messages[0], "WOULD IMPORT")

	// Verify NOT actually created
	var count int64
	suite.db.Model(&models.Artist{}).Where("name = ?", "Dry Run Band").Count(&count)
	suite.Equal(int64(0), count)
}

// =============================================================================
// ImportData Tests — Venues
// =============================================================================

func (suite *DataSyncServiceIntegrationTestSuite) TestImportVenue_Success() {
	result, err := suite.service.ImportData(DataImportRequest{
		Venues: []ExportedVenue{
			{Name: "New Venue", City: "NYC", State: "NY"},
		},
	})
	suite.Require().NoError(err)
	suite.Equal(1, result.Venues.Imported)
	suite.Contains(result.Venues.Messages[0], "IMPORTED")

	// Verify created with slug
	var venue models.Venue
	err = suite.db.Where("name = ?", "New Venue").First(&venue).Error
	suite.Require().NoError(err)
	suite.NotNil(venue.Slug)
}

func (suite *DataSyncServiceIntegrationTestSuite) TestImportVenue_Duplicate() {
	suite.createVenue("Existing Venue", "NYC", "NY", true)

	result, err := suite.service.ImportData(DataImportRequest{
		Venues: []ExportedVenue{
			{Name: "Existing Venue", City: "NYC", State: "NY"},
		},
	})
	suite.Require().NoError(err)
	suite.Equal(1, result.Venues.Duplicates)
	suite.Contains(result.Venues.Messages[0], "DUPLICATE")
}

func (suite *DataSyncServiceIntegrationTestSuite) TestImportVenue_MissingFields() {
	result, err := suite.service.ImportData(DataImportRequest{
		Venues: []ExportedVenue{
			{Name: "Venue Only"}, // Missing city and state
		},
	})
	suite.Require().NoError(err)
	suite.Equal(1, result.Venues.Errors)
	suite.Contains(result.Venues.Messages[0], "SKIP")
}

func (suite *DataSyncServiceIntegrationTestSuite) TestImportVenue_DryRun() {
	result, err := suite.service.ImportData(DataImportRequest{
		Venues: []ExportedVenue{
			{Name: "Dry Run Venue", City: "NYC", State: "NY"},
		},
		DryRun: true,
	})
	suite.Require().NoError(err)
	suite.Equal(1, result.Venues.Imported)
	suite.Contains(result.Venues.Messages[0], "WOULD IMPORT")

	var count int64
	suite.db.Model(&models.Venue{}).Where("name = ?", "Dry Run Venue").Count(&count)
	suite.Equal(int64(0), count)
}

// =============================================================================
// ImportData Tests — Shows
// =============================================================================

func (suite *DataSyncServiceIntegrationTestSuite) TestImportShow_Success() {
	result, err := suite.service.ImportData(DataImportRequest{
		Shows: []ExportedShow{
			{
				Title:     "New Show",
				EventDate: time.Now().Add(24 * time.Hour).UTC().Format(time.RFC3339),
				Status:    "approved",
				Venues:    []ExportedVenue{{Name: "Test Venue", City: "NYC", State: "NY"}},
				Artists:   []ExportedShowArtist{{Name: "Test Band", Position: 0, SetType: "performer"}},
			},
		},
	})
	suite.Require().NoError(err)
	suite.Equal(1, result.Shows.Imported)
	suite.Contains(result.Shows.Messages[0], "IMPORTED")

	// Verify show, venue, and artist all created
	var show models.Show
	err = suite.db.Where("title = ?", "New Show").First(&show).Error
	suite.Require().NoError(err)
	suite.NotNil(show.Slug)

	var venue models.Venue
	err = suite.db.Where("name = ?", "Test Venue").First(&venue).Error
	suite.Require().NoError(err)

	var artist models.Artist
	err = suite.db.Where("name = ?", "Test Band").First(&artist).Error
	suite.Require().NoError(err)
}

func (suite *DataSyncServiceIntegrationTestSuite) TestImportShow_Duplicate() {
	venue := suite.createVenue("Dupe Venue", "NYC", "NY", true)
	eventDate := time.Date(2025, 6, 15, 20, 0, 0, 0, time.UTC)
	suite.createShow("Dupe Show", eventDate, models.ShowStatusApproved, venue)

	result, err := suite.service.ImportData(DataImportRequest{
		Shows: []ExportedShow{
			{
				Title:     "Dupe Show",
				EventDate: eventDate.Format(time.RFC3339),
				Status:    "approved",
				Venues:    []ExportedVenue{{Name: "Dupe Venue", City: "NYC", State: "NY"}},
			},
		},
	})
	suite.Require().NoError(err)
	suite.Equal(1, result.Shows.Duplicates)
	suite.Contains(result.Shows.Messages[0], "DUPLICATE")
}

func (suite *DataSyncServiceIntegrationTestSuite) TestImportShow_DuplicateBackfillSlugs() {
	// Create entities without slugs
	venue := &models.Venue{Name: "No Slug Venue", City: "NYC", State: "NY", Verified: true}
	suite.db.Create(venue)
	artist := &models.Artist{Name: "No Slug Artist"}
	suite.db.Create(artist)

	eventDate := time.Date(2025, 7, 1, 20, 0, 0, 0, time.UTC)
	show := &models.Show{
		Title:     "No Slug Show",
		EventDate: eventDate,
		Status:    models.ShowStatusApproved,
		Source:    models.ShowSourceUser,
	}
	suite.db.Create(show)
	suite.db.Create(&models.ShowVenue{ShowID: show.ID, VenueID: venue.ID})
	suite.db.Create(&models.ShowArtist{ShowID: show.ID, ArtistID: artist.ID, Position: 0, SetType: "performer"})

	// Import duplicate — should backfill slugs
	result, err := suite.service.ImportData(DataImportRequest{
		Shows: []ExportedShow{
			{
				Title:     "No Slug Show",
				EventDate: eventDate.Format(time.RFC3339),
				Status:    "approved",
				Venues:    []ExportedVenue{{Name: "No Slug Venue", City: "NYC", State: "NY"}},
				Artists:   []ExportedShowArtist{{Name: "No Slug Artist", Position: 0, SetType: "performer"}},
			},
		},
	})
	suite.Require().NoError(err)
	suite.Equal(1, result.Shows.Duplicates)

	// Verify slugs backfilled
	var updatedShow models.Show
	suite.db.First(&updatedShow, show.ID)
	suite.NotNil(updatedShow.Slug)

	var updatedVenue models.Venue
	suite.db.First(&updatedVenue, venue.ID)
	suite.NotNil(updatedVenue.Slug)

	var updatedArtist models.Artist
	suite.db.First(&updatedArtist, artist.ID)
	suite.NotNil(updatedArtist.Slug)
}

func (suite *DataSyncServiceIntegrationTestSuite) TestImportShow_MissingFields() {
	result, err := suite.service.ImportData(DataImportRequest{
		Shows: []ExportedShow{
			{Title: "", EventDate: ""},
		},
	})
	suite.Require().NoError(err)
	suite.Equal(1, result.Shows.Errors)
	suite.Contains(result.Shows.Messages[0], "SKIP")
}

func (suite *DataSyncServiceIntegrationTestSuite) TestImportShow_InvalidDate() {
	result, err := suite.service.ImportData(DataImportRequest{
		Shows: []ExportedShow{
			{Title: "Bad Date Show", EventDate: "not-a-date"},
		},
	})
	suite.Require().NoError(err)
	suite.Equal(1, result.Shows.Errors)
	suite.Contains(result.Shows.Messages[0], "ERROR")
	suite.Contains(result.Shows.Messages[0], "Invalid event date")
}

func (suite *DataSyncServiceIntegrationTestSuite) TestImportShow_CreatesNewVenueAndArtist() {
	result, err := suite.service.ImportData(DataImportRequest{
		Shows: []ExportedShow{
			{
				Title:     "Show With New Everything",
				EventDate: time.Now().Add(48 * time.Hour).UTC().Format(time.RFC3339),
				Status:    "approved",
				Venues:    []ExportedVenue{{Name: "Brand New Venue", City: "Portland", State: "OR"}},
				Artists:   []ExportedShowArtist{{Name: "Brand New Band", Position: 0, SetType: "headliner"}},
			},
		},
	})
	suite.Require().NoError(err)
	suite.Equal(1, result.Shows.Imported)

	// Verify venue created with slug
	var venue models.Venue
	err = suite.db.Where("name = ?", "Brand New Venue").First(&venue).Error
	suite.Require().NoError(err)
	suite.NotNil(venue.Slug)
	suite.Equal("Portland", venue.City)

	// Verify artist created with slug
	var artist models.Artist
	err = suite.db.Where("name = ?", "Brand New Band").First(&artist).Error
	suite.Require().NoError(err)
	suite.NotNil(artist.Slug)
}

func (suite *DataSyncServiceIntegrationTestSuite) TestImportShow_DryRun() {
	result, err := suite.service.ImportData(DataImportRequest{
		Shows: []ExportedShow{
			{
				Title:     "Dry Run Show",
				EventDate: time.Now().Add(24 * time.Hour).UTC().Format(time.RFC3339),
				Status:    "approved",
				Venues:    []ExportedVenue{{Name: "Dry Venue", City: "NYC", State: "NY"}},
			},
		},
		DryRun: true,
	})
	suite.Require().NoError(err)
	suite.Equal(1, result.Shows.Imported)
	suite.Contains(result.Shows.Messages[0], "WOULD IMPORT")

	var count int64
	suite.db.Model(&models.Show{}).Where("title = ?", "Dry Run Show").Count(&count)
	suite.Equal(int64(0), count)
}

func (suite *DataSyncServiceIntegrationTestSuite) TestImportShow_StatusParsing() {
	eventDates := []time.Time{
		time.Now().Add(24 * time.Hour),
		time.Now().Add(48 * time.Hour),
		time.Now().Add(72 * time.Hour),
	}

	result, err := suite.service.ImportData(DataImportRequest{
		Shows: []ExportedShow{
			{
				Title:     "Pending Import",
				EventDate: eventDates[0].UTC().Format(time.RFC3339),
				Status:    "pending",
				Venues:    []ExportedVenue{{Name: "Venue A", City: "NYC", State: "NY"}},
			},
			{
				Title:     "Rejected Import",
				EventDate: eventDates[1].UTC().Format(time.RFC3339),
				Status:    "rejected",
				Venues:    []ExportedVenue{{Name: "Venue B", City: "LA", State: "CA"}},
			},
			{
				Title:     "Private Import",
				EventDate: eventDates[2].UTC().Format(time.RFC3339),
				Status:    "private",
				Venues:    []ExportedVenue{{Name: "Venue C", City: "CHI", State: "IL"}},
			},
		},
	})
	suite.Require().NoError(err)
	suite.Equal(3, result.Shows.Imported)

	var pendingShow models.Show
	suite.db.Where("title = ?", "Pending Import").First(&pendingShow)
	suite.Equal(models.ShowStatusPending, pendingShow.Status)

	var rejectedShow models.Show
	suite.db.Where("title = ?", "Rejected Import").First(&rejectedShow)
	suite.Equal(models.ShowStatusRejected, rejectedShow.Status)

	var privateShow models.Show
	suite.db.Where("title = ?", "Private Import").First(&privateShow)
	suite.Equal(models.ShowStatusPrivate, privateShow.Status)
}

// =============================================================================
// Full Round-Trip Test
// =============================================================================

func (suite *DataSyncServiceIntegrationTestSuite) TestImportData_FullRoundTrip() {
	// Create data to export
	insta := "@thevenue"
	venue := suite.createVenueWithSocial("RT Venue", "NYC", "NY", &insta)
	artist := suite.createArtist("RT Band")
	eventDate := time.Date(2025, 8, 1, 20, 0, 0, 0, time.UTC)
	suite.createShow("RT Show", eventDate, models.ShowStatusApproved, venue, artist)

	// Export
	exportResult, err := suite.service.ExportShows(ExportShowsParams{Status: "approved"})
	suite.Require().NoError(err)
	suite.Require().Len(exportResult.Shows, 1)

	exportedShow := exportResult.Shows[0]
	suite.Equal("RT Show", exportedShow.Title)
	suite.Require().Len(exportedShow.Venues, 1)
	suite.Equal("RT Venue", exportedShow.Venues[0].Name)

	// Clean up data
	sqlDB, _ := suite.db.DB()
	_, _ = sqlDB.Exec("DELETE FROM show_artists")
	_, _ = sqlDB.Exec("DELETE FROM show_venues")
	_, _ = sqlDB.Exec("DELETE FROM shows")
	_, _ = sqlDB.Exec("DELETE FROM artists")
	_, _ = sqlDB.Exec("DELETE FROM venues")

	// Import the exported data
	importResult, err := suite.service.ImportData(DataImportRequest{
		Shows: exportResult.Shows,
	})
	suite.Require().NoError(err)
	suite.Equal(1, importResult.Shows.Imported)

	// Verify re-created
	var show models.Show
	err = suite.db.Where("title = ?", "RT Show").Preload("Venues").Preload("Artists").First(&show).Error
	suite.Require().NoError(err)
	suite.NotNil(show.Slug)
	suite.Equal(models.ShowStatusApproved, show.Status)
	suite.Require().Len(show.Venues, 1)
	suite.Equal("RT Venue", show.Venues[0].Name)
	suite.Require().Len(show.Artists, 1)
	suite.Equal("RT Band", show.Artists[0].Name)
}
