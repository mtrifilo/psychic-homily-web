package services

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"psychic-homily-backend/internal/models"
)

// =============================================================================
// UNIT TESTS (No Database Required)
// =============================================================================

func TestNewAPITokenService(t *testing.T) {
	svc := NewAPITokenService(nil)
	assert.NotNil(t, svc)
}

func TestAPITokenService_NilDatabase(t *testing.T) {
	svc := &APITokenService{db: nil}

	t.Run("CreateToken", func(t *testing.T) {
		resp, err := svc.CreateToken(1, nil, 90)
		assert.Error(t, err)
		assert.Equal(t, "database not initialized", err.Error())
		assert.Nil(t, resp)
	})

	t.Run("ValidateToken", func(t *testing.T) {
		user, token, err := svc.ValidateToken("phk_abc123")
		assert.Error(t, err)
		assert.Equal(t, "database not initialized", err.Error())
		assert.Nil(t, user)
		assert.Nil(t, token)
	})

	t.Run("ListTokens", func(t *testing.T) {
		tokens, err := svc.ListTokens(1)
		assert.Error(t, err)
		assert.Equal(t, "database not initialized", err.Error())
		assert.Nil(t, tokens)
	})

	t.Run("RevokeToken", func(t *testing.T) {
		err := svc.RevokeToken(1, 1)
		assert.Error(t, err)
		assert.Equal(t, "database not initialized", err.Error())
	})

	t.Run("GetToken", func(t *testing.T) {
		resp, err := svc.GetToken(1, 1)
		assert.Error(t, err)
		assert.Equal(t, "database not initialized", err.Error())
		assert.Nil(t, resp)
	})

	t.Run("CleanupExpiredTokens", func(t *testing.T) {
		count, err := svc.CleanupExpiredTokens()
		assert.Error(t, err)
		assert.Equal(t, "database not initialized", err.Error())
		assert.Zero(t, count)
	})
}

func TestHashToken_Deterministic(t *testing.T) {
	token := "phk_abc123def456"
	hash1 := hashToken(token)
	hash2 := hashToken(token)
	assert.Equal(t, hash1, hash2, "same input should produce same hash")
	assert.Len(t, hash1, 64, "SHA-256 hex digest should be 64 chars")
}

func TestHashToken_DifferentInputs(t *testing.T) {
	hash1 := hashToken("phk_token_a")
	hash2 := hashToken("phk_token_b")
	assert.NotEqual(t, hash1, hash2, "different inputs should produce different hashes")
}

func TestGenerateToken_Format(t *testing.T) {
	token, err := generateToken()
	assert.NoError(t, err)
	assert.True(t, strings.HasPrefix(token, TokenPrefix), "token should start with %s", TokenPrefix)
	assert.Len(t, token, 4+64, "token should be prefix(4) + hex(64) = 68 chars")
}

func TestGenerateToken_Unique(t *testing.T) {
	token1, err1 := generateToken()
	token2, err2 := generateToken()
	assert.NoError(t, err1)
	assert.NoError(t, err2)
	assert.NotEqual(t, token1, token2, "two generated tokens should be different")
}

// =============================================================================
// INTEGRATION TESTS (With Real Database)
// =============================================================================

type APITokenIntegrationTestSuite struct {
	suite.Suite
	container testcontainers.Container
	db        *gorm.DB
	svc       *APITokenService
	ctx       context.Context
}

func (suite *APITokenIntegrationTestSuite) SetupSuite() {
	suite.ctx = context.Background()

	container, err := testcontainers.GenericContainer(suite.ctx, testcontainers.GenericContainerRequest{
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
		suite.T().Fatalf("failed to start postgres container: %v", err)
	}
	suite.container = container

	host, err := container.Host(suite.ctx)
	if err != nil {
		suite.T().Fatalf("failed to get host: %v", err)
	}
	port, err := container.MappedPort(suite.ctx, "5432")
	if err != nil {
		suite.T().Fatalf("failed to get port: %v", err)
	}

	dsn := fmt.Sprintf("host=%s port=%s user=test_user password=test_password dbname=test_db sslmode=disable",
		host, port.Port())

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		suite.T().Fatalf("failed to connect to test database: %v", err)
	}
	suite.db = db

	sqlDB, err := db.DB()
	if err != nil {
		suite.T().Fatalf("failed to get sql.DB: %v", err)
	}

	// Migrations needed: 000001 (users), 000012, 000014, 000021 (api_tokens)
	migrations := []string{
		"000001_create_initial_schema.up.sql",
		"000012_add_user_deletion_fields.up.sql",
		"000014_add_account_lockout.up.sql",
		"000021_add_api_tokens.up.sql",
		"000031_add_user_terms_acceptance.up.sql",
	}
	for _, m := range migrations {
		migrationSQL, err := os.ReadFile(filepath.Join("..", "..", "db", "migrations", m))
		if err != nil {
			suite.T().Fatalf("failed to read migration file %s: %v", m, err)
		}
		_, err = sqlDB.Exec(string(migrationSQL))
		if err != nil {
			suite.T().Fatalf("failed to run migration %s: %v", m, err)
		}
	}

	suite.svc = &APITokenService{db: db}
}

func (suite *APITokenIntegrationTestSuite) TearDownSuite() {
	if suite.container != nil {
		if err := suite.container.Terminate(suite.ctx); err != nil {
			suite.T().Logf("failed to terminate container: %v", err)
		}
	}
}

func (suite *APITokenIntegrationTestSuite) TearDownTest() {
	sqlDB, err := suite.db.DB()
	suite.Require().NoError(err)
	_, _ = sqlDB.Exec("DELETE FROM api_tokens")
	_, _ = sqlDB.Exec("DELETE FROM users")
}

func TestAPITokenIntegrationTestSuite(t *testing.T) {
	suite.Run(t, new(APITokenIntegrationTestSuite))
}

// =============================================================================
// HELPERS
// =============================================================================

func (suite *APITokenIntegrationTestSuite) createTestUser(admin, active bool) *models.User {
	user := &models.User{
		Email:         stringPtr(fmt.Sprintf("token-user-%d@test.com", time.Now().UnixNano())),
		FirstName:     stringPtr("Token"),
		LastName:      stringPtr("User"),
		IsActive:      active,
		IsAdmin:       admin,
		EmailVerified: true,
	}
	err := suite.db.Create(user).Error
	suite.Require().NoError(err)
	return user
}

// =============================================================================
// CreateToken tests
// =============================================================================

func (suite *APITokenIntegrationTestSuite) TestCreateToken_Success() {
	user := suite.createTestUser(true, true)
	resp, err := suite.svc.CreateToken(user.ID, nil, 0)
	suite.Require().NoError(err)
	suite.Require().NotNil(resp)

	suite.True(strings.HasPrefix(resp.Token, TokenPrefix))
	suite.Equal("admin", resp.Scope)
	suite.NotZero(resp.ID)

	// Default expiration should be ~90 days
	expectedExpiry := time.Now().Add(90 * 24 * time.Hour)
	suite.InDelta(expectedExpiry.Unix(), resp.ExpiresAt.Unix(), 60, "expiry should be ~90 days from now")
}

func (suite *APITokenIntegrationTestSuite) TestCreateToken_CustomExpiration() {
	user := suite.createTestUser(true, true)
	desc := "30-day token"
	resp, err := suite.svc.CreateToken(user.ID, &desc, 30)
	suite.Require().NoError(err)
	suite.Require().NotNil(resp)

	expectedExpiry := time.Now().Add(30 * 24 * time.Hour)
	suite.InDelta(expectedExpiry.Unix(), resp.ExpiresAt.Unix(), 60, "expiry should be ~30 days from now")
	suite.Equal(&desc, resp.Description)
}

func (suite *APITokenIntegrationTestSuite) TestCreateToken_ZeroExpiration() {
	user := suite.createTestUser(true, true)
	resp, err := suite.svc.CreateToken(user.ID, nil, 0)
	suite.Require().NoError(err)
	suite.Require().NotNil(resp)

	// 0 falls back to 90-day default
	expectedExpiry := time.Now().Add(90 * 24 * time.Hour)
	suite.InDelta(expectedExpiry.Unix(), resp.ExpiresAt.Unix(), 60)
}

// =============================================================================
// ValidateToken tests
// =============================================================================

func (suite *APITokenIntegrationTestSuite) TestValidateToken_Success() {
	user := suite.createTestUser(true, true)
	createResp, err := suite.svc.CreateToken(user.ID, nil, 90)
	suite.Require().NoError(err)

	validatedUser, validatedToken, err := suite.svc.ValidateToken(createResp.Token)
	suite.Require().NoError(err)
	suite.Equal(user.ID, validatedUser.ID)
	suite.True(validatedUser.IsAdmin)
	suite.Equal(createResp.ID, validatedToken.ID)

	// Give the async goroutine time to update last_used_at
	time.Sleep(100 * time.Millisecond)
}

func (suite *APITokenIntegrationTestSuite) TestValidateToken_InvalidToken() {
	_, _, err := suite.svc.ValidateToken("phk_nonexistent_token_value")
	suite.Error(err)
	suite.Equal("invalid token", err.Error())
}

func (suite *APITokenIntegrationTestSuite) TestValidateToken_ExpiredToken() {
	user := suite.createTestUser(true, true)
	createResp, err := suite.svc.CreateToken(user.ID, nil, 90)
	suite.Require().NoError(err)

	// Manually set expires_at to the past
	suite.db.Model(&models.APIToken{}).Where("id = ?", createResp.ID).
		Update("expires_at", time.Now().Add(-1*time.Hour))

	_, _, err = suite.svc.ValidateToken(createResp.Token)
	suite.Error(err)
	suite.Equal("token has expired", err.Error())
}

func (suite *APITokenIntegrationTestSuite) TestValidateToken_RevokedToken() {
	user := suite.createTestUser(true, true)
	createResp, err := suite.svc.CreateToken(user.ID, nil, 90)
	suite.Require().NoError(err)

	err = suite.svc.RevokeToken(user.ID, createResp.ID)
	suite.Require().NoError(err)

	_, _, err = suite.svc.ValidateToken(createResp.Token)
	suite.Error(err)
	suite.Equal("token has been revoked", err.Error())
}

func (suite *APITokenIntegrationTestSuite) TestValidateToken_InactiveUser() {
	user := suite.createTestUser(true, true) // create as active first
	createResp, err := suite.svc.CreateToken(user.ID, nil, 90)
	suite.Require().NoError(err)

	// Deactivate the user after token creation (GORM skips false bool on Create due to zero value)
	suite.db.Model(&models.User{}).Where("id = ?", user.ID).Update("is_active", false)

	_, _, err = suite.svc.ValidateToken(createResp.Token)
	suite.Error(err)
	suite.Equal("user account is not active", err.Error())
}

func (suite *APITokenIntegrationTestSuite) TestValidateToken_NonAdminUser() {
	user := suite.createTestUser(false, true) // active but not admin
	createResp, err := suite.svc.CreateToken(user.ID, nil, 90)
	suite.Require().NoError(err)

	_, _, err = suite.svc.ValidateToken(createResp.Token)
	suite.Error(err)
	suite.Equal("user is not an admin", err.Error())
}

// =============================================================================
// ListTokens tests
// =============================================================================

func (suite *APITokenIntegrationTestSuite) TestListTokens_Success() {
	user := suite.createTestUser(true, true)
	_, err := suite.svc.CreateToken(user.ID, nil, 90)
	suite.Require().NoError(err)
	time.Sleep(10 * time.Millisecond) // ensure different created_at
	_, err = suite.svc.CreateToken(user.ID, nil, 90)
	suite.Require().NoError(err)

	tokens, err := suite.svc.ListTokens(user.ID)
	suite.Require().NoError(err)
	suite.Len(tokens, 2)
	// Ordered by created_at DESC — newer first
	suite.True(tokens[0].CreatedAt.After(tokens[1].CreatedAt) || tokens[0].CreatedAt.Equal(tokens[1].CreatedAt))
}

func (suite *APITokenIntegrationTestSuite) TestListTokens_ExcludesRevoked() {
	user := suite.createTestUser(true, true)
	resp1, err := suite.svc.CreateToken(user.ID, nil, 90)
	suite.Require().NoError(err)
	_, err = suite.svc.CreateToken(user.ID, nil, 90)
	suite.Require().NoError(err)

	err = suite.svc.RevokeToken(user.ID, resp1.ID)
	suite.Require().NoError(err)

	tokens, err := suite.svc.ListTokens(user.ID)
	suite.Require().NoError(err)
	suite.Len(tokens, 1)
}

func (suite *APITokenIntegrationTestSuite) TestListTokens_Empty() {
	user := suite.createTestUser(true, true)
	tokens, err := suite.svc.ListTokens(user.ID)
	suite.Require().NoError(err)
	suite.Empty(tokens)
}

// =============================================================================
// RevokeToken tests
// =============================================================================

func (suite *APITokenIntegrationTestSuite) TestRevokeToken_Success() {
	user := suite.createTestUser(true, true)
	resp, err := suite.svc.CreateToken(user.ID, nil, 90)
	suite.Require().NoError(err)

	err = suite.svc.RevokeToken(user.ID, resp.ID)
	suite.NoError(err)
}

func (suite *APITokenIntegrationTestSuite) TestRevokeToken_NotFound() {
	user := suite.createTestUser(true, true)
	err := suite.svc.RevokeToken(user.ID, 99999)
	suite.Error(err)
	suite.Contains(err.Error(), "not found or already revoked")
}

func (suite *APITokenIntegrationTestSuite) TestRevokeToken_AlreadyRevoked() {
	user := suite.createTestUser(true, true)
	resp, err := suite.svc.CreateToken(user.ID, nil, 90)
	suite.Require().NoError(err)

	err = suite.svc.RevokeToken(user.ID, resp.ID)
	suite.Require().NoError(err)

	err = suite.svc.RevokeToken(user.ID, resp.ID)
	suite.Error(err)
	suite.Contains(err.Error(), "not found or already revoked")
}

func (suite *APITokenIntegrationTestSuite) TestRevokeToken_WrongUser() {
	user1 := suite.createTestUser(true, true)
	user2 := suite.createTestUser(true, true)

	resp, err := suite.svc.CreateToken(user1.ID, nil, 90)
	suite.Require().NoError(err)

	err = suite.svc.RevokeToken(user2.ID, resp.ID)
	suite.Error(err)
	suite.Contains(err.Error(), "not found or already revoked")
}

// =============================================================================
// GetToken tests
// =============================================================================

func (suite *APITokenIntegrationTestSuite) TestGetToken_Success() {
	user := suite.createTestUser(true, true)
	desc := "my test token"
	createResp, err := suite.svc.CreateToken(user.ID, &desc, 90)
	suite.Require().NoError(err)

	getResp, err := suite.svc.GetToken(user.ID, createResp.ID)
	suite.Require().NoError(err)
	suite.Equal(createResp.ID, getResp.ID)
	suite.Equal("admin", getResp.Scope)
	suite.Equal(&desc, getResp.Description)
}

func (suite *APITokenIntegrationTestSuite) TestGetToken_NotFound() {
	user := suite.createTestUser(true, true)
	_, err := suite.svc.GetToken(user.ID, 99999)
	suite.Error(err)
	suite.Equal("token not found", err.Error())
}

func (suite *APITokenIntegrationTestSuite) TestGetToken_WrongUser() {
	user1 := suite.createTestUser(true, true)
	user2 := suite.createTestUser(true, true)

	resp, err := suite.svc.CreateToken(user1.ID, nil, 90)
	suite.Require().NoError(err)

	_, err = suite.svc.GetToken(user2.ID, resp.ID)
	suite.Error(err)
	suite.Equal("token not found", err.Error())
}

// =============================================================================
// CleanupExpiredTokens tests
// =============================================================================

func (suite *APITokenIntegrationTestSuite) TestCleanupExpiredTokens_RemovesOld() {
	user := suite.createTestUser(true, true)
	resp, err := suite.svc.CreateToken(user.ID, nil, 90)
	suite.Require().NoError(err)

	// Set expires_at to 31+ days ago
	suite.db.Model(&models.APIToken{}).Where("id = ?", resp.ID).
		Update("expires_at", time.Now().Add(-32*24*time.Hour))

	count, err := suite.svc.CleanupExpiredTokens()
	suite.Require().NoError(err)
	suite.Equal(int64(1), count)

	// Verify it's gone
	var remaining int64
	suite.db.Model(&models.APIToken{}).Where("id = ?", resp.ID).Count(&remaining)
	suite.Zero(remaining)
}

func (suite *APITokenIntegrationTestSuite) TestCleanupExpiredTokens_KeepsRecent() {
	user := suite.createTestUser(true, true)
	resp, err := suite.svc.CreateToken(user.ID, nil, 90)
	suite.Require().NoError(err)

	// Set expires_at to 1 day ago (within 30-day retention)
	suite.db.Model(&models.APIToken{}).Where("id = ?", resp.ID).
		Update("expires_at", time.Now().Add(-1*24*time.Hour))

	count, err := suite.svc.CleanupExpiredTokens()
	suite.Require().NoError(err)
	suite.Equal(int64(0), count)
}
