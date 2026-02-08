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
			// Check explicit allowed origins
			if allowedOriginsMap[origin] {
				return true
			}
			// Allow Vercel preview deployments (*.vercel.app)
			if strings.HasSuffix(origin, ".vercel.app") {
				return true
			}
			return false
		},
		AllowedMethods:   cfg.CORS.AllowedMethods,
		AllowedHeaders:   cfg.CORS.AllowedHeaders,
		AllowCredentials: cfg.CORS.AllowCredentials,
		MaxAge:           300,            // Cache preflight for 5 minutes
		Debug:            !isProduction, // Only enable debug logging in development
	})

	router.Use(corsMiddleware.Handler)

	// Add security headers middleware
	// Adds headers like X-Content-Type-Options, X-Frame-Options, CSP, HSTS (in production)
	router.Use(middleware.SecurityHeaders)

	// Setup routes
	_ = routes.SetupRoutes(router, cfg)

	// Start account cleanup service (background job for permanent deletion)
	cleanupService := services.NewCleanupService(nil)
	cleanupCtx, cleanupCancel := context.WithCancel(context.Background())
	cleanupService.Start(cleanupCtx)

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

	// Stop cleanup service
	cleanupCancel()
	cleanupService.Stop()

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
