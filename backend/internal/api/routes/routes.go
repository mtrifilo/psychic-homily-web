package routes

import (
	"encoding/json"
	"net/http"
	"os"

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
	huma.Post(protectedGroup, "/auth/verify-email/send", authHandler.SendVerificationEmailHandler)
	huma.Post(protectedGroup, "/auth/change-password", authHandler.ChangePasswordHandler)

	// Public email verification confirm endpoint (user clicks link from email)
	huma.Post(api, "/auth/verify-email/confirm", authHandler.ConfirmVerificationHandler)

	// Setup passkey routes (some public, some protected)
	setupPasskeyRoutes(api, protectedGroup, jwtService, cfg)

	setupShowRoutes(api, protectedGroup, cfg)
	setupArtistRoutes(api, protectedGroup)
	setupVenueRoutes(api, protectedGroup)
	setupSavedShowRoutes(protectedGroup)
	setupAdminRoutes(protectedGroup, cfg)

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

	// Magic link endpoints (public)
	huma.Post(api, "/auth/magic-link/send", authHandler.SendMagicLinkHandler)
	huma.Post(api, "/auth/magic-link/verify", authHandler.VerifyMagicLinkHandler)
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

// setupPasskeyRoutes configures WebAuthn/passkey endpoints
func setupPasskeyRoutes(api huma.API, protected *huma.Group, jwtService *services.JWTService, cfg *config.Config) {
	// Initialize WebAuthn service
	webauthnService, err := services.NewWebAuthnService(cfg)
	if err != nil {
		// Log error but don't fail - passkeys are optional
		// In production, you might want to handle this differently
		return
	}

	userService := services.NewUserService()
	passkeyHandler := handlers.NewPasskeyHandler(webauthnService, jwtService, userService, cfg)

	// Public passkey login endpoints (no auth required)
	huma.Post(api, "/auth/passkey/login/begin", passkeyHandler.BeginLoginHandler)
	huma.Post(api, "/auth/passkey/login/finish", passkeyHandler.FinishLoginHandler)

	// Protected passkey registration endpoints (user must be logged in)
	huma.Post(protected, "/auth/passkey/register/begin", passkeyHandler.BeginRegisterHandler)
	huma.Post(protected, "/auth/passkey/register/finish", passkeyHandler.FinishRegisterHandler)

	// Protected passkey management endpoints
	huma.Get(protected, "/auth/passkey/credentials", passkeyHandler.ListCredentialsHandler)
	huma.Delete(protected, "/auth/passkey/credentials/{credential_id}", passkeyHandler.DeleteCredentialHandler)
}

// SetupShowRoutes configures all show-related endpoints
func setupShowRoutes(api huma.API, protected *huma.Group, cfg *config.Config) {
	showHandler := handlers.NewShowHandler(cfg)

	// Public show endpoints - registered on main API without middleware
	huma.Get(api, "/shows", showHandler.GetShowsHandler)
	huma.Get(api, "/shows/upcoming", showHandler.GetUpcomingShowsHandler)
	huma.Get(api, "/shows/{show_id}", showHandler.GetShowHandler)

	// Export endpoint - only register in development environment
	if os.Getenv("ENVIRONMENT") == "development" {
		huma.Get(api, "/shows/{show_id}/export", showHandler.ExportShowHandler)
	}

	// Protected show endpoints - registered on protected group with middleware
	huma.Post(protected, "/shows", showHandler.CreateShowHandler)
	huma.Put(protected, "/shows/{show_id}", showHandler.UpdateShowHandler)
	huma.Delete(protected, "/shows/{show_id}", showHandler.DeleteShowHandler)
	huma.Post(protected, "/shows/{show_id}/unpublish", showHandler.UnpublishShowHandler)
	huma.Post(protected, "/shows/{show_id}/make-private", showHandler.MakePrivateShowHandler)
	huma.Post(protected, "/shows/{show_id}/publish", showHandler.PublishShowHandler)
	huma.Get(protected, "/shows/my-submissions", showHandler.GetMySubmissionsHandler)
	huma.Post(protected, "/shows/ai-process", showHandler.AIProcessShowHandler)
}

func setupArtistRoutes(api huma.API, protected *huma.Group) {
	artistHandler := handlers.NewArtistHandler()

	// Public artist endpoints - registered on main API without middleware
	// Note: Static routes must come before parameterized routes
	huma.Get(api, "/artists/search", artistHandler.SearchArtistsHandler)
	huma.Get(api, "/artists/{artist_id}", artistHandler.GetArtistHandler)
	huma.Get(api, "/artists/{artist_id}/shows", artistHandler.GetArtistShowsHandler)

	// Note: Add protected artist endpoints here if needed in the future
}

func setupVenueRoutes(api huma.API, protected *huma.Group) {
	venueHandler := handlers.NewVenueHandler()

	// Public venue endpoints - registered on main API without middleware
	// Note: Static routes must come before parameterized routes
	huma.Get(api, "/venues", venueHandler.ListVenuesHandler)
	huma.Get(api, "/venues/cities", venueHandler.GetVenueCitiesHandler)
	huma.Get(api, "/venues/search", venueHandler.SearchVenuesHandler)
	huma.Get(api, "/venues/{venue_id}", venueHandler.GetVenueHandler)
	huma.Get(api, "/venues/{venue_id}/shows", venueHandler.GetVenueShowsHandler)

	// Protected venue endpoints - require authentication
	huma.Put(protected, "/venues/{venue_id}", venueHandler.UpdateVenueHandler)
	huma.Delete(protected, "/venues/{venue_id}", venueHandler.DeleteVenueHandler)
	huma.Get(protected, "/venues/{venue_id}/my-pending-edit", venueHandler.GetMyPendingEditHandler)
	huma.Delete(protected, "/venues/{venue_id}/my-pending-edit", venueHandler.CancelMyPendingEditHandler)
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
func setupAdminRoutes(protected *huma.Group, cfg *config.Config) {
	adminHandler := handlers.NewAdminHandler(cfg)
	artistHandler := handlers.NewArtistHandler()

	// Admin show management endpoints
	huma.Get(protected, "/admin/shows/pending", adminHandler.GetPendingShowsHandler)
	huma.Get(protected, "/admin/shows/rejected", adminHandler.GetRejectedShowsHandler)
	huma.Post(protected, "/admin/shows/{show_id}/approve", adminHandler.ApproveShowHandler)
	huma.Post(protected, "/admin/shows/{show_id}/reject", adminHandler.RejectShowHandler)

	// Admin show import endpoints
	huma.Post(protected, "/admin/shows/import/preview", adminHandler.ImportShowPreviewHandler)
	huma.Post(protected, "/admin/shows/import/confirm", adminHandler.ImportShowConfirmHandler)

	// Admin venue management endpoints
	huma.Post(protected, "/admin/venues/{venue_id}/verify", adminHandler.VerifyVenueHandler)

	// Admin pending venue edit endpoints
	huma.Get(protected, "/admin/venues/pending-edits", adminHandler.GetPendingVenueEditsHandler)
	huma.Post(protected, "/admin/venues/pending-edits/{edit_id}/approve", adminHandler.ApproveVenueEditHandler)
	huma.Post(protected, "/admin/venues/pending-edits/{edit_id}/reject", adminHandler.RejectVenueEditHandler)

	// Admin artist management endpoints
	huma.Patch(protected, "/admin/artists/{artist_id}/bandcamp", artistHandler.UpdateArtistBandcampHandler)
	huma.Patch(protected, "/admin/artists/{artist_id}/spotify", artistHandler.UpdateArtistSpotifyHandler)
}
