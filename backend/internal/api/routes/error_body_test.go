package routes

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/go-chi/chi/v5"

	"psychic-homily-backend/internal/config"
	"psychic-homily-backend/internal/services"
)

// SetupRoutes must leave huma's error constructors replaced, or a handler's
// raw service error is rendered into the response body. This drives a real
// route whose service fails (no database) and whose handler returns the
// service error as-is.
func TestSetupRoutesKeepsServiceErrorTextOutOfTheBody(t *testing.T) {
	prevNewError, prevNewErrorWithContext := huma.NewError, huma.NewErrorWithContext
	t.Cleanup(func() {
		huma.NewError = prevNewError
		huma.NewErrorWithContext = prevNewErrorWithContext
	})

	cfg := &config.Config{
		Server: config.ServerConfig{Addr: "localhost:8080"},
		JWT:    config.JWTConfig{SecretKey: "error-body-test-placeholder-secret-key", Expiry: 24},
		OAuth:  config.OAuthConfig{SecretKey: "error-body-test-placeholder-secret-key"},
	}
	router := chi.NewRouter()
	SetupRoutes(router, services.NewServiceContainer(nil, cfg), cfg)

	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/venues/search?q=bottle", nil))

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500; body: %s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	if strings.Contains(body, "database not initialized") {
		t.Errorf("the service error text reached the body: %s", body)
	}
	if strings.Contains(body, `"errors"`) {
		t.Errorf("the body carries an errors[] entry built from the service error: %s", body)
	}
}
