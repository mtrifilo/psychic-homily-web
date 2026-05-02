package routes

import (
	"net/http"
	"os"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/httprate"

	authh "psychic-homily-backend/internal/api/handlers/auth"
	"psychic-homily-backend/internal/api/middleware"
)

// setupAuthRoutes configures all authentication-related endpoints
func setupAuthRoutes(rc RouteContext) {
	authHandler := authh.NewAuthHandler(rc.SC.Auth, rc.SC.JWT, rc.SC.User, rc.SC.Email, rc.SC.Discord, rc.SC.PasswordValidator, rc.Cfg)
	oauthHTTPHandler := authh.NewOAuthHTTPHandler(rc.SC.Auth, rc.Cfg)

	// Create rate limiter for auth endpoints: 10 requests per minute per IP
	// This helps prevent:
	// - Brute force attacks on login
	// - Credential stuffing
	// - Email bombing via magic links
	// - Spam account creation
	//
	// PSY-475: replaced with a no-op when DISABLE_AUTH_RATE_LIMITS=1 in a
	// whitelisted ENVIRONMENT. All E2E workers share 127.0.0.1, so the
	// 10/min budget got exhausted and broke register/magic-link tests on
	// shard 3. Default-deny env check in cmd/server/main.go refuses to
	// boot with the flag set anywhere other than test/ci/development.
	var authRateLimiter func(http.Handler) http.Handler
	if IsAuthRateLimitDisabled(os.Getenv) {
		authRateLimiter = noopRateLimiter()
	} else {
		authRateLimiter = httprate.Limit(
			10,            // requests
			1*time.Minute, // per duration
			httprate.WithKeyFuncs(httprate.KeyByIP),
			httprate.WithLimitHandler(rateLimitHandler),
		)
	}

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
		appleAuthHandler := authh.NewAppleAuthHandler(rc.SC.AppleAuth, rc.SC.Discord, rc.Cfg)
		huma.Post(rateLimitedAPI, "/auth/apple/callback", appleAuthHandler.AppleCallbackHandler)

		// Account recovery endpoints (public, rate-limited)
		huma.Post(rateLimitedAPI, "/auth/recover-account", authHandler.RecoverAccountHandler)
		huma.Post(rateLimitedAPI, "/auth/recover-account/request", authHandler.RequestAccountRecoveryHandler)
		huma.Post(rateLimitedAPI, "/auth/recover-account/confirm", authHandler.ConfirmAccountRecoveryHandler)
	})

	// Logout doesn't need strict rate limiting (already requires valid session)
	huma.Post(rc.API, "/auth/logout", authHandler.LogoutHandler)
}

// setupProtectedAuthRoutes configures the auth-related Huma routes that run on
// the protected group (or the public API for HMAC-signed unsubscribe endpoints
// and the email-verify confirm endpoint). Split out of SetupRoutes during the
// PSY-422 routes.go decomposition; behavior unchanged.
func setupProtectedAuthRoutes(rc RouteContext) {
	authHandler := authh.NewAuthHandler(rc.SC.Auth, rc.SC.JWT, rc.SC.User, rc.SC.Email, rc.SC.Discord, rc.SC.PasswordValidator, rc.Cfg)

	huma.Get(rc.Protected, "/auth/profile", authHandler.GetProfileHandler)
	huma.Patch(rc.Protected, "/auth/profile", authHandler.UpdateProfileHandler)
	huma.Post(rc.Protected, "/auth/verify-email/send", authHandler.SendVerificationEmailHandler)
	huma.Post(rc.Protected, "/auth/change-password", authHandler.ChangePasswordHandler)

	// Token refresh uses lenient middleware (accepts tokens expired within 7 days)
	lenientGroup := huma.NewGroup(rc.API, "")
	lenientGroup.UseMiddleware(middleware.LenientHumaJWTMiddleware(rc.SC.JWT, 7*24*time.Hour))
	huma.Post(lenientGroup, "/auth/refresh", authHandler.RefreshTokenHandler)

	// Account deletion endpoints
	huma.Get(rc.Protected, "/auth/account/deletion-summary", authHandler.GetDeletionSummaryHandler)
	huma.Post(rc.Protected, "/auth/account/delete", authHandler.DeleteAccountHandler)

	// Data export endpoint (GDPR Right to Portability)
	huma.Get(rc.Protected, "/auth/account/export", authHandler.ExportDataHandler)

	// CLI token generation endpoint (admin only)
	huma.Post(rc.Protected, "/auth/cli-token", authHandler.GenerateCLITokenHandler)

	// OAuth account management endpoints
	oauthAccountHandler := authh.NewOAuthAccountHandler(rc.SC.User)
	huma.Get(rc.Protected, "/auth/oauth/accounts", oauthAccountHandler.GetOAuthAccountsHandler)
	huma.Delete(rc.Protected, "/auth/oauth/accounts/{provider}", oauthAccountHandler.UnlinkOAuthAccountHandler)

	// User preferences endpoints
	userPrefsHandler := authh.NewUserPreferencesHandler(rc.SC.User, rc.Cfg.JWT.SecretKey)
	huma.Put(rc.Protected, "/auth/preferences/favorite-cities", userPrefsHandler.SetFavoriteCitiesHandler)
	huma.Patch(rc.Protected, "/auth/preferences/show-reminders", userPrefsHandler.SetShowRemindersHandler)
	// PSY-296: default reply permission applied to new top-level comments.
	huma.Patch(rc.Protected, "/auth/preferences/default-reply-permission", userPrefsHandler.SetDefaultReplyPermissionHandler)
	// PSY-289: comment + mention notification preferences.
	huma.Patch(rc.Protected, "/auth/preferences/comment-notifications", userPrefsHandler.SetCommentNotificationsHandler)
	// PSY-350: collection digest preference toggle (weekly cadence; opt-IN).
	huma.Patch(rc.Protected, "/auth/preferences/collection-digest", userPrefsHandler.SetCollectionDigestHandler)

	// Public unsubscribe endpoint (HMAC-signed, no auth required)
	huma.Post(rc.API, "/auth/unsubscribe/show-reminders", userPrefsHandler.UnsubscribeShowRemindersHandler)
	// PSY-289: public one-click unsubscribe for comment + mention emails.
	huma.Post(rc.API, "/unsubscribe/comment-subscription", userPrefsHandler.UnsubscribeCommentSubscriptionHandler)
	huma.Post(rc.API, "/unsubscribe/mention", userPrefsHandler.UnsubscribeMentionHandler)
	// PSY-350: public unsubscribe for collection digest emails. Registered as
	// chi routes (NOT Huma) so the same path serves both a manual GET (HTML
	// confirmation page) and an RFC 8058 / RFC 2369 one-click POST. Mailbox
	// providers (Gmail, Yahoo) send the POST when a user clicks the native
	// "Unsubscribe" button next to the sender name; the GET is the link in
	// the email body. Both verify the same HMAC signature.
	rc.Router.Get("/unsubscribe/collection-digest", userPrefsHandler.UnsubscribeCollectionDigestPageHandler)
	rc.Router.Post("/unsubscribe/collection-digest", userPrefsHandler.UnsubscribeCollectionDigestPageHandler)

	// Public email verification confirm endpoint (user clicks link from email)
	huma.Post(rc.API, "/auth/verify-email/confirm", authHandler.ConfirmVerificationHandler)

	// Account recovery endpoints (public - user is not authenticated)
	// These are registered in setupAuthRoutes with rate limiting
}

// setupPasskeyRoutes configures WebAuthn/passkey endpoints
func setupPasskeyRoutes(rc RouteContext) {
	if rc.SC.WebAuthn == nil {
		// WebAuthn service failed to initialize - passkeys are optional
		return
	}

	passkeyHandler := authh.NewPasskeyHandler(rc.SC.WebAuthn, rc.SC.JWT, rc.SC.User, rc.Cfg)

	// Create rate limiter for passkey endpoints: 20 requests per minute per IP
	// Slightly more lenient than auth due to multi-step WebAuthn flow.
	// PSY-475: same env-flagged no-op gate as the auth limiter.
	var passkeyRateLimiter func(http.Handler) http.Handler
	if IsAuthRateLimitDisabled(os.Getenv) {
		passkeyRateLimiter = noopRateLimiter()
	} else {
		passkeyRateLimiter = httprate.Limit(
			20,            // requests
			1*time.Minute, // per duration
			httprate.WithKeyFuncs(httprate.KeyByIP),
			httprate.WithLimitHandler(rateLimitHandler),
		)
	}

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
