package routes

import (
	"net/http"
	"strings"
	"testing"
)

// PSY-1598 step 3: the auth and passkey groups move off their own humachi.New
// instances onto the main API.
//
// This is the group the ticket deliberately sequenced last. The whole
// authentication surface was undocumented — an external consumer reading the
// published spec saw no way to obtain a token at all — but it is also the surface
// where a mistake is worst: weaken it and login loses brute-force protection,
// over-tighten it and legitimate users are locked out.
//
// Two properties are specific to this group and are the ones most likely to break
// silently, so each gets a dedicated test:
//
//  1. authRateLimiter is ONE closure shared with the raw chi OAuth routes. The
//     conversion passes the existing variable; constructing a fresh limiter would
//     look identical and quietly double the budget.
//  2. Under DISABLE_AUTH_RATE_LIMITS the limiter is noopRateLimiter() (PSY-475).
//     If humaFromHTTP mishandled a middleware that simply returns `next`, every
//     E2E shard — all sharing 127.0.0.1 — would start failing register and
//     magic-link again, which is the exact regression PSY-475 fixed.

const (
	authLimitPerMinute    = 10 // auth.go, matching middleware.AuthRequestsPerMinute
	passkeyLimitPerMinute = 20 // auth.go, matching middleware.PasskeyRequestsPerMinute
)

func TestAuthOperationsAreInMainSpec(t *testing.T) {
	paths := servedSpecPaths(t, newTestRouter(t))

	for _, p := range []string{
		"/auth/login",
		"/auth/register",
		"/auth/magic-link/send",
		"/auth/magic-link/verify",
		"/auth/apple/callback",
		"/auth/recover-account",
		"/auth/recover-account/request",
		"/auth/recover-account/confirm",
		"/auth/passkey/login/begin",
		"/auth/passkey/login/finish",
		"/auth/passkey/signup/begin",
		"/auth/passkey/signup/finish",
	} {
		item, ok := paths[p]
		if !ok {
			t.Errorf("%s is missing from the served spec — the auth surface is still undocumented", p)
			continue
		}
		if _, ok := item["post"]; !ok {
			t.Errorf("expected a documented POST operation for %s", p)
		}
	}
}

func TestAuthStillRateLimited(t *testing.T) {
	router := newTestRouter(t)
	const ip = "203.0.113.41:1234"

	burstUnderCap(t, router, "POST", "/auth/login", ip, authLimitPerMinute)
	if code := send(t, router, "POST", "/auth/login", ip, nil); code != http.StatusTooManyRequests {
		t.Errorf("request %d returned %d, want 429 — brute-force protection did NOT survive the move to a Huma group",
			authLimitPerMinute+1, code)
	}
}

// Every operation in the group draws on the same counter, as they did under one
// chi group — otherwise an attacker just rotates endpoints to multiply the budget.
func TestAuthRateLimitIsSharedAcrossTheGroup(t *testing.T) {
	router := newTestRouter(t)
	const ip = "203.0.113.42:1234"

	paths := []string{"/auth/login", "/auth/register", "/auth/magic-link/send", "/auth/recover-account"}
	for i := 0; i < authLimitPerMinute; i++ {
		p := paths[i%len(paths)]
		if code := send(t, router, "POST", p, ip, nil); code == http.StatusTooManyRequests {
			t.Fatalf("%s (request %d/%d) was limited early", p, i+1, authLimitPerMinute)
		}
	}
	if code := send(t, router, "POST", "/auth/register", ip, nil); code != http.StatusTooManyRequests {
		t.Errorf("rotating endpoints returned %d, want 429 — auth operations no longer share one budget", code)
	}
}

// The subtle one. authRateLimiter is also used by the raw chi OAuth routes, which
// were NOT converted. Passing the same closure keeps one counter across both; a
// freshly-constructed limiter would give the Huma group its own, doubling the
// effective budget for anyone willing to alternate.
func TestAuthAndOAuthShareOneBudget(t *testing.T) {
	router := newTestRouter(t)
	const ip = "203.0.113.43:1234"

	// Spend the whole budget on the UNCONVERTED raw chi OAuth route.
	for i := 0; i < authLimitPerMinute; i++ {
		if code := send(t, router, "GET", "/auth/login/google", ip, nil); code == http.StatusTooManyRequests {
			t.Fatalf("OAuth request %d/%d was limited early", i+1, authLimitPerMinute)
		}
	}

	// The converted Huma operation must now be out of budget too.
	if code := send(t, router, "POST", "/auth/login", ip, nil); code != http.StatusTooManyRequests {
		t.Errorf("POST /auth/login returned %d after the OAuth routes exhausted the budget, want 429 — "+
			"the Huma group got its own limiter instead of sharing authRateLimiter", code)
	}
}

func TestPasskeyStillRateLimited(t *testing.T) {
	router := newTestRouter(t)
	const ip = "203.0.113.44:1234"

	burstUnderCap(t, router, "POST", "/auth/passkey/login/begin", ip, passkeyLimitPerMinute)
	if code := send(t, router, "POST", "/auth/passkey/login/begin", ip, nil); code != http.StatusTooManyRequests {
		t.Errorf("request %d returned %d, want 429 — the passkey limit did NOT survive the move",
			passkeyLimitPerMinute+1, code)
	}
}

// Passkey is deliberately more lenient than auth (multi-step WebAuthn). The two
// must stay on separate counters, or the looser limit silently relaxes login.
func TestPasskeyAndAuthDoNotShareABudget(t *testing.T) {
	router := newTestRouter(t)
	const ip = "203.0.113.45:1234"

	for i := 0; i <= authLimitPerMinute; i++ {
		send(t, router, "POST", "/auth/login", ip, nil)
	}
	if code := send(t, router, "POST", "/auth/login", ip, nil); code != http.StatusTooManyRequests {
		t.Fatalf("auth budget should be exhausted, got %d", code)
	}
	if code := send(t, router, "POST", "/auth/passkey/login/begin", ip, nil); code == http.StatusTooManyRequests {
		t.Error("passkey was limited by the auth budget — the two groups share a counter they should not")
	}
}

func TestAuthRateLimitIsPerIP(t *testing.T) {
	router := newTestRouter(t)

	for i := 0; i <= authLimitPerMinute; i++ {
		send(t, router, "POST", "/auth/login", "198.51.100.41:5000", nil)
	}
	if code := send(t, router, "POST", "/auth/login", "198.51.100.41:5000", nil); code != http.StatusTooManyRequests {
		t.Fatalf("first client should be limited, got %d", code)
	}
	if code := send(t, router, "POST", "/auth/login", "198.51.100.42:5000", nil); code == http.StatusTooManyRequests {
		t.Error("a second IP was limited by the first client's budget; KeyByClientIP did not survive the move")
	}
}

// PSY-475 through the bridge. noopRateLimiter returns `next` unchanged, so
// humaFromHTTP's sentinel must still record that the chain ran. If it did not,
// every auth request would be dropped rather than merely unthrottled — a far
// louder failure, but one that would only show up in E2E.
func TestAuthRateLimitDisabledFlagIsHonoredThroughTheBridge(t *testing.T) {
	t.Setenv(DisableAuthRateLimitsEnvVar, "1")
	router := newTestRouter(t)
	const ip = "203.0.113.46:1234"

	for i := 0; i < authLimitPerMinute*3; i++ {
		code, body := sendWithBody(t, router, "POST", "/auth/login", ip, nil)
		if code == http.StatusTooManyRequests {
			t.Fatalf("request %d returned 429 with %s=1 — the no-op limiter did not survive the bridge",
				i+1, DisableAuthRateLimitsEnvVar)
		}
		assertReachedHandler(t, code, body, "/auth/login")
	}

	// Same for passkey, which shares the flag.
	for i := 0; i < passkeyLimitPerMinute*2; i++ {
		code, body := sendWithBody(t, router, "POST", "/auth/passkey/login/begin", ip, nil)
		if code == http.StatusTooManyRequests {
			t.Fatalf("passkey request %d returned 429 with %s=1", i+1, DisableAuthRateLimitsEnvVar)
		}
		assertReachedHandler(t, code, body, "/auth/passkey/login/begin")
	}
}

// assertReachedHandler is the guard that "not 429" alone does NOT give you.
//
// humaFromHTTP decides whether the request was allowed by seeing if a sentinel
// handler ran. If that detection breaks, the middleware returns without calling
// next — and Huma, having never run the operation, emits a bare 200 WITH AN EMPTY
// BODY. Every "expect not-429" assertion passes against that. The endpoint looks
// healthy and silently does nothing, which is worse than a loud failure.
//
// Verified by negative control: with `if allowed` forced false, POST /auth/login
// returns 200/"" instead of its normal 422 validation error.
func assertReachedHandler(t *testing.T, code int, body, path string) {
	t.Helper()
	if code == http.StatusOK && strings.TrimSpace(body) == "" {
		t.Fatalf("%s returned 200 with an EMPTY body — the operation never ran. humaFromHTTP "+
			"swallowed the request instead of continuing the chain; a not-429 assertion cannot see this", path)
	}
}

// With the flag OFF the limiter must be real — the negative control for the test
// above, so "disabled works" can't pass by the limiter never having been wired up.
func TestAuthRateLimitEnabledWhenFlagAbsent(t *testing.T) {
	t.Setenv(DisableAuthRateLimitsEnvVar, "")
	router := newTestRouter(t)
	const ip = "203.0.113.47:1234"

	for i := 0; i <= authLimitPerMinute; i++ {
		send(t, router, "POST", "/auth/login", ip, nil)
	}
	if code := send(t, router, "POST", "/auth/login", ip, nil); code != http.StatusTooManyRequests {
		t.Errorf("with %s unset the limiter must still bite; got %d", DisableAuthRateLimitsEnvVar, code)
	}
}

// The auth group carries NO JWT middleware, deliberately — these endpoints are how
// a caller obtains a token. Pinning it because adding one would lock every user out
// of logging in, and the group is now a plain Huma group where adding middleware is
// a one-line change.
func TestAuthGroupDoesNotRequireAuthentication(t *testing.T) {
	router := newTestRouter(t)
	code := send(t, router, "POST", "/auth/login", "203.0.113.48:1234", nil)
	if code == http.StatusUnauthorized {
		t.Error("POST /auth/login returned 401 without credentials — a JWT middleware leaked onto the auth group")
	}
	if code == http.StatusNotFound {
		t.Error("POST /auth/login returned 404 — the operation is not registered")
	}
}

// PSY-1598 removed TestAuthPathsNoLongerAssertedAbsent here. It existed to prove
// two paths had been flipped out of the characterization test that pinned the
// remaining sub-API operations as absent; that test is gone now that no sub-APIs
// remain, and TestAuthOperationsAreInMainSpec above already asserts both paths.
