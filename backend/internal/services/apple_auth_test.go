package services

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"psychic-homily-backend/internal/config"
	"psychic-homily-backend/internal/models"
)

// =============================================================================
// NewAppleAuthService TESTS
// =============================================================================

func TestNewAppleAuthService(t *testing.T) {
	cfg := &config.Config{
		Apple: config.AppleConfig{BundleID: "com.test.app"},
		JWT:   config.JWTConfig{SecretKey: "test-key", Expiry: 24},
	}
	svc := NewAppleAuthService(nil, cfg)
	assert.NotNil(t, svc)
	assert.NotNil(t, svc.config)
	assert.NotNil(t, svc.userService)
	assert.NotNil(t, svc.jwtService)
	assert.NotNil(t, svc.keys)
	assert.Empty(t, svc.keys)
}

// =============================================================================
// GenerateToken TESTS
// =============================================================================

func TestAppleAuthService_GenerateToken(t *testing.T) {
	cfg := &config.Config{
		Apple: config.AppleConfig{BundleID: "com.test.app"},
		JWT:   config.JWTConfig{SecretKey: "test-key-for-apple-generate", Expiry: 24},
	}
	svc := NewAppleAuthService(nil, cfg)

	user := &models.User{ID: 42, Email: stringPtr("apple@example.com")}
	token, err := svc.GenerateToken(user)
	assert.NoError(t, err)
	assert.NotEmpty(t, token)

	// Parse token to verify claims
	parsed, parseErr := jwt.Parse(token, func(t *jwt.Token) (interface{}, error) {
		return []byte(cfg.JWT.SecretKey), nil
	})
	assert.NoError(t, parseErr)
	assert.True(t, parsed.Valid)
	claims := parsed.Claims.(jwt.MapClaims)
	assert.Equal(t, float64(42), claims["user_id"])
}

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

// =============================================================================
// INTEGRATION TESTS (Database Required)
// =============================================================================

type AppleAuthIntegrationTestSuite struct {
	suite.Suite
	container testcontainers.Container
	db        *gorm.DB
	ctx       context.Context
	cfg       *config.Config
}

func (s *AppleAuthIntegrationTestSuite) SetupSuite() {
	s.ctx = context.Background()

	container, err := testcontainers.GenericContainer(s.ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        "postgres:18",
			ExposedPorts: []string{"5432/tcp"},
			Env: map[string]string{
				"POSTGRES_DB":       "test_db",
				"POSTGRES_USER":     "test_user",
				"POSTGRES_PASSWORD": "test_password",
			},
			WaitingFor: wait.ForLog("database system is ready to accept connections").WithOccurrence(2).WithStartupTimeout(120 * time.Second),
		},
		Started: true,
	})
	if err != nil {
		s.T().Fatalf("failed to start postgres container: %v", err)
	}
	s.container = container

	host, err := container.Host(s.ctx)
	if err != nil {
		s.T().Fatalf("failed to get host: %v", err)
	}
	port, err := container.MappedPort(s.ctx, "5432")
	if err != nil {
		s.T().Fatalf("failed to get port: %v", err)
	}

	dsn := fmt.Sprintf("host=%s port=%s user=test_user password=test_password dbname=test_db sslmode=disable",
		host, port.Port())
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		s.T().Fatalf("failed to connect to test database: %v", err)
	}
	s.db = db

	sqlDB, err := db.DB()
	if err != nil {
		s.T().Fatalf("failed to get sql.DB: %v", err)
	}

	migrations := []string{
		"000001_create_initial_schema.up.sql",
		"000005_add_show_status.up.sql",
		"000012_add_user_deletion_fields.up.sql",
		"000014_add_account_lockout.up.sql",
		"000031_add_user_terms_acceptance.up.sql",
	}
	for _, m := range migrations {
		migrationSQL, err := os.ReadFile(filepath.Join("..", "..", "db", "migrations", m))
		if err != nil {
			s.T().Fatalf("failed to read migration file %s: %v", m, err)
		}
		_, err = sqlDB.Exec(string(migrationSQL))
		if err != nil {
			s.T().Fatalf("failed to run migration %s: %v", m, err)
		}
	}

	s.cfg = &config.Config{
		Apple: config.AppleConfig{BundleID: "com.test.app"},
		JWT:   config.JWTConfig{SecretKey: "test-secret-apple-integration", Expiry: 24},
	}
}

func (s *AppleAuthIntegrationTestSuite) TearDownSuite() {
	if s.container != nil {
		if err := s.container.Terminate(s.ctx); err != nil {
			s.T().Logf("failed to terminate container: %v", err)
		}
	}
}

func (s *AppleAuthIntegrationTestSuite) TearDownTest() {
	// Clean up all rows between tests
	s.db.Exec("DELETE FROM user_preferences")
	s.db.Exec("DELETE FROM oauth_accounts")
	s.db.Exec("DELETE FROM users")
}

func (s *AppleAuthIntegrationTestSuite) newService() *AppleAuthService {
	return &AppleAuthService{
		db:          s.db,
		config:      s.cfg,
		userService: &UserService{db: s.db},
		jwtService:  NewJWTService(s.db, s.cfg),
		keys:        make(map[string]*rsa.PublicKey),
	}
}

// ---------------------------------------------------------------------------
// FindOrCreateAppleUser
// ---------------------------------------------------------------------------

func (s *AppleAuthIntegrationTestSuite) TestFindOrCreateAppleUser_NewUser_CreatesUserAndOAuth() {
	svc := s.newService()
	claims := &AppleIdentityTokenClaims{
		Email:         "apple@example.com",
		EmailVerified: true,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject: "apple-sub-001",
		},
	}

	user, err := svc.FindOrCreateAppleUser(claims, "John", "Doe")

	s.Require().NoError(err)
	s.Require().NotNil(user)
	s.NotZero(user.ID)
	s.Equal("apple@example.com", *user.Email)
	s.Equal("John", *user.FirstName)
	s.Equal("Doe", *user.LastName)
	s.True(user.IsActive)
	s.True(user.EmailVerified)

	// OAuthAccount preloaded
	s.Require().Len(user.OAuthAccounts, 1)
	s.Equal("apple", user.OAuthAccounts[0].Provider)
	s.Equal("apple-sub-001", user.OAuthAccounts[0].ProviderUserID)
	s.Equal("apple@example.com", *user.OAuthAccounts[0].ProviderEmail)

	// UserPreferences created
	s.Require().NotNil(user.Preferences)
	s.Equal(user.ID, user.Preferences.UserID)
}

func (s *AppleAuthIntegrationTestSuite) TestFindOrCreateAppleUser_NewUser_NoEmail() {
	svc := s.newService()
	claims := &AppleIdentityTokenClaims{
		Email: "",
		RegisteredClaims: jwt.RegisteredClaims{
			Subject: "apple-sub-no-email",
		},
	}

	user, err := svc.FindOrCreateAppleUser(claims, "No", "Email")

	s.Require().NoError(err)
	s.Require().NotNil(user)
	s.Nil(user.Email)

	s.Require().Len(user.OAuthAccounts, 1)
	s.Nil(user.OAuthAccounts[0].ProviderEmail)
}

func (s *AppleAuthIntegrationTestSuite) TestFindOrCreateAppleUser_ExistingAppleUser_ReturnsUser() {
	svc := s.newService()

	// Pre-create user via first call
	claims := &AppleIdentityTokenClaims{
		Email: "existing-apple@example.com",
		RegisteredClaims: jwt.RegisteredClaims{
			Subject: "apple-sub-existing",
		},
	}
	firstUser, err := svc.FindOrCreateAppleUser(claims, "First", "Call")
	s.Require().NoError(err)

	// Second call with same apple subject
	secondUser, err := svc.FindOrCreateAppleUser(claims, "Second", "Call")

	s.Require().NoError(err)
	s.Equal(firstUser.ID, secondUser.ID)

	// No duplicate OAuth accounts
	var count int64
	s.db.Model(&models.OAuthAccount{}).Where("provider = ? AND provider_user_id = ?", "apple", "apple-sub-existing").Count(&count)
	s.Equal(int64(1), count)
}

func (s *AppleAuthIntegrationTestSuite) TestFindOrCreateAppleUser_ExistingEmail_LinksAppleAccount() {
	// Pre-create a user with email but no OAuth
	existingUser := &models.User{
		Email:         strPtr("link-apple@example.com"),
		FirstName:     strPtr("Existing"),
		LastName:      strPtr("User"),
		IsActive:      true,
		EmailVerified: true,
	}
	s.Require().NoError(s.db.Create(existingUser).Error)

	svc := s.newService()
	claims := &AppleIdentityTokenClaims{
		Email: "link-apple@example.com",
		RegisteredClaims: jwt.RegisteredClaims{
			Subject: "apple-sub-link",
		},
	}

	user, err := svc.FindOrCreateAppleUser(claims, "Apple", "Name")

	s.Require().NoError(err)
	s.Equal(existingUser.ID, user.ID) // Same user, not a new one
	s.Require().Len(user.OAuthAccounts, 1)
	s.Equal("apple", user.OAuthAccounts[0].Provider)
	s.Equal("apple-sub-link", user.OAuthAccounts[0].ProviderUserID)
}

func (s *AppleAuthIntegrationTestSuite) TestFindOrCreateAppleUser_ExistingEmail_DifferentAppleID() {
	svc := s.newService()

	// Create first user with apple account
	claims1 := &AppleIdentityTokenClaims{
		Email: "shared-email@example.com",
		RegisteredClaims: jwt.RegisteredClaims{
			Subject: "apple-sub-first",
		},
	}
	firstUser, err := svc.FindOrCreateAppleUser(claims1, "First", "User")
	s.Require().NoError(err)

	// Second call with different apple subject but same email
	// Since first user already has an apple OAuth, this looks up by apple subject (not found),
	// then finds user by email and links the new apple ID
	claims2 := &AppleIdentityTokenClaims{
		Email: "shared-email@example.com",
		RegisteredClaims: jwt.RegisteredClaims{
			Subject: "apple-sub-second",
		},
	}
	secondUser, err := svc.FindOrCreateAppleUser(claims2, "Second", "User")

	s.Require().NoError(err)
	// Links to same user since email matches
	s.Equal(firstUser.ID, secondUser.ID)

	// Now user has two apple OAuth accounts
	var count int64
	s.db.Model(&models.OAuthAccount{}).Where("user_id = ? AND provider = ?", firstUser.ID, "apple").Count(&count)
	s.Equal(int64(2), count)
}

// ---------------------------------------------------------------------------
// linkAppleAccount
// ---------------------------------------------------------------------------

func (s *AppleAuthIntegrationTestSuite) TestLinkAppleAccount_Success() {
	// Pre-create user
	user := &models.User{
		Email:     strPtr("link-test@example.com"),
		FirstName: strPtr("Link"),
		LastName:  strPtr("Test"),
		IsActive:  true,
	}
	s.Require().NoError(s.db.Create(user).Error)

	svc := s.newService()
	result, err := svc.linkAppleAccount(user, "apple-link-id", "link-test@example.com")

	s.Require().NoError(err)
	s.Require().NotNil(result)
	s.Equal(user.ID, result.ID)

	// OAuthAccount created and preloaded
	s.Require().Len(result.OAuthAccounts, 1)
	s.Equal("apple", result.OAuthAccounts[0].Provider)
	s.Equal("apple-link-id", result.OAuthAccounts[0].ProviderUserID)
	s.Equal("link-test@example.com", *result.OAuthAccounts[0].ProviderEmail)
}

func (s *AppleAuthIntegrationTestSuite) TestLinkAppleAccount_UserHasPreferences() {
	// Pre-create user with preferences
	user := &models.User{
		Email:     strPtr("prefs-test@example.com"),
		FirstName: strPtr("Prefs"),
		LastName:  strPtr("Test"),
		IsActive:  true,
	}
	s.Require().NoError(s.db.Create(user).Error)
	prefs := &models.UserPreferences{UserID: user.ID}
	s.Require().NoError(s.db.Create(prefs).Error)

	svc := s.newService()
	result, err := svc.linkAppleAccount(user, "apple-prefs-id", "prefs-test@example.com")

	s.Require().NoError(err)
	s.Require().NotNil(result.Preferences)
	s.Equal(user.ID, result.Preferences.UserID)
}

// ---------------------------------------------------------------------------
// createAppleUser
// ---------------------------------------------------------------------------

func (s *AppleAuthIntegrationTestSuite) TestCreateAppleUser_Success() {
	svc := s.newService()

	user, err := svc.createAppleUser("apple-create-id", "create@example.com", "Create", "User")

	s.Require().NoError(err)
	s.Require().NotNil(user)
	s.NotZero(user.ID)
	s.Equal("create@example.com", *user.Email)
	s.Equal("Create", *user.FirstName)
	s.Equal("User", *user.LastName)
	s.True(user.IsActive)
	s.True(user.EmailVerified)

	// OAuthAccount
	s.Require().Len(user.OAuthAccounts, 1)
	s.Equal("apple", user.OAuthAccounts[0].Provider)
	s.Equal("apple-create-id", user.OAuthAccounts[0].ProviderUserID)
	s.Equal("create@example.com", *user.OAuthAccounts[0].ProviderEmail)

	// UserPreferences
	s.Require().NotNil(user.Preferences)
	s.Equal(user.ID, user.Preferences.UserID)
}

func (s *AppleAuthIntegrationTestSuite) TestCreateAppleUser_NoEmail() {
	svc := s.newService()

	user, err := svc.createAppleUser("apple-noemail-id", "", "No", "Email")

	s.Require().NoError(err)
	s.Require().NotNil(user)
	s.Nil(user.Email)

	s.Require().Len(user.OAuthAccounts, 1)
	s.Nil(user.OAuthAccounts[0].ProviderEmail)
}

func (s *AppleAuthIntegrationTestSuite) TestCreateAppleUser_DuplicateEmail() {
	// Pre-create a user with the same email
	existing := &models.User{
		Email:    strPtr("dupe@example.com"),
		IsActive: true,
	}
	s.Require().NoError(s.db.Create(existing).Error)

	svc := s.newService()

	_, err := svc.createAppleUser("apple-dupe-id", "dupe@example.com", "Dupe", "User")

	s.Error(err)
	s.Contains(err.Error(), "failed to create user")
}

// ---------------------------------------------------------------------------
// Suite Runner
// ---------------------------------------------------------------------------

func TestAppleAuthIntegrationTestSuite(t *testing.T) {
	suite.Run(t, new(AppleAuthIntegrationTestSuite))
}

// strPtr is a local helper for string pointer literals
func strPtr(s string) *string {
	return &s
}
