package services

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"gorm.io/gorm"

	"psychic-homily-backend/db"
	"psychic-homily-backend/internal/models"
)

const (
	// DefaultTokenExpirationDays is the default token lifetime in days
	DefaultTokenExpirationDays = 90
	// TokenLength is the length of the generated token in bytes (32 bytes = 64 hex chars)
	TokenLength = 32
	// TokenPrefix is prepended to tokens for easy identification
	TokenPrefix = "phk_" // "psychic homily key"
)

// APITokenService handles API token operations
type APITokenService struct {
	db *gorm.DB
}

// NewAPITokenService creates a new API token service
func NewAPITokenService(database *gorm.DB) *APITokenService {
	if database == nil {
		database = db.GetDB()
	}
	return &APITokenService{
		db: database,
	}
}

// APITokenResponse represents a token in API responses
type APITokenResponse struct {
	ID          uint       `json:"id"`
	Description *string    `json:"description"`
	Scope       string     `json:"scope"`
	CreatedAt   time.Time  `json:"created_at"`
	ExpiresAt   time.Time  `json:"expires_at"`
	LastUsedAt  *time.Time `json:"last_used_at"`
	IsExpired   bool       `json:"is_expired"`
}

// APITokenCreateResponse includes the plaintext token (only returned on creation)
type APITokenCreateResponse struct {
	ID          uint       `json:"id"`
	Token       string     `json:"token"` // Plaintext token - only shown once!
	Description *string    `json:"description"`
	Scope       string     `json:"scope"`
	CreatedAt   time.Time  `json:"created_at"`
	ExpiresAt   time.Time  `json:"expires_at"`
}

// generateToken creates a cryptographically secure random token
func generateToken() (string, error) {
	bytes := make([]byte, TokenLength)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("failed to generate random bytes: %w", err)
	}
	return TokenPrefix + hex.EncodeToString(bytes), nil
}

// hashToken creates a SHA-256 hash of a token for storage
func hashToken(token string) string {
	hash := sha256.Sum256([]byte(token))
	return hex.EncodeToString(hash[:])
}

// CreateToken generates a new API token for a user
func (s *APITokenService) CreateToken(userID uint, description *string, expirationDays int) (*APITokenCreateResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	if expirationDays <= 0 {
		expirationDays = DefaultTokenExpirationDays
	}

	// Generate the plaintext token
	plainToken, err := generateToken()
	if err != nil {
		return nil, fmt.Errorf("failed to generate token: %w", err)
	}

	// Hash the token for storage
	tokenHash := hashToken(plainToken)

	// Create the token record
	token := &models.APIToken{
		UserID:      userID,
		TokenHash:   tokenHash,
		Description: description,
		Scope:       "admin",
		ExpiresAt:   time.Now().Add(time.Duration(expirationDays) * 24 * time.Hour),
	}

	if err := s.db.Create(token).Error; err != nil {
		return nil, fmt.Errorf("failed to create token: %w", err)
	}

	return &APITokenCreateResponse{
		ID:          token.ID,
		Token:       plainToken, // Return plaintext only this once
		Description: token.Description,
		Scope:       token.Scope,
		CreatedAt:   token.CreatedAt,
		ExpiresAt:   token.ExpiresAt,
	}, nil
}

// ValidateToken checks if a token is valid and returns the associated user
func (s *APITokenService) ValidateToken(plainToken string) (*models.User, *models.APIToken, error) {
	if s.db == nil {
		return nil, nil, fmt.Errorf("database not initialized")
	}

	// Hash the provided token
	tokenHash := hashToken(plainToken)

	// Look up the token
	var token models.APIToken
	err := s.db.Preload("User").Where("token_hash = ?", tokenHash).First(&token).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil, fmt.Errorf("invalid token")
		}
		return nil, nil, fmt.Errorf("failed to validate token: %w", err)
	}

	// Check if token is valid
	if !token.IsValid() {
		if token.IsRevoked() {
			return nil, nil, fmt.Errorf("token has been revoked")
		}
		if token.IsExpired() {
			return nil, nil, fmt.Errorf("token has expired")
		}
		return nil, nil, fmt.Errorf("invalid token")
	}

	// Check if user is active and admin
	if !token.User.IsActive {
		return nil, nil, fmt.Errorf("user account is not active")
	}

	if token.Scope == "admin" && !token.User.IsAdmin {
		return nil, nil, fmt.Errorf("user is not an admin")
	}

	// Update last used timestamp (async to not slow down requests)
	go func() {
		s.db.Model(&token).Update("last_used_at", time.Now())
	}()

	return &token.User, &token, nil
}

// ListTokens returns all tokens for a user (without hashes)
func (s *APITokenService) ListTokens(userID uint) ([]APITokenResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	var tokens []models.APIToken
	err := s.db.Where("user_id = ? AND revoked_at IS NULL", userID).
		Order("created_at DESC").
		Find(&tokens).Error
	if err != nil {
		return nil, fmt.Errorf("failed to list tokens: %w", err)
	}

	responses := make([]APITokenResponse, len(tokens))
	for i, token := range tokens {
		responses[i] = APITokenResponse{
			ID:          token.ID,
			Description: token.Description,
			Scope:       token.Scope,
			CreatedAt:   token.CreatedAt,
			ExpiresAt:   token.ExpiresAt,
			LastUsedAt:  token.LastUsedAt,
			IsExpired:   token.IsExpired(),
		}
	}

	return responses, nil
}

// RevokeToken revokes a token by ID (must belong to the user)
func (s *APITokenService) RevokeToken(userID uint, tokenID uint) error {
	if s.db == nil {
		return fmt.Errorf("database not initialized")
	}

	now := time.Now()
	result := s.db.Model(&models.APIToken{}).
		Where("id = ? AND user_id = ? AND revoked_at IS NULL", tokenID, userID).
		Update("revoked_at", now)

	if result.Error != nil {
		return fmt.Errorf("failed to revoke token: %w", result.Error)
	}

	if result.RowsAffected == 0 {
		return fmt.Errorf("token not found or already revoked")
	}

	return nil
}

// GetToken retrieves a single token by ID (must belong to the user)
func (s *APITokenService) GetToken(userID uint, tokenID uint) (*APITokenResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	var token models.APIToken
	err := s.db.Where("id = ? AND user_id = ?", tokenID, userID).First(&token).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("token not found")
		}
		return nil, fmt.Errorf("failed to get token: %w", err)
	}

	return &APITokenResponse{
		ID:          token.ID,
		Description: token.Description,
		Scope:       token.Scope,
		CreatedAt:   token.CreatedAt,
		ExpiresAt:   token.ExpiresAt,
		LastUsedAt:  token.LastUsedAt,
		IsExpired:   token.IsExpired(),
	}, nil
}

// CleanupExpiredTokens removes tokens that have been expired or revoked for over 30 days
// This is meant to be called by a scheduled job
func (s *APITokenService) CleanupExpiredTokens() (int64, error) {
	if s.db == nil {
		return 0, fmt.Errorf("database not initialized")
	}

	cutoff := time.Now().Add(-30 * 24 * time.Hour)

	result := s.db.Where(
		"(expires_at < ? AND expires_at < ?) OR (revoked_at IS NOT NULL AND revoked_at < ?)",
		time.Now(), cutoff, cutoff,
	).Delete(&models.APIToken{})

	if result.Error != nil {
		return 0, fmt.Errorf("failed to cleanup expired tokens: %w", result.Error)
	}

	return result.RowsAffected, nil
}
