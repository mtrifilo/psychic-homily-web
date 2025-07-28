package auth

import (
	"log"
	"net/http"

	"github.com/gorilla/sessions"
	"github.com/markbates/goth"
	"github.com/markbates/goth/gothic"
	"github.com/markbates/goth/providers/github"
	"github.com/markbates/goth/providers/google"

	"psychic-homily-backend/internal/config"
)

var (
	// SessionStore holds the session store for authentication
	SessionStore sessions.Store
)

// SetupGoth configures Goth with OAuth providers
func SetupGoth(cfg *config.Config) error {
	// Debug: Check the secret key being used
	log.Printf("DEBUG: Setting up Goth with SecretKey: %s (length: %d)", 
		cfg.OAuth.SecretKey, len(cfg.OAuth.SecretKey))
	
	// Configure session store
	SessionStore = sessions.NewCookieStore([]byte(cfg.OAuth.SecretKey))
	
	// Configure session store with environment-specific settings
	if cookieStore, ok := SessionStore.(*sessions.CookieStore); ok {
		cookieStore.Options = &sessions.Options{
			Path:     cfg.Session.Path,
			Domain:   cfg.Session.Domain,
			MaxAge:   cfg.Session.MaxAge,
			HttpOnly: cfg.Session.HttpOnly,
			Secure:   cfg.Session.Secure,
			SameSite: cfg.Session.GetSameSite(),
		}
		log.Printf("DEBUG: Session store configured with Path: %s, Domain: '%s', MaxAge: %d, HttpOnly: %t, Secure: %t, SameSite: %s", 
			cfg.Session.Path, cfg.Session.Domain, cfg.Session.MaxAge, cfg.Session.HttpOnly, cfg.Session.Secure, cfg.Session.SameSite)
	}
		// Configure gothic to use our custom session store
	gothic.Store = SessionStore

	// Configure OAuth providers
	providers := []goth.Provider{}

// Add Google provider if configured
if cfg.OAuth.GoogleClientID != "" && cfg.OAuth.GoogleClientSecret != "" {
    googleProvider := google.New(
        cfg.OAuth.GoogleClientID,
        cfg.OAuth.GoogleClientSecret,
        cfg.OAuth.GoogleCallbackURL,
    )
    providers = append(providers, googleProvider)
    log.Printf("DEBUG: Google provider configured with callback URL: %s", cfg.OAuth.GoogleCallbackURL)
}

// Add GitHub provider if configured  
if cfg.OAuth.GitHubClientID != "" && cfg.OAuth.GitHubClientSecret != "" {
    githubProvider := github.New(
        cfg.OAuth.GitHubClientID,
        cfg.OAuth.GitHubClientSecret,
        cfg.OAuth.GitHubCallbackURL,
    )
    providers = append(providers, githubProvider)
    log.Printf("DEBUG: GitHub provider configured with callback URL: %s", cfg.OAuth.GitHubCallbackURL)
}

	// Register providers with Goth
	goth.UseProviders(providers...)

	return nil
}

// GetSession retrieves the session from the request
func GetSession(r *http.Request) (*sessions.Session, error) {
	session, err := SessionStore.Get(r, "_gothic_session")
	if err != nil {
		log.Printf("DEBUG: SessionStore.Get error: %v", err)
	} else {
		log.Printf("DEBUG: SessionStore.Get successful, session ID: %s", session.ID)
	}
	return session, err
}

// SaveSession saves the session to the response
func SaveSession(w http.ResponseWriter, r *http.Request, session *sessions.Session) error {
	return session.Save(r, w)
}
