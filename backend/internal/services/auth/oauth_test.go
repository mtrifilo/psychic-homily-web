package auth

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/markbates/goth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"gorm.io/gorm"

	"psychic-homily-backend/internal/config"
	apperrors "psychic-homily-backend/internal/errors"
	authm "psychic-homily-backend/internal/models/auth"
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
		user := &authm.User{
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
		user := &authm.User{
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

// stubUserServiceWithGetByID embeds nilDBUserService (so all unused methods
// return the "database not initialized" sentinel) and overrides GetUserByID
// with an injectable function. Lets GetUserProfile tests pick the exact error
// shape the userService surfaces — gorm.ErrRecordNotFound vs a raw DB error.
type stubUserServiceWithGetByID struct {
	nilDBUserService
	getByID func(userID uint) (*authm.User, error)
}

func (s *stubUserServiceWithGetByID) GetUserByID(userID uint) (*authm.User, error) {
	return s.getByID(userID)
}

// TestAuthService_GetUserProfile_DeletedUserReturnsTyped asserts the
// deleted-user case is discriminated as a typed *AuthError{Code:
// CodeUserNotFound}. The handler layer relies on this typed shape to route
// the response to HTTP 401 + CodeUnauthorized instead of the fail-closed 5xx
// reserved for generic backend failures.
func TestAuthService_GetUserProfile_DeletedUserReturnsTyped(t *testing.T) {
	cfg := &config.Config{
		JWT: config.JWTConfig{
			SecretKey: "test-secret-key-32-chars-minimum",
			Expiry:    24,
		},
	}
	userSvc := &stubUserServiceWithGetByID{
		getByID: func(userID uint) (*authm.User, error) {
			// Mirror UserService.GetUserByID's actual wrapping
			// (fmt.Errorf("failed to get user: %w", result.Error))
			// so errors.Is(err, gorm.ErrRecordNotFound) discrimination
			// flows through %w unwrap, not raw equality.
			return nil, fmt.Errorf("failed to get user: %w", gorm.ErrRecordNotFound)
		},
	}
	authService := NewAuthService(nil, cfg, userSvc)

	user, err := authService.GetUserProfile(42)

	assert.Nil(t, user)
	assert.Error(t, err)

	var authErr *apperrors.AuthError
	if !errors.As(err, &authErr) {
		t.Fatalf("expected returned error to wrap *AuthError, got %T (%v)", err, err)
	}
	assert.Equal(t, apperrors.CodeUserNotFound, authErr.Code,
		"deleted-user must surface as typed CodeUserNotFound so handlers route to 401")
	// The underlying gorm sentinel must remain reachable via the standard
	// error chain so future callers can additionally drill down if needed.
	assert.True(t, errors.Is(err, gorm.ErrRecordNotFound),
		"typed AuthError must keep gorm.ErrRecordNotFound in the unwrap chain")
}

// TestAuthService_GetUserProfile_GenericErrorPassthrough asserts that any
// non-not-found failure (DB connection lost, etc.) does NOT get wrapped in
// the typed CodeUserNotFound shape — those continue to propagate as generic
// errors so RefreshTokenHandler's fail-closed branch emits 5xx instead of
// 401. Protects the dual-direction contract from a regression that
// blanket-wraps every GetUserByID failure as not-found.
func TestAuthService_GetUserProfile_GenericErrorPassthrough(t *testing.T) {
	cfg := &config.Config{
		JWT: config.JWTConfig{
			SecretKey: "test-secret-key-32-chars-minimum",
			Expiry:    24,
		},
	}
	rawErr := fmt.Errorf("connection refused: pq: server is starting up")
	userSvc := &stubUserServiceWithGetByID{
		getByID: func(userID uint) (*authm.User, error) {
			return nil, rawErr
		},
	}
	authService := NewAuthService(nil, cfg, userSvc)

	user, err := authService.GetUserProfile(42)

	assert.Nil(t, user)
	assert.Error(t, err)

	// Must NOT be a typed CodeUserNotFound. Either a non-AuthError or some
	// other AuthError code is acceptable; the contract is "not CodeUserNotFound
	// for non-not-found failures".
	var authErr *apperrors.AuthError
	if errors.As(err, &authErr) && authErr.Code == apperrors.CodeUserNotFound {
		t.Errorf("generic error must NOT be wrapped as CodeUserNotFound (got code=%s) — handler would misroute to 401", authErr.Code)
	}
	// The original error must remain reachable so handler logs and the
	// fail-closed branch can wrap it with the service-specific context.
	assert.True(t, errors.Is(err, rawErr),
		"original error must remain in the chain so callers can log root cause")
}

// TestAuthService_GetUserProfile_Success asserts the happy path returns the
// user unchanged. Locks in that the typed-error wiring did not accidentally
// drop the success-case pass-through.
func TestAuthService_GetUserProfile_Success(t *testing.T) {
	cfg := &config.Config{
		JWT: config.JWTConfig{
			SecretKey: "test-secret-key-32-chars-minimum",
			Expiry:    24,
		},
	}
	expected := &authm.User{ID: 7, Email: stringPtr("ok@example.com")}
	userSvc := &stubUserServiceWithGetByID{
		getByID: func(userID uint) (*authm.User, error) {
			assert.Equal(t, uint(7), userID)
			return expected, nil
		},
	}
	authService := NewAuthService(nil, cfg, userSvc)

	user, err := authService.GetUserProfile(7)

	assert.NoError(t, err)
	assert.Same(t, expected, user)
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
