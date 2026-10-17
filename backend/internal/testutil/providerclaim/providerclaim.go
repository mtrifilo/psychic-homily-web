// Package providerclaim holds the identity-provider email claims Postgres
// rejects as text, and the assertion every sign-in path's refusal of them
// must pass.
package providerclaim

import (
	"errors"
	"strings"
	"testing"

	apperrors "psychic-homily-backend/internal/errors"
)

// UnstorableEmails are email claims Postgres rejects as text, keyed by why.
var UnstorableEmails = map[string]string{
	"NUL":           "no\x00body@example.com",
	"invalid UTF-8": "no\xffbody@example.com",
}

// RequireUnstorableEmailRefusal asserts err is the validation refusal for
// claim, and that its text, which callers log, does not carry the claim.
func RequireUnstorableEmailRefusal(t *testing.T, err error, claim string) {
	t.Helper()
	var refusal *apperrors.AuthError
	if !errors.As(err, &refusal) {
		t.Fatalf("error = %v, want an *AuthError refusal", err)
	}
	if refusal.Code != apperrors.CodeValidationFailed {
		t.Fatalf("refusal code = %q, want %q", refusal.Code, apperrors.CodeValidationFailed)
	}
	if strings.Contains(err.Error(), claim) {
		t.Fatalf("refusal text carries the claim: %q", err.Error())
	}
}
