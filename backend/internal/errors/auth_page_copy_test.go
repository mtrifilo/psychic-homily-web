package errors

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// frontendAuthRefusalCopy is the frontend module holding the fixed copy the
// auth page shows for a refusal code, relative to the repo root.
const frontendAuthRefusalCopy = "frontend/lib/errors/authRefusalCopy.ts"

// The auth page renders fixed copy for each refusal code and never the text a
// redirect could carry. For these refusals that copy is this package's own
// sentence, repeated in the frontend module, so the page says what every other
// client of the code is told. Nothing in a build checks the two spellings
// agree; this does, by finding each sentence verbatim as a string literal.
func TestAuthPageRefusalCopyMatchesTheFrontend(t *testing.T) {
	repoRoot, err := filepath.Abs("../../..")
	if err != nil {
		t.Fatalf("resolving the repo root: %v", err)
	}
	content, err := os.ReadFile(filepath.Join(repoRoot, frontendAuthRefusalCopy))
	if err != nil {
		// A moved frontend module is a broken assumption, not a pass.
		t.Fatalf("reading %s: %v", frontendAuthRefusalCopy, err)
	}

	for _, refusal := range []*AuthError{
		ErrOAuthLinkRefused("someone@example.com"),
		ErrAccountInactive(),
		ErrProviderEmailUnstorable(),
		ErrUserExists("someone@example.com"),
		ErrTermsAcceptanceRequired("detail"),
		ErrAgeConfirmationRequired("detail"),
	} {
		// The frontend writes every one of these in single quotes.
		literal := "'" + refusal.UserMessage() + "'"
		if !strings.Contains(string(content), literal) {
			t.Errorf("%s does not contain %s's sentence %s.\n"+
				"The auth page shows this package's sentence for the code as fixed copy:\n"+
				"change both, or neither.",
				frontendAuthRefusalCopy, refusal.Code, strconv.Quote(refusal.UserMessage()))
		}
	}
}

// The owner's sentence for an age refusal, verbatim. It names no checkbox and
// gives no verdict on the person's age, because the code also answers a
// sign-up that sent no date of birth.
func TestAgeConfirmationRequiredMessage(t *testing.T) {
	const want = "Your account was not created. Psychic Homily accounts are for people 16 and over."
	if got := ErrAgeConfirmationRequired("detail").UserMessage(); got != want {
		t.Errorf("message = %q, want %q", got, want)
	}
}
