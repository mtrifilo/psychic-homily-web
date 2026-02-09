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
// UNIT TESTS — parseEventDate
// =============================================================================

func TestParseEventDate_ISODateOnly(t *testing.T) {
	result, err := parseEventDate("2026-01-25", nil)
	assert.NoError(t, err)
	assert.Equal(t, time.Date(2026, 1, 25, 0, 0, 0, 0, time.UTC), result)
}

func TestParseEventDate_ISOTimestamp(t *testing.T) {
	result, err := parseEventDate("2026-01-25T19:00:00Z", nil)
	assert.NoError(t, err)
	assert.Equal(t, time.Date(2026, 1, 25, 19, 0, 0, 0, time.UTC), result)
}

func TestParseEventDate_RFC3339(t *testing.T) {
	result, err := parseEventDate("2026-01-25T19:00:00-07:00", nil)
	assert.NoError(t, err)
	expected := time.Date(2026, 1, 26, 2, 0, 0, 0, time.UTC)
	assert.Equal(t, expected, result)
}

func TestParseEventDate_WithShowTimePM(t *testing.T) {
	showTime := "7:00 pm"
	result, err := parseEventDate("2026-01-25", &showTime)
	assert.NoError(t, err)
	assert.Equal(t, time.Date(2026, 1, 25, 19, 0, 0, 0, time.UTC), result)
}

func TestParseEventDate_WithShowTimeAM(t *testing.T) {
	showTime := "11:00 am"
	result, err := parseEventDate("2026-01-25", &showTime)
	assert.NoError(t, err)
	assert.Equal(t, time.Date(2026, 1, 25, 11, 0, 0, 0, time.UTC), result)
}

func TestParseEventDate_12PM(t *testing.T) {
	showTime := "12:00 pm"
	result, err := parseEventDate("2026-01-25", &showTime)
	assert.NoError(t, err)
	assert.Equal(t, time.Date(2026, 1, 25, 12, 0, 0, 0, time.UTC), result)
}

func TestParseEventDate_12AM(t *testing.T) {
	showTime := "12:00 am"
	result, err := parseEventDate("2026-01-25", &showTime)
	assert.NoError(t, err)
	assert.Equal(t, time.Date(2026, 1, 25, 0, 0, 0, 0, time.UTC), result)
}

func TestParseEventDate_WithSpacesInTime(t *testing.T) {
	showTime := " 7:00 PM "
	result, err := parseEventDate("2026-01-25", &showTime)
	assert.NoError(t, err)
	assert.Equal(t, time.Date(2026, 1, 25, 19, 0, 0, 0, time.UTC), result)
}

func TestParseEventDate_EmptyShowTime(t *testing.T) {
	showTime := ""
	result, err := parseEventDate("2026-01-25", &showTime)
	assert.NoError(t, err)
	assert.Equal(t, time.Date(2026, 1, 25, 0, 0, 0, 0, time.UTC), result)
}

func TestParseEventDate_InvalidDate(t *testing.T) {
	_, err := parseEventDate("not-a-date", nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unable to parse date")
}

func TestParseEventDate_NilShowTime(t *testing.T) {
	result, err := parseEventDate("2026-01-25", nil)
	assert.NoError(t, err)
	assert.Equal(t, time.Date(2026, 1, 25, 0, 0, 0, 0, time.UTC), result)
}

func TestParseEventDate_UnparseableTime(t *testing.T) {
	showTime := "doors at 7"
	result, err := parseEventDate("2026-01-25", &showTime)
	assert.NoError(t, err)
	// Unparseable time is silently ignored, date is returned without time
	assert.Equal(t, time.Date(2026, 1, 25, 0, 0, 0, 0, time.UTC), result)
}

// =============================================================================
// UNIT TESTS — parseArtistsFromTitle
// =============================================================================

func TestParseArtistsFromTitle_SingleArtist(t *testing.T) {
	result := parseArtistsFromTitle("The National")
	assert.Equal(t, []string{"The National"}, result)
}

func TestParseArtistsFromTitle_CommaSeparated(t *testing.T) {
	result := parseArtistsFromTitle("Artist A, Artist B, Artist C")
	assert.Equal(t, []string{"Artist A", "Artist B", "Artist C"}, result)
}

func TestParseArtistsFromTitle_WithSeparator(t *testing.T) {
	result := parseArtistsFromTitle("Artist A with Artist B")
	assert.Equal(t, []string{"Artist A", "Artist B"}, result)
}

func TestParseArtistsFromTitle_WithPlusComma(t *testing.T) {
	// Comma takes priority, so "with" inside a comma segment is preserved
	result := parseArtistsFromTitle("Artist A with Artist B, Artist C")
	assert.Equal(t, []string{"Artist A with Artist B", "Artist C"}, result)
}

func TestParseArtistsFromTitle_SlashSeparator(t *testing.T) {
	result := parseArtistsFromTitle("Artist A / Artist B")
	assert.Equal(t, []string{"Artist A", "Artist B"}, result)
}

func TestParseArtistsFromTitle_PipeSeparator(t *testing.T) {
	result := parseArtistsFromTitle("Artist A | Artist B")
	assert.Equal(t, []string{"Artist A", "Artist B"}, result)
}

func TestParseArtistsFromTitle_PlusSeparator(t *testing.T) {
	result := parseArtistsFromTitle("Artist A + Artist B")
	assert.Equal(t, []string{"Artist A", "Artist B"}, result)
}

func TestParseArtistsFromTitle_AmpersandShortNames(t *testing.T) {
	// Parts <=10 chars: treated as a single artist name
	result := parseArtistsFromTitle("Tom & Jerry")
	assert.Equal(t, []string{"Tom & Jerry"}, result)
}

func TestParseArtistsFromTitle_AmpersandLongNames(t *testing.T) {
	// Parts >10 chars: split into separate artists
	result := parseArtistsFromTitle("The National Band & Radiohead Group")
	assert.Equal(t, []string{"The National Band", "Radiohead Group"}, result)
}

func TestParseArtistsFromTitle_EmptyString(t *testing.T) {
	result := parseArtistsFromTitle("")
	assert.Equal(t, []string{""}, result)
}

func TestParseArtistsFromTitle_WhitespaceTrimmed(t *testing.T) {
	result := parseArtistsFromTitle(" Artist A , Artist B ")
	assert.Equal(t, []string{"Artist A", "Artist B"}, result)
}

func TestParseArtistsFromTitle_CommaPriorityOverWith(t *testing.T) {
	// Comma is checked first — "with" inside a comma segment is preserved
	result := parseArtistsFromTitle("A, B with C")
	assert.Equal(t, []string{"A", "B with C"}, result)
}

// =============================================================================
// UNIT TESTS — splitAndTrim
// =============================================================================

func TestSplitAndTrim_Basic(t *testing.T) {
	result := splitAndTrim("a, b, c", ",")
	assert.Equal(t, []string{"a", "b", "c"}, result)
}

func TestSplitAndTrim_FiltersEmpty(t *testing.T) {
	result := splitAndTrim("a,,b", ",")
	assert.Equal(t, []string{"a", "b"}, result)
}

func TestSplitAndTrim_WhitespaceOnlyParts(t *testing.T) {
	result := splitAndTrim("a, , b", ",")
	assert.Equal(t, []string{"a", "b"}, result)
}

func TestSplitAndTrim_NoSeparator(t *testing.T) {
	result := splitAndTrim("abc", ",")
	assert.Equal(t, []string{"abc"}, result)
}

// =============================================================================
// UNIT TESTS — Constructor + nil DB
// =============================================================================

func TestNewDiscoveryService(t *testing.T) {
	svc := NewDiscoveryService(nil)
	assert.NotNil(t, svc)
}

func TestImportEvents_NilDB(t *testing.T) {
	svc := &DiscoveryService{db: nil}
	result, err := svc.ImportEvents([]DiscoveredEvent{}, false)
	assert.Error(t, err)
	assert.Equal(t, "database not initialized", err.Error())
	assert.Nil(t, result)
}

func TestCheckEvents_NilDB(t *testing.T) {
	svc := &DiscoveryService{db: nil}
	result, err := svc.CheckEvents([]CheckEventInput{})
	assert.Error(t, err)
	assert.Equal(t, "database not initialized", err.Error())
	assert.Nil(t, result)
}

// =============================================================================
// INTEGRATION TESTS (With Real Database)
// =============================================================================

type DiscoveryIntegrationTestSuite struct {
	suite.Suite
	container testcontainers.Container
	db        *gorm.DB
	svc       *DiscoveryService
	ctx       context.Context
}

func (suite *DiscoveryIntegrationTestSuite) SetupSuite() {
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

	// Discovery needs: shows, venues, artists, users, show_venues, show_artists,
	// show_status enum, show_source enum, source fields, slugs, duplicate_of_show_id
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

	suite.svc = NewDiscoveryService(db)
}

func (suite *DiscoveryIntegrationTestSuite) TearDownSuite() {
	if suite.container != nil {
		if err := suite.container.Terminate(suite.ctx); err != nil {
			suite.T().Logf("failed to terminate container: %v", err)
		}
	}
}

func (suite *DiscoveryIntegrationTestSuite) TearDownTest() {
	sqlDB, err := suite.db.DB()
	suite.Require().NoError(err)
	_, _ = sqlDB.Exec("DELETE FROM show_artists")
	_, _ = sqlDB.Exec("DELETE FROM show_venues")
	_, _ = sqlDB.Exec("DELETE FROM shows")
	_, _ = sqlDB.Exec("DELETE FROM artists")
	_, _ = sqlDB.Exec("DELETE FROM venues")
	_, _ = sqlDB.Exec("DELETE FROM users")
}

func TestDiscoveryIntegrationTestSuite(t *testing.T) {
	suite.Run(t, new(DiscoveryIntegrationTestSuite))
}

// =============================================================================
// HELPERS
// =============================================================================

func (suite *DiscoveryIntegrationTestSuite) makeEvent(id, title, venueSlug, date string, artists []string) DiscoveredEvent {
	return DiscoveredEvent{
		ID:        id,
		Title:     title,
		Date:      date,
		Venue:     "Valley Bar",
		VenueSlug: venueSlug,
		Artists:   artists,
		ScrapedAt: time.Now().UTC().Format(time.RFC3339),
	}
}

// =============================================================================
// ImportEvents tests
// =============================================================================

func (suite *DiscoveryIntegrationTestSuite) TestImportEvents_Success() {
	events := []DiscoveredEvent{
		suite.makeEvent("evt-001", "The National", "valley-bar", "2026-06-15", []string{"The National"}),
	}

	result, err := suite.svc.ImportEvents(events, false)
	suite.Require().NoError(err)
	suite.Equal(1, result.Total)
	suite.Equal(1, result.Imported)
	suite.Equal(0, result.Duplicates)
	suite.Equal(0, result.Errors)

	// Verify show was created
	var show models.Show
	err = suite.db.Where("source_event_id = ?", "evt-001").First(&show).Error
	suite.Require().NoError(err)
	suite.Equal("The National", show.Title)
	suite.Equal(models.ShowStatusApproved, show.Status)
	suite.Equal(models.ShowSourceDiscovery, show.Source)
	suite.NotNil(show.Slug)
}

func (suite *DiscoveryIntegrationTestSuite) TestImportEvents_SourceDuplicate() {
	events := []DiscoveredEvent{
		suite.makeEvent("evt-002", "Radiohead", "valley-bar", "2026-07-01", []string{"Radiohead"}),
	}

	// First import
	result1, err := suite.svc.ImportEvents(events, false)
	suite.Require().NoError(err)
	suite.Equal(1, result1.Imported)

	// Second import — same source_venue + source_event_id
	result2, err := suite.svc.ImportEvents(events, false)
	suite.Require().NoError(err)
	suite.Equal(0, result2.Imported)
	suite.Equal(1, result2.Duplicates)
}

func (suite *DiscoveryIntegrationTestSuite) TestImportEvents_UnknownVenue() {
	events := []DiscoveredEvent{
		suite.makeEvent("evt-003", "Test Band", "unknown-venue-xyz", "2026-08-01", []string{"Test Band"}),
	}

	result, err := suite.svc.ImportEvents(events, false)
	suite.Require().NoError(err)
	suite.Equal(1, result.Errors)
	suite.Contains(result.Messages[0], "Unknown venue slug")
}

func (suite *DiscoveryIntegrationTestSuite) TestImportEvents_HeadlinerDuplicate() {
	// Import one show first
	events1 := []DiscoveredEvent{
		suite.makeEvent("evt-004a", "Bon Iver", "valley-bar", "2026-09-01", []string{"Bon Iver"}),
	}
	result1, err := suite.svc.ImportEvents(events1, false)
	suite.Require().NoError(err)
	suite.Equal(1, result1.Imported)

	// Import another event with the same headliner at the same venue on the same date
	// but different source_event_id
	events2 := []DiscoveredEvent{
		suite.makeEvent("evt-004b", "Bon Iver (Late Show)", "valley-bar", "2026-09-01", []string{"Bon Iver"}),
	}
	result2, err := suite.svc.ImportEvents(events2, false)
	suite.Require().NoError(err)
	suite.Equal(1, result2.PendingReview)

	// Verify the second show is pending
	var show models.Show
	err = suite.db.Where("source_event_id = ?", "evt-004b").First(&show).Error
	suite.Require().NoError(err)
	suite.Equal(models.ShowStatusPending, show.Status)
	suite.NotNil(show.DuplicateOfShowID)
}

func (suite *DiscoveryIntegrationTestSuite) TestImportEvents_RejectedShowSkipped() {
	// Create a rejected show at Valley Bar on a specific date
	venue := &models.Venue{Name: "Valley Bar", City: "Phoenix", State: "AZ"}
	err := suite.db.Create(venue).Error
	suite.Require().NoError(err)

	show := &models.Show{
		Title:     "Old Rejected Show",
		EventDate: time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC),
		Status:    models.ShowStatusRejected,
		Source:    models.ShowSourceUser,
	}
	err = suite.db.Create(show).Error
	suite.Require().NoError(err)

	showVenue := models.ShowVenue{ShowID: show.ID, VenueID: venue.ID}
	err = suite.db.Exec("INSERT INTO show_venues (show_id, venue_id) VALUES (?, ?)", showVenue.ShowID, showVenue.VenueID).Error
	suite.Require().NoError(err)

	// Try to import an event at the same venue and date
	events := []DiscoveredEvent{
		suite.makeEvent("evt-005", "Some New Band", "valley-bar", "2026-10-01", []string{"Some New Band"}),
	}

	result, err := suite.svc.ImportEvents(events, false)
	suite.Require().NoError(err)
	suite.Equal(1, result.Rejected)
	suite.Contains(result.Messages[0], "REJECTED")
}

func (suite *DiscoveryIntegrationTestSuite) TestImportEvents_DryRun() {
	events := []DiscoveredEvent{
		suite.makeEvent("evt-006", "Dry Run Band", "valley-bar", "2026-11-01", []string{"Dry Run Band"}),
	}

	result, err := suite.svc.ImportEvents(events, true)
	suite.Require().NoError(err)
	suite.Equal(1, result.Total)
	// In dry run, nothing is actually imported but message says "WOULD IMPORT"
	suite.Contains(result.Messages[0], "WOULD IMPORT")

	// Verify nothing was actually created
	var count int64
	suite.db.Model(&models.Show{}).Where("source_event_id = ?", "evt-006").Count(&count)
	suite.Zero(count)
}

// =============================================================================
// CheckEvents tests
// =============================================================================

func (suite *DiscoveryIntegrationTestSuite) TestCheckEvents_Found() {
	// Import an event first
	events := []DiscoveredEvent{
		suite.makeEvent("evt-check-1", "Check Band", "valley-bar", "2026-12-01", []string{"Check Band"}),
	}
	_, err := suite.svc.ImportEvents(events, false)
	suite.Require().NoError(err)

	// Check it
	checkInputs := []CheckEventInput{
		{ID: "evt-check-1", VenueSlug: "valley-bar"},
	}
	result, err := suite.svc.CheckEvents(checkInputs)
	suite.Require().NoError(err)

	status, ok := result.Events["evt-check-1"]
	suite.True(ok, "event should be found")
	suite.True(status.Exists)
	suite.Equal("approved", status.Status)
	suite.NotZero(status.ShowID)
}

func (suite *DiscoveryIntegrationTestSuite) TestCheckEvents_NotFound() {
	checkInputs := []CheckEventInput{
		{ID: "evt-nonexistent", VenueSlug: "valley-bar"},
	}
	result, err := suite.svc.CheckEvents(checkInputs)
	suite.Require().NoError(err)
	suite.Empty(result.Events)
}

func (suite *DiscoveryIntegrationTestSuite) TestCheckEvents_EmptyInput() {
	result, err := suite.svc.CheckEvents([]CheckEventInput{})
	suite.Require().NoError(err)
	suite.Empty(result.Events)
}
