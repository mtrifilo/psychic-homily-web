package routes

import (
	"strings"
	"testing"
)

// The membership of every auth POST budget is written by hand at the
// registration sites, and nothing in the router says a new route was added. A
// route that verifies a password and lands on bare rc.Protected is unmetered,
// and no test that names the routes it knows about can see it.
//
// These guards walk the BUILT router instead. A new POST under /auth/ fails
// here until its author writes down which budget meters it, which is the point
// at which "does this one answer a password guess?" gets asked.

// Budget names, spelled once so a disposition cannot drift into a typo that
// reads as a different scope.
const (
	scopePublicAuth       = "rateLimitedAuthGroup: the 10/min public auth budget"
	scopePasswordConfirm  = "passwordConfirmGroup: the shared password-confirm budget"
	scopePasskey          = "passkeyGroup: the 20/min passkey budget"
	scopeResendSized      = "its own limiter at the verification-resend size"
	scopeProtectedNoLimit = "rc.Protected with no per-route limiter"
	scopeAdminNoLimit     = "rc.Admin with no per-route limiter"
	scopePublicNoLimit    = "rc.API with no per-route limiter"
	scopeLenient          = "lenientGroup: the refresh path, no per-route limiter"
)

// authPostRateLimitScope disposes of every POST the router serves under /auth/.
//
// The two entries reading scopePasswordConfirm must match
// passwordConfirmBudgetRoutes exactly; TestPasswordConfirmDispositionsCoverTheBudget
// closes that loop, and the throttle test drives each member through the router.
var authPostRateLimitScope = map[string]string{
	"/auth/login":                      scopePublicAuth,
	"/auth/register":                   scopePublicAuth,
	"/auth/magic-link/send":            scopePublicAuth,
	"/auth/magic-link/verify":          scopePublicAuth,
	"/auth/apple/callback":             scopePublicAuth,
	"/auth/recover-account":            scopePublicAuth,
	"/auth/recover-account/request":    scopePublicAuth,
	"/auth/recover-account/confirm":    scopePublicAuth,
	"/auth/change-password":            scopePasswordConfirm,
	"/auth/account/delete":             scopePasswordConfirm,
	"/auth/passkey/login/begin":        scopePasskey,
	"/auth/passkey/login/finish":       scopePasskey,
	"/auth/passkey/signup/begin":       scopePasskey,
	"/auth/passkey/signup/finish":      scopePasskey,
	"/auth/verify-email/send":          scopeResendSized,
	"/auth/oauth/link-token":           scopeResendSized,
	"/auth/passkey/register/begin":     scopeProtectedNoLimit,
	"/auth/passkey/register/finish":    scopeProtectedNoLimit,
	"/auth/profile/sections":           scopeProtectedNoLimit,
	"/auth/logout":                     scopePublicNoLimit,
	"/auth/verify-email/confirm":       scopePublicNoLimit,
	"/auth/unsubscribe/show-reminders": scopePublicNoLimit,
	"/auth/cli-token":                  scopeAdminNoLimit,
	"/auth/refresh":                    scopeLenient,
}

func TestEveryAuthPostHasARateLimitDisposition(t *testing.T) {
	routes := chiRoutes(t, newTestRouter(t))

	served := map[string]bool{}
	for _, pattern := range routes["POST"] {
		if !strings.HasPrefix(pattern, "/auth/") {
			continue
		}
		served[pattern] = true
		if _, ok := authPostRateLimitScope[pattern]; !ok {
			t.Errorf("the router serves POST %q and no budget is written down for it: "+
				"if it answers a password guess it belongs on passwordConfirmGroup, and either way "+
				"its scope has to be recorded here", pattern)
		}
	}

	for pattern := range authPostRateLimitScope {
		if !served[pattern] {
			t.Errorf("POST %q is dispositioned here but the router no longer serves it: "+
				"a stale entry hides the next route that takes its place", pattern)
		}
	}
}

// The disposition table and the routes the throttle test drives must name the
// same set. A route mounted on passwordConfirmGroup and left out of
// passwordConfirmBudgetRoutes ships with the budget but with nothing proving it.
func TestPasswordConfirmDispositionsCoverTheBudget(t *testing.T) {
	dispositioned := map[string]bool{}
	for pattern, scope := range authPostRateLimitScope {
		if scope == scopePasswordConfirm {
			dispositioned[pattern] = true
		}
	}

	driven := map[string]bool{}
	for _, route := range passwordConfirmBudgetRoutes {
		driven[route.path] = true
	}

	for pattern := range dispositioned {
		if !driven[pattern] {
			t.Errorf("POST %q is dispositioned onto the password-confirm budget but no test drives it "+
				"through the router; add it to passwordConfirmBudgetRoutes", pattern)
		}
	}
	for pattern := range driven {
		if !dispositioned[pattern] {
			t.Errorf("passwordConfirmBudgetRoutes drives POST %q but the disposition table does not put it "+
				"on that budget; one of the two is wrong about where the route is mounted", pattern)
		}
	}
}
