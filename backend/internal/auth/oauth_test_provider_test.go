package auth

import (
	"strings"
	"testing"

	"github.com/markbates/goth"
)

func envFromMap(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestIsOAuthTestProviderEnabled(t *testing.T) {
	cases := []struct {
		name string
		env  map[string]string
		want bool
	}{
		{"unset", map[string]string{}, false},
		{"empty", map[string]string{EnableOAuthTestProviderEnvVar: ""}, false},
		{"zero", map[string]string{EnableOAuthTestProviderEnvVar: "0"}, false},
		{"truthy-but-not-1", map[string]string{EnableOAuthTestProviderEnvVar: "true"}, false},
		{"exactly-1", map[string]string{EnableOAuthTestProviderEnvVar: "1"}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsOAuthTestProviderEnabled(envFromMap(tc.env)); got != tc.want {
				t.Errorf("want %v got %v", tc.want, got)
			}
		})
	}
}

// TestValidateOAuthTestProviderEnvironment is the security contract for the
// faux "google" provider: the flag may only be honored in {test, ci,
// development}. Anywhere else — production, stage, preview, or unset — the
// guard MUST return an error so cmd/server/main.go refuses to boot. A
// regression here would let the fake provider go live and mint sessions as
// TestProviderEmail (critical auth bypass).
func TestValidateOAuthTestProviderEnvironment(t *testing.T) {
	cases := []struct {
		name        string
		env         map[string]string
		wantError   bool
		errContains string
	}{
		// Flag off = always safe.
		{"flag-off / env-unset", map[string]string{}, false, ""},
		{"flag-off / env-production", map[string]string{"ENVIRONMENT": "production"}, false, ""},
		{"flag-0 / env-production", map[string]string{EnableOAuthTestProviderEnvVar: "0", "ENVIRONMENT": "production"}, false, ""},

		// Flag on + allowed env = safe.
		{"flag-on / env-test", map[string]string{EnableOAuthTestProviderEnvVar: "1", "ENVIRONMENT": "test"}, false, ""},
		{"flag-on / env-ci", map[string]string{EnableOAuthTestProviderEnvVar: "1", "ENVIRONMENT": "ci"}, false, ""},
		{"flag-on / env-development", map[string]string{EnableOAuthTestProviderEnvVar: "1", "ENVIRONMENT": "development"}, false, ""},

		// Flag on + non-allowed env = refuse to boot (the security-critical rows).
		{"flag-on / env-production", map[string]string{EnableOAuthTestProviderEnvVar: "1", "ENVIRONMENT": "production"}, true, "production"},
		{"flag-on / env-stage", map[string]string{EnableOAuthTestProviderEnvVar: "1", "ENVIRONMENT": "stage"}, true, "stage"},
		{"flag-on / env-preview", map[string]string{EnableOAuthTestProviderEnvVar: "1", "ENVIRONMENT": "preview"}, true, "preview"},
		{"flag-on / env-unset", map[string]string{EnableOAuthTestProviderEnvVar: "1"}, true, ""},
		{"flag-on / env-casing", map[string]string{EnableOAuthTestProviderEnvVar: "1", "ENVIRONMENT": "Test"}, true, "Test"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateOAuthTestProviderEnvironment(envFromMap(tc.env))
			if tc.wantError {
				if err == nil {
					t.Fatal("want error got nil — faux provider would be allowed to register!")
				}
				if !strings.Contains(err.Error(), EnableOAuthTestProviderEnvVar) {
					t.Errorf("error %q should name the flag", err.Error())
				}
				if tc.errContains != "" && !strings.Contains(err.Error(), tc.errContains) {
					t.Errorf("error %q missing %q", err.Error(), tc.errContains)
				}
			} else if err != nil {
				t.Errorf("want no error got %v", err)
			}
		})
	}
}

// TestTestProvider_Name asserts the clone registers/resolves as "google",
// not faux's default "faux" — UseProviders keys the registry on Name() and
// gothic looks it up by the "google" request param, so they must match.
func TestTestProvider_Name(t *testing.T) {
	p := newTestProvider()
	if got := p.Name(); got != TestProviderName {
		t.Errorf("Name() = %q, want %q", got, TestProviderName)
	}
	if TestProviderName != "google" {
		t.Errorf("TestProviderName = %q, want \"google\" (frontend initiates /auth/login/google)", TestProviderName)
	}
}

// TestTestProvider_FetchUser_FixedIdentity asserts the clone returns a
// deterministic, NON-EMPTY identity (the whole reason for cloning faux, whose
// FetchUser returns an empty email). The full begin->authorize->callback round
// trip is exercised by the e2e spec; here we drive FetchUser directly via an
// already-authorized faux session.
func TestTestProvider_FetchUser_FixedIdentity(t *testing.T) {
	p := newTestProvider()

	// faux.FetchUser errors unless the session carries an AccessToken (mirrors
	// a real provider pre-token-exchange). Begin then Authorize to get one,
	// exactly as gothic.CompleteUserAuth does.
	sess, err := p.BeginAuth("test-state")
	if err != nil {
		t.Fatalf("BeginAuth: %v", err)
	}
	if _, err := sess.Authorize(p, goth.Params(nil)); err != nil {
		t.Fatalf("Authorize: %v", err)
	}

	user, err := p.FetchUser(sess)
	if err != nil {
		t.Fatalf("FetchUser: %v", err)
	}
	if user.Email != TestProviderEmail {
		t.Errorf("Email = %q, want %q (empty email would break FindOrCreateUserWithConsent)", user.Email, TestProviderEmail)
	}
	if user.Email == "" {
		t.Error("Email is empty — the clone exists precisely to avoid faux's empty email")
	}
	if user.UserID != TestProviderUserID {
		t.Errorf("UserID = %q, want %q", user.UserID, TestProviderUserID)
	}
	if user.Provider != TestProviderName {
		t.Errorf("Provider = %q, want %q", user.Provider, TestProviderName)
	}
}

// TestTestProvider_FetchUser_RequiresAccessToken confirms the clone still
// honors faux's pre-authorize gate: FetchUser must fail before the session is
// Authorized. This keeps the e2e flow faithful to the real two-FetchUser dance
// in gothic.CompleteUserAuth.
func TestTestProvider_FetchUser_RequiresAccessToken(t *testing.T) {
	p := newTestProvider()
	sess, err := p.BeginAuth("test-state")
	if err != nil {
		t.Fatalf("BeginAuth: %v", err)
	}
	// No Authorize() -> no AccessToken -> FetchUser should error.
	if _, err := p.FetchUser(sess); err == nil {
		t.Error("FetchUser succeeded before Authorize; expected the faux access-token gate to fail")
	}
}
