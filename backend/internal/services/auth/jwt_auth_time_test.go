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

const authTimeTestSecret = "auth-time-test-secret-key-32-characters"

func authTimeTestService(t *testing.T, secret string, expiryHours int64) *JWTService {
	t.Helper()
	cfg := &config.Config{
		JWT: config.JWTConfig{SecretKey: secret, Expiry: expiryHours},
	}
	return NewJWTService(nil, cfg, newNilDBUserService())
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

// sessionAuthTime is what the middlewares get from ValidateSession, without the
// database lookup those tests would otherwise need.
func sessionAuthTime(t *testing.T, svc *JWTService, token string) (time.Time, error) {
	t.Helper()
	claims, err := svc.parseSessionToken(token)
	if err != nil {
		return time.Time{}, err
	}
	return authTimeFromClaims(claims), nil
}

func TestCreateToken_StampsAuthTime(t *testing.T) {
	svc := authTimeTestService(t, authTimeTestSecret, 24)
	user := &authm.User{ID: 7, Email: stringPtr("factor@example.com")}

	before := time.Now().Add(-time.Second)
	token, err := svc.CreateToken(user)
	require.NoError(t, err)
	after := time.Now().Add(time.Second)

	require.Contains(t, claimsOf(t, authTimeTestSecret, token), "auth_at",
		"a factor mint must state when the factor completed")

	authAt, err := sessionAuthTime(t, svc, token)
	require.NoError(t, err)
	assert.False(t, authAt.Before(before))
	assert.False(t, authAt.After(after))
}

func TestRenewSessionToken_CarriesAuthTimeThroughUnchanged(t *testing.T) {
	svc := authTimeTestService(t, authTimeTestSecret, 24)
	user := &authm.User{ID: 8, Email: stringPtr("renew@example.com")}

	authAt := time.Now().Add(-3 * time.Hour).Truncate(time.Second)
	renewed, err := svc.RenewSessionToken(user, authAt)
	require.NoError(t, err)

	got, err := sessionAuthTime(t, svc, renewed)
	require.NoError(t, err)
	assert.True(t, got.Equal(authAt.UTC()), "renewal must not move auth_at")

	// iat is the claim renewal DOES move; the point of auth_at is that the two
	// now disagree.
	issuedAt := time.Unix(int64(claimsOf(t, authTimeTestSecret, renewed)["iat"].(float64)), 0)
	assert.True(t, issuedAt.After(got.Add(time.Hour)),
		"the renewed token is freshly issued but not freshly authenticated")
}

// A session that never carried the claim must not acquire one by being renewed.
// This is the migration case: every token minted before this change is such a
// session.
func TestRenewSessionToken_ZeroAuthTimeWritesNoClaim(t *testing.T) {
	svc := authTimeTestService(t, authTimeTestSecret, 24)
	user := &authm.User{ID: 9, Email: stringPtr("legacy@example.com")}

	legacy, err := svc.RenewSessionToken(user, time.Time{})
	require.NoError(t, err)

	assert.NotContains(t, claimsOf(t, authTimeTestSecret, legacy), "auth_at")

	authAt, err := sessionAuthTime(t, svc, legacy)
	require.NoError(t, err)
	assert.True(t, authAt.IsZero(), "absence of the claim is not evidence of freshness")

	// It is still an ordinary, usable session token: the claim gates the
	// security-relevant surfaces, not authentication itself.
	uid, ok := svc.SessionUserID(legacy)
	assert.True(t, ok)
	assert.Equal(t, uint(9), uid)

	renewedAgain, err := svc.RenewSessionToken(user, time.Time{})
	require.NoError(t, err)
	authAt, err = sessionAuthTime(t, svc, renewedAgain)
	require.NoError(t, err)
	assert.True(t, authAt.IsZero(), "renewing a legacy session must not manufacture an auth time")
}

func TestSessionAuthTime_RefusesTokensItCannotVouchFor(t *testing.T) {
	svc := authTimeTestService(t, authTimeTestSecret, 24)
	user := &authm.User{ID: 10, Email: stringPtr("refuse@example.com")}

	t.Run("garbage", func(t *testing.T) {
		_, err := sessionAuthTime(t, svc, "not-a-jwt")
		assert.Error(t, err)
	})

	t.Run("empty", func(t *testing.T) {
		_, err := sessionAuthTime(t, svc, "")
		assert.Error(t, err)
	})

	t.Run("signed with another secret", func(t *testing.T) {
		forged, err := authTimeTestService(t, "a-completely-different-secret-value", 24).CreateToken(user)
		require.NoError(t, err)
		_, err = sessionAuthTime(t, svc, forged)
		assert.Error(t, err, "an auth time is only as good as the signature over it")
	})

	t.Run("expired", func(t *testing.T) {
		token, err := authTimeTestService(t, authTimeTestSecret, 0).CreateToken(user)
		require.NoError(t, err)
		time.Sleep(1100 * time.Millisecond)
		_, err = sessionAuthTime(t, svc, token)
		assert.Error(t, err)
	})

	t.Run("non-positive claim value", func(t *testing.T) {
		zeroStamped := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
			"user_id": 10,
			"exp":     time.Now().Add(time.Hour).Unix(),
			"iat":     time.Now().Unix(),
			"iss":     jwtIssuer,
			"aud":     jwtAudience,
			"sub":     jwtSessionSubject,
			"auth_at": 0,
		})
		signed, err := zeroStamped.SignedString([]byte(authTimeTestSecret))
		require.NoError(t, err)
		authAt, err := sessionAuthTime(t, svc, signed)
		require.NoError(t, err)
		assert.True(t, authAt.IsZero(), "epoch zero is an unset stamp, not a 1970 authentication")
	})
}

// A single-purpose token (magic link, verification, recovery) must not be read
// as a session on the lenient path any more than on the strict one.
func TestParseSessionTokenLenient_RefusesCrossTypeTokens(t *testing.T) {
	svc := authTimeTestService(t, authTimeTestSecret, 24)

	magicLink, err := svc.CreateMagicLinkToken(12, "cross@example.com")
	require.NoError(t, err)

	_, err = svc.parseSessionTokenLenient(magicLink, time.Hour)
	assert.Error(t, err)
}

// The lenient path reads a token that expired inside the grace window, which is
// the shape POST /auth/refresh receives. Losing the claim there would strip the
// standing from every session that renews late.
func TestParseSessionTokenLenient_ReadsAuthTimeAcrossExpiry(t *testing.T) {
	svc := authTimeTestService(t, authTimeTestSecret, 24)

	user := &authm.User{ID: 11, Email: stringPtr("grace@example.com")}
	token, err := authTimeTestService(t, authTimeTestSecret, 0).CreateToken(user)
	require.NoError(t, err)
	time.Sleep(1100 * time.Millisecond)

	_, err = svc.parseSessionToken(token)
	require.Error(t, err, "the strict parse refuses an expired token")

	claims, err := svc.parseSessionTokenLenient(token, time.Hour)
	require.NoError(t, err)
	assert.WithinDuration(t, time.Now(), authTimeFromClaims(claims), 10*time.Second)

	_, err = svc.parseSessionTokenLenient(token, time.Nanosecond)
	assert.Error(t, err, "past the grace period it is refused like any other expired token")
}
