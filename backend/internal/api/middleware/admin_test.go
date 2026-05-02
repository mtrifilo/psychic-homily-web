package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/danielgtaylor/huma/v2"

	authm "psychic-homily-backend/internal/models/auth"
)

// HumaAdminMiddleware tests
//
// PSY-423: this middleware enforces IsAdmin=true after JWTMiddleware has
// populated the user. We test it independently of JWT — the contract is "if
// the user in context isn't an admin, reject with 403".

func TestHumaAdminMiddleware_NoUser_Returns403(t *testing.T) {
	mw := HumaAdminMiddleware()
	req := httptest.NewRequest(http.MethodGet, "/admin/test", nil)
	ctx, rr := newHumaContext(t, req)

	called := false
	mw(ctx, func(huma.Context) { called = true })

	if called {
		t.Error("next() should not have been called for unauthenticated request")
	}
	if rr.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusForbidden)
	}

	var body AdminErrorResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to parse body: %v", err)
	}
	if body.Success {
		t.Error("Success should be false")
	}
	if body.ErrorCode == "" {
		t.Error("ErrorCode should be set")
	}
}

func TestHumaAdminMiddleware_NonAdmin_Returns403(t *testing.T) {
	mw := HumaAdminMiddleware()
	req := httptest.NewRequest(http.MethodGet, "/admin/test", nil)
	base, rr := newHumaContext(t, req)
	user := &authm.User{IsAdmin: false}
	user.ID = 42
	ctx := huma.WithValue(base, UserContextKey, user)

	called := false
	mw(ctx, func(huma.Context) { called = true })

	if called {
		t.Error("next() should not have been called for non-admin user")
	}
	if rr.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusForbidden)
	}

	var body AdminErrorResponse
	json.Unmarshal(rr.Body.Bytes(), &body)
	if !contains(body.Message, "Admin") && !contains(body.Message, "admin") {
		t.Errorf("Message should mention admin, got %q", body.Message)
	}
}

func TestHumaAdminMiddleware_AdminUser_PassesThrough(t *testing.T) {
	mw := HumaAdminMiddleware()
	req := httptest.NewRequest(http.MethodGet, "/admin/test", nil)
	base, rr := newHumaContext(t, req)
	user := &authm.User{IsAdmin: true}
	user.ID = 1
	ctx := huma.WithValue(base, UserContextKey, user)

	var capturedUser *authm.User
	called := false
	mw(ctx, func(next huma.Context) {
		called = true
		if u, ok := next.Context().Value(UserContextKey).(*authm.User); ok {
			capturedUser = u
		}
	})

	if !called {
		t.Fatal("next() should have been called for admin user")
	}
	if rr.Code != 0 && rr.Code != http.StatusOK {
		// SetStatus is not called by the middleware on the success path; the
		// recorder default is 200 once Write is called. Either is acceptable.
		t.Errorf("status = %d, expected 0 (unset) or 200", rr.Code)
	}
	if capturedUser == nil {
		t.Fatal("user should be propagated to next handler")
	}
	if !capturedUser.IsAdmin {
		t.Error("admin flag should be preserved")
	}
	if capturedUser.ID != 1 {
		t.Errorf("user ID = %d, want 1", capturedUser.ID)
	}
}

// TestHumaAdminMiddleware_RequestIDPropagated checks that the error response
// echoes the request ID so the frontend / Sentry can correlate.
func TestHumaAdminMiddleware_RequestIDPropagated(t *testing.T) {
	mw := HumaAdminMiddleware()
	req := httptest.NewRequest(http.MethodGet, "/admin/test", nil)
	ctx, rr := newHumaContextWithRequestID(t, req, "req-admin-7")

	mw(ctx, func(huma.Context) { t.Error("next() should not run") })

	var body AdminErrorResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("parse body: %v", err)
	}
	if body.RequestID != "req-admin-7" {
		t.Errorf("RequestID = %q, want req-admin-7", body.RequestID)
	}
}

func contains(haystack, needle string) bool {
	if len(needle) == 0 {
		return true
	}
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
