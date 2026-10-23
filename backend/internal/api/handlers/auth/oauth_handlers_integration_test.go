package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/markbates/goth"
	"github.com/stretchr/testify/suite"

	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	"psychic-homily-backend/internal/config"
	autherrors "psychic-homily-backend/internal/errors"
	"psychic-homily-backend/internal/legal"
	authm "psychic-homily-backend/internal/models/auth"
	"psychic-homily-backend/internal/services/auth"
	"psychic-homily-backend/internal/services/contracts"
)

// mockOAuthCompleter stands in for the provider token exchange. calls counts
// the exchanges, so a test can assert that a refusal came before the provider
// was contacted; onExchange, when set, runs during the exchange, the window in
// which the reader is at the provider.
type mockOAuthCompleter struct {
	user       goth.User
	err        error
	calls      int
	onExchange func()
}

func (m *mockOAuthCompleter) CompleteUserAuth(w http.ResponseWriter, r *http.Request) (goth.User, error) {
	m.calls++
	if m.onExchange != nil {
		m.onExchange()
	}
	return m.user, m.err
}

type OAuthHandlerIntegrationSuite struct {
	suite.Suite
	deps *testhelpers.IntegrationDeps
	cfg  *config.Config
}

func (s *OAuthHandlerIntegrationSuite) SetupSuite() {
	s.deps = testhelpers.SetupIntegrationDeps(s.T())
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
	testhelpers.CleanupTables(s.deps.DB)
	cleanCLICallbackStore()
}

func (s *OAuthHandlerIntegrationSuite) TearDownSuite() {
	s.deps.TestDB.Cleanup()
}

func TestOAuthHandlerIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	suite.Run(t, new(OAuthHandlerIntegrationSuite))
}

func (s *OAuthHandlerIntegrationSuite) newHandler(completer contracts.OAuthCompleter) *OAuthHTTPHandler {
	authService := auth.NewAuthService(s.deps.DB, s.cfg, s.deps.UserService)
	authService.SetOAuthCompleter(completer)
	return NewOAuthHTTPHandler(authService, s.jwtService(), s.cfg)
}

func (s *OAuthHandlerIntegrationSuite) jwtService() *auth.JWTService {
	return auth.NewJWTService(s.deps.DB, s.cfg, s.deps.UserService)
}

// addSessionCookie puts a real session credential on a callback request. The
// link path reads it to confirm the person finishing a handshake is the one
// who started it, and the callback is a public route so nothing else supplies
// a principal.
func (s *OAuthHandlerIntegrationSuite) addSessionCookie(req *http.Request, user *authm.User) {
	s.T().Helper()
	token, err := s.jwtService().CreateToken(user)
	s.Require().NoError(err)
	req.AddCookie(&http.Cookie{Name: "auth_token", Value: token})
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

// signUp creates the account identity resolves to, with valid consent, so a
// callback for identity is the returning sign-in rather than a new one.
func (s *OAuthHandlerIntegrationSuite) signUp(identity goth.User) *authm.User {
	s.T().Helper()
	user, err := s.deps.UserService.FindOrCreateUserWithConsent(identity, identity.Provider, &contracts.OAuthSignupConsent{
		TermsAccepted:  true,
		TermsVersion:   legal.CurrentTermsVersion,
		PrivacyVersion: legal.CurrentPrivacyVersion,
		AcceptedAt:     time.Now().UTC(),
		AgeConfirmed:   true,
		MinAgeAttested: MinSignupAge,
	})
	s.Require().NoError(err)
	return user
}

// --- OAuthCallbackHTTPHandler: error paths ---

func (s *OAuthHandlerIntegrationSuite) TestCallback_Error_RedirectsToFrontend() {
	handler := s.newHandler(&mockOAuthCompleter{
		err: http.ErrNoCookie, // any error
	})

	w, req := oauthCallbackRequest("google")
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
	authService := auth.NewAuthService(s.deps.DB, customCfg, s.deps.UserService)
	authService.SetOAuthCompleter(&mockOAuthCompleter{err: http.ErrNoCookie})
	handler := NewOAuthHTTPHandler(authService, s.jwtService(), customCfg)

	w, req := oauthCallbackRequest("google")
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
	authService := auth.NewAuthService(s.deps.DB, emptyCfg, s.deps.UserService)
	authService.SetOAuthCompleter(&mockOAuthCompleter{err: http.ErrNoCookie})
	handler := NewOAuthHTTPHandler(authService, s.jwtService(), emptyCfg)

	w, req := oauthCallbackRequest("google")
	handler.OAuthCallbackHTTPHandler(w, req)

	location := w.Header().Get("Location")
	s.True(strings.HasPrefix(location, "http://localhost:3000/auth?error="),
		"expected fallback to localhost:3000, got %s", location)
}

// A new Google identity is held as a pending sign-up and sent to the step,
// with no account and no session yet.
func (s *OAuthHandlerIntegrationSuite) TestCallback_NewGoogleIdentity_StartsTheSignupStep() {
	identity := goth.User{
		Email:    "oauth-new-identity@test.com",
		Provider: "google",
		UserID:   "google-new-identity-789",
		Name:     "New Identity",
		RawData:  map[string]any{"verified_email": true},
	}
	handler := s.newHandler(&mockOAuthCompleter{user: identity})

	w, req := oauthCallbackRequest("google")
	handler.OAuthCallbackHTTPHandler(w, req)

	s.Equal(http.StatusTemporaryRedirect, w.Code)
	s.Equal("http://localhost:3000"+oauthSignupStepPath, w.Header().Get("Location"))
	pending := findCookie(w.Result(), oauthPendingSignupCookie.name)
	s.Require().NotNil(pending, "expected the pending sign-up cookie")
	s.NotEmpty(pending.Value)
	s.Equal(int(oauthPendingSignupCookie.lifetime/time.Second), pending.MaxAge)
	s.Nil(findCookie(w.Result(), "auth_token"), "no session before the step is finished")

	var users, rows int64
	s.Require().NoError(s.deps.DB.Model(&authm.User{}).Where("email = ?", identity.Email).Count(&users).Error)
	s.Require().NoError(s.deps.DB.Model(&authm.OAuthPendingSignup{}).Where("provider_user_id = ?", identity.UserID).Count(&rows).Error)
	s.Zero(users, "the callback creates no account")
	s.Equal(int64(1), rows, "the callback holds the identity as a pending sign-up")
}

// The step names the provider as Google, so a new GitHub identity keeps the
// consent refusal.
func (s *OAuthHandlerIntegrationSuite) TestCallback_NewGithubIdentity_RefusedForConsent() {
	handler := s.newHandler(&mockOAuthCompleter{user: goth.User{
		Email: "oauth-noterms@test.com", Provider: "github", UserID: "github-noterms-789",
	}})

	w, req := oauthCallbackRequest("github")
	handler.OAuthCallbackHTTPHandler(w, req)

	s.assertAuthPageRefusal(w, autherrors.ErrTermsAcceptanceRequired(""))
	s.Nil(findCookie(w.Result(), oauthPendingSignupCookie.name))
}

// A CLI sign-in has no page to show the step on, so a new identity gets the
// consent refusal on its loopback, and nothing is held.
func (s *OAuthHandlerIntegrationSuite) TestCallback_NewIdentityFromTheCLI_RefusedForConsent() {
	handler := s.newHandler(&mockOAuthCompleter{user: goth.User{
		Email: "oauth-cli-new@test.com", Provider: "google", UserID: "google-cli-new-1",
	}})
	storeCLICallback("cli-new-identity", "http://localhost:8888/cli-new")

	w, req := oauthCallbackRequest("google")
	req.AddCookie(&http.Cookie{Name: "cli_callback_id", Value: "cli-new-identity"})
	handler.OAuthCallbackHTTPHandler(w, req)

	location, err := url.Parse(w.Header().Get("Location"))
	s.Require().NoError(err)
	s.Equal("localhost:8888", location.Host)
	s.Equal(autherrors.ErrTermsAcceptanceRequired("").UserMessage(), location.Query().Get("error"))
	s.Nil(findCookie(w.Result(), oauthPendingSignupCookie.name))
	var rows int64
	s.Require().NoError(s.deps.DB.Model(&authm.OAuthPendingSignup{}).Count(&rows).Error)
	s.Zero(rows)
}

// A browser that was refused for its age gets no pending sign-up: the step
// shows it the refusal again.
func (s *OAuthHandlerIntegrationSuite) TestCallback_NewIdentityWithTheAgeMarker_HoldsNothing() {
	handler := s.newHandler(&mockOAuthCompleter{user: goth.User{
		Email: "oauth-marked@test.com", Provider: "google", UserID: "google-marked-1",
	}})

	w, req := oauthCallbackRequest("google")
	req.AddCookie(&http.Cookie{Name: signupAgeRefusalCookieName, Value: "1"})
	handler.OAuthCallbackHTTPHandler(w, req)

	s.Equal("http://localhost:3000"+oauthSignupStepPath, w.Header().Get("Location"))
	s.Nil(findCookie(w.Result(), oauthPendingSignupCookie.name))
	var rows int64
	s.Require().NoError(s.deps.DB.Model(&authm.OAuthPendingSignup{}).Count(&rows).Error)
	s.Zero(rows)
}

// --- OAuthCallbackHTTPHandler: success paths ---

func (s *OAuthHandlerIntegrationSuite) TestCallback_Success_SetsCookieAndRedirects() {
	identity := goth.User{
		Email:    "oauth-web@test.com",
		Provider: "google",
		UserID:   "google-web-123",
		Name:     "Web User",
	}
	s.signUp(identity)
	handler := s.newHandler(&mockOAuthCompleter{user: identity})

	w, req := oauthCallbackRequest("google")
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
	identity := goth.User{
		Email:    "oauth-cli@test.com",
		Provider: "google",
		UserID:   "google-cli-456",
		Name:     "CLI User",
	}
	s.signUp(identity)
	handler := s.newHandler(&mockOAuthCompleter{user: identity})

	// Store CLI callback
	callbackID := "cli-success-id"
	storeCLICallback(callbackID, "http://localhost:8888/cli-success")

	w, req := oauthCallbackRequest("google")
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
	identity := goth.User{
		Email:    "oauth-github@test.com",
		Provider: "github",
		UserID:   "github-789",
		Name:     "GitHub User",
	}
	s.signUp(identity)
	handler := s.newHandler(&mockOAuthCompleter{user: identity})

	w, req := oauthCallbackRequest("github")
	handler.OAuthCallbackHTTPHandler(w, req)

	s.Equal(http.StatusTemporaryRedirect, w.Code)
	location := w.Header().Get("Location")
	s.Equal("http://localhost:3000", location)
}

func (s *OAuthHandlerIntegrationSuite) TestCallback_Success_ExistingUser_ReturnsToken() {
	// Create user via first OAuth callback
	identity := goth.User{
		Email:    "existing-oauth@test.com",
		Provider: "google",
		UserID:   "google-existing-999",
		Name:     "Existing User",
	}
	s.signUp(identity)
	handler := s.newHandler(&mockOAuthCompleter{user: identity})

	w1, req1 := oauthCallbackRequest("google")
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
	identity := goth.User{
		Email:    "oauth-expired-cli@test.com",
		Provider: "google",
		UserID:   "google-expired-cli-123",
		Name:     "Expired CLI User",
	}
	s.signUp(identity)
	handler := s.newHandler(&mockOAuthCompleter{user: identity})

	// Store an expired CLI callback
	storeExpiredCLICallback("expired-cli-id", "http://localhost:8888/expired")

	w, req := oauthCallbackRequest("google")
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
