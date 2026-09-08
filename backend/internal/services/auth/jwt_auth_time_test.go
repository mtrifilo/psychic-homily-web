package auth

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"psychic-homily-backend/internal/config"
	authm "psychic-homily-backend/internal/models/auth"
)

// The auth_at claim is what separates "this session authenticated recently"
// from "this session was minted recently". Renewal moves the second and must
// not move the first, or a caller holding only a stolen session can satisfy a
// re-authentication gate by asking for a new token.
//
// These drive ValidateSession, the reader every middleware calls, rather than
// the claim decode underneath it.

const authTimeTestSecret = "auth-time-test-secret-key-32-characters"

// authTimeTestUser is the principal every token here names.
func authTimeTestUser() *authm.User {
	return &authm.User{ID: 7, Email: stringPtr("factor@example.com"), IsActive: true}
}

func authTimeTestService(t *testing.T, secret string, expiryHours int64) *JWTService {
	t.Helper()
	cfg := &config.Config{
		JWT: config.JWTConfig{SecretKey: secret, Expiry: expiryHours},
	}
	return NewJWTService(nil, cfg, newStubUserService(authTimeTestUser()))
}

// claimsOf parses a token with the given secret and returns its claims without
// asserting expiry, so an expired token can still be inspected.
func claimsOf(t *testing.T, secret, token string) jwt.MapClaims {
	t.Helper()
	parser := jwt.NewParser(jwt.WithoutClaimsValidation())
	parsed, err := parser.Parse(token, func(*jwt.Token) (interface{}, error) {
		return []byte(secret), nil
	})
	require.NoError(t, err)
	claims, ok := parsed.Claims.(jwt.MapClaims)
	require.True(t, ok)
	return claims
}

// signSessionClaims mints a correctly-signed session token with claims chosen
// by the caller, which is how a shape the minting code never produces gets in
// front of the reader.
func signSessionClaims(t *testing.T, secret string, claims jwt.MapClaims) string {
	t.Helper()
	base := jwt.MapClaims{
		"user_id": 7,
		"exp":     time.Now().Add(time.Hour).Unix(),
		"iat":     time.Now().Unix(),
		"iss":     jwtIssuer,
		"aud":     jwtAudience,
		"sub":     jwtSessionSubject,
	}
	for k, v := range claims {
		base[k] = v
	}
	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, base).SignedString([]byte(secret))
	require.NoError(t, err)
	return signed
}

func TestValidateSession_CreateTokenStampsAuthTime(t *testing.T) {
	svc := authTimeTestService(t, authTimeTestSecret, 24)

	before := time.Now().Add(-time.Second)
	token, err := svc.CreateToken(authTimeTestUser())
	require.NoError(t, err)
	after := time.Now().Add(time.Second)

	require.Contains(t, claimsOf(t, authTimeTestSecret, token), "auth_at",
		"a factor mint must state when the factor completed")

	user, authAt, err := svc.ValidateSession(token)
	require.NoError(t, err)
	require.NotNil(t, user)
	assert.Equal(t, uint(7), user.ID)
	assert.False(t, authAt.Before(before))
	assert.False(t, authAt.After(after))
}

func TestValidateSession_RenewalCarriesAuthTimeThroughUnchanged(t *testing.T) {
	svc := authTimeTestService(t, authTimeTestSecret, 24)

	authAt := time.Now().Add(-3 * time.Hour).Truncate(time.Second)
	renewed, err := svc.RenewSessionToken(authTimeTestUser(), authAt)
	require.NoError(t, err)

	_, got, err := svc.ValidateSession(renewed)
	require.NoError(t, err)
	assert.True(t, got.Equal(authAt.UTC()), "renewal must not move auth_at")

	// iat is the claim renewal DOES move; the point of auth_at is that the two
	// now disagree.
	issuedAt := time.Unix(int64(claimsOf(t, authTimeTestSecret, renewed)["iat"].(float64)), 0)
	assert.True(t, issuedAt.After(got.Add(time.Hour)),
		"the renewed token is freshly issued but not freshly authenticated")
}

// A session that never carried the claim must not acquire one by being renewed.
func TestValidateSession_ZeroAuthTimeWritesNoClaim(t *testing.T) {
	svc := authTimeTestService(t, authTimeTestSecret, 24)

	bare, err := svc.RenewSessionToken(authTimeTestUser(), time.Time{})
	require.NoError(t, err)

	assert.NotContains(t, claimsOf(t, authTimeTestSecret, bare), "auth_at")

	// It is still an ordinary, usable session token: the claim gates the
	// security-relevant surfaces, not authentication itself.
	user, authAt, err := svc.ValidateSession(bare)
	require.NoError(t, err)
	require.NotNil(t, user)
	assert.Equal(t, uint(7), user.ID)
	assert.True(t, authAt.IsZero(), "absence of the claim is not evidence of freshness")

	renewedAgain, err := svc.RenewSessionToken(authTimeTestUser(), time.Time{})
	require.NoError(t, err)
	_, authAt, err = svc.ValidateSession(renewedAgain)
	require.NoError(t, err)
	assert.True(t, authAt.IsZero(), "renewing must not manufacture an auth time")
}

func TestValidateSession_RefusesTokensItCannotVouchFor(t *testing.T) {
	svc := authTimeTestService(t, authTimeTestSecret, 24)

	t.Run("garbage", func(t *testing.T) {
		_, _, err := svc.ValidateSession("not-a-jwt")
		assert.Error(t, err)
	})

	t.Run("empty", func(t *testing.T) {
		_, _, err := svc.ValidateSession("")
		assert.Error(t, err)
	})

	t.Run("signed with another secret", func(t *testing.T) {
		forged, err := authTimeTestService(t, "a-completely-different-secret-value", 24).
			CreateToken(authTimeTestUser())
		require.NoError(t, err)
		_, _, err = svc.ValidateSession(forged)
		assert.Error(t, err, "an auth time is only as good as the signature over it")
	})

	t.Run("expired", func(t *testing.T) {
		token, err := authTimeTestService(t, authTimeTestSecret, 0).CreateToken(authTimeTestUser())
		require.NoError(t, err)
		time.Sleep(1100 * time.Millisecond)
		_, _, err = svc.ValidateSession(token)
		assert.Error(t, err)
	})

	t.Run("user_id absent", func(t *testing.T) {
		signed := signSessionClaims(t, authTimeTestSecret, jwt.MapClaims{"user_id": nil})
		_, _, err := svc.ValidateSession(signed)
		assert.Error(t, err)
	})

	t.Run("user_id below one", func(t *testing.T) {
		signed := signSessionClaims(t, authTimeTestSecret, jwt.MapClaims{"user_id": -3})
		_, _, err := svc.ValidateSession(signed)
		assert.Error(t, err, "a negative id must not be converted to a uint and looked up")
	})
}

// Claim shapes the minting code never writes, which only a holder of the
// signing key could produce, but which the reader must still answer safely.
func TestValidateSession_ImplausibleAuthTimeEstablishesNone(t *testing.T) {
	svc := authTimeTestService(t, authTimeTestSecret, 24)

	cases := map[string]any{
		"epoch zero is an unset stamp, not a 1970 authentication": 0,
		"negative":                    -1,
		"a string is not a timestamp": "2026-09-07T00:00:00Z",
		"a bool is not a timestamp":   true,
		"null":                        nil,
		// Beyond maxAuthTimeSkew: a renewal would otherwise copy it forward
		// forever and every gate would accept the session for that long.
		"hours in the future":     time.Now().Add(4 * time.Hour).Unix(),
		"years in the future":     time.Now().AddDate(1000, 0, 0).Unix(),
		"beyond int64 as a float": 1e300,
	}

	for name, value := range cases {
		t.Run(name, func(t *testing.T) {
			signed := signSessionClaims(t, authTimeTestSecret, jwt.MapClaims{"auth_at": value})
			user, authAt, err := svc.ValidateSession(signed)
			require.NoError(t, err, "the token itself stays valid")
			require.NotNil(t, user)
			assert.True(t, authAt.IsZero())
		})
	}
}

// Inside the skew allowance a future stamp still counts, so a reader whose clock
// trails the minting clock by a moment does not refuse its own fresh sessions.
func TestValidateSession_SlightlyFutureAuthTimeIsBelieved(t *testing.T) {
	svc := authTimeTestService(t, authTimeTestSecret, 24)

	stamped := time.Now().Add(maxAuthTimeSkew / 2).Truncate(time.Second)
	signed := signSessionClaims(t, authTimeTestSecret, jwt.MapClaims{"auth_at": stamped.Unix()})

	_, authAt, err := svc.ValidateSession(signed)
	require.NoError(t, err)
	assert.True(t, authAt.Equal(stamped.UTC()))
}

// A single-purpose token (magic link, verification, recovery) must not be read
// as a session on the lenient path any more than on the strict one.
func TestValidateSessionLenient_RefusesCrossTypeTokens(t *testing.T) {
	svc := authTimeTestService(t, authTimeTestSecret, 24)

	magicLink, err := svc.CreateMagicLinkToken(7, "cross@example.com")
	require.NoError(t, err)

	_, _, err = svc.ValidateSessionLenient(magicLink, time.Hour)
	assert.Error(t, err)
}

// The lenient path reads a token that expired inside the grace window, which is
// the shape POST /auth/refresh receives. Losing the claim there would strip the
// standing from every session that renews late.
func TestValidateSessionLenient_ReadsAuthTimeAcrossExpiry(t *testing.T) {
	svc := authTimeTestService(t, authTimeTestSecret, 24)

	token, err := authTimeTestService(t, authTimeTestSecret, 0).CreateToken(authTimeTestUser())
	require.NoError(t, err)
	time.Sleep(1100 * time.Millisecond)

	_, _, err = svc.ValidateSession(token)
	require.Error(t, err, "the strict reader refuses an expired token")

	user, authAt, err := svc.ValidateSessionLenient(token, time.Hour)
	require.NoError(t, err)
	require.NotNil(t, user)
	assert.WithinDuration(t, time.Now(), authAt, 10*time.Second)

	_, _, err = svc.ValidateSessionLenient(token, time.Nanosecond)
	assert.Error(t, err, "past the grace period it is refused like any other expired token")
}
