package auth

import (
	"errors"
	"strings"
	"testing"

	"psychic-homily-backend/internal/config"
	apperrors "psychic-homily-backend/internal/errors"
)

const pendingTestSecret = "pending-signup-test-secret-at-least-32"

func signedPendingToken(secret, id string) string {
	return id + "." + signPendingSignupID(secret, id)
}

func TestVerifyPendingSignupToken(t *testing.T) {
	id := strings.Repeat("0a", oauthPendingSignupIDBytes)
	valid := signedPendingToken(pendingTestSecret, id)

	hash, err := verifyPendingSignupToken(pendingTestSecret, valid)
	if err != nil {
		t.Fatalf("a token this secret signed was refused: %v", err)
	}
	if hash != pendingSignupTokenHash(id) || hash == id {
		t.Errorf("hash = %q, want the SHA-256 of the id", hash)
	}

	for name, token := range map[string]string{
		"empty":               "",
		"no dot":              id,
		"no signature":        id + ".",
		"no id":               "." + signPendingSignupID(pendingTestSecret, id),
		"short id":            signedPendingToken(pendingTestSecret, "0a0a"),
		"signature flipped":   valid[:len(valid)-1] + "A",
		"another secret":      signedPendingToken("another-secret-at-least-32-characters", id),
		"another id":          strings.Repeat("0b", oauthPendingSignupIDBytes) + "." + signPendingSignupID(pendingTestSecret, id),
		"trailing separators": valid + ".",
	} {
		if _, err := verifyPendingSignupToken(pendingTestSecret, token); !errors.Is(err, errPendingSignupMalformed) {
			t.Errorf("%s: got %v, want errPendingSignupMalformed", name, err)
		}
	}

	if _, err := verifyPendingSignupToken("", valid); !errors.Is(err, errPendingSignupMalformed) {
		t.Errorf("an empty secret verified a token: %v", err)
	}
}

// A bad token is refused as an expired step before any query: this service
// has no database, so a query would fail with a different error.
func TestPeekAndConsumeRefuseABadTokenBeforeAnyQuery(t *testing.T) {
	svc := NewAuthService(nil, &config.Config{JWT: config.JWTConfig{SecretKey: pendingTestSecret}}, &nilDBUserService{})
	svc.db = nil

	reads := map[string]func(string) error{
		"peek":    func(token string) error { _, err := svc.PeekOAuthSignup(token); return err },
		"consume": func(token string) error { _, err := svc.ConsumeOAuthSignup(token); return err },
	}
	for name, read := range reads {
		var authErr *apperrors.AuthError
		if err := read("not-a-token"); !errors.As(err, &authErr) || authErr.Code != apperrors.CodeOAuthSignupExpired {
			t.Errorf("%s: got %v, want an OAUTH_SIGNUP_EXPIRED refusal", name, err)
		}
	}
}
