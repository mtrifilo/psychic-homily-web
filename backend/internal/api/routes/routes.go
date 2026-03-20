package routes

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"
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
	userPrefsHandler := handlers.NewUserPreferencesHandler(sc.User, cfg.JWT.SecretKey)
	huma.Put(protectedGroup, "/auth/preferences/favorite-cities", userPrefsHandler.SetFavoriteCitiesHandler)
	huma.Patch(protectedGroup, "/auth/preferences/show-reminders", userPrefsHandler.SetShowRemindersHandler)

	// Public unsubscribe endpoint (HMAC-signed, no auth required)
	huma.Post(api, "/auth/unsubscribe/show-reminders", userPrefsHandler.UnsubscribeShowRemindersHandler)

	// Public email verification confirm endpoint (user clicks link from email)
	huma.Post(api, "/auth/verify-email/confirm", authHandler.ConfirmVerificationHandler)

	// Account recovery endpoints (public - user is not authenticated)
	// These are registered in setupAuthRoutes with rate limiting

	// Setup passkey routes (some public, some protected) - with rate limiting
	setupPasskeyRoutes(router, api, protectedGroup, sc, cfg)

	setupShowRoutes(router, api, protectedGroup, sc, cfg)
	setupArtistRoutes(api, protectedGroup, sc)
	setupReleaseRoutes(api, protectedGroup, sc)
	setupLabelRoutes(api, protectedGroup, sc)
	setupFestivalRoutes(api, protectedGroup, sc)
	setupVenueRoutes(api, protectedGroup, sc)
	setupCalendarRoutes(router, protectedGroup, sc, cfg)
	setupSavedShowRoutes(protectedGroup, sc)
	setupFavoriteVenueRoutes(protectedGroup, sc)
	setupShowReportRoutes(router, protectedGroup, sc, cfg)
	setupArtistReportRoutes(router, protectedGroup, sc, cfg)
	setupAdminRoutes(protectedGroup, sc)
	setupPipelineRoutes(protectedGroup, sc)
	setupContributorProfileRoutes(api, protectedGroup, sc)
	setupCollectionRoutes(api, protectedGroup, sc)
	setupRequestRoutes(api, protectedGroup, sc)
	setupRevisionRoutes(api, protectedGroup, sc)
	setupTagRoutes(api, protectedGroup, sc)
	setupArtistRelationshipRoutes(api, protectedGroup, sc)
	setupSceneRoutes(api, sc)
	setupAttendanceRoutes(api, protectedGroup, sc)
	setupFollowRoutes(api, protectedGroup, sc)
	setupNotificationFilterRoutes(api, protectedGroup, sc, cfg)
	setupChartsRoutes(api, sc)

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
	// API token requests (phk_ prefix) bypass the rate limit — they're trusted admin clients
	router.Group(func(r chi.Router) {
		r.Use(rateLimitUnlessAPIToken(
			middleware.ShowCreateRequestsPerHour,
			time.Hour,
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
	artistHandler := handlers.NewArtistHandler(sc.Artist, sc.AuditLog, sc.Revision)

	// Public artist endpoints - registered on main API without middleware
	// Note: Static routes must come before parameterized routes
	huma.Get(api, "/artists", artistHandler.ListArtistsHandler)
	huma.Get(api, "/artists/cities", artistHandler.GetArtistCitiesHandler)
	huma.Get(api, "/artists/search", artistHandler.SearchArtistsHandler)
	huma.Get(api, "/artists/{artist_id}", artistHandler.GetArtistHandler)
	huma.Get(api, "/artists/{artist_id}/shows", artistHandler.GetArtistShowsHandler)
	huma.Get(api, "/artists/{artist_id}/labels", artistHandler.GetArtistLabelsHandler)
	huma.Get(api, "/artists/{artist_id}/aliases", artistHandler.GetArtistAliasesHandler)

	// Protected artist endpoints
	huma.Delete(protected, "/artists/{artist_id}", artistHandler.DeleteArtistHandler)
	huma.Post(protected, "/admin/artists", artistHandler.AdminCreateArtistHandler)
	huma.Patch(protected, "/admin/artists/{artist_id}", artistHandler.AdminUpdateArtistHandler)
	huma.Post(protected, "/admin/artists/{artist_id}/aliases", artistHandler.AddArtistAliasHandler)
	huma.Delete(protected, "/admin/artists/{artist_id}/aliases/{alias_id}", artistHandler.DeleteArtistAliasHandler)
	huma.Post(protected, "/admin/artists/merge", artistHandler.MergeArtistsHandler)
}

func setupReleaseRoutes(api huma.API, protected *huma.Group, sc *services.ServiceContainer) {
	releaseHandler := handlers.NewReleaseHandler(sc.Release, sc.Artist, sc.AuditLog)

	// Public release endpoints
	// Note: Static routes must come before parameterized routes
	huma.Get(api, "/releases", releaseHandler.ListReleasesHandler)
	huma.Get(api, "/releases/search", releaseHandler.SearchReleasesHandler)
	huma.Get(api, "/releases/{release_id}", releaseHandler.GetReleaseHandler)
	huma.Get(api, "/artists/{artist_id}/releases", releaseHandler.GetArtistReleasesHandler)

	// Protected release endpoints (admin-only checks inside handlers)
	huma.Post(protected, "/releases", releaseHandler.CreateReleaseHandler)
	huma.Put(protected, "/releases/{release_id}", releaseHandler.UpdateReleaseHandler)
	huma.Delete(protected, "/releases/{release_id}", releaseHandler.DeleteReleaseHandler)
	huma.Post(protected, "/releases/{release_id}/links", releaseHandler.AddExternalLinkHandler)
	huma.Delete(protected, "/releases/{release_id}/links/{link_id}", releaseHandler.RemoveExternalLinkHandler)
}

func setupLabelRoutes(api huma.API, protected *huma.Group, sc *services.ServiceContainer) {
	labelHandler := handlers.NewLabelHandler(sc.Label, sc.AuditLog)

	// Public label endpoints
	// Note: Static routes must come before parameterized routes
	huma.Get(api, "/labels", labelHandler.ListLabelsHandler)
	huma.Get(api, "/labels/search", labelHandler.SearchLabelsHandler)
	huma.Get(api, "/labels/{label_id}", labelHandler.GetLabelHandler)
	huma.Get(api, "/labels/{label_id}/artists", labelHandler.GetLabelRosterHandler)
	huma.Get(api, "/labels/{label_id}/releases", labelHandler.GetLabelCatalogHandler)

	// Protected label endpoints (admin-only checks inside handlers)
	huma.Post(protected, "/labels", labelHandler.CreateLabelHandler)
	huma.Put(protected, "/labels/{label_id}", labelHandler.UpdateLabelHandler)
	huma.Delete(protected, "/labels/{label_id}", labelHandler.DeleteLabelHandler)
}

func setupFestivalRoutes(api huma.API, protected *huma.Group, sc *services.ServiceContainer) {
	festivalHandler := handlers.NewFestivalHandler(sc.Festival, sc.Artist, sc.AuditLog, sc.Revision)

	// Public festival endpoints
	// Note: Static routes must come before parameterized routes
	huma.Get(api, "/festivals", festivalHandler.ListFestivalsHandler)
	huma.Get(api, "/festivals/search", festivalHandler.SearchFestivalsHandler)
	huma.Get(api, "/festivals/{festival_id}", festivalHandler.GetFestivalHandler)
	huma.Get(api, "/festivals/{festival_id}/artists", festivalHandler.GetFestivalArtistsHandler)
	huma.Get(api, "/festivals/{festival_id}/venues", festivalHandler.GetFestivalVenuesHandler)
	huma.Get(api, "/artists/{artist_id}/festivals", festivalHandler.GetArtistFestivalsHandler)

	// Festival intelligence endpoints (public, computed from existing data)
	intelHandler := handlers.NewFestivalIntelligenceHandler(sc.FestivalIntelligence, sc.Festival, sc.Artist)
	huma.Get(api, "/festivals/{festival_id}/similar", intelHandler.GetSimilarFestivalsHandler)
	huma.Get(api, "/festivals/{festival_a_id}/overlap/{festival_b_id}", intelHandler.GetFestivalOverlapHandler)
	huma.Get(api, "/festivals/{festival_id}/breakouts", intelHandler.GetFestivalBreakoutsHandler)
	huma.Get(api, "/artists/{artist_id}/festival-trajectory", intelHandler.GetArtistFestivalTrajectoryHandler)
	huma.Get(api, "/festivals/series/{series_slug}/compare", intelHandler.GetSeriesComparisonHandler)

	// Protected festival endpoints (admin-only checks inside handlers)
	huma.Post(protected, "/festivals", festivalHandler.CreateFestivalHandler)
	huma.Put(protected, "/festivals/{festival_id}", festivalHandler.UpdateFestivalHandler)
	huma.Delete(protected, "/festivals/{festival_id}", festivalHandler.DeleteFestivalHandler)
	huma.Post(protected, "/festivals/{festival_id}/artists", festivalHandler.AddFestivalArtistHandler)
	huma.Put(protected, "/festivals/{festival_id}/artists/{artist_id}", festivalHandler.UpdateFestivalArtistHandler)
	huma.Delete(protected, "/festivals/{festival_id}/artists/{artist_id}", festivalHandler.RemoveFestivalArtistHandler)
	huma.Post(protected, "/festivals/{festival_id}/venues", festivalHandler.AddFestivalVenueHandler)
	huma.Delete(protected, "/festivals/{festival_id}/venues/{venue_id}", festivalHandler.RemoveFestivalVenueHandler)
}

func setupVenueRoutes(api huma.API, protected *huma.Group, sc *services.ServiceContainer) {
	venueHandler := handlers.NewVenueHandler(sc.Venue, sc.Discord, sc.AuditLog, sc.Revision)

	// Public venue endpoints - registered on main API without middleware
	// Note: Static routes must come before parameterized routes
	huma.Get(api, "/venues", venueHandler.ListVenuesHandler)
	huma.Get(api, "/venues/cities", venueHandler.GetVenueCitiesHandler)
	huma.Get(api, "/venues/search", venueHandler.SearchVenuesHandler)
	huma.Get(api, "/venues/{venue_id}", venueHandler.GetVenueHandler)
	huma.Get(api, "/venues/{venue_id}/shows", venueHandler.GetVenueShowsHandler)
	huma.Get(api, "/venues/{venue_id}/genres", venueHandler.GetVenueGenresHandler)

	// Protected venue endpoints - require authentication
	huma.Post(protected, "/admin/venues", venueHandler.AdminCreateVenueHandler)
	huma.Put(protected, "/venues/{venue_id}", venueHandler.UpdateVenueHandler)
	huma.Delete(protected, "/venues/{venue_id}", venueHandler.DeleteVenueHandler)
	huma.Get(protected, "/venues/{venue_id}/my-pending-edit", venueHandler.GetMyPendingEditHandler)
	huma.Delete(protected, "/venues/{venue_id}/my-pending-edit", venueHandler.CancelMyPendingEditHandler)
}

// setupCalendarRoutes configures calendar feed and token management endpoints
func setupCalendarRoutes(router *chi.Mux, protected *huma.Group, sc *services.ServiceContainer, cfg *config.Config) {
	calendarHandler := handlers.NewCalendarHandler(sc.Calendar, cfg)

	// Public Chi route for ICS feed (token-authenticated, not JWT)
	router.Get("/calendar/{token}", calendarHandler.GetCalendarFeedHandler)

	// Protected Huma routes for token CRUD
	huma.Post(protected, "/calendar/token", calendarHandler.CreateCalendarTokenHandler)
	huma.Get(protected, "/calendar/token", calendarHandler.GetCalendarTokenStatusHandler)
	huma.Delete(protected, "/calendar/token", calendarHandler.DeleteCalendarTokenHandler)
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
		sc.NotificationFilter,
	)
	artistHandler := handlers.NewArtistHandler(sc.Artist, sc.AuditLog, sc.Revision)
	auditLogHandler := handlers.NewAuditLogHandler(sc.AuditLog)

	// Admin dashboard stats endpoint
	huma.Get(protected, "/admin/stats", adminHandler.GetAdminStatsHandler)
	huma.Get(protected, "/admin/activity", adminHandler.GetActivityFeedHandler)

	// Admin show listing endpoint (for CLI export)
	huma.Get(protected, "/admin/shows", adminHandler.GetAdminShowsHandler)

	// Admin show management endpoints
	huma.Get(protected, "/admin/shows/pending", adminHandler.GetPendingShowsHandler)
	huma.Get(protected, "/admin/shows/rejected", adminHandler.GetRejectedShowsHandler)
	huma.Post(protected, "/admin/shows/{show_id}/approve", adminHandler.ApproveShowHandler)
	huma.Post(protected, "/admin/shows/{show_id}/reject", adminHandler.RejectShowHandler)
	huma.Post(protected, "/admin/shows/batch-approve", adminHandler.BatchApproveShowsHandler)
	huma.Post(protected, "/admin/shows/batch-reject", adminHandler.BatchRejectShowsHandler)

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

	// Admin data quality endpoints
	dataQualityHandler := handlers.NewDataQualityHandler(sc.DataQuality)
	huma.Get(protected, "/admin/data-quality", dataQualityHandler.GetDataQualitySummaryHandler)
	huma.Get(protected, "/admin/data-quality/{category}", dataQualityHandler.GetDataQualityCategoryHandler)

	// Admin analytics endpoints
	analyticsHandler := handlers.NewAnalyticsHandler(sc.Analytics)
	huma.Get(protected, "/admin/analytics/growth", analyticsHandler.GetGrowthMetricsHandler)
	huma.Get(protected, "/admin/analytics/engagement", analyticsHandler.GetEngagementMetricsHandler)
	huma.Get(protected, "/admin/analytics/community", analyticsHandler.GetCommunityHealthHandler)
	huma.Get(protected, "/admin/analytics/data-quality", analyticsHandler.GetDataQualityTrendsHandler)
}

// setupContributorProfileRoutes configures contributor profile endpoints
func setupContributorProfileRoutes(api huma.API, protected *huma.Group, sc *services.ServiceContainer) {
	profileHandler := handlers.NewContributorProfileHandler(sc.ContributorProfile, sc.User)

	// Public profile endpoints with optional auth (so profile owner can see their own private profile)
	optionalAuthGroup := huma.NewGroup(api, "")
	optionalAuthGroup.UseMiddleware(middleware.OptionalHumaJWTMiddleware(sc.JWT))
	huma.Get(optionalAuthGroup, "/users/{username}", profileHandler.GetPublicProfileHandler)
	huma.Get(optionalAuthGroup, "/users/{username}/contributions", profileHandler.GetContributionHistoryHandler)
	huma.Get(optionalAuthGroup, "/users/{username}/sections", profileHandler.GetUserSectionsHandler)

	// Protected endpoints for authenticated user's own profile
	huma.Get(protected, "/auth/profile/contributor", profileHandler.GetOwnProfileHandler)
	huma.Get(protected, "/auth/profile/contributions", profileHandler.GetOwnContributionsHandler)
	huma.Patch(protected, "/auth/profile/visibility", profileHandler.UpdateProfileVisibilityHandler)
	huma.Patch(protected, "/auth/profile/privacy", profileHandler.UpdatePrivacySettingsHandler)
	huma.Get(protected, "/auth/profile/sections", profileHandler.GetOwnSectionsHandler)
	huma.Post(protected, "/auth/profile/sections", profileHandler.CreateSectionHandler)
	huma.Put(protected, "/auth/profile/sections/{section_id}", profileHandler.UpdateSectionHandler)
	huma.Delete(protected, "/auth/profile/sections/{section_id}", profileHandler.DeleteSectionHandler)
}

// setupPipelineRoutes configures AI extraction pipeline admin endpoints.
// Admin check is performed inside handlers, JWT auth is required via protected group.
func setupPipelineRoutes(protected *huma.Group, sc *services.ServiceContainer) {
	pipelineHandler := handlers.NewPipelineHandler(sc.Pipeline, sc.VenueSourceConfig)

	huma.Post(protected, "/admin/pipeline/extract/{venue_id}", pipelineHandler.ExtractVenueHandler)
	huma.Get(protected, "/admin/pipeline/venues", pipelineHandler.ListPipelineVenuesHandler)
	huma.Get(protected, "/admin/pipeline/venues/{venue_id}/stats", pipelineHandler.VenueRejectionStatsHandler)
	huma.Patch(protected, "/admin/pipeline/venues/{venue_id}/notes", pipelineHandler.UpdateExtractionNotesHandler)
	huma.Put(protected, "/admin/pipeline/venues/{venue_id}/config", pipelineHandler.UpdateVenueConfigHandler)
	huma.Get(protected, "/admin/pipeline/venues/{venue_id}/runs", pipelineHandler.GetVenueRunsHandler)
	huma.Post(protected, "/admin/pipeline/venues/{venue_id}/reset-render-method", pipelineHandler.ResetRenderMethodHandler)
}

// setupCollectionRoutes configures collection endpoints.
// Public endpoints use optional auth (for private collection access checks).
// CRUD, item management, and subscription endpoints require authentication.
func setupCollectionRoutes(api huma.API, protected *huma.Group, sc *services.ServiceContainer) {
	collectionHandler := handlers.NewCollectionHandler(sc.Collection, sc.AuditLog)

	// Public collection endpoints with optional auth
	optionalAuthGroup := huma.NewGroup(api, "")
	optionalAuthGroup.UseMiddleware(middleware.OptionalHumaJWTMiddleware(sc.JWT))
	huma.Get(optionalAuthGroup, "/collections", collectionHandler.ListCollectionsHandler)
	huma.Get(optionalAuthGroup, "/collections/{slug}", collectionHandler.GetCollectionHandler)
	huma.Get(optionalAuthGroup, "/collections/{slug}/stats", collectionHandler.GetCollectionStatsHandler)

	// Protected collection endpoints
	huma.Post(protected, "/collections", collectionHandler.CreateCollectionHandler)
	huma.Put(protected, "/collections/{slug}", collectionHandler.UpdateCollectionHandler)
	huma.Delete(protected, "/collections/{slug}", collectionHandler.DeleteCollectionHandler)

	// Collection item management
	huma.Post(protected, "/collections/{slug}/items", collectionHandler.AddItemHandler)
	huma.Delete(protected, "/collections/{slug}/items/{item_id}", collectionHandler.RemoveItemHandler)
	huma.Put(protected, "/collections/{slug}/items/reorder", collectionHandler.ReorderItemsHandler)

	// Collection subscription
	huma.Post(protected, "/collections/{slug}/subscribe", collectionHandler.SubscribeHandler)
	huma.Delete(protected, "/collections/{slug}/subscribe", collectionHandler.UnsubscribeHandler)

	// Admin: feature/unfeature collections
	huma.Put(protected, "/collections/{slug}/feature", collectionHandler.SetFeaturedHandler)

	// User's own collections (created + subscribed)
	huma.Get(protected, "/auth/collections", collectionHandler.GetUserCollectionsHandler)
}

// setupRequestRoutes configures community request endpoints.
// Public endpoints use optional auth (so authenticated users see their vote).
// CRUD, voting, fulfillment, and closing require authentication.
func setupRequestRoutes(api huma.API, protected *huma.Group, sc *services.ServiceContainer) {
	requestHandler := handlers.NewRequestHandler(sc.Request, sc.AuditLog)

	// Public request endpoints with optional auth (to include user's vote)
	optionalAuthGroup := huma.NewGroup(api, "")
	optionalAuthGroup.UseMiddleware(middleware.OptionalHumaJWTMiddleware(sc.JWT))
	huma.Get(optionalAuthGroup, "/requests", requestHandler.ListRequestsHandler)
	huma.Get(optionalAuthGroup, "/requests/{request_id}", requestHandler.GetRequestHandler)

	// Protected request endpoints
	huma.Post(protected, "/requests", requestHandler.CreateRequestHandler)
	huma.Put(protected, "/requests/{request_id}", requestHandler.UpdateRequestHandler)
	huma.Delete(protected, "/requests/{request_id}", requestHandler.DeleteRequestHandler)
	huma.Post(protected, "/requests/{request_id}/vote", requestHandler.VoteRequestHandler)
	huma.Delete(protected, "/requests/{request_id}/vote", requestHandler.RemoveVoteRequestHandler)
	huma.Post(protected, "/requests/{request_id}/fulfill", requestHandler.FulfillRequestHandler)
	huma.Post(protected, "/requests/{request_id}/close", requestHandler.CloseRequestHandler)
}

// setupRevisionRoutes configures revision history endpoints.
// Public endpoints for viewing history; admin endpoint for rollback.
func setupRevisionRoutes(api huma.API, protected *huma.Group, sc *services.ServiceContainer) {
	revisionHandler := handlers.NewRevisionHandler(sc.Revision, sc.AuditLog)

	// Public revision endpoints
	huma.Get(api, "/revisions/{entity_type}/{entity_id}", revisionHandler.GetEntityHistoryHandler)
	huma.Get(api, "/revisions/{revision_id}", revisionHandler.GetRevisionHandler)
	huma.Get(api, "/users/{user_id}/revisions", revisionHandler.GetUserRevisionsHandler)

	// Admin rollback endpoint
	huma.Post(protected, "/admin/revisions/{revision_id}/rollback", revisionHandler.RollbackRevisionHandler)
}

// setupTagRoutes configures tag, entity tagging, and tag voting endpoints.
// Public endpoints for browsing tags. Optional auth for entity tags (user's vote).
// Protected endpoints for tagging and voting. Admin endpoints for tag CRUD and aliases.
func setupTagRoutes(api huma.API, protected *huma.Group, sc *services.ServiceContainer) {
	tagHandler := handlers.NewTagHandler(sc.Tag, sc.AuditLog)

	// Public tag endpoints
	huma.Get(api, "/tags", tagHandler.ListTagsHandler)
	huma.Get(api, "/tags/search", tagHandler.SearchTagsHandler)
	huma.Get(api, "/tags/{tag_id}", tagHandler.GetTagHandler)
	huma.Get(api, "/tags/{tag_id}/aliases", tagHandler.ListAliasesHandler)

	// Entity tags with optional auth (includes user's vote if authenticated)
	optionalAuthGroup := huma.NewGroup(api, "")
	optionalAuthGroup.UseMiddleware(middleware.OptionalHumaJWTMiddleware(sc.JWT))
	huma.Get(optionalAuthGroup, "/entities/{entity_type}/{entity_id}/tags", tagHandler.ListEntityTagsHandler)

	// Protected: tagging and voting
	huma.Post(protected, "/entities/{entity_type}/{entity_id}/tags", tagHandler.AddTagToEntityHandler)
	huma.Delete(protected, "/entities/{entity_type}/{entity_id}/tags/{tag_id}", tagHandler.RemoveTagFromEntityHandler)
	huma.Post(protected, "/tags/{tag_id}/entities/{entity_type}/{entity_id}/votes", tagHandler.VoteTagHandler)
	huma.Delete(protected, "/tags/{tag_id}/entities/{entity_type}/{entity_id}/votes", tagHandler.RemoveTagVoteHandler)

	// Admin: tag CRUD and alias management
	huma.Post(protected, "/tags", tagHandler.CreateTagHandler)
	huma.Put(protected, "/tags/{tag_id}", tagHandler.UpdateTagHandler)
	huma.Delete(protected, "/tags/{tag_id}", tagHandler.DeleteTagHandler)
	huma.Post(protected, "/tags/{tag_id}/aliases", tagHandler.CreateAliasHandler)
	huma.Delete(protected, "/tags/{tag_id}/aliases/{alias_id}", tagHandler.DeleteAliasHandler)
}

// setupArtistRelationshipRoutes configures artist relationship and similar artist endpoints.
func setupArtistRelationshipRoutes(api huma.API, protected *huma.Group, sc *services.ServiceContainer) {
	relHandler := handlers.NewArtistRelationshipHandler(sc.ArtistRelationship, sc.AuditLog)

	// Public: get related artists with optional auth (for user's votes)
	optionalAuthGroup := huma.NewGroup(api, "")
	optionalAuthGroup.UseMiddleware(middleware.OptionalHumaJWTMiddleware(sc.JWT))
	huma.Get(optionalAuthGroup, "/artists/{artist_id}/related", relHandler.GetRelatedArtistsHandler)
	huma.Get(optionalAuthGroup, "/artists/{artist_id}/graph", relHandler.GetArtistGraphHandler)

	// Protected: create relationships and vote
	huma.Post(protected, "/artists/relationships", relHandler.CreateRelationshipHandler)
	huma.Post(protected, "/artists/relationships/{source_id}/{target_id}/vote", relHandler.VoteHandler)
	huma.Delete(protected, "/artists/relationships/{source_id}/{target_id}/vote", relHandler.RemoveVoteHandler)

	// Admin: delete relationships
	huma.Delete(protected, "/artists/relationships/{source_id}/{target_id}", relHandler.DeleteRelationshipHandler)
}

// setupSceneRoutes configures scene (city aggregation) endpoints.
// All endpoints are public — no authentication required.
func setupSceneRoutes(api huma.API, sc *services.ServiceContainer) {
	sceneHandler := handlers.NewSceneHandler(sc.Scene)

	huma.Get(api, "/scenes", sceneHandler.ListScenesHandler)
	huma.Get(api, "/scenes/{slug}", sceneHandler.GetSceneDetailHandler)
	huma.Get(api, "/scenes/{slug}/artists", sceneHandler.GetSceneActiveArtistsHandler)
	huma.Get(api, "/scenes/{slug}/genres", sceneHandler.GetSceneGenresHandler)
}

// setupAttendanceRoutes configures show attendance (going/interested) endpoints.
// Public endpoints use optional auth (counts always available; user status if authenticated).
// Set/remove attendance requires authentication.
func setupAttendanceRoutes(api huma.API, protected *huma.Group, sc *services.ServiceContainer) {
	attendanceHandler := handlers.NewAttendanceHandler(sc.Attendance)

	// Public endpoints with optional auth (counts + user status if authenticated)
	optionalAuthGroup := huma.NewGroup(api, "")
	optionalAuthGroup.UseMiddleware(middleware.OptionalHumaJWTMiddleware(sc.JWT))
	huma.Get(optionalAuthGroup, "/shows/{show_id}/attendance", attendanceHandler.GetAttendanceHandler)
	huma.Post(optionalAuthGroup, "/shows/attendance/batch", attendanceHandler.BatchAttendanceHandler)

	// Protected endpoints (require authentication)
	huma.Post(protected, "/shows/{show_id}/attendance", attendanceHandler.SetAttendanceHandler)
	huma.Delete(protected, "/shows/{show_id}/attendance", attendanceHandler.RemoveAttendanceHandler)
	huma.Get(protected, "/attendance/my-shows", attendanceHandler.GetMyShowsHandler)
}

// setupFollowRoutes configures follow/unfollow endpoints for entities.
// Follow/unfollow requires authentication. Follower counts use optional auth
// (counts always available; user follow status if authenticated).
func setupFollowRoutes(api huma.API, protected *huma.Group, sc *services.ServiceContainer) {
	followHandler := handlers.NewFollowHandler(sc.Follow)

	// Optional auth group for public follower counts/lists
	optionalAuthGroup := huma.NewGroup(api, "")
	optionalAuthGroup.UseMiddleware(middleware.OptionalHumaJWTMiddleware(sc.JWT))

	// Follow/unfollow (protected): entity_type is a path param (artists, venues, labels, festivals)
	huma.Post(protected, "/{entity_type}/{entity_id}/follow", followHandler.FollowEntityHandler)
	huma.Delete(protected, "/{entity_type}/{entity_id}/follow", followHandler.UnfollowEntityHandler)

	// Public with optional auth: follower count + user follow status
	huma.Get(optionalAuthGroup, "/{entity_type}/{entity_id}/followers", followHandler.GetFollowersHandler)

	// Public with optional auth: follower list
	huma.Get(optionalAuthGroup, "/{entity_type}/{entity_id}/followers/list", followHandler.GetFollowersListHandler)

	// Batch follow counts (optional auth)
	huma.Post(optionalAuthGroup, "/follows/batch", followHandler.BatchFollowHandler)

	// User's following list (protected)
	huma.Get(protected, "/me/following", followHandler.GetMyFollowingHandler)
}

// setupNotificationFilterRoutes configures notification filter and notification log endpoints.
// CRUD and notifications require authentication. Unsubscribe is public (HMAC-signed).
func setupNotificationFilterRoutes(api huma.API, protected *huma.Group, sc *services.ServiceContainer, cfg *config.Config) {
	filterHandler := handlers.NewNotificationFilterHandler(sc.NotificationFilter, cfg.JWT.SecretKey)

	// Protected: filter CRUD
	huma.Get(protected, "/me/notification-filters", filterHandler.ListFiltersHandler)
	huma.Post(protected, "/me/notification-filters", filterHandler.CreateFilterHandler)
	huma.Patch(protected, "/me/notification-filters/{id}", filterHandler.UpdateFilterHandler)
	huma.Delete(protected, "/me/notification-filters/{id}", filterHandler.DeleteFilterHandler)
	huma.Post(protected, "/me/notification-filters/quick", filterHandler.QuickCreateFilterHandler)

	// Protected: notification log
	huma.Get(protected, "/me/notifications", filterHandler.GetNotificationsHandler)

	// Public: HMAC-signed unsubscribe
	huma.Post(api, "/unsubscribe/filter/{id}", filterHandler.UnsubscribeFilterHandler)
}

// setupChartsRoutes configures public top charts endpoints.
// All endpoints are public — no authentication required.
func setupChartsRoutes(api huma.API, sc *services.ServiceContainer) {
	chartsHandler := handlers.NewChartsHandler(sc.Charts)

	huma.Get(api, "/charts/trending-shows", chartsHandler.GetTrendingShowsHandler)
	huma.Get(api, "/charts/popular-artists", chartsHandler.GetPopularArtistsHandler)
	huma.Get(api, "/charts/active-venues", chartsHandler.GetActiveVenuesHandler)
	huma.Get(api, "/charts/hot-releases", chartsHandler.GetHotReleasesHandler)
	huma.Get(api, "/charts/overview", chartsHandler.GetChartsOverviewHandler)
}

// rateLimitUnlessAPIToken wraps httprate.Limit but skips rate limiting for
// requests authenticated with an API token (phk_ prefix). API tokens are
// admin-only and trusted — they shouldn't be throttled during batch imports.
func rateLimitUnlessAPIToken(requestLimit int, windowLength time.Duration) func(http.Handler) http.Handler {
	limiter := httprate.Limit(
		requestLimit,
		windowLength,
		httprate.WithKeyFuncs(httprate.KeyByIP),
		httprate.WithLimitHandler(rateLimitHandler),
	)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			auth := r.Header.Get("Authorization")
			if strings.HasPrefix(auth, "Bearer phk_") {
				// API token — bypass rate limit
				next.ServeHTTP(w, r)
				return
			}
			// Normal request — apply rate limit
			limiter(next).ServeHTTP(w, r)
		})
	}
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
