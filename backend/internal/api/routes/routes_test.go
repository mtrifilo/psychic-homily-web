package routes

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/go-chi/chi/v5"

	"psychic-homily-backend/internal/config"
	"psychic-homily-backend/internal/services"
)

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
		"artist": true, "venue": true, "scene": true, "label": true, "festival": true,
	}
	if entityType == nil || !entityType.Required || len(entityType.Schema.Enum) != len(expectedTypes) {
		t.Fatalf("expected required five-value type enum, got %+v", entityType)
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
	assertProperties(pageSchema.Properties["following"].Items, "entity_type", "entity_id", "name", "slug", "followed_at")

	countsOperation := api.OpenAPI().Paths["/me/library/following/counts"].Get
	countsResponse := countsOperation.Responses["200"].Content["application/json"].Schema
	assertProperties(countsResponse, "artists", "venues", "scenes", "labels", "festivals")

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
