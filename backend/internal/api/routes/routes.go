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

// RouteContext holds the shared dependencies passed to every route setup function.
// Each function uses only what it needs from the struct.
type RouteContext struct {
	Router    *chi.Mux                  // The chi mux (for Chi-level middleware groups and raw HTTP routes)
	API       huma.API                  // The public Huma API wrapper
	Protected *huma.Group               // Protected (auth-required) Huma API group
	SC        *services.ServiceContainer // All instantiated services
	Cfg       *config.Config            // Application configuration
}

// SetupRoutes configures all API routes
func SetupRoutes(router *chi.Mux, sc *services.ServiceContainer, cfg *config.Config) huma.API {
	api := humachi.New(router, huma.DefaultConfig("Psychic Homily", "1.0.0"))

	// Add request ID middleware to all Huma routes
	api.UseMiddleware(middleware.HumaRequestIDMiddleware)

	// Enrich Sentry scope with request ID and HTTP metadata on all routes
	api.UseMiddleware(middleware.HumaSentryContextMiddleware)

	// Create a protected group that will require authentication
	protectedGroup := huma.NewGroup(api, "")
	protectedGroup.UseMiddleware(middleware.HumaJWTMiddleware(sc.JWT, cfg.Session))
	// Enrich Sentry scope with authenticated user context (runs after JWT middleware)
	protectedGroup.UseMiddleware(middleware.HumaSentryContextMiddleware)

	// Build the shared RouteContext once, pass to all setup functions
	rc := RouteContext{
		Router:    router,
		API:       api,
		Protected: protectedGroup,
		SC:        sc,
		Cfg:       cfg,
	}

	// Setup domain-specific routes
	setupSystemRoutes(rc)
	setupAuthRoutes(rc)

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
	setupPasskeyRoutes(rc)

	setupShowRoutes(rc)
	setupArtistRoutes(rc)
	setupReleaseRoutes(rc)
	setupLabelRoutes(rc)
	setupFestivalRoutes(rc)
	setupVenueRoutes(rc)
	setupCalendarRoutes(rc)
	setupSavedShowRoutes(rc)
	setupFavoriteVenueRoutes(rc)
	setupShowReportRoutes(rc)
	setupArtistReportRoutes(rc)
	setupAdminRoutes(rc)
	setupPipelineRoutes(rc)
	setupContributorProfileRoutes(rc)
	setupCollectionRoutes(rc)
	setupRequestRoutes(rc)
	setupRevisionRoutes(rc)
	setupTagRoutes(rc)
	setupArtistRelationshipRoutes(rc)
	setupSceneRoutes(rc)
	setupAttendanceRoutes(rc)
	setupFollowRoutes(rc)
	setupNotificationFilterRoutes(rc)
	setupChartsRoutes(rc)
	setupPendingEditRoutes(rc)
	setupEntityReportRoutes(rc)
	setupContributeRoutes(rc)

	return api
}

// setupAuthRoutes configures all authentication-related endpoints
func setupAuthRoutes(rc RouteContext) {
	authHandler := handlers.NewAuthHandler(rc.SC.Auth, rc.SC.JWT, rc.SC.User, rc.SC.Email, rc.SC.Discord, rc.SC.PasswordValidator, rc.Cfg)
	oauthHTTPHandler := handlers.NewOAuthHTTPHandler(rc.SC.Auth, rc.Cfg)

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
	rc.Router.Group(func(r chi.Router) {
		r.Use(authRateLimiter)
		r.Get("/auth/login/{provider}", oauthHTTPHandler.OAuthLoginHTTPHandler)
		r.Get("/auth/callback/{provider}", oauthHTTPHandler.OAuthCallbackHTTPHandler)
	})

	// Rate-limited auth API endpoints using Chi middleware wrapper
	// We register these directly on the router with rate limiting, then Huma picks them up
	rc.Router.Group(func(r chi.Router) {
		r.Use(authRateLimiter)

		// Create a sub-API for rate-limited routes
		rateLimitedAPI := humachi.New(r, huma.DefaultConfig("Psychic Homily Auth", "1.0.0"))
		rateLimitedAPI.UseMiddleware(middleware.HumaRequestIDMiddleware)

		huma.Post(rateLimitedAPI, "/auth/login", authHandler.LoginHandler)
		huma.Post(rateLimitedAPI, "/auth/register", authHandler.RegisterHandler)
		huma.Post(rateLimitedAPI, "/auth/magic-link/send", authHandler.SendMagicLinkHandler)
		huma.Post(rateLimitedAPI, "/auth/magic-link/verify", authHandler.VerifyMagicLinkHandler)

		// Sign in with Apple (public, rate-limited)
		appleAuthHandler := handlers.NewAppleAuthHandler(rc.SC.AppleAuth, rc.SC.Discord, rc.Cfg)
		huma.Post(rateLimitedAPI, "/auth/apple/callback", appleAuthHandler.AppleCallbackHandler)

		// Account recovery endpoints (public, rate-limited)
		huma.Post(rateLimitedAPI, "/auth/recover-account", authHandler.RecoverAccountHandler)
		huma.Post(rateLimitedAPI, "/auth/recover-account/request", authHandler.RequestAccountRecoveryHandler)
		huma.Post(rateLimitedAPI, "/auth/recover-account/confirm", authHandler.ConfirmAccountRecoveryHandler)
	})

	// Logout doesn't need strict rate limiting (already requires valid session)
	huma.Post(rc.API, "/auth/logout", authHandler.LogoutHandler)
}

// setupSystemRoutes configures system/infrastructure endpoints
func setupSystemRoutes(rc RouteContext) {
	// Health check endpoint
	huma.Get(rc.API, "/health", handlers.HealthHandler)

	// OpenAPI specification endpoint
	api := rc.API
	rc.Router.Get("/openapi.json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(api.OpenAPI())
	})
}

// setupPasskeyRoutes configures WebAuthn/passkey endpoints
func setupPasskeyRoutes(rc RouteContext) {
	if rc.SC.WebAuthn == nil {
		// WebAuthn service failed to initialize - passkeys are optional
		return
	}

	passkeyHandler := handlers.NewPasskeyHandler(rc.SC.WebAuthn, rc.SC.JWT, rc.SC.User, rc.Cfg)

	// Create rate limiter for passkey endpoints: 20 requests per minute per IP
	// Slightly more lenient than auth due to multi-step WebAuthn flow
	passkeyRateLimiter := httprate.Limit(
		20,              // requests
		1*time.Minute,   // per duration
		httprate.WithKeyFuncs(httprate.KeyByIP),
		httprate.WithLimitHandler(rateLimitHandler),
	)

	// Rate-limited public passkey endpoints
	rc.Router.Group(func(r chi.Router) {
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
	huma.Post(rc.Protected, "/auth/passkey/register/begin", passkeyHandler.BeginRegisterHandler)
	huma.Post(rc.Protected, "/auth/passkey/register/finish", passkeyHandler.FinishRegisterHandler)

	// Protected passkey management endpoints
	huma.Get(rc.Protected, "/auth/passkey/credentials", passkeyHandler.ListCredentialsHandler)
	huma.Delete(rc.Protected, "/auth/passkey/credentials/{credential_id}", passkeyHandler.DeleteCredentialHandler)
}

// setupShowRoutes configures all show-related endpoints
func setupShowRoutes(rc RouteContext) {
	showHandler := handlers.NewShowHandler(rc.SC.Show, rc.SC.Show, rc.SC.Show, rc.SC.SavedShow, rc.SC.Discord, rc.SC.MusicDiscovery, rc.SC.Extraction)

	// Public show endpoints - registered on main API without middleware
	// Note: Static routes must come before parameterized routes
	huma.Get(rc.API, "/shows", showHandler.GetShowsHandler)
	huma.Get(rc.API, "/shows/cities", showHandler.GetShowCitiesHandler)
	huma.Get(rc.API, "/shows/upcoming", showHandler.GetUpcomingShowsHandler)

	// Show detail with optional auth for access control on non-approved shows
	optionalAuthGroup := huma.NewGroup(rc.API, "")
	optionalAuthGroup.UseMiddleware(middleware.OptionalHumaJWTMiddleware(rc.SC.JWT))
	huma.Get(optionalAuthGroup, "/shows/{show_id}", showHandler.GetShowHandler)

	// Export endpoint - only register in development environment
	if os.Getenv("ENVIRONMENT") == "development" {
		huma.Get(rc.API, "/shows/{show_id}/export", showHandler.ExportShowHandler)
	}

	// Rate-limited show creation: 10 requests per hour per IP
	// Prevents flooding the admin approval queue
	// API token requests (phk_ prefix) bypass the rate limit — they're trusted admin clients
	rc.Router.Group(func(r chi.Router) {
		r.Use(rateLimitUnlessAPIToken(
			middleware.ShowCreateRequestsPerHour,
			time.Hour,
		))
		showCreateAPI := humachi.New(r, huma.DefaultConfig("Psychic Homily Show Create", "1.0.0"))
		showCreateAPI.UseMiddleware(middleware.HumaRequestIDMiddleware)
		showCreateAPI.UseMiddleware(middleware.HumaJWTMiddleware(rc.SC.JWT, rc.Cfg.Session))
		huma.Post(showCreateAPI, "/shows", showHandler.CreateShowHandler)
	})

	// Rate-limited AI processing: 5 requests per minute per IP
	// Calls external Anthropic API — expensive operation
	rc.Router.Group(func(r chi.Router) {
		r.Use(httprate.Limit(
			middleware.AIProcessRequestsPerMinute,
			time.Minute,
			httprate.WithKeyFuncs(httprate.KeyByIP),
			httprate.WithLimitHandler(rateLimitHandler),
		))
		aiProcessAPI := humachi.New(r, huma.DefaultConfig("Psychic Homily AI Process", "1.0.0"))
		aiProcessAPI.UseMiddleware(middleware.HumaRequestIDMiddleware)
		aiProcessAPI.UseMiddleware(middleware.HumaJWTMiddleware(rc.SC.JWT, rc.Cfg.Session))
		huma.Post(aiProcessAPI, "/shows/ai-process", showHandler.AIProcessShowHandler)
	})

	// Protected show endpoints (no additional rate limiting needed)
	huma.Put(rc.Protected, "/shows/{show_id}", showHandler.UpdateShowHandler)
	huma.Delete(rc.Protected, "/shows/{show_id}", showHandler.DeleteShowHandler)
	huma.Post(rc.Protected, "/shows/{show_id}/unpublish", showHandler.UnpublishShowHandler)
	huma.Post(rc.Protected, "/shows/{show_id}/make-private", showHandler.MakePrivateShowHandler)
	huma.Post(rc.Protected, "/shows/{show_id}/publish", showHandler.PublishShowHandler)
	huma.Post(rc.Protected, "/shows/{show_id}/sold-out", showHandler.SetShowSoldOutHandler)
	huma.Post(rc.Protected, "/shows/{show_id}/cancelled", showHandler.SetShowCancelledHandler)
	huma.Get(rc.Protected, "/shows/my-submissions", showHandler.GetMySubmissionsHandler)
}

func setupArtistRoutes(rc RouteContext) {
	artistHandler := handlers.NewArtistHandler(rc.SC.Artist, rc.SC.AuditLog, rc.SC.Revision)

	// Public artist endpoints - registered on main API without middleware
	// Note: Static routes must come before parameterized routes
	huma.Get(rc.API, "/artists", artistHandler.ListArtistsHandler)
	huma.Get(rc.API, "/artists/cities", artistHandler.GetArtistCitiesHandler)
	huma.Get(rc.API, "/artists/search", artistHandler.SearchArtistsHandler)
	huma.Get(rc.API, "/artists/{artist_id}", artistHandler.GetArtistHandler)
	huma.Get(rc.API, "/artists/{artist_id}/shows", artistHandler.GetArtistShowsHandler)
	huma.Get(rc.API, "/artists/{artist_id}/labels", artistHandler.GetArtistLabelsHandler)
	huma.Get(rc.API, "/artists/{artist_id}/aliases", artistHandler.GetArtistAliasesHandler)

	// Protected artist endpoints
	huma.Delete(rc.Protected, "/artists/{artist_id}", artistHandler.DeleteArtistHandler)
	huma.Post(rc.Protected, "/admin/artists", artistHandler.AdminCreateArtistHandler)
	huma.Patch(rc.Protected, "/admin/artists/{artist_id}", artistHandler.AdminUpdateArtistHandler)
	huma.Post(rc.Protected, "/admin/artists/{artist_id}/aliases", artistHandler.AddArtistAliasHandler)
	huma.Delete(rc.Protected, "/admin/artists/{artist_id}/aliases/{alias_id}", artistHandler.DeleteArtistAliasHandler)
	huma.Post(rc.Protected, "/admin/artists/merge", artistHandler.MergeArtistsHandler)
}

func setupReleaseRoutes(rc RouteContext) {
	releaseHandler := handlers.NewReleaseHandler(rc.SC.Release, rc.SC.Artist, rc.SC.AuditLog)

	// Public release endpoints
	// Note: Static routes must come before parameterized routes
	huma.Get(rc.API, "/releases", releaseHandler.ListReleasesHandler)
	huma.Get(rc.API, "/releases/search", releaseHandler.SearchReleasesHandler)
	huma.Get(rc.API, "/releases/{release_id}", releaseHandler.GetReleaseHandler)
	huma.Get(rc.API, "/artists/{artist_id}/releases", releaseHandler.GetArtistReleasesHandler)

	// Protected release endpoints (admin-only checks inside handlers)
	huma.Post(rc.Protected, "/releases", releaseHandler.CreateReleaseHandler)
	huma.Put(rc.Protected, "/releases/{release_id}", releaseHandler.UpdateReleaseHandler)
	huma.Delete(rc.Protected, "/releases/{release_id}", releaseHandler.DeleteReleaseHandler)
	huma.Post(rc.Protected, "/releases/{release_id}/links", releaseHandler.AddExternalLinkHandler)
	huma.Delete(rc.Protected, "/releases/{release_id}/links/{link_id}", releaseHandler.RemoveExternalLinkHandler)
}

func setupLabelRoutes(rc RouteContext) {
	labelHandler := handlers.NewLabelHandler(rc.SC.Label, rc.SC.AuditLog)

	// Public label endpoints
	// Note: Static routes must come before parameterized routes
	huma.Get(rc.API, "/labels", labelHandler.ListLabelsHandler)
	huma.Get(rc.API, "/labels/search", labelHandler.SearchLabelsHandler)
	huma.Get(rc.API, "/labels/{label_id}", labelHandler.GetLabelHandler)
	huma.Get(rc.API, "/labels/{label_id}/artists", labelHandler.GetLabelRosterHandler)
	huma.Get(rc.API, "/labels/{label_id}/releases", labelHandler.GetLabelCatalogHandler)

	// Protected label endpoints (admin-only checks inside handlers)
	huma.Post(rc.Protected, "/labels", labelHandler.CreateLabelHandler)
	huma.Put(rc.Protected, "/labels/{label_id}", labelHandler.UpdateLabelHandler)
	huma.Delete(rc.Protected, "/labels/{label_id}", labelHandler.DeleteLabelHandler)
	huma.Post(rc.Protected, "/admin/labels/{label_id}/artists", labelHandler.AddArtistToLabelHandler)
	huma.Post(rc.Protected, "/admin/labels/{label_id}/releases", labelHandler.AddReleaseToLabelHandler)
}

func setupFestivalRoutes(rc RouteContext) {
	festivalHandler := handlers.NewFestivalHandler(rc.SC.Festival, rc.SC.Artist, rc.SC.AuditLog, rc.SC.Revision)

	// Public festival endpoints
	// Note: Static routes must come before parameterized routes
	huma.Get(rc.API, "/festivals", festivalHandler.ListFestivalsHandler)
	huma.Get(rc.API, "/festivals/search", festivalHandler.SearchFestivalsHandler)
	huma.Get(rc.API, "/festivals/{festival_id}", festivalHandler.GetFestivalHandler)
	huma.Get(rc.API, "/festivals/{festival_id}/artists", festivalHandler.GetFestivalArtistsHandler)
	huma.Get(rc.API, "/festivals/{festival_id}/venues", festivalHandler.GetFestivalVenuesHandler)
	huma.Get(rc.API, "/artists/{artist_id}/festivals", festivalHandler.GetArtistFestivalsHandler)

	// Festival intelligence endpoints (public, computed from existing data)
	intelHandler := handlers.NewFestivalIntelligenceHandler(rc.SC.FestivalIntelligence, rc.SC.Festival, rc.SC.Artist)
	huma.Get(rc.API, "/festivals/{festival_id}/similar", intelHandler.GetSimilarFestivalsHandler)
	huma.Get(rc.API, "/festivals/{festival_a_id}/overlap/{festival_b_id}", intelHandler.GetFestivalOverlapHandler)
	huma.Get(rc.API, "/festivals/{festival_id}/breakouts", intelHandler.GetFestivalBreakoutsHandler)
	huma.Get(rc.API, "/artists/{artist_id}/festival-trajectory", intelHandler.GetArtistFestivalTrajectoryHandler)
	huma.Get(rc.API, "/festivals/series/{series_slug}/compare", intelHandler.GetSeriesComparisonHandler)

	// Protected festival endpoints (admin-only checks inside handlers)
	huma.Post(rc.Protected, "/festivals", festivalHandler.CreateFestivalHandler)
	huma.Put(rc.Protected, "/festivals/{festival_id}", festivalHandler.UpdateFestivalHandler)
	huma.Delete(rc.Protected, "/festivals/{festival_id}", festivalHandler.DeleteFestivalHandler)
	huma.Post(rc.Protected, "/festivals/{festival_id}/artists", festivalHandler.AddFestivalArtistHandler)
	huma.Put(rc.Protected, "/festivals/{festival_id}/artists/{artist_id}", festivalHandler.UpdateFestivalArtistHandler)
	huma.Delete(rc.Protected, "/festivals/{festival_id}/artists/{artist_id}", festivalHandler.RemoveFestivalArtistHandler)
	huma.Post(rc.Protected, "/festivals/{festival_id}/venues", festivalHandler.AddFestivalVenueHandler)
	huma.Delete(rc.Protected, "/festivals/{festival_id}/venues/{venue_id}", festivalHandler.RemoveFestivalVenueHandler)
}

func setupVenueRoutes(rc RouteContext) {
	venueHandler := handlers.NewVenueHandler(rc.SC.Venue, rc.SC.Discord, rc.SC.AuditLog, rc.SC.Revision)

	// Public venue endpoints - registered on main API without middleware
	// Note: Static routes must come before parameterized routes
	huma.Get(rc.API, "/venues", venueHandler.ListVenuesHandler)
	huma.Get(rc.API, "/venues/cities", venueHandler.GetVenueCitiesHandler)
	huma.Get(rc.API, "/venues/search", venueHandler.SearchVenuesHandler)
	huma.Get(rc.API, "/venues/{venue_id}", venueHandler.GetVenueHandler)
	huma.Get(rc.API, "/venues/{venue_id}/shows", venueHandler.GetVenueShowsHandler)
	huma.Get(rc.API, "/venues/{venue_id}/genres", venueHandler.GetVenueGenresHandler)

	// Protected venue endpoints - require authentication
	huma.Post(rc.Protected, "/admin/venues", venueHandler.AdminCreateVenueHandler)
	huma.Put(rc.Protected, "/venues/{venue_id}", venueHandler.UpdateVenueHandler)
	huma.Delete(rc.Protected, "/venues/{venue_id}", venueHandler.DeleteVenueHandler)
	huma.Get(rc.Protected, "/venues/{venue_id}/my-pending-edit", venueHandler.GetMyPendingEditHandler)
	huma.Delete(rc.Protected, "/venues/{venue_id}/my-pending-edit", venueHandler.CancelMyPendingEditHandler)
}

// setupCalendarRoutes configures calendar feed and token management endpoints
func setupCalendarRoutes(rc RouteContext) {
	calendarHandler := handlers.NewCalendarHandler(rc.SC.Calendar, rc.Cfg)

	// Public Chi route for ICS feed (token-authenticated, not JWT)
	rc.Router.Get("/calendar/{token}", calendarHandler.GetCalendarFeedHandler)

	// Protected Huma routes for token CRUD
	huma.Post(rc.Protected, "/calendar/token", calendarHandler.CreateCalendarTokenHandler)
	huma.Get(rc.Protected, "/calendar/token", calendarHandler.GetCalendarTokenStatusHandler)
	huma.Delete(rc.Protected, "/calendar/token", calendarHandler.DeleteCalendarTokenHandler)
}

// setupSavedShowRoutes configures saved show endpoints (user's personal "My List")
// All endpoints require authentication via protected group
func setupSavedShowRoutes(rc RouteContext) {
	savedShowHandler := handlers.NewSavedShowHandler(rc.SC.SavedShow)

	// Protected saved show endpoints
	huma.Post(rc.Protected, "/saved-shows/{show_id}", savedShowHandler.SaveShowHandler)
	huma.Delete(rc.Protected, "/saved-shows/{show_id}", savedShowHandler.UnsaveShowHandler)
	huma.Get(rc.Protected, "/saved-shows", savedShowHandler.GetSavedShowsHandler)
	huma.Get(rc.Protected, "/saved-shows/{show_id}/check", savedShowHandler.CheckSavedHandler)
	huma.Post(rc.Protected, "/saved-shows/check-batch", savedShowHandler.CheckBatchSavedHandler)
}

// setupFavoriteVenueRoutes configures favorite venue endpoints
// All endpoints require authentication via protected group
func setupFavoriteVenueRoutes(rc RouteContext) {
	favoriteVenueHandler := handlers.NewFavoriteVenueHandler(rc.SC.FavoriteVenue)

	// Protected favorite venue endpoints
	huma.Post(rc.Protected, "/favorite-venues/{venue_id}", favoriteVenueHandler.FavoriteVenueHandler)
	huma.Delete(rc.Protected, "/favorite-venues/{venue_id}", favoriteVenueHandler.UnfavoriteVenueHandler)
	huma.Get(rc.Protected, "/favorite-venues", favoriteVenueHandler.GetFavoriteVenuesHandler)
	huma.Get(rc.Protected, "/favorite-venues/{venue_id}/check", favoriteVenueHandler.CheckFavoritedHandler)
	huma.Get(rc.Protected, "/favorite-venues/shows", favoriteVenueHandler.GetFavoriteVenueShowsHandler)
}

// setupShowReportRoutes configures show report endpoints
// All endpoints require authentication via protected group
func setupShowReportRoutes(rc RouteContext) {
	showReportHandler := handlers.NewShowReportHandler(rc.SC.ShowReport, rc.SC.Discord, rc.SC.User, rc.SC.AuditLog)

	// Rate-limited report submission: 5 requests per minute per IP
	// Prevents spamming admins with reports
	rc.Router.Group(func(r chi.Router) {
		r.Use(httprate.Limit(
			middleware.ReportRequestsPerMinute,
			time.Minute,
			httprate.WithKeyFuncs(httprate.KeyByIP),
			httprate.WithLimitHandler(rateLimitHandler),
		))
		reportAPI := humachi.New(r, huma.DefaultConfig("Psychic Homily Reports", "1.0.0"))
		reportAPI.UseMiddleware(middleware.HumaRequestIDMiddleware)
		reportAPI.UseMiddleware(middleware.HumaJWTMiddleware(rc.SC.JWT, rc.Cfg.Session))
		huma.Post(reportAPI, "/shows/{show_id}/report", showReportHandler.ReportShowHandler)
	})

	// Protected report endpoints (no additional rate limiting)
	huma.Get(rc.Protected, "/shows/{show_id}/my-report", showReportHandler.GetMyReportHandler)

	// Admin endpoints for managing reports
	huma.Get(rc.Protected, "/admin/reports", showReportHandler.GetPendingReportsHandler)
	huma.Post(rc.Protected, "/admin/reports/{report_id}/dismiss", showReportHandler.DismissReportHandler)
	huma.Post(rc.Protected, "/admin/reports/{report_id}/resolve", showReportHandler.ResolveReportHandler)
}

// setupArtistReportRoutes configures artist report endpoints
func setupArtistReportRoutes(rc RouteContext) {
	artistReportHandler := handlers.NewArtistReportHandler(rc.SC.ArtistReport, rc.SC.Discord, rc.SC.User, rc.SC.AuditLog)

	// Rate-limited report submission: 5 requests per minute per IP
	rc.Router.Group(func(r chi.Router) {
		r.Use(httprate.Limit(
			middleware.ReportRequestsPerMinute,
			time.Minute,
			httprate.WithKeyFuncs(httprate.KeyByIP),
			httprate.WithLimitHandler(rateLimitHandler),
		))
		reportAPI := humachi.New(r, huma.DefaultConfig("Psychic Homily Artist Reports", "1.0.0"))
		reportAPI.UseMiddleware(middleware.HumaRequestIDMiddleware)
		reportAPI.UseMiddleware(middleware.HumaJWTMiddleware(rc.SC.JWT, rc.Cfg.Session))
		huma.Post(reportAPI, "/artists/{artist_id}/report", artistReportHandler.ReportArtistHandler)
	})

	// Protected report endpoints (no additional rate limiting)
	huma.Get(rc.Protected, "/artists/{artist_id}/my-report", artistReportHandler.GetMyArtistReportHandler)

	// Admin endpoints for managing artist reports
	huma.Get(rc.Protected, "/admin/artist-reports", artistReportHandler.GetPendingArtistReportsHandler)
	huma.Post(rc.Protected, "/admin/artist-reports/{report_id}/dismiss", artistReportHandler.DismissArtistReportHandler)
	huma.Post(rc.Protected, "/admin/artist-reports/{report_id}/resolve", artistReportHandler.ResolveArtistReportHandler)
}

// setupAdminRoutes configures admin-only endpoints
// Note: Admin check is performed inside handlers, JWT auth is required via protected group
func setupAdminRoutes(rc RouteContext) {
	// Domain-specific admin handlers
	statsHandler := handlers.NewAdminStatsHandler(rc.SC.AdminStats)
	showHandler := handlers.NewAdminShowHandler(
		rc.SC.Show, rc.SC.Show, rc.SC.Show, rc.SC.Discord, rc.SC.AuditLog, rc.SC.NotificationFilter,
		rc.SC.MusicDiscovery,
	)
	venueHandler := handlers.NewAdminVenueHandler(rc.SC.Venue, rc.SC.AuditLog)
	userHandler := handlers.NewAdminUserHandler(rc.SC.User)
	tokenHandler := handlers.NewAdminTokenHandler(rc.SC.APIToken)
	dataHandler := handlers.NewAdminDataHandler(rc.SC.DataSync)
	discoveryHandler := handlers.NewAdminDiscoveryHandler(rc.SC.Discovery)

	artistHandler := handlers.NewArtistHandler(rc.SC.Artist, rc.SC.AuditLog, rc.SC.Revision)
	auditLogHandler := handlers.NewAuditLogHandler(rc.SC.AuditLog)

	// Admin dashboard stats endpoint
	huma.Get(rc.Protected, "/admin/stats", statsHandler.GetAdminStatsHandler)
	huma.Get(rc.Protected, "/admin/activity", statsHandler.GetActivityFeedHandler)

	// Admin show listing endpoint (for CLI export)
	huma.Get(rc.Protected, "/admin/shows", showHandler.GetAdminShowsHandler)

	// Admin show management endpoints
	huma.Get(rc.Protected, "/admin/shows/pending", showHandler.GetPendingShowsHandler)
	huma.Get(rc.Protected, "/admin/shows/rejected", showHandler.GetRejectedShowsHandler)
	huma.Post(rc.Protected, "/admin/shows/{show_id}/approve", showHandler.ApproveShowHandler)
	huma.Post(rc.Protected, "/admin/shows/{show_id}/reject", showHandler.RejectShowHandler)
	huma.Post(rc.Protected, "/admin/shows/batch-approve", showHandler.BatchApproveShowsHandler)
	huma.Post(rc.Protected, "/admin/shows/batch-reject", showHandler.BatchRejectShowsHandler)

	// Admin show import endpoints (single)
	huma.Post(rc.Protected, "/admin/shows/import/preview", showHandler.ImportShowPreviewHandler)
	huma.Post(rc.Protected, "/admin/shows/import/confirm", showHandler.ImportShowConfirmHandler)

	// Admin show export/import endpoints (bulk - for CLI)
	huma.Post(rc.Protected, "/admin/shows/export/bulk", showHandler.BulkExportShowsHandler)
	huma.Post(rc.Protected, "/admin/shows/import/bulk/preview", showHandler.BulkImportPreviewHandler)
	huma.Post(rc.Protected, "/admin/shows/import/bulk/confirm", showHandler.BulkImportConfirmHandler)

	// Admin venue management endpoints
	huma.Get(rc.Protected, "/admin/venues/unverified", venueHandler.GetUnverifiedVenuesHandler)
	huma.Post(rc.Protected, "/admin/venues/{venue_id}/verify", venueHandler.VerifyVenueHandler)

	// Admin pending venue edit endpoints
	huma.Get(rc.Protected, "/admin/venues/pending-edits", venueHandler.GetPendingVenueEditsHandler)
	huma.Post(rc.Protected, "/admin/venues/pending-edits/{edit_id}/approve", venueHandler.ApproveVenueEditHandler)
	huma.Post(rc.Protected, "/admin/venues/pending-edits/{edit_id}/reject", venueHandler.RejectVenueEditHandler)

	// Admin artist management endpoints
	huma.Patch(rc.Protected, "/admin/artists/{artist_id}/bandcamp", artistHandler.UpdateArtistBandcampHandler)
	huma.Patch(rc.Protected, "/admin/artists/{artist_id}/spotify", artistHandler.UpdateArtistSpotifyHandler)

	// Admin discovery endpoints (for local discovery app)
	huma.Post(rc.Protected, "/admin/discovery/import", discoveryHandler.DiscoveryImportHandler)
	huma.Post(rc.Protected, "/admin/discovery/check", discoveryHandler.DiscoveryCheckHandler)

	// Admin API token management endpoints
	huma.Post(rc.Protected, "/admin/tokens", tokenHandler.CreateAPITokenHandler)
	huma.Get(rc.Protected, "/admin/tokens", tokenHandler.ListAPITokensHandler)
	huma.Delete(rc.Protected, "/admin/tokens/{token_id}", tokenHandler.RevokeAPITokenHandler)

	// Admin data export endpoints (for syncing local data to Stage/Production)
	huma.Get(rc.Protected, "/admin/export/shows", dataHandler.ExportShowsHandler)
	huma.Get(rc.Protected, "/admin/export/artists", dataHandler.ExportArtistsHandler)
	huma.Get(rc.Protected, "/admin/export/venues", dataHandler.ExportVenuesHandler)

	// Admin data import endpoint (for syncing local data to Stage/Production)
	huma.Post(rc.Protected, "/admin/data/import", dataHandler.DataImportHandler)

	// Admin audit log endpoint
	huma.Get(rc.Protected, "/admin/audit-logs", auditLogHandler.GetAuditLogsHandler)

	// Admin user list endpoint
	huma.Get(rc.Protected, "/admin/users", userHandler.GetAdminUsersHandler)

	// Admin data quality endpoints
	dataQualityHandler := handlers.NewDataQualityHandler(rc.SC.DataQuality)
	huma.Get(rc.Protected, "/admin/data-quality", dataQualityHandler.GetDataQualitySummaryHandler)
	huma.Get(rc.Protected, "/admin/data-quality/{category}", dataQualityHandler.GetDataQualityCategoryHandler)

	// Admin analytics endpoints
	analyticsHandler := handlers.NewAnalyticsHandler(rc.SC.Analytics)
	huma.Get(rc.Protected, "/admin/analytics/growth", analyticsHandler.GetGrowthMetricsHandler)
	huma.Get(rc.Protected, "/admin/analytics/engagement", analyticsHandler.GetEngagementMetricsHandler)
	huma.Get(rc.Protected, "/admin/analytics/community", analyticsHandler.GetCommunityHealthHandler)
	huma.Get(rc.Protected, "/admin/analytics/data-quality", analyticsHandler.GetDataQualityTrendsHandler)
}

// setupContributorProfileRoutes configures contributor profile endpoints
func setupContributorProfileRoutes(rc RouteContext) {
	profileHandler := handlers.NewContributorProfileHandler(rc.SC.ContributorProfile, rc.SC.User)

	// Public profile endpoints with optional auth (so profile owner can see their own private profile)
	optionalAuthGroup := huma.NewGroup(rc.API, "")
	optionalAuthGroup.UseMiddleware(middleware.OptionalHumaJWTMiddleware(rc.SC.JWT))
	huma.Get(optionalAuthGroup, "/users/{username}", profileHandler.GetPublicProfileHandler)
	huma.Get(optionalAuthGroup, "/users/{username}/contributions", profileHandler.GetContributionHistoryHandler)
	huma.Get(optionalAuthGroup, "/users/{username}/sections", profileHandler.GetUserSectionsHandler)

	// Protected endpoints for authenticated user's own profile
	huma.Get(rc.Protected, "/auth/profile/contributor", profileHandler.GetOwnProfileHandler)
	huma.Get(rc.Protected, "/auth/profile/contributions", profileHandler.GetOwnContributionsHandler)
	huma.Patch(rc.Protected, "/auth/profile/visibility", profileHandler.UpdateProfileVisibilityHandler)
	huma.Patch(rc.Protected, "/auth/profile/privacy", profileHandler.UpdatePrivacySettingsHandler)
	huma.Get(rc.Protected, "/auth/profile/sections", profileHandler.GetOwnSectionsHandler)
	huma.Post(rc.Protected, "/auth/profile/sections", profileHandler.CreateSectionHandler)
	huma.Put(rc.Protected, "/auth/profile/sections/{section_id}", profileHandler.UpdateSectionHandler)
	huma.Delete(rc.Protected, "/auth/profile/sections/{section_id}", profileHandler.DeleteSectionHandler)
}

// setupPipelineRoutes configures AI extraction pipeline admin endpoints.
// Admin check is performed inside handlers, JWT auth is required via protected group.
func setupPipelineRoutes(rc RouteContext) {
	pipelineHandler := handlers.NewPipelineHandler(rc.SC.Pipeline, rc.SC.VenueSourceConfig, rc.SC.Enrichment)

	huma.Post(rc.Protected, "/admin/pipeline/extract/{venue_id}", pipelineHandler.ExtractVenueHandler)
	huma.Get(rc.Protected, "/admin/pipeline/imports", pipelineHandler.GetImportHistoryHandler)
	huma.Get(rc.Protected, "/admin/pipeline/venues", pipelineHandler.ListPipelineVenuesHandler)
	huma.Get(rc.Protected, "/admin/pipeline/venues/{venue_id}/stats", pipelineHandler.VenueRejectionStatsHandler)
	huma.Patch(rc.Protected, "/admin/pipeline/venues/{venue_id}/notes", pipelineHandler.UpdateExtractionNotesHandler)
	huma.Put(rc.Protected, "/admin/pipeline/venues/{venue_id}/config", pipelineHandler.UpdateVenueConfigHandler)
	huma.Get(rc.Protected, "/admin/pipeline/venues/{venue_id}/runs", pipelineHandler.GetVenueRunsHandler)
	huma.Post(rc.Protected, "/admin/pipeline/venues/{venue_id}/reset-render-method", pipelineHandler.ResetRenderMethodHandler)
	huma.Get(rc.Protected, "/admin/pipeline/enrichment/status", pipelineHandler.EnrichmentStatusHandler)
	huma.Post(rc.Protected, "/admin/pipeline/enrichment/trigger/{show_id}", pipelineHandler.TriggerEnrichmentHandler)
}

// setupCollectionRoutes configures crate (formerly "collection") endpoints.
// Both /crates/ and /collections/ paths are registered for backward compatibility.
// Public endpoints use optional auth (for private crate access checks).
// CRUD, item management, and subscription endpoints require authentication.
func setupCollectionRoutes(rc RouteContext) {
	collectionHandler := handlers.NewCollectionHandler(rc.SC.Collection, rc.SC.AuditLog)

	// Public crate endpoints with optional auth
	optionalAuthGroup := huma.NewGroup(rc.API, "")
	optionalAuthGroup.UseMiddleware(middleware.OptionalHumaJWTMiddleware(rc.SC.JWT))

	// Canonical /crates/ paths
	huma.Get(optionalAuthGroup, "/crates", collectionHandler.ListCollectionsHandler)
	huma.Get(optionalAuthGroup, "/crates/{slug}", collectionHandler.GetCollectionHandler)
	huma.Get(optionalAuthGroup, "/crates/{slug}/stats", collectionHandler.GetCollectionStatsHandler)

	// Legacy /collections/ paths (backward compat — same handlers, different operation IDs)
	huma.Get(optionalAuthGroup, "/collections", collectionHandler.ListCollectionsHandler)
	huma.Get(optionalAuthGroup, "/collections/{slug}", collectionHandler.GetCollectionHandler)
	huma.Get(optionalAuthGroup, "/collections/{slug}/stats", collectionHandler.GetCollectionStatsHandler)

	// Protected crate endpoints — canonical /crates/ paths
	huma.Post(rc.Protected, "/crates", collectionHandler.CreateCollectionHandler)
	huma.Put(rc.Protected, "/crates/{slug}", collectionHandler.UpdateCollectionHandler)
	huma.Delete(rc.Protected, "/crates/{slug}", collectionHandler.DeleteCollectionHandler)

	// Protected crate endpoints — legacy /collections/ paths (backward compat)
	huma.Post(rc.Protected, "/collections", collectionHandler.CreateCollectionHandler)
	huma.Put(rc.Protected, "/collections/{slug}", collectionHandler.UpdateCollectionHandler)
	huma.Delete(rc.Protected, "/collections/{slug}", collectionHandler.DeleteCollectionHandler)

	// Crate item management — canonical /crates/ paths
	huma.Post(rc.Protected, "/crates/{slug}/items", collectionHandler.AddItemHandler)
	huma.Delete(rc.Protected, "/crates/{slug}/items/{item_id}", collectionHandler.RemoveItemHandler)
	huma.Put(rc.Protected, "/crates/{slug}/items/reorder", collectionHandler.ReorderItemsHandler)

	// Crate item management — legacy /collections/ paths (backward compat)
	huma.Post(rc.Protected, "/collections/{slug}/items", collectionHandler.AddItemHandler)
	huma.Delete(rc.Protected, "/collections/{slug}/items/{item_id}", collectionHandler.RemoveItemHandler)
	huma.Put(rc.Protected, "/collections/{slug}/items/reorder", collectionHandler.ReorderItemsHandler)

	// Crate subscription — canonical /crates/ paths
	huma.Post(rc.Protected, "/crates/{slug}/subscribe", collectionHandler.SubscribeHandler)
	huma.Delete(rc.Protected, "/crates/{slug}/subscribe", collectionHandler.UnsubscribeHandler)

	// Crate subscription — legacy /collections/ paths (backward compat)
	huma.Post(rc.Protected, "/collections/{slug}/subscribe", collectionHandler.SubscribeHandler)
	huma.Delete(rc.Protected, "/collections/{slug}/subscribe", collectionHandler.UnsubscribeHandler)

	// Admin: feature/unfeature crates — canonical /crates/ paths
	huma.Put(rc.Protected, "/crates/{slug}/feature", collectionHandler.SetFeaturedHandler)

	// Admin: feature/unfeature crates — legacy /collections/ paths (backward compat)
	huma.Put(rc.Protected, "/collections/{slug}/feature", collectionHandler.SetFeaturedHandler)

	// User's own crates (created + subscribed)
	huma.Get(rc.Protected, "/auth/crates", collectionHandler.GetUserCollectionsHandler)

	// Legacy user crates path (backward compat)
	huma.Get(rc.Protected, "/auth/collections", collectionHandler.GetUserCollectionsHandler)
}

// setupRequestRoutes configures community request endpoints.
// Public endpoints use optional auth (so authenticated users see their vote).
// CRUD, voting, fulfillment, and closing require authentication.
func setupRequestRoutes(rc RouteContext) {
	requestHandler := handlers.NewRequestHandler(rc.SC.Request, rc.SC.AuditLog)

	// Public request endpoints with optional auth (to include user's vote)
	optionalAuthGroup := huma.NewGroup(rc.API, "")
	optionalAuthGroup.UseMiddleware(middleware.OptionalHumaJWTMiddleware(rc.SC.JWT))
	huma.Get(optionalAuthGroup, "/requests", requestHandler.ListRequestsHandler)
	huma.Get(optionalAuthGroup, "/requests/{request_id}", requestHandler.GetRequestHandler)

	// Protected request endpoints
	huma.Post(rc.Protected, "/requests", requestHandler.CreateRequestHandler)
	huma.Put(rc.Protected, "/requests/{request_id}", requestHandler.UpdateRequestHandler)
	huma.Delete(rc.Protected, "/requests/{request_id}", requestHandler.DeleteRequestHandler)
	huma.Post(rc.Protected, "/requests/{request_id}/vote", requestHandler.VoteRequestHandler)
	huma.Delete(rc.Protected, "/requests/{request_id}/vote", requestHandler.RemoveVoteRequestHandler)
	huma.Post(rc.Protected, "/requests/{request_id}/fulfill", requestHandler.FulfillRequestHandler)
	huma.Post(rc.Protected, "/requests/{request_id}/close", requestHandler.CloseRequestHandler)
}

// setupRevisionRoutes configures revision history endpoints.
// Public endpoints for viewing history; admin endpoint for rollback.
func setupRevisionRoutes(rc RouteContext) {
	revisionHandler := handlers.NewRevisionHandler(rc.SC.Revision, rc.SC.AuditLog)

	// Public revision endpoints
	huma.Get(rc.API, "/revisions/{entity_type}/{entity_id}", revisionHandler.GetEntityHistoryHandler)
	huma.Get(rc.API, "/revisions/{revision_id}", revisionHandler.GetRevisionHandler)
	huma.Get(rc.API, "/users/{user_id}/revisions", revisionHandler.GetUserRevisionsHandler)

	// Admin rollback endpoint
	huma.Post(rc.Protected, "/admin/revisions/{revision_id}/rollback", revisionHandler.RollbackRevisionHandler)
}

// setupTagRoutes configures tag, entity tagging, and tag voting endpoints.
// Public endpoints for browsing tags. Optional auth for entity tags (user's vote).
// Protected endpoints for tagging and voting. Admin endpoints for tag CRUD and aliases.
func setupTagRoutes(rc RouteContext) {
	tagHandler := handlers.NewTagHandler(rc.SC.Tag, rc.SC.AuditLog)

	// Public tag endpoints
	huma.Get(rc.API, "/tags", tagHandler.ListTagsHandler)
	huma.Get(rc.API, "/tags/search", tagHandler.SearchTagsHandler)
	huma.Get(rc.API, "/tags/{tag_id}", tagHandler.GetTagHandler)
	huma.Get(rc.API, "/tags/{tag_id}/aliases", tagHandler.ListAliasesHandler)

	// Entity tags with optional auth (includes user's vote if authenticated)
	optionalAuthGroup := huma.NewGroup(rc.API, "")
	optionalAuthGroup.UseMiddleware(middleware.OptionalHumaJWTMiddleware(rc.SC.JWT))
	huma.Get(optionalAuthGroup, "/entities/{entity_type}/{entity_id}/tags", tagHandler.ListEntityTagsHandler)

	// Protected: tagging and voting
	huma.Post(rc.Protected, "/entities/{entity_type}/{entity_id}/tags", tagHandler.AddTagToEntityHandler)
	huma.Delete(rc.Protected, "/entities/{entity_type}/{entity_id}/tags/{tag_id}", tagHandler.RemoveTagFromEntityHandler)
	huma.Post(rc.Protected, "/tags/{tag_id}/entities/{entity_type}/{entity_id}/votes", tagHandler.VoteTagHandler)
	huma.Delete(rc.Protected, "/tags/{tag_id}/entities/{entity_type}/{entity_id}/votes", tagHandler.RemoveTagVoteHandler)

	// Admin: tag CRUD and alias management
	huma.Post(rc.Protected, "/tags", tagHandler.CreateTagHandler)
	huma.Put(rc.Protected, "/tags/{tag_id}", tagHandler.UpdateTagHandler)
	huma.Delete(rc.Protected, "/tags/{tag_id}", tagHandler.DeleteTagHandler)
	huma.Post(rc.Protected, "/tags/{tag_id}/aliases", tagHandler.CreateAliasHandler)
	huma.Delete(rc.Protected, "/tags/{tag_id}/aliases/{alias_id}", tagHandler.DeleteAliasHandler)
}

// setupArtistRelationshipRoutes configures artist relationship and similar artist endpoints.
func setupArtistRelationshipRoutes(rc RouteContext) {
	relHandler := handlers.NewArtistRelationshipHandler(rc.SC.ArtistRelationship, rc.SC.AuditLog)

	// Public: get related artists with optional auth (for user's votes)
	optionalAuthGroup := huma.NewGroup(rc.API, "")
	optionalAuthGroup.UseMiddleware(middleware.OptionalHumaJWTMiddleware(rc.SC.JWT))
	huma.Get(optionalAuthGroup, "/artists/{artist_id}/related", relHandler.GetRelatedArtistsHandler)
	huma.Get(optionalAuthGroup, "/artists/{artist_id}/graph", relHandler.GetArtistGraphHandler)

	// Protected: create relationships and vote
	huma.Post(rc.Protected, "/artists/relationships", relHandler.CreateRelationshipHandler)
	huma.Post(rc.Protected, "/artists/relationships/{source_id}/{target_id}/vote", relHandler.VoteHandler)
	huma.Delete(rc.Protected, "/artists/relationships/{source_id}/{target_id}/vote", relHandler.RemoveVoteHandler)

	// Admin: delete relationships
	huma.Delete(rc.Protected, "/artists/relationships/{source_id}/{target_id}", relHandler.DeleteRelationshipHandler)
}

// setupSceneRoutes configures scene (city aggregation) endpoints.
// All endpoints are public — no authentication required.
func setupSceneRoutes(rc RouteContext) {
	sceneHandler := handlers.NewSceneHandler(rc.SC.Scene)

	huma.Get(rc.API, "/scenes", sceneHandler.ListScenesHandler)
	huma.Get(rc.API, "/scenes/{slug}", sceneHandler.GetSceneDetailHandler)
	huma.Get(rc.API, "/scenes/{slug}/artists", sceneHandler.GetSceneActiveArtistsHandler)
	huma.Get(rc.API, "/scenes/{slug}/genres", sceneHandler.GetSceneGenresHandler)
}

// setupAttendanceRoutes configures show attendance (going/interested) endpoints.
// Public endpoints use optional auth (counts always available; user status if authenticated).
// Set/remove attendance requires authentication.
func setupAttendanceRoutes(rc RouteContext) {
	attendanceHandler := handlers.NewAttendanceHandler(rc.SC.Attendance)

	// Public endpoints with optional auth (counts + user status if authenticated)
	optionalAuthGroup := huma.NewGroup(rc.API, "")
	optionalAuthGroup.UseMiddleware(middleware.OptionalHumaJWTMiddleware(rc.SC.JWT))
	huma.Get(optionalAuthGroup, "/shows/{show_id}/attendance", attendanceHandler.GetAttendanceHandler)
	huma.Post(optionalAuthGroup, "/shows/attendance/batch", attendanceHandler.BatchAttendanceHandler)

	// Protected endpoints (require authentication)
	huma.Post(rc.Protected, "/shows/{show_id}/attendance", attendanceHandler.SetAttendanceHandler)
	huma.Delete(rc.Protected, "/shows/{show_id}/attendance", attendanceHandler.RemoveAttendanceHandler)
	huma.Get(rc.Protected, "/attendance/my-shows", attendanceHandler.GetMyShowsHandler)
}

// setupFollowRoutes configures follow/unfollow endpoints for entities.
// Follow/unfollow requires authentication. Follower counts use optional auth
// (counts always available; user follow status if authenticated).
func setupFollowRoutes(rc RouteContext) {
	followHandler := handlers.NewFollowHandler(rc.SC.Follow)

	// Optional auth group for public follower counts/lists
	optionalAuthGroup := huma.NewGroup(rc.API, "")
	optionalAuthGroup.UseMiddleware(middleware.OptionalHumaJWTMiddleware(rc.SC.JWT))

	// Follow/unfollow (protected): entity_type is a path param (artists, venues, labels, festivals)
	huma.Post(rc.Protected, "/{entity_type}/{entity_id}/follow", followHandler.FollowEntityHandler)
	huma.Delete(rc.Protected, "/{entity_type}/{entity_id}/follow", followHandler.UnfollowEntityHandler)

	// Public with optional auth: follower count + user follow status
	huma.Get(optionalAuthGroup, "/{entity_type}/{entity_id}/followers", followHandler.GetFollowersHandler)

	// Public with optional auth: follower list
	huma.Get(optionalAuthGroup, "/{entity_type}/{entity_id}/followers/list", followHandler.GetFollowersListHandler)

	// Batch follow counts (optional auth)
	huma.Post(optionalAuthGroup, "/follows/batch", followHandler.BatchFollowHandler)

	// User's following list (protected)
	huma.Get(rc.Protected, "/me/following", followHandler.GetMyFollowingHandler)
}

// setupNotificationFilterRoutes configures notification filter and notification log endpoints.
// CRUD and notifications require authentication. Unsubscribe is public (HMAC-signed).
func setupNotificationFilterRoutes(rc RouteContext) {
	filterHandler := handlers.NewNotificationFilterHandler(rc.SC.NotificationFilter, rc.Cfg.JWT.SecretKey)

	// Protected: filter CRUD
	huma.Get(rc.Protected, "/me/notification-filters", filterHandler.ListFiltersHandler)
	huma.Post(rc.Protected, "/me/notification-filters", filterHandler.CreateFilterHandler)
	huma.Patch(rc.Protected, "/me/notification-filters/{id}", filterHandler.UpdateFilterHandler)
	huma.Delete(rc.Protected, "/me/notification-filters/{id}", filterHandler.DeleteFilterHandler)
	huma.Post(rc.Protected, "/me/notification-filters/quick", filterHandler.QuickCreateFilterHandler)

	// Protected: notification log
	huma.Get(rc.Protected, "/me/notifications", filterHandler.GetNotificationsHandler)

	// Public: HMAC-signed unsubscribe
	huma.Post(rc.API, "/unsubscribe/filter/{id}", filterHandler.UnsubscribeFilterHandler)
}

// setupChartsRoutes configures public top charts endpoints.
// All endpoints are public — no authentication required.
func setupChartsRoutes(rc RouteContext) {
	chartsHandler := handlers.NewChartsHandler(rc.SC.Charts)

	huma.Get(rc.API, "/charts/trending-shows", chartsHandler.GetTrendingShowsHandler)
	huma.Get(rc.API, "/charts/popular-artists", chartsHandler.GetPopularArtistsHandler)
	huma.Get(rc.API, "/charts/active-venues", chartsHandler.GetActiveVenuesHandler)
	huma.Get(rc.API, "/charts/hot-releases", chartsHandler.GetHotReleasesHandler)
	huma.Get(rc.API, "/charts/overview", chartsHandler.GetChartsOverviewHandler)
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

// setupPendingEditRoutes configures pending entity edit endpoints.
// Protected endpoints for suggesting edits and managing own edits.
// Admin endpoints for reviewing, approving, and rejecting edits.
func setupPendingEditRoutes(rc RouteContext) {
	pendingEditHandler := handlers.NewPendingEditHandler(rc.SC.PendingEdit, rc.SC.AuditLog)

	// Protected: suggest edits (creates pending or auto-applies for trusted users)
	huma.Put(rc.Protected, "/artists/{entity_id}/suggest-edit", pendingEditHandler.SuggestArtistEditHandler)
	huma.Put(rc.Protected, "/venues/{entity_id}/suggest-edit", pendingEditHandler.SuggestVenueEditHandler)
	huma.Put(rc.Protected, "/festivals/{entity_id}/suggest-edit", pendingEditHandler.SuggestFestivalEditHandler)

	// Protected: user's own pending edits
	huma.Get(rc.Protected, "/my/pending-edits", pendingEditHandler.GetMyPendingEditsHandler)
	huma.Delete(rc.Protected, "/my/pending-edits/{edit_id}", pendingEditHandler.CancelMyPendingEditHandler)

	// Admin: review queue
	huma.Get(rc.Protected, "/admin/pending-edits", pendingEditHandler.AdminListPendingEditsHandler)
	huma.Get(rc.Protected, "/admin/pending-edits/{edit_id}", pendingEditHandler.AdminGetPendingEditHandler)
	huma.Post(rc.Protected, "/admin/pending-edits/{edit_id}/approve", pendingEditHandler.AdminApprovePendingEditHandler)
	huma.Post(rc.Protected, "/admin/pending-edits/{edit_id}/reject", pendingEditHandler.AdminRejectPendingEditHandler)
	huma.Get(rc.Protected, "/admin/pending-edits/entity/{entity_type}/{entity_id}", pendingEditHandler.AdminGetEntityPendingEditsHandler)
}

// setupEntityReportRoutes configures entity report endpoints.
// Protected endpoints for submitting reports.
// Admin endpoints for reviewing, resolving, and dismissing reports.
func setupEntityReportRoutes(rc RouteContext) {
	entityReportHandler := handlers.NewEntityReportHandler(rc.SC.EntityReport, rc.SC.AuditLog)

	// Rate-limited report submission: 5 requests per minute per IP
	rc.Router.Group(func(r chi.Router) {
		r.Use(httprate.Limit(
			middleware.ReportRequestsPerMinute,
			time.Minute,
			httprate.WithKeyFuncs(httprate.KeyByIP),
			httprate.WithLimitHandler(rateLimitHandler),
		))
		reportAPI := humachi.New(r, huma.DefaultConfig("Psychic Homily Entity Reports", "1.0.0"))
		reportAPI.UseMiddleware(middleware.HumaRequestIDMiddleware)
		reportAPI.UseMiddleware(middleware.HumaJWTMiddleware(rc.SC.JWT, rc.Cfg.Session))
		huma.Post(reportAPI, "/artists/{entity_id}/report", entityReportHandler.ReportArtistHandler)
		huma.Post(reportAPI, "/venues/{entity_id}/report", entityReportHandler.ReportVenueHandler)
		huma.Post(reportAPI, "/festivals/{entity_id}/report", entityReportHandler.ReportFestivalHandler)
		// Note: shows already have /shows/{show_id}/report in setupShowReportRoutes.
		// The generic entity report handler + service support shows, so the admin queue
		// can display show reports submitted through the existing endpoint or this one.
		huma.Post(reportAPI, "/shows/{entity_id}/entity-report", entityReportHandler.ReportShowHandler)
	})

	// Admin: entity report management
	huma.Get(rc.Protected, "/admin/entity-reports", entityReportHandler.AdminListEntityReportsHandler)
	huma.Get(rc.Protected, "/admin/entity-reports/{report_id}", entityReportHandler.AdminGetEntityReportHandler)
	huma.Post(rc.Protected, "/admin/entity-reports/{report_id}/resolve", entityReportHandler.AdminResolveEntityReportHandler)
	huma.Post(rc.Protected, "/admin/entity-reports/{report_id}/dismiss", entityReportHandler.AdminDismissEntityReportHandler)
}

// setupContributeRoutes configures public contribution opportunity endpoints.
func setupContributeRoutes(rc RouteContext) {
	contributeHandler := handlers.NewContributeHandler(rc.SC.DataQuality)
	huma.Get(rc.API, "/contribute/opportunities", contributeHandler.GetOpportunitiesHandler)
	huma.Get(rc.API, "/contribute/opportunities/{category}", contributeHandler.GetOpportunityCategoryHandler)
}
