package config

import (
	"net/http"
	"os"
	"testing"
	"time"
)

func TestGetEnvAsInt(t *testing.T) {
	tests := []struct {
		name         string
		envKey       string
		envValue     string
		defaultValue int
		expected     int
		shouldLog    bool
	}{
		{
			name:         "valid positive integer",
			envKey:       "TEST_PORT",
			envValue:     "8080",
			defaultValue: 3000,
			expected:     8080,
			shouldLog:    false,
		},
		{
			name:         "valid negative integer",
			envKey:       "TEST_TIMEOUT",
			envValue:     "-30",
			defaultValue: 60,
			expected:     -30,
			shouldLog:    false,
		},
		{
			name:         "valid zero",
			envKey:       "TEST_RETRIES",
			envValue:     "0",
			defaultValue: 3,
			expected:     0,
			shouldLog:    false,
		},
		{
			name:         "invalid integer string",
			envKey:       "TEST_PORT",
			envValue:     "not-a-number",
			defaultValue: 3000,
			expected:     3000,
			shouldLog:    true,
		},
		{
			name:         "empty string",
			envKey:       "TEST_PORT",
			envValue:     "",
			defaultValue: 3000,
			expected:     3000,
			shouldLog:    true,
		},
		{
			name:         "decimal number",
			envKey:       "TEST_PORT",
			envValue:     "8080.5",
			defaultValue: 3000,
			expected:     3000,
			shouldLog:    true,
		},
		{
			name:         "large number",
			envKey:       "TEST_LARGE",
			envValue:     "2147483647",
			defaultValue: 1000,
			expected:     2147483647,
			shouldLog:    false,
		},
		{
			name:         "very large number",
			envKey:       "TEST_VERY_LARGE",
			envValue:     "9223372036854775807",
			defaultValue: 1000,
			expected:     9223372036854775807,
			shouldLog:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up environment variable
			if tt.envValue != "" {
				os.Setenv(tt.envKey, tt.envValue)
				defer os.Unsetenv(tt.envKey)
			} else {
				// Ensure the environment variable is not set
				os.Unsetenv(tt.envKey)
			}

			// Call the function
			result := getEnvAsInt(tt.envKey, tt.defaultValue)

			// Check the result
			if result != tt.expected {
				t.Errorf("getEnvAsInt(%q, %d) = %d, want %d", tt.envKey, tt.defaultValue, result, tt.expected)
			}
		})
	}
}

func TestGetEnvAsIntEnvironmentIsolation(t *testing.T) {
	// Test that environment variables don't interfere with each other
	os.Setenv("TEST_A", "100")
	os.Setenv("TEST_B", "200")
	defer func() {
		os.Unsetenv("TEST_A")
		os.Unsetenv("TEST_B")
	}()

	// Test that each variable returns its own value
	if result := getEnvAsInt("TEST_A", 0); result != 100 {
		t.Errorf("getEnvAsInt(\"TEST_A\", 0) = %d, want 100", result)
	}

	if result := getEnvAsInt("TEST_B", 0); result != 200 {
		t.Errorf("getEnvAsInt(\"TEST_B\", 0) = %d, want 200", result)
	}

	// Test that unset variable returns default
	if result := getEnvAsInt("TEST_C", 300); result != 300 {
		t.Errorf("getEnvAsInt(\"TEST_C\", 300) = %d, want 300", result)
	}
}

func TestGetEnvAsIntEdgeCases(t *testing.T) {
	tests := []struct {
		name         string
		envKey       string
		envValue     string
		defaultValue int
		expected     int
	}{
		{
			name:         "whitespace only",
			envKey:       "TEST_WHITESPACE",
			envValue:     "   ",
			defaultValue: 100,
			expected:     100,
		},
		{
			name:         "whitespace around valid number",
			envKey:       "TEST_WHITESPACE_NUMBER",
			envValue:     "  123  ",
			defaultValue: 100,
			expected:     100, // strconv.Atoi doesn't trim whitespace
		},
		{
			name:         "hexadecimal string",
			envKey:       "TEST_HEX",
			envValue:     "0xFF",
			defaultValue: 100,
			expected:     100,
		},
		{
			name:         "binary string",
			envKey:       "TEST_BINARY",
			envValue:     "1010",
			defaultValue: 100,
			expected:     1010, // This is valid as decimal
		},
		{
			name:         "scientific notation",
			envKey:       "TEST_SCIENTIFIC",
			envValue:     "1e3",
			defaultValue: 100,
			expected:     100,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Setenv(tt.envKey, tt.envValue)
			defer os.Unsetenv(tt.envKey)

			result := getEnvAsInt(tt.envKey, tt.defaultValue)

			if result != tt.expected {
				t.Errorf("getEnvAsInt(%q, %d) = %d, want %d", tt.envKey, tt.defaultValue, result, tt.expected)
			}
		})
	}
}

// Benchmark test for performance
func BenchmarkGetEnvAsInt(b *testing.B) {
	os.Setenv("BENCHMARK_TEST", "12345")
	defer os.Unsetenv("BENCHMARK_TEST")

	for i := 0; i < b.N; i++ {
		getEnvAsInt("BENCHMARK_TEST", 0)
	}
}

func BenchmarkGetEnvAsIntDefault(b *testing.B) {
	os.Unsetenv("BENCHMARK_TEST_DEFAULT")

	for i := 0; i < b.N; i++ {
		getEnvAsInt("BENCHMARK_TEST_DEFAULT", 12345)
	}
}

// --- GetEnv tests ---

func TestGetEnv(t *testing.T) {
	t.Run("returns env value when set", func(t *testing.T) {
		t.Setenv("TEST_GETENV", "myvalue")
		if got := GetEnv("TEST_GETENV", "default"); got != "myvalue" {
			t.Errorf("GetEnv() = %q, want %q", got, "myvalue")
		}
	})

	t.Run("returns default when unset", func(t *testing.T) {
		if got := GetEnv("TEST_GETENV_UNSET_12345", "fallback"); got != "fallback" {
			t.Errorf("GetEnv() = %q, want %q", got, "fallback")
		}
	})
}

// --- getEnvAsBool tests ---

func TestGetEnvAsBool(t *testing.T) {
	tests := []struct {
		name     string
		value    string
		defVal   bool
		expected bool
	}{
		{"true string", "true", false, true},
		{"false string", "false", true, false},
		{"1 string", "1", false, true},
		{"0 string", "0", true, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("TEST_BOOL", tt.value)
			if got := getEnvAsBool("TEST_BOOL", tt.defVal); got != tt.expected {
				t.Errorf("getEnvAsBool(%q, %v) = %v, want %v", tt.value, tt.defVal, got, tt.expected)
			}
		})
	}

	t.Run("invalid value returns default", func(t *testing.T) {
		t.Setenv("TEST_BOOL_INVALID", "notabool")
		if got := getEnvAsBool("TEST_BOOL_INVALID", true); got != true {
			t.Errorf("getEnvAsBool(notabool, true) = %v, want true", got)
		}
	})

	t.Run("unset returns default", func(t *testing.T) {
		if got := getEnvAsBool("TEST_BOOL_MISSING_12345", false); got != false {
			t.Errorf("getEnvAsBool(unset, false) = %v, want false", got)
		}
	})
}

// --- GetSameSite tests ---

func TestGetSameSite(t *testing.T) {
	tests := []struct {
		name     string
		value    string
		expected http.SameSite
	}{
		{"strict", "strict", http.SameSiteStrictMode},
		{"Strict uppercase", "Strict", http.SameSiteStrictMode},
		{"none", "none", http.SameSiteNoneMode},
		{"lax", "lax", http.SameSiteLaxMode},
		{"empty defaults to lax", "", http.SameSiteLaxMode},
		{"unknown defaults to lax", "unknown", http.SameSiteLaxMode},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := SessionConfig{SameSite: tt.value}
			if got := s.GetSameSite(); got != tt.expected {
				t.Errorf("GetSameSite() with %q = %v, want %v", tt.value, got, tt.expected)
			}
		})
	}
}

// --- NewAuthCookie tests ---

func TestNewAuthCookie(t *testing.T) {
	s := SessionConfig{
		Path:     "/",
		Domain:   "example.com",
		HttpOnly: true,
		Secure:   true,
		SameSite: "strict",
	}

	before := time.Now()
	cookie := s.NewAuthCookie("test-token", 24*time.Hour)
	after := time.Now()

	if cookie.Name != AuthCookieName {
		t.Errorf("Name = %q, want %q", cookie.Name, AuthCookieName)
	}
	if cookie.Value != "test-token" {
		t.Errorf("Value = %q, want test-token", cookie.Value)
	}
	if cookie.Path != "/" {
		t.Errorf("Path = %q, want /", cookie.Path)
	}
	if cookie.Domain != "example.com" {
		t.Errorf("Domain = %q, want example.com", cookie.Domain)
	}
	if !cookie.HttpOnly {
		t.Error("HttpOnly should be true")
	}
	if !cookie.Secure {
		t.Error("Secure should be true")
	}
	if cookie.SameSite != http.SameSiteStrictMode {
		t.Errorf("SameSite = %v, want StrictMode", cookie.SameSite)
	}
	// Expires should be approximately now + 24h
	expectedMin := before.Add(24 * time.Hour)
	expectedMax := after.Add(24 * time.Hour)
	if cookie.Expires.Before(expectedMin) || cookie.Expires.After(expectedMax) {
		t.Errorf("Expires = %v, expected between %v and %v", cookie.Expires, expectedMin, expectedMax)
	}
}

// --- ClearAuthCookie tests ---

func TestClearAuthCookie(t *testing.T) {
	s := SessionConfig{
		Path:     "/",
		Domain:   "example.com",
		HttpOnly: true,
		Secure:   true,
		SameSite: "lax",
	}

	cookie := s.ClearAuthCookie()

	if cookie.Name != AuthCookieName {
		t.Errorf("Name = %q, want %q", cookie.Name, AuthCookieName)
	}
	if cookie.Value != "" {
		t.Errorf("Value = %q, want empty", cookie.Value)
	}
	if cookie.MaxAge != -1 {
		t.Errorf("MaxAge = %d, want -1", cookie.MaxAge)
	}
	if !cookie.Expires.Equal(time.Unix(0, 0)) {
		t.Errorf("Expires = %v, want Unix epoch", cookie.Expires)
	}
	if cookie.SameSite != http.SameSiteStrictMode {
		t.Errorf("SameSite = %v, want StrictMode (hardcoded for clear)", cookie.SameSite)
	}
}

// --- Validate tests ---

func TestValidate(t *testing.T) {
	t.Run("development env always passes", func(t *testing.T) {
		t.Setenv("ENVIRONMENT", "development")
		cfg := &Config{
			JWT:   JWTConfig{SecretKey: "your-super-secret-jwt-key-32-chars-minimum"},
			OAuth: OAuthConfig{SecretKey: "your-secret-key-here"},
		}
		if err := cfg.Validate(); err != nil {
			t.Errorf("expected nil error in development, got: %v", err)
		}
	})

	t.Run("no env with localhost DB passes", func(t *testing.T) {
		t.Setenv("ENVIRONMENT", "")
		t.Setenv("DATABASE_URL", "postgres://user:pass@localhost:5432/db")
		cfg := &Config{
			JWT:   JWTConfig{SecretKey: "placeholder"},
			OAuth: OAuthConfig{SecretKey: "placeholder"},
		}
		if err := cfg.Validate(); err != nil {
			t.Errorf("expected nil error for localhost DB, got: %v", err)
		}
	})

	t.Run("no env with remote DB errors", func(t *testing.T) {
		t.Setenv("ENVIRONMENT", "")
		t.Setenv("DATABASE_URL", "postgres://user:pass@db.example.com:5432/db")
		cfg := &Config{}
		if err := cfg.Validate(); err == nil {
			t.Error("expected error for remote DB without ENVIRONMENT, got nil")
		}
	})

	t.Run("production with placeholder JWT secret errors", func(t *testing.T) {
		t.Setenv("ENVIRONMENT", "production")
		cfg := &Config{
			JWT:   JWTConfig{SecretKey: "your-super-secret-jwt-key-32-chars-minimum"},
			OAuth: OAuthConfig{SecretKey: "real-secret"},
		}
		err := cfg.Validate()
		if err == nil {
			t.Error("expected error for placeholder JWT secret, got nil")
		}
	})

	t.Run("production with placeholder OAuth secret errors", func(t *testing.T) {
		t.Setenv("ENVIRONMENT", "production")
		cfg := &Config{
			JWT:   JWTConfig{SecretKey: "real-jwt-secret"},
			OAuth: OAuthConfig{SecretKey: "your-secret-key-here"},
		}
		err := cfg.Validate()
		if err == nil {
			t.Error("expected error for placeholder OAuth secret, got nil")
		}
	})

	t.Run("production with real secrets passes", func(t *testing.T) {
		t.Setenv("ENVIRONMENT", "production")
		cfg := &Config{
			JWT:   JWTConfig{SecretKey: "a-real-production-jwt-secret-key"},
			OAuth: OAuthConfig{SecretKey: "a-real-production-oauth-secret"},
		}
		if err := cfg.Validate(); err != nil {
			t.Errorf("expected nil error with real secrets, got: %v", err)
		}
	})
}

// --- getFrontendURL tests ---

func TestGetFrontendURL(t *testing.T) {
	t.Run("production", func(t *testing.T) {
		t.Setenv("ENVIRONMENT", "production")
		t.Setenv("FRONTEND_URL", "")
		if got := getFrontendURL(); got != "https://psychichomily.com" {
			t.Errorf("getFrontendURL() = %q, want https://psychichomily.com", got)
		}
	})

	t.Run("stage", func(t *testing.T) {
		t.Setenv("ENVIRONMENT", "stage")
		t.Setenv("FRONTEND_URL", "")
		if got := getFrontendURL(); got != "https://stage.psychichomily.com" {
			t.Errorf("getFrontendURL() = %q, want https://stage.psychichomily.com", got)
		}
	})

	t.Run("unset defaults to localhost", func(t *testing.T) {
		t.Setenv("ENVIRONMENT", "")
		t.Setenv("FRONTEND_URL", "")
		if got := getFrontendURL(); got != "http://localhost:3000" {
			t.Errorf("getFrontendURL() = %q, want http://localhost:3000", got)
		}
	})

	t.Run("explicit FRONTEND_URL overrides", func(t *testing.T) {
		t.Setenv("ENVIRONMENT", "production")
		t.Setenv("FRONTEND_URL", "https://custom.example.com")
		if got := getFrontendURL(); got != "https://custom.example.com" {
			t.Errorf("getFrontendURL() = %q, want https://custom.example.com", got)
		}
	})
}

// --- getCORSOrigins tests ---

func TestGetCORSOrigins(t *testing.T) {
	t.Run("custom env var split by comma", func(t *testing.T) {
		t.Setenv("CORS_ALLOWED_ORIGINS", "https://a.com,https://b.com")
		t.Setenv("ENVIRONMENT", "")
		origins := getCORSOrigins()
		if len(origins) != 2 {
			t.Fatalf("expected 2 origins, got %d", len(origins))
		}
		if origins[0] != "https://a.com" || origins[1] != "https://b.com" {
			t.Errorf("origins = %v, want [https://a.com https://b.com]", origins)
		}
	})

	t.Run("production defaults", func(t *testing.T) {
		t.Setenv("CORS_ALLOWED_ORIGINS", "")
		t.Setenv("ENVIRONMENT", "production")
		origins := getCORSOrigins()
		if len(origins) != 2 {
			t.Fatalf("expected 2 production origins, got %d: %v", len(origins), origins)
		}
		if origins[0] != "https://psychichomily.com" {
			t.Errorf("origins[0] = %q, want https://psychichomily.com", origins[0])
		}
	})

	t.Run("stage defaults", func(t *testing.T) {
		t.Setenv("CORS_ALLOWED_ORIGINS", "")
		t.Setenv("ENVIRONMENT", "stage")
		origins := getCORSOrigins()
		if len(origins) != 2 {
			t.Fatalf("expected 2 stage origins, got %d: %v", len(origins), origins)
		}
		if origins[0] != "https://stage.psychichomily.com" {
			t.Errorf("origins[0] = %q, want https://stage.psychichomily.com", origins[0])
		}
	})

	t.Run("development defaults", func(t *testing.T) {
		t.Setenv("CORS_ALLOWED_ORIGINS", "")
		t.Setenv("ENVIRONMENT", "development")
		origins := getCORSOrigins()
		if len(origins) != 3 {
			t.Fatalf("expected 3 development origins, got %d: %v", len(origins), origins)
		}
	})

	t.Run("unset env includes all defaults", func(t *testing.T) {
		t.Setenv("CORS_ALLOWED_ORIGINS", "")
		t.Setenv("ENVIRONMENT", "")
		origins := getCORSOrigins()
		if len(origins) != 5 {
			t.Fatalf("expected 5 default origins, got %d: %v", len(origins), origins)
		}
	})
}

// --- getWebAuthnRPID tests ---

func TestGetWebAuthnRPID(t *testing.T) {
	t.Run("production", func(t *testing.T) {
		t.Setenv("WEBAUTHN_RP_ID", "")
		t.Setenv("ENVIRONMENT", "production")
		if got := getWebAuthnRPID(); got != "psychichomily.com" {
			t.Errorf("getWebAuthnRPID() = %q, want psychichomily.com", got)
		}
	})

	t.Run("stage", func(t *testing.T) {
		t.Setenv("WEBAUTHN_RP_ID", "")
		t.Setenv("ENVIRONMENT", "stage")
		if got := getWebAuthnRPID(); got != "stage.psychichomily.com" {
			t.Errorf("getWebAuthnRPID() = %q, want stage.psychichomily.com", got)
		}
	})

	t.Run("unset defaults to localhost", func(t *testing.T) {
		t.Setenv("WEBAUTHN_RP_ID", "")
		t.Setenv("ENVIRONMENT", "")
		if got := getWebAuthnRPID(); got != "localhost" {
			t.Errorf("getWebAuthnRPID() = %q, want localhost", got)
		}
	})

	t.Run("explicit env var overrides", func(t *testing.T) {
		t.Setenv("WEBAUTHN_RP_ID", "custom.example.com")
		t.Setenv("ENVIRONMENT", "production")
		if got := getWebAuthnRPID(); got != "custom.example.com" {
			t.Errorf("getWebAuthnRPID() = %q, want custom.example.com", got)
		}
	})
}

// --- getWebAuthnOrigins tests ---

func TestGetWebAuthnOrigins(t *testing.T) {
	t.Run("explicit env var split by comma", func(t *testing.T) {
		t.Setenv("WEBAUTHN_RP_ORIGINS", "https://a.com,https://b.com")
		origins := getWebAuthnOrigins()
		if len(origins) != 2 {
			t.Fatalf("expected 2 origins, got %d", len(origins))
		}
		if origins[0] != "https://a.com" || origins[1] != "https://b.com" {
			t.Errorf("origins = %v, want [https://a.com https://b.com]", origins)
		}
	})

	t.Run("unset defaults to frontend URL", func(t *testing.T) {
		t.Setenv("WEBAUTHN_RP_ORIGINS", "")
		t.Setenv("FRONTEND_URL", "")
		t.Setenv("ENVIRONMENT", "production")
		origins := getWebAuthnOrigins()
		if len(origins) != 1 {
			t.Fatalf("expected 1 origin, got %d: %v", len(origins), origins)
		}
		if origins[0] != "https://psychichomily.com" {
			t.Errorf("origins[0] = %q, want https://psychichomily.com", origins[0])
		}
	})

	t.Run("unset defaults to localhost in dev", func(t *testing.T) {
		t.Setenv("WEBAUTHN_RP_ORIGINS", "")
		t.Setenv("FRONTEND_URL", "")
		t.Setenv("ENVIRONMENT", "")
		origins := getWebAuthnOrigins()
		if len(origins) != 1 {
			t.Fatalf("expected 1 origin, got %d: %v", len(origins), origins)
		}
		if origins[0] != "http://localhost:3000" {
			t.Errorf("origins[0] = %q, want http://localhost:3000", origins[0])
		}
	})
}

// --- Load tests ---

func TestLoad(t *testing.T) {
	t.Run("loads with development defaults", func(t *testing.T) {
		t.Setenv("ENVIRONMENT", "development")
		// Clear any env vars that might interfere
		t.Setenv("DATABASE_URL", "")
		t.Setenv("JWT_SECRET_KEY", "")
		t.Setenv("OAUTH_SECRET_KEY", "")

		cfg, err := Load()
		if err != nil {
			t.Fatalf("Load() returned error: %v", err)
		}

		// Verify default values
		if cfg.Server.Addr != "localhost:8080" {
			t.Errorf("Server.Addr = %q, want localhost:8080", cfg.Server.Addr)
		}
		if cfg.Session.Path != "/" {
			t.Errorf("Session.Path = %q, want /", cfg.Session.Path)
		}
		if cfg.Session.MaxAge != 86400 {
			t.Errorf("Session.MaxAge = %d, want 86400", cfg.Session.MaxAge)
		}
		if !cfg.Session.HttpOnly {
			t.Error("Session.HttpOnly should be true by default")
		}
		if cfg.JWT.Expiry != 24 {
			t.Errorf("JWT.Expiry = %d, want 24", cfg.JWT.Expiry)
		}
		if cfg.Email.FromEmail != "noreply@psychichomily.com" {
			t.Errorf("Email.FromEmail = %q, want noreply@psychichomily.com", cfg.Email.FromEmail)
		}
		if cfg.WebAuthn.RPDisplayName != "Psychic Homily" {
			t.Errorf("WebAuthn.RPDisplayName = %q, want 'Psychic Homily'", cfg.WebAuthn.RPDisplayName)
		}
		if cfg.Apple.BundleID != "com.psychichomily.ios" {
			t.Errorf("Apple.BundleID = %q, want com.psychichomily.ios", cfg.Apple.BundleID)
		}
		if !cfg.CORS.AllowCredentials {
			t.Error("CORS.AllowCredentials should be true")
		}
	})

	t.Run("loads with custom env vars", func(t *testing.T) {
		t.Setenv("ENVIRONMENT", "development")
		t.Setenv("API_ADDR", "0.0.0.0:9090")
		t.Setenv("JWT_EXPIRY_HOURS", "48")
		t.Setenv("SESSION_MAX_AGE", "3600")
		t.Setenv("SESSION_SECURE", "true")
		t.Setenv("DISCORD_NOTIFICATIONS_ENABLED", "true")

		cfg, err := Load()
		if err != nil {
			t.Fatalf("Load() returned error: %v", err)
		}

		if cfg.Server.Addr != "0.0.0.0:9090" {
			t.Errorf("Server.Addr = %q, want 0.0.0.0:9090", cfg.Server.Addr)
		}
		if cfg.JWT.Expiry != 48 {
			t.Errorf("JWT.Expiry = %d, want 48", cfg.JWT.Expiry)
		}
		if cfg.Session.MaxAge != 3600 {
			t.Errorf("Session.MaxAge = %d, want 3600", cfg.Session.MaxAge)
		}
		if !cfg.Session.Secure {
			t.Error("Session.Secure should be true")
		}
		if !cfg.Discord.Enabled {
			t.Error("Discord.Enabled should be true")
		}
	})

	t.Run("validation failure returns error", func(t *testing.T) {
		t.Setenv("ENVIRONMENT", "production")
		// Leave JWT and OAuth secrets as placeholders
		t.Setenv("JWT_SECRET_KEY", "")
		t.Setenv("OAUTH_SECRET_KEY", "")

		_, err := Load()
		if err == nil {
			t.Error("expected validation error for production with placeholder secrets, got nil")
		}
	})
}
