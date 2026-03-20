package services

// This file re-exports all types from the contracts sub-package as type aliases,
// and concrete types from sub-packages (auth, engagement, pipeline) for backward
// compatibility. Existing code that references services.FooType continues to work.

import (
	"gorm.io/gorm"

	"psychic-homily-backend/internal/config"
	"psychic-homily-backend/internal/services/auth"
	"psychic-homily-backend/internal/services/contracts"
	usersvc "psychic-homily-backend/internal/services/user"
)

// ──────────────────────────────────────────────
// Auth concrete type aliases (from auth sub-package)
// ──────────────────────────────────────────────

type AuthService = auth.AuthService
type JWTService = auth.JWTService
type AppleAuthService = auth.AppleAuthService
type WebAuthnService = auth.WebAuthnService
type PasswordValidator = auth.PasswordValidator

// Backward-compatible constructor wrappers for auth sub-package.
// These maintain the old signatures so callers outside services/ keep compiling.

// NewAuthService creates an AuthService.
// Deprecated: prefer auth.NewAuthService with explicit userService dependency.
func NewAuthService(database *gorm.DB, cfg *config.Config) *auth.AuthService {
	return auth.NewAuthService(database, cfg, usersvc.NewUserService(database))
}

// NewJWTService creates a JWTService.
// Deprecated: prefer auth.NewJWTService with explicit userService dependency.
func NewJWTService(database *gorm.DB, cfg *config.Config) *auth.JWTService {
	return auth.NewJWTService(database, cfg, usersvc.NewUserService(database))
}

// NewAppleAuthService creates an AppleAuthService.
// Deprecated: prefer auth.NewAppleAuthService with explicit jwtService dependency.
func NewAppleAuthService(database *gorm.DB, cfg *config.Config) *auth.AppleAuthService {
	jwtSvc := auth.NewJWTService(database, cfg, usersvc.NewUserService(database))
	return auth.NewAppleAuthService(database, cfg, jwtSvc)
}

// NewPasswordValidator creates a PasswordValidator.
func NewPasswordValidator() *auth.PasswordValidator {
	return auth.NewPasswordValidator()
}

// NewWebAuthnService creates a WebAuthnService.
func NewWebAuthnService(database *gorm.DB, cfg *config.Config) (*auth.WebAuthnService, error) {
	return auth.NewWebAuthnService(database, cfg)
}

// CalculatePasswordStrength re-exports auth.CalculatePasswordStrength.
var CalculatePasswordStrength = auth.CalculatePasswordStrength

// GetStrengthLabel re-exports auth.GetStrengthLabel.
var GetStrengthLabel = auth.GetStrengthLabel

// MinPasswordLength re-exports auth.MinPasswordLength.
const (
	MinPasswordLength = auth.MinPasswordLength
	MaxPasswordLength = auth.MaxPasswordLength
)

// ──────────────────────────────────────────────
// Catalog types (show, venue, artist)
// ──────────────────────────────────────────────

type CreateShowVenue = contracts.CreateShowVenue
type CreateShowArtist = contracts.CreateShowArtist
type CreateShowRequest = contracts.CreateShowRequest
type ShowResponse = contracts.ShowResponse
type VenueResponse = contracts.VenueResponse
type ShowArtistSocials = contracts.ShowArtistSocials
type ArtistResponse = contracts.ArtistResponse
type BatchShowResult = contracts.BatchShowResult
type BatchShowError = contracts.BatchShowError
type PendingShowsFilter = contracts.PendingShowsFilter
type CityStateFilter = contracts.CityStateFilter
type UpcomingShowsFilter = contracts.UpcomingShowsFilter
type ShowCityResponse = contracts.ShowCityResponse
type OrphanedArtist = contracts.OrphanedArtist
type AdminShowFilters = contracts.AdminShowFilters
type ParsedShowImport = contracts.ParsedShowImport
type VenueMatchResult = contracts.VenueMatchResult
type ArtistMatchResult = contracts.ArtistMatchResult
type ImportPreviewResponse = contracts.ImportPreviewResponse
type ExportShowData = contracts.ExportShowData
type ExportVenueSocial = contracts.ExportVenueSocial
type ExportVenueData = contracts.ExportVenueData
type ExportArtistSocial = contracts.ExportArtistSocial
type ExportArtistData = contracts.ExportArtistData
type ExportFrontmatter = contracts.ExportFrontmatter

type CreateVenueRequest = contracts.CreateVenueRequest
type VenueDetailResponse = contracts.VenueDetailResponse
type VenueWithShowCountResponse = contracts.VenueWithShowCountResponse
type VenueListFilters = contracts.VenueListFilters
type VenueShowResponse = contracts.VenueShowResponse
type VenueCityResponse = contracts.VenueCityResponse
type VenueEditRequest = contracts.VenueEditRequest
type PendingVenueEditResponse = contracts.PendingVenueEditResponse
type UnverifiedVenueResponse = contracts.UnverifiedVenueResponse

type CreateArtistRequest = contracts.CreateArtistRequest
type ArtistDetailResponse = contracts.ArtistDetailResponse
type SocialResponse = contracts.SocialResponse
type ArtistWithShowCountResponse = contracts.ArtistWithShowCountResponse
type ArtistCityResponse = contracts.ArtistCityResponse
type ArtistLabelResponse = contracts.ArtistLabelResponse
type ArtistShowResponse = contracts.ArtistShowResponse
type ArtistShowVenueResponse = contracts.ArtistShowVenueResponse
type ArtistShowArtist = contracts.ArtistShowArtist
type ArtistAliasResponse = contracts.ArtistAliasResponse
type MergeArtistResult = contracts.MergeArtistResult

// ──────────────────────────────────────────────
// Scene types
// ──────────────────────────────────────────────

type SceneListResponse = contracts.SceneListResponse
type SceneDetailResponse = contracts.SceneDetailResponse
type SceneStats = contracts.SceneStats
type ScenePulse = contracts.ScenePulse
type SceneArtistResponse = contracts.SceneArtistResponse
type GenreCount = contracts.GenreCount
type SceneGenreResponse = contracts.SceneGenreResponse
type VenueGenreResponse = contracts.VenueGenreResponse

// ──────────────────────────────────────────────
// Label types
// ──────────────────────────────────────────────

type CreateLabelRequest = contracts.CreateLabelRequest
type UpdateLabelRequest = contracts.UpdateLabelRequest
type LabelDetailResponse = contracts.LabelDetailResponse
type LabelListResponse = contracts.LabelListResponse
type LabelArtistResponse = contracts.LabelArtistResponse
type LabelReleaseResponse = contracts.LabelReleaseResponse

// ──────────────────────────────────────────────
// Release types
// ──────────────────────────────────────────────

type CreateReleaseRequest = contracts.CreateReleaseRequest
type CreateReleaseArtistEntry = contracts.CreateReleaseArtistEntry
type CreateReleaseLinkEntry = contracts.CreateReleaseLinkEntry
type UpdateReleaseRequest = contracts.UpdateReleaseRequest
type ReleaseDetailResponse = contracts.ReleaseDetailResponse
type ReleaseArtistResponse = contracts.ReleaseArtistResponse
type ReleaseExternalLinkResponse = contracts.ReleaseExternalLinkResponse
type ReleaseListResponse = contracts.ReleaseListResponse
type ArtistReleaseListResponse = contracts.ArtistReleaseListResponse

// ──────────────────────────────────────────────
// Festival types
// ──────────────────────────────────────────────

type CreateFestivalRequest = contracts.CreateFestivalRequest
type UpdateFestivalRequest = contracts.UpdateFestivalRequest
type FestivalDetailResponse = contracts.FestivalDetailResponse
type FestivalListResponse = contracts.FestivalListResponse
type FestivalArtistResponse = contracts.FestivalArtistResponse
type FestivalVenueResponse = contracts.FestivalVenueResponse
type ArtistFestivalListResponse = contracts.ArtistFestivalListResponse
type AddFestivalArtistRequest = contracts.AddFestivalArtistRequest
type UpdateFestivalArtistRequest = contracts.UpdateFestivalArtistRequest
type AddFestivalVenueRequest = contracts.AddFestivalVenueRequest

// Festival intelligence types
type FestivalSummary = contracts.FestivalSummary
type ArtistSummary = contracts.ArtistSummary
type SimilarFestival = contracts.SimilarFestival
type SharedArtist = contracts.SharedArtist
type FestivalOverlap = contracts.FestivalOverlap
type FestivalBreakouts = contracts.FestivalBreakouts
type ArtistBreakout = contracts.ArtistBreakout
type TrajectoryEntry = contracts.TrajectoryEntry
type ArtistMilestone = contracts.ArtistMilestone
type ArtistTrajectory = contracts.ArtistTrajectory
type SeriesComparison = contracts.SeriesComparison
type SeriesEdition = contracts.SeriesEdition
type ReturningArtist = contracts.ReturningArtist
type SeriesNewcomer = contracts.SeriesNewcomer

// ──────────────────────────────────────────────
// Collection types
// ──────────────────────────────────────────────

type CreateCollectionRequest = contracts.CreateCollectionRequest
type UpdateCollectionRequest = contracts.UpdateCollectionRequest
type AddCollectionItemRequest = contracts.AddCollectionItemRequest
type ReorderCollectionItemsRequest = contracts.ReorderCollectionItemsRequest
type ReorderItem = contracts.ReorderItem
type CollectionFilters = contracts.CollectionFilters
type CollectionDetailResponse = contracts.CollectionDetailResponse
type CollectionListResponse = contracts.CollectionListResponse
type CollectionItemResponse = contracts.CollectionItemResponse
type CollectionStatsResponse = contracts.CollectionStatsResponse

// ──────────────────────────────────────────────
// Request types
// ──────────────────────────────────────────────

type RequestResponse = contracts.RequestResponse

// ──────────────────────────────────────────────
// Auth / JWT / Apple / Password types
// ──────────────────────────────────────────────

type OAuthCompleter = contracts.OAuthCompleter
type VerificationTokenClaims = contracts.VerificationTokenClaims
type MagicLinkTokenClaims = contracts.MagicLinkTokenClaims
type AccountRecoveryTokenClaims = contracts.AccountRecoveryTokenClaims
type AppleIdentityTokenClaims = contracts.AppleIdentityTokenClaims
type PasswordValidationResult = contracts.PasswordValidationResult
type LegalAcceptance = contracts.LegalAcceptance
type OAuthSignupConsent = contracts.OAuthSignupConsent

// ──────────────────────────────────────────────
// User / Contributor Profile types
// ──────────────────────────────────────────────

type AdminUserFilters = contracts.AdminUserFilters
type UserSubmissionStats = contracts.UserSubmissionStats
type AdminUserResponse = contracts.AdminUserResponse
type DeletionSummary = contracts.DeletionSummary
type UserDataExport = contracts.UserDataExport
type UserProfileExport = contracts.UserProfileExport
type UserPreferencesExport = contracts.UserPreferencesExport
type OAuthAccountExport = contracts.OAuthAccountExport
type PasskeyExport = contracts.PasskeyExport
type SavedShowExport = contracts.SavedShowExport
type SubmittedShowExport = contracts.SubmittedShowExport

type PrivacyLevel = contracts.PrivacyLevel
type PrivacySettings = contracts.PrivacySettings
type ContributionStats = contracts.ContributionStats
type PublicProfileResponse = contracts.PublicProfileResponse
type ProfileSectionResponse = contracts.ProfileSectionResponse
type ContributionEntry = contracts.ContributionEntry

// PrivacyLevel constants — Go cannot alias const, so we re-export.
const (
	PrivacyVisible   = contracts.PrivacyVisible
	PrivacyCountOnly = contracts.PrivacyCountOnly
	PrivacyHidden    = contracts.PrivacyHidden
)

// DefaultPrivacySettings re-exported via var (Go cannot alias functions).
var DefaultPrivacySettings = contracts.DefaultPrivacySettings

// ──────────────────────────────────────────────
// Engagement types (saved shows, favorites, reports, calendar)
// ──────────────────────────────────────────────

type SavedShowResponse = contracts.SavedShowResponse
type FavoriteVenueResponse = contracts.FavoriteVenueResponse
type FavoriteVenueShowResponse = contracts.FavoriteVenueShowResponse
type ShowReportResponse = contracts.ShowReportResponse
type ShowReportShowInfo = contracts.ShowReportShowInfo
type ArtistReportResponse = contracts.ArtistReportResponse
type ArtistReportArtistInfo = contracts.ArtistReportArtistInfo
type CalendarTokenCreateResponse = contracts.CalendarTokenCreateResponse
type CalendarTokenStatusResponse = contracts.CalendarTokenStatusResponse
type AttendanceCountsResponse = contracts.AttendanceCountsResponse
type AttendingShowResponse = contracts.AttendingShowResponse
type FollowingEntityResponse = contracts.FollowingEntityResponse
type FollowerResponse = contracts.FollowerResponse
type FollowStatusResponse = contracts.FollowStatusResponse

// ──────────────────────────────────────────────
// Notification types (audit log, notification filters)
// ──────────────────────────────────────────────

type AuditLogFilters = contracts.AuditLogFilters
type AuditLogResponse = contracts.AuditLogResponse
type CreateFilterInput = contracts.CreateFilterInput
type UpdateFilterInput = contracts.UpdateFilterInput
type NotificationFilterResponse = contracts.NotificationFilterResponse
type NotificationLogEntry = contracts.NotificationLogEntry

// ──────────────────────────────────────────────
// Admin types (stats, API tokens, data sync)
// ──────────────────────────────────────────────

type AdminDashboardStats = contracts.AdminDashboardStats
type ActivityEvent = contracts.ActivityEvent
type ActivityFeedResponse = contracts.ActivityFeedResponse
type APITokenResponse = contracts.APITokenResponse
type APITokenCreateResponse = contracts.APITokenCreateResponse
type ExportedArtist = contracts.ExportedArtist
type ExportedVenue = contracts.ExportedVenue
type ExportedShowArtist = contracts.ExportedShowArtist
type ExportedShow = contracts.ExportedShow
type ExportShowsParams = contracts.ExportShowsParams
type ExportShowsResult = contracts.ExportShowsResult
type ExportArtistsParams = contracts.ExportArtistsParams
type ExportArtistsResult = contracts.ExportArtistsResult
type ExportVenuesParams = contracts.ExportVenuesParams
type ExportVenuesResult = contracts.ExportVenuesResult
type DataImportRequest = contracts.DataImportRequest
type DataImportResult = contracts.DataImportResult

// ──────────────────────────────────────────────
// Analytics types
// ──────────────────────────────────────────────

type MonthlyCount = contracts.MonthlyCount
type GrowthMetricsResponse = contracts.GrowthMetricsResponse
type EngagementMetric = contracts.EngagementMetric
type EngagementMetricsResponse = contracts.EngagementMetricsResponse
type TopContributor = contracts.TopContributor
type WeeklyContributions = contracts.WeeklyContributions
type CommunityHealthResponse = contracts.CommunityHealthResponse
type DataQualityTrendsResponse = contracts.DataQualityTrendsResponse

// ──────────────────────────────────────────────
// Charts types
// ──────────────────────────────────────────────

type TrendingShow = contracts.TrendingShow
type PopularArtist = contracts.PopularArtist
type ActiveVenue = contracts.ActiveVenue
type HotRelease = contracts.HotRelease
type ChartsOverview = contracts.ChartsOverview

// ──────────────────────────────────────────────
// Pipeline types (extraction, fetcher, discovery, pipeline)
// ──────────────────────────────────────────────

type ExtractShowRequest = contracts.ExtractShowRequest
type MatchSuggestion = contracts.MatchSuggestion
type VenueMatchSuggestion = contracts.VenueMatchSuggestion
type ExtractedArtist = contracts.ExtractedArtist
type ExtractedVenue = contracts.ExtractedVenue
type ExtractedShowData = contracts.ExtractedShowData
type ExtractShowResponse = contracts.ExtractShowResponse

type CalendarEvent = contracts.CalendarEvent
type CalendarArtist = contracts.CalendarArtist
type CalendarExtractionResponse = contracts.CalendarExtractionResponse

type FetchResult = contracts.FetchResult
type FetchError = contracts.FetchError

type PipelineResult = contracts.PipelineResult
type VenueRejectionStats = contracts.VenueRejectionStats
type ImportHistoryEntry = contracts.ImportHistoryEntry

type DiscoveredEvent = contracts.DiscoveredEvent
type ImportResult = contracts.ImportResult
type CheckEventInput = contracts.CheckEventInput
type ShowCurrentData = contracts.ShowCurrentData
type CheckEventStatus = contracts.CheckEventStatus
type CheckEventsResult = contracts.CheckEventsResult

type EnrichmentResult = contracts.EnrichmentResult
type ArtistMatchEnrichment = contracts.ArtistMatchEnrichment
type MBEnrichment = contracts.MBEnrichment
type SeatGeekEnrichment = contracts.SeatGeekEnrichment
type EnrichmentQueueStats = contracts.EnrichmentQueueStats

// IsFetchError re-exported via var (Go cannot alias functions).
var IsFetchError = contracts.IsFetchError

// CalendarEventsToDiscoveredEvents is defined in the pipeline sub-package
// (internal/services/pipeline/extraction_calendar.go). Use pipeline.CalendarEventsToDiscoveredEvents.

// RenderMethod constants — Go cannot alias const, so we re-export.
const (
	RenderMethodStatic     = contracts.RenderMethodStatic
	RenderMethodDynamic    = contracts.RenderMethodDynamic
	RenderMethodScreenshot = contracts.RenderMethodScreenshot
)
