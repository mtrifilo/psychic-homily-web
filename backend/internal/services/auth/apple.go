package auth

import (
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"gorm.io/gorm"

	"psychic-homily-backend/db"
	"psychic-homily-backend/internal/config"
	apperrors "psychic-homily-backend/internal/errors"
	"psychic-homily-backend/internal/logger"
	authm "psychic-homily-backend/internal/models/auth"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/services/shared"
)

const (
	appleIssuer  = "https://appleid.apple.com"
	appleKeysURL = "https://appleid.apple.com/auth/keys"
)

// AppleAuthService handles Sign in with Apple authentication
type AppleAuthService struct {
	db         *gorm.DB
	config     *config.Config
	jwtService *JWTService

	// Cached Apple public keys
	keysMu    sync.RWMutex
	keys      map[string]*rsa.PublicKey
	keysExpAt time.Time

	// fetchAppleKeysFromURL overrides appleKeysURL for testing
	fetchAppleKeysFromURL string
}

// NewAppleAuthService creates a new Apple auth service
func NewAppleAuthService(database *gorm.DB, cfg *config.Config, jwtService *JWTService) *AppleAuthService {
	if database == nil {
		database = db.GetDB()
	}
	return &AppleAuthService{
		db:         database,
		config:     cfg,
		jwtService: jwtService,
		keys:       make(map[string]*rsa.PublicKey),
	}
}

// appleJWKSet represents Apple's JWK set response
type appleJWKSet struct {
	Keys []appleJWK `json:"keys"`
}

type appleJWK struct {
	Kty string `json:"kty"`
	Kid string `json:"kid"`
	Use string `json:"use"`
	Alg string `json:"alg"`
	N   string `json:"n"`
	E   string `json:"e"`
}

// ValidateIdentityToken validates an Apple identity token and returns the claims
func (s *AppleAuthService) ValidateIdentityToken(identityToken string) (*contracts.AppleIdentityTokenClaims, error) {
	// Parse the token header to get the key ID
	parser := jwt.NewParser(jwt.WithoutClaimsValidation())
	unverifiedToken, _, err := parser.ParseUnverified(identityToken, &contracts.AppleIdentityTokenClaims{})
	if err != nil {
		return nil, fmt.Errorf("failed to parse token header: %w", err)
	}

	kid, ok := unverifiedToken.Header["kid"].(string)
	if !ok {
		return nil, fmt.Errorf("missing kid in token header")
	}

	// Get the Apple public key for this kid
	publicKey, err := s.getApplePublicKey(kid)
	if err != nil {
		return nil, fmt.Errorf("failed to get Apple public key: %w", err)
	}

	// Parse and validate the token with the public key
	claims := &contracts.AppleIdentityTokenClaims{}
	token, err := jwt.ParseWithClaims(identityToken, claims, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodRSA); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return publicKey, nil
	},
		jwt.WithIssuer(appleIssuer),
		jwt.WithAudience(s.config.Apple.BundleID),
	)

	if err != nil {
		return nil, fmt.Errorf("invalid Apple identity token: %w", err)
	}

	if !token.Valid {
		return nil, fmt.Errorf("apple identity token is not valid")
	}

	return claims, nil
}

// FindOrCreateAppleUser finds or creates a user from Apple Sign In data.
// claims must already be verified; the caller is responsible for that.
//
// Returns a typed *apperrors.AuthError with CodeUserExists when the address
// already belongs to an account and the claims do not assert Apple verified
// it. Callers that render a refusal differently from a fault must
// discriminate on that code.
func (s *AppleAuthService) FindOrCreateAppleUser(claims *contracts.AppleIdentityTokenClaims, firstName, lastName string) (*authm.User, error) {
	appleUserID := claims.Subject

	// The subject is the identity this whole function resolves on, and
	// provider_user_id permits the empty string. Without this, a token with no
	// subject would match any row stored with an empty one and sign in as its
	// owner, ahead of every check below.
	if appleUserID == "" {
		return nil, fmt.Errorf("apple identity token has no subject")
	}

	// Look for existing OAuth account with provider=apple
	var oauthAccount authm.OAuthAccount
	result := s.db.
		Where("provider = ? AND provider_user_id = ?", "apple", appleUserID).
		First(&oauthAccount)

	if result.Error == nil {
		// Existing Apple user — fetch and return
		var user authm.User
		if err := s.db.Preload("OAuthAccounts").Preload("Preferences").First(&user, oauthAccount.UserID).Error; err != nil {
			return nil, fmt.Errorf("failed to get user: %w", err)
		}
		return &user, nil
	}

	if !errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("database error: %w", result.Error)
	}

	// No existing Apple account. Check if a user exists with the same email.
	if claims.Email != "" {
		var existingUser authm.User
		err := s.db.Where(authm.EmailIdentityWhere, claims.Email).First(&existingUser).Error
		switch {
		case err == nil:
			// The address is the only thing tying this Apple identity to an
			// account that already exists, so Apple has to assert it verified
			// the address.
			if !claims.IsEmailVerified() {
				logger.Default().Warn("oauth_link_refused_unverified_email",
					"provider", "apple",
					"email_hash", logger.HashEmail(claims.Email))
				return nil, apperrors.ErrUserExists(claims.Email)
			}
			// Link Apple account to existing user
			return s.linkAppleAccount(&existingUser, appleUserID, claims.Email)
		case !errors.Is(err, gorm.ErrRecordNotFound):
			// A failed lookup is not proof that no account holds the address,
			// so it must not fall through to creating one.
			return nil, fmt.Errorf("database error: %w", err)
		}
	}

	// Create a new user
	return s.createAppleUser(appleUserID, claims.Email, firstName, lastName)
}

// GenerateToken creates a JWT for the user
func (s *AppleAuthService) GenerateToken(user *authm.User) (string, error) {
	return s.jwtService.CreateToken(user)
}

// linkAppleAccount links an Apple OAuth account to an existing user
func (s *AppleAuthService) linkAppleAccount(user *authm.User, appleUserID, email string) (*authm.User, error) {
	oauthAccount := &authm.OAuthAccount{
		UserID:         user.ID,
		Provider:       "apple",
		ProviderUserID: appleUserID,
		ProviderEmail:  &email,
	}

	if err := s.db.Create(oauthAccount).Error; err != nil {
		return nil, fmt.Errorf("failed to link Apple account: %w", err)
	}

	// Reload user with relationships
	var updatedUser authm.User
	if err := s.db.Preload("OAuthAccounts").Preload("Preferences").First(&updatedUser, user.ID).Error; err != nil {
		return nil, fmt.Errorf("failed to reload user: %w", err)
	}

	return &updatedUser, nil
}

// createAppleUser creates a new user from Apple Sign In data
func (s *AppleAuthService) createAppleUser(appleUserID, email, firstName, lastName string) (*authm.User, error) {
	tx := s.db.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	user := &authm.User{
		FirstName: &firstName,
		LastName:  &lastName,
		IsActive:  true,
		// Stamped for every Apple-created account. This path is reached with an
		// unverified claim too, so the column records verification the token
		// did not assert.
		EmailVerified: true,
	}
	if email != "" {
		user.Email = &email
	}

	if err := tx.Create(user).Error; err != nil {
		tx.Rollback()
		if dup := shared.UserExistsIfDuplicate(email, err); dup != nil {
			return nil, dup
		}
		return nil, fmt.Errorf("failed to create user: %w", err)
	}

	oauthAccount := &authm.OAuthAccount{
		UserID:         user.ID,
		Provider:       "apple",
		ProviderUserID: appleUserID,
	}
	if email != "" {
		oauthAccount.ProviderEmail = &email
	}

	if err := tx.Create(oauthAccount).Error; err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("failed to create OAuth account: %w", err)
	}

	preferences := &authm.UserPreferences{
		UserID: user.ID,
	}
	if err := tx.Create(preferences).Error; err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("failed to create user preferences: %w", err)
	}

	if err := tx.Commit().Error; err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	// Reload with relationships
	var createdUser authm.User
	if err := s.db.Preload("OAuthAccounts").Preload("Preferences").First(&createdUser, user.ID).Error; err != nil {
		return nil, fmt.Errorf("failed to reload user: %w", err)
	}

	return &createdUser, nil
}

// getApplePublicKey fetches and caches Apple's public keys, returning the key for the given kid
func (s *AppleAuthService) getApplePublicKey(kid string) (*rsa.PublicKey, error) {
	// Check cache first
	s.keysMu.RLock()
	if time.Now().Before(s.keysExpAt) {
		if key, ok := s.keys[kid]; ok {
			s.keysMu.RUnlock()
			return key, nil
		}
	}
	s.keysMu.RUnlock()

	// Fetch fresh keys
	if err := s.fetchAppleKeys(); err != nil {
		return nil, err
	}

	s.keysMu.RLock()
	defer s.keysMu.RUnlock()

	key, ok := s.keys[kid]
	if !ok {
		return nil, fmt.Errorf("apple public key not found for kid: %s", kid)
	}

	return key, nil
}

// fetchAppleKeys downloads Apple's JWK set and parses it into RSA public keys
func (s *AppleAuthService) fetchAppleKeys() error {
	keysURL := appleKeysURL
	if s.fetchAppleKeysFromURL != "" {
		keysURL = s.fetchAppleKeysFromURL
	}
	resp, err := http.Get(keysURL)
	if err != nil {
		return fmt.Errorf("failed to fetch Apple keys: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck // deferred Close; nothing actionable on failure

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("apple keys endpoint returned status %d", resp.StatusCode)
	}

	var jwkSet appleJWKSet
	if err := json.NewDecoder(resp.Body).Decode(&jwkSet); err != nil {
		return fmt.Errorf("failed to decode Apple JWK set: %w", err)
	}

	keys := make(map[string]*rsa.PublicKey)
	for _, jwk := range jwkSet.Keys {
		if jwk.Kty != "RSA" {
			continue
		}

		nBytes, err := base64.RawURLEncoding.DecodeString(jwk.N)
		if err != nil {
			return fmt.Errorf("failed to decode modulus for kid %s: %w", jwk.Kid, err)
		}

		eBytes, err := base64.RawURLEncoding.DecodeString(jwk.E)
		if err != nil {
			return fmt.Errorf("failed to decode exponent for kid %s: %w", jwk.Kid, err)
		}

		n := new(big.Int).SetBytes(nBytes)
		e := new(big.Int).SetBytes(eBytes)

		keys[jwk.Kid] = &rsa.PublicKey{
			N: n,
			E: int(e.Int64()),
		}
	}

	s.keysMu.Lock()
	s.keys = keys
	s.keysExpAt = time.Now().Add(24 * time.Hour) // Cache for 24 hours
	s.keysMu.Unlock()

	return nil
}
