package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/cors"
	"github.com/joho/godotenv"

	"psychic-homily-backend/db"
	"psychic-homily-backend/internal/api/routes"
	"psychic-homily-backend/internal/auth"
	"psychic-homily-backend/internal/config"
)

func main() {
	// Load environment-specific .env file
	envFile := fmt.Sprintf(".env.%s", getEnv("ENVIRONMENT", config.EnvDevelopment))
	log.Printf("Loading environment file for environment: %s", config.EnvDevelopment)
	if err := godotenv.Load(envFile); err != nil {
		log.Printf("Warning: %s file not found, trying .env: %v", envFile, err)
		// Fallback to .env if environment-specific file doesn't exist
		if err := godotenv.Load(); err != nil {
			log.Printf("Warning: no .env file found: %v", err)
		}
	}

	// Load configuration
	cfg := config.Load()

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

	// Setup CORS middleware
	router.Use(cors.Handler(cors.Options{
		AllowedOrigins:   cfg.CORS.AllowedOrigins,
		AllowedMethods:   cfg.CORS.AllowedMethods,
		AllowedHeaders:   cfg.CORS.AllowedHeaders,
		AllowCredentials: cfg.CORS.AllowCredentials,
	}))

	// Setup routes
	_ = routes.SetupRoutes(router, cfg)

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
