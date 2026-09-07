package auth

import (
	"strings"
	"testing"

	"psychic-homily-backend/internal/config"
	"psychic-homily-backend/internal/testlog"
)

// TestSetupGothNeverLogsTheOAuthSecret asserts the invariant that the OAuth
// bootstrap logs the LENGTH of cfg.OAuth.SecretKey and never its value. That
// key is the HMAC key the gothic cookie store signs the session with, so a
// reader of the log stream could forge a session cookie.
func TestSetupGothNeverLogsTheOAuthSecret(t *testing.T) {
	const sentinelOAuthSecret = "SENTINEL-OAUTH-SECRET-KEY-f13b8e0a"

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
	if !strings.Contains(output, "length") {
		t.Errorf("expected the secret key LENGTH to stay in the log; captured log:\n%s", output)
	}
}
