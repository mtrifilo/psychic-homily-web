package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/markbates/goth"
	"github.com/stretchr/testify/suite"

	"psychic-homily-backend/internal/config"
	"psychic-homily-backend/internal/services"
)

// mockOAuthCompleter implements services.OAuthCompleter for testing
type mockOAuthCompleter struct {
	user goth.User
	err  error
}

func (m *mockOAuthCompleter) CompleteUserAuth(w http.ResponseWriter, r *http.Request) (goth.User, error) {
	return m.user, m.err
}

type OAuthHandlerIntegrationSuite struct {
	suite.Suite
	deps *handlerIntegrationDeps
	cfg  *config.Config
}

func (s *OAuthHandlerIntegrationSuite) SetupSuite() {
	s.deps = setupHandlerIntegrationDeps(s.T())
	s.cfg = &config.Config{
		JWT: config.JWTConfig{
			SecretKey: "test-secret-key-at-least-32-characters-long",
			Expiry:    24,
		},
		Session: config.SessionConfig{
			Path:     "/",
			Domain:   "",
			MaxAge:   86400,
			HttpOnly: true,
			Secure:   false,
			SameSite: "lax",
		},
		Email: config.EmailConfig{
			FrontendURL: "http://localhost:3000",
		},
	}
}

func (s *OAuthHandlerIntegrationSuite) TearDownTest() {
	cleanupTables(s.deps.db)
	cleanCLICallbackStore()
}

func (s *OAuthHandlerIntegrationSuite) TearDownSuite() {
	if s.deps.container != nil {
		s.deps.container.Terminate(s.deps.ctx)
	}
}

func TestOAuthHandlerIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	suite.Run(t, new(OAuthHandlerIntegrationSuite))
}

func (s *OAuthHandlerIntegrationSuite) newHandler(completer services.OAuthCompleter) *OAuthHTTPHandler {
	authService := services.NewAuthService(s.deps.db, s.cfg)
	authService.SetOAuthCompleter(completer)
	return NewOAuthHTTPHandler(authService, s.cfg)
}

func oauthCallbackRequest(provider string) (*httptest.ResponseRecorder, *http.Request) {
	path := "/auth/callback/" + provider
	req := httptest.NewRequest("GET", path, nil)
	w := httptest.NewRecorder()

	if provider != "" {
		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("provider", provider)
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	}

	return w, req
}

func (s *OAuthHandlerIntegrationSuite) addSignupConsentCookie(req *http.Request) {
	encoded, err := encodeOAuthSignupConsent(services.OAuthSignupConsent{
		TermsAccepted:  true,
		TermsVersion:   "2026-01-31",
		PrivacyVersion: "2026-02-15",
		AcceptedAt:     time.Now().UTC(),
	})
	s.Require().NoError(err)
	req.AddCookie(&http.Cookie{
		Name:  oauthSignupConsentCookieName,
		Value: encoded,
	})
}

// --- OAuthCallbackHTTPHandler: error paths ---

func (s *OAuthHandlerIntegrationSuite) TestCallback_Error_RedirectsToFrontend() {
	handler := s.newHandler(&mockOAuthCompleter{
		err: http.ErrNoCookie, // any error
	})

	w, req := oauthCallbackRequest("google")
	s.addSignupConsentCookie(req)
	handler.OAuthCallbackHTTPHandler(w, req)

	s.Equal(http.StatusTemporaryRedirect, w.Code)
	location := w.Header().Get("Location")
	s.True(strings.HasPrefix(location, "http://localhost:3000/auth?error="),
		"expected redirect to frontend with error, got %s", location)
}

func (s *OAuthHandlerIntegrationSuite) TestCallback_Error_CLICallback_RedirectsToCLI() {
	handler := s.newHandler(&mockOAuthCompleter{
		err: http.ErrNoCookie,
	})

	// Store a CLI callback and set the cookie
	callbackID := "test-cli-id"
	storeCLICallback(callbackID, "http://localhost:8888/cli-cb")

	w, req := oauthCallbackRequest("google")
	s.addSignupConsentCookie(req)
	req.AddCookie(&http.Cookie{
		Name:  "cli_callback_id",
		Value: callbackID,
	})

	handler.OAuthCallbackHTTPHandler(w, req)

	s.Equal(http.StatusTemporaryRedirect, w.Code)
	location := w.Header().Get("Location")
	s.True(strings.HasPrefix(location, "http://localhost:8888/cli-cb?error="),
		"expected redirect to CLI callback with error, got %s", location)
}

func (s *OAuthHandlerIntegrationSuite) TestCallback_Error_CLICallbackCookie_Cleared() {
	handler := s.newHandler(&mockOAuthCompleter{
		err: http.ErrNoCookie,
	})

	callbackID := "clear-test-id"
	storeCLICallback(callbackID, "http://localhost:8888/cli-cb")

	w, req := oauthCallbackRequest("google")
	req.AddCookie(&http.Cookie{
		Name:  "cli_callback_id",
		Value: callbackID,
	})

	handler.OAuthCallbackHTTPHandler(w, req)

	// Verify the cli_callback_id cookie was cleared (MaxAge -1)
	resp := w.Result()
	for _, c := range resp.Cookies() {
		if c.Name == "cli_callback_id" {
			s.Equal("", c.Value, "expected cleared cookie value")
			s.Equal(-1, c.MaxAge, "expected MaxAge -1 to clear cookie")
			return
		}
	}
	s.Fail("expected cli_callback_id cookie in response")
}

func (s *OAuthHandlerIntegrationSuite) TestCallback_Error_CLICallbackMemory_Deleted() {
	handler := s.newHandler(&mockOAuthCompleter{
		err: http.ErrNoCookie,
	})

	callbackID := "mem-del-id"
	storeCLICallback(callbackID, "http://localhost:8888/cli-cb")

	w, req := oauthCallbackRequest("google")
	req.AddCookie(&http.Cookie{
		Name:  "cli_callback_id",
		Value: callbackID,
	})

	handler.OAuthCallbackHTTPHandler(w, req)

	// Verify the callback was removed from the memory store
	_, ok := getCLICallback(callbackID)
	s.False(ok, "expected CLI callback to be deleted from memory after use")

	_ = w // used
}

func (s *OAuthHandlerIntegrationSuite) TestCallback_NoProvider_FallbackToGoogle() {
	handler := s.newHandler(&mockOAuthCompleter{
		err: http.ErrNoCookie, // will fail, but we want to test provider fallback
	})

	// Request with no provider in chi params
	req := httptest.NewRequest("GET", "/auth/callback", nil)
	w := httptest.NewRecorder()

	handler.OAuthCallbackHTTPHandler(w, req)

	// Should redirect with error (google provider used as fallback)
	s.Equal(http.StatusTemporaryRedirect, w.Code)
	location := w.Header().Get("Location")
	s.Contains(location, "error=", "expected error redirect")
}

func (s *OAuthHandlerIntegrationSuite) TestCallback_CustomFrontendURL() {
	customCfg := &config.Config{
		JWT:     s.cfg.JWT,
		Session: s.cfg.Session,
		Email: config.EmailConfig{
			FrontendURL: "https://myapp.example.com",
		},
	}
	authService := services.NewAuthService(s.deps.db, customCfg)
	authService.SetOAuthCompleter(&mockOAuthCompleter{err: http.ErrNoCookie})
	handler := NewOAuthHTTPHandler(authService, customCfg)

	w, req := oauthCallbackRequest("google")
	s.addSignupConsentCookie(req)
	handler.OAuthCallbackHTTPHandler(w, req)

	location := w.Header().Get("Location")
	s.True(strings.HasPrefix(location, "https://myapp.example.com/auth?error="),
		"expected redirect to custom frontend URL, got %s", location)
}

func (s *OAuthHandlerIntegrationSuite) TestCallback_EmptyFrontendURL_Fallback() {
	emptyCfg := &config.Config{
		JWT:     s.cfg.JWT,
		Session: s.cfg.Session,
		Email:   config.EmailConfig{FrontendURL: ""},
	}
	authService := services.NewAuthService(s.deps.db, emptyCfg)
	authService.SetOAuthCompleter(&mockOAuthCompleter{err: http.ErrNoCookie})
	handler := NewOAuthHTTPHandler(authService, emptyCfg)

	w, req := oauthCallbackRequest("google")
	s.addSignupConsentCookie(req)
	handler.OAuthCallbackHTTPHandler(w, req)

	location := w.Header().Get("Location")
	s.True(strings.HasPrefix(location, "http://localhost:3000/auth?error="),
		"expected fallback to localhost:3000, got %s", location)
}

// --- OAuthCallbackHTTPHandler: success paths ---

func (s *OAuthHandlerIntegrationSuite) TestCallback_Success_SetsCookieAndRedirects() {
	handler := s.newHandler(&mockOAuthCompleter{
		user: goth.User{
			Email:    "oauth-web@test.com",
			Provider: "google",
			UserID:   "google-web-123",
			Name:     "Web User",
		},
	})

	w, req := oauthCallbackRequest("google")
	s.addSignupConsentCookie(req)
	handler.OAuthCallbackHTTPHandler(w, req)

	s.Equal(http.StatusTemporaryRedirect, w.Code)

	// Should redirect to frontend home
	location := w.Header().Get("Location")
	s.Equal("http://localhost:3000", location)

	// Should set auth cookie
	resp := w.Result()
	var authCookie *http.Cookie
	for _, c := range resp.Cookies() {
		if c.Name == "auth_token" {
			authCookie = c
			break
		}
	}
	s.Require().NotNil(authCookie, "expected auth_token cookie to be set")
	s.NotEmpty(authCookie.Value, "expected non-empty token in cookie")
	s.True(authCookie.HttpOnly, "expected HttpOnly cookie")
}

func (s *OAuthHandlerIntegrationSuite) TestCallback_Success_CLICallback_RedirectsWithToken() {
	handler := s.newHandler(&mockOAuthCompleter{
		user: goth.User{
			Email:    "oauth-cli@test.com",
			Provider: "google",
			UserID:   "google-cli-456",
			Name:     "CLI User",
		},
	})

	// Store CLI callback
	callbackID := "cli-success-id"
	storeCLICallback(callbackID, "http://localhost:8888/cli-success")

	w, req := oauthCallbackRequest("google")
	s.addSignupConsentCookie(req)
	req.AddCookie(&http.Cookie{
		Name:  "cli_callback_id",
		Value: callbackID,
	})

	handler.OAuthCallbackHTTPHandler(w, req)

	s.Equal(http.StatusTemporaryRedirect, w.Code)

	location := w.Header().Get("Location")
	s.True(strings.HasPrefix(location, "http://localhost:8888/cli-success?token="),
		"expected redirect to CLI callback with token, got %s", location)
	s.Contains(location, "expires_in=86400", "expected expires_in parameter")

	// Should NOT set auth cookie for CLI flow
	resp := w.Result()
	for _, c := range resp.Cookies() {
		if c.Name == "auth_token" {
			s.Fail("expected no auth_token cookie for CLI flow")
		}
	}
}

func (s *OAuthHandlerIntegrationSuite) TestCallback_Success_GithubProvider() {
	handler := s.newHandler(&mockOAuthCompleter{
		user: goth.User{
			Email:    "oauth-github@test.com",
			Provider: "github",
			UserID:   "github-789",
			Name:     "GitHub User",
		},
	})

	w, req := oauthCallbackRequest("github")
	s.addSignupConsentCookie(req)
	handler.OAuthCallbackHTTPHandler(w, req)

	s.Equal(http.StatusTemporaryRedirect, w.Code)
	location := w.Header().Get("Location")
	s.Equal("http://localhost:3000", location)
}

func (s *OAuthHandlerIntegrationSuite) TestCallback_Success_ExistingUser_ReturnsToken() {
	// Create user via first OAuth callback
	handler := s.newHandler(&mockOAuthCompleter{
		user: goth.User{
			Email:    "existing-oauth@test.com",
			Provider: "google",
			UserID:   "google-existing-999",
			Name:     "Existing User",
		},
	})

	w1, req1 := oauthCallbackRequest("google")
	s.addSignupConsentCookie(req1)
	handler.OAuthCallbackHTTPHandler(w1, req1)
	s.Equal(http.StatusTemporaryRedirect, w1.Code)

	// Second callback for same user should still work
	w2, req2 := oauthCallbackRequest("google")
	handler.OAuthCallbackHTTPHandler(w2, req2)
	s.Equal(http.StatusTemporaryRedirect, w2.Code)

	location := w2.Header().Get("Location")
	s.Equal("http://localhost:3000", location)
}

func (s *OAuthHandlerIntegrationSuite) TestCallback_ExpiredCLICallback_NotUsed() {
	handler := s.newHandler(&mockOAuthCompleter{
		user: goth.User{
			Email:    "oauth-expired-cli@test.com",
			Provider: "google",
			UserID:   "google-expired-cli-123",
			Name:     "Expired CLI User",
		},
	})

	// Store an expired CLI callback
	cliCallbackStore.Lock()
	cliCallbackStore.callbacks["expired-cli-id"] = cliCallbackEntry{
		callbackURL: "http://localhost:8888/expired",
		expiresAt:   time.Now().Add(-1 * time.Minute),
	}
	cliCallbackStore.Unlock()

	w, req := oauthCallbackRequest("google")
	s.addSignupConsentCookie(req)
	req.AddCookie(&http.Cookie{
		Name:  "cli_callback_id",
		Value: "expired-cli-id",
	})

	handler.OAuthCallbackHTTPHandler(w, req)

	// Should fall back to web flow (expired CLI callback not found)
	s.Equal(http.StatusTemporaryRedirect, w.Code)
	location := w.Header().Get("Location")
	s.Equal("http://localhost:3000", location,
		"expected web redirect when CLI callback is expired")
}
