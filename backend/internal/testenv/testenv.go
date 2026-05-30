// Package testenv centralizes the "test-only feature flag" safety pattern
// shared by every flag that unlocks an E2E/test affordance the server must
// NEVER expose in production.
//
// The pattern, introduced piecemeal by PSY-432 (ENABLE_TEST_FIXTURES) and
// PSY-475 (DISABLE_AUTH_RATE_LIMITS), is default-deny:
//
//   - The flag is only honored when set to exactly "1".
//   - Even when set, it is only safe in an allow-listed ENVIRONMENT
//     ({test, ci, development}). In any other ENVIRONMENT — production,
//     stage, preview, or unset — the server must refuse to boot.
//
// Allow-listing (rather than black-listing "production") is deliberate:
// staging/preview and any future real-data environment must fail closed if
// someone forgets to add them to a blacklist. Forgetting to add a *new*
// safe environment to the allowlist only costs a failed boot in that one
// environment — a loud, safe failure — never a silent prod auth bypass.
//
// PSY-914 is the third flag of this exact shape (ENABLE_OAUTH_TEST_PROVIDER,
// which registers a fake OAuth provider answering as "google"), so the
// shape is factored here per the TODO left in auth_rate_limit.go. Both the
// pre-existing guards and the new OAuth guard delegate to this package.
package testenv

import "fmt"

// AllowedEnvironments is the set of ENVIRONMENT values in which a test-only
// feature flag may safely be enabled. Production, stage, preview, and unset
// are intentionally absent: any flag of this shape must fail closed there.
//
// Exposed (read-only by convention) so callers can echo the list in their
// own docs/errors; do NOT mutate it.
var AllowedEnvironments = map[string]bool{
	"test":        true,
	"ci":          true,
	"development": true,
}

// IsAllowedEnvironment reports whether env is one in which test-only flags
// may be enabled. Matching is exact and case-sensitive — "Test" is rejected,
// matching how ENVIRONMENT is compared elsewhere in the codebase.
func IsAllowedEnvironment(env string) bool {
	return AllowedEnvironments[env]
}

// IsFlagEnabled reports whether the named env var is set to exactly "1".
// Anything else ("", "0", "true", unset) reads as disabled — the flag must
// be opted into explicitly and unambiguously.
func IsFlagEnabled(flagName string, getenv func(string) string) bool {
	return getenv(flagName) == "1"
}

// ValidateFlagEnvironment is the keystone default-deny guard. It returns an
// error if flagName is enabled (=="1") while ENVIRONMENT is not allow-listed.
// A nil return means either the flag is off (always safe) or it is on in a
// safe environment.
//
// Call this at startup (cmd/server/main.go) for every test-only flag; a
// returned error must cause the server to refuse to boot. The route/registration
// guard (IsFlagEnabled) only helps if the process actually starts, so this
// boot-time check is the real safety net.
func ValidateFlagEnvironment(flagName string, getenv func(string) string) error {
	if !IsFlagEnabled(flagName, getenv) {
		return nil
	}
	env := getenv("ENVIRONMENT")
	if !IsAllowedEnvironment(env) {
		return fmt.Errorf(
			"%s=1 requires ENVIRONMENT to be one of %v (got %q); refusing to boot",
			flagName, allowedList(), env,
		)
	}
	return nil
}

// allowedList returns the allow-listed environments as a slice for error
// messages. Order is not guaranteed (map iteration); callers use it only for
// human-readable diagnostics, never for logic.
func allowedList() []string {
	allowed := make([]string, 0, len(AllowedEnvironments))
	for k := range AllowedEnvironments {
		allowed = append(allowed, k)
	}
	return allowed
}
