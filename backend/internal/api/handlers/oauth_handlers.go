package handlers

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"psychic-homily-backend/internal/config"
	"psychic-homily-backend/internal/services"

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

// OAuthHTTPHandler handles OAuth HTTP requests directly
type OAuthHTTPHandler struct {
	authService *services.AuthService
	config      *config.Config
}

// NewOAuthHTTPHandler creates a new OAuth HTTP handler
func NewOAuthHTTPHandler(authService *services.AuthService, cfg *config.Config) *OAuthHTTPHandler {
	return &OAuthHTTPHandler{
		authService: authService,
		config:      cfg,
	}
}

// OAuthLoginHTTPHandler handles OAuth login initiation via HTTP
func (h *OAuthHTTPHandler) OAuthLoginHTTPHandler(w http.ResponseWriter, r *http.Request) {
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

		encodedConsent, err := encodeOAuthSignupConsent(services.OAuthSignupConsent{
			TermsAccepted:  true,
			TermsVersion:   termsVersion,
			PrivacyVersion: privacyVersion,
			AcceptedAt:     time.Now().UTC(),
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
		log.Printf("DEBUG: CLI callback stored with ID %s: %s", callbackID, cliCallback)
	}

	// Add provider to query parameters for Goth (following Goth best practices)
	q := r.URL.Query()
	q.Add("provider", provider)
	r.URL.RawQuery = q.Encode()

	// DEBUG: Check session before OAuth
	log.Printf("DEBUG: Login - Request URL: %s", r.URL.String())
	log.Printf("DEBUG: Login - Request cookies BEFORE: %+v", r.Cookies())

	// Use Goth's standard BeginAuthHandler directly
	gothic.BeginAuthHandler(w, r)

	log.Printf("DEBUG: Login - After BeginAuthHandler call")
}

// OAuthCallbackHTTPHandler handles OAuth callback via HTTP
func (h *OAuthHTTPHandler) OAuthCallbackHTTPHandler(w http.ResponseWriter, r *http.Request) {
	secureCookie := false
	if h != nil && h.config != nil {
		secureCookie = h.config.Session.Secure
	}

	// Get provider from path parameter
	provider := chi.URLParam(r, "provider")
	if provider == "" {
		provider = "google" // fallback
	}

	log.Printf("DEBUG: Using provider '%s' from URL path", provider)

	// Check for CLI callback via cookie ID + memory store
	var cliCallback string
	if cookie, err := r.Cookie("cli_callback_id"); err == nil {
		callbackID := cookie.Value
		if callback, ok := getCLICallback(callbackID); ok {
			cliCallback = callback
			deleteCLICallback(callbackID)
			log.Printf("DEBUG: CLI callback found for ID %s: %s", callbackID, cliCallback)
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
	var signupConsent *services.OAuthSignupConsent
	if cookie, err := r.Cookie(oauthSignupConsentCookieName); err == nil {
		consent, decodeErr := decodeOAuthSignupConsent(cookie.Value)
		if decodeErr != nil {
			log.Printf("WARN: failed to decode OAuth signup consent cookie: %v", decodeErr)
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
		log.Printf("OAuth callback failed: %v", err)
		errorMessage := "authentication failed"
		if strings.Contains(err.Error(), "terms acceptance required") || strings.Contains(err.Error(), "terms version is required") {
			errorMessage = "Please accept the Terms of Service and Privacy Policy before creating an account."
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

	log.Printf("OAuth callback successful for user ID: %d", user.ID)

	// Handle CLI callback - redirect with token instead of setting cookie
	if cliCallback != "" {
		// Token expires in 24 hours (86400 seconds)
		redirectURL := cliCallback + "?token=" + url.QueryEscape(token) + "&expires_in=86400"
		log.Printf("DEBUG: Redirecting to CLI callback: %s", cliCallback)
		http.Redirect(w, r, redirectURL, http.StatusTemporaryRedirect)
		return
	}

	// Standard web flow - set HTTP-only auth cookie
	cookie := h.config.Session.NewAuthCookie(token, 7*24*time.Hour)
	http.SetCookie(w, &cookie)

	// Redirect to frontend home page
	http.Redirect(w, r, frontendURL, http.StatusTemporaryRedirect)
}

func encodeOAuthSignupConsent(consent services.OAuthSignupConsent) (string, error) {
	data, err := json.Marshal(consent)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(data), nil
}

func decodeOAuthSignupConsent(value string) (*services.OAuthSignupConsent, error) {
	data, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return nil, err
	}
	var consent services.OAuthSignupConsent
	if err := json.Unmarshal(data, &consent); err != nil {
		return nil, err
	}
	return &consent, nil
}
