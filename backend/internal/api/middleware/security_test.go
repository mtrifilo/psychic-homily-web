package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSecurityHeaders_DefaultHeaders(t *testing.T) {
	handler := SecurityHeaders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	expected := map[string]string{
		"X-Content-Type-Options":          "nosniff",
		"X-Frame-Options":                 "DENY",
		"X-Xss-Protection":               "1; mode=block",
		"Referrer-Policy":                 "strict-origin-when-cross-origin",
		"Permissions-Policy":              "geolocation=(), microphone=(), camera=(), payment=(), usb=()",
		"Content-Security-Policy":         "default-src 'none'; frame-ancestors 'none'; base-uri 'none'; form-action 'none'",
		"X-Permitted-Cross-Domain-Policies": "none",
	}

	for header, want := range expected {
		got := rr.Header().Get(header)
		if got != want {
			t.Errorf("header %s = %q, want %q", header, got, want)
		}
	}
}

func TestSecurityHeaders_HSTS_NonProduction(t *testing.T) {
	t.Setenv("ENVIRONMENT", "development")

	handler := SecurityHeaders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if got := rr.Header().Get("Strict-Transport-Security"); got != "" {
		t.Errorf("expected no HSTS header in non-production, got %q", got)
	}
}

func TestSecurityHeaders_HSTS_Production(t *testing.T) {
	t.Setenv("ENVIRONMENT", "production")

	handler := SecurityHeaders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	want := "max-age=31536000; includeSubDomains"
	if got := rr.Header().Get("Strict-Transport-Security"); got != want {
		t.Errorf("HSTS header = %q, want %q", got, want)
	}
}

func TestSecurityHeaders_CallsNext(t *testing.T) {
	called := false
	handler := SecurityHeaders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if !called {
		t.Error("next handler was not called")
	}
	if rr.Code != http.StatusOK {
		t.Errorf("status code = %d, want %d", rr.Code, http.StatusOK)
	}
}

func TestBuildCSP(t *testing.T) {
	want := "default-src 'none'; frame-ancestors 'none'; base-uri 'none'; form-action 'none'"
	if got := buildCSP(); got != want {
		t.Errorf("buildCSP() = %q, want %q", got, want)
	}
}

func TestIsProduction(t *testing.T) {
	tests := []struct {
		name string
		env  string
		want bool
	}{
		{"production", "production", true},
		{"prod", "prod", true},
		{"empty", "", false},
		{"development", "development", false},
		{"stage", "stage", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("ENVIRONMENT", tt.env)
			if got := isProduction(); got != tt.want {
				t.Errorf("isProduction() with ENVIRONMENT=%q = %v, want %v", tt.env, got, tt.want)
			}
		})
	}
}

func TestDefaultSecurityConfig(t *testing.T) {
	cfg := DefaultSecurityConfig()

	if cfg.EnableHSTS {
		t.Error("EnableHSTS should be false by default")
	}
	if cfg.HSTSMaxAge != 31536000 {
		t.Errorf("HSTSMaxAge = %d, want 31536000", cfg.HSTSMaxAge)
	}
	if !cfg.HSTSIncludeSubDomains {
		t.Error("HSTSIncludeSubDomains should be true by default")
	}
	if cfg.HSTSPreload {
		t.Error("HSTSPreload should be false by default")
	}
	if cfg.CustomCSP != "" {
		t.Errorf("CustomCSP = %q, want empty", cfg.CustomCSP)
	}
	if cfg.FrameOptions != "DENY" {
		t.Errorf("FrameOptions = %q, want DENY", cfg.FrameOptions)
	}
}

func TestSecurityHeadersWithConfig_CustomCSP(t *testing.T) {
	t.Setenv("ENVIRONMENT", "development")

	customCSP := "default-src 'self'; script-src 'self'"
	cfg := SecurityHeadersConfig{CustomCSP: customCSP}

	handler := SecurityHeadersWithConfig(cfg)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if got := rr.Header().Get("Content-Security-Policy"); got != customCSP {
		t.Errorf("CSP = %q, want %q", got, customCSP)
	}
}

func TestSecurityHeadersWithConfig_CustomFrameOptions(t *testing.T) {
	t.Setenv("ENVIRONMENT", "development")

	cfg := SecurityHeadersConfig{FrameOptions: "SAMEORIGIN"}
	handler := SecurityHeadersWithConfig(cfg)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if got := rr.Header().Get("X-Frame-Options"); got != "SAMEORIGIN" {
		t.Errorf("X-Frame-Options = %q, want SAMEORIGIN", got)
	}
}

func TestSecurityHeadersWithConfig_EmptyFrameOptions(t *testing.T) {
	t.Setenv("ENVIRONMENT", "development")

	cfg := SecurityHeadersConfig{FrameOptions: ""}
	handler := SecurityHeadersWithConfig(cfg)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if got := rr.Header().Get("X-Frame-Options"); got != "DENY" {
		t.Errorf("X-Frame-Options = %q, want DENY (default when empty)", got)
	}
}

func TestSecurityHeadersWithConfig_HSTSEnabled(t *testing.T) {
	t.Setenv("ENVIRONMENT", "development")

	cfg := SecurityHeadersConfig{
		EnableHSTS:            true,
		HSTSIncludeSubDomains: true,
		HSTSPreload:           true,
	}
	handler := SecurityHeadersWithConfig(cfg)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	got := rr.Header().Get("Strict-Transport-Security")
	if got == "" {
		t.Fatal("expected HSTS header to be set")
	}
	// With HSTSMaxAge=0, it falls back to "max-age=31536000"
	if want := "max-age=31536000; includeSubDomains; preload"; got != want {
		t.Errorf("HSTS = %q, want %q", got, want)
	}
}

func TestSecurityHeadersWithConfig_CallsNext(t *testing.T) {
	t.Setenv("ENVIRONMENT", "development")

	called := false
	cfg := DefaultSecurityConfig()
	handler := SecurityHeadersWithConfig(cfg)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if !called {
		t.Error("next handler was not called")
	}
}
