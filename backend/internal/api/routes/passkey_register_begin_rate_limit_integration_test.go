package routes

import (
	"net/http"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"psychic-homily-backend/internal/api/middleware"
	"psychic-homily-backend/internal/config"
	authm "psychic-homily-backend/internal/models/auth"
	"psychic-homily-backend/internal/services"
	"psychic-homily-backend/internal/testutil"
)

// Adding a passkey to a signed-in account, driven through the ROUTER by a
// caller holding a real session. Register begin sits behind HumaJWTMiddleware,
// so reaching its counter takes a session, and a session takes a database.

const (
	passkeyRegisterBeginPath  = "/auth/passkey/register/begin"
	passkeyRegisterFinishPath = "/auth/passkey/register/finish"
	// The handler's own success reply, which only a request that passed the
	// session, the recent-auth gate and the limiter can carry.
	passkeyRegisterBeginReached = `"success":true`
)

// passkeyRegisterFixture builds a signed-in user, returning a constructor for
// routers over one container and that user's session. Building the router is
// left to the caller because the limiters read DISABLE_AUTH_RATE_LIMITS when
// the routes are set up.
func passkeyRegisterFixture(t *testing.T, email string) (func() *chi.Mux, string) {
	t.Helper()

	td := testutil.SetupTestPostgres(t)
	t.Cleanup(td.Cleanup)

	cfg := testConfig()
	// go-webauthn refuses to begin a ceremony for a relying party with no
	// display name.
	cfg.WebAuthn = config.WebAuthnConfig{RPDisplayName: "Test", RPID: "localhost", RPOrigins: []string{"http://localhost:3000"}}
	sc := services.NewServiceContainer(td.DB, cfg)
	if sc.WebAuthn == nil {
		t.Fatal("the WebAuthn service did not build, so no passkey route is registered")
	}

	user := &authm.User{Email: &email, IsActive: true, EmailVerified: true}
	if err := td.DB.Create(user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	newRouter := func() *chi.Mux {
		router := chi.NewRouter()
		SetupRoutes(router, sc, cfg)
		return router
	}
	return newRouter, mintToken(t, sc, user)
}

func requireRegisterBeginReached(t *testing.T, router *chi.Mux, ip, session, why string) {
	t.Helper()
	w := sendAuthed(t, router, http.MethodPost, passkeyRegisterBeginPath, "{}", ip, session)
	if w.Code == http.StatusTooManyRequests {
		t.Fatalf("register begin returned 429: %s", why)
	}
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), passkeyRegisterBeginReached) {
		t.Fatalf("register begin did not reach its handler (%s); status %d body %q", why, w.Code, w.Body.String())
	}
}

func TestPasskeyRegisterBeginThrottledThroughRouter(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	newRouter, session := passkeyRegisterFixture(t, "passkey-register-throttle@test.com")

	t.Run("limits on", func(t *testing.T) {
		t.Setenv(DisableAuthRateLimitsEnvVar, "")
		router := newRouter()
		const ip = "198.51.100.120:4444"

		// A caller with no session is refused by HumaJWTMiddleware before the
		// counter, so any number of these leaves the whole budget.
		for i := 0; i < passkeyRegisterBeginLimitPerMinute*2; i++ {
			if w := sendAuthed(t, router, http.MethodPost, passkeyRegisterBeginPath, "{}", ip, ""); w.Code != http.StatusUnauthorized {
				t.Fatalf("register begin with no session = %d, want 401", w.Code)
			}
		}

		for i := 0; i < passkeyRegisterBeginLimitPerMinute; i++ {
			requireRegisterBeginReached(t, router, ip, session, "refused before its budget was spent")
		}

		req := jsonRequest(http.MethodPost, passkeyRegisterBeginPath, ip)
		req.Header.Set("Authorization", "Bearer "+session)
		refused, limiters := serveLoggingRejections(router, req)
		assertJSONRefusal(t, refused, "POST "+passkeyRegisterBeginPath)
		if len(limiters) != 1 || limiters[0] != string(middleware.LimiterPasskeyRegisterBegin) {
			t.Errorf("rejection lines name limiters %v, want exactly [%s]", limiters, middleware.LimiterPasskeyRegisterBegin)
		}

		// The finishes are not on this budget: the authenticator has written
		// the credential by the time one is sent.
		finish := sendAuthed(t, router, http.MethodPost, passkeyRegisterFinishPath, "{}", ip, session)
		if finish.Code == http.StatusTooManyRequests || finish.Code == http.StatusUnauthorized {
			t.Errorf("register finish after the begin budget was spent = %d, want its handler's answer", finish.Code)
		}
		assertReachedHandler(t, finish.Code, finish.Body.String(), passkeyRegisterFinishPath)
		for _, path := range []string{passkeyLoginFinishPath, passkeySignupFinishPath} {
			requirePasskeyRouteReached(t, router, path, ip, "refused after only the register begin budget was spent")
		}

		// The reach of the mount. A Huma group binds its middleware into an
		// operation as that operation is registered, so the probe is a
		// protected route setupPasskeyRoutes registers AFTER the group: it
		// keeps answering only while the limiter is on the child group and not
		// on rc.Protected.
		if w := sendAuthed(t, router, http.MethodGet, "/auth/passkey/credentials", "", ip, session); w.Code != http.StatusOK {
			t.Errorf("GET /auth/passkey/credentials with only the register begin budget spent = %d, want 200", w.Code)
		}

		// The budget is per IP.
		requireRegisterBeginReached(t, router, "198.51.100.121:4444", session, "a second address was refused by the first one's budget")
	})

	t.Run("limits disabled", func(t *testing.T) {
		t.Setenv(DisableAuthRateLimitsEnvVar, "1")
		router := newRouter()
		const ip = "198.51.100.122:4444"

		for i := 0; i < passkeyRegisterBeginLimitPerMinute*3; i++ {
			requireRegisterBeginReached(t, router, ip, session, "refused with the rate limits disabled")
		}
	})
}
