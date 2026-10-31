package auth

import (
	"errors"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apperrors "psychic-homily-backend/internal/errors"
	"psychic-homily-backend/internal/services/contracts"
)

// Every token this service mints carries exp, and jwt reads a token without
// one as never expiring. These tests sign tokens no minter produces, correct
// in every other respect, and require each reader to refuse them while it
// still accepts the same token with an exp.

const expiryRequiredTestSecret = "expiry-required-test-secret-32-chars!"

// signHS256 signs claims with secret the way every minter here does.
func signHS256(t *testing.T, secret string, claims jwt.Claims) string {
	t.Helper()
	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secret))
	require.NoError(t, err)
	return signed
}

// signSessionWithoutExp signs the session claim set for authTimeTestUser with
// no exp claim at all.
func signSessionWithoutExp(t *testing.T) string {
	t.Helper()
	claims := sessionTestClaims()
	delete(claims, "exp")
	return signHS256(t, expiryRequiredTestSecret, claims)
}

// requireAuthCode asserts err is an AuthError carrying code.
func requireAuthCode(t *testing.T, err error, code string) {
	t.Helper()
	var authErr *apperrors.AuthError
	require.True(t, errors.As(err, &authErr), "error %v is not an AuthError", err)
	assert.Equal(t, code, authErr.Code)
}

func TestValidateSession_RequiresExpiry(t *testing.T) {
	svc := authTimeTestService(t, expiryRequiredTestSecret, 24)

	user, _, err := svc.ValidateSession(signSessionClaims(t, expiryRequiredTestSecret, nil))
	require.NoError(t, err, "the control token with an exp was refused")
	require.Equal(t, uint(7), user.ID)

	// jwt reads an exp of zero in map claims as no exp at all.
	for name, token := range map[string]string{
		"no exp":      signSessionWithoutExp(t),
		"exp of zero": signSessionClaims(t, expiryRequiredTestSecret, jwt.MapClaims{"exp": 0}),
	} {
		t.Run(name, func(t *testing.T) {
			user, _, err := svc.ValidateSession(token)
			require.Error(t, err)
			assert.Nil(t, user)
			requireAuthCode(t, err, apperrors.CodeTokenInvalid)
			assert.ErrorIs(t, err, jwt.ErrTokenRequiredClaimMissing)

			_, ok := svc.SessionUserID(token)
			assert.False(t, ok, "SessionUserID honored a session with no expiry")
		})
	}
}

// The refresh path first tries the strict reader, which would honor a session
// with no exp if it did not require one; the lenient reader behind it checks
// exp by hand because it parses with claims validation off, and gives the
// strict reader's answer.
func TestValidateSessionLenient_RequiresExpiry(t *testing.T) {
	svc := authTimeTestService(t, expiryRequiredTestSecret, 24)

	for name, token := range map[string]string{
		"no exp":      signSessionWithoutExp(t),
		"exp of zero": signSessionClaims(t, expiryRequiredTestSecret, jwt.MapClaims{"exp": 0}),
	} {
		t.Run(name, func(t *testing.T) {
			user, _, err := svc.ValidateSessionLenient(token, 7*24*time.Hour)
			require.Error(t, err)
			assert.Nil(t, user)
			requireAuthCode(t, err, apperrors.CodeTokenInvalid)
			assert.Contains(t, err.Error(), "missing expiration claim", "the lenient reader did not refuse it")
		})
	}

	inGrace := signSessionClaims(t, expiryRequiredTestSecret, jwt.MapClaims{"exp": time.Now().Add(-time.Hour).Unix()})
	user, _, err := svc.ValidateSessionLenient(inGrace, 7*24*time.Hour)
	require.NoError(t, err, "the control token expired inside the grace period was refused")
	require.Equal(t, uint(7), user.ID)
}

// The single-purpose readers refuse a token with no exp, and accept the same
// token with one.
func TestSinglePurposeTokens_RequireExpiry(t *testing.T) {
	svc := authTimeTestService(t, expiryRequiredTestSecret, 24)

	registered := func(subject string, exp *jwt.NumericDate) jwt.RegisteredClaims {
		return jwt.RegisteredClaims{
			ExpiresAt: exp,
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Issuer:    jwtIssuer,
			Subject:   subject,
		}
	}

	cases := []struct {
		name string
		// claims builds the token's claims with exp, or with none when exp is nil.
		claims   func(exp *jwt.NumericDate) jwt.Claims
		validate func(token string) error
	}{
		{
			name: "verification",
			claims: func(exp *jwt.NumericDate) jwt.Claims {
				return contracts.VerificationTokenClaims{UserID: 7, Email: "factor@example.com", RegisteredClaims: registered("email-verification", exp)}
			},
			validate: func(token string) error { _, err := svc.ValidateVerificationToken(token); return err },
		},
		{
			name: "magic link",
			claims: func(exp *jwt.NumericDate) jwt.Claims {
				return contracts.MagicLinkTokenClaims{UserID: 7, Email: "factor@example.com", RegisteredClaims: registered("magic-link", exp)}
			},
			validate: func(token string) error { _, err := svc.ValidateMagicLinkToken(token); return err },
		},
		{
			name: "account recovery",
			claims: func(exp *jwt.NumericDate) jwt.Claims {
				return contracts.AccountRecoveryTokenClaims{UserID: 7, Email: "factor@example.com", RegisteredClaims: registered("account-recovery", exp)}
			},
			validate: func(token string) error { _, err := svc.ValidateAccountRecoveryToken(token); return err },
		},
		{
			name: "oauth link",
			claims: func(exp *jwt.NumericDate) jwt.Claims {
				return contracts.OAuthLinkTokenClaims{UserID: 7, Email: "factor@example.com", PendingLink: "ref-hash", RegisteredClaims: registered(oauthLinkTokenSubject, exp)}
			},
			validate: func(token string) error { _, err := svc.ValidateOAuthLinkToken(token); return err },
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			withExp := signHS256(t, expiryRequiredTestSecret, tc.claims(jwt.NewNumericDate(time.Now().Add(time.Hour))))
			require.NoError(t, tc.validate(withExp), "the control token with an exp was refused")

			err := tc.validate(signHS256(t, expiryRequiredTestSecret, tc.claims(nil)))
			require.Error(t, err)
			assert.ErrorIs(t, err, jwt.ErrTokenRequiredClaimMissing)
		})
	}
}
