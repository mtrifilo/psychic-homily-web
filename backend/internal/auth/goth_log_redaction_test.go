package auth

import (
	"bytes"
	"log"
	"strings"
	"testing"

	"psychic-homily-backend/internal/config"
)

// TestSetupGothNeverLogsTheOAuthSecret pins the OAuth bootstrap against logging
// the value of cfg.OAuth.SecretKey. That key is the HMAC key the gothic cookie
// store signs the OAuth session with, so a reader of the log stream could forge
// a session cookie. Its length stays loggable.
func TestSetupGothNeverLogsTheOAuthSecret(t *testing.T) {
	const sentinelOAuthSecret = "SENTINEL-OAUTH-SECRET-KEY-f13b8e0a"

	var buf bytes.Buffer
	prevWriter := log.Writer()
	prevFlags := log.Flags()
	defer func() {
		log.SetOutput(prevWriter)
		log.SetFlags(prevFlags)
	}()
	log.SetOutput(&buf)
	log.SetFlags(0)

	cfg := &config.Config{}
	cfg.OAuth.SecretKey = sentinelOAuthSecret

	if err := SetupGoth(cfg); err != nil {
		t.Fatalf("SetupGoth: %v", err)
	}

	output := buf.String()
	if strings.Contains(output, sentinelOAuthSecret) {
		t.Errorf("SetupGoth logged the OAuth secret key; captured log:\n%s", output)
	}
	if !strings.Contains(output, "length") {
		t.Errorf("expected the secret key LENGTH to stay in the log; captured log:\n%s", output)
	}
}
