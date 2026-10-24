package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"psychic-homily-backend/internal/api/middleware"
	"psychic-homily-backend/internal/api/routes"
	"psychic-homily-backend/internal/config"
	"psychic-homily-backend/internal/services"
)

// The router stacks the response-header policy, an anonymous public-read
// limiter, then routes.SetupRoutes, in the relative order main() mounts them;
// it leaves out the middleware that never answers on its own (request
// correlation, compression). Every kind of answer it gives carries the
// noindex directive: a CORS preflight the CORS middleware answers itself, a
// Huma JSON document, a Huma error, the OpenAPI document, the router's own 404,
// and a limiter refusal.
func TestResponseHeaderPolicyMarksEveryResponseNoIndex(t *testing.T) {
	cfg := &config.Config{
		JWT:   config.JWTConfig{SecretKey: "test-secret-key-32-chars-minimum", Expiry: 24},
		OAuth: config.OAuthConfig{SecretKey: "test-oauth-secret-key-32-chars"},
	}
	router := chi.NewMux()
	useResponseHeaderPolicy(router, testCORSConfig, true)
	router.Use(middleware.RateLimitPublicReadAnonymousEndpoints())
	routes.SetupRoutes(router, services.NewServiceContainer(nil, cfg), cfg)

	serve := func(req *http.Request) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		return rec
	}
	assertNoIndex := func(t *testing.T, name string, rec *httptest.ResponseRecorder, wantStatus int) {
		t.Helper()
		if rec.Code != wantStatus {
			t.Fatalf("%s: status = %d, want %d", name, rec.Code, wantStatus)
		}
		if got := rec.Header().Get("X-Robots-Tag"); got != "noindex, nofollow" {
			t.Errorf("%s: X-Robots-Tag = %q, want %q", name, got, "noindex, nofollow")
		}
	}

	preflight := httptest.NewRequest(http.MethodOptions, "/health", nil)
	preflight.Header.Set("Origin", testAllowedOrigin)
	preflight.Header.Set("Access-Control-Request-Method", http.MethodGet)
	rec := serve(preflight)
	assertNoIndex(t, "preflight", rec, http.StatusOK)
	if rec.Header().Get("Access-Control-Allow-Origin") != testAllowedOrigin {
		t.Fatalf("preflight was not answered by CORS: headers %v", rec.Header())
	}

	assertNoIndex(t, "huma JSON", serve(httptest.NewRequest(http.MethodGet, "/health", nil)), http.StatusOK)
	assertNoIndex(t, "huma error", serve(httptest.NewRequest(http.MethodGet, "/auth/profile", nil)), http.StatusUnauthorized)
	assertNoIndex(t, "openapi", serve(httptest.NewRequest(http.MethodGet, "/openapi.json", nil)), http.StatusOK)
	assertNoIndex(t, "not found", serve(httptest.NewRequest(http.MethodGet, "/does-not-exist", nil)), http.StatusNotFound)

	var last *httptest.ResponseRecorder
	for range middleware.APIRequestsPerMinute + 1 {
		last = serve(httptest.NewRequest(http.MethodGet, "/does-not-exist", nil))
	}
	assertNoIndex(t, "limiter refusal", last, http.StatusTooManyRequests)
}
