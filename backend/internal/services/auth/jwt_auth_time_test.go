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

func authTimeTestService(t *testing.T, expiryHours int64) (*JWTService, *config.Config) {
	t.Helper()
	cfg := &config.Config{
		JWT: config.JWTConfig{
			SecretKey: "auth-time-test-secret-key-32-characters",
			Expiry:    expiryHours,
		},
	}
	return NewJWTService(nil, cfg, newNilDBUserService()), cfg
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

func TestCreateToken_StampsAuthTime(t *testing.T) {
	svc, cfg := authTimeTestService(t, 24)
	user := &authm.User{ID: 7, Email: stringPtr("factor@example.com")}

	before := time.Now().Add(-time.Second)
	token, err := svc.CreateToken(user)
	require.NoError(t, err)
	after := time.Now().Add(time.Second)

	claims := claimsOf(t, cfg.JWT.SecretKey, token)
	require.Contains(t, claims, "auth_at", "a factor mint must state when the factor completed")

	authAt, ok := svc.SessionAuthTime(token)
	require.True(t, ok)
	assert.False(t, authAt.Before(before))
	assert.False(t, authAt.After(after))
}

func TestRenewSessionToken_CarriesAuthTimeThroughUnchanged(t *testing.T) {
	svc, cfg := authTimeTestService(t, 24)
	user := &authm.User{ID: 8, Email: stringPtr("renew@example.com")}

	authAt := time.Now().Add(-3 * time.Hour).Truncate(time.Second)
	renewed, err := svc.RenewSessionToken(user, authAt)
	require.NoError(t, err)

	got, ok := svc.SessionAuthTime(renewed)
	require.True(t, ok)
	assert.True(t, got.Equal(authAt.UTC()), "renewal must not move auth_at")

	// iat is the claim renewal DOES move; the point of auth_at is that the two
	// now disagree.
	claims := claimsOf(t, cfg.JWT.SecretKey, renewed)
	issuedAt := time.Unix(int64(claims["iat"].(float64)), 0)
	assert.True(t, issuedAt.After(got.Add(time.Hour)),
		"the renewed token is freshly issued but not freshly authenticated")
}

// A session that never carried the claim must not acquire one by being renewed.
// This is the migration case: every token minted before this change is such a
// session.
func TestRenewSessionToken_ZeroAuthTimeWritesNoClaim(t *testing.T) {
	svc, cfg := authTimeTestService(t, 24)
	user := &authm.User{ID: 9, Email: stringPtr("legacy@example.com")}

	legacy, err := svc.RenewSessionToken(user, time.Time{})
	require.NoError(t, err)

	claims := claimsOf(t, cfg.JWT.SecretKey, legacy)
	assert.NotContains(t, claims, "auth_at")

	_, ok := svc.SessionAuthTime(legacy)
	assert.False(t, ok, "absence of the claim is not evidence of freshness")

	// It is still an ordinary, usable session token: the claim gates the
	// security-relevant surfaces, not authentication itself.
	uid, ok := svc.SessionUserID(legacy)
	assert.True(t, ok)
	assert.Equal(t, uint(9), uid)

	renewedAgain, err := svc.RenewSessionToken(user, time.Time{})
	require.NoError(t, err)
	_, ok = svc.SessionAuthTime(renewedAgain)
	assert.False(t, ok, "renewing a legacy session must not manufacture an auth time")
}

func TestSessionAuthTime_RefusesTokensItCannotVouchFor(t *testing.T) {
	svc, _ := authTimeTestService(t, 24)
	user := &authm.User{ID: 10, Email: stringPtr("refuse@example.com")}

	t.Run("garbage", func(t *testing.T) {
		_, ok := svc.SessionAuthTime("not-a-jwt")
		assert.False(t, ok)
	})

	t.Run("empty", func(t *testing.T) {
		_, ok := svc.SessionAuthTime("")
		assert.False(t, ok)
	})

	t.Run("signed with another secret", func(t *testing.T) {
		other, _ := authTimeTestService(t, 24)
		other.config.JWT.SecretKey = "a-completely-different-secret-value"
		forged, err := other.CreateToken(user)
		require.NoError(t, err)
		_, ok := svc.SessionAuthTime(forged)
		assert.False(t, ok, "an auth time is only as good as the signature over it")
	})

	t.Run("expired", func(t *testing.T) {
		expiredSvc, _ := authTimeTestService(t, 0)
		expiredSvc.config.JWT.SecretKey = svc.config.JWT.SecretKey
		token, err := expiredSvc.CreateToken(user)
		require.NoError(t, err)
		time.Sleep(1100 * time.Millisecond)
		_, ok := svc.SessionAuthTime(token)
		assert.False(t, ok)
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
		signed, err := zeroStamped.SignedString([]byte(svc.config.JWT.SecretKey))
		require.NoError(t, err)
		_, ok := svc.SessionAuthTime(signed)
		assert.False(t, ok, "epoch zero is an unset stamp, not a 1970 authentication")
	})
}

// The refresh endpoint reads the presented token through the same grace period
// its validation uses. Without the lenient read, a session renewed after expiry
// would silently lose the claim and the user would be asked to sign in again
// for no reason.
func TestSessionAuthTimeLenient_ReadsAnExpiredTokenInsideTheGrace(t *testing.T) {
	svc, _ := authTimeTestService(t, 24)
	expiredSvc, _ := authTimeTestService(t, 0)
	expiredSvc.config.JWT.SecretKey = svc.config.JWT.SecretKey

	user := &authm.User{ID: 11, Email: stringPtr("grace@example.com")}
	token, err := expiredSvc.CreateToken(user)
	require.NoError(t, err)
	time.Sleep(1100 * time.Millisecond)

	_, ok := svc.SessionAuthTime(token)
	require.False(t, ok, "strict read refuses an expired token")

	authAt, ok := svc.SessionAuthTimeLenient(token, time.Hour)
	require.True(t, ok)
	assert.WithinDuration(t, time.Now(), authAt, 10*time.Second)

	_, ok = svc.SessionAuthTimeLenient(token, time.Nanosecond)
	assert.False(t, ok, "past the grace period it is refused like any other expired token")
}

// A single-purpose token (magic link, verification, recovery) caught inside the
// grace window must not be read as a session, on this path any more than on the
// validation path it shares.
func TestSessionAuthTimeLenient_RefusesCrossTypeTokens(t *testing.T) {
	svc, _ := authTimeTestService(t, 24)

	magicLink, err := svc.CreateMagicLinkToken(12, "cross@example.com")
	require.NoError(t, err)

	_, ok := svc.SessionAuthTimeLenient(magicLink, time.Hour)
	assert.False(t, ok)
}
