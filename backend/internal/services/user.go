package services

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/markbates/goth"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"psychic-homily-backend/db"
	apperrors "psychic-homily-backend/internal/errors"
	"psychic-homily-backend/internal/models"
)

// Account lockout constants
const (
	MaxFailedLoginAttempts = 5
	AccountLockDuration    = 15 * time.Minute
)

// AdminUserFilters contains filter criteria for listing users
type AdminUserFilters struct {
	Search string // ILIKE match on email or username
}

// UserSubmissionStats contains show submission counts by status
type UserSubmissionStats struct {
	Approved int64 `json:"approved"`
	Pending  int64 `json:"pending"`
	Rejected int64 `json:"rejected"`
	Total    int64 `json:"total"`
}

// AdminUserResponse is the response type for the admin user list
type AdminUserResponse struct {
	ID              uint                `json:"id"`
	Email           *string             `json:"email"`
	Username        *string             `json:"username"`
	FirstName       *string             `json:"first_name"`
	LastName        *string             `json:"last_name"`
	AvatarURL       *string             `json:"avatar_url"`
	IsActive        bool                `json:"is_active"`
	IsAdmin         bool                `json:"is_admin"`
	EmailVerified   bool                `json:"email_verified"`
	AuthMethods     []string            `json:"auth_methods"`
	SubmissionStats UserSubmissionStats  `json:"submission_stats"`
	CreatedAt       time.Time           `json:"created_at"`
	DeletedAt       *time.Time          `json:"deleted_at,omitempty"`
}

// ListUsers returns a paginated list of users for the admin console
func (s *UserService) ListUsers(limit, offset int, filters AdminUserFilters) ([]*AdminUserResponse, int64, error) {
	if s.db == nil {
		return nil, 0, fmt.Errorf("database not initialized")
	}

	// Base query
	query := s.db.Model(&models.User{})

	// Apply search filter
	if filters.Search != "" {
		searchPattern := "%" + filters.Search + "%"
		query = query.Where("(email ILIKE ? OR username ILIKE ?)", searchPattern, searchPattern)
	}

	// Get total count
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to count users: %w", err)
	}

	// Fetch users with OAuth accounts preloaded
	var users []models.User
	if err := query.
		Preload("OAuthAccounts").
		Order("created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&users).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to list users: %w", err)
	}

	if len(users) == 0 {
		return []*AdminUserResponse{}, total, nil
	}

	// Collect user IDs for batch queries
	userIDs := make([]uint, len(users))
	for i, u := range users {
		userIDs[i] = u.ID
	}

	// Batch load passkey counts
	type passkeyCount struct {
		UserID uint
		Count  int64
	}
	var passkeyCounts []passkeyCount
	s.db.Model(&models.WebAuthnCredential{}).
		Select("user_id, COUNT(*) as count").
		Where("user_id IN ?", userIDs).
		Group("user_id").
		Scan(&passkeyCounts)

	passkeyMap := make(map[uint]int64, len(passkeyCounts))
	for _, pc := range passkeyCounts {
		passkeyMap[pc.UserID] = pc.Count
	}

	// Batch load show stats
	type showStat struct {
		SubmittedBy uint
		Status      string
		Count       int64
	}
	var showStats []showStat
	s.db.Model(&models.Show{}).
		Select("submitted_by, status, COUNT(*) as count").
		Where("submitted_by IN ?", userIDs).
		Group("submitted_by, status").
		Scan(&showStats)

	statsMap := make(map[uint]UserSubmissionStats, len(users))
	for _, ss := range showStats {
		stats := statsMap[ss.SubmittedBy]
		switch ss.Status {
		case "approved":
			stats.Approved = ss.Count
		case "pending":
			stats.Pending = ss.Count
		case "rejected":
			stats.Rejected = ss.Count
		}
		stats.Total += ss.Count
		statsMap[ss.SubmittedBy] = stats
	}

	// Build response
	result := make([]*AdminUserResponse, len(users))
	for i, u := range users {
		// Derive auth methods
		var authMethods []string
		if u.PasswordHash != nil && *u.PasswordHash != "" {
			authMethods = append(authMethods, "password")
		}
		for _, oauth := range u.OAuthAccounts {
			authMethods = append(authMethods, oauth.Provider)
		}
		if passkeyMap[u.ID] > 0 {
			authMethods = append(authMethods, "passkey")
		}

		result[i] = &AdminUserResponse{
			ID:              u.ID,
			Email:           u.Email,
			Username:        u.Username,
			FirstName:       u.FirstName,
			LastName:        u.LastName,
			AvatarURL:       u.AvatarURL,
			IsActive:        u.IsActive,
			IsAdmin:         u.IsAdmin,
			EmailVerified:   u.EmailVerified,
			AuthMethods:     authMethods,
			SubmissionStats: statsMap[u.ID],
			CreatedAt:       u.CreatedAt,
			DeletedAt:       u.DeletedAt,
		}
	}

	return result, total, nil
}

// UserService handles user-related business logic
type UserService struct {
	db *gorm.DB
}

// NewUserService creates a new user service
func NewUserService(database *gorm.DB) *UserService {
	if database == nil {
		database = db.GetDB()
	}
	return &UserService{
		db: database,
	}
}

// FindOrCreateUser finds existing user or creates new one from OAuth data
func (s *UserService) FindOrCreateUser(gothUser goth.User, provider string) (*models.User, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	// First, try to find existing OAuth account
	var oauthAccount models.OAuthAccount

	result := s.db.
		Where("provider = ? AND provider_user_id = ?", provider, gothUser.UserID).
		First(&oauthAccount)

	if result.Error == nil {
		// OAuth account exists, get the user
		var user models.User
		if err := s.db.Preload("OAuthAccounts").Preload("Preferences").First(&user, oauthAccount.UserID).Error; err != nil {
			return nil, fmt.Errorf("failed to get user: %w", err)
		}
		return &user, nil
	}

	if result.Error != gorm.ErrRecordNotFound {
		return nil, fmt.Errorf("database error: %w", result.Error)
	}

	// OAuth account doesn't exist, check if user exists by email
	var existingUser models.User
	if gothUser.Email != "" {
		result.Error = s.db.Where("email = ?", gothUser.Email).First(&existingUser).Error
		if result.Error == nil {
			// User exists, link OAuth account
			return s.linkOAuthAccount(&existingUser, gothUser, provider)
		}
		if result.Error != gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("database error: %w", result.Error)
		}
	}

	// Create new user
	return s.createNewUserOauth(gothUser, provider)
}

// creates a new user with email and password
func (s *UserService) CreateUserWithPassword(email, password, firstName, lastName string) (
	*models.User, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	existingUser, err := s.GetUserByEmail(email)
	if err != nil {
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	if existingUser != nil {
		return nil, apperrors.ErrUserExists(email)
	}

	hashedPassword, err := s.HashPassword(password)
	if err != nil {
		return nil, fmt.Errorf("failed to hash password: %w", err)
	}

	tx := s.db.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()
	// Create user
	user := &models.User{
		Email:         &email,
		PasswordHash:  &hashedPassword,
		FirstName:     &firstName,
		LastName:      &lastName,
		IsActive:      true,
		EmailVerified: false, // Email verification required for password users
	}

	if err := tx.Create(user).Error; err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("failed to create user: %w", err)
	}

	// Create default preferences
	preferences := &models.UserPreferences{
		UserID: user.ID,
	}

	if err := tx.Create(preferences).Error; err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("failed to create user preferences: %w", err)
	}

	if err := tx.Commit().Error; err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	// Load relationships
	if err := s.db.Preload("Preferences").First(user, user.ID).Error; err != nil {
		return nil, fmt.Errorf("failed to load user relationships: %w", err)
	}

	return user, nil
}

func (s *UserService) AuthenticateUserWithPassword(email, password string) (*models.User, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	user, err := s.GetUserByEmail(email)
	if err != nil {
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	// To prevent timing attacks that reveal whether an email exists,
	// always perform a bcrypt comparison even if user doesn't exist
	if user == nil {
		// Use a dummy hash to maintain constant time
		// This will always fail but takes the same time as a real verification
		dummyHash := "$2a$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy" // bcrypt hash of "dummy"
		bcrypt.CompareHashAndPassword([]byte(dummyHash), []byte(password))
		return nil, apperrors.ErrInvalidCredentials(nil)
	}

	// Check if account is locked before password verification
	if s.IsAccountLocked(user) {
		remaining := s.GetLockTimeRemaining(user)
		minutes := int(remaining.Minutes()) + 1 // Round up
		return nil, apperrors.ErrAccountLockedWithMinutes(minutes)
	}

	if user.PasswordHash == nil {
		// Also do dummy verification for OAuth-only accounts
		dummyHash := "$2a$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy"
		bcrypt.CompareHashAndPassword([]byte(dummyHash), []byte(password))
		return nil, apperrors.ErrInvalidCredentials(nil)
	}

	if err := s.VerifyPassword(*user.PasswordHash, password); err != nil {
		// Increment failed attempts on password failure
		s.IncrementFailedAttempts(user.ID)
		return nil, apperrors.ErrInvalidCredentials(nil)
	}

	if !user.IsActive {
		return nil, fmt.Errorf("user account is not active")
	}

	// Reset failed attempts on successful login
	s.ResetFailedAttempts(user.ID)

	return user, nil
}

// createNewUserOauth creates a new user with OAuth account
func (s *UserService) createNewUserOauth(gothUser goth.User, provider string) (*models.User, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	// Start transaction
	tx := s.db.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// Create user
	user := &models.User{
		Email:         &gothUser.Email,
		FirstName:     &gothUser.FirstName,
		LastName:      &gothUser.LastName,
		AvatarURL:     &gothUser.AvatarURL,
		IsActive:      true,
		EmailVerified: true, // OAuth users are email verified
	}

	if err := tx.Create(user).Error; err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("failed to create user: %w", err)
	}

	// Create OAuth account
	oauthAccount := &models.OAuthAccount{
		UserID:            user.ID,
		Provider:          provider,
		ProviderUserID:    gothUser.UserID,
		ProviderEmail:     &gothUser.Email,
		ProviderName:      &gothUser.Name,
		ProviderAvatarURL: &gothUser.AvatarURL,
		AccessToken:       &gothUser.AccessToken,
		RefreshToken:      &gothUser.RefreshToken,
	}

	// Check if ExpiresAt is not zero time (which indicates it's set)
	if !gothUser.ExpiresAt.IsZero() {
		oauthAccount.ExpiresAt = &gothUser.ExpiresAt
	}

	if err := tx.Create(oauthAccount).Error; err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("failed to create OAuth account: %w", err)
	}

	// Create default preferences
	preferences := &models.UserPreferences{
		UserID: user.ID,
	}

	if err := tx.Create(preferences).Error; err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("failed to create user preferences: %w", err)
	}

	// Commit transaction
	if err := tx.Commit().Error; err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	// Load relationships
	if err := s.db.Preload("OAuthAccounts").Preload("Preferences").First(user, user.ID).Error; err != nil {
		return nil, fmt.Errorf("failed to load user relationships: %w", err)
	}

	return user, nil
}

// linkOAuthAccount links OAuth account to existing user
func (s *UserService) linkOAuthAccount(user *models.User, gothUser goth.User, provider string) (*models.User, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	// Check if OAuth account already exists for this provider
	var existingOAuth models.OAuthAccount
	err := s.db.Where("user_id = ? AND provider = ?", user.ID, provider).First(&existingOAuth).Error

	if err == nil {
		// Update existing OAuth account
		existingOAuth.ProviderEmail = &gothUser.Email
		existingOAuth.ProviderName = &gothUser.Name
		existingOAuth.ProviderAvatarURL = &gothUser.AvatarURL
		existingOAuth.AccessToken = &gothUser.AccessToken
		existingOAuth.RefreshToken = &gothUser.RefreshToken
		// Check if ExpiresAt is not zero time (which indicates it's set)
		if !gothUser.ExpiresAt.IsZero() {
			existingOAuth.ExpiresAt = &gothUser.ExpiresAt
		}

		if err := s.db.Save(&existingOAuth).Error; err != nil {
			return nil, fmt.Errorf("failed to update OAuth account: %w", err)
		}
	} else if err == gorm.ErrRecordNotFound {
		// Create new OAuth account
		oauthAccount := &models.OAuthAccount{
			UserID:            user.ID,
			Provider:          provider,
			ProviderUserID:    gothUser.UserID,
			ProviderEmail:     &gothUser.Email,
			ProviderName:      &gothUser.Name,
			ProviderAvatarURL: &gothUser.AvatarURL,
			AccessToken:       &gothUser.AccessToken,
			RefreshToken:      &gothUser.RefreshToken,
		}

		// Check if ExpiresAt is not zero time (which indicates it's set)
		if !gothUser.ExpiresAt.IsZero() {
			oauthAccount.ExpiresAt = &gothUser.ExpiresAt
		}

		if err := s.db.Create(oauthAccount).Error; err != nil {
			return nil, fmt.Errorf("failed to create OAuth account: %w", err)
		}
	} else {
		return nil, fmt.Errorf("database error: %w", err)
	}

	// Load updated user with relationships
	if err := s.db.Preload("OAuthAccounts").Preload("Preferences").First(user, user.ID).Error; err != nil {
		return nil, fmt.Errorf("failed to load user: %w", err)
	}

	return user, nil
}

// GetUserByID retrieves a user by ID
func (s *UserService) GetUserByID(userID uint) (*models.User, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	var user models.User

	result := s.db.Preload("OAuthAccounts").
		Preload("Preferences").
		First(&user, userID)

	if result.Error != nil {
		return nil, fmt.Errorf("failed to get user: %w", result.Error)
	}

	return &user, nil
}

// GetUserByEmail retrieves a user by email
func (s *UserService) GetUserByEmail(email string) (*models.User, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	var user models.User

	result := s.db.Where("email = ?", email).
		Preload("OAuthAccounts").
		Preload("Preferences").
		First(&user)

	if result.Error != nil {
		if result.Error == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get user: %w", result.Error)
	}

	return &user, nil
}

func (s *UserService) GetUserByUsername(username string) (*models.User, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	var user models.User

	result := s.db.Where("username = ?", username).
		Preload("OAuthAccounts").
		Preload("Preferences").
		First(&user)

	if result.Error != nil {
		if result.Error == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get user: %w", result.Error)
	}

	return &user, nil
}

// UpdateUser updates user information
func (s *UserService) UpdateUser(userID uint, updates map[string]any) (*models.User, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	var user models.User

	result := s.db.Model(&user).
		Where("id = ?", userID).
		Updates(updates)

	if result.Error != nil {
		return nil, fmt.Errorf("failed to update user: %w", result.Error)
	}

	return s.GetUserByID(userID)
}

func (s *UserService) HashPassword(password string) (string, error) {
	hashedBytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", fmt.Errorf("failed to hash password: %w", err)
	}
	return string(hashedBytes), nil
}

func (s *UserService) VerifyPassword(hashedPassword, password string) error {
	return bcrypt.CompareHashAndPassword([]byte(hashedPassword), []byte(password))
}

// IsAccountLocked checks if the user account is currently locked
func (s *UserService) IsAccountLocked(user *models.User) bool {
	return user.LockedUntil != nil && time.Now().Before(*user.LockedUntil)
}

// GetLockTimeRemaining returns the duration until the account is unlocked
func (s *UserService) GetLockTimeRemaining(user *models.User) time.Duration {
	if user.LockedUntil == nil {
		return 0
	}
	remaining := time.Until(*user.LockedUntil)
	if remaining < 0 {
		return 0
	}
	return remaining
}

// IncrementFailedAttempts increments the counter and locks if threshold reached
func (s *UserService) IncrementFailedAttempts(userID uint) error {
	if s.db == nil {
		return fmt.Errorf("database not initialized")
	}

	// Use a transaction to atomically increment and check
	return s.db.Transaction(func(tx *gorm.DB) error {
		var user models.User
		if err := tx.First(&user, userID).Error; err != nil {
			return fmt.Errorf("failed to get user: %w", err)
		}

		user.FailedLoginAttempts++

		// Lock account if threshold reached
		if user.FailedLoginAttempts >= MaxFailedLoginAttempts {
			lockUntil := time.Now().Add(AccountLockDuration)
			user.LockedUntil = &lockUntil
		}

		if err := tx.Save(&user).Error; err != nil {
			return fmt.Errorf("failed to update user: %w", err)
		}

		return nil
	})
}

// ResetFailedAttempts clears the counter and unlocks the account
func (s *UserService) ResetFailedAttempts(userID uint) error {
	if s.db == nil {
		return fmt.Errorf("database not initialized")
	}

	result := s.db.Model(&models.User{}).
		Where("id = ?", userID).
		Updates(map[string]interface{}{
			"failed_login_attempts": 0,
			"locked_until":          nil,
		})

	if result.Error != nil {
		return fmt.Errorf("failed to reset failed attempts: %w", result.Error)
	}

	return nil
}

// UpdatePassword updates a user's password after verifying the current password
func (s *UserService) UpdatePassword(userID uint, currentPassword, newPassword string) error {
	if s.db == nil {
		return fmt.Errorf("database not initialized")
	}

	// Get user with current password hash
	user, err := s.GetUserByID(userID)
	if err != nil {
		return fmt.Errorf("failed to get user: %w", err)
	}

	// Check if user has a password (not OAuth-only)
	if user.PasswordHash == nil {
		return apperrors.ErrNoPasswordSet()
	}

	// Verify current password
	if err := s.VerifyPassword(*user.PasswordHash, currentPassword); err != nil {
		return apperrors.ErrInvalidCredentials(nil)
	}

	// Hash the new password
	hashedPassword, err := s.HashPassword(newPassword)
	if err != nil {
		return fmt.Errorf("failed to hash password: %w", err)
	}

	// Update the password in the database
	result := s.db.Model(&models.User{}).
		Where("id = ?", userID).
		Update("password_hash", hashedPassword)

	if result.Error != nil {
		return fmt.Errorf("failed to update password: %w", result.Error)
	}

	if result.RowsAffected == 0 {
		return fmt.Errorf("user not found")
	}

	return nil
}

// SetEmailVerified updates the email_verified status for a user
func (s *UserService) SetEmailVerified(userID uint, verified bool) error {
	if s.db == nil {
		return fmt.Errorf("database not initialized")
	}

	result := s.db.Model(&models.User{}).
		Where("id = ?", userID).
		Update("email_verified", verified)

	if result.Error != nil {
		return fmt.Errorf("failed to update email verified status: %w", result.Error)
	}

	if result.RowsAffected == 0 {
		return fmt.Errorf("user not found")
	}

	return nil
}

// DeletionSummary contains counts of user data that will be affected by deletion
type DeletionSummary struct {
	ShowsCount      int64 `json:"shows_count"`
	SavedShowsCount int64 `json:"saved_shows_count"`
	PasskeysCount   int64 `json:"passkeys_count"`
}

// GetDeletionSummary returns counts of data that will be affected by account deletion
func (s *UserService) GetDeletionSummary(userID uint) (*DeletionSummary, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	summary := &DeletionSummary{}

	// Count shows submitted by this user
	if err := s.db.Model(&models.Show{}).Where("submitted_by = ?", userID).Count(&summary.ShowsCount).Error; err != nil {
		return nil, fmt.Errorf("failed to count shows: %w", err)
	}

	// Count saved shows
	if err := s.db.Model(&models.UserSavedShow{}).Where("user_id = ?", userID).Count(&summary.SavedShowsCount).Error; err != nil {
		return nil, fmt.Errorf("failed to count saved shows: %w", err)
	}

	// Count passkeys
	if err := s.db.Model(&models.WebAuthnCredential{}).Where("user_id = ?", userID).Count(&summary.PasskeysCount).Error; err != nil {
		return nil, fmt.Errorf("failed to count passkeys: %w", err)
	}

	return summary, nil
}

// SoftDeleteAccount performs a soft delete on the user account
// Sets is_active=false, deleted_at=now, and stores optional deletion reason
func (s *UserService) SoftDeleteAccount(userID uint, reason *string) error {
	if s.db == nil {
		return fmt.Errorf("database not initialized")
	}

	now := time.Now()
	updates := map[string]interface{}{
		"is_active":  false,
		"deleted_at": now,
	}

	if reason != nil && *reason != "" {
		updates["deletion_reason"] = *reason
	}

	result := s.db.Model(&models.User{}).
		Where("id = ?", userID).
		Updates(updates)

	if result.Error != nil {
		return fmt.Errorf("failed to soft delete user: %w", result.Error)
	}

	if result.RowsAffected == 0 {
		return fmt.Errorf("user not found")
	}

	return nil
}

// CreateUserWithoutPassword creates a user account without a password
// (for passkey-only or OAuth-only accounts)
func (s *UserService) CreateUserWithoutPassword(email string) (*models.User, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	existingUser, err := s.GetUserByEmail(email)
	if err != nil {
		return nil, fmt.Errorf("failed to check existing user: %w", err)
	}

	if existingUser != nil {
		return nil, apperrors.ErrUserExists(email)
	}

	tx := s.db.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// Create user without password
	user := &models.User{
		Email:         &email,
		IsActive:      true,
		EmailVerified: false, // Email verification required
	}

	if err := tx.Create(user).Error; err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("failed to create user: %w", err)
	}

	// Create default preferences
	preferences := &models.UserPreferences{
		UserID: user.ID,
	}

	if err := tx.Create(preferences).Error; err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("failed to create user preferences: %w", err)
	}

	if err := tx.Commit().Error; err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	// Load relationships
	if err := s.db.Preload("Preferences").First(user, user.ID).Error; err != nil {
		return nil, fmt.Errorf("failed to load user relationships: %w", err)
	}

	return user, nil
}

// UserDataExport represents all user data in a portable format (GDPR compliance)
type UserDataExport struct {
	ExportedAt     time.Time                `json:"exported_at"`
	ExportVersion  string                   `json:"export_version"`
	Profile        UserProfileExport        `json:"profile"`
	Preferences    *UserPreferencesExport   `json:"preferences,omitempty"`
	OAuthAccounts  []OAuthAccountExport     `json:"oauth_accounts,omitempty"`
	Passkeys       []PasskeyExport          `json:"passkeys,omitempty"`
	SavedShows     []SavedShowExport        `json:"saved_shows,omitempty"`
	SubmittedShows []SubmittedShowExport    `json:"submitted_shows,omitempty"`
}

// UserProfileExport contains user profile data for export
type UserProfileExport struct {
	ID            uint       `json:"id"`
	Email         *string    `json:"email"`
	Username      *string    `json:"username,omitempty"`
	FirstName     *string    `json:"first_name,omitempty"`
	LastName      *string    `json:"last_name,omitempty"`
	AvatarURL     *string    `json:"avatar_url,omitempty"`
	Bio           *string    `json:"bio,omitempty"`
	EmailVerified bool       `json:"email_verified"`
	CreatedAt     time.Time  `json:"account_created_at"`
	UpdatedAt     time.Time  `json:"last_updated_at"`
}

// UserPreferencesExport contains user preferences for export
type UserPreferencesExport struct {
	NotificationEmail bool   `json:"notification_email"`
	NotificationPush  bool   `json:"notification_push"`
	Theme             string `json:"theme"`
	Timezone          string `json:"timezone"`
	Language          string `json:"language"`
}

// OAuthAccountExport contains OAuth account data for export (no tokens)
type OAuthAccountExport struct {
	Provider      string    `json:"provider"`
	ProviderEmail *string   `json:"provider_email,omitempty"`
	ProviderName  *string   `json:"provider_name,omitempty"`
	LinkedAt      time.Time `json:"linked_at"`
}

// PasskeyExport contains passkey metadata for export (no keys)
type PasskeyExport struct {
	DisplayName    string     `json:"display_name"`
	CreatedAt      time.Time  `json:"created_at"`
	LastUsedAt     *time.Time `json:"last_used_at,omitempty"`
	BackupEligible bool       `json:"backup_eligible"`
	BackupState    bool       `json:"backup_state"`
}

// SavedShowExport contains saved show data for export
type SavedShowExport struct {
	ShowID    uint      `json:"show_id"`
	Title     string    `json:"title"`
	EventDate time.Time `json:"event_date"`
	Venue     *string   `json:"venue,omitempty"`
	City      *string   `json:"city,omitempty"`
	SavedAt   time.Time `json:"saved_at"`
}

// SubmittedShowExport contains submitted show data for export
type SubmittedShowExport struct {
	ShowID      uint      `json:"show_id"`
	Title       string    `json:"title"`
	EventDate   time.Time `json:"event_date"`
	Status      string    `json:"status"`
	SubmittedAt time.Time `json:"submitted_at"`
	Venue       *string   `json:"venue,omitempty"`
	City        *string   `json:"city,omitempty"`
	Artists     []string  `json:"artists,omitempty"`
}

// ExportUserData exports all user data in a portable JSON format
// This supports GDPR Article 20 - Right to data portability
func (s *UserService) ExportUserData(userID uint) (*UserDataExport, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	// Get user with all relationships
	var user models.User
	if err := s.db.
		Preload("OAuthAccounts").
		Preload("Preferences").
		Preload("PasskeyCredentials").
		First(&user, userID).Error; err != nil {
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	export := &UserDataExport{
		ExportedAt:    time.Now().UTC(),
		ExportVersion: "1.0",
		Profile: UserProfileExport{
			ID:            user.ID,
			Email:         user.Email,
			Username:      user.Username,
			FirstName:     user.FirstName,
			LastName:      user.LastName,
			AvatarURL:     user.AvatarURL,
			Bio:           user.Bio,
			EmailVerified: user.EmailVerified,
			CreatedAt:     user.CreatedAt,
			UpdatedAt:     user.UpdatedAt,
		},
	}

	// Export preferences
	if user.Preferences != nil {
		export.Preferences = &UserPreferencesExport{
			NotificationEmail: user.Preferences.NotificationEmail,
			NotificationPush:  user.Preferences.NotificationPush,
			Theme:             user.Preferences.Theme,
			Timezone:          user.Preferences.Timezone,
			Language:          user.Preferences.Language,
		}
	}

	// Export OAuth accounts (without tokens)
	for _, oauth := range user.OAuthAccounts {
		export.OAuthAccounts = append(export.OAuthAccounts, OAuthAccountExport{
			Provider:      oauth.Provider,
			ProviderEmail: oauth.ProviderEmail,
			ProviderName:  oauth.ProviderName,
			LinkedAt:      oauth.CreatedAt,
		})
	}

	// Export passkey metadata (no keys)
	for _, passkey := range user.PasskeyCredentials {
		export.Passkeys = append(export.Passkeys, PasskeyExport{
			DisplayName:    passkey.DisplayName,
			CreatedAt:      passkey.CreatedAt,
			LastUsedAt:     passkey.LastUsedAt,
			BackupEligible: passkey.BackupEligible,
			BackupState:    passkey.BackupState,
		})
	}

	// Get saved shows with venue info
	var savedShowRecords []models.UserSavedShow
	if err := s.db.Where("user_id = ?", userID).Find(&savedShowRecords).Error; err != nil {
		return nil, fmt.Errorf("failed to get saved shows: %w", err)
	}

	for _, ss := range savedShowRecords {
		var show models.Show
		if err := s.db.Preload("Venues").First(&show, ss.ShowID).Error; err != nil {
			continue // Skip if show not found
		}

		savedExport := SavedShowExport{
			ShowID:    show.ID,
			Title:     show.Title,
			EventDate: show.EventDate,
			City:      show.City,
			SavedAt:   ss.SavedAt,
		}

		if len(show.Venues) > 0 {
			savedExport.Venue = &show.Venues[0].Name
		}

		export.SavedShows = append(export.SavedShows, savedExport)
	}

	// Export submitted shows with details
	var submittedShows []models.Show
	if err := s.db.
		Preload("Venues").
		Preload("Artists").
		Where("submitted_by = ?", userID).
		Find(&submittedShows).Error; err != nil {
		return nil, fmt.Errorf("failed to get submitted shows: %w", err)
	}

	for _, show := range submittedShows {
		submittedExport := SubmittedShowExport{
			ShowID:      show.ID,
			Title:       show.Title,
			EventDate:   show.EventDate,
			Status:      string(show.Status),
			SubmittedAt: show.CreatedAt,
			City:        show.City,
		}

		if len(show.Venues) > 0 {
			submittedExport.Venue = &show.Venues[0].Name
		}

		for _, artist := range show.Artists {
			submittedExport.Artists = append(submittedExport.Artists, artist.Name)
		}

		export.SubmittedShows = append(export.SubmittedShows, submittedExport)
	}

	return export, nil
}

// ExportUserDataJSON exports user data as a JSON byte slice
func (s *UserService) ExportUserDataJSON(userID uint) ([]byte, error) {
	export, err := s.ExportUserData(userID)
	if err != nil {
		return nil, err
	}

	return json.MarshalIndent(export, "", "  ")
}

// GetOAuthAccounts returns all OAuth accounts linked to a user
func (s *UserService) GetOAuthAccounts(userID uint) ([]models.OAuthAccount, error) {
	var accounts []models.OAuthAccount
	err := s.db.Where("user_id = ?", userID).Find(&accounts).Error
	if err != nil {
		return nil, fmt.Errorf("failed to get OAuth accounts: %w", err)
	}
	return accounts, nil
}

// Account recovery grace period constant
const AccountRecoveryGracePeriod = 30 * 24 * time.Hour // 30 days

// GetUserByEmailIncludingDeleted retrieves a user by email, including soft-deleted accounts
func (s *UserService) GetUserByEmailIncludingDeleted(email string) (*models.User, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	var user models.User

	// Query without filtering by is_active to include soft-deleted accounts
	result := s.db.Where("email = ?", email).
		Preload("OAuthAccounts").
		Preload("Preferences").
		First(&user)

	if result.Error != nil {
		if result.Error == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get user: %w", result.Error)
	}

	return &user, nil
}

// IsAccountRecoverable checks if a soft-deleted account can still be recovered
// Returns true if account is deleted and within the 30-day grace period
func (s *UserService) IsAccountRecoverable(user *models.User) bool {
	if user == nil {
		return false
	}

	// Account must be soft-deleted (not active and has deleted_at set)
	if user.IsActive || user.DeletedAt == nil {
		return false
	}

	// Check if within grace period
	deletedAt := *user.DeletedAt
	gracePeriodEnd := deletedAt.Add(AccountRecoveryGracePeriod)

	return time.Now().Before(gracePeriodEnd)
}

// GetDaysUntilPermanentDeletion returns the number of days until an account is permanently deleted
// Returns 0 if account is not recoverable
func (s *UserService) GetDaysUntilPermanentDeletion(user *models.User) int {
	if user == nil || user.DeletedAt == nil || user.IsActive {
		return 0
	}

	gracePeriodEnd := user.DeletedAt.Add(AccountRecoveryGracePeriod)
	remaining := time.Until(gracePeriodEnd)

	if remaining <= 0 {
		return 0
	}

	// Round up to nearest day
	days := int(remaining.Hours()/24) + 1
	return days
}

// RestoreAccount restores a soft-deleted account
func (s *UserService) RestoreAccount(userID uint) error {
	if s.db == nil {
		return fmt.Errorf("database not initialized")
	}

	result := s.db.Model(&models.User{}).
		Where("id = ?", userID).
		Updates(map[string]interface{}{
			"is_active":       true,
			"deleted_at":      nil,
			"deletion_reason": nil,
		})

	if result.Error != nil {
		return fmt.Errorf("failed to restore account: %w", result.Error)
	}

	if result.RowsAffected == 0 {
		return fmt.Errorf("user not found")
	}

	return nil
}

// GetExpiredDeletedAccounts returns users whose accounts have been soft-deleted
// for longer than the grace period and are ready for permanent deletion
func (s *UserService) GetExpiredDeletedAccounts() ([]models.User, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	cutoffDate := time.Now().Add(-AccountRecoveryGracePeriod)

	var users []models.User
	result := s.db.
		Where("is_active = ? AND deleted_at IS NOT NULL AND deleted_at < ?", false, cutoffDate).
		Find(&users)

	if result.Error != nil {
		return nil, fmt.Errorf("failed to get expired deleted accounts: %w", result.Error)
	}

	return users, nil
}

// PermanentlyDeleteUser hard-deletes a user and all associated data
// This should only be called for accounts that have exceeded the grace period
func (s *UserService) PermanentlyDeleteUser(userID uint) error {
	if s.db == nil {
		return fmt.Errorf("database not initialized")
	}

	return s.db.Transaction(func(tx *gorm.DB) error {
		// Set shows.submitted_by to NULL for shows submitted by this user
		// (this is already handled by ON DELETE SET NULL in the FK, but being explicit)
		if err := tx.Model(&models.Show{}).
			Where("submitted_by = ?", userID).
			Update("submitted_by", nil).Error; err != nil {
			return fmt.Errorf("failed to nullify show submissions: %w", err)
		}

		// Hard delete the user (cascades will handle related data like OAuth accounts,
		// preferences, passkeys, saved shows, favorite venues)
		if err := tx.Unscoped().Delete(&models.User{}, userID).Error; err != nil {
			return fmt.Errorf("failed to permanently delete user: %w", err)
		}

		return nil
	})
}

// CanUnlinkOAuthAccount checks if a user can safely unlink an OAuth account
// Returns (canUnlink, reason, error)
func (s *UserService) CanUnlinkOAuthAccount(userID uint, provider string) (bool, string, error) {
	// Get user
	var user models.User
	if err := s.db.First(&user, userID).Error; err != nil {
		return false, "", fmt.Errorf("failed to get user: %w", err)
	}

	// Check if this OAuth account exists
	var oauthAccount models.OAuthAccount
	err := s.db.Where("user_id = ? AND provider = ?", userID, provider).First(&oauthAccount).Error
	if err != nil {
		return false, "OAuth account not found", nil
	}

	// Count alternative auth methods
	hasPassword := user.PasswordHash != nil && *user.PasswordHash != ""

	// Count other OAuth accounts
	var otherOAuthCount int64
	s.db.Model(&models.OAuthAccount{}).Where("user_id = ? AND provider != ?", userID, provider).Count(&otherOAuthCount)

	// Count passkeys
	var passkeyCount int64
	s.db.Model(&models.WebAuthnCredential{}).Where("user_id = ?", userID).Count(&passkeyCount)

	// User must have at least one other auth method
	if !hasPassword && otherOAuthCount == 0 && passkeyCount == 0 {
		return false, "Cannot unlink - this is your only sign-in method. Add a password or passkey first.", nil
	}

	return true, "", nil
}

// UnlinkOAuthAccount removes an OAuth account from a user
func (s *UserService) UnlinkOAuthAccount(userID uint, provider string) error {
	result := s.db.Where("user_id = ? AND provider = ?", userID, provider).Delete(&models.OAuthAccount{})
	if result.Error != nil {
		return fmt.Errorf("failed to unlink OAuth account: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("OAuth account not found")
	}
	return nil
}
