package middleware

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/humatest"

	"psychic-homily-backend/internal/config"
	"psychic-homily-backend/internal/logger"
	"psychic-homily-backend/internal/models"
	"psychic-homily-backend/internal/services"
)

// --- GetUserFromContext tests ---

func strPtr(s string) *string { return &s }

func TestGetUserFromContext_WithUser(t *testing.T) {
	email := "test@example.com"
	user := &models.User{Email: &email}
	ctx := context.WithValue(context.Background(), UserContextKey, user)

	got := GetUserFromContext(ctx)
	if got == nil {
		t.Fatal("expected user, got nil")
	}
	if got.Email == nil || *got.Email != "test@example.com" {
		t.Errorf("Email = %v, want test@example.com", got.Email)
	}
}

func TestGetUserFromContext_NoUser(t *testing.T) {
	ctx := context.Background()
	got := GetUserFromContext(ctx)
	if got != nil {
		t.Errorf("expected nil, got %v", got)
	}
}

func TestGetUserFromContext_WrongType(t *testing.T) {
	ctx := context.WithValue(context.Background(), UserContextKey, "not-a-user")
	got := GetUserFromContext(ctx)
	if got != nil {
		t.Errorf("expected nil for wrong type, got %v", got)
	}
}

// --- writeJWTError tests ---

func TestWriteJWTError(t *testing.T) {
	rr := httptest.NewRecorder()

	writeJWTError(rr, "req-123", "TOKEN_MISSING", "Authentication required", http.StatusUnauthorized)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusUnauthorized)
	}

	if got := rr.Header().Get("Content-Type"); got != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", got)
	}

	var body JWTErrorResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to parse body: %v", err)
	}

	if body.Success {
		t.Error("Success should be false")
	}
	if body.ErrorCode != "TOKEN_MISSING" {
		t.Errorf("ErrorCode = %q, want TOKEN_MISSING", body.ErrorCode)
	}
	if body.Message != "Authentication required" {
		t.Errorf("Message = %q, want 'Authentication required'", body.Message)
	}
	if body.RequestID != "req-123" {
		t.Errorf("RequestID = %q, want req-123", body.RequestID)
	}
}

func TestWriteJWTError_EmptyRequestID(t *testing.T) {
	rr := httptest.NewRecorder()

	writeJWTError(rr, "", "TOKEN_INVALID", "Invalid token", http.StatusUnauthorized)

	var body JWTErrorResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to parse body: %v", err)
	}

	if body.RequestID != "" {
		t.Errorf("RequestID = %q, want empty", body.RequestID)
	}
}

// --- JWTMiddleware tests (paths that don't hit DB) ---

func newTestJWTService() *services.JWTService {
	cfg := &config.Config{
		JWT: config.JWTConfig{
			SecretKey: "test-secret-key-for-unit-tests-32chars",
			Expiry:    24,
		},
	}
	return services.NewJWTService(nil, cfg)
}

func TestJWTMiddleware_NoToken(t *testing.T) {
	jwtService := newTestJWTService()
	handler := JWTMiddleware(jwtService)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("next handler should not be called")
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusUnauthorized)
	}

	var body JWTErrorResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to parse body: %v", err)
	}
	if body.ErrorCode != "TOKEN_MISSING" {
		t.Errorf("ErrorCode = %q, want TOKEN_MISSING", body.ErrorCode)
	}
}

func TestJWTMiddleware_InvalidAuthHeaderFormat(t *testing.T) {
	jwtService := newTestJWTService()
	handler := JWTMiddleware(jwtService)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("next handler should not be called")
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req.Header.Set("Authorization", "NotBearer some-token")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusUnauthorized)
	}

	var body JWTErrorResponse
	json.Unmarshal(rr.Body.Bytes(), &body)
	if body.ErrorCode != "TOKEN_MISSING" {
		t.Errorf("ErrorCode = %q, want TOKEN_MISSING", body.ErrorCode)
	}
}

func TestJWTMiddleware_InvalidToken(t *testing.T) {
	jwtService := newTestJWTService()
	handler := JWTMiddleware(jwtService)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("next handler should not be called")
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req.Header.Set("Authorization", "Bearer totally-invalid-token")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusUnauthorized)
	}

	var body JWTErrorResponse
	json.Unmarshal(rr.Body.Bytes(), &body)
	if body.ErrorCode != "TOKEN_INVALID" {
		t.Errorf("ErrorCode = %q, want TOKEN_INVALID", body.ErrorCode)
	}
}

func TestJWTMiddleware_InvalidTokenFromCookie(t *testing.T) {
	jwtService := newTestJWTService()
	handler := JWTMiddleware(jwtService)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("next handler should not be called")
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req.AddCookie(&http.Cookie{Name: "auth_token", Value: "bad-cookie-token"})
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusUnauthorized)
	}

	var body JWTErrorResponse
	json.Unmarshal(rr.Body.Bytes(), &body)
	if body.ErrorCode != "TOKEN_INVALID" {
		t.Errorf("ErrorCode = %q, want TOKEN_INVALID", body.ErrorCode)
	}
}

func TestJWTMiddleware_ExpiredToken(t *testing.T) {
	// Create a JWTService with 0 hour expiry to generate an already-expired token
	cfg := &config.Config{
		JWT: config.JWTConfig{
			SecretKey: "test-secret-key-for-unit-tests-32chars",
			Expiry:    0, // 0 hours = expires immediately
		},
	}
	jwtService := services.NewJWTService(nil, cfg)

	// Create a token (it will be expired since expiry is 0 hours from now)
	user := &models.User{Email: strPtr("test@example.com")}
	user.ID = 1
	token, err := jwtService.CreateToken(user)
	if err != nil {
		t.Fatalf("failed to create token: %v", err)
	}

	handler := JWTMiddleware(jwtService)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("next handler should not be called for expired token")
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusUnauthorized)
	}

	var body JWTErrorResponse
	json.Unmarshal(rr.Body.Bytes(), &body)
	if body.ErrorCode != "TOKEN_EXPIRED" {
		t.Errorf("ErrorCode = %q, want TOKEN_EXPIRED", body.ErrorCode)
	}
}

func TestJWTMiddleware_BearerTokenPreferredOverCookie(t *testing.T) {
	jwtService := newTestJWTService()

	handler := JWTMiddleware(jwtService)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("next handler should not be called")
	}))

	// Set both header and cookie — header token should be tried first
	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req.Header.Set("Authorization", "Bearer invalid-header-token")
	req.AddCookie(&http.Cookie{Name: "auth_token", Value: "invalid-cookie-token"})
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	// Should get TOKEN_INVALID (from header token), not TOKEN_MISSING
	var body JWTErrorResponse
	json.Unmarshal(rr.Body.Bytes(), &body)
	if body.ErrorCode != "TOKEN_INVALID" {
		t.Errorf("ErrorCode = %q, want TOKEN_INVALID (header token should be used)", body.ErrorCode)
	}
}

// --- Helper: create a Huma context with optional request ID in context ---

func newHumaContext(t *testing.T, req *http.Request) (huma.Context, *httptest.ResponseRecorder) {
	t.Helper()
	rr := httptest.NewRecorder()
	ctx := humatest.NewContext(nil, req, rr)
	return ctx, rr
}

func newHumaContextWithRequestID(t *testing.T, req *http.Request, requestID string) (huma.Context, *httptest.ResponseRecorder) {
	t.Helper()
	rr := httptest.NewRecorder()
	base := humatest.NewContext(nil, req, rr)
	ctx := huma.WithValue(base, logger.RequestIDContextKey, requestID)
	return ctx, rr
}

// --- writeHumaJWTError tests ---

func TestWriteHumaJWTError_BasicResponse(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	ctx, rr := newHumaContext(t, req)

	writeHumaJWTError(ctx, "req-abc", "TOKEN_MISSING", "Authentication required", nil)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusUnauthorized)
	}

	var body JWTErrorResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to parse body: %v", err)
	}
	if body.Success {
		t.Error("Success should be false")
	}
	if body.ErrorCode != "TOKEN_MISSING" {
		t.Errorf("ErrorCode = %q, want TOKEN_MISSING", body.ErrorCode)
	}
	if body.Message != "Authentication required" {
		t.Errorf("Message = %q, want 'Authentication required'", body.Message)
	}
	if body.RequestID != "req-abc" {
		t.Errorf("RequestID = %q, want req-abc", body.RequestID)
	}
}

func TestWriteHumaJWTError_WithSessionConfig(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	ctx, rr := newHumaContext(t, req)

	sessConfig := &config.SessionConfig{
		Path:     "/",
		Domain:   "example.com",
		HttpOnly: true,
		Secure:   true,
		SameSite: "strict",
	}

	writeHumaJWTError(ctx, "req-xyz", "TOKEN_INVALID", "Invalid token", sessConfig)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusUnauthorized)
	}

	// Should have Set-Cookie header to clear the auth cookie
	setCookie := rr.Header().Get("Set-Cookie")
	if setCookie == "" {
		t.Error("expected Set-Cookie header to clear auth cookie")
	}
}

func TestWriteHumaJWTError_NilSessionConfig(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	ctx, rr := newHumaContext(t, req)

	writeHumaJWTError(ctx, "", "TOKEN_EXPIRED", "Session expired", nil)

	// Should NOT have Set-Cookie header
	setCookie := rr.Header().Get("Set-Cookie")
	if setCookie != "" {
		t.Errorf("expected no Set-Cookie header, got %q", setCookie)
	}

	var body JWTErrorResponse
	json.Unmarshal(rr.Body.Bytes(), &body)
	if body.RequestID != "" {
		t.Errorf("RequestID = %q, want empty", body.RequestID)
	}
}

// --- HumaJWTMiddleware tests ---

func TestHumaJWTMiddleware_NoToken(t *testing.T) {
	jwtService := newTestJWTService()
	mw := HumaJWTMiddleware(jwtService)

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	ctx, rr := newHumaContext(t, req)

	nextCalled := false
	mw(ctx, func(next huma.Context) {
		nextCalled = true
	})

	if nextCalled {
		t.Error("next should not be called when no token")
	}

	var body JWTErrorResponse
	json.Unmarshal(rr.Body.Bytes(), &body)
	if body.ErrorCode != "TOKEN_MISSING" {
		t.Errorf("ErrorCode = %q, want TOKEN_MISSING", body.ErrorCode)
	}
}

func TestHumaJWTMiddleware_InvalidBearerToken(t *testing.T) {
	jwtService := newTestJWTService()
	mw := HumaJWTMiddleware(jwtService)

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req.Header.Set("Authorization", "Bearer totally-invalid-jwt")
	ctx, rr := newHumaContext(t, req)

	nextCalled := false
	mw(ctx, func(next huma.Context) {
		nextCalled = true
	})

	if nextCalled {
		t.Error("next should not be called for invalid token")
	}

	var body JWTErrorResponse
	json.Unmarshal(rr.Body.Bytes(), &body)
	if body.ErrorCode != "TOKEN_INVALID" {
		t.Errorf("ErrorCode = %q, want TOKEN_INVALID", body.ErrorCode)
	}
}

func TestHumaJWTMiddleware_ExpiredToken(t *testing.T) {
	// Create a JWTService with 0-hour expiry
	cfg := &config.Config{
		JWT: config.JWTConfig{
			SecretKey: "test-secret-key-for-unit-tests-32chars",
			Expiry:    0,
		},
	}
	jwtService := services.NewJWTService(nil, cfg)

	user := &models.User{Email: strPtr("test@example.com")}
	user.ID = 1
	token, err := jwtService.CreateToken(user)
	if err != nil {
		t.Fatalf("failed to create token: %v", err)
	}

	mw := HumaJWTMiddleware(jwtService)
	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	ctx, rr := newHumaContext(t, req)

	nextCalled := false
	mw(ctx, func(next huma.Context) {
		nextCalled = true
	})

	if nextCalled {
		t.Error("next should not be called for expired token")
	}

	var body JWTErrorResponse
	json.Unmarshal(rr.Body.Bytes(), &body)
	if body.ErrorCode != "TOKEN_EXPIRED" {
		t.Errorf("ErrorCode = %q, want TOKEN_EXPIRED", body.ErrorCode)
	}
}

func TestHumaJWTMiddleware_ValidJWT_FailsDBLookup(t *testing.T) {
	// A structurally valid JWT that passes signature verification but fails
	// the DB user lookup (no DB). This exercises the JWT parsing success path
	// up to the DB boundary. Full happy-path requires integration test with DB.
	jwtService := newTestJWTService()

	user := &models.User{Email: strPtr("valid@example.com")}
	user.ID = 42
	token, err := jwtService.CreateToken(user)
	if err != nil {
		t.Fatalf("failed to create token: %v", err)
	}

	mw := HumaJWTMiddleware(jwtService)
	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	ctx, rr := newHumaContext(t, req)

	nextCalled := false
	mw(ctx, func(next huma.Context) {
		nextCalled = true
	})

	// Token is valid but DB is nil, so user lookup fails → TOKEN_INVALID
	if nextCalled {
		t.Error("next should not be called when DB user lookup fails")
	}

	var body JWTErrorResponse
	json.Unmarshal(rr.Body.Bytes(), &body)
	if body.ErrorCode != "TOKEN_INVALID" {
		t.Errorf("ErrorCode = %q, want TOKEN_INVALID (DB user lookup failure)", body.ErrorCode)
	}
}

func TestHumaJWTMiddleware_CookieFallback(t *testing.T) {
	// Verifies that the middleware extracts the token from the Cookie header
	// when no Authorization header is present. DB lookup fails (no DB),
	// but the cookie extraction path is exercised.
	jwtService := newTestJWTService()

	user := &models.User{Email: strPtr("cookie@example.com")}
	user.ID = 7
	token, err := jwtService.CreateToken(user)
	if err != nil {
		t.Fatalf("failed to create token: %v", err)
	}

	mw := HumaJWTMiddleware(jwtService)
	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req.AddCookie(&http.Cookie{Name: "auth_token", Value: token})
	ctx, rr := newHumaContext(t, req)

	nextCalled := false
	mw(ctx, func(next huma.Context) {
		nextCalled = true
	})

	// Token extracted from cookie, parsed OK, but DB lookup fails
	if nextCalled {
		t.Error("next should not be called when DB user lookup fails")
	}

	var body JWTErrorResponse
	json.Unmarshal(rr.Body.Bytes(), &body)
	// Should get TOKEN_INVALID (not TOKEN_MISSING — cookie was found)
	if body.ErrorCode != "TOKEN_INVALID" {
		t.Errorf("ErrorCode = %q, want TOKEN_INVALID (cookie extracted, DB failed)", body.ErrorCode)
	}
}

func TestHumaJWTMiddleware_InvalidAPIToken(t *testing.T) {
	jwtService := newTestJWTService()
	mw := HumaJWTMiddleware(jwtService)

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req.Header.Set("Authorization", "Bearer phk_invalid-api-token")
	ctx, rr := newHumaContext(t, req)

	nextCalled := false
	mw(ctx, func(next huma.Context) {
		nextCalled = true
	})

	if nextCalled {
		t.Error("next should not be called for invalid API token")
	}

	var body JWTErrorResponse
	json.Unmarshal(rr.Body.Bytes(), &body)
	if body.ErrorCode != "TOKEN_INVALID" {
		t.Errorf("ErrorCode = %q, want TOKEN_INVALID", body.ErrorCode)
	}
}

func TestHumaJWTMiddleware_InvalidAuthHeaderFormat(t *testing.T) {
	jwtService := newTestJWTService()
	mw := HumaJWTMiddleware(jwtService)

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req.Header.Set("Authorization", "NotBearer some-token")
	ctx, rr := newHumaContext(t, req)

	nextCalled := false
	mw(ctx, func(next huma.Context) {
		nextCalled = true
	})

	if nextCalled {
		t.Error("next should not be called for invalid auth header format")
	}

	// No Bearer prefix extracted → falls through to cookie check → no cookie → TOKEN_MISSING
	var body JWTErrorResponse
	json.Unmarshal(rr.Body.Bytes(), &body)
	if body.ErrorCode != "TOKEN_MISSING" {
		t.Errorf("ErrorCode = %q, want TOKEN_MISSING", body.ErrorCode)
	}
}

func TestHumaJWTMiddleware_WithSessionConfig_ExpiredClearsCookie(t *testing.T) {
	cfg := &config.Config{
		JWT: config.JWTConfig{
			SecretKey: "test-secret-key-for-unit-tests-32chars",
			Expiry:    0,
		},
	}
	jwtService := services.NewJWTService(nil, cfg)

	user := &models.User{Email: strPtr("expired@example.com")}
	user.ID = 1
	token, _ := jwtService.CreateToken(user)

	sessConfig := config.SessionConfig{
		Path:     "/",
		Domain:   "example.com",
		HttpOnly: true,
		Secure:   true,
		SameSite: "strict",
	}

	mw := HumaJWTMiddleware(jwtService, sessConfig)
	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	ctx, rr := newHumaContext(t, req)

	mw(ctx, func(next huma.Context) {
		t.Error("next should not be called for expired token")
	})

	// Should have Set-Cookie header to clear the auth cookie
	setCookie := rr.Header().Get("Set-Cookie")
	if setCookie == "" {
		t.Error("expected Set-Cookie header to clear auth cookie on expired JWT")
	}
}

func TestHumaJWTMiddleware_RequestIDFromContext(t *testing.T) {
	jwtService := newTestJWTService()
	mw := HumaJWTMiddleware(jwtService)

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	ctx, rr := newHumaContextWithRequestID(t, req, "ctx-req-id-123")

	mw(ctx, func(next huma.Context) {
		t.Error("next should not be called when no token")
	})

	var body JWTErrorResponse
	json.Unmarshal(rr.Body.Bytes(), &body)
	// The request ID should be propagated to the error response
	if body.RequestID != "ctx-req-id-123" {
		t.Errorf("RequestID = %q, want ctx-req-id-123", body.RequestID)
	}
}

// --- LenientHumaJWTMiddleware tests ---

func TestLenientHumaJWTMiddleware_NoToken(t *testing.T) {
	jwtService := newTestJWTService()
	mw := LenientHumaJWTMiddleware(jwtService, 5*time.Minute)

	req := httptest.NewRequest(http.MethodGet, "/api/refresh", nil)
	ctx, rr := newHumaContext(t, req)

	nextCalled := false
	mw(ctx, func(next huma.Context) {
		nextCalled = true
	})

	if nextCalled {
		t.Error("next should not be called when no token")
	}

	var body JWTErrorResponse
	json.Unmarshal(rr.Body.Bytes(), &body)
	if body.ErrorCode != "TOKEN_MISSING" {
		t.Errorf("ErrorCode = %q, want TOKEN_MISSING", body.ErrorCode)
	}
}

func TestLenientHumaJWTMiddleware_InvalidToken(t *testing.T) {
	jwtService := newTestJWTService()
	mw := LenientHumaJWTMiddleware(jwtService, 5*time.Minute)

	req := httptest.NewRequest(http.MethodGet, "/api/refresh", nil)
	req.Header.Set("Authorization", "Bearer garbage-token")
	ctx, rr := newHumaContext(t, req)

	nextCalled := false
	mw(ctx, func(next huma.Context) {
		nextCalled = true
	})

	if nextCalled {
		t.Error("next should not be called for invalid token")
	}

	var body JWTErrorResponse
	json.Unmarshal(rr.Body.Bytes(), &body)
	if body.ErrorCode != "TOKEN_INVALID" {
		t.Errorf("ErrorCode = %q, want TOKEN_INVALID", body.ErrorCode)
	}
}

func TestLenientHumaJWTMiddleware_ValidToken_FailsDBLookup(t *testing.T) {
	// Valid JWT passes parsing but DB lookup fails (no DB).
	// LenientHumaJWTMiddleware delegates to ValidateTokenLenient → ValidateToken.
	jwtService := newTestJWTService()

	user := &models.User{Email: strPtr("lenient@example.com")}
	user.ID = 99
	token, err := jwtService.CreateToken(user)
	if err != nil {
		t.Fatalf("failed to create token: %v", err)
	}

	mw := LenientHumaJWTMiddleware(jwtService, 5*time.Minute)
	req := httptest.NewRequest(http.MethodGet, "/api/refresh", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	ctx, rr := newHumaContext(t, req)

	nextCalled := false
	mw(ctx, func(next huma.Context) {
		nextCalled = true
	})

	if nextCalled {
		t.Error("next should not be called when DB user lookup fails")
	}

	var body JWTErrorResponse
	json.Unmarshal(rr.Body.Bytes(), &body)
	if body.ErrorCode != "TOKEN_INVALID" {
		t.Errorf("ErrorCode = %q, want TOKEN_INVALID (DB lookup failed)", body.ErrorCode)
	}
}

func TestLenientHumaJWTMiddleware_ExpiredWithinGrace_FailsDBLookup(t *testing.T) {
	// Expired token within grace period: ValidateTokenLenient first tries strict
	// (fails with expired+DB), then re-parses without expiry check, passes grace
	// period check, but DB user lookup still fails.
	cfg := &config.Config{
		JWT: config.JWTConfig{
			SecretKey: "test-secret-key-for-unit-tests-32chars",
			Expiry:    0,
		},
	}
	jwtService := services.NewJWTService(nil, cfg)

	user := &models.User{Email: strPtr("grace@example.com")}
	user.ID = 55
	token, err := jwtService.CreateToken(user)
	if err != nil {
		t.Fatalf("failed to create token: %v", err)
	}

	// Grace period of 10 minutes should pass the grace check,
	// but DB lookup still fails → TOKEN_INVALID
	mw := LenientHumaJWTMiddleware(jwtService, 10*time.Minute)
	req := httptest.NewRequest(http.MethodGet, "/api/refresh", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	ctx, rr := newHumaContext(t, req)

	nextCalled := false
	mw(ctx, func(next huma.Context) {
		nextCalled = true
	})

	if nextCalled {
		t.Error("next should not be called when DB user lookup fails")
	}

	var body JWTErrorResponse
	json.Unmarshal(rr.Body.Bytes(), &body)
	// Token passes grace check but DB fails → TOKEN_INVALID
	if body.ErrorCode != "TOKEN_INVALID" {
		t.Errorf("ErrorCode = %q, want TOKEN_INVALID (grace OK, DB failed)", body.ErrorCode)
	}
}

func TestLenientHumaJWTMiddleware_ExpiredBeyondGrace(t *testing.T) {
	cfg := &config.Config{
		JWT: config.JWTConfig{
			SecretKey: "test-secret-key-for-unit-tests-32chars",
			Expiry:    0,
		},
	}
	jwtService := services.NewJWTService(nil, cfg)

	user := &models.User{Email: strPtr("expired@example.com")}
	user.ID = 1
	token, err := jwtService.CreateToken(user)
	if err != nil {
		t.Fatalf("failed to create token: %v", err)
	}

	// 0-second grace period — expired token should be rejected
	mw := LenientHumaJWTMiddleware(jwtService, 0)
	req := httptest.NewRequest(http.MethodGet, "/api/refresh", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	ctx, rr := newHumaContext(t, req)

	nextCalled := false
	mw(ctx, func(next huma.Context) {
		nextCalled = true
	})

	if nextCalled {
		t.Error("next should not be called for token expired beyond grace period")
	}

	var body JWTErrorResponse
	json.Unmarshal(rr.Body.Bytes(), &body)
	if body.ErrorCode != "TOKEN_EXPIRED" {
		t.Errorf("ErrorCode = %q, want TOKEN_EXPIRED", body.ErrorCode)
	}
}

func TestLenientHumaJWTMiddleware_CookieFallback(t *testing.T) {
	// Verifies token extraction from cookie in lenient middleware.
	// DB lookup fails but cookie path is exercised.
	jwtService := newTestJWTService()

	user := &models.User{Email: strPtr("cookie-lenient@example.com")}
	user.ID = 33
	token, err := jwtService.CreateToken(user)
	if err != nil {
		t.Fatalf("failed to create token: %v", err)
	}

	mw := LenientHumaJWTMiddleware(jwtService, 5*time.Minute)
	req := httptest.NewRequest(http.MethodGet, "/api/refresh", nil)
	req.AddCookie(&http.Cookie{Name: "auth_token", Value: token})
	ctx, rr := newHumaContext(t, req)

	nextCalled := false
	mw(ctx, func(next huma.Context) {
		nextCalled = true
	})

	if nextCalled {
		t.Error("next should not be called when DB user lookup fails")
	}

	var body JWTErrorResponse
	json.Unmarshal(rr.Body.Bytes(), &body)
	// Cookie was found and parsed, but DB lookup fails
	if body.ErrorCode != "TOKEN_INVALID" {
		t.Errorf("ErrorCode = %q, want TOKEN_INVALID (cookie extracted, DB failed)", body.ErrorCode)
	}
}

// --- OptionalHumaJWTMiddleware tests ---

func TestOptionalHumaJWTMiddleware_NoToken(t *testing.T) {
	jwtService := newTestJWTService()
	mw := OptionalHumaJWTMiddleware(jwtService)

	req := httptest.NewRequest(http.MethodGet, "/api/public", nil)
	ctx, _ := newHumaContext(t, req)

	nextCalled := false
	var userInCtx *models.User
	mw(ctx, func(next huma.Context) {
		nextCalled = true
		if u, ok := next.Context().Value(UserContextKey).(*models.User); ok {
			userInCtx = u
		}
	})

	if !nextCalled {
		t.Error("next should be called even without token")
	}
	if userInCtx != nil {
		t.Error("expected no user in context when no token provided")
	}
}

func TestOptionalHumaJWTMiddleware_ValidJWT_FailsDBLookup_ProceedsWithoutUser(t *testing.T) {
	// Valid JWT structure but DB lookup fails → optional middleware proceeds
	// without user (does NOT block the request).
	jwtService := newTestJWTService()

	user := &models.User{Email: strPtr("optional@example.com")}
	user.ID = 77
	token, err := jwtService.CreateToken(user)
	if err != nil {
		t.Fatalf("failed to create token: %v", err)
	}

	mw := OptionalHumaJWTMiddleware(jwtService)
	req := httptest.NewRequest(http.MethodGet, "/api/public", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	ctx, _ := newHumaContext(t, req)

	nextCalled := false
	var userInCtx *models.User
	mw(ctx, func(next huma.Context) {
		nextCalled = true
		if u, ok := next.Context().Value(UserContextKey).(*models.User); ok {
			userInCtx = u
		}
	})

	if !nextCalled {
		t.Error("next should be called (optional middleware proceeds even on validation failure)")
	}
	if userInCtx != nil {
		t.Error("expected no user in context when DB lookup fails")
	}
}

func TestOptionalHumaJWTMiddleware_InvalidJWT_ProceedsWithoutUser(t *testing.T) {
	jwtService := newTestJWTService()
	mw := OptionalHumaJWTMiddleware(jwtService)

	req := httptest.NewRequest(http.MethodGet, "/api/public", nil)
	req.Header.Set("Authorization", "Bearer invalid-jwt-token")
	ctx, _ := newHumaContext(t, req)

	nextCalled := false
	var userInCtx *models.User
	mw(ctx, func(next huma.Context) {
		nextCalled = true
		if u, ok := next.Context().Value(UserContextKey).(*models.User); ok {
			userInCtx = u
		}
	})

	if !nextCalled {
		t.Error("next should be called even with invalid token (optional middleware)")
	}
	if userInCtx != nil {
		t.Error("expected no user in context for invalid token")
	}
}

func TestOptionalHumaJWTMiddleware_ExpiredJWT_ProceedsWithoutUser(t *testing.T) {
	cfg := &config.Config{
		JWT: config.JWTConfig{
			SecretKey: "test-secret-key-for-unit-tests-32chars",
			Expiry:    0,
		},
	}
	jwtService := services.NewJWTService(nil, cfg)

	user := &models.User{Email: strPtr("expired@example.com")}
	user.ID = 1
	token, _ := jwtService.CreateToken(user)

	mw := OptionalHumaJWTMiddleware(jwtService)
	req := httptest.NewRequest(http.MethodGet, "/api/public", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	ctx, _ := newHumaContext(t, req)

	nextCalled := false
	var userInCtx *models.User
	mw(ctx, func(next huma.Context) {
		nextCalled = true
		if u, ok := next.Context().Value(UserContextKey).(*models.User); ok {
			userInCtx = u
		}
	})

	if !nextCalled {
		t.Error("next should be called even with expired token (optional middleware)")
	}
	if userInCtx != nil {
		t.Error("expected no user in context for expired token")
	}
}

func TestOptionalHumaJWTMiddleware_InvalidAPIToken_ProceedsWithoutUser(t *testing.T) {
	jwtService := newTestJWTService()
	mw := OptionalHumaJWTMiddleware(jwtService)

	req := httptest.NewRequest(http.MethodGet, "/api/public", nil)
	req.Header.Set("Authorization", "Bearer phk_invalid-api-token")
	ctx, _ := newHumaContext(t, req)

	nextCalled := false
	var userInCtx *models.User
	mw(ctx, func(next huma.Context) {
		nextCalled = true
		if u, ok := next.Context().Value(UserContextKey).(*models.User); ok {
			userInCtx = u
		}
	})

	if !nextCalled {
		t.Error("next should be called even with invalid API token (optional middleware)")
	}
	if userInCtx != nil {
		t.Error("expected no user in context for invalid API token")
	}
}

func TestOptionalHumaJWTMiddleware_CookieFallback_ProceedsWithoutUser(t *testing.T) {
	// Cookie token found but DB lookup fails → proceeds without user.
	jwtService := newTestJWTService()

	user := &models.User{Email: strPtr("cookie-optional@example.com")}
	user.ID = 11
	token, err := jwtService.CreateToken(user)
	if err != nil {
		t.Fatalf("failed to create token: %v", err)
	}

	mw := OptionalHumaJWTMiddleware(jwtService)
	req := httptest.NewRequest(http.MethodGet, "/api/public", nil)
	req.AddCookie(&http.Cookie{Name: "auth_token", Value: token})
	ctx, _ := newHumaContext(t, req)

	nextCalled := false
	var userInCtx *models.User
	mw(ctx, func(next huma.Context) {
		nextCalled = true
		if u, ok := next.Context().Value(UserContextKey).(*models.User); ok {
			userInCtx = u
		}
	})

	if !nextCalled {
		t.Error("next should be called (optional middleware proceeds on failure)")
	}
	if userInCtx != nil {
		t.Error("expected no user in context when DB lookup fails")
	}
}

func TestOptionalHumaJWTMiddleware_InvalidAuthHeaderFormat(t *testing.T) {
	jwtService := newTestJWTService()
	mw := OptionalHumaJWTMiddleware(jwtService)

	req := httptest.NewRequest(http.MethodGet, "/api/public", nil)
	req.Header.Set("Authorization", "Basic dXNlcjpwYXNz")
	ctx, _ := newHumaContext(t, req)

	nextCalled := false
	var userInCtx *models.User
	mw(ctx, func(next huma.Context) {
		nextCalled = true
		if u, ok := next.Context().Value(UserContextKey).(*models.User); ok {
			userInCtx = u
		}
	})

	if !nextCalled {
		t.Error("next should be called for non-Bearer auth header (optional middleware)")
	}
	if userInCtx != nil {
		t.Error("expected no user for non-Bearer auth header")
	}
}
