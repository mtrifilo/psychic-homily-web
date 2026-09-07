package auth

import (
	"errors"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"

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

// oauthLinkTokenTTL bounds the one-time token Settings mints before starting a
// link. It only has to survive the click that follows minting it.
const oauthLinkTokenTTL = 5 * time.Minute

// oauthLinkTokenParam carries that token on the start URL.
const oauthLinkTokenParam = "t"

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
// refused instead of signing the user in.
func peekOAuthLinkIntent(id string) *oauthLinkIntent {
	oauthLinkIntentStore.Lock()
	defer oauthLinkIntentStore.Unlock()

	intent, ok := oauthLinkIntentStore.intents[id]
	if !ok || time.Now().After(intent.expiresAt) {
		return nil
	}
	return &intent
}

// consumeOAuthLinkIntent removes an intent once it has been matched to the
// handshake in hand. Single use: a completed or refused link must not leave the
// next callback on this browser armed.
func consumeOAuthLinkIntent(id string) {
	oauthLinkIntentStore.Lock()
	defer oauthLinkIntentStore.Unlock()
	delete(oauthLinkIntentStore.intents, id)
}

// oauthLinkToken is the one-time proof that a link was started from this
// application's own Settings page rather than from a page an attacker
// controls. See mintOAuthLinkToken for why the route needs one.
type oauthLinkToken struct {
	userID    uint
	expiresAt time.Time
}

var oauthLinkTokenStore = struct {
	sync.Mutex
	tokens map[string]oauthLinkToken
}{
	tokens: make(map[string]oauthLinkToken),
}

// mintOAuthLinkToken issues a token bound to userID.
//
// /auth/link/{provider} is a cookie-authenticated GET, and the auth cookie is
// SameSite=Lax, which a browser DOES send on a cross-site top-level
// navigation. Without this token any page on the internet could navigate a
// signed-in user into the link flow, and a user with a live provider session
// completes it with no interaction at all, attaching the attacker's identity
// to their account. The token cannot be minted cross-site: it comes from an
// authenticated same-origin request that CORS will not let another origin read.
func mintOAuthLinkToken(userID uint) (string, error) {
	token, err := randomHexID(oauthLinkIntentIDBytes)
	if err != nil {
		return "", err
	}

	oauthLinkTokenStore.Lock()
	defer oauthLinkTokenStore.Unlock()

	now := time.Now()
	for k, v := range oauthLinkTokenStore.tokens {
		if now.After(v.expiresAt) {
			delete(oauthLinkTokenStore.tokens, k)
		}
	}
	oauthLinkTokenStore.tokens[token] = oauthLinkToken{
		userID:    userID,
		expiresAt: now.Add(oauthLinkTokenTTL),
	}
	return token, nil
}

// consumeOAuthLinkToken spends a token and reports whether it was live and
// belonged to userID. Bound to the user, not just to existence, so one
// account's token cannot start a link on another's session.
func consumeOAuthLinkToken(token string, userID uint) bool {
	if token == "" {
		return false
	}

	oauthLinkTokenStore.Lock()
	defer oauthLinkTokenStore.Unlock()

	stored, ok := oauthLinkTokenStore.tokens[token]
	delete(oauthLinkTokenStore.tokens, token)
	return ok && stored.userID == userID && time.Now().Before(stored.expiresAt)
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

// requestIsCrossSite reports whether the browser told us this navigation came
// from another site. Fetch metadata is sent by every current browser and cannot
// be set by page script, so a present "cross-site" is trustworthy. Absent is
// not evidence either way, which is why it is only ever a second lock: the
// one-time token is the one that holds on a browser that sends nothing.
func requestIsCrossSite(r *http.Request) bool {
	return strings.EqualFold(r.Header.Get("Sec-Fetch-Site"), "cross-site")
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

	if requestIsCrossSite(r) {
		logger.AuthWarn(ctx, "oauth_link_refused_cross_site",
			"provider", provider,
			"user_id", user.ID,
		)
		http.Error(w, "Start the connection from Settings", http.StatusForbidden)
		return
	}

	if !consumeOAuthLinkToken(r.URL.Query().Get(oauthLinkTokenParam), user.ID) {
		logger.AuthWarn(ctx, "oauth_link_refused_missing_token",
			"provider", provider,
			"user_id", user.ID,
		)
		http.Error(w, "Start the connection from Settings", http.StatusForbidden)
		return
	}

	sessionIssuedAt, _ := middleware.GetSessionIssuedAtFromContext(ctx)
	if factor := linkReauthFactorFor(user, sessionIssuedAt, time.Now()); factor != reauthAlreadySatisfied {
		logger.AuthWarn(ctx, "oauth_link_refused_stale_session",
			"provider", provider,
			"user_id", user.ID,
			"required_factor", string(factor),
		)
		http.Error(w, "Sign in again before connecting an account", http.StatusForbidden)
		return
	}

	intentID, err := randomHexID(oauthLinkIntentIDBytes)
	if err != nil {
		logger.AuthError(ctx, "oauth_link_intent_id_failed", err, "provider", provider)
		http.Error(w, "Failed to start account connection", http.StatusInternalServerError)
		return
	}

	// gothic generates its own state when the request carries none. Supplying
	// one here is what lets the callback check that the intent it found was
	// armed for the handshake that just came back.
	state, err := randomHexID(oauthLinkStateBytes)
	if err != nil {
		logger.AuthError(ctx, "oauth_link_state_failed", err, "provider", provider)
		http.Error(w, "Failed to start account connection", http.StatusInternalServerError)
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
		http.Error(w, "Failed to start account connection", http.StatusInternalServerError)
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

	// The account gained a way to sign in. Nothing emails the owner about that
	// yet; this line is what an operator has to correlate from until something
	// does. Follow-up: an account-security email on link.
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
