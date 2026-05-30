package routes

import (
	"net/http"

	"psychic-homily-backend/internal/testenv"
)

// PSY-475: env-flagged skip of the IP-scoped auth/passkey rate limiters
// during E2E runs. All E2E workers on a CI shard share one IP
// (127.0.0.1), so auth flows intermittently hit HTTP 429 and break
// `register.spec.ts` / `magic-link.spec.ts`. Pattern mirrors PSY-432's
// ENABLE_TEST_FIXTURES gate: env flag honored only when ENVIRONMENT is
// allowlisted; default-deny at startup refuses to boot in prod/staging/
// preview/unset with the flag on.
//
// PSY-914: the {test,ci,development} allowlist + default-deny boot check
// are now shared in internal/testenv (third flag of this shape landed —
// ENABLE_OAUTH_TEST_PROVIDER). These thin wrappers stay so callers/tests
// keep a flag-specific name; the policy lives in one place.
const DisableAuthRateLimitsEnvVar = "DISABLE_AUTH_RATE_LIMITS"

// IsAuthRateLimitDisabled reports whether the auth + passkey rate
// limiters should be replaced with no-ops. ValidateAuthRateLimitEnvironment
// is the safety gate — callers should invoke it at startup before relying
// on this value for route setup.
func IsAuthRateLimitDisabled(getenv func(string) string) bool {
	return testenv.IsFlagEnabled(DisableAuthRateLimitsEnvVar, getenv)
}

// ValidateAuthRateLimitEnvironment returns an error if the disable flag is
// on in a non-allowlisted ENVIRONMENT. Call from cmd/server/main.go
// before route setup; a returned error should cause the server to refuse
// to boot.
func ValidateAuthRateLimitEnvironment(getenv func(string) string) error {
	return testenv.ValidateFlagEnvironment(DisableAuthRateLimitsEnvVar, getenv)
}

// noopRateLimiter returns a pass-through middleware. Used in place of
// httprate.Limit when IsAuthRateLimitDisabled reports true.
func noopRateLimiter() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler { return next }
}
