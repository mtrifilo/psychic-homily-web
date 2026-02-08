package services

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"psychic-homily-backend/internal/config"
)

// TestValidateTokenLenient tests the lenient token validation for refresh flows
func TestValidateTokenLenient(t *testing.T) {
	secretKey := "test-secret-key-lenient-tests"
	cfg := &config.Config{
		JWT: config.JWTConfig{
			SecretKey: secretKey,
			Expiry:    24,
		},
	}
	jwtService := NewJWTService(nil, cfg)
	gracePeriod := 7 * 24 * time.Hour // 7 days

	t.Run("valid_token_passes_strict", func(t *testing.T) {
		// A non-expired token should pass through strict validation
		token := createTestToken(t, secretKey, 123, "test@example.com", time.Now().Add(1*time.Hour))

		// ValidateTokenLenient calls ValidateToken first, which needs DB access.
		// Since we have no DB, it will fail on the user lookup even if the token parses.
		// This tests that the lenient path handles the fallback correctly.
		_, err := jwtService.ValidateTokenLenient(token, gracePeriod)
		// We expect an error because there's no DB, but not a token-related error
		assert.Error(t, err)
		// The error should be about the database, not about token validity
		assert.Contains(t, err.Error(), "failed to get user")
	})

	t.Run("expired_within_grace_period_passes_parsing", func(t *testing.T) {
		// Token expired 1 hour ago — well within 7-day grace period
		token := createTestToken(t, secretKey, 456, "grace@example.com", time.Now().Add(-1*time.Hour))

		_, err := jwtService.ValidateTokenLenient(token, gracePeriod)
		// Should get past token validation to the user lookup stage
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get user")
	})

	t.Run("expired_beyond_grace_period_rejected", func(t *testing.T) {
		// Token expired 8 days ago — beyond 7-day grace period
		token := createTestToken(t, secretKey, 789, "expired@example.com", time.Now().Add(-8*24*time.Hour))

		_, err := jwtService.ValidateTokenLenient(token, gracePeriod)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "expired beyond grace period")
	})

	t.Run("expired_at_exact_grace_boundary_rejected", func(t *testing.T) {
		// Token expired exactly 7 days + 1 second ago
		token := createTestToken(t, secretKey, 100, "boundary@example.com", time.Now().Add(-gracePeriod-1*time.Second))

		_, err := jwtService.ValidateTokenLenient(token, gracePeriod)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "expired beyond grace period")
	})

	t.Run("invalid_signature_rejected", func(t *testing.T) {
		// Token signed with different key
		wrongKey := "wrong-secret-key-12345678"
		token := createTestToken(t, wrongKey, 123, "wrong@example.com", time.Now().Add(-1*time.Hour))

		_, err := jwtService.ValidateTokenLenient(token, gracePeriod)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "TOKEN_INVALID")
	})

	t.Run("malformed_token_rejected", func(t *testing.T) {
		_, err := jwtService.ValidateTokenLenient("not-a-jwt", gracePeriod)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "TOKEN_INVALID")
	})

	t.Run("empty_token_rejected", func(t *testing.T) {
		_, err := jwtService.ValidateTokenLenient("", gracePeriod)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "TOKEN_INVALID")
	})

	t.Run("wrong_issuer_rejected", func(t *testing.T) {
		// Token with wrong issuer
		claims := jwt.MapClaims{
			"user_id": float64(123),
			"email":   "wrong-iss@example.com",
			"exp":     time.Now().Add(-1 * time.Hour).Unix(),
			"iat":     time.Now().Add(-2 * time.Hour).Unix(),
			"iss":     "wrong-issuer",
			"aud":     "psychic-homily-users",
		}
		token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
		tokenStr, err := token.SignedString([]byte(secretKey))
		require.NoError(t, err)

		_, err = jwtService.ValidateTokenLenient(tokenStr, gracePeriod)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid token issuer or audience")
	})

	t.Run("wrong_audience_rejected", func(t *testing.T) {
		// Token with wrong audience
		claims := jwt.MapClaims{
			"user_id": float64(123),
			"email":   "wrong-aud@example.com",
			"exp":     time.Now().Add(-1 * time.Hour).Unix(),
			"iat":     time.Now().Add(-2 * time.Hour).Unix(),
			"iss":     "psychic-homily-backend",
			"aud":     "wrong-audience",
		}
		token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
		tokenStr, err := token.SignedString([]byte(secretKey))
		require.NoError(t, err)

		_, err = jwtService.ValidateTokenLenient(tokenStr, gracePeriod)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid token issuer or audience")
	})

	t.Run("missing_exp_claim_rejected", func(t *testing.T) {
		// Token with no exp claim
		claims := jwt.MapClaims{
			"user_id": float64(123),
			"email":   "no-exp@example.com",
			"iat":     time.Now().Unix(),
			"iss":     "psychic-homily-backend",
			"aud":     "psychic-homily-users",
		}
		token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
		tokenStr, err := token.SignedString([]byte(secretKey))
		require.NoError(t, err)

		_, err = jwtService.ValidateTokenLenient(tokenStr, gracePeriod)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "missing expiration claim")
	})

	t.Run("recently_expired_1_day_passes", func(t *testing.T) {
		// Token expired 1 day ago — within grace period
		token := createTestToken(t, secretKey, 555, "oneday@example.com", time.Now().Add(-24*time.Hour))

		_, err := jwtService.ValidateTokenLenient(token, gracePeriod)
		assert.Error(t, err)
		// Should fail at user lookup, not at token validation
		assert.Contains(t, err.Error(), "failed to get user")
	})

	t.Run("recently_expired_6_days_passes", func(t *testing.T) {
		// Token expired 6 days ago — within grace period
		token := createTestToken(t, secretKey, 666, "sixdays@example.com", time.Now().Add(-6*24*time.Hour))

		_, err := jwtService.ValidateTokenLenient(token, gracePeriod)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get user")
	})
}

// createTestToken creates a JWT with specific expiration for testing
func createTestToken(t *testing.T, secretKey string, userID int, email string, exp time.Time) string {
	t.Helper()
	claims := jwt.MapClaims{
		"user_id": float64(userID),
		"email":   email,
		"exp":     exp.Unix(),
		"iat":     time.Now().Add(-2 * time.Hour).Unix(),
		"iss":     "psychic-homily-backend",
		"aud":     "psychic-homily-users",
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenStr, err := token.SignedString([]byte(secretKey))
	require.NoError(t, err)
	return tokenStr
}
