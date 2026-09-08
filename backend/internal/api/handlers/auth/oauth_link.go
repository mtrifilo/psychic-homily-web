package auth

import (
	"errors"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"

	"psychic-homily-backend/internal/api/handlers/shared"
	"psychic-homily-backend/internal/api/middleware"
	autherrors "psychic-homily-backend/internal/errors"
	"psychic-homily-backend/internal/logger"
	"psychic-homily-backend/internal/observability"
	"psychic-homily-backend/internal/utils"
)

// oauthLinkIntentCookieName holds an opaque id for the pending link. The
// account being linked to lives in the server-side store, never in a value the
// browser holds, so a cookie an attacker could plant names nothing.
const oauthLinkIntentCookieName = "oauth_link_intent"

// oauthLinkIntentTTL bounds a pending link. Long enough for a consent screen
// and a second factor at the provider, short enough that an abandoned attempt
// does not stay armed on a shared machine. Matches the signup consent cookie.
const oauthLinkIntentTTL = 10 * time.Minute

// Where a finished link attempt returns the browser: the surface that started
// it, so the result is read where the control is. Split into path and query
// parts because the result is appended as another query parameter, and a bare
// string carrying its own "?" makes that append depend on punctuation two
// hundred lines away.
const (
	oauthLinkSettingsPath     = "/profile"
	oauthLinkSettingsTabKey   = "tab"
	oauthLinkSettingsTabValue = "settings"
)

// The query keys the result travels back under. The Settings panel reads the
// same two names.
//
// The error parameter carries a CODE, never prose. The panel renders whatever
// it finds there into the page, so server-authored copy travelling in the URL
// is copy an attacker can rewrite by handing the user a link: a redirect to
// Settings with any sentence they like, displayed inside the signed-in account
// as if this application had said it. A code that does not map to known copy
// renders the generic failure.
const (
	oauthLinkResultParam = "oauth_link"
	oauthLinkErrorParam  = "oauth_link_error"
)

// Codes the START refuses with. They travel the same way the callback's do,
// because this route is reached by a top-level navigation: answering with a
// plain-text body would leave the user on the backend origin, with no nav and
// no way back to the page they started from.
const (
	oauthLinkErrorNotFromSettings = "OAUTH_LINK_NOT_FROM_SETTINGS"
	oauthLinkErrorReauthRequired  = "OAUTH_LINK_REAUTH_REQUIRED"
	oauthLinkErrorStartFailed     = "OAUTH_LINK_START_FAILED"
)

// oauthLinkIntent records which account a pending OAuth link belongs to, and
// which handshake it was armed for.
//
// state is the OAuth state parameter this intent's initiation put on the
// provider's authorization URL. Binding to it is what keeps an intent from
// being spent by a DIFFERENT handshake: without it, an abandoned link leaves
// the cookie live for its whole TTL, and the next callback on this browser,
// including an ordinary sign-in, is diverted into the link path. On a shared
// machine that would attach the next person's provider identity to the
// account whose owner started the abandoned link.
type oauthLinkIntent struct {
	userID    uint
	provider  string
	state     string
	expiresAt time.Time
}

// oauthLinkIntentStore holds pending link intents by opaque id. Process-local
// and deliberately not durable: an intent that does not survive a restart or a
// second instance produces an expired-link refusal, which is the fail-closed
// direction. Mirrors cliCallbackStore, which makes the same trade.
var oauthLinkIntentStore = struct {
	sync.Mutex
	intents map[string]oauthLinkIntent
}{
	intents: make(map[string]oauthLinkIntent),
}

func storeOAuthLinkIntent(id string, intent oauthLinkIntent) {
	oauthLinkIntentStore.Lock()
	defer oauthLinkIntentStore.Unlock()

	now := time.Now()
	for k, v := range oauthLinkIntentStore.intents {
		if now.After(v.expiresAt) {
			delete(oauthLinkIntentStore.intents, k)
		}
	}

	oauthLinkIntentStore.intents[id] = intent
}

// peekOAuthLinkIntent reads an intent WITHOUT consuming it, so the callback can
// decide whether the intent is even for this handshake before spending it. A
// stale intent belonging to some other flow has to survive the look, or an
// ordinary sign-in that happens to carry the cookie would burn it and then be
// refused instead of signing the reader in.
//
// found distinguishes "this process never held it" from "it is past its TTL",
// which are different situations: the first is a restart or another replica
// and must not turn a sign-in into a refusal, the second is a reader who
// genuinely ran out of time and is owed an explanation.
func peekOAuthLinkIntent(id string) (intent oauthLinkIntent, found bool) {
	oauthLinkIntentStore.Lock()
	defer oauthLinkIntentStore.Unlock()

	intent, found = oauthLinkIntentStore.intents[id]
	return intent, found
}

// consumeOAuthLinkIntent removes an intent once it has been matched to the
// handshake in hand. Single use: a completed or refused link must not leave the
// next callback on this browser armed.
func consumeOAuthLinkIntent(id string) {
	oauthLinkIntentStore.Lock()
	defer oauthLinkIntentStore.Unlock()
	delete(oauthLinkIntentStore.intents, id)
}

// oauthLinkIntentIDBytes sizes the intent id and the link token,
// oauthLinkStateBytes the OAuth state a link initiation supplies.
const (
	oauthLinkIntentIDBytes = 32
	oauthLinkStateBytes    = 32
)

func (h *OAuthHTTPHandler) newLinkIntentCookie(value string, maxAge int) *http.Cookie {
	secure := false
	if h.config != nil {
		secure = h.config.Session.Secure
	}
	return &http.Cookie{
		Name:     oauthLinkIntentCookieName,
		Value:    value,
		Path:     "/",
		MaxAge:   maxAge,
		HttpOnly: true,
		// Lax, not Strict: the provider returns the browser to the callback by
		// a cross-site top-level navigation, which Strict would strip. Same
		// reasoning as the signup consent cookie.
		SameSite: http.SameSiteLaxMode,
		Secure:   secure,
	}
}

// OAuthLinkHTTPHandler begins an OAuth link for the authenticated caller.
//
// It is the authenticated counterpart to OAuthLoginHTTPHandler and shares its
// callback, because a provider's redirect URI is registered once and cannot
// vary per flow. What distinguishes the two at the callback is the intent
// cookie this handler sets.
//
// Three things have to hold before the handshake starts: a session (the JWT
// middleware this route sits behind), a one-time token minted to this account
// from Settings, and a re-authentication recent enough to stand behind adding
// a new way to sign in.
func (h *OAuthHTTPHandler) OAuthLinkHTTPHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	frontendURL := h.frontendURL()

	provider := chi.URLParam(r, "provider")
	if !isGothOAuthProvider(provider) {
		http.Error(w, "Invalid provider", http.StatusBadRequest)
		return
	}

	user := middleware.GetUserFromContext(ctx)
	if user == nil {
		logger.AuthWarn(ctx, "oauth_link_no_session", "provider", provider)
		http.Error(w, "Authentication required", http.StatusUnauthorized)
		return
	}

	if !requestIsFromOurFrontend(r, frontendURL) {
		logger.AuthWarn(ctx, "oauth_link_refused_not_from_frontend",
			"provider", provider,
			"user_id", user.ID,
		)
		redirectToLinkResult(w, r, frontendURL, oauthLinkErrorNotFromSettings)
		return
	}

	// Re-authentication is checked BEFORE the one-time token is spent. A
	// refusal here sends the user to sign in and come back, and burning the
	// token on the way would make the return trip fail for a second reason.
	//
	// Re-authenticating is "sign in again", the one challenge every account
	// shape already has: password, passkey, provider or magic link, whichever
	// it holds. shared.ReauthFactorFor names the factor for the log so the rule
	// stays legible and in one place.
	// hasPasskey is false: see the KNOWN GAP on shared.ReauthFactorFor.
	sessionAuthAt := middleware.GetSessionAuthTimeFromContext(ctx)
	if factor := shared.ReauthFactorFor(shared.AccountHasPassword(user), false, sessionAuthAt, time.Now()); factor != shared.ReauthAlreadySatisfied {
		logger.AuthWarn(ctx, "oauth_link_refused_stale_session",
			"provider", provider,
			"user_id", user.ID,
			"required_factor", string(factor),
		)
		redirectToReauth(w, r, frontendURL)
		return
	}

	if !consumeOAuthLinkToken(h.jwtSecret(), r.URL.Query().Get(oauthLinkTokenParam), user.ID) {
		logger.AuthWarn(ctx, "oauth_link_refused_missing_token",
			"provider", provider,
			"user_id", user.ID,
		)
		redirectToLinkResult(w, r, frontendURL, oauthLinkErrorNotFromSettings)
		return
	}

	intentID, err := randomHexID(oauthLinkIntentIDBytes)
	if err != nil {
		logger.AuthError(ctx, "oauth_link_intent_id_failed", err, "provider", provider)
		redirectToLinkResult(w, r, frontendURL, oauthLinkErrorStartFailed)
		return
	}

	// gothic generates its own state when the request carries none. Supplying
	// one here is what lets the callback check that the intent it found was
	// armed for the handshake that just came back.
	state, err := randomHexID(oauthLinkStateBytes)
	if err != nil {
		logger.AuthError(ctx, "oauth_link_state_failed", err, "provider", provider)
		redirectToLinkResult(w, r, frontendURL, oauthLinkErrorStartFailed)
		return
	}

	storeOAuthLinkIntent(intentID, oauthLinkIntent{
		userID:    user.ID,
		provider:  provider,
		state:     state,
		expiresAt: time.Now().Add(oauthLinkIntentTTL),
	})
	http.SetCookie(w, h.newLinkIntentCookie(intentID, int(oauthLinkIntentTTL.Seconds())))

	logger.AuthInfo(ctx, "oauth_link_started",
		"provider", provider,
		"user_id", user.ID,
	)

	// state is already minted above and stored on the intent, so this cannot
	// take the generating branch; the error is handled because the signature
	// admits one, not because this call is expected to fail.
	if err := beginOAuthHandshake(w, r, provider, state); err != nil {
		logger.AuthError(ctx, "oauth_link_handshake_failed", err, "provider", provider)
		redirectToLinkResult(w, r, frontendURL, oauthLinkErrorStartFailed)
		return
	}
}

// completeOAuthLink finishes a callback whose intent matched this handshake.
// The caller has already established the match, so reaching here means the
// account to attach to is known.
func (h *OAuthHTTPHandler) completeOAuthLink(
	w http.ResponseWriter,
	r *http.Request,
	provider string,
	intent *oauthLinkIntent,
	frontendURL string,
) {
	ctx := r.Context()

	// The intent names the account, but only the LIVE session proves the
	// person finishing this handshake is still the one who started it.
	//
	// Without this, an abandoned link stays resumable by whoever is at the
	// browser next: the authorization URL still carries the state, so pressing
	// Back and signing in with THEIR provider account attaches their identity
	// to the account named by the intent. Signing out does not clear the
	// intent cookie either, so it survives the obvious precaution.
	sessionUserID, ok := middleware.SessionUserIDFromRequest(h.jwtService, r)
	if !ok || sessionUserID != intent.userID {
		logger.AuthWarn(ctx, "oauth_link_refused_session_mismatch",
			"provider", provider,
			"intent_user_id", intent.userID,
			"session_present", ok,
		)
		redirectToLinkResult(w, r, h.frontendURL(), oauthLinkErrorReauthRequired)
		return
	}

	linked, err := h.authService.CompleteOAuthLink(w, r, provider, intent.userID)
	if err != nil {
		// A provider error can carry credentials in its URL or in an embedded
		// response body, the same hazard the sign-in callback scrubs for.
		logger.AuthWarn(ctx, "oauth_link_failed",
			"provider", provider,
			"user_id", intent.userID,
			"error", observability.ScrubText(utils.RedactErrorURL(err).Error()),
		)
		redirectToLinkResult(w, r, frontendURL, refusalCode(err, autherrors.CodeUnknown))
		return
	}

	// The account gained a way to sign in. This line is the record of that.
	logger.AuthInfo(ctx, "oauth_link_completed",
		"provider", provider,
		"user_id", linked.ID,
	)
	redirectToLinkResult(w, r, frontendURL, "")
}

// redirectToLinkResult returns the browser to Settings. An empty code is the
// success case; any other value is an error code the panel maps to its own
// copy. Codes, not sentences: see the parameter's own comment.
func redirectToLinkResult(w http.ResponseWriter, r *http.Request, frontendURL, code string) {
	query := url.Values{oauthLinkSettingsTabKey: {oauthLinkSettingsTabValue}}
	if code == "" {
		query.Set(oauthLinkResultParam, "connected")
	} else {
		query.Set(oauthLinkErrorParam, code)
	}
	http.Redirect(w, r, frontendURL+oauthLinkSettingsPath+"?"+query.Encode(), http.StatusTemporaryRedirect)
}

// redirectToReauth sends the browser to the sign-in page with a destination
// that brings it back to the control it started from. Signing in mints a fresh
// session, which is what satisfies shared.ReauthFactorFor on the next attempt.
//
// returnTo is the contract app/auth reads through sanitizeReturnTo, which
// requires a same-origin path; the Settings tab qualifies.
func redirectToReauth(w http.ResponseWriter, r *http.Request, frontendURL string) {
	settings := oauthLinkSettingsPath + "?" + url.Values{
		oauthLinkSettingsTabKey: {oauthLinkSettingsTabValue},
	}.Encode()
	query := url.Values{
		"returnTo": {settings},
		"reason":   {oauthLinkErrorReauthRequired},
	}
	http.Redirect(w, r, frontendURL+"/auth?"+query.Encode(), http.StatusTemporaryRedirect)
}

// jwtSecret signs the one-time link token. The same secret the session uses:
// both are this server asserting something to itself across a round trip.
func (h *OAuthHTTPHandler) jwtSecret() string {
	if h.config == nil {
		return ""
	}
	return h.config.JWT.SecretKey
}

// frontendURL is where every browser-facing redirect from this handler goes.
func (h *OAuthHTTPHandler) frontendURL() string {
	if h.config != nil && h.config.Email.FrontendURL != "" {
		return h.config.Email.FrontendURL
	}
	return "http://localhost:3000"
}

// refusalCode picks the error code a link result reports: the refusal's own
// code when it is one a caller may act on, and fallback for everything else,
// so a backend fault is never described to a caller in its own terms.
func refusalCode(err error, fallback string) string {
	var authErr *autherrors.AuthError
	if errors.As(err, &authErr) && authRefusalCarriesItsOwnCopy(authErr.Code) {
		return authErr.Code
	}
	return fallback
}
