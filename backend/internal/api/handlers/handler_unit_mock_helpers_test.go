package handlers

import (
	"time"

	"github.com/markbates/goth"

	"psychic-homily-backend/internal/models"
	"psychic-homily-backend/internal/services"
)

// ============================================================================
// Mock: SavedShowServiceInterface
// ============================================================================

type mockSavedShowService struct {
	saveShowFn       func(userID, showID uint) error
	unsaveShowFn     func(userID, showID uint) error
	getUserSavedFn   func(userID uint, limit, offset int) ([]*services.SavedShowResponse, int64, error)
	isShowSavedFn    func(userID, showID uint) (bool, error)
	getSavedShowIDFn func(userID uint, showIDs []uint) (map[uint]bool, error)
}

func (m *mockSavedShowService) SaveShow(userID, showID uint) error {
	return m.saveShowFn(userID, showID)
}
func (m *mockSavedShowService) UnsaveShow(userID, showID uint) error {
	return m.unsaveShowFn(userID, showID)
}
func (m *mockSavedShowService) GetUserSavedShows(userID uint, limit, offset int) ([]*services.SavedShowResponse, int64, error) {
	return m.getUserSavedFn(userID, limit, offset)
}
func (m *mockSavedShowService) IsShowSaved(userID, showID uint) (bool, error) {
	return m.isShowSavedFn(userID, showID)
}
func (m *mockSavedShowService) GetSavedShowIDs(userID uint, showIDs []uint) (map[uint]bool, error) {
	return m.getSavedShowIDFn(userID, showIDs)
}

// ============================================================================
// Mock: FavoriteVenueServiceInterface
// ============================================================================

type mockFavoriteVenueService struct {
	favoriteVenueFn    func(userID, venueID uint) error
	unfavoriteVenueFn  func(userID, venueID uint) error
	getUserFavoritesFn func(userID uint, limit, offset int) ([]*services.FavoriteVenueResponse, int64, error)
	isVenueFavoritedFn func(userID, venueID uint) (bool, error)
	getUpcomingShowsFn func(userID uint, timezone string, limit, offset int) ([]*services.FavoriteVenueShowResponse, int64, error)
	getFavoriteIDsFn   func(userID uint, venueIDs []uint) (map[uint]bool, error)
}

func (m *mockFavoriteVenueService) FavoriteVenue(userID, venueID uint) error {
	return m.favoriteVenueFn(userID, venueID)
}
func (m *mockFavoriteVenueService) UnfavoriteVenue(userID, venueID uint) error {
	return m.unfavoriteVenueFn(userID, venueID)
}
func (m *mockFavoriteVenueService) GetUserFavoriteVenues(userID uint, limit, offset int) ([]*services.FavoriteVenueResponse, int64, error) {
	return m.getUserFavoritesFn(userID, limit, offset)
}
func (m *mockFavoriteVenueService) IsVenueFavorited(userID, venueID uint) (bool, error) {
	return m.isVenueFavoritedFn(userID, venueID)
}
func (m *mockFavoriteVenueService) GetUpcomingShowsFromFavorites(userID uint, timezone string, limit, offset int) ([]*services.FavoriteVenueShowResponse, int64, error) {
	return m.getUpcomingShowsFn(userID, timezone, limit, offset)
}
func (m *mockFavoriteVenueService) GetFavoriteVenueIDs(userID uint, venueIDs []uint) (map[uint]bool, error) {
	return m.getFavoriteIDsFn(userID, venueIDs)
}

// ============================================================================
// Mock: AuditLogServiceInterface
// ============================================================================

type mockAuditLogService struct {
	logActionFn    func(actorID uint, action string, entityType string, entityID uint, metadata map[string]interface{})
	getAuditLogsFn func(limit, offset int, filters services.AuditLogFilters) ([]*services.AuditLogResponse, int64, error)
}

func (m *mockAuditLogService) LogAction(actorID uint, action string, entityType string, entityID uint, metadata map[string]interface{}) {
	if m.logActionFn != nil {
		m.logActionFn(actorID, action, entityType, entityID, metadata)
	}
}
func (m *mockAuditLogService) GetAuditLogs(limit, offset int, filters services.AuditLogFilters) ([]*services.AuditLogResponse, int64, error) {
	return m.getAuditLogsFn(limit, offset, filters)
}

// ============================================================================
// Mock: ShowReportServiceInterface
// ============================================================================

type mockShowReportService struct {
	createReportFn        func(userID, showID uint, reportType string, details *string) (*services.ShowReportResponse, error)
	getUserReportFn       func(userID, showID uint) (*services.ShowReportResponse, error)
	getPendingReportsFn   func(limit, offset int) ([]*services.ShowReportResponse, int64, error)
	dismissReportFn       func(reportID, adminID uint, notes *string) (*services.ShowReportResponse, error)
	resolveReportFn       func(reportID, adminID uint, notes *string) (*services.ShowReportResponse, error)
	resolveWithFlagFn     func(reportID, adminID uint, notes *string, setShowFlag bool) (*services.ShowReportResponse, error)
	getReportByIDFn       func(reportID uint) (*models.ShowReport, error)
}

func (m *mockShowReportService) CreateReport(userID, showID uint, reportType string, details *string) (*services.ShowReportResponse, error) {
	return m.createReportFn(userID, showID, reportType, details)
}
func (m *mockShowReportService) GetUserReportForShow(userID, showID uint) (*services.ShowReportResponse, error) {
	return m.getUserReportFn(userID, showID)
}
func (m *mockShowReportService) GetPendingReports(limit, offset int) ([]*services.ShowReportResponse, int64, error) {
	return m.getPendingReportsFn(limit, offset)
}
func (m *mockShowReportService) DismissReport(reportID, adminID uint, notes *string) (*services.ShowReportResponse, error) {
	return m.dismissReportFn(reportID, adminID, notes)
}
func (m *mockShowReportService) ResolveReport(reportID, adminID uint, notes *string) (*services.ShowReportResponse, error) {
	return m.resolveReportFn(reportID, adminID, notes)
}
func (m *mockShowReportService) ResolveReportWithFlag(reportID, adminID uint, notes *string, setShowFlag bool) (*services.ShowReportResponse, error) {
	return m.resolveWithFlagFn(reportID, adminID, notes, setShowFlag)
}
func (m *mockShowReportService) GetReportByID(reportID uint) (*models.ShowReport, error) {
	return m.getReportByIDFn(reportID)
}

// ============================================================================
// Mock: ArtistReportServiceInterface
// ============================================================================

type mockArtistReportService struct {
	createReportFn      func(userID, artistID uint, reportType string, details *string) (*services.ArtistReportResponse, error)
	getUserReportFn     func(userID, artistID uint) (*services.ArtistReportResponse, error)
	getPendingReportsFn func(limit, offset int) ([]*services.ArtistReportResponse, int64, error)
	dismissReportFn     func(reportID, adminID uint, notes *string) (*services.ArtistReportResponse, error)
	resolveReportFn     func(reportID, adminID uint, notes *string) (*services.ArtistReportResponse, error)
	getReportByIDFn     func(reportID uint) (*models.ArtistReport, error)
}

func (m *mockArtistReportService) CreateReport(userID, artistID uint, reportType string, details *string) (*services.ArtistReportResponse, error) {
	return m.createReportFn(userID, artistID, reportType, details)
}
func (m *mockArtistReportService) GetUserReportForArtist(userID, artistID uint) (*services.ArtistReportResponse, error) {
	return m.getUserReportFn(userID, artistID)
}
func (m *mockArtistReportService) GetPendingReports(limit, offset int) ([]*services.ArtistReportResponse, int64, error) {
	return m.getPendingReportsFn(limit, offset)
}
func (m *mockArtistReportService) DismissReport(reportID, adminID uint, notes *string) (*services.ArtistReportResponse, error) {
	return m.dismissReportFn(reportID, adminID, notes)
}
func (m *mockArtistReportService) ResolveReport(reportID, adminID uint, notes *string) (*services.ArtistReportResponse, error) {
	return m.resolveReportFn(reportID, adminID, notes)
}
func (m *mockArtistReportService) GetReportByID(reportID uint) (*models.ArtistReport, error) {
	return m.getReportByIDFn(reportID)
}

// ============================================================================
// Mock: DiscordServiceInterface (all no-ops)
// ============================================================================

type mockDiscordService struct{}

func (m *mockDiscordService) IsConfigured() bool                     { return false }
func (m *mockDiscordService) NotifyNewUser(_ *models.User)           {}
func (m *mockDiscordService) NotifyNewShow(_ *services.ShowResponse, _ string) {}
func (m *mockDiscordService) NotifyShowStatusChange(_ string, _ uint, _, _, _ string) {}
func (m *mockDiscordService) NotifyShowApproved(_ *services.ShowResponse)              {}
func (m *mockDiscordService) NotifyShowRejected(_ *services.ShowResponse, _ string)    {}
func (m *mockDiscordService) NotifyShowReport(_ *models.ShowReport, _ string)          {}
func (m *mockDiscordService) NotifyArtistReport(_ *models.ArtistReport, _ string)      {}
func (m *mockDiscordService) NotifyNewVenue(_ uint, _, _, _ string, _ *string, _ string) {}
func (m *mockDiscordService) NotifyPendingVenueEdit(_, _ uint, _, _ string)            {}

// ============================================================================
// Mock: UserServiceInterface (all no-op stubs)
// ============================================================================

type mockUserService struct{}

func (m *mockUserService) ListUsers(_, _ int, _ services.AdminUserFilters) ([]*services.AdminUserResponse, int64, error) {
	return nil, 0, nil
}
func (m *mockUserService) FindOrCreateUser(_ goth.User, _ string) (*models.User, error) {
	return nil, nil
}
func (m *mockUserService) FindOrCreateUserWithConsent(_ goth.User, _ string, _ *services.OAuthSignupConsent) (*models.User, error) {
	return nil, nil
}
func (m *mockUserService) AuthenticateUserWithPassword(_, _ string) (*models.User, error) {
	return nil, nil
}
func (m *mockUserService) CreateUserWithPassword(_, _, _, _ string) (*models.User, error) {
	return nil, nil
}
func (m *mockUserService) CreateUserWithPasswordWithLegal(_, _, _, _ string, _ services.LegalAcceptance) (*models.User, error) {
	return nil, nil
}
func (m *mockUserService) GetUserByID(_ uint) (*models.User, error)      { return nil, nil }
func (m *mockUserService) GetUserByEmail(_ string) (*models.User, error) { return nil, nil }
func (m *mockUserService) GetUserByUsername(_ string) (*models.User, error) {
	return nil, nil
}
func (m *mockUserService) UpdateUser(_ uint, _ map[string]any) (*models.User, error) {
	return nil, nil
}
func (m *mockUserService) HashPassword(_ string) (string, error)          { return "", nil }
func (m *mockUserService) VerifyPassword(_, _ string) error               { return nil }
func (m *mockUserService) IsAccountLocked(_ *models.User) bool            { return false }
func (m *mockUserService) GetLockTimeRemaining(_ *models.User) time.Duration { return 0 }
func (m *mockUserService) IncrementFailedAttempts(_ uint) error           { return nil }
func (m *mockUserService) ResetFailedAttempts(_ uint) error               { return nil }
func (m *mockUserService) UpdatePassword(_ uint, _, _ string) error       { return nil }
func (m *mockUserService) SetEmailVerified(_ uint, _ bool) error          { return nil }
func (m *mockUserService) GetDeletionSummary(_ uint) (*services.DeletionSummary, error) {
	return nil, nil
}
func (m *mockUserService) SoftDeleteAccount(_ uint, _ *string) error { return nil }
func (m *mockUserService) CreateUserWithoutPassword(_ string) (*models.User, error) {
	return nil, nil
}
func (m *mockUserService) ExportUserData(_ uint) (*services.UserDataExport, error) {
	return nil, nil
}
func (m *mockUserService) ExportUserDataJSON(_ uint) ([]byte, error) { return nil, nil }
func (m *mockUserService) GetOAuthAccounts(_ uint) ([]models.OAuthAccount, error) {
	return nil, nil
}
func (m *mockUserService) GetUserByEmailIncludingDeleted(_ string) (*models.User, error) {
	return nil, nil
}
func (m *mockUserService) IsAccountRecoverable(_ *models.User) bool { return false }
func (m *mockUserService) GetDaysUntilPermanentDeletion(_ *models.User) int {
	return 0
}
func (m *mockUserService) RestoreAccount(_ uint) error                 { return nil }
func (m *mockUserService) GetExpiredDeletedAccounts() ([]models.User, error) { return nil, nil }
func (m *mockUserService) PermanentlyDeleteUser(_ uint) error          { return nil }
func (m *mockUserService) CanUnlinkOAuthAccount(_ uint, _ string) (bool, string, error) {
	return false, "", nil
}
func (m *mockUserService) UnlinkOAuthAccount(_ uint, _ string) error { return nil }

// ============================================================================
// Compile-time interface satisfaction checks
// ============================================================================

var _ services.SavedShowServiceInterface = (*mockSavedShowService)(nil)
var _ services.FavoriteVenueServiceInterface = (*mockFavoriteVenueService)(nil)
var _ services.AuditLogServiceInterface = (*mockAuditLogService)(nil)
var _ services.ShowReportServiceInterface = (*mockShowReportService)(nil)
var _ services.ArtistReportServiceInterface = (*mockArtistReportService)(nil)
var _ services.DiscordServiceInterface = (*mockDiscordService)(nil)
var _ services.UserServiceInterface = (*mockUserService)(nil)

