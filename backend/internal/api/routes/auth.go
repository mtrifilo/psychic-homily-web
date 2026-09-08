package routes

import (
	"net/http"
	"os"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/httprate"

	authh "psychic-homily-backend/internal/api/handlers/auth"
	"psychic-homily-backend/internal/api/middleware"
)

// setupAuthRoutes configures all authentication-related endpoints
func setupAuthRoutes(rc RouteContext) {
	authHandler := authh.NewAuthHandler(rc.SC.Auth, rc.SC.JWT, rc.SC.User, rc.SC.Email, rc.SC.Discord, rc.SC.PasswordValidator, rc.Cfg)
	oauthHTTPHandler := authh.NewOAuthHTTPHandler(rc.SC.Auth, rc.SC.JWT, rc.Cfg)

	// The public auth budget: brute force on login, credential stuffing,
	// email bombing via magic links, and spam account creation all draw on this
	// one per-IP counter. Built once and shared by the chi OAuth groups below
	// and by rateLimitedAuthGroup, which is what keeps them on one budget.
	authRateLimiter := authScopedRateLimiter(middleware.AuthRequestsPerMinute)

	// Rate-limited OAuth routes
	rc.Router.Group(func(r chi.Router) {
		r.Use(authRateLimiter)
		r.Get("/auth/login/{provider}", oauthHTTPHandler.OAuthLoginHTTPHandler)
		r.Get("/auth/callback/{provider}", oauthHTTPHandler.OAuthCallbackHTTPHandler)
	})

	// Connecting a provider to an account you are already signed in to. A raw
	// chi route rather than a Huma operation for the same reason login and
	// callback are: it answers with a redirect into the provider's consent
	// screen, not a JSON body.
	//
	// JWTMiddleware is what makes this the authenticated path. The account the
	// identity attaches to comes from the session, never from the address the
	// provider returns, which is the whole difference from /auth/login.
	rc.Router.Group(func(r chi.Router) {
		r.Use(authRateLimiter)
		r.Use(middleware.JWTMiddleware(rc.SC.JWT))
		r.Get("/auth/link/{provider}", oauthHTTPHandler.OAuthLinkHTTPHandler)
	})

	// Rate-limited auth API endpoints.
	//
	// PSY-1598: on the MAIN api via a Huma group rather than its own humachi.New —
	// a separate instance owns a separate OpenAPI document, which is why the entire
	// authentication surface was missing from the published spec. An external
	// consumer reading it saw no way to authenticate at all.
	//
	// Two properties this conversion must preserve, both easy to break silently:
	//
	//  1. authRateLimiter is the SAME closure the OAuth chi group above uses, so
	//     those raw routes and these operations share ONE counter. Passing the
	//     variable (rather than building a second limiter here) is what keeps that
	//     budget shared; a fresh httprate.Limit would quietly double it.
	//  2. It may be noopRateLimiter() under DISABLE_AUTH_RATE_LIMITS (PSY-475).
	//     humaFromHTTP has to let that through cleanly, or every E2E shard — all
	//     sharing 127.0.0.1 — starts failing register/magic-link again.
	//
	// No JWT middleware here on purpose: these endpoints are how you GET a token.
	rateLimitedAuthGroup := huma.NewGroup(rc.API, "")
	rateLimitedAuthGroup.UseMiddleware(humaFromHTTP(authRateLimiter))
	rateLimitedAuthGroup.UseMiddleware(middleware.HumaRequestIDMiddleware)

	huma.Post(rateLimitedAuthGroup, "/auth/login", authHandler.LoginHandler)
	huma.Post(rateLimitedAuthGroup, "/auth/register", authHandler.RegisterHandler)
	huma.Post(rateLimitedAuthGroup, "/auth/magic-link/send", authHandler.SendMagicLinkHandler)
	huma.Post(rateLimitedAuthGroup, "/auth/magic-link/verify", authHandler.VerifyMagicLinkHandler)

	// Sign in with Apple (public, rate-limited)
	appleAuthHandler := authh.NewAppleAuthHandler(rc.SC.AppleAuth, rc.SC.Discord, rc.Cfg)
	huma.Post(rateLimitedAuthGroup, "/auth/apple/callback", appleAuthHandler.AppleCallbackHandler)

	// Account recovery endpoints (public, rate-limited)
	huma.Post(rateLimitedAuthGroup, "/auth/recover-account", authHandler.RecoverAccountHandler)
	huma.Post(rateLimitedAuthGroup, "/auth/recover-account/request", authHandler.RequestAccountRecoveryHandler)
	huma.Post(rateLimitedAuthGroup, "/auth/recover-account/confirm", authHandler.ConfirmAccountRecoveryHandler)

	// Logout doesn't need strict rate limiting (already requires valid session)
	huma.Post(rc.API, "/auth/logout", authHandler.LogoutHandler)
}

// VerificationResendPerMinute is the per-IP budget for POST
// /auth/verify-email/send. A real user clicks resend once, maybe twice after a
// typo or a slow inbox; five leaves room for a shared NAT without leaving the
// endpoint open as an email-bombing amplifier.
const VerificationResendPerMinute = 5

// ChangePasswordAttemptsPerMinute is the per-IP budget for POST
// /auth/change-password, which verifies the current password before setting the
// new one. It matches the resend budget on its own counter, so tuning one does
// not tune the other.
//
// The whole budget is not guesses: a new password the server rejects on policy
// (breach-list and common-password checks the browser form cannot run) spends
// an attempt too, so a user iterating on a password costs the same as an
// attacker iterating on the current one. Five per minute per IP is a floor
// against unsophisticated abuse, not a bound on a determined caller, who can
// rotate source addresses for a fresh counter each time.
const ChangePasswordAttemptsPerMinute = 5

// authScopedRateLimiter builds a per-IP minute limiter for an auth route,
// honoring the DISABLE_AUTH_RATE_LIMITS escape hatch: every E2E worker shares
// 127.0.0.1, so a live limiter would 429 unrelated shards.
//
// Each CALL returns a limiter with its own counter, so routes share a budget
// only when they are handed the same value. The counters are per process, so a
// deployment of N replicas serves N times the budget per IP.
func authScopedRateLimiter(requestsPerMinute int) func(http.Handler) http.Handler {
	if IsAuthRateLimitDisabled(os.Getenv) {
		return noopRateLimiter()
	}
	return httprate.Limit(
		requestsPerMinute,
		1*time.Minute,
		httprate.WithKeyFuncs(middleware.KeyByClientIP),
		httprate.WithLimitHandler(rateLimitHandler),
	)
}

// setupProtectedAuthRoutes configures the auth-related Huma routes that run on
// the protected group (or the public API for HMAC-signed unsubscribe endpoints
// and the email-verify confirm endpoint). Split out of SetupRoutes during the
// PSY-422 routes.go decomposition; behavior unchanged.
func setupProtectedAuthRoutes(rc RouteContext) {
	authHandler := authh.NewAuthHandler(rc.SC.Auth, rc.SC.JWT, rc.SC.User, rc.SC.Email, rc.SC.Discord, rc.SC.PasswordValidator, rc.Cfg)

	huma.Get(rc.Protected, "/auth/profile", authHandler.GetProfileHandler)
	huma.Patch(rc.Protected, "/auth/profile", authHandler.UpdateProfileHandler)

	// PSY-1087: self-scoped next-tier progress (approved-edits bar + met/unmet list).
	advancementHandler := authh.NewAdvancementHandler(rc.SC.AutoPromotion)
	huma.Get(rc.Protected, "/auth/profile/advancement", advancementHandler.GetAdvancementHandler)
	// Verification resend is rate-limited (PSY-1871). Signup now emails the
	// link automatically and the /shows/submit gate puts a resend button in
	// front of every blocked user, so this endpoint went from obscure to
	// one-click. Unthrottled, it is an inbox-bombing and Resend-quota vector
	// for any authenticated caller.
	//
	// Budget is deliberately separate from the public auth budget: this
	// endpoint requires a session, and sharing a counter with /auth/login would
	// let resend clicks lock a user out of logging in. Same per-IP key and same
	// DISABLE_AUTH_RATE_LIMITS escape hatch as the public auth group, so E2E
	// shards sharing 127.0.0.1 are unaffected.
	verifyEmailGroup := huma.NewGroup(rc.Protected, "")
	verifyEmailGroup.UseMiddleware(humaFromHTTP(authScopedRateLimiter(VerificationResendPerMinute)))
	huma.Post(verifyEmailGroup, "/auth/verify-email/send", authHandler.SendVerificationEmailHandler)

	// Change-password meters current-password attempts per client IP. Its
	// counter is separate from the public auth budget, so attempts here cannot
	// lock the same person out of /auth/login. The group hangs off
	// rc.Protected, so HumaJWTMiddleware runs first and an unauthenticated
	// caller is refused before it reaches the counter.
	changePasswordGroup := huma.NewGroup(rc.Protected, "")
	changePasswordGroup.UseMiddleware(humaFromHTTP(authScopedRateLimiter(ChangePasswordAttemptsPerMinute)))
	huma.Post(changePasswordGroup, "/auth/change-password", authHandler.ChangePasswordHandler)

	// Token refresh uses lenient middleware (accepts tokens expired within 7 days)
	lenientGroup := huma.NewGroup(rc.API, "")
	lenientGroup.UseMiddleware(middleware.LenientHumaJWTMiddleware(rc.SC.JWT, 7*24*time.Hour))
	huma.Post(lenientGroup, "/auth/refresh", authHandler.RefreshTokenHandler)

	// Account deletion endpoints
	huma.Get(rc.Protected, "/auth/account/deletion-summary", authHandler.GetDeletionSummaryHandler)
	huma.Post(rc.Protected, "/auth/account/delete", authHandler.DeleteAccountHandler)

	// Data export endpoint (GDPR Right to Portability)
	huma.Get(rc.Protected, "/auth/account/export", authHandler.ExportDataHandler)

	// CLI token generation endpoint (admin only — gated by HumaAdminMiddleware
	// on the rc.Admin group; PSY-550 follow-up to PSY-423).
	huma.Post(rc.Admin, "/auth/cli-token", authHandler.GenerateCLITokenHandler)

	// OAuth account management endpoints
	oauthAccountHandler := authh.NewOAuthAccountHandler(rc.SC.User, rc.Cfg.JWT.SecretKey, rc.Cfg.Email.FrontendURL)
	huma.Get(rc.Protected, "/auth/oauth/accounts", oauthAccountHandler.GetOAuthAccountsHandler)
	huma.Delete(rc.Protected, "/auth/oauth/accounts/{provider}", oauthAccountHandler.UnlinkOAuthAccountHandler)
	// Mints the one-time token /auth/link/{provider} requires. Same-origin and
	// authenticated, which is what that route cannot verify for itself.
	//
	// Rate limited per IP at the verification-resend size, on a counter of its
	// own: it is an unauthenticated-shaped primitive behind a session, and
	// nothing else bounds how fast a client can ask for signed tokens.
	linkTokenGroup := huma.NewGroup(rc.Protected, "")
	linkTokenGroup.UseMiddleware(humaFromHTTP(authScopedRateLimiter(VerificationResendPerMinute)))
	huma.Post(linkTokenGroup, "/auth/oauth/link-token", oauthAccountHandler.StartOAuthLinkHandler)

	// User preferences endpoints
	userPrefsHandler := authh.NewUserPreferencesHandler(rc.SC.User, rc.Cfg.JWT.SecretKey)
	huma.Put(rc.Protected, "/auth/preferences/favorite-cities", userPrefsHandler.SetFavoriteCitiesHandler)
	// PSY-1423: /charts window + scene landing defaults.
	huma.Put(rc.Protected, "/auth/preferences/chart-defaults", userPrefsHandler.SetChartDefaultsHandler)
	huma.Patch(rc.Protected, "/auth/preferences/show-reminders", userPrefsHandler.SetShowRemindersHandler)
	// PSY-296: default reply permission applied to new top-level comments.
	huma.Patch(rc.Protected, "/auth/preferences/default-reply-permission", userPrefsHandler.SetDefaultReplyPermissionHandler)
	// PSY-289: comment + mention notification preferences.
	huma.Patch(rc.Protected, "/auth/preferences/comment-notifications", userPrefsHandler.SetCommentNotificationsHandler)
	// PSY-350: collection digest preference toggle (weekly cadence; opt-IN).
	huma.Patch(rc.Protected, "/auth/preferences/collection-digest", userPrefsHandler.SetCollectionDigestHandler)
	// PSY-1342: weekly scene digest preference toggle (opt-IN).
	huma.Patch(rc.Protected, "/auth/preferences/scene-digest", userPrefsHandler.SetSceneDigestHandler)
	// Tier-change + edit-review notification toggles (opt-OUT).
	huma.Patch(rc.Protected, "/auth/preferences/tier-edit-notifications", userPrefsHandler.SetTierEditNotificationsHandler)
	// PSY-1907: home area + account-level alert defaults. One read serves the
	// whole alerts settings surface; each preference keeps its own write.
	huma.Get(rc.Protected, "/auth/preferences/alerts", userPrefsHandler.GetAlertPreferencesHandler)
	huma.Put(rc.Protected, "/auth/preferences/home-metro", userPrefsHandler.SetHomeMetroHandler)
	huma.Patch(rc.Protected, "/auth/preferences/alert-defaults", userPrefsHandler.SetAlertDefaultsHandler)

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
	// Tier-change + edit-review unsubscribe (same chi GET+POST shape).
	rc.Router.Get("/unsubscribe/tier-notifications", userPrefsHandler.UnsubscribeTierNotificationsPageHandler)
	rc.Router.Post("/unsubscribe/tier-notifications", userPrefsHandler.UnsubscribeTierNotificationsPageHandler)
	rc.Router.Get("/unsubscribe/edit-notifications", userPrefsHandler.UnsubscribeEditNotificationsPageHandler)
	rc.Router.Post("/unsubscribe/edit-notifications", userPrefsHandler.UnsubscribeEditNotificationsPageHandler)
	// PSY-1342: weekly scene digest unsubscribe (same chi GET+POST shape).
	rc.Router.Get("/unsubscribe/scene-digest", userPrefsHandler.UnsubscribeSceneDigestPageHandler)
	rc.Router.Post("/unsubscribe/scene-digest", userPrefsHandler.UnsubscribeSceneDigestPageHandler)
	// PSY-1896: artist new-show alert emails (same chi GET+POST shape). The
	// path segment IS the signed scope, so it must stay in step with
	// engagement.UnsubscribeScopeArtistShowAlerts or every link already in an
	// inbox 404s.
	rc.Router.Get("/unsubscribe/artist-show-alerts", userPrefsHandler.UnsubscribeArtistShowAlertsPageHandler)
	rc.Router.Post("/unsubscribe/artist-show-alerts", userPrefsHandler.UnsubscribeArtistShowAlertsPageHandler)

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

	passkeyHandler := authh.NewPasskeyHandler(rc.SC.WebAuthn, rc.SC.JWT, rc.SC.User, rc.SC.Email, rc.Cfg)

	// Passkey gets its own budget, more lenient than the auth one because a
	// WebAuthn flow is several requests. Built once and shared across the four
	// public passkey operations.
	passkeyRateLimiter := authScopedRateLimiter(middleware.PasskeyRequestsPerMinute)

	// Rate-limited public passkey endpoints.
	//
	// PSY-1598: same move as the auth group above, same two properties to preserve —
	// passkeyRateLimiter is passed as the existing closure so its counter is shared
	// across all four operations exactly as the chi group shared it, and it may be a
	// no-op under DISABLE_AUTH_RATE_LIMITS.
	passkeyGroup := huma.NewGroup(rc.API, "")
	passkeyGroup.UseMiddleware(humaFromHTTP(passkeyRateLimiter))
	passkeyGroup.UseMiddleware(middleware.HumaRequestIDMiddleware)

	// Public passkey login endpoints (no auth required)
	huma.Post(passkeyGroup, "/auth/passkey/login/begin", passkeyHandler.BeginLoginHandler)
	huma.Post(passkeyGroup, "/auth/passkey/login/finish", passkeyHandler.FinishLoginHandler)

	// Public passkey signup endpoints (passkey-first registration, no auth required)
	huma.Post(passkeyGroup, "/auth/passkey/signup/begin", passkeyHandler.BeginSignupHandler)
	huma.Post(passkeyGroup, "/auth/passkey/signup/finish", passkeyHandler.FinishSignupHandler)

	// Protected passkey registration endpoints (user must be logged in)
	huma.Post(rc.Protected, "/auth/passkey/register/begin", passkeyHandler.BeginRegisterHandler)
	huma.Post(rc.Protected, "/auth/passkey/register/finish", passkeyHandler.FinishRegisterHandler)

	// Protected passkey management endpoints
	huma.Get(rc.Protected, "/auth/passkey/credentials", passkeyHandler.ListCredentialsHandler)
	huma.Delete(rc.Protected, "/auth/passkey/credentials/{credential_id}", passkeyHandler.DeleteCredentialHandler)
}
