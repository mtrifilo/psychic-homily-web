package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"

	apperrors "psychic-homily-backend/internal/errors"
	"psychic-homily-backend/internal/logger"
	authm "psychic-homily-backend/internal/models/auth"
	"psychic-homily-backend/internal/services/contracts"
)

// A pending sign-up token is "<id>.<mac>": id is 32 random bytes in hex, and
// mac is an HMAC-SHA256 under the JWT secret over oauthPendingSignupDomain
// and id. The row is keyed by the SHA-256 of id, so neither a read of the
// table nor a guessed id yields a token, and a token whose MAC fails is
// refused before any query.
const (
	oauthPendingSignupIDBytes = 32
	oauthPendingSignupDomain  = "oauthpendingsignup|"
)

var (
	errPendingSignupMalformed = errors.New("pending sign-up token is malformed or its signature fails")
	errPendingSignupNotFound  = errors.New("pending sign-up not found")
	errPendingSignupExpired   = errors.New("pending sign-up expired")
)

func (s *AuthService) pendingSignupSecret() string {
	if s.config == nil {
		return ""
	}
	return s.config.JWT.SecretKey
}

func signPendingSignupID(secret, id string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(oauthPendingSignupDomain))
	mac.Write([]byte(id))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func pendingSignupTokenHash(id string) string {
	sum := sha256.Sum256([]byte(id))
	return hex.EncodeToString(sum[:])
}

// verifyPendingSignupToken returns the hash of the id token carries when its
// MAC verifies under secret. An empty secret verifies nothing.
func verifyPendingSignupToken(secret, token string) (string, error) {
	if secret == "" {
		return "", errPendingSignupMalformed
	}
	dot := strings.IndexByte(token, '.')
	if dot <= 0 || dot == len(token)-1 {
		return "", errPendingSignupMalformed
	}
	id, mac := token[:dot], token[dot+1:]
	if len(id) != 2*oauthPendingSignupIDBytes {
		return "", errPendingSignupMalformed
	}
	if subtle.ConstantTimeCompare([]byte(mac), []byte(signPendingSignupID(secret, id))) != 1 {
		return "", errPendingSignupMalformed
	}
	return pendingSignupTokenHash(id), nil
}

// StartOAuthSignup stores identity and returnTo for
// contracts.OAuthPendingSignupTTL and returns the token that names them. It
// deletes every expired pending sign-up first, so the table holds little more
// than the sign-ups started within one TTL; a failed sweep is logged and the
// sign-up goes ahead, since an expired row is refused whether or not it is
// gone.
func (s *AuthService) StartOAuthSignup(identity contracts.OAuthIdentity, returnTo string) (string, error) {
	if s.db == nil {
		return "", fmt.Errorf("database not initialized")
	}
	secret := s.pendingSignupSecret()
	if secret == "" {
		return "", fmt.Errorf("pending sign-up signing key is not configured")
	}

	buf := make([]byte, oauthPendingSignupIDBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("failed to mint pending sign-up id: %w", err)
	}
	id := hex.EncodeToString(buf)

	now := time.Now().UTC()
	if err := s.db.Where("expires_at <= ?", now).Delete(&authm.OAuthPendingSignup{}).Error; err != nil {
		logger.Default().Error("oauth_pending_signup_sweep_failed", "error", err)
	}

	row := authm.OAuthPendingSignup{
		TokenHash:      pendingSignupTokenHash(id),
		Provider:       identity.Provider,
		ProviderUserID: identity.Subject,
		Email:          identity.Email,
		EmailVerified:  identity.EmailVerified,
		Name:           identity.Name,
		FirstName:      identity.FirstName,
		LastName:       identity.LastName,
		AvatarURL:      identity.AvatarURL,
		ReturnTo:       returnTo,
		ExpiresAt:      now.Add(contracts.OAuthPendingSignupTTL),
	}
	if err := s.db.Create(&row).Error; err != nil {
		return "", fmt.Errorf("failed to store pending sign-up: %w", err)
	}
	return id + "." + signPendingSignupID(secret, id), nil
}

// PeekOAuthSignup returns the pending sign-up token names, leaving it in place.
func (s *AuthService) PeekOAuthSignup(token string) (*contracts.OAuthPendingSignup, error) {
	return s.readPendingSignup(token, func(hash string, row *authm.OAuthPendingSignup) *gorm.DB {
		return s.db.Where("token_hash = ?", hash).Limit(1).Find(row)
	})
}

// ConsumeOAuthSignup removes the pending sign-up token names and returns it.
// The read and the delete are one statement, so a pending sign-up reaches at
// most one caller however many requests carry its token. An expired one is
// deleted all the same and refused.
func (s *AuthService) ConsumeOAuthSignup(token string) (*contracts.OAuthPendingSignup, error) {
	return s.readPendingSignup(token, func(hash string, row *authm.OAuthPendingSignup) *gorm.DB {
		return s.db.Raw("DELETE FROM oauth_pending_signups WHERE token_hash = ? RETURNING *", hash).Scan(row)
	})
}

// readPendingSignup verifies token before any query, runs query for the row
// it names, and classifies the outcome: a CodeOAuthSignupExpired refusal for a bad token, no
// row, or an expired row, and any other error when the row could not be read.
func (s *AuthService) readPendingSignup(
	token string,
	query func(hash string, row *authm.OAuthPendingSignup) *gorm.DB,
) (*contracts.OAuthPendingSignup, error) {
	hash, err := verifyPendingSignupToken(s.pendingSignupSecret(), token)
	if err != nil {
		return nil, apperrors.ErrOAuthSignupExpired(err)
	}
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	var row authm.OAuthPendingSignup
	result := query(hash, &row)
	if result.Error != nil {
		return nil, fmt.Errorf("failed to read pending sign-up: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return nil, apperrors.ErrOAuthSignupExpired(errPendingSignupNotFound)
	}
	if !time.Now().Before(row.ExpiresAt) {
		return nil, apperrors.ErrOAuthSignupExpired(errPendingSignupExpired)
	}
	return &contracts.OAuthPendingSignup{
		Identity: contracts.OAuthIdentity{
			Provider:      row.Provider,
			Subject:       row.ProviderUserID,
			Email:         row.Email,
			EmailVerified: row.EmailVerified,
			Name:          row.Name,
			FirstName:     row.FirstName,
			LastName:      row.LastName,
			AvatarURL:     row.AvatarURL,
		},
		ReturnTo:  row.ReturnTo,
		ExpiresAt: row.ExpiresAt,
	}, nil
}

// CompleteOAuthSignup resolves or creates the account for signup and issues a
// session for it. signup must already be consumed: this is the only use it
// gets.
func (s *AuthService) CompleteOAuthSignup(
	signup *contracts.OAuthPendingSignup,
	consent *contracts.OAuthSignupConsent,
	displayName *string,
) (*authm.User, string, error) {
	if signup == nil {
		return nil, "", fmt.Errorf("pending sign-up cannot be nil")
	}
	user, err := s.userService.FindOrCreateUserFromOAuthIdentity(signup.Identity, consent, displayName)
	if err != nil {
		return nil, "", fmt.Errorf("failed to find or create user: %w", err)
	}
	token, err := s.jwtService.CreateToken(user)
	if err != nil {
		return nil, "", fmt.Errorf("failed to create token: %w", err)
	}
	return user, token, nil
}
