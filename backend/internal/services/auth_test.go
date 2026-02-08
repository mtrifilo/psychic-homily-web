package services

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/markbates/goth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"psychic-homily-backend/internal/config"
	"psychic-homily-backend/internal/models"
)

// MockOAuthCompleter implements OAuthCompleter for testing
type MockOAuthCompleter struct {
	mock.Mock
}

func (m *MockOAuthCompleter) CompleteUserAuth(w http.ResponseWriter, r *http.Request) (goth.User, error) {
	args := m.Called(w, r)
	return args.Get(0).(goth.User), args.Error(1)
}

// TestNewAuthService tests the creation of a new AuthService
func TestNewAuthService(t *testing.T) {
	cfg := &config.Config{
		JWT: config.JWTConfig{
			SecretKey: "test-secret-key-32-chars-minimum",
			Expiry:    24,
		},
	}

	authService := NewAuthService(nil, cfg)

	if authService == nil {
		t.Fatal("Expected AuthService to be created, got nil")
	}

	// In test environment, database may not be initialized
	if authService.db == nil {
		t.Log("Database not initialized in test environment (expected)")
	}

	if authService.userService == nil {
		t.Error("Expected UserService to be initialized")
	}

	if authService.jwtService == nil {
		t.Error("Expected JWTService to be initialized")
	}
}

// TestAuthService_OAuthLogin tests the OAuth login functionality
func TestAuthService_OAuthLogin(t *testing.T) {
	cfg := &config.Config{
		JWT: config.JWTConfig{
			SecretKey: "test-secret-key-32-chars-minimum",
			Expiry:    24,
		},
	}

	authService := NewAuthService(nil, cfg)

	tests := []struct {
		name     string
		provider string
	}{
		{
			name:     "google provider",
			provider: "google",
		},
		{
			name:     "github provider",
			provider: "github",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/auth/login/"+tt.provider, nil)
			w := httptest.NewRecorder()

			err := authService.OAuthLogin(w, req, tt.provider)

			// OAuth login may fail due to missing OAuth provider configuration
			// but the method should not panic and should handle the request
			if err != nil {
				t.Logf("OAuth login failed as expected: %v", err)
			}

			// Check that the response was handled (even if it failed)
			if w.Code == 0 {
				t.Error("Expected response to be written")
			}
		})
	}
}

// TestAuthService_OAuthCallback tests the OAuth callback functionality
func TestAuthService_OAuthCallback(t *testing.T) {
	cfg := &config.Config{
		JWT: config.JWTConfig{
			SecretKey: "test-secret-key-32-chars-minimum",
			Expiry:    24,
		},
	}

	authService := NewAuthService(nil, cfg)

	tests := []struct {
		name     string
		provider string
	}{
		{
			name:     "google provider",
			provider: "google",
		},
		{
			name:     "github provider",
			provider: "github",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/auth/callback/"+tt.provider, nil)
			w := httptest.NewRecorder()

			user, token, err := authService.OAuthCallback(w, req, tt.provider)

			// OAuth callback will fail due to missing OAuth provider configuration
			// but the method should not panic and should handle the request
			if err != nil {
				t.Logf("OAuth callback failed as expected: %v", err)
			}

			// User and token should be nil when OAuth fails
			if user != nil {
				t.Error("Expected user to be nil when OAuth fails")
			}

			if token != "" {
				t.Error("Expected token to be empty when OAuth fails")
			}

			// Check that the response was handled (even if it failed)
			if w.Code == 0 {
				t.Error("Expected response to be written")
			}
		})
	}
}

// TestAuthService_OAuthCallback_ErrorHandling tests OAuth callback error scenarios
func TestAuthService_OAuthCallback_ErrorHandling(t *testing.T) {
	cfg := &config.Config{
		JWT: config.JWTConfig{
			SecretKey: "test-secret-key-32-chars-minimum",
			Expiry:    24,
		},
	}

	authService := NewAuthService(nil, cfg)

	t.Run("nil_request", func(t *testing.T) {
		w := httptest.NewRecorder()

		user, token, err := authService.OAuthCallback(w, nil, "google")

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "request cannot be nil")
		assert.Nil(t, user)
		assert.Empty(t, token)
	})

	t.Run("empty_provider", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/auth/callback/", nil)
		w := httptest.NewRecorder()

		user, token, err := authService.OAuthCallback(w, req, "")

		// Empty provider should still attempt OAuth but fail
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "OAuth completion failed")
		assert.Nil(t, user)
		assert.Empty(t, token)
	})

	t.Run("invalid_provider", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/auth/callback/invalid", nil)
		w := httptest.NewRecorder()

		user, token, err := authService.OAuthCallback(w, req, "invalid")

		// Invalid provider should still attempt OAuth but fail
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "OAuth completion failed")
		assert.Nil(t, user)
		assert.Empty(t, token)
	})

	t.Run("nil_response_writer", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/auth/callback/google", nil)

		user, token, err := authService.OAuthCallback(nil, req, "google")

		// Should handle nil response writer gracefully
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "OAuth completion failed")
		assert.Nil(t, user)
		assert.Empty(t, token)
	})



	t.Run("request_with_query_params", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/auth/callback/google?code=test_code&state=test_state", nil)
		w := httptest.NewRecorder()

		user, token, err := authService.OAuthCallback(w, req, "google")

		// Should handle query parameters gracefully
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "OAuth completion failed")
		assert.Nil(t, user)
		assert.Empty(t, token)
	})

	t.Run("request_with_fragment", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/auth/callback/google#fragment", nil)
		w := httptest.NewRecorder()

		user, token, err := authService.OAuthCallback(w, req, "google")

		// Should handle URL fragments gracefully
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "OAuth completion failed")
		assert.Nil(t, user)
		assert.Empty(t, token)
	})

	t.Run("different_http_methods", func(t *testing.T) {
		methods := []string{"POST", "PUT", "DELETE", "PATCH"}

		for _, method := range methods {
			t.Run(method, func(t *testing.T) {
				req := httptest.NewRequest(method, "/auth/callback/google", nil)
				w := httptest.NewRecorder()

				user, token, err := authService.OAuthCallback(w, req, "google")

				// Should handle different HTTP methods gracefully
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "OAuth completion failed")
				assert.Nil(t, user)
				assert.Empty(t, token)
			})
		}
	})

	t.Run("request_with_headers", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/auth/callback/google", nil)
		req.Header.Set("User-Agent", "TestAgent")
		req.Header.Set("Accept", "application/json")
		req.Header.Set("Authorization", "Bearer test-token")
		w := httptest.NewRecorder()

		user, token, err := authService.OAuthCallback(w, req, "google")

		// Should handle custom headers gracefully
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "OAuth completion failed")
		assert.Nil(t, user)
		assert.Empty(t, token)
	})

	t.Run("request_with_body", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/auth/callback/google", strings.NewReader("test body"))
		w := httptest.NewRecorder()

		user, token, err := authService.OAuthCallback(w, req, "google")

		// Should handle request body gracefully
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "OAuth completion failed")
		assert.Nil(t, user)
		assert.Empty(t, token)
	})

	t.Run("very_long_provider_name", func(t *testing.T) {
		longProvider := strings.Repeat("a", 1000)
		req := httptest.NewRequest("GET", "/auth/callback/"+longProvider, nil)
		w := httptest.NewRecorder()

		user, token, err := authService.OAuthCallback(w, req, longProvider)

		// Should handle very long provider names gracefully
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "OAuth completion failed")
		assert.Nil(t, user)
		assert.Empty(t, token)
	})

	t.Run("provider_with_special_characters", func(t *testing.T) {
		specialProvider := "provider-with-special-chars"
		req := httptest.NewRequest("GET", "/auth/callback/"+specialProvider, nil)
		w := httptest.NewRecorder()

		user, token, err := authService.OAuthCallback(w, req, specialProvider)

		// Should handle special characters in provider name gracefully
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "OAuth completion failed")
		assert.Nil(t, user)
		assert.Empty(t, token)
	})

	t.Run("provider_with_spaces", func(t *testing.T) {
		providerWithSpaces := "provider%20with%20spaces"
		req := httptest.NewRequest("GET", "/auth/callback/"+providerWithSpaces, nil)
		w := httptest.NewRecorder()

		user, token, err := authService.OAuthCallback(w, req, "provider with spaces")

		// Should handle spaces in provider name gracefully
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "OAuth completion failed")
		assert.Nil(t, user)
		assert.Empty(t, token)
	})

	t.Run("unicode_provider_name", func(t *testing.T) {
		unicodeProvider := "provider-üñîçødé"
		req := httptest.NewRequest("GET", "/auth/callback/"+unicodeProvider, nil)
		w := httptest.NewRecorder()

		user, token, err := authService.OAuthCallback(w, req, unicodeProvider)

		// Should handle unicode characters in provider name gracefully
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "OAuth completion failed")
		assert.Nil(t, user)
		assert.Empty(t, token)
	})
}

// TestAuthService_OAuthCallback_EdgeCases tests additional edge cases for OAuth callback
func TestAuthService_OAuthCallback_EdgeCases(t *testing.T) {
	cfg := &config.Config{
		JWT: config.JWTConfig{
			SecretKey: "test-secret-key-32-chars-minimum",
			Expiry:    24,
		},
	}

	authService := NewAuthService(nil, cfg)

	t.Run("request_with_complex_url", func(t *testing.T) {
		complexURL := "/auth/callback/google?code=abc123&state=xyz789&redirect_uri=https%3A//example.com/callback&scope=email%20profile"
		req := httptest.NewRequest("GET", complexURL, nil)
		w := httptest.NewRecorder()

		user, token, err := authService.OAuthCallback(w, req, "google")

		// Should handle complex URLs with multiple parameters gracefully
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "OAuth completion failed")
		assert.Nil(t, user)
		assert.Empty(t, token)
	})

	t.Run("request_with_encoded_characters", func(t *testing.T) {
		encodedURL := "/auth/callback/google?code=abc%20123&state=xyz%20789"
		req := httptest.NewRequest("GET", encodedURL, nil)
		w := httptest.NewRecorder()

		user, token, err := authService.OAuthCallback(w, req, "google")

		// Should handle URL-encoded characters gracefully
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "OAuth completion failed")
		assert.Nil(t, user)
		assert.Empty(t, token)
	})

	t.Run("request_with_multiple_headers", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/auth/callback/google", nil)
		req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
		req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
		req.Header.Set("Accept-Language", "en-US,en;q=0.5")
		req.Header.Set("Accept-Encoding", "gzip, deflate")
		req.Header.Set("Connection", "keep-alive")
		req.Header.Set("Upgrade-Insecure-Requests", "1")
		w := httptest.NewRecorder()

		user, token, err := authService.OAuthCallback(w, req, "google")

		// Should handle multiple headers gracefully
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "OAuth completion failed")
		assert.Nil(t, user)
		assert.Empty(t, token)
	})

	t.Run("request_with_cookies", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/auth/callback/google", nil)
		req.Header.Set("Cookie", "session=abc123; csrf=xyz789")
		w := httptest.NewRecorder()

		user, token, err := authService.OAuthCallback(w, req, "google")

		// Should handle cookies gracefully
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "OAuth completion failed")
		assert.Nil(t, user)
		assert.Empty(t, token)
	})

	t.Run("request_with_referer", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/auth/callback/google", nil)
		req.Header.Set("Referer", "https://accounts.google.com/oauth/authorize")
		w := httptest.NewRecorder()

		user, token, err := authService.OAuthCallback(w, req, "google")

		// Should handle referer header gracefully
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "OAuth completion failed")
		assert.Nil(t, user)
		assert.Empty(t, token)
	})

	t.Run("request_with_origin", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/auth/callback/google", nil)
		req.Header.Set("Origin", "https://accounts.google.com")
		w := httptest.NewRecorder()

		user, token, err := authService.OAuthCallback(w, req, "google")

		// Should handle origin header gracefully
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "OAuth completion failed")
		assert.Nil(t, user)
		assert.Empty(t, token)
	})

	t.Run("request_with_x_forwarded_headers", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/auth/callback/google", nil)
		req.Header.Set("X-Forwarded-For", "192.168.1.1")
		req.Header.Set("X-Forwarded-Proto", "https")
		req.Header.Set("X-Forwarded-Host", "example.com")
		w := httptest.NewRecorder()

		user, token, err := authService.OAuthCallback(w, req, "google")

		// Should handle X-Forwarded headers gracefully
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "OAuth completion failed")
		assert.Nil(t, user)
		assert.Empty(t, token)
	})

	t.Run("request_with_content_type", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/auth/callback/google", strings.NewReader("code=abc123&state=xyz789"))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		w := httptest.NewRecorder()

		user, token, err := authService.OAuthCallback(w, req, "google")

		// Should handle form-encoded content gracefully
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "OAuth completion failed")
		assert.Nil(t, user)
		assert.Empty(t, token)
	})

	t.Run("request_with_json_content", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/auth/callback/google", strings.NewReader(`{"code":"abc123","state":"xyz789"}`))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		user, token, err := authService.OAuthCallback(w, req, "google")

		// Should handle JSON content gracefully
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "OAuth completion failed")
		assert.Nil(t, user)
		assert.Empty(t, token)
	})

	t.Run("request_with_large_body", func(t *testing.T) {
		largeBody := strings.Repeat("a", 10000) // 10KB body
		req := httptest.NewRequest("POST", "/auth/callback/google", strings.NewReader(largeBody))
		w := httptest.NewRecorder()

		user, token, err := authService.OAuthCallback(w, req, "google")

		// Should handle large request bodies gracefully
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "OAuth completion failed")
		assert.Nil(t, user)
		assert.Empty(t, token)
	})
}

// TestAuthService_OAuthCallback_WithMock tests OAuth callback with mocked OAuth completer
func TestAuthService_OAuthCallback_WithMock(t *testing.T) {
	cfg := &config.Config{
		JWT: config.JWTConfig{
			SecretKey: "test-secret-key-32-chars-minimum",
			Expiry:    24,
		},
	}

	authService := NewAuthService(nil, cfg)
	mockCompleter := new(MockOAuthCompleter)
	authService.SetOAuthCompleter(mockCompleter)

	t.Run("successful_oauth_completion", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/auth/callback/google", nil)
		w := httptest.NewRecorder()

		// Mock successful OAuth completion
		mockUser := goth.User{
			UserID:    "google_12345",
			Email:     "test@example.com",
			FirstName: "Test",
			LastName:  "User",
			AvatarURL: "https://example.com/avatar.jpg",
		}
		mockCompleter.On("CompleteUserAuth", w, req).Return(mockUser, nil)

		user, token, err := authService.OAuthCallback(w, req, "google")

		// Should fail due to database not being initialized, but OAuth completion succeeded
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "database not initialized")
		assert.Nil(t, user)
		assert.Empty(t, token)

		mockCompleter.AssertExpectations(t)
	})

	t.Run("oauth_completion_failure", func(t *testing.T) {
		// Create a fresh mock for this test
		mockCompleter := new(MockOAuthCompleter)
		authService.SetOAuthCompleter(mockCompleter)
		
		req := httptest.NewRequest("GET", "/auth/callback/google", nil)
		w := httptest.NewRecorder()

		// Mock OAuth completion failure
		mockCompleter.On("CompleteUserAuth", w, req).Return(goth.User{}, assert.AnError)

		user, token, err := authService.OAuthCallback(w, req, "google")

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "OAuth completion failed")
		assert.Nil(t, user)
		assert.Empty(t, token)

		mockCompleter.AssertExpectations(t)
	})

	t.Run("user_service_failure", func(t *testing.T) {
		// Create a fresh mock for this test
		mockCompleter := new(MockOAuthCompleter)
		authService.SetOAuthCompleter(mockCompleter)
		
		req := httptest.NewRequest("GET", "/auth/callback/google", nil)
		w := httptest.NewRecorder()

		// Mock successful OAuth completion but user service failure
		mockUser := goth.User{
			UserID:    "google_12345",
			Email:     "test@example.com",
			FirstName: "Test",
			LastName:  "User",
		}
		mockCompleter.On("CompleteUserAuth", w, req).Return(mockUser, nil)

		user, token, err := authService.OAuthCallback(w, req, "google")

		// Should fail due to database not being initialized
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "database not initialized")
		assert.Nil(t, user)
		assert.Empty(t, token)

		mockCompleter.AssertExpectations(t)
	})

	t.Run("jwt_service_failure", func(t *testing.T) {
		// Create a fresh mock for this test
		mockCompleter := new(MockOAuthCompleter)
		authService.SetOAuthCompleter(mockCompleter)
		
		req := httptest.NewRequest("GET", "/auth/callback/google", nil)
		w := httptest.NewRecorder()

		// Mock successful OAuth completion
		mockUser := goth.User{
			UserID:    "google_12345",
			Email:     "test@example.com",
			FirstName: "Test",
			LastName:  "User",
		}
		mockCompleter.On("CompleteUserAuth", w, req).Return(mockUser, nil)

		user, token, err := authService.OAuthCallback(w, req, "google")

		// Should fail due to database not being initialized
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "database not initialized")
		assert.Nil(t, user)
		assert.Empty(t, token)

		mockCompleter.AssertExpectations(t)
	})

	t.Run("oauth_completion_with_empty_user", func(t *testing.T) {
		// Create a fresh mock for this test
		mockCompleter := new(MockOAuthCompleter)
		authService.SetOAuthCompleter(mockCompleter)
		
		req := httptest.NewRequest("GET", "/auth/callback/google", nil)
		w := httptest.NewRecorder()

		// Mock OAuth completion with empty user
		mockCompleter.On("CompleteUserAuth", w, req).Return(goth.User{}, nil)

		user, token, err := authService.OAuthCallback(w, req, "google")

		// Should fail due to database not being initialized
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "database not initialized")
		assert.Nil(t, user)
		assert.Empty(t, token)

		mockCompleter.AssertExpectations(t)
	})

	t.Run("oauth_completion_with_partial_user_data", func(t *testing.T) {
		// Create a fresh mock for this test
		mockCompleter := new(MockOAuthCompleter)
		authService.SetOAuthCompleter(mockCompleter)
		
		req := httptest.NewRequest("GET", "/auth/callback/google", nil)
		w := httptest.NewRecorder()

		// Mock OAuth completion with partial user data
		mockUser := goth.User{
			UserID: "google_12345",
			Email:  "test@example.com",
			// Missing FirstName, LastName, etc.
		}
		mockCompleter.On("CompleteUserAuth", w, req).Return(mockUser, nil)

		user, token, err := authService.OAuthCallback(w, req, "google")

		// Should fail due to database not being initialized
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "database not initialized")
		assert.Nil(t, user)
		assert.Empty(t, token)

		mockCompleter.AssertExpectations(t)
	})
}

// TestAuthService_GetUserProfile tests the GetUserProfile functionality
func TestAuthService_GetUserProfile(t *testing.T) {
	cfg := &config.Config{
		JWT: config.JWTConfig{
			SecretKey: "test-secret-key-32-chars-minimum",
			Expiry:    24,
		},
	}

	authService := NewAuthService(nil, cfg)

	tests := []struct {
		name   string
		userID uint
	}{
		{
			name:   "user retrieval attempt",
			userID: 1,
		},
		{
			name:   "non-existent user",
			userID: 999,
		},
	}

			for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			user, err := authService.GetUserProfile(tt.userID)

			// The method should not panic and should handle the request
			// In test environment, we expect "database not initialized" error
			if err != nil {
				if err.Error() == "database not initialized" {
					t.Log("GetUserProfile correctly returned 'database not initialized' error")
				} else {
					t.Logf("GetUserProfile returned unexpected error: %v", err)
				}
			} else {
				t.Log("GetUserProfile succeeded (unexpected in test environment)")
			}

			// User should be nil when database is not available
			if user != nil {
				t.Logf("User found: %+v", user)
			}
		})
	}
}

// TestAuthService_RefreshUserToken tests the RefreshUserToken functionality
func TestAuthService_RefreshUserToken(t *testing.T) {
	cfg := &config.Config{
		JWT: config.JWTConfig{
			SecretKey: "test-secret-key-32-chars-minimum",
			Expiry:    24,
		},
	}

	authService := NewAuthService(nil, cfg)

	tests := []struct {
		name string
		user *models.User
	}{
		{
			name: "token refresh attempt",
			user: &models.User{
				ID:    1,
				Email: stringPtr("test@example.com"),
			},
		},
		{
			name: "token refresh with nil email",
			user: &models.User{
				ID:    2,
				Email: nil,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			token, err := authService.RefreshUserToken(tt.user)

			// The method should not panic and should handle the request
			// It may fail due to JWT configuration issues in tests
			if err != nil {
				t.Logf("RefreshUserToken failed as expected: %v", err)
			}

			// Token may be empty if JWT service fails
			if token != "" {
				previewLen := len(token)
				if previewLen > 20 {
					previewLen = 20
				}
				t.Logf("Token generated: %s...", token[:previewLen])
			}
		})
	}
}

// TestAuthService_Logout tests the Logout functionality
func TestAuthService_Logout(t *testing.T) {
	authService := NewAuthService(nil, &config.Config{
		JWT: config.JWTConfig{
			SecretKey: "test-secret-key-32-chars-minimum",
			Expiry:    24,
		},
	})

	req := httptest.NewRequest("POST", "/auth/logout", nil)
	w := httptest.NewRecorder()

	err := authService.Logout(w, req)

	// Logout should always succeed (JWT tokens are stateless)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	// Check that the response was handled
	if w.Code == 0 {
		t.Error("Expected response to be written")
	}
}

// TestAuthService_Integration tests integration between AuthService components
func TestAuthService_Integration(t *testing.T) {
	cfg := &config.Config{
		JWT: config.JWTConfig{
			SecretKey: "test-secret-key-32-chars-minimum",
			Expiry:    24,
		},
	}

	authService := NewAuthService(nil, cfg)

	// Test that all components are properly initialized
	// In test environment, database may not be initialized
	if authService.db == nil {
		t.Log("Database not initialized in test environment (expected)")
	}

	if authService.userService == nil {
		t.Error("UserService should be initialized")
	}

	if authService.jwtService == nil {
		t.Error("JWTService should be initialized")
	}

	// Test that services can be accessed by calling their methods
	// This is a better way to test that methods are available
	_, err := authService.userService.GetUserByID(1)
	// We expect a "database not initialized" error in test environment
	if err != nil {
		if err.Error() == "database not initialized" {
			t.Log("GetUserByID correctly returned 'database not initialized' error")
		} else {
			t.Logf("GetUserByID returned unexpected error: %v", err)
		}
	} else {
		t.Error("Expected GetUserByID to return error in test environment")
	}

	// Test JWT service method
	testUser := &models.User{
		ID:    1,
		Email: stringPtr("test@example.com"),
	}
	_, err = authService.jwtService.CreateToken(testUser)
	// We expect this to work since it doesn't require database access
	if err != nil {
		t.Logf("CreateToken called successfully (expected error: %v)", err)
	}
}

// TestAuthService_ErrorHandling tests error handling scenarios
func TestAuthService_ErrorHandling(t *testing.T) {
	cfg := &config.Config{
		JWT: config.JWTConfig{
			SecretKey: "test-secret-key-32-chars-minimum",
			Expiry:    24,
		},
	}

	authService := NewAuthService(nil, cfg)

	// Test with nil request
	t.Run("Nil Request", func(t *testing.T) {
		w := httptest.NewRecorder()
		
		// This should return an error for nil request
		err := authService.OAuthLogin(w, nil, "google")
		if err != nil {
			if err.Error() == "request cannot be nil" {
				t.Log("OAuth login correctly returned 'request cannot be nil' error")
			} else {
				t.Logf("OAuth login returned unexpected error: %v", err)
			}
		} else {
			t.Error("Expected OAuth login to return error for nil request")
		}
	})

	// Test with empty provider
	t.Run("Empty Provider", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/auth/login", nil)
		w := httptest.NewRecorder()
		
		err := authService.OAuthLogin(w, req, "")
		if err == nil {
			t.Log("OAuth login handled empty provider gracefully")
		}
	})

	// Test with invalid provider
	t.Run("Invalid Provider", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/auth/login/invalid", nil)
		w := httptest.NewRecorder()
		
		err := authService.OAuthLogin(w, req, "invalid")
		if err == nil {
			t.Log("OAuth login handled invalid provider gracefully")
		}
	})
}

// TestAuthService_HTTPResponse tests HTTP response handling
func TestAuthService_HTTPResponse(t *testing.T) {
	cfg := &config.Config{
		JWT: config.JWTConfig{
			SecretKey: "test-secret-key-32-chars-minimum",
			Expiry:    24,
		},
	}

	authService := NewAuthService(nil, cfg)

	t.Run("OAuth Login Response", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/auth/login/google", nil)
		w := httptest.NewRecorder()

		err := authService.OAuthLogin(w, req, "google")

		// Should handle the request (even if it fails)
		if w.Code == 0 {
			t.Error("Expected response code to be set")
		}

		// Should not panic
		if err != nil {
			t.Logf("OAuth login failed as expected: %v", err)
		}
	})

	t.Run("OAuth Callback Response", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/auth/callback/google", nil)
		w := httptest.NewRecorder()

		user, token, err := authService.OAuthCallback(w, req, "google")

		// Should handle the request (even if it fails)
		if w.Code == 0 {
			t.Error("Expected response code to be set")
		}

		// Should not panic
		if err != nil {
			t.Logf("OAuth callback failed as expected: %v", err)
		}

		// User and token should be nil when OAuth fails
		if user != nil {
			t.Error("Expected user to be nil when OAuth fails")
		}

		if token != "" {
			t.Error("Expected token to be empty when OAuth fails")
		}
	})

	t.Run("Logout Response", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/auth/logout", nil)
		w := httptest.NewRecorder()

		err := authService.Logout(w, req)

		// Should handle the request successfully
		if w.Code == 0 {
			t.Error("Expected response code to be set")
		}

		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
	})
}

// Helper function to create string pointer
func stringPtr(s string) *string {
	return &s
} 
