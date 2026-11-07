package auth

import (
	"os"
	"strings"
	"testing"
)

// Every session cookie this package sets comes from Config.NewSessionCookie,
// which takes no lifetime, so no mint site can re-specify one. The handler
// tests assert the lifetime on some paths; this guard covers every file: a
// handler that names the auth cookie is building or clearing it by hand.
// (Clearing goes through middleware.RefuseSession.)
func TestSessionCookieMintedOnlyByConfig(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		body, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		src := string(body)
		if strings.Contains(src, "config.AuthCookieName") || strings.Contains(src, `"auth_token"`) {
			t.Errorf("%s names the auth cookie; mint it with h.config.NewSessionCookie and clear it with middleware.RefuseSession", name)
		}
	}
}
