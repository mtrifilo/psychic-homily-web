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

// A pending token is "<id>.<mac>": id is 32 random bytes in hex, and mac is
// an HMAC-SHA256 under the JWT secret over the purpose's domain and id. The
// row is keyed by the SHA-256 of id, so neither a read of the table nor a
// guessed id yields a token, and a token whose MAC fails is refused before
// any query. A pending sign-up and a pending link sign under different
// domains, so neither's token verifies as the other's.
const (
	oauthPendingSignupIDBytes = 32
	oauthPendingSignupDomain  = "oauthpendingsignup|"
	oauthPendingLinkDomain    = "oauthpendinglink|"
)

var (
	errPendingMalformed = errors.New("pending token is malformed or its signature fails")
	errPendingNotFound  = errors.New("pending row not found")
	errPendingExpired   = errors.New("pending row expired")
)

// pendingPurpose is what tells the two kinds of oauth_pending_signups row
// apart: the purpose column, the domain the token is signed under, and the
// refusal for a token that names no live row of that purpose.
type pendingPurpose struct {
	purpose string
	domain  string
	expired func(error) *apperrors.AuthError
}

var (
	pendingSignupPurpose = pendingPurpose{authm.OAuthPendingPurposeSignup, oauthPendingSignupDomain, apperrors.ErrOAuthSignupExpired}
	pendingLinkPurpose   = pendingPurpose{authm.OAuthPendingPurposeLink, oauthPendingLinkDomain, apperrors.ErrOAuthLinkPendingExpired}
)

func (s *AuthService) pendingSecret() string {
	if s.config == nil {
		return ""
	}
	return s.config.JWT.SecretKey
}

func signPendingID(secret, domain, id string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(domain))
	mac.Write([]byte(id))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func pendingTokenHash(id string) string {
	sum := sha256.Sum256([]byte(id))
	return hex.EncodeToString(sum[:])
}

// verifyPendingToken returns the hash of the id token carries when its MAC
// verifies under secret and domain. An empty secret verifies nothing.
func verifyPendingToken(secret, domain, token string) (string, error) {
	if secret == "" {
		return "", errPendingMalformed
	}
	dot := strings.IndexByte(token, '.')
	if dot <= 0 || dot == len(token)-1 {
		return "", errPendingMalformed
	}
	id, mac := token[:dot], token[dot+1:]
	if len(id) != 2*oauthPendingSignupIDBytes {
		return "", errPendingMalformed
	}
	if subtle.ConstantTimeCompare([]byte(mac), []byte(signPendingID(secret, domain, id))) != 1 {
		return "", errPendingMalformed
	}
	return pendingTokenHash(id), nil
}

// storePending writes row under kind's purpose for ttl and returns the token
// that names it. It deletes every expired pending row first, so the table
// holds little more than the rows started within one lifetime; a failed sweep
// is logged and the write goes ahead, since an expired row is refused whether
// or not it is gone.
func (s *AuthService) storePending(row *authm.OAuthPendingSignup, kind pendingPurpose, ttl time.Duration) (string, error) {
	if s.db == nil {
		return "", fmt.Errorf("database not initialized")
	}
	secret := s.pendingSecret()
	if secret == "" {
		return "", fmt.Errorf("pending sign-up signing key is not configured")
	}

	buf := make([]byte, oauthPendingSignupIDBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("failed to mint pending %s id: %w", kind.purpose, err)
	}
	id := hex.EncodeToString(buf)

	now := time.Now().UTC()
	if err := s.db.Where("expires_at <= ?", now).Delete(&authm.OAuthPendingSignup{}).Error; err != nil {
		logger.Default().Error("oauth_pending_signup_sweep_failed", "error", err)
	}

	row.TokenHash = pendingTokenHash(id)
	row.Purpose = kind.purpose
	row.ExpiresAt = now.Add(ttl)
	if err := s.db.Create(row).Error; err != nil {
		return "", fmt.Errorf("failed to store pending %s: %w", kind.purpose, err)
	}
	return id + "." + signPendingID(secret, kind.domain, id), nil
}

// pendingRowFromIdentity is the row for identity and returnTo, before its
// purpose, key and expiry are set.
func pendingRowFromIdentity(identity contracts.OAuthIdentity, returnTo string) *authm.OAuthPendingSignup {
	return &authm.OAuthPendingSignup{
		Provider:       identity.Provider,
		ProviderUserID: identity.Subject,
		Email:          identity.Email,
		EmailVerified:  identity.EmailVerified,
		Name:           identity.Name,
		FirstName:      identity.FirstName,
		LastName:       identity.LastName,
		AvatarURL:      identity.AvatarURL,
		ReturnTo:       returnTo,
	}
}

// identityFromPendingRow is the provider identity row holds.
func identityFromPendingRow(row *authm.OAuthPendingSignup) contracts.OAuthIdentity {
	return contracts.OAuthIdentity{
		Provider:      row.Provider,
		Subject:       row.ProviderUserID,
		Email:         row.Email,
		EmailVerified: row.EmailVerified,
		Name:          row.Name,
		FirstName:     row.FirstName,
		LastName:      row.LastName,
		AvatarURL:     row.AvatarURL,
	}
}

// StartOAuthSignup stores identity and returnTo for
// contracts.OAuthPendingSignupTTL and returns the token that names them.
func (s *AuthService) StartOAuthSignup(identity contracts.OAuthIdentity, returnTo string) (string, error) {
	return s.storePending(pendingRowFromIdentity(identity, returnTo), pendingSignupPurpose, contracts.OAuthPendingSignupTTL)
}

// PeekOAuthSignup returns the pending sign-up token names, leaving it in place.
func (s *AuthService) PeekOAuthSignup(token string) (*contracts.OAuthPendingSignup, error) {
	row, _, err := s.readPending(s.db, pendingSignupPurpose, token, peekPendingRow)
	if err != nil {
		return nil, err
	}
	return pendingSignupFromRow(row), nil
}

// ConsumeOAuthSignup removes the pending sign-up token names and returns it.
// The read and the delete are one statement, so a pending sign-up reaches at
// most one caller however many requests carry its token. An expired one is
// deleted all the same and refused.
func (s *AuthService) ConsumeOAuthSignup(token string) (*contracts.OAuthPendingSignup, error) {
	row, _, err := s.readPending(s.db, pendingSignupPurpose, token, consumePendingRow)
	if err != nil {
		return nil, err
	}
	return pendingSignupFromRow(row), nil
}

func pendingSignupFromRow(row *authm.OAuthPendingSignup) *contracts.OAuthPendingSignup {
	return &contracts.OAuthPendingSignup{
		Identity:  identityFromPendingRow(row),
		ReturnTo:  row.ReturnTo,
		ExpiresAt: row.ExpiresAt,
	}
}

// pendingRowQuery reads the row of purpose keyed by hash into row through db.
type pendingRowQuery func(db *gorm.DB, purpose, hash string, row *authm.OAuthPendingSignup) *gorm.DB

func peekPendingRow(db *gorm.DB, purpose, hash string, row *authm.OAuthPendingSignup) *gorm.DB {
	return db.Where("token_hash = ? AND purpose = ?", hash, purpose).Limit(1).Find(row)
}

func consumePendingRow(db *gorm.DB, purpose, hash string, row *authm.OAuthPendingSignup) *gorm.DB {
	return db.Raw("DELETE FROM oauth_pending_signups WHERE token_hash = ? AND purpose = ? RETURNING *", hash, purpose).Scan(row)
}

// readPending verifies token under kind before any query and reads the row
// it names through db.
func (s *AuthService) readPending(db *gorm.DB, kind pendingPurpose, token string, query pendingRowQuery) (*authm.OAuthPendingSignup, string, error) {
	hash, err := verifyPendingToken(s.pendingSecret(), kind.domain, token)
	if err != nil {
		return nil, "", kind.expired(err)
	}
	row, err := readPendingByHash(db, kind, hash, query)
	return row, hash, err
}

// readPendingByHash runs query for the row of kind's purpose keyed by hash,
// and classifies the outcome: kind's expired refusal for no row or an
// expired row, and any other error when the row could not be read.
func readPendingByHash(db *gorm.DB, kind pendingPurpose, hash string, query pendingRowQuery) (*authm.OAuthPendingSignup, error) {
	if db == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	var row authm.OAuthPendingSignup
	result := query(db, kind.purpose, hash, &row)
	if result.Error != nil {
		return nil, fmt.Errorf("failed to read pending %s: %w", kind.purpose, result.Error)
	}
	if result.RowsAffected == 0 {
		return nil, kind.expired(errPendingNotFound)
	}
	if !time.Now().Before(row.ExpiresAt) {
		return nil, kind.expired(errPendingExpired)
	}
	return &row, nil
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
