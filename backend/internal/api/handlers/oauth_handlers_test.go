package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"psychic-homily-backend/internal/models"

	"github.com/go-chi/chi/v5"
)

// AuthServiceInterface defines the interface for AuthService
type AuthServiceInterface interface {
	OAuthCallback(w http.ResponseWriter, r *http.Request, provider string) (*models.User, string, error)
}

// MockAuthService is a mock implementation of AuthService for testing
type MockAuthService struct {
	callbackUser  *models.User
	callbackToken string
	callbackError error
}

func (m *MockAuthService) OAuthCallback(w http.ResponseWriter, r *http.Request, provider string) (*models.User, string, error) {
	return m.callbackUser, m.callbackToken, m.callbackError
}

// TestOAuthHTTPHandler is a test version of OAuthHTTPHandler that uses the interface
type TestOAuthHTTPHandler struct {
	authService AuthServiceInterface
}

func NewTestOAuthHTTPHandler(authService AuthServiceInterface) *TestOAuthHTTPHandler {
	return &TestOAuthHTTPHandler{
		authService: authService,
	}
}

func (h *TestOAuthHTTPHandler) OAuthLoginHTTPHandler(w http.ResponseWriter, r *http.Request) {
	// Get provider from path parameter using chi
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

	// In the real implementation, this would call gothic.BeginAuthHandler(w, r)
	// For testing, we'll just return success
	w.WriteHeader(http.StatusOK)
}

func (h *TestOAuthHTTPHandler) OAuthCallbackHTTPHandler(w http.ResponseWriter, r *http.Request) {
	// Get provider from path parameter
	provider := chi.URLParam(r, "provider")
	if provider == "" {
		provider = "google" // fallback
	}

	// Add provider to query parameters for Goth (following best practices)
	q := r.URL.Query()
	q.Add("provider", provider)
	r.URL.RawQuery = q.Encode()

	// Use AuthService to handle the complete OAuth flow
	user, token, err := h.authService.OAuthCallback(w, r, provider)
	if err != nil {
		// Redirect to frontend with error
		http.Redirect(w, r, "/login?error="+err.Error(), http.StatusTemporaryRedirect)
		return
	}

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

func TestNewOAuthHTTPHandler(t *testing.T) {
	authService := &MockAuthService{}
	handler := NewTestOAuthHTTPHandler(authService)

	if handler == nil {
		t.Fatal("Expected handler to be created, got nil")
	}

	if handler.authService != authService {
		t.Error("Expected authService to be set correctly")
	}
}

func TestOAuthLoginHTTPHandler_ValidProvider(t *testing.T) {
	tests := []struct {
		name           string
		provider       string
		expectedStatus int
	}{
		{
			name:           "google provider success",
			provider:       "google",
			expectedStatus: http.StatusOK,
		},
		{
			name:           "github provider success",
			provider:       "github",
			expectedStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock auth service
			mockAuth := &MockAuthService{}

			// Create handler
			handler := NewTestOAuthHTTPHandler(mockAuth)

			// Create request with chi context
			req := httptest.NewRequest("GET", "/auth/login/"+tt.provider, nil)
			w := httptest.NewRecorder()

			// Set up chi context with URL parameters
			rctx := chi.NewRouteContext()
			rctx.URLParams.Add("provider", tt.provider)
			req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

			// Call handler
			handler.OAuthLoginHTTPHandler(w, req)

			// Check status code
			if w.Code != tt.expectedStatus {
				t.Errorf("Expected status %d, got %d", tt.expectedStatus, w.Code)
			}
		})
	}
}

func TestOAuthLoginHTTPHandler_InvalidProvider(t *testing.T) {
	invalidProviders := []string{"facebook", "twitter", "linkedin", "invalid"}

	for _, provider := range invalidProviders {
		t.Run("invalid provider: "+provider, func(t *testing.T) {
			mockAuth := &MockAuthService{}
			handler := NewTestOAuthHTTPHandler(mockAuth)

			req := httptest.NewRequest("GET", "/auth/login/"+provider, nil)
			w := httptest.NewRecorder()

			// Set up chi context with URL parameters
			rctx := chi.NewRouteContext()
			rctx.URLParams.Add("provider", provider)
			req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

			handler.OAuthLoginHTTPHandler(w, req)

			if w.Code != http.StatusBadRequest {
				t.Errorf("Expected status %d for provider %s, got %d", http.StatusBadRequest, provider, w.Code)
			}

			if !strings.Contains(w.Body.String(), "Invalid provider") {
				t.Errorf("Expected 'Invalid provider' error for provider %s", provider)
			}
		})
	}
}

func TestOAuthLoginHTTPHandler_NoProvider(t *testing.T) {
	mockAuth := &MockAuthService{}
	handler := NewTestOAuthHTTPHandler(mockAuth)

	// Test with no provider in URL
	req := httptest.NewRequest("GET", "/auth/login", nil)
	w := httptest.NewRecorder()

	handler.OAuthLoginHTTPHandler(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status %d, got %d", http.StatusBadRequest, w.Code)
	}

	if !strings.Contains(w.Body.String(), "Provider required") {
		t.Errorf("Expected 'Provider required' error")
	}
}

func TestOAuthCallbackHTTPHandler_Success(t *testing.T) {
	email := "user@example.com"
	emptyEmail := ""
	tests := []struct {
		name           string
		provider       string
		user           *models.User
		token          string
		expectedStatus int
		expectedEmail  string
	}{
		{
			name:     "user with email",
			provider: "google",
			user: &models.User{
				ID:    1,
				Email: &email,
			},
			token:          "jwt-token-123",
			expectedStatus: http.StatusOK,
			expectedEmail:  "user@example.com",
		},
		{
			name:     "user without email",
			provider: "github",
			user: &models.User{
				ID:    2,
				Email: nil,
			},
			token:          "jwt-token-456",
			expectedStatus: http.StatusOK,
			expectedEmail:  "",
		},
		{
			name:     "user with empty email",
			provider: "google",
			user: &models.User{
				ID:    3,
				Email: &emptyEmail,
			},
			token:          "jwt-token-789",
			expectedStatus: http.StatusOK,
			expectedEmail:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockAuth := &MockAuthService{
				callbackUser:  tt.user,
				callbackToken: tt.token,
				callbackError: nil,
			}

			handler := NewTestOAuthHTTPHandler(mockAuth)

			req := httptest.NewRequest("GET", "/auth/callback/"+tt.provider, nil)
			w := httptest.NewRecorder()

			// Set up chi context with URL parameters
			rctx := chi.NewRouteContext()
			rctx.URLParams.Add("provider", tt.provider)
			req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

			handler.OAuthCallbackHTTPHandler(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("Expected status %d, got %d", tt.expectedStatus, w.Code)
			}

			// Check content type
			contentType := w.Header().Get("Content-Type")
			if contentType != "application/json" {
				t.Errorf("Expected Content-Type application/json, got %s", contentType)
			}

			// Parse response
			var response map[string]interface{}
			if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
				t.Fatalf("Failed to parse JSON response: %v", err)
			}

			// Check success field
			if success, ok := response["success"].(bool); !ok || !success {
				t.Error("Expected success to be true")
			}

			// Check token field
			if token, ok := response["token"].(string); !ok || token != tt.token {
				t.Errorf("Expected token %s, got %s", tt.token, token)
			}

			// Check user object
			if userObj, ok := response["user"].(map[string]interface{}); ok {
				if id, ok := userObj["id"].(float64); !ok || int(id) != int(tt.user.ID) {
					t.Errorf("Expected user ID %d, got %v", tt.user.ID, id)
				}

				if email, ok := userObj["email"].(string); !ok || email != tt.expectedEmail {
					t.Errorf("Expected email %s, got %s", tt.expectedEmail, email)
				}
			} else {
				t.Error("Expected user object in response")
			}
		})
	}
}

func TestOAuthCallbackHTTPHandler_Error(t *testing.T) {
	tests := []struct {
		name             string
		provider         string
		error            error
		expectedStatus   int
		expectedRedirect bool
	}{
		{
			name:             "OAuth callback error",
			provider:         "google",
			error:            fmt.Errorf("OAuth provider not configured"),
			expectedStatus:   http.StatusTemporaryRedirect,
			expectedRedirect: true,
		},
		{
			name:             "database error",
			provider:         "github",
			error:            fmt.Errorf("Database connection failed"),
			expectedStatus:   http.StatusTemporaryRedirect,
			expectedRedirect: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockAuth := &MockAuthService{
				callbackUser:  nil,
				callbackToken: "",
				callbackError: tt.error,
			}

			handler := NewTestOAuthHTTPHandler(mockAuth)

			req := httptest.NewRequest("GET", "/auth/callback/"+tt.provider, nil)
			w := httptest.NewRecorder()

			// Set up chi context with URL parameters
			rctx := chi.NewRouteContext()
			rctx.URLParams.Add("provider", tt.provider)
			req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

			handler.OAuthCallbackHTTPHandler(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("Expected status %d, got %d", tt.expectedStatus, w.Code)
			}

			if tt.expectedRedirect {
				location := w.Header().Get("Location")
				if location == "" {
					t.Error("Expected redirect location header")
				}
				if !strings.Contains(location, "/login?error=") {
					t.Errorf("Expected redirect to login with error, got %s", location)
				}
			}
		})
	}
}

func TestOAuthCallbackHTTPHandler_NoProvider(t *testing.T) {
	email := "user@example.com"
	mockAuth := &MockAuthService{
		callbackUser: &models.User{
			ID:    1,
			Email: &email,
		},
		callbackToken: "jwt-token-123",
		callbackError: nil,
	}

	handler := NewTestOAuthHTTPHandler(mockAuth)

	// Test with no provider in URL (should fallback to "google")
	req := httptest.NewRequest("GET", "/auth/callback", nil)
	w := httptest.NewRecorder()

	handler.OAuthCallbackHTTPHandler(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}

	// Check content type
	contentType := w.Header().Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("Expected Content-Type application/json, got %s", contentType)
	}
}
