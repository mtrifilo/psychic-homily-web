package services

import (
	"fmt"
	"time"

	"psychic-homily-backend/internal/config"
	"psychic-homily-backend/internal/models"

	"github.com/golang-jwt/jwt/v5"
)

type JWTService struct {
	config *config.Config
}

func NewJWTService(cfg *config.Config) *JWTService {
	return &JWTService{config: cfg}
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
		
		// Handle nil email case
		var email *string
		if claims["email"] != nil {
			emailStr := claims["email"].(string)
			email = &emailStr
		}

		return &models.User{
			ID:    userID,
			Email: email,
		}, nil
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
