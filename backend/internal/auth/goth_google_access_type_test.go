package auth

import (
	"net/url"
	"testing"

	"github.com/markbates/goth"
	"github.com/markbates/goth/gothic"

	"psychic-homily-backend/internal/config"
	"psychic-homily-backend/internal/testlog"
)

// TestSetupGothAsksGoogleForOnlineAccessOnly asserts that the Google
// authorization URL asks for online access, so Google issues no refresh
// token for a sign-in.
func TestSetupGothAsksGoogleForOnlineAccessOnly(t *testing.T) {
	// SetupGoth reads the process environment and assigns package-global
	// stores and the goth provider registry; pin the one and restore the rest.
	t.Setenv(EnableOAuthTestProviderEnvVar, "")
	prevSessionStore := SessionStore
	prevGothicStore := gothic.Store
	prevProviders := goth.GetProviders()
	t.Cleanup(func() {
		SessionStore = prevSessionStore
		gothic.Store = prevGothicStore
		goth.ClearProviders()
		for _, p := range prevProviders {
			goth.UseProviders(p)
		}
	})

	cfg := &config.Config{}
	cfg.OAuth.SecretKey = "test-oauth-secret-key"
	cfg.OAuth.GoogleClientID = "test-google-client-id"
	cfg.OAuth.GoogleClientSecret = "test-google-client-secret"
	cfg.OAuth.GoogleCallbackURL = "http://localhost:8080/auth/callback/google"

	var setupErr error
	testlog.Capture(t, func() {
		setupErr = SetupGoth(cfg)
	})
	if setupErr != nil {
		t.Fatalf("SetupGoth: %v", setupErr)
	}

	provider, err := goth.GetProvider("google")
	if err != nil {
		t.Fatalf("google provider not registered: %v", err)
	}
	session, err := provider.BeginAuth("test-state")
	if err != nil {
		t.Fatalf("BeginAuth: %v", err)
	}
	authURL, err := session.GetAuthURL()
	if err != nil {
		t.Fatalf("GetAuthURL: %v", err)
	}
	parsed, err := url.Parse(authURL)
	if err != nil {
		t.Fatalf("parse %q: %v", authURL, err)
	}

	if got := parsed.Query()["access_type"]; len(got) != 1 || got[0] != "online" {
		t.Errorf("access_type = %q, want exactly [\"online\"]; authorization URL: %s", got, authURL)
	}
}
