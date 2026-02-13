package auth

import (
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
}

// Add GitHub provider if configured  
if cfg.OAuth.GitHubClientID != "" && cfg.OAuth.GitHubClientSecret != "" {
    githubProvider := github.New(
        cfg.OAuth.GitHubClientID,
        cfg.OAuth.GitHubClientSecret,
        cfg.OAuth.GitHubCallbackURL,
    )
    providers = append(providers, githubProvider)
}

	// Register providers with Goth
	goth.UseProviders(providers...)

	return nil
}

// GetSession retrieves the session from the request
func GetSession(r *http.Request) (*sessions.Session, error) {
	return SessionStore.Get(r, "_gothic_session")
}

// SaveSession saves the session to the response
func SaveSession(w http.ResponseWriter, r *http.Request, session *sessions.Session) error {
	return session.Save(r, w)
}
