package services

import (
	"log"

	"gorm.io/gorm"

	"psychic-homily-backend/internal/config"
	"psychic-homily-backend/internal/services/auth"
	"psychic-homily-backend/internal/services/catalog"
	"psychic-homily-backend/internal/services/engagement"
	"psychic-homily-backend/internal/services/notification"
	"psychic-homily-backend/internal/services/pipeline"
)

// ServiceContainer eagerly creates all services once at startup.
// Exported fields — no getters needed for a simple data-holding struct.
type ServiceContainer struct {
	// DB-only leaf services
	AdminStats         *AdminStatsService
	APIToken           *APITokenService
	Artist             *catalog.ArtistService
	ContributorProfile *ContributorProfileService
	ArtistReport  *ArtistReportService
	AuditLog      *AuditLogService
	Bookmark      *engagement.BookmarkService
	Calendar      *engagement.CalendarService
	Collection    *CollectionService
	FavoriteVenue *engagement.FavoriteVenueService
	Festival      *catalog.FestivalService
	Label         *catalog.LabelService
	Release       *catalog.ReleaseService
	SavedShow     *engagement.SavedShowService
	Show          *catalog.ShowService
	ShowReport    *ShowReportService
	User              *UserService
	Venue             *catalog.VenueService
	VenueSourceConfig *pipeline.VenueSourceConfigService

	// Config-only services
	Discord        *notification.DiscordService
	Email          *notification.EmailService
	MusicDiscovery *pipeline.MusicDiscoveryService

	// No-param services
	Fetcher           *pipeline.FetcherService
	PasswordValidator *auth.PasswordValidator

	// DB + Config composite services
	Auth       *auth.AuthService
	JWT        *auth.JWTService
	AppleAuth  *auth.AppleAuthService
	Extraction *pipeline.ExtractionService
	WebAuthn   *auth.WebAuthnService // nil if init fails (passkeys optional)
	Cleanup    *CleanupService
	DataSync   *DataSyncService
	Discovery  *pipeline.DiscoveryService
	Pipeline   *pipeline.PipelineService
	Reminder   *engagement.ReminderService
}

// newFetcherWithChromedp creates a FetcherService with chromedp initialized at 3 workers.
func newFetcherWithChromedp() *pipeline.FetcherService {
	f := pipeline.NewFetcherService()
	f.InitChromedp(3)
	return f
}

// NewServiceContainer creates all services once. WebAuthn failure is non-fatal
// (passkeys are optional) — all other services are infallible constructors.
func NewServiceContainer(database *gorm.DB, cfg *config.Config) *ServiceContainer {
	// WebAuthn may fail — log warning, store nil
	webauthnService, err := auth.NewWebAuthnService(database, cfg)
	if err != nil {
		log.Printf("Warning: WebAuthn service init failed (passkeys disabled): %v", err)
	}

	savedShow := engagement.NewSavedShowService(database)
	email := notification.NewEmailService(cfg)
	userService := NewUserService(database)

	// Services needed by PipelineService — created first so we can inject them.
	artist := catalog.NewArtistService(database)
	venue := catalog.NewVenueService(database)
	fetcher := newFetcherWithChromedp()
	extraction := pipeline.NewExtractionService(database, cfg, artist, venue)
	discovery := pipeline.NewDiscoveryService(database, venue)
	venueSourceConfig := pipeline.NewVenueSourceConfigService(database)

	// Auth services — created first so we can share the JWT service with AppleAuth.
	jwtService := auth.NewJWTService(database, cfg, userService)

	return &ServiceContainer{
		// DB-only leaf services
		AdminStats:         NewAdminStatsService(database),
		APIToken:           NewAPITokenService(database),
		Artist:             artist,
		ContributorProfile: NewContributorProfileService(database),
		ArtistReport:  NewArtistReportService(database),
		AuditLog:      NewAuditLogService(database),
		Bookmark:      engagement.NewBookmarkService(database),
		Calendar:      engagement.NewCalendarService(database, savedShow),
		Collection:    NewCollectionService(database),
		FavoriteVenue: engagement.NewFavoriteVenueService(database),
		Festival:      catalog.NewFestivalService(database),
		Label:         catalog.NewLabelService(database),
		Release:       catalog.NewReleaseService(database),
		SavedShow:     savedShow,
		Show:          catalog.NewShowService(database),
		ShowReport:    NewShowReportService(database),
		User:          userService,
		Venue:             venue,
		VenueSourceConfig: venueSourceConfig,

		// Config-only services
		Discord:        notification.NewDiscordService(cfg),
		Email:          email,
		MusicDiscovery: pipeline.NewMusicDiscoveryService(cfg),

		// No-param services
		Fetcher:           fetcher,
		PasswordValidator: auth.NewPasswordValidator(),

		// DB + Config composite services
		Auth:       auth.NewAuthService(database, cfg, userService),
		JWT:        jwtService,
		AppleAuth:  auth.NewAppleAuthService(database, cfg, jwtService),
		Extraction: extraction,
		WebAuthn:   webauthnService,
		Cleanup:    NewCleanupService(database),
		DataSync:   NewDataSyncService(database),
		Discovery:  discovery,
		Pipeline:   pipeline.NewPipelineService(fetcher, extraction, discovery, venueSourceConfig, venue),
		Reminder:   engagement.NewReminderService(database, email, cfg),
	}
}
