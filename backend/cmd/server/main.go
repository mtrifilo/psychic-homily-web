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
	"psychic-homily-backend/internal/services"
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

	// Create a map for fast origin lookup
	allowedOriginsMap := make(map[string]bool)
	for _, origin := range cfg.CORS.AllowedOrigins {
		allowedOriginsMap[origin] = true
	}

	// CORS middleware with dynamic origin validation
	corsMiddleware := cors.New(cors.Options{
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
		AllowedMethods:   cfg.CORS.AllowedMethods,
		AllowedHeaders:   cfg.CORS.AllowedHeaders,
		AllowCredentials: cfg.CORS.AllowCredentials,
		MaxAge:           300,           // Cache preflight for 5 minutes
		Debug:            !isProduction, // Only enable debug logging in development
	})

	router.Use(corsMiddleware.Handler)

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
		cleanupCancel          context.CancelFunc
		reminderCancel         context.CancelFunc
		schedulerCancel        context.CancelFunc
		enrichmentCancel       context.CancelFunc
		autoPromotionCancel    context.CancelFunc
		radioFetchCancel       context.CancelFunc
		relDerivationCancel    context.CancelFunc
		collectionDigestCancel context.CancelFunc
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

	// Start extraction scheduler (background job for automated venue extraction)
	if os.Getenv("DISABLE_SCHEDULER") != "1" {
		var schedulerCtx context.Context
		schedulerCtx, schedulerCancel = context.WithCancel(context.Background())
		sc.Scheduler.Start(schedulerCtx)
	} else {
		log.Printf("DISABLE_SCHEDULER=1: skipping extraction scheduler startup")
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
	if schedulerCancel != nil {
		schedulerCancel()
		sc.Scheduler.Stop()
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

	// Shut down chromedp browser pool
	sc.Fetcher.ShutdownChromedp()

	// Graceful shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("error during shutdown: %s\n", err)
	}

	log.Println("Server gracefully stopped.")
}

// Helper function (you can move this to config package if you prefer)
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
