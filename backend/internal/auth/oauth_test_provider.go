package auth

import (
	"github.com/markbates/goth"
	"github.com/markbates/goth/providers/faux"

	"psychic-homily-backend/internal/testenv"
)

// PSY-914: a FAKE OAuth provider that answers as "google", used to exercise
// the real OAuth login -> callback -> session flow in E2E without a live
// Google IdP.
//
// SECURITY: if this provider were ever registered in production it would be a
// CRITICAL auth bypass — anyone hitting /auth/login/google could mint a session
// as TestProviderEmail. Registration is therefore double-gated:
//
//  1. SetupGoth only appends it when ENABLE_OAUTH_TEST_PROVIDER=1, AND
//  2. main.go calls ValidateOAuthTestProviderEnvironment at boot, which
//     (via internal/testenv) refuses to start the server if the flag is set in
//     any ENVIRONMENT outside {test, ci, development}. Production / stage /
//     preview / unset all fail closed.
//
// Mirrors the default-deny shape of ENABLE_TEST_FIXTURES (PSY-432) and
// DISABLE_AUTH_RATE_LIMITS (PSY-475).
const EnableOAuthTestProviderEnvVar = "ENABLE_OAUTH_TEST_PROVIDER"

// TestProviderName is the provider key the faux clone registers under. It must
// be a real provider the frontend can initiate ("google"), so the existing
// /auth/login/google button and /auth/callback/google route drive it unchanged.
const TestProviderName = "google"

// TestProviderEmail is the deterministic email the faux clone always returns.
// The E2E seed pre-creates a user with this email so the first faux login
// resolves to an EXISTING user (a login, via linkOAuthAccount), not a brand-new
// signup (which would trigger the terms/consent flow). Stock goth/faux returns
// an EMPTY email, which would make FindOrCreateUserWithConsent key on "" and
// create a degenerate account — hence the clone.
const TestProviderEmail = "e2e-oauth@test.local"

// TestProviderUserID is the deterministic provider_user_id the faux clone
// returns. Fixed so repeat logins resolve to the same oauth_accounts row.
const TestProviderUserID = "e2e-oauth-faux-user-id"

// testProvider wraps goth's faux provider so it (a) registers/resolves under
// the name "google" and (b) returns a deterministic, non-empty user. Every
// method other than Name/FetchUser delegates to the embedded faux provider, so
// the state-token round-trip (BeginAuth -> AuthURL -> Authorize -> session) is
// the real goth machinery — only the identity surface is faked.
type testProvider struct {
	*faux.Provider
}

// newTestProvider builds the faux clone. The embedded faux.Provider supplies
// BeginAuth (state nonce + http://example.com/auth AuthURL), UnmarshalSession,
// RefreshToken, etc.
func newTestProvider() *testProvider {
	return &testProvider{Provider: &faux.Provider{}}
}

// Name returns "google". goth.UseProviders keys the registry on Name(), and
// gothic.CompleteUserAuth looks the provider up by the request's "provider"
// query param ("google"). Both must agree, so this override — not faux's
// SetName — is what makes the clone resolve as Google.
func (p *testProvider) Name() string {
	return TestProviderName
}

// FetchUser delegates to faux for the AccessToken-gating round-trip (faux
// errors until the session has been Authorize()d, exactly as a real provider
// would before the token exchange), then overwrites the identity fields with
// the deterministic test values. The empty-email problem in stock faux is
// fixed here.
func (p *testProvider) FetchUser(session goth.Session) (goth.User, error) {
	user, err := p.Provider.FetchUser(session)
	if err != nil {
		return user, err
	}
	user.Provider = TestProviderName
	user.UserID = TestProviderUserID
	user.Email = TestProviderEmail
	user.Name = "E2E OAuth User"
	return user, nil
}

// IsOAuthTestProviderEnabled reports whether the faux "google" provider should
// be registered. ValidateOAuthTestProviderEnvironment is the safety gate —
// callers MUST invoke it at startup before relying on this for registration.
func IsOAuthTestProviderEnabled(getenv func(string) string) bool {
	return testenv.IsFlagEnabled(EnableOAuthTestProviderEnvVar, getenv)
}

// ValidateOAuthTestProviderEnvironment returns an error if the faux-provider
// flag is set in a non-allowlisted ENVIRONMENT. Call from cmd/server/main.go
// before SetupGoth; a returned error MUST cause the server to refuse to boot.
// This is the keystone defense against the faux provider ever going live.
func ValidateOAuthTestProviderEnvironment(getenv func(string) string) error {
	return testenv.ValidateFlagEnvironment(EnableOAuthTestProviderEnvVar, getenv)
}
