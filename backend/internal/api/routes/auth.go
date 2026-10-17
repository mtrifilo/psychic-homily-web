package routes

import (
	"net/http"
	"os"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/go-chi/chi/v5"

	authh "psychic-homily-backend/internal/api/handlers/auth"
	"psychic-homily-backend/internal/api/middleware"
	"psychic-homily-backend/internal/services/auth"
)

// oauthRedirectRoutes are the routes on oauthRedirectRateLimiter: the OAuth
// sign-in start, its callback, and the Settings link start. A browser reaches
// each by top-level navigation, so a refusal there is a redirect, and the
// routes are registered and exempted from the public-read limiter by these
// same templates. Each ends in its one {provider} parameter, which
// isOAuthRedirectRequest relies on, and each needs its refusal declared in
// TestEveryOAuthRedirectRouteRefusesToADeclaredPage.
var oauthRedirectRoutes = []string{authh.OAuthLoginRoute, authh.OAuthCallbackRoute, authh.OAuthLinkRoute}

// setupAuthRoutes configures all authentication-related endpoints
func setupAuthRoutes(rc RouteContext) {
	authHandler := authh.NewAuthHandler(rc.SC.Auth, rc.SC.JWT, rc.SC.User, rc.SC.Email, rc.SC.Discord, rc.SC.PasswordValidator, rc.Cfg)
	oauthHTTPHandler := authh.NewOAuthHTTPHandler(rc.SC.Auth, rc.SC.JWT, rc.Cfg)

	// The OAuth redirects meter on oauthRedirectRateLimiter alone: never on the
	// public auth budget, and never on the public-read limiter, which exempts
	// oauthRedirectRoutes. One limiter on the group is one counter for all
	// three routes.
	rc.Router.Group(func(r chi.Router) {
		r.Use(oauthRedirectRateLimiter(oauthHTTPHandler.RedirectRateLimited))
		r.Get(authh.OAuthLoginRoute, oauthHTTPHandler.OAuthLoginHTTPHandler)
		r.Get(authh.OAuthCallbackRoute, oauthHTTPHandler.OAuthCallbackHTTPHandler)

		// Connecting a provider to an account you are already signed in to.
		// A raw chi route rather than a Huma operation for the same reason
		// login and callback are: it answers with a redirect into the
		// provider's consent screen, not a JSON body.
		//
		// JWTMiddleware is what makes this the authenticated path. The account
		// the identity attaches to comes from the session, never from the
		// address the provider returns, which is the whole difference from
		// the sign-in start. It runs after the limiter, so a request with no
		// session is metered before it is refused.
		r.With(middleware.JWTMiddleware(rc.SC.JWT)).Get(authh.OAuthLinkRoute, oauthHTTPHandler.OAuthLinkHTTPHandler)
	})

	// Rate-limited auth API endpoints.
	//
	// PSY-1598: on the MAIN api via a Huma group rather than its own humachi.New —
	// a separate instance owns a separate OpenAPI document, which is why the entire
	// authentication surface was missing from the published spec. An external
	// consumer reading it saw no way to authenticate at all.
	//
	// The public auth budget: brute force on login, credential stuffing, email
	// bombing via magic links, and spam account creation all draw on this one
	// per-IP counter. It may be noopRateLimiter() under DISABLE_AUTH_RATE_LIMITS;
	// humaFromHTTP has to let that through cleanly, or every E2E shard, all
	// sharing 127.0.0.1, fails register and magic-link.
	//
	// No JWT middleware here on purpose: these endpoints are how you GET a token.
	rateLimitedAuthGroup := huma.NewGroup(rc.API, "")
	useAuthBudget(rateLimitedAuthGroup, authScopedRateLimiter(middleware.LimiterAuth, middleware.AuthRequestsPerMinute))

	huma.Post(rateLimitedAuthGroup, "/auth/login", authHandler.LoginHandler)
	huma.Post(rateLimitedAuthGroup, "/auth/register", authHandler.RegisterHandler)
	huma.Post(rateLimitedAuthGroup, "/auth/magic-link/send", authHandler.SendMagicLinkHandler)
	huma.Post(rateLimitedAuthGroup, "/auth/magic-link/verify", authHandler.VerifyMagicLinkHandler)

	// Sign in with Apple. The caller is a native client posting the identity
	// token as JSON, not a browser, so a refusal stays the JSON 429 with its
	// Retry-After. It meters on a per-IP budget of its own: password attempts
	// from the same address cannot spend it, and it cannot spend theirs.
	appleAuthHandler := authh.NewAppleAuthHandler(rc.SC.AppleAuth, rc.SC.Discord, rc.Cfg)
	appleSignInGroup := huma.NewGroup(rc.API, "")
	useAuthBudget(appleSignInGroup, authScopedRateLimiter(middleware.LimiterAppleSignIn, middleware.AppleSignInRequestsPerMinute))
	huma.Post(appleSignInGroup, "/auth/apple/callback", appleAuthHandler.AppleCallbackHandler)

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

// PasswordConfirmAttemptsPerMinute is the per-IP budget shared by the routes on
// passwordConfirmGroup, each of which takes the account password as
// confirmation. It matches the resend budget on its own counter, so tuning one
// does not tune the other.
//
// The counter meters requests, not guesses. A new password the server rejects
// on policy spends one on change-password, and an account with no password
// spends one on the delete route before anything is compared. Sharing means one
// of those surfaces can refuse the other, per source address, for a window.
const PasswordConfirmAttemptsPerMinute = 5

// authScopedRateLimiter builds a per-IP minute limiter for an auth route,
// honoring the DISABLE_AUTH_RATE_LIMITS escape hatch: every E2E worker shares
// 127.0.0.1, so a live limiter would 429 unrelated shards.
//
// Each CALL returns a limiter with its own counter, so routes share a budget
// only when they are handed the same value. The counters are per process, so a
// deployment of N replicas serves N times the budget per IP.
func authScopedRateLimiter(name middleware.LimiterName, requestsPerMinute int) func(http.Handler) http.Handler {
	if IsAuthRateLimitDisabled(os.Getenv) {
		return noopRateLimiter()
	}
	return ipRateLimiter(name, requestsPerMinute, time.Minute)
}

// authScopedUserRateLimiter meters a route that already sits behind
// HumaJWTMiddleware with middleware.RateLimitBySessionUser: userLimiter keyed by
// the session user, inside it ipCeilingLimiter keyed by the client address. It
// honors the same DISABLE_AUTH_RATE_LIMITS escape hatch as authScopedRateLimiter,
// which turns both off. Each limiter keeps its own counter, so routes share a
// budget only when they are handed the same limiters.
func authScopedUserRateLimiter(jwtService *auth.JWTService, userLimiter, ipCeilingLimiter func(http.Handler) http.Handler) func(http.Handler) http.Handler {
	if IsAuthRateLimitDisabled(os.Getenv) {
		return noopRateLimiter()
	}
	return middleware.RateLimitBySessionUser(jwtService, userLimiter, ipCeilingLimiter)
}

// useAuthBudget puts limiter in front of every operation registered on group
// after this call, ahead of any middleware added to group later. group is a
// child group built for that one budget: on a shared parent such as rc.API or
// rc.Protected, the limiter would meter every route registered there
// afterwards.
func useAuthBudget(group huma.API, limiter func(http.Handler) http.Handler) {
	group.UseMiddleware(humaFromHTTP(limiter))
}

// oauthRedirectRateLimiter builds the per-IP minute limiter for
// oauthRedirectRoutes, at middleware.OAuthRedirectRequestsPerMinute and honoring
// the same DISABLE_AUTH_RATE_LIMITS escape hatch as authScopedRateLimiter.
//
// A refusal logs as middleware.LimiterOAuthRedirect and is answered by
// redirect, handed the message the JSON 429 carries, so the reader lands on a
// page that says why rather than on a response body.
func oauthRedirectRateLimiter(redirect func(http.ResponseWriter, *http.Request, string)) func(http.Handler) http.Handler {
	if IsAuthRateLimitDisabled(os.Getenv) {
		return noopRateLimiter()
	}
	return ipRateLimiterRefusingWith(middleware.LimiterOAuthRedirect, middleware.OAuthRedirectRequestsPerMinute, time.Minute, func(w http.ResponseWriter, r *http.Request) {
		// httprate sets Retry-After before it calls a refusal handler. On a
		// redirect that header asks the browser to wait before following it,
		// and the page it leads to is not what is limited.
		w.Header().Del("Retry-After")
		redirect(w, r, rateLimitExceededMessage)
	})
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
	useAuthBudget(verifyEmailGroup, authScopedRateLimiter(middleware.LimiterVerificationResend, VerificationResendPerMinute))
	huma.Post(verifyEmailGroup, "/auth/verify-email/send", authHandler.SendVerificationEmailHandler)

	// Every route on this group takes the account password as confirmation, and
	// one group means one counter: a wrong guess at any of them spends from the
	// same per-IP budget. That counter is separate from the public auth budget,
	// so attempts here cannot lock the same person out of /auth/login. The group
	// hangs off rc.Protected, so HumaJWTMiddleware runs first and an
	// unauthenticated caller is refused before reaching the counter.
	//
	// A route registered on rc.Protected instead of here carries no budget of
	// its own. The route inventory guard requires every mutating route under
	// /auth/ to record which budget meters it.
	passwordConfirmGroup := huma.NewGroup(rc.Protected, "")
	useAuthBudget(passwordConfirmGroup, authScopedRateLimiter(middleware.LimiterPasswordConfirm, PasswordConfirmAttemptsPerMinute))
	huma.Post(passwordConfirmGroup, "/auth/change-password", authHandler.ChangePasswordHandler)

	// Token refresh uses lenient middleware (accepts tokens expired within 7 days)
	lenientGroup := huma.NewGroup(rc.API, "")
	lenientGroup.UseMiddleware(middleware.LenientHumaJWTMiddleware(rc.SC.JWT, 7*24*time.Hour))
	huma.Post(lenientGroup, "/auth/refresh", authHandler.RefreshTokenHandler)

	// Account deletion endpoints. The delete route takes the account password,
	// so it is on passwordConfirmGroup; the summary read takes none, so putting
	// it there would spend a password guess on opening the dialog.
	huma.Get(rc.Protected, "/auth/account/deletion-summary", authHandler.GetDeletionSummaryHandler)
	huma.Post(passwordConfirmGroup, "/auth/account/delete", authHandler.DeleteAccountHandler)

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
	useAuthBudget(linkTokenGroup, authScopedRateLimiter(middleware.LimiterOAuthLinkToken, VerificationResendPerMinute))
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
	// PSY-386: signed-in home section order + visibility. No GET: the document
	// rides in the /auth/profile preferences payload the client already loads.
	huma.Put(rc.Protected, "/auth/preferences/home-layout", userPrefsHandler.SetHomeLayoutHandler)
	huma.Delete(rc.Protected, "/auth/preferences/home-layout", userPrefsHandler.ClearHomeLayoutHandler)
	// One-way stamp for the one-time first-save hint; the value rides in the
	// /auth/profile preferences payload.
	huma.Put(rc.Protected, "/auth/preferences/first-save-hint", userPrefsHandler.DismissFirstSaveHintHandler)

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

	// Passkey sign-in and the sign-up finish share one per-IP budget, more
	// lenient than the auth one because a WebAuthn flow is several requests.
	// One group means one counter across its operations, and the limiter may be
	// a no-op under DISABLE_AUTH_RATE_LIMITS.
	passkeyGroup := huma.NewGroup(rc.API, "")
	useAuthBudget(passkeyGroup, authScopedRateLimiter(middleware.LimiterPasskey, middleware.PasskeyRequestsPerMinute))

	// Public passkey login endpoints (no auth required)
	huma.Post(passkeyGroup, "/auth/passkey/login/begin", passkeyHandler.BeginLoginHandler)
	huma.Post(passkeyGroup, "/auth/passkey/login/finish", passkeyHandler.FinishLoginHandler)

	// Passkey-first sign-up (no auth required). Begin reserves a users.id from
	// the sequence on every call, so it meters on a tighter per-IP budget of
	// its own. Finish stays on passkeyGroup: by the time it is sent the
	// authenticator has written the credential, and a begin flood from the same
	// address must not be able to refuse it.
	signupBeginGroup := huma.NewGroup(rc.API, "")
	useAuthBudget(signupBeginGroup, authScopedRateLimiter(middleware.LimiterPasskeySignupBegin, middleware.PasskeySignupBeginRequestsPerMinute))
	huma.Post(signupBeginGroup, "/auth/passkey/signup/begin", passkeyHandler.BeginSignupHandler)
	huma.Post(passkeyGroup, "/auth/passkey/signup/finish", passkeyHandler.FinishSignupHandler)

	// Adding a passkey to a signed-in account. Begin meters each account on a
	// budget of its own, keyed by the session user, so accounts behind one
	// address do not share it, and every account behind one address together
	// on a per-IP ceiling. The group hangs off rc.Protected, so
	// HumaJWTMiddleware runs first and a caller with no session is refused
	// before reaching either counter. Finish carries no per-route limiter, for
	// the same reason the sign-up finish is kept off a begin budget.
	registerBeginGroup := huma.NewGroup(rc.Protected, "")
	useAuthBudget(registerBeginGroup, authScopedUserRateLimiter(rc.SC.JWT,
		middleware.RateLimitPasskeyRegisterBeginUser(), middleware.RateLimitPasskeyRegisterBeginIPCeiling()))
	huma.Post(registerBeginGroup, "/auth/passkey/register/begin", passkeyHandler.BeginRegisterHandler)
	huma.Post(rc.Protected, "/auth/passkey/register/finish", passkeyHandler.FinishRegisterHandler)

	// Protected passkey management endpoints
	huma.Get(rc.Protected, "/auth/passkey/credentials", passkeyHandler.ListCredentialsHandler)
	huma.Delete(rc.Protected, "/auth/passkey/credentials/{credential_id}", passkeyHandler.DeleteCredentialHandler)
}
