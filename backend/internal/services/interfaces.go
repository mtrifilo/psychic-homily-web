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

// Compile-time interface satisfaction checks.
var (
	_ ShowServiceInterface          = (*catalog.ShowService)(nil)
	_ VenueServiceInterface         = (*catalog.VenueService)(nil)
	_ ArtistServiceInterface        = (*catalog.ArtistService)(nil)
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
	_ CalendarServiceInterface      = (*CalendarService)(nil)
	_ ReminderServiceInterface      = (*ReminderService)(nil)
	_ FestivalServiceInterface       = (*catalog.FestivalService)(nil)
	_ LabelServiceInterface          = (*catalog.LabelService)(nil)
	_ ReleaseServiceInterface       = (*catalog.ReleaseService)(nil)
	_ BookmarkServiceInterface              = (*BookmarkService)(nil)
	_ ContributorProfileServiceInterface    = (*ContributorProfileService)(nil)
	_ VenueSourceConfigServiceInterface     = (*VenueSourceConfigService)(nil)
	_ FetcherServiceInterface               = (*FetcherService)(nil)
	_ PipelineServiceInterface              = (*PipelineService)(nil)
)
