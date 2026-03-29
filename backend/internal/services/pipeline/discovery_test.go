package pipeline

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"

	"psychic-homily-backend/internal/models"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/testutil"
	"psychic-homily-backend/internal/utils"
)

// =============================================================================
// UNIT TESTS — parseEventDate
// =============================================================================

func TestParseEventDate_ISODateOnly(t *testing.T) {
	result, err := parseEventDate("2026-01-25", nil, "AZ")
	assert.NoError(t, err)
	assert.Equal(t, time.Date(2026, 1, 25, 0, 0, 0, 0, time.UTC), result)
}

func TestParseEventDate_ISOTimestamp(t *testing.T) {
	result, err := parseEventDate("2026-01-25T19:00:00Z", nil, "AZ")
	assert.NoError(t, err)
	assert.Equal(t, time.Date(2026, 1, 25, 19, 0, 0, 0, time.UTC), result)
}

func TestParseEventDate_RFC3339(t *testing.T) {
	result, err := parseEventDate("2026-01-25T19:00:00-07:00", nil, "AZ")
	assert.NoError(t, err)
	expected := time.Date(2026, 1, 26, 2, 0, 0, 0, time.UTC)
	assert.Equal(t, expected, result)
}

func TestParseEventDate_WithShowTimePM_AZ(t *testing.T) {
	showTime := "7:00 pm"
	result, err := parseEventDate("2026-01-25", &showTime, "AZ")
	assert.NoError(t, err)
	// 7:00 PM Phoenix (UTC-7) = 2:00 AM UTC next day
	assert.Equal(t, time.Date(2026, 1, 26, 2, 0, 0, 0, time.UTC), result)
}

func TestParseEventDate_WithShowTimePM_NY(t *testing.T) {
	showTime := "8:00 pm"
	result, err := parseEventDate("2026-01-25", &showTime, "NY")
	assert.NoError(t, err)
	// 8:00 PM New York (UTC-5 in January) = 1:00 AM UTC next day
	assert.Equal(t, time.Date(2026, 1, 26, 1, 0, 0, 0, time.UTC), result)
}

func TestParseEventDate_WithShowTimeAM_AZ(t *testing.T) {
	showTime := "11:00 am"
	result, err := parseEventDate("2026-01-25", &showTime, "AZ")
	assert.NoError(t, err)
	// 11:00 AM Phoenix (UTC-7) = 6:00 PM UTC same day
	assert.Equal(t, time.Date(2026, 1, 25, 18, 0, 0, 0, time.UTC), result)
}

func TestParseEventDate_12PM_AZ(t *testing.T) {
	showTime := "12:00 pm"
	result, err := parseEventDate("2026-01-25", &showTime, "AZ")
	assert.NoError(t, err)
	// 12:00 PM Phoenix (UTC-7) = 7:00 PM UTC same day
	assert.Equal(t, time.Date(2026, 1, 25, 19, 0, 0, 0, time.UTC), result)
}

func TestParseEventDate_12AM_AZ(t *testing.T) {
	showTime := "12:00 am"
	result, err := parseEventDate("2026-01-25", &showTime, "AZ")
	assert.NoError(t, err)
	// 12:00 AM Phoenix (UTC-7) = 7:00 AM UTC same day
	assert.Equal(t, time.Date(2026, 1, 25, 7, 0, 0, 0, time.UTC), result)
}

func TestParseEventDate_WithSpacesInTime(t *testing.T) {
	showTime := " 7:00 PM "
	result, err := parseEventDate("2026-01-25", &showTime, "AZ")
	assert.NoError(t, err)
	// 7:00 PM Phoenix (UTC-7) = 2:00 AM UTC next day
	assert.Equal(t, time.Date(2026, 1, 26, 2, 0, 0, 0, time.UTC), result)
}

func TestParseEventDate_EmptyShowTime(t *testing.T) {
	showTime := ""
	result, err := parseEventDate("2026-01-25", &showTime, "AZ")
	assert.NoError(t, err)
	assert.Equal(t, time.Date(2026, 1, 25, 0, 0, 0, 0, time.UTC), result)
}

func TestParseEventDate_InvalidDate(t *testing.T) {
	_, err := parseEventDate("not-a-date", nil, "AZ")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unable to parse date")
}

func TestParseEventDate_NilShowTime(t *testing.T) {
	result, err := parseEventDate("2026-01-25", nil, "AZ")
	assert.NoError(t, err)
	assert.Equal(t, time.Date(2026, 1, 25, 0, 0, 0, 0, time.UTC), result)
}

func TestParseEventDate_UnparseableTime(t *testing.T) {
	showTime := "doors at 7"
	result, err := parseEventDate("2026-01-25", &showTime, "AZ")
	assert.NoError(t, err)
	// Unparseable time is silently ignored, date is returned without time
	assert.Equal(t, time.Date(2026, 1, 25, 0, 0, 0, 0, time.UTC), result)
}

func TestParseEventDate_UnknownStateDefaultsToPhoenix(t *testing.T) {
	showTime := "8:00 pm"
	result, err := parseEventDate("2026-01-25", &showTime, "XX")
	assert.NoError(t, err)
	// Unknown state defaults to America/Phoenix (UTC-7)
	// 8:00 PM Phoenix = 3:00 AM UTC next day
	assert.Equal(t, time.Date(2026, 1, 26, 3, 0, 0, 0, time.UTC), result)
}

func TestParseEventDate_DSTAwareState(t *testing.T) {
	// California in summer (PDT = UTC-7), vs winter (PST = UTC-8)
	showTime := "8:00 pm"

	// January (PST = UTC-8): 8:00 PM = 4:00 AM UTC next day
	winter, err := parseEventDate("2026-01-25", &showTime, "CA")
	assert.NoError(t, err)
	assert.Equal(t, time.Date(2026, 1, 26, 4, 0, 0, 0, time.UTC), winter)

	// July (PDT = UTC-7): 8:00 PM = 3:00 AM UTC next day
	summer, err := parseEventDate("2026-07-15", &showTime, "CA")
	assert.NoError(t, err)
	assert.Equal(t, time.Date(2026, 7, 16, 3, 0, 0, 0, time.UTC), summer)
}

// =============================================================================
// UNIT TESTS — getTimezoneForState
// =============================================================================

func TestGetTimezoneForState(t *testing.T) {
	assert.Equal(t, "America/Phoenix", getTimezoneForState("AZ"))
	assert.Equal(t, "America/Phoenix", getTimezoneForState("az"))
	assert.Equal(t, "America/Los_Angeles", getTimezoneForState("CA"))
	assert.Equal(t, "America/Denver", getTimezoneForState("CO"))
	assert.Equal(t, "America/Chicago", getTimezoneForState("TX"))
	assert.Equal(t, "America/New_York", getTimezoneForState("NY"))
	// Unknown state defaults to Phoenix
	assert.Equal(t, "America/Phoenix", getTimezoneForState("XX"))
	assert.Equal(t, "America/Phoenix", getTimezoneForState(""))
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
// UNIT TESTS — normalizeSetType
// =============================================================================

func TestNormalizeSetType(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"headliner", "headliner"},
		{"Headliner", "headliner"},
		{"HEADLINER", "headliner"},
		{"support", "opener"}, // support maps to opener
		{"Support", "opener"},
		{"opener", "opener"},
		{"special_guest", "special_guest"},
		{"performer", "performer"},
		{"dj", "performer"}, // dj maps to performer
		{"DJ", "performer"},
		{"host", "performer"}, // host maps to performer
		{"Host", "performer"},
		{"", ""},                       // empty returns empty
		{"unknown", ""},                // unknown returns empty
		{"  headliner  ", "headliner"}, // whitespace trimmed
		{"  support ", "opener"},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("input_%s", tt.input), func(t *testing.T) {
			result := normalizeSetType(tt.input)
			assert.Equal(t, tt.expected, result, "normalizeSetType(%q)", tt.input)
		})
	}
}

// =============================================================================
// UNIT TESTS — Constructor
// =============================================================================

func TestNewDiscoveryService(t *testing.T) {
	svc := NewDiscoveryService(nil, nil)
	assert.NotNil(t, svc)
}

// =============================================================================
// testVenueFinderCreator — lightweight impl of venueFinderCreator for tests
// =============================================================================

// testVenueFinderCreator implements the venueFinderCreator interface using direct
// GORM queries, replicating the core FindOrCreateVenue behavior from VenueService.
type testVenueFinderCreator struct {
	db *gorm.DB
}

func (v *testVenueFinderCreator) FindOrCreateVenue(name, city, state string, address, zipcode *string, txDB *gorm.DB, isAdmin bool) (*models.Venue, bool, error) {
	query := txDB
	if query == nil {
		query = v.db
	}
	if query == nil {
		return nil, false, fmt.Errorf("database not initialized")
	}

	// Check if venue already exists by name and city
	var venue models.Venue
	err := query.Where("LOWER(name) = LOWER(?) AND LOWER(city) = LOWER(?)", name, city).First(&venue).Error
	if err == nil {
		// Venue exists — backfill slug if missing
		if venue.Slug == nil {
			baseSlug := utils.GenerateVenueSlug(venue.Name, venue.City, venue.State)
			slug := utils.GenerateUniqueSlug(baseSlug, func(candidate string) bool {
				var count int64
				query.Model(&models.Venue{}).Where("slug = ?", candidate).Count(&count)
				return count > 0
			})
			venue.Slug = &slug
			query.Model(&venue).Update("slug", slug)
		}
		return &venue, false, nil
	} else if err != gorm.ErrRecordNotFound {
		return nil, false, fmt.Errorf("failed to check existing venue: %w", err)
	}

	// Venue doesn't exist, create it
	baseSlug := utils.GenerateVenueSlug(name, city, state)
	slug := utils.GenerateUniqueSlug(baseSlug, func(candidate string) bool {
		var count int64
		query.Model(&models.Venue{}).Where("slug = ?", candidate).Count(&count)
		return count > 0
	})

	venue = models.Venue{
		Name:  name,
		City:  city,
		State: state,
		Slug:  &slug,
	}
	if address != nil {
		venue.Address = address
	}
	if err := query.Create(&venue).Error; err != nil {
		return nil, false, fmt.Errorf("failed to create venue: %w", err)
	}

	return &venue, true, nil
}

// =============================================================================
// INTEGRATION TESTS (With Real Database)
// =============================================================================

type DiscoveryIntegrationTestSuite struct {
	suite.Suite
	testDB *testutil.TestDatabase
	db     *gorm.DB
	svc    *DiscoveryService
}

func (suite *DiscoveryIntegrationTestSuite) SetupSuite() {
	suite.testDB = testutil.SetupTestPostgres(suite.T())
	suite.db = suite.testDB.DB

	venueSvc := &testVenueFinderCreator{db: suite.testDB.DB}
	suite.svc = NewDiscoveryService(suite.testDB.DB, venueSvc)
}

func (suite *DiscoveryIntegrationTestSuite) TearDownSuite() {
	suite.testDB.Cleanup()
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

func (suite *DiscoveryIntegrationTestSuite) makeEvent(id, title, venueSlug, date string, artists []string) contracts.DiscoveredEvent {
	return contracts.DiscoveredEvent{
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
	events := []contracts.DiscoveredEvent{
		suite.makeEvent("evt-001", "The National", "valley-bar", "2026-06-15", []string{"The National"}),
	}

	result, err := suite.svc.ImportEvents(events, false, false, models.ShowStatusApproved)
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
	events := []contracts.DiscoveredEvent{
		suite.makeEvent("evt-002", "Radiohead", "valley-bar", "2026-07-01", []string{"Radiohead"}),
	}

	// First import
	result1, err := suite.svc.ImportEvents(events, false, false, models.ShowStatusApproved)
	suite.Require().NoError(err)
	suite.Equal(1, result1.Imported)

	// Second import — same source_venue + source_event_id
	result2, err := suite.svc.ImportEvents(events, false, false, models.ShowStatusApproved)
	suite.Require().NoError(err)
	suite.Equal(0, result2.Imported)
	suite.Equal(1, result2.Duplicates)
}

func (suite *DiscoveryIntegrationTestSuite) TestImportEvents_UnknownVenue() {
	events := []contracts.DiscoveredEvent{
		suite.makeEvent("evt-003", "Test Band", "unknown-venue-xyz", "2026-08-01", []string{"Test Band"}),
	}

	result, err := suite.svc.ImportEvents(events, false, false, models.ShowStatusApproved)
	suite.Require().NoError(err)
	suite.Equal(1, result.Errors)
	suite.Contains(result.Messages[0], "Unknown venue slug")
}

func (suite *DiscoveryIntegrationTestSuite) TestImportEvents_HeadlinerDuplicate() {
	// Import one show first
	events1 := []contracts.DiscoveredEvent{
		suite.makeEvent("evt-004a", "Bon Iver", "valley-bar", "2026-09-01", []string{"Bon Iver"}),
	}
	result1, err := suite.svc.ImportEvents(events1, false, false, models.ShowStatusApproved)
	suite.Require().NoError(err)
	suite.Equal(1, result1.Imported)

	// Import another event with the same headliner at the same venue on the same date
	// but different source_event_id — should be blocked as duplicate
	events2 := []contracts.DiscoveredEvent{
		suite.makeEvent("evt-004b", "Bon Iver (Late Show)", "valley-bar", "2026-09-01", []string{"Bon Iver"}),
	}
	result2, err := suite.svc.ImportEvents(events2, false, false, models.ShowStatusApproved)
	suite.Require().NoError(err)
	suite.Equal(1, result2.Duplicates)

	// Verify the second show was NOT created
	var count int64
	suite.db.Model(&models.Show{}).Where("source_event_id = ?", "evt-004b").Count(&count)
	suite.Equal(int64(0), count)
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
	events := []contracts.DiscoveredEvent{
		suite.makeEvent("evt-005", "Some New Band", "valley-bar", "2026-10-01", []string{"Some New Band"}),
	}

	result, err := suite.svc.ImportEvents(events, false, false, models.ShowStatusApproved)
	suite.Require().NoError(err)
	suite.Equal(1, result.Rejected)
	suite.Contains(result.Messages[0], "REJECTED")
}

func (suite *DiscoveryIntegrationTestSuite) TestImportEvents_DryRun() {
	events := []contracts.DiscoveredEvent{
		suite.makeEvent("evt-006", "Dry Run Band", "valley-bar", "2026-11-01", []string{"Dry Run Band"}),
	}

	result, err := suite.svc.ImportEvents(events, true, false, models.ShowStatusApproved)
	suite.Require().NoError(err)
	suite.Equal(1, result.Total)
	// In dry run, nothing is actually imported but message says "WOULD IMPORT"
	suite.Contains(result.Messages[0], "WOULD IMPORT")

	// Verify nothing was actually created
	var count int64
	suite.db.Model(&models.Show{}).Where("source_event_id = ?", "evt-006").Count(&count)
	suite.Zero(count)
}

func (suite *DiscoveryIntegrationTestSuite) TestImportEvents_PendingStatus() {
	events := []contracts.DiscoveredEvent{
		suite.makeEvent("evt-pending-1", "Pending Band", "valley-bar", "2026-11-15", []string{"Pending Band"}),
	}

	result, err := suite.svc.ImportEvents(events, false, false, models.ShowStatusPending)
	suite.Require().NoError(err)
	suite.Equal(1, result.Imported)

	// Verify show was created with pending status
	var show models.Show
	err = suite.db.Where("source_event_id = ?", "evt-pending-1").First(&show).Error
	suite.Require().NoError(err)
	suite.Equal(models.ShowStatusPending, show.Status)
}

// =============================================================================
// CheckEvents tests
// =============================================================================

func (suite *DiscoveryIntegrationTestSuite) TestCheckEvents_Found() {
	// Import an event first
	events := []contracts.DiscoveredEvent{
		suite.makeEvent("evt-check-1", "Check Band", "valley-bar", "2026-12-01", []string{"Check Band"}),
	}
	_, err := suite.svc.ImportEvents(events, false, false, models.ShowStatusApproved)
	suite.Require().NoError(err)

	// Check it
	checkInputs := []contracts.CheckEventInput{
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
	checkInputs := []contracts.CheckEventInput{
		{ID: "evt-nonexistent", VenueSlug: "valley-bar"},
	}
	result, err := suite.svc.CheckEvents(checkInputs)
	suite.Require().NoError(err)
	suite.Empty(result.Events)
}

func (suite *DiscoveryIntegrationTestSuite) TestCheckEvents_EmptyInput() {
	result, err := suite.svc.CheckEvents([]contracts.CheckEventInput{})
	suite.Require().NoError(err)
	suite.Empty(result.Events)
}

// =============================================================================
// BillingArtists import tests (PSY-30)
// =============================================================================

func (suite *DiscoveryIntegrationTestSuite) TestImportEvents_WithBillingArtists() {
	events := []contracts.DiscoveredEvent{
		{
			ID:        "evt-billing-1",
			Title:     "Main Act with Support",
			Date:      "2026-12-15",
			Venue:     "Valley Bar",
			VenueSlug: "valley-bar",
			Artists:   []string{"Main Act", "Support Band", "Opener Band"},
			BillingArtists: []contracts.DiscoveredArtist{
				{Name: "Main Act", SetType: "headliner", BillingOrder: 1},
				{Name: "Support Band", SetType: "support", BillingOrder: 2},
				{Name: "Opener Band", SetType: "opener", BillingOrder: 3},
			},
			ScrapedAt: time.Now().UTC().Format(time.RFC3339),
		},
	}

	result, err := suite.svc.ImportEvents(events, false, false, models.ShowStatusApproved)
	suite.Require().NoError(err)
	suite.Equal(1, result.Imported)

	// Verify show was created
	var show models.Show
	err = suite.db.Where("source_event_id = ?", "evt-billing-1").First(&show).Error
	suite.Require().NoError(err)

	// Verify show_artists have correct set_type and position
	var showArtists []models.ShowArtist
	err = suite.db.Where("show_id = ?", show.ID).Order("position").Find(&showArtists).Error
	suite.Require().NoError(err)
	suite.Require().Len(showArtists, 3)

	// Main Act: headliner, position 0 (billing_order 1 → position 0)
	suite.Equal("headliner", showArtists[0].SetType)
	suite.Equal(0, showArtists[0].Position)

	// Support Band: normalized "support" → "opener", position 1 (billing_order 2 → position 1)
	suite.Equal("opener", showArtists[1].SetType)
	suite.Equal(1, showArtists[1].Position)

	// Opener Band: "opener", position 2 (billing_order 3 → position 2)
	suite.Equal("opener", showArtists[2].SetType)
	suite.Equal(2, showArtists[2].Position)
}

func (suite *DiscoveryIntegrationTestSuite) TestImportEvents_FallbackWithoutBillingArtists() {
	// When BillingArtists is empty, should fall back to old logic:
	// position 0 = headliner, others = opener
	events := []contracts.DiscoveredEvent{
		{
			ID:        "evt-no-billing-1",
			Title:     "Legacy Import",
			Date:      "2026-12-20",
			Venue:     "Valley Bar",
			VenueSlug: "valley-bar",
			Artists:   []string{"First Band", "Second Band"},
			ScrapedAt: time.Now().UTC().Format(time.RFC3339),
		},
	}

	result, err := suite.svc.ImportEvents(events, false, false, models.ShowStatusApproved)
	suite.Require().NoError(err)
	suite.Equal(1, result.Imported)

	// Verify show_artists have default set_type
	var show models.Show
	err = suite.db.Where("source_event_id = ?", "evt-no-billing-1").First(&show).Error
	suite.Require().NoError(err)

	var showArtists []models.ShowArtist
	err = suite.db.Where("show_id = ?", show.ID).Order("position").Find(&showArtists).Error
	suite.Require().NoError(err)
	suite.Require().Len(showArtists, 2)

	suite.Equal("headliner", showArtists[0].SetType)
	suite.Equal(0, showArtists[0].Position)
	suite.Equal("opener", showArtists[1].SetType)
	suite.Equal(1, showArtists[1].Position)
}

func (suite *DiscoveryIntegrationTestSuite) TestImportEvents_WithSpecialGuestAndDJ() {
	events := []contracts.DiscoveredEvent{
		{
			ID:        "evt-special-1",
			Title:     "Complex Bill",
			Date:      "2026-12-25",
			Venue:     "Valley Bar",
			VenueSlug: "valley-bar",
			Artists:   []string{"Main Act", "Guest Artist", "DJ Spinz"},
			BillingArtists: []contracts.DiscoveredArtist{
				{Name: "Main Act", SetType: "headliner", BillingOrder: 1},
				{Name: "Guest Artist", SetType: "special_guest", BillingOrder: 2},
				{Name: "DJ Spinz", SetType: "dj", BillingOrder: 3},
			},
			ScrapedAt: time.Now().UTC().Format(time.RFC3339),
		},
	}

	result, err := suite.svc.ImportEvents(events, false, false, models.ShowStatusApproved)
	suite.Require().NoError(err)
	suite.Equal(1, result.Imported)

	var show models.Show
	err = suite.db.Where("source_event_id = ?", "evt-special-1").First(&show).Error
	suite.Require().NoError(err)

	var showArtists []models.ShowArtist
	err = suite.db.Where("show_id = ?", show.ID).Order("position").Find(&showArtists).Error
	suite.Require().NoError(err)
	suite.Require().Len(showArtists, 3)

	suite.Equal("headliner", showArtists[0].SetType)
	suite.Equal("special_guest", showArtists[1].SetType)
	suite.Equal("performer", showArtists[2].SetType) // dj normalized to performer
}

func (suite *DiscoveryIntegrationTestSuite) TestImportEvents_HeadlinerDuplicate_WithBillingArtists() {
	// Import a show with billing data
	events1 := []contracts.DiscoveredEvent{
		{
			ID:        "evt-billing-001",
			Title:     "Big Show Night",
			Date:      "2026-11-01",
			Venue:     "Valley Bar",
			VenueSlug: "valley-bar",
			Artists:   []string{"Star Band", "Opener"},
			BillingArtists: []contracts.DiscoveredArtist{
				{Name: "Star Band", SetType: "headliner", BillingOrder: 1},
				{Name: "Opener", SetType: "opener", BillingOrder: 2},
			},
			ScrapedAt: time.Now().UTC().Format(time.RFC3339),
		},
	}
	result1, err := suite.svc.ImportEvents(events1, false, false, models.ShowStatusApproved)
	suite.Require().NoError(err)
	suite.Equal(1, result1.Imported)

	// Import another event with the same headliner via billing data but different source_event_id
	events2 := []contracts.DiscoveredEvent{
		{
			ID:        "evt-billing-002",
			Title:     "Big Show Night (Late)",
			Date:      "2026-11-01",
			Venue:     "Valley Bar",
			VenueSlug: "valley-bar",
			Artists:   []string{"Star Band"},
			BillingArtists: []contracts.DiscoveredArtist{
				{Name: "Star Band", SetType: "headliner", BillingOrder: 1},
			},
			ScrapedAt: time.Now().UTC().Format(time.RFC3339),
		},
	}
	result2, err := suite.svc.ImportEvents(events2, false, false, models.ShowStatusApproved)
	suite.Require().NoError(err)
	suite.Equal(1, result2.Duplicates)

	// Verify the second show was NOT created
	var count int64
	suite.db.Model(&models.Show{}).Where("source_event_id = ?", "evt-billing-002").Count(&count)
	suite.Equal(int64(0), count)
}

func (suite *DiscoveryIntegrationTestSuite) TestImportEvents_HeadlinerDuplicate_Position0Match() {
	// Import a show where the headliner is assigned position=0 but set_type is just "performer"
	// This simulates shows created without explicit headliner tagging
	venue := &models.Venue{Name: "Valley Bar", City: "Phoenix", State: "AZ"}
	suite.Require().NoError(suite.db.Create(venue).Error)

	artist := &models.Artist{Name: "Position Zero Band"}
	slug := utils.GenerateArtistSlug(artist.Name)
	artist.Slug = &slug
	suite.Require().NoError(suite.db.Create(artist).Error)

	show := &models.Show{
		Title:     "Existing Show",
		EventDate: time.Date(2026, 11, 15, 2, 0, 0, 0, time.UTC),
		Status:    models.ShowStatusApproved,
		Source:    models.ShowSourceUser,
	}
	suite.Require().NoError(suite.db.Create(show).Error)

	suite.Require().NoError(suite.db.Exec(
		"INSERT INTO show_venues (show_id, venue_id) VALUES (?, ?)", show.ID, venue.ID).Error)
	suite.Require().NoError(suite.db.Exec(
		"INSERT INTO show_artists (show_id, artist_id, position, set_type) VALUES (?, ?, 0, 'performer')",
		show.ID, artist.ID).Error)

	// Now try to import an event with the same artist at the same venue on the same date
	events := []contracts.DiscoveredEvent{
		suite.makeEvent("evt-pos0-001", "Position Zero Band Live", "valley-bar", "2026-11-15", []string{"Position Zero Band"}),
	}
	result, err := suite.svc.ImportEvents(events, false, false, models.ShowStatusApproved)
	suite.Require().NoError(err)
	suite.Equal(1, result.Duplicates)
	suite.Contains(result.Messages[0], "DUPLICATE")
}

// =============================================================================
// UNIT TESTS — resolveHeadlinerName
// =============================================================================

func TestResolveHeadlinerName_BillingArtists_ExplicitHeadliner(t *testing.T) {
	svc := &DiscoveryService{}
	event := &contracts.DiscoveredEvent{
		BillingArtists: []contracts.DiscoveredArtist{
			{Name: "Opener", SetType: "opener", BillingOrder: 2},
			{Name: "Headliner Band", SetType: "headliner", BillingOrder: 1},
		},
		Artists: []string{"Opener", "Headliner Band"},
	}
	assert.Equal(t, "Headliner Band", svc.resolveHeadlinerName(event))
}

func TestResolveHeadlinerName_BillingArtists_ByBillingOrder(t *testing.T) {
	svc := &DiscoveryService{}
	event := &contracts.DiscoveredEvent{
		BillingArtists: []contracts.DiscoveredArtist{
			{Name: "Second Act", SetType: "performer", BillingOrder: 2},
			{Name: "First Act", SetType: "performer", BillingOrder: 1},
		},
	}
	// Should return the one with lowest billing order
	assert.Equal(t, "First Act", svc.resolveHeadlinerName(event))
}

func TestResolveHeadlinerName_BillingArtists_NoHeadlinerNoOrder(t *testing.T) {
	svc := &DiscoveryService{}
	event := &contracts.DiscoveredEvent{
		BillingArtists: []contracts.DiscoveredArtist{
			{Name: "First Entry", SetType: "performer"},
			{Name: "Second Entry", SetType: "performer"},
		},
	}
	// Should return first entry when no explicit headliner or billing order
	assert.Equal(t, "First Entry", svc.resolveHeadlinerName(event))
}

func TestResolveHeadlinerName_FallbackToArtistsList(t *testing.T) {
	svc := &DiscoveryService{}
	event := &contracts.DiscoveredEvent{
		Artists: []string{"Main Band", "Support Band"},
	}
	assert.Equal(t, "Main Band", svc.resolveHeadlinerName(event))
}

func TestResolveHeadlinerName_NoArtists(t *testing.T) {
	svc := &DiscoveryService{}
	event := &contracts.DiscoveredEvent{}
	assert.Equal(t, "", svc.resolveHeadlinerName(event))
}
