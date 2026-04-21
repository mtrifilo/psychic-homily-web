package routes

import (
	"fmt"
	"net/http"
)

// PSY-475: env-flagged skip of the IP-scoped auth/passkey rate limiters
// during E2E runs. All E2E workers on a CI shard share one IP
// (127.0.0.1), so auth flows intermittently hit HTTP 429 and break
// `register.spec.ts` / `magic-link.spec.ts`. Pattern mirrors PSY-432's
// ENABLE_TEST_FIXTURES gate: env flag honored only when ENVIRONMENT is
// allowlisted; default-deny at startup refuses to boot in prod/staging/
// preview/unset with the flag on.
//
// Not combined with PSY-432's TestFixturesAllowedEnvironments enum on
// purpose — separating the two flags lets them be audited / flipped
// independently. Factor out into a shared `testenv` helper when a third
// flag lands with the same shape.
const DisableAuthRateLimitsEnvVar = "DISABLE_AUTH_RATE_LIMITS"

// authRateLimitAllowedEnvironments mirrors the PSY-432 allowed list:
// test, ci, development. Production / stage / preview / unset all
// refuse when the flag is on.
var authRateLimitAllowedEnvironments = map[string]bool{
	"test":        true,
	"ci":          true,
	"development": true,
}

// IsAuthRateLimitDisabled reports whether the auth + passkey rate
// limiters should be replaced with no-ops. ValidateAuthRateLimitEnvironment
// is the safety gate — callers should invoke it at startup before relying
// on this value for route setup.
func IsAuthRateLimitDisabled(getenv func(string) string) bool {
	return getenv(DisableAuthRateLimitsEnvVar) == "1"
}

// ValidateAuthRateLimitEnvironment returns an error if the disable flag is
// on in a non-allowlisted ENVIRONMENT. Call from cmd/server/main.go
// before route setup; a returned error should cause the server to refuse
// to boot.
func ValidateAuthRateLimitEnvironment(getenv func(string) string) error {
	if !IsAuthRateLimitDisabled(getenv) {
		return nil
	}
	env := getenv("ENVIRONMENT")
	if !authRateLimitAllowedEnvironments[env] {
		allowed := make([]string, 0, len(authRateLimitAllowedEnvironments))
		for k := range authRateLimitAllowedEnvironments {
			allowed = append(allowed, k)
		}
		return fmt.Errorf(
			"%s=1 requires ENVIRONMENT to be one of %v (got %q). Refusing to boot.",
			DisableAuthRateLimitsEnvVar, allowed, env,
		)
	}
	return nil
}

// noopRateLimiter returns a pass-through middleware. Used in place of
// httprate.Limit when IsAuthRateLimitDisabled reports true.
func noopRateLimiter() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler { return next }
}
