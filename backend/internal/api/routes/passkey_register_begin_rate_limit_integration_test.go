package routes

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"psychic-homily-backend/internal/api/middleware"
	"psychic-homily-backend/internal/config"
	authm "psychic-homily-backend/internal/models/auth"
	"psychic-homily-backend/internal/services"
	"psychic-homily-backend/internal/testutil"
)

// Adding a passkey to a signed-in account, driven through the ROUTER by callers
// holding real sessions. Register begin sits behind HumaJWTMiddleware, so
// reaching its counter takes a session, and a session takes a database.

const (
	passkeyRegisterBeginPath  = "/auth/passkey/register/begin"
	passkeyRegisterFinishPath = "/auth/passkey/register/finish"
	// The handler's own success reply, which only a request that passed the
	// session, the recent-auth gate and the limiter can carry.
	passkeyRegisterBeginReached = `"success":true`
)

// passkeyRegisterFixture returns a constructor for routers over one container
// and a function that creates a signed-in user and returns its session.
// Building the router is left to the caller because the limiters read
// DISABLE_AUTH_RATE_LIMITS when the routes are set up.
func passkeyRegisterFixture(t *testing.T) (newRouter func() *chi.Mux, signIn func(email string) string) {
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

	newRouter = func() *chi.Mux {
		router := chi.NewRouter()
		SetupRoutes(router, sc, cfg)
		return router
	}
	signIn = func(email string) string {
		t.Helper()
		user := &authm.User{Email: &email, IsActive: true, EmailVerified: true}
		if err := td.DB.Create(user).Error; err != nil {
			t.Fatalf("create user %s: %v", email, err)
		}
		return mintToken(t, sc, user)
	}
	return newRouter, signIn
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
	newRouter, signIn := passkeyRegisterFixture(t)
	session := signIn("passkey-register-throttle@test.com")
	neighbour := signIn("passkey-register-neighbour@test.com")

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

		// The budget follows the session, not the address: the same account
		// from another address is still refused.
		if w := sendAuthed(t, router, http.MethodPost, passkeyRegisterBeginPath, "{}", "198.51.100.121:4444", session); w.Code != http.StatusTooManyRequests {
			t.Errorf("register begin for the spent account from a second address = %d, want 429", w.Code)
		}

		// Another account behind the same address has a budget of its own.
		for i := 0; i < passkeyRegisterBeginLimitPerMinute; i++ {
			requireRegisterBeginReached(t, router, ip, neighbour, "a second account on the same address was refused by the first one's budget")
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
	})

	t.Run("limits disabled", func(t *testing.T) {
		t.Setenv(DisableAuthRateLimitsEnvVar, "1")
		router := newRouter()
		const ip = "198.51.100.122:4444"

		// Past both the account's budget and the address's ceiling.
		for i := 0; i <= passkeyRegisterBeginIPCeiling; i++ {
			requireRegisterBeginReached(t, router, ip, session, "refused with the rate limits disabled")
		}
	})
}

// Every account behind one address together is bounded by the per-IP ceiling,
// while each stays bounded by its own budget. Filling the ceiling takes
// ceiling/budget accounts spending their whole budgets; one more account, its
// own budget untouched, is then refused by the ceiling, and only on that
// address.
func TestPasskeyRegisterBeginIPCeilingThroughRouter(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	newRouter, signIn := passkeyRegisterFixture(t)
	t.Setenv(DisableAuthRateLimitsEnvVar, "")
	router := newRouter()
	const ip = "198.51.100.130:4444"

	accounts := passkeyRegisterBeginIPCeiling / passkeyRegisterBeginLimitPerMinute
	sessions := make([]string, accounts)
	for a := range sessions {
		sessions[a] = signIn(fmt.Sprintf("passkey-register-ceiling-%d@test.com", a))
	}
	fresh := signIn("passkey-register-ceiling-fresh@test.com")

	window := startInOneRateWindow(t, ceilingTestWindowNeed)
	for a := range sessions {
		for i := 0; i < passkeyRegisterBeginLimitPerMinute; i++ {
			requireRegisterBeginReached(t, router, ip, sessions[a],
				fmt.Sprintf("account %d begin %d refused below both its own budget and the ceiling", a+1, i+1))
		}
	}

	// An account past its own budget is refused by its own limiter, not the
	// ceiling, even with the ceiling full.
	req := jsonRequest(http.MethodPost, passkeyRegisterBeginPath, ip)
	req.Header.Set("Authorization", "Bearer "+sessions[0])
	refused, limiters := serveLoggingRejections(router, req)
	assertJSONRefusal(t, refused, "POST "+passkeyRegisterBeginPath+" past the account's own budget")
	if len(limiters) != 1 || limiters[0] != string(middleware.LimiterPasskeyRegisterBegin) {
		t.Errorf("own-budget rejection lines name limiters %v, want exactly [%s]", limiters, middleware.LimiterPasskeyRegisterBegin)
	}

	req = jsonRequest(http.MethodPost, passkeyRegisterBeginPath, ip)
	req.Header.Set("Authorization", "Bearer "+fresh)
	refused, limiters = serveLoggingRejections(router, req)
	assertJSONRefusal(t, refused, "POST "+passkeyRegisterBeginPath+" on a full ceiling")
	if len(limiters) != 1 || limiters[0] != string(middleware.LimiterPasskeyRegisterBeginIPCeiling) {
		t.Errorf("ceiling rejection lines name limiters %v, want exactly [%s]", limiters, middleware.LimiterPasskeyRegisterBeginIPCeiling)
	}
	requireSameRateWindow(t, window)

	// The ceiling is per address: the same fresh account from another one is
	// served.
	requireRegisterBeginReached(t, router, "198.51.100.131:4444", fresh, "refused by another address's ceiling")
}

// ceilingTestWindowNeed is how much of one minute window the ceiling test's
// burst and refusals must fit in.
const ceilingTestWindowNeed = 20 * time.Second

// startInOneRateWindow waits, if it has to, until at least need is left in the
// current minute window, and returns that window's start. httprate meters on a
// sliding window over UTC minutes: once a minute boundary passes, the previous
// minute's count decays, and a full ceiling of 50 decays below its limit about
// 0.6s later. A refusal asserted across a boundary would then pass by chance.
func startInOneRateWindow(t *testing.T, need time.Duration) time.Time {
	t.Helper()
	now := time.Now().UTC()
	window := now.Truncate(time.Minute)
	if left := window.Add(time.Minute).Sub(now); left < need {
		time.Sleep(left)
		window = time.Now().UTC().Truncate(time.Minute)
	}
	return window
}

// requireSameRateWindow fails the test if the minute window that
// startInOneRateWindow returned has ended, since the refusals asserted since
// then no longer prove the limit.
func requireSameRateWindow(t *testing.T, window time.Time) {
	t.Helper()
	if now := time.Now().UTC(); !now.Truncate(time.Minute).Equal(window) {
		t.Fatalf("the test ran past its minute window (started %s, now %s); its refusals are not evidence of the limit", window, now)
	}
}
