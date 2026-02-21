package services

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"psychic-homily-backend/internal/config"
	"psychic-homily-backend/internal/models"
)

// =============================================================================
// UNIT TESTS (No Database Required)
// =============================================================================

func TestNewWebAuthnService_Success(t *testing.T) {
	cfg := &config.Config{
		WebAuthn: config.WebAuthnConfig{
			RPDisplayName: "Test App",
			RPID:          "localhost",
			RPOrigins:     []string{"http://localhost:3000"},
		},
		Email: config.EmailConfig{FrontendURL: "http://localhost:3000"},
	}
	// Pass a non-nil gorm.DB to avoid calling db.GetDB()
	svc, err := NewWebAuthnService(&gorm.DB{}, cfg)
	assert.NoError(t, err)
	assert.NotNil(t, svc)
}

func TestNewWebAuthnService_DefaultRPID(t *testing.T) {
	cfg := &config.Config{
		WebAuthn: config.WebAuthnConfig{
			RPDisplayName: "Test App",
			RPID:          "", // Empty — should default to "localhost"
			RPOrigins:     []string{"http://localhost:3000"},
		},
		Email: config.EmailConfig{FrontendURL: "http://localhost:3000"},
	}
	svc, err := NewWebAuthnService(&gorm.DB{}, cfg)
	assert.NoError(t, err)
	assert.NotNil(t, svc)
}

func TestNewWebAuthnService_DefaultOrigins(t *testing.T) {
	cfg := &config.Config{
		WebAuthn: config.WebAuthnConfig{
			RPDisplayName: "Test App",
			RPID:          "localhost",
			RPOrigins:     nil, // Empty — should use FrontendURL
		},
		Email: config.EmailConfig{FrontendURL: "http://localhost:3000"},
	}
	svc, err := NewWebAuthnService(&gorm.DB{}, cfg)
	assert.NoError(t, err)
	assert.NotNil(t, svc)
}

// =============================================================================
// INTEGRATION TESTS (With Real Database)
// =============================================================================

type WebAuthnServiceIntegrationTestSuite struct {
	suite.Suite
	container testcontainers.Container
	db        *gorm.DB
	service   *WebAuthnService
	ctx       context.Context
}

func (suite *WebAuthnServiceIntegrationTestSuite) SetupSuite() {
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

	// Migrations needed for webauthn: users + webauthn tables + user columns GORM expects
	migrations := []string{
		"000001_create_initial_schema.up.sql",
		"000005_add_show_status.up.sql",
		"000011_add_webauthn_tables.up.sql",
		"000012_add_user_deletion_fields.up.sql",
		"000014_add_account_lockout.up.sql",
		"000031_add_user_terms_acceptance.up.sql",
		"000032_add_favorite_cities.up.sql",
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

	cfg := &config.Config{
		WebAuthn: config.WebAuthnConfig{
			RPDisplayName: "Test App",
			RPID:          "localhost",
			RPOrigins:     []string{"http://localhost:3000"},
		},
		Email: config.EmailConfig{FrontendURL: "http://localhost:3000"},
	}

	svc, err := NewWebAuthnService(db, cfg)
	if err != nil {
		suite.T().Fatalf("failed to create webauthn service: %v", err)
	}
	suite.service = svc
}

func (suite *WebAuthnServiceIntegrationTestSuite) TearDownSuite() {
	if suite.container != nil {
		if err := suite.container.Terminate(suite.ctx); err != nil {
			suite.T().Logf("failed to terminate container: %v", err)
		}
	}
}

func (suite *WebAuthnServiceIntegrationTestSuite) TearDownTest() {
	sqlDB, err := suite.db.DB()
	suite.Require().NoError(err)
	_, _ = sqlDB.Exec("DELETE FROM webauthn_challenges")
	_, _ = sqlDB.Exec("DELETE FROM webauthn_credentials")
	_, _ = sqlDB.Exec("DELETE FROM users")
}

func TestWebAuthnServiceIntegrationTestSuite(t *testing.T) {
	suite.Run(t, new(WebAuthnServiceIntegrationTestSuite))
}

// =============================================================================
// HELPERS
// =============================================================================

func (suite *WebAuthnServiceIntegrationTestSuite) createUser(email string) *models.User {
	user := &models.User{
		Email:         &email,
		IsActive:      true,
		EmailVerified: true,
	}
	err := suite.db.Create(user).Error
	suite.Require().NoError(err)
	return user
}

// insertCredential uses raw SQL to avoid GORM AAGUID→aa_guid mapping issue
func (suite *WebAuthnServiceIntegrationTestSuite) insertCredential(userID uint, displayName string) uint {
	var id uint
	err := suite.db.Raw(
		`INSERT INTO webauthn_credentials (user_id, credential_id, public_key, display_name, created_at, updated_at)
		 VALUES (?, ?, ?, ?, NOW(), NOW()) RETURNING id`,
		userID, []byte(fmt.Sprintf("cred-%d-%s", userID, displayName)), []byte("test-pk"), displayName,
	).Scan(&id).Error
	suite.Require().NoError(err)
	return id
}

func (suite *WebAuthnServiceIntegrationTestSuite) insertCredentialWithTime(userID uint, displayName string, createdAt time.Time) uint {
	var id uint
	err := suite.db.Raw(
		`INSERT INTO webauthn_credentials (user_id, credential_id, public_key, display_name, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?) RETURNING id`,
		userID, []byte(fmt.Sprintf("cred-%d-%s", userID, displayName)), []byte("test-pk"), displayName, createdAt, createdAt,
	).Scan(&id).Error
	suite.Require().NoError(err)
	return id
}

// =============================================================================
// Credential Management Tests
// =============================================================================

func (suite *WebAuthnServiceIntegrationTestSuite) TestGetUserCredentials_Empty() {
	user := suite.createUser("user@test.com")
	creds, err := suite.service.GetUserCredentials(user.ID)
	suite.Require().NoError(err)
	suite.Empty(creds)
}

func (suite *WebAuthnServiceIntegrationTestSuite) TestGetUserCredentials_Multiple() {
	user := suite.createUser("user@test.com")
	// Create 2 creds at different times — ordered by created_at DESC
	suite.insertCredentialWithTime(user.ID, "Key 1", time.Now().Add(-1*time.Hour))
	suite.insertCredentialWithTime(user.ID, "Key 2", time.Now())

	creds, err := suite.service.GetUserCredentials(user.ID)
	suite.Require().NoError(err)
	suite.Len(creds, 2)
	// Most recent first
	suite.Equal("Key 2", creds[0].DisplayName)
	suite.Equal("Key 1", creds[1].DisplayName)
}

func (suite *WebAuthnServiceIntegrationTestSuite) TestGetUserCredentials_WrongUser() {
	user1 := suite.createUser("user1@test.com")
	user2 := suite.createUser("user2@test.com")
	suite.insertCredential(user1.ID, "Key 1")

	creds, err := suite.service.GetUserCredentials(user2.ID)
	suite.Require().NoError(err)
	suite.Empty(creds)
}

func (suite *WebAuthnServiceIntegrationTestSuite) TestDeleteCredential_Success() {
	user := suite.createUser("user@test.com")
	credID := suite.insertCredential(user.ID, "Key 1")

	err := suite.service.DeleteCredential(user.ID, credID)
	suite.Require().NoError(err)

	// Verify deleted
	creds, _ := suite.service.GetUserCredentials(user.ID)
	suite.Empty(creds)
}

func (suite *WebAuthnServiceIntegrationTestSuite) TestDeleteCredential_NotFound() {
	user := suite.createUser("user@test.com")
	err := suite.service.DeleteCredential(user.ID, 99999)
	suite.Require().Error(err)
	suite.Contains(err.Error(), "credential not found")
}

func (suite *WebAuthnServiceIntegrationTestSuite) TestDeleteCredential_WrongUser() {
	user1 := suite.createUser("user1@test.com")
	user2 := suite.createUser("user2@test.com")
	credID := suite.insertCredential(user1.ID, "Key 1")

	err := suite.service.DeleteCredential(user2.ID, credID)
	suite.Require().Error(err)
	suite.Contains(err.Error(), "credential not found")
}

func (suite *WebAuthnServiceIntegrationTestSuite) TestUpdateCredentialName_Success() {
	user := suite.createUser("user@test.com")
	credID := suite.insertCredential(user.ID, "Old Name")

	err := suite.service.UpdateCredentialName(user.ID, credID, "New Name")
	suite.Require().NoError(err)

	// Verify updated
	creds, _ := suite.service.GetUserCredentials(user.ID)
	suite.Require().Len(creds, 1)
	suite.Equal("New Name", creds[0].DisplayName)
}

func (suite *WebAuthnServiceIntegrationTestSuite) TestUpdateCredentialName_NotFound() {
	user := suite.createUser("user@test.com")
	err := suite.service.UpdateCredentialName(user.ID, 99999, "New Name")
	suite.Require().Error(err)
	suite.Contains(err.Error(), "credential not found")
}

func (suite *WebAuthnServiceIntegrationTestSuite) TestUpdateCredentialName_WrongUser() {
	user1 := suite.createUser("user1@test.com")
	user2 := suite.createUser("user2@test.com")
	credID := suite.insertCredential(user1.ID, "Key 1")

	err := suite.service.UpdateCredentialName(user2.ID, credID, "Stolen Name")
	suite.Require().Error(err)
	suite.Contains(err.Error(), "credential not found")
}

// =============================================================================
// Challenge Storage Tests
// =============================================================================

func (suite *WebAuthnServiceIntegrationTestSuite) TestStoreChallenge_Success() {
	user := suite.createUser("user@test.com")
	session := &webauthn.SessionData{
		Challenge:        "test-challenge",
		UserID:           []byte("user-id"),
		UserVerification: protocol.VerificationPreferred,
	}

	challengeID, err := suite.service.StoreChallenge(user.ID, session, "registration")
	suite.Require().NoError(err)
	suite.NotEmpty(challengeID)
}

func (suite *WebAuthnServiceIntegrationTestSuite) TestGetChallenge_Success() {
	user := suite.createUser("user@test.com")
	session := &webauthn.SessionData{
		Challenge:        "test-challenge-data",
		UserID:           []byte("user-id"),
		UserVerification: protocol.VerificationPreferred,
	}

	challengeID, err := suite.service.StoreChallenge(user.ID, session, "registration")
	suite.Require().NoError(err)

	retrieved, userID, err := suite.service.GetChallenge(challengeID, "registration")
	suite.Require().NoError(err)
	suite.Equal(user.ID, userID)
	suite.Equal("test-challenge-data", retrieved.Challenge)
}

func (suite *WebAuthnServiceIntegrationTestSuite) TestGetChallenge_NotFound() {
	_, _, err := suite.service.GetChallenge("non-existent-id", "registration")
	suite.Require().Error(err)
	suite.Contains(err.Error(), "challenge not found")
}

func (suite *WebAuthnServiceIntegrationTestSuite) TestGetChallenge_WrongOperation() {
	user := suite.createUser("user@test.com")
	session := &webauthn.SessionData{
		Challenge: "test-challenge",
		UserID:    []byte("user-id"),
	}

	challengeID, err := suite.service.StoreChallenge(user.ID, session, "registration")
	suite.Require().NoError(err)

	_, _, err = suite.service.GetChallenge(challengeID, "authentication")
	suite.Require().Error(err)
	suite.Contains(err.Error(), "challenge not found")
}

func (suite *WebAuthnServiceIntegrationTestSuite) TestGetChallenge_Expired() {
	user := suite.createUser("user@test.com")
	session := &webauthn.SessionData{
		Challenge: "test-challenge",
		UserID:    []byte("user-id"),
	}

	challengeID, err := suite.service.StoreChallenge(user.ID, session, "registration")
	suite.Require().NoError(err)

	// Force-expire the challenge via raw SQL
	suite.db.Exec("UPDATE webauthn_challenges SET expires_at = ? WHERE id = ?",
		time.Now().Add(-1*time.Hour), challengeID)

	_, _, err = suite.service.GetChallenge(challengeID, "registration")
	suite.Require().Error(err)
	suite.Contains(err.Error(), "challenge expired")
}

func (suite *WebAuthnServiceIntegrationTestSuite) TestDeleteChallenge_Success() {
	user := suite.createUser("user@test.com")
	session := &webauthn.SessionData{
		Challenge: "test-challenge",
		UserID:    []byte("user-id"),
	}

	challengeID, err := suite.service.StoreChallenge(user.ID, session, "registration")
	suite.Require().NoError(err)

	err = suite.service.DeleteChallenge(challengeID)
	suite.Require().NoError(err)

	// Verify it's gone
	_, _, err = suite.service.GetChallenge(challengeID, "registration")
	suite.Require().Error(err)
	suite.Contains(err.Error(), "challenge not found")
}

func (suite *WebAuthnServiceIntegrationTestSuite) TestCleanupExpiredChallenges_Success() {
	user := suite.createUser("user@test.com")

	// Create an active challenge
	activeSession := &webauthn.SessionData{
		Challenge: "active-challenge",
		UserID:    []byte("user-id"),
	}
	activeID, err := suite.service.StoreChallenge(user.ID, activeSession, "registration")
	suite.Require().NoError(err)

	// Create an expired challenge
	expiredSession := &webauthn.SessionData{
		Challenge: "expired-challenge",
		UserID:    []byte("user-id"),
	}
	expiredID, err := suite.service.StoreChallenge(user.ID, expiredSession, "authentication")
	suite.Require().NoError(err)
	suite.db.Exec("UPDATE webauthn_challenges SET expires_at = ? WHERE id = ?",
		time.Now().Add(-1*time.Hour), expiredID)

	// Cleanup
	err = suite.service.CleanupExpiredChallenges()
	suite.Require().NoError(err)

	// Active should still exist
	_, _, err = suite.service.GetChallenge(activeID, "registration")
	suite.Require().NoError(err)

	// Expired should be gone
	_, _, err = suite.service.GetChallenge(expiredID, "authentication")
	suite.Require().Error(err)
}

func (suite *WebAuthnServiceIntegrationTestSuite) TestStoreAndGetChallenge_RoundTrip() {
	user := suite.createUser("user@test.com")
	session := &webauthn.SessionData{
		Challenge:        "round-trip-challenge",
		UserID:           []byte("round-trip-user"),
		UserVerification: protocol.VerificationRequired,
	}

	challengeID, err := suite.service.StoreChallenge(user.ID, session, "registration")
	suite.Require().NoError(err)

	retrieved, userID, err := suite.service.GetChallenge(challengeID, "registration")
	suite.Require().NoError(err)
	suite.Equal(user.ID, userID)
	suite.Equal(session.Challenge, retrieved.Challenge)
	suite.Equal(session.UserVerification, retrieved.UserVerification)
}

// =============================================================================
// Challenge with Email Tests (signup flow)
// =============================================================================

func (suite *WebAuthnServiceIntegrationTestSuite) TestStoreChallengeWithEmail_Success() {
	session := &webauthn.SessionData{
		Challenge: "signup-challenge",
		UserID:    []byte("temp-user"),
	}

	challengeID, err := suite.service.StoreChallengeWithEmail("new@test.com", session, "signup_registration")
	suite.Require().NoError(err)
	suite.NotEmpty(challengeID)
}

func (suite *WebAuthnServiceIntegrationTestSuite) TestGetChallengeWithEmail_Success() {
	session := &webauthn.SessionData{
		Challenge: "signup-challenge-data",
		UserID:    []byte("temp-user"),
	}

	challengeID, err := suite.service.StoreChallengeWithEmail("new@test.com", session, "signup_registration")
	suite.Require().NoError(err)

	retrieved, email, err := suite.service.GetChallengeWithEmail(challengeID, "signup_registration")
	suite.Require().NoError(err)
	suite.Equal("new@test.com", email)
	suite.Equal("signup-challenge-data", retrieved.Challenge)
}

func (suite *WebAuthnServiceIntegrationTestSuite) TestGetChallengeWithEmail_Expired() {
	session := &webauthn.SessionData{
		Challenge: "expired-signup",
		UserID:    []byte("temp-user"),
	}

	challengeID, err := suite.service.StoreChallengeWithEmail("new@test.com", session, "signup_registration")
	suite.Require().NoError(err)

	suite.db.Exec("UPDATE webauthn_challenges SET expires_at = ? WHERE id = ?",
		time.Now().Add(-1*time.Hour), challengeID)

	_, _, err = suite.service.GetChallengeWithEmail(challengeID, "signup_registration")
	suite.Require().Error(err)
	suite.Contains(err.Error(), "challenge expired")
}

func (suite *WebAuthnServiceIntegrationTestSuite) TestGetChallengeWithEmail_NotFound() {
	_, _, err := suite.service.GetChallengeWithEmail("non-existent-id", "signup_registration")
	suite.Require().Error(err)
	suite.Contains(err.Error(), "challenge not found")
}

func (suite *WebAuthnServiceIntegrationTestSuite) TestStoreAndGetChallengeWithEmail_RoundTrip() {
	session := &webauthn.SessionData{
		Challenge:        "signup-round-trip",
		UserID:           []byte("temp-user"),
		UserVerification: protocol.VerificationPreferred,
	}

	challengeID, err := suite.service.StoreChallengeWithEmail("roundtrip@test.com", session, "signup_registration")
	suite.Require().NoError(err)

	retrieved, email, err := suite.service.GetChallengeWithEmail(challengeID, "signup_registration")
	suite.Require().NoError(err)
	suite.Equal("roundtrip@test.com", email)
	suite.Equal(session.Challenge, retrieved.Challenge)
	suite.Equal(session.UserVerification, retrieved.UserVerification)
}

// =============================================================================
// Library Integration Tests (Begin* methods)
// =============================================================================

func (suite *WebAuthnServiceIntegrationTestSuite) TestBeginRegistration_Success() {
	email := "user@test.com"
	user := &models.User{
		Email:         &email,
		IsActive:      true,
		EmailVerified: true,
	}
	suite.Require().NoError(suite.db.Create(user).Error)

	options, session, err := suite.service.BeginRegistration(user)
	suite.Require().NoError(err)
	suite.NotNil(options)
	suite.NotNil(session)
	suite.NotEmpty(session.Challenge)
}

func (suite *WebAuthnServiceIntegrationTestSuite) TestBeginRegistration_WithExclusions() {
	email := "user@test.com"
	user := &models.User{
		Email:         &email,
		IsActive:      true,
		EmailVerified: true,
	}
	suite.Require().NoError(suite.db.Create(user).Error)

	// Add an existing credential
	suite.insertCredential(user.ID, "Existing Key")

	options, session, err := suite.service.BeginRegistration(user)
	suite.Require().NoError(err)
	suite.NotNil(options)
	suite.NotNil(session)
	// The exclusion list should be populated
	suite.Len(options.Response.CredentialExcludeList, 1)
}

func (suite *WebAuthnServiceIntegrationTestSuite) TestBeginLogin_NoCredentials() {
	email := "user@test.com"
	user := &models.User{
		Email:         &email,
		IsActive:      true,
		EmailVerified: true,
	}
	suite.Require().NoError(suite.db.Create(user).Error)

	_, _, err := suite.service.BeginLogin(user)
	suite.Require().Error(err)
	suite.Contains(err.Error(), "user has no registered passkeys")
}

func (suite *WebAuthnServiceIntegrationTestSuite) TestBeginLogin_WithCredentials() {
	email := "user@test.com"
	user := &models.User{
		Email:         &email,
		IsActive:      true,
		EmailVerified: true,
	}
	suite.Require().NoError(suite.db.Create(user).Error)
	suite.insertCredential(user.ID, "My Key")

	options, session, err := suite.service.BeginLogin(user)
	suite.Require().NoError(err)
	suite.NotNil(options)
	suite.NotNil(session)
	suite.NotEmpty(session.Challenge)
}

func (suite *WebAuthnServiceIntegrationTestSuite) TestBeginDiscoverableLogin_Success() {
	options, session, err := suite.service.BeginDiscoverableLogin()
	suite.Require().NoError(err)
	suite.NotNil(options)
	suite.NotNil(session)
	suite.NotEmpty(session.Challenge)
}

func (suite *WebAuthnServiceIntegrationTestSuite) TestBeginRegistrationForEmail_Success() {
	options, session, err := suite.service.BeginRegistrationForEmail("signup@test.com")
	suite.Require().NoError(err)
	suite.NotNil(options)
	suite.NotNil(session)
	suite.NotEmpty(session.Challenge)
}

// =============================================================================
// Nil DB Tests
// =============================================================================

func TestWebAuthnService_NilDB_GetUserCredentials(t *testing.T) {
	cfg := &config.Config{
		WebAuthn: config.WebAuthnConfig{
			RPDisplayName: "Test",
			RPID:          "localhost",
			RPOrigins:     []string{"http://localhost:3000"},
		},
		Email: config.EmailConfig{FrontendURL: "http://localhost:3000"},
	}
	svc, err := NewWebAuthnService(&gorm.DB{}, cfg)
	assert.NoError(t, err)

	// gorm.DB{} has nil internals — operations panic
	assert.Panics(t, func() {
		svc.GetUserCredentials(1)
	})
}

func TestWebAuthnService_NilDB_StoreChallenge(t *testing.T) {
	cfg := &config.Config{
		WebAuthn: config.WebAuthnConfig{
			RPDisplayName: "Test",
			RPID:          "localhost",
			RPOrigins:     []string{"http://localhost:3000"},
		},
		Email: config.EmailConfig{FrontendURL: "http://localhost:3000"},
	}
	svc, err := NewWebAuthnService(&gorm.DB{}, cfg)
	assert.NoError(t, err)

	session := &webauthn.SessionData{Challenge: "test"}
	assert.Panics(t, func() {
		svc.StoreChallenge(1, session, "registration")
	})
}

func TestWebAuthnService_NilDB_DeleteCredential(t *testing.T) {
	cfg := &config.Config{
		WebAuthn: config.WebAuthnConfig{
			RPDisplayName: "Test",
			RPID:          "localhost",
			RPOrigins:     []string{"http://localhost:3000"},
		},
		Email: config.EmailConfig{FrontendURL: "http://localhost:3000"},
	}
	svc, err := NewWebAuthnService(&gorm.DB{}, cfg)
	assert.NoError(t, err)

	assert.Panics(t, func() {
		svc.DeleteCredential(1, 1)
	})
}
