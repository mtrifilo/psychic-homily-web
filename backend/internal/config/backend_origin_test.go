package config

import (
	"os"
	"strings"
	"testing"
)

const (
	productionAPI = "https://api.psychichomily.com"
	stageAPI      = "https://api-stage.psychichomily.com"
)

func TestResolveBackendOrigin(t *testing.T) {
	tests := []struct {
		name             string
		frontendURL      string
		backendPublicURL string
		serverAddr       string
		wantOrigin       string
		wantRecognised   bool
	}{
		// Production runs with FRONTEND_URL=https://www.psychichomily.com and
		// BACKEND_PUBLIC_URL unset.
		{"production www frontend", "https://www.psychichomily.com", "", "0.0.0.0:8080", productionAPI, true},
		{"production apex frontend", "https://psychichomily.com", "", "0.0.0.0:8080", productionAPI, true},
		{"production www frontend trailing slash", "https://www.psychichomily.com/", "", "0.0.0.0:8080", productionAPI, true},
		{"production apex frontend trailing slashes", "https://psychichomily.com//", "", "0.0.0.0:8080", productionAPI, true},
		{"production www frontend surrounding whitespace", " https://www.psychichomily.com\n", "", "0.0.0.0:8080", productionAPI, true},
		{"stage frontend", "https://stage.psychichomily.com", "", "0.0.0.0:8080", stageAPI, true},
		{"stage frontend trailing slash", "https://stage.psychichomily.com/", "", "0.0.0.0:8080", stageAPI, true},

		{"local frontend on the default listen address", "http://localhost:3000", "", "localhost:8080", "http://localhost:8080", true},
		{"local frontend with no listen address", "http://localhost:3000", "", "", "http://localhost:8080", true},
		{"local frontend on another listen port", "http://localhost:3000", "", "localhost:63516", "http://localhost:63516", true},
		{"local frontend on a portless listen address", "http://localhost:3000", "", ":9090", "http://localhost:9090", true},
		{"local frontend with an unparseable listen address", "http://localhost:3000", "", "localhost", "http://localhost:8080", true},
		{"local frontend over https", "https://localhost:3000", "", "localhost:8080", "http://localhost:8080", true},
		{"local frontend without a port", "http://localhost", "", "localhost:8080", "http://localhost:8080", true},
		{"loopback IPv4 frontend", "http://127.0.0.1:3000", "", "localhost:8080", "http://localhost:8080", true},
		{"loopback IPv6 frontend", "http://[::1]:3000", "", "localhost:8080", "http://localhost:8080", true},
		{"dispatch stack", "http://localhost:61000", "http://localhost:61001", "localhost:61001", "http://localhost:61001", true},

		{"BACKEND_PUBLIC_URL wins over the production mapping", "https://www.psychichomily.com", "https://api.example.test", "0.0.0.0:8080", "https://api.example.test", true},
		{"BACKEND_PUBLIC_URL wins over the stage mapping", "https://stage.psychichomily.com", "https://api.example.test", "0.0.0.0:8080", "https://api.example.test", true},
		{"BACKEND_PUBLIC_URL trimmed", "http://localhost:61000", "  http://localhost:61001/ \n", "localhost:61001", "http://localhost:61001", true},
		{"whitespace-only BACKEND_PUBLIC_URL counts as unset", "https://www.psychichomily.com", " \t", "0.0.0.0:8080", productionAPI, true},

		{"unknown host", "https://example.invalid", "", "0.0.0.0:8080", "http://localhost:8080", false},
		{"unknown host with BACKEND_PUBLIC_URL", "https://example.invalid", "https://api.example.invalid", "0.0.0.0:8080", "https://api.example.invalid", false},
		{"lookalike host", "https://psychichomily.com.example.net", "", "0.0.0.0:8080", "http://localhost:8080", false},
		{"production host over http", "http://psychichomily.com", "", "0.0.0.0:8080", "http://localhost:8080", false},
		{"www stage host", "https://www.stage.psychichomily.com", "", "0.0.0.0:8080", "http://localhost:8080", false},
		{"production frontend with a path", "https://www.psychichomily.com/app", "", "0.0.0.0:8080", "http://localhost:8080", false},
		{"local frontend with a path", "http://localhost:3000/app", "", "localhost:8080", "http://localhost:8080", false},
		{"local frontend with userinfo", "http://user@localhost:3000", "", "localhost:8080", "http://localhost:8080", false},
		{"localhost as a subdomain label", "http://localhost.example.com:3000", "", "localhost:8080", "http://localhost:8080", false},
		{"non-http scheme on localhost", "ftp://localhost:3000", "", "localhost:8080", "http://localhost:8080", false},
		{"no scheme", "localhost:3000", "", "localhost:8080", "http://localhost:8080", false},
		{"empty", "", "", "localhost:8080", "http://localhost:8080", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			origin, recognised := ResolveBackendOrigin(tt.frontendURL, tt.backendPublicURL, tt.serverAddr)
			if origin != tt.wantOrigin || recognised != tt.wantRecognised {
				t.Errorf("ResolveBackendOrigin(%q, %q, %q) = (%q, %v), want (%q, %v)",
					tt.frontendURL, tt.backendPublicURL, tt.serverAddr, origin, recognised, tt.wantOrigin, tt.wantRecognised)
			}
		})
	}
}

func TestValidateBackendOrigin(t *testing.T) {
	t.Run("recognised frontends pass with BACKEND_PUBLIC_URL unset", func(t *testing.T) {
		for _, frontendURL := range []string{
			"https://www.psychichomily.com",
			"https://psychichomily.com",
			"https://stage.psychichomily.com",
			"https://www.psychichomily.com/ ",
			"http://localhost:3000",
			"http://127.0.0.1:61000",
		} {
			if err := ValidateBackendOrigin(frontendURL, ""); err != nil {
				t.Errorf("ValidateBackendOrigin(%q, \"\") = %v, want nil", frontendURL, err)
			}
		}
	})

	t.Run("an unrecognised frontend passes with BACKEND_PUBLIC_URL set", func(t *testing.T) {
		if err := ValidateBackendOrigin("https://example.invalid", "https://api.example.invalid"); err != nil {
			t.Errorf("ValidateBackendOrigin = %v, want nil", err)
		}
	})

	t.Run("an unrecognised frontend is refused with the host and the recognised forms named", func(t *testing.T) {
		err := ValidateBackendOrigin("https://example.invalid", "")
		want := `FRONTEND_URL (host "example.invalid") is not a recognised frontend, so backend-hosted links ` +
			`(email unsubscribe links, the calendar feed, OAuth callbacks) cannot name this backend's public origin; ` +
			`set FRONTEND_URL to one of https://psychichomily.com, https://www.psychichomily.com, ` +
			`https://stage.psychichomily.com, or an http(s) origin on localhost, 127.0.0.1 or [::1], ` +
			`or set BACKEND_PUBLIC_URL to this backend's public origin`
		if err == nil || err.Error() != want {
			t.Errorf("ValidateBackendOrigin error = %v\nwant %s", err, want)
		}
	})

	t.Run("whitespace-only BACKEND_PUBLIC_URL counts as unset", func(t *testing.T) {
		if err := ValidateBackendOrigin("https://example.invalid", "  "); err == nil {
			t.Error("ValidateBackendOrigin = nil, want an error")
		}
	})

	t.Run("the error names the host, never userinfo, path or query", func(t *testing.T) {
		err := ValidateBackendOrigin("https://user:probe-secret@example.invalid/probe-path?probe=query", "")
		if err == nil {
			t.Fatal("ValidateBackendOrigin = nil, want an error")
		}
		msg := err.Error()
		if !strings.Contains(msg, `(host "example.invalid")`) {
			t.Errorf("error %q does not name the host", msg)
		}
		for _, leak := range []string{"user", "probe-secret", "probe-path", "probe=query"} {
			if strings.Contains(msg, leak) {
				t.Errorf("error %q carries %q", msg, leak)
			}
		}
	})

	t.Run("an empty or hostless frontend is described without a host", func(t *testing.T) {
		for frontendURL, want := range map[string]string{
			"":               "FRONTEND_URL (empty)",
			"localhost:3000": "FRONTEND_URL (no parseable host)",
			"http://[::1":    "FRONTEND_URL (no parseable host)",
		} {
			err := ValidateBackendOrigin(frontendURL, "")
			if err == nil || !strings.HasPrefix(err.Error(), want) {
				t.Errorf("ValidateBackendOrigin(%q) = %v, want prefix %q", frontendURL, err, want)
			}
		}
	})
}

// The Railway env templates document the FRONTEND_URL each deployed
// environment runs with; with BACKEND_PUBLIC_URL unset each must be
// recognised and resolve to its environment's API host.
func TestResolveBackendOriginRailwayTemplates(t *testing.T) {
	templates := map[string]string{
		"../../.env.railway.prod.template":  productionAPI,
		"../../.env.railway.stage.template": stageAPI,
	}
	for path, want := range templates {
		t.Run(path, func(t *testing.T) {
			body, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read template: %v", err)
			}
			var frontendURL string
			for _, line := range strings.Split(string(body), "\n") {
				if value, found := strings.CutPrefix(strings.TrimSpace(line), EnvFrontendURL+"="); found {
					frontendURL = value
				}
			}
			if frontendURL == "" {
				t.Fatal("template has no FRONTEND_URL line")
			}
			origin, recognised := ResolveBackendOrigin(frontendURL, "", "0.0.0.0:8080")
			if !recognised || origin != want {
				t.Errorf("FRONTEND_URL %q resolves to (%q, %v), want (%q, true)", frontendURL, origin, recognised, want)
			}
			if err := ValidateBackendOrigin(frontendURL, ""); err != nil {
				t.Errorf("ValidateBackendOrigin(%q) = %v, want nil", frontendURL, err)
			}
		})
	}
}

// Load trims FRONTEND_URL once, so the production value with stray
// whitespace still resolves to the production API host.
func TestLoadBackendOriginTrimsFrontendURL(t *testing.T) {
	t.Setenv(EnvEnvironment, EnvProduction)
	t.Setenv(EnvDatabaseURL, "postgres://u:p@db.example.test:5432/d")
	t.Setenv(EnvJWTSecretKey, "deployed-shape-jwt-secret")
	t.Setenv(EnvOAuthSecretKey, "deployed-shape-oauth-secret")
	t.Setenv(EnvInternalAPISecret, strings.Repeat("x", minInternalAPISecretLength))
	t.Setenv(EnvAPIAddr, "0.0.0.0:8080")
	t.Setenv(EnvFrontendURL, "https://www.psychichomily.com \n")
	t.Setenv(EnvBackendPublicURL, "")
	t.Setenv(EnvGoogleCallbackURL, "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() returned error: %v", err)
	}
	if cfg.Email.FrontendURL != "https://www.psychichomily.com" {
		t.Errorf("Email.FrontendURL = %q, want the trimmed value", cfg.Email.FrontendURL)
	}
	if got := cfg.BackendOrigin(); got != productionAPI {
		t.Errorf("BackendOrigin() = %q, want %q", got, productionAPI)
	}
	if got := BackendOriginFromEnv(cfg.Email.FrontendURL); got != productionAPI {
		t.Errorf("BackendOriginFromEnv() = %q, want %q", got, productionAPI)
	}
	if want := productionAPI + "/auth/callback/google"; cfg.OAuth.GoogleCallbackURL != want {
		t.Errorf("GoogleCallbackURL = %q, want %q", cfg.OAuth.GoogleCallbackURL, want)
	}
}
