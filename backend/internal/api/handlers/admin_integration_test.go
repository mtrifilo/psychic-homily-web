package handlers

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	"psychic-homily-backend/internal/models"
	"psychic-homily-backend/internal/services"
)

type AdminHandlerIntegrationSuite struct {
	suite.Suite
	deps    *handlerIntegrationDeps
	handler *AdminHandler
}

func (s *AdminHandlerIntegrationSuite) SetupSuite() {
	s.deps = setupHandlerIntegrationDeps(s.T())
	s.handler = NewAdminHandler(
		s.deps.showService,
		s.deps.venueService,
		s.deps.discordService,
		s.deps.musicDiscoveryService,
		s.deps.discoveryService,
		s.deps.apiTokenService,
		s.deps.dataSyncService,
		s.deps.auditLogService,
		s.deps.userService,
		s.deps.adminStatsService,
	)
}

func (s *AdminHandlerIntegrationSuite) TearDownTest() {
	cleanupTables(s.deps.db)
}

func (s *AdminHandlerIntegrationSuite) TearDownSuite() {
	if s.deps.container != nil {
		s.deps.container.Terminate(s.deps.ctx)
	}
}

func TestAdminHandlerIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	suite.Run(t, new(AdminHandlerIntegrationSuite))
}

// --- GetPendingShowsHandler ---

func (s *AdminHandlerIntegrationSuite) TestGetPendingShows_Empty() {
	admin := createAdminUser(s.deps.db)
	ctx := ctxWithUser(admin)

	req := &GetPendingShowsRequest{Limit: 50, Offset: 0}
	resp, err := s.handler.GetPendingShowsHandler(ctx, req)
	s.NoError(err)
	s.Equal(int64(0), resp.Body.Total)
}

func (s *AdminHandlerIntegrationSuite) TestGetPendingShows_Success() {
	admin := createAdminUser(s.deps.db)
	user := createTestUser(s.deps.db)

	createPendingShow(s.deps.db, user.ID, "Pending Show 1")
	createPendingShow(s.deps.db, user.ID, "Pending Show 2")

	ctx := ctxWithUser(admin)
	req := &GetPendingShowsRequest{Limit: 50, Offset: 0}
	resp, err := s.handler.GetPendingShowsHandler(ctx, req)
	s.NoError(err)
	s.Equal(int64(2), resp.Body.Total)
	s.Len(resp.Body.Shows, 2)
}

// --- ApproveShowHandler ---

func (s *AdminHandlerIntegrationSuite) TestApproveShow_Success() {
	admin := createAdminUser(s.deps.db)
	user := createTestUser(s.deps.db)
	show := createPendingShow(s.deps.db, user.ID, "Pending Show")

	ctx := ctxWithUser(admin)
	req := &ApproveShowRequest{ShowID: fmt.Sprintf("%d", show.ID)}

	resp, err := s.handler.ApproveShowHandler(ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.Equal("approved", resp.Body.Status)
}

func (s *AdminHandlerIntegrationSuite) TestApproveShow_NotFound() {
	admin := createAdminUser(s.deps.db)
	ctx := ctxWithUser(admin)

	req := &ApproveShowRequest{ShowID: "99999"}
	_, err := s.handler.ApproveShowHandler(ctx, req)
	s.Error(err)
}

func (s *AdminHandlerIntegrationSuite) TestApproveShow_AlreadyApproved() {
	admin := createAdminUser(s.deps.db)
	user := createTestUser(s.deps.db)
	show := createApprovedShow(s.deps.db, user.ID, "Approved Show")

	ctx := ctxWithUser(admin)
	req := &ApproveShowRequest{ShowID: fmt.Sprintf("%d", show.ID)}

	_, err := s.handler.ApproveShowHandler(ctx, req)
	s.Error(err)
}

func (s *AdminHandlerIntegrationSuite) TestApproveShow_WithVerifyVenues() {
	admin := createAdminUser(s.deps.db)
	user := createTestUser(s.deps.db)

	// Create pending show with unverified venue
	show := &models.Show{
		Title:       "Show With Unverified Venue",
		EventDate:   futureDate(7),
		City:        stringPtr("Phoenix"),
		State:       stringPtr("AZ"),
		Status:      models.ShowStatusPending,
		SubmittedBy: &user.ID,
	}
	s.deps.db.Create(show)

	venue := createUnverifiedVenue(s.deps.db, "New Venue", "Phoenix", "AZ")
	artist := createArtist(s.deps.db, "Test Artist")
	s.deps.db.Exec("INSERT INTO show_venues (show_id, venue_id) VALUES (?, ?)", show.ID, venue.ID)
	s.deps.db.Exec("INSERT INTO show_artists (show_id, artist_id, position, set_type) VALUES (?, ?, 0, 'headliner')", show.ID, artist.ID)

	ctx := ctxWithUser(admin)
	req := &ApproveShowRequest{ShowID: fmt.Sprintf("%d", show.ID)}
	req.Body.VerifyVenues = true

	resp, err := s.handler.ApproveShowHandler(ctx, req)
	s.NoError(err)
	s.Equal("approved", resp.Body.Status)
}

// --- RejectShowHandler ---

func (s *AdminHandlerIntegrationSuite) TestRejectShow_Success() {
	admin := createAdminUser(s.deps.db)
	user := createTestUser(s.deps.db)
	show := createPendingShow(s.deps.db, user.ID, "Pending Show")

	ctx := ctxWithUser(admin)
	req := &RejectShowRequest{ShowID: fmt.Sprintf("%d", show.ID)}
	req.Body.Reason = "Duplicate event"

	resp, err := s.handler.RejectShowHandler(ctx, req)
	s.NoError(err)
	s.Equal("rejected", resp.Body.Status)
}

func (s *AdminHandlerIntegrationSuite) TestRejectShow_NotFound() {
	admin := createAdminUser(s.deps.db)
	ctx := ctxWithUser(admin)

	req := &RejectShowRequest{ShowID: "99999"}
	req.Body.Reason = "Not found"
	_, err := s.handler.RejectShowHandler(ctx, req)
	s.Error(err)
}

func (s *AdminHandlerIntegrationSuite) TestRejectShow_EmptyReason() {
	admin := createAdminUser(s.deps.db)
	user := createTestUser(s.deps.db)
	show := createPendingShow(s.deps.db, user.ID, "Pending Show")

	ctx := ctxWithUser(admin)
	req := &RejectShowRequest{ShowID: fmt.Sprintf("%d", show.ID)}
	req.Body.Reason = ""

	_, err := s.handler.RejectShowHandler(ctx, req)
	s.Error(err)
}

// --- GetRejectedShowsHandler ---

func (s *AdminHandlerIntegrationSuite) TestGetRejectedShows_Success() {
	admin := createAdminUser(s.deps.db)
	user := createTestUser(s.deps.db)

	// Create and reject a show
	show := createPendingShow(s.deps.db, user.ID, "Will Be Rejected")
	ctx := ctxWithUser(admin)
	rejectReq := &RejectShowRequest{ShowID: fmt.Sprintf("%d", show.ID)}
	rejectReq.Body.Reason = "Test rejection"
	_, err := s.handler.RejectShowHandler(ctx, rejectReq)
	s.NoError(err)

	req := &GetRejectedShowsRequest{Limit: 50, Offset: 0}
	resp, err := s.handler.GetRejectedShowsHandler(ctx, req)
	s.NoError(err)
	s.GreaterOrEqual(resp.Body.Total, int64(1))
}

// --- VerifyVenueHandler ---

func (s *AdminHandlerIntegrationSuite) TestVerifyVenue_Success() {
	admin := createAdminUser(s.deps.db)
	venue := createUnverifiedVenue(s.deps.db, "Unverified Venue", "Phoenix", "AZ")

	ctx := ctxWithUser(admin)
	req := &VerifyVenueRequest{VenueID: fmt.Sprintf("%d", venue.ID)}

	resp, err := s.handler.VerifyVenueHandler(ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.True(resp.Body.Verified)
}

func (s *AdminHandlerIntegrationSuite) TestVerifyVenue_NotFound() {
	admin := createAdminUser(s.deps.db)
	ctx := ctxWithUser(admin)

	req := &VerifyVenueRequest{VenueID: "99999"}
	_, err := s.handler.VerifyVenueHandler(ctx, req)
	s.Error(err)
}

// --- GetUnverifiedVenuesHandler ---

func (s *AdminHandlerIntegrationSuite) TestGetUnverifiedVenues_Success() {
	admin := createAdminUser(s.deps.db)
	createUnverifiedVenue(s.deps.db, "Unverified 1", "Phoenix", "AZ")
	createUnverifiedVenue(s.deps.db, "Unverified 2", "Tucson", "AZ")

	ctx := ctxWithUser(admin)
	req := &GetUnverifiedVenuesRequest{Limit: 50, Offset: 0}
	resp, err := s.handler.GetUnverifiedVenuesHandler(ctx, req)
	s.NoError(err)
	s.GreaterOrEqual(resp.Body.Total, int64(2))
}

func (s *AdminHandlerIntegrationSuite) TestGetUnverifiedVenues_Empty() {
	admin := createAdminUser(s.deps.db)
	ctx := ctxWithUser(admin)

	req := &GetUnverifiedVenuesRequest{Limit: 50, Offset: 0}
	resp, err := s.handler.GetUnverifiedVenuesHandler(ctx, req)
	s.NoError(err)
	s.Equal(int64(0), resp.Body.Total)
}

// --- GetPendingVenueEditsHandler ---

func (s *AdminHandlerIntegrationSuite) TestGetPendingVenueEdits_Empty() {
	admin := createAdminUser(s.deps.db)
	ctx := ctxWithUser(admin)

	req := &GetPendingVenueEditsRequest{Limit: 50, Offset: 0}
	resp, err := s.handler.GetPendingVenueEditsHandler(ctx, req)
	s.NoError(err)
	s.Equal(int64(0), resp.Body.Total)
}

func (s *AdminHandlerIntegrationSuite) TestGetPendingVenueEdits_Success() {
	admin := createAdminUser(s.deps.db)
	user := createTestUser(s.deps.db)

	// Create venue owned by user
	venue := &models.Venue{
		Name:        "User Venue",
		City:        "Phoenix",
		State:       "AZ",
		Verified:    true,
		SubmittedBy: &user.ID,
	}
	s.deps.db.Create(venue)

	// Create pending edit via venue handler
	venueHandler := NewVenueHandler(s.deps.venueService, s.deps.discordService)
	userCtx := ctxWithUser(user)
	newName := "Updated Name"
	updateReq := &UpdateVenueRequest{VenueID: fmt.Sprintf("%d", venue.ID)}
	updateReq.Body.Name = &newName
	_, err := venueHandler.UpdateVenueHandler(userCtx, updateReq)
	s.NoError(err)

	// Admin queries pending edits
	adminCtx := ctxWithUser(admin)
	req := &GetPendingVenueEditsRequest{Limit: 50, Offset: 0}
	resp, err := s.handler.GetPendingVenueEditsHandler(adminCtx, req)
	s.NoError(err)
	s.GreaterOrEqual(resp.Body.Total, int64(1))
}

// --- ApproveVenueEditHandler ---

func (s *AdminHandlerIntegrationSuite) TestApproveVenueEdit_Success() {
	admin := createAdminUser(s.deps.db)
	user := createTestUser(s.deps.db)

	venue := &models.Venue{
		Name:        "User Venue",
		City:        "Phoenix",
		State:       "AZ",
		Verified:    true,
		SubmittedBy: &user.ID,
	}
	s.deps.db.Create(venue)

	// Create pending edit
	venueHandler := NewVenueHandler(s.deps.venueService, s.deps.discordService)
	userCtx := ctxWithUser(user)
	newName := "Approved Name"
	updateReq := &UpdateVenueRequest{VenueID: fmt.Sprintf("%d", venue.ID)}
	updateReq.Body.Name = &newName
	updateResp, err := venueHandler.UpdateVenueHandler(userCtx, updateReq)
	s.NoError(err)
	s.NotNil(updateResp.Body.PendingEdit)

	// Admin approves
	adminCtx := ctxWithUser(admin)
	approveReq := &ApproveVenueEditRequest{EditID: fmt.Sprintf("%d", updateResp.Body.PendingEdit.ID)}
	resp, err := s.handler.ApproveVenueEditHandler(adminCtx, approveReq)
	s.NoError(err)
	s.NotNil(resp)
	s.Equal("Approved Name", resp.Body.Name)
}

func (s *AdminHandlerIntegrationSuite) TestApproveVenueEdit_NotFound() {
	admin := createAdminUser(s.deps.db)
	ctx := ctxWithUser(admin)

	req := &ApproveVenueEditRequest{EditID: "99999"}
	_, err := s.handler.ApproveVenueEditHandler(ctx, req)
	s.Error(err)
}

// --- RejectVenueEditHandler ---

func (s *AdminHandlerIntegrationSuite) TestRejectVenueEdit_Success() {
	admin := createAdminUser(s.deps.db)
	user := createTestUser(s.deps.db)

	venue := &models.Venue{
		Name:        "User Venue",
		City:        "Phoenix",
		State:       "AZ",
		Verified:    true,
		SubmittedBy: &user.ID,
	}
	s.deps.db.Create(venue)

	// Create pending edit
	venueHandler := NewVenueHandler(s.deps.venueService, s.deps.discordService)
	userCtx := ctxWithUser(user)
	newName := "Rejected Name"
	updateReq := &UpdateVenueRequest{VenueID: fmt.Sprintf("%d", venue.ID)}
	updateReq.Body.Name = &newName
	updateResp, err := venueHandler.UpdateVenueHandler(userCtx, updateReq)
	s.NoError(err)

	// Admin rejects
	adminCtx := ctxWithUser(admin)
	rejectReq := &RejectVenueEditRequest{EditID: fmt.Sprintf("%d", updateResp.Body.PendingEdit.ID)}
	rejectReq.Body.Reason = "Not accurate"
	resp, err := s.handler.RejectVenueEditHandler(adminCtx, rejectReq)
	s.NoError(err)
	s.NotNil(resp)
	s.Equal(models.VenueEditStatusRejected, resp.Body.Status)
}

// --- GetAdminShowsHandler ---

func (s *AdminHandlerIntegrationSuite) TestGetAdminShows_Success() {
	admin := createAdminUser(s.deps.db)
	user := createTestUser(s.deps.db)

	createApprovedShow(s.deps.db, user.ID, "Show 1")
	createPendingShow(s.deps.db, user.ID, "Show 2")

	ctx := ctxWithUser(admin)
	req := &GetAdminShowsRequest{Limit: 50, Offset: 0}
	resp, err := s.handler.GetAdminShowsHandler(ctx, req)
	s.NoError(err)
	s.GreaterOrEqual(resp.Body.Total, int64(2))
}

func (s *AdminHandlerIntegrationSuite) TestGetAdminShows_StatusFilter() {
	admin := createAdminUser(s.deps.db)
	user := createTestUser(s.deps.db)

	createApprovedShow(s.deps.db, user.ID, "Approved Show")
	createPendingShow(s.deps.db, user.ID, "Pending Show")

	ctx := ctxWithUser(admin)
	req := &GetAdminShowsRequest{Limit: 50, Offset: 0, Status: "pending"}
	resp, err := s.handler.GetAdminShowsHandler(ctx, req)
	s.NoError(err)
	s.GreaterOrEqual(resp.Body.Total, int64(1))
	// All returned shows should be pending
	for _, show := range resp.Body.Shows {
		s.Equal("pending", show.Status)
	}
}

// --- GetAdminStatsHandler ---

func (s *AdminHandlerIntegrationSuite) TestGetAdminStats_Success() {
	admin := createAdminUser(s.deps.db)
	ctx := ctxWithUser(admin)

	req := &GetAdminStatsRequest{}
	resp, err := s.handler.GetAdminStatsHandler(ctx, req)
	s.NoError(err)
	s.NotNil(resp)
}

// --- GetAdminUsersHandler ---

func (s *AdminHandlerIntegrationSuite) TestGetAdminUsers_Success() {
	admin := createAdminUser(s.deps.db)
	createTestUser(s.deps.db)
	createTestUser(s.deps.db)

	ctx := ctxWithUser(admin)
	req := &GetAdminUsersRequest{Limit: 50, Offset: 0}
	resp, err := s.handler.GetAdminUsersHandler(ctx, req)
	s.NoError(err)
	s.GreaterOrEqual(resp.Body.Total, int64(3)) // admin + 2 users
}

func (s *AdminHandlerIntegrationSuite) TestGetAdminUsers_Pagination() {
	admin := createAdminUser(s.deps.db)
	for i := 0; i < 5; i++ {
		createTestUser(s.deps.db)
	}

	ctx := ctxWithUser(admin)
	req := &GetAdminUsersRequest{Limit: 3, Offset: 0}
	resp, err := s.handler.GetAdminUsersHandler(ctx, req)
	s.NoError(err)
	s.Len(resp.Body.Users, 3)
	s.GreaterOrEqual(resp.Body.Total, int64(6))
}

// --- CreateAPITokenHandler ---

func (s *AdminHandlerIntegrationSuite) TestCreateAPIToken_Success() {
	admin := createAdminUser(s.deps.db)
	ctx := ctxWithUser(admin)

	req := &CreateAPITokenRequest{}
	req.Body.Description = "Test token"
	req.Body.ExpirationDays = 30

	resp, err := s.handler.CreateAPITokenHandler(ctx, req)
	s.NoError(err)
	s.NotNil(resp)
	s.NotEmpty(resp.Body.Token)
}

func (s *AdminHandlerIntegrationSuite) TestCreateAPIToken_DefaultExpiration() {
	admin := createAdminUser(s.deps.db)
	ctx := ctxWithUser(admin)

	req := &CreateAPITokenRequest{}
	req.Body.Description = "Default expiry token"

	resp, err := s.handler.CreateAPITokenHandler(ctx, req)
	s.NoError(err)
	s.NotNil(resp)
}

func (s *AdminHandlerIntegrationSuite) TestCreateAPIToken_ExceededMaxDays() {
	admin := createAdminUser(s.deps.db)
	ctx := ctxWithUser(admin)

	req := &CreateAPITokenRequest{}
	req.Body.ExpirationDays = 500

	_, err := s.handler.CreateAPITokenHandler(ctx, req)
	s.Error(err)
}

// --- ListAPITokensHandler ---

func (s *AdminHandlerIntegrationSuite) TestListAPITokens_Success() {
	admin := createAdminUser(s.deps.db)
	ctx := ctxWithUser(admin)

	// Create a token first
	createReq := &CreateAPITokenRequest{}
	createReq.Body.Description = "Token 1"
	createReq.Body.ExpirationDays = 30
	_, err := s.handler.CreateAPITokenHandler(ctx, createReq)
	s.NoError(err)

	// List
	listReq := &ListAPITokensRequest{}
	resp, err := s.handler.ListAPITokensHandler(ctx, listReq)
	s.NoError(err)
	s.GreaterOrEqual(len(resp.Body.Tokens), 1)
}

// --- RevokeAPITokenHandler ---

func (s *AdminHandlerIntegrationSuite) TestRevokeAPIToken_Success() {
	admin := createAdminUser(s.deps.db)
	ctx := ctxWithUser(admin)

	// Create a token
	createReq := &CreateAPITokenRequest{}
	createReq.Body.Description = "Revoke me"
	createReq.Body.ExpirationDays = 30
	createResp, err := s.handler.CreateAPITokenHandler(ctx, createReq)
	s.NoError(err)

	// Revoke
	revokeReq := &RevokeAPITokenRequest{TokenID: fmt.Sprintf("%d", createResp.Body.ID)}
	resp, err := s.handler.RevokeAPITokenHandler(ctx, revokeReq)
	s.NoError(err)
	s.Contains(resp.Body.Message, "revoked")
}

func (s *AdminHandlerIntegrationSuite) TestRevokeAPIToken_NotFound() {
	admin := createAdminUser(s.deps.db)
	ctx := ctxWithUser(admin)

	req := &RevokeAPITokenRequest{TokenID: "99999"}
	_, err := s.handler.RevokeAPITokenHandler(ctx, req)
	s.Error(err)
}

// --- ExportShowsHandler ---

func (s *AdminHandlerIntegrationSuite) TestExportShows_Success() {
	admin := createAdminUser(s.deps.db)
	user := createTestUser(s.deps.db)
	createApprovedShow(s.deps.db, user.ID, "Export Show")

	ctx := ctxWithUser(admin)
	req := &ExportShowsRequest{Limit: 50, Offset: 0}
	resp, err := s.handler.ExportShowsHandler(ctx, req)
	s.NoError(err)
	s.GreaterOrEqual(len(resp.Body.Shows), 1)
}

// --- ExportArtistsHandler ---

func (s *AdminHandlerIntegrationSuite) TestExportArtists_Success() {
	admin := createAdminUser(s.deps.db)
	createArtist(s.deps.db, "Test Artist")

	ctx := ctxWithUser(admin)
	req := &ExportArtistsRequest{Limit: 50, Offset: 0}
	resp, err := s.handler.ExportArtistsHandler(ctx, req)
	s.NoError(err)
	s.GreaterOrEqual(len(resp.Body.Artists), 1)
}

// --- ExportVenuesHandler ---

func (s *AdminHandlerIntegrationSuite) TestExportVenues_Success() {
	admin := createAdminUser(s.deps.db)
	createVerifiedVenue(s.deps.db, "Test Venue", "Phoenix", "AZ")

	ctx := ctxWithUser(admin)
	req := &ExportVenuesRequest{Limit: 50, Offset: 0}
	resp, err := s.handler.ExportVenuesHandler(ctx, req)
	s.NoError(err)
	s.GreaterOrEqual(len(resp.Body.Venues), 1)
}

// helper
func futureDate(daysFromNow int) time.Time {
	return time.Now().UTC().AddDate(0, 0, daysFromNow)
}

// Ensure imports are used
var (
	_ *models.Show
	_ *services.ShowService
)
