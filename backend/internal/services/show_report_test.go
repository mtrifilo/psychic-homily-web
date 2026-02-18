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

func TestNewShowReportService(t *testing.T) {
	svc := NewShowReportService(nil)
	assert.NotNil(t, svc)
}

func TestShowReportService_NilDatabase(t *testing.T) {
	svc := &ShowReportService{db: nil}

	t.Run("CreateReport", func(t *testing.T) {
		resp, err := svc.CreateReport(1, 1, "cancelled", nil)
		assert.Error(t, err)
		assert.Equal(t, "database not initialized", err.Error())
		assert.Nil(t, resp)
	})

	t.Run("GetUserReportForShow", func(t *testing.T) {
		resp, err := svc.GetUserReportForShow(1, 1)
		assert.Error(t, err)
		assert.Equal(t, "database not initialized", err.Error())
		assert.Nil(t, resp)
	})

	t.Run("GetPendingReports", func(t *testing.T) {
		resp, total, err := svc.GetPendingReports(10, 0)
		assert.Error(t, err)
		assert.Equal(t, "database not initialized", err.Error())
		assert.Nil(t, resp)
		assert.Zero(t, total)
	})

	t.Run("DismissReport", func(t *testing.T) {
		resp, err := svc.DismissReport(1, 1, nil)
		assert.Error(t, err)
		assert.Equal(t, "database not initialized", err.Error())
		assert.Nil(t, resp)
	})

	t.Run("ResolveReport", func(t *testing.T) {
		resp, err := svc.ResolveReport(1, 1, nil)
		assert.Error(t, err)
		assert.Equal(t, "database not initialized", err.Error())
		assert.Nil(t, resp)
	})

	t.Run("ResolveReportWithFlag", func(t *testing.T) {
		resp, err := svc.ResolveReportWithFlag(1, 1, nil, true)
		assert.Error(t, err)
		assert.Equal(t, "database not initialized", err.Error())
		assert.Nil(t, resp)
	})

	t.Run("GetReportByID", func(t *testing.T) {
		resp, err := svc.GetReportByID(1)
		assert.Error(t, err)
		assert.Equal(t, "database not initialized", err.Error())
		assert.Nil(t, resp)
	})
}

// =============================================================================
// INTEGRATION TESTS (With Real Database)
// =============================================================================

type ShowReportServiceIntegrationTestSuite struct {
	suite.Suite
	container     testcontainers.Container
	db            *gorm.DB
	reportService *ShowReportService
	ctx           context.Context
}

func (suite *ShowReportServiceIntegrationTestSuite) SetupSuite() {
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
		"000007_add_private_show_status.up.sql",
		"000008_add_pending_venue_edits.up.sql",
		"000009_add_bandcamp_embed_url.up.sql",
		"000010_add_scraper_source_fields.up.sql",
		"000012_add_user_deletion_fields.up.sql",
		"000013_add_slugs.up.sql",
		"000014_add_account_lockout.up.sql",
		"000018_add_show_reports.up.sql",
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

	suite.reportService = &ShowReportService{db: db}
}

func (suite *ShowReportServiceIntegrationTestSuite) TearDownSuite() {
	if suite.container != nil {
		if err := suite.container.Terminate(suite.ctx); err != nil {
			suite.T().Logf("failed to terminate container: %v", err)
		}
	}
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
