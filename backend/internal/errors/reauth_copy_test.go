package errors

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The re-authentication refusal is spelled on both sides of the wire: the code
// travels in the body, and the browser surfaces render copy they own rather
// than the server's sentence. Nothing in a build or a type checks that the two
// halves still name the same thing, so renaming the Go constant would leave
// every surface falling back to generic failure copy with no test failing.
//
// This is that check. It reads the frontend module and asserts the code and the
// sentence appear in it verbatim. It fails on a rename of either, which is the
// moment to change the other.
func TestReauthRefusalMatchesTheFrontend(t *testing.T) {
	repoRoot, err := filepath.Abs("../../..")
	if err != nil {
		t.Fatalf("resolving the repo root: %v", err)
	}

	for _, tc := range []struct {
		file string
		want string
		what string
	}{
		{"frontend/lib/errors/authErrors.ts", CodeReauthRequired, "the error code the body carries"},
		{"frontend/lib/errors/authErrors.ts", reauthRequiredMessage, "the refusal sentence"},
		{"frontend/lib/auth-href.ts", CodeReauthRequired, "the reason the sign-in link carries"},
	} {
		path := filepath.Join(repoRoot, tc.file)
		content, err := os.ReadFile(path)
		if err != nil {
			// A moved frontend module is a broken assumption, not a pass.
			t.Fatalf("reading %s: %v", tc.file, err)
		}
		if !strings.Contains(string(content), tc.want) {
			t.Errorf("%s does not contain %s (%q).\n"+
				"The two sides of this refusal have to name the same thing: change both, or\n"+
				"neither. See CodeReauthRequired and reauthRequiredMessage in this package.",
				tc.file, tc.what, tc.want)
		}
	}
}
