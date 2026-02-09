package services

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"psychic-homily-backend/internal/config"
	"psychic-homily-backend/internal/models"
)

// =============================================================================
// UNIT TESTS
// =============================================================================

// TestNewJWTService tests the creation of a new JWTService
func TestNewJWTService(t *testing.T) {
	cfg := &config.Config{
		JWT: config.JWTConfig{
			SecretKey: "test-secret-key",
			Expiry:    24,
		},
	}

	jwtService := NewJWTService(nil, cfg)

	assert.NotNil(t, jwtService)
	assert.Equal(t, cfg, jwtService.config)
}

// TestJWTService_CreateToken tests JWT token creation
func TestJWTService_CreateToken(t *testing.T) {
	cfg := &config.Config{
		JWT: config.JWTConfig{
			SecretKey: "test-secret-key-123",
			Expiry:    24,
		},
	}

	jwtService := NewJWTService(nil, cfg)

	t.Run("CreateToken_Success", func(t *testing.T) {
		user := &models.User{
			ID:    123,
			Email: stringPtr("test@example.com"),
		}

		token, err := jwtService.CreateToken(user)

		assert.NoError(t, err)
		assert.NotEmpty(t, token)

		// Verify the token can be parsed and contains expected claims
		parsedToken, err := jwt.Parse(token, func(token *jwt.Token) (interface{}, error) {
			return []byte(cfg.JWT.SecretKey), nil
		})

		assert.NoError(t, err)
		assert.True(t, parsedToken.Valid)

		claims, ok := parsedToken.Claims.(jwt.MapClaims)
		assert.True(t, ok)
		assert.Equal(t, float64(123), claims["user_id"])
		assert.Equal(t, "test@example.com", claims["email"])
		assert.Equal(t, "psychic-homily-backend", claims["iss"])
		assert.Equal(t, "psychic-homily-users", claims["aud"])
		assert.NotNil(t, claims["exp"])
		assert.NotNil(t, claims["iat"])
	})

	t.Run("CreateToken_WithNilEmail", func(t *testing.T) {
		user := &models.User{
			ID:    456,
			Email: nil,
		}

		token, err := jwtService.CreateToken(user)

		assert.NoError(t, err)
		assert.NotEmpty(t, token)

		// Verify the token can be parsed
		parsedToken, err := jwt.Parse(token, func(token *jwt.Token) (interface{}, error) {
			return []byte(cfg.JWT.SecretKey), nil
		})

		assert.NoError(t, err)
		assert.True(t, parsedToken.Valid)

		claims, ok := parsedToken.Claims.(jwt.MapClaims)
		assert.True(t, ok)
		assert.Equal(t, float64(456), claims["user_id"])
		assert.Nil(t, claims["email"])
	})

	t.Run("CreateToken_ExpiryCalculation", func(t *testing.T) {
		user := &models.User{
			ID:    789,
			Email: stringPtr("expiry@example.com"),
		}

		beforeCreation := time.Now()
		token, err := jwtService.CreateToken(user)
		afterCreation := time.Now()

		assert.NoError(t, err)

		// Parse token and check expiry
		parsedToken, err := jwt.Parse(token, func(token *jwt.Token) (interface{}, error) {
			return []byte(cfg.JWT.SecretKey), nil
		})

		assert.NoError(t, err)
		claims, ok := parsedToken.Claims.(jwt.MapClaims)
		assert.True(t, ok)

		exp := int64(claims["exp"].(float64))
		iat := int64(claims["iat"].(float64))

		// Check that issued at time is within expected range
		assert.GreaterOrEqual(t, iat, beforeCreation.Unix())
		assert.LessOrEqual(t, iat, afterCreation.Unix())

		// Check that expiry is 24 hours from issued at time
		expectedExp := iat + int64(24*3600) // 24 hours in seconds
		assert.Equal(t, expectedExp, exp)
	})
}

// TestJWTService_ValidateToken tests JWT token validation
func TestJWTService_ValidateToken(t *testing.T) {
	cfg := &config.Config{
		JWT: config.JWTConfig{
			SecretKey: "test-secret-key-456",
			Expiry:    24,
		},
	}

	jwtService := NewJWTService(nil, cfg)

	t.Run("ValidateToken_Success", func(t *testing.T) {
		// Create a valid token first
		user := &models.User{
			ID:    123,
			Email: stringPtr("valid@example.com"),
		}

		token, err := jwtService.CreateToken(user)
		require.NoError(t, err)

		// Validate the token — in test environment without DB, this will
		// parse the token successfully but fail on user lookup
		_, err = jwtService.ValidateToken(token)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get user")
	})

	t.Run("ValidateToken_WithNilEmail", func(t *testing.T) {
		// Create a token with nil email
		user := &models.User{
			ID:    456,
			Email: nil,
		}

		token, err := jwtService.CreateToken(user)
		require.NoError(t, err)

		// Validate the token — in test environment without DB, this will
		// parse the token successfully but fail on user lookup
		_, err = jwtService.ValidateToken(token)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get user")
	})

	t.Run("ValidateToken_InvalidToken", func(t *testing.T) {
		invalidToken := "invalid.token.string"

		validatedUser, err := jwtService.ValidateToken(invalidToken)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "TOKEN_INVALID")
		assert.Nil(t, validatedUser)
	})

	t.Run("ValidateToken_EmptyToken", func(t *testing.T) {
		validatedUser, err := jwtService.ValidateToken("")

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "TOKEN_INVALID")
		assert.Nil(t, validatedUser)
	})

	t.Run("ValidateToken_WrongSecret", func(t *testing.T) {
		// Create token with one secret
		cfg1 := &config.Config{
			JWT: config.JWTConfig{
				SecretKey: "secret-1",
				Expiry:    24,
			},
		}
		jwtService1 := NewJWTService(nil, cfg1)

		user := &models.User{
			ID:    123,
			Email: stringPtr("test@example.com"),
		}

		token, err := jwtService1.CreateToken(user)
		require.NoError(t, err)

		// Try to validate with different secret
		validatedUser, err := jwtService.ValidateToken(token)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "TOKEN_INVALID")
		assert.Nil(t, validatedUser)
	})

	t.Run("ValidateToken_ExpiredToken", func(t *testing.T) {
		// Create a token with very short expiry
		cfgShort := &config.Config{
			JWT: config.JWTConfig{
				SecretKey: "test-secret-key-456",
				Expiry:    0, // 0 hours = immediate expiry
			},
		}
		jwtServiceShort := NewJWTService(nil, cfgShort)

		user := &models.User{
			ID:    123,
			Email: stringPtr("expired@example.com"),
		}

		token, err := jwtServiceShort.CreateToken(user)
		require.NoError(t, err)

		// Wait a moment for token to expire
		time.Sleep(100 * time.Millisecond)

		// Try to validate expired token
		validatedUser, err := jwtService.ValidateToken(token)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "TOKEN_EXPIRED")
		assert.Nil(t, validatedUser)
	})

	t.Run("ValidateToken_TamperedToken", func(t *testing.T) {
		// Create a valid token
		user := &models.User{
			ID:    123,
			Email: stringPtr("tampered@example.com"),
		}

		token, err := jwtService.CreateToken(user)
		require.NoError(t, err)

		// Tamper with the token by changing a character
		tamperedToken := token[:len(token)-1] + "X"

		// Try to validate tampered token
		validatedUser, err := jwtService.ValidateToken(tamperedToken)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "TOKEN_INVALID")
		assert.Nil(t, validatedUser)
	})
}

// TestJWTService_RefreshToken tests JWT token refresh functionality
func TestJWTService_RefreshToken(t *testing.T) {
	cfg := &config.Config{
		JWT: config.JWTConfig{
			SecretKey: "test-secret-key-789",
			Expiry:    24,
		},
	}

	jwtService := NewJWTService(nil, cfg)

	t.Run("RefreshToken_Success", func(t *testing.T) {
		// Create an original token
		user := &models.User{
			ID:    123,
			Email: stringPtr("refresh@example.com"),
		}

		originalToken, err := jwtService.CreateToken(user)
		require.NoError(t, err)

		// Wait a moment to ensure different timestamps
		time.Sleep(100 * time.Millisecond)

		// Refresh the token — fails at DB lookup since no DB
		_, err = jwtService.RefreshToken(originalToken)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get user")
	})

	t.Run("RefreshToken_InvalidToken", func(t *testing.T) {
		invalidToken := "invalid.token.string"

		refreshedToken, err := jwtService.RefreshToken(invalidToken)

		assert.Error(t, err)
		assert.Empty(t, refreshedToken)
		assert.Contains(t, err.Error(), "TOKEN_INVALID")
	})

	t.Run("RefreshToken_EmptyToken", func(t *testing.T) {
		refreshedToken, err := jwtService.RefreshToken("")

		assert.Error(t, err)
		assert.Empty(t, refreshedToken)
		assert.Contains(t, err.Error(), "TOKEN_INVALID")
	})

	t.Run("RefreshToken_ExpiredToken", func(t *testing.T) {
		// Create a token with immediate expiry
		cfgShort := &config.Config{
			JWT: config.JWTConfig{
				SecretKey: "test-secret-key-789",
				Expiry:    0, // 0 hours = immediate expiry
			},
		}
		jwtServiceShort := NewJWTService(nil, cfgShort)

		user := &models.User{
			ID:    123,
			Email: stringPtr("expired@example.com"),
		}

		expiredToken, err := jwtServiceShort.CreateToken(user)
		require.NoError(t, err)

		// Wait for token to expire
		time.Sleep(100 * time.Millisecond)

		// Try to refresh expired token
		refreshedToken, err := jwtService.RefreshToken(expiredToken)

		assert.Error(t, err)
		assert.Empty(t, refreshedToken)
		assert.Contains(t, err.Error(), "TOKEN_EXPIRED")
	})
}

// TestJWTService_EdgeCases tests edge cases and boundary conditions
func TestJWTService_EdgeCases(t *testing.T) {
	cfg := &config.Config{
		JWT: config.JWTConfig{
			SecretKey: "test-secret-key-edge",
			Expiry:    24,
		},
	}

	jwtService := NewJWTService(nil, cfg)

	t.Run("CreateToken_ZeroUserID", func(t *testing.T) {
		user := &models.User{
			ID:    0,
			Email: stringPtr("zero@example.com"),
		}

		token, err := jwtService.CreateToken(user)

		assert.NoError(t, err)
		assert.NotEmpty(t, token)

		// Token creation succeeds; validation fails at DB lookup
		_, err = jwtService.ValidateToken(token)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get user")
	})

	t.Run("CreateToken_VeryLongEmail", func(t *testing.T) {
		longEmail := "very.long.email.address.that.is.quite.lengthy.and.might.test.boundaries@example.com"
		user := &models.User{
			ID:    123,
			Email: &longEmail,
		}

		token, err := jwtService.CreateToken(user)

		assert.NoError(t, err)
		assert.NotEmpty(t, token)

		// Token creation succeeds; validation fails at DB lookup
		_, err = jwtService.ValidateToken(token)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get user")
	})

	t.Run("CreateToken_SpecialCharactersInEmail", func(t *testing.T) {
		specialEmail := "test+tag@example.com"
		user := &models.User{
			ID:    123,
			Email: &specialEmail,
		}

		token, err := jwtService.CreateToken(user)

		assert.NoError(t, err)
		assert.NotEmpty(t, token)

		// Token creation succeeds; validation fails at DB lookup
		_, err = jwtService.ValidateToken(token)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get user")
	})
}

// TestJWTService_Integration tests integration scenarios
func TestJWTService_Integration(t *testing.T) {
	cfg := &config.Config{
		JWT: config.JWTConfig{
			SecretKey: "test-secret-key-integration",
			Expiry:    24,
		},
	}

	jwtService := NewJWTService(nil, cfg)

	t.Run("Complete_Token_Lifecycle", func(t *testing.T) {
		// 1. Create a user
		user := &models.User{
			ID:    999,
			Email: stringPtr("lifecycle@example.com"),
		}

		// 2. Create initial token
		token1, err := jwtService.CreateToken(user)
		assert.NoError(t, err)
		assert.NotEmpty(t, token1)

		// 3. Validate the token — hits DB lookup (no DB in tests)
		_, err = jwtService.ValidateToken(token1)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get user")

		// 4. Refresh the token — also hits DB lookup
		time.Sleep(100 * time.Millisecond)
		_, err = jwtService.RefreshToken(token1)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get user")
	})

	t.Run("Multiple_Users_Same_Service", func(t *testing.T) {
		users := []*models.User{
			{ID: 1, Email: stringPtr("user1@example.com")},
			{ID: 2, Email: stringPtr("user2@example.com")},
			{ID: 3, Email: stringPtr("user3@example.com")},
		}

		tokens := make([]string, len(users))

		// Create tokens for all users
		for i, user := range users {
			token, err := jwtService.CreateToken(user)
			assert.NoError(t, err)
			tokens[i] = token
		}

		// Validate all tokens — fails at DB lookup (no DB in tests)
		for _, token := range tokens {
			_, err := jwtService.ValidateToken(token)
			assert.Error(t, err)
			assert.Contains(t, err.Error(), "failed to get user")
		}

		// Verify tokens are unique
		tokenSet := make(map[string]bool)
		for _, token := range tokens {
			assert.False(t, tokenSet[token], "Duplicate token found")
			tokenSet[token] = true
		}
	})
}

// =============================================================================
// VERIFICATION TOKEN TESTS
// =============================================================================

func TestJWTService_VerificationToken(t *testing.T) {
	secretKey := "test-secret-key-verification"
	cfg := &config.Config{
		JWT: config.JWTConfig{
			SecretKey: secretKey,
			Expiry:    24,
		},
	}
	jwtService := NewJWTService(nil, cfg)

	t.Run("CreateAndValidate_Success", func(t *testing.T) {
		token, err := jwtService.CreateVerificationToken(123, "verify@example.com")
		require.NoError(t, err)
		assert.NotEmpty(t, token)

		claims, err := jwtService.ValidateVerificationToken(token)
		require.NoError(t, err)
		assert.Equal(t, uint(123), claims.UserID)
		assert.Equal(t, "verify@example.com", claims.Email)
		assert.Equal(t, "email-verification", claims.Subject)
		assert.Equal(t, "psychic-homily-backend", claims.Issuer)
	})

	t.Run("Validate_ExpiredToken", func(t *testing.T) {
		// Create a token that's already expired
		claims := VerificationTokenClaims{
			UserID: 456,
			Email:  "expired@example.com",
			RegisteredClaims: jwt.RegisteredClaims{
				ExpiresAt: jwt.NewNumericDate(time.Now().Add(-1 * time.Hour)),
				IssuedAt:  jwt.NewNumericDate(time.Now().Add(-25 * time.Hour)),
				Issuer:    "psychic-homily-backend",
				Subject:   "email-verification",
			},
		}
		token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
		tokenStr, err := token.SignedString([]byte(secretKey))
		require.NoError(t, err)

		_, err = jwtService.ValidateVerificationToken(tokenStr)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid verification token")
	})

	t.Run("Validate_WrongSubject", func(t *testing.T) {
		// Create a token with wrong subject
		claims := VerificationTokenClaims{
			UserID: 789,
			Email:  "wrong@example.com",
			RegisteredClaims: jwt.RegisteredClaims{
				ExpiresAt: jwt.NewNumericDate(time.Now().Add(1 * time.Hour)),
				IssuedAt:  jwt.NewNumericDate(time.Now()),
				Issuer:    "psychic-homily-backend",
				Subject:   "magic-link", // wrong subject
			},
		}
		token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
		tokenStr, err := token.SignedString([]byte(secretKey))
		require.NoError(t, err)

		_, err = jwtService.ValidateVerificationToken(tokenStr)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid token type")
	})

	t.Run("Validate_InvalidToken", func(t *testing.T) {
		_, err := jwtService.ValidateVerificationToken("not-a-token")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid verification token")
	})

	t.Run("Validate_WrongSecret", func(t *testing.T) {
		otherCfg := &config.Config{JWT: config.JWTConfig{SecretKey: "other-key", Expiry: 24}}
		otherService := NewJWTService(nil, otherCfg)
		token, err := otherService.CreateVerificationToken(123, "wrong-key@example.com")
		require.NoError(t, err)

		_, err = jwtService.ValidateVerificationToken(token)
		assert.Error(t, err)
	})
}

// =============================================================================
// MAGIC LINK TOKEN TESTS
// =============================================================================

func TestJWTService_MagicLinkToken(t *testing.T) {
	secretKey := "test-secret-key-magiclink"
	cfg := &config.Config{
		JWT: config.JWTConfig{
			SecretKey: secretKey,
			Expiry:    24,
		},
	}
	jwtService := NewJWTService(nil, cfg)

	t.Run("CreateAndValidate_Success", func(t *testing.T) {
		token, err := jwtService.CreateMagicLinkToken(123, "magic@example.com")
		require.NoError(t, err)
		assert.NotEmpty(t, token)

		claims, err := jwtService.ValidateMagicLinkToken(token)
		require.NoError(t, err)
		assert.Equal(t, uint(123), claims.UserID)
		assert.Equal(t, "magic@example.com", claims.Email)
		assert.Equal(t, "magic-link", claims.Subject)
		assert.Equal(t, "psychic-homily-backend", claims.Issuer)
	})

	t.Run("Validate_ExpiredToken", func(t *testing.T) {
		claims := MagicLinkTokenClaims{
			UserID: 456,
			Email:  "expired@example.com",
			RegisteredClaims: jwt.RegisteredClaims{
				ExpiresAt: jwt.NewNumericDate(time.Now().Add(-1 * time.Hour)),
				IssuedAt:  jwt.NewNumericDate(time.Now().Add(-2 * time.Hour)),
				Issuer:    "psychic-homily-backend",
				Subject:   "magic-link",
			},
		}
		token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
		tokenStr, err := token.SignedString([]byte(secretKey))
		require.NoError(t, err)

		_, err = jwtService.ValidateMagicLinkToken(tokenStr)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid magic link token")
	})

	t.Run("Validate_WrongSubject", func(t *testing.T) {
		claims := MagicLinkTokenClaims{
			UserID: 789,
			Email:  "wrong@example.com",
			RegisteredClaims: jwt.RegisteredClaims{
				ExpiresAt: jwt.NewNumericDate(time.Now().Add(1 * time.Hour)),
				IssuedAt:  jwt.NewNumericDate(time.Now()),
				Issuer:    "psychic-homily-backend",
				Subject:   "email-verification", // wrong subject
			},
		}
		token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
		tokenStr, err := token.SignedString([]byte(secretKey))
		require.NoError(t, err)

		_, err = jwtService.ValidateMagicLinkToken(tokenStr)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid token type")
	})

	t.Run("Validate_InvalidToken", func(t *testing.T) {
		_, err := jwtService.ValidateMagicLinkToken("garbage")
		assert.Error(t, err)
	})
}

// =============================================================================
// ACCOUNT RECOVERY TOKEN TESTS
// =============================================================================

func TestJWTService_AccountRecoveryToken(t *testing.T) {
	secretKey := "test-secret-key-recovery"
	cfg := &config.Config{
		JWT: config.JWTConfig{
			SecretKey: secretKey,
			Expiry:    24,
		},
	}
	jwtService := NewJWTService(nil, cfg)

	t.Run("CreateAndValidate_Success", func(t *testing.T) {
		token, err := jwtService.CreateAccountRecoveryToken(123, "recover@example.com")
		require.NoError(t, err)
		assert.NotEmpty(t, token)

		claims, err := jwtService.ValidateAccountRecoveryToken(token)
		require.NoError(t, err)
		assert.Equal(t, uint(123), claims.UserID)
		assert.Equal(t, "recover@example.com", claims.Email)
		assert.Equal(t, "account-recovery", claims.Subject)
		assert.Equal(t, "psychic-homily-backend", claims.Issuer)
	})

	t.Run("Validate_ExpiredToken", func(t *testing.T) {
		claims := AccountRecoveryTokenClaims{
			UserID: 456,
			Email:  "expired@example.com",
			RegisteredClaims: jwt.RegisteredClaims{
				ExpiresAt: jwt.NewNumericDate(time.Now().Add(-1 * time.Hour)),
				IssuedAt:  jwt.NewNumericDate(time.Now().Add(-2 * time.Hour)),
				Issuer:    "psychic-homily-backend",
				Subject:   "account-recovery",
			},
		}
		token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
		tokenStr, err := token.SignedString([]byte(secretKey))
		require.NoError(t, err)

		_, err = jwtService.ValidateAccountRecoveryToken(tokenStr)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid account recovery token")
	})

	t.Run("Validate_WrongSubject", func(t *testing.T) {
		claims := AccountRecoveryTokenClaims{
			UserID: 789,
			Email:  "wrong@example.com",
			RegisteredClaims: jwt.RegisteredClaims{
				ExpiresAt: jwt.NewNumericDate(time.Now().Add(1 * time.Hour)),
				IssuedAt:  jwt.NewNumericDate(time.Now()),
				Issuer:    "psychic-homily-backend",
				Subject:   "magic-link", // wrong subject
			},
		}
		token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
		tokenStr, err := token.SignedString([]byte(secretKey))
		require.NoError(t, err)

		_, err = jwtService.ValidateAccountRecoveryToken(tokenStr)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid token type")
	})

	t.Run("Validate_InvalidToken", func(t *testing.T) {
		_, err := jwtService.ValidateAccountRecoveryToken("garbage")
		assert.Error(t, err)
	})

	t.Run("Validate_WrongSecret", func(t *testing.T) {
		otherCfg := &config.Config{JWT: config.JWTConfig{SecretKey: "other-key", Expiry: 24}}
		otherService := NewJWTService(nil, otherCfg)
		token, err := otherService.CreateAccountRecoveryToken(123, "wrong-key@example.com")
		require.NoError(t, err)

		_, err = jwtService.ValidateAccountRecoveryToken(token)
		assert.Error(t, err)
	})
}

// =============================================================================
// HELPER FUNCTIONS
// =============================================================================

// parseTokenClaims is a helper function to parse JWT claims
func parseTokenClaims(t *testing.T, tokenString, secretKey string) jwt.MapClaims {
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		return []byte(secretKey), nil
	})
	require.NoError(t, err)
	require.True(t, token.Valid)

	claims, ok := token.Claims.(jwt.MapClaims)
	require.True(t, ok)
	return claims
} 
