package services

import (
	"fmt"
	"log"
	"net/http"

	"github.com/markbates/goth"
	"github.com/markbates/goth/gothic"
	"gorm.io/gorm"

	"psychic-homily-backend/db"
	"psychic-homily-backend/internal/config"
	"psychic-homily-backend/internal/models"
)

// OAuthCompleter interface for mocking OAuth completion
type OAuthCompleter interface {
	CompleteUserAuth(w http.ResponseWriter, r *http.Request) (goth.User, error)
}

// RealOAuthCompleter implements OAuthCompleter using gothic
type RealOAuthCompleter struct{}

func (r *RealOAuthCompleter) CompleteUserAuth(w http.ResponseWriter, req *http.Request) (goth.User, error) {
	return gothic.CompleteUserAuth(w, req)
}

// AuthService handles authentication business logic
type AuthService struct {
	db          *gorm.DB
	userService *UserService
	jwtService  *JWTService
	oauthCompleter OAuthCompleter
}

// NewAuthService creates a new authentication service
func NewAuthService(database *gorm.DB, cfg *config.Config) *AuthService {
	if database == nil {
		database = db.GetDB()
	}
	return &AuthService{
		db:          database,
		userService: NewUserService(database),
		jwtService:  NewJWTService(database, cfg),
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
	
	// Begin OAuth flow - this will redirect to the OAuth provider
	log.Printf("DEBUG: About to call gothic.BeginAuthHandler with provider: %s", provider)
	gothic.BeginAuthHandler(w, r)
	log.Printf("DEBUG: After gothic.BeginAuthHandler call")
	return nil
}

// OAuthCallback handles OAuth callback and user creation/linking
func (s *AuthService) OAuthCallback(w http.ResponseWriter, r *http.Request, provider string) (*models.User, string, error) {
    // Check for nil request
    if r == nil {
        return nil, "", fmt.Errorf("request cannot be nil")
    }

    log.Printf("DEBUG: Request URL: %s", r.URL.String())
    log.Printf("DEBUG: Using provider: '%s'", provider)
    
    // Use the OAuth completer interface (can be mocked for testing)
    log.Printf("DEBUG: About to call OAuth completer")
    gothUser, err := s.oauthCompleter.CompleteUserAuth(w, r)
    if err != nil {
        return nil, "", fmt.Errorf("OAuth completion failed: %w", err)
    }
    
    log.Printf("DEBUG: Successfully completed OAuth! gothUser: %+v", gothUser)

    // Find or create user using user service
    user, err := s.userService.FindOrCreateUser(gothUser, provider)
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

// Add this to backend/internal/services/auth.go
// GetUserProfile retrieves user profile using the user service
func (s *AuthService) GetUserProfile(userID uint) (*models.User, error) {
	return s.userService.GetUserByID(userID)
}

// RefreshUserToken generates a new JWT token for the user
func (s *AuthService) RefreshUserToken(user *models.User) (string, error) {
	return s.jwtService.CreateToken(user)
}

// Logout handles user logout (JWT tokens are stateless, so just return success)
func (s *AuthService) Logout(w http.ResponseWriter, r *http.Request) error {
	// JWT tokens are stateless, so logout is handled client-side
	// The client should remove the token from storage
	return nil
}

// SetOAuthCompleter allows setting a mock OAuth completer for testing
func (s *AuthService) SetOAuthCompleter(completer OAuthCompleter) {
	s.oauthCompleter = completer
}
