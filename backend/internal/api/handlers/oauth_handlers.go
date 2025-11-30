package handlers

import (
	"fmt"
	"log"
	"net/http"

	"psychic-homily-backend/internal/services"

	"github.com/go-chi/chi/v5"
	"github.com/markbates/goth/gothic"
)

// OAuthHTTPHandler handles OAuth HTTP requests directly
type OAuthHTTPHandler struct {
	authService *services.AuthService
}

// NewOAuthHTTPHandler creates a new OAuth HTTP handler
func NewOAuthHTTPHandler(authService *services.AuthService) *OAuthHTTPHandler {
	return &OAuthHTTPHandler{
		authService: authService,
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

    // Use AuthService to handle the complete OAuth flow
    user, token, err := h.authService.OAuthCallback(w, r, provider)
    if err != nil {
        log.Printf("OAuth callback failed: %v", err)
        http.Redirect(w, r, "/login?error="+err.Error(), http.StatusTemporaryRedirect)
        return
    }

    log.Printf("OAuth callback successful for user: %v", user)

    // Return JWT token to frontend
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(http.StatusOK)

    email := ""
    if user.Email != nil {
        email = *user.Email
    }

    w.Write([]byte(fmt.Sprintf(`{
        "success": true,
        "token": "%s",
        "user": {
            "id": %d,
            "email": "%s"
        }
    }`, token, user.ID, email)))
}
