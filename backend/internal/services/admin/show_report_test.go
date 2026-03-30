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
// INTEGRATION TESTS (With Real Database)
// =============================================================================

type ShowReportServiceIntegrationTestSuite struct {
	suite.Suite
	testDB        *testutil.TestDatabase
	db            *gorm.DB
	reportService *ShowReportService
}

func (suite *ShowReportServiceIntegrationTestSuite) SetupSuite() {
	suite.testDB = testutil.SetupTestPostgres(suite.T())
	suite.db = suite.testDB.DB

	suite.reportService = &ShowReportService{db: suite.testDB.DB}
}

func (suite *ShowReportServiceIntegrationTestSuite) TearDownSuite() {
	suite.testDB.Cleanup()
}

func (suite *ShowReportServiceIntegrationTestSuite) TearDownTest() {
	sqlDB, err := suite.db.DB()
	suite.Require().NoError(err)
	_, _ = sqlDB.Exec("DELETE FROM show_reports")
	_, _ = sqlDB.Exec("DELETE FROM show_artists")
	_, _ = sqlDB.Exec("DELETE FROM show_venues")
	_, _ = sqlDB.Exec("DELETE FROM shows")
	_, _ = sqlDB.Exec("DELETE FROM artists")
	_, _ = sqlDB.Exec("DELETE FROM venues")
	_, _ = sqlDB.Exec("DELETE FROM users")
}

func TestShowReportServiceIntegrationTestSuite(t *testing.T) {
	suite.Run(t, new(ShowReportServiceIntegrationTestSuite))
}

// =============================================================================
// HELPERS
// =============================================================================

func (suite *ShowReportServiceIntegrationTestSuite) createTestUser() *models.User {
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

func (suite *ShowReportServiceIntegrationTestSuite) createApprovedShow(title string) *models.Show {
	user := suite.createTestUser()
	show := &models.Show{
		Title:       title,
		EventDate:   time.Now().UTC().AddDate(0, 0, 7),
		City:        stringPtr("Phoenix"),
		State:       stringPtr("AZ"),
		Status:      models.ShowStatusApproved,
		SubmittedBy: &user.ID,
	}
	err := suite.db.Create(show).Error
	suite.Require().NoError(err)
	return show
}

func (suite *ShowReportServiceIntegrationTestSuite) createPendingReport(userID, showID uint, reportType string) *models.ShowReport {
	report := &models.ShowReport{
		ShowID:     showID,
		ReportedBy: userID,
		ReportType: models.ShowReportType(reportType),
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

func (suite *ShowReportServiceIntegrationTestSuite) TestCreateReport_Success() {
	show := suite.createApprovedShow("Reported Show")
	user := suite.createTestUser()

	resp, err := suite.reportService.CreateReport(user.ID, show.ID, "cancelled", stringPtr("Band announced cancellation"))

	suite.Require().NoError(err)
	suite.Require().NotNil(resp)
	suite.NotZero(resp.ID)
	suite.Equal(show.ID, resp.ShowID)
	suite.Equal("cancelled", resp.ReportType)
	suite.Equal("pending", resp.Status)
	suite.Equal("Band announced cancellation", *resp.Details)
	suite.Require().NotNil(resp.Show)
	suite.Equal(show.Title, resp.Show.Title)
}

func (suite *ShowReportServiceIntegrationTestSuite) TestCreateReport_AllReportTypes() {
	for _, reportType := range []string{"cancelled", "sold_out", "inaccurate"} {
		show := suite.createApprovedShow(fmt.Sprintf("Show for %s", reportType))
		user := suite.createTestUser()

		resp, err := suite.reportService.CreateReport(user.ID, show.ID, reportType, nil)

		suite.Require().NoError(err, "report type %s should succeed", reportType)
		suite.Equal(reportType, resp.ReportType)
	}
}

func (suite *ShowReportServiceIntegrationTestSuite) TestCreateReport_InvalidType_Fails() {
	show := suite.createApprovedShow("Invalid Type Show")
	user := suite.createTestUser()

	resp, err := suite.reportService.CreateReport(user.ID, show.ID, "bogus_type", nil)

	suite.Require().Error(err)
	suite.Contains(err.Error(), "invalid report type")
	suite.Nil(resp)
}

func (suite *ShowReportServiceIntegrationTestSuite) TestCreateReport_ShowNotFound() {
	user := suite.createTestUser()

	resp, err := suite.reportService.CreateReport(user.ID, 99999, "cancelled", nil)

	suite.Require().Error(err)
	suite.Contains(err.Error(), "show not found")
	suite.Nil(resp)
}

func (suite *ShowReportServiceIntegrationTestSuite) TestCreateReport_DuplicateReport_Fails() {
	show := suite.createApprovedShow("Dup Report Show")
	user := suite.createTestUser()

	_, err := suite.reportService.CreateReport(user.ID, show.ID, "cancelled", nil)
	suite.Require().NoError(err)

	// Same user, same show — should fail
	resp, err := suite.reportService.CreateReport(user.ID, show.ID, "sold_out", nil)

	suite.Require().Error(err)
	suite.Contains(err.Error(), "already reported")
	suite.Nil(resp)
}

func (suite *ShowReportServiceIntegrationTestSuite) TestCreateReport_DifferentUsers_OK() {
	show := suite.createApprovedShow("Multi Report Show")
	user1 := suite.createTestUser()
	user2 := suite.createTestUser()

	_, err := suite.reportService.CreateReport(user1.ID, show.ID, "cancelled", nil)
	suite.Require().NoError(err)

	resp, err := suite.reportService.CreateReport(user2.ID, show.ID, "cancelled", nil)

	suite.Require().NoError(err)
	suite.NotNil(resp)
}

// =============================================================================
// Group 2: GetUserReportForShow
// =============================================================================

func (suite *ShowReportServiceIntegrationTestSuite) TestGetUserReportForShow_Found() {
	show := suite.createApprovedShow("User Report Show")
	user := suite.createTestUser()

	created, err := suite.reportService.CreateReport(user.ID, show.ID, "sold_out", nil)
	suite.Require().NoError(err)

	resp, err := suite.reportService.GetUserReportForShow(user.ID, show.ID)

	suite.Require().NoError(err)
	suite.Require().NotNil(resp)
	suite.Equal(created.ID, resp.ID)
	suite.Equal("sold_out", resp.ReportType)
}

func (suite *ShowReportServiceIntegrationTestSuite) TestGetUserReportForShow_NotFound() {
	show := suite.createApprovedShow("No Report Show")
	user := suite.createTestUser()

	resp, err := suite.reportService.GetUserReportForShow(user.ID, show.ID)

	suite.Require().NoError(err)
	suite.Nil(resp) // Returns nil, nil — not an error
}

// =============================================================================
// Group 3: GetPendingReports
// =============================================================================

func (suite *ShowReportServiceIntegrationTestSuite) TestGetPendingReports_Success() {
	show := suite.createApprovedShow("Pending Report Show")
	user1 := suite.createTestUser()
	user2 := suite.createTestUser()

	suite.reportService.CreateReport(user1.ID, show.ID, "cancelled", nil)

	show2 := suite.createApprovedShow("Another Show")
	suite.reportService.CreateReport(user2.ID, show2.ID, "sold_out", nil)

	resp, total, err := suite.reportService.GetPendingReports(10, 0)

	suite.Require().NoError(err)
	suite.Equal(int64(2), total)
	suite.Len(resp, 2)
	// Should include show info
	suite.NotNil(resp[0].Show)
	suite.NotNil(resp[1].Show)
}

func (suite *ShowReportServiceIntegrationTestSuite) TestGetPendingReports_ExcludesReviewed() {
	show := suite.createApprovedShow("Reviewed Report Show")
	user := suite.createTestUser()
	admin := suite.createTestUser()

	report := suite.createPendingReport(user.ID, show.ID, "cancelled")

	// Dismiss the report
	suite.reportService.DismissReport(report.ID, admin.ID, nil)

	// Create another pending one
	show2 := suite.createApprovedShow("Still Pending Show")
	user2 := suite.createTestUser()
	suite.reportService.CreateReport(user2.ID, show2.ID, "inaccurate", nil)

	resp, total, err := suite.reportService.GetPendingReports(10, 0)

	suite.Require().NoError(err)
	suite.Equal(int64(1), total)
	suite.Len(resp, 1)
	suite.Equal("inaccurate", resp[0].ReportType)
}

func (suite *ShowReportServiceIntegrationTestSuite) TestGetPendingReports_Pagination() {
	// Create 5 pending reports
	for i := 0; i < 5; i++ {
		show := suite.createApprovedShow(fmt.Sprintf("Paginated Show %d", i))
		user := suite.createTestUser()
		suite.reportService.CreateReport(user.ID, show.ID, "cancelled", nil)
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

// =============================================================================
// Group 4: DismissReport
// =============================================================================

func (suite *ShowReportServiceIntegrationTestSuite) TestDismissReport_Success() {
	show := suite.createApprovedShow("Dismiss Show")
	user := suite.createTestUser()
	admin := suite.createTestUser()

	report := suite.createPendingReport(user.ID, show.ID, "inaccurate")

	resp, err := suite.reportService.DismissReport(report.ID, admin.ID, stringPtr("Not a real issue"))

	suite.Require().NoError(err)
	suite.Equal("dismissed", resp.Status)
	suite.Equal("Not a real issue", *resp.AdminNotes)
	suite.Require().NotNil(resp.ReviewedBy)
	suite.Equal(admin.ID, *resp.ReviewedBy)
	suite.NotNil(resp.ReviewedAt)
}

func (suite *ShowReportServiceIntegrationTestSuite) TestDismissReport_NotFound() {
	resp, err := suite.reportService.DismissReport(99999, 1, nil)

	suite.Require().Error(err)
	suite.Contains(err.Error(), "report not found")
	suite.Nil(resp)
}

func (suite *ShowReportServiceIntegrationTestSuite) TestDismissReport_AlreadyReviewed_Fails() {
	show := suite.createApprovedShow("Already Dismissed Show")
	user := suite.createTestUser()
	admin := suite.createTestUser()

	report := suite.createPendingReport(user.ID, show.ID, "cancelled")
	suite.reportService.DismissReport(report.ID, admin.ID, nil)

	// Try to dismiss again
	resp, err := suite.reportService.DismissReport(report.ID, admin.ID, nil)

	suite.Require().Error(err)
	suite.Contains(err.Error(), "already been reviewed")
	suite.Nil(resp)
}

// =============================================================================
// Group 5: ResolveReport / ResolveReportWithFlag
// =============================================================================

func (suite *ShowReportServiceIntegrationTestSuite) TestResolveReport_Success() {
	show := suite.createApprovedShow("Resolve Show")
	user := suite.createTestUser()
	admin := suite.createTestUser()

	report := suite.createPendingReport(user.ID, show.ID, "inaccurate")

	resp, err := suite.reportService.ResolveReport(report.ID, admin.ID, stringPtr("Fixed the info"))

	suite.Require().NoError(err)
	suite.Equal("resolved", resp.Status)
	suite.Equal("Fixed the info", *resp.AdminNotes)
	suite.Require().NotNil(resp.ReviewedBy)
	suite.Equal(admin.ID, *resp.ReviewedBy)
	suite.NotNil(resp.ReviewedAt)
}

func (suite *ShowReportServiceIntegrationTestSuite) TestResolveReport_NotFound() {
	resp, err := suite.reportService.ResolveReport(99999, 1, nil)

	suite.Require().Error(err)
	suite.Contains(err.Error(), "report not found")
	suite.Nil(resp)
}

func (suite *ShowReportServiceIntegrationTestSuite) TestResolveReport_AlreadyReviewed_Fails() {
	show := suite.createApprovedShow("Already Resolved Show")
	user := suite.createTestUser()
	admin := suite.createTestUser()

	report := suite.createPendingReport(user.ID, show.ID, "cancelled")
	suite.reportService.ResolveReport(report.ID, admin.ID, nil)

	resp, err := suite.reportService.ResolveReport(report.ID, admin.ID, nil)

	suite.Require().Error(err)
	suite.Contains(err.Error(), "already been reviewed")
	suite.Nil(resp)
}

func (suite *ShowReportServiceIntegrationTestSuite) TestResolveReportWithFlag_Cancelled_SetsShowFlag() {
	show := suite.createApprovedShow("Cancelled Show")
	user := suite.createTestUser()
	admin := suite.createTestUser()

	report := suite.createPendingReport(user.ID, show.ID, "cancelled")

	resp, err := suite.reportService.ResolveReportWithFlag(report.ID, admin.ID, stringPtr("Confirmed cancelled"), true)

	suite.Require().NoError(err)
	suite.Equal("resolved", resp.Status)

	// Verify show flag was set
	var updatedShow models.Show
	suite.db.First(&updatedShow, show.ID)
	suite.True(updatedShow.IsCancelled, "show should be marked as cancelled")
	suite.False(updatedShow.IsSoldOut, "sold_out should not be affected")
}

func (suite *ShowReportServiceIntegrationTestSuite) TestResolveReportWithFlag_SoldOut_SetsShowFlag() {
	show := suite.createApprovedShow("Sold Out Show")
	user := suite.createTestUser()
	admin := suite.createTestUser()

	report := suite.createPendingReport(user.ID, show.ID, "sold_out")

	resp, err := suite.reportService.ResolveReportWithFlag(report.ID, admin.ID, nil, true)

	suite.Require().NoError(err)
	suite.Equal("resolved", resp.Status)

	// Verify show flag was set
	var updatedShow models.Show
	suite.db.First(&updatedShow, show.ID)
	suite.True(updatedShow.IsSoldOut, "show should be marked as sold out")
	suite.False(updatedShow.IsCancelled, "cancelled should not be affected")
}

func (suite *ShowReportServiceIntegrationTestSuite) TestResolveReportWithFlag_Inaccurate_NoShowFlag() {
	show := suite.createApprovedShow("Inaccurate Show")
	user := suite.createTestUser()
	admin := suite.createTestUser()

	report := suite.createPendingReport(user.ID, show.ID, "inaccurate")

	resp, err := suite.reportService.ResolveReportWithFlag(report.ID, admin.ID, nil, true)

	suite.Require().NoError(err)
	suite.Equal("resolved", resp.Status)

	// Inaccurate reports don't set any flag
	var updatedShow models.Show
	suite.db.First(&updatedShow, show.ID)
	suite.False(updatedShow.IsCancelled)
	suite.False(updatedShow.IsSoldOut)
}

func (suite *ShowReportServiceIntegrationTestSuite) TestResolveReportWithFlag_FlagFalse_NoShowUpdate() {
	show := suite.createApprovedShow("No Flag Show")
	user := suite.createTestUser()
	admin := suite.createTestUser()

	report := suite.createPendingReport(user.ID, show.ID, "cancelled")

	// setShowFlag = false — resolve without updating show
	resp, err := suite.reportService.ResolveReportWithFlag(report.ID, admin.ID, nil, false)

	suite.Require().NoError(err)
	suite.Equal("resolved", resp.Status)

	var updatedShow models.Show
	suite.db.First(&updatedShow, show.ID)
	suite.False(updatedShow.IsCancelled, "flag should not be set when setShowFlag is false")
}

// =============================================================================
// Group 6: GetReportByID
// =============================================================================

func (suite *ShowReportServiceIntegrationTestSuite) TestGetReportByID_Success() {
	show := suite.createApprovedShow("Get By ID Show")
	user := suite.createTestUser()

	created := suite.createPendingReport(user.ID, show.ID, "sold_out")

	report, err := suite.reportService.GetReportByID(created.ID)

	suite.Require().NoError(err)
	suite.Require().NotNil(report)
	suite.Equal(created.ID, report.ID)
	suite.Equal(show.ID, report.ShowID)
	suite.Equal(models.ShowReportTypeSoldOut, report.ReportType)
	// Show should be preloaded
	suite.Equal(show.Title, report.Show.Title)
}

func (suite *ShowReportServiceIntegrationTestSuite) TestGetReportByID_NotFound() {
	report, err := suite.reportService.GetReportByID(99999)

	suite.Require().Error(err)
	suite.Contains(err.Error(), "report not found")
	suite.Nil(report)
}

// =============================================================================
// Group 7: buildReportResponse behavior
// =============================================================================

func (suite *ShowReportServiceIntegrationTestSuite) TestBuildReportResponse_IncludesShowInfo() {
	show := suite.createApprovedShow("Response Show")
	user := suite.createTestUser()

	resp, err := suite.reportService.CreateReport(user.ID, show.ID, "cancelled", nil)

	suite.Require().NoError(err)
	suite.Require().NotNil(resp.Show)
	suite.Equal(show.ID, resp.Show.ID)
	suite.Equal("Response Show", resp.Show.Title)
	suite.Equal("Phoenix", *resp.Show.City)
	suite.Equal("AZ", *resp.Show.State)
}

func (suite *ShowReportServiceIntegrationTestSuite) TestBuildReportResponse_ReviewedAtFormatted() {
	show := suite.createApprovedShow("Reviewed At Show")
	user := suite.createTestUser()
	admin := suite.createTestUser()

	report := suite.createPendingReport(user.ID, show.ID, "cancelled")
	resp, err := suite.reportService.DismissReport(report.ID, admin.ID, nil)

	suite.Require().NoError(err)
	suite.Require().NotNil(resp.ReviewedAt)
	// Should be RFC3339 formatted
	_, parseErr := time.Parse(time.RFC3339, *resp.ReviewedAt)
	suite.NoError(parseErr, "ReviewedAt should be RFC3339 formatted")
}
