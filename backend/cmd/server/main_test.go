package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/cors"

	"psychic-homily-backend/internal/api/middleware"
	"psychic-homily-backend/internal/config"
)

// testCORSConfig is the allowlist every CORS middleware test builds from.
var testCORSConfig = config.CORSConfig{
	AllowedOrigins:   []string{testAllowedOrigin},
	AllowedMethods:   []string{"GET", "POST", "OPTIONS"},
	AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type"},
	AllowCredentials: true,
}

const (
	testAllowedOrigin = "https://app.example.com"
	// testPreviewOrigin is allowed only outside production.
	testPreviewOrigin = "https://psychic-homily-abc123-matts-projects.vercel.app"
)

// firePreflight drives a real CORS preflight (OPTIONS + Access-Control-Request-*)
// through the constructed middleware and returns the response, so the tests
// assert the bytes a browser would actually see rather than re-implementing the
// go-chi/cors decision logic.
func firePreflight(t *testing.T, mw *cors.Cors, origin, reqHeaders string) *http.Response {
	t.Helper()
	req := httptest.NewRequest(http.MethodOptions, "/explore/upcoming-shows", nil)
	req.Header.Set("Origin", origin)
	req.Header.Set("Access-Control-Request-Method", http.MethodGet)
	req.Header.Set("Access-Control-Request-Headers", reqHeaders)

	rec := httptest.NewRecorder()
	handler := mw.Handler(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	handler.ServeHTTP(rec, req)
	return rec.Result()
}

func allowHeadersContains(resp *http.Response, header string) bool {
	got := strings.ToLower(resp.Header.Get("Access-Control-Allow-Headers"))
	return strings.Contains(got, strings.ToLower(header))
}

// TestNewCORSMiddlewarePreflight is the gate-critical behavioural contract for
// PSY-929: it proves the WIRED middleware (not just the CORSAllowedHeaders
// helper) echoes the Lighthouse x-vercel-protection-bypass header on a real
// preflight in non-prod and withholds it in prod. A wiring drop in
// newCORSMiddleware or a go-chi/cors bump that changed preflight echoing would
// fail here even though the helper unit test still passed — which is exactly
// how the /explore gate silently broke before.
func TestNewCORSMiddlewarePreflight(t *testing.T) {
	corsCfg := testCORSConfig

	t.Run("non-prod preflight echoes the bypass header for a preview origin", func(t *testing.T) {
		mw := newCORSMiddleware(corsCfg, false)
		resp := firePreflight(t, mw, testPreviewOrigin, config.LighthouseBypassHeader)

		if !allowHeadersContains(resp, config.LighthouseBypassHeader) {
			t.Errorf("non-prod preflight must echo %q in Access-Control-Allow-Headers; got %q",
				config.LighthouseBypassHeader, resp.Header.Get("Access-Control-Allow-Headers"))
		}
		if got := resp.Header.Get("Access-Control-Allow-Origin"); got != testPreviewOrigin {
			t.Errorf("non-prod preflight must allow the preview origin; Access-Control-Allow-Origin = %q, want %q", got, testPreviewOrigin)
		}
	})

	t.Run("prod preflight withholds the bypass header (browser would block)", func(t *testing.T) {
		mw := newCORSMiddleware(corsCfg, true)
		// Use an explicitly allowed origin so this isolates the HEADER decision
		// from the origin decision: the request fails only because the bypass
		// header is not allow-listed in prod.
		resp := firePreflight(t, mw, "https://app.example.com", config.LighthouseBypassHeader)

		if allowHeadersContains(resp, config.LighthouseBypassHeader) {
			t.Errorf("prod preflight must NOT echo %q; got %q",
				config.LighthouseBypassHeader, resp.Header.Get("Access-Control-Allow-Headers"))
		}
	})

	t.Run("prod preflight still works for a normal header", func(t *testing.T) {
		mw := newCORSMiddleware(corsCfg, true)
		resp := firePreflight(t, mw, "https://app.example.com", "Content-Type")

		if !allowHeadersContains(resp, "Content-Type") {
			t.Errorf("prod preflight must still echo Content-Type; got %q", resp.Header.Get("Access-Control-Allow-Headers"))
		}
		if got := resp.Header.Get("Access-Control-Allow-Origin"); got != "https://app.example.com" {
			t.Errorf("prod preflight must allow the configured origin; got %q", got)
		}
	})

	t.Run("prod still rejects unlisted vercel preview origins", func(t *testing.T) {
		mw := newCORSMiddleware(corsCfg, true)
		resp := firePreflight(t, mw, testPreviewOrigin, "Content-Type")

		if got := resp.Header.Get("Access-Control-Allow-Origin"); got != "" {
			t.Errorf("prod must not allow unlisted *.vercel.app origins; Access-Control-Allow-Origin = %q", got)
		}
	})
}

// fireRateLimitedRequest drives an actual (non-preflight) cross-origin GET
// through the constructed middleware to the backend's own limiter 429 handler.
func fireRateLimitedRequest(t *testing.T, mw *cors.Cors, origin string) *http.Response {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/artists/some-band", nil)
	req.Header.Set("Origin", origin)

	rec := httptest.NewRecorder()
	mw.Handler(http.HandlerFunc(middleware.RateLimitExceededHandler)).ServeHTTP(rec, req)
	return rec.Result()
}

// TestNewCORSMiddlewareExposedHeaders pins the exact set of non-safelisted
// response headers a cross-origin browser may read. go-chi/cors writes the list
// as ONE header value joined with ", ", so a widened list is compared as that
// joined string. The expected value is a literal rather than
// config.CORSExposedHeaders() so that widening the list is a deliberate edit to
// this test, not a silent change to the CORS posture.
func TestNewCORSMiddlewareExposedHeaders(t *testing.T) {
	const wantExposed = "Retry-After"

	tests := []struct {
		name            string
		isProduction    bool
		origin          string
		wantAllowOrigin string
		wantExposed     string
	}{
		{"prod allowed origin", true, testAllowedOrigin, testAllowedOrigin, wantExposed},
		{"non-prod allowed origin", false, testAllowedOrigin, testAllowedOrigin, wantExposed},
		{"non-prod preview origin", false, testPreviewOrigin, testPreviewOrigin, wantExposed},
		{"prod disallowed origin gets no CORS headers", true, testPreviewOrigin, "", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			resp := fireRateLimitedRequest(t, newCORSMiddleware(testCORSConfig, tc.isProduction), tc.origin)

			if resp.StatusCode != http.StatusTooManyRequests {
				t.Fatalf("status = %d, want 429", resp.StatusCode)
			}
			if resp.Header.Get("Retry-After") == "" {
				t.Fatal("the limiter's 429 must carry Retry-After for exposure to matter")
			}
			if got := resp.Header.Get("Access-Control-Allow-Origin"); got != tc.wantAllowOrigin {
				t.Errorf("Access-Control-Allow-Origin = %q, want %q", got, tc.wantAllowOrigin)
			}
			if got := resp.Header.Values("Access-Control-Expose-Headers"); strings.Join(got, "|") != tc.wantExposed {
				t.Errorf("Access-Control-Expose-Headers = %q, want exactly %q", got, tc.wantExposed)
			}
		})
	}

	t.Run("preflight carries no Expose-Headers", func(t *testing.T) {
		// Browsers read Access-Control-Expose-Headers from the actual response
		// only; a preflight answer has no body or headers for script to read.
		resp := firePreflight(t, newCORSMiddleware(testCORSConfig, true), testAllowedOrigin, "Content-Type")

		if got := resp.Header.Get("Access-Control-Allow-Origin"); got != testAllowedOrigin {
			t.Fatalf("preflight Access-Control-Allow-Origin = %q, want %q", got, testAllowedOrigin)
		}
		if got := resp.Header.Get("Access-Control-Expose-Headers"); got != "" {
			t.Errorf("preflight Access-Control-Expose-Headers = %q, want empty", got)
		}
	})
}
