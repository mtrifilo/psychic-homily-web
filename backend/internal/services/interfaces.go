package services

import (
	"psychic-homily-backend/internal/services/catalog"
	"psychic-homily-backend/internal/services/contracts"
)

// Interface aliases — every interface now lives in contracts;
// these aliases keep existing code compiling unchanged.

type ShowServiceInterface = contracts.ShowServiceInterface
type VenueServiceInterface = contracts.VenueServiceInterface
type ArtistServiceInterface = contracts.ArtistServiceInterface
type SavedShowServiceInterface = contracts.SavedShowServiceInterface
type FavoriteVenueServiceInterface = contracts.FavoriteVenueServiceInterface
type BookmarkServiceInterface = contracts.BookmarkServiceInterface
type ShowReportServiceInterface = contracts.ShowReportServiceInterface
type ArtistReportServiceInterface = contracts.ArtistReportServiceInterface
type AuditLogServiceInterface = contracts.AuditLogServiceInterface
type AuthServiceInterface = contracts.AuthServiceInterface
type JWTServiceInterface = contracts.JWTServiceInterface
type UserServiceInterface = contracts.UserServiceInterface
type EmailServiceInterface = contracts.EmailServiceInterface
type ReminderServiceInterface = contracts.ReminderServiceInterface
type DiscordServiceInterface = contracts.DiscordServiceInterface
type PasswordValidatorInterface = contracts.PasswordValidatorInterface
type ExtractionServiceInterface = contracts.ExtractionServiceInterface
type MusicDiscoveryServiceInterface = contracts.MusicDiscoveryServiceInterface
type AppleAuthServiceInterface = contracts.AppleAuthServiceInterface
type WebAuthnServiceInterface = contracts.WebAuthnServiceInterface
type DiscoveryServiceInterface = contracts.DiscoveryServiceInterface
type APITokenServiceInterface = contracts.APITokenServiceInterface
type DataSyncServiceInterface = contracts.DataSyncServiceInterface
type AdminStatsServiceInterface = contracts.AdminStatsServiceInterface
type LabelServiceInterface = contracts.LabelServiceInterface
type FestivalServiceInterface = contracts.FestivalServiceInterface
type ReleaseServiceInterface = contracts.ReleaseServiceInterface
type FetcherServiceInterface = contracts.FetcherServiceInterface
type ContributorProfileServiceInterface = contracts.ContributorProfileServiceInterface
type CalendarServiceInterface = contracts.CalendarServiceInterface
type PipelineServiceInterface = contracts.PipelineServiceInterface
type VenueSourceConfigServiceInterface = contracts.VenueSourceConfigServiceInterface
type CollectionServiceInterface = contracts.CollectionServiceInterface

// Compile-time interface satisfaction checks.
// Engagement services (Bookmark, SavedShow, FavoriteVenue, Calendar, Reminder)
// have their checks in internal/services/engagement/interfaces.go.
// Pipeline services (Extraction, MusicDiscovery, Discovery, VenueSourceConfig,
// Fetcher, Pipeline) are checked in internal/services/pipeline/interfaces.go.
// Auth services (Auth, JWT, PasswordValidator, AppleAuth, WebAuthn)
// are checked in internal/services/auth/interfaces.go.
// Notification services (Email, Discord) are checked in
// internal/services/notification/interfaces.go.
// User services (UserService, ContributorProfileService) are checked in
// internal/services/user/interfaces.go.
// Admin services (AdminStats, AuditLog, DataSync, ShowReport, ArtistReport,
// APIToken) are checked in internal/services/admin/interfaces.go.
var (
	_ ShowServiceInterface          = (*catalog.ShowService)(nil)
	_ VenueServiceInterface         = (*catalog.VenueService)(nil)
	_ ArtistServiceInterface        = (*catalog.ArtistService)(nil)
	_ FestivalServiceInterface       = (*catalog.FestivalService)(nil)
	_ LabelServiceInterface          = (*catalog.LabelService)(nil)
	_ ReleaseServiceInterface       = (*catalog.ReleaseService)(nil)
	_ CollectionServiceInterface            = (*CollectionService)(nil)
)
