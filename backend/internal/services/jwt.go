package services

import (
	"fmt"
	"time"

	"psychic-homily-backend/internal/config"
	"psychic-homily-backend/internal/models"

	"github.com/golang-jwt/jwt/v5"
)

type JWTService struct {
	config      *config.Config
	userService *UserService
}

func NewJWTService(cfg *config.Config) *JWTService {
	return &JWTService{
		config:      cfg,
		userService: NewUserService(),
	}
}

// CreateToken generates a JWT for a user
func (s *JWTService) CreateToken(user *models.User) (string, error) {
	claims := jwt.MapClaims{
		"user_id": user.ID,
		"email":   user.Email,
		"exp":     time.Now().Add(time.Duration(s.config.JWT.Expiry) * time.Hour).Unix(),
		"iat":     time.Now().Unix(),
		"iss":     "psychic-homily-backend",
		"aud":     "psychic-homily-users",
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(s.config.JWT.SecretKey))
}

// ValidateToken validates and extracts user info from JWT
// Fetches the full user from the database to ensure we have current admin status
func (s *JWTService) ValidateToken(tokenString string) (*models.User, error) {
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(s.config.JWT.SecretKey), nil
	})

	if err != nil {
		return nil, fmt.Errorf("invalid token: %w", err)
	}

	if claims, ok := token.Claims.(jwt.MapClaims); ok && token.Valid {
		userID := uint(claims["user_id"].(float64))
		
		// Fetch full user from database to get current admin status and other fields
		// This ensures we always have the most up-to-date user information
		user, err := s.userService.GetUserByID(userID)
		if err != nil {
			return nil, fmt.Errorf("failed to get user: %w", err)
		}

		return user, nil
	}

	return nil, fmt.Errorf("invalid token claims")
}

// RefreshToken creates a new token with extended expiry
func (s *JWTService) RefreshToken(tokenString string) (string, error) {
	user, err := s.ValidateToken(tokenString)
	if err != nil {
		return "", err
	}

	return s.CreateToken(user)
}

// VerificationTokenClaims holds the claims for email verification tokens
type VerificationTokenClaims struct {
	UserID uint   `json:"user_id"`
	Email  string `json:"email"`
	jwt.RegisteredClaims
}

// CreateVerificationToken generates a JWT token for email verification
// Token expires in 24 hours
func (s *JWTService) CreateVerificationToken(userID uint, email string) (string, error) {
	claims := VerificationTokenClaims{
		UserID: userID,
		Email:  email,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Issuer:    "psychic-homily-backend",
			Subject:   "email-verification",
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(s.config.JWT.SecretKey))
}

// ValidateVerificationToken validates an email verification token and returns the claims
func (s *JWTService) ValidateVerificationToken(tokenString string) (*VerificationTokenClaims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &VerificationTokenClaims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(s.config.JWT.SecretKey), nil
	})

	if err != nil {
		return nil, fmt.Errorf("invalid verification token: %w", err)
	}

	claims, ok := token.Claims.(*VerificationTokenClaims)
	if !ok || !token.Valid {
		return nil, fmt.Errorf("invalid verification token claims")
	}

	// Verify the token is for email verification
	if claims.Subject != "email-verification" {
		return nil, fmt.Errorf("invalid token type")
	}

	return claims, nil
}
