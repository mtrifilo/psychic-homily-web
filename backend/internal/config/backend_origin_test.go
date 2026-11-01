package config

import (
	"os"
	"strings"
	"testing"
)

const (
	productionAPI = "https://api.psychichomily.com"
	stageAPI      = "https://stage.api.psychichomily.com"
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
		{"frontend on a name under .localhost", "http://app.localhost:3000", "", "localhost:8080", "http://localhost:8080", true},
		{"listen address with a scheme falls back to 8080", "http://localhost:3000", "", "http://localhost:63516/", "http://localhost:8080", true},
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
			if origin := ResolveBackendOrigin(tt.frontendURL, tt.backendPublicURL, tt.serverAddr); origin != tt.wantOrigin {
				t.Errorf("ResolveBackendOrigin(%q, %q, %q) = %q, want %q",
					tt.frontendURL, tt.backendPublicURL, tt.serverAddr, origin, tt.wantOrigin)
			}
			if recognised := isRecognisedFrontend(tt.frontendURL, false); recognised != tt.wantRecognised {
				t.Errorf("isRecognisedFrontend(%q, false) = %v, want %v", tt.frontendURL, recognised, tt.wantRecognised)
			}
		})
	}
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
			if origin := ResolveBackendOrigin(frontendURL, "", "0.0.0.0:8080"); origin != want {
				t.Errorf("FRONTEND_URL %q resolves to %q, want %q", frontendURL, origin, want)
			}
			environment := EnvProduction
			if strings.Contains(path, "stage") {
				environment = EnvStage
			}
			if err := ValidateBackendOrigin(environment, frontendURL, ""); err != nil {
				t.Errorf("ValidateBackendOrigin(%q, %q, \"\") = %v, want nil", environment, frontendURL, err)
			}
		})
	}
}

func TestValidateBackendOrigin(t *testing.T) {
	passes := []struct {
		name, environment, frontendURL, backendPublicURL string
	}{
		// Production runs with FRONTEND_URL=https://www.psychichomily.com and
		// BACKEND_PUBLIC_URL unset.
		{"production www frontend", EnvProduction, "https://www.psychichomily.com", ""},
		{"production apex frontend", EnvProduction, "https://psychichomily.com", ""},
		{"production www frontend with stray whitespace", EnvProduction, "https://www.psychichomily.com/ ", ""},
		{"stage frontend", EnvStage, "https://stage.psychichomily.com", ""},
		{"local frontend in development", EnvDevelopment, "http://localhost:3000", ""},
		{"local frontend with ENVIRONMENT unset", "", "http://127.0.0.1:61000", ""},
		{"dispatch stack", "test", "http://localhost:61000", "http://localhost:61001"},
		{"loopback override outside production and stage", EnvDevelopment, "https://example.invalid", "http://localhost:8080"},
		{"unrecognised frontend with an https override in production", EnvProduction, "https://example.invalid", "https://api.example.invalid"},
		{"override with a trailing slash", EnvProduction, "https://www.psychichomily.com", "https://api.example.invalid/"},
	}
	for _, tt := range passes {
		t.Run("passes: "+tt.name, func(t *testing.T) {
			if err := ValidateBackendOrigin(tt.environment, tt.frontendURL, tt.backendPublicURL); err != nil {
				t.Errorf("ValidateBackendOrigin(%q, %q, %q) = %v, want nil", tt.environment, tt.frontendURL, tt.backendPublicURL, err)
			}
		})
	}

	refusals := []struct {
		name, environment, frontendURL, backendPublicURL, wantPrefix string
	}{
		{"unknown frontend in production", EnvProduction, "https://example.invalid", "", `FRONTEND_URL (host "example.invalid") is not a recognised frontend`},
		{"unknown frontend in development", EnvDevelopment, "https://example.invalid", "", `FRONTEND_URL (host "example.invalid") is not a recognised frontend`},
		{"whitespace-only override counts as unset", EnvDevelopment, "https://example.invalid", "  ", `FRONTEND_URL (host "example.invalid") is not a recognised frontend`},
		{"local frontend in production", EnvProduction, "http://localhost:3000", "", `FRONTEND_URL (host "localhost:3000") is not a recognised frontend`},
		{"local frontend in stage", EnvStage, "http://127.0.0.1:3000", "", `FRONTEND_URL (host "127.0.0.1:3000") is not a recognised frontend`},
		{"empty frontend", EnvDevelopment, "", "", "FRONTEND_URL (empty)"},
		{"frontend without a scheme", EnvDevelopment, "localhost:3000", "", "FRONTEND_URL (no parseable host)"},
		{"unparseable frontend", EnvDevelopment, "http://[::1", "", "FRONTEND_URL (no parseable host)"},
		{"override without a scheme", EnvProduction, "https://www.psychichomily.com", "api.psychichomily.com", "BACKEND_PUBLIC_URL (no parseable host) is not a bare http(s) origin"},
		{"override with a path", EnvProduction, "https://www.psychichomily.com", "https://api.psychichomily.com/v1", `BACKEND_PUBLIC_URL (host "api.psychichomily.com") is not a bare http(s) origin`},
		{"override with userinfo", EnvDevelopment, "http://localhost:3000", "http://user:pw@localhost:8080", `BACKEND_PUBLIC_URL (host "localhost:8080") is not a bare http(s) origin`},
		{"loopback override in production", EnvProduction, "https://www.psychichomily.com", "http://localhost:8080", `BACKEND_PUBLIC_URL (host "localhost:8080") is a loopback or wildcard origin in ENVIRONMENT=production`},
		{"loopback override in stage", EnvStage, "https://stage.psychichomily.com", "http://[::1]:8080", `BACKEND_PUBLIC_URL (host "[::1]:8080") is a loopback or wildcard origin in ENVIRONMENT=stage`},
		{"wildcard override in production", EnvProduction, "https://www.psychichomily.com", "http://0.0.0.0:8080", `BACKEND_PUBLIC_URL (host "0.0.0.0:8080") is a loopback or wildcard origin`},
		{"IPv6 wildcard override in production", EnvProduction, "https://www.psychichomily.com", "http://[::]:8080", `BACKEND_PUBLIC_URL (host "[::]:8080") is a loopback or wildcard origin`},
		{"capitalised loopback override in production", EnvProduction, "https://www.psychichomily.com", "http://LOCALHOST:8080", `BACKEND_PUBLIC_URL (host "LOCALHOST:8080") is a loopback or wildcard origin`},
		{"trailing-dot loopback override in production", EnvProduction, "https://www.psychichomily.com", "http://localhost.:8080", `BACKEND_PUBLIC_URL (host "localhost.:8080") is a loopback or wildcard origin`},
		{"trailing-dot wildcard override in production", EnvProduction, "https://www.psychichomily.com", "http://0.0.0.0.:8080", `BACKEND_PUBLIC_URL (host "0.0.0.0.:8080") is a loopback or wildcard origin`},
		{"double-trailing-dot loopback override in production", EnvProduction, "https://www.psychichomily.com", "http://LOCALHOST..:8080", `BACKEND_PUBLIC_URL (host "LOCALHOST..:8080") is a loopback or wildcard origin`},
		{"subdomain of localhost override in production", EnvProduction, "https://www.psychichomily.com", "http://api.localhost:8080", `BACKEND_PUBLIC_URL (host "api.localhost:8080") is a loopback or wildcard origin`},
		{"override with an empty query", EnvProduction, "https://www.psychichomily.com", "https://api.psychichomily.com?", `BACKEND_PUBLIC_URL (host "api.psychichomily.com") is not a bare http(s) origin`},
		{"override with an empty fragment", EnvProduction, "https://www.psychichomily.com", "https://api.psychichomily.com#", `BACKEND_PUBLIC_URL (host "api.psychichomily.com") is not a bare http(s) origin`},
	}
	for _, tt := range refusals {
		t.Run("refuses: "+tt.name, func(t *testing.T) {
			err := ValidateBackendOrigin(tt.environment, tt.frontendURL, tt.backendPublicURL)
			if err == nil || !strings.HasPrefix(err.Error(), tt.wantPrefix) {
				t.Errorf("ValidateBackendOrigin(%q, %q, %q) = %v, want prefix %q", tt.environment, tt.frontendURL, tt.backendPublicURL, err, tt.wantPrefix)
			}
		})
	}

	t.Run("the unrecognised-frontend refusal names the recognised forms", func(t *testing.T) {
		err := ValidateBackendOrigin(EnvProduction, "https://example.invalid", "")
		want := `FRONTEND_URL (host "example.invalid") is not a recognised frontend, so backend-hosted links ` +
			`(email unsubscribe links, the calendar feed, OAuth callbacks) cannot name this backend's public origin; ` +
			`set FRONTEND_URL to one of https://psychichomily.com, https://stage.psychichomily.com, ` +
			`https://www.psychichomily.com, or (outside production and stage) an http(s) origin on localhost, 127.0.0.1 or [::1], ` +
			`or set BACKEND_PUBLIC_URL to this backend's public origin`
		if err == nil || err.Error() != want {
			t.Errorf("ValidateBackendOrigin error = %v\nwant %s", err, want)
		}
	})

	t.Run("errors name the host, never userinfo, path or query", func(t *testing.T) {
		for _, values := range [][2]string{
			{"https://user:probe-secret@example.invalid/probe-path?probe=query", ""},
			{"https://www.psychichomily.com", "https://user:probe-secret@api.example.invalid/probe-path?probe=query"},
		} {
			err := ValidateBackendOrigin(EnvProduction, values[0], values[1])
			if err == nil {
				t.Fatalf("ValidateBackendOrigin(%q, %q) = nil, want an error", values[0], values[1])
			}
			msg := err.Error()
			if !strings.Contains(msg, `example.invalid"`) {
				t.Errorf("error %q does not name the host", msg)
			}
			for _, leak := range []string{"user:", "probe-secret", "probe-path", "probe=query"} {
				if strings.Contains(msg, leak) {
					t.Errorf("error %q carries %q", msg, leak)
				}
			}
		}
	})
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
	if cfg.MusicDiscovery.FrontendURL != "https://www.psychichomily.com" {
		t.Errorf("MusicDiscovery.FrontendURL = %q, want the trimmed value", cfg.MusicDiscovery.FrontendURL)
	}
	if got := cfg.BackendOrigin(); got != productionAPI {
		t.Errorf("BackendOrigin() = %q, want %q", got, productionAPI)
	}
	if want := productionAPI + "/auth/callback/google"; cfg.OAuth.GoogleCallbackURL != want {
		t.Errorf("GoogleCallbackURL = %q, want %q", cfg.OAuth.GoogleCallbackURL, want)
	}
}
