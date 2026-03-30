package services

import (
	"log"
	"os"

	"gorm.io/gorm"

	"psychic-homily-backend/internal/config"
	adminsvc "psychic-homily-backend/internal/services/admin"
	"psychic-homily-backend/internal/services/auth"
	"psychic-homily-backend/internal/services/catalog"
	"psychic-homily-backend/internal/services/engagement"
	"psychic-homily-backend/internal/services/notification"
	"psychic-homily-backend/internal/services/pipeline"
	usersvc "psychic-homily-backend/internal/services/user"
)

// ServiceContainer eagerly creates all services once at startup.
// Exported fields — no getters needed for a simple data-holding struct.
type ServiceContainer struct {
	// DB-only leaf services
	AdminStats         *adminsvc.AdminStatsService
	Analytics          *adminsvc.AnalyticsService
	APIToken           *adminsvc.APITokenService
	DataQuality        *adminsvc.DataQualityService
	Revision           *adminsvc.RevisionService
	PendingEdit        *adminsvc.PendingEditService
	Charts             *catalog.ChartsService
	Artist             *catalog.ArtistService
	ContributorProfile *usersvc.ContributorProfileService
	ArtistReport  *adminsvc.ArtistReportService
	AuditLog      *adminsvc.AuditLogService
	Bookmark      *engagement.BookmarkService
	Calendar      *engagement.CalendarService
	Collection    *CollectionService
	Request       *RequestService
	Tag                *catalog.TagService
	ArtistRelationship *catalog.ArtistRelationshipService
	Scene              *catalog.SceneService
	Attendance         *engagement.AttendanceService
	Follow             *engagement.FollowService
	FavoriteVenue      *engagement.FavoriteVenueService
	Festival               *catalog.FestivalService
	FestivalIntelligence   *catalog.FestivalIntelligenceService
	Label         *catalog.LabelService
	Release       *catalog.ReleaseService
	SavedShow     *engagement.SavedShowService
	Show          *catalog.ShowService
	ShowReport    *adminsvc.ShowReportService
	EntityReport  *adminsvc.EntityReportService
	User              *usersvc.UserService
	Venue             *catalog.VenueService
	VenueSourceConfig *pipeline.VenueSourceConfigService

	// Config-only services
	Discord            *notification.DiscordService
	Email              *notification.EmailService
	NotificationFilter *notification.NotificationFilterService
	MusicDiscovery     *pipeline.MusicDiscoveryService

	// No-param services
	Fetcher           *pipeline.FetcherService
	PasswordValidator *auth.PasswordValidator

	// DB + Config composite services
	Auth       *auth.AuthService
	JWT        *auth.JWTService
	AppleAuth  *auth.AppleAuthService
	Extraction *pipeline.ExtractionService
	WebAuthn   *auth.WebAuthnService // nil if init fails (passkeys optional)
	Cleanup    *adminsvc.CleanupService
	DataSync   *adminsvc.DataSyncService
	Discovery        *pipeline.DiscoveryService
	Pipeline         *pipeline.PipelineService
	Reminder         *engagement.ReminderService
	Scheduler        *pipeline.SchedulerService
	Enrichment       *pipeline.EnrichmentService
	EnrichmentWorker *pipeline.EnrichmentWorker
	AutoPromotion    *adminsvc.AutoPromotionService
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
	userService := usersvc.NewUserService(database)

	// Services needed by PipelineService — created first so we can inject them.
	artist := catalog.NewArtistService(database)
	venue := catalog.NewVenueService(database)
	fetcher := newFetcherWithChromedp()
	extraction := pipeline.NewExtractionService(database, cfg, artist, venue)
	discovery := pipeline.NewDiscoveryService(database, venue)
	venueSourceConfig := pipeline.NewVenueSourceConfigService(database)

	// Auth services — created first so we can share the JWT service with AppleAuth.
	jwtService := auth.NewJWTService(database, cfg, userService)

	// Services needed by SchedulerService — created before the container.
	discord := notification.NewDiscordService(cfg)
	pipelineSvc := pipeline.NewPipelineService(fetcher, extraction, discovery, venueSourceConfig, venue)

	// Enrichment service — SeatGeek client ID is optional
	seatgeekClientID := os.Getenv("SEATGEEK_CLIENT_ID")
	enrichmentSvc := pipeline.NewEnrichmentService(database, artist, seatgeekClientID)
	enrichmentWorker := pipeline.NewEnrichmentWorker(enrichmentSvc)

	// Wire enrichment queuing into discovery service (fire-and-forget after imports)
	discovery.SetEnrichmentService(enrichmentSvc)

	revisionSvc := adminsvc.NewRevisionService(database)

	return &ServiceContainer{
		// DB-only leaf services
		AdminStats:         adminsvc.NewAdminStatsService(database),
		Analytics:          adminsvc.NewAnalyticsService(database),
		APIToken:           adminsvc.NewAPITokenService(database),
		DataQuality:        adminsvc.NewDataQualityService(database),
		Revision:           revisionSvc,
		PendingEdit:        adminsvc.NewPendingEditService(database, revisionSvc),
		Charts:             catalog.NewChartsService(database),
		Artist:             artist,
		ContributorProfile: usersvc.NewContributorProfileService(database),
		ArtistReport:  adminsvc.NewArtistReportService(database),
		AuditLog:      adminsvc.NewAuditLogService(database),
		Bookmark:      engagement.NewBookmarkService(database),
		Calendar:      engagement.NewCalendarService(database, savedShow),
		Collection:    NewCollectionService(database),
		Request:       NewRequestService(database),
		Tag:                catalog.NewTagService(database),
		ArtistRelationship: catalog.NewArtistRelationshipService(database),
		Scene:              catalog.NewSceneService(database),
		Attendance:         engagement.NewAttendanceService(database),
		Follow:             engagement.NewFollowService(database),
		FavoriteVenue:      engagement.NewFavoriteVenueService(database),
		Festival:               catalog.NewFestivalService(database),
		FestivalIntelligence:   catalog.NewFestivalIntelligenceService(database),
		Label:         catalog.NewLabelService(database),
		Release:       catalog.NewReleaseService(database),
		SavedShow:     savedShow,
		Show:          catalog.NewShowService(database),
		ShowReport:    adminsvc.NewShowReportService(database),
		EntityReport:  adminsvc.NewEntityReportService(database),
		User:          userService,
		Venue:             venue,
		VenueSourceConfig: venueSourceConfig,

		// Config-only services
		Discord:            discord,
		Email:              email,
		NotificationFilter: notification.NewNotificationFilterService(database, email, cfg.JWT.SecretKey, cfg.Email.FrontendURL),
		MusicDiscovery:     pipeline.NewMusicDiscoveryService(cfg),

		// No-param services
		Fetcher:           fetcher,
		PasswordValidator: auth.NewPasswordValidator(),

		// DB + Config composite services
		Auth:       auth.NewAuthService(database, cfg, userService),
		JWT:        jwtService,
		AppleAuth:  auth.NewAppleAuthService(database, cfg, jwtService),
		Extraction: extraction,
		WebAuthn:   webauthnService,
		Cleanup:    adminsvc.NewCleanupService(database, userService),
		DataSync:   adminsvc.NewDataSyncService(database),
		Discovery:  discovery,
		Pipeline:   pipelineSvc,
		Reminder:   engagement.NewReminderService(database, email, cfg),
		Scheduler:        pipeline.NewSchedulerService(database, pipelineSvc, venueSourceConfig, discord),
		Enrichment:       enrichmentSvc,
		EnrichmentWorker: enrichmentWorker,
		AutoPromotion:    adminsvc.NewAutoPromotionService(database),
	}
}
