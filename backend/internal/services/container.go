package services

import (
	"log"

	"gorm.io/gorm"

	"psychic-homily-backend/internal/config"
)

// ServiceContainer eagerly creates all services once at startup.
// Exported fields — no getters needed for a simple data-holding struct.
type ServiceContainer struct {
	// DB-only leaf services
	AdminStats         *AdminStatsService
	APIToken           *APITokenService
	Artist             *ArtistService
	ContributorProfile *ContributorProfileService
	ArtistReport  *ArtistReportService
	AuditLog      *AuditLogService
	Bookmark      *BookmarkService
	Calendar      *CalendarService
	FavoriteVenue *FavoriteVenueService
	Festival      *FestivalService
	Label         *LabelService
	Release       *ReleaseService
	SavedShow     *SavedShowService
	Show          *ShowService
	ShowReport    *ShowReportService
	User              *UserService
	Venue             *VenueService
	VenueSourceConfig *VenueSourceConfigService

	// Config-only services
	Discord        *DiscordService
	Email          *EmailService
	MusicDiscovery *MusicDiscoveryService

	// No-param service
	PasswordValidator *PasswordValidator

	// DB + Config composite services
	Auth       *AuthService
	JWT        *JWTService
	AppleAuth  *AppleAuthService
	Extraction *ExtractionService
	WebAuthn   *WebAuthnService // nil if init fails (passkeys optional)
	Cleanup    *CleanupService
	DataSync   *DataSyncService
	Discovery  *DiscoveryService
	Reminder   *ReminderService
}

// NewServiceContainer creates all services once. WebAuthn failure is non-fatal
// (passkeys are optional) — all other services are infallible constructors.
func NewServiceContainer(database *gorm.DB, cfg *config.Config) *ServiceContainer {
	// WebAuthn may fail — log warning, store nil
	webauthnService, err := NewWebAuthnService(database, cfg)
	if err != nil {
		log.Printf("Warning: WebAuthn service init failed (passkeys disabled): %v", err)
	}

	savedShow := NewSavedShowService(database)
	email := NewEmailService(cfg)

	return &ServiceContainer{
		// DB-only leaf services
		AdminStats:         NewAdminStatsService(database),
		APIToken:           NewAPITokenService(database),
		Artist:             NewArtistService(database),
		ContributorProfile: NewContributorProfileService(database),
		ArtistReport:  NewArtistReportService(database),
		AuditLog:      NewAuditLogService(database),
		Bookmark:      NewBookmarkService(database),
		Calendar:      NewCalendarService(database, savedShow),
		FavoriteVenue: NewFavoriteVenueService(database),
		Festival:      NewFestivalService(database),
		Label:         NewLabelService(database),
		Release:       NewReleaseService(database),
		SavedShow:     savedShow,
		Show:          NewShowService(database),
		ShowReport:    NewShowReportService(database),
		User:          NewUserService(database),
		Venue:             NewVenueService(database),
		VenueSourceConfig: NewVenueSourceConfigService(database),

		// Config-only services
		Discord:        NewDiscordService(cfg),
		Email:          email,
		MusicDiscovery: NewMusicDiscoveryService(cfg),

		// No-param service
		PasswordValidator: NewPasswordValidator(),

		// DB + Config composite services
		Auth:       NewAuthService(database, cfg),
		JWT:        NewJWTService(database, cfg),
		AppleAuth:  NewAppleAuthService(database, cfg),
		Extraction: NewExtractionService(database, cfg),
		WebAuthn:   webauthnService,
		Cleanup:    NewCleanupService(database),
		DataSync:   NewDataSyncService(database),
		Discovery:  NewDiscoveryService(database),
		Reminder:   NewReminderService(database, email, cfg),
	}
}
