package auth

import (
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"

	"psychic-homily-backend/internal/api/middleware"
	autherrors "psychic-homily-backend/internal/errors"
	"psychic-homily-backend/internal/logger"
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

// takeOAuthLinkIntent consumes an intent, returning nil when nothing usable
// stands behind id. Single use: a completed or refused handshake must not
// leave the next callback on this browser armed to link.
func takeOAuthLinkIntent(id string) *oauthLinkIntent {
	oauthLinkIntentStore.Lock()
	defer oauthLinkIntentStore.Unlock()

	intent, ok := oauthLinkIntentStore.intents[id]
	delete(oauthLinkIntentStore.intents, id)
	if !ok || time.Now().After(intent.expiresAt) {
		return nil
	}
	return &intent
}

// oauthLinkIntentIDBytes sizes the intent id, oauthLinkStateBytes the OAuth
// state a link initiation supplies.
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
// The session is read by the JWT middleware this route is registered behind,
// so an unauthenticated caller never reaches here.
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

// completeOAuthLink finishes a callback that carried a link intent. It never
// falls through to the sign-in path: that path resolves an account from the
// provider's address, and a link resolves it from the session that started the
// attempt. Reaching here with no usable intent means the account to attach to
// is unknown, which is a refusal rather than a sign-in.
func (h *OAuthHTTPHandler) completeOAuthLink(
	w http.ResponseWriter,
	r *http.Request,
	provider string,
	intent *oauthLinkIntent,
	frontendURL string,
) {
	ctx := r.Context()

	if intent == nil {
		logger.AuthWarn(ctx, "oauth_link_intent_expired", "provider", provider)
		redirectToLinkResult(w, r, frontendURL, autherrors.ErrOAuthLinkExpired().UserMessage())
		return
	}

	// An intent authorizes ONE handshake: the provider it was started for, and
	// the state its initiation put on that provider's authorization URL. A
	// callback that matches neither is a different flow, and diverting it into
	// the link path would attach whoever just authenticated to the account the
	// intent names.
	if intent.provider != provider || intent.state != r.URL.Query().Get("state") {
		logger.AuthWarn(ctx, "oauth_link_intent_not_for_this_handshake",
			"intent_provider", intent.provider,
			"callback_provider", provider,
			"state_matches", intent.state == r.URL.Query().Get("state"),
			"user_id", intent.userID,
		)
		redirectToLinkResult(w, r, frontendURL, autherrors.ErrOAuthLinkExpired().UserMessage())
		return
	}

	linked, err := h.authService.CompleteOAuthLink(w, r, provider, intent.userID)
	if err != nil {
		message := refusalMessage(err, "Could not connect that account")
		logger.AuthWarn(ctx, "oauth_link_failed",
			"provider", provider,
			"user_id", intent.userID,
			"error", err.Error(),
		)
		redirectToLinkResult(w, r, frontendURL, message)
		return
	}

	logger.AuthInfo(ctx, "oauth_link_connected",
		"provider", provider,
		"user_id", linked.ID,
	)
	redirectToLinkResult(w, r, frontendURL, "")
}

// redirectToLinkResult returns the browser to Settings. An empty message is
// the success case. The message travels in the URL because the destination is
// a fresh page load with no other channel to it, and it is server-authored
// copy, never caller-supplied text.
func redirectToLinkResult(w http.ResponseWriter, r *http.Request, frontendURL, message string) {
	query := url.Values{oauthLinkSettingsTabKey: {oauthLinkSettingsTabValue}}
	if message == "" {
		query.Set(oauthLinkResultParam, "connected")
	} else {
		query.Set(oauthLinkErrorParam, message)
	}
	http.Redirect(w, r, frontendURL+oauthLinkSettingsPath+"?"+query.Encode(), http.StatusTemporaryRedirect)
}
