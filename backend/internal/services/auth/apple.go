package auth

import (
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
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

	// appleKeysTTL is how long a fetched key set answers lookups before the
	// next lookup refreshes it.
	appleKeysTTL = 24 * time.Hour

	// appleKeysFetchTimeout bounds one whole key-set exchange: connect,
	// response headers and body.
	appleKeysFetchTimeout = 5 * time.Second

	// appleKeysForcedRefreshCooldown is the minimum spacing between refreshes
	// forced by a key id the fresh cached set does not hold. The key id comes
	// from the unverified token header, so this spacing is what keeps a caller
	// from driving one fetch per request.
	appleKeysForcedRefreshCooldown = time.Minute

	// appleKeysFailureLogInterval is the minimum spacing between two logged
	// key-set fetch failures.
	appleKeysFailureLogInterval = time.Minute
)

// appleKeysHTTPClient serves every key-set fetch of a service whose
// keysHTTPClient is nil.
var appleKeysHTTPClient = &http.Client{Timeout: appleKeysFetchTimeout}

// AppleAuthService handles Sign in with Apple authentication
type AppleAuthService struct {
	db         *gorm.DB
	config     *config.Config
	jwtService *JWTService

	// Cached Apple public keys. keysMu guards every field in this group.
	keysMu    sync.Mutex
	keys      map[string]*rsa.PublicKey
	keysExpAt time.Time
	// keysFetch is the key-set fetch in progress, nil when none is.
	keysFetch *appleKeysFetch
	// lastForcedRefreshAt is when the latest refresh forced by an unknown key
	// id started.
	lastForcedRefreshAt time.Time
	// lastFetchFailureLogAt is when a fetch failure was last logged.
	lastFetchFailureLogAt time.Time

	// Test overrides. Each zero value selects the production behavior.
	fetchAppleKeysFromURL string           // replaces appleKeysURL
	keysHTTPClient        *http.Client     // replaces appleKeysHTTPClient
	keysNow               func() time.Time // replaces time.Now for the key cache
	keysLogger            *slog.Logger     // replaces logger.Default() for fetch failures
}

// appleKeysFetch is one key-set fetch that concurrent lookups share. err is
// written before done closes and read only after it has.
type appleKeysFetch struct {
	done chan struct{}
	err  error
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
// Returns a typed *apperrors.AuthError with CodeOAuthLinkRefused whenever the
// address already belongs to an account. There is no by-address link on any
// path: a matching address says who owns the mailbox, never who owns the
// account. Returns CodeAccountInactive when the Apple identity resolves to an
// account that may not sign in (authm.User.CanSignIn). Callers that render a
// refusal differently from a fault must discriminate on the code.
//
// A created account carries email_verified from the token's own claim, so an
// address Apple will not vouch for produces an unverified account.
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
		if !user.CanSignIn() {
			logger.Default().Warn("oauth_signin_refused_account_inactive",
				"provider", "apple",
				"user_id", user.ID)
			return nil, apperrors.ErrAccountInactive()
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
			// No by-address link, for the reason the goth path states: a
			// matching address says who owns the mailbox, never who owns the
			// account. Apple's refusal is its own, because Apple has no link
			// path to send anyone to: its callback is a native-token POST, not
			// the goth handshake /auth/link/{provider} drives, so there is no
			// Apple control in Settings to name.
			logger.Default().Warn("oauth_signin_refused_address_belongs_to_account",
				"provider", "apple",
				"email_hash", logger.HashEmail(claims.Email))
			return nil, apperrors.ErrAppleSignInRefused(claims.Email)
		case !errors.Is(err, gorm.ErrRecordNotFound):
			// A failed lookup is not proof that no account holds the address,
			// so it must not fall through to creating one.
			return nil, fmt.Errorf("database error: %w", err)
		}
	}

	// Create a new user
	return s.createAppleUser(appleUserID, claims.Email, firstName, lastName, claims.IsEmailVerified())
}

// GenerateToken creates a JWT for the user
func (s *AppleAuthService) GenerateToken(user *authm.User) (string, error) {
	return s.jwtService.CreateToken(user)
}

// createAppleUser creates a new user from Apple Sign In data
func (s *AppleAuthService) createAppleUser(appleUserID, email, firstName, lastName string, emailVerified bool) (*authm.User, error) {
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
		// The column carries what the identity token asserted. An account
		// created from an address Apple would not vouch for stays unverified,
		// which closes every capability gated on the flag until this system's
		// own verification email opens them.
		EmailVerified: emailVerified,
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

// getApplePublicKey returns the key for kid, refreshing the cached set when it
// is stale or lacks kid. Each call starts or joins at most one fetch: a call
// that arrives while a fetch runs waits for that fetch instead of starting
// another. A refresh for a kid the fresh set lacks starts at most once per
// appleKeysForcedRefreshCooldown, and inside that window the lookup fails
// without a fetch. A failed fetch leaves the cached set and its expiry as they
// were.
func (s *AppleAuthService) getApplePublicKey(kid string) (*rsa.PublicKey, error) {
	s.keysMu.Lock()
	now := s.keysClock()
	fresh := now.Before(s.keysExpAt)
	if key, ok := s.keys[kid]; ok && fresh {
		s.keysMu.Unlock()
		return key, nil
	}

	fetch := s.keysFetch
	starter := fetch == nil
	if starter && fresh && now.Sub(s.lastForcedRefreshAt) < appleKeysForcedRefreshCooldown {
		s.keysMu.Unlock()
		return nil, errAppleKeyNotFound(kid)
	}
	if starter {
		if fresh {
			s.lastForcedRefreshAt = now
		}
		fetch = &appleKeysFetch{done: make(chan struct{})}
		s.keysFetch = fetch
	}
	s.keysMu.Unlock()

	if starter {
		s.runAppleKeysFetch(fetch)
	} else {
		<-fetch.done
	}

	s.keysMu.Lock()
	defer s.keysMu.Unlock()
	if key, ok := s.keys[kid]; ok && s.keysClock().Before(s.keysExpAt) {
		return key, nil
	}
	if fetch.err != nil {
		return nil, fetch.err
	}
	return nil, errAppleKeyNotFound(kid)
}

func errAppleKeyNotFound(kid string) error {
	return fmt.Errorf("apple public key not found for kid: %s", kid)
}

// runAppleKeysFetch performs fetch and publishes its outcome. The publish is
// deferred so it also runs when the fetch panics: keysFetch never stays set
// and no waiter blocks on a fetch that has stopped.
func (s *AppleAuthService) runAppleKeysFetch(fetch *appleKeysFetch) {
	var keys map[string]*rsa.PublicKey
	err := errors.New("apple key fetch did not complete")
	defer func() {
		s.keysMu.Lock()
		now := s.keysClock()
		logFailure := false
		if err == nil {
			s.keys = keys
			s.keysExpAt = now.Add(appleKeysTTL)
		} else if now.Sub(s.lastFetchFailureLogAt) >= appleKeysFailureLogInterval {
			s.lastFetchFailureLogAt = now
			logFailure = true
		}
		fetch.err = err
		s.keysFetch = nil
		s.keysMu.Unlock()
		close(fetch.done)
		if logFailure {
			s.keysLog().Error("apple_keys_fetch_failed", "error", err.Error())
		}
	}()
	keys, err = s.fetchAppleKeys()
}

// fetchAppleKeys downloads Apple's JWK set and parses it into RSA public keys
func (s *AppleAuthService) fetchAppleKeys() (map[string]*rsa.PublicKey, error) {
	keysURL := appleKeysURL
	if s.fetchAppleKeysFromURL != "" {
		keysURL = s.fetchAppleKeysFromURL
	}
	resp, err := s.keysClient().Get(keysURL)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch Apple keys: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck // deferred Close; nothing actionable on failure

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("apple keys endpoint returned status %d", resp.StatusCode)
	}

	var jwkSet appleJWKSet
	if err := json.NewDecoder(resp.Body).Decode(&jwkSet); err != nil {
		return nil, fmt.Errorf("failed to decode Apple JWK set: %w", err)
	}

	keys := make(map[string]*rsa.PublicKey)
	for _, jwk := range jwkSet.Keys {
		if jwk.Kty != "RSA" {
			continue
		}

		nBytes, err := base64.RawURLEncoding.DecodeString(jwk.N)
		if err != nil {
			return nil, fmt.Errorf("failed to decode modulus for kid %s: %w", jwk.Kid, err)
		}

		eBytes, err := base64.RawURLEncoding.DecodeString(jwk.E)
		if err != nil {
			return nil, fmt.Errorf("failed to decode exponent for kid %s: %w", jwk.Kid, err)
		}

		n := new(big.Int).SetBytes(nBytes)
		e := new(big.Int).SetBytes(eBytes)

		keys[jwk.Kid] = &rsa.PublicKey{
			N: n,
			E: int(e.Int64()),
		}
	}

	return keys, nil
}

func (s *AppleAuthService) keysClient() *http.Client {
	if s.keysHTTPClient != nil {
		return s.keysHTTPClient
	}
	return appleKeysHTTPClient
}

func (s *AppleAuthService) keysClock() time.Time {
	if s.keysNow != nil {
		return s.keysNow()
	}
	return time.Now()
}

func (s *AppleAuthService) keysLog() *slog.Logger {
	if s.keysLogger != nil {
		return s.keysLogger
	}
	return logger.Default()
}
