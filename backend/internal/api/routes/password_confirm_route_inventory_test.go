package routes

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"testing"

	"psychic-homily-backend/internal/api/middleware"
)

// The budget on each auth route is chosen by hand at its registration site, and
// nothing in the router says a new route was added. A route that takes the
// account password and lands on bare rc.Protected is unmetered, and no test
// that names the routes it already knows about can see it.
//
// This guard walks the BUILT router and requires every mutating route under
// /auth/ to be written down here, so adding one fails until its author says
// which budget meters it. That is the moment "does this one answer a password
// guess?" gets asked.
//
// What the guard CANNOT see, stated so it is not mistaken for more than it is:
//
//   - It compares the KEYS against the router. Each value is a sentence, not a
//     reading of the middleware chain, because a Huma group binds its
//     middleware inside the operation handler where chi.Walk cannot reach it.
//     A route moved off passwordConfirmGroup and left labelled as though it
//     were still there passes here; what catches that is
//     TestPasswordConfirmRoutesThrottledThroughRouter, which drives every
//     member through the router and needs a database.
//   - It sweeps /auth/ only. The public /unsubscribe/ family registered in
//     auth.go on rc.API and on the chi router is invisible to it.
//   - It sweeps mutating methods only. No GET takes a password today.
//   - It sees only the routes THIS build registers. setupPasskeyRoutes returns
//     early when rc.SC.WebAuthn is nil, so a mutating route added inside that
//     block, or inside any other conditional registration, is invisible to the
//     sweep on a build where the condition is false.
const authInventoryPrefix = "/auth/"

var authInventoryMethods = []string{"POST", "PUT", "PATCH", "DELETE"}

// Budget names, spelled once so a disposition cannot drift into a typo that
// reads as a different scope. The sizes are read from the constants the mounts
// are built from rather than written out, so a budget retuned in one place
// cannot leave a false number here.
var (
	scopePublicAuth       = fmt.Sprintf("rateLimitedAuthGroup: the %d/min public auth budget", middleware.AuthRequestsPerMinute)
	scopePasswordConfirm  = fmt.Sprintf("passwordConfirmGroup: the shared %d/min password-confirm budget", PasswordConfirmAttemptsPerMinute)
	scopePasskey          = fmt.Sprintf("passkeyGroup: the %d/min passkey budget", middleware.PasskeyRequestsPerMinute)
	scopeResendSized      = fmt.Sprintf("its own limiter at the %d/min verification-resend size", VerificationResendPerMinute)
	scopeProtectedNoLimit = "rc.Protected with no per-route limiter"
	scopeAdminNoLimit     = "rc.Admin with no per-route limiter"
	scopePublicNoLimit    = "rc.API with no per-route limiter"
	scopeLenient          = "lenientGroup: the refresh path, no per-route limiter"
)

// authRouteRateLimitScope disposes of every mutating route the router serves
// under /auth/, keyed "METHOD /path".
//
// The rows reading scopePasswordConfirm must name the same routes as
// passwordConfirmBudgetRoutes; TestPasswordConfirmDispositionsCoverTheBudget
// requires that, and the throttle test drives each of those through the router.
// No other row carries an assertion beyond its own presence.
var authRouteRateLimitScope = map[string]string{
	"POST /auth/login":                      scopePublicAuth,
	"POST /auth/register":                   scopePublicAuth,
	"POST /auth/magic-link/send":            scopePublicAuth,
	"POST /auth/magic-link/verify":          scopePublicAuth,
	"POST /auth/apple/callback":             scopePublicAuth,
	"POST /auth/recover-account":            scopePublicAuth,
	"POST /auth/recover-account/request":    scopePublicAuth,
	"POST /auth/recover-account/confirm":    scopePublicAuth,
	"POST /auth/change-password":            scopePasswordConfirm,
	"POST /auth/account/delete":             scopePasswordConfirm,
	"POST /auth/passkey/login/begin":        scopePasskey,
	"POST /auth/passkey/login/finish":       scopePasskey,
	"POST /auth/passkey/signup/begin":       scopePasskey,
	"POST /auth/passkey/signup/finish":      scopePasskey,
	"POST /auth/verify-email/send":          scopeResendSized,
	"POST /auth/oauth/link-token":           scopeResendSized,
	"POST /auth/passkey/register/begin":     scopeProtectedNoLimit,
	"POST /auth/passkey/register/finish":    scopeProtectedNoLimit,
	"POST /auth/profile/sections":           scopeProtectedNoLimit,
	"POST /auth/logout":                     scopePublicNoLimit,
	"POST /auth/verify-email/confirm":       scopePublicNoLimit,
	"POST /auth/unsubscribe/show-reminders": scopePublicNoLimit,
	"POST /auth/cli-token":                  scopeAdminNoLimit,
	"POST /auth/refresh":                    scopeLenient,

	"PATCH /auth/profile":            scopeProtectedNoLimit,
	"PATCH /auth/profile/privacy":    scopeProtectedNoLimit,
	"PATCH /auth/profile/visibility": scopeProtectedNoLimit,

	"PATCH /auth/preferences/alert-defaults":           scopeProtectedNoLimit,
	"PATCH /auth/preferences/collection-digest":        scopeProtectedNoLimit,
	"PATCH /auth/preferences/comment-notifications":    scopeProtectedNoLimit,
	"PATCH /auth/preferences/default-reply-permission": scopeProtectedNoLimit,
	"PATCH /auth/preferences/scene-digest":             scopeProtectedNoLimit,
	"PATCH /auth/preferences/show-reminders":           scopeProtectedNoLimit,
	"PATCH /auth/preferences/tier-edit-notifications":  scopeProtectedNoLimit,

	"PUT /auth/preferences/chart-defaults":  scopeProtectedNoLimit,
	"PUT /auth/preferences/favorite-cities": scopeProtectedNoLimit,
	"PUT /auth/preferences/home-metro":      scopeProtectedNoLimit,

	"PUT /auth/profile/sections/{section_id}":          scopeProtectedNoLimit,
	"DELETE /auth/profile/sections/{section_id}":       scopeProtectedNoLimit,
	"DELETE /auth/oauth/accounts/{provider}":           scopeProtectedNoLimit,
	"DELETE /auth/passkey/credentials/{credential_id}": scopeProtectedNoLimit,
}

func TestEveryMutatingAuthRouteHasARateLimitDisposition(t *testing.T) {
	routes := chiRoutes(t, newTestRouter(t))

	served := map[string]bool{}
	for _, method := range authInventoryMethods {
		for _, pattern := range routes[method] {
			if !strings.HasPrefix(pattern, authInventoryPrefix) {
				continue
			}
			key := method + " " + pattern
			served[key] = true
			if _, ok := authRouteRateLimitScope[key]; !ok {
				t.Errorf("the router serves %q and no budget is written down for it: "+
					"record the group it is REGISTERED on, reading auth.go rather than copying a "+
					"neighbouring constant, since a wrong sentence here passes this test and "+
					"misleads the next reader; if it takes the account password it belongs on "+
					"passwordConfirmGroup", key)
			}
		}
	}

	// The passkey routes are registered only when the WebAuthn service built,
	// so on a build without it every passkey row would read as stale and this
	// guard would go red over configuration rather than over routing. Absent as
	// a block is the condition being false; absent one at a time is a real
	// stale row.
	passkeyServed := 0
	for key, scope := range authRouteRateLimitScope {
		if scope == scopePasskey && served[key] {
			passkeyServed++
		}
	}

	var stale []string
	for key, scope := range authRouteRateLimitScope {
		if served[key] {
			continue
		}
		if scope == scopePasskey && passkeyServed == 0 {
			continue
		}
		stale = append(stale, key)
	}
	sort.Strings(stale)
	for _, key := range stale {
		t.Errorf("%q is dispositioned here but the router no longer serves it: "+
			"a stale entry hides the next route that takes its place", key)
	}
	if passkeyServed == 0 {
		t.Log("this build registered no passkey routes, so their dispositions were not swept")
	}
}

// The disposition table and the routes the throttle test drives must name the
// same set. A route mounted on passwordConfirmGroup and left out of
// passwordConfirmBudgetRoutes ships with the budget but with nothing proving it.
func TestPasswordConfirmDispositionsCoverTheBudget(t *testing.T) {
	dispositioned := map[string]bool{}
	for key, scope := range authRouteRateLimitScope {
		if scope == scopePasswordConfirm {
			dispositioned[strings.TrimPrefix(key, "POST ")] = true
		}
	}

	driven := map[string]bool{}
	for _, route := range passwordConfirmBudgetRoutes {
		driven[route.path] = true
	}

	for pattern := range dispositioned {
		if !driven[pattern] {
			t.Errorf("%q is dispositioned onto the password-confirm budget but no test drives it "+
				"through the router; add it to passwordConfirmBudgetRoutes", pattern)
		}
	}
	for pattern := range driven {
		if !dispositioned[pattern] {
			t.Errorf("passwordConfirmBudgetRoutes drives %q but the disposition table does not put it "+
				"on that budget; one of the two is wrong about where the route is mounted", pattern)
		}
	}
}

// passwordConfirmOverReachProbe is the route the throttle test asks for after
// exhausting the budget, to catch a limiter mounted on rc.Protected rather than
// on the child group.
const passwordConfirmOverReachProbe = `huma.Get(rc.Protected, "/auth/preferences/alerts"`

// The probe only proves anything while it is registered AFTER the group: a Huma
// group binds its middleware into an operation as that operation is registered,
// so a route registered earlier answers normally whatever the group is attached
// to.
//
// The property is CALL order. This reads text order inside one function body,
// which stands in for it only while both sites live in that body, so the search
// is bounded to setupProtectedAuthRoutes: hoisting the preference registrations
// into a helper called before it fails here rather than quietly turning the
// over-reach probe into an assertion that cannot fail.
func TestPasswordConfirmOverReachProbeIsRegisteredAfterTheGroup(t *testing.T) {
	const mount = "passwordConfirmGroup := huma.NewGroup("
	const enclosing = "func setupProtectedAuthRoutes(rc RouteContext) {"

	source, err := os.ReadFile("auth.go")
	if err != nil {
		t.Fatalf("read auth.go: %v", err)
	}
	text := string(source)

	bodyAt := strings.Index(text, enclosing)
	if bodyAt < 0 {
		t.Fatalf("auth.go no longer declares %q; the over-reach probe's premise cannot be checked", enclosing)
	}
	body := text[bodyAt:]
	if next := strings.Index(body[len(enclosing):], "\nfunc "); next >= 0 {
		body = body[:len(enclosing)+next]
	}

	mountAt := strings.Index(body, mount)
	if mountAt < 0 {
		t.Fatalf("%s no longer builds %q; the over-reach probe's premise cannot be checked", enclosing, mount)
	}
	// Exactly one, so a commented-out or duplicated registration earlier in the
	// body cannot satisfy the ordering on behalf of the live one.
	if n := strings.Count(body, passwordConfirmOverReachProbe); n != 1 {
		t.Fatalf("%s contains %d occurrences of %q, want exactly 1: with more than one, the ordering "+
			"below is checked against whichever comes first rather than against the registration the "+
			"throttle test calls", enclosing, n, passwordConfirmOverReachProbe)
	}
	probeAt := strings.Index(body, passwordConfirmOverReachProbe)

	if probeAt < mountAt {
		t.Errorf("the over-reach probe is registered before passwordConfirmGroup, so it would answer 200 whether " +
			"the limiter is on the group or on rc.Protected; the throttle test's reach assertion can no longer fail")
	}
}
