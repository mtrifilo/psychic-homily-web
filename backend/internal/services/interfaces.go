package services

import (
	"net/http"
	"time"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/markbates/goth"
	"gorm.io/gorm"

	"psychic-homily-backend/internal/models"
)

// ShowServiceInterface defines the contract for show operations.
type ShowServiceInterface interface {
	CreateShow(req *CreateShowRequest) (*ShowResponse, error)
	GetShow(showID uint) (*ShowResponse, error)
	GetShowBySlug(slug string) (*ShowResponse, error)
	GetShows(filters map[string]interface{}) ([]*ShowResponse, error)
	GetUserSubmissions(userID uint, limit, offset int) ([]ShowResponse, int, error)
	UpdateShow(showID uint, updates map[string]interface{}) (*ShowResponse, error)
	UpdateShowWithRelations(showID uint, updates map[string]interface{}, venues []CreateShowVenue, artists []CreateShowArtist, isAdmin bool) (*ShowResponse, []OrphanedArtist, error)
	GetUpcomingShows(timezone string, cursor string, limit int, includeNonApproved bool, filters *UpcomingShowsFilter) ([]*ShowResponse, *string, error)
	GetShowCities(timezone string) ([]ShowCityResponse, error)
	DeleteShow(showID uint) error
	GetPendingShows(limit, offset int) ([]*ShowResponse, int64, error)
	GetRejectedShows(limit, offset int, search string) ([]*ShowResponse, int64, error)
	ApproveShow(showID uint, verifyVenues bool) (*ShowResponse, error)
	RejectShow(showID uint, reason string) (*ShowResponse, error)
	UnpublishShow(showID uint, userID uint, isAdmin bool) (*ShowResponse, error)
	MakePrivateShow(showID uint, userID uint, isAdmin bool) (*ShowResponse, error)
	PublishShow(showID uint, userID uint, isAdmin bool) (*ShowResponse, error)
	ExportShowToMarkdown(showID uint) ([]byte, string, error)
	ParseShowMarkdown(content []byte) (*ParsedShowImport, error)
	PreviewShowImport(content []byte) (*ImportPreviewResponse, error)
	ConfirmShowImport(content []byte, isAdmin bool) (*ShowResponse, error)
	GetAdminShows(limit, offset int, filters AdminShowFilters) ([]*ShowResponse, int64, error)
	SetShowSoldOut(showID uint, isSoldOut bool) (*ShowResponse, error)
	SetShowCancelled(showID uint, isCancelled bool) (*ShowResponse, error)
}

// VenueServiceInterface defines the contract for venue operations.
type VenueServiceInterface interface {
	CreateVenue(req *CreateVenueRequest, isAdmin bool) (*VenueDetailResponse, error)
	GetVenue(venueID uint) (*VenueDetailResponse, error)
	GetVenueBySlug(slug string) (*VenueDetailResponse, error)
	GetVenues(filters map[string]interface{}) ([]*VenueDetailResponse, error)
	UpdateVenue(venueID uint, updates map[string]interface{}) (*VenueDetailResponse, error)
	DeleteVenue(venueID uint) error
	SearchVenues(query string) ([]*VenueDetailResponse, error)
	FindOrCreateVenue(name, city, state string, address, zipcode *string, db *gorm.DB, isAdmin bool) (*models.Venue, bool, error)
	VerifyVenue(venueID uint) (*VenueDetailResponse, error)
	GetVenuesWithShowCounts(filters VenueListFilters, limit, offset int) ([]*VenueWithShowCountResponse, int64, error)
	GetUpcomingShowsForVenue(venueID uint, timezone string, limit int) ([]*VenueShowResponse, int64, error)
	GetShowsForVenue(venueID uint, timezone string, limit int, timeFilter string) ([]*VenueShowResponse, int64, error)
	GetVenueCities() ([]*VenueCityResponse, error)
	CreatePendingVenueEdit(venueID uint, userID uint, req *VenueEditRequest) (*PendingVenueEditResponse, error)
	GetPendingEditForVenue(venueID uint, userID uint) (*PendingVenueEditResponse, error)
	GetPendingVenueEdits(limit, offset int) ([]*PendingVenueEditResponse, int64, error)
	GetPendingVenueEdit(editID uint) (*PendingVenueEditResponse, error)
	ApproveVenueEdit(editID uint, reviewerID uint) (*VenueDetailResponse, error)
	RejectVenueEdit(editID uint, reviewerID uint, reason string) (*PendingVenueEditResponse, error)
	CancelPendingVenueEdit(editID uint, userID uint) error
	GetVenueModel(venueID uint) (*models.Venue, error)
	GetUnverifiedVenues(limit, offset int) ([]*UnverifiedVenueResponse, int64, error)
}

// ArtistServiceInterface defines the contract for artist operations.
type ArtistServiceInterface interface {
	CreateArtist(req *CreateArtistRequest) (*ArtistDetailResponse, error)
	GetArtist(artistID uint) (*ArtistDetailResponse, error)
	GetArtistByName(name string) (*ArtistDetailResponse, error)
	GetArtistBySlug(slug string) (*ArtistDetailResponse, error)
	GetArtists(filters map[string]interface{}) ([]*ArtistDetailResponse, error)
	UpdateArtist(artistID uint, updates map[string]interface{}) (*ArtistDetailResponse, error)
	DeleteArtist(artistID uint) error
	SearchArtists(query string) ([]*ArtistDetailResponse, error)
	GetShowsForArtist(artistID uint, timezone string, limit int, timeFilter string) ([]*ArtistShowResponse, int64, error)
}

// SavedShowServiceInterface defines the contract for saved show operations.
type SavedShowServiceInterface interface {
	SaveShow(userID, showID uint) error
	UnsaveShow(userID, showID uint) error
	GetUserSavedShows(userID uint, limit, offset int) ([]*SavedShowResponse, int64, error)
	IsShowSaved(userID, showID uint) (bool, error)
	GetSavedShowIDs(userID uint, showIDs []uint) (map[uint]bool, error)
}

// FavoriteVenueServiceInterface defines the contract for favorite venue operations.
type FavoriteVenueServiceInterface interface {
	FavoriteVenue(userID, venueID uint) error
	UnfavoriteVenue(userID, venueID uint) error
	GetUserFavoriteVenues(userID uint, limit, offset int) ([]*FavoriteVenueResponse, int64, error)
	IsVenueFavorited(userID, venueID uint) (bool, error)
	GetUpcomingShowsFromFavorites(userID uint, timezone string, limit, offset int) ([]*FavoriteVenueShowResponse, int64, error)
	GetFavoriteVenueIDs(userID uint, venueIDs []uint) (map[uint]bool, error)
}

// ShowReportServiceInterface defines the contract for show report operations.
type ShowReportServiceInterface interface {
	CreateReport(userID, showID uint, reportType string, details *string) (*ShowReportResponse, error)
	GetUserReportForShow(userID, showID uint) (*ShowReportResponse, error)
	GetPendingReports(limit, offset int) ([]*ShowReportResponse, int64, error)
	DismissReport(reportID, adminID uint, notes *string) (*ShowReportResponse, error)
	ResolveReport(reportID, adminID uint, notes *string) (*ShowReportResponse, error)
	ResolveReportWithFlag(reportID, adminID uint, notes *string, setShowFlag bool) (*ShowReportResponse, error)
	GetReportByID(reportID uint) (*models.ShowReport, error)
}

// ArtistReportServiceInterface defines the contract for artist report operations.
type ArtistReportServiceInterface interface {
	CreateReport(userID, artistID uint, reportType string, details *string) (*ArtistReportResponse, error)
	GetUserReportForArtist(userID, artistID uint) (*ArtistReportResponse, error)
	GetPendingReports(limit, offset int) ([]*ArtistReportResponse, int64, error)
	DismissReport(reportID, adminID uint, notes *string) (*ArtistReportResponse, error)
	ResolveReport(reportID, adminID uint, notes *string) (*ArtistReportResponse, error)
	GetReportByID(reportID uint) (*models.ArtistReport, error)
}

// AuditLogServiceInterface defines the contract for audit log operations.
type AuditLogServiceInterface interface {
	LogAction(actorID uint, action string, entityType string, entityID uint, metadata map[string]interface{})
	GetAuditLogs(limit, offset int, filters AuditLogFilters) ([]*AuditLogResponse, int64, error)
}

// AuthServiceInterface defines the contract for authentication operations.
type AuthServiceInterface interface {
	OAuthLogin(w http.ResponseWriter, r *http.Request, provider string) error
	OAuthCallback(w http.ResponseWriter, r *http.Request, provider string) (*models.User, string, error)
	OAuthCallbackWithConsent(w http.ResponseWriter, r *http.Request, provider string, consent *OAuthSignupConsent) (*models.User, string, error)
	GetUserProfile(userID uint) (*models.User, error)
	RefreshUserToken(user *models.User) (string, error)
	Logout(w http.ResponseWriter, r *http.Request) error
	SetOAuthCompleter(completer OAuthCompleter)
}

// JWTServiceInterface defines the contract for JWT token operations.
type JWTServiceInterface interface {
	CreateToken(user *models.User) (string, error)
	ValidateToken(tokenString string) (*models.User, error)
	RefreshToken(tokenString string) (string, error)
	ValidateTokenLenient(tokenString string, gracePeriod time.Duration) (*models.User, error)
	CreateVerificationToken(userID uint, email string) (string, error)
	ValidateVerificationToken(tokenString string) (*VerificationTokenClaims, error)
	CreateMagicLinkToken(userID uint, email string) (string, error)
	ValidateMagicLinkToken(tokenString string) (*MagicLinkTokenClaims, error)
	CreateAccountRecoveryToken(userID uint, email string) (string, error)
	ValidateAccountRecoveryToken(tokenString string) (*AccountRecoveryTokenClaims, error)
}

// UserServiceInterface defines the contract for user operations.
type UserServiceInterface interface {
	ListUsers(limit, offset int, filters AdminUserFilters) ([]*AdminUserResponse, int64, error)
	FindOrCreateUser(gothUser goth.User, provider string) (*models.User, error)
	FindOrCreateUserWithConsent(gothUser goth.User, provider string, consent *OAuthSignupConsent) (*models.User, error)
	AuthenticateUserWithPassword(email, password string) (*models.User, error)
	CreateUserWithPassword(email, password, firstName, lastName string) (*models.User, error)
	CreateUserWithPasswordWithLegal(email, password, firstName, lastName string, acceptance LegalAcceptance) (*models.User, error)
	GetUserByID(userID uint) (*models.User, error)
	GetUserByEmail(email string) (*models.User, error)
	GetUserByUsername(username string) (*models.User, error)
	UpdateUser(userID uint, updates map[string]any) (*models.User, error)
	HashPassword(password string) (string, error)
	VerifyPassword(hashedPassword, password string) error
	IsAccountLocked(user *models.User) bool
	GetLockTimeRemaining(user *models.User) time.Duration
	IncrementFailedAttempts(userID uint) error
	ResetFailedAttempts(userID uint) error
	UpdatePassword(userID uint, currentPassword, newPassword string) error
	SetEmailVerified(userID uint, verified bool) error
	GetDeletionSummary(userID uint) (*DeletionSummary, error)
	SoftDeleteAccount(userID uint, reason *string) error
	CreateUserWithoutPassword(email string) (*models.User, error)
	ExportUserData(userID uint) (*UserDataExport, error)
	ExportUserDataJSON(userID uint) ([]byte, error)
	GetOAuthAccounts(userID uint) ([]models.OAuthAccount, error)
	GetUserByEmailIncludingDeleted(email string) (*models.User, error)
	IsAccountRecoverable(user *models.User) bool
	GetDaysUntilPermanentDeletion(user *models.User) int
	RestoreAccount(userID uint) error
	GetExpiredDeletedAccounts() ([]models.User, error)
	PermanentlyDeleteUser(userID uint) error
	CanUnlinkOAuthAccount(userID uint, provider string) (bool, string, error)
	UnlinkOAuthAccount(userID uint, provider string) error
	GetFavoriteCities(userID uint) ([]models.FavoriteCity, error)
	SetFavoriteCities(userID uint, cities []models.FavoriteCity) error
}

// EmailServiceInterface defines the contract for email operations.
type EmailServiceInterface interface {
	IsConfigured() bool
	SendVerificationEmail(toEmail, token string) error
	SendMagicLinkEmail(toEmail, token string) error
	SendAccountRecoveryEmail(toEmail, token string, daysRemaining int) error
}

// DiscordServiceInterface defines the contract for Discord notification operations.
type DiscordServiceInterface interface {
	IsConfigured() bool
	NotifyNewUser(user *models.User)
	NotifyNewShow(show *ShowResponse, submitterEmail string)
	NotifyShowStatusChange(showTitle string, showID uint, oldStatus, newStatus, actorEmail string)
	NotifyShowApproved(show *ShowResponse)
	NotifyShowRejected(show *ShowResponse, reason string)
	NotifyShowReport(report *models.ShowReport, reporterEmail string)
	NotifyArtistReport(report *models.ArtistReport, reporterEmail string)
	NotifyNewVenue(venueID uint, venueName, city, state string, address *string, submitterEmail string)
	NotifyPendingVenueEdit(editID, venueID uint, venueName, submitterEmail string)
}

// PasswordValidatorInterface defines the contract for password validation operations.
type PasswordValidatorInterface interface {
	ValidatePassword(password string) (*PasswordValidationResult, error)
	IsBreached(password string) (bool, error)
	IsCommonPassword(password string) bool
}

// ExtractionServiceInterface defines the contract for AI show extraction operations.
type ExtractionServiceInterface interface {
	ExtractShow(req *ExtractShowRequest) (*ExtractShowResponse, error)
}

// MusicDiscoveryServiceInterface defines the contract for music discovery operations.
type MusicDiscoveryServiceInterface interface {
	IsConfigured() bool
	DiscoverMusicForArtist(artistID uint, artistName string)
}

// AppleAuthServiceInterface defines the contract for Apple authentication operations.
type AppleAuthServiceInterface interface {
	ValidateIdentityToken(identityToken string) (*AppleIdentityTokenClaims, error)
	FindOrCreateAppleUser(claims *AppleIdentityTokenClaims, firstName, lastName string) (*models.User, error)
	GenerateToken(user *models.User) (string, error)
}

// WebAuthnServiceInterface defines the contract for WebAuthn/passkey operations.
type WebAuthnServiceInterface interface {
	BeginRegistration(user *models.User) (*protocol.CredentialCreation, *webauthn.SessionData, error)
	FinishRegistration(user *models.User, session *webauthn.SessionData, response *protocol.ParsedCredentialCreationData, displayName string) (*models.WebAuthnCredential, error)
	BeginLogin(user *models.User) (*protocol.CredentialAssertion, *webauthn.SessionData, error)
	BeginDiscoverableLogin() (*protocol.CredentialAssertion, *webauthn.SessionData, error)
	FinishLogin(user *models.User, session *webauthn.SessionData, response *protocol.ParsedCredentialAssertionData) (*models.WebAuthnCredential, error)
	FinishDiscoverableLogin(session *webauthn.SessionData, response *protocol.ParsedCredentialAssertionData) (*models.User, *models.WebAuthnCredential, error)
	GetUserCredentials(userID uint) ([]models.WebAuthnCredential, error)
	DeleteCredential(userID uint, credentialID uint) error
	UpdateCredentialName(userID uint, credentialID uint, displayName string) error
	StoreChallenge(userID uint, session *webauthn.SessionData, operation string) (string, error)
	GetChallenge(challengeID string, operation string) (*webauthn.SessionData, uint, error)
	DeleteChallenge(challengeID string) error
	CleanupExpiredChallenges() error
	BeginRegistrationForEmail(email string) (*protocol.CredentialCreation, *webauthn.SessionData, error)
	StoreChallengeWithEmail(email string, session *webauthn.SessionData, operation string) (string, error)
	GetChallengeWithEmail(challengeID string, operation string) (*webauthn.SessionData, string, error)
	FinishSignupRegistration(email string, session *webauthn.SessionData, response *protocol.ParsedCredentialCreationData, displayName string) (*models.User, error)
	FinishSignupRegistrationWithLegal(email string, session *webauthn.SessionData, response *protocol.ParsedCredentialCreationData, displayName string, acceptance LegalAcceptance) (*models.User, error)
}

// DiscoveryServiceInterface defines the contract for venue discovery/import operations.
type DiscoveryServiceInterface interface {
	ImportFromJSON(filepath string, dryRun bool) (*ImportResult, error)
	ImportFromJSONWithDB(filepath string, dryRun bool, database *gorm.DB) (*ImportResult, error)
	CheckEvents(events []CheckEventInput) (*CheckEventsResult, error)
	ImportEvents(events []DiscoveredEvent, dryRun bool, allowUpdates bool) (*ImportResult, error)
}

// APITokenServiceInterface defines the contract for API token operations.
type APITokenServiceInterface interface {
	CreateToken(userID uint, description *string, expirationDays int) (*APITokenCreateResponse, error)
	ValidateToken(plainToken string) (*models.User, *models.APIToken, error)
	ListTokens(userID uint) ([]APITokenResponse, error)
	RevokeToken(userID uint, tokenID uint) error
	GetToken(userID uint, tokenID uint) (*APITokenResponse, error)
	CleanupExpiredTokens() (int64, error)
}

// DataSyncServiceInterface defines the contract for data export/import operations.
type DataSyncServiceInterface interface {
	ExportShows(params ExportShowsParams) (*ExportShowsResult, error)
	ExportArtists(params ExportArtistsParams) (*ExportArtistsResult, error)
	ExportVenues(params ExportVenuesParams) (*ExportVenuesResult, error)
	ImportData(req DataImportRequest) (*DataImportResult, error)
}

// AdminStatsServiceInterface defines the contract for admin statistics operations.
type AdminStatsServiceInterface interface {
	GetDashboardStats() (*AdminDashboardStats, error)
}

// Compile-time interface satisfaction checks.
var (
	_ ShowServiceInterface          = (*ShowService)(nil)
	_ VenueServiceInterface         = (*VenueService)(nil)
	_ ArtistServiceInterface        = (*ArtistService)(nil)
	_ SavedShowServiceInterface     = (*SavedShowService)(nil)
	_ FavoriteVenueServiceInterface = (*FavoriteVenueService)(nil)
	_ ShowReportServiceInterface    = (*ShowReportService)(nil)
	_ ArtistReportServiceInterface  = (*ArtistReportService)(nil)
	_ AuditLogServiceInterface      = (*AuditLogService)(nil)
	_ AuthServiceInterface          = (*AuthService)(nil)
	_ JWTServiceInterface           = (*JWTService)(nil)
	_ UserServiceInterface          = (*UserService)(nil)
	_ EmailServiceInterface         = (*EmailService)(nil)
	_ DiscordServiceInterface       = (*DiscordService)(nil)
	_ PasswordValidatorInterface    = (*PasswordValidator)(nil)
	_ ExtractionServiceInterface    = (*ExtractionService)(nil)
	_ MusicDiscoveryServiceInterface = (*MusicDiscoveryService)(nil)
	_ AppleAuthServiceInterface     = (*AppleAuthService)(nil)
	_ WebAuthnServiceInterface      = (*WebAuthnService)(nil)
	_ DiscoveryServiceInterface     = (*DiscoveryService)(nil)
	_ APITokenServiceInterface      = (*APITokenService)(nil)
	_ DataSyncServiceInterface      = (*DataSyncService)(nil)
	_ AdminStatsServiceInterface    = (*AdminStatsService)(nil)
)
