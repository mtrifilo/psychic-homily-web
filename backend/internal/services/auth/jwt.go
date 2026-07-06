package auth

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"psychic-homily-backend/internal/config"
	apperrors "psychic-homily-backend/internal/errors"
	authm "psychic-homily-backend/internal/models/auth"
	"psychic-homily-backend/internal/services/contracts"
)

// Session JWT claim values. The issuer and audience are shared by every token
// this service mints; the subject distinguishes a full session token from the
// short-lived single-purpose tokens (email-verification, magic-link,
// account-recovery). ValidateToken asserts all three so a leaked short-lived
// token — which travels through email URLs, query strings, and browser history
// — can never be replayed as a session credential.
const (
	jwtIssuer         = "psychic-homily-backend"
	jwtAudience       = "psychic-homily-users"
	jwtSessionSubject = "session"
)

type JWTService struct {
	config      *config.Config
	userService contracts.UserServiceInterface
}

func NewJWTService(database interface{}, cfg *config.Config, userService contracts.UserServiceInterface) *JWTService {
	return &JWTService{
		config:      cfg,
		userService: userService,
	}
}

// CreateToken generates a JWT for a user
func (s *JWTService) CreateToken(user *authm.User) (string, error) {
	claims := jwt.MapClaims{
		"user_id": user.ID,
		"email":   user.Email,
		"exp":     time.Now().Add(time.Duration(s.config.JWT.Expiry) * time.Hour).Unix(),
		"iat":     time.Now().Unix(),
		"iss":     jwtIssuer,
		"aud":     jwtAudience,
		"sub":     jwtSessionSubject,
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(s.config.JWT.SecretKey))
}

// parseSessionToken parses and cryptographically verifies a session JWT —
// signing method (HMAC), signature (secret), issuer, audience, subject, and
// expiry — WITHOUT any database lookup, returning the validated claims.
//
// Enforce the session subject, issuer, and audience as part of parsing.
// WithSubject/WithIssuer/WithAudience require the claim to exist and match, so
// single-purpose tokens (email-verification, magic-link, account-recovery) and
// legacy session tokens minted before the subject was added are rejected here
// rather than being honored as session credentials.
func (s *JWTService) parseSessionToken(tokenString string) (jwt.MapClaims, error) {
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(s.config.JWT.SecretKey), nil
	}, jwt.WithIssuer(jwtIssuer), jwt.WithAudience(jwtAudience), jwt.WithSubject(jwtSessionSubject))

	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, apperrors.ErrTokenExpired(err)
		}
		return nil, apperrors.ErrTokenInvalid(err)
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok || !token.Valid {
		return nil, apperrors.ErrTokenInvalid(nil)
	}
	return claims, nil
}

// SessionUserID returns the user id from a validly-signed, unexpired session
// token — WITHOUT loading the user from the database. ok is false for any
// invalid/expired/forged token or a missing/non-numeric user_id claim.
//
// Used by the public-read rate limiter (middleware.RateLimitPublicReadsByAuthState,
// PSY-1373) to (a) tell anonymous traffic from logged-in users and (b) key the
// authenticated per-user rate bucket by id — on EVERY request, so a per-request DB
// query would be wasteful. Deliberately does NOT check IsActive/admin: for
// metering read abuse, a validly-signed session token identifies a real account,
// which is all the per-user cap needs.
func (s *JWTService) SessionUserID(tokenString string) (uint, bool) {
	claims, err := s.parseSessionToken(tokenString)
	if err != nil {
		return 0, false
	}
	uid, ok := claims["user_id"].(float64)
	if !ok || uid < 0 {
		return 0, false
	}
	return uint(uid), true
}

// ValidateToken validates and extracts user info from JWT
// Fetches the full user from the database to ensure we have current admin status
func (s *JWTService) ValidateToken(tokenString string) (*authm.User, error) {
	claims, err := s.parseSessionToken(tokenString)
	if err != nil {
		return nil, err
	}

	userID := uint(claims["user_id"].(float64))

	// Fetch full user from database to get current admin status and other fields
	// This ensures we always have the most up-to-date user information
	user, err := s.userService.GetUserByID(userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	if !user.IsActive {
		return nil, apperrors.ErrTokenInvalid(fmt.Errorf("user account is not active"))
	}

	return user, nil
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
func (s *JWTService) ValidateTokenLenient(tokenString string, gracePeriod time.Duration) (*authm.User, error) {
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

	// Validate subject, issuer, and audience manually. The without-validation
	// parse above skips claim checks, so we must assert the session subject here
	// too — otherwise a leaked single-purpose token (magic-link, verification,
	// recovery) caught inside the grace window could be refreshed into a session.
	sub, _ := claims["sub"].(string)
	iss, _ := claims["iss"].(string)
	aud, _ := claims["aud"].(string)
	if sub != jwtSessionSubject || iss != jwtIssuer || aud != jwtAudience {
		return nil, apperrors.ErrTokenInvalid(fmt.Errorf("invalid token subject, issuer, or audience"))
	}

	// Token is within grace period — extract user
	userID := uint(claims["user_id"].(float64))
	user, userErr := s.userService.GetUserByID(userID)
	if userErr != nil {
		return nil, fmt.Errorf("failed to get user: %w", userErr)
	}

	if !user.IsActive {
		return nil, apperrors.ErrTokenInvalid(fmt.Errorf("user account is not active"))
	}

	return user, nil
}

// CreateVerificationToken generates a JWT token for email verification
// Token expires in 24 hours
func (s *JWTService) CreateVerificationToken(userID uint, email string) (string, error) {
	claims := contracts.VerificationTokenClaims{
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
func (s *JWTService) ValidateVerificationToken(tokenString string) (*contracts.VerificationTokenClaims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &contracts.VerificationTokenClaims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(s.config.JWT.SecretKey), nil
	})

	if err != nil {
		return nil, fmt.Errorf("invalid verification token: %w", err)
	}

	claims, ok := token.Claims.(*contracts.VerificationTokenClaims)
	if !ok || !token.Valid {
		return nil, fmt.Errorf("invalid verification token claims")
	}

	// Verify the token is for email verification
	if claims.Subject != "email-verification" {
		return nil, fmt.Errorf("invalid token type")
	}

	return claims, nil
}

// CreateMagicLinkToken generates a JWT token for magic link login
// Token expires in 15 minutes for security
func (s *JWTService) CreateMagicLinkToken(userID uint, email string) (string, error) {
	claims := contracts.MagicLinkTokenClaims{
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
func (s *JWTService) ValidateMagicLinkToken(tokenString string) (*contracts.MagicLinkTokenClaims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &contracts.MagicLinkTokenClaims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(s.config.JWT.SecretKey), nil
	})

	if err != nil {
		return nil, fmt.Errorf("invalid magic link token: %w", err)
	}

	claims, ok := token.Claims.(*contracts.MagicLinkTokenClaims)
	if !ok || !token.Valid {
		return nil, fmt.Errorf("invalid magic link token claims")
	}

	// Verify the token is for magic link
	if claims.Subject != "magic-link" {
		return nil, fmt.Errorf("invalid token type")
	}

	return claims, nil
}

// CreateAccountRecoveryToken generates a JWT token for account recovery
// Token expires in 1 hour for security
func (s *JWTService) CreateAccountRecoveryToken(userID uint, email string) (string, error) {
	claims := contracts.AccountRecoveryTokenClaims{
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
func (s *JWTService) ValidateAccountRecoveryToken(tokenString string) (*contracts.AccountRecoveryTokenClaims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &contracts.AccountRecoveryTokenClaims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(s.config.JWT.SecretKey), nil
	})

	if err != nil {
		return nil, fmt.Errorf("invalid account recovery token: %w", err)
	}

	claims, ok := token.Claims.(*contracts.AccountRecoveryTokenClaims)
	if !ok || !token.Valid {
		return nil, fmt.Errorf("invalid account recovery token claims")
	}

	// Verify the token is for account recovery
	if claims.Subject != "account-recovery" {
		return nil, fmt.Errorf("invalid token type")
	}

	return claims, nil
}
