package community

import (
	"fmt"
	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	"testing"

	"github.com/stretchr/testify/suite"
)

type ShowReportHandlerIntegrationSuite struct {
	suite.Suite
	deps    *testhelpers.IntegrationDeps
	handler *ShowReportHandler
}

func (s *ShowReportHandlerIntegrationSuite) SetupSuite() {
	s.deps = testhelpers.SetupIntegrationDeps(s.T())
	s.handler = NewShowReportHandler(
		s.deps.ShowReportService,
		s.deps.DiscordService,
		s.deps.UserService,
		s.deps.AuditLogService,
	)
}

func (s *ShowReportHandlerIntegrationSuite) TearDownTest() {
	testhelpers.CleanupTables(s.deps.DB)
}

func (s *ShowReportHandlerIntegrationSuite) TearDownSuite() {
	s.deps.TestDB.Cleanup()
}

func TestShowReportHandlerIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	suite.Run(t, new(ShowReportHandlerIntegrationSuite))
}

// --- ReportShowHandler ---

func (s *ShowReportHandlerIntegrationSuite) TestReportShow_Success() {
	user := testhelpers.CreateTestUser(s.deps.DB)
	show := testhelpers.CreateApprovedShow(s.deps.DB, user.ID, "Test Show")

	ctx := testhelpers.CtxWithUser(user)
	req := &ReportShowRequest{
		ShowID: fmt.Sprintf("%d", show.ID),
	}
	req.Body.ReportType = "cancelled"

	resp, err := s.handler.ReportShowHandler(ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.Equal(show.ID, resp.Body.ShowID)
	s.Equal("cancelled", resp.Body.ReportType)
	s.Equal("pending", resp.Body.Status)
}

func (s *ShowReportHandlerIntegrationSuite) TestReportShow_WithDetails() {
	user := testhelpers.CreateTestUser(s.deps.DB)
	show := testhelpers.CreateApprovedShow(s.deps.DB, user.ID, "Test Show")

	ctx := testhelpers.CtxWithUser(user)
	details := "Wrong date listed"
	req := &ReportShowRequest{
		ShowID: fmt.Sprintf("%d", show.ID),
	}
	req.Body.ReportType = "inaccurate"
	req.Body.Details = &details

	resp, err := s.handler.ReportShowHandler(ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.Equal("inaccurate", resp.Body.ReportType)
}

func (s *ShowReportHandlerIntegrationSuite) TestReportShow_AlreadyReported() {
	user := testhelpers.CreateTestUser(s.deps.DB)
	show := testhelpers.CreateApprovedShow(s.deps.DB, user.ID, "Test Show")

	ctx := testhelpers.CtxWithUser(user)
	req := &ReportShowRequest{
		ShowID: fmt.Sprintf("%d", show.ID),
	}
	req.Body.ReportType = "cancelled"

	// Report once
	_, err := s.handler.ReportShowHandler(ctx, req)
	s.NoError(err)

	// Report again
	_, err = s.handler.ReportShowHandler(ctx, req)
	s.Error(err)
}

// --- GetMyReportHandler ---

func (s *ShowReportHandlerIntegrationSuite) TestGetMyReport_Exists() {
	user := testhelpers.CreateTestUser(s.deps.DB)
	show := testhelpers.CreateApprovedShow(s.deps.DB, user.ID, "Test Show")

	ctx := testhelpers.CtxWithUser(user)

	// Create report
	reportReq := &ReportShowRequest{ShowID: fmt.Sprintf("%d", show.ID)}
	reportReq.Body.ReportType = "sold_out"
	_, err := s.handler.ReportShowHandler(ctx, reportReq)
	s.NoError(err)

	// Get my report
	getReq := &GetMyReportRequest{ShowID: fmt.Sprintf("%d", show.ID)}
	resp, err := s.handler.GetMyReportHandler(ctx, getReq)
	s.NoError(err)
	s.NotNil(resp)
	s.NotNil(resp.Body.Report)
	s.Equal("sold_out", resp.Body.Report.ReportType)
}

func (s *ShowReportHandlerIntegrationSuite) TestGetMyReport_NotExists() {
	user := testhelpers.CreateTestUser(s.deps.DB)
	show := testhelpers.CreateApprovedShow(s.deps.DB, user.ID, "Test Show")

	ctx := testhelpers.CtxWithUser(user)
	getReq := &GetMyReportRequest{ShowID: fmt.Sprintf("%d", show.ID)}
	resp, err := s.handler.GetMyReportHandler(ctx, getReq)
	s.NoError(err)
	s.NotNil(resp)
	s.Nil(resp.Body.Report)
}

// --- GetPendingReportsHandler (admin) ---

func (s *ShowReportHandlerIntegrationSuite) TestGetPendingReports_Empty() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	ctx := testhelpers.CtxWithUser(admin)

	req := &GetPendingReportsRequest{Limit: 50, Offset: 0}
	resp, err := s.handler.GetPendingReportsHandler(ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.Equal(int64(0), resp.Body.Total)
}

func (s *ShowReportHandlerIntegrationSuite) TestGetPendingReports_WithReports() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	user := testhelpers.CreateTestUser(s.deps.DB)

	// Create 2 reports
	for i := 0; i < 2; i++ {
		show := testhelpers.CreateApprovedShow(s.deps.DB, user.ID, fmt.Sprintf("Show %d", i))
		ctx := testhelpers.CtxWithUser(user)
		req := &ReportShowRequest{ShowID: fmt.Sprintf("%d", show.ID)}
		req.Body.ReportType = "cancelled"
		_, err := s.handler.ReportShowHandler(ctx, req)
		s.NoError(err)
	}

	ctx := testhelpers.CtxWithUser(admin)
	req := &GetPendingReportsRequest{Limit: 50, Offset: 0}
	resp, err := s.handler.GetPendingReportsHandler(ctx, req)
	s.NoError(err)
	s.Equal(int64(2), resp.Body.Total)
	s.Len(resp.Body.Reports, 2)
}

// --- DismissReportHandler (admin) ---

func (s *ShowReportHandlerIntegrationSuite) TestDismissReport_Success() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	user := testhelpers.CreateTestUser(s.deps.DB)
	show := testhelpers.CreateApprovedShow(s.deps.DB, user.ID, "Test Show")

	// Create report
	userCtx := testhelpers.CtxWithUser(user)
	reportReq := &ReportShowRequest{ShowID: fmt.Sprintf("%d", show.ID)}
	reportReq.Body.ReportType = "cancelled"
	reportResp, err := s.handler.ReportShowHandler(userCtx, reportReq)
	s.NoError(err)

	// Admin dismisses
	adminCtx := testhelpers.CtxWithUser(admin)
	dismissReq := &DismissReportRequest{
		ReportID: fmt.Sprintf("%d", reportResp.Body.ID),
	}
	resp, err := s.handler.DismissReportHandler(adminCtx, dismissReq)
	s.NoError(err)
	s.NotNil(resp)
	s.Equal("dismissed", resp.Body.Status)
}

func (s *ShowReportHandlerIntegrationSuite) TestDismissReport_NotFound() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	adminCtx := testhelpers.CtxWithUser(admin)

	dismissReq := &DismissReportRequest{ReportID: "99999"}
	_, err := s.handler.DismissReportHandler(adminCtx, dismissReq)
	s.Error(err)
}

// --- ResolveReportHandler (admin) ---

func (s *ShowReportHandlerIntegrationSuite) TestResolveReport_Success() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	user := testhelpers.CreateTestUser(s.deps.DB)
	show := testhelpers.CreateApprovedShow(s.deps.DB, user.ID, "Test Show")

	// Create report
	userCtx := testhelpers.CtxWithUser(user)
	reportReq := &ReportShowRequest{ShowID: fmt.Sprintf("%d", show.ID)}
	reportReq.Body.ReportType = "cancelled"
	reportResp, err := s.handler.ReportShowHandler(userCtx, reportReq)
	s.NoError(err)

	// Admin resolves
	adminCtx := testhelpers.CtxWithUser(admin)
	resolveReq := &ResolveReportRequest{
		ReportID: fmt.Sprintf("%d", reportResp.Body.ID),
	}
	resolveReq.Body.SetShowFlag = testhelpers.BoolPtr(false)
	resp, err := s.handler.ResolveReportHandler(adminCtx, resolveReq)
	s.NoError(err)
	s.NotNil(resp)
	s.Equal("resolved", resp.Body.Status)
}

func (s *ShowReportHandlerIntegrationSuite) TestResolveReport_WithShowFlag() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	user := testhelpers.CreateTestUser(s.deps.DB)
	show := testhelpers.CreateApprovedShow(s.deps.DB, user.ID, "Test Show")

	// Create report
	userCtx := testhelpers.CtxWithUser(user)
	reportReq := &ReportShowRequest{ShowID: fmt.Sprintf("%d", show.ID)}
	reportReq.Body.ReportType = "cancelled"
	reportResp, err := s.handler.ReportShowHandler(userCtx, reportReq)
	s.NoError(err)

	// Admin resolves with flag
	adminCtx := testhelpers.CtxWithUser(admin)
	resolveReq := &ResolveReportRequest{
		ReportID: fmt.Sprintf("%d", reportResp.Body.ID),
	}
	resolveReq.Body.SetShowFlag = testhelpers.BoolPtr(true)
	resp, err := s.handler.ResolveReportHandler(adminCtx, resolveReq)
	s.NoError(err)
	s.NotNil(resp)
	s.Equal("resolved", resp.Body.Status)
}

func (s *ShowReportHandlerIntegrationSuite) TestResolveReport_NotFound() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	adminCtx := testhelpers.CtxWithUser(admin)

	resolveReq := &ResolveReportRequest{ReportID: "99999"}
	resolveReq.Body.SetShowFlag = testhelpers.BoolPtr(false)
	_, err := s.handler.ResolveReportHandler(adminCtx, resolveReq)
	s.Error(err)
}
