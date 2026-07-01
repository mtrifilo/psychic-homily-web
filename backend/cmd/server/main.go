package main

import (
	"context"
	"fmt"
	"log"
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
	"psychic-homily-backend/internal/observability"
	"psychic-homily-backend/internal/services"
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
	// runs after the slog.Error inside RunTickerLoop's recover paths, so a
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

	// Connect to database
	if err := db.Connect(cfg); err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	database := db.GetDB()

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

	// Setup routes
	_ = routes.SetupRoutes(router, sc, cfg)

	// Background services can be individually disabled via DISABLE_* env flags.
	// Defaults preserve current local behavior (flag unset → service starts).
	// E2E tests set all flags to "1" to get a lean, deterministic backend.
	//
	// Each service keeps its own cancel func; a nil cancel signals "not started"
	// so the shutdown path can skip it without panicking.
	var (
		cleanupCancel             context.CancelFunc
		reminderCancel            context.CancelFunc
		enrichmentCancel          context.CancelFunc
		autoPromotionCancel       context.CancelFunc
		radioFetchCancel          context.CancelFunc
		relDerivationCancel       context.CancelFunc
		collectionDigestCancel    context.CancelFunc
		imageEnrichSweepCancel    context.CancelFunc
		imageEnrichOutboxCancel   context.CancelFunc
		artistLocationSweepCancel    context.CancelFunc
		artistDiscographySweepCancel context.CancelFunc
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
	if imageEnrichSweepCancel != nil {
		imageEnrichSweepCancel()
		sc.ImageEnrichSweep.Stop()
	}
	if imageEnrichOutboxCancel != nil {
		imageEnrichOutboxCancel()
		sc.ImageEnrichOutbox.Stop()
	}
	if artistLocationSweepCancel != nil {
		artistLocationSweepCancel()
		sc.ArtistLocationSweep.Stop()
	}
	if artistDiscographySweepCancel != nil {
		artistDiscographySweepCancel()
		sc.ArtistDiscographySweep.Stop()
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
