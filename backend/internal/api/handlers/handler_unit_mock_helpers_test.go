package handlers

import (
	"net/http"
	"time"

	"github.com/markbates/goth"
	"gorm.io/gorm"

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
	if m.saveShowFn != nil {
		return m.saveShowFn(userID, showID)
	}
	return nil
}
func (m *mockSavedShowService) UnsaveShow(userID, showID uint) error {
	if m.unsaveShowFn != nil {
		return m.unsaveShowFn(userID, showID)
	}
	return nil
}
func (m *mockSavedShowService) GetUserSavedShows(userID uint, limit, offset int) ([]*services.SavedShowResponse, int64, error) {
	if m.getUserSavedFn != nil {
		return m.getUserSavedFn(userID, limit, offset)
	}
	return nil, 0, nil
}
func (m *mockSavedShowService) IsShowSaved(userID, showID uint) (bool, error) {
	if m.isShowSavedFn != nil {
		return m.isShowSavedFn(userID, showID)
	}
	return false, nil
}
func (m *mockSavedShowService) GetSavedShowIDs(userID uint, showIDs []uint) (map[uint]bool, error) {
	if m.getSavedShowIDFn != nil {
		return m.getSavedShowIDFn(userID, showIDs)
	}
	return nil, nil
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
	if m.favoriteVenueFn != nil {
		return m.favoriteVenueFn(userID, venueID)
	}
	return nil
}
func (m *mockFavoriteVenueService) UnfavoriteVenue(userID, venueID uint) error {
	if m.unfavoriteVenueFn != nil {
		return m.unfavoriteVenueFn(userID, venueID)
	}
	return nil
}
func (m *mockFavoriteVenueService) GetUserFavoriteVenues(userID uint, limit, offset int) ([]*services.FavoriteVenueResponse, int64, error) {
	if m.getUserFavoritesFn != nil {
		return m.getUserFavoritesFn(userID, limit, offset)
	}
	return nil, 0, nil
}
func (m *mockFavoriteVenueService) IsVenueFavorited(userID, venueID uint) (bool, error) {
	if m.isVenueFavoritedFn != nil {
		return m.isVenueFavoritedFn(userID, venueID)
	}
	return false, nil
}
func (m *mockFavoriteVenueService) GetUpcomingShowsFromFavorites(userID uint, timezone string, limit, offset int) ([]*services.FavoriteVenueShowResponse, int64, error) {
	if m.getUpcomingShowsFn != nil {
		return m.getUpcomingShowsFn(userID, timezone, limit, offset)
	}
	return nil, 0, nil
}
func (m *mockFavoriteVenueService) GetFavoriteVenueIDs(userID uint, venueIDs []uint) (map[uint]bool, error) {
	if m.getFavoriteIDsFn != nil {
		return m.getFavoriteIDsFn(userID, venueIDs)
	}
	return nil, nil
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
	if m.getAuditLogsFn != nil {
		return m.getAuditLogsFn(limit, offset, filters)
	}
	return nil, 0, nil
}

// ============================================================================
// Mock: ShowReportServiceInterface
// ============================================================================

type mockShowReportService struct {
	createReportFn    func(userID, showID uint, reportType string, details *string) (*services.ShowReportResponse, error)
	getUserReportFn   func(userID, showID uint) (*services.ShowReportResponse, error)
	getPendingReportsFn func(limit, offset int) ([]*services.ShowReportResponse, int64, error)
	dismissReportFn   func(reportID, adminID uint, notes *string) (*services.ShowReportResponse, error)
	resolveReportFn   func(reportID, adminID uint, notes *string) (*services.ShowReportResponse, error)
	resolveWithFlagFn func(reportID, adminID uint, notes *string, setShowFlag bool) (*services.ShowReportResponse, error)
	getReportByIDFn   func(reportID uint) (*models.ShowReport, error)
}

func (m *mockShowReportService) CreateReport(userID, showID uint, reportType string, details *string) (*services.ShowReportResponse, error) {
	if m.createReportFn != nil {
		return m.createReportFn(userID, showID, reportType, details)
	}
	return nil, nil
}
func (m *mockShowReportService) GetUserReportForShow(userID, showID uint) (*services.ShowReportResponse, error) {
	if m.getUserReportFn != nil {
		return m.getUserReportFn(userID, showID)
	}
	return nil, nil
}
func (m *mockShowReportService) GetPendingReports(limit, offset int) ([]*services.ShowReportResponse, int64, error) {
	if m.getPendingReportsFn != nil {
		return m.getPendingReportsFn(limit, offset)
	}
	return nil, 0, nil
}
func (m *mockShowReportService) DismissReport(reportID, adminID uint, notes *string) (*services.ShowReportResponse, error) {
	if m.dismissReportFn != nil {
		return m.dismissReportFn(reportID, adminID, notes)
	}
	return nil, nil
}
func (m *mockShowReportService) ResolveReport(reportID, adminID uint, notes *string) (*services.ShowReportResponse, error) {
	if m.resolveReportFn != nil {
		return m.resolveReportFn(reportID, adminID, notes)
	}
	return nil, nil
}
func (m *mockShowReportService) ResolveReportWithFlag(reportID, adminID uint, notes *string, setShowFlag bool) (*services.ShowReportResponse, error) {
	if m.resolveWithFlagFn != nil {
		return m.resolveWithFlagFn(reportID, adminID, notes, setShowFlag)
	}
	return nil, nil
}
func (m *mockShowReportService) GetReportByID(reportID uint) (*models.ShowReport, error) {
	if m.getReportByIDFn != nil {
		return m.getReportByIDFn(reportID)
	}
	return nil, nil
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
	if m.createReportFn != nil {
		return m.createReportFn(userID, artistID, reportType, details)
	}
	return nil, nil
}
func (m *mockArtistReportService) GetUserReportForArtist(userID, artistID uint) (*services.ArtistReportResponse, error) {
	if m.getUserReportFn != nil {
		return m.getUserReportFn(userID, artistID)
	}
	return nil, nil
}
func (m *mockArtistReportService) GetPendingReports(limit, offset int) ([]*services.ArtistReportResponse, int64, error) {
	if m.getPendingReportsFn != nil {
		return m.getPendingReportsFn(limit, offset)
	}
	return nil, 0, nil
}
func (m *mockArtistReportService) DismissReport(reportID, adminID uint, notes *string) (*services.ArtistReportResponse, error) {
	if m.dismissReportFn != nil {
		return m.dismissReportFn(reportID, adminID, notes)
	}
	return nil, nil
}
func (m *mockArtistReportService) ResolveReport(reportID, adminID uint, notes *string) (*services.ArtistReportResponse, error) {
	if m.resolveReportFn != nil {
		return m.resolveReportFn(reportID, adminID, notes)
	}
	return nil, nil
}
func (m *mockArtistReportService) GetReportByID(reportID uint) (*models.ArtistReport, error) {
	if m.getReportByIDFn != nil {
		return m.getReportByIDFn(reportID)
	}
	return nil, nil
}

// ============================================================================
// Mock: DiscordServiceInterface (upgraded from no-op to func-field, nil-safe)
// ============================================================================

type mockDiscordService struct {
	isConfiguredFn             func() bool
	notifyNewUserFn            func(user *models.User)
	notifyNewShowFn            func(show *services.ShowResponse, submitterEmail string)
	notifyShowStatusChangeFn   func(showTitle string, showID uint, oldStatus, newStatus, actorEmail string)
	notifyShowApprovedFn       func(show *services.ShowResponse)
	notifyShowRejectedFn       func(show *services.ShowResponse, reason string)
	notifyShowReportFn         func(report *models.ShowReport, reporterEmail string)
	notifyArtistReportFn       func(report *models.ArtistReport, reporterEmail string)
	notifyNewVenueFn           func(venueID uint, venueName, city, state string, address *string, submitterEmail string)
	notifyPendingVenueEditFn   func(editID, venueID uint, venueName, submitterEmail string)
}

func (m *mockDiscordService) IsConfigured() bool {
	if m.isConfiguredFn != nil {
		return m.isConfiguredFn()
	}
	return false
}
func (m *mockDiscordService) NotifyNewUser(user *models.User) {
	if m.notifyNewUserFn != nil {
		m.notifyNewUserFn(user)
	}
}
func (m *mockDiscordService) NotifyNewShow(show *services.ShowResponse, submitterEmail string) {
	if m.notifyNewShowFn != nil {
		m.notifyNewShowFn(show, submitterEmail)
	}
}
func (m *mockDiscordService) NotifyShowStatusChange(showTitle string, showID uint, oldStatus, newStatus, actorEmail string) {
	if m.notifyShowStatusChangeFn != nil {
		m.notifyShowStatusChangeFn(showTitle, showID, oldStatus, newStatus, actorEmail)
	}
}
func (m *mockDiscordService) NotifyShowApproved(show *services.ShowResponse) {
	if m.notifyShowApprovedFn != nil {
		m.notifyShowApprovedFn(show)
	}
}
func (m *mockDiscordService) NotifyShowRejected(show *services.ShowResponse, reason string) {
	if m.notifyShowRejectedFn != nil {
		m.notifyShowRejectedFn(show, reason)
	}
}
func (m *mockDiscordService) NotifyShowReport(report *models.ShowReport, reporterEmail string) {
	if m.notifyShowReportFn != nil {
		m.notifyShowReportFn(report, reporterEmail)
	}
}
func (m *mockDiscordService) NotifyArtistReport(report *models.ArtistReport, reporterEmail string) {
	if m.notifyArtistReportFn != nil {
		m.notifyArtistReportFn(report, reporterEmail)
	}
}
func (m *mockDiscordService) NotifyNewVenue(venueID uint, venueName, city, state string, address *string, submitterEmail string) {
	if m.notifyNewVenueFn != nil {
		m.notifyNewVenueFn(venueID, venueName, city, state, address, submitterEmail)
	}
}
func (m *mockDiscordService) NotifyPendingVenueEdit(editID, venueID uint, venueName, submitterEmail string) {
	if m.notifyPendingVenueEditFn != nil {
		m.notifyPendingVenueEditFn(editID, venueID, venueName, submitterEmail)
	}
}

// ============================================================================
// Mock: UserServiceInterface (upgraded from no-op to func-field, nil-safe)
// ============================================================================

type mockUserService struct {
	listUsersFn                     func(limit, offset int, filters services.AdminUserFilters) ([]*services.AdminUserResponse, int64, error)
	findOrCreateUserFn              func(gothUser goth.User, provider string) (*models.User, error)
	findOrCreateUserWithConsentFn   func(gothUser goth.User, provider string, consent *services.OAuthSignupConsent) (*models.User, error)
	authenticateUserWithPasswordFn  func(email, password string) (*models.User, error)
	createUserWithPasswordFn        func(email, password, firstName, lastName string) (*models.User, error)
	createUserWithPasswordWithLegalFn func(email, password, firstName, lastName string, acceptance services.LegalAcceptance) (*models.User, error)
	getUserByIDFn                   func(userID uint) (*models.User, error)
	getUserByEmailFn                func(email string) (*models.User, error)
	getUserByUsernameFn             func(username string) (*models.User, error)
	updateUserFn                    func(userID uint, updates map[string]any) (*models.User, error)
	hashPasswordFn                  func(password string) (string, error)
	verifyPasswordFn                func(hashedPassword, password string) error
	isAccountLockedFn               func(user *models.User) bool
	getLockTimeRemainingFn          func(user *models.User) time.Duration
	incrementFailedAttemptsFn       func(userID uint) error
	resetFailedAttemptsFn           func(userID uint) error
	updatePasswordFn                func(userID uint, currentPassword, newPassword string) error
	setEmailVerifiedFn              func(userID uint, verified bool) error
	getDeletionSummaryFn            func(userID uint) (*services.DeletionSummary, error)
	softDeleteAccountFn             func(userID uint, reason *string) error
	createUserWithoutPasswordFn     func(email string) (*models.User, error)
	exportUserDataFn                func(userID uint) (*services.UserDataExport, error)
	exportUserDataJSONFn            func(userID uint) ([]byte, error)
	getOAuthAccountsFn              func(userID uint) ([]models.OAuthAccount, error)
	getUserByEmailIncludingDeletedFn func(email string) (*models.User, error)
	isAccountRecoverableFn          func(user *models.User) bool
	getDaysUntilPermanentDeletionFn func(user *models.User) int
	restoreAccountFn                func(userID uint) error
	getExpiredDeletedAccountsFn     func() ([]models.User, error)
	permanentlyDeleteUserFn         func(userID uint) error
	canUnlinkOAuthAccountFn         func(userID uint, provider string) (bool, string, error)
	unlinkOAuthAccountFn            func(userID uint, provider string) error
}

func (m *mockUserService) ListUsers(limit, offset int, filters services.AdminUserFilters) ([]*services.AdminUserResponse, int64, error) {
	if m.listUsersFn != nil {
		return m.listUsersFn(limit, offset, filters)
	}
	return nil, 0, nil
}
func (m *mockUserService) FindOrCreateUser(gothUser goth.User, provider string) (*models.User, error) {
	if m.findOrCreateUserFn != nil {
		return m.findOrCreateUserFn(gothUser, provider)
	}
	return nil, nil
}
func (m *mockUserService) FindOrCreateUserWithConsent(gothUser goth.User, provider string, consent *services.OAuthSignupConsent) (*models.User, error) {
	if m.findOrCreateUserWithConsentFn != nil {
		return m.findOrCreateUserWithConsentFn(gothUser, provider, consent)
	}
	return nil, nil
}
func (m *mockUserService) AuthenticateUserWithPassword(email, password string) (*models.User, error) {
	if m.authenticateUserWithPasswordFn != nil {
		return m.authenticateUserWithPasswordFn(email, password)
	}
	return nil, nil
}
func (m *mockUserService) CreateUserWithPassword(email, password, firstName, lastName string) (*models.User, error) {
	if m.createUserWithPasswordFn != nil {
		return m.createUserWithPasswordFn(email, password, firstName, lastName)
	}
	return nil, nil
}
func (m *mockUserService) CreateUserWithPasswordWithLegal(email, password, firstName, lastName string, acceptance services.LegalAcceptance) (*models.User, error) {
	if m.createUserWithPasswordWithLegalFn != nil {
		return m.createUserWithPasswordWithLegalFn(email, password, firstName, lastName, acceptance)
	}
	return nil, nil
}
func (m *mockUserService) GetUserByID(userID uint) (*models.User, error) {
	if m.getUserByIDFn != nil {
		return m.getUserByIDFn(userID)
	}
	return nil, nil
}
func (m *mockUserService) GetUserByEmail(email string) (*models.User, error) {
	if m.getUserByEmailFn != nil {
		return m.getUserByEmailFn(email)
	}
	return nil, nil
}
func (m *mockUserService) GetUserByUsername(username string) (*models.User, error) {
	if m.getUserByUsernameFn != nil {
		return m.getUserByUsernameFn(username)
	}
	return nil, nil
}
func (m *mockUserService) UpdateUser(userID uint, updates map[string]any) (*models.User, error) {
	if m.updateUserFn != nil {
		return m.updateUserFn(userID, updates)
	}
	return nil, nil
}
func (m *mockUserService) HashPassword(password string) (string, error) {
	if m.hashPasswordFn != nil {
		return m.hashPasswordFn(password)
	}
	return "", nil
}
func (m *mockUserService) VerifyPassword(hashedPassword, password string) error {
	if m.verifyPasswordFn != nil {
		return m.verifyPasswordFn(hashedPassword, password)
	}
	return nil
}
func (m *mockUserService) IsAccountLocked(user *models.User) bool {
	if m.isAccountLockedFn != nil {
		return m.isAccountLockedFn(user)
	}
	return false
}
func (m *mockUserService) GetLockTimeRemaining(user *models.User) time.Duration {
	if m.getLockTimeRemainingFn != nil {
		return m.getLockTimeRemainingFn(user)
	}
	return 0
}
func (m *mockUserService) IncrementFailedAttempts(userID uint) error {
	if m.incrementFailedAttemptsFn != nil {
		return m.incrementFailedAttemptsFn(userID)
	}
	return nil
}
func (m *mockUserService) ResetFailedAttempts(userID uint) error {
	if m.resetFailedAttemptsFn != nil {
		return m.resetFailedAttemptsFn(userID)
	}
	return nil
}
func (m *mockUserService) UpdatePassword(userID uint, currentPassword, newPassword string) error {
	if m.updatePasswordFn != nil {
		return m.updatePasswordFn(userID, currentPassword, newPassword)
	}
	return nil
}
func (m *mockUserService) SetEmailVerified(userID uint, verified bool) error {
	if m.setEmailVerifiedFn != nil {
		return m.setEmailVerifiedFn(userID, verified)
	}
	return nil
}
func (m *mockUserService) GetDeletionSummary(userID uint) (*services.DeletionSummary, error) {
	if m.getDeletionSummaryFn != nil {
		return m.getDeletionSummaryFn(userID)
	}
	return nil, nil
}
func (m *mockUserService) SoftDeleteAccount(userID uint, reason *string) error {
	if m.softDeleteAccountFn != nil {
		return m.softDeleteAccountFn(userID, reason)
	}
	return nil
}
func (m *mockUserService) CreateUserWithoutPassword(email string) (*models.User, error) {
	if m.createUserWithoutPasswordFn != nil {
		return m.createUserWithoutPasswordFn(email)
	}
	return nil, nil
}
func (m *mockUserService) ExportUserData(userID uint) (*services.UserDataExport, error) {
	if m.exportUserDataFn != nil {
		return m.exportUserDataFn(userID)
	}
	return nil, nil
}
func (m *mockUserService) ExportUserDataJSON(userID uint) ([]byte, error) {
	if m.exportUserDataJSONFn != nil {
		return m.exportUserDataJSONFn(userID)
	}
	return nil, nil
}
func (m *mockUserService) GetOAuthAccounts(userID uint) ([]models.OAuthAccount, error) {
	if m.getOAuthAccountsFn != nil {
		return m.getOAuthAccountsFn(userID)
	}
	return nil, nil
}
func (m *mockUserService) GetUserByEmailIncludingDeleted(email string) (*models.User, error) {
	if m.getUserByEmailIncludingDeletedFn != nil {
		return m.getUserByEmailIncludingDeletedFn(email)
	}
	return nil, nil
}
func (m *mockUserService) IsAccountRecoverable(user *models.User) bool {
	if m.isAccountRecoverableFn != nil {
		return m.isAccountRecoverableFn(user)
	}
	return false
}
func (m *mockUserService) GetDaysUntilPermanentDeletion(user *models.User) int {
	if m.getDaysUntilPermanentDeletionFn != nil {
		return m.getDaysUntilPermanentDeletionFn(user)
	}
	return 0
}
func (m *mockUserService) RestoreAccount(userID uint) error {
	if m.restoreAccountFn != nil {
		return m.restoreAccountFn(userID)
	}
	return nil
}
func (m *mockUserService) GetExpiredDeletedAccounts() ([]models.User, error) {
	if m.getExpiredDeletedAccountsFn != nil {
		return m.getExpiredDeletedAccountsFn()
	}
	return nil, nil
}
func (m *mockUserService) PermanentlyDeleteUser(userID uint) error {
	if m.permanentlyDeleteUserFn != nil {
		return m.permanentlyDeleteUserFn(userID)
	}
	return nil
}
func (m *mockUserService) CanUnlinkOAuthAccount(userID uint, provider string) (bool, string, error) {
	if m.canUnlinkOAuthAccountFn != nil {
		return m.canUnlinkOAuthAccountFn(userID, provider)
	}
	return false, "", nil
}
func (m *mockUserService) UnlinkOAuthAccount(userID uint, provider string) error {
	if m.unlinkOAuthAccountFn != nil {
		return m.unlinkOAuthAccountFn(userID, provider)
	}
	return nil
}
func (m *mockUserService) GetFavoriteCities(userID uint) ([]models.FavoriteCity, error) {
	return []models.FavoriteCity{}, nil
}
func (m *mockUserService) SetFavoriteCities(userID uint, cities []models.FavoriteCity) error {
	return nil
}

// ============================================================================
// Mock: ShowServiceInterface
// ============================================================================

type mockShowService struct {
	createShowFn            func(req *services.CreateShowRequest) (*services.ShowResponse, error)
	getShowFn               func(showID uint) (*services.ShowResponse, error)
	getShowBySlugFn         func(slug string) (*services.ShowResponse, error)
	getShowsFn              func(filters map[string]interface{}) ([]*services.ShowResponse, error)
	getUserSubmissionsFn    func(userID uint, limit, offset int) ([]services.ShowResponse, int, error)
	updateShowFn            func(showID uint, updates map[string]interface{}) (*services.ShowResponse, error)
	updateShowWithRelationsFn func(showID uint, updates map[string]interface{}, venues []services.CreateShowVenue, artists []services.CreateShowArtist, isAdmin bool) (*services.ShowResponse, []services.OrphanedArtist, error)
	getUpcomingShowsFn      func(timezone string, cursor string, limit int, includeNonApproved bool, filters *services.UpcomingShowsFilter) ([]*services.ShowResponse, *string, error)
	getShowCitiesFn         func(timezone string) ([]services.ShowCityResponse, error)
	deleteShowFn            func(showID uint) error
	getPendingShowsFn       func(limit, offset int) ([]*services.ShowResponse, int64, error)
	getRejectedShowsFn      func(limit, offset int, search string) ([]*services.ShowResponse, int64, error)
	approveShowFn           func(showID uint, verifyVenues bool) (*services.ShowResponse, error)
	rejectShowFn            func(showID uint, reason string) (*services.ShowResponse, error)
	unpublishShowFn         func(showID uint, userID uint, isAdmin bool) (*services.ShowResponse, error)
	makePrivateShowFn       func(showID uint, userID uint, isAdmin bool) (*services.ShowResponse, error)
	publishShowFn           func(showID uint, userID uint, isAdmin bool) (*services.ShowResponse, error)
	exportShowToMarkdownFn  func(showID uint) ([]byte, string, error)
	parseShowMarkdownFn     func(content []byte) (*services.ParsedShowImport, error)
	previewShowImportFn     func(content []byte) (*services.ImportPreviewResponse, error)
	confirmShowImportFn     func(content []byte, isAdmin bool) (*services.ShowResponse, error)
	getAdminShowsFn         func(limit, offset int, filters services.AdminShowFilters) ([]*services.ShowResponse, int64, error)
	setShowSoldOutFn        func(showID uint, isSoldOut bool) (*services.ShowResponse, error)
	setShowCancelledFn      func(showID uint, isCancelled bool) (*services.ShowResponse, error)
}

func (m *mockShowService) CreateShow(req *services.CreateShowRequest) (*services.ShowResponse, error) {
	if m.createShowFn != nil {
		return m.createShowFn(req)
	}
	return nil, nil
}
func (m *mockShowService) GetShow(showID uint) (*services.ShowResponse, error) {
	if m.getShowFn != nil {
		return m.getShowFn(showID)
	}
	return nil, nil
}
func (m *mockShowService) GetShowBySlug(slug string) (*services.ShowResponse, error) {
	if m.getShowBySlugFn != nil {
		return m.getShowBySlugFn(slug)
	}
	return nil, nil
}
func (m *mockShowService) GetShows(filters map[string]interface{}) ([]*services.ShowResponse, error) {
	if m.getShowsFn != nil {
		return m.getShowsFn(filters)
	}
	return nil, nil
}
func (m *mockShowService) GetUserSubmissions(userID uint, limit, offset int) ([]services.ShowResponse, int, error) {
	if m.getUserSubmissionsFn != nil {
		return m.getUserSubmissionsFn(userID, limit, offset)
	}
	return nil, 0, nil
}
func (m *mockShowService) UpdateShow(showID uint, updates map[string]interface{}) (*services.ShowResponse, error) {
	if m.updateShowFn != nil {
		return m.updateShowFn(showID, updates)
	}
	return nil, nil
}
func (m *mockShowService) UpdateShowWithRelations(showID uint, updates map[string]interface{}, venues []services.CreateShowVenue, artists []services.CreateShowArtist, isAdmin bool) (*services.ShowResponse, []services.OrphanedArtist, error) {
	if m.updateShowWithRelationsFn != nil {
		return m.updateShowWithRelationsFn(showID, updates, venues, artists, isAdmin)
	}
	return nil, nil, nil
}
func (m *mockShowService) GetUpcomingShows(timezone string, cursor string, limit int, includeNonApproved bool, filters *services.UpcomingShowsFilter) ([]*services.ShowResponse, *string, error) {
	if m.getUpcomingShowsFn != nil {
		return m.getUpcomingShowsFn(timezone, cursor, limit, includeNonApproved, filters)
	}
	return nil, nil, nil
}
func (m *mockShowService) GetShowCities(timezone string) ([]services.ShowCityResponse, error) {
	if m.getShowCitiesFn != nil {
		return m.getShowCitiesFn(timezone)
	}
	return nil, nil
}
func (m *mockShowService) DeleteShow(showID uint) error {
	if m.deleteShowFn != nil {
		return m.deleteShowFn(showID)
	}
	return nil
}
func (m *mockShowService) GetPendingShows(limit, offset int) ([]*services.ShowResponse, int64, error) {
	if m.getPendingShowsFn != nil {
		return m.getPendingShowsFn(limit, offset)
	}
	return nil, 0, nil
}
func (m *mockShowService) GetRejectedShows(limit, offset int, search string) ([]*services.ShowResponse, int64, error) {
	if m.getRejectedShowsFn != nil {
		return m.getRejectedShowsFn(limit, offset, search)
	}
	return nil, 0, nil
}
func (m *mockShowService) ApproveShow(showID uint, verifyVenues bool) (*services.ShowResponse, error) {
	if m.approveShowFn != nil {
		return m.approveShowFn(showID, verifyVenues)
	}
	return nil, nil
}
func (m *mockShowService) RejectShow(showID uint, reason string) (*services.ShowResponse, error) {
	if m.rejectShowFn != nil {
		return m.rejectShowFn(showID, reason)
	}
	return nil, nil
}
func (m *mockShowService) UnpublishShow(showID uint, userID uint, isAdmin bool) (*services.ShowResponse, error) {
	if m.unpublishShowFn != nil {
		return m.unpublishShowFn(showID, userID, isAdmin)
	}
	return nil, nil
}
func (m *mockShowService) MakePrivateShow(showID uint, userID uint, isAdmin bool) (*services.ShowResponse, error) {
	if m.makePrivateShowFn != nil {
		return m.makePrivateShowFn(showID, userID, isAdmin)
	}
	return nil, nil
}
func (m *mockShowService) PublishShow(showID uint, userID uint, isAdmin bool) (*services.ShowResponse, error) {
	if m.publishShowFn != nil {
		return m.publishShowFn(showID, userID, isAdmin)
	}
	return nil, nil
}
func (m *mockShowService) ExportShowToMarkdown(showID uint) ([]byte, string, error) {
	if m.exportShowToMarkdownFn != nil {
		return m.exportShowToMarkdownFn(showID)
	}
	return nil, "", nil
}
func (m *mockShowService) ParseShowMarkdown(content []byte) (*services.ParsedShowImport, error) {
	if m.parseShowMarkdownFn != nil {
		return m.parseShowMarkdownFn(content)
	}
	return nil, nil
}
func (m *mockShowService) PreviewShowImport(content []byte) (*services.ImportPreviewResponse, error) {
	if m.previewShowImportFn != nil {
		return m.previewShowImportFn(content)
	}
	return nil, nil
}
func (m *mockShowService) ConfirmShowImport(content []byte, isAdmin bool) (*services.ShowResponse, error) {
	if m.confirmShowImportFn != nil {
		return m.confirmShowImportFn(content, isAdmin)
	}
	return nil, nil
}
func (m *mockShowService) GetAdminShows(limit, offset int, filters services.AdminShowFilters) ([]*services.ShowResponse, int64, error) {
	if m.getAdminShowsFn != nil {
		return m.getAdminShowsFn(limit, offset, filters)
	}
	return nil, 0, nil
}
func (m *mockShowService) SetShowSoldOut(showID uint, isSoldOut bool) (*services.ShowResponse, error) {
	if m.setShowSoldOutFn != nil {
		return m.setShowSoldOutFn(showID, isSoldOut)
	}
	return nil, nil
}
func (m *mockShowService) SetShowCancelled(showID uint, isCancelled bool) (*services.ShowResponse, error) {
	if m.setShowCancelledFn != nil {
		return m.setShowCancelledFn(showID, isCancelled)
	}
	return nil, nil
}

// ============================================================================
// Mock: VenueServiceInterface
// ============================================================================

type mockVenueService struct {
	createVenueFn              func(req *services.CreateVenueRequest, isAdmin bool) (*services.VenueDetailResponse, error)
	getVenueFn                 func(venueID uint) (*services.VenueDetailResponse, error)
	getVenueBySlugFn           func(slug string) (*services.VenueDetailResponse, error)
	getVenuesFn                func(filters map[string]interface{}) ([]*services.VenueDetailResponse, error)
	updateVenueFn              func(venueID uint, updates map[string]interface{}) (*services.VenueDetailResponse, error)
	deleteVenueFn              func(venueID uint) error
	searchVenuesFn             func(query string) ([]*services.VenueDetailResponse, error)
	findOrCreateVenueFn        func(name, city, state string, address, zipcode *string, db *gorm.DB, isAdmin bool) (*models.Venue, bool, error)
	verifyVenueFn              func(venueID uint) (*services.VenueDetailResponse, error)
	getVenuesWithShowCountsFn  func(filters services.VenueListFilters, limit, offset int) ([]*services.VenueWithShowCountResponse, int64, error)
	getUpcomingShowsForVenueFn func(venueID uint, timezone string, limit int) ([]*services.VenueShowResponse, int64, error)
	getShowsForVenueFn         func(venueID uint, timezone string, limit int, timeFilter string) ([]*services.VenueShowResponse, int64, error)
	getVenueCitiesFn           func() ([]*services.VenueCityResponse, error)
	createPendingVenueEditFn   func(venueID uint, userID uint, req *services.VenueEditRequest) (*services.PendingVenueEditResponse, error)
	getPendingEditForVenueFn   func(venueID uint, userID uint) (*services.PendingVenueEditResponse, error)
	getPendingVenueEditsFn     func(limit, offset int) ([]*services.PendingVenueEditResponse, int64, error)
	getPendingVenueEditFn      func(editID uint) (*services.PendingVenueEditResponse, error)
	approveVenueEditFn         func(editID uint, reviewerID uint) (*services.VenueDetailResponse, error)
	rejectVenueEditFn          func(editID uint, reviewerID uint, reason string) (*services.PendingVenueEditResponse, error)
	cancelPendingVenueEditFn   func(editID uint, userID uint) error
	getVenueModelFn            func(venueID uint) (*models.Venue, error)
	getUnverifiedVenuesFn      func(limit, offset int) ([]*services.UnverifiedVenueResponse, int64, error)
}

func (m *mockVenueService) CreateVenue(req *services.CreateVenueRequest, isAdmin bool) (*services.VenueDetailResponse, error) {
	if m.createVenueFn != nil {
		return m.createVenueFn(req, isAdmin)
	}
	return nil, nil
}
func (m *mockVenueService) GetVenue(venueID uint) (*services.VenueDetailResponse, error) {
	if m.getVenueFn != nil {
		return m.getVenueFn(venueID)
	}
	return nil, nil
}
func (m *mockVenueService) GetVenueBySlug(slug string) (*services.VenueDetailResponse, error) {
	if m.getVenueBySlugFn != nil {
		return m.getVenueBySlugFn(slug)
	}
	return nil, nil
}
func (m *mockVenueService) GetVenues(filters map[string]interface{}) ([]*services.VenueDetailResponse, error) {
	if m.getVenuesFn != nil {
		return m.getVenuesFn(filters)
	}
	return nil, nil
}
func (m *mockVenueService) UpdateVenue(venueID uint, updates map[string]interface{}) (*services.VenueDetailResponse, error) {
	if m.updateVenueFn != nil {
		return m.updateVenueFn(venueID, updates)
	}
	return nil, nil
}
func (m *mockVenueService) DeleteVenue(venueID uint) error {
	if m.deleteVenueFn != nil {
		return m.deleteVenueFn(venueID)
	}
	return nil
}
func (m *mockVenueService) SearchVenues(query string) ([]*services.VenueDetailResponse, error) {
	if m.searchVenuesFn != nil {
		return m.searchVenuesFn(query)
	}
	return nil, nil
}
func (m *mockVenueService) FindOrCreateVenue(name, city, state string, address, zipcode *string, db *gorm.DB, isAdmin bool) (*models.Venue, bool, error) {
	if m.findOrCreateVenueFn != nil {
		return m.findOrCreateVenueFn(name, city, state, address, zipcode, db, isAdmin)
	}
	return nil, false, nil
}
func (m *mockVenueService) VerifyVenue(venueID uint) (*services.VenueDetailResponse, error) {
	if m.verifyVenueFn != nil {
		return m.verifyVenueFn(venueID)
	}
	return nil, nil
}
func (m *mockVenueService) GetVenuesWithShowCounts(filters services.VenueListFilters, limit, offset int) ([]*services.VenueWithShowCountResponse, int64, error) {
	if m.getVenuesWithShowCountsFn != nil {
		return m.getVenuesWithShowCountsFn(filters, limit, offset)
	}
	return nil, 0, nil
}
func (m *mockVenueService) GetUpcomingShowsForVenue(venueID uint, timezone string, limit int) ([]*services.VenueShowResponse, int64, error) {
	if m.getUpcomingShowsForVenueFn != nil {
		return m.getUpcomingShowsForVenueFn(venueID, timezone, limit)
	}
	return nil, 0, nil
}
func (m *mockVenueService) GetShowsForVenue(venueID uint, timezone string, limit int, timeFilter string) ([]*services.VenueShowResponse, int64, error) {
	if m.getShowsForVenueFn != nil {
		return m.getShowsForVenueFn(venueID, timezone, limit, timeFilter)
	}
	return nil, 0, nil
}
func (m *mockVenueService) GetVenueCities() ([]*services.VenueCityResponse, error) {
	if m.getVenueCitiesFn != nil {
		return m.getVenueCitiesFn()
	}
	return nil, nil
}
func (m *mockVenueService) CreatePendingVenueEdit(venueID uint, userID uint, req *services.VenueEditRequest) (*services.PendingVenueEditResponse, error) {
	if m.createPendingVenueEditFn != nil {
		return m.createPendingVenueEditFn(venueID, userID, req)
	}
	return nil, nil
}
func (m *mockVenueService) GetPendingEditForVenue(venueID uint, userID uint) (*services.PendingVenueEditResponse, error) {
	if m.getPendingEditForVenueFn != nil {
		return m.getPendingEditForVenueFn(venueID, userID)
	}
	return nil, nil
}
func (m *mockVenueService) GetPendingVenueEdits(limit, offset int) ([]*services.PendingVenueEditResponse, int64, error) {
	if m.getPendingVenueEditsFn != nil {
		return m.getPendingVenueEditsFn(limit, offset)
	}
	return nil, 0, nil
}
func (m *mockVenueService) GetPendingVenueEdit(editID uint) (*services.PendingVenueEditResponse, error) {
	if m.getPendingVenueEditFn != nil {
		return m.getPendingVenueEditFn(editID)
	}
	return nil, nil
}
func (m *mockVenueService) ApproveVenueEdit(editID uint, reviewerID uint) (*services.VenueDetailResponse, error) {
	if m.approveVenueEditFn != nil {
		return m.approveVenueEditFn(editID, reviewerID)
	}
	return nil, nil
}
func (m *mockVenueService) RejectVenueEdit(editID uint, reviewerID uint, reason string) (*services.PendingVenueEditResponse, error) {
	if m.rejectVenueEditFn != nil {
		return m.rejectVenueEditFn(editID, reviewerID, reason)
	}
	return nil, nil
}
func (m *mockVenueService) CancelPendingVenueEdit(editID uint, userID uint) error {
	if m.cancelPendingVenueEditFn != nil {
		return m.cancelPendingVenueEditFn(editID, userID)
	}
	return nil
}
func (m *mockVenueService) GetVenueModel(venueID uint) (*models.Venue, error) {
	if m.getVenueModelFn != nil {
		return m.getVenueModelFn(venueID)
	}
	return nil, nil
}
func (m *mockVenueService) GetUnverifiedVenues(limit, offset int) ([]*services.UnverifiedVenueResponse, int64, error) {
	if m.getUnverifiedVenuesFn != nil {
		return m.getUnverifiedVenuesFn(limit, offset)
	}
	return nil, 0, nil
}

// ============================================================================
// Mock: ArtistServiceInterface
// ============================================================================

type mockArtistService struct {
	createArtistFn      func(req *services.CreateArtistRequest) (*services.ArtistDetailResponse, error)
	getArtistFn         func(artistID uint) (*services.ArtistDetailResponse, error)
	getArtistByNameFn   func(name string) (*services.ArtistDetailResponse, error)
	getArtistBySlugFn   func(slug string) (*services.ArtistDetailResponse, error)
	getArtistsFn        func(filters map[string]interface{}) ([]*services.ArtistDetailResponse, error)
	updateArtistFn      func(artistID uint, updates map[string]interface{}) (*services.ArtistDetailResponse, error)
	deleteArtistFn      func(artistID uint) error
	searchArtistsFn     func(query string) ([]*services.ArtistDetailResponse, error)
	getShowsForArtistFn func(artistID uint, timezone string, limit int, timeFilter string) ([]*services.ArtistShowResponse, int64, error)
}

func (m *mockArtistService) CreateArtist(req *services.CreateArtistRequest) (*services.ArtistDetailResponse, error) {
	if m.createArtistFn != nil {
		return m.createArtistFn(req)
	}
	return nil, nil
}
func (m *mockArtistService) GetArtist(artistID uint) (*services.ArtistDetailResponse, error) {
	if m.getArtistFn != nil {
		return m.getArtistFn(artistID)
	}
	return nil, nil
}
func (m *mockArtistService) GetArtistByName(name string) (*services.ArtistDetailResponse, error) {
	if m.getArtistByNameFn != nil {
		return m.getArtistByNameFn(name)
	}
	return nil, nil
}
func (m *mockArtistService) GetArtistBySlug(slug string) (*services.ArtistDetailResponse, error) {
	if m.getArtistBySlugFn != nil {
		return m.getArtistBySlugFn(slug)
	}
	return nil, nil
}
func (m *mockArtistService) GetArtists(filters map[string]interface{}) ([]*services.ArtistDetailResponse, error) {
	if m.getArtistsFn != nil {
		return m.getArtistsFn(filters)
	}
	return nil, nil
}
func (m *mockArtistService) UpdateArtist(artistID uint, updates map[string]interface{}) (*services.ArtistDetailResponse, error) {
	if m.updateArtistFn != nil {
		return m.updateArtistFn(artistID, updates)
	}
	return nil, nil
}
func (m *mockArtistService) DeleteArtist(artistID uint) error {
	if m.deleteArtistFn != nil {
		return m.deleteArtistFn(artistID)
	}
	return nil
}
func (m *mockArtistService) SearchArtists(query string) ([]*services.ArtistDetailResponse, error) {
	if m.searchArtistsFn != nil {
		return m.searchArtistsFn(query)
	}
	return nil, nil
}
func (m *mockArtistService) GetShowsForArtist(artistID uint, timezone string, limit int, timeFilter string) ([]*services.ArtistShowResponse, int64, error) {
	if m.getShowsForArtistFn != nil {
		return m.getShowsForArtistFn(artistID, timezone, limit, timeFilter)
	}
	return nil, 0, nil
}

// ============================================================================
// Mock: MusicDiscoveryServiceInterface
// ============================================================================

type mockMusicDiscoveryService struct {
	isConfiguredFn           func() bool
	discoverMusicForArtistFn func(artistID uint, artistName string)
}

func (m *mockMusicDiscoveryService) IsConfigured() bool {
	if m.isConfiguredFn != nil {
		return m.isConfiguredFn()
	}
	return false
}
func (m *mockMusicDiscoveryService) DiscoverMusicForArtist(artistID uint, artistName string) {
	if m.discoverMusicForArtistFn != nil {
		m.discoverMusicForArtistFn(artistID, artistName)
	}
}

// ============================================================================
// Mock: ExtractionServiceInterface
// ============================================================================

type mockExtractionService struct {
	extractShowFn func(req *services.ExtractShowRequest) (*services.ExtractShowResponse, error)
}

func (m *mockExtractionService) ExtractShow(req *services.ExtractShowRequest) (*services.ExtractShowResponse, error) {
	if m.extractShowFn != nil {
		return m.extractShowFn(req)
	}
	return nil, nil
}

// ============================================================================
// Mock: DiscoveryServiceInterface
// ============================================================================

type mockDiscoveryService struct {
	importFromJSONFn       func(filepath string, dryRun bool) (*services.ImportResult, error)
	importFromJSONWithDBFn func(filepath string, dryRun bool, database *gorm.DB) (*services.ImportResult, error)
	checkEventsFn          func(events []services.CheckEventInput) (*services.CheckEventsResult, error)
	importEventsFn         func(events []services.DiscoveredEvent, dryRun bool, allowUpdates bool) (*services.ImportResult, error)
}

func (m *mockDiscoveryService) ImportFromJSON(filepath string, dryRun bool) (*services.ImportResult, error) {
	if m.importFromJSONFn != nil {
		return m.importFromJSONFn(filepath, dryRun)
	}
	return nil, nil
}
func (m *mockDiscoveryService) ImportFromJSONWithDB(filepath string, dryRun bool, database *gorm.DB) (*services.ImportResult, error) {
	if m.importFromJSONWithDBFn != nil {
		return m.importFromJSONWithDBFn(filepath, dryRun, database)
	}
	return nil, nil
}
func (m *mockDiscoveryService) CheckEvents(events []services.CheckEventInput) (*services.CheckEventsResult, error) {
	if m.checkEventsFn != nil {
		return m.checkEventsFn(events)
	}
	return nil, nil
}
func (m *mockDiscoveryService) ImportEvents(events []services.DiscoveredEvent, dryRun bool, allowUpdates bool) (*services.ImportResult, error) {
	if m.importEventsFn != nil {
		return m.importEventsFn(events, dryRun, allowUpdates)
	}
	return nil, nil
}

// ============================================================================
// Mock: APITokenServiceInterface
// ============================================================================

type mockAPITokenService struct {
	createTokenFn          func(userID uint, description *string, expirationDays int) (*services.APITokenCreateResponse, error)
	validateTokenFn        func(plainToken string) (*models.User, *models.APIToken, error)
	listTokensFn           func(userID uint) ([]services.APITokenResponse, error)
	revokeTokenFn          func(userID uint, tokenID uint) error
	getTokenFn             func(userID uint, tokenID uint) (*services.APITokenResponse, error)
	cleanupExpiredTokensFn func() (int64, error)
}

func (m *mockAPITokenService) CreateToken(userID uint, description *string, expirationDays int) (*services.APITokenCreateResponse, error) {
	if m.createTokenFn != nil {
		return m.createTokenFn(userID, description, expirationDays)
	}
	return nil, nil
}
func (m *mockAPITokenService) ValidateToken(plainToken string) (*models.User, *models.APIToken, error) {
	if m.validateTokenFn != nil {
		return m.validateTokenFn(plainToken)
	}
	return nil, nil, nil
}
func (m *mockAPITokenService) ListTokens(userID uint) ([]services.APITokenResponse, error) {
	if m.listTokensFn != nil {
		return m.listTokensFn(userID)
	}
	return nil, nil
}
func (m *mockAPITokenService) RevokeToken(userID uint, tokenID uint) error {
	if m.revokeTokenFn != nil {
		return m.revokeTokenFn(userID, tokenID)
	}
	return nil
}
func (m *mockAPITokenService) GetToken(userID uint, tokenID uint) (*services.APITokenResponse, error) {
	if m.getTokenFn != nil {
		return m.getTokenFn(userID, tokenID)
	}
	return nil, nil
}
func (m *mockAPITokenService) CleanupExpiredTokens() (int64, error) {
	if m.cleanupExpiredTokensFn != nil {
		return m.cleanupExpiredTokensFn()
	}
	return 0, nil
}

// ============================================================================
// Mock: DataSyncServiceInterface
// ============================================================================

type mockDataSyncService struct {
	exportShowsFn   func(params services.ExportShowsParams) (*services.ExportShowsResult, error)
	exportArtistsFn func(params services.ExportArtistsParams) (*services.ExportArtistsResult, error)
	exportVenuesFn  func(params services.ExportVenuesParams) (*services.ExportVenuesResult, error)
	importDataFn    func(req services.DataImportRequest) (*services.DataImportResult, error)
}

func (m *mockDataSyncService) ExportShows(params services.ExportShowsParams) (*services.ExportShowsResult, error) {
	if m.exportShowsFn != nil {
		return m.exportShowsFn(params)
	}
	return nil, nil
}
func (m *mockDataSyncService) ExportArtists(params services.ExportArtistsParams) (*services.ExportArtistsResult, error) {
	if m.exportArtistsFn != nil {
		return m.exportArtistsFn(params)
	}
	return nil, nil
}
func (m *mockDataSyncService) ExportVenues(params services.ExportVenuesParams) (*services.ExportVenuesResult, error) {
	if m.exportVenuesFn != nil {
		return m.exportVenuesFn(params)
	}
	return nil, nil
}
func (m *mockDataSyncService) ImportData(req services.DataImportRequest) (*services.DataImportResult, error) {
	if m.importDataFn != nil {
		return m.importDataFn(req)
	}
	return nil, nil
}

// ============================================================================
// Mock: AdminStatsServiceInterface
// ============================================================================

type mockAdminStatsService struct {
	getDashboardStatsFn func() (*services.AdminDashboardStats, error)
}

func (m *mockAdminStatsService) GetDashboardStats() (*services.AdminDashboardStats, error) {
	if m.getDashboardStatsFn != nil {
		return m.getDashboardStatsFn()
	}
	return nil, nil
}

// ============================================================================
// Mock: AuthServiceInterface
// ============================================================================

type mockAuthService struct {
	oauthLoginFn              func(w http.ResponseWriter, r *http.Request, provider string) error
	oauthCallbackFn           func(w http.ResponseWriter, r *http.Request, provider string) (*models.User, string, error)
	oauthCallbackWithConsentFn func(w http.ResponseWriter, r *http.Request, provider string, consent *services.OAuthSignupConsent) (*models.User, string, error)
	getUserProfileFn          func(userID uint) (*models.User, error)
	refreshUserTokenFn        func(user *models.User) (string, error)
	logoutFn                  func(w http.ResponseWriter, r *http.Request) error
	setOAuthCompleterFn       func(completer services.OAuthCompleter)
}

func (m *mockAuthService) OAuthLogin(w http.ResponseWriter, r *http.Request, provider string) error {
	if m.oauthLoginFn != nil {
		return m.oauthLoginFn(w, r, provider)
	}
	return nil
}
func (m *mockAuthService) OAuthCallback(w http.ResponseWriter, r *http.Request, provider string) (*models.User, string, error) {
	if m.oauthCallbackFn != nil {
		return m.oauthCallbackFn(w, r, provider)
	}
	return nil, "", nil
}
func (m *mockAuthService) OAuthCallbackWithConsent(w http.ResponseWriter, r *http.Request, provider string, consent *services.OAuthSignupConsent) (*models.User, string, error) {
	if m.oauthCallbackWithConsentFn != nil {
		return m.oauthCallbackWithConsentFn(w, r, provider, consent)
	}
	return nil, "", nil
}
func (m *mockAuthService) GetUserProfile(userID uint) (*models.User, error) {
	if m.getUserProfileFn != nil {
		return m.getUserProfileFn(userID)
	}
	return nil, nil
}
func (m *mockAuthService) RefreshUserToken(user *models.User) (string, error) {
	if m.refreshUserTokenFn != nil {
		return m.refreshUserTokenFn(user)
	}
	return "", nil
}
func (m *mockAuthService) Logout(w http.ResponseWriter, r *http.Request) error {
	if m.logoutFn != nil {
		return m.logoutFn(w, r)
	}
	return nil
}
func (m *mockAuthService) SetOAuthCompleter(completer services.OAuthCompleter) {
	if m.setOAuthCompleterFn != nil {
		m.setOAuthCompleterFn(completer)
	}
}

// ============================================================================
// Mock: JWTServiceInterface
// ============================================================================

type mockJWTService struct {
	createTokenFn                  func(user *models.User) (string, error)
	validateTokenFn                func(tokenString string) (*models.User, error)
	refreshTokenFn                 func(tokenString string) (string, error)
	validateTokenLenientFn         func(tokenString string, gracePeriod time.Duration) (*models.User, error)
	createVerificationTokenFn      func(userID uint, email string) (string, error)
	validateVerificationTokenFn    func(tokenString string) (*services.VerificationTokenClaims, error)
	createMagicLinkTokenFn         func(userID uint, email string) (string, error)
	validateMagicLinkTokenFn       func(tokenString string) (*services.MagicLinkTokenClaims, error)
	createAccountRecoveryTokenFn   func(userID uint, email string) (string, error)
	validateAccountRecoveryTokenFn func(tokenString string) (*services.AccountRecoveryTokenClaims, error)
}

func (m *mockJWTService) CreateToken(user *models.User) (string, error) {
	if m.createTokenFn != nil {
		return m.createTokenFn(user)
	}
	return "", nil
}
func (m *mockJWTService) ValidateToken(tokenString string) (*models.User, error) {
	if m.validateTokenFn != nil {
		return m.validateTokenFn(tokenString)
	}
	return nil, nil
}
func (m *mockJWTService) RefreshToken(tokenString string) (string, error) {
	if m.refreshTokenFn != nil {
		return m.refreshTokenFn(tokenString)
	}
	return "", nil
}
func (m *mockJWTService) ValidateTokenLenient(tokenString string, gracePeriod time.Duration) (*models.User, error) {
	if m.validateTokenLenientFn != nil {
		return m.validateTokenLenientFn(tokenString, gracePeriod)
	}
	return nil, nil
}
func (m *mockJWTService) CreateVerificationToken(userID uint, email string) (string, error) {
	if m.createVerificationTokenFn != nil {
		return m.createVerificationTokenFn(userID, email)
	}
	return "", nil
}
func (m *mockJWTService) ValidateVerificationToken(tokenString string) (*services.VerificationTokenClaims, error) {
	if m.validateVerificationTokenFn != nil {
		return m.validateVerificationTokenFn(tokenString)
	}
	return nil, nil
}
func (m *mockJWTService) CreateMagicLinkToken(userID uint, email string) (string, error) {
	if m.createMagicLinkTokenFn != nil {
		return m.createMagicLinkTokenFn(userID, email)
	}
	return "", nil
}
func (m *mockJWTService) ValidateMagicLinkToken(tokenString string) (*services.MagicLinkTokenClaims, error) {
	if m.validateMagicLinkTokenFn != nil {
		return m.validateMagicLinkTokenFn(tokenString)
	}
	return nil, nil
}
func (m *mockJWTService) CreateAccountRecoveryToken(userID uint, email string) (string, error) {
	if m.createAccountRecoveryTokenFn != nil {
		return m.createAccountRecoveryTokenFn(userID, email)
	}
	return "", nil
}
func (m *mockJWTService) ValidateAccountRecoveryToken(tokenString string) (*services.AccountRecoveryTokenClaims, error) {
	if m.validateAccountRecoveryTokenFn != nil {
		return m.validateAccountRecoveryTokenFn(tokenString)
	}
	return nil, nil
}

// ============================================================================
// Mock: EmailServiceInterface
// ============================================================================

type mockEmailService struct {
	isConfiguredFn              func() bool
	sendVerificationEmailFn     func(toEmail, token string) error
	sendMagicLinkEmailFn        func(toEmail, token string) error
	sendAccountRecoveryEmailFn  func(toEmail, token string, daysRemaining int) error
}

func (m *mockEmailService) IsConfigured() bool {
	if m.isConfiguredFn != nil {
		return m.isConfiguredFn()
	}
	return false
}
func (m *mockEmailService) SendVerificationEmail(toEmail, token string) error {
	if m.sendVerificationEmailFn != nil {
		return m.sendVerificationEmailFn(toEmail, token)
	}
	return nil
}
func (m *mockEmailService) SendMagicLinkEmail(toEmail, token string) error {
	if m.sendMagicLinkEmailFn != nil {
		return m.sendMagicLinkEmailFn(toEmail, token)
	}
	return nil
}
func (m *mockEmailService) SendAccountRecoveryEmail(toEmail, token string, daysRemaining int) error {
	if m.sendAccountRecoveryEmailFn != nil {
		return m.sendAccountRecoveryEmailFn(toEmail, token, daysRemaining)
	}
	return nil
}

// ============================================================================
// Mock: PasswordValidatorInterface
// ============================================================================

type mockPasswordValidator struct {
	validatePasswordFn func(password string) (*services.PasswordValidationResult, error)
	isBreachedFn       func(password string) (bool, error)
	isCommonPasswordFn func(password string) bool
}

func (m *mockPasswordValidator) ValidatePassword(password string) (*services.PasswordValidationResult, error) {
	if m.validatePasswordFn != nil {
		return m.validatePasswordFn(password)
	}
	return &services.PasswordValidationResult{Valid: true}, nil
}
func (m *mockPasswordValidator) IsBreached(password string) (bool, error) {
	if m.isBreachedFn != nil {
		return m.isBreachedFn(password)
	}
	return false, nil
}
func (m *mockPasswordValidator) IsCommonPassword(password string) bool {
	if m.isCommonPasswordFn != nil {
		return m.isCommonPasswordFn(password)
	}
	return false
}

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
var _ services.ShowServiceInterface = (*mockShowService)(nil)
var _ services.VenueServiceInterface = (*mockVenueService)(nil)
var _ services.ArtistServiceInterface = (*mockArtistService)(nil)
var _ services.MusicDiscoveryServiceInterface = (*mockMusicDiscoveryService)(nil)
var _ services.ExtractionServiceInterface = (*mockExtractionService)(nil)
var _ services.DiscoveryServiceInterface = (*mockDiscoveryService)(nil)
var _ services.APITokenServiceInterface = (*mockAPITokenService)(nil)
var _ services.DataSyncServiceInterface = (*mockDataSyncService)(nil)
var _ services.AdminStatsServiceInterface = (*mockAdminStatsService)(nil)
var _ services.AuthServiceInterface = (*mockAuthService)(nil)
var _ services.JWTServiceInterface = (*mockJWTService)(nil)
var _ services.EmailServiceInterface = (*mockEmailService)(nil)
var _ services.PasswordValidatorInterface = (*mockPasswordValidator)(nil)
