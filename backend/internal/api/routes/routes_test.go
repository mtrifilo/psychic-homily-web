package routes

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/go-chi/chi/v5"

	"psychic-homily-backend/db"
	"psychic-homily-backend/internal/api/middleware"
	"psychic-homily-backend/internal/config"
	authm "psychic-homily-backend/internal/models/auth"
	"psychic-homily-backend/internal/services"
)

// =============================================================================
// DRIVING THE BUILT ROUTER
// =============================================================================
//
// The three visibility matrices (show_subresource_visibility_test.go,
// comment_subscription_visibility_test.go, collection_subscription_visibility_test.go)
// all do the same thing: mint a real JWT and send it through the real router on
// the real carrier. These helpers are that, once (PSY-1987).
//
// The CARRIER is the load-bearing detail and the reason this is shared rather
// than re-typed per file: credentials ride the `auth_token` COOKIE, not an
// Authorization header, and a matrix that got that wrong would see every caller
// as anonymous — which reads as "the gate works" on every single assertion.
// One spelling, so a change to the carrier fails all three files at once.

// mintToken issues a real session token for u through the container's JWT
// service, so the middleware validates it exactly as it validates a browser's.
func mintToken(t *testing.T, sc *services.ServiceContainer, u *authm.User) string {
	t.Helper()
	tok, err := sc.JWT.CreateToken(u)
	if err != nil {
		t.Fatalf("mint token for user %d: %v", u.ID, err)
	}
	return tok
}

// doRequest sends one request through the built router and returns the status
// and raw body.
//
// The RAW body, because the assertions search it rather than decoded fields: a
// field-by-field check passes if a future writer echoes a withheld title under
// another key or into an error message.
func doRequest(t *testing.T, router http.Handler, method, path, credential string, body []byte) (int, []byte) {
	t.Helper()
	var req *http.Request
	if body == nil {
		req = httptest.NewRequest(method, path, nil)
	} else {
		req = httptest.NewRequest(method, path, bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
	}
	if credential != "" {
		req.AddCookie(&http.Cookie{Name: "auth_token", Value: credential})
	}
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w.Code, w.Body.Bytes()
}

// getRequest is doRequest for the GET case, which is most of them.
func getRequest(t *testing.T, router http.Handler, path, credential string) (int, []byte) {
	t.Helper()
	return doRequest(t, router, http.MethodGet, path, credential, nil)
}

func testConfig() *config.Config {
	return &config.Config{
		Server: config.ServerConfig{
			Addr: "localhost:8080",
		},
		JWT: config.JWTConfig{
			SecretKey: "test-secret-key-32-chars-minimum",
			Expiry:    24,
		},
		OAuth: config.OAuthConfig{
			SecretKey: "test-oauth-secret-key-32-chars",
		},
	}
}

func testContainer(cfg *config.Config) *services.ServiceContainer {
	return services.NewServiceContainer(nil, cfg)
}

// TestSetupRoutes tests the main route setup function
func TestSetupRoutes(t *testing.T) {
	cfg := testConfig()
	sc := testContainer(cfg)

	router := chi.NewRouter()
	api := SetupRoutes(router, sc, cfg)

	if api == nil {
		t.Fatal("Expected API to be created, got nil")
	}

	// Test that routes are registered by checking if we can get the OpenAPI spec
	req := httptest.NewRequest("GET", "/openapi.json", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}

	var openAPI map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &openAPI); err != nil {
		t.Fatalf("Failed to parse OpenAPI spec: %v", err)
	}

	// Check that it's a valid OpenAPI spec
	if _, ok := openAPI["openapi"]; !ok {
		t.Error("Expected OpenAPI spec to contain 'openapi' field")
	}
}

// TestAdvancementRouteOpenAPI locks GET /auth/profile/advancement into the
// protected OpenAPI surface (PSY-1087).
func TestAdvancementRouteOpenAPI(t *testing.T) {
	cfg := testConfig()
	sc := testContainer(cfg)
	router := chi.NewRouter()
	api := humachi.New(router, huma.DefaultConfig("Advancement route", "1.0.0"))
	protected := huma.NewGroup(api, "")
	// setupProtectedAuthRoutes also registers /auth/cli-token on rc.Admin
	// (PSY-550); a nil Admin group panics inside huma.Post.
	admin := huma.NewGroup(api, "")

	setupProtectedAuthRoutes(RouteContext{
		Router: router, API: api, Protected: protected, Admin: admin, SC: sc, Cfg: cfg,
	})

	item, exists := api.OpenAPI().Paths["/auth/profile/advancement"]
	if !exists || item.Get == nil {
		t.Fatal("Expected OpenAPI GET operation for /auth/profile/advancement")
	}
	response := item.Get.Responses["200"]
	if response == nil || response.Content["application/json"] == nil {
		t.Fatal("Expected documented JSON 200 response for /auth/profile/advancement")
	}
}

func TestSetupFollowRoutesOpenAPI(t *testing.T) {
	cfg := testConfig()
	sc := testContainer(cfg)
	router := chi.NewRouter()
	api := humachi.New(router, huma.DefaultConfig("Follow routes", "1.0.0"))
	protected := huma.NewGroup(api, "")

	setupFollowRoutes(RouteContext{
		Router: router, API: api, Protected: protected, SC: sc, Cfg: cfg,
	})

	for _, path := range []string{
		"/me/library/following",
		"/me/library/following/counts",
	} {
		item, exists := api.OpenAPI().Paths[path]
		if !exists || item.Get == nil {
			t.Errorf("Expected OpenAPI GET operation for %s", path)
		}
	}

	operation := api.OpenAPI().Paths["/me/library/following"].Get
	params := make(map[string]*huma.Param, len(operation.Parameters))
	for _, param := range operation.Parameters {
		params[param.Name] = param
	}
	entityType := params["type"]
	expectedTypes := map[any]bool{
		"artist": true, "venue": true, "scene": true, "label": true, "festival": true, "tag": true,
	}
	if entityType == nil || !entityType.Required || len(entityType.Schema.Enum) != len(expectedTypes) {
		t.Fatalf("expected required six-value type enum, got %+v", entityType)
	}
	for _, value := range entityType.Schema.Enum {
		if !expectedTypes[value] {
			t.Fatalf("unexpected type enum value %v", value)
		}
	}
	limit := params["limit"]
	if limit == nil || limit.Schema.Default != 50 || limit.Schema.Minimum == nil || *limit.Schema.Minimum != 1 || limit.Schema.Maximum == nil || *limit.Schema.Maximum != 100 {
		t.Fatalf("expected documented limit default/max, got %+v", limit)
	}
	cursor := params["cursor"]
	if len(params) != 3 || cursor == nil || cursor.Required || cursor.Schema.MaxLength == nil || *cursor.Schema.MaxLength != 1024 {
		t.Fatalf("expected exact type/limit/cursor parameters, got %+v", params)
	}

	resolveSchema := func(schema *huma.Schema) *huma.Schema {
		if schema == nil || schema.Ref == "" {
			return schema
		}
		return api.OpenAPI().Components.Schemas.SchemaFromRef(schema.Ref)
	}
	assertProperties := func(schema *huma.Schema, expected ...string) {
		t.Helper()
		schema = resolveSchema(schema)
		propertyCount := 0
		if schema != nil {
			for name := range schema.Properties {
				if name != "$schema" {
					propertyCount++
				}
			}
		}
		if schema == nil || propertyCount != len(expected) {
			t.Fatalf("expected response properties %v, got %+v", expected, schema)
		}
		for _, name := range expected {
			if schema.Properties[name] == nil {
				t.Errorf("expected response property %q", name)
			}
		}
	}
	response := operation.Responses["200"]
	if response == nil || response.Content["application/json"] == nil || response.Content["application/json"].Schema == nil {
		t.Fatal("expected documented JSON response schema")
	}
	pageSchema := resolveSchema(response.Content["application/json"].Schema)
	assertProperties(pageSchema, "following", "limit", "next_cursor")
	// alerts (PSY-1893): the row's resolved alert subscription, so the per-row
	// alerts control renders without a request per row.
	assertProperties(pageSchema.Properties["following"].Items, "entity_type", "entity_id", "name", "slug", "followed_at", "alerts")

	countsOperation := api.OpenAPI().Paths["/me/library/following/counts"].Get
	countsResponse := countsOperation.Responses["200"].Content["application/json"].Schema
	assertProperties(countsResponse, "artists", "venues", "scenes", "labels", "festivals", "tags")

	// PSY-1496: username-addressed user follow routes must be documented.
	for _, check := range []struct {
		path   string
		method string
	}{
		{"/users/{username}/follow", "POST"},
		{"/users/{username}/follow", "DELETE"},
		{"/users/{username}/followers", "GET"},
	} {
		item, exists := api.OpenAPI().Paths[check.path]
		if !exists {
			t.Errorf("Expected OpenAPI path %s", check.path)
			continue
		}
		var op any
		switch check.method {
		case "POST":
			op = item.Post
		case "DELETE":
			op = item.Delete
		case "GET":
			op = item.Get
		}
		if op == nil {
			t.Errorf("Expected OpenAPI %s operation for %s", check.method, check.path)
		}
	}

	// PSY-1466: the scene follow body's notify_mode enum must include "off"
	// alongside the pre-existing "all" and "followed_bands_only".
	sceneFollowOp := api.OpenAPI().Paths["/scenes/{slug}/follow"].Post
	if sceneFollowOp == nil || sceneFollowOp.RequestBody == nil {
		t.Fatal("expected documented POST /scenes/{slug}/follow request body")
	}
	bodySchema := resolveSchema(sceneFollowOp.RequestBody.Content["application/json"].Schema)
	notifyModeSchema := bodySchema.Properties["notify_mode"]
	if notifyModeSchema == nil {
		t.Fatal("expected notify_mode property on scene follow body")
	}
	expectedModes := map[any]bool{"all": true, "followed_bands_only": true, "off": true}
	if len(notifyModeSchema.Enum) != len(expectedModes) {
		t.Fatalf("expected three-value notify_mode enum, got %+v", notifyModeSchema.Enum)
	}
	for _, value := range notifyModeSchema.Enum {
		if !expectedModes[value] {
			t.Fatalf("unexpected notify_mode enum value %v", value)
		}
	}
}

// TestSetupAuthRoutes tests authentication route setup
func TestSetupAuthRoutes(t *testing.T) {
	cfg := testConfig()
	sc := testContainer(cfg)

	router := chi.NewRouter()
	api := humachi.New(router, huma.DefaultConfig("Test", "1.0.0"))

	setupAuthRoutes(RouteContext{Router: router, API: api, SC: sc, Cfg: cfg})

	// Test OAuth login route
	t.Run("OAuth Login Route", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/auth/login/google", nil)
		w := httptest.NewRecorder()

		// Set up chi context with URL parameters
		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("provider", "google")
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

		router.ServeHTTP(w, req)

		// OAuth login may fail due to missing OAuth provider configuration in tests
		// Accept various status codes that indicate the route is working
		if w.Code != http.StatusOK && w.Code != http.StatusTemporaryRedirect && w.Code != http.StatusBadRequest && w.Code != http.StatusInternalServerError {
			t.Errorf("Expected status 200, 302, 400, or 500, got %d", w.Code)
		}
	})

	// Test OAuth callback route
	t.Run("OAuth Callback Route", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/auth/callback/google", nil)
		w := httptest.NewRecorder()

		// Set up chi context with URL parameters
		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("provider", "google")
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

		router.ServeHTTP(w, req)

		// OAuth callback may fail due to missing OAuth provider configuration in tests
		// Accept various status codes that indicate the route is working
		if w.Code != http.StatusOK && w.Code != http.StatusTemporaryRedirect && w.Code != http.StatusBadRequest && w.Code != http.StatusInternalServerError {
			t.Errorf("Expected status 200, 302, 400, or 500, got %d", w.Code)
		}
	})

	// Test logout route
	t.Run("Logout Route", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/auth/logout", nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
		}
	})
}

// TestSetupSystemRoutes tests system route setup
func TestSetupSystemRoutes(t *testing.T) {
	router := chi.NewRouter()
	api := humachi.New(router, huma.DefaultConfig("Test", "1.0.0"))

	setupSystemRoutes(RouteContext{Router: router, API: api})

	// Test health check route
	t.Run("Health Check Route", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/health", nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
		}

		// Check response format
		var response map[string]interface{}
		if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
			t.Fatalf("Failed to parse health response: %v", err)
		}

		// Without a database, health check returns "unhealthy"
		validStatuses := map[string]bool{"healthy": true, "unhealthy": true, "degraded": true}
		if status, ok := response["status"].(string); !ok || !validStatuses[status] {
			t.Errorf("Expected valid health status, got %v", status)
		}
	})

	// The readiness route is asserted THROUGH the router, not by calling the
	// handler directly, because the failure this guards against is registration,
	// not handler logic: a typo in the path, registering on rc.Admin instead of
	// rc.API, or dropping the line entirely all leave handler-level tests green
	// while the URL an external monitor polls returns 404. That is not
	// hypothetical: a WIP commit on this branch had the handler written with no
	// route registration, so /health/ready 404'd at runtime. This test is what
	// makes that state unshippable rather than merely noticed.
	//
	// Both methods are asserted because an uptime monitor may probe with either,
	// and a 405 matches no alert rule.
	for _, method := range []string{"GET", "HEAD"} {
		t.Run("Readiness Route "+method, func(t *testing.T) {
			// db.DB is a package global; pin it nil so this asserts the
			// unhealthy path regardless of what other tests left behind.
			prev := db.DB
			db.DB = nil
			t.Cleanup(func() { db.DB = prev })

			req := httptest.NewRequest(method, "/health/ready", nil)
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			if w.Code != http.StatusServiceUnavailable {
				t.Fatalf("Expected status %d with no database, got %d (body: %s)",
					http.StatusServiceUnavailable, w.Code, w.Body.String())
			}

			// Liveness must NOT follow readiness down — same router, same
			// missing database, opposite status code. This pairing is the whole
			// contract.
			livenessReq := httptest.NewRequest(method, "/health", nil)
			livenessW := httptest.NewRecorder()
			router.ServeHTTP(livenessW, livenessReq)
			if livenessW.Code != http.StatusOK {
				t.Errorf("Expected /health to stay %d with no database, got %d",
					http.StatusOK, livenessW.Code)
			}

			// Body suppression for HEAD happens in net/http's response
			// writer, NOT in httptest.ResponseRecorder — the recorder holds
			// the full body for both methods. So these assertions cover the
			// content shape the handler produces on both paths; they do not
			// describe what a real HEAD response puts on the wire.
			if ct := w.Header().Get("Content-Type"); !strings.Contains(ct, "application/problem+json") {
				t.Errorf("Expected a problem+json error body, got Content-Type %q", ct)
			}
			if body := w.Body.String(); !strings.Contains(body, "database") {
				t.Errorf("Expected the 503 detail to name the failing component, got %s", body)
			}
		})
	}

	// Test OpenAPI spec route
	t.Run("OpenAPI Spec Route", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/openapi.json", nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
		}

		contentType := w.Header().Get("Content-Type")
		if contentType != "application/json" {
			t.Errorf("Expected Content-Type application/json, got %s", contentType)
		}

		var openAPI map[string]interface{}
		if err := json.Unmarshal(w.Body.Bytes(), &openAPI); err != nil {
			t.Fatalf("Failed to parse OpenAPI spec: %v", err)
		}

		// Check required OpenAPI fields
		requiredFields := []string{"openapi", "info", "paths"}
		for _, field := range requiredFields {
			if _, ok := openAPI[field]; !ok {
				t.Errorf("Expected OpenAPI spec to contain '%s' field", field)
			}
		}
	})
}

// TestProtectedRoutes tests protected route behavior
func TestProtectedRoutes(t *testing.T) {
	cfg := testConfig()
	sc := testContainer(cfg)

	router := chi.NewRouter()
	SetupRoutes(router, sc, cfg)

	// Test protected profile route without token
	t.Run("Protected Profile Route Without Token", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/auth/profile", nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		// Should return 401 Unauthorized
		if w.Code != http.StatusUnauthorized {
			t.Errorf("Expected status %d, got %d", http.StatusUnauthorized, w.Code)
		}
	})

	// Test protected profile route with invalid token
	t.Run("Protected Profile Route With Invalid Token", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/auth/profile", nil)
		req.Header.Set("Authorization", "Bearer invalid-token")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		// Should return 401 Unauthorized
		if w.Code != http.StatusUnauthorized {
			t.Errorf("Expected status %d, got %d", http.StatusUnauthorized, w.Code)
		}
	})

	// Test protected refresh route without token
	t.Run("Protected Refresh Route Without Token", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/auth/refresh", nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		// Should return 401 Unauthorized
		if w.Code != http.StatusUnauthorized {
			t.Errorf("Expected status %d, got %d", http.StatusUnauthorized, w.Code)
		}
	})

	// Pin /charts/me to the Protected (JWT) group. The handler carries its own
	// belt-and-suspenders 401, so status alone can't detect the route sliding
	// onto the public group — but only the middleware's rejection body carries
	// error_code (JWTErrorResponse); the handler's huma 401 does not. Asserting
	// it proves the JWT middleware fired before the handler.
	t.Run("Personal Charts Route Without Token", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/charts/me", nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		if w.Code != http.StatusUnauthorized {
			t.Errorf("Expected status %d, got %d", http.StatusUnauthorized, w.Code)
		}
		if !strings.Contains(w.Body.String(), "error_code") {
			t.Errorf("Expected the JWT middleware's 401 body (with error_code), got: %s", w.Body.String())
		}
	})
}

// TestRouteMiddleware tests middleware integration
func TestRouteMiddleware(t *testing.T) {
	cfg := testConfig()
	sc := testContainer(cfg)

	router := chi.NewRouter()
	api := humachi.New(router, huma.DefaultConfig("Test", "1.0.0"))

	setupAuthRoutes(RouteContext{Router: router, API: api, SC: sc, Cfg: cfg})

	// Test that CORS headers are set (if middleware is configured)
	t.Run("CORS Headers", func(t *testing.T) {
		req := httptest.NewRequest("OPTIONS", "/health", nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		// Should handle preflight requests
		if w.Code != http.StatusOK && w.Code != http.StatusNotFound {
			t.Errorf("Expected status 200 or 404, got %d", w.Code)
		}
	})
}

// TestRouteErrorHandling tests error handling in routes
func TestRouteErrorHandling(t *testing.T) {
	cfg := testConfig()
	sc := testContainer(cfg)

	router := chi.NewRouter()
	api := humachi.New(router, huma.DefaultConfig("Test", "1.0.0"))

	setupAuthRoutes(RouteContext{Router: router, API: api, SC: sc, Cfg: cfg})

	// Test OAuth callback with error (this will fail due to missing OAuth setup)
	t.Run("OAuth Callback With Error", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/auth/callback/google", nil)
		w := httptest.NewRecorder()

		// Set up chi context with URL parameters
		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("provider", "google")
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

		router.ServeHTTP(w, req)

		// Should handle the request (may fail due to missing OAuth setup)
		if w.Code != http.StatusOK && w.Code != http.StatusTemporaryRedirect && w.Code != http.StatusInternalServerError {
			t.Errorf("Expected status 200, 302, or 500, got %d", w.Code)
		}
	})
}

// TestRouteParameterExtraction tests URL parameter extraction
func TestRouteParameterExtraction(t *testing.T) {
	cfg := testConfig()
	sc := testContainer(cfg)

	router := chi.NewRouter()
	api := humachi.New(router, huma.DefaultConfig("Test", "1.0.0"))

	setupAuthRoutes(RouteContext{Router: router, API: api, SC: sc, Cfg: cfg})

	// Test different provider parameters
	providers := []string{"google", "github"}

	for _, provider := range providers {
		t.Run("Provider Parameter: "+provider, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/auth/login/"+provider, nil)
			w := httptest.NewRecorder()

			// Set up chi context with URL parameters
			rctx := chi.NewRouteContext()
			rctx.URLParams.Add("provider", provider)
			req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

			router.ServeHTTP(w, req)

			// Should handle the request (may fail due to missing OAuth setup)
			if w.Code != http.StatusOK && w.Code != http.StatusTemporaryRedirect && w.Code != http.StatusBadRequest && w.Code != http.StatusInternalServerError {
				t.Errorf("Expected status 200, 302, 400, or 500 for provider %s, got %d", provider, w.Code)
			}
		})
	}
}

// TestRouteRegistration tests that routes are properly registered
func TestRouteRegistration(t *testing.T) {
	cfg := testConfig()
	sc := testContainer(cfg)

	router := chi.NewRouter()
	api := humachi.New(router, huma.DefaultConfig("Test", "1.0.0"))

	setupAuthRoutes(RouteContext{Router: router, API: api, SC: sc, Cfg: cfg})

	// Test that routes are registered by checking if they respond
	// (even if they fail due to missing OAuth configuration)

	t.Run("OAuth Login Route Registered", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/auth/login/google", nil)
		w := httptest.NewRecorder()

		// Set up chi context with URL parameters
		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("provider", "google")
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

		router.ServeHTTP(w, req)

		// Route should be registered and respond (even if it fails)
		if w.Code == http.StatusNotFound {
			t.Error("Route not found - route not properly registered")
		}
	})

	t.Run("OAuth Callback Route Registered", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/auth/callback/google", nil)
		w := httptest.NewRecorder()

		// Set up chi context with URL parameters
		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("provider", "google")
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

		router.ServeHTTP(w, req)

		// Route should be registered and respond (even if it fails)
		if w.Code == http.StatusNotFound {
			t.Error("Route not found - route not properly registered")
		}
	})

	t.Run("Logout Route Registered", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/auth/logout", nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		// Route should be registered and respond
		if w.Code == http.StatusNotFound {
			t.Error("Route not found - route not properly registered")
		}
	})

	t.Run("Non-existent Route Returns 404", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/auth/nonexistent", nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		// Non-existent route should return 404
		if w.Code != http.StatusNotFound {
			t.Errorf("Expected status 404 for non-existent route, got %d", w.Code)
		}
	})
}

// TestServedOpenAPISpecIsTheWholeAPI locks PSY-1554: the spec served at
// /openapi.json must be the MAIN API, not a fragment from one of the
// rate-limit sub-instances.
//
// The bug this guards against is silent by construction. Every sub-API built
// with a bare huma.DefaultConfig registers its own /openapi.json on the shared
// chi routing tree, and chi REPLACES a duplicate method+path instead of
// erroring — so the last instance registered wins with no warning at all.
// Production served a spec titled "Psychic Homily Entity Reports" with 8 paths.
//
// TestSetupRoutes already fetched this endpoint and only asserted an "openapi"
// key was present, which a fragment satisfies just as well. Hence the title,
// volume, and cross-group sampling below: a second Huma instance that shadowed
// the spec would have to fail something here.
func TestServedOpenAPISpecIsTheWholeAPI(t *testing.T) {
	cfg := testConfig()
	sc := testContainer(cfg)
	router := chi.NewRouter()
	SetupRoutes(router, sc, cfg)

	req := httptest.NewRequest("GET", "/openapi.json", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("GET /openapi.json = %d, want 200", w.Code)
	}

	var spec struct {
		Info struct {
			Title string `json:"title"`
		} `json:"info"`
		Paths map[string]map[string]interface{} `json:"paths"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &spec); err != nil {
		t.Fatalf("parse spec: %v", err)
	}

	if spec.Info.Title != "Psychic Homily" {
		t.Errorf("served spec title = %q, want %q — a second Huma instance has shadowed the main spec. "+
			"This package must contain exactly one; see TestNoSubAPIInstancesRemain", spec.Info.Title, "Psychic Homily")
	}

	// Volume check. The real surface is in the hundreds; the shadowing fragment
	// had 8. A floor well above that catches shadowing without being brittle as
	// routes come and go.
	if len(spec.Paths) < 100 {
		t.Errorf("served spec describes %d paths, want >=100 — this looks like a fragment, not the API", len(spec.Paths))
	}

	// Cross-group sampling across route groups, so a second Huma instance that
	// shadowed the spec would make whole groups disappear here.
	//
	// The two report paths are here because PSY-1554 pinned them as known-ABSENT
	// while the entity-report group still lived on its own Huma instance, with the
	// standing instruction to flip rather than delete that coverage once the group
	// moved. PSY-1598 moved it, so they are asserted present here. Their full
	// behaviour — limiter, budget sharing, auth gate — is in subapi_reports_test.go.
	for _, path := range []string{
		"/shows",
		"/artists",
		"/tags",
		"/radio-stations",
		"/auth/profile",
		"/venues",
		"/artists/{entity_id}/report",
		"/venues/{entity_id}/report",
	} {
		if _, ok := spec.Paths[path]; !ok {
			t.Errorf("served spec is missing %q", path)
		}
	}
}

// TestNoSubAPIInstancesRemain pins the invariant PSY-1598 earned: this package
// builds exactly ONE Huma instance.
//
// The history: rate-limited endpoints used to be registered on extra humachi.New
// instances, because a chi.Group gave them a middleware stack and a Huma instance
// was the only way to hang Huma operations off one. Each instance owns a SEPARATE
// OpenAPI document, which produced two distinct failures. Its /openapi.json
// registration shadowed the main one — chi replaces a duplicate method+path
// rather than erroring, so the LAST registration wins and production served a
// fragment titled "Psychic Homily Entity Reports" (PSY-1554). And its operations
// never reached the real document: 27 of them, the entire auth surface included.
//
// PSY-1554 patched the first failure with subAPIConfig, a config helper that
// blanked the doc paths so a sub-API could not shadow anything. PSY-1598 fixed
// the second by moving every group onto the main API behind huma.NewGroup +
// humaFromHTTP, which removed that helper's last caller — so "make a sub-API
// safe" stopped being the policy, and the helper and its guard test went with it.
// Both are recoverable from git history if a second instance is ever genuinely
// warranted.
//
// This test is the replacement, and it is a TRADE, not a pure win. It is stronger
// on the axis that mattered least to PSY-1554 and most to PSY-1598: it catches
// operations going silently missing from the spec, which no runtime assertion
// can see. It is weaker on the axis subAPIConfig covered: there is no longer a
// runtime backstop making an accidental second instance harmless, so a sub-API
// that slips past this scan shadows the served spec immediately.
// TestServedOpenAPISpecIsTheWholeAPI is what would catch that, after the fact.
//
// Parsing rather than grepping is deliberate. A text scan is defeated by an
// import alias (`import hc ".../humachi"` then `hc.New(...)`), and it trips over
// this package's own comments, which discuss humachi.New by name.
//
// Test files are excluded: they build throwaway Huma instances to exercise single
// handlers in isolation, which is unrelated to the routing tree this pins.
func TestNoSubAPIInstancesRemain(t *testing.T) {
	// Both ways to build an instance. humachi.New is literally a wrapper around
	// huma.NewAPI with a chi adapter, so matching only the former leaves
	// `huma.NewAPI(cfg, humachi.NewAdapter(r))` as a silent bypass.
	constructors := map[string]string{
		"github.com/danielgtaylor/huma/v2/adapters/humachi": "New",
		"github.com/danielgtaylor/huma/v2":                  "NewAPI",
	}

	type site struct {
		file string
		line int
	}
	var sites []site

	// Walk the tree rather than one directory, so splitting routes into
	// subpackages does not silently drop the guard.
	err := filepath.WalkDir(".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		fset := token.NewFileSet()
		// Comments deliberately not parsed: only real call expressions count.
		f, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return fmt.Errorf("parse %s: %w", path, err)
		}

		// Resolve this file's import names, so an alias cannot hide a call.
		names := map[string]string{} // local name -> constructor func
		for _, imp := range f.Imports {
			importPath, uerr := strconv.Unquote(imp.Path.Value)
			if uerr != nil {
				continue
			}
			fn, ok := constructors[importPath]
			if !ok {
				continue
			}
			local := defaultImportName(importPath)
			if imp.Name != nil {
				local = imp.Name.Name
			}
			names[local] = fn
		}

		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			pkg, ok := sel.X.(*ast.Ident)
			if !ok {
				return true
			}
			if names[pkg.Name] == sel.Sel.Name {
				sites = append(sites, site{file: path, line: fset.Position(call.Pos()).Line})
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatalf("scan package sources: %v", err)
	}

	if len(sites) != 1 {
		for _, s := range sites {
			t.Logf("  instance constructed at %s:%d", s.file, s.line)
		}
		t.Fatalf("found %d Huma instance constructions in non-test sources, want exactly 1 (the main API "+
			"in SetupRoutes). A second instance owns a second OpenAPI document: its operations are "+
			"reachable but ABSENT from /openapi.json, and its own doc-route registration can shadow the "+
			"served spec. Use huma.NewGroup(rc.API, \"\") instead, with humaFromHTTP to carry any "+
			"net/http middleware onto it — see reports.go or auth.go.", len(sites))
	}

	// Pin WHERE it is, so the one instance cannot drift out of SetupRoutes
	// unnoticed. Update this deliberately if SetupRoutes is ever split.
	if got := filepath.Base(sites[0].file); got != "routes.go" {
		t.Errorf("the single Huma instance is constructed in %s, want routes.go (SetupRoutes)", got)
	}
}

// defaultImportName is the identifier an unaliased import binds. Usually the last
// path segment, EXCEPT for a Go module version suffix: `huma/v2` binds `huma`,
// not `v2`. Getting this wrong silently disables whichever constructor is
// version-suffixed, which is exactly what a negative control caught here.
func defaultImportName(importPath string) string {
	segments := strings.Split(importPath, "/")
	last := len(segments) - 1
	if last > 0 && len(segments[last]) > 1 && segments[last][0] == 'v' {
		if _, err := strconv.Atoi(segments[last][1:]); err == nil {
			last--
		}
	}
	return segments[last]
}

// --- PSY-1598: rate-limited operations moved onto the main API ---

// TestReportSubmitIsInMainSpec proves the point of the change: an operation
// that carries a rate limit now appears in the PUBLISHED spec.
//
// Before PSY-1598 this route lived on its own humachi.New inside a chi.Group,
// and a separate instance owns a separate OpenAPI document — so the operation
// was reachable but undocumented. It was the first to graduate out of that
// state; subapi_reports_test.go covers the eight that graduated last.
func TestReportSubmitIsInMainSpec(t *testing.T) {
	cfg := testConfig()
	sc := testContainer(cfg)
	router := chi.NewRouter()
	SetupRoutes(router, sc, cfg)

	req := httptest.NewRequest("GET", "/openapi.json", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	var spec struct {
		Paths map[string]map[string]interface{} `json:"paths"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &spec); err != nil {
		t.Fatalf("parse spec: %v", err)
	}

	item, ok := spec.Paths["/shows/{show_id}/report"]
	if !ok {
		t.Fatal("/shows/{show_id}/report is missing from the served spec")
	}
	if _, ok := item["post"]; !ok {
		t.Error("expected a documented POST operation for /shows/{show_id}/report")
	}
}

// TestReportSubmitStillRateLimited is the safety half. Moving the limiter from
// chi middleware to a Huma group must not weaken it: this is abuse protection,
// and a silently-disabled limiter looks exactly like a working one until
// someone floods the endpoint.
//
// The limiter is registered BEFORE the JWT middleware, so it applies to
// unauthenticated traffic too — which is the traffic that matters here. The
// burst therefore expects "anything but 429" up to the cap (401 is fine; the
// request got past the limiter) and 429 after it.
func TestReportSubmitStillRateLimited(t *testing.T) {
	cfg := testConfig()
	sc := testContainer(cfg)
	router := chi.NewRouter()
	SetupRoutes(router, sc, cfg)

	const ip = "203.0.113.7:1234" // fixed source so httprate keys consistently
	send := func() int {
		req := httptest.NewRequest("POST", "/shows/1/report", strings.NewReader(`{}`))
		req.Header.Set("Content-Type", "application/json")
		req.RemoteAddr = ip
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w.Code
	}

	limit := middleware.ReportRequestsPerMinute
	for i := 0; i < limit; i++ {
		if code := send(); code == http.StatusTooManyRequests {
			t.Fatalf("request %d/%d was rate limited early (429); the limiter is tighter than %d/min",
				i+1, limit, limit)
		}
	}

	if code := send(); code != http.StatusTooManyRequests {
		t.Errorf("request %d returned %d, want 429 — the rate limit did NOT survive the move to a Huma group",
			limit+1, code)
	}
}

// TestReportSubmitRateLimitIsPerIP guards the key function surviving the move:
// a different source address must get its own budget, or one abusive client
// would lock out everyone.
func TestReportSubmitRateLimitIsPerIP(t *testing.T) {
	cfg := testConfig()
	sc := testContainer(cfg)
	router := chi.NewRouter()
	SetupRoutes(router, sc, cfg)

	send := func(ip string) int {
		req := httptest.NewRequest("POST", "/shows/1/report", strings.NewReader(`{}`))
		req.Header.Set("Content-Type", "application/json")
		req.RemoteAddr = ip
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w.Code
	}

	// Exhaust one client's budget.
	for i := 0; i <= middleware.ReportRequestsPerMinute; i++ {
		send("198.51.100.1:5000")
	}
	if code := send("198.51.100.1:5000"); code != http.StatusTooManyRequests {
		t.Fatalf("first client should be limited, got %d", code)
	}

	if code := send("198.51.100.2:5000"); code == http.StatusTooManyRequests {
		t.Error("a different IP was rate limited by the first client's budget; KeyByIP did not survive the move")
	}
}
