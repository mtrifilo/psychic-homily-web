package auth

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	autherrors "psychic-homily-backend/internal/errors"
)

// The auth page shows fixed copy keyed by the code a redirect carries, and the
// code is the only thing the redirect says, so a code renamed on one side
// leaves the page on its generic line with nothing failing. This checks that
// every code a redirect to the auth page can carry is spelled, quoted, in the
// frontend's code list.
func TestAuthPageCodesMatchTheFrontend(t *testing.T) {
	const frontendCodes = "frontend/lib/errors/authErrors.ts"
	repoRoot, err := filepath.Abs("../../../../..")
	if err != nil {
		t.Fatalf("resolving the repo root: %v", err)
	}
	raw, err := os.ReadFile(filepath.Join(repoRoot, frontendCodes))
	if err != nil {
		// A moved frontend module is a broken assumption, not a pass.
		t.Fatalf("reading %s: %v", frontendCodes, err)
	}
	content := string(raw)

	for _, code := range []string{
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
	} {
		if !strings.Contains(content, "'"+code+"'") {
			t.Errorf("%s does not spell the code %q. The auth page reads this code; "+
				"rename it on both sides or neither.", frontendCodes, code)
		}
	}
}
