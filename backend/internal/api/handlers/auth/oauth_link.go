package auth

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
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

// oauthLinkSettingsPath is where a finished link attempt returns the browser:
// the surface that started it, so the result is read where the control is.
const oauthLinkSettingsPath = "/profile?tab=settings"

// oauthLinkIntent records which account a pending OAuth link belongs to.
type oauthLinkIntent struct {
	userID    uint
	provider  string
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

// takeOAuthLinkIntent consumes an intent. Single use: a completed or refused
// handshake must not leave the next callback on this browser armed to link.
func takeOAuthLinkIntent(id string) (oauthLinkIntent, bool) {
	oauthLinkIntentStore.Lock()
	defer oauthLinkIntentStore.Unlock()

	intent, ok := oauthLinkIntentStore.intents[id]
	delete(oauthLinkIntentStore.intents, id)
	if !ok || time.Now().After(intent.expiresAt) {
		return oauthLinkIntent{}, false
	}
	return intent, true
}

// newOAuthLinkIntentID returns an unguessable id, or an error rather than a
// weak one. The id is the only thing standing between two concurrent link
// attempts, so a degraded value must fail the request instead of being used.
func newOAuthLinkIntentID() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func (h *OAuthHTTPHandler) newLinkIntentCookie(value string, maxAge int) *http.Cookie {
	secure := false
	if h != nil && h.config != nil {
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
	if !isLinkableOAuthProvider(provider) {
		http.Error(w, "Invalid provider", http.StatusBadRequest)
		return
	}

	user := middleware.GetUserFromContext(ctx)
	if user == nil {
		logger.AuthWarn(ctx, "oauth_link_no_session", "provider", provider)
		http.Error(w, "Authentication required", http.StatusUnauthorized)
		return
	}

	intentID, err := newOAuthLinkIntentID()
	if err != nil {
		logger.AuthError(ctx, "oauth_link_intent_id_failed", err, "provider", provider)
		http.Error(w, "Failed to start account connection", http.StatusInternalServerError)
		return
	}

	storeOAuthLinkIntent(intentID, oauthLinkIntent{
		userID:    user.ID,
		provider:  provider,
		expiresAt: time.Now().Add(oauthLinkIntentTTL),
	})
	http.SetCookie(w, h.newLinkIntentCookie(intentID, int(oauthLinkIntentTTL.Seconds())))

	logger.AuthInfo(ctx, "oauth_link_started",
		"provider", provider,
		"user_id", user.ID,
	)

	beginOAuthHandshake(w, r, provider)
}

// isLinkableOAuthProvider names the providers a signed-in user may attach.
// Apple is absent: its callback is a POST to a Huma handler, not this chi
// callback, so it does not carry the intent cookie's flow.
func isLinkableOAuthProvider(provider string) bool {
	return provider == "google" || provider == "github"
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
	intent oauthLinkIntent,
	intentUsable bool,
	frontendURL string,
) {
	ctx := r.Context()

	if !intentUsable {
		logger.AuthWarn(ctx, "oauth_link_intent_expired", "provider", provider)
		redirectToLinkResult(w, r, frontendURL, autherrors.ErrOAuthLinkExpired().UserMessage())
		return
	}

	// The intent names the provider it was started for. A callback for any
	// other provider is not the handshake this intent authorized.
	if intent.provider != provider {
		logger.AuthWarn(ctx, "oauth_link_provider_mismatch",
			"intent_provider", intent.provider,
			"callback_provider", provider,
			"user_id", intent.userID,
		)
		redirectToLinkResult(w, r, frontendURL, autherrors.ErrOAuthLinkExpired().UserMessage())
		return
	}

	linked, err := h.authService.CompleteOAuthLink(w, r, provider, intent.userID)
	if err != nil {
		message := "Could not connect that account"
		var authErr *autherrors.AuthError
		if errors.As(err, &authErr) && authRefusalCarriesItsOwnCopy(authErr.Code) {
			message = authErr.UserMessage()
		}
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
	target := frontendURL + oauthLinkSettingsPath
	if message == "" {
		target += "&oauth_link=connected"
	} else {
		target += "&oauth_link_error=" + url.QueryEscape(message)
	}
	http.Redirect(w, r, target, http.StatusTemporaryRedirect)
}
