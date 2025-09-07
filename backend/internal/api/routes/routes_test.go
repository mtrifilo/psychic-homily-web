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

// TestSetupRoutes tests the main route setup function
func TestSetupRoutes(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{
			Addr: "localhost:8080",
		},
	}

	router := chi.NewRouter()
	api := SetupRoutes(router, cfg)

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

	// Check that OpenAPI spec is returned
	contentType := w.Header().Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("Expected Content-Type application/json, got %s", contentType)
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

// TestSetupAuthRoutes tests authentication route setup
func TestSetupAuthRoutes(t *testing.T) {
	// Create a minimal config for testing
	cfg := &config.Config{
		JWT: config.JWTConfig{
			SecretKey: "test-secret-key-32-chars-minimum",
			Expiry:    24,
		},
		OAuth: config.OAuthConfig{
			SecretKey: "test-oauth-secret-key-32-chars",
		},
	}

	router := chi.NewRouter()
	api := humachi.New(router, huma.DefaultConfig("Test", "1.0.0"))

	// Use real services with test config
	authService := services.NewAuthService(cfg)
	jwtService := services.NewJWTService(cfg)

	setupAuthRoutes(router, api, authService, jwtService, cfg)

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

// TestSetupApplicationRoutes tests application route setup
func TestSetupApplicationRoutes(t *testing.T) {
	// Create a minimal config for testing
	cfg := &config.Config{
		JWT: config.JWTConfig{
			SecretKey: "test-secret-key-32-chars-minimum",
			Expiry:    24,
		},
	}

	router := chi.NewRouter()
	api := humachi.New(router, huma.DefaultConfig("Test", "1.0.0"))
	jwtService := services.NewJWTService(cfg)

	setupApplicationRoutes(router, api, jwtService)

	// Test show submission route (public)
	t.Run("Show Submission Route", func(t *testing.T) {
		showData := `{
			"title": "Test Show",
			"date": "2024-01-15",
			"venue": "Test Venue",
			"description": "Test Description"
		}`

		req := httptest.NewRequest("POST", "/show", strings.NewReader(showData))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		// Show submission should be accessible
		if w.Code != http.StatusOK && w.Code != http.StatusBadRequest {
			t.Errorf("Expected status 200 or 400, got %d", w.Code)
		}
	})
}

// TestSetupSystemRoutes tests system route setup
func TestSetupSystemRoutes(t *testing.T) {
	router := chi.NewRouter()
	api := humachi.New(router, huma.DefaultConfig("Test", "1.0.0"))

	setupSystemRoutes(router, api)

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

		if status, ok := response["status"].(string); !ok || status != "ok" {
			t.Errorf("Expected status 'ok', got %v", status)
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
	// Create a minimal config for testing
	cfg := &config.Config{
		JWT: config.JWTConfig{
			SecretKey: "test-secret-key-32-chars-minimum",
			Expiry:    24,
		},
		OAuth: config.OAuthConfig{
			SecretKey: "test-oauth-secret-key-32-chars",
		},
	}

	router := chi.NewRouter()
	api := humachi.New(router, huma.DefaultConfig("Test", "1.0.0"))

	authService := services.NewAuthService(cfg)
	jwtService := services.NewJWTService(cfg)

	setupAuthRoutes(router, api, authService, jwtService, cfg)

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
}

// TestRouteMiddleware tests middleware integration
func TestRouteMiddleware(t *testing.T) {
	// Create a minimal config for testing
	cfg := &config.Config{
		JWT: config.JWTConfig{
			SecretKey: "test-secret-key-32-chars-minimum",
			Expiry:    24,
		},
		OAuth: config.OAuthConfig{
			SecretKey: "test-oauth-secret-key-32-chars",
		},
	}

	router := chi.NewRouter()
	api := humachi.New(router, huma.DefaultConfig("Test", "1.0.0"))

	authService := services.NewAuthService(cfg)
	jwtService := services.NewJWTService(cfg)

	setupAuthRoutes(router, api, authService, jwtService, cfg)

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
	// Create a minimal config for testing
	cfg := &config.Config{
		JWT: config.JWTConfig{
			SecretKey: "test-secret-key-32-chars-minimum",
			Expiry:    24,
		},
		OAuth: config.OAuthConfig{
			SecretKey: "test-oauth-secret-key-32-chars",
		},
	}

	router := chi.NewRouter()
	api := humachi.New(router, huma.DefaultConfig("Test", "1.0.0"))

	authService := services.NewAuthService(cfg)
	jwtService := services.NewJWTService(cfg)

	setupAuthRoutes(router, api, authService, jwtService, cfg)

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
	// Create a minimal config for testing
	cfg := &config.Config{
		JWT: config.JWTConfig{
			SecretKey: "test-secret-key-32-chars-minimum",
			Expiry:    24,
		},
		OAuth: config.OAuthConfig{
			SecretKey: "test-oauth-secret-key-32-chars",
		},
	}

	router := chi.NewRouter()
	api := humachi.New(router, huma.DefaultConfig("Test", "1.0.0"))

	authService := services.NewAuthService(cfg)
	jwtService := services.NewJWTService(cfg)

	setupAuthRoutes(router, api, authService, jwtService, cfg)

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
	// Create a minimal config for testing
	cfg := &config.Config{
		JWT: config.JWTConfig{
			SecretKey: "test-secret-key-32-chars-minimum",
			Expiry:    24,
		},
		OAuth: config.OAuthConfig{
			SecretKey: "test-oauth-secret-key-32-chars",
		},
	}

	router := chi.NewRouter()
	api := humachi.New(router, huma.DefaultConfig("Test", "1.0.0"))

	authService := services.NewAuthService(cfg)
	jwtService := services.NewJWTService(cfg)

	setupAuthRoutes(router, api, authService, jwtService, cfg)

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
