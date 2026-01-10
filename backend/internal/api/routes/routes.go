package routes

import (
	"encoding/json"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/go-chi/chi/v5"

	"psychic-homily-backend/internal/api/handlers"
	"psychic-homily-backend/internal/api/middleware"
	"psychic-homily-backend/internal/config"
	"psychic-homily-backend/internal/services"
)

// SetupRoutes configures all API routes
func SetupRoutes(router *chi.Mux, cfg *config.Config) huma.API {
	api := humachi.New(router, huma.DefaultConfig("Psychic Homily", "1.0.0"))

	// Add request ID middleware to all Huma routes
	api.UseMiddleware(middleware.HumaRequestIDMiddleware)

	// Create services
	authService := services.NewAuthService(cfg)
	jwtService := services.NewJWTService(cfg)

	// Setup domain-specific routes
	setupSystemRoutes(router, api)
	setupAuthRoutes(router, api, authService, jwtService, cfg)

	// Create a protected group that will require authentication
	protectedGroup := huma.NewGroup(api, "")
	protectedGroup.UseMiddleware(middleware.HumaJWTMiddleware(jwtService))

	// Add protected auth routes
	authHandler := handlers.NewAuthHandler(authService, jwtService, services.NewUserService(), cfg)
	huma.Get(protectedGroup, "/auth/profile", authHandler.GetProfileHandler)
	huma.Post(protectedGroup, "/auth/refresh", authHandler.RefreshTokenHandler)

	setupShowRoutes(api, protectedGroup)
	setupArtistRoutes(api, protectedGroup)
	setupVenueRoutes(api, protectedGroup)
	setupSavedShowRoutes(protectedGroup)
	setupAdminRoutes(protectedGroup)

	return api
}

// setupAuthRoutes configures all authentication-related endpoints
func setupAuthRoutes(router *chi.Mux, api huma.API, authService *services.AuthService,
	jwtService *services.JWTService, cfg *config.Config) {
	userService := services.NewUserService()
	authHandler := handlers.NewAuthHandler(authService, jwtService, userService, cfg)
	oauthHTTPHandler := handlers.NewOAuthHTTPHandler(authService)

	// Public OAuth routes
	router.Get("/auth/login/{provider}", oauthHTTPHandler.OAuthLoginHTTPHandler)
	router.Get("/auth/callback/{provider}", oauthHTTPHandler.OAuthCallbackHTTPHandler)

	// Public auth endpoints
	huma.Post(api, "/auth/login", authHandler.LoginHandler)
	huma.Post(api, "/auth/logout", authHandler.LogoutHandler)
	huma.Post(api, "/auth/register", authHandler.RegisterHandler)
}

// setupSystemRoutes configures system/infrastructure endpoints
func setupSystemRoutes(router *chi.Mux, api huma.API) {
	// Health check endpoint
	huma.Get(api, "/health", handlers.HealthHandler)

	// OpenAPI specification endpoint
	router.Get("/openapi.json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(api.OpenAPI())
	})
}

// SetupShowRoutes configures all show-related endpoints
func setupShowRoutes(api huma.API, protected *huma.Group) {
	showHandler := handlers.NewShowHandler()

	// Public show endpoints - registered on main API without middleware
	huma.Get(api, "/shows", showHandler.GetShowsHandler)
	huma.Get(api, "/shows/upcoming", showHandler.GetUpcomingShowsHandler)
	huma.Get(api, "/shows/{show_id}", showHandler.GetShowHandler)

	// Protected show endpoints - registered on protected group with middleware
	huma.Post(protected, "/shows", showHandler.CreateShowHandler)
	huma.Put(protected, "/shows/{show_id}", showHandler.UpdateShowHandler)
	huma.Delete(protected, "/shows/{show_id}", showHandler.DeleteShowHandler)
	huma.Post(protected, "/shows/{show_id}/unpublish", showHandler.UnpublishShowHandler)
	huma.Post(protected, "/shows/ai-process", showHandler.AIProcessShowHandler)
}

func setupArtistRoutes(api huma.API, protected *huma.Group) {
	artistHandler := handlers.NewArtistHandler()

	// Public artist endpoints - registered on main API without middleware
	huma.Get(api, "/artists/search", artistHandler.SearchArtistsHandler)

	// Note: Add protected artist endpoints here if needed in the future
}

func setupVenueRoutes(api huma.API, protected *huma.Group) {
	venueHandler := handlers.NewVenueHandler()

	// Public venue endpoints - registered on main API without middleware
	huma.Get(api, "/venues/search", venueHandler.SearchVenuesHandler)

	// Note: Add protected venue endpoints here if needed in the future
}

// setupSavedShowRoutes configures saved show endpoints (user's personal "My List")
// All endpoints require authentication via protected group
func setupSavedShowRoutes(protected *huma.Group) {
	savedShowHandler := handlers.NewSavedShowHandler()

	// Protected saved show endpoints
	huma.Post(protected, "/saved-shows/{show_id}", savedShowHandler.SaveShowHandler)
	huma.Delete(protected, "/saved-shows/{show_id}", savedShowHandler.UnsaveShowHandler)
	huma.Get(protected, "/saved-shows", savedShowHandler.GetSavedShowsHandler)
	huma.Get(protected, "/saved-shows/{show_id}/check", savedShowHandler.CheckSavedHandler)
}

// setupAdminRoutes configures admin-only endpoints
// Note: Admin check is performed inside handlers, JWT auth is required via protected group
func setupAdminRoutes(protected *huma.Group) {
	adminHandler := handlers.NewAdminHandler()

	// Admin show management endpoints
	huma.Get(protected, "/admin/shows/pending", adminHandler.GetPendingShowsHandler)
	huma.Post(protected, "/admin/shows/{show_id}/approve", adminHandler.ApproveShowHandler)
	huma.Post(protected, "/admin/shows/{show_id}/reject", adminHandler.RejectShowHandler)

	// Admin venue management endpoints
	huma.Post(protected, "/admin/venues/{venue_id}/verify", adminHandler.VerifyVenueHandler)
}
