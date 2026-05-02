package admin

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/catalog"
)

type AdminHandlerIntegrationSuite struct {
	suite.Suite
	deps         *testhelpers.IntegrationDeps
	showHandler  *AdminShowHandler
	venueHandler *AdminVenueHandler
	tokenHandler *AdminTokenHandler
	dataHandler  *AdminDataHandler
	userHandler  *AdminUserHandler
	statsHandler *AdminStatsHandler
}

func (s *AdminHandlerIntegrationSuite) SetupSuite() {
	s.deps = testhelpers.SetupIntegrationDeps(s.T())
	s.showHandler = NewAdminShowHandler(
		s.deps.ShowService,
		s.deps.ShowService,
		s.deps.ShowService,
		s.deps.DiscordService,
		s.deps.AuditLogService,
		nil, // notificationFilterService
		s.deps.MusicDiscoveryService,
	)
	s.venueHandler = NewAdminVenueHandler(
		s.deps.VenueService,
		s.deps.AuditLogService,
	)
	s.tokenHandler = NewAdminTokenHandler(
		s.deps.APITokenService,
	)
	s.dataHandler = NewAdminDataHandler(
		s.deps.DataSyncService,
	)
	s.userHandler = NewAdminUserHandler(
		s.deps.UserService,
	)
	s.statsHandler = NewAdminStatsHandler(
		s.deps.AdminStatsService,
	)
}

func (s *AdminHandlerIntegrationSuite) TearDownTest() {
	testhelpers.CleanupTables(s.deps.DB)
}

func (s *AdminHandlerIntegrationSuite) TearDownSuite() {
	s.deps.TestDB.Cleanup()
}

func TestAdminHandlerIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	suite.Run(t, new(AdminHandlerIntegrationSuite))
}

// --- GetPendingShowsHandler ---

func (s *AdminHandlerIntegrationSuite) TestGetPendingShows_Empty() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	ctx := testhelpers.CtxWithUser(admin)

	req := &GetPendingShowsRequest{Limit: 50, Offset: 0}
	resp, err := s.showHandler.GetPendingShowsHandler(ctx, req)
	s.NoError(err)
	s.Equal(int64(0), resp.Body.Total)
}

func (s *AdminHandlerIntegrationSuite) TestGetPendingShows_Success() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	user := testhelpers.CreateTestUser(s.deps.DB)

	testhelpers.CreatePendingShow(s.deps.DB, user.ID, "Pending Show 1")
	testhelpers.CreatePendingShow(s.deps.DB, user.ID, "Pending Show 2")

	ctx := testhelpers.CtxWithUser(admin)
	req := &GetPendingShowsRequest{Limit: 50, Offset: 0}
	resp, err := s.showHandler.GetPendingShowsHandler(ctx, req)
	s.NoError(err)
	s.Equal(int64(2), resp.Body.Total)
	s.Len(resp.Body.Shows, 2)
}

// --- ApproveShowHandler ---

func (s *AdminHandlerIntegrationSuite) TestApproveShow_Success() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	user := testhelpers.CreateTestUser(s.deps.DB)
	show := testhelpers.CreatePendingShow(s.deps.DB, user.ID, "Pending Show")

	ctx := testhelpers.CtxWithUser(admin)
	req := &ApproveShowRequest{ShowID: fmt.Sprintf("%d", show.ID)}

	resp, err := s.showHandler.ApproveShowHandler(ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.Equal("approved", resp.Body.Status)
}

func (s *AdminHandlerIntegrationSuite) TestApproveShow_NotFound() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	ctx := testhelpers.CtxWithUser(admin)

	req := &ApproveShowRequest{ShowID: "99999"}
	_, err := s.showHandler.ApproveShowHandler(ctx, req)
	s.Error(err)
}

func (s *AdminHandlerIntegrationSuite) TestApproveShow_AlreadyApproved() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	user := testhelpers.CreateTestUser(s.deps.DB)
	show := testhelpers.CreateApprovedShow(s.deps.DB, user.ID, "Approved Show")

	ctx := testhelpers.CtxWithUser(admin)
	req := &ApproveShowRequest{ShowID: fmt.Sprintf("%d", show.ID)}

	_, err := s.showHandler.ApproveShowHandler(ctx, req)
	s.Error(err)
}

func (s *AdminHandlerIntegrationSuite) TestApproveShow_WithVerifyVenues() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	user := testhelpers.CreateTestUser(s.deps.DB)

	// Create pending show with unverified venue
	show := &catalogm.Show{
		Title:       "Show With Unverified Venue",
		EventDate:   futureDate(7),
		City:        testhelpers.StringPtr("Phoenix"),
		State:       testhelpers.StringPtr("AZ"),
		Status:      catalogm.ShowStatusPending,
		SubmittedBy: &user.ID,
	}
	s.deps.DB.Create(show)

	venue := testhelpers.CreateUnverifiedVenue(s.deps.DB, "New Venue", "Phoenix", "AZ")
	artist := testhelpers.CreateArtist(s.deps.DB, "Test Artist")
	s.deps.DB.Exec("INSERT INTO show_venues (show_id, venue_id) VALUES (?, ?)", show.ID, venue.ID)
	s.deps.DB.Exec("INSERT INTO show_artists (show_id, artist_id, position, set_type) VALUES (?, ?, 0, 'headliner')", show.ID, artist.ID)

	ctx := testhelpers.CtxWithUser(admin)
	req := &ApproveShowRequest{ShowID: fmt.Sprintf("%d", show.ID)}
	req.Body.VerifyVenues = true

	resp, err := s.showHandler.ApproveShowHandler(ctx, req)
	s.NoError(err)
	s.Equal("approved", resp.Body.Status)
}

// --- RejectShowHandler ---

func (s *AdminHandlerIntegrationSuite) TestRejectShow_Success() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	user := testhelpers.CreateTestUser(s.deps.DB)
	show := testhelpers.CreatePendingShow(s.deps.DB, user.ID, "Pending Show")

	ctx := testhelpers.CtxWithUser(admin)
	req := &RejectShowRequest{ShowID: fmt.Sprintf("%d", show.ID)}
	req.Body.Reason = "Duplicate event"

	resp, err := s.showHandler.RejectShowHandler(ctx, req)
	s.NoError(err)
	s.Equal("rejected", resp.Body.Status)
}

func (s *AdminHandlerIntegrationSuite) TestRejectShow_NotFound() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	ctx := testhelpers.CtxWithUser(admin)

	req := &RejectShowRequest{ShowID: "99999"}
	req.Body.Reason = "Not found"
	_, err := s.showHandler.RejectShowHandler(ctx, req)
	s.Error(err)
}

func (s *AdminHandlerIntegrationSuite) TestRejectShow_EmptyReason() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	user := testhelpers.CreateTestUser(s.deps.DB)
	show := testhelpers.CreatePendingShow(s.deps.DB, user.ID, "Pending Show")

	ctx := testhelpers.CtxWithUser(admin)
	req := &RejectShowRequest{ShowID: fmt.Sprintf("%d", show.ID)}
	req.Body.Reason = ""

	_, err := s.showHandler.RejectShowHandler(ctx, req)
	s.Error(err)
}

// --- GetRejectedShowsHandler ---

func (s *AdminHandlerIntegrationSuite) TestGetRejectedShows_Success() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	user := testhelpers.CreateTestUser(s.deps.DB)

	// Create and reject a show
	show := testhelpers.CreatePendingShow(s.deps.DB, user.ID, "Will Be Rejected")
	ctx := testhelpers.CtxWithUser(admin)
	rejectReq := &RejectShowRequest{ShowID: fmt.Sprintf("%d", show.ID)}
	rejectReq.Body.Reason = "Test rejection"
	_, err := s.showHandler.RejectShowHandler(ctx, rejectReq)
	s.NoError(err)

	req := &GetRejectedShowsRequest{Limit: 50, Offset: 0}
	resp, err := s.showHandler.GetRejectedShowsHandler(ctx, req)
	s.NoError(err)
	s.GreaterOrEqual(resp.Body.Total, int64(1))
}

// --- VerifyVenueHandler ---

func (s *AdminHandlerIntegrationSuite) TestVerifyVenue_Success() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	venue := testhelpers.CreateUnverifiedVenue(s.deps.DB, "Unverified Venue", "Phoenix", "AZ")

	ctx := testhelpers.CtxWithUser(admin)
	req := &VerifyVenueRequest{VenueID: fmt.Sprintf("%d", venue.ID)}

	resp, err := s.venueHandler.VerifyVenueHandler(ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.True(resp.Body.Verified)
}

func (s *AdminHandlerIntegrationSuite) TestVerifyVenue_NotFound() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	ctx := testhelpers.CtxWithUser(admin)

	req := &VerifyVenueRequest{VenueID: "99999"}
	_, err := s.venueHandler.VerifyVenueHandler(ctx, req)
	s.Error(err)
}

// --- GetUnverifiedVenuesHandler ---

func (s *AdminHandlerIntegrationSuite) TestGetUnverifiedVenues_Success() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	testhelpers.CreateUnverifiedVenue(s.deps.DB, "Unverified 1", "Phoenix", "AZ")
	testhelpers.CreateUnverifiedVenue(s.deps.DB, "Unverified 2", "Tucson", "AZ")

	ctx := testhelpers.CtxWithUser(admin)
	req := &GetUnverifiedVenuesRequest{Limit: 50, Offset: 0}
	resp, err := s.venueHandler.GetUnverifiedVenuesHandler(ctx, req)
	s.NoError(err)
	s.GreaterOrEqual(resp.Body.Total, int64(2))
}

func (s *AdminHandlerIntegrationSuite) TestGetUnverifiedVenues_Empty() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	ctx := testhelpers.CtxWithUser(admin)

	req := &GetUnverifiedVenuesRequest{Limit: 50, Offset: 0}
	resp, err := s.venueHandler.GetUnverifiedVenuesHandler(ctx, req)
	s.NoError(err)
	s.Equal(int64(0), resp.Body.Total)
}

// --- GetAdminShowsHandler ---

func (s *AdminHandlerIntegrationSuite) TestGetAdminShows_Success() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	user := testhelpers.CreateTestUser(s.deps.DB)

	testhelpers.CreateApprovedShow(s.deps.DB, user.ID, "Show 1")
	testhelpers.CreatePendingShow(s.deps.DB, user.ID, "Show 2")

	ctx := testhelpers.CtxWithUser(admin)
	req := &GetAdminShowsRequest{Limit: 50, Offset: 0}
	resp, err := s.showHandler.GetAdminShowsHandler(ctx, req)
	s.NoError(err)
	s.GreaterOrEqual(resp.Body.Total, int64(2))
}

func (s *AdminHandlerIntegrationSuite) TestGetAdminShows_StatusFilter() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	user := testhelpers.CreateTestUser(s.deps.DB)

	testhelpers.CreateApprovedShow(s.deps.DB, user.ID, "Approved Show")
	testhelpers.CreatePendingShow(s.deps.DB, user.ID, "Pending Show")

	ctx := testhelpers.CtxWithUser(admin)
	req := &GetAdminShowsRequest{Limit: 50, Offset: 0, Status: "pending"}
	resp, err := s.showHandler.GetAdminShowsHandler(ctx, req)
	s.NoError(err)
	s.GreaterOrEqual(resp.Body.Total, int64(1))
	// All returned shows should be pending
	for _, show := range resp.Body.Shows {
		s.Equal("pending", show.Status)
	}
}

// --- GetAdminStatsHandler ---

func (s *AdminHandlerIntegrationSuite) TestGetAdminStats_Success() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	ctx := testhelpers.CtxWithUser(admin)

	req := &GetAdminStatsRequest{}
	resp, err := s.statsHandler.GetAdminStatsHandler(ctx, req)
	s.NoError(err)
	s.NotNil(resp)
}

// --- GetAdminUsersHandler ---

func (s *AdminHandlerIntegrationSuite) TestGetAdminUsers_Success() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	testhelpers.CreateTestUser(s.deps.DB)
	testhelpers.CreateTestUser(s.deps.DB)

	ctx := testhelpers.CtxWithUser(admin)
	req := &GetAdminUsersRequest{Limit: 50, Offset: 0}
	resp, err := s.userHandler.GetAdminUsersHandler(ctx, req)
	s.NoError(err)
	s.GreaterOrEqual(resp.Body.Total, int64(3)) // admin + 2 users
}

func (s *AdminHandlerIntegrationSuite) TestGetAdminUsers_Pagination() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	for i := 0; i < 5; i++ {
		testhelpers.CreateTestUser(s.deps.DB)
	}

	ctx := testhelpers.CtxWithUser(admin)
	req := &GetAdminUsersRequest{Limit: 3, Offset: 0}
	resp, err := s.userHandler.GetAdminUsersHandler(ctx, req)
	s.NoError(err)
	s.Len(resp.Body.Users, 3)
	s.GreaterOrEqual(resp.Body.Total, int64(6))
}

// --- CreateAPITokenHandler ---

func (s *AdminHandlerIntegrationSuite) TestCreateAPIToken_Success() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	ctx := testhelpers.CtxWithUser(admin)

	req := &CreateAPITokenRequest{}
	req.Body.Description = "Test token"
	req.Body.ExpirationDays = 30

	resp, err := s.tokenHandler.CreateAPITokenHandler(ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.NotEmpty(resp.Body.Token)
}

func (s *AdminHandlerIntegrationSuite) TestCreateAPIToken_DefaultExpiration() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	ctx := testhelpers.CtxWithUser(admin)

	req := &CreateAPITokenRequest{}
	req.Body.Description = "Default expiry token"

	resp, err := s.tokenHandler.CreateAPITokenHandler(ctx, req)
	s.NoError(err)
	s.NotNil(resp)
}

func (s *AdminHandlerIntegrationSuite) TestCreateAPIToken_ExceededMaxDays() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	ctx := testhelpers.CtxWithUser(admin)

	req := &CreateAPITokenRequest{}
	req.Body.ExpirationDays = 500

	_, err := s.tokenHandler.CreateAPITokenHandler(ctx, req)
	s.Error(err)
}

// --- ListAPITokensHandler ---

func (s *AdminHandlerIntegrationSuite) TestListAPITokens_Success() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	ctx := testhelpers.CtxWithUser(admin)

	// Create a token first
	createReq := &CreateAPITokenRequest{}
	createReq.Body.Description = "Token 1"
	createReq.Body.ExpirationDays = 30
	_, err := s.tokenHandler.CreateAPITokenHandler(ctx, createReq)
	s.NoError(err)

	// List
	listReq := &ListAPITokensRequest{}
	resp, err := s.tokenHandler.ListAPITokensHandler(ctx, listReq)
	s.NoError(err)
	s.GreaterOrEqual(len(resp.Body.Tokens), 1)
}

// --- RevokeAPITokenHandler ---

func (s *AdminHandlerIntegrationSuite) TestRevokeAPIToken_Success() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	ctx := testhelpers.CtxWithUser(admin)

	// Create a token
	createReq := &CreateAPITokenRequest{}
	createReq.Body.Description = "Revoke me"
	createReq.Body.ExpirationDays = 30
	createResp, err := s.tokenHandler.CreateAPITokenHandler(ctx, createReq)
	s.NoError(err)

	// Revoke
	revokeReq := &RevokeAPITokenRequest{TokenID: fmt.Sprintf("%d", createResp.Body.ID)}
	resp, err := s.tokenHandler.RevokeAPITokenHandler(ctx, revokeReq)
	s.NoError(err)
	s.Contains(resp.Body.Message, "revoked")
}

func (s *AdminHandlerIntegrationSuite) TestRevokeAPIToken_NotFound() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	ctx := testhelpers.CtxWithUser(admin)

	req := &RevokeAPITokenRequest{TokenID: "99999"}
	_, err := s.tokenHandler.RevokeAPITokenHandler(ctx, req)
	s.Error(err)
}

// --- ExportShowsHandler ---

func (s *AdminHandlerIntegrationSuite) TestExportShows_Success() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	user := testhelpers.CreateTestUser(s.deps.DB)
	testhelpers.CreateApprovedShow(s.deps.DB, user.ID, "Export Show")

	ctx := testhelpers.CtxWithUser(admin)
	req := &ExportShowsRequest{Limit: 50, Offset: 0}
	resp, err := s.dataHandler.ExportShowsHandler(ctx, req)
	s.NoError(err)
	s.GreaterOrEqual(len(resp.Body.Shows), 1)
}

// --- ExportArtistsHandler ---

func (s *AdminHandlerIntegrationSuite) TestExportArtists_Success() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	testhelpers.CreateArtist(s.deps.DB, "Test Artist")

	ctx := testhelpers.CtxWithUser(admin)
	req := &ExportArtistsRequest{Limit: 50, Offset: 0}
	resp, err := s.dataHandler.ExportArtistsHandler(ctx, req)
	s.NoError(err)
	s.GreaterOrEqual(len(resp.Body.Artists), 1)
}

// --- ExportVenuesHandler ---

func (s *AdminHandlerIntegrationSuite) TestExportVenues_Success() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	testhelpers.CreateVerifiedVenue(s.deps.DB, "Test Venue", "Phoenix", "AZ")

	ctx := testhelpers.CtxWithUser(admin)
	req := &ExportVenuesRequest{Limit: 50, Offset: 0}
	resp, err := s.dataHandler.ExportVenuesHandler(ctx, req)
	s.NoError(err)
	s.GreaterOrEqual(len(resp.Body.Venues), 1)
}

// helper
func futureDate(daysFromNow int) time.Time {
	return time.Now().UTC().AddDate(0, 0, daysFromNow)
}

// Ensure imports are used
var (
	_ *catalogm.Show
	_ *catalog.ShowService
)
