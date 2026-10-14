package routes

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"psychic-homily-backend/internal/api/middleware"
	"psychic-homily-backend/internal/logger"
)

// Written out rather than read from middleware, so a budget retuned there and
// not here fails this file instead of passing silently.
const (
	passkeySignupBeginLimitPerMinute   = 5 // middleware.PasskeySignupBeginRequestsPerMinute
	passkeyRegisterBeginLimitPerMinute = 5 // middleware.PasskeyRegisterBeginRequestsPerMinute
)

const (
	passkeySignupBeginPath  = "/auth/passkey/signup/begin"
	passkeySignupFinishPath = "/auth/passkey/signup/finish"
	passkeyLoginBeginPath   = "/auth/passkey/login/begin"
	passkeyLoginFinishPath  = "/auth/passkey/login/finish"
)

// requirePasskeyRouteReached sends one unauthenticated POST and requires it to
// have reached its operation: not refused by a limiter, and not the empty 200
// a broken humaFromHTTP answers with.
func requirePasskeyRouteReached(t *testing.T, router *chi.Mux, path, ip, why string) {
	t.Helper()
	code, body := sendWithBody(t, router, http.MethodPost, path, ip, nil)
	if code == http.StatusTooManyRequests {
		t.Fatalf("%s returned 429: %s", path, why)
	}
	assertReachedHandler(t, code, body, path)
}

func TestPasskeySignupBeginHasItsOwnBudget(t *testing.T) {
	t.Setenv(DisableAuthRateLimitsEnvVar, "")
	router := newTestRouter(t)
	const ip = "203.0.113.81:1234"

	for i := 0; i < passkeySignupBeginLimitPerMinute; i++ {
		requirePasskeyRouteReached(t, router, passkeySignupBeginPath, ip,
			fmt.Sprintf("begin %d was refused before its %d/min budget was spent", i+1, passkeySignupBeginLimitPerMinute))
	}

	assertJSONRefusal(t, serveFrom(router, http.MethodPost, passkeySignupBeginPath, ip), "POST "+passkeySignupBeginPath)
}

// A sign-up finish arrives after the authenticator has written the credential,
// so nothing a begin flood spends may refuse it. The flood here is four times
// the begin budget; every finish and the login begin must then still get the
// WHOLE passkey budget, which a begin counted on that budget would have eaten
// into.
func TestPasskeySignupBeginFloodRefusesNoFinish(t *testing.T) {
	t.Setenv(DisableAuthRateLimitsEnvVar, "")
	router := newTestRouter(t)
	const ip = "203.0.113.82:1234"

	for i := 0; i < passkeySignupBeginLimitPerMinute*4; i++ {
		serveFrom(router, http.MethodPost, passkeySignupBeginPath, ip)
	}
	if code := send(t, router, http.MethodPost, passkeySignupBeginPath, ip, nil); code != http.StatusTooManyRequests {
		t.Fatalf("signup begin after the flood = %d, want 429; the flood did not spend the begin budget", code)
	}

	shared := []string{passkeySignupFinishPath, passkeyLoginFinishPath, passkeyLoginBeginPath}
	for i := 0; i < passkeyLimitPerMinute; i++ {
		path := shared[i%len(shared)]
		requirePasskeyRouteReached(t, router, path, ip,
			fmt.Sprintf("request %d/%d on the passkey budget was refused after a signup begin flood; "+
				"signup begin spends the passkey counter", i+1, passkeyLimitPerMinute))
	}

	// The negative control: the passkey counter is live and was spent by
	// exactly the requests above.
	if code := send(t, router, http.MethodPost, passkeyLoginFinishPath, ip, nil); code != http.StatusTooManyRequests {
		t.Errorf("login finish past the passkey budget = %d, want 429", code)
	}
}

// The other direction: a spent passkey budget leaves sign-up begin its own.
func TestPasskeyBudgetSpentLeavesSignupBeginItsBudget(t *testing.T) {
	t.Setenv(DisableAuthRateLimitsEnvVar, "")
	router := newTestRouter(t)
	const ip = "203.0.113.83:1234"

	burstUnderCap(t, router, http.MethodPost, passkeyLoginBeginPath, ip, passkeyLimitPerMinute)
	if code := send(t, router, http.MethodPost, passkeyLoginBeginPath, ip, nil); code != http.StatusTooManyRequests {
		t.Fatalf("login begin past the passkey budget = %d, want 429", code)
	}

	for i := 0; i < passkeySignupBeginLimitPerMinute; i++ {
		requirePasskeyRouteReached(t, router, passkeySignupBeginPath, ip,
			"signup begin was refused after only the passkey budget was spent; the two share a counter")
	}
}

func TestPasskeySignupBeginRefusalLogsItsOwnLimiter(t *testing.T) {
	t.Setenv(DisableAuthRateLimitsEnvVar, "")
	router := newTestRouter(t)
	var buf bytes.Buffer
	log := slog.New(slog.NewJSONHandler(&buf, nil))

	for i := 0; i <= passkeySignupBeginLimitPerMinute; i++ {
		req := httptest.NewRequest(http.MethodPost, passkeySignupBeginPath, strings.NewReader(`{}`))
		req.Header.Set("Content-Type", "application/json")
		req.RemoteAddr = "203.0.113.84:1234"
		router.ServeHTTP(httptest.NewRecorder(), req.WithContext(logger.NewContext(req.Context(), log)))
	}

	if got := rejectedLimiters(buf.Bytes()); len(got) != 1 || got[0] != string(middleware.LimiterPasskeySignupBegin) {
		t.Errorf("rejection lines name limiters %v, want exactly [%s]", got, middleware.LimiterPasskeySignupBegin)
	}
}

func TestPasskeySignupBeginDisabledFlagIsHonored(t *testing.T) {
	t.Setenv(DisableAuthRateLimitsEnvVar, "1")
	router := newTestRouter(t)
	const ip = "203.0.113.85:1234"

	for i := 0; i < passkeySignupBeginLimitPerMinute*3; i++ {
		requirePasskeyRouteReached(t, router, passkeySignupBeginPath, ip,
			fmt.Sprintf("begin %d was refused with %s=1", i+1, DisableAuthRateLimitsEnvVar))
	}
}

// rejectedLimiters returns the limiter named on each ratelimit_rejected line
// of a JSON log, in order.
func rejectedLimiters(logged []byte) []string {
	var limiters []string
	for _, raw := range bytes.Split(bytes.TrimSpace(logged), []byte("\n")) {
		var line map[string]any
		if json.Unmarshal(raw, &line) == nil && line["event"] == "ratelimit_rejected" {
			limiters = append(limiters, fmt.Sprint(line["limiter"]))
		}
	}
	return limiters
}
