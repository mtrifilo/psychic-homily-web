package admin

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"

	authm "psychic-homily-backend/internal/models/auth"
	catalogm "psychic-homily-backend/internal/models/catalog"
	communitym "psychic-homily-backend/internal/models/community"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/testutil"
)

// =============================================================================
// UNIT TESTS (No Database Required)
// =============================================================================

func TestEntityReportModel_Validation(t *testing.T) {
	// Valid entity types
	assert.True(t, communitym.IsValidEntityReportEntityType("artist"))
	assert.True(t, communitym.IsValidEntityReportEntityType("venue"))
	assert.True(t, communitym.IsValidEntityReportEntityType("festival"))
	assert.True(t, communitym.IsValidEntityReportEntityType("show"))
	assert.False(t, communitym.IsValidEntityReportEntityType(""))
	assert.False(t, communitym.IsValidEntityReportEntityType("release"))
	assert.False(t, communitym.IsValidEntityReportEntityType("label"))

	// Valid report types per entity
	assert.True(t, communitym.IsValidReportType("artist", "inaccurate"))
	assert.True(t, communitym.IsValidReportType("artist", "duplicate"))
	assert.True(t, communitym.IsValidReportType("artist", "wrong_image"))
	assert.True(t, communitym.IsValidReportType("artist", "removal_request"))
	assert.True(t, communitym.IsValidReportType("artist", "missing_info"))
	assert.False(t, communitym.IsValidReportType("artist", "cancelled"))

	assert.True(t, communitym.IsValidReportType("venue", "closed_permanently"))
	assert.True(t, communitym.IsValidReportType("venue", "wrong_address"))
	assert.True(t, communitym.IsValidReportType("venue", "duplicate"))
	assert.False(t, communitym.IsValidReportType("venue", "cancelled"))

	assert.True(t, communitym.IsValidReportType("festival", "cancelled"))
	assert.True(t, communitym.IsValidReportType("festival", "wrong_dates"))
	assert.False(t, communitym.IsValidReportType("festival", "sold_out"))

	assert.True(t, communitym.IsValidReportType("show", "cancelled"))
	assert.True(t, communitym.IsValidReportType("show", "sold_out"))
	assert.True(t, communitym.IsValidReportType("show", "wrong_venue"))
	assert.True(t, communitym.IsValidReportType("show", "wrong_date"))
	assert.False(t, communitym.IsValidReportType("show", "removal_request"))

	// Invalid entity type
	assert.False(t, communitym.IsValidReportType("release", "inaccurate"))
}

func TestValidReportTypesForEntity(t *testing.T) {
	artistTypes := communitym.ValidReportTypesForEntity("artist")
	assert.Len(t, artistTypes, 5)

	venueTypes := communitym.ValidReportTypesForEntity("venue")
	assert.Len(t, venueTypes, 5)

	festivalTypes := communitym.ValidReportTypesForEntity("festival")
	assert.Len(t, festivalTypes, 4)

	showTypes := communitym.ValidReportTypesForEntity("show")
	assert.Len(t, showTypes, 5)

	unknownTypes := communitym.ValidReportTypesForEntity("release")
	assert.Nil(t, unknownTypes)
}

func TestValidEntityReportEntityTypes(t *testing.T) {
	types := communitym.ValidEntityReportEntityTypes()
	assert.Len(t, types, 5)
	assert.Contains(t, types, "artist")
	assert.Contains(t, types, "venue")
	assert.Contains(t, types, "festival")
	assert.Contains(t, types, "show")
	assert.Contains(t, types, "comment")
}

// =============================================================================
// INTEGRATION TESTS (With Real Database)
// =============================================================================

type EntityReportServiceIntegrationTestSuite struct {
	suite.Suite
	testDB *testutil.TestDatabase
	db     *gorm.DB
	svc    *EntityReportService
}

func (s *EntityReportServiceIntegrationTestSuite) SetupSuite() {
	s.testDB = testutil.SetupTestPostgres(s.T())
	s.db = s.testDB.DB
	s.svc = NewEntityReportService(s.db)
}

func (s *EntityReportServiceIntegrationTestSuite) TearDownSuite() {
	s.testDB.Cleanup()
}

func (s *EntityReportServiceIntegrationTestSuite) TearDownTest() {
	sqlDB, err := s.db.DB()
	s.Require().NoError(err)
	_, _ = sqlDB.Exec("DELETE FROM entity_reports")
	_, _ = sqlDB.Exec("DELETE FROM artists")
	_, _ = sqlDB.Exec("DELETE FROM venues")
	_, _ = sqlDB.Exec("DELETE FROM festivals")
	_, _ = sqlDB.Exec("DELETE FROM shows")
	_, _ = sqlDB.Exec("DELETE FROM users")
}

func TestEntityReportServiceIntegrationSuite(t *testing.T) {
	suite.Run(t, new(EntityReportServiceIntegrationTestSuite))
}

// =============================================================================
// HELPERS
// =============================================================================

func (s *EntityReportServiceIntegrationTestSuite) createTestUser() *authm.User {
	user := &authm.User{
		Email:         stringPtr(fmt.Sprintf("er-user-%d@test.com", time.Now().UnixNano())),
		Username:      stringPtr(fmt.Sprintf("er-user-%d", time.Now().UnixNano())),
		FirstName:     stringPtr("Test"),
		LastName:      stringPtr("User"),
		IsActive:      true,
		EmailVerified: true,
	}
	err := s.db.Create(user).Error
	s.Require().NoError(err)
	return user
}

func (s *EntityReportServiceIntegrationTestSuite) createTestArtist(name string) *catalogm.Artist {
	slug := fmt.Sprintf("test-artist-%d", time.Now().UnixNano())
	artist := &catalogm.Artist{
		Name: name,
		Slug: &slug,
	}
	err := s.db.Create(artist).Error
	s.Require().NoError(err)
	return artist
}

func (s *EntityReportServiceIntegrationTestSuite) createTestVenue(name string) *catalogm.Venue {
	slug := fmt.Sprintf("test-venue-%d", time.Now().UnixNano())
	venue := &catalogm.Venue{
		Name:  name,
		Slug:  &slug,
		City:  "Phoenix",
		State: "AZ",
	}
	err := s.db.Create(venue).Error
	s.Require().NoError(err)
	return venue
}

func (s *EntityReportServiceIntegrationTestSuite) createTestFestival(name string) *catalogm.Festival {
	slug := fmt.Sprintf("test-festival-%d", time.Now().UnixNano())
	festival := &catalogm.Festival{
		Name:        name,
		Slug:        slug,
		SeriesSlug:  slug,
		EditionYear: 2026,
		StartDate:   "2026-06-01",
		EndDate:     "2026-06-03",
	}
	err := s.db.Create(festival).Error
	s.Require().NoError(err)
	return festival
}

func (s *EntityReportServiceIntegrationTestSuite) createTestShow() *catalogm.Show {
	show := &catalogm.Show{
		Title:     "Test Show",
		EventDate: time.Date(2026, 6, 15, 20, 0, 0, 0, time.UTC),
		Status:    "approved",
	}
	err := s.db.Create(show).Error
	s.Require().NoError(err)
	return show
}

func (s *EntityReportServiceIntegrationTestSuite) createReport(entityType string, entityID, userID uint, reportType string) *contracts.EntityReportResponse {
	resp, err := s.svc.CreateEntityReport(&contracts.CreateEntityReportRequest{
		EntityType: entityType,
		EntityID:   entityID,
		UserID:     userID,
		ReportType: reportType,
	})
	s.Require().NoError(err)
	s.Require().NotNil(resp)
	return resp
}

// =============================================================================
// CreateEntityReport tests
// =============================================================================

func (s *EntityReportServiceIntegrationTestSuite) TestCreateEntityReport_ArtistSuccess() {
	user := s.createTestUser()
	artist := s.createTestArtist("Test Artist")

	details := "This artist profile has wrong bio info"
	resp, err := s.svc.CreateEntityReport(&contracts.CreateEntityReportRequest{
		EntityType: "artist",
		EntityID:   artist.ID,
		UserID:     user.ID,
		ReportType: "inaccurate",
		Details:    &details,
	})

	s.NoError(err)
	s.Require().NotNil(resp)
	s.Equal("artist", resp.EntityType)
	s.Equal(artist.ID, resp.EntityID)
	s.Equal(user.ID, resp.ReportedBy)
	s.Equal("inaccurate", resp.ReportType)
	s.Equal("pending", resp.Status)
	s.Require().NotNil(resp.Details)
	s.Equal(details, *resp.Details)
	s.NotEmpty(resp.ReporterName)
}

func (s *EntityReportServiceIntegrationTestSuite) TestCreateEntityReport_VenueSuccess() {
	user := s.createTestUser()
	venue := s.createTestVenue("Test Venue")

	resp, err := s.svc.CreateEntityReport(&contracts.CreateEntityReportRequest{
		EntityType: "venue",
		EntityID:   venue.ID,
		UserID:     user.ID,
		ReportType: "closed_permanently",
	})

	s.NoError(err)
	s.Require().NotNil(resp)
	s.Equal("venue", resp.EntityType)
	s.Equal("closed_permanently", resp.ReportType)
}

func (s *EntityReportServiceIntegrationTestSuite) TestCreateEntityReport_FestivalSuccess() {
	user := s.createTestUser()
	festival := s.createTestFestival("Test Fest")

	resp, err := s.svc.CreateEntityReport(&contracts.CreateEntityReportRequest{
		EntityType: "festival",
		EntityID:   festival.ID,
		UserID:     user.ID,
		ReportType: "cancelled",
	})

	s.NoError(err)
	s.Require().NotNil(resp)
	s.Equal("festival", resp.EntityType)
	s.Equal("cancelled", resp.ReportType)
}

func (s *EntityReportServiceIntegrationTestSuite) TestCreateEntityReport_ShowSuccess() {
	user := s.createTestUser()
	show := s.createTestShow()

	resp, err := s.svc.CreateEntityReport(&contracts.CreateEntityReportRequest{
		EntityType: "show",
		EntityID:   show.ID,
		UserID:     user.ID,
		ReportType: "wrong_venue",
	})

	s.NoError(err)
	s.Require().NotNil(resp)
	s.Equal("show", resp.EntityType)
	s.Equal("wrong_venue", resp.ReportType)
}

func (s *EntityReportServiceIntegrationTestSuite) TestCreateEntityReport_InvalidEntityType() {
	user := s.createTestUser()

	_, err := s.svc.CreateEntityReport(&contracts.CreateEntityReportRequest{
		EntityType: "release",
		EntityID:   1,
		UserID:     user.ID,
		ReportType: "inaccurate",
	})

	s.Error(err)
	s.Contains(err.Error(), "invalid entity type")
}

func (s *EntityReportServiceIntegrationTestSuite) TestCreateEntityReport_InvalidReportType() {
	user := s.createTestUser()
	artist := s.createTestArtist("Test Artist")

	_, err := s.svc.CreateEntityReport(&contracts.CreateEntityReportRequest{
		EntityType: "artist",
		EntityID:   artist.ID,
		UserID:     user.ID,
		ReportType: "cancelled",
	})

	s.Error(err)
	s.Contains(err.Error(), "invalid report type")
}

func (s *EntityReportServiceIntegrationTestSuite) TestCreateEntityReport_EntityNotFound() {
	user := s.createTestUser()

	_, err := s.svc.CreateEntityReport(&contracts.CreateEntityReportRequest{
		EntityType: "artist",
		EntityID:   99999,
		UserID:     user.ID,
		ReportType: "inaccurate",
	})

	s.Error(err)
	s.Contains(err.Error(), "entity not found")
}

func (s *EntityReportServiceIntegrationTestSuite) TestCreateEntityReport_DuplicatePendingReport() {
	user := s.createTestUser()
	artist := s.createTestArtist("Test Artist")

	// First report succeeds
	_, err := s.svc.CreateEntityReport(&contracts.CreateEntityReportRequest{
		EntityType: "artist",
		EntityID:   artist.ID,
		UserID:     user.ID,
		ReportType: "inaccurate",
	})
	s.NoError(err)

	// Second report from same user fails
	_, err = s.svc.CreateEntityReport(&contracts.CreateEntityReportRequest{
		EntityType: "artist",
		EntityID:   artist.ID,
		UserID:     user.ID,
		ReportType: "duplicate",
	})
	s.Error(err)
	s.Contains(err.Error(), "already have a pending report")
}

func (s *EntityReportServiceIntegrationTestSuite) TestCreateEntityReport_AllowAfterResolved() {
	user := s.createTestUser()
	admin := s.createTestUser()
	artist := s.createTestArtist("Test Artist")

	// First report
	report := s.createReport("artist", artist.ID, user.ID, "inaccurate")

	// Resolve it
	_, err := s.svc.ResolveEntityReport(report.ID, admin.ID, "fixed")
	s.NoError(err)

	// New report from same user should succeed
	resp, err := s.svc.CreateEntityReport(&contracts.CreateEntityReportRequest{
		EntityType: "artist",
		EntityID:   artist.ID,
		UserID:     user.ID,
		ReportType: "missing_info",
	})
	s.NoError(err)
	s.NotNil(resp)
}

// =============================================================================
// GetEntityReport tests
// =============================================================================

func (s *EntityReportServiceIntegrationTestSuite) TestGetEntityReport_Success() {
	user := s.createTestUser()
	artist := s.createTestArtist("Test Artist")
	report := s.createReport("artist", artist.ID, user.ID, "inaccurate")

	resp, err := s.svc.GetEntityReport(report.ID)
	s.NoError(err)
	s.Require().NotNil(resp)
	s.Equal(report.ID, resp.ID)
	s.Equal("artist", resp.EntityType)
}

func (s *EntityReportServiceIntegrationTestSuite) TestGetEntityReport_NotFound() {
	resp, err := s.svc.GetEntityReport(99999)
	s.NoError(err)
	s.Nil(resp)
}

// =============================================================================
// GetEntityReports tests
// =============================================================================

func (s *EntityReportServiceIntegrationTestSuite) TestGetEntityReports_Success() {
	user1 := s.createTestUser()
	user2 := s.createTestUser()
	artist := s.createTestArtist("Test Artist")

	s.createReport("artist", artist.ID, user1.ID, "inaccurate")
	s.createReport("artist", artist.ID, user2.ID, "duplicate")

	reports, err := s.svc.GetEntityReports("artist", artist.ID)
	s.NoError(err)
	s.Len(reports, 2)
}

func (s *EntityReportServiceIntegrationTestSuite) TestGetEntityReports_Empty() {
	reports, err := s.svc.GetEntityReports("artist", 99999)
	s.NoError(err)
	s.Empty(reports)
}

// =============================================================================
// ListEntityReports tests
// =============================================================================

func (s *EntityReportServiceIntegrationTestSuite) TestListEntityReports_DefaultFilters() {
	user := s.createTestUser()
	artist := s.createTestArtist("Test Artist")
	venue := s.createTestVenue("Test Venue")

	s.createReport("artist", artist.ID, user.ID, "inaccurate")
	s.createReport("venue", venue.ID, user.ID, "wrong_address")

	reports, total, err := s.svc.ListEntityReports(nil)
	s.NoError(err)
	s.Equal(int64(2), total)
	s.Len(reports, 2)
}

func (s *EntityReportServiceIntegrationTestSuite) TestListEntityReports_FilterByStatus() {
	user := s.createTestUser()
	admin := s.createTestUser()
	artist := s.createTestArtist("Test Artist")
	venue := s.createTestVenue("Test Venue")

	s.createReport("artist", artist.ID, user.ID, "inaccurate")
	venueReport := s.createReport("venue", venue.ID, user.ID, "wrong_address")

	// Resolve one report
	_, err := s.svc.ResolveEntityReport(venueReport.ID, admin.ID, "fixed")
	s.NoError(err)

	// Filter by pending
	reports, total, err := s.svc.ListEntityReports(&contracts.EntityReportFilters{
		Status: "pending",
	})
	s.NoError(err)
	s.Equal(int64(1), total)
	s.Len(reports, 1)
	s.Equal("artist", reports[0].EntityType)
}

func (s *EntityReportServiceIntegrationTestSuite) TestListEntityReports_FilterByEntityType() {
	user := s.createTestUser()
	artist := s.createTestArtist("Test Artist")
	venue := s.createTestVenue("Test Venue")

	s.createReport("artist", artist.ID, user.ID, "inaccurate")
	s.createReport("venue", venue.ID, user.ID, "wrong_address")

	reports, total, err := s.svc.ListEntityReports(&contracts.EntityReportFilters{
		EntityType: "venue",
	})
	s.NoError(err)
	s.Equal(int64(1), total)
	s.Len(reports, 1)
	s.Equal("venue", reports[0].EntityType)
}

func (s *EntityReportServiceIntegrationTestSuite) TestListEntityReports_Pagination() {
	user := s.createTestUser()
	artist := s.createTestArtist("Test Artist")

	// Create 3 reports from different users
	user2 := s.createTestUser()
	user3 := s.createTestUser()
	s.createReport("artist", artist.ID, user.ID, "inaccurate")
	s.createReport("artist", artist.ID, user2.ID, "duplicate")
	s.createReport("artist", artist.ID, user3.ID, "missing_info")

	// Page 1: limit 2
	reports, total, err := s.svc.ListEntityReports(&contracts.EntityReportFilters{
		Limit:  2,
		Offset: 0,
	})
	s.NoError(err)
	s.Equal(int64(3), total)
	s.Len(reports, 2)

	// Page 2: offset 2
	reports, total, err = s.svc.ListEntityReports(&contracts.EntityReportFilters{
		Limit:  2,
		Offset: 2,
	})
	s.NoError(err)
	s.Equal(int64(3), total)
	s.Len(reports, 1)
}

// =============================================================================
// ResolveEntityReport tests
// =============================================================================

func (s *EntityReportServiceIntegrationTestSuite) TestResolveEntityReport_Success() {
	user := s.createTestUser()
	admin := s.createTestUser()
	artist := s.createTestArtist("Test Artist")
	report := s.createReport("artist", artist.ID, user.ID, "inaccurate")

	resolved, err := s.svc.ResolveEntityReport(report.ID, admin.ID, "Fixed the bio")
	s.NoError(err)
	s.Require().NotNil(resolved)
	s.Equal("resolved", resolved.Status)
	s.Require().NotNil(resolved.ReviewedBy)
	s.Equal(admin.ID, *resolved.ReviewedBy)
	s.NotNil(resolved.ReviewedAt)
	s.Require().NotNil(resolved.AdminNotes)
	s.Equal("Fixed the bio", *resolved.AdminNotes)
}

func (s *EntityReportServiceIntegrationTestSuite) TestResolveEntityReport_NoNotes() {
	user := s.createTestUser()
	admin := s.createTestUser()
	artist := s.createTestArtist("Test Artist")
	report := s.createReport("artist", artist.ID, user.ID, "inaccurate")

	resolved, err := s.svc.ResolveEntityReport(report.ID, admin.ID, "")
	s.NoError(err)
	s.Require().NotNil(resolved)
	s.Equal("resolved", resolved.Status)
	s.Nil(resolved.AdminNotes)
}

func (s *EntityReportServiceIntegrationTestSuite) TestResolveEntityReport_NotFound() {
	_, err := s.svc.ResolveEntityReport(99999, 1, "notes")
	s.Error(err)
	s.Contains(err.Error(), "not found")
}

func (s *EntityReportServiceIntegrationTestSuite) TestResolveEntityReport_AlreadyReviewed() {
	user := s.createTestUser()
	admin := s.createTestUser()
	artist := s.createTestArtist("Test Artist")
	report := s.createReport("artist", artist.ID, user.ID, "inaccurate")

	_, err := s.svc.ResolveEntityReport(report.ID, admin.ID, "fixed")
	s.NoError(err)

	_, err = s.svc.ResolveEntityReport(report.ID, admin.ID, "again")
	s.Error(err)
	s.Contains(err.Error(), "already been reviewed")
}

// =============================================================================
// DismissEntityReport tests
// =============================================================================

func (s *EntityReportServiceIntegrationTestSuite) TestDismissEntityReport_Success() {
	user := s.createTestUser()
	admin := s.createTestUser()
	artist := s.createTestArtist("Test Artist")
	report := s.createReport("artist", artist.ID, user.ID, "inaccurate")

	dismissed, err := s.svc.DismissEntityReport(report.ID, admin.ID, "Not a valid report")
	s.NoError(err)
	s.Require().NotNil(dismissed)
	s.Equal("dismissed", dismissed.Status)
	s.Require().NotNil(dismissed.ReviewedBy)
	s.Equal(admin.ID, *dismissed.ReviewedBy)
	s.NotNil(dismissed.ReviewedAt)
	s.Require().NotNil(dismissed.AdminNotes)
	s.Equal("Not a valid report", *dismissed.AdminNotes)
}

func (s *EntityReportServiceIntegrationTestSuite) TestDismissEntityReport_NoNotes() {
	user := s.createTestUser()
	admin := s.createTestUser()
	artist := s.createTestArtist("Test Artist")
	report := s.createReport("artist", artist.ID, user.ID, "inaccurate")

	dismissed, err := s.svc.DismissEntityReport(report.ID, admin.ID, "")
	s.NoError(err)
	s.Require().NotNil(dismissed)
	s.Equal("dismissed", dismissed.Status)
	s.Nil(dismissed.AdminNotes)
}

func (s *EntityReportServiceIntegrationTestSuite) TestDismissEntityReport_NotFound() {
	_, err := s.svc.DismissEntityReport(99999, 1, "notes")
	s.Error(err)
	s.Contains(err.Error(), "not found")
}

func (s *EntityReportServiceIntegrationTestSuite) TestDismissEntityReport_AlreadyReviewed() {
	user := s.createTestUser()
	admin := s.createTestUser()
	artist := s.createTestArtist("Test Artist")
	report := s.createReport("artist", artist.ID, user.ID, "inaccurate")

	_, err := s.svc.DismissEntityReport(report.ID, admin.ID, "spam")
	s.NoError(err)

	_, err = s.svc.DismissEntityReport(report.ID, admin.ID, "spam again")
	s.Error(err)
	s.Contains(err.Error(), "already been reviewed")
}

// =============================================================================
// Relationships and reporter name tests
// =============================================================================

func (s *EntityReportServiceIntegrationTestSuite) TestReporterName_Included() {
	user := s.createTestUser()
	artist := s.createTestArtist("Test Artist")
	report := s.createReport("artist", artist.ID, user.ID, "inaccurate")

	resp, err := s.svc.GetEntityReport(report.ID)
	s.NoError(err)
	s.Require().NotNil(resp)
	s.NotEmpty(resp.ReporterName)
}

func (s *EntityReportServiceIntegrationTestSuite) TestReviewerName_Included() {
	user := s.createTestUser()
	admin := s.createTestUser()
	artist := s.createTestArtist("Test Artist")
	report := s.createReport("artist", artist.ID, user.ID, "inaccurate")

	resolved, err := s.svc.ResolveEntityReport(report.ID, admin.ID, "fixed")
	s.NoError(err)
	s.Require().NotNil(resolved)
	s.NotEmpty(resolved.ReviewerName)
}
