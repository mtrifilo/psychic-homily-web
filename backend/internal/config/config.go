package config

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	EnvProduction  = "production"
	EnvStage       = "stage"
	EnvDevelopment = "development"
)

// Environment variable constants
const (
	// Core
	EnvEnvironment = "ENVIRONMENT"

	// Server
	EnvAPIAddr = "API_ADDR"

	// Database
	EnvDatabaseURL = "DATABASE_URL"

	// OAuth
	EnvGoogleClientID     = "GOOGLE_CLIENT_ID"
	EnvGoogleClientSecret = "GOOGLE_CLIENT_SECRET"
	EnvGoogleCallbackURL  = "GOOGLE_CALLBACK_URL"
	EnvGitHubClientID     = "GITHUB_CLIENT_ID"
	EnvGitHubClientSecret = "GITHUB_CLIENT_SECRET"
	EnvGitHubCallbackURL  = "GITHUB_CALLBACK_URL"
	EnvOAuthSecretKey     = "OAUTH_SECRET_KEY"

	// JWT
	EnvJWTSecretKey   = "JWT_SECRET_KEY"
	EnvJWTExpiryHours = "JWT_EXPIRY_HOURS"

	// Session
	EnvSessionPath     = "SESSION_PATH"
	EnvSessionDomain   = "SESSION_DOMAIN"
	EnvSessionMaxAge   = "SESSION_MAX_AGE"
	EnvSessionHTTPOnly = "SESSION_HTTP_ONLY"
	EnvSessionSecure   = "SESSION_SECURE"
	EnvSessionSameSite = "SESSION_SAME_SITE"

	// CORS
	EnvCORSAllowedOrigins = "CORS_ALLOWED_ORIGINS"

	// Email (Resend)
	// RESEND_API_KEY: Your Resend API key (e.g., "re_123abc...")
	// FROM_EMAIL: Sender email address (e.g., "noreply@psychichomily.com")
	// FRONTEND_URL: Base URL for email links (e.g., "http://localhost:3000" or "https://psychichomily.com")
	EnvResendAPIKey = "RESEND_API_KEY"
	EnvFromEmail    = "FROM_EMAIL"
	EnvFrontendURL  = "FRONTEND_URL"

	// Discord
	EnvDiscordWebhookURL = "DISCORD_WEBHOOK_URL"
	EnvDiscordEnabled    = "DISCORD_NOTIFICATIONS_ENABLED"

	// Music Discovery
	EnvInternalAPISecret        = "INTERNAL_API_SECRET"
	EnvMusicDiscoveryEnabled    = "MUSIC_DISCOVERY_ENABLED"

	// WebAuthn / Passkeys
	EnvWebAuthnRPID          = "WEBAUTHN_RP_ID"
	EnvWebAuthnRPDisplayName = "WEBAUTHN_RP_NAME"
	EnvWebAuthnRPOrigins     = "WEBAUTHN_RP_ORIGINS"

	// Apple Sign In
	EnvAppleBundleID = "APPLE_BUNDLE_ID"

	// Anthropic AI
	EnvAnthropicAPIKey = "ANTHROPIC_API_KEY"
)

// Config holds all configuration for the application
type Config struct {
	Server         ServerConfig
	CORS           CORSConfig
	OAuth          OAuthConfig
	Database       DatabaseConfig
	JWT            JWTConfig
	Session        SessionConfig
	Email          EmailConfig
	Discord        DiscordConfig
	MusicDiscovery MusicDiscoveryConfig
	WebAuthn       WebAuthnConfig
	Apple          AppleConfig
	Anthropic      AnthropicConfig
}

// AppleConfig holds Sign in with Apple configuration
type AppleConfig struct {
	BundleID string // iOS app bundle ID for audience validation
}

// AnthropicConfig holds Anthropic API configuration
type AnthropicConfig struct {
	APIKey string
}

// DiscordConfig holds Discord webhook configuration for admin notifications
type DiscordConfig struct {
	WebhookURL string
	Enabled    bool
}

// MusicDiscoveryConfig holds configuration for automatic music discovery
type MusicDiscoveryConfig struct {
	InternalAPISecret string
	Enabled           bool
	FrontendURL       string
}

// WebAuthnConfig holds WebAuthn/passkey configuration
type WebAuthnConfig struct {
	RPID          string   // Relying Party ID (e.g., "localhost" or "psychichomily.com")
	RPDisplayName string   // Relying Party display name (e.g., "Psychic Homily")
	RPOrigins     []string // Allowed origins for WebAuthn (e.g., ["https://psychichomily.com"])
}

// EmailConfig holds email-related configuration (Resend)
type EmailConfig struct {
	ResendAPIKey string
	FromEmail    string
	FrontendURL  string
}

// ServerConfig holds server-related configuration
type ServerConfig struct {
	Addr     string
	LogLevel string
}

// CORSConfig holds CORS-related configuration
type CORSConfig struct {
	AllowedOrigins   []string
	AllowedMethods   []string
	AllowedHeaders   []string
	AllowCredentials bool
}

// OAuthConfig holds OAuth-related configuration
type OAuthConfig struct {
	GoogleClientID     string `env:"GOOGLE_CLIENT_ID"`
	GoogleClientSecret string `env:"GOOGLE_CLIENT_SECRET"`
	GoogleCallbackURL  string `env:"GOOGLE_CALLBACK_URL" envDefault:"http://localhost:8080/auth/callback/google"`
	GitHubClientID     string `env:"GITHUB_CLIENT_ID"`
	GitHubClientSecret string `env:"GITHUB_CLIENT_SECRET"`
	GitHubCallbackURL  string `env:"GITHUB_CALLBACK_URL" envDefault:"http://localhost:8080/auth/callback/github"`
	SecretKey          string `env:"OAUTH_SECRET_KEY" envDefault:"your-secret-key-change-in-production"`
}

// DatabaseConfig holds database-related configuration
type DatabaseConfig struct {
	URL string
}

// JWTConfig holds JWT-related configuration
type JWTConfig struct {
	SecretKey string `env:"JWT_SECRET_KEY"`
	Expiry    int64  `env:"JWT_EXPIRY_HOURS" envDefault:"24"`
}

// SessionConfig holds session-related configuration
type SessionConfig struct {
	Path     string `env:"SESSION_PATH" envDefault:"/"`
	Domain   string `env:"SESSION_DOMAIN" envDefault:""`
	MaxAge   int    `env:"SESSION_MAX_AGE" envDefault:"86400"` // 24 hours
	HttpOnly bool   `env:"SESSION_HTTP_ONLY" envDefault:"true"`
	Secure   bool   `env:"SESSION_SECURE" envDefault:"false"`
	SameSite string `env:"SESSION_SAME_SITE" envDefault:"lax"`
}

// GetSameSite returns the http.SameSite value for the session configuration.
func (s SessionConfig) GetSameSite() http.SameSite {
	switch strings.ToLower(s.SameSite) {
	case "strict":
		return http.SameSiteStrictMode
	case "none":
		return http.SameSiteNoneMode
	default:
		return http.SameSiteLaxMode
	}
}

// AuthCookieName is the name of the authentication cookie
const AuthCookieName = "auth_token"

// NewAuthCookie creates a new authentication cookie with the given token and expiry duration.
func (s SessionConfig) NewAuthCookie(token string, expiry time.Duration) http.Cookie {
	return http.Cookie{
		Name:     AuthCookieName,
		Value:    token,
		Path:     s.Path,
		Domain:   s.Domain,
		HttpOnly: s.HttpOnly,
		Secure:   s.Secure,
		SameSite: s.GetSameSite(),
		Expires:  time.Now().Add(expiry),
	}
}

// ClearAuthCookie creates a cookie that clears the authentication token.
func (s SessionConfig) ClearAuthCookie() http.Cookie {
	return http.Cookie{
		Name:     AuthCookieName,
		Value:    "",
		Path:     s.Path,
		Domain:   s.Domain,
		HttpOnly: s.HttpOnly,
		Secure:   s.Secure,
		SameSite: http.SameSiteStrictMode,
		Expires:  time.Unix(0, 0),
		MaxAge:   -1,
	}
}

// Update your Load() function to make CORS configurable
func Load() (*Config, error) {
	// Debug: Check if OAUTH_SECRET_KEY is being loaded
	oauthSecretKey := GetEnv(EnvOAuthSecretKey, "your-secret-key-here")

	// Get CORS origins from environment or use defaults
	corsOrigins := getCORSOrigins()

	cfg := &Config{
		Server: ServerConfig{
			Addr: GetEnv(EnvAPIAddr, "localhost:8080"),
		},
		CORS: CORSConfig{
			AllowedOrigins:   corsOrigins,
			AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
			AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-Requested-With", "Origin", "Access-Control-Request-Method", "Access-Control-Request-Headers"},
			AllowCredentials: true,
		},
		OAuth: OAuthConfig{
			GoogleClientID:     GetEnv(EnvGoogleClientID, ""),
			GoogleClientSecret: GetEnv(EnvGoogleClientSecret, ""),
			GoogleCallbackURL:  GetEnv(EnvGoogleCallbackURL, "http://localhost:8080/auth/callback/google"),
			GitHubClientID:     GetEnv(EnvGitHubClientID, ""),
			GitHubClientSecret: GetEnv(EnvGitHubClientSecret, ""),
			GitHubCallbackURL:  GetEnv(EnvGitHubCallbackURL, "http://localhost:8080/auth/callback/github"),
			SecretKey:          oauthSecretKey,
		},
		Database: DatabaseConfig{
			URL: GetEnv(EnvDatabaseURL, "postgres://psychicadmin:secretpassword@localhost:5432/psychicdb?sslmode=disable"),
		},
		JWT: JWTConfig{
			SecretKey: GetEnv(EnvJWTSecretKey, "your-super-secret-jwt-key-32-chars-minimum"),
			Expiry:    int64(getEnvAsInt(EnvJWTExpiryHours, 24)),
		},
		Session: SessionConfig{
			Path:     GetEnv(EnvSessionPath, "/"),
			Domain:   GetEnv(EnvSessionDomain, ""),
			MaxAge:   getEnvAsInt(EnvSessionMaxAge, 86400),
			HttpOnly: getEnvAsBool(EnvSessionHTTPOnly, true),
			Secure:   getEnvAsBool(EnvSessionSecure, false),
			SameSite: GetEnv(EnvSessionSameSite, "lax"),
		},
		Email: EmailConfig{
			ResendAPIKey: GetEnv(EnvResendAPIKey, ""),
			FromEmail:    GetEnv(EnvFromEmail, "noreply@psychichomily.com"),
			FrontendURL:  getFrontendURL(),
		},
		Discord: DiscordConfig{
			WebhookURL: GetEnv(EnvDiscordWebhookURL, ""),
			Enabled:    getEnvAsBool(EnvDiscordEnabled, false),
		},
		MusicDiscovery: MusicDiscoveryConfig{
			InternalAPISecret: GetEnv(EnvInternalAPISecret, ""),
			Enabled:           getEnvAsBool(EnvMusicDiscoveryEnabled, false),
			FrontendURL:       getFrontendURL(),
		},
		WebAuthn: WebAuthnConfig{
			RPID:          getWebAuthnRPID(),
			RPDisplayName: GetEnv(EnvWebAuthnRPDisplayName, "Psychic Homily"),
			RPOrigins:     getWebAuthnOrigins(),
		},
		Apple: AppleConfig{
			BundleID: GetEnv(EnvAppleBundleID, "com.psychichomily.ios"),
		},
		Anthropic: AnthropicConfig{
			APIKey: GetEnv(EnvAnthropicAPIKey, ""),
		},
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("configuration validation failed: %w", err)
	}

	return cfg, nil
}

// getFrontendURL returns the frontend URL based on environment
func getFrontendURL() string {
	if url := os.Getenv(EnvFrontendURL); url != "" {
		return url
	}

	env := os.Getenv(EnvEnvironment)
	switch env {
	case EnvProduction:
		return "https://psychichomily.com"
	case EnvStage:
		return "https://stage.psychichomily.com"
	default:
		return "http://localhost:3000"
	}
}

// Add this helper function
func getCORSOrigins() []string {
	if corsEnv := os.Getenv(EnvCORSAllowedOrigins); corsEnv != "" {
		return strings.Split(corsEnv, ",")
	}

	// Default origins based on environment
	env := os.Getenv(EnvEnvironment)
	if env == EnvProduction {
		return []string{
			"https://psychichomily.com",
			"https://www.psychichomily.com",
		}
	}

	if env == EnvStage {
		return []string{
			"https://stage.psychichomily.com",
			"https://www.stage.psychichomily.com",
		}
	}

	if env == EnvDevelopment {
		return []string{
			"http://localhost:3000",
			"http://localhost:5173",
			"http://localhost:1313", // Hugo dev server
		}
	}

	// Development defaults
	return []string{
		"https://psychichomily.com",
		"https://www.psychichomily.com",
		"http://localhost:3000",
		"http://localhost:5173",
		"http://localhost:1313", // Hugo dev server
	}
}

// getWebAuthnRPID returns the WebAuthn Relying Party ID based on environment
func getWebAuthnRPID() string {
	if rpID := os.Getenv(EnvWebAuthnRPID); rpID != "" {
		return rpID
	}

	env := os.Getenv(EnvEnvironment)
	switch env {
	case EnvProduction:
		return "psychichomily.com"
	case EnvStage:
		return "stage.psychichomily.com"
	default:
		return "localhost"
	}
}

// getWebAuthnOrigins returns the allowed WebAuthn origins based on environment
func getWebAuthnOrigins() []string {
	if origins := os.Getenv(EnvWebAuthnRPOrigins); origins != "" {
		return strings.Split(origins, ",")
	}

	// Default to frontend URL
	return []string{getFrontendURL()}
}

// placeholderSecrets contains default placeholder values that must not be used in production.
var placeholderSecrets = []string{
	"your-secret-key-here",
	"your-secret-key-change-in-production",
	"your-super-secret-jwt-key-32-chars-minimum",
}

// Validate checks that security-critical secrets are not placeholder defaults.
// Only enforced when ENVIRONMENT is set and is not "development".
func (c *Config) Validate() error {
	env := os.Getenv(EnvEnvironment)
	if env == "" || env == EnvDevelopment {
		return nil
	}

	for _, placeholder := range placeholderSecrets {
		if c.JWT.SecretKey == placeholder {
			return fmt.Errorf("JWT_SECRET_KEY is using a placeholder default; set a unique secret for %s", env)
		}
		if c.OAuth.SecretKey == placeholder {
			return fmt.Errorf("OAUTH_SECRET_KEY is using a placeholder default; set a unique secret for %s", env)
		}
	}

	return nil
}

// Helper function for environment variable parsing
func GetEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}

	log.Printf("Environment variable %s is not set. Falling back to default value: %s", key, defaultValue)

	return defaultValue
}

// getEnvAsInt safely parses an environment variable as an integer.
// Returns the parsed integer if the env var exists and is valid,
// otherwise returns the provided default value.
func getEnvAsInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		intValue, err := strconv.Atoi(value)

		if err == nil {
			return intValue
		}

		log.Printf("Environment variable %s is not a valid integer. Falling back to default value: %d", key, defaultValue)
	}

	log.Printf("Environment variable %s is not set. Falling back to default value: %d", key, defaultValue)

	return defaultValue
}

// getEnvAsBool safely parses an environment variable as a boolean.
// Returns the parsed boolean if the env var exists and is valid,
// otherwise returns the provided default value.
func getEnvAsBool(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		boolValue, err := strconv.ParseBool(value)
		if err == nil {
			return boolValue
		}
		log.Printf("Environment variable %s is not a valid boolean. Falling back to default value: %t", key, defaultValue)
	}
	log.Printf("Environment variable %s is not set. Falling back to default value: %t", key, defaultValue)
	return defaultValue
}
