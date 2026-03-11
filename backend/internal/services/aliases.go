package services

// This file re-exports all types from the contracts sub-package as type aliases.
// Existing code that references services.FooType continues to work unchanged.

import "psychic-homily-backend/internal/services/contracts"

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

// ──────────────────────────────────────────────
// Notification types (audit log)
// ──────────────────────────────────────────────

type AuditLogFilters = contracts.AuditLogFilters
type AuditLogResponse = contracts.AuditLogResponse

// ──────────────────────────────────────────────
// Admin types (stats, API tokens, data sync)
// ──────────────────────────────────────────────

type AdminDashboardStats = contracts.AdminDashboardStats
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

type DiscoveredEvent = contracts.DiscoveredEvent
type ImportResult = contracts.ImportResult
type CheckEventInput = contracts.CheckEventInput
type ShowCurrentData = contracts.ShowCurrentData
type CheckEventStatus = contracts.CheckEventStatus
type CheckEventsResult = contracts.CheckEventsResult

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
