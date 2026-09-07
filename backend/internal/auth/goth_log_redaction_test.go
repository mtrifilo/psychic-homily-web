package auth

import (
	"fmt"
	"strings"
	"testing"

	"github.com/markbates/goth"
	"github.com/markbates/goth/gothic"

	"psychic-homily-backend/internal/config"
	"psychic-homily-backend/internal/testlog"
)

// TestSetupGothNeverLogsTheOAuthSecret asserts the invariant that the OAuth
// bootstrap logs the LENGTH of cfg.OAuth.SecretKey and never its value. That
// key is the HMAC key the gothic cookie store signs the session with, so a
// reader of the log stream could forge a session cookie.
func TestSetupGothNeverLogsTheOAuthSecret(t *testing.T) {
	const sentinelOAuthSecret = "SENTINEL-OAUTH-SECRET-KEY-f13b8e0a"

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
	cfg.OAuth.SecretKey = sentinelOAuthSecret

	var setupErr error
	output := testlog.Capture(t, func() {
		setupErr = SetupGoth(cfg)
	})
	if setupErr != nil {
		t.Fatalf("SetupGoth: %v", setupErr)
	}

	if strings.Contains(output, sentinelOAuthSecret) {
		t.Errorf("SetupGoth logged the OAuth secret key; captured log:\n%s", output)
	}

	// Pin the rendered datum, not a stray word: an unrelated line containing
	// "length" must not satisfy this, and the assertion must fail when someone
	// drops the length rather than when someone rewords the message.
	wantLength := fmt.Sprintf("OAuth secret key length: %d", len(sentinelOAuthSecret))
	if !strings.Contains(output, wantLength) {
		t.Fatalf("captured log is missing %q, so either the length diagnostic was dropped or "+
			"testlog.Capture no longer intercepts this path and the assertion above is vacuous; "+
			"captured log:\n%s", wantLength, output)
	}
}
