package config

import (
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
)

// Config holds all configuration for the application
type Config struct {
	Server   ServerConfig
	CORS     CORSConfig
	OAuth    OAuthConfig
	Database DatabaseConfig
	JWT      JWTConfig
	Session  SessionConfig
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
	GoogleClientID        string `env:"GOOGLE_CLIENT_ID"`
	GoogleClientSecret    string `env:"GOOGLE_CLIENT_SECRET"`
	GoogleCallbackURL     string `env:"GOOGLE_CALLBACK_URL" envDefault:"http://localhost:8080/auth/callback/google"`
	GitHubClientID        string `env:"GITHUB_CLIENT_ID"`
	GitHubClientSecret    string `env:"GITHUB_CLIENT_SECRET"`
	GitHubCallbackURL     string `env:"GITHUB_CALLBACK_URL" envDefault:"http://localhost:8080/auth/callback/github"`
	SecretKey             string `env:"OAUTH_SECRET_KEY" envDefault:"your-secret-key-change-in-production"`
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

// Update your Load() function to make CORS configurable
func Load() *Config {
	// Debug: Check if OAUTH_SECRET_KEY is being loaded
	oauthSecretKey := getEnv("OAUTH_SECRET_KEY", "your-secret-key-here")
	log.Printf("DEBUG: OAUTH_SECRET_KEY loaded: %s (length: %d)", 
		oauthSecretKey, len(oauthSecretKey))
	
	// Get CORS origins from environment or use defaults
	corsOrigins := getCORSOrigins()
	
	return &Config{
		Server: ServerConfig{
			Addr: getEnv("API_ADDR", "localhost:8080"),
		},
		CORS: CORSConfig{
			AllowedOrigins:   corsOrigins,
			AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
			AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-Requested-With"},
			AllowCredentials: true,
		},
		OAuth: OAuthConfig{
			GoogleClientID:        getEnv("GOOGLE_CLIENT_ID", ""),
			GoogleClientSecret:    getEnv("GOOGLE_CLIENT_SECRET", ""),
			GoogleCallbackURL:     getEnv("GOOGLE_CALLBACK_URL", "http://localhost:8080/auth/callback/google"),
			GitHubClientID:        getEnv("GITHUB_CLIENT_ID", ""),
			GitHubClientSecret:    getEnv("GITHUB_CLIENT_SECRET", ""),
			GitHubCallbackURL:     getEnv("GITHUB_CALLBACK_URL", "http://localhost:8080/auth/callback/github"),
			SecretKey:             oauthSecretKey,
		},
		Database: DatabaseConfig{
			URL: getEnv("DATABASE_URL", "postgres://psychicadmin:secretpassword@localhost:5432/psychicdb?sslmode=disable"),
		},
		JWT: JWTConfig{
			SecretKey: getEnv("JWT_SECRET_KEY", "your-super-secret-jwt-key-32-chars-minimum"),
			Expiry:    int64(getEnvAsInt("JWT_EXPIRY_HOURS", 24)),
		},
		Session: SessionConfig{
			Path:     getEnv("SESSION_PATH", "/"),
			Domain:   getEnv("SESSION_DOMAIN", ""),
			MaxAge:   getEnvAsInt("SESSION_MAX_AGE", 86400),
			HttpOnly: getEnvAsBool("SESSION_HTTP_ONLY", true),
			Secure:   getEnvAsBool("SESSION_SECURE", false),
			SameSite: getEnv("SESSION_SAME_SITE", "lax"),
		},
	}
}

// Add this helper function
func getCORSOrigins() []string {
	if corsEnv := os.Getenv("CORS_ALLOWED_ORIGINS"); corsEnv != "" {
		return strings.Split(corsEnv, ",")
	}
	
	// Default origins based on environment
	if os.Getenv("NODE_ENV") == "production" {
		return []string{
			"https://psychichomily.com",
			"https://www.psychichomily.com",
		}
	}
	
	// Development defaults
	return []string{
		"https://psychichomily.com",
		"https://www.psychichomily.com", 
		"http://localhost:3000",
		"http://localhost:5173",
	}
}

// Helper function for environment variable parsing
func getEnv(key, defaultValue string) string {
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
