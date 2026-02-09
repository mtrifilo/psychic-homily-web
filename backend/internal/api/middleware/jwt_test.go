package middleware

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"psychic-homily-backend/internal/config"
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
