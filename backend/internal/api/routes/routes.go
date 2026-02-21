package routes

import (
	"encoding/json"
	"net/http"
	"os"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/httprate"

	"psychic-homily-backend/internal/api/handlers"
	"psychic-homily-backend/internal/api/middleware"
	"psychic-homily-backend/internal/config"
	"psychic-homily-backend/internal/logger"
	"psychic-homily-backend/internal/services"
)

// SetupRoutes configures all API routes
func SetupRoutes(router *chi.Mux, sc *services.ServiceContainer, cfg *config.Config) huma.API {
	api := humachi.New(router, huma.DefaultConfig("Psychic Homily", "1.0.0"))

	// Add request ID middleware to all Huma routes
	api.UseMiddleware(middleware.HumaRequestIDMiddleware)

	// Enrich Sentry scope with request ID and HTTP metadata on all routes
	api.UseMiddleware(middleware.HumaSentryContextMiddleware)

	// Setup domain-specific routes
	setupSystemRoutes(router, api)
	setupAuthRoutes(router, api, sc, cfg)

	// Create a protected group that will require authentication
	protectedGroup := huma.NewGroup(api, "")
	protectedGroup.UseMiddleware(middleware.HumaJWTMiddleware(sc.JWT, cfg.Session))
	// Enrich Sentry scope with authenticated user context (runs after JWT middleware)
	protectedGroup.UseMiddleware(middleware.HumaSentryContextMiddleware)

	// Add protected auth routes
	authHandler := handlers.NewAuthHandler(sc.Auth, sc.JWT, sc.User, sc.Email, sc.Discord, sc.PasswordValidator, cfg)
	huma.Get(protectedGroup, "/auth/profile", authHandler.GetProfileHandler)
	huma.Post(protectedGroup, "/auth/verify-email/send", authHandler.SendVerificationEmailHandler)
	huma.Post(protectedGroup, "/auth/change-password", authHandler.ChangePasswordHandler)

	// Token refresh uses lenient middleware (accepts tokens expired within 7 days)
	lenientGroup := huma.NewGroup(api, "")
	lenientGroup.UseMiddleware(middleware.LenientHumaJWTMiddleware(sc.JWT, 7*24*time.Hour))
	huma.Post(lenientGroup, "/auth/refresh", authHandler.RefreshTokenHandler)

	// Account deletion endpoints
	huma.Get(protectedGroup, "/auth/account/deletion-summary", authHandler.GetDeletionSummaryHandler)
	huma.Post(protectedGroup, "/auth/account/delete", authHandler.DeleteAccountHandler)

	// Data export endpoint (GDPR Right to Portability)
	huma.Get(protectedGroup, "/auth/account/export", authHandler.ExportDataHandler)

	// CLI token generation endpoint (admin only)
	huma.Post(protectedGroup, "/auth/cli-token", authHandler.GenerateCLITokenHandler)

	// OAuth account management endpoints
	oauthAccountHandler := handlers.NewOAuthAccountHandler(sc.User)
	huma.Get(protectedGroup, "/auth/oauth/accounts", oauthAccountHandler.GetOAuthAccountsHandler)
	huma.Delete(protectedGroup, "/auth/oauth/accounts/{provider}", oauthAccountHandler.UnlinkOAuthAccountHandler)

	// User preferences endpoints
	userPrefsHandler := handlers.NewUserPreferencesHandler(sc.User)
	huma.Put(protectedGroup, "/auth/preferences/favorite-cities", userPrefsHandler.SetFavoriteCitiesHandler)

	// Public email verification confirm endpoint (user clicks link from email)
	huma.Post(api, "/auth/verify-email/confirm", authHandler.ConfirmVerificationHandler)

	// Account recovery endpoints (public - user is not authenticated)
	// These are registered in setupAuthRoutes with rate limiting

	// Setup passkey routes (some public, some protected) - with rate limiting
	setupPasskeyRoutes(router, api, protectedGroup, sc, cfg)

	setupShowRoutes(router, api, protectedGroup, sc, cfg)
	setupArtistRoutes(api, protectedGroup, sc)
	setupVenueRoutes(api, protectedGroup, sc)
	setupSavedShowRoutes(protectedGroup, sc)
	setupFavoriteVenueRoutes(protectedGroup, sc)
	setupShowReportRoutes(router, protectedGroup, sc, cfg)
	setupArtistReportRoutes(router, protectedGroup, sc, cfg)
	setupAdminRoutes(protectedGroup, sc)

	return api
}

// setupAuthRoutes configures all authentication-related endpoints
func setupAuthRoutes(router *chi.Mux, api huma.API, sc *services.ServiceContainer, cfg *config.Config) {
	authHandler := handlers.NewAuthHandler(sc.Auth, sc.JWT, sc.User, sc.Email, sc.Discord, sc.PasswordValidator, cfg)
	oauthHTTPHandler := handlers.NewOAuthHTTPHandler(sc.Auth, cfg)

	// Create rate limiter for auth endpoints: 10 requests per minute per IP
	// This helps prevent:
	// - Brute force attacks on login
	// - Credential stuffing
	// - Email bombing via magic links
	// - Spam account creation
	authRateLimiter := httprate.Limit(
		10,              // requests
		1*time.Minute,   // per duration
		httprate.WithKeyFuncs(httprate.KeyByIP),
		httprate.WithLimitHandler(rateLimitHandler),
	)

	// Rate-limited OAuth routes
	router.Group(func(r chi.Router) {
		r.Use(authRateLimiter)
		r.Get("/auth/login/{provider}", oauthHTTPHandler.OAuthLoginHTTPHandler)
		r.Get("/auth/callback/{provider}", oauthHTTPHandler.OAuthCallbackHTTPHandler)
	})

	// Rate-limited auth API endpoints using Chi middleware wrapper
	// We register these directly on the router with rate limiting, then Huma picks them up
	router.Group(func(r chi.Router) {
		r.Use(authRateLimiter)

		// Create a sub-API for rate-limited routes
		rateLimitedAPI := humachi.New(r, huma.DefaultConfig("Psychic Homily Auth", "1.0.0"))
		rateLimitedAPI.UseMiddleware(middleware.HumaRequestIDMiddleware)

		huma.Post(rateLimitedAPI, "/auth/login", authHandler.LoginHandler)
		huma.Post(rateLimitedAPI, "/auth/register", authHandler.RegisterHandler)
		huma.Post(rateLimitedAPI, "/auth/magic-link/send", authHandler.SendMagicLinkHandler)
		huma.Post(rateLimitedAPI, "/auth/magic-link/verify", authHandler.VerifyMagicLinkHandler)

		// Sign in with Apple (public, rate-limited)
		appleAuthHandler := handlers.NewAppleAuthHandler(sc.AppleAuth, sc.Discord, cfg)
		huma.Post(rateLimitedAPI, "/auth/apple/callback", appleAuthHandler.AppleCallbackHandler)

		// Account recovery endpoints (public, rate-limited)
		huma.Post(rateLimitedAPI, "/auth/recover-account", authHandler.RecoverAccountHandler)
		huma.Post(rateLimitedAPI, "/auth/recover-account/request", authHandler.RequestAccountRecoveryHandler)
		huma.Post(rateLimitedAPI, "/auth/recover-account/confirm", authHandler.ConfirmAccountRecoveryHandler)
	})

	// Logout doesn't need strict rate limiting (already requires valid session)
	huma.Post(api, "/auth/logout", authHandler.LogoutHandler)
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
func setupPasskeyRoutes(router *chi.Mux, api huma.API, protected *huma.Group, sc *services.ServiceContainer, cfg *config.Config) {
	if sc.WebAuthn == nil {
		// WebAuthn service failed to initialize - passkeys are optional
		return
	}

	passkeyHandler := handlers.NewPasskeyHandler(sc.WebAuthn, sc.JWT, sc.User, cfg)

	// Create rate limiter for passkey endpoints: 20 requests per minute per IP
	// Slightly more lenient than auth due to multi-step WebAuthn flow
	passkeyRateLimiter := httprate.Limit(
		20,              // requests
		1*time.Minute,   // per duration
		httprate.WithKeyFuncs(httprate.KeyByIP),
		httprate.WithLimitHandler(rateLimitHandler),
	)

	// Rate-limited public passkey endpoints
	router.Group(func(r chi.Router) {
		r.Use(passkeyRateLimiter)

		passkeyAPI := humachi.New(r, huma.DefaultConfig("Psychic Homily Passkey", "1.0.0"))
		passkeyAPI.UseMiddleware(middleware.HumaRequestIDMiddleware)

		// Public passkey login endpoints (no auth required)
		huma.Post(passkeyAPI, "/auth/passkey/login/begin", passkeyHandler.BeginLoginHandler)
		huma.Post(passkeyAPI, "/auth/passkey/login/finish", passkeyHandler.FinishLoginHandler)

		// Public passkey signup endpoints (passkey-first registration, no auth required)
		huma.Post(passkeyAPI, "/auth/passkey/signup/begin", passkeyHandler.BeginSignupHandler)
		huma.Post(passkeyAPI, "/auth/passkey/signup/finish", passkeyHandler.FinishSignupHandler)
	})

	// Protected passkey registration endpoints (user must be logged in)
	huma.Post(protected, "/auth/passkey/register/begin", passkeyHandler.BeginRegisterHandler)
	huma.Post(protected, "/auth/passkey/register/finish", passkeyHandler.FinishRegisterHandler)

	// Protected passkey management endpoints
	huma.Get(protected, "/auth/passkey/credentials", passkeyHandler.ListCredentialsHandler)
	huma.Delete(protected, "/auth/passkey/credentials/{credential_id}", passkeyHandler.DeleteCredentialHandler)
}

// setupShowRoutes configures all show-related endpoints
func setupShowRoutes(router *chi.Mux, api huma.API, protected *huma.Group, sc *services.ServiceContainer, cfg *config.Config) {
	showHandler := handlers.NewShowHandler(sc.Show, sc.SavedShow, sc.Discord, sc.MusicDiscovery, sc.Extraction)

	// Public show endpoints - registered on main API without middleware
	// Note: Static routes must come before parameterized routes
	huma.Get(api, "/shows", showHandler.GetShowsHandler)
	huma.Get(api, "/shows/cities", showHandler.GetShowCitiesHandler)
	huma.Get(api, "/shows/upcoming", showHandler.GetUpcomingShowsHandler)

	// Show detail with optional auth for access control on non-approved shows
	optionalAuthGroup := huma.NewGroup(api, "")
	optionalAuthGroup.UseMiddleware(middleware.OptionalHumaJWTMiddleware(sc.JWT))
	huma.Get(optionalAuthGroup, "/shows/{show_id}", showHandler.GetShowHandler)

	// Export endpoint - only register in development environment
	if os.Getenv("ENVIRONMENT") == "development" {
		huma.Get(api, "/shows/{show_id}/export", showHandler.ExportShowHandler)
	}

	// Rate-limited show creation: 10 requests per hour per IP
	// Prevents flooding the admin approval queue
	router.Group(func(r chi.Router) {
		r.Use(httprate.Limit(
			middleware.ShowCreateRequestsPerHour,
			time.Hour,
			httprate.WithKeyFuncs(httprate.KeyByIP),
			httprate.WithLimitHandler(rateLimitHandler),
		))
		showCreateAPI := humachi.New(r, huma.DefaultConfig("Psychic Homily Show Create", "1.0.0"))
		showCreateAPI.UseMiddleware(middleware.HumaRequestIDMiddleware)
		showCreateAPI.UseMiddleware(middleware.HumaJWTMiddleware(sc.JWT, cfg.Session))
		huma.Post(showCreateAPI, "/shows", showHandler.CreateShowHandler)
	})

	// Rate-limited AI processing: 5 requests per minute per IP
	// Calls external Anthropic API — expensive operation
	router.Group(func(r chi.Router) {
		r.Use(httprate.Limit(
			middleware.AIProcessRequestsPerMinute,
			time.Minute,
			httprate.WithKeyFuncs(httprate.KeyByIP),
			httprate.WithLimitHandler(rateLimitHandler),
		))
		aiProcessAPI := humachi.New(r, huma.DefaultConfig("Psychic Homily AI Process", "1.0.0"))
		aiProcessAPI.UseMiddleware(middleware.HumaRequestIDMiddleware)
		aiProcessAPI.UseMiddleware(middleware.HumaJWTMiddleware(sc.JWT, cfg.Session))
		huma.Post(aiProcessAPI, "/shows/ai-process", showHandler.AIProcessShowHandler)
	})

	// Protected show endpoints (no additional rate limiting needed)
	huma.Put(protected, "/shows/{show_id}", showHandler.UpdateShowHandler)
	huma.Delete(protected, "/shows/{show_id}", showHandler.DeleteShowHandler)
	huma.Post(protected, "/shows/{show_id}/unpublish", showHandler.UnpublishShowHandler)
	huma.Post(protected, "/shows/{show_id}/make-private", showHandler.MakePrivateShowHandler)
	huma.Post(protected, "/shows/{show_id}/publish", showHandler.PublishShowHandler)
	huma.Post(protected, "/shows/{show_id}/sold-out", showHandler.SetShowSoldOutHandler)
	huma.Post(protected, "/shows/{show_id}/cancelled", showHandler.SetShowCancelledHandler)
	huma.Get(protected, "/shows/my-submissions", showHandler.GetMySubmissionsHandler)
}

func setupArtistRoutes(api huma.API, protected *huma.Group, sc *services.ServiceContainer) {
	artistHandler := handlers.NewArtistHandler(sc.Artist, sc.AuditLog)

	// Public artist endpoints - registered on main API without middleware
	// Note: Static routes must come before parameterized routes
	huma.Get(api, "/artists", artistHandler.ListArtistsHandler)
	huma.Get(api, "/artists/search", artistHandler.SearchArtistsHandler)
	huma.Get(api, "/artists/{artist_id}", artistHandler.GetArtistHandler)
	huma.Get(api, "/artists/{artist_id}/shows", artistHandler.GetArtistShowsHandler)

	// Protected artist endpoints
	huma.Delete(protected, "/artists/{artist_id}", artistHandler.DeleteArtistHandler)
	huma.Patch(protected, "/admin/artists/{artist_id}", artistHandler.AdminUpdateArtistHandler)
}

func setupVenueRoutes(api huma.API, protected *huma.Group, sc *services.ServiceContainer) {
	venueHandler := handlers.NewVenueHandler(sc.Venue, sc.Discord)

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
func setupSavedShowRoutes(protected *huma.Group, sc *services.ServiceContainer) {
	savedShowHandler := handlers.NewSavedShowHandler(sc.SavedShow)

	// Protected saved show endpoints
	huma.Post(protected, "/saved-shows/{show_id}", savedShowHandler.SaveShowHandler)
	huma.Delete(protected, "/saved-shows/{show_id}", savedShowHandler.UnsaveShowHandler)
	huma.Get(protected, "/saved-shows", savedShowHandler.GetSavedShowsHandler)
	huma.Get(protected, "/saved-shows/{show_id}/check", savedShowHandler.CheckSavedHandler)
	huma.Post(protected, "/saved-shows/check-batch", savedShowHandler.CheckBatchSavedHandler)
}

// setupFavoriteVenueRoutes configures favorite venue endpoints
// All endpoints require authentication via protected group
func setupFavoriteVenueRoutes(protected *huma.Group, sc *services.ServiceContainer) {
	favoriteVenueHandler := handlers.NewFavoriteVenueHandler(sc.FavoriteVenue)

	// Protected favorite venue endpoints
	huma.Post(protected, "/favorite-venues/{venue_id}", favoriteVenueHandler.FavoriteVenueHandler)
	huma.Delete(protected, "/favorite-venues/{venue_id}", favoriteVenueHandler.UnfavoriteVenueHandler)
	huma.Get(protected, "/favorite-venues", favoriteVenueHandler.GetFavoriteVenuesHandler)
	huma.Get(protected, "/favorite-venues/{venue_id}/check", favoriteVenueHandler.CheckFavoritedHandler)
	huma.Get(protected, "/favorite-venues/shows", favoriteVenueHandler.GetFavoriteVenueShowsHandler)
}

// setupShowReportRoutes configures show report endpoints
// All endpoints require authentication via protected group
func setupShowReportRoutes(router *chi.Mux, protected *huma.Group, sc *services.ServiceContainer, cfg *config.Config) {
	showReportHandler := handlers.NewShowReportHandler(sc.ShowReport, sc.Discord, sc.User, sc.AuditLog)

	// Rate-limited report submission: 5 requests per minute per IP
	// Prevents spamming admins with reports
	router.Group(func(r chi.Router) {
		r.Use(httprate.Limit(
			middleware.ReportRequestsPerMinute,
			time.Minute,
			httprate.WithKeyFuncs(httprate.KeyByIP),
			httprate.WithLimitHandler(rateLimitHandler),
		))
		reportAPI := humachi.New(r, huma.DefaultConfig("Psychic Homily Reports", "1.0.0"))
		reportAPI.UseMiddleware(middleware.HumaRequestIDMiddleware)
		reportAPI.UseMiddleware(middleware.HumaJWTMiddleware(sc.JWT, cfg.Session))
		huma.Post(reportAPI, "/shows/{show_id}/report", showReportHandler.ReportShowHandler)
	})

	// Protected report endpoints (no additional rate limiting)
	huma.Get(protected, "/shows/{show_id}/my-report", showReportHandler.GetMyReportHandler)

	// Admin endpoints for managing reports
	huma.Get(protected, "/admin/reports", showReportHandler.GetPendingReportsHandler)
	huma.Post(protected, "/admin/reports/{report_id}/dismiss", showReportHandler.DismissReportHandler)
	huma.Post(protected, "/admin/reports/{report_id}/resolve", showReportHandler.ResolveReportHandler)
}

// setupArtistReportRoutes configures artist report endpoints
func setupArtistReportRoutes(router *chi.Mux, protected *huma.Group, sc *services.ServiceContainer, cfg *config.Config) {
	artistReportHandler := handlers.NewArtistReportHandler(sc.ArtistReport, sc.Discord, sc.User, sc.AuditLog)

	// Rate-limited report submission: 5 requests per minute per IP
	router.Group(func(r chi.Router) {
		r.Use(httprate.Limit(
			middleware.ReportRequestsPerMinute,
			time.Minute,
			httprate.WithKeyFuncs(httprate.KeyByIP),
			httprate.WithLimitHandler(rateLimitHandler),
		))
		reportAPI := humachi.New(r, huma.DefaultConfig("Psychic Homily Artist Reports", "1.0.0"))
		reportAPI.UseMiddleware(middleware.HumaRequestIDMiddleware)
		reportAPI.UseMiddleware(middleware.HumaJWTMiddleware(sc.JWT, cfg.Session))
		huma.Post(reportAPI, "/artists/{artist_id}/report", artistReportHandler.ReportArtistHandler)
	})

	// Protected report endpoints (no additional rate limiting)
	huma.Get(protected, "/artists/{artist_id}/my-report", artistReportHandler.GetMyArtistReportHandler)

	// Admin endpoints for managing artist reports
	huma.Get(protected, "/admin/artist-reports", artistReportHandler.GetPendingArtistReportsHandler)
	huma.Post(protected, "/admin/artist-reports/{report_id}/dismiss", artistReportHandler.DismissArtistReportHandler)
	huma.Post(protected, "/admin/artist-reports/{report_id}/resolve", artistReportHandler.ResolveArtistReportHandler)
}

// setupAdminRoutes configures admin-only endpoints
// Note: Admin check is performed inside handlers, JWT auth is required via protected group
func setupAdminRoutes(protected *huma.Group, sc *services.ServiceContainer) {
	adminHandler := handlers.NewAdminHandler(
		sc.Show, sc.Venue, sc.Discord, sc.MusicDiscovery, sc.Discovery,
		sc.APIToken, sc.DataSync, sc.AuditLog, sc.User, sc.AdminStats,
	)
	artistHandler := handlers.NewArtistHandler(sc.Artist, sc.AuditLog)
	auditLogHandler := handlers.NewAuditLogHandler(sc.AuditLog)

	// Admin dashboard stats endpoint
	huma.Get(protected, "/admin/stats", adminHandler.GetAdminStatsHandler)

	// Admin show listing endpoint (for CLI export)
	huma.Get(protected, "/admin/shows", adminHandler.GetAdminShowsHandler)

	// Admin show management endpoints
	huma.Get(protected, "/admin/shows/pending", adminHandler.GetPendingShowsHandler)
	huma.Get(protected, "/admin/shows/rejected", adminHandler.GetRejectedShowsHandler)
	huma.Post(protected, "/admin/shows/{show_id}/approve", adminHandler.ApproveShowHandler)
	huma.Post(protected, "/admin/shows/{show_id}/reject", adminHandler.RejectShowHandler)

	// Admin show import endpoints (single)
	huma.Post(protected, "/admin/shows/import/preview", adminHandler.ImportShowPreviewHandler)
	huma.Post(protected, "/admin/shows/import/confirm", adminHandler.ImportShowConfirmHandler)

	// Admin show export/import endpoints (bulk - for CLI)
	huma.Post(protected, "/admin/shows/export/bulk", adminHandler.BulkExportShowsHandler)
	huma.Post(protected, "/admin/shows/import/bulk/preview", adminHandler.BulkImportPreviewHandler)
	huma.Post(protected, "/admin/shows/import/bulk/confirm", adminHandler.BulkImportConfirmHandler)

	// Admin venue management endpoints
	huma.Get(protected, "/admin/venues/unverified", adminHandler.GetUnverifiedVenuesHandler)
	huma.Post(protected, "/admin/venues/{venue_id}/verify", adminHandler.VerifyVenueHandler)

	// Admin pending venue edit endpoints
	huma.Get(protected, "/admin/venues/pending-edits", adminHandler.GetPendingVenueEditsHandler)
	huma.Post(protected, "/admin/venues/pending-edits/{edit_id}/approve", adminHandler.ApproveVenueEditHandler)
	huma.Post(protected, "/admin/venues/pending-edits/{edit_id}/reject", adminHandler.RejectVenueEditHandler)

	// Admin artist management endpoints
	huma.Patch(protected, "/admin/artists/{artist_id}/bandcamp", artistHandler.UpdateArtistBandcampHandler)
	huma.Patch(protected, "/admin/artists/{artist_id}/spotify", artistHandler.UpdateArtistSpotifyHandler)

	// Admin discovery endpoints (for local discovery app)
	huma.Post(protected, "/admin/discovery/import", adminHandler.DiscoveryImportHandler)
	huma.Post(protected, "/admin/discovery/check", adminHandler.DiscoveryCheckHandler)

	// Admin API token management endpoints
	huma.Post(protected, "/admin/tokens", adminHandler.CreateAPITokenHandler)
	huma.Get(protected, "/admin/tokens", adminHandler.ListAPITokensHandler)
	huma.Delete(protected, "/admin/tokens/{token_id}", adminHandler.RevokeAPITokenHandler)

	// Admin data export endpoints (for syncing local data to Stage/Production)
	huma.Get(protected, "/admin/export/shows", adminHandler.ExportShowsHandler)
	huma.Get(protected, "/admin/export/artists", adminHandler.ExportArtistsHandler)
	huma.Get(protected, "/admin/export/venues", adminHandler.ExportVenuesHandler)

	// Admin data import endpoint (for syncing local data to Stage/Production)
	huma.Post(protected, "/admin/data/import", adminHandler.DataImportHandler)

	// Admin audit log endpoint
	huma.Get(protected, "/admin/audit-logs", auditLogHandler.GetAuditLogsHandler)

	// Admin user list endpoint
	huma.Get(protected, "/admin/users", adminHandler.GetAdminUsersHandler)
}

// rateLimitHandler handles rate limit exceeded responses with JSON
func rateLimitHandler(w http.ResponseWriter, r *http.Request) {
	// Log the rate limit hit
	log := logger.FromContext(r.Context())
	if log == nil {
		log = logger.Default()
	}
	log.Warn("rate limit exceeded",
		"path", r.URL.Path,
		"method", r.Method,
		"remote_addr", r.RemoteAddr,
	)

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Retry-After", "60")
	w.WriteHeader(http.StatusTooManyRequests)
	w.Write([]byte(`{"success":false,"error":"too_many_requests","message":"Rate limit exceeded. Please try again in 60 seconds."}`))
}
