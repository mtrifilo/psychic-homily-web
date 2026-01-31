package handlers

import (
	"log"
	"net/http"
	"net/url"
	"time"

	"psychic-homily-backend/internal/config"
	"psychic-homily-backend/internal/services"

	"github.com/go-chi/chi/v5"
	"github.com/markbates/goth/gothic"
)

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
	// Get provider from path parameter
	provider := chi.URLParam(r, "provider")
	if provider == "" {
		provider = "google" // fallback
	}

	log.Printf("DEBUG: Using provider '%s' from URL path", provider)

	// Add provider to query parameters for Goth (following best practices)
	q := r.URL.Query()
	q.Add("provider", provider)
	r.URL.RawQuery = q.Encode()

	// Get frontend URL for redirects
	frontendURL := h.config.Email.FrontendURL
	if frontendURL == "" {
		frontendURL = "http://localhost:3000"
	}

	// Use AuthService to handle the complete OAuth flow
	user, token, err := h.authService.OAuthCallback(w, r, provider)
	if err != nil {
		log.Printf("OAuth callback failed: %v", err)
		// Redirect to frontend auth page with error
		redirectURL := frontendURL + "/auth?error=" + url.QueryEscape(err.Error())
		http.Redirect(w, r, redirectURL, http.StatusTemporaryRedirect)
		return
	}

	log.Printf("OAuth callback successful for user ID: %d", user.ID)

	// Set HTTP-only cookie (same pattern as login/passkey handlers)
	http.SetCookie(w, &http.Cookie{
		Name:     "auth_token",
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   h.config.Session.Secure,
		SameSite: h.config.Session.GetSameSite(),
		Expires:  time.Now().Add(7 * 24 * time.Hour), // 7 days
	})

	// Redirect to frontend home page
	http.Redirect(w, r, frontendURL, http.StatusTemporaryRedirect)
}
