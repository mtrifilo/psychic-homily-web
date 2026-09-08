package auth

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/markbates/goth"
	"github.com/markbates/goth/gothic"
	"gorm.io/gorm"

	"psychic-homily-backend/db"
	"psychic-homily-backend/internal/config"
	apperrors "psychic-homily-backend/internal/errors"
	"psychic-homily-backend/internal/logger"
	authm "psychic-homily-backend/internal/models/auth"
	"psychic-homily-backend/internal/services/contracts"
)

// RealOAuthCompleter implements OAuthCompleter using gothic
type RealOAuthCompleter struct{}

func (r *RealOAuthCompleter) CompleteUserAuth(w http.ResponseWriter, req *http.Request) (goth.User, error) {
	return gothic.CompleteUserAuth(w, req)
}

// AuthService handles authentication business logic
type AuthService struct {
	db             *gorm.DB
	userService    contracts.UserServiceInterface
	jwtService     *JWTService
	oauthCompleter contracts.OAuthCompleter
}

// NewAuthService creates a new authentication service
func NewAuthService(database *gorm.DB, cfg *config.Config, userService contracts.UserServiceInterface) *AuthService {
	if database == nil {
		database = db.GetDB()
	}
	return &AuthService{
		db:             database,
		userService:    userService,
		jwtService:     NewJWTService(database, cfg, userService),
		oauthCompleter: &RealOAuthCompleter{},
	}
}

// OAuthLogin initiates OAuth login flow
func (s *AuthService) OAuthLogin(w http.ResponseWriter, r *http.Request, provider string) error {
	// Check for nil request
	if r == nil {
		return fmt.Errorf("request cannot be nil")
	}

	// Use Goth's recommended way to set the provider in context
	r = gothic.GetContextWithProvider(r, provider)
	ctx := r.Context()

	// Begin OAuth flow - this will redirect to the OAuth provider
	logger.AuthDebug(ctx, "oauth_login_begin", "provider", provider)
	gothic.BeginAuthHandler(w, r)
	logger.AuthDebug(ctx, "oauth_login_begin_returned", "provider", provider)
	return nil
}

// OAuthCallback handles OAuth callback and user creation/linking.
// This legacy path does not enforce signup consent requirements.
func (s *AuthService) OAuthCallback(w http.ResponseWriter, r *http.Request, provider string) (*authm.User, string, error) {
	return s.oauthCallbackInternal(w, r, provider, nil, false)
}

// OAuthCallbackWithConsent enforces terms acceptance for brand-new OAuth users.
func (s *AuthService) OAuthCallbackWithConsent(
	w http.ResponseWriter,
	r *http.Request,
	provider string,
	consent *contracts.OAuthSignupConsent,
) (*authm.User, string, error) {
	return s.oauthCallbackInternal(w, r, provider, consent, true)
}

// CompleteOAuthLink finishes the provider handshake and attaches the resulting
// identity to userID, which the caller has already authenticated. No session is
// issued and no token is returned: the caller already holds one, and a link
// must never be able to change who the browser is signed in as.
//
// Returns a typed *apperrors.AuthError for the refusals
// UserServiceInterface.LinkOAuthAccountToUser documents.
func (s *AuthService) CompleteOAuthLink(
	w http.ResponseWriter,
	r *http.Request,
	provider string,
	userID uint,
) (*authm.User, error) {
	if r == nil {
		return nil, fmt.Errorf("request cannot be nil")
	}

	gothUser, err := s.oauthCompleter.CompleteUserAuth(w, r)
	if err != nil {
		return nil, fmt.Errorf("OAuth completion failed: %w", err)
	}

	linked, err := s.userService.LinkOAuthAccountToUser(userID, gothUser, provider)
	if err != nil {
		return nil, fmt.Errorf("failed to link oauth account: %w", err)
	}
	return linked, nil
}

func (s *AuthService) oauthCallbackInternal(
	w http.ResponseWriter,
	r *http.Request,
	provider string,
	consent *contracts.OAuthSignupConsent,
	enforceConsent bool,
) (*authm.User, string, error) {
	// Check for nil request
	if r == nil {
		return nil, "", fmt.Errorf("request cannot be nil")
	}

	ctx := r.Context()

	// The callback query string carries the provider authorization code and the
	// state nonce, both single-use credentials, so only the path is loggable.
	logger.AuthDebug(ctx, "oauth_callback_received", "provider", provider, "path", r.URL.Path)

	// Use the OAuth completer interface (can be mocked for testing)
	logger.AuthDebug(ctx, "oauth_completer_attempt", "provider", provider)
	gothUser, err := s.oauthCompleter.CompleteUserAuth(w, r)
	if err != nil {
		return nil, "", fmt.Errorf("OAuth completion failed: %w", err)
	}

	// goth.User carries live credentials; never log it as a value. The provider
	// user id is the opaque provider subject, not an address.
	logger.AuthDebug(ctx, "oauth_completer_success",
		"provider", provider,
		"provider_user_id", gothUser.UserID,
	)

	// Find or create user using user service
	var user *authm.User
	if enforceConsent {
		user, err = s.userService.FindOrCreateUserWithConsent(gothUser, provider, consent)
	} else {
		user, err = s.userService.FindOrCreateUser(gothUser, provider)
	}
	if err != nil {
		return nil, "", fmt.Errorf("failed to find or create user: %w", err)
	}

	// Generate JWT token
	token, err := s.jwtService.CreateToken(user)
	if err != nil {
		return nil, "", fmt.Errorf("failed to create token: %w", err)
	}

	return user, token, nil
}

// GetUserProfile retrieves user profile using the user service.
//
// Discriminates the not-found case (the principal was hard- or soft-deleted
// between token issuance and this lookup) from generic backend failures by
// returning a typed *AuthError{Code: CodeUserNotFound}. Callers that need
// session-invalidation semantics (RefreshTokenHandler) route the typed
// error to HTTP 401 + CodeUnauthorized so the client clears the session and
// redirects to login. Generic backend errors (DB outage, etc.) keep flowing
// through as wrapped errors so handlers' fail-closed branches emit 5xx.
func (s *AuthService) GetUserProfile(userID uint) (*authm.User, error) {
	user, err := s.userService.GetUserByID(userID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, apperrors.ErrUserNotFoundByID(userID, err)
		}
		return nil, err
	}
	return user, nil
}

// RefreshUserToken generates a new JWT token for the user, carrying authAt (the
// presented session's authentication time) through unchanged.
func (s *AuthService) RefreshUserToken(user *authm.User, authAt time.Time) (string, error) {
	return s.jwtService.RenewSessionToken(user, authAt)
}

// Logout handles user logout (JWT tokens are stateless, so just return success)
func (s *AuthService) Logout(w http.ResponseWriter, r *http.Request) error {
	// JWT tokens are stateless, so logout is handled client-side
	// The client should remove the token from storage
	return nil
}

// SetOAuthCompleter allows setting a mock OAuth completer for testing
func (s *AuthService) SetOAuthCompleter(completer contracts.OAuthCompleter) {
	s.oauthCompleter = completer
}
