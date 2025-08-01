package services

import (
	"fmt"

	"github.com/markbates/goth"
	"gorm.io/gorm"

	"psychic-homily-backend/db"
	"psychic-homily-backend/internal/models"
)

// UserService handles user-related business logic
type UserService struct {
	db *gorm.DB
}

// NewUserService creates a new user service
func NewUserService() *UserService {
	return &UserService{
		db: db.GetDB(),
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
	return s.createNewUser(gothUser, provider)
}

// createNewUser creates a new user with OAuth account
func (s *UserService) createNewUser(gothUser goth.User, provider string) (*models.User, error) {
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
