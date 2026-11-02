package errors

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The Settings panel renders its own copy for the email-verification refusal,
// keyed on the code the link redirect carries, so nothing in a build ties the
// panel's entry to this package. This reads the panel and asserts the code
// and the sentence appear in it verbatim; it fails on a rename or a copy edit
// on one side only.
func TestEmailVerificationRefusalMatchesTheSettingsPanel(t *testing.T) {
	repoRoot, err := filepath.Abs("../../..")
	if err != nil {
		t.Fatalf("resolving the repo root: %v", err)
	}
	const file = "frontend/features/auth/components/settings/oauth-accounts.tsx"
	content, err := os.ReadFile(filepath.Join(repoRoot, file))
	if err != nil {
		// A moved frontend module is a broken assumption, not a pass.
		t.Fatalf("reading %s: %v", file, err)
	}
	for _, want := range []string{"'" + CodeEmailVerificationRequired + "'", emailVerificationRequiredMessage} {
		if !strings.Contains(string(content), want) {
			t.Errorf("%s does not contain %q. The panel and this package must name the same "+
				"code and sentence for CodeEmailVerificationRequired: change both, or neither.", file, want)
		}
	}
}
