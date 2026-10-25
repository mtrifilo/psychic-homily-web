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
	return id + "." + signPendingID(secret, oauthPendingSignupDomain, id)
}

func TestVerifyPendingSignupToken(t *testing.T) {
	id := strings.Repeat("0a", oauthPendingSignupIDBytes)
	valid := signedPendingToken(pendingTestSecret, id)

	hash, err := verifyPendingToken(pendingTestSecret, oauthPendingSignupDomain, valid)
	if err != nil {
		t.Fatalf("a token this secret signed was refused: %v", err)
	}
	if hash != pendingTokenHash(id) || hash == id {
		t.Errorf("hash = %q, want the SHA-256 of the id", hash)
	}

	for name, token := range map[string]string{
		"empty":               "",
		"no dot":              id,
		"no signature":        id + ".",
		"no id":               "." + signPendingID(pendingTestSecret, oauthPendingSignupDomain, id),
		"short id":            signedPendingToken(pendingTestSecret, "0a0a"),
		"signature flipped":   valid[:len(valid)-1] + "A",
		"another secret":      signedPendingToken("another-secret-at-least-32-characters", id),
		"another id":          strings.Repeat("0b", oauthPendingSignupIDBytes) + "." + signPendingID(pendingTestSecret, oauthPendingSignupDomain, id),
		"trailing separators": valid + ".",
	} {
		if _, err := verifyPendingToken(pendingTestSecret, oauthPendingSignupDomain, token); !errors.Is(err, errPendingMalformed) {
			t.Errorf("%s: got %v, want errPendingMalformed", name, err)
		}
	}

	if _, err := verifyPendingToken("", oauthPendingSignupDomain, valid); !errors.Is(err, errPendingMalformed) {
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
