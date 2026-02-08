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
	AdminStats    *AdminStatsService
	APIToken      *APITokenService
	Artist        *ArtistService
	AuditLog      *AuditLogService
	FavoriteVenue *FavoriteVenueService
	SavedShow     *SavedShowService
	Show          *ShowService
	ShowReport    *ShowReportService
	User          *UserService
	Venue         *VenueService

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
}

// NewServiceContainer creates all services once. WebAuthn failure is non-fatal
// (passkeys are optional) — all other services are infallible constructors.
func NewServiceContainer(database *gorm.DB, cfg *config.Config) *ServiceContainer {
	// WebAuthn may fail — log warning, store nil
	webauthnService, err := NewWebAuthnService(database, cfg)
	if err != nil {
		log.Printf("Warning: WebAuthn service init failed (passkeys disabled): %v", err)
	}

	return &ServiceContainer{
		// DB-only leaf services
		AdminStats:    NewAdminStatsService(database),
		APIToken:      NewAPITokenService(database),
		Artist:        NewArtistService(database),
		AuditLog:      NewAuditLogService(database),
		FavoriteVenue: NewFavoriteVenueService(database),
		SavedShow:     NewSavedShowService(database),
		Show:          NewShowService(database),
		ShowReport:    NewShowReportService(database),
		User:          NewUserService(database),
		Venue:         NewVenueService(database),

		// Config-only services
		Discord:        NewDiscordService(cfg),
		Email:          NewEmailService(cfg),
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
	}
}
