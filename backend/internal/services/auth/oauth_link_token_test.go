package auth

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"psychic-homily-backend/internal/config"
	"psychic-homily-backend/internal/services/contracts"
)

func linkTokenTestService() *JWTService {
	return NewJWTService(nil, &config.Config{
		JWT: config.JWTConfig{SecretKey: "oauth-link-token-test-secret-32chars!", Expiry: 24},
	}, nil)
}

func TestOAuthLinkToken_RoundTrip(t *testing.T) {
	svc := linkTokenTestService()
	token, err := svc.CreateOAuthLinkToken(7, "alice@example.com", "ref-hash")
	if err != nil {
		t.Fatal(err)
	}
	claims, err := svc.ValidateOAuthLinkToken(token)
	if err != nil {
		t.Fatal(err)
	}
	if claims.UserID != 7 || claims.Email != "alice@example.com" || claims.PendingLink != "ref-hash" {
		t.Errorf("claims = %+v", claims)
	}
	lifetime := claims.ExpiresAt.Sub(claims.IssuedAt.Time)
	if lifetime != contracts.MagicLinkTokenTTL {
		t.Errorf("lifetime = %v, want %v", lifetime, contracts.MagicLinkTokenTTL)
	}
}

// Neither validator accepts the other's token: a plain magic link must never
// complete a link, and a link token must never sign in without one.
func TestOAuthLinkToken_IsNotAMagicLinkOrASession(t *testing.T) {
	svc := linkTokenTestService()
	link, err := svc.CreateOAuthLinkToken(7, "alice@example.com", "ref-hash")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ValidateMagicLinkToken(link); err == nil {
		t.Error("a link token validated as a plain magic link")
	}
	if _, err := svc.parseSessionToken(link); err == nil {
		t.Error("a link token parsed as a session")
	}

	plain, err := svc.CreateMagicLinkToken(7, "alice@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ValidateOAuthLinkToken(plain); err == nil {
		t.Error("a plain magic link validated as a link token")
	}
}

func TestOAuthLinkToken_Refusals(t *testing.T) {
	svc := linkTokenTestService()
	sign := func(claims contracts.OAuthLinkTokenClaims, secret string) string {
		t.Helper()
		token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secret))
		if err != nil {
			t.Fatal(err)
		}
		return token
	}
	valid := func() contracts.OAuthLinkTokenClaims {
		return contracts.OAuthLinkTokenClaims{
			UserID:      7,
			Email:       "alice@example.com",
			PendingLink: "ref-hash",
			RegisteredClaims: jwt.RegisteredClaims{
				ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Minute)),
				Issuer:    jwtIssuer,
				Subject:   oauthLinkTokenSubject,
			},
		}
	}
	secret := svc.config.JWT.SecretKey

	expired := valid()
	expired.ExpiresAt = jwt.NewNumericDate(time.Now().Add(-time.Second))
	noExpiry := valid()
	noExpiry.ExpiresAt = nil
	noRef := valid()
	noRef.PendingLink = ""
	noUser := valid()
	noUser.UserID = 0
	wrongIssuer := valid()
	wrongIssuer.Issuer = "someone-else"

	for name, token := range map[string]string{
		"expired":        sign(expired, secret),
		"no expiry":      sign(noExpiry, secret),
		"no reference":   sign(noRef, secret),
		"no account":     sign(noUser, secret),
		"wrong issuer":   sign(wrongIssuer, secret),
		"another secret": sign(valid(), "another-secret-of-at-least-32-chars!"),
		"garbage":        "not-a-token",
	} {
		if _, err := svc.ValidateOAuthLinkToken(token); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	if _, err := svc.ValidateOAuthLinkToken(sign(valid(), secret)); err != nil {
		t.Errorf("the control token was refused: %v", err)
	}
}
