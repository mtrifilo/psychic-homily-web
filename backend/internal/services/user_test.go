package services

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/markbates/goth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	apperrors "psychic-homily-backend/internal/errors"
	"psychic-homily-backend/internal/models"
)

// =============================================================================
// UNIT TESTS (No Database Required)
// =============================================================================

// TestNewUserService tests the creation of a new UserService
func TestNewUserService(t *testing.T) {
	userService := NewUserService(nil)

	assert.NotNil(t, userService)
	// In test environment, database may be nil
	if userService.db == nil {
		t.Log("Database is nil in test environment (expected)")
	}
}

// TestUserService_NilDatabase tests all methods with nil database
func TestUserService_NilDatabase(t *testing.T) {
	userService := &UserService{db: nil}

	t.Run("GetUserByID", func(t *testing.T) {
		user, err := userService.GetUserByID(1)
		assert.Error(t, err)
		assert.Equal(t, "database not initialized", err.Error())
		assert.Nil(t, user)
	})

	t.Run("GetUserByEmail", func(t *testing.T) {
		user, err := userService.GetUserByEmail("test@example.com")
		assert.Error(t, err)
		assert.Equal(t, "database not initialized", err.Error())
		assert.Nil(t, user)
	})

	t.Run("GetUserByUsername", func(t *testing.T) {
		user, err := userService.GetUserByUsername("testuser")
		assert.Error(t, err)
		assert.Equal(t, "database not initialized", err.Error())
		assert.Nil(t, user)
	})

	t.Run("UpdateUser", func(t *testing.T) {
		user, err := userService.UpdateUser(1, map[string]any{"first_name": "Test"})
		assert.Error(t, err)
		assert.Equal(t, "database not initialized", err.Error())
		assert.Nil(t, user)
	})

	t.Run("FindOrCreateUser", func(t *testing.T) {
		gothUser := goth.User{
			UserID: "12345",
			Email:  "test@example.com",
		}
		user, err := userService.FindOrCreateUser(gothUser, "google")
		assert.Error(t, err)
		assert.Equal(t, "database not initialized", err.Error())
		assert.Nil(t, user)
	})

	t.Run("createNewUser", func(t *testing.T) {
		gothUser := goth.User{
			UserID: "12345",
			Email:  "test@example.com",
		}
		user, err := userService.createNewUserOauth(gothUser, "google")
		assert.Error(t, err)
		assert.Equal(t, "database not initialized", err.Error())
		assert.Nil(t, user)
	})

	t.Run("linkOAuthAccount", func(t *testing.T) {
		existingUser := &models.User{
			ID:    1,
			Email: &[]string{"test@example.com"}[0],
		}
		gothUser := goth.User{
			UserID: "12345",
			Email:  "test@example.com",
		}
		user, err := userService.linkOAuthAccount(existingUser, gothUser, "google")
		assert.Error(t, err)
		assert.Equal(t, "database not initialized", err.Error())
		assert.Nil(t, user)
	})

	t.Run("CreateUserWithPassword", func(t *testing.T) {
		user, err := userService.CreateUserWithPassword("test@example.com", "password", "First", "Last")
		assert.Error(t, err)
		assert.Equal(t, "database not initialized", err.Error())
		assert.Nil(t, user)
	})

	t.Run("AuthenticateUserWithPassword", func(t *testing.T) {
		user, err := userService.AuthenticateUserWithPassword("test@example.com", "password")
		assert.Error(t, err)
		assert.Equal(t, "database not initialized", err.Error())
		assert.Nil(t, user)
	})

	t.Run("CreateUserWithoutPassword", func(t *testing.T) {
		user, err := userService.CreateUserWithoutPassword("test@example.com")
		assert.Error(t, err)
		assert.Equal(t, "database not initialized", err.Error())
		assert.Nil(t, user)
	})

	t.Run("SoftDeleteAccount", func(t *testing.T) {
		err := userService.SoftDeleteAccount(1, nil)
		assert.Error(t, err)
		assert.Equal(t, "database not initialized", err.Error())
	})

	t.Run("GetDeletionSummary", func(t *testing.T) {
		summary, err := userService.GetDeletionSummary(1)
		assert.Error(t, err)
		assert.Equal(t, "database not initialized", err.Error())
		assert.Nil(t, summary)
	})

	t.Run("ExportUserData", func(t *testing.T) {
		export, err := userService.ExportUserData(1)
		assert.Error(t, err)
		assert.Equal(t, "database not initialized", err.Error())
		assert.Nil(t, export)
	})

	t.Run("GetUserByEmailIncludingDeleted", func(t *testing.T) {
		user, err := userService.GetUserByEmailIncludingDeleted("test@example.com")
		assert.Error(t, err)
		assert.Equal(t, "database not initialized", err.Error())
		assert.Nil(t, user)
	})

	t.Run("ListUsers", func(t *testing.T) {
		users, total, err := userService.ListUsers(10, 0, AdminUserFilters{})
		assert.Error(t, err)
		assert.Equal(t, "database not initialized", err.Error())
		assert.Nil(t, users)
		assert.Equal(t, int64(0), total)
	})

	t.Run("RestoreAccount", func(t *testing.T) {
		err := userService.RestoreAccount(1)
		assert.Error(t, err)
		assert.Equal(t, "database not initialized", err.Error())
	})

	t.Run("GetExpiredDeletedAccounts", func(t *testing.T) {
		users, err := userService.GetExpiredDeletedAccounts()
		assert.Error(t, err)
		assert.Equal(t, "database not initialized", err.Error())
		assert.Nil(t, users)
	})

	t.Run("PermanentlyDeleteUser", func(t *testing.T) {
		err := userService.PermanentlyDeleteUser(1)
		assert.Error(t, err)
		assert.Equal(t, "database not initialized", err.Error())
	})

	t.Run("IncrementFailedAttempts", func(t *testing.T) {
		err := userService.IncrementFailedAttempts(1)
		assert.Error(t, err)
		assert.Equal(t, "database not initialized", err.Error())
	})

	t.Run("ResetFailedAttempts", func(t *testing.T) {
		err := userService.ResetFailedAttempts(1)
		assert.Error(t, err)
		assert.Equal(t, "database not initialized", err.Error())
	})

	t.Run("UpdatePassword", func(t *testing.T) {
		err := userService.UpdatePassword(1, "old", "new")
		assert.Error(t, err)
		assert.Equal(t, "database not initialized", err.Error())
	})

	t.Run("SetEmailVerified", func(t *testing.T) {
		err := userService.SetEmailVerified(1, true)
		assert.Error(t, err)
		assert.Equal(t, "database not initialized", err.Error())
	})
}

// TestUserService_EdgeCases tests edge cases and boundary conditions
func TestUserService_EdgeCases(t *testing.T) {
	userService := &UserService{db: nil}

	t.Run("GetUserByID with zero ID", func(t *testing.T) {
		user, err := userService.GetUserByID(0)
		assert.Error(t, err)
		assert.Equal(t, "database not initialized", err.Error())
		assert.Nil(t, user)
	})

	t.Run("GetUserByEmail with empty email", func(t *testing.T) {
		user, err := userService.GetUserByEmail("")
		assert.Error(t, err)
		assert.Equal(t, "database not initialized", err.Error())
		assert.Nil(t, user)
	})

	t.Run("GetUserByUsername with empty username", func(t *testing.T) {
		user, err := userService.GetUserByUsername("")
		assert.Error(t, err)
		assert.Equal(t, "database not initialized", err.Error())
		assert.Nil(t, user)
	})

	t.Run("UpdateUser with empty updates map", func(t *testing.T) {
		user, err := userService.UpdateUser(1, map[string]any{})
		assert.Error(t, err)
		assert.Equal(t, "database not initialized", err.Error())
		assert.Nil(t, user)
	})
}

// =============================================================================
// PURE LOGIC UNIT TESTS (No Database Required)
// =============================================================================

func TestHashPassword_Success(t *testing.T) {
	svc := &UserService{}
	hash, err := svc.HashPassword("mysecretpassword")
	assert.NoError(t, err)
	assert.NotEmpty(t, hash)
	assert.NotEqual(t, "mysecretpassword", hash)
	// bcrypt hashes start with $2a$ or $2b$
	assert.Contains(t, hash, "$2a$")
}

func TestVerifyPassword_Success(t *testing.T) {
	svc := &UserService{}
	hash, err := svc.HashPassword("correctpassword")
	assert.NoError(t, err)

	err = svc.VerifyPassword(hash, "correctpassword")
	assert.NoError(t, err)
}

func TestVerifyPassword_Wrong(t *testing.T) {
	svc := &UserService{}
	hash, err := svc.HashPassword("correctpassword")
	assert.NoError(t, err)

	err = svc.VerifyPassword(hash, "wrongpassword")
	assert.Error(t, err)
}

func TestIsAccountLocked_NotLocked(t *testing.T) {
	svc := &UserService{}
	user := &models.User{LockedUntil: nil}
	assert.False(t, svc.IsAccountLocked(user))
}

func TestIsAccountLocked_Locked(t *testing.T) {
	svc := &UserService{}
	future := time.Now().Add(10 * time.Minute)
	user := &models.User{LockedUntil: &future}
	assert.True(t, svc.IsAccountLocked(user))
}

func TestIsAccountLocked_Expired(t *testing.T) {
	svc := &UserService{}
	past := time.Now().Add(-10 * time.Minute)
	user := &models.User{LockedUntil: &past}
	assert.False(t, svc.IsAccountLocked(user))
}

func TestGetLockTimeRemaining(t *testing.T) {
	svc := &UserService{}

	t.Run("nil LockedUntil", func(t *testing.T) {
		user := &models.User{LockedUntil: nil}
		assert.Equal(t, time.Duration(0), svc.GetLockTimeRemaining(user))
	})

	t.Run("future lock", func(t *testing.T) {
		future := time.Now().Add(10 * time.Minute)
		user := &models.User{LockedUntil: &future}
		remaining := svc.GetLockTimeRemaining(user)
		assert.True(t, remaining > 9*time.Minute)
		assert.True(t, remaining <= 10*time.Minute)
	})

	t.Run("expired lock", func(t *testing.T) {
		past := time.Now().Add(-10 * time.Minute)
		user := &models.User{LockedUntil: &past}
		assert.Equal(t, time.Duration(0), svc.GetLockTimeRemaining(user))
	})
}

// =============================================================================
// ACCOUNT RECOVERY PURE LOGIC TESTS
// =============================================================================

func TestIsAccountRecoverable_Nil(t *testing.T) {
	svc := &UserService{}
	assert.False(t, svc.IsAccountRecoverable(nil))
}

func TestIsAccountRecoverable_Active(t *testing.T) {
	svc := &UserService{}
	user := &models.User{IsActive: true}
	assert.False(t, svc.IsAccountRecoverable(user))
}

func TestIsAccountRecoverable_WithinGrace(t *testing.T) {
	svc := &UserService{}
	deletedAt := time.Now().Add(-5 * 24 * time.Hour) // 5 days ago
	user := &models.User{IsActive: false, DeletedAt: &deletedAt}
	assert.True(t, svc.IsAccountRecoverable(user))
}

func TestIsAccountRecoverable_Expired(t *testing.T) {
	svc := &UserService{}
	deletedAt := time.Now().Add(-31 * 24 * time.Hour) // 31 days ago
	user := &models.User{IsActive: false, DeletedAt: &deletedAt}
	assert.False(t, svc.IsAccountRecoverable(user))
}

func TestGetDaysUntilPermanentDeletion(t *testing.T) {
	svc := &UserService{}

	t.Run("nil user", func(t *testing.T) {
		assert.Equal(t, 0, svc.GetDaysUntilPermanentDeletion(nil))
	})

	t.Run("active user", func(t *testing.T) {
		user := &models.User{IsActive: true}
		assert.Equal(t, 0, svc.GetDaysUntilPermanentDeletion(user))
	})

	t.Run("deleted 5 days ago", func(t *testing.T) {
		deletedAt := time.Now().Add(-5 * 24 * time.Hour)
		user := &models.User{IsActive: false, DeletedAt: &deletedAt}
		days := svc.GetDaysUntilPermanentDeletion(user)
		// 30 - 5 = 25 days remaining (+ rounding up = 25 or 26)
		assert.True(t, days >= 24 && days <= 26, "expected ~25 days, got %d", days)
	})

	t.Run("deleted 31 days ago", func(t *testing.T) {
		deletedAt := time.Now().Add(-31 * 24 * time.Hour)
		user := &models.User{IsActive: false, DeletedAt: &deletedAt}
		assert.Equal(t, 0, svc.GetDaysUntilPermanentDeletion(user))
	})

	t.Run("nil DeletedAt", func(t *testing.T) {
		user := &models.User{IsActive: false, DeletedAt: nil}
		assert.Equal(t, 0, svc.GetDaysUntilPermanentDeletion(user))
	})
}

// =============================================================================
// INTEGRATION TESTS (With Real Database)
// =============================================================================

// UserServiceIntegrationTestSuite tests UserService with real PostgreSQL database
type UserServiceIntegrationTestSuite struct {
	suite.Suite
	container   testcontainers.Container
	db          *gorm.DB
	userService *UserService
	ctx         context.Context
}

func (suite *UserServiceIntegrationTestSuite) SetupSuite() {
	suite.ctx = context.Background()

	// Start PostgreSQL container
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

	// Get container host and port
	host, err := container.Host(suite.ctx)
	if err != nil {
		suite.T().Fatalf("failed to get host: %v", err)
	}

	port, err := container.MappedPort(suite.ctx, "5432")
	if err != nil {
		suite.T().Fatalf("failed to get port: %v", err)
	}

	// Connect to database
	dsn := fmt.Sprintf("host=%s port=%s user=test_user password=test_password dbname=test_db sslmode=disable",
		host, port.Port())

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		suite.T().Fatalf("failed to connect to test database: %v", err)
	}
	suite.db = db

	// Run migrations
	sqlDB, err := db.DB()
	if err != nil {
		suite.T().Fatalf("failed to get sql.DB: %v", err)
	}

	migrations := []string{
		"000001_create_initial_schema.up.sql",
		"000005_add_show_status.up.sql",
		"000006_add_user_saved_shows.up.sql",
		"000011_add_webauthn_tables.up.sql",
		"000012_add_user_deletion_fields.up.sql",
		"000014_add_account_lockout.up.sql",
		"000015_add_user_favorite_venues.up.sql",
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

	// Create UserService
	suite.userService = &UserService{db: db}
}

func (suite *UserServiceIntegrationTestSuite) TearDownSuite() {
	if suite.container != nil {
		if err := suite.container.Terminate(suite.ctx); err != nil {
			suite.T().Logf("failed to terminate container: %v", err)
		}
	}
}

// ---- Existing tests (GetUserByID, GetUserByEmail, UpdateUser, etc.) --------

// TestGetUserByID_Success tests successful user retrieval by ID
func (suite *UserServiceIntegrationTestSuite) TestGetUserByID_Success() {
	user := &models.User{
		Email:         stringPtr("test@example.com"),
		FirstName:     stringPtr("Test"),
		LastName:      stringPtr("User"),
		AvatarURL:     stringPtr("https://example.com/avatar.jpg"),
		IsActive:      true,
		EmailVerified: true,
	}

	err := suite.db.Create(user).Error
	assert.NoError(suite.T(), err)
	assert.NotZero(suite.T(), user.ID)

	retrievedUser, err := suite.userService.GetUserByID(user.ID)

	assert.NoError(suite.T(), err)
	assert.NotNil(suite.T(), retrievedUser)
	assert.Equal(suite.T(), user.ID, retrievedUser.ID)
	assert.Equal(suite.T(), *user.Email, *retrievedUser.Email)
	assert.Equal(suite.T(), *user.FirstName, *retrievedUser.FirstName)
	assert.Equal(suite.T(), *user.LastName, *retrievedUser.LastName)
}

func (suite *UserServiceIntegrationTestSuite) TestGetUserByEmail_Success() {
	user := &models.User{
		Email:         stringPtr("email@example.com"),
		FirstName:     stringPtr("Email"),
		LastName:      stringPtr("User"),
		AvatarURL:     stringPtr("https://example.com/avatar.jpg"),
		IsActive:      true,
		EmailVerified: true,
	}

	err := suite.db.Create(user).Error
	assert.NoError(suite.T(), err)
	assert.NotZero(suite.T(), user.ID)

	retrievedUser, err := suite.userService.GetUserByEmail(*user.Email)

	assert.NoError(suite.T(), err)
	assert.NotNil(suite.T(), retrievedUser)
	assert.Equal(suite.T(), user.ID, retrievedUser.ID)
	assert.Equal(suite.T(), *user.Email, *retrievedUser.Email)
}

func (suite *UserServiceIntegrationTestSuite) TestGetUserByEmail_NotFound() {
	retrievedUser, err := suite.userService.GetUserByEmail("nonexistent@example.com")

	assert.NoError(suite.T(), err)
	assert.Nil(suite.T(), retrievedUser)
}

func (suite *UserServiceIntegrationTestSuite) TestUpdateUser_Success() {
	user := &models.User{
		Email:         stringPtr("update@example.com"),
		FirstName:     stringPtr("Original"),
		LastName:      stringPtr("Name"),
		AvatarURL:     stringPtr("https://example.com/avatar.jpg"),
		IsActive:      true,
		EmailVerified: true,
	}

	err := suite.db.Create(user).Error
	assert.NoError(suite.T(), err)
	assert.NotZero(suite.T(), user.ID)

	updates := map[string]any{
		"first_name": "Updated",
		"last_name":  "Name",
	}

	updatedUser, err := suite.userService.UpdateUser(user.ID, updates)

	assert.NoError(suite.T(), err)
	assert.NotNil(suite.T(), updatedUser)
	assert.Equal(suite.T(), user.ID, updatedUser.ID)
	assert.Equal(suite.T(), "Updated", *updatedUser.FirstName)
	assert.Equal(suite.T(), "Name", *updatedUser.LastName)
}

func (suite *UserServiceIntegrationTestSuite) TestFindOrCreateUser_NewUser() {
	gothUser := goth.User{
		UserID:    "new_user_123",
		Email:     "newuser@example.com",
		FirstName: "New",
		LastName:  "User",
		Name:      "New User",
		AvatarURL: "https://example.com/avatar.jpg",
		ExpiresAt: time.Now().Add(24 * time.Hour),
	}

	user, err := suite.userService.FindOrCreateUser(gothUser, "google")

	assert.NoError(suite.T(), err)
	assert.NotNil(suite.T(), user)
	assert.NotZero(suite.T(), user.ID)
	assert.Equal(suite.T(), "newuser@example.com", *user.Email)
	assert.Equal(suite.T(), "New", *user.FirstName)
	assert.Equal(suite.T(), "User", *user.LastName)
	assert.True(suite.T(), user.IsActive)
	assert.True(suite.T(), user.EmailVerified)

	var oauthCount int64
	err = suite.db.Model(&models.OAuthAccount{}).Where("user_id = ? AND provider = ?", user.ID, "google").Count(&oauthCount).Error
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), int64(1), oauthCount)

	var prefCount int64
	err = suite.db.Model(&models.UserPreferences{}).Where("user_id = ?", user.ID).Count(&prefCount).Error
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), int64(1), prefCount)
}

func (suite *UserServiceIntegrationTestSuite) TestFindOrCreateUser_ExistingOAuthAccount() {
	user := &models.User{
		Email:         stringPtr("existing@example.com"),
		FirstName:     stringPtr("Existing"),
		LastName:      stringPtr("User"),
		AvatarURL:     stringPtr("https://example.com/avatar.jpg"),
		IsActive:      true,
		EmailVerified: true,
	}

	err := suite.db.Create(user).Error
	assert.NoError(suite.T(), err)

	oauthAccount := &models.OAuthAccount{
		UserID:         user.ID,
		Provider:       "github",
		ProviderUserID: "github_123",
		ProviderEmail:  stringPtr("existing@example.com"),
		ProviderName:   stringPtr("Existing User"),
	}

	err = suite.db.Create(oauthAccount).Error
	assert.NoError(suite.T(), err)

	gothUser := goth.User{
		UserID:    "github_123",
		Email:     "existing@example.com",
		FirstName: "Existing",
		LastName:  "User",
		Name:      "Existing User",
		AvatarURL: "https://example.com/avatar.jpg",
	}

	retrievedUser, err := suite.userService.FindOrCreateUser(gothUser, "github")

	assert.NoError(suite.T(), err)
	assert.NotNil(suite.T(), retrievedUser)
	assert.Equal(suite.T(), user.ID, retrievedUser.ID)
	assert.Equal(suite.T(), *user.Email, *retrievedUser.Email)
}

// Run the integration test suite
func TestUserServiceIntegrationTestSuite(t *testing.T) {
	suite.Run(t, new(UserServiceIntegrationTestSuite))
}

func (suite *UserServiceIntegrationTestSuite) TestLinkOAuthAccount_NewAccount() {
	user := &models.User{
		Email: stringPtr("linktest@example.com"),
	}

	err := suite.db.Create(user).Error
	suite.Require().NoError(err)
	suite.Require().NotZero(user.ID)

	gothUser := goth.User{
		UserID:       "google_new_account_12345",
		Email:        "linktest@example.com",
		Name:         "Test User",
		AvatarURL:    "https://example.com/avatar.jpg",
		AccessToken:  "access_token_123",
		RefreshToken: "refresh_token_123",
		ExpiresAt:    time.Now().Add(24 * time.Hour),
	}

	linkedUser, err := suite.userService.linkOAuthAccount(user, gothUser, "google")

	suite.Require().NoError(err)
	suite.Require().NotNil(linkedUser)
	suite.Require().Len(linkedUser.OAuthAccounts, 1)

	oauthAccount := linkedUser.OAuthAccounts[0]
	suite.Equal("google", oauthAccount.Provider)
	suite.Equal("google_new_account_12345", oauthAccount.ProviderUserID)
	suite.Equal("linktest@example.com", *oauthAccount.ProviderEmail)
	suite.Equal("Test User", *oauthAccount.ProviderName)
	suite.Equal("https://example.com/avatar.jpg", *oauthAccount.ProviderAvatarURL)
	suite.Equal("access_token_123", *oauthAccount.AccessToken)
	suite.Equal("refresh_token_123", *oauthAccount.RefreshToken)
	suite.NotNil(oauthAccount.ExpiresAt)
	suite.NotZero(oauthAccount.CreatedAt)
	suite.NotZero(oauthAccount.UpdatedAt)
}

func (suite *UserServiceIntegrationTestSuite) TestLinkOAuthAccount_UpdateExisting() {
	user := &models.User{
		Email: stringPtr("updatetest@example.com"),
	}

	err := suite.db.Create(user).Error
	suite.Require().NoError(err)
	suite.Require().NotZero(user.ID)

	existingOAuth := &models.OAuthAccount{
		UserID:         user.ID,
		Provider:       "google",
		ProviderUserID: "google_update_existing_12345",
		ProviderEmail:  stringPtr("old@example.com"),
		ProviderName:   stringPtr("Old Name"),
		AccessToken:    stringPtr("old_access_token"),
		RefreshToken:   stringPtr("old_refresh_token"),
	}

	err = suite.db.Create(existingOAuth).Error
	suite.Require().NoError(err)

	gothUser := goth.User{
		UserID:       "google_update_existing_12345",
		Email:        "new@example.com",
		Name:         "New Name",
		AvatarURL:    "https://example.com/new-avatar.jpg",
		AccessToken:  "new_access_token",
		RefreshToken: "new_refresh_token",
		ExpiresAt:    time.Now().Add(48 * time.Hour),
	}

	linkedUser, err := suite.userService.linkOAuthAccount(user, gothUser, "google")

	suite.Require().NoError(err)
	suite.Require().NotNil(linkedUser)
	suite.Require().Len(linkedUser.OAuthAccounts, 1)

	oauthAccount := linkedUser.OAuthAccounts[0]
	suite.Equal("google", oauthAccount.Provider)
	suite.Equal("google_update_existing_12345", oauthAccount.ProviderUserID)
	suite.Equal("new@example.com", *oauthAccount.ProviderEmail)
	suite.Equal("New Name", *oauthAccount.ProviderName)
	suite.Equal("https://example.com/new-avatar.jpg", *oauthAccount.ProviderAvatarURL)
	suite.Equal("new_access_token", *oauthAccount.AccessToken)
	suite.Equal("new_refresh_token", *oauthAccount.RefreshToken)
	suite.NotNil(oauthAccount.ExpiresAt)

	suite.Equal(existingOAuth.ID, oauthAccount.ID)
}

func (suite *UserServiceIntegrationTestSuite) TestLinkOAuthAccount_WithoutExpiresAt() {
	user := &models.User{
		Email: stringPtr("noexpiry@example.com"),
	}

	err := suite.db.Create(user).Error
	suite.Require().NoError(err)
	suite.Require().NotZero(user.ID)

	gothUser := goth.User{
		UserID:       "github_no_expiry_67890",
		Email:        "noexpiry@example.com",
		Name:         "No Expiry User",
		AvatarURL:    "https://github.com/avatar.jpg",
		AccessToken:  "github_access_token",
		RefreshToken: "github_refresh_token",
	}

	linkedUser, err := suite.userService.linkOAuthAccount(user, gothUser, "github")

	suite.Require().NoError(err)
	suite.Require().NotNil(linkedUser)
	suite.Require().Len(linkedUser.OAuthAccounts, 1)

	oauthAccount := linkedUser.OAuthAccounts[0]
	suite.Equal("github", oauthAccount.Provider)
	suite.Equal("github_no_expiry_67890", oauthAccount.ProviderUserID)
	suite.Equal("noexpiry@example.com", *oauthAccount.ProviderEmail)
	suite.Equal("No Expiry User", *oauthAccount.ProviderName)
	suite.Equal("https://github.com/avatar.jpg", *oauthAccount.ProviderAvatarURL)
	suite.Equal("github_access_token", *oauthAccount.AccessToken)
	suite.Equal("github_refresh_token", *oauthAccount.RefreshToken)
	suite.Nil(oauthAccount.ExpiresAt)
}

func (suite *UserServiceIntegrationTestSuite) TestLinkOAuthAccount_MultipleProviders() {
	user := &models.User{
		Email: stringPtr("multiprovider@example.com"),
	}

	err := suite.db.Create(user).Error
	suite.Require().NoError(err)
	suite.Require().NotZero(user.ID)

	googleUser := goth.User{
		UserID:       "google_multi_12345",
		Email:        "multiprovider@example.com",
		Name:         "Google User",
		AvatarURL:    "https://google.com/avatar.jpg",
		AccessToken:  "google_access_token",
		RefreshToken: "google_refresh_token",
	}

	linkedUser, err := suite.userService.linkOAuthAccount(user, googleUser, "google")
	suite.Require().NoError(err)
	suite.Require().NotNil(linkedUser)

	githubUser := goth.User{
		UserID:       "github_multi_67890",
		Email:        "multiprovider@example.com",
		Name:         "GitHub User",
		AvatarURL:    "https://github.com/avatar.jpg",
		AccessToken:  "github_access_token",
		RefreshToken: "github_refresh_token",
	}

	linkedUser, err = suite.userService.linkOAuthAccount(user, githubUser, "github")
	suite.Require().NoError(err)
	suite.Require().NotNil(linkedUser)
	suite.Require().Len(linkedUser.OAuthAccounts, 2)

	providers := make(map[string]bool)
	for _, oauth := range linkedUser.OAuthAccounts {
		providers[oauth.Provider] = true
	}
	suite.True(providers["google"])
	suite.True(providers["github"])
}

func (suite *UserServiceIntegrationTestSuite) TestLinkOAuthAccount_EmptyFields() {
	user := &models.User{
		Email: stringPtr("emptyfields@example.com"),
	}

	err := suite.db.Create(user).Error
	suite.Require().NoError(err)
	suite.Require().NotZero(user.ID)

	gothUser := goth.User{
		UserID:       "empty_fields_12345",
		Email:        "",
		Name:         "",
		AvatarURL:    "",
		AccessToken:  "access_token",
		RefreshToken: "refresh_token",
	}

	linkedUser, err := suite.userService.linkOAuthAccount(user, gothUser, "empty")

	suite.Require().NoError(err)
	suite.Require().NotNil(linkedUser)
	suite.Require().Len(linkedUser.OAuthAccounts, 1)

	oauthAccount := linkedUser.OAuthAccounts[0]
	suite.Equal("empty", oauthAccount.Provider)
	suite.Equal("empty_fields_12345", oauthAccount.ProviderUserID)
	suite.Equal("", *oauthAccount.ProviderEmail)
	suite.Equal("", *oauthAccount.ProviderName)
	suite.Equal("", *oauthAccount.ProviderAvatarURL)
	suite.Equal("access_token", *oauthAccount.AccessToken)
	suite.Equal("refresh_token", *oauthAccount.RefreshToken)
}

func (suite *UserServiceIntegrationTestSuite) TestGetUserByUsername_Success() {
	username := "testuser123"
	user := &models.User{
		Email:    stringPtr("username@example.com"),
		Username: &username,
	}

	err := suite.db.Create(user).Error
	suite.Require().NoError(err)
	suite.Require().NotZero(user.ID)

	retrievedUser, err := suite.userService.GetUserByUsername(username)

	suite.Require().NoError(err)
	suite.Require().NotNil(retrievedUser)
	suite.Equal(user.ID, retrievedUser.ID)
	suite.Equal(*user.Email, *retrievedUser.Email)
	suite.Equal(*user.Username, *retrievedUser.Username)
	suite.NotZero(retrievedUser.CreatedAt)
	suite.NotZero(retrievedUser.UpdatedAt)
}

func (suite *UserServiceIntegrationTestSuite) TestGetUserByUsername_NotFound() {
	retrievedUser, err := suite.userService.GetUserByUsername("nonexistent_username")

	suite.Require().NoError(err)
	suite.Require().Nil(retrievedUser)
}

func (suite *UserServiceIntegrationTestSuite) TestGetUserByUsername_WithOAuthAccounts() {
	username := "oauthuser456"
	user := &models.User{
		Email:    stringPtr("oauthuser@example.com"),
		Username: &username,
	}

	err := suite.db.Create(user).Error
	suite.Require().NoError(err)
	suite.Require().NotZero(user.ID)

	oauthAccount := &models.OAuthAccount{
		UserID:         user.ID,
		Provider:       "google",
		ProviderUserID: "google_oauth_test_123",
		ProviderEmail:  stringPtr("oauthuser@example.com"),
		ProviderName:   stringPtr("OAuth Test User"),
		AccessToken:    stringPtr("test_access_token"),
		RefreshToken:   stringPtr("test_refresh_token"),
	}

	err = suite.db.Create(oauthAccount).Error
	suite.Require().NoError(err)

	retrievedUser, err := suite.userService.GetUserByUsername(username)

	suite.Require().NoError(err)
	suite.Require().NotNil(retrievedUser)
	suite.Equal(user.ID, retrievedUser.ID)
	suite.Equal(*user.Email, *retrievedUser.Email)
	suite.Equal(*user.Username, *retrievedUser.Username)
	suite.Require().Len(retrievedUser.OAuthAccounts, 1)

	oauth := retrievedUser.OAuthAccounts[0]
	suite.Equal("google", oauth.Provider)
	suite.Equal("google_oauth_test_123", oauth.ProviderUserID)
	suite.Equal("oauthuser@example.com", *oauth.ProviderEmail)
	suite.Equal("OAuth Test User", *oauth.ProviderName)
}

func (suite *UserServiceIntegrationTestSuite) TestGetUserByUsername_WithPreferences() {
	username := "prefuser789"
	user := &models.User{
		Email:    stringPtr("prefuser@example.com"),
		Username: &username,
	}

	err := suite.db.Create(user).Error
	suite.Require().NoError(err)
	suite.Require().NotZero(user.ID)

	preferences := &models.UserPreferences{
		UserID:            user.ID,
		NotificationEmail: true,
		NotificationPush:  false,
		Theme:             "dark",
		Timezone:          "America/New_York",
		Language:          "en",
	}

	err = suite.db.Create(preferences).Error
	suite.Require().NoError(err)

	retrievedUser, err := suite.userService.GetUserByUsername(username)

	suite.Require().NoError(err)
	suite.Require().NotNil(retrievedUser)
	suite.Equal(user.ID, retrievedUser.ID)
	suite.Equal(*user.Email, *retrievedUser.Email)
	suite.Equal(*user.Username, *retrievedUser.Username)
	suite.Require().NotNil(retrievedUser.Preferences)

	prefs := retrievedUser.Preferences
	suite.Equal(user.ID, prefs.UserID)
	suite.True(prefs.NotificationEmail)
	suite.False(prefs.NotificationPush)
	suite.Equal("dark", prefs.Theme)
	suite.Equal("America/New_York", prefs.Timezone)
	suite.Equal("en", prefs.Language)
}

func (suite *UserServiceIntegrationTestSuite) TestGetUserByUsername_WithOAuthAndPreferences() {
	username := "fulluser999"
	user := &models.User{
		Email:    stringPtr("fulluser@example.com"),
		Username: &username,
	}

	err := suite.db.Create(user).Error
	suite.Require().NoError(err)
	suite.Require().NotZero(user.ID)

	oauthAccount := &models.OAuthAccount{
		UserID:         user.ID,
		Provider:       "github",
		ProviderUserID: "github_full_test_456",
		ProviderEmail:  stringPtr("fulluser@example.com"),
		ProviderName:   stringPtr("Full Test User"),
		AccessToken:    stringPtr("github_access_token"),
		RefreshToken:   stringPtr("github_refresh_token"),
	}

	err = suite.db.Create(oauthAccount).Error
	suite.Require().NoError(err)

	err = suite.db.Exec(`
		INSERT INTO user_preferences (user_id, notification_email, notification_push, theme, timezone, language, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, NOW(), NOW())
	`, user.ID, false, true, "light", "UTC", "es").Error
	suite.Require().NoError(err)

	retrievedUser, err := suite.userService.GetUserByUsername(username)

	suite.Require().NoError(err)
	suite.Require().NotNil(retrievedUser)
	suite.Equal(user.ID, retrievedUser.ID)
	suite.Equal(*user.Email, *retrievedUser.Email)
	suite.Equal(*user.Username, *retrievedUser.Username)
	suite.Require().Len(retrievedUser.OAuthAccounts, 1)
	suite.Require().NotNil(retrievedUser.Preferences)

	oauth := retrievedUser.OAuthAccounts[0]
	suite.Equal("github", oauth.Provider)
	suite.Equal("github_full_test_456", oauth.ProviderUserID)

	prefs := retrievedUser.Preferences
	suite.Equal(user.ID, prefs.UserID)
	suite.False(prefs.NotificationEmail)
	suite.True(prefs.NotificationPush)
	suite.Equal("light", prefs.Theme)
	suite.Equal("UTC", prefs.Timezone)
	suite.Equal("es", prefs.Language)
}

func (suite *UserServiceIntegrationTestSuite) TestGetUserByUsername_EmptyUsername() {
	retrievedUser, err := suite.userService.GetUserByUsername("")

	suite.Require().NoError(err)
	suite.Require().Nil(retrievedUser)
}

func (suite *UserServiceIntegrationTestSuite) TestGetUserByUsername_SpecialCharacters() {
	username := "user-name_with.underscores+plus"
	user := &models.User{
		Email:    stringPtr("special@example.com"),
		Username: &username,
	}

	err := suite.db.Create(user).Error
	suite.Require().NoError(err)
	suite.Require().NotZero(user.ID)

	retrievedUser, err := suite.userService.GetUserByUsername(username)

	suite.Require().NoError(err)
	suite.Require().NotNil(retrievedUser)
	suite.Equal(user.ID, retrievedUser.ID)
	suite.Equal(*user.Email, *retrievedUser.Email)
	suite.Equal(*user.Username, *retrievedUser.Username)
}

func (suite *UserServiceIntegrationTestSuite) TestGetUserByUsername_VeryLongUsername() {
	username := "very_long_username_that_is_quite_lengthy_and_might_test_boundaries_123456789"
	user := &models.User{
		Email:    stringPtr("longuser@example.com"),
		Username: &username,
	}

	err := suite.db.Create(user).Error
	suite.Require().NoError(err)
	suite.Require().NotZero(user.ID)

	retrievedUser, err := suite.userService.GetUserByUsername(username)

	suite.Require().NoError(err)
	suite.Require().NotNil(retrievedUser)
	suite.Equal(user.ID, retrievedUser.ID)
	suite.Equal(*user.Email, *retrievedUser.Email)
	suite.Equal(*user.Username, *retrievedUser.Username)
}

// =============================================================================
// NEW INTEGRATION TESTS: CreateUserWithPassword
// =============================================================================

func (suite *UserServiceIntegrationTestSuite) TestCreateUserWithPassword_Success() {
	user, err := suite.userService.CreateUserWithPassword(
		"pwuser@example.com", "StrongPass123!", "Alice", "Smith",
	)

	suite.Require().NoError(err)
	suite.Require().NotNil(user)
	suite.NotZero(user.ID)
	suite.Equal("pwuser@example.com", *user.Email)
	suite.Equal("Alice", *user.FirstName)
	suite.Equal("Smith", *user.LastName)
	suite.True(user.IsActive)
	suite.False(user.EmailVerified) // password users need verification
	suite.NotNil(user.PasswordHash)
	suite.NotEqual("StrongPass123!", *user.PasswordHash) // hashed, not plaintext
	suite.NotNil(user.Preferences)
}

func (suite *UserServiceIntegrationTestSuite) TestCreateUserWithPassword_DuplicateEmail() {
	_, err := suite.userService.CreateUserWithPassword(
		"dupepw@example.com", "Pass123!", "First", "Last",
	)
	suite.Require().NoError(err)

	// Try again with same email
	user, err := suite.userService.CreateUserWithPassword(
		"dupepw@example.com", "Pass456!", "Other", "Name",
	)
	suite.Require().Error(err)
	suite.Nil(user)

	// Verify it's the right error type
	authErr, ok := err.(*apperrors.AuthError)
	suite.Require().True(ok)
	suite.Equal(apperrors.CodeUserExists, authErr.Code)
}

func (suite *UserServiceIntegrationTestSuite) TestCreateUserWithPassword_PreferencesCreated() {
	user, err := suite.userService.CreateUserWithPassword(
		"prefcheck@example.com", "Pass123!", "Pref", "Check",
	)
	suite.Require().NoError(err)

	var prefCount int64
	suite.db.Model(&models.UserPreferences{}).Where("user_id = ?", user.ID).Count(&prefCount)
	suite.Equal(int64(1), prefCount)
}

func (suite *UserServiceIntegrationTestSuite) TestCreateUserWithPassword_PasswordHashed() {
	user, err := suite.userService.CreateUserWithPassword(
		"hashcheck@example.com", "MyPassword99!", "Hash", "Check",
	)
	suite.Require().NoError(err)
	suite.Require().NotNil(user.PasswordHash)

	// Verify bcrypt hash
	suite.Contains(*user.PasswordHash, "$2a$")

	// Verify the password can be verified
	err = suite.userService.VerifyPassword(*user.PasswordHash, "MyPassword99!")
	suite.NoError(err)
}

// =============================================================================
// NEW INTEGRATION TESTS: AuthenticateUserWithPassword
// =============================================================================

func (suite *UserServiceIntegrationTestSuite) TestAuthenticate_Success() {
	_, err := suite.userService.CreateUserWithPassword(
		"authsuccess@example.com", "CorrectPassword1!", "Auth", "Success",
	)
	suite.Require().NoError(err)

	// Must set email_verified and is_active for auth to succeed
	suite.db.Model(&models.User{}).Where("email = ?", "authsuccess@example.com").
		Update("email_verified", true)

	user, err := suite.userService.AuthenticateUserWithPassword(
		"authsuccess@example.com", "CorrectPassword1!",
	)

	suite.Require().NoError(err)
	suite.Require().NotNil(user)
	suite.Equal("authsuccess@example.com", *user.Email)
}

func (suite *UserServiceIntegrationTestSuite) TestAuthenticate_WrongPassword() {
	_, err := suite.userService.CreateUserWithPassword(
		"authwrong@example.com", "CorrectPassword1!", "Auth", "Wrong",
	)
	suite.Require().NoError(err)

	user, err := suite.userService.AuthenticateUserWithPassword(
		"authwrong@example.com", "WrongPassword!",
	)

	suite.Require().Error(err)
	suite.Nil(user)
	authErr, ok := err.(*apperrors.AuthError)
	suite.Require().True(ok)
	suite.Equal(apperrors.CodeInvalidCredentials, authErr.Code)

	// Verify failed attempts were incremented
	var dbUser models.User
	suite.db.Where("email = ?", "authwrong@example.com").First(&dbUser)
	suite.Equal(1, dbUser.FailedLoginAttempts)
}

func (suite *UserServiceIntegrationTestSuite) TestAuthenticate_NonexistentEmail() {
	user, err := suite.userService.AuthenticateUserWithPassword(
		"doesnotexist@example.com", "SomePassword!",
	)

	suite.Require().Error(err)
	suite.Nil(user)
	authErr, ok := err.(*apperrors.AuthError)
	suite.Require().True(ok)
	suite.Equal(apperrors.CodeInvalidCredentials, authErr.Code)
}

func (suite *UserServiceIntegrationTestSuite) TestAuthenticate_OAuthOnlyUser() {
	// Create OAuth-only user (no password)
	_, err := suite.userService.CreateUserWithoutPassword("oauthonly-auth@example.com")
	suite.Require().NoError(err)

	user, err := suite.userService.AuthenticateUserWithPassword(
		"oauthonly-auth@example.com", "SomePassword!",
	)

	suite.Require().Error(err)
	suite.Nil(user)
	authErr, ok := err.(*apperrors.AuthError)
	suite.Require().True(ok)
	suite.Equal(apperrors.CodeInvalidCredentials, authErr.Code)
}

func (suite *UserServiceIntegrationTestSuite) TestAuthenticate_LockedAccount() {
	_, err := suite.userService.CreateUserWithPassword(
		"authlocked@example.com", "CorrectPassword1!", "Locked", "User",
	)
	suite.Require().NoError(err)

	// Lock the account
	lockUntil := time.Now().Add(15 * time.Minute)
	suite.db.Model(&models.User{}).Where("email = ?", "authlocked@example.com").
		Updates(map[string]interface{}{
			"locked_until":          lockUntil,
			"failed_login_attempts": 5,
		})

	user, err := suite.userService.AuthenticateUserWithPassword(
		"authlocked@example.com", "CorrectPassword1!",
	)

	suite.Require().Error(err)
	suite.Nil(user)
	authErr, ok := err.(*apperrors.AuthError)
	suite.Require().True(ok)
	suite.Equal(apperrors.CodeAccountLocked, authErr.Code)
}

func (suite *UserServiceIntegrationTestSuite) TestAuthenticate_InactiveUser() {
	_, err := suite.userService.CreateUserWithPassword(
		"authinactive@example.com", "CorrectPassword1!", "Inactive", "User",
	)
	suite.Require().NoError(err)

	// Soft-delete (is_active = false)
	suite.db.Model(&models.User{}).Where("email = ?", "authinactive@example.com").
		Update("is_active", false)

	user, err := suite.userService.AuthenticateUserWithPassword(
		"authinactive@example.com", "CorrectPassword1!",
	)

	suite.Require().Error(err)
	suite.Nil(user)
	suite.Contains(err.Error(), "user account is not active")
}

// =============================================================================
// NEW INTEGRATION TESTS: Account Lockout
// =============================================================================

func (suite *UserServiceIntegrationTestSuite) TestIncrementFailedAttempts_BelowThreshold() {
	user := &models.User{
		Email:    stringPtr("lockout1@example.com"),
		IsActive: true,
	}
	suite.db.Create(user)

	err := suite.userService.IncrementFailedAttempts(user.ID)
	suite.Require().NoError(err)

	var dbUser models.User
	suite.db.First(&dbUser, user.ID)
	suite.Equal(1, dbUser.FailedLoginAttempts)
	suite.Nil(dbUser.LockedUntil)
}

func (suite *UserServiceIntegrationTestSuite) TestIncrementFailedAttempts_AtThreshold() {
	user := &models.User{
		Email:               stringPtr("lockout5@example.com"),
		IsActive:            true,
		FailedLoginAttempts: 4, // One more will hit threshold of 5
	}
	suite.db.Create(user)

	err := suite.userService.IncrementFailedAttempts(user.ID)
	suite.Require().NoError(err)

	var dbUser models.User
	suite.db.First(&dbUser, user.ID)
	suite.Equal(5, dbUser.FailedLoginAttempts)
	suite.NotNil(dbUser.LockedUntil)
	// Lock should be ~15 minutes in the future
	suite.True(dbUser.LockedUntil.After(time.Now().Add(14 * time.Minute)))
	suite.True(dbUser.LockedUntil.Before(time.Now().Add(16 * time.Minute)))
}

func (suite *UserServiceIntegrationTestSuite) TestResetFailedAttempts_Success() {
	lockUntil := time.Now().Add(15 * time.Minute)
	user := &models.User{
		Email:               stringPtr("resetattempts@example.com"),
		IsActive:            true,
		FailedLoginAttempts: 3,
		LockedUntil:         &lockUntil,
	}
	suite.db.Create(user)

	err := suite.userService.ResetFailedAttempts(user.ID)
	suite.Require().NoError(err)

	var dbUser models.User
	suite.db.First(&dbUser, user.ID)
	suite.Equal(0, dbUser.FailedLoginAttempts)
	suite.Nil(dbUser.LockedUntil)
}

// =============================================================================
// NEW INTEGRATION TESTS: Password & Email Management
// =============================================================================

func (suite *UserServiceIntegrationTestSuite) TestUpdatePassword_Success() {
	user, err := suite.userService.CreateUserWithPassword(
		"updatepw@example.com", "OldPassword1!", "Update", "PW",
	)
	suite.Require().NoError(err)

	err = suite.userService.UpdatePassword(user.ID, "OldPassword1!", "NewPassword2!")
	suite.Require().NoError(err)

	// Verify old password no longer works
	var dbUser models.User
	suite.db.First(&dbUser, user.ID)
	suite.Error(suite.userService.VerifyPassword(*dbUser.PasswordHash, "OldPassword1!"))

	// Verify new password works
	suite.NoError(suite.userService.VerifyPassword(*dbUser.PasswordHash, "NewPassword2!"))
}

func (suite *UserServiceIntegrationTestSuite) TestUpdatePassword_WrongCurrent() {
	user, err := suite.userService.CreateUserWithPassword(
		"updatepwwrong@example.com", "CorrectOld1!", "Wrong", "Current",
	)
	suite.Require().NoError(err)

	err = suite.userService.UpdatePassword(user.ID, "WrongOld!", "NewPassword2!")
	suite.Require().Error(err)

	authErr, ok := err.(*apperrors.AuthError)
	suite.Require().True(ok)
	suite.Equal(apperrors.CodeInvalidCredentials, authErr.Code)
}

func (suite *UserServiceIntegrationTestSuite) TestSetEmailVerified_Success() {
	user := &models.User{
		Email:         stringPtr("verifyemail@example.com"),
		IsActive:      true,
		EmailVerified: false,
	}
	suite.db.Create(user)

	err := suite.userService.SetEmailVerified(user.ID, true)
	suite.Require().NoError(err)

	var dbUser models.User
	suite.db.First(&dbUser, user.ID)
	suite.True(dbUser.EmailVerified)
}

// =============================================================================
// NEW INTEGRATION TESTS: Account Deletion & Recovery
// =============================================================================

func (suite *UserServiceIntegrationTestSuite) TestSoftDeleteAccount_Success() {
	user := &models.User{
		Email:    stringPtr("softdelete@example.com"),
		IsActive: true,
	}
	suite.db.Create(user)

	err := suite.userService.SoftDeleteAccount(user.ID, nil)
	suite.Require().NoError(err)

	var dbUser models.User
	suite.db.First(&dbUser, user.ID)
	suite.False(dbUser.IsActive)
	suite.NotNil(dbUser.DeletedAt)
}

func (suite *UserServiceIntegrationTestSuite) TestSoftDeleteAccount_WithReason() {
	user := &models.User{
		Email:    stringPtr("softdeletereason@example.com"),
		IsActive: true,
	}
	suite.db.Create(user)

	reason := "No longer using the service"
	err := suite.userService.SoftDeleteAccount(user.ID, &reason)
	suite.Require().NoError(err)

	var dbUser models.User
	suite.db.First(&dbUser, user.ID)
	suite.False(dbUser.IsActive)
	suite.NotNil(dbUser.DeletedAt)
	suite.NotNil(dbUser.DeletionReason)
	suite.Equal("No longer using the service", *dbUser.DeletionReason)
}

func (suite *UserServiceIntegrationTestSuite) TestSoftDeleteAccount_NotFound() {
	err := suite.userService.SoftDeleteAccount(999999, nil)
	suite.Require().Error(err)
	suite.Contains(err.Error(), "user not found")
}

func (suite *UserServiceIntegrationTestSuite) TestRestoreAccount_Success() {
	now := time.Now()
	reason := "testing"
	user := &models.User{
		Email:          stringPtr("restoreacct@example.com"),
		IsActive:       true, // create as true first (GORM zero-value gotcha)
		DeletedAt:      &now,
		DeletionReason: &reason,
	}
	suite.db.Create(user)
	// Then set is_active to false
	suite.db.Model(&models.User{}).Where("id = ?", user.ID).Update("is_active", false)

	err := suite.userService.RestoreAccount(user.ID)
	suite.Require().NoError(err)

	var dbUser models.User
	suite.db.First(&dbUser, user.ID)
	suite.True(dbUser.IsActive)
	suite.Nil(dbUser.DeletedAt)
	suite.Nil(dbUser.DeletionReason)
}

func (suite *UserServiceIntegrationTestSuite) TestRestoreAccount_NotFound() {
	err := suite.userService.RestoreAccount(999998)
	suite.Require().Error(err)
	suite.Contains(err.Error(), "user not found")
}

func (suite *UserServiceIntegrationTestSuite) TestGetExpiredDeletedAccounts() {
	// Create an expired deleted account (31 days ago)
	expiredTime := time.Now().Add(-31 * 24 * time.Hour)
	expiredUser := &models.User{
		Email:    stringPtr("expired-del@example.com"),
		IsActive: true,
	}
	suite.db.Create(expiredUser)
	suite.db.Model(&models.User{}).Where("id = ?", expiredUser.ID).
		Updates(map[string]interface{}{
			"is_active":  false,
			"deleted_at": expiredTime,
		})

	// Create a recent deleted account (5 days ago)
	recentTime := time.Now().Add(-5 * 24 * time.Hour)
	recentUser := &models.User{
		Email:    stringPtr("recent-del@example.com"),
		IsActive: true,
	}
	suite.db.Create(recentUser)
	suite.db.Model(&models.User{}).Where("id = ?", recentUser.ID).
		Updates(map[string]interface{}{
			"is_active":  false,
			"deleted_at": recentTime,
		})

	accounts, err := suite.userService.GetExpiredDeletedAccounts()
	suite.Require().NoError(err)

	// Should contain the expired one
	foundExpired := false
	foundRecent := false
	for _, u := range accounts {
		if u.ID == expiredUser.ID {
			foundExpired = true
		}
		if u.ID == recentUser.ID {
			foundRecent = true
		}
	}
	suite.True(foundExpired, "expired account should be returned")
	suite.False(foundRecent, "recent account should NOT be returned")
}

func (suite *UserServiceIntegrationTestSuite) TestGetDeletionSummary_Success() {
	// Create user
	user := &models.User{
		Email:    stringPtr("delsummary@example.com"),
		IsActive: true,
	}
	suite.Require().NoError(suite.db.Create(user).Error)

	// Create shows submitted by this user (raw SQL to avoid GORM model column mismatches)
	var show1ID, show2ID uint
	suite.Require().NoError(suite.db.Raw(
		`INSERT INTO shows (title, event_date, submitted_by, created_at, updated_at) VALUES (?, NOW() + interval '1 day', ?, NOW(), NOW()) RETURNING id`,
		"Summary Show 1", user.ID).Scan(&show1ID).Error)
	suite.Require().NoError(suite.db.Raw(
		`INSERT INTO shows (title, event_date, submitted_by, created_at, updated_at) VALUES (?, NOW() + interval '2 days', ?, NOW(), NOW()) RETURNING id`,
		"Summary Show 2", user.ID).Scan(&show2ID).Error)

	// Create saved show
	suite.Require().NoError(suite.db.Create(&models.UserSavedShow{
		UserID:  user.ID,
		ShowID:  show1ID,
		SavedAt: time.Now(),
	}).Error)

	// Create a passkey (raw SQL to avoid GORM column name mismatch on aaguid)
	suite.Require().NoError(suite.db.Exec(
		`INSERT INTO webauthn_credentials (user_id, credential_id, public_key, display_name, created_at, updated_at)
		VALUES (?, E'\\x637265642D73756D6D6172792D31', E'\\x7075626B65792D73756D6D6172792D31', 'My Key', NOW(), NOW())`,
		user.ID).Error)

	summary, err := suite.userService.GetDeletionSummary(user.ID)
	suite.Require().NoError(err)
	suite.Require().NotNil(summary)
	suite.Equal(int64(2), summary.ShowsCount)
	suite.Equal(int64(1), summary.SavedShowsCount)
	suite.Equal(int64(1), summary.PasskeysCount)
}

func (suite *UserServiceIntegrationTestSuite) TestPermanentlyDeleteUser() {
	// Create user with related data
	user := &models.User{
		Email:    stringPtr("permdelete@example.com"),
		IsActive: true,
	}
	suite.Require().NoError(suite.db.Create(user).Error)

	// Create preferences
	suite.Require().NoError(suite.db.Create(&models.UserPreferences{UserID: user.ID}).Error)

	// Create OAuth account
	suite.Require().NoError(suite.db.Create(&models.OAuthAccount{
		UserID:         user.ID,
		Provider:       "google",
		ProviderUserID: "perm_del_google_123",
		ProviderEmail:  stringPtr("permdelete@example.com"),
	}).Error)

	// Create a show submitted by this user (raw SQL)
	var showID uint
	suite.Require().NoError(suite.db.Raw(
		`INSERT INTO shows (title, event_date, submitted_by, created_at, updated_at) VALUES (?, NOW() + interval '1 day', ?, NOW(), NOW()) RETURNING id`,
		"Perm Delete Show", user.ID).Scan(&showID).Error)

	// Create saved show
	suite.Require().NoError(suite.db.Create(&models.UserSavedShow{
		UserID:  user.ID,
		ShowID:  showID,
		SavedAt: time.Now(),
	}).Error)

	// Create favorite venue (raw SQL for venue insert)
	var venueID uint
	suite.Require().NoError(suite.db.Raw(
		`INSERT INTO venues (name, city, state, created_at, updated_at) VALUES (?, ?, ?, NOW(), NOW()) RETURNING id`,
		"Del Venue", "Nashville", "TN").Scan(&venueID).Error)
	suite.Require().NoError(suite.db.Create(&models.UserFavoriteVenue{
		UserID:      user.ID,
		VenueID:     venueID,
		FavoritedAt: time.Now(),
	}).Error)

	// Create passkey (raw SQL)
	suite.Require().NoError(suite.db.Exec(
		`INSERT INTO webauthn_credentials (user_id, credential_id, public_key, display_name, created_at, updated_at)
		VALUES (?, E'\\x637265642D7065726D64656C6574652D31', E'\\x7075626B65792D7065726D64656C6574652D31', 'Del Key', NOW(), NOW())`,
		user.ID).Error)

	err := suite.userService.PermanentlyDeleteUser(user.ID)
	suite.Require().NoError(err)

	// Verify user is gone
	var count int64
	suite.db.Model(&models.User{}).Where("id = ?", user.ID).Count(&count)
	suite.Equal(int64(0), count)

	// Verify cascaded data is gone
	suite.db.Model(&models.OAuthAccount{}).Where("user_id = ?", user.ID).Count(&count)
	suite.Equal(int64(0), count)

	suite.db.Model(&models.UserPreferences{}).Where("user_id = ?", user.ID).Count(&count)
	suite.Equal(int64(0), count)

	suite.db.Model(&models.UserSavedShow{}).Where("user_id = ?", user.ID).Count(&count)
	suite.Equal(int64(0), count)

	suite.db.Model(&models.UserFavoriteVenue{}).Where("user_id = ?", user.ID).Count(&count)
	suite.Equal(int64(0), count)

	var passkeyCount int64
	suite.db.Raw(`SELECT COUNT(*) FROM webauthn_credentials WHERE user_id = ?`, user.ID).Scan(&passkeyCount)
	suite.Equal(int64(0), passkeyCount)

	// Verify show still exists but submitted_by is nullified
	var submittedBy *uint
	suite.db.Raw(`SELECT submitted_by FROM shows WHERE id = ?`, showID).Scan(&submittedBy)
	suite.Nil(submittedBy)
}

// =============================================================================
// NEW INTEGRATION TESTS: CreateUserWithoutPassword
// =============================================================================

func (suite *UserServiceIntegrationTestSuite) TestCreateUserWithoutPassword_Success() {
	user, err := suite.userService.CreateUserWithoutPassword("nopw@example.com")

	suite.Require().NoError(err)
	suite.Require().NotNil(user)
	suite.NotZero(user.ID)
	suite.Equal("nopw@example.com", *user.Email)
	suite.Nil(user.PasswordHash)
	suite.True(user.IsActive)
	suite.False(user.EmailVerified)
	suite.NotNil(user.Preferences)
}

func (suite *UserServiceIntegrationTestSuite) TestCreateUserWithoutPassword_DuplicateEmail() {
	_, err := suite.userService.CreateUserWithoutPassword("dupenopw@example.com")
	suite.Require().NoError(err)

	user, err := suite.userService.CreateUserWithoutPassword("dupenopw@example.com")
	suite.Require().Error(err)
	suite.Nil(user)

	authErr, ok := err.(*apperrors.AuthError)
	suite.Require().True(ok)
	suite.Equal(apperrors.CodeUserExists, authErr.Code)
}

// =============================================================================
// NEW INTEGRATION TESTS: Data Export
// =============================================================================

func (suite *UserServiceIntegrationTestSuite) TestExportUserData_Success() {
	// Create user with password
	user, err := suite.userService.CreateUserWithPassword(
		"exportuser@example.com", "ExportPass1!", "Export", "User",
	)
	suite.Require().NoError(err)

	// Add OAuth account
	suite.Require().NoError(suite.db.Create(&models.OAuthAccount{
		UserID:         user.ID,
		Provider:       "google",
		ProviderUserID: "export_google_123",
		ProviderEmail:  stringPtr("exportuser@example.com"),
		ProviderName:   stringPtr("Export User"),
	}).Error)

	// Create a show submitted by this user (raw SQL)
	var showID uint
	suite.Require().NoError(suite.db.Raw(
		`INSERT INTO shows (title, event_date, submitted_by, created_at, updated_at) VALUES (?, NOW() + interval '1 day', ?, NOW(), NOW()) RETURNING id`,
		"Export Show", user.ID).Scan(&showID).Error)

	// Save the show
	suite.Require().NoError(suite.db.Create(&models.UserSavedShow{
		UserID:  user.ID,
		ShowID:  showID,
		SavedAt: time.Now(),
	}).Error)

	// Create a passkey (raw SQL to avoid GORM column name mismatch on aaguid)
	suite.Require().NoError(suite.db.Exec(
		`INSERT INTO webauthn_credentials (user_id, credential_id, public_key, display_name, created_at, updated_at)
		VALUES (?, E'\\x637265642D6578706F72742D31', E'\\x7075626B65792D6578706F72742D31', 'Export Key', NOW(), NOW())`,
		user.ID).Error)

	export, err := suite.userService.ExportUserData(user.ID)
	suite.Require().NoError(err)
	suite.Require().NotNil(export)

	// Verify profile
	suite.Equal(user.ID, export.Profile.ID)
	suite.Equal("exportuser@example.com", *export.Profile.Email)
	suite.Equal("Export", *export.Profile.FirstName)

	// Verify export version
	suite.Equal("1.0", export.ExportVersion)

	// Verify preferences
	suite.NotNil(export.Preferences)

	// Verify OAuth (without tokens)
	suite.Require().Len(export.OAuthAccounts, 1)
	suite.Equal("google", export.OAuthAccounts[0].Provider)
	suite.Equal("exportuser@example.com", *export.OAuthAccounts[0].ProviderEmail)

	// Verify passkeys
	suite.Require().Len(export.Passkeys, 1)
	suite.Equal("Export Key", export.Passkeys[0].DisplayName)

	// Verify submitted shows
	suite.Require().Len(export.SubmittedShows, 1)
	suite.Equal("Export Show", export.SubmittedShows[0].Title)

	// Verify saved shows
	suite.Require().Len(export.SavedShows, 1)
	suite.Equal(showID, export.SavedShows[0].ShowID)
}

func (suite *UserServiceIntegrationTestSuite) TestExportUserDataJSON() {
	user, err := suite.userService.CreateUserWithPassword(
		"exportjson@example.com", "ExportPass1!", "JSON", "Export",
	)
	suite.Require().NoError(err)

	jsonBytes, err := suite.userService.ExportUserDataJSON(user.ID)
	suite.Require().NoError(err)
	suite.NotEmpty(jsonBytes)

	// Verify it's valid JSON
	var parsed map[string]interface{}
	err = json.Unmarshal(jsonBytes, &parsed)
	suite.Require().NoError(err)

	// Verify required fields exist
	suite.Contains(parsed, "export_version")
	suite.Equal("1.0", parsed["export_version"])
	suite.Contains(parsed, "profile")
	suite.Contains(parsed, "exported_at")
}

// =============================================================================
// NEW INTEGRATION TESTS: OAuth Management
// =============================================================================

func (suite *UserServiceIntegrationTestSuite) TestGetOAuthAccounts_Success() {
	user := &models.User{
		Email:    stringPtr("getoauth@example.com"),
		IsActive: true,
	}
	suite.db.Create(user)

	suite.db.Create(&models.OAuthAccount{
		UserID:         user.ID,
		Provider:       "google",
		ProviderUserID: "get_oauth_google_123",
		ProviderEmail:  stringPtr("getoauth@example.com"),
	})
	suite.db.Create(&models.OAuthAccount{
		UserID:         user.ID,
		Provider:       "github",
		ProviderUserID: "get_oauth_github_456",
		ProviderEmail:  stringPtr("getoauth@example.com"),
	})

	accounts, err := suite.userService.GetOAuthAccounts(user.ID)
	suite.Require().NoError(err)
	suite.Require().Len(accounts, 2)
}

func (suite *UserServiceIntegrationTestSuite) TestCanUnlinkOAuthAccount_HasPassword() {
	// Create user with password + OAuth
	user, err := suite.userService.CreateUserWithPassword(
		"canunlink-pw@example.com", "Pass123!", "Can", "Unlink",
	)
	suite.Require().NoError(err)

	suite.db.Create(&models.OAuthAccount{
		UserID:         user.ID,
		Provider:       "google",
		ProviderUserID: "canunlink_google_123",
		ProviderEmail:  stringPtr("canunlink-pw@example.com"),
	})

	canUnlink, reason, err := suite.userService.CanUnlinkOAuthAccount(user.ID, "google")
	suite.Require().NoError(err)
	suite.True(canUnlink)
	suite.Empty(reason)
}

func (suite *UserServiceIntegrationTestSuite) TestCanUnlinkOAuthAccount_OnlyAuth() {
	// Create user without password, with a single OAuth account
	user, err := suite.userService.CreateUserWithoutPassword("canunlink-only@example.com")
	suite.Require().NoError(err)

	suite.db.Create(&models.OAuthAccount{
		UserID:         user.ID,
		Provider:       "google",
		ProviderUserID: "canunlink_only_google_123",
		ProviderEmail:  stringPtr("canunlink-only@example.com"),
	})

	canUnlink, reason, err := suite.userService.CanUnlinkOAuthAccount(user.ID, "google")
	suite.Require().NoError(err)
	suite.False(canUnlink)
	suite.Contains(reason, "only sign-in method")
}

func (suite *UserServiceIntegrationTestSuite) TestUnlinkOAuthAccount_Success() {
	// Create user with password + two OAuth accounts
	user, err := suite.userService.CreateUserWithPassword(
		"unlinktest@example.com", "Pass123!", "Unlink", "Test",
	)
	suite.Require().NoError(err)

	suite.db.Create(&models.OAuthAccount{
		UserID:         user.ID,
		Provider:       "google",
		ProviderUserID: "unlink_google_123",
		ProviderEmail:  stringPtr("unlinktest@example.com"),
	})
	suite.db.Create(&models.OAuthAccount{
		UserID:         user.ID,
		Provider:       "github",
		ProviderUserID: "unlink_github_456",
		ProviderEmail:  stringPtr("unlinktest@example.com"),
	})

	err = suite.userService.UnlinkOAuthAccount(user.ID, "google")
	suite.Require().NoError(err)

	// Verify only github remains
	accounts, err := suite.userService.GetOAuthAccounts(user.ID)
	suite.Require().NoError(err)
	suite.Require().Len(accounts, 1)
	suite.Equal("github", accounts[0].Provider)
}

// =============================================================================
// NEW INTEGRATION TESTS: GetUserByEmailIncludingDeleted
// =============================================================================

func (suite *UserServiceIntegrationTestSuite) TestGetUserByEmailIncludingDeleted_Active() {
	user := &models.User{
		Email:    stringPtr("incldeleted-active@example.com"),
		IsActive: true,
	}
	suite.db.Create(user)

	found, err := suite.userService.GetUserByEmailIncludingDeleted("incldeleted-active@example.com")
	suite.Require().NoError(err)
	suite.Require().NotNil(found)
	suite.Equal(user.ID, found.ID)
}

func (suite *UserServiceIntegrationTestSuite) TestGetUserByEmailIncludingDeleted_Deleted() {
	now := time.Now()
	user := &models.User{
		Email:     stringPtr("incldeleted-gone@example.com"),
		IsActive:  true,
		DeletedAt: &now,
	}
	suite.db.Create(user)
	suite.db.Model(&models.User{}).Where("id = ?", user.ID).Update("is_active", false)

	found, err := suite.userService.GetUserByEmailIncludingDeleted("incldeleted-gone@example.com")
	suite.Require().NoError(err)
	suite.Require().NotNil(found)
	suite.Equal(user.ID, found.ID)
	suite.False(found.IsActive)
}

func (suite *UserServiceIntegrationTestSuite) TestGetUserByEmailIncludingDeleted_NotFound() {
	found, err := suite.userService.GetUserByEmailIncludingDeleted("nope-incldeleted@example.com")
	suite.Require().NoError(err)
	suite.Nil(found)
}

// =============================================================================
// NEW INTEGRATION TESTS: ListUsers Admin
// =============================================================================

func (suite *UserServiceIntegrationTestSuite) TestListUsers_Success() {
	// Create 3 users with unique prefix for search isolation
	for i := 1; i <= 3; i++ {
		suite.db.Create(&models.User{
			Email:    stringPtr(fmt.Sprintf("listuser%d@listtest.example.com", i)),
			IsActive: true,
		})
	}

	users, total, err := suite.userService.ListUsers(100, 0, AdminUserFilters{
		Search: "listtest.example.com",
	})
	suite.Require().NoError(err)
	suite.GreaterOrEqual(total, int64(3))
	suite.GreaterOrEqual(len(users), 3)
}

func (suite *UserServiceIntegrationTestSuite) TestListUsers_WithSearch() {
	suite.db.Create(&models.User{
		Email:    stringPtr("searchmatch@uniquesearch.example.com"),
		IsActive: true,
	})
	suite.db.Create(&models.User{
		Email:    stringPtr("nomatch@other.example.com"),
		IsActive: true,
	})

	users, total, err := suite.userService.ListUsers(100, 0, AdminUserFilters{
		Search: "uniquesearch",
	})
	suite.Require().NoError(err)
	suite.Equal(int64(1), total)
	suite.Require().Len(users, 1)
	suite.Equal("searchmatch@uniquesearch.example.com", *users[0].Email)
}

func (suite *UserServiceIntegrationTestSuite) TestListUsers_WithAuthMethods() {
	// Create user with password + OAuth
	user, err := suite.userService.CreateUserWithPassword(
		"listauth@authmethods.example.com", "Pass123!", "List", "Auth",
	)
	suite.Require().NoError(err)

	suite.db.Create(&models.OAuthAccount{
		UserID:         user.ID,
		Provider:       "google",
		ProviderUserID: "listauth_google_123",
		ProviderEmail:  stringPtr("listauth@authmethods.example.com"),
	})

	users, _, err := suite.userService.ListUsers(100, 0, AdminUserFilters{
		Search: "authmethods.example.com",
	})
	suite.Require().NoError(err)
	suite.Require().Len(users, 1)

	authMethods := users[0].AuthMethods
	suite.Contains(authMethods, "password")
	suite.Contains(authMethods, "google")
}

// =============================================================================
// HELPER FUNCTIONS
// =============================================================================

// Note: stringPtr helper function is defined in auth_test.go
