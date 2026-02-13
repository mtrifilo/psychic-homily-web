package handlers

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/suite"
)

type ShowReportHandlerIntegrationSuite struct {
	suite.Suite
	deps    *handlerIntegrationDeps
	handler *ShowReportHandler
}

func (s *ShowReportHandlerIntegrationSuite) SetupSuite() {
	s.deps = setupHandlerIntegrationDeps(s.T())
	s.handler = NewShowReportHandler(
		s.deps.showReportService,
		s.deps.discordService,
		s.deps.userService,
		s.deps.auditLogService,
	)
}

func (s *ShowReportHandlerIntegrationSuite) TearDownTest() {
	cleanupTables(s.deps.db)
}

func (s *ShowReportHandlerIntegrationSuite) TearDownSuite() {
	if s.deps.container != nil {
		s.deps.container.Terminate(s.deps.ctx)
	}
}

func TestShowReportHandlerIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	suite.Run(t, new(ShowReportHandlerIntegrationSuite))
}

// --- ReportShowHandler ---

func (s *ShowReportHandlerIntegrationSuite) TestReportShow_Success() {
	user := createTestUser(s.deps.db)
	show := createApprovedShow(s.deps.db, user.ID, "Test Show")

	ctx := ctxWithUser(user)
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
	user := createTestUser(s.deps.db)
	show := createApprovedShow(s.deps.db, user.ID, "Test Show")

	ctx := ctxWithUser(user)
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
	user := createTestUser(s.deps.db)
	show := createApprovedShow(s.deps.db, user.ID, "Test Show")

	ctx := ctxWithUser(user)
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
	user := createTestUser(s.deps.db)
	show := createApprovedShow(s.deps.db, user.ID, "Test Show")

	ctx := ctxWithUser(user)

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
	user := createTestUser(s.deps.db)
	show := createApprovedShow(s.deps.db, user.ID, "Test Show")

	ctx := ctxWithUser(user)
	getReq := &GetMyReportRequest{ShowID: fmt.Sprintf("%d", show.ID)}
	resp, err := s.handler.GetMyReportHandler(ctx, getReq)
	s.NoError(err)
	s.NotNil(resp)
	s.Nil(resp.Body.Report)
}

// --- GetPendingReportsHandler (admin) ---

func (s *ShowReportHandlerIntegrationSuite) TestGetPendingReports_Empty() {
	admin := createAdminUser(s.deps.db)
	ctx := ctxWithUser(admin)

	req := &GetPendingReportsRequest{Limit: 50, Offset: 0}
	resp, err := s.handler.GetPendingReportsHandler(ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.Equal(int64(0), resp.Body.Total)
}

func (s *ShowReportHandlerIntegrationSuite) TestGetPendingReports_WithReports() {
	admin := createAdminUser(s.deps.db)
	user := createTestUser(s.deps.db)

	// Create 2 reports
	for i := 0; i < 2; i++ {
		show := createApprovedShow(s.deps.db, user.ID, fmt.Sprintf("Show %d", i))
		ctx := ctxWithUser(user)
		req := &ReportShowRequest{ShowID: fmt.Sprintf("%d", show.ID)}
		req.Body.ReportType = "cancelled"
		_, err := s.handler.ReportShowHandler(ctx, req)
		s.NoError(err)
	}

	ctx := ctxWithUser(admin)
	req := &GetPendingReportsRequest{Limit: 50, Offset: 0}
	resp, err := s.handler.GetPendingReportsHandler(ctx, req)
	s.NoError(err)
	s.Equal(int64(2), resp.Body.Total)
	s.Len(resp.Body.Reports, 2)
}

// --- DismissReportHandler (admin) ---

func (s *ShowReportHandlerIntegrationSuite) TestDismissReport_Success() {
	admin := createAdminUser(s.deps.db)
	user := createTestUser(s.deps.db)
	show := createApprovedShow(s.deps.db, user.ID, "Test Show")

	// Create report
	userCtx := ctxWithUser(user)
	reportReq := &ReportShowRequest{ShowID: fmt.Sprintf("%d", show.ID)}
	reportReq.Body.ReportType = "cancelled"
	reportResp, err := s.handler.ReportShowHandler(userCtx, reportReq)
	s.NoError(err)

	// Admin dismisses
	adminCtx := ctxWithUser(admin)
	dismissReq := &DismissReportRequest{
		ReportID: fmt.Sprintf("%d", reportResp.Body.ID),
	}
	resp, err := s.handler.DismissReportHandler(adminCtx, dismissReq)
	s.NoError(err)
	s.NotNil(resp)
	s.Equal("dismissed", resp.Body.Status)
}

func (s *ShowReportHandlerIntegrationSuite) TestDismissReport_NotFound() {
	admin := createAdminUser(s.deps.db)
	adminCtx := ctxWithUser(admin)

	dismissReq := &DismissReportRequest{ReportID: "99999"}
	_, err := s.handler.DismissReportHandler(adminCtx, dismissReq)
	s.Error(err)
}

// --- ResolveReportHandler (admin) ---

func (s *ShowReportHandlerIntegrationSuite) TestResolveReport_Success() {
	admin := createAdminUser(s.deps.db)
	user := createTestUser(s.deps.db)
	show := createApprovedShow(s.deps.db, user.ID, "Test Show")

	// Create report
	userCtx := ctxWithUser(user)
	reportReq := &ReportShowRequest{ShowID: fmt.Sprintf("%d", show.ID)}
	reportReq.Body.ReportType = "cancelled"
	reportResp, err := s.handler.ReportShowHandler(userCtx, reportReq)
	s.NoError(err)

	// Admin resolves
	adminCtx := ctxWithUser(admin)
	resolveReq := &ResolveReportRequest{
		ReportID: fmt.Sprintf("%d", reportResp.Body.ID),
	}
	resolveReq.Body.SetShowFlag = boolPtr(false)
	resp, err := s.handler.ResolveReportHandler(adminCtx, resolveReq)
	s.NoError(err)
	s.NotNil(resp)
	s.Equal("resolved", resp.Body.Status)
}

func (s *ShowReportHandlerIntegrationSuite) TestResolveReport_WithShowFlag() {
	admin := createAdminUser(s.deps.db)
	user := createTestUser(s.deps.db)
	show := createApprovedShow(s.deps.db, user.ID, "Test Show")

	// Create report
	userCtx := ctxWithUser(user)
	reportReq := &ReportShowRequest{ShowID: fmt.Sprintf("%d", show.ID)}
	reportReq.Body.ReportType = "cancelled"
	reportResp, err := s.handler.ReportShowHandler(userCtx, reportReq)
	s.NoError(err)

	// Admin resolves with flag
	adminCtx := ctxWithUser(admin)
	resolveReq := &ResolveReportRequest{
		ReportID: fmt.Sprintf("%d", reportResp.Body.ID),
	}
	resolveReq.Body.SetShowFlag = boolPtr(true)
	resp, err := s.handler.ResolveReportHandler(adminCtx, resolveReq)
	s.NoError(err)
	s.NotNil(resp)
	s.Equal("resolved", resp.Body.Status)
}

func (s *ShowReportHandlerIntegrationSuite) TestResolveReport_NotFound() {
	admin := createAdminUser(s.deps.db)
	adminCtx := ctxWithUser(admin)

	resolveReq := &ResolveReportRequest{ReportID: "99999"}
	resolveReq.Body.SetShowFlag = boolPtr(false)
	_, err := s.handler.ResolveReportHandler(adminCtx, resolveReq)
	s.Error(err)
}
