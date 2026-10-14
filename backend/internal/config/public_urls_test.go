package config

import (
	"slices"
	"strings"
	"testing"
)

func TestBackendPublicURL(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  string
	}{
		{"unset defaults to localhost:8080", "", "http://localhost:8080"},
		{"explicit value", "http://localhost:61001", "http://localhost:61001"},
		{"trailing slash trimmed", "http://localhost:61001/", "http://localhost:61001"},
		{"surrounding whitespace trimmed", "  http://localhost:61001  ", "http://localhost:61001"},
		{"whitespace only counts as unset", "   ", "http://localhost:8080"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(EnvBackendPublicURL, tt.value)
			if got := BackendPublicURL(); got != tt.want {
				t.Errorf("BackendPublicURL() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestGetCORSOriginsFrontendURLDefault(t *testing.T) {
	unsetDefaults := []string{
		"https://psychichomily.com", "https://www.psychichomily.com",
		"http://localhost:3000", "http://localhost:5173", "http://localhost:1313",
	}
	devDefaults := []string{"http://localhost:3000", "http://localhost:5173", "http://localhost:1313"}

	tests := []struct {
		name        string
		environment string
		cors        string
		frontendURL string
		want        []string
	}{
		{
			name:        "only FRONTEND_URL set adds its origin to the defaults",
			frontendURL: "http://localhost:61000",
			want:        append(slices.Clone(unsetDefaults), "http://localhost:61000"),
		},
		{
			name:        "dispatch stack shape adds the stack frontend",
			environment: "test",
			frontendURL: "http://localhost:61000",
			want:        append(slices.Clone(unsetDefaults), "http://localhost:61000"),
		},
		{
			name:        "explicit CORS_ALLOWED_ORIGINS wins over FRONTEND_URL",
			cors:        "https://a.example,https://b.example",
			frontendURL: "http://localhost:61000",
			want:        []string{"https://a.example", "https://b.example"},
		},
		{
			name:        "path and trailing slash reduce to the origin",
			environment: "development",
			frontendURL: "http://localhost:61000/app/",
			want:        append(slices.Clone(devDefaults), "http://localhost:61000"),
		},
		{
			name:        "unset FRONTEND_URL leaves the defaults unchanged",
			environment: "development",
			want:        devDefaults,
		},
		{
			name:        "production frontend already in the defaults is not repeated",
			environment: "production",
			frontendURL: "https://www.psychichomily.com",
			want:        []string{"https://psychichomily.com", "https://www.psychichomily.com"},
		},
		{
			name:        "stage frontend already in the defaults is not repeated",
			environment: "stage",
			frontendURL: "https://stage.psychichomily.com",
			want:        []string{"https://stage.psychichomily.com", "https://www.stage.psychichomily.com"},
		},
		{
			name:        "FRONTEND_URL without a scheme adds nothing",
			environment: "development",
			frontendURL: "localhost:61000",
			want:        devDefaults,
		},
		{
			name:        "FRONTEND_URL without a host adds nothing",
			environment: "development",
			frontendURL: "http://",
			want:        devDefaults,
		},
		{
			name:        "unparseable FRONTEND_URL adds nothing",
			environment: "development",
			frontendURL: "http://[::1",
			want:        devDefaults,
		},
		{
			name:        "surrounding whitespace is trimmed",
			environment: "development",
			frontendURL: "  http://localhost:61000  ",
			want:        append(slices.Clone(devDefaults), "http://localhost:61000"),
		},
		{
			name:        "host case is kept as configured",
			environment: "development",
			frontendURL: "http://LOCALHOST:61000",
			want:        append(slices.Clone(devDefaults), "http://LOCALHOST:61000"),
		},
		{
			name: "unset FRONTEND_URL and ENVIRONMENT leave the defaults unchanged",
			want: unsetDefaults,
		},
		{
			name:        "unset FRONTEND_URL in production leaves the defaults unchanged",
			environment: "production",
			want:        []string{"https://psychichomily.com", "https://www.psychichomily.com"},
		},
		{
			name:        "unset FRONTEND_URL in stage leaves the defaults unchanged",
			environment: "stage",
			want:        []string{"https://stage.psychichomily.com", "https://www.stage.psychichomily.com"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(EnvEnvironment, tt.environment)
			t.Setenv(EnvCORSAllowedOrigins, tt.cors)
			t.Setenv(EnvFrontendURL, tt.frontendURL)
			if got := getCORSOrigins(); !slices.Equal(got, tt.want) {
				t.Errorf("getCORSOrigins() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestLoadOAuthCallbackDefaults(t *testing.T) {
	setDev := func(t *testing.T) {
		t.Helper()
		t.Setenv("ENVIRONMENT", "development")
		t.Setenv("DATABASE_URL", "")
		t.Setenv("GOOGLE_CALLBACK_URL", "")
		t.Setenv("GITHUB_CALLBACK_URL", "")
		t.Setenv(EnvBackendPublicURL, "")
	}
	assertCallbacks := func(t *testing.T, cfg *Config, wantGoogle, wantGitHub string) {
		t.Helper()
		if cfg.OAuth.GoogleCallbackURL != wantGoogle {
			t.Errorf("GoogleCallbackURL = %q, want %q", cfg.OAuth.GoogleCallbackURL, wantGoogle)
		}
		if cfg.OAuth.GitHubCallbackURL != wantGitHub {
			t.Errorf("GitHubCallbackURL = %q, want %q", cfg.OAuth.GitHubCallbackURL, wantGitHub)
		}
	}

	t.Run("unset defaults to localhost:8080", func(t *testing.T) {
		setDev(t)
		cfg, err := Load()
		if err != nil {
			t.Fatalf("Load() returned error: %v", err)
		}
		assertCallbacks(t, cfg,
			"http://localhost:8080/auth/callback/google",
			"http://localhost:8080/auth/callback/github")
	})

	t.Run("BACKEND_PUBLIC_URL sets the default", func(t *testing.T) {
		setDev(t)
		t.Setenv(EnvBackendPublicURL, "http://localhost:61001")
		cfg, err := Load()
		if err != nil {
			t.Fatalf("Load() returned error: %v", err)
		}
		assertCallbacks(t, cfg,
			"http://localhost:61001/auth/callback/google",
			"http://localhost:61001/auth/callback/github")
	})

	t.Run("explicit callback URLs win over BACKEND_PUBLIC_URL", func(t *testing.T) {
		setDev(t)
		t.Setenv(EnvBackendPublicURL, "http://localhost:61001")
		t.Setenv("GOOGLE_CALLBACK_URL", "https://api.example.test/auth/callback/google")
		t.Setenv("GITHUB_CALLBACK_URL", "https://api.example.test/auth/callback/github")
		cfg, err := Load()
		if err != nil {
			t.Fatalf("Load() returned error: %v", err)
		}
		assertCallbacks(t, cfg,
			"https://api.example.test/auth/callback/google",
			"https://api.example.test/auth/callback/github")
	})
}

// TestLoadDeployedShapes sets the variable names production and stage set
// (ENVIRONMENT, FRONTEND_URL, API_ADDR, CORS_ALLOWED_ORIGINS and both callback
// URLs; never BACKEND_PUBLIC_URL) and asserts the explicit values pass
// through unchanged.
func TestLoadDeployedShapes(t *testing.T) {
	shapes := []struct {
		environment string
		frontendURL string
	}{
		{"production", "https://www.psychichomily.com"},
		{"stage", "https://stage.psychichomily.com"},
	}
	for _, sh := range shapes {
		t.Run(sh.environment, func(t *testing.T) {
			t.Setenv("ENVIRONMENT", sh.environment)
			t.Setenv("DATABASE_URL", "postgres://u:p@db.example.test:5432/d")
			t.Setenv("JWT_SECRET_KEY", "deployed-shape-jwt-secret")
			t.Setenv("OAUTH_SECRET_KEY", "deployed-shape-oauth-secret")
			t.Setenv("INTERNAL_API_SECRET", strings.Repeat("x", minInternalAPISecretLength))
			t.Setenv("API_ADDR", "0.0.0.0:8080")
			t.Setenv("FRONTEND_URL", sh.frontendURL)
			t.Setenv("CORS_ALLOWED_ORIGINS", "https://one.example.test,https://two.example.test")
			t.Setenv("GOOGLE_CALLBACK_URL", "https://api.example.test/auth/callback/google")
			t.Setenv("GITHUB_CALLBACK_URL", "https://api.example.test/auth/callback/github")
			t.Setenv(EnvBackendPublicURL, "")

			cfg, err := Load()
			if err != nil {
				t.Fatalf("Load() returned error: %v", err)
			}
			wantCORS := []string{"https://one.example.test", "https://two.example.test"}
			if !slices.Equal(cfg.CORS.AllowedOrigins, wantCORS) {
				t.Errorf("CORS.AllowedOrigins = %v, want %v", cfg.CORS.AllowedOrigins, wantCORS)
			}
			if cfg.OAuth.GoogleCallbackURL != "https://api.example.test/auth/callback/google" {
				t.Errorf("GoogleCallbackURL = %q", cfg.OAuth.GoogleCallbackURL)
			}
			if cfg.OAuth.GitHubCallbackURL != "https://api.example.test/auth/callback/github" {
				t.Errorf("GitHubCallbackURL = %q", cfg.OAuth.GitHubCallbackURL)
			}
			if cfg.Email.FrontendURL != sh.frontendURL {
				t.Errorf("Email.FrontendURL = %q, want %q", cfg.Email.FrontendURL, sh.frontendURL)
			}
		})
	}
}

// TestLoadDeployedShapesWithoutExplicitValues drops CORS_ALLOWED_ORIGINS and
// the callback URLs from the deployed shapes and asserts the fallbacks: the
// environment's CORS defaults (which already hold the frontend origin) and
// localhost:8080 callbacks.
func TestLoadDeployedShapesWithoutExplicitValues(t *testing.T) {
	shapes := []struct {
		environment string
		frontendURL string
		wantCORS    []string
	}{
		{"production", "https://www.psychichomily.com", []string{"https://psychichomily.com", "https://www.psychichomily.com"}},
		{"stage", "https://stage.psychichomily.com", []string{"https://stage.psychichomily.com", "https://www.stage.psychichomily.com"}},
	}
	for _, sh := range shapes {
		t.Run(sh.environment, func(t *testing.T) {
			t.Setenv(EnvEnvironment, sh.environment)
			t.Setenv(EnvDatabaseURL, "postgres://u:p@db.example.test:5432/d")
			t.Setenv(EnvJWTSecretKey, "deployed-shape-jwt-secret")
			t.Setenv(EnvOAuthSecretKey, "deployed-shape-oauth-secret")
			t.Setenv(EnvInternalAPISecret, strings.Repeat("x", minInternalAPISecretLength))
			t.Setenv(EnvAPIAddr, "0.0.0.0:8080")
			t.Setenv(EnvFrontendURL, sh.frontendURL)
			t.Setenv(EnvCORSAllowedOrigins, "")
			t.Setenv(EnvGoogleCallbackURL, "")
			t.Setenv(EnvGitHubCallbackURL, "")
			t.Setenv(EnvBackendPublicURL, "")

			cfg, err := Load()
			if err != nil {
				t.Fatalf("Load() returned error: %v", err)
			}
			if !slices.Equal(cfg.CORS.AllowedOrigins, sh.wantCORS) {
				t.Errorf("CORS.AllowedOrigins = %v, want %v", cfg.CORS.AllowedOrigins, sh.wantCORS)
			}
			if cfg.OAuth.GoogleCallbackURL != "http://localhost:8080/auth/callback/google" {
				t.Errorf("GoogleCallbackURL = %q", cfg.OAuth.GoogleCallbackURL)
			}
			if cfg.OAuth.GitHubCallbackURL != "http://localhost:8080/auth/callback/github" {
				t.Errorf("GitHubCallbackURL = %q", cfg.OAuth.GitHubCallbackURL)
			}
		})
	}
}
