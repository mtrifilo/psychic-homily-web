package auth

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	autherrors "psychic-homily-backend/internal/errors"
)

// assertFrontendSpells requires every one of values to appear, single-quoted,
// in the frontend module at frontendPath (relative to the repo root). noun
// and reader name the values and their frontend reader in the failure.
func assertFrontendSpells(t *testing.T, frontendPath, noun, reader string, values []string) {
	t.Helper()
	repoRoot, err := filepath.Abs("../../../../..")
	if err != nil {
		t.Fatalf("resolving the repo root: %v", err)
	}
	raw, err := os.ReadFile(filepath.Join(repoRoot, frontendPath))
	if err != nil {
		// A moved frontend module is a broken assumption, not a pass.
		t.Fatalf("reading %s: %v", frontendPath, err)
	}
	content := string(raw)

	if len(values) == 0 {
		t.Fatalf("no %ss to check", noun)
	}
	for _, value := range values {
		if !strings.Contains(content, "'"+value+"'") {
			t.Errorf("%s does not spell the %s %q. %s; rename it on both sides or neither.",
				frontendPath, noun, value, reader)
		}
	}
}

// The auth page shows fixed copy keyed by the code a redirect carries, and the
// code is the only thing the redirect says, so a code renamed on one side
// leaves the page on its generic line with nothing failing. This checks that
// every code a redirect to the auth page can carry is spelled, quoted, in the
// frontend's code list.
func TestAuthPageCodesMatchTheFrontend(t *testing.T) {
	assertFrontendSpells(t, "frontend/lib/errors/authErrors.ts", "code", "The auth page reads this code", []string{
		OAuthRedirectRateLimitedCode,
		authPageCodeOAuthCancelled,
		authPageCodeOAuthLinkPending,
		autherrors.CodeUnknown,
		autherrors.CodeTermsAcceptanceRequired,
		autherrors.CodeAgeConfirmationRequired,
		autherrors.CodeUserExists,
		autherrors.CodeOAuthLinkRefused,
		autherrors.CodeAccountInactive,
		autherrors.CodeAccountRecoverable,
		autherrors.CodeValidationFailed,
	})
}

// A sign-in start records only the reasons oauthReauthReasons lists, and the
// auth page writes them from REAUTH_REASONS, so a reason renamed on one side
// is dropped at the start and a refused re-authentication bounces again with
// nothing failing. This checks every listed reason is spelled, quoted, in the
// module that defines the frontend's.
func TestReauthReasonsMatchTheFrontend(t *testing.T) {
	assertFrontendSpells(t, "frontend/lib/auth-href.ts", "reason", "The Google start records this reason", oauthReauthReasons)
}
