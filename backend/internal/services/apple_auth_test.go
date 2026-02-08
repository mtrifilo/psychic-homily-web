package services

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"psychic-homily-backend/internal/config"
)

// =============================================================================
// IsEmailVerified TESTS
// =============================================================================

func TestAppleIdentityTokenClaims_IsEmailVerified(t *testing.T) {
	t.Run("bool_true", func(t *testing.T) {
		claims := &AppleIdentityTokenClaims{EmailVerified: true}
		assert.True(t, claims.IsEmailVerified())
	})

	t.Run("bool_false", func(t *testing.T) {
		claims := &AppleIdentityTokenClaims{EmailVerified: false}
		assert.False(t, claims.IsEmailVerified())
	})

	t.Run("string_true", func(t *testing.T) {
		claims := &AppleIdentityTokenClaims{EmailVerified: "true"}
		assert.True(t, claims.IsEmailVerified())
	})

	t.Run("string_false", func(t *testing.T) {
		claims := &AppleIdentityTokenClaims{EmailVerified: "false"}
		assert.False(t, claims.IsEmailVerified())
	})

	t.Run("nil_value", func(t *testing.T) {
		claims := &AppleIdentityTokenClaims{EmailVerified: nil}
		assert.False(t, claims.IsEmailVerified())
	})

	t.Run("unexpected_type_int", func(t *testing.T) {
		claims := &AppleIdentityTokenClaims{EmailVerified: 1}
		assert.False(t, claims.IsEmailVerified())
	})
}

// =============================================================================
// ValidateIdentityToken TESTS (with mock Apple keys)
// =============================================================================

func TestValidateIdentityToken(t *testing.T) {
	// Generate a test RSA key pair
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	kid := "test-kid-123"
	bundleID := "com.psychichomily.ios"

	// Create a mock Apple keys server
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Encode public key as JWK
		nBase64 := base64.RawURLEncoding.EncodeToString(privateKey.PublicKey.N.Bytes())
		eBytes := big.NewInt(int64(privateKey.PublicKey.E)).Bytes()
		eBase64 := base64.RawURLEncoding.EncodeToString(eBytes)

		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"keys":[{"kty":"RSA","kid":"` + kid + `","use":"sig","alg":"RS256","n":"` + nBase64 + `","e":"` + eBase64 + `"}]}`))
	}))
	defer mockServer.Close()

	cfg := &config.Config{
		Apple: config.AppleConfig{BundleID: bundleID},
		JWT: config.JWTConfig{
			SecretKey: "test-secret-for-apple-auth",
			Expiry:    24,
		},
	}

	t.Run("valid_token", func(t *testing.T) {
		svc := createTestAppleService(cfg, mockServer.URL)

		// Create a valid Apple identity token
		token := createAppleIdentityToken(t, privateKey, kid, bundleID, "apple-user-123", "test@example.com", time.Now().Add(1*time.Hour))

		claims, err := svc.ValidateIdentityToken(token)

		assert.NoError(t, err)
		require.NotNil(t, claims)
		assert.Equal(t, "test@example.com", claims.Email)
		assert.Equal(t, "apple-user-123", claims.Subject)
	})

	t.Run("expired_token_rejected", func(t *testing.T) {
		svc := createTestAppleService(cfg, mockServer.URL)

		token := createAppleIdentityToken(t, privateKey, kid, bundleID, "apple-user-456", "expired@example.com", time.Now().Add(-1*time.Hour))

		_, err := svc.ValidateIdentityToken(token)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid Apple identity token")
	})

	t.Run("wrong_audience_rejected", func(t *testing.T) {
		svc := createTestAppleService(cfg, mockServer.URL)

		token := createAppleIdentityToken(t, privateKey, kid, "com.wrong.bundle", "apple-user-789", "wrong-aud@example.com", time.Now().Add(1*time.Hour))

		_, err := svc.ValidateIdentityToken(token)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid Apple identity token")
	})

	t.Run("wrong_kid_rejected", func(t *testing.T) {
		svc := createTestAppleService(cfg, mockServer.URL)

		token := createAppleIdentityToken(t, privateKey, "wrong-kid", bundleID, "apple-user-000", "wrong-kid@example.com", time.Now().Add(1*time.Hour))

		_, err := svc.ValidateIdentityToken(token)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "Apple public key not found")
	})

	t.Run("wrong_signing_key_rejected", func(t *testing.T) {
		svc := createTestAppleService(cfg, mockServer.URL)

		// Generate a different key pair
		wrongKey, err := rsa.GenerateKey(rand.Reader, 2048)
		require.NoError(t, err)

		token := createAppleIdentityToken(t, wrongKey, kid, bundleID, "apple-user-wrong", "wrong-key@example.com", time.Now().Add(1*time.Hour))

		_, err = svc.ValidateIdentityToken(token)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid Apple identity token")
	})

	t.Run("malformed_token_rejected", func(t *testing.T) {
		svc := createTestAppleService(cfg, mockServer.URL)

		_, err := svc.ValidateIdentityToken("not-a-jwt")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to parse token header")
	})
}

// =============================================================================
// Apple JWK Fetching and Caching TESTS
// =============================================================================

func TestAppleKeyFetching(t *testing.T) {
	cfg := &config.Config{
		Apple: config.AppleConfig{BundleID: "com.psychichomily.ios"},
		JWT: config.JWTConfig{
			SecretKey: "test-secret",
			Expiry:    24,
		},
	}

	t.Run("caches_keys", func(t *testing.T) {
		callCount := 0
		privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
		require.NoError(t, err)

		mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			callCount++
			nBase64 := base64.RawURLEncoding.EncodeToString(privateKey.PublicKey.N.Bytes())
			eBytes := big.NewInt(int64(privateKey.PublicKey.E)).Bytes()
			eBase64 := base64.RawURLEncoding.EncodeToString(eBytes)
			w.Write([]byte(`{"keys":[{"kty":"RSA","kid":"cached-kid","use":"sig","alg":"RS256","n":"` + nBase64 + `","e":"` + eBase64 + `"}]}`))
		}))
		defer mockServer.Close()

		svc := createTestAppleService(cfg, mockServer.URL)

		// First call fetches
		_, err = svc.getApplePublicKey("cached-kid")
		assert.NoError(t, err)
		assert.Equal(t, 1, callCount)

		// Second call uses cache
		_, err = svc.getApplePublicKey("cached-kid")
		assert.NoError(t, err)
		assert.Equal(t, 1, callCount) // Should still be 1
	})

	t.Run("refetches_on_unknown_kid", func(t *testing.T) {
		callCount := 0
		privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
		require.NoError(t, err)

		mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			callCount++
			nBase64 := base64.RawURLEncoding.EncodeToString(privateKey.PublicKey.N.Bytes())
			eBytes := big.NewInt(int64(privateKey.PublicKey.E)).Bytes()
			eBase64 := base64.RawURLEncoding.EncodeToString(eBytes)
			w.Write([]byte(`{"keys":[{"kty":"RSA","kid":"known-kid","use":"sig","alg":"RS256","n":"` + nBase64 + `","e":"` + eBase64 + `"}]}`))
		}))
		defer mockServer.Close()

		svc := createTestAppleService(cfg, mockServer.URL)

		// First call
		_, err = svc.getApplePublicKey("known-kid")
		assert.NoError(t, err)

		// Unknown kid triggers refetch
		_, err = svc.getApplePublicKey("unknown-kid")
		assert.Error(t, err)
		assert.Equal(t, 2, callCount) // Should have refetched
	})

	t.Run("handles_server_error", func(t *testing.T) {
		mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer mockServer.Close()

		svc := createTestAppleService(cfg, mockServer.URL)

		_, err := svc.getApplePublicKey("any-kid")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "status 500")
	})
}

// =============================================================================
// HELPERS
// =============================================================================

// createTestAppleService creates an AppleAuthService that fetches keys from a mock server
func createTestAppleService(cfg *config.Config, mockKeysURL string) *AppleAuthService {
	svc := &AppleAuthService{
		config:                cfg,
		jwtService:            NewJWTService(nil, cfg),
		keys:                  make(map[string]*rsa.PublicKey),
		fetchAppleKeysFromURL: mockKeysURL,
	}
	return svc
}

// createAppleIdentityToken creates a test Apple identity token signed with RSA
func createAppleIdentityToken(t *testing.T, privateKey *rsa.PrivateKey, kid, audience, subject, email string, exp time.Time) string {
	t.Helper()

	claims := jwt.MapClaims{
		"iss":            appleIssuer,
		"aud":            audience,
		"sub":            subject,
		"email":          email,
		"email_verified": "true",
		"exp":            exp.Unix(),
		"iat":            time.Now().Unix(),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = kid

	tokenStr, err := token.SignedString(privateKey)
	require.NoError(t, err)
	return tokenStr
}
