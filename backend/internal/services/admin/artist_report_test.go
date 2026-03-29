package admin

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"

	"psychic-homily-backend/internal/models"
	"psychic-homily-backend/internal/testutil"
)

// =============================================================================
// UNIT TESTS (No Database Required)
// =============================================================================

func TestArtistReportService_NilDatabase(t *testing.T) {
	svc := &ArtistReportService{db: nil}

	t.Run("CreateReport", func(t *testing.T) {
		testutil.AssertNilDBErrorWithResult(t, func() (interface{}, error) {
			return svc.CreateReport(1, 1, "inaccurate", nil)
		})
	})

	t.Run("GetUserReportForArtist", func(t *testing.T) {
		testutil.AssertNilDBErrorWithResult(t, func() (interface{}, error) {
			return svc.GetUserReportForArtist(1, 1)
		})
	})

	t.Run("GetPendingReports", func(t *testing.T) {
		testutil.AssertNilDBError(t, func() error {
			_, _, err := svc.GetPendingReports(10, 0)
			return err
		})
	})

	t.Run("DismissReport", func(t *testing.T) {
		testutil.AssertNilDBErrorWithResult(t, func() (interface{}, error) {
			return svc.DismissReport(1, 1, nil)
		})
	})

	t.Run("ResolveReport", func(t *testing.T) {
		testutil.AssertNilDBErrorWithResult(t, func() (interface{}, error) {
			return svc.ResolveReport(1, 1, nil)
		})
	})

	t.Run("GetReportByID", func(t *testing.T) {
		testutil.AssertNilDBErrorWithResult(t, func() (interface{}, error) {
			return svc.GetReportByID(1)
		})
	})
}

// =============================================================================
// INTEGRATION TESTS (With Real Database)
// =============================================================================

type ArtistReportServiceIntegrationTestSuite struct {
	suite.Suite
	testDB        *testutil.TestDatabase
	db            *gorm.DB
	reportService *ArtistReportService
}

func (suite *ArtistReportServiceIntegrationTestSuite) SetupSuite() {
	suite.testDB = testutil.SetupTestPostgres(suite.T())
	suite.db = suite.testDB.DB

	suite.reportService = &ArtistReportService{db: suite.testDB.DB}
}

func (suite *ArtistReportServiceIntegrationTestSuite) TearDownSuite() {
	suite.testDB.Cleanup()
}

func (suite *ArtistReportServiceIntegrationTestSuite) TearDownTest() {
	sqlDB, err := suite.db.DB()
	suite.Require().NoError(err)
	_, _ = sqlDB.Exec("DELETE FROM artist_reports")
	_, _ = sqlDB.Exec("DELETE FROM artists")
	_, _ = sqlDB.Exec("DELETE FROM users")
}

func TestArtistReportServiceIntegrationTestSuite(t *testing.T) {
	suite.Run(t, new(ArtistReportServiceIntegrationTestSuite))
}

// =============================================================================
// HELPERS
// =============================================================================

func (suite *ArtistReportServiceIntegrationTestSuite) createTestUser() *models.User {
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

func (suite *ArtistReportServiceIntegrationTestSuite) createTestArtist(name string) *models.Artist {
	slug := fmt.Sprintf("%s-%d", name, time.Now().UnixNano())
	artist := &models.Artist{
		Name: name,
		Slug: &slug,
	}
	err := suite.db.Create(artist).Error
	suite.Require().NoError(err)
	return artist
}

func (suite *ArtistReportServiceIntegrationTestSuite) createPendingReport(userID, artistID uint, reportType string) *models.ArtistReport {
	report := &models.ArtistReport{
		ArtistID:   artistID,
		ReportedBy: userID,
		ReportType: models.ArtistReportType(reportType),
		Status:     models.ShowReportStatusPending,
		CreatedAt:  time.Now().UTC(),
		UpdatedAt:  time.Now().UTC(),
	}
	err := suite.db.Create(report).Error
	suite.Require().NoError(err)
	return report
}

// =============================================================================
// Group 1: CreateReport
// =============================================================================

func (suite *ArtistReportServiceIntegrationTestSuite) TestCreateReport_Success() {
	artist := suite.createTestArtist("Reported Artist")
	user := suite.createTestUser()

	resp, err := suite.reportService.CreateReport(user.ID, artist.ID, "inaccurate", stringPtr("Wrong genre listed"))

	suite.Require().NoError(err)
	suite.Require().NotNil(resp)
	suite.NotZero(resp.ID)
	suite.Equal(artist.ID, resp.ArtistID)
	suite.Equal("inaccurate", resp.ReportType)
	suite.Equal("pending", resp.Status)
	suite.Equal("Wrong genre listed", *resp.Details)
	suite.Require().NotNil(resp.Artist)
	suite.Equal(artist.Name, resp.Artist.Name)
}

func (suite *ArtistReportServiceIntegrationTestSuite) TestCreateReport_AllReportTypes() {
	for _, reportType := range []string{"inaccurate", "removal_request"} {
		artist := suite.createTestArtist(fmt.Sprintf("Artist for %s", reportType))
		user := suite.createTestUser()

		resp, err := suite.reportService.CreateReport(user.ID, artist.ID, reportType, nil)

		suite.Require().NoError(err, "report type %s should succeed", reportType)
		suite.Equal(reportType, resp.ReportType)
	}
}

func (suite *ArtistReportServiceIntegrationTestSuite) TestCreateReport_InvalidType_Fails() {
	artist := suite.createTestArtist("Invalid Type Artist")
	user := suite.createTestUser()

	resp, err := suite.reportService.CreateReport(user.ID, artist.ID, "bogus_type", nil)

	suite.Require().Error(err)
	suite.Contains(err.Error(), "invalid report type")
	suite.Nil(resp)
}

func (suite *ArtistReportServiceIntegrationTestSuite) TestCreateReport_ArtistNotFound() {
	user := suite.createTestUser()

	resp, err := suite.reportService.CreateReport(user.ID, 99999, "inaccurate", nil)

	suite.Require().Error(err)
	suite.Contains(err.Error(), "artist not found")
	suite.Nil(resp)
}

func (suite *ArtistReportServiceIntegrationTestSuite) TestCreateReport_DuplicateReport_Fails() {
	artist := suite.createTestArtist("Dup Report Artist")
	user := suite.createTestUser()

	_, err := suite.reportService.CreateReport(user.ID, artist.ID, "inaccurate", nil)
	suite.Require().NoError(err)

	// Same user, same artist — should fail
	resp, err := suite.reportService.CreateReport(user.ID, artist.ID, "removal_request", nil)

	suite.Require().Error(err)
	suite.Contains(err.Error(), "already reported")
	suite.Nil(resp)
}

func (suite *ArtistReportServiceIntegrationTestSuite) TestCreateReport_DifferentUsers_OK() {
	artist := suite.createTestArtist("Multi Report Artist")
	user1 := suite.createTestUser()
	user2 := suite.createTestUser()

	_, err := suite.reportService.CreateReport(user1.ID, artist.ID, "inaccurate", nil)
	suite.Require().NoError(err)

	resp, err := suite.reportService.CreateReport(user2.ID, artist.ID, "inaccurate", nil)

	suite.Require().NoError(err)
	suite.NotNil(resp)
}

func (suite *ArtistReportServiceIntegrationTestSuite) TestCreateReport_WithoutDetails() {
	artist := suite.createTestArtist("No Details Artist")
	user := suite.createTestUser()

	resp, err := suite.reportService.CreateReport(user.ID, artist.ID, "inaccurate", nil)

	suite.Require().NoError(err)
	suite.Nil(resp.Details)
}

// =============================================================================
// Group 2: GetUserReportForArtist
// =============================================================================

func (suite *ArtistReportServiceIntegrationTestSuite) TestGetUserReportForArtist_Found() {
	artist := suite.createTestArtist("User Report Artist")
	user := suite.createTestUser()

	created, err := suite.reportService.CreateReport(user.ID, artist.ID, "inaccurate", nil)
	suite.Require().NoError(err)

	resp, err := suite.reportService.GetUserReportForArtist(user.ID, artist.ID)

	suite.Require().NoError(err)
	suite.Require().NotNil(resp)
	suite.Equal(created.ID, resp.ID)
	suite.Equal("inaccurate", resp.ReportType)
}

func (suite *ArtistReportServiceIntegrationTestSuite) TestGetUserReportForArtist_NotFound() {
	artist := suite.createTestArtist("No Report Artist")
	user := suite.createTestUser()

	resp, err := suite.reportService.GetUserReportForArtist(user.ID, artist.ID)

	suite.Require().NoError(err)
	suite.Nil(resp) // Returns nil, nil — not an error
}

func (suite *ArtistReportServiceIntegrationTestSuite) TestGetUserReportForArtist_DifferentUser_ReturnsNil() {
	artist := suite.createTestArtist("Other User Artist")
	user1 := suite.createTestUser()
	user2 := suite.createTestUser()

	_, err := suite.reportService.CreateReport(user1.ID, artist.ID, "inaccurate", nil)
	suite.Require().NoError(err)

	// user2 has no report for this artist
	resp, err := suite.reportService.GetUserReportForArtist(user2.ID, artist.ID)

	suite.Require().NoError(err)
	suite.Nil(resp)
}

// =============================================================================
// Group 3: GetPendingReports
// =============================================================================

func (suite *ArtistReportServiceIntegrationTestSuite) TestGetPendingReports_Success() {
	artist1 := suite.createTestArtist("Pending Artist 1")
	artist2 := suite.createTestArtist("Pending Artist 2")
	user1 := suite.createTestUser()
	user2 := suite.createTestUser()

	suite.reportService.CreateReport(user1.ID, artist1.ID, "inaccurate", nil)
	suite.reportService.CreateReport(user2.ID, artist2.ID, "removal_request", nil)

	resp, total, err := suite.reportService.GetPendingReports(10, 0)

	suite.Require().NoError(err)
	suite.Equal(int64(2), total)
	suite.Len(resp, 2)
	// Should include artist info
	suite.NotNil(resp[0].Artist)
	suite.NotNil(resp[1].Artist)
}

func (suite *ArtistReportServiceIntegrationTestSuite) TestGetPendingReports_ExcludesReviewed() {
	artist := suite.createTestArtist("Reviewed Report Artist")
	user := suite.createTestUser()
	admin := suite.createTestUser()

	report := suite.createPendingReport(user.ID, artist.ID, "inaccurate")

	// Dismiss the report
	suite.reportService.DismissReport(report.ID, admin.ID, nil)

	// Create another pending one
	artist2 := suite.createTestArtist("Still Pending Artist")
	user2 := suite.createTestUser()
	suite.reportService.CreateReport(user2.ID, artist2.ID, "removal_request", nil)

	resp, total, err := suite.reportService.GetPendingReports(10, 0)

	suite.Require().NoError(err)
	suite.Equal(int64(1), total)
	suite.Len(resp, 1)
	suite.Equal("removal_request", resp[0].ReportType)
}

func (suite *ArtistReportServiceIntegrationTestSuite) TestGetPendingReports_Pagination() {
	// Create 5 pending reports
	for i := 0; i < 5; i++ {
		artist := suite.createTestArtist(fmt.Sprintf("Paginated Artist %d", i))
		user := suite.createTestUser()
		suite.reportService.CreateReport(user.ID, artist.ID, "inaccurate", nil)
	}

	// Page 1
	resp1, total, err := suite.reportService.GetPendingReports(2, 0)
	suite.Require().NoError(err)
	suite.Equal(int64(5), total)
	suite.Len(resp1, 2)

	// Page 2
	resp2, _, err := suite.reportService.GetPendingReports(2, 2)
	suite.Require().NoError(err)
	suite.Len(resp2, 2)

	// Page 3
	resp3, _, err := suite.reportService.GetPendingReports(2, 4)
	suite.Require().NoError(err)
	suite.Len(resp3, 1)

	// No overlap
	suite.NotEqual(resp1[0].ID, resp2[0].ID)
}

func (suite *ArtistReportServiceIntegrationTestSuite) TestGetPendingReports_Empty() {
	resp, total, err := suite.reportService.GetPendingReports(10, 0)

	suite.Require().NoError(err)
	suite.Equal(int64(0), total)
	suite.Empty(resp)
}

// =============================================================================
// Group 4: DismissReport
// =============================================================================

func (suite *ArtistReportServiceIntegrationTestSuite) TestDismissReport_Success() {
	artist := suite.createTestArtist("Dismiss Artist")
	user := suite.createTestUser()
	admin := suite.createTestUser()

	report := suite.createPendingReport(user.ID, artist.ID, "inaccurate")

	resp, err := suite.reportService.DismissReport(report.ID, admin.ID, stringPtr("Not a real issue"))

	suite.Require().NoError(err)
	suite.Equal("dismissed", resp.Status)
	suite.Equal("Not a real issue", *resp.AdminNotes)
	suite.Require().NotNil(resp.ReviewedBy)
	suite.Equal(admin.ID, *resp.ReviewedBy)
	suite.NotNil(resp.ReviewedAt)
}

func (suite *ArtistReportServiceIntegrationTestSuite) TestDismissReport_NotFound() {
	resp, err := suite.reportService.DismissReport(99999, 1, nil)

	suite.Require().Error(err)
	suite.Contains(err.Error(), "report not found")
	suite.Nil(resp)
}

func (suite *ArtistReportServiceIntegrationTestSuite) TestDismissReport_AlreadyReviewed_Fails() {
	artist := suite.createTestArtist("Already Dismissed Artist")
	user := suite.createTestUser()
	admin := suite.createTestUser()

	report := suite.createPendingReport(user.ID, artist.ID, "inaccurate")
	suite.reportService.DismissReport(report.ID, admin.ID, nil)

	// Try to dismiss again
	resp, err := suite.reportService.DismissReport(report.ID, admin.ID, nil)

	suite.Require().Error(err)
	suite.Contains(err.Error(), "already been reviewed")
	suite.Nil(resp)
}

func (suite *ArtistReportServiceIntegrationTestSuite) TestDismissReport_WithoutNotes() {
	artist := suite.createTestArtist("No Notes Dismiss Artist")
	user := suite.createTestUser()
	admin := suite.createTestUser()

	report := suite.createPendingReport(user.ID, artist.ID, "inaccurate")

	resp, err := suite.reportService.DismissReport(report.ID, admin.ID, nil)

	suite.Require().NoError(err)
	suite.Equal("dismissed", resp.Status)
	suite.Nil(resp.AdminNotes)
}

// =============================================================================
// Group 5: ResolveReport
// =============================================================================

func (suite *ArtistReportServiceIntegrationTestSuite) TestResolveReport_Success() {
	artist := suite.createTestArtist("Resolve Artist")
	user := suite.createTestUser()
	admin := suite.createTestUser()

	report := suite.createPendingReport(user.ID, artist.ID, "inaccurate")

	resp, err := suite.reportService.ResolveReport(report.ID, admin.ID, stringPtr("Fixed the info"))

	suite.Require().NoError(err)
	suite.Equal("resolved", resp.Status)
	suite.Equal("Fixed the info", *resp.AdminNotes)
	suite.Require().NotNil(resp.ReviewedBy)
	suite.Equal(admin.ID, *resp.ReviewedBy)
	suite.NotNil(resp.ReviewedAt)
}

func (suite *ArtistReportServiceIntegrationTestSuite) TestResolveReport_NotFound() {
	resp, err := suite.reportService.ResolveReport(99999, 1, nil)

	suite.Require().Error(err)
	suite.Contains(err.Error(), "report not found")
	suite.Nil(resp)
}

func (suite *ArtistReportServiceIntegrationTestSuite) TestResolveReport_AlreadyReviewed_Fails() {
	artist := suite.createTestArtist("Already Resolved Artist")
	user := suite.createTestUser()
	admin := suite.createTestUser()

	report := suite.createPendingReport(user.ID, artist.ID, "inaccurate")
	suite.reportService.ResolveReport(report.ID, admin.ID, nil)

	resp, err := suite.reportService.ResolveReport(report.ID, admin.ID, nil)

	suite.Require().Error(err)
	suite.Contains(err.Error(), "already been reviewed")
	suite.Nil(resp)
}

func (suite *ArtistReportServiceIntegrationTestSuite) TestResolveReport_CannotResolveAfterDismiss() {
	artist := suite.createTestArtist("Dismiss Then Resolve Artist")
	user := suite.createTestUser()
	admin := suite.createTestUser()

	report := suite.createPendingReport(user.ID, artist.ID, "inaccurate")
	suite.reportService.DismissReport(report.ID, admin.ID, nil)

	resp, err := suite.reportService.ResolveReport(report.ID, admin.ID, nil)

	suite.Require().Error(err)
	suite.Contains(err.Error(), "already been reviewed")
	suite.Nil(resp)
}

func (suite *ArtistReportServiceIntegrationTestSuite) TestResolveReport_WithoutNotes() {
	artist := suite.createTestArtist("No Notes Resolve Artist")
	user := suite.createTestUser()
	admin := suite.createTestUser()

	report := suite.createPendingReport(user.ID, artist.ID, "removal_request")

	resp, err := suite.reportService.ResolveReport(report.ID, admin.ID, nil)

	suite.Require().NoError(err)
	suite.Equal("resolved", resp.Status)
	suite.Nil(resp.AdminNotes)
}

// =============================================================================
// Group 6: GetReportByID
// =============================================================================

func (suite *ArtistReportServiceIntegrationTestSuite) TestGetReportByID_Success() {
	artist := suite.createTestArtist("Get By ID Artist")
	user := suite.createTestUser()

	created := suite.createPendingReport(user.ID, artist.ID, "inaccurate")

	report, err := suite.reportService.GetReportByID(created.ID)

	suite.Require().NoError(err)
	suite.Require().NotNil(report)
	suite.Equal(created.ID, report.ID)
	suite.Equal(artist.ID, report.ArtistID)
	suite.Equal(models.ArtistReportTypeInaccurate, report.ReportType)
	// Artist should be preloaded
	suite.Equal(artist.Name, report.Artist.Name)
}

func (suite *ArtistReportServiceIntegrationTestSuite) TestGetReportByID_NotFound() {
	report, err := suite.reportService.GetReportByID(99999)

	suite.Require().Error(err)
	suite.Contains(err.Error(), "report not found")
	suite.Nil(report)
}

// =============================================================================
// Group 7: buildReportResponse behavior
// =============================================================================

func (suite *ArtistReportServiceIntegrationTestSuite) TestBuildReportResponse_IncludesArtistInfo() {
	artist := suite.createTestArtist("Response Artist")
	user := suite.createTestUser()

	resp, err := suite.reportService.CreateReport(user.ID, artist.ID, "inaccurate", nil)

	suite.Require().NoError(err)
	suite.Require().NotNil(resp.Artist)
	suite.Equal(artist.ID, resp.Artist.ID)
	suite.Equal("Response Artist", resp.Artist.Name)
	suite.NotEmpty(resp.Artist.Slug)
}

func (suite *ArtistReportServiceIntegrationTestSuite) TestBuildReportResponse_ReviewedAtFormatted() {
	artist := suite.createTestArtist("Reviewed At Artist")
	user := suite.createTestUser()
	admin := suite.createTestUser()

	report := suite.createPendingReport(user.ID, artist.ID, "inaccurate")
	resp, err := suite.reportService.DismissReport(report.ID, admin.ID, nil)

	suite.Require().NoError(err)
	suite.Require().NotNil(resp.ReviewedAt)
	// Should be RFC3339 formatted
	_, parseErr := time.Parse(time.RFC3339, *resp.ReviewedAt)
	suite.NoError(parseErr, "ReviewedAt should be RFC3339 formatted")
}

func (suite *ArtistReportServiceIntegrationTestSuite) TestBuildReportResponse_PendingHasNoReviewedAt() {
	artist := suite.createTestArtist("Pending At Artist")
	user := suite.createTestUser()

	resp, err := suite.reportService.CreateReport(user.ID, artist.ID, "inaccurate", nil)

	suite.Require().NoError(err)
	suite.Nil(resp.ReviewedAt)
	suite.Nil(resp.ReviewedBy)
}
