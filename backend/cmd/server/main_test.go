package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/cors"

	"psychic-homily-backend/internal/config"
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
	corsCfg := config.CORSConfig{
		AllowedOrigins:   []string{"https://app.example.com"},
		AllowedMethods:   []string{"GET", "POST", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type"},
		AllowCredentials: true,
	}
	const previewOrigin = "https://psychic-homily-abc123-matts-projects.vercel.app"

	t.Run("non-prod preflight echoes the bypass header for a preview origin", func(t *testing.T) {
		mw := newCORSMiddleware(corsCfg, false)
		resp := firePreflight(t, mw, previewOrigin, config.LighthouseBypassHeader)

		if !allowHeadersContains(resp, config.LighthouseBypassHeader) {
			t.Errorf("non-prod preflight must echo %q in Access-Control-Allow-Headers; got %q",
				config.LighthouseBypassHeader, resp.Header.Get("Access-Control-Allow-Headers"))
		}
		if got := resp.Header.Get("Access-Control-Allow-Origin"); got != previewOrigin {
			t.Errorf("non-prod preflight must allow the preview origin; Access-Control-Allow-Origin = %q, want %q", got, previewOrigin)
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
		resp := firePreflight(t, mw, previewOrigin, "Content-Type")

		if got := resp.Header.Get("Access-Control-Allow-Origin"); got != "" {
			t.Errorf("prod must not allow unlisted *.vercel.app origins; Access-Control-Allow-Origin = %q", got)
		}
	})
}

// fireRateLimitedRequest drives an actual (non-preflight) cross-origin GET
// through the constructed middleware to a handler that answers the way the
// backend's limiters do: 429 with Retry-After.
func fireRateLimitedRequest(t *testing.T, mw *cors.Cors, origin string) *http.Response {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/artists/some-band", nil)
	req.Header.Set("Origin", origin)

	rec := httptest.NewRecorder()
	handler := mw.Handler(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Retry-After", "60")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	handler.ServeHTTP(rec, req)
	return rec.Result()
}

// TestNewCORSMiddlewareExposedHeaders pins the exact set of non-safelisted
// response headers a cross-origin browser may read. The expected value is a
// literal rather than config.CORSExposedHeaders() so that widening the list is
// a deliberate edit to this test, not a silent change to the CORS posture.
func TestNewCORSMiddlewareExposedHeaders(t *testing.T) {
	corsCfg := config.CORSConfig{
		AllowedOrigins:   []string{"https://app.example.com"},
		AllowedMethods:   []string{"GET", "POST", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type"},
		AllowCredentials: true,
	}
	const (
		allowedOrigin = "https://app.example.com"
		previewOrigin = "https://psychic-homily-abc123-matts-projects.vercel.app"
		wantExposed   = "Retry-After"
	)

	for _, isProduction := range []bool{true, false} {
		t.Run(fmt.Sprintf("allowed origin reads Retry-After on a 429 (production=%t)", isProduction), func(t *testing.T) {
			resp := fireRateLimitedRequest(t, newCORSMiddleware(corsCfg, isProduction), allowedOrigin)

			if resp.StatusCode != http.StatusTooManyRequests {
				t.Fatalf("status = %d, want 429", resp.StatusCode)
			}
			if got := resp.Header.Get("Access-Control-Allow-Origin"); got != allowedOrigin {
				t.Errorf("Access-Control-Allow-Origin = %q, want %q", got, allowedOrigin)
			}
			if got := resp.Header.Values("Access-Control-Expose-Headers"); len(got) != 1 || got[0] != wantExposed {
				t.Errorf("Access-Control-Expose-Headers = %q, want exactly [%q]", got, wantExposed)
			}
			if got := resp.Header.Get("Retry-After"); got != "60" {
				t.Errorf("Retry-After = %q, want the handler's value to pass through", got)
			}
		})
	}

	t.Run("non-prod preview origin reads Retry-After", func(t *testing.T) {
		resp := fireRateLimitedRequest(t, newCORSMiddleware(corsCfg, false), previewOrigin)

		if got := resp.Header.Get("Access-Control-Expose-Headers"); got != wantExposed {
			t.Errorf("Access-Control-Expose-Headers = %q, want %q", got, wantExposed)
		}
	})

	t.Run("disallowed origin gets no CORS headers at all", func(t *testing.T) {
		resp := fireRateLimitedRequest(t, newCORSMiddleware(corsCfg, true), previewOrigin)

		if got := resp.Header.Get("Access-Control-Allow-Origin"); got != "" {
			t.Errorf("Access-Control-Allow-Origin = %q, want empty", got)
		}
		if got := resp.Header.Get("Access-Control-Expose-Headers"); got != "" {
			t.Errorf("Access-Control-Expose-Headers = %q, want empty for a disallowed origin", got)
		}
	})

	t.Run("preflight carries no Expose-Headers", func(t *testing.T) {
		// Browsers read Access-Control-Expose-Headers from the actual response
		// only; a preflight answer has no body or headers for script to read.
		resp := firePreflight(t, newCORSMiddleware(corsCfg, true), allowedOrigin, "Content-Type")

		if got := resp.Header.Get("Access-Control-Allow-Origin"); got != allowedOrigin {
			t.Fatalf("preflight Access-Control-Allow-Origin = %q, want %q", got, allowedOrigin)
		}
		if got := resp.Header.Get("Access-Control-Expose-Headers"); got != "" {
			t.Errorf("preflight Access-Control-Expose-Headers = %q, want empty", got)
		}
	})
}
