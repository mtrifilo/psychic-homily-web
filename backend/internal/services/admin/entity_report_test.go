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
	assert.True(t, communitym.IsValidEntityReportEntityType("comment"))
	assert.True(t, communitym.IsValidEntityReportEntityType("collection"))
	// PSY-661: releases are now reportable.
	assert.True(t, communitym.IsValidEntityReportEntityType("release"))
	// PSY-666: labels are now reportable.
	assert.True(t, communitym.IsValidEntityReportEntityType("label"))
	assert.False(t, communitym.IsValidEntityReportEntityType(""))

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

	// PSY-578: collection-specific taxonomy.
	assert.True(t, communitym.IsValidReportType("collection", "spam"))
	assert.True(t, communitym.IsValidReportType("collection", "inappropriate"))
	assert.True(t, communitym.IsValidReportType("collection", "misleading"))
	assert.True(t, communitym.IsValidReportType("collection", "other"))
	// Legacy comment-vocabulary types no longer accepted for collections.
	assert.False(t, communitym.IsValidReportType("collection", "harassment"))
	assert.False(t, communitym.IsValidReportType("collection", "off_topic"))
	assert.False(t, communitym.IsValidReportType("collection", "inaccurate"))
	assert.False(t, communitym.IsValidReportType("collection", "cancelled"))
	assert.False(t, communitym.IsValidReportType("collection", "wrong_image"))

	// PSY-661: release-tailored taxonomy.
	assert.True(t, communitym.IsValidReportType("release", "inaccurate"))
	assert.True(t, communitym.IsValidReportType("release", "duplicate"))
	assert.True(t, communitym.IsValidReportType("release", "wrong_cover_art"))
	assert.True(t, communitym.IsValidReportType("release", "wrong_release_date"))
	assert.True(t, communitym.IsValidReportType("release", "wrong_artist_attribution"))
	assert.True(t, communitym.IsValidReportType("release", "missing_info"))
	// Types from other taxonomies are not valid for releases.
	assert.False(t, communitym.IsValidReportType("release", "wrong_image"))
	assert.False(t, communitym.IsValidReportType("release", "cancelled"))

	// PSY-666: label-tailored taxonomy (inaccurate/duplicate/wrong_image/
	// missing_info). "Defunct" is intentionally NOT a report type — it's a
	// status-field edit.
	assert.True(t, communitym.IsValidReportType("label", "inaccurate"))
	assert.True(t, communitym.IsValidReportType("label", "duplicate"))
	assert.True(t, communitym.IsValidReportType("label", "wrong_image"))
	assert.True(t, communitym.IsValidReportType("label", "missing_info"))
	// Types from other taxonomies are not valid for labels.
	assert.False(t, communitym.IsValidReportType("label", "removal_request"))
	assert.False(t, communitym.IsValidReportType("label", "defunct"))
	assert.False(t, communitym.IsValidReportType("label", "cancelled"))
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

	// PSY-578: collection has 4 types (spam/inappropriate/misleading/other).
	collectionTypes := communitym.ValidReportTypesForEntity("collection")
	assert.Len(t, collectionTypes, 4)

	// PSY-661: release-tailored taxonomy has 6 types.
	releaseTypes := communitym.ValidReportTypesForEntity("release")
	assert.Len(t, releaseTypes, 6)

	// PSY-666: label-tailored taxonomy has 4 types.
	labelTypes := communitym.ValidReportTypesForEntity("label")
	assert.Len(t, labelTypes, 4)

	// An entity type with no taxonomy still returns nil.
	unknownTypes := communitym.ValidReportTypesForEntity("nonsense")
	assert.Nil(t, unknownTypes)
}

func TestValidEntityReportEntityTypes(t *testing.T) {
	types := communitym.ValidEntityReportEntityTypes()
	assert.Len(t, types, 8)
	assert.Contains(t, types, "artist")
	assert.Contains(t, types, "venue")
	assert.Contains(t, types, "festival")
	assert.Contains(t, types, "show")
	assert.Contains(t, types, "comment")
	assert.Contains(t, types, "collection")
	// PSY-661
	assert.Contains(t, types, "release")
	// PSY-666
	assert.Contains(t, types, "label")
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
	_, _ = sqlDB.Exec("DELETE FROM labels")
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

func (s *EntityReportServiceIntegrationTestSuite) createTestRelease(title string) *catalogm.Release {
	slug := fmt.Sprintf("test-release-%d", time.Now().UnixNano())
	release := &catalogm.Release{
		Title:       title,
		Slug:        &slug,
		ReleaseType: catalogm.ReleaseTypeLP,
	}
	err := s.db.Create(release).Error
	s.Require().NoError(err)
	return release
}

func (s *EntityReportServiceIntegrationTestSuite) createTestLabel(name string) *catalogm.Label {
	slug := fmt.Sprintf("test-label-%d", time.Now().UnixNano())
	label := &catalogm.Label{
		Name:   name,
		Slug:   &slug,
		Status: catalogm.LabelStatusActive,
	}
	err := s.db.Create(label).Error
	s.Require().NoError(err)
	return label
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
	// PSY-619: ReporterUsername populated for users who set a username so the
	// frontend renders the moderation-queue byline as a /users/:username link.
	s.Require().NotNil(resp.ReporterUsername)
	s.Equal(*user.Username, *resp.ReporterUsername)
}

// TestCreateEntityReport_NoUsername covers the unlinked-byline path (PSY-619):
// a reporter without a username on file should ship `reporter_username: null`
// so the frontend renders plain text rather than a broken /users/null link.
func (s *EntityReportServiceIntegrationTestSuite) TestCreateEntityReport_NoUsername() {
	user := &authm.User{
		Email:         stringPtr(fmt.Sprintf("er-user-no-uname-%d@test.com", time.Now().UnixNano())),
		FirstName:     stringPtr("Test"),
		LastName:      stringPtr("User"),
		IsActive:      true,
		EmailVerified: true,
	}
	s.Require().NoError(s.db.Create(user).Error)
	artist := s.createTestArtist("Test Artist")

	resp, err := s.svc.CreateEntityReport(&contracts.CreateEntityReportRequest{
		EntityType: "artist",
		EntityID:   artist.ID,
		UserID:     user.ID,
		ReportType: "inaccurate",
	})

	s.NoError(err)
	s.Require().NotNil(resp)
	s.NotEmpty(resp.ReporterName, "name should fall back through the resolution chain")
	s.Nil(resp.ReporterUsername, "username should be nil when not set on the user")
}

// TestCreateEntityReport_EmailOnlyReporter_NoEmailLeak locks in the PSY-607
// invariant: when a reporter has only an email (no username, no first/last
// name), the byline must render the email PREFIX (local-part before "@")
// — never the full email address.
//
// Pre-PSY-612 regression: the entity_report service had its own `displayName`
// helper whose terminal branch returned `*u.Email` verbatim, which leaked
// `asdf@admin.com` into the admin moderation queue (dogfood ISSUE-009 —
// `dogfood-output/pending-edits/screenshots/reject-step-4-result.png`).
// PSY-612 swapped to `shared.ResolveUserName`, which falls through to the
// email-prefix step instead. This test asserts that contract at the service
// boundary so a future regression can't reintroduce the full-email leak.
func (s *EntityReportServiceIntegrationTestSuite) TestCreateEntityReport_EmailOnlyReporter_NoEmailLeak() {
	emailLocalPart := fmt.Sprintf("er-emailonly-%d", time.Now().UnixNano())
	emailFull := emailLocalPart + "@admin.com"
	user := &authm.User{
		Email:         &emailFull,
		IsActive:      true,
		EmailVerified: true,
	}
	s.Require().NoError(s.db.Create(user).Error)
	artist := s.createTestArtist("Test Artist")

	resp, err := s.svc.CreateEntityReport(&contracts.CreateEntityReportRequest{
		EntityType: "artist",
		EntityID:   artist.ID,
		UserID:     user.ID,
		ReportType: "inaccurate",
	})

	s.NoError(err)
	s.Require().NotNil(resp)
	s.Equal(emailLocalPart, resp.ReporterName, "must render email prefix, not full email")
	s.NotContains(resp.ReporterName, "@", "byline must never contain '@' for email-only users")
	s.Nil(resp.ReporterUsername, "username should be nil when not set on the user")
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

// PSY-661: end-to-end create + entity-name/slug resolution for releases.
func (s *EntityReportServiceIntegrationTestSuite) TestCreateEntityReport_ReleaseSuccess() {
	user := s.createTestUser()
	release := s.createTestRelease("Test Release")

	resp, err := s.svc.CreateEntityReport(&contracts.CreateEntityReportRequest{
		EntityType: "release",
		EntityID:   release.ID,
		UserID:     user.ID,
		ReportType: "wrong_cover_art",
	})

	s.NoError(err)
	s.Require().NotNil(resp)
	s.Equal("release", resp.EntityType)
	s.Equal("wrong_cover_art", resp.ReportType)
	// The moderation queue deep-links via the resolved name + slug.
	s.Equal("Test Release", resp.EntityName)
	s.Require().NotNil(resp.EntitySlug)
	s.NotEmpty(*resp.EntitySlug)
}

// PSY-666: end-to-end create + entity-name/slug resolution for labels.
func (s *EntityReportServiceIntegrationTestSuite) TestCreateEntityReport_LabelSuccess() {
	user := s.createTestUser()
	label := s.createTestLabel("Test Label")

	resp, err := s.svc.CreateEntityReport(&contracts.CreateEntityReportRequest{
		EntityType: "label",
		EntityID:   label.ID,
		UserID:     user.ID,
		ReportType: "wrong_image",
	})

	s.NoError(err)
	s.Require().NotNil(resp)
	s.Equal("label", resp.EntityType)
	s.Equal("wrong_image", resp.ReportType)
	// The moderation queue deep-links via the resolved name + slug.
	s.Equal("Test Label", resp.EntityName)
	s.Require().NotNil(resp.EntitySlug)
	s.NotEmpty(*resp.EntitySlug)
}

func (s *EntityReportServiceIntegrationTestSuite) TestCreateEntityReport_InvalidEntityType() {
	user := s.createTestUser()

	// `widget` is not a reportable entity type (PSY-661 added `release`,
	// PSY-666 added `label`; this stays the canonical never-valid example).
	_, err := s.svc.CreateEntityReport(&contracts.CreateEntityReportRequest{
		EntityType: "widget",
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

// TestListEntityReports_MixedReporterAccounts_NoEmailLeak locks in the
// PSY-607 invariant for the LIST path — the admin moderation queue's hot
// path. Reports from a username-set reporter and an email-only reporter
// must render with consistent attribution formatting: username for the
// first, email-PREFIX (never the full email) for the second.
//
// Repro evidence: dogfood ISSUE-009 showed two adjacent Van Buren report
// rows with mismatched byline format (one `by testuser2`, one
// `by asdf@admin.com`) — the email-leak side of that mismatch is what this
// test pins.
func (s *EntityReportServiceIntegrationTestSuite) TestListEntityReports_MixedReporterAccounts_NoEmailLeak() {
	withUsername := s.createTestUser()

	emailLocalPart := fmt.Sprintf("er-mixed-emailonly-%d", time.Now().UnixNano())
	emailFull := emailLocalPart + "@admin.com"
	emailOnly := &authm.User{
		Email:         &emailFull,
		IsActive:      true,
		EmailVerified: true,
	}
	s.Require().NoError(s.db.Create(emailOnly).Error)

	venue := s.createTestVenue("Mixed Reporters Venue")
	s.createReport("venue", venue.ID, withUsername.ID, "closed_permanently")
	s.createReport("venue", venue.ID, emailOnly.ID, "inaccurate")

	reports, _, err := s.svc.ListEntityReports(&contracts.EntityReportFilters{
		Status:     "pending",
		EntityType: "venue",
	})
	s.NoError(err)
	s.Require().Len(reports, 2)

	// The username-set reporter renders their username; the email-only
	// reporter renders their email PREFIX. Neither byline contains "@" —
	// that's the no-email-leak invariant. Keyed by report_type so the
	// assertion is robust to listing order changes.
	resolvedNames := make(map[string]string, len(reports))
	for _, r := range reports {
		s.NotContains(
			r.ReporterName, "@",
			"reporter byline must never contain '@' — that's an email leak (PSY-607)",
		)
		resolvedNames[r.ReportType] = r.ReporterName
	}
	s.Equal(*withUsername.Username, resolvedNames["closed_permanently"])
	s.Equal(emailLocalPart, resolvedNames["inaccurate"])
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
	// PSY-619: ReporterUsername populated for users who set a username.
	s.Require().NotNil(resp.ReporterUsername)
	s.Equal(*user.Username, *resp.ReporterUsername)
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
	// PSY-619: ReviewerUsername populated for admins who set a username.
	s.Require().NotNil(resolved.ReviewerUsername)
	s.Equal(*admin.Username, *resolved.ReviewerUsername)
}
