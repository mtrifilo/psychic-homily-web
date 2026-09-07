package auth

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"

	"psychic-homily-backend/internal/config"
	autherrors "psychic-homily-backend/internal/errors"
	"psychic-homily-backend/internal/logger"
	"psychic-homily-backend/internal/observability"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/utils"

	"github.com/go-chi/chi/v5"
	"github.com/markbates/goth/gothic"
)

const oauthSignupConsentCookieName = "oauth_signup_consent"

func generateRandomID() string {
	bytes := make([]byte, 16)
	rand.Read(bytes)
	return hex.EncodeToString(bytes)
}

// cliCallbackStore stores CLI callback URLs temporarily during OAuth flow
// Key is a unique identifier stored in a cookie, value is the callback URL
var cliCallbackStore = struct {
	sync.RWMutex
	callbacks map[string]cliCallbackEntry
}{
	callbacks: make(map[string]cliCallbackEntry),
}

type cliCallbackEntry struct {
	callbackURL string
	expiresAt   time.Time
}

func storeCLICallback(id, callbackURL string) {
	cliCallbackStore.Lock()
	defer cliCallbackStore.Unlock()

	// Clean up expired entries
	now := time.Now()
	for k, v := range cliCallbackStore.callbacks {
		if now.After(v.expiresAt) {
			delete(cliCallbackStore.callbacks, k)
		}
	}

	cliCallbackStore.callbacks[id] = cliCallbackEntry{
		callbackURL: callbackURL,
		expiresAt:   now.Add(5 * time.Minute),
	}
}

func getCLICallback(id string) (string, bool) {
	cliCallbackStore.RLock()
	defer cliCallbackStore.RUnlock()

	entry, ok := cliCallbackStore.callbacks[id]
	if !ok || time.Now().After(entry.expiresAt) {
		return "", false
	}
	return entry.callbackURL, true
}

func deleteCLICallback(id string) {
	cliCallbackStore.Lock()
	defer cliCallbackStore.Unlock()
	delete(cliCallbackStore.callbacks, id)
}

// requestCookieNames returns the names of the request's cookies. Cookie values
// are credentials; only names are loggable.
func requestCookieNames(r *http.Request) []string {
	cookies := r.Cookies()
	names := make([]string, 0, len(cookies))
	for _, c := range cookies {
		names = append(names, c.Name)
	}
	return names
}

// OAuthHTTPHandler handles OAuth HTTP requests directly
type OAuthHTTPHandler struct {
	authService contracts.AuthServiceInterface
	config      *config.Config
}

// NewOAuthHTTPHandler creates a new OAuth HTTP handler
func NewOAuthHTTPHandler(authService contracts.AuthServiceInterface, cfg *config.Config) *OAuthHTTPHandler {
	return &OAuthHTTPHandler{
		authService: authService,
		config:      cfg,
	}
}

// OAuthLoginHTTPHandler handles OAuth login initiation via HTTP
func (h *OAuthHTTPHandler) OAuthLoginHTTPHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	secureCookie := false
	if h != nil && h.config != nil {
		secureCookie = h.config.Session.Secure
	}

	// Get provider from path parameter
	provider := chi.URLParam(r, "provider")
	if provider == "" {
		http.Error(w, "Provider required", http.StatusBadRequest)
		return
	}

	// Validate provider
	if provider != "google" && provider != "github" {
		http.Error(w, "Invalid provider", http.StatusBadRequest)
		return
	}

	// Capture explicit signup consent for later enforcement in callback.
	signupIntent := r.URL.Query().Get("signup_intent") == "1"
	if signupIntent {
		termsAccepted := r.URL.Query().Get("terms_accepted") == "true"
		termsVersion := r.URL.Query().Get("terms_version")
		privacyVersion := r.URL.Query().Get("privacy_version")
		if !termsAccepted || termsVersion == "" {
			http.Error(w, "Terms acceptance is required for account creation", http.StatusBadRequest)
			return
		}

		// Require the minimum-age confirmation (PSY-1023), mirroring the terms
		// gate. min_age_attested is re-checked server-side against MinSignupAge so
		// a tampered/absent query param can never bypass the gate.
		ageConfirmed := r.URL.Query().Get("age_confirmed") == "true"
		minAgeAttested, _ := strconv.Atoi(r.URL.Query().Get("min_age_attested"))
		if !ageConfirmed || minAgeAttested < MinSignupAge {
			http.Error(w, "Age confirmation is required for account creation", http.StatusBadRequest)
			return
		}

		encodedConsent, err := encodeOAuthSignupConsent(contracts.OAuthSignupConsent{
			TermsAccepted:  true,
			TermsVersion:   termsVersion,
			PrivacyVersion: privacyVersion,
			AcceptedAt:     time.Now().UTC(),
			AgeConfirmed:   true,
			MinAgeAttested: minAgeAttested,
		})
		if err != nil {
			http.Error(w, "Failed to process signup consent", http.StatusInternalServerError)
			return
		}

		http.SetCookie(w, &http.Cookie{
			Name:     oauthSignupConsentCookieName,
			Value:    encodedConsent,
			Path:     "/",
			MaxAge:   600, // 10 minutes for full OAuth round-trip
			HttpOnly: true,
			SameSite: http.SameSiteLaxMode,
			Secure:   secureCookie,
		})
	}

	// Check for CLI callback parameter
	cliCallback := r.URL.Query().Get("cli_callback")
	if cliCallback != "" {
		// Reject any callback that is not a loopback URL. The OAuth callback
		// appends a 24h JWT to this destination, so an attacker-controlled
		// host (e.g. ?cli_callback=https://evil.com/steal) would exfiltrate
		// the victim's token. SameSite=Lax does NOT mitigate — the OAuth
		// cookie is set on this same top-level navigation.
		validated, err := validateCLICallback(cliCallback)
		if err != nil {
			logger.AuthWarn(ctx, "oauth_cli_callback_rejected",
				"stage", "initiation",
				"provider", provider,
				"error", err.Error(),
			)
			http.Error(w, "Invalid cli_callback", http.StatusBadRequest)
			return
		}
		cliCallback = validated

		// Generate unique ID and store callback in memory
		callbackID := generateRandomID()
		storeCLICallback(callbackID, cliCallback)

		// Store only the ID in a cookie (not the full URL)
		http.SetCookie(w, &http.Cookie{
			Name:     "cli_callback_id",
			Value:    callbackID,
			Path:     "/",
			MaxAge:   300, // 5 minutes
			HttpOnly: true,
			SameSite: http.SameSiteLaxMode,
		})
		// callbackID is the cli_callback_id cookie value: the correlation key
		// that gates the token-bearing redirect, so it is never logged.
		logger.AuthDebug(ctx, "oauth_cli_callback_stored", "callback", cliCallback)
	}

	// Add provider to query parameters for Goth (following Goth best practices)
	q := r.URL.Query()
	q.Add("provider", provider)
	r.URL.RawQuery = q.Encode()

	logger.AuthDebug(ctx, "oauth_login_request",
		"provider", provider,
		"path", r.URL.Path,
		"cookie_names", requestCookieNames(r),
	)

	// Use Goth's standard BeginAuthHandler directly
	gothic.BeginAuthHandler(w, r)

	logger.AuthDebug(ctx, "oauth_login_request_returned", "provider", provider)
}

// OAuthCallbackHTTPHandler handles OAuth callback via HTTP
func (h *OAuthHTTPHandler) OAuthCallbackHTTPHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	secureCookie := false
	if h != nil && h.config != nil {
		secureCookie = h.config.Session.Secure
	}

	// Get provider from path parameter
	provider := chi.URLParam(r, "provider")
	if provider == "" {
		provider = "google" // fallback
	}

	logger.AuthDebug(ctx, "oauth_callback_provider_resolved", "provider", provider)

	// Check for CLI callback via cookie ID + memory store
	var cliCallback string
	if cookie, err := r.Cookie("cli_callback_id"); err == nil {
		callbackID := cookie.Value
		if callback, ok := getCLICallback(callbackID); ok {
			deleteCLICallback(callbackID)
			// Defense in depth: re-validate the stored callback before we
			// build a token-bearing redirect from it. Even though the value
			// was checked at initiation, never trust a stored redirect target
			// against the loopback allowlist a second time. A rejected value
			// falls back to the standard web flow (no token leaked).
			if validated, verr := validateCLICallback(callback); verr == nil {
				cliCallback = validated
				logger.AuthDebug(ctx, "oauth_cli_callback_found", "callback", cliCallback)
			} else {
				logger.AuthWarn(ctx, "oauth_cli_callback_rejected",
					"stage", "callback",
					"provider", provider,
					"error", verr.Error(),
				)
			}
		}
		// Clear the CLI callback ID cookie
		http.SetCookie(w, &http.Cookie{
			Name:     "cli_callback_id",
			Value:    "",
			Path:     "/",
			MaxAge:   -1,
			HttpOnly: true,
		})
	}

	// Read signup consent captured during OAuth initiation (if present).
	var signupConsent *contracts.OAuthSignupConsent
	if cookie, err := r.Cookie(oauthSignupConsentCookieName); err == nil {
		consent, decodeErr := decodeOAuthSignupConsent(cookie.Value)
		if decodeErr != nil {
			logger.AuthWarn(ctx, "oauth_signup_consent_cookie_decode_failed",
				"provider", provider,
				"error", decodeErr.Error(),
			)
		} else {
			signupConsent = consent
		}

		// Always clear the one-time consent cookie.
		http.SetCookie(w, &http.Cookie{
			Name:     oauthSignupConsentCookieName,
			Value:    "",
			Path:     "/",
			MaxAge:   -1,
			HttpOnly: true,
			SameSite: http.SameSiteLaxMode,
			Secure:   secureCookie,
		})
	}

	// Add provider to query parameters for Goth (following best practices)
	q := r.URL.Query()
	q.Add("provider", provider)
	r.URL.RawQuery = q.Encode()

	// Get frontend URL for redirects
	frontendURL := h.config.Email.FrontendURL
	if frontendURL == "" {
		frontendURL = "http://localhost:3000"
	}

	// Use AuthService to handle the complete OAuth flow. New users require consent.
	user, token, err := h.authService.OAuthCallbackWithConsent(w, r, provider, signupConsent)
	if err != nil {
		// A provider error can carry credentials two ways: a *url.Error whose
		// URL holds an access token in the query, and an error whose text
		// embeds the token endpoint's raw response body. RedactErrorURL keeps
		// scheme and host and drops path and query, which also drops the
		// wrapper text; ScrubText then covers the shapes that are not a
		// *url.Error and caps an unbounded body. The wrapped value carries the
		// scrubbed text and not the original chain, because AuthError renders
		// whatever error it is handed; the chain is still read below for
		// errors.As.
		logger.AuthError(ctx, "oauth_callback_failed",
			errors.New(observability.ScrubText(utils.RedactErrorURL(err).Error())),
			"provider", provider,
		)
		errorMessage := "authentication failed"
		var authErr *autherrors.AuthError
		if errors.As(err, &authErr) && authErr.Code == autherrors.CodeTermsAcceptanceRequired {
			errorMessage = authErr.UserMessage()
		}

		// Handle CLI callback error
		if cliCallback != "" {
			redirectURL := cliCallback + "?error=" + url.QueryEscape(errorMessage)
			http.Redirect(w, r, redirectURL, http.StatusTemporaryRedirect)
			return
		}

		// Redirect to frontend auth page with error
		redirectURL := frontendURL + "/auth?error=" + url.QueryEscape(errorMessage)
		http.Redirect(w, r, redirectURL, http.StatusTemporaryRedirect)
		return
	}

	logger.AuthInfo(ctx, "oauth_callback_success", "provider", provider, "user_id", user.ID)

	// Handle CLI callback - redirect with token instead of setting cookie
	if cliCallback != "" {
		// Token expires in 24 hours (86400 seconds)
		redirectURL := cliCallback + "?token=" + url.QueryEscape(token) + "&expires_in=86400"
		logger.AuthDebug(ctx, "oauth_cli_redirect", "callback", cliCallback)
		http.Redirect(w, r, redirectURL, http.StatusTemporaryRedirect)
		return
	}

	// Standard web flow - set HTTP-only auth cookie
	cookie := h.config.Session.NewAuthCookie(token, 7*24*time.Hour)
	http.SetCookie(w, &cookie)

	// Redirect to frontend home page
	http.Redirect(w, r, frontendURL, http.StatusTemporaryRedirect)
}

func encodeOAuthSignupConsent(consent contracts.OAuthSignupConsent) (string, error) {
	data, err := json.Marshal(consent)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(data), nil
}

func decodeOAuthSignupConsent(value string) (*contracts.OAuthSignupConsent, error) {
	data, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return nil, err
	}
	var consent contracts.OAuthSignupConsent
	if err := json.Unmarshal(data, &consent); err != nil {
		return nil, err
	}
	return &consent, nil
}
