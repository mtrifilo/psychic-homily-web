package services

import (
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	"psychic-homily-backend/internal/config"
	apperrors "psychic-homily-backend/internal/errors"
	"psychic-homily-backend/internal/models"

	"github.com/golang-jwt/jwt/v5"
)

type JWTService struct {
	config      *config.Config
	userService *UserService
}

func NewJWTService(database *gorm.DB, cfg *config.Config) *JWTService {
	return &JWTService{
		config:      cfg,
		userService: NewUserService(database),
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
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, apperrors.ErrTokenExpired(err)
		}
		return nil, apperrors.ErrTokenInvalid(err)
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

	return nil, apperrors.ErrTokenInvalid(nil)
}

// RefreshToken creates a new token with extended expiry
func (s *JWTService) RefreshToken(tokenString string) (string, error) {
	user, err := s.ValidateToken(tokenString)
	if err != nil {
		return "", err
	}

	return s.CreateToken(user)
}

// ValidateTokenLenient validates a JWT but allows tokens that expired within a grace period.
// This is used for token refresh — the client sends an expired token to get a new one.
// The grace period prevents forcing re-login when the token expired recently.
func (s *JWTService) ValidateTokenLenient(tokenString string, gracePeriod time.Duration) (*models.User, error) {
	// First try strict validation (covers the case where token is still valid)
	user, err := s.ValidateToken(tokenString)
	if err == nil {
		return user, nil
	}

	// If strict validation failed, try parsing without expiration validation
	parser := jwt.NewParser(jwt.WithoutClaimsValidation())
	token, parseErr := parser.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(s.config.JWT.SecretKey), nil
	})

	if parseErr != nil {
		return nil, apperrors.ErrTokenInvalid(parseErr)
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return nil, apperrors.ErrTokenInvalid(nil)
	}

	// Manually check expiration with grace period
	exp, ok := claims["exp"].(float64)
	if !ok {
		return nil, apperrors.ErrTokenInvalid(fmt.Errorf("missing expiration claim"))
	}

	expTime := time.Unix(int64(exp), 0)
	if time.Since(expTime) > gracePeriod {
		return nil, apperrors.ErrTokenExpired(fmt.Errorf("token expired beyond grace period"))
	}

	// Validate issuer and audience manually
	iss, _ := claims["iss"].(string)
	aud, _ := claims["aud"].(string)
	if iss != "psychic-homily-backend" || aud != "psychic-homily-users" {
		return nil, apperrors.ErrTokenInvalid(fmt.Errorf("invalid token issuer or audience"))
	}

	// Token is within grace period — extract user
	userID := uint(claims["user_id"].(float64))
	user, userErr := s.userService.GetUserByID(userID)
	if userErr != nil {
		return nil, fmt.Errorf("failed to get user: %w", userErr)
	}

	return user, nil
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

// MagicLinkTokenClaims holds the claims for magic link tokens
type MagicLinkTokenClaims struct {
	UserID uint   `json:"user_id"`
	Email  string `json:"email"`
	jwt.RegisteredClaims
}

// CreateMagicLinkToken generates a JWT token for magic link login
// Token expires in 15 minutes for security
func (s *JWTService) CreateMagicLinkToken(userID uint, email string) (string, error) {
	claims := MagicLinkTokenClaims{
		UserID: userID,
		Email:  email,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(15 * time.Minute)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Issuer:    "psychic-homily-backend",
			Subject:   "magic-link",
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(s.config.JWT.SecretKey))
}

// ValidateMagicLinkToken validates a magic link token and returns the claims
func (s *JWTService) ValidateMagicLinkToken(tokenString string) (*MagicLinkTokenClaims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &MagicLinkTokenClaims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(s.config.JWT.SecretKey), nil
	})

	if err != nil {
		return nil, fmt.Errorf("invalid magic link token: %w", err)
	}

	claims, ok := token.Claims.(*MagicLinkTokenClaims)
	if !ok || !token.Valid {
		return nil, fmt.Errorf("invalid magic link token claims")
	}

	// Verify the token is for magic link
	if claims.Subject != "magic-link" {
		return nil, fmt.Errorf("invalid token type")
	}

	return claims, nil
}

// AccountRecoveryTokenClaims holds the claims for account recovery tokens
type AccountRecoveryTokenClaims struct {
	UserID uint   `json:"user_id"`
	Email  string `json:"email"`
	jwt.RegisteredClaims
}

// CreateAccountRecoveryToken generates a JWT token for account recovery
// Token expires in 1 hour for security
func (s *JWTService) CreateAccountRecoveryToken(userID uint, email string) (string, error) {
	claims := AccountRecoveryTokenClaims{
		UserID: userID,
		Email:  email,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(1 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Issuer:    "psychic-homily-backend",
			Subject:   "account-recovery",
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(s.config.JWT.SecretKey))
}

// ValidateAccountRecoveryToken validates an account recovery token and returns the claims
func (s *JWTService) ValidateAccountRecoveryToken(tokenString string) (*AccountRecoveryTokenClaims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &AccountRecoveryTokenClaims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(s.config.JWT.SecretKey), nil
	})

	if err != nil {
		return nil, fmt.Errorf("invalid account recovery token: %w", err)
	}

	claims, ok := token.Claims.(*AccountRecoveryTokenClaims)
	if !ok || !token.Valid {
		return nil, fmt.Errorf("invalid account recovery token claims")
	}

	// Verify the token is for account recovery
	if claims.Subject != "account-recovery" {
		return nil, fmt.Errorf("invalid token type")
	}

	return claims, nil
}
