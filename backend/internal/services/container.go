package services

import (
	"log"
	"os"

	"gorm.io/gorm"

	"psychic-homily-backend/internal/config"
	adminsvc "psychic-homily-backend/internal/services/admin"
	"psychic-homily-backend/internal/services/auth"
	"psychic-homily-backend/internal/services/catalog"
	"psychic-homily-backend/internal/services/community"
	"psychic-homily-backend/internal/services/engagement"
	exploresvc "psychic-homily-backend/internal/services/explore"
	"psychic-homily-backend/internal/services/imageenrich"
	"psychic-homily-backend/internal/services/notification"
	"psychic-homily-backend/internal/services/pipeline"
	"psychic-homily-backend/internal/services/ratelimit"
	"psychic-homily-backend/internal/services/sourceregistry"
	usersvc "psychic-homily-backend/internal/services/user"
	"psychic-homily-backend/internal/utils"
)

// ServiceContainer eagerly creates all services once at startup.
// Exported fields — no getters needed for a simple data-holding struct.
type ServiceContainer struct {
	// DB-only leaf services
	AdminStats             *adminsvc.AdminStatsService
	Analytics              *adminsvc.AnalyticsService
	APIToken               *adminsvc.APITokenService
	DataQuality            *adminsvc.DataQualityService
	Revision               *adminsvc.RevisionService
	PendingEdit            *adminsvc.PendingEditService
	Charts                 *catalog.ChartsService
	Artist                 *catalog.ArtistService
	ContributorProfile     *usersvc.ContributorProfileService
	ArtistReport           *adminsvc.ArtistReportService
	AuditLog               *adminsvc.AuditLogService
	FeaturedSlot           *adminsvc.FeaturedSlotService
	Explore                *exploresvc.ExploreService
	EntityExistence        *catalog.EntityExistenceService
	Bookmark               *engagement.BookmarkService
	Calendar               *engagement.CalendarService
	Collection             *community.CollectionService
	Request                *community.RequestService
	EntityRequest          *community.EntityRequestService
	EntityRequestFulfiller *community.EntityRequestFulfiller
	Tag                    *catalog.TagService
	ArtistRelationship     *catalog.ArtistRelationshipService
	Scene                  *catalog.SceneService
	Attendance             *engagement.AttendanceService
	Comment                *engagement.CommentService
	CommentVote            *engagement.CommentVoteService
	CommentSubscription    *engagement.CommentSubscriptionService
	CommentNotification    *engagement.CommentNotificationService
	Follow                 *engagement.FollowService
	Festival               *catalog.FestivalService
	FestivalIntelligence   *catalog.FestivalIntelligenceService
	Label                  *catalog.LabelService
	Release                *catalog.ReleaseService
	SavedShow              *engagement.SavedShowService
	Show                   *catalog.ShowService
	ShowReport             *adminsvc.ShowReportService
	EntityReport           *adminsvc.EntityReportService
	User                   *usersvc.UserService
	Leaderboard            *usersvc.LeaderboardService
	Radio                  *catalog.RadioService
	RadioFetch             *catalog.RadioFetchService
	RelationshipDerivation *catalog.RelationshipDerivationService
	Venue                  *catalog.VenueService
	SourceConfig           *sourceregistry.SourceConfigService
	AIExtractionThrottle   *ratelimit.AIExtractionThrottleService
	StreamingWorklist      *pipeline.StreamingWorklistService
	DiscoverMusic          *pipeline.DiscoverMusicService
	LinkSuggestion         *pipeline.LinkSuggestionService

	// Config-only services
	Discord            *notification.DiscordService
	Email              *notification.EmailService
	NotificationFilter *notification.NotificationFilterService

	// No-param services
	PasswordValidator *auth.PasswordValidator

	// DB + Config composite services
	Auth             *auth.AuthService
	JWT              *auth.JWTService
	AppleAuth        *auth.AppleAuthService
	Extraction       *pipeline.ExtractionService
	WebAuthn         *auth.WebAuthnService // nil if init fails (passkeys optional)
	Cleanup          *adminsvc.CleanupService
	DataSync         *adminsvc.DataSyncService
	Discovery        *pipeline.DiscoveryService
	Reminder         *engagement.ReminderService
	Enrichment       *pipeline.EnrichmentService
	EnrichmentWorker *pipeline.EnrichmentWorker
	ImageEnrichSweep *imageenrich.ImageEnrichmentSweep
	AutoPromotion    *adminsvc.AutoPromotionService
	// PSY-350: weekly collection-subscription digest emails (opt-IN).
	CollectionDigest *engagement.CollectionDigestService
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

	// Shared catalog services. extraction backs the ShowHandler AI
	// show-from-text path; discovery powers the external discovery-app import.
	artist := catalog.NewArtistService(database)
	venue := catalog.NewVenueService(database)
	extraction := pipeline.NewExtractionService(database, cfg, artist, venue)
	discovery := pipeline.NewDiscoveryService(database, venue)
	sourceConfig := sourceregistry.NewSourceConfigService(database)

	// Auth services — created first so we can share the JWT service with AppleAuth.
	jwtService := auth.NewJWTService(database, cfg, userService)

	discord := notification.NewDiscordService(cfg)

	// PSY-1208: ONE shared MusicBrainz client across discovery + enrichment.
	// MusicBrainz blocks for exceeding ~1 req/s/IP; two independent clients (one
	// per service) each have their own throttle and could combine for ~2 req/s.
	// Injecting the same instance into both services routes ALL MB calls through
	// a single mutex-serialized throttle, enforcing a true ~1 req/s process-wide.
	mbClient := pipeline.NewMusicBrainzClient()

	// Enrichment service — SeatGeek client ID is optional
	seatgeekClientID := os.Getenv("SEATGEEK_CLIENT_ID")
	enrichmentSvc := pipeline.NewEnrichmentService(database, artist, seatgeekClientID, mbClient)
	enrichmentWorker := pipeline.NewEnrichmentWorker(enrichmentSvc)

	// PSY-1246: ongoing image-enrichment sweep. Reuses the SAME shared MB client
	// (mbClient) so its MusicBrainz traffic stays under the one ~1 req/s throttle.
	imageEnrichSweep := imageenrich.NewImageEnrichmentSweep(database, mbClient, cfg.Discogs.Token)

	// Wire enrichment queuing into discovery service (fire-and-forget after imports)
	discovery.SetEnrichmentService(enrichmentSvc)

	revisionSvc := adminsvc.NewRevisionService(database)
	radioSvc := catalog.NewRadioService(database)
	artistRelSvc := catalog.NewArtistRelationshipService(database)

	// PSY-997: entity_requests creation queue + its fulfillment adapter. The
	// fulfiller composes the catalog create services so the admin decide-approve
	// handler can create the entity from an approved request's payload.
	labelSvc := catalog.NewLabelService(database)
	releaseSvc := catalog.NewReleaseService(database)
	festivalSvc := catalog.NewFestivalService(database)
	showSvc := catalog.NewShowService(database)
	entityRequestSvc := community.NewEntityRequestService(database)
	entityRequestFulfiller := community.NewEntityRequestFulfiller(artist, venue, labelSvc, releaseSvc, festivalSvc, showSvc)

	// PSY-289: wire the comment notifier into the comment service so new
	// comments fan out notification emails fire-and-forget.
	commentSvc := engagement.NewCommentService(database, utils.NewMarkdownRenderer())
	commentNotificationSvc := engagement.NewCommentNotificationService(database, email, cfg.JWT.SecretKey, cfg.Email.FrontendURL)
	commentSvc.SetNotifier(commentNotificationSvc)

	// PSY-354: collections get tag support via the polymorphic entity_tags
	// system. Wire the tag service into the collection service so curators
	// can apply/remove tags + the get/list responses can surface chips.
	collectionSvc := community.NewCollectionService(database)
	tagSvc := catalog.NewTagService(database)
	collectionSvc.SetTagService(tagSvc)

	// /explore landing reads reuse the FeaturedSlot service to look up
	// the admin-curated bill + collection. Construct FeaturedSlot up
	// front so the explore service can take it as a dependency rather
	// than reaching back through the container.
	featuredSlotSvc := adminsvc.NewFeaturedSlotService(database)
	exploreService := exploresvc.NewExploreService(database, featuredSlotSvc)

	// PSY-1190: a trusted/community pending edit that sets an artist's
	// social.bandcamp applies via a direct UPDATE in ApprovePendingEdit, bypassing
	// ArtistService.UpdateArtist's profile→embed resolver. Inject the artist
	// service so the approval flow can resolve a newly-set profile root into the
	// bandcamp_embed_url (fill-when-empty).
	pendingEditSvc := adminsvc.NewPendingEditService(database, revisionSvc, email, cfg.Email.FrontendURL, engagement.DeriveBackendURL(cfg.Email.FrontendURL), cfg.JWT.SecretKey)
	pendingEditSvc.SetBandcampFiller(artist)

	return &ServiceContainer{
		// DB-only leaf services
		AdminStats:             adminsvc.NewAdminStatsService(database),
		Analytics:              adminsvc.NewAnalyticsService(database),
		APIToken:               adminsvc.NewAPITokenService(database),
		DataQuality:            adminsvc.NewDataQualityService(database),
		Revision:               revisionSvc,
		PendingEdit:            pendingEditSvc,
		Charts:                 catalog.NewChartsService(database),
		Artist:                 artist,
		ContributorProfile:     usersvc.NewContributorProfileService(database),
		ArtistReport:           adminsvc.NewArtistReportService(database),
		AuditLog:               adminsvc.NewAuditLogService(database),
		FeaturedSlot:           featuredSlotSvc,
		Explore:                exploreService,
		EntityExistence:        catalog.NewEntityExistenceService(database),
		Bookmark:               engagement.NewBookmarkService(database),
		Calendar:               engagement.NewCalendarService(database, savedShow),
		Collection:             collectionSvc,
		Request:                community.NewRequestService(database),
		EntityRequest:          entityRequestSvc,
		EntityRequestFulfiller: entityRequestFulfiller,
		Tag:                    tagSvc,
		ArtistRelationship:     artistRelSvc,
		Scene:                  catalog.NewSceneService(database),
		Attendance:             engagement.NewAttendanceService(database),
		Comment:                commentSvc,
		CommentVote:            engagement.NewCommentVoteService(database),
		CommentSubscription:    engagement.NewCommentSubscriptionService(database),
		CommentNotification:    commentNotificationSvc,
		Follow:                 engagement.NewFollowService(database),
		Festival:               festivalSvc,
		FestivalIntelligence:   catalog.NewFestivalIntelligenceService(database),
		Label:                  labelSvc,
		Release:                releaseSvc,
		SavedShow:              savedShow,
		Show:                   showSvc,
		ShowReport:             adminsvc.NewShowReportService(database),
		EntityReport:           adminsvc.NewEntityReportService(database),
		User:                   userService,
		Leaderboard:            usersvc.NewLeaderboardService(database),
		Radio:                  radioSvc,
		RadioFetch:             catalog.NewRadioFetchService(radioSvc, discord),
		RelationshipDerivation: catalog.NewRelationshipDerivationService(artistRelSvc),
		Venue:                  venue,
		SourceConfig:           sourceConfig,
		AIExtractionThrottle:   ratelimit.NewAIExtractionThrottleService(database),
		StreamingWorklist:      pipeline.NewStreamingWorklistService(database),
		DiscoverMusic:          pipeline.NewDiscoverMusicService(database, mbClient),
		// PSY-1199: the link-suggestion accept path reuses the artist write path
		// (UpdateArtist → bandcamp/spotify setters + PSY-1190 resolver), so it
		// takes the already-constructed artist service.
		LinkSuggestion: pipeline.NewLinkSuggestionService(database, artist),

		// Config-only services
		Discord:            discord,
		Email:              email,
		NotificationFilter: notification.NewNotificationFilterService(database, email, cfg.JWT.SecretKey, cfg.Email.FrontendURL),

		// No-param services
		PasswordValidator: auth.NewPasswordValidator(),

		// DB + Config composite services
		Auth:             auth.NewAuthService(database, cfg, userService),
		JWT:              jwtService,
		AppleAuth:        auth.NewAppleAuthService(database, cfg, jwtService),
		Extraction:       extraction,
		WebAuthn:         webauthnService,
		Cleanup:          adminsvc.NewCleanupService(database, userService),
		DataSync:         adminsvc.NewDataSyncService(database),
		Discovery:        discovery,
		Reminder:         engagement.NewReminderService(database, email, cfg),
		Enrichment:       enrichmentSvc,
		EnrichmentWorker: enrichmentWorker,
		ImageEnrichSweep: imageEnrichSweep,
		AutoPromotion:    adminsvc.NewAutoPromotionService(database, email, engagement.DeriveBackendURL(cfg.Email.FrontendURL), cfg.JWT.SecretKey),
		CollectionDigest: engagement.NewCollectionDigestService(database, email, cfg),
	}
}
