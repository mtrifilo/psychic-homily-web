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
					"if it takes the account password it belongs on passwordConfirmGroup, and either "+
					"way its scope has to be recorded here", key)
			}
		}
	}

	var stale []string
	for key := range authRouteRateLimitScope {
		if !served[key] {
			stale = append(stale, key)
		}
	}
	sort.Strings(stale)
	for _, key := range stale {
		t.Errorf("%q is dispositioned here but the router no longer serves it: "+
			"a stale entry hides the next route that takes its place", key)
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
// to. That ordering lives in the source and has no runtime representation, so
// this reads the source.
//
// Without this, tidying the preference registrations into a block above the
// group turns the over-reach probe into an assertion that cannot fail, and a
// limiter mistakenly throttling every authenticated route at this budget ships
// green.
func TestPasswordConfirmOverReachProbeIsRegisteredAfterTheGroup(t *testing.T) {
	const mount = "passwordConfirmGroup := huma.NewGroup("

	source, err := os.ReadFile("auth.go")
	if err != nil {
		t.Fatalf("read auth.go: %v", err)
	}
	text := string(source)

	mountAt := strings.Index(text, mount)
	if mountAt < 0 {
		t.Fatalf("auth.go no longer contains %q; the over-reach probe's premise cannot be checked", mount)
	}
	probeAt := strings.Index(text, passwordConfirmOverReachProbe)
	if probeAt < 0 {
		t.Fatalf("auth.go no longer registers the over-reach probe as %q; the throttle test is asking for a "+
			"route this guard cannot find, so pick a new probe and update both", passwordConfirmOverReachProbe)
	}
	if probeAt < mountAt {
		t.Errorf("the over-reach probe is registered before passwordConfirmGroup, so it would answer 200 whether " +
			"the limiter is on the group or on rc.Protected; the throttle test's reach assertion can no longer fail")
	}
}
