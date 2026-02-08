package services

import (
	"context"
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
			Image:        "postgres:17.5",
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
	migrationSQL, err := os.ReadFile(filepath.Join("..", "..", "db", "migrations", "000001_create_initial_schema.up.sql"))
	if err != nil {
		suite.T().Fatalf("failed to read migration file: %v", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		suite.T().Fatalf("failed to get sql.DB: %v", err)
	}

	_, err = sqlDB.Exec(string(migrationSQL))
	if err != nil {
		suite.T().Fatalf("failed to run migrations: %v", err)
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

// TestGetUserByID_Success tests successful user retrieval by ID
func (suite *UserServiceIntegrationTestSuite) TestGetUserByID_Success() {
	// Create a test user
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

	// Test GetUserByID
	retrievedUser, err := suite.userService.GetUserByID(user.ID)

	assert.NoError(suite.T(), err)
	assert.NotNil(suite.T(), retrievedUser)
	assert.Equal(suite.T(), user.ID, retrievedUser.ID)
	assert.Equal(suite.T(), *user.Email, *retrievedUser.Email)
	assert.Equal(suite.T(), *user.FirstName, *retrievedUser.FirstName)
	assert.Equal(suite.T(), *user.LastName, *retrievedUser.LastName)
}

// TestGetUserByEmail_Success tests successful user retrieval by email
func (suite *UserServiceIntegrationTestSuite) TestGetUserByEmail_Success() {
	// Create a test user
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

	// Test GetUserByEmail
	retrievedUser, err := suite.userService.GetUserByEmail(*user.Email)

	assert.NoError(suite.T(), err)
	assert.NotNil(suite.T(), retrievedUser)
	assert.Equal(suite.T(), user.ID, retrievedUser.ID)
	assert.Equal(suite.T(), *user.Email, *retrievedUser.Email)
}

// TestGetUserByEmail_NotFound tests graceful handling of non-existent users
func (suite *UserServiceIntegrationTestSuite) TestGetUserByEmail_NotFound() {
	// Test GetUserByEmail with non-existent email
	retrievedUser, err := suite.userService.GetUserByEmail("nonexistent@example.com")

	assert.NoError(suite.T(), err) // Should not return error for not found
	assert.Nil(suite.T(), retrievedUser)
}

// TestUpdateUser_Success tests successful user updates
func (suite *UserServiceIntegrationTestSuite) TestUpdateUser_Success() {
	// Create a test user
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

	// Test UpdateUser
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

// TestFindOrCreateUser_NewUser tests OAuth user creation flow
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

	// Test FindOrCreateUser with new user
	user, err := suite.userService.FindOrCreateUser(gothUser, "google")

	assert.NoError(suite.T(), err)
	assert.NotNil(suite.T(), user)
	assert.NotZero(suite.T(), user.ID)
	assert.Equal(suite.T(), "newuser@example.com", *user.Email)
	assert.Equal(suite.T(), "New", *user.FirstName)
	assert.Equal(suite.T(), "User", *user.LastName)
	assert.True(suite.T(), user.IsActive)
	assert.True(suite.T(), user.EmailVerified)

	// Verify OAuth account was created
	var oauthCount int64
	err = suite.db.Model(&models.OAuthAccount{}).Where("user_id = ? AND provider = ?", user.ID, "google").Count(&oauthCount).Error
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), int64(1), oauthCount)

	// Verify user preferences were created
	var prefCount int64
	err = suite.db.Model(&models.UserPreferences{}).Where("user_id = ?", user.ID).Count(&prefCount).Error
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), int64(1), prefCount)
}

// TestFindOrCreateUser_ExistingOAuthAccount tests OAuth user linking
func (suite *UserServiceIntegrationTestSuite) TestFindOrCreateUser_ExistingOAuthAccount() {
	// Create a test user first
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

	// Create OAuth account
	oauthAccount := &models.OAuthAccount{
		UserID:         user.ID,
		Provider:       "github",
		ProviderUserID: "github_123",
		ProviderEmail:  stringPtr("existing@example.com"),
		ProviderName:   stringPtr("Existing User"),
	}

	err = suite.db.Create(oauthAccount).Error
	assert.NoError(suite.T(), err)

	// Test FindOrCreateUser with existing OAuth account
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

// TestUserServiceIntegrationTestSuite_LinkOAuthAccount tests the linkOAuthAccount functionality
func (suite *UserServiceIntegrationTestSuite) TestLinkOAuthAccount_NewAccount() {
	// Create a test user first
	user := &models.User{
		Email: stringPtr("linktest@example.com"),
	}

	err := suite.db.Create(user).Error
	suite.Require().NoError(err)
	suite.Require().NotZero(user.ID)

	// Test linking a new OAuth account
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

	// Verify OAuth account details
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
	// Create a test user first
	user := &models.User{
		Email: stringPtr("updatetest@example.com"),
	}

	err := suite.db.Create(user).Error
	suite.Require().NoError(err)
	suite.Require().NotZero(user.ID)

	// Create an existing OAuth account
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

	// Test updating the existing OAuth account
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

	// Verify OAuth account was updated
	oauthAccount := linkedUser.OAuthAccounts[0]
	suite.Equal("google", oauthAccount.Provider)
	suite.Equal("google_update_existing_12345", oauthAccount.ProviderUserID)
	suite.Equal("new@example.com", *oauthAccount.ProviderEmail)
	suite.Equal("New Name", *oauthAccount.ProviderName)
	suite.Equal("https://example.com/new-avatar.jpg", *oauthAccount.ProviderAvatarURL)
	suite.Equal("new_access_token", *oauthAccount.AccessToken)
	suite.Equal("new_refresh_token", *oauthAccount.RefreshToken)
	suite.NotNil(oauthAccount.ExpiresAt)

	// Verify the record was updated (not created new)
	suite.Equal(existingOAuth.ID, oauthAccount.ID)
}

func (suite *UserServiceIntegrationTestSuite) TestLinkOAuthAccount_WithoutExpiresAt() {
	// Create a test user first
	user := &models.User{
		Email: stringPtr("noexpiry@example.com"),
	}

	err := suite.db.Create(user).Error
	suite.Require().NoError(err)
	suite.Require().NotZero(user.ID)

	// Test linking OAuth account without expiry time
	gothUser := goth.User{
		UserID:       "github_no_expiry_67890",
		Email:        "noexpiry@example.com",
		Name:         "No Expiry User",
		AvatarURL:    "https://github.com/avatar.jpg",
		AccessToken:  "github_access_token",
		RefreshToken: "github_refresh_token",
		// ExpiresAt is zero time (not set)
	}

	linkedUser, err := suite.userService.linkOAuthAccount(user, gothUser, "github")

	suite.Require().NoError(err)
	suite.Require().NotNil(linkedUser)
	suite.Require().Len(linkedUser.OAuthAccounts, 1)

	// Verify OAuth account details
	oauthAccount := linkedUser.OAuthAccounts[0]
	suite.Equal("github", oauthAccount.Provider)
	suite.Equal("github_no_expiry_67890", oauthAccount.ProviderUserID)
	suite.Equal("noexpiry@example.com", *oauthAccount.ProviderEmail)
	suite.Equal("No Expiry User", *oauthAccount.ProviderName)
	suite.Equal("https://github.com/avatar.jpg", *oauthAccount.ProviderAvatarURL)
	suite.Equal("github_access_token", *oauthAccount.AccessToken)
	suite.Equal("github_refresh_token", *oauthAccount.RefreshToken)
	suite.Nil(oauthAccount.ExpiresAt) // Should be nil when not set
}

func (suite *UserServiceIntegrationTestSuite) TestLinkOAuthAccount_MultipleProviders() {
	// Create a test user first
	user := &models.User{
		Email: stringPtr("multiprovider@example.com"),
	}

	err := suite.db.Create(user).Error
	suite.Require().NoError(err)
	suite.Require().NotZero(user.ID)

	// Link Google OAuth account
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

	// Link GitHub OAuth account to the same user
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

	// Verify both OAuth accounts exist
	providers := make(map[string]bool)
	for _, oauth := range linkedUser.OAuthAccounts {
		providers[oauth.Provider] = true
	}
	suite.True(providers["google"])
	suite.True(providers["github"])
}

func (suite *UserServiceIntegrationTestSuite) TestLinkOAuthAccount_EmptyFields() {
	// Create a test user first
	user := &models.User{
		Email: stringPtr("emptyfields@example.com"),
	}

	err := suite.db.Create(user).Error
	suite.Require().NoError(err)
	suite.Require().NotZero(user.ID)

	// Test linking OAuth account with empty fields
	gothUser := goth.User{
		UserID:       "empty_fields_12345",
		Email:        "", // Empty email
		Name:         "", // Empty name
		AvatarURL:    "", // Empty avatar
		AccessToken:  "access_token",
		RefreshToken: "refresh_token",
	}

	linkedUser, err := suite.userService.linkOAuthAccount(user, gothUser, "empty")

	suite.Require().NoError(err)
	suite.Require().NotNil(linkedUser)
	suite.Require().Len(linkedUser.OAuthAccounts, 1)

	// Verify OAuth account details
	oauthAccount := linkedUser.OAuthAccounts[0]
	suite.Equal("empty", oauthAccount.Provider)
	suite.Equal("empty_fields_12345", oauthAccount.ProviderUserID)
	suite.Equal("", *oauthAccount.ProviderEmail)
	suite.Equal("", *oauthAccount.ProviderName)
	suite.Equal("", *oauthAccount.ProviderAvatarURL)
	suite.Equal("access_token", *oauthAccount.AccessToken)
	suite.Equal("refresh_token", *oauthAccount.RefreshToken)
}

// TestUserServiceIntegrationTestSuite_GetUserByUsername tests the GetUserByUsername functionality
func (suite *UserServiceIntegrationTestSuite) TestGetUserByUsername_Success() {
	// Create a test user with username
	username := "testuser123"
	user := &models.User{
		Email:    stringPtr("username@example.com"),
		Username: &username,
	}

	err := suite.db.Create(user).Error
	suite.Require().NoError(err)
	suite.Require().NotZero(user.ID)

	// Test retrieving user by username
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
	// Test retrieving non-existent username
	retrievedUser, err := suite.userService.GetUserByUsername("nonexistent_username")

	suite.Require().NoError(err)
	suite.Require().Nil(retrievedUser) // Should return nil, not error
}

func (suite *UserServiceIntegrationTestSuite) TestGetUserByUsername_WithOAuthAccounts() {
	// Create a test user with username
	username := "oauthuser456"
	user := &models.User{
		Email:    stringPtr("oauthuser@example.com"),
		Username: &username,
	}

	err := suite.db.Create(user).Error
	suite.Require().NoError(err)
	suite.Require().NotZero(user.ID)

	// Create OAuth account for the user
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

	// Test retrieving user by username with OAuth accounts
	retrievedUser, err := suite.userService.GetUserByUsername(username)

	suite.Require().NoError(err)
	suite.Require().NotNil(retrievedUser)
	suite.Equal(user.ID, retrievedUser.ID)
	suite.Equal(*user.Email, *retrievedUser.Email)
	suite.Equal(*user.Username, *retrievedUser.Username)
	suite.Require().Len(retrievedUser.OAuthAccounts, 1)

	// Verify OAuth account details
	oauth := retrievedUser.OAuthAccounts[0]
	suite.Equal("google", oauth.Provider)
	suite.Equal("google_oauth_test_123", oauth.ProviderUserID)
	suite.Equal("oauthuser@example.com", *oauth.ProviderEmail)
	suite.Equal("OAuth Test User", *oauth.ProviderName)
}

func (suite *UserServiceIntegrationTestSuite) TestGetUserByUsername_WithPreferences() {
	// Create a test user with username
	username := "prefuser789"
	user := &models.User{
		Email:    stringPtr("prefuser@example.com"),
		Username: &username,
	}

	err := suite.db.Create(user).Error
	suite.Require().NoError(err)
	suite.Require().NotZero(user.ID)

	// Create user preferences
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

	// Test retrieving user by username with preferences
	retrievedUser, err := suite.userService.GetUserByUsername(username)

	suite.Require().NoError(err)
	suite.Require().NotNil(retrievedUser)
	suite.Equal(user.ID, retrievedUser.ID)
	suite.Equal(*user.Email, *retrievedUser.Email)
	suite.Equal(*user.Username, *retrievedUser.Username)
	suite.Require().NotNil(retrievedUser.Preferences)

	// Verify preferences details
	prefs := retrievedUser.Preferences
	suite.Equal(user.ID, prefs.UserID)
	suite.True(prefs.NotificationEmail)
	suite.False(prefs.NotificationPush)
	suite.Equal("dark", prefs.Theme)
	suite.Equal("America/New_York", prefs.Timezone)
	suite.Equal("en", prefs.Language)
}

func (suite *UserServiceIntegrationTestSuite) TestGetUserByUsername_WithOAuthAndPreferences() {
	// Create a test user with username
	username := "fulluser999"
	user := &models.User{
		Email:    stringPtr("fulluser@example.com"),
		Username: &username,
	}

	err := suite.db.Create(user).Error
	suite.Require().NoError(err)
	suite.Require().NotZero(user.ID)

	// Create OAuth account
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

	// Create user preferences with explicit values
	err = suite.db.Exec(`
		INSERT INTO user_preferences (user_id, notification_email, notification_push, theme, timezone, language, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, NOW(), NOW())
	`, user.ID, false, true, "light", "UTC", "es").Error
	suite.Require().NoError(err)

	// Test retrieving user by username with both OAuth and preferences
	retrievedUser, err := suite.userService.GetUserByUsername(username)

	suite.Require().NoError(err)
	suite.Require().NotNil(retrievedUser)
	suite.Equal(user.ID, retrievedUser.ID)
	suite.Equal(*user.Email, *retrievedUser.Email)
	suite.Equal(*user.Username, *retrievedUser.Username)
	suite.Require().Len(retrievedUser.OAuthAccounts, 1)
	suite.Require().NotNil(retrievedUser.Preferences)

	// Verify OAuth account
	oauth := retrievedUser.OAuthAccounts[0]
	suite.Equal("github", oauth.Provider)
	suite.Equal("github_full_test_456", oauth.ProviderUserID)

	// Verify preferences
	prefs := retrievedUser.Preferences
	suite.Equal(user.ID, prefs.UserID)
	suite.False(prefs.NotificationEmail)
	suite.True(prefs.NotificationPush)
	suite.Equal("light", prefs.Theme)
	suite.Equal("UTC", prefs.Timezone)
	suite.Equal("es", prefs.Language)
}

func (suite *UserServiceIntegrationTestSuite) TestGetUserByUsername_EmptyUsername() {
	// Test retrieving with empty username
	retrievedUser, err := suite.userService.GetUserByUsername("")

	suite.Require().NoError(err)
	suite.Require().Nil(retrievedUser) // Should return nil, not error
}

func (suite *UserServiceIntegrationTestSuite) TestGetUserByUsername_SpecialCharacters() {
	// Create a test user with special characters in username
	username := "user-name_with.underscores+plus"
	user := &models.User{
		Email:    stringPtr("special@example.com"),
		Username: &username,
	}

	err := suite.db.Create(user).Error
	suite.Require().NoError(err)
	suite.Require().NotZero(user.ID)

	// Test retrieving user by username with special characters
	retrievedUser, err := suite.userService.GetUserByUsername(username)

	suite.Require().NoError(err)
	suite.Require().NotNil(retrievedUser)
	suite.Equal(user.ID, retrievedUser.ID)
	suite.Equal(*user.Email, *retrievedUser.Email)
	suite.Equal(*user.Username, *retrievedUser.Username)
}

func (suite *UserServiceIntegrationTestSuite) TestGetUserByUsername_VeryLongUsername() {
	// Create a test user with very long username
	username := "very_long_username_that_is_quite_lengthy_and_might_test_boundaries_123456789"
	user := &models.User{
		Email:    stringPtr("longuser@example.com"),
		Username: &username,
	}

	err := suite.db.Create(user).Error
	suite.Require().NoError(err)
	suite.Require().NotZero(user.ID)

	// Test retrieving user by very long username
	retrievedUser, err := suite.userService.GetUserByUsername(username)

	suite.Require().NoError(err)
	suite.Require().NotNil(retrievedUser)
	suite.Equal(user.ID, retrievedUser.ID)
	suite.Equal(*user.Email, *retrievedUser.Email)
	suite.Equal(*user.Username, *retrievedUser.Username)
}

// =============================================================================
// HELPER FUNCTIONS
// =============================================================================

// Note: stringPtr helper function is defined in auth_test.go
