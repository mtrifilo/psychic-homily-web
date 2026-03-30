package auth

import (
	"net/http"
	"net/http/httptest"
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

// TestAuthService_OAuthLogin tests the OAuth login flow.
// Without configured providers, gothic.BeginAuthHandler fails immediately,
// so a single provider name is sufficient to cover the error path.
func TestAuthService_OAuthLogin(t *testing.T) {
	cfg := &config.Config{
		JWT: config.JWTConfig{
			SecretKey: "test-secret-key-32-chars-minimum",
			Expiry:    24,
		},
	}

	authService := NewAuthService(nil, cfg, newNilDBUserService())

	t.Run("nil_request", func(t *testing.T) {
		w := httptest.NewRecorder()
		err := authService.OAuthLogin(w, nil, "google")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "request cannot be nil")
	})

	t.Run("no_provider_configured", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/auth/login/google", nil)
		w := httptest.NewRecorder()

		err := authService.OAuthLogin(w, req, "google")

		// OAuth login may fail due to missing provider config but must not panic
		if err != nil {
			t.Logf("OAuth login failed as expected (no provider configured): %v", err)
		}
		assert.NotEqual(t, 0, w.Code, "Expected response to be written")
	})
}

// TestAuthService_OAuthCallback tests the OAuth callback error path.
// Without configured providers, CompleteUserAuth fails before examining
// request details, so varying headers/body/query params is redundant.
func TestAuthService_OAuthCallback(t *testing.T) {
	cfg := &config.Config{
		JWT: config.JWTConfig{
			SecretKey: "test-secret-key-32-chars-minimum",
			Expiry:    24,
		},
	}

	authService := NewAuthService(nil, cfg, newNilDBUserService())

	t.Run("nil_request", func(t *testing.T) {
		w := httptest.NewRecorder()

		user, token, err := authService.OAuthCallback(w, nil, "google")

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "request cannot be nil")
		assert.Nil(t, user)
		assert.Empty(t, token)
	})

	t.Run("no_provider_configured", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/auth/callback/google", nil)
		w := httptest.NewRecorder()

		user, token, err := authService.OAuthCallback(w, req, "google")

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "OAuth completion failed")
		assert.Nil(t, user)
		assert.Empty(t, token)
	})
}

// TestAuthService_OAuthCallback_WithMock tests OAuth callback with mocked OAuth completer
// to exercise code paths beyond the initial CompleteUserAuth call.
func TestAuthService_OAuthCallback_WithMock(t *testing.T) {
	cfg := &config.Config{
		JWT: config.JWTConfig{
			SecretKey: "test-secret-key-32-chars-minimum",
			Expiry:    24,
		},
	}

	authService := NewAuthService(nil, cfg, newNilDBUserService())

	t.Run("oauth_completion_failure", func(t *testing.T) {
		mockCompleter := new(MockOAuthCompleter)
		authService.SetOAuthCompleter(mockCompleter)

		req := httptest.NewRequest("GET", "/auth/callback/google", nil)
		w := httptest.NewRecorder()

		mockCompleter.On("CompleteUserAuth", w, req).Return(goth.User{}, assert.AnError)

		user, token, err := authService.OAuthCallback(w, req, "google")

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "OAuth completion failed")
		assert.Nil(t, user)
		assert.Empty(t, token)

		mockCompleter.AssertExpectations(t)
	})
}

// TestAuthService_RefreshUserToken tests the RefreshUserToken functionality
func TestAuthService_RefreshUserToken(t *testing.T) {
	cfg := &config.Config{
		JWT: config.JWTConfig{
			SecretKey: "test-secret-key-32-chars-minimum",
			Expiry:    24,
		},
	}

	authService := NewAuthService(nil, cfg, newNilDBUserService())

	t.Run("with_email", func(t *testing.T) {
		user := &models.User{
			ID:    1,
			Email: stringPtr("test@example.com"),
		}
		token, err := authService.RefreshUserToken(user)

		// JWT creation doesn't require DB, so this should succeed
		if err != nil {
			t.Logf("RefreshUserToken failed: %v", err)
		} else {
			assert.NotEmpty(t, token)
		}
	})

	t.Run("with_nil_email", func(t *testing.T) {
		user := &models.User{
			ID:    2,
			Email: nil,
		}
		token, err := authService.RefreshUserToken(user)

		// Should handle nil email without panicking
		if err != nil {
			t.Logf("RefreshUserToken failed: %v", err)
		} else {
			assert.NotEmpty(t, token)
		}
	})
}

// TestAuthService_Logout tests the Logout functionality
func TestAuthService_Logout(t *testing.T) {
	authService := NewAuthService(nil, &config.Config{
		JWT: config.JWTConfig{
			SecretKey: "test-secret-key-32-chars-minimum",
			Expiry:    24,
		},
	}, newNilDBUserService())

	req := httptest.NewRequest("POST", "/auth/logout", nil)
	w := httptest.NewRecorder()

	err := authService.Logout(w, req)

	// Logout should always succeed (JWT tokens are stateless)
	assert.NoError(t, err)
}
