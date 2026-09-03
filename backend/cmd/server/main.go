package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/getsentry/sentry-go"
	sentryhttp "github.com/getsentry/sentry-go/http"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/cors"
	"github.com/joho/godotenv"

	"psychic-homily-backend/db"
	adminh "psychic-homily-backend/internal/api/handlers/admin"
	"psychic-homily-backend/internal/api/middleware"
	"psychic-homily-backend/internal/api/routes"
	"psychic-homily-backend/internal/auth"
	"psychic-homily-backend/internal/config"
	"psychic-homily-backend/internal/logger"
	catalogm "psychic-homily-backend/internal/models/catalog"
	notificationm "psychic-homily-backend/internal/models/notification"
	"psychic-homily-backend/internal/observability"
	"psychic-homily-backend/internal/services"
	"psychic-homily-backend/internal/services/catalog"
	"psychic-homily-backend/internal/services/notification"
	servicesshared "psychic-homily-backend/internal/services/shared"
)

func main() {
	// Load environment-specific .env file
	environment := getEnv("ENVIRONMENT", config.EnvDevelopment)
	envFile := fmt.Sprintf(".env.%s", environment)
	log.Printf("Loading environment file for environment: %s", environment)
	if err := godotenv.Load(envFile); err != nil {
		log.Printf("Warning: %s file not found, trying .env: %v", envFile, err)
		// Fallback to .env if environment-specific file doesn't exist
		if err := godotenv.Load(); err != nil {
			log.Printf("Warning: no .env file found: %v", err)
		}
	}

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// PSY-432: refuse to boot if the test-fixtures reset flag is set in a
	// non-allowed environment. This is the keystone defense for the admin-
	// only reset endpoint; the route also only registers when the flag is
	// set, but that only helps if we actually start up.
	if err := adminh.ValidateTestFixturesEnvironment(os.Getenv); err != nil {
		log.Fatalf("PSY-432 test-fixtures misconfiguration: %v", err)
	}

	// PSY-475: same default-deny check for the auth-rate-limit disable
	// flag. Refuses to boot if DISABLE_AUTH_RATE_LIMITS=1 is combined
	// with a non-allowed ENVIRONMENT (production, stage, preview, unset).
	if err := routes.ValidateAuthRateLimitEnvironment(os.Getenv); err != nil {
		log.Fatalf("PSY-475 auth-rate-limit misconfiguration: %v", err)
	}

	// PSY-914: keystone guard for the faux OAuth "google" provider. Refuses
	// to boot if ENABLE_OAUTH_TEST_PROVIDER=1 outside {test, ci, development}.
	// Without this, the fake provider (which mints a session as a fixed email)
	// could be a critical auth bypass in production. SetupGoth also re-checks,
	// but the registration only matters if the process starts — this is the
	// real safety net.
	if err := auth.ValidateOAuthTestProviderEnvironment(os.Getenv); err != nil {
		log.Fatalf("PSY-914 oauth-test-provider misconfiguration: %v", err)
	}

	// Initialize structured logger
	// Use JSON format in production, text format with debug in development
	isProduction := environment == config.EnvProduction
	logger.Init(isProduction, !isProduction)

	// Initialize Sentry for error tracking
	if sentryDSN := os.Getenv("SENTRY_DSN"); sentryDSN != "" {
		if err := sentry.Init(sentry.ClientOptions{
			Dsn:              sentryDSN,
			Environment:      environment,
			Debug:            !isProduction,
			TracesSampleRate: 0.1, // Sample 10% of transactions for performance monitoring
			EnableTracing:    true,
			// Explicit (it already defaults false): never attach cookies/headers/
			// body as PII. PSY-1145.
			SendDefaultPII: false,
			// PSY-1145: best-effort secondary net that caps oversized values +
			// strips common secret shapes (token-bearing URL userinfo/query,
			// Bearer, key=value / JSON secrets, request body, auth cookie/headers)
			// from every captured event. NOT a complete guarantee — path-segment
			// secrets (e.g. the Discord webhook token) must still be redacted at
			// the call site (utils.RedactErrorURL). See ScrubSentryEvent's doc.
			BeforeSend: observability.ScrubSentryEvent,
		}); err != nil {
			log.Printf("Sentry initialization failed: %v", err)
		} else {
			log.Printf("Sentry initialized for environment: %s", environment)
		}
		// Flush buffered events before the program terminates
		defer sentry.Flush(2 * time.Second)
	} else {
		log.Printf("SENTRY_DSN not set, error tracking disabled")
	}

	// PSY-617: escalate background-service panics to Sentry. The handler
	// runs after the slog.Error inside RunScheduledLoop's recover paths, so a
	// panicking ticker now logs AND pages, instead of only logging. Safe
	// when SENTRY_DSN is unset — sentry.CaptureException no-ops without a
	// configured hub.
	servicesshared.SetPanicHandler(func(service string, panicValue any, stack []byte) {
		sentry.WithScope(func(scope *sentry.Scope) {
			scope.SetTag("service", service)
			scope.SetTag("source", "background_ticker")
			scope.SetExtra("stack", string(stack))
			sentry.CaptureException(fmt.Errorf("background service panic in %s: %v", service, panicValue))
		})
	})

	// PSY-1612: escalate a background sweep that has STOPPED RUNNING to Sentry.
	// The panic handler above only fires when something goes wrong loudly; PSY-1606
	// was the other failure — no panic, no error, just silence — so this reports the
	// absence of cycles instead of the presence of exceptions.
	//
	// Throttling lives in the database (last_overdue_alert_at), not here: it must
	// survive the deploys and hold across replicas, and process-local alert state
	// is the same mistake that caused the incident being monitored for.
	//
	// Fingerprint pins one issue per sweep per failure MODE, so a never-run sweep
	// and a stalled one stay separable in Sentry and neither buries the other.
	servicesshared.SetOverdueHandler(func(loop servicesshared.OverdueLoop) {
		sentry.WithScope(func(scope *sentry.Scope) {
			mode := loop.FailureMode()
			scope.SetLevel(sentry.LevelError)
			scope.SetTag("service", loop.Name)
			scope.SetTag("source", "sweep_health_check")
			scope.SetTag("failure_mode", mode)
			scope.SetFingerprint([]string{"background-sweep-overdue", loop.Name, mode})
			scope.SetExtra("interval", loop.Interval().String())
			scope.SetExtra("overdue_by", loop.Overdue().Round(time.Minute).String())
			scope.SetExtra("last_outcome", loop.OutcomeLabel())
			scope.SetExtra("consecutive_failures", loop.ConsecutiveFailures)
			scope.SetExtra("run_count", loop.RunCount)
			if loop.LastCompletedAt != nil {
				scope.SetExtra("last_completed_at", loop.LastCompletedAt.UTC().Format(time.RFC3339))
			}
			if loop.LastSuccessAt != nil {
				scope.SetExtra("last_success_at", loop.LastSuccessAt.UTC().Format(time.RFC3339))
			}
			if loop.LastRowsProcessed != nil {
				scope.SetExtra("last_rows_processed", *loop.LastRowsProcessed)
			}
			if loop.LastError != nil {
				scope.SetExtra("last_error", *loop.LastError)
			}
			sentry.CaptureException(errors.New(loop.Summary()))
		})
		// Flush before returning, so an alert raised moments before a kill — exactly
		// when deploys break sweeps — is pushed rather than waiting on the deferred
		// process-level Flush that only runs on a graceful shutdown.
		//
		// Its result is INFORMATIONAL and must NOT be treated as this event's
		// delivery status. Flush drains the whole current batch — which on a live
		// server also holds sampled transaction events from concurrent HTTP traffic
		// — and reports whether that batch cleared in time, not whether this alert
		// got through. Wiring it to the release path (an earlier attempt) meant an
		// ordinary Sentry latency blip released the claim and re-reported every
		// pass, ~96 events/day/sweep, for an event that had in fact been delivered:
		// the exact flood this feature exists to prevent, triggered by something
		// unrelated to the sweeps.
		//
		// So: flush to get the alert out promptly, log if the batch did not clear,
		// and leave delivery-failure detection to the one signal that genuinely
		// means this handler failed — a panic.
		if !sentry.Flush(2*time.Second) && sentry.CurrentHub().Client() != nil {
			log.Printf("sentry batch did not flush within 2s while reporting overdue sweep %q "+
				"(the event is queued; delivery is not confirmed)", loop.Name)
		}
	})

	// Connect to database
	if err := db.Connect(cfg); err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	database := db.GetDB()

	// PSY-1384: refuse to boot if critical columns are missing — catches
	// schema_migrations/DDL drift before engagement writes 422 at runtime.
	if err := db.AssertRequiredSchema(database); err != nil {
		log.Fatalf("PSY-1384 schema assertion: %v", err)
	}

	// PSY-1761: bring the venue-local zone guard's allowlist up to date with
	// this server's catalog. Boot is when it is most likely to have moved — a
	// Postgres upgrade, a base-image bump or a restore is what changes
	// pg_timezone_names, and all three restart this process.
	//
	// Unconditional, but OFF the boot path. Nothing downstream waits on it: the
	// table is seeded by its own migration, so until this lands the guard is
	// merely as fresh as the last deploy, which is today's behaviour and not
	// worse. Blocking the listener on it would trade a real risk (a slow
	// database stalling the healthcheck) for no gain. Bounded anyway, so a
	// hung query cannot leak a goroutine for the process's lifetime.
	//
	// Not a shared.RunScheduledLoop service: this is a one-shot reconcile, and
	// the recurring cadence (plus its overdue-sweep alerting) already belongs
	// to VenueTimezoneSweep, which refreshes the same table on every cycle.
	go func() {
		// Recovered explicitly. A bare goroutine is outside both
		// shared.RunScheduledLoop's recover and the Sentry panic handler, so an
		// unrecovered panic here would take the whole server down at boot to
		// protect a refresh that is, by design, optional.
		defer func() {
			if r := recover(); r != nil {
				slog.Error("timezone snapshot refresh panicked; the venue-local zone guard "+
					"is running against a possibly stale allowlist", "panic", r)
				sentry.CurrentHub().Recover(r)
			}
		}()
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		catalog.RefreshAndReportTimezoneNamesSnapshot(ctx, database, slog.Default())
	}()

	// Setup Goth authentication
	if err := auth.SetupGoth(cfg); err != nil {
		log.Fatalf("Failed to setup Goth: %v", err)
	}

	// Create router
	router := chi.NewMux()

	// Add request ID middleware (must be first to ensure all subsequent middleware has access)
	router.Use(middleware.RequestIDMiddleware)

	// Add Sentry middleware for error tracking and panic recovery
	// Must come early to capture errors from all subsequent handlers
	sentryHandler := sentryhttp.New(sentryhttp.Options{
		Repanic:         false, // Recover from panics gracefully (no other recoverer in chain)
		WaitForDelivery: false, // Don't block responses waiting for Sentry
		Timeout:         2 * time.Second,
	})
	router.Use(sentryHandler.Handle)

	// Add request logging middleware
	router.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requestID := logger.GetRequestID(r.Context())
			logger.Default().Info("request",
				"method", r.Method,
				"path", r.URL.Path,
				"origin", r.Header.Get("Origin"),
				"request_id", requestID,
			)
			next.ServeHTTP(w, r)
		})
	})

	// Setup CORS middleware
	log.Printf("CORS Configuration: Origins=%v, Methods=%v, Headers=%v, Credentials=%v",
		cfg.CORS.AllowedOrigins, cfg.CORS.AllowedMethods, cfg.CORS.AllowedHeaders, cfg.CORS.AllowCredentials)

	// CORS middleware with dynamic origin validation. Construction is
	// extracted to newCORSMiddleware so the preflight contract — notably
	// that non-prod echoes the Lighthouse x-vercel-protection-bypass header
	// (PSY-929) — is unit-testable.
	router.Use(newCORSMiddleware(cfg.CORS, isProduction).Handler)

	// Add security headers middleware
	// Adds headers like X-Content-Type-Options, X-Frame-Options, CSP, HSTS (in production)
	router.Use(middleware.SecurityHeaders)

	// Create service container (all services instantiated once)
	sc := services.NewServiceContainer(database, cfg)

	// A phk_ bearer is trusted only where APITokenService.ValidateToken resolves
	// it to a live token. The route-level limiters build the same adapter from
	// the same service (RouteContext.ValidateAPIToken).
	//
	// PSY-1362/1373/1814: rate-limit public-READ traffic (GET/HEAD) by auth state —
	// anonymous per-IP (100/min), authenticated per-USER (300/min, so shared-IP
	// logged-in users don't collide), validated phk_ API tokens exempt (ingest
	// search must not share the anonymous bucket). Mounted here — after sc
	// (needs sc.JWT + sc.APIToken), before SetupRoutes (chi middleware must be
	// registered before routes). OPT-IN (default noop) for stage-first rollout:
	// set ENABLE_PUBLIC_READ_RATE_LIMITS=1 per environment (stage, observe 429
	// rates, then prod).
	validateAPIToken := middleware.APITokenValidator(sc.APIToken)

	router.Use(routes.PublicReadRateLimiter(sc.JWT, validateAPIToken, os.Getenv))

	// PSY-1482: rate-limit authenticated engagement-toggle mutations (save/unsave
	// show+release, follow/unfollow entity+scene) against a SHARED per-USER budget
	// (60/min burst + 600/hr sustained, both must pass). Public-read limiting
	// exempts writes, so these mutations otherwise had no ceiling on rc.Protected.
	// Admin JWTs and validated API tokens bypass. Mounted after sc (needs sc.JWT),
	// before SetupRoutes (chi middleware must register before routes). OPT-IN
	// (default noop): set ENABLE_ENGAGEMENT_MUTATION_RATE_LIMITS=1 per environment
	// (stage, observe 429 rates, then prod).
	router.Use(routes.EngagementMutationRateLimiter(sc.JWT, validateAPIToken, os.Getenv))

	// PSY-1734: gzip compressible response bodies. Mounted LAST of the global
	// middleware, i.e. INSIDE the rate limiters, so a rejected request is never
	// compressed — a 429 body is a few dozen bytes. See
	// middleware.ResponseCompression for why this belongs here rather than in the
	// frontend's Vercel proxy, and why it carries no ENABLE_* flag.
	router.Use(middleware.ResponseCompression())

	// Setup routes
	_ = routes.SetupRoutes(router, sc, cfg)

	// Background services can be individually disabled via DISABLE_* env flags.
	// Defaults preserve current local behavior (flag unset → service starts).
	// E2E tests set all flags to "1" to get a lean, deterministic backend.
	//
	// Each service keeps its own cancel func; a nil cancel signals "not started"
	// so the shutdown path can skip it without panicking.
	var (
		cleanupCancel                context.CancelFunc
		reminderCancel               context.CancelFunc
		enrichmentCancel             context.CancelFunc
		autoPromotionCancel          context.CancelFunc
		radioFetchCancel             context.CancelFunc
		relDerivationCancel          context.CancelFunc
		collectionDigestCancel       context.CancelFunc
		sceneDigestCancel            context.CancelFunc
		imageEnrichSweepCancel       context.CancelFunc
		imageEnrichOutboxCancel      context.CancelFunc
		showNotifyOutboxCancel       context.CancelFunc
		venueAlertFlushCancel        context.CancelFunc
		artistLocationSweepCancel    context.CancelFunc
		artistDiscographySweepCancel context.CancelFunc
		artistLinksSweepCancel       context.CancelFunc
		releaseLinksSweepCancel      context.CancelFunc
		streetGeocodeSweepCancel     context.CancelFunc
		venueTimezoneSweepCancel     context.CancelFunc
		sweepHealthCheckCancel       context.CancelFunc
	)

	// Start account cleanup service (background job for permanent deletion)
	if os.Getenv("DISABLE_CLEANUP") != "1" {
		var cleanupCtx context.Context
		cleanupCtx, cleanupCancel = context.WithCancel(context.Background())
		sc.Cleanup.Start(cleanupCtx)
	} else {
		log.Printf("DISABLE_CLEANUP=1: skipping cleanup service startup")
	}

	// Start show reminder service (background job for 24h-before email reminders)
	if os.Getenv("DISABLE_REMINDERS") != "1" {
		var reminderCtx context.Context
		reminderCtx, reminderCancel = context.WithCancel(context.Background())
		sc.Reminder.Start(reminderCtx)
	} else {
		log.Printf("DISABLE_REMINDERS=1: skipping reminder service startup")
	}

	// Start enrichment worker (background job for post-import enrichment)
	if os.Getenv("DISABLE_ENRICHMENT_WORKER") != "1" {
		var enrichmentCtx context.Context
		enrichmentCtx, enrichmentCancel = context.WithCancel(context.Background())
		sc.EnrichmentWorker.Start(enrichmentCtx)
	} else {
		log.Printf("DISABLE_ENRICHMENT_WORKER=1: skipping enrichment worker startup")
	}

	// Start auto-promotion scheduler (background job for daily user tier evaluation)
	if os.Getenv("DISABLE_AUTO_PROMOTION") != "1" {
		var autoPromotionCtx context.Context
		autoPromotionCtx, autoPromotionCancel = context.WithCancel(context.Background())
		sc.AutoPromotion.Start(autoPromotionCtx)
	} else {
		log.Printf("DISABLE_AUTO_PROMOTION=1: skipping auto-promotion scheduler startup")
	}

	// Start radio fetch service (background job for playlist ingestion, affinity, re-matching)
	if os.Getenv("DISABLE_RADIO_FETCH") != "1" {
		var radioFetchCtx context.Context
		radioFetchCtx, radioFetchCancel = context.WithCancel(context.Background())
		sc.RadioFetch.Start(radioFetchCtx)
	} else {
		log.Printf("DISABLE_RADIO_FETCH=1: skipping radio fetch service startup")
	}

	// Start relationship derivation service (background job for shared_bills + shared_label)
	if os.Getenv("DISABLE_RELATIONSHIP_DERIVATION") != "1" {
		var relDerivationCtx context.Context
		relDerivationCtx, relDerivationCancel = context.WithCancel(context.Background())
		sc.RelationshipDerivation.Start(relDerivationCtx)
	} else {
		log.Printf("DISABLE_RELATIONSHIP_DERIVATION=1: skipping relationship derivation service startup")
	}

	// Start collection digest service (PSY-350: background job for weekly
	// collection-subscription digest emails, batching items added across all
	// of a user's subscribed collections into one email per week. Opt-IN —
	// users must enable the toggle in notification settings).
	if os.Getenv("DISABLE_COLLECTION_DIGEST") != "1" {
		var collectionDigestCtx context.Context
		collectionDigestCtx, collectionDigestCancel = context.WithCancel(context.Background())
		sc.CollectionDigest.Start(collectionDigestCtx)
	} else {
		log.Printf("DISABLE_COLLECTION_DIGEST=1: skipping collection digest service startup")
	}

	// Start scene digest service (PSY-1342: weekly batched email of the next 7
	// days of shows + new bands across all the scenes a user follows. Opt-IN —
	// users enable the toggle in notification settings).
	if os.Getenv("DISABLE_SCENE_DIGEST") != "1" {
		var sceneDigestCtx context.Context
		sceneDigestCtx, sceneDigestCancel = context.WithCancel(context.Background())
		sc.SceneDigest.Start(sceneDigestCtx)
	} else {
		log.Printf("DISABLE_SCENE_DIGEST=1: skipping scene digest service startup")
	}

	// Start image enrichment sweep (PSY-1246: background job filling missing artist
	// photos + release covers via the shipped fill-when-empty enrichers). OPT-IN,
	// default OFF (note the inverted polarity vs the DISABLE_* services above):
	// image enrichment is paused at the hotlink tier pending a product signal and
	// display is gated on PSY-1242, so the sweep runs only where explicitly enabled
	// (set ENABLE_IMAGE_ENRICH_SWEEP=1 — e.g. stage first) rather than auto-starting
	// in prod on deploy.
	if os.Getenv("ENABLE_IMAGE_ENRICH_SWEEP") == "1" {
		var imageEnrichSweepCtx context.Context
		imageEnrichSweepCtx, imageEnrichSweepCancel = context.WithCancel(context.Background())
		sc.ImageEnrichSweep.Start(imageEnrichSweepCtx)

		// PSY-1247: the on-create outbox poller shares the same opt-in gate as the
		// Phase-A sweep (both are paused at the hotlink tier pending PSY-1242).
		var imageEnrichOutboxCtx context.Context
		imageEnrichOutboxCtx, imageEnrichOutboxCancel = context.WithCancel(context.Background())
		sc.ImageEnrichOutbox.Start(imageEnrichOutboxCtx)
	} else {
		log.Printf("image enrichment sweep + outbox disabled (set ENABLE_IMAGE_ENRICH_SWEEP=1 to enable)")
	}

	// Start the show-notify outbox poller (PSY-1894: fires follower notifications
	// for shows that become visible via INGEST, which never enter the admin
	// approval flow that owns the only other MatchAndNotify call sites).
	//
	// ON by default, so it takes a DISABLE_* flag like the other default-ON
	// services above rather than the ENABLE_* opt-in the image-enrich pair uses:
	// making ingest-created shows notify IS the feature, so an opt-in default would
	// ship it dormant. DISABLE_SHOW_NOTIFY_OUTBOX=1 gates the ENQUEUE side too, so
	// turning it off stops rows being written rather than letting them pile up for
	// a burst when it is turned back on. Note it is read per PROCESS: an ingest CLI
	// run elsewhere needs the flag in its own environment.
	//
	// Starting this on a deploy cannot notify anyone about the existing catalogue:
	// show_notify_queue ships empty and nothing backfills it, so the poller has
	// literally nothing to find until a new show becomes visible.
	if notification.ShowNotifyOutboxEnabled() {
		var showNotifyOutboxCtx context.Context
		showNotifyOutboxCtx, showNotifyOutboxCancel = context.WithCancel(context.Background())
		sc.ShowNotifyOutbox.Start(showNotifyOutboxCtx)
	} else {
		log.Printf("show notify outbox disabled (%s=1); ingest-created shows will not notify followers",
			catalogm.ShowNotifyOutboxDisableFlag)
	}

	// Start the venue-alert flush poller (PSY-1895: delivers the coalesced
	// venue new-show alert once a venue-day's batch has gone quiet). Accrual
	// runs inline in MatchAndNotify; without this loop the rows accumulate and
	// nobody is ever told.
	//
	// ON by default with a DISABLE_* flag, matching the show-notify outbox: venue
	// followers being told about new shows IS the feature, so an opt-in default
	// would ship it dormant. The flag gates ACCRUAL too, so turning it off stops
	// rows being written rather than letting them pile up for a burst when it is
	// turned back on. Read per PROCESS — an ingest CLI run elsewhere needs the
	// flag in its own environment to stop accruing.
	//
	// Starting this on a deploy cannot alert anyone about the existing
	// catalogue: venue_show_alert_batch ships empty and only accrual writes to
	// it, so the poller has nothing to find until a show becomes visible at a
	// venue somebody already follows.
	if notification.VenueShowAlertsEnabled() {
		var venueAlertFlushCtx context.Context
		venueAlertFlushCtx, venueAlertFlushCancel = context.WithCancel(context.Background())
		sc.VenueAlertFlush.Start(venueAlertFlushCtx)
	} else {
		log.Printf("venue show alerts disabled (%s=1); venue followers will not be told about new shows",
			notificationm.VenueShowAlertsDisableFlag)
	}

	// Start artist-location sweep (PSY-1250: Phase-A background job filling missing
	// artist city/state/country via MusicBrainz + Bandcamp, fill-when-empty). OPT-IN,
	// default OFF (inverted polarity vs the DISABLE_* services above): the resolver
	// AUTO-WRITES a name-matched location, and the manual cmd's dry-run review is the
	// documented homonym backstop, so this runs only where explicitly enabled
	// (ENABLE_ARTIST_LOCATION_SWEEP=1 — e.g. stage first) rather than auto-starting on
	// deploy. Mirrors the image-sweep posture.
	if os.Getenv("ENABLE_ARTIST_LOCATION_SWEEP") == "1" {
		var artistLocationSweepCtx context.Context
		artistLocationSweepCtx, artistLocationSweepCancel = context.WithCancel(context.Background())
		sc.ArtistLocationSweep.Start(artistLocationSweepCtx)
	} else {
		log.Printf("artist location sweep disabled (set ENABLE_ARTIST_LOCATION_SWEEP=1 to enable)")
	}

	// Start artist-discography sweep (PSY-1291: Phase-A background job importing primary
	// discography for MBID-bearing artists via MusicBrainz browse + Cover Art Archive).
	// OPT-IN, default OFF (inverted polarity vs the DISABLE_* services above): releases
	// are the highest flood-risk enrichment, so this runs only where explicitly enabled
	// (ENABLE_ARTIST_DISCOGRAPHY_SWEEP=1 — e.g. stage first) rather than auto-starting on
	// deploy. Mirrors the location + image sweep posture.
	if os.Getenv("ENABLE_ARTIST_DISCOGRAPHY_SWEEP") == "1" {
		var artistDiscographySweepCtx context.Context
		artistDiscographySweepCtx, artistDiscographySweepCancel = context.WithCancel(context.Background())
		sc.ArtistDiscographySweep.Start(artistDiscographySweepCtx)
	} else {
		log.Printf("artist discography sweep disabled (set ENABLE_ARTIST_DISCOGRAPHY_SWEEP=1 to enable)")
	}

	// Start artist-links sweep (PSY-1279: Phase-A background job filling missing
	// spotify/bandcamp/website from MBID-keyed MusicBrainz url-rels, fill-when-empty,
	// auto-applied via UpdateArtist). OPT-IN, default OFF — the name-search discovery
	// + artist_link_suggestions queue remains for MBID-less artists; this sweep is the
	// durable backstop once an MBID exists.
	if os.Getenv("ENABLE_ARTIST_LINKS_SWEEP") == "1" {
		var artistLinksSweepCtx context.Context
		artistLinksSweepCtx, artistLinksSweepCancel = context.WithCancel(context.Background())
		sc.ArtistLinksSweep.Start(artistLinksSweepCtx)
	} else {
		log.Printf("artist links sweep disabled (set ENABLE_ARTIST_LINKS_SWEEP=1 to enable)")
	}

	// Start release-links sweep (PSY-1316: Phase-A background job filling missing
	// bandcamp/spotify release links from RG-MBID-keyed MusicBrainz url-rels,
	// fill-when-empty, source=mb_backfill). OPT-IN, default OFF.
	if os.Getenv("ENABLE_RELEASE_LINKS_SWEEP") == "1" {
		var releaseLinksSweepCtx context.Context
		releaseLinksSweepCtx, releaseLinksSweepCancel = context.WithCancel(context.Background())
		sc.ReleaseLinksSweep.Start(releaseLinksSweepCtx)
	} else {
		log.Printf("release links sweep disabled (set ENABLE_RELEASE_LINKS_SWEEP=1 to enable)")
	}

	// Start street-geocode sweep (PSY-1544: daily reconciliation of venue
	// street-level pins for the write paths that skip inline Nominatim lookups —
	// FindOrCreateVenue via show submission, data-sync import, contribution
	// address edits). Default ON like the DISABLE_* workers — NOT opt-in like
	// the ENABLE_* enrichment sweeps: it performs the exact deterministic write
	// CreateVenue/UpdateVenue already make inline by default, and shares their
	// geo.DefaultNominatim() limiter so combined traffic stays under the OSM
	// 1 req/s budget.
	if os.Getenv("DISABLE_STREET_GEOCODE_SWEEP") != "1" {
		var streetGeocodeSweepCtx context.Context
		streetGeocodeSweepCtx, streetGeocodeSweepCancel = context.WithCancel(context.Background())
		sc.StreetGeocodeSweep.Start(streetGeocodeSweepCtx)
	} else {
		log.Printf("DISABLE_STREET_GEOCODE_SWEEP=1: skipping street geocode sweep startup")
	}

	// Start venue-timezone integrity sweep (PSY-1695: re-validate stored venue
	// zones against the live pg_timezone_names and NULL the casualties). This is
	// the second layer under the show-list partition, which reads the column
	// straight into AT TIME ZONE with no per-row validation because validating
	// there cost 8.1s on a 20k-show venue; the write gate (PSY-1707) is layer
	// one, and this catches the drift a point-in-time gate cannot — the zone
	// catalog is a property of the server's tzdata packaging and changes under
	// image bumps and Postgres upgrades.
	//
	// Opt-in ENABLE_*, not default-on DISABLE_*: it writes NULLs over
	// operator-visible data on a schedule, so switching it on in an environment
	// should be a deliberate act rather than something that arrives with a
	// deploy.
	if os.Getenv(catalog.EnableVenueTimezoneSweepEnvVar) == "1" {
		var venueTimezoneSweepCtx context.Context
		venueTimezoneSweepCtx, venueTimezoneSweepCancel = context.WithCancel(context.Background())
		sc.VenueTimezoneSweep.Start(venueTimezoneSweepCtx)
	} else {
		log.Printf("%s is not 1: skipping venue timezone sweep startup", catalog.EnableVenueTimezoneSweepEnvVar)
	}

	// Start sweep health check (PSY-1612: reports background loops that have
	// stopped running). Default ON — this is the monitoring that would have caught
	// PSY-1606's seven silently-dead sweeps in days instead of weeks, so it should
	// require a deliberate act to switch off, never an omission.
	//
	// It watches every loop started above that has an interval of an hour or more
	// (shorter loops keep no run state, so they are outside its coverage).
	//
	// Switching a sweep off with its DISABLE_*/ENABLE_* flag does NOT silence it
	// immediately if it has run in this environment before: the row survives with
	// its last registration, so the sweep reads as overdue and reports once a day
	// until the retirement window (7 days without any process registering it)
	// expires. A sweep that has never been enabled here has no row and is never
	// reported. To retire one immediately (see backend/README.md, "Overdue-sweep
	// alerting", for the full procedure):
	//
	//	UPDATE background_service_runs SET last_registered_at = NULL WHERE name = '<loop name>';
	//
	// Do NOT DELETE the row: if any process still owns that loop the health check
	// re-creates it within one pass with created_at = NOW(), which hands a
	// genuinely stalled sweep a FRESH grace window, makes it report as never_ran
	// afterwards, and discards its run history.
	if os.Getenv("DISABLE_SWEEP_HEALTH_CHECK") != "1" {
		var sweepHealthCheckCtx context.Context
		sweepHealthCheckCtx, sweepHealthCheckCancel = context.WithCancel(context.Background())
		sc.SweepHealthCheck.Start(sweepHealthCheckCtx)
	} else {
		log.Printf("DISABLE_SWEEP_HEALTH_CHECK=1: skipping sweep health check startup")
	}

	// Create HTTP server
	srv := &http.Server{
		Addr:    cfg.Server.Addr,
		Handler: router,
	}

	// Start server in goroutine
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("error while attempting to listen: %s\n", err)
		}
	}()

	log.Printf("Now serving Psychic Homily API at http://%s\n", cfg.Server.Addr)
	log.Printf("OAuth providers configured: Google=%t, GitHub=%t",
		cfg.OAuth.GoogleClientID != "", cfg.OAuth.GitHubClientID != "")

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down Psychic Homily API...")

	// Stop background services only if they were started (cancel is nil if DISABLE_* was set).
	if cleanupCancel != nil {
		cleanupCancel()
		sc.Cleanup.Stop()
	}
	if reminderCancel != nil {
		reminderCancel()
		sc.Reminder.Stop()
	}
	if enrichmentCancel != nil {
		enrichmentCancel()
		sc.EnrichmentWorker.Stop()
	}
	if autoPromotionCancel != nil {
		autoPromotionCancel()
		sc.AutoPromotion.Stop()
	}
	if radioFetchCancel != nil {
		radioFetchCancel()
		sc.RadioFetch.Stop()
	}
	if relDerivationCancel != nil {
		relDerivationCancel()
		sc.RelationshipDerivation.Stop()
	}
	if collectionDigestCancel != nil {
		collectionDigestCancel()
		sc.CollectionDigest.Stop()
	}
	if sceneDigestCancel != nil {
		sceneDigestCancel()
		sc.SceneDigest.Stop()
	}
	if imageEnrichSweepCancel != nil {
		imageEnrichSweepCancel()
		sc.ImageEnrichSweep.Stop()
	}
	if imageEnrichOutboxCancel != nil {
		imageEnrichOutboxCancel()
		sc.ImageEnrichOutbox.Stop()
	}
	if showNotifyOutboxCancel != nil {
		showNotifyOutboxCancel()
		sc.ShowNotifyOutbox.Stop()
	}
	if venueAlertFlushCancel != nil {
		venueAlertFlushCancel()
		sc.VenueAlertFlush.Stop()
	}
	if artistLocationSweepCancel != nil {
		artistLocationSweepCancel()
		sc.ArtistLocationSweep.Stop()
	}
	if artistDiscographySweepCancel != nil {
		artistDiscographySweepCancel()
		sc.ArtistDiscographySweep.Stop()
	}
	if artistLinksSweepCancel != nil {
		artistLinksSweepCancel()
		sc.ArtistLinksSweep.Stop()
	}
	if releaseLinksSweepCancel != nil {
		releaseLinksSweepCancel()
		sc.ReleaseLinksSweep.Stop()
	}
	if streetGeocodeSweepCancel != nil {
		streetGeocodeSweepCancel()
		sc.StreetGeocodeSweep.Stop()
	}
	if venueTimezoneSweepCancel != nil {
		venueTimezoneSweepCancel()
		sc.VenueTimezoneSweep.Stop()
	}
	if sweepHealthCheckCancel != nil {
		sweepHealthCheckCancel()
		sc.SweepHealthCheck.Stop()
	}

	// Graceful shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("error during shutdown: %s\n", err)
	}

	log.Println("Server gracefully stopped.")
}

// newCORSMiddleware builds the chi CORS middleware from the configured
// allowlists. Extracted from main() so the preflight behaviour is
// unit-testable end-to-end (cmd/server/main_test.go) rather than only the
// CORSAllowedHeaders helper in isolation: a wiring drop or a go-chi/cors
// version bump that stopped echoing the Lighthouse bypass header would slip
// past a helper-only test and silently re-break the /explore gate (PSY-929).
func newCORSMiddleware(corsCfg config.CORSConfig, isProduction bool) *cors.Cors {
	// Map for fast origin lookup against the explicit allowlist.
	allowedOriginsMap := make(map[string]bool)
	for _, origin := range corsCfg.AllowedOrigins {
		allowedOriginsMap[origin] = true
	}

	return cors.New(cors.Options{
		AllowOriginFunc: func(r *http.Request, origin string) bool {
			// Check explicit allowed origins (from CORS_ALLOWED_ORIGINS or env defaults)
			if allowedOriginsMap[origin] {
				return true
			}
			// Allow Vercel preview deployments only in non-production environments.
			// For production, add specific preview URLs to CORS_ALLOWED_ORIGINS instead.
			if !isProduction && strings.HasSuffix(origin, ".vercel.app") {
				return true
			}
			return false
		},
		AllowedMethods: corsCfg.AllowedMethods,
		// Non-prod also allows the Lighthouse perf gate's Vercel SSO
		// bypass header so its cross-origin API calls pass preflight
		// (PSY-929). Prod stays tight — see config.CORSAllowedHeaders.
		AllowedHeaders:   config.CORSAllowedHeaders(corsCfg.AllowedHeaders, isProduction),
		AllowCredentials: corsCfg.AllowCredentials,
		MaxAge:           300,           // Cache preflight for 5 minutes
		Debug:            !isProduction, // Only enable debug logging in development
	})
}

// Helper function (you can move this to config package if you prefer)
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
