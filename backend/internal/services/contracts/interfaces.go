package contracts

import (
	"context"
	"net/http"
	"time"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/markbates/goth"
	"gorm.io/gorm"

	"psychic-homily-backend/internal/models"
)

// ShowServiceInterface defines the contract for core show CRUD and search operations.
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
}

// ShowAdminServiceInterface defines the contract for admin show management operations
// including pending/rejected queries, approval flows, and batch operations.
type ShowAdminServiceInterface interface {
	GetPendingShows(limit, offset int, filters *PendingShowsFilter) ([]*ShowResponse, int64, error)
	GetRejectedShows(limit, offset int, search string) ([]*ShowResponse, int64, error)
	ApproveShow(showID uint, verifyVenues bool) (*ShowResponse, error)
	RejectShow(showID uint, reason string) (*ShowResponse, error)
	BatchApproveShows(showIDs []uint) (*BatchShowResult, error)
	BatchRejectShows(showIDs []uint, reason string, category string) (*BatchShowResult, error)
	GetAdminShows(limit, offset int, filters AdminShowFilters) ([]*ShowResponse, int64, error)
}

// ShowImportServiceInterface defines the contract for show import/export operations.
type ShowImportServiceInterface interface {
	ExportShowToMarkdown(showID uint) ([]byte, string, error)
	ParseShowMarkdown(content []byte) (*ParsedShowImport, error)
	PreviewShowImport(content []byte) (*ImportPreviewResponse, error)
	ConfirmShowImport(content []byte, isAdmin bool) (*ShowResponse, error)
}

// ShowStateServiceInterface defines the contract for show state mutation operations
// such as publishing, unpublishing, and setting sold-out/cancelled flags.
type ShowStateServiceInterface interface {
	UnpublishShow(showID uint, userID uint, isAdmin bool) (*ShowResponse, error)
	MakePrivateShow(showID uint, userID uint, isAdmin bool) (*ShowResponse, error)
	PublishShow(showID uint, userID uint, isAdmin bool) (*ShowResponse, error)
	SetShowSoldOut(showID uint, isSoldOut bool) (*ShowResponse, error)
	SetShowCancelled(showID uint, isCancelled bool) (*ShowResponse, error)
}

// ShowFullServiceInterface is the composite interface that embeds all show service
// concerns. The concrete ShowService satisfies this. Useful for the service container
// and backward compatibility where a single reference to all methods is needed.
type ShowFullServiceInterface interface {
	ShowServiceInterface
	ShowAdminServiceInterface
	ShowImportServiceInterface
	ShowStateServiceInterface
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
	GetVenueGenreProfile(venueID uint) ([]GenreCount, error)
}

// ArtistServiceInterface defines the contract for artist operations.
type ArtistServiceInterface interface {
	CreateArtist(req *CreateArtistRequest) (*ArtistDetailResponse, error)
	GetArtist(artistID uint) (*ArtistDetailResponse, error)
	GetArtistByName(name string) (*ArtistDetailResponse, error)
	GetArtistBySlug(slug string) (*ArtistDetailResponse, error)
	GetArtists(filters map[string]interface{}) ([]*ArtistDetailResponse, error)
	GetArtistsWithShowCounts(filters map[string]interface{}) ([]*ArtistWithShowCountResponse, error)
	UpdateArtist(artistID uint, updates map[string]interface{}) (*ArtistDetailResponse, error)
	DeleteArtist(artistID uint) error
	SearchArtists(query string) ([]*ArtistDetailResponse, error)
	GetShowsForArtist(artistID uint, timezone string, limit int, timeFilter string) ([]*ArtistShowResponse, int64, error)
	GetArtistCities() ([]*ArtistCityResponse, error)
	GetLabelsForArtist(artistID uint) ([]*ArtistLabelResponse, error)
	AddArtistAlias(artistID uint, alias string) (*ArtistAliasResponse, error)
	RemoveArtistAlias(aliasID uint) error
	GetArtistAliases(artistID uint) ([]*ArtistAliasResponse, error)
	MergeArtists(canonicalID, mergeFromID uint) (*MergeArtistResult, error)
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

// BookmarkServiceInterface defines the contract for generic bookmark operations.
type BookmarkServiceInterface interface {
	CreateBookmark(userID uint, entityType models.BookmarkEntityType, entityID uint, action models.BookmarkAction) error
	DeleteBookmark(userID uint, entityType models.BookmarkEntityType, entityID uint, action models.BookmarkAction) error
	IsBookmarked(userID uint, entityType models.BookmarkEntityType, entityID uint, action models.BookmarkAction) (bool, error)
	GetBookmarkedEntityIDs(userID uint, entityType models.BookmarkEntityType, action models.BookmarkAction, entityIDs []uint) (map[uint]bool, error)
	GetUserBookmarks(userID uint, entityType models.BookmarkEntityType, action models.BookmarkAction, limit, offset int) ([]models.UserBookmark, int64, error)
	GetUserBookmarksByEntityType(userID uint, entityType models.BookmarkEntityType, action models.BookmarkAction) ([]models.UserBookmark, error)
	CountUserBookmarks(userID uint, entityType models.BookmarkEntityType, action models.BookmarkAction) (int64, error)
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
	SetShowReminders(userID uint, enabled bool) error
}

// EmailServiceInterface defines the contract for email operations.
type EmailServiceInterface interface {
	IsConfigured() bool
	SendVerificationEmail(toEmail, token string) error
	SendMagicLinkEmail(toEmail, token string) error
	SendAccountRecoveryEmail(toEmail, token string, daysRemaining int) error
	SendShowReminderEmail(toEmail, showTitle, showURL, unsubscribeURL string, eventDate time.Time, venues []string) error
	SendFilterNotificationEmail(toEmail, subject, htmlBody, unsubscribeURL string) error
	SendTierPromotionEmail(toEmail, username, oldTier, newTier, reason string, newPermissions []string) error
	SendTierDemotionEmail(toEmail, username, oldTier, newTier, reason string) error
	SendTierDemotionWarningEmail(toEmail, username, currentTier string, currentRate float64, threshold float64) error
	SendEditApprovedEmail(toEmail, username, entityType, entityName, entityURL string) error
	SendEditRejectedEmail(toEmail, username, entityType, entityName, rejectionReason string) error
}

// ReminderServiceInterface defines the contract for the show reminder background service.
type ReminderServiceInterface interface {
	Start(ctx context.Context)
	Stop()
	RunReminderCycleNow()
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
	ExtractCalendarPage(venueName string, content string, contentType string, extractionNotes ...string) (*CalendarExtractionResponse, error)
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
	ImportEvents(events []DiscoveredEvent, dryRun bool, allowUpdates bool, initialStatus models.ShowStatus) (*ImportResult, error)
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
	GetRecentActivity() (*ActivityFeedResponse, error)
}

// LabelServiceInterface defines the contract for label operations.
type LabelServiceInterface interface {
	CreateLabel(req *CreateLabelRequest) (*LabelDetailResponse, error)
	GetLabel(labelID uint) (*LabelDetailResponse, error)
	GetLabelBySlug(slug string) (*LabelDetailResponse, error)
	ListLabels(filters map[string]interface{}) ([]*LabelListResponse, error)
	SearchLabels(query string) ([]*LabelListResponse, error)
	UpdateLabel(labelID uint, req *UpdateLabelRequest) (*LabelDetailResponse, error)
	DeleteLabel(labelID uint) error
	GetLabelRoster(labelID uint) ([]*LabelArtistResponse, error)
	GetLabelCatalog(labelID uint) ([]*LabelReleaseResponse, error)
	AddArtistToLabel(labelID, artistID uint) error
	AddReleaseToLabel(labelID, releaseID uint, catalogNumber *string) error
}

// FestivalServiceInterface defines the contract for festival operations.
type FestivalServiceInterface interface {
	CreateFestival(req *CreateFestivalRequest) (*FestivalDetailResponse, error)
	GetFestival(festivalID uint) (*FestivalDetailResponse, error)
	GetFestivalBySlug(slug string) (*FestivalDetailResponse, error)
	ListFestivals(filters map[string]interface{}) ([]*FestivalListResponse, error)
	SearchFestivals(query string) ([]*FestivalListResponse, error)
	UpdateFestival(festivalID uint, req *UpdateFestivalRequest) (*FestivalDetailResponse, error)
	DeleteFestival(festivalID uint) error
	GetFestivalArtists(festivalID uint, dayDate *string) ([]*FestivalArtistResponse, error)
	AddFestivalArtist(festivalID uint, req *AddFestivalArtistRequest) (*FestivalArtistResponse, error)
	UpdateFestivalArtist(festivalID, artistID uint, req *UpdateFestivalArtistRequest) (*FestivalArtistResponse, error)
	RemoveFestivalArtist(festivalID, artistID uint) error
	GetFestivalVenues(festivalID uint) ([]*FestivalVenueResponse, error)
	AddFestivalVenue(festivalID uint, req *AddFestivalVenueRequest) (*FestivalVenueResponse, error)
	RemoveFestivalVenue(festivalID, venueID uint) error
	GetFestivalsForArtist(artistID uint) ([]*ArtistFestivalListResponse, error)
}

// ReleaseServiceInterface defines the contract for release operations.
type ReleaseServiceInterface interface {
	CreateRelease(req *CreateReleaseRequest) (*ReleaseDetailResponse, error)
	GetRelease(releaseID uint) (*ReleaseDetailResponse, error)
	GetReleaseBySlug(slug string) (*ReleaseDetailResponse, error)
	ListReleases(filters map[string]interface{}) ([]*ReleaseListResponse, error)
	SearchReleases(query string) ([]*ReleaseListResponse, error)
	UpdateRelease(releaseID uint, req *UpdateReleaseRequest) (*ReleaseDetailResponse, error)
	DeleteRelease(releaseID uint) error
	GetReleasesForArtist(artistID uint) ([]*ReleaseListResponse, error)
	GetReleasesForArtistWithRoles(artistID uint) ([]*ArtistReleaseListResponse, error)
	AddExternalLink(releaseID uint, platform, url string) (*ReleaseExternalLinkResponse, error)
	RemoveExternalLink(linkID uint) error
}

// FetcherServiceInterface defines the contract for HTTP fetching with change detection.
type FetcherServiceInterface interface {
	Fetch(url string, lastETag string, lastContentHash string) (*FetchResult, error)
	FetchDynamic(url string) (*FetchResult, error)
	FetchScreenshot(url string) (*FetchResult, error)
	DetectRenderMethod(url string) (string, error)
}

// ContributorProfileServiceInterface defines the contract for contributor profile operations.
type ContributorProfileServiceInterface interface {
	GetPublicProfile(username string, viewerID *uint) (*PublicProfileResponse, error)
	GetOwnProfile(userID uint) (*PublicProfileResponse, error)
	GetContributionStats(userID uint) (*ContributionStats, error)
	GetContributionHistory(userID uint, limit, offset int, entityType string) ([]*ContributionEntry, int64, error)
	UpdatePrivacySettings(userID uint, settings PrivacySettings) (*PrivacySettings, error)
	GetUserSections(userID uint) ([]*ProfileSectionResponse, error)
	GetOwnSections(userID uint) ([]*ProfileSectionResponse, error)
	CreateSection(userID uint, title string, content string, position int) (*ProfileSectionResponse, error)
	UpdateSection(userID uint, sectionID uint, updates map[string]interface{}) (*ProfileSectionResponse, error)
	DeleteSection(userID uint, sectionID uint) error
	GetActivityHeatmap(userID uint) (*ActivityHeatmapResponse, error)
	GetPercentileRankings(userID uint) (*PercentileRankings, error)
}

// LeaderboardServiceInterface defines the contract for contributor leaderboard operations.
type LeaderboardServiceInterface interface {
	GetLeaderboard(dimension string, period string, limit int) ([]LeaderboardEntry, error)
	GetUserRank(userID uint, dimension string, period string) (*int, error)
}

// CalendarServiceInterface defines the contract for calendar feed operations.
type CalendarServiceInterface interface {
	CreateToken(userID uint, apiBaseURL string) (*CalendarTokenCreateResponse, error)
	GetTokenStatus(userID uint) (*CalendarTokenStatusResponse, error)
	DeleteToken(userID uint) error
	ValidateCalendarToken(plainToken string) (*models.User, error)
	GenerateICSFeed(userID uint, frontendURL string) ([]byte, error)
}

// PipelineServiceInterface defines the contract for the AI extraction pipeline orchestrator.
type PipelineServiceInterface interface {
	ExtractVenue(venueID uint, dryRun bool) (*PipelineResult, error)
}

// VenueSourceConfigServiceInterface defines the contract for venue source config operations.
type VenueSourceConfigServiceInterface interface {
	GetByVenueID(venueID uint) (*models.VenueSourceConfig, error)
	CreateOrUpdate(config *models.VenueSourceConfig) (*models.VenueSourceConfig, error)
	UpdateAfterRun(venueID uint, contentHash, etag *string, eventsExtracted int) error
	IncrementFailures(venueID uint) error
	RecordRun(run *models.VenueExtractionRun) error
	GetRecentRuns(venueID uint, limit int) ([]models.VenueExtractionRun, error)
	GetAllRecentRuns(limit, offset int) ([]ImportHistoryEntry, int64, error)
	ListConfigured() ([]models.VenueSourceConfig, error)
	GetRejectionStats(venueID uint) (*VenueRejectionStats, error)
	UpdateExtractionNotes(venueID uint, notes *string) error
	ResetRenderMethod(venueID uint) error
}

// SchedulerServiceInterface defines the contract for the background extraction scheduler.
type SchedulerServiceInterface interface {
	Start(ctx context.Context)
	Stop()
}

// EnrichmentServiceInterface defines the contract for post-import enrichment operations.
type EnrichmentServiceInterface interface {
	QueueShowForEnrichment(showID uint, enrichmentType string) error
	ProcessQueue(ctx context.Context, batchSize int) (int, error)
	EnrichShow(ctx context.Context, showID uint) (*EnrichmentResult, error)
	GetQueueStats() (*EnrichmentQueueStats, error)
}

// EnrichmentWorkerInterface defines the contract for the background enrichment worker.
type EnrichmentWorkerInterface interface {
	Start(ctx context.Context)
	Stop()
}

// RevisionServiceInterface defines the contract for revision history operations.
type RevisionServiceInterface interface {
	RecordRevision(entityType string, entityID uint, userID uint, changes []models.FieldChange, summary string) error
	GetEntityHistory(entityType string, entityID uint, limit, offset int) ([]models.Revision, int64, error)
	GetRevision(revisionID uint) (*models.Revision, error)
	GetUserRevisions(userID uint, limit, offset int) ([]models.Revision, int64, error)
	Rollback(revisionID uint, adminUserID uint) error
}

// SceneServiceInterface defines the contract for computed city scene aggregations.
type SceneServiceInterface interface {
	ListScenes() ([]*SceneListResponse, error)
	GetSceneDetail(city, state string) (*SceneDetailResponse, error)
	GetActiveArtists(city, state string, periodDays, limit, offset int) ([]*SceneArtistResponse, int64, error)
	ParseSceneSlug(slug string) (string, string, error)
	GetSceneGenreDistribution(city, state string) ([]GenreCount, error)
	GetGenreDiversityIndex(city, state string) (float64, error)
}

// DataQualityServiceInterface defines the contract for data quality dashboard operations.
type DataQualityServiceInterface interface {
	GetSummary() (*DataQualitySummary, error)
	GetCategoryItems(category string, limit, offset int) ([]*DataQualityItem, int64, error)
}

// AnalyticsServiceInterface defines the contract for platform analytics dashboard operations.
type AnalyticsServiceInterface interface {
	GetGrowthMetrics(months int) (*GrowthMetricsResponse, error)
	GetEngagementMetrics(months int) (*EngagementMetricsResponse, error)
	GetCommunityHealth() (*CommunityHealthResponse, error)
	GetDataQualityTrends(months int) (*DataQualityTrendsResponse, error)
}

// AttendanceServiceInterface defines the contract for show attendance (going/interested) operations.
type AttendanceServiceInterface interface {
	SetAttendance(userID, showID uint, status string) error
	RemoveAttendance(userID, showID uint) error
	GetUserAttendance(userID, showID uint) (string, error)
	GetAttendanceCounts(showID uint) (*AttendanceCountsResponse, error)
	GetBatchAttendanceCounts(showIDs []uint) (map[uint]*AttendanceCountsResponse, error)
	GetBatchUserAttendance(userID uint, showIDs []uint) (map[uint]string, error)
	GetUserAttendingShows(userID uint, status string, limit, offset int) ([]*AttendingShowResponse, int64, error)
}

// ChartsServiceInterface defines the contract for top charts / trending content.
type ChartsServiceInterface interface {
	GetTrendingShows(limit int) ([]TrendingShow, error)
	GetPopularArtists(limit int) ([]PopularArtist, error)
	GetActiveVenues(limit int) ([]ActiveVenue, error)
	GetHotReleases(limit int) ([]HotRelease, error)
	GetChartsOverview() (*ChartsOverview, error)
}

// FollowServiceInterface defines the contract for entity follow operations.
type FollowServiceInterface interface {
	Follow(userID uint, entityType string, entityID uint) error
	Unfollow(userID uint, entityType string, entityID uint) error
	IsFollowing(userID uint, entityType string, entityID uint) (bool, error)
	GetFollowerCount(entityType string, entityID uint) (int64, error)
	GetBatchFollowerCounts(entityType string, entityIDs []uint) (map[uint]int64, error)
	GetBatchUserFollowing(userID uint, entityType string, entityIDs []uint) (map[uint]bool, error)
	GetUserFollowing(userID uint, entityType string, limit, offset int) ([]*FollowingEntityResponse, int64, error)
	GetFollowers(entityType string, entityID uint, limit, offset int) ([]*FollowerResponse, int64, error)
}

// RadioServiceInterface defines the contract for radio station, show, episode, and play operations.
type RadioServiceInterface interface {
	// Station CRUD
	CreateStation(req *CreateRadioStationRequest) (*RadioStationDetailResponse, error)
	GetStation(stationID uint) (*RadioStationDetailResponse, error)
	GetStationBySlug(slug string) (*RadioStationDetailResponse, error)
	ListStations(filters map[string]interface{}) ([]*RadioStationListResponse, error)
	UpdateStation(stationID uint, req *UpdateRadioStationRequest) (*RadioStationDetailResponse, error)
	DeleteStation(stationID uint) error

	// Show CRUD
	CreateShow(stationID uint, req *CreateRadioShowRequest) (*RadioShowDetailResponse, error)
	GetShow(showID uint) (*RadioShowDetailResponse, error)
	GetShowBySlug(slug string) (*RadioShowDetailResponse, error)
	ListShows(stationID uint) ([]*RadioShowListResponse, error)
	UpdateShow(showID uint, req *UpdateRadioShowRequest) (*RadioShowDetailResponse, error)
	DeleteShow(showID uint) error

	// Episodes
	GetEpisodes(showID uint, limit, offset int) ([]*RadioEpisodeResponse, int64, error)
	GetEpisodeByShowAndDate(showID uint, airDate string) (*RadioEpisodeDetailResponse, error)
	GetEpisodeDetail(episodeID uint) (*RadioEpisodeDetailResponse, error)

	// Aggregation queries
	GetTopArtistsForShow(showID uint, periodDays, limit int) ([]*RadioTopArtistResponse, error)
	GetTopLabelsForShow(showID uint, periodDays, limit int) ([]*RadioTopLabelResponse, error)
	GetAsHeardOnForArtist(artistID uint) ([]*RadioAsHeardOnResponse, error)
	GetAsHeardOnForRelease(releaseID uint) ([]*RadioAsHeardOnResponse, error)
	GetNewReleaseRadar(stationID uint, limit int) ([]*RadioNewReleaseRadarEntry, error)

	// Stats
	GetRadioStats() (*RadioStatsResponse, error)

	// Import pipeline
	ImportStation(stationID uint, backfillDays int) (*RadioImportResult, error)
	FetchNewEpisodes(stationID uint) (*RadioImportResult, error)
	ImportEpisodePlaylist(showID uint, episodeExternalID string) (*EpisodeImportResult, error)

	// Matching
	MatchPlays(episodeID uint) (*MatchResult, error)
}
