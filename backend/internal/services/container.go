package services

import (
	"context"
	"log"
	"os"

	"gorm.io/gorm"

	"psychic-homily-backend/internal/config"
	adminsvc "psychic-homily-backend/internal/services/admin"
	"psychic-homily-backend/internal/services/auth"
	"psychic-homily-backend/internal/services/catalog"
	"psychic-homily-backend/internal/services/community"
	"psychic-homily-backend/internal/services/discography"
	"psychic-homily-backend/internal/services/engagement"
	"psychic-homily-backend/internal/services/enrich"
	exploresvc "psychic-homily-backend/internal/services/explore"
	"psychic-homily-backend/internal/services/imageenrich"
	"psychic-homily-backend/internal/services/notification"
	"psychic-homily-backend/internal/services/pipeline"
	"psychic-homily-backend/internal/services/ratelimit"
	"psychic-homily-backend/internal/services/shared"
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
	Comment                *engagement.CommentService
	CommentVote            *engagement.CommentVoteService
	CommentSubscription    *engagement.CommentSubscriptionService
	CommentNotification    *engagement.CommentNotificationService
	Follow                 *engagement.FollowService
	Festival               *catalog.FestivalService
	FestivalIntelligence   *catalog.FestivalIntelligenceService
	Label                  *catalog.LabelService
	Release                *catalog.ReleaseService
	SavedRelease           *engagement.SavedReleaseService
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
	Auth                   *auth.AuthService
	JWT                    *auth.JWTService
	AppleAuth              *auth.AppleAuthService
	Extraction             *pipeline.ExtractionService
	WebAuthn               *auth.WebAuthnService // nil if init fails (passkeys optional)
	Cleanup                *adminsvc.CleanupService
	DataSync               *adminsvc.DataSyncService
	Discovery              *pipeline.DiscoveryService
	Reminder               *engagement.ReminderService
	Enrichment             *pipeline.EnrichmentService
	EnrichmentWorker       *pipeline.EnrichmentWorker
	ImageEnrichSweep       *imageenrich.ImageEnrichmentSweep
	ArtistLocationSweep    *enrich.ArtistLocationSweep
	ArtistDiscographySweep *discography.ArtistDiscographySweep
	ArtistLinksSweep       *enrich.ArtistLinksSweep
	ReleaseLinksSweep      *enrich.ReleaseLinksSweep
	ImageEnrichOutbox      *imageenrich.ImageEnrichOutboxPoller
	AutoPromotion          *adminsvc.AutoPromotionService
	// PSY-350: weekly collection-subscription digest emails (opt-IN).
	CollectionDigest *engagement.CollectionDigestService
	// PSY-1342: weekly followed-scenes digest emails (opt-IN).
	SceneDigest *engagement.SceneDigestService
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

	// PSY-1246/1247/1266: the image-enrichment subsystem. ONE shared Enricher (the
	// engine — reuses the SAME mbClient so all MB traffic stays under the one ~1 req/s
	// throttle, PSY-1208) drives both triggers: the Phase-A sweep (backfill ticker)
	// and the Phase-B outbox poller (prompt on-create). Both hold the same Enricher.
	imageEnricher := imageenrich.NewEnricher(database, mbClient, cfg.Discogs.Token)
	imageEnrichSweep := imageenrich.NewImageEnrichmentSweep(database, imageEnricher)
	imageEnrichOutbox := imageenrich.NewImageEnrichOutboxPoller(database, imageEnricher)

	// PSY-1250: artist-location sweep (Phase A). Reuses the SAME shared mbClient
	// (PSY-1208) so its MusicBrainz traffic stays under the one ~1 req/s throttle; a
	// fresh Bandcamp resolver (stateless, no global rate limit) handles the fallback.
	locationBandcamp := catalog.NewBandcampProfileResolver()
	artistLocationSweep := enrich.NewArtistLocationSweep(database, locationBandcamp, mbClient)

	// PSY-1291: artist-discography sweep (Phase A). Reuses the SAME shared mbClient
	// (PSY-1208) so MusicBrainz browse stays under the one ~1 req/s throttle.
	coverArtClient := catalog.NewCoverArtArchiveClient()
	artistDiscographySweep := discography.NewArtistDiscographySweep(database, mbClient, coverArtClient)

	// PSY-1292 (Phase B): eager discography import when musicbrainz_artist_id is first
	// written. NB the flag is shared with the Phase-A sweep on purpose — ENABLE_ARTIST_
	// DISCOGRAPHY_SWEEP is the discography-enrichment FEATURE switch, so =1 turns on BOTH
	// the nightly sweep AND these per-MBID-stamp imports (off the request goroutine).
	if os.Getenv("ENABLE_ARTIST_DISCOGRAPHY_SWEEP") == "1" {
		shared.OnArtistMBIDStamped = func(artistID uint) {
			shared.GoSafe(context.Background(), "artist_discography_on_mbid", func() {
				if _, err := discography.ImportArtistDiscographyByID(
					context.Background(), database, mbClient, coverArtClient, artistID,
				); err != nil {
					log.Printf("on-mbid discography import failed for artist %d: %v", artistID, err)
				}
			})
		}
	}

	// PSY-1279: artist-links sweep (Phase A). Reuses the SAME shared mbClient
	// (PSY-1208); auto-applies fill-when-empty via ArtistService.UpdateArtist.
	artistLinksSweep := enrich.NewArtistLinksSweep(database, mbClient, artist)

	// PSY-1251 (Phase B): on-create location/MBID enrichment for interactively-created
	// artists. NB the flag is shared with the Phase-A sweep on purpose — ENABLE_ARTIST_
	// LOCATION_SWEEP is the location-enrichment FEATURE switch, so =1 turns on BOTH the
	// nightly sweep AND these per-create MusicBrainz calls (off the request goroutine).
	// Off by default → the hook stays nil and CreateArtist's call is a no-op; the sweep
	// is the durability backstop, so this only adds promptness.
	if os.Getenv("ENABLE_ARTIST_LOCATION_SWEEP") == "1" {
		artist.SetLocationEnricher(func(artistID uint) {
			// Fire-and-forget: log a genuine DB/load failure (the sweep retries the row);
			// the no-op cases (gone / located / miss) return nil.
			if err := enrich.EnrichArtistLocationByID(database, locationBandcamp, mbClient, artistID); err != nil {
				log.Printf("on-create location enrich failed for artist %d: %v", artistID, err)
			}
		})
	}

	// Wire enrichment queuing into discovery service (fire-and-forget after imports)
	discovery.SetEnrichmentService(enrichmentSvc)

	revisionSvc := adminsvc.NewRevisionService(database)
	radioSvc := catalog.NewRadioService(database)
	shared.OnRadioArtistNameRematch = radioSvc.ScheduleRematchForArtistName
	shared.OnRadioLabelNameRematch = radioSvc.ScheduleRematchForLabelName
	artistRelSvc := catalog.NewArtistRelationshipService(database)

	// PSY-997: entity_requests creation queue + its fulfillment adapter. The
	// fulfiller composes the catalog create services so the admin decide-approve
	// handler can create the entity from an approved request's payload.
	labelSvc := catalog.NewLabelService(database)
	releaseSvc := catalog.NewReleaseService(database)
	savedRelease := engagement.NewSavedReleaseService(database, releaseSvc)
	festivalSvc := catalog.NewFestivalService(database)
	showSvc := catalog.NewShowService(database)
	entityRequestSvc := community.NewEntityRequestService(database)
	entityRequestFulfiller := community.NewEntityRequestFulfiller(artist, venue, labelSvc, releaseSvc, festivalSvc, showSvc)

	// PSY-1316: release-links sweep (Phase A). Same shared mbClient (PSY-1208);
	// auto-applies fill-when-empty via ReleaseService.AddExternalLinkWithSource
	// (source=mb_backfill).
	releaseLinksSweep := enrich.NewReleaseLinksSweep(database, mbClient, releaseSvc)

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
	// Shared instance: also injected into the scene digest service (PSY-1342)
	// for its per-scene content queries.
	sceneSvc := catalog.NewSceneService(database)
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
		Scene:                  sceneSvc,
		Comment:                commentSvc,
		CommentVote:            engagement.NewCommentVoteService(database),
		CommentSubscription:    engagement.NewCommentSubscriptionService(database),
		CommentNotification:    commentNotificationSvc,
		Follow:                 engagement.NewFollowService(database),
		Festival:               festivalSvc,
		FestivalIntelligence:   catalog.NewFestivalIntelligenceService(database),
		Label:                  labelSvc,
		Release:                releaseSvc,
		SavedRelease:           savedRelease,
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
		Auth:                   auth.NewAuthService(database, cfg, userService),
		JWT:                    jwtService,
		AppleAuth:              auth.NewAppleAuthService(database, cfg, jwtService),
		Extraction:             extraction,
		WebAuthn:               webauthnService,
		Cleanup:                adminsvc.NewCleanupService(database, userService),
		DataSync:               adminsvc.NewDataSyncService(database),
		Discovery:              discovery,
		Reminder:               engagement.NewReminderService(database, email, cfg),
		Enrichment:             enrichmentSvc,
		EnrichmentWorker:       enrichmentWorker,
		ImageEnrichSweep:       imageEnrichSweep,
		ImageEnrichOutbox:      imageEnrichOutbox,
		ArtistLocationSweep:    artistLocationSweep,
		ArtistDiscographySweep: artistDiscographySweep,
		ArtistLinksSweep:       artistLinksSweep,
		ReleaseLinksSweep:      releaseLinksSweep,
		AutoPromotion:          adminsvc.NewAutoPromotionService(database, email, engagement.DeriveBackendURL(cfg.Email.FrontendURL), cfg.JWT.SecretKey),
		CollectionDigest:       engagement.NewCollectionDigestService(database, email, cfg),
		SceneDigest:            engagement.NewSceneDigestService(database, email, sceneSvc, cfg),
	}
}
