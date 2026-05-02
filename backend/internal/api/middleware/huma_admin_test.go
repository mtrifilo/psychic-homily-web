package middleware

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/danielgtaylor/huma/v2"

	authm "psychic-homily-backend/internal/models/auth"
)

// TestHumaAdminMiddleware_AdminUser_PassesThrough verifies that a request
// carrying an admin user reaches the next handler in the chain.
func TestHumaAdminMiddleware_AdminUser_PassesThrough(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/admin/stats", nil)
	ctx, _ := newHumaContext(t, req)

	// Inject an admin user the way HumaJWTMiddleware would.
	user := &authm.User{ID: 42, IsAdmin: true}
	ctx = huma.WithValue(ctx, UserContextKey, user)

	called := false
	HumaAdminMiddleware(ctx, func(next huma.Context) {
		called = true
		// User must still be reachable downstream.
		got := GetUserFromContext(next.Context())
		if got == nil || got.ID != 42 || !got.IsAdmin {
			t.Errorf("expected admin user (id=42) in context, got %+v", got)
		}
	})

	if !called {
		t.Fatal("next() was not called for an admin user")
	}
}

// TestHumaAdminMiddleware_NonAdmin_ShortCircuits403 verifies that a request
// from a non-admin user is rejected with 403 and that the next handler in
// the chain is never invoked.
func TestHumaAdminMiddleware_NonAdmin_ShortCircuits403(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/admin/stats", nil)
	ctx, rr := newHumaContext(t, req)

	user := &authm.User{ID: 7, IsAdmin: false}
	ctx = huma.WithValue(ctx, UserContextKey, user)

	called := false
	HumaAdminMiddleware(ctx, func(next huma.Context) {
		called = true
	})

	if called {
		t.Fatal("next() was called for a non-admin user — middleware did not short-circuit")
	}
	if rr.Code != http.StatusForbidden {
		t.Errorf("expected status 403, got %d", rr.Code)
	}

	var body JWTErrorResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to decode response body: %v", err)
	}
	if body.Success {
		t.Error("expected success=false")
	}
	if body.Message != "Admin access required" {
		t.Errorf("expected message 'Admin access required', got %q", body.Message)
	}
}

// TestHumaAdminMiddleware_NoUser_ShortCircuits403 verifies the defensive
// branch: if the user is missing from the context (JWT middleware was not
// chained, or stored nothing), the middleware refuses rather than 500-ing.
func TestHumaAdminMiddleware_NoUser_ShortCircuits403(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/admin/stats", nil)
	ctx, rr := newHumaContext(t, req)
	// No user in context.

	called := false
	HumaAdminMiddleware(ctx, func(next huma.Context) {
		called = true
	})

	if called {
		t.Fatal("next() was called when no user was in context")
	}
	if rr.Code != http.StatusForbidden {
		t.Errorf("expected status 403, got %d", rr.Code)
	}
}

// TestHumaAdminMiddleware_NilUser_ShortCircuits403 covers the edge case
// where a nil *authm.User pointer was explicitly stored at UserContextKey
// (as opposed to "no value at all"). The type assertion path differs
// slightly inside GetUserFromContext, so test both shapes.
func TestHumaAdminMiddleware_NilUser_ShortCircuits403(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/admin/stats", nil)
	ctx, rr := newHumaContext(t, req)

	var nilUser *authm.User
	ctx = huma.WithValue(ctx, UserContextKey, nilUser)

	called := false
	HumaAdminMiddleware(ctx, func(next huma.Context) {
		called = true
	})

	if called {
		t.Fatal("next() was called when user was nil")
	}
	if rr.Code != http.StatusForbidden {
		t.Errorf("expected status 403, got %d", rr.Code)
	}
}

// TestHumaAdminMiddleware_ChainedAfterJWT_NonAdmin verifies the realistic
// chain: HumaJWTMiddleware would validate the token and place a user in
// context; HumaAdminMiddleware then asserts IsAdmin. We synthesize that
// scenario without instantiating the JWT middleware (which needs a
// JWTService + DB) to keep this a tight unit test.
func TestHumaAdminMiddleware_ChainedAfterJWT_NonAdmin(t *testing.T) {
	// Step 1: JWT placed a non-admin user in context.
	req := httptest.NewRequest(http.MethodPost, "/admin/shows/1/approve", nil)
	ctx, rr := newHumaContext(t, req)
	user := &authm.User{ID: 99, IsAdmin: false, EmailVerified: true}
	ctx = huma.WithValue(ctx, UserContextKey, user)

	// Step 2: HumaAdminMiddleware runs. Verify it rejects.
	handlerInvoked := false
	HumaAdminMiddleware(ctx, func(next huma.Context) {
		handlerInvoked = true
	})

	if handlerInvoked {
		t.Fatal("admin handler was invoked for a non-admin user — gate failed")
	}
	if rr.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rr.Code)
	}
}

// Compile-time assertion that HumaAdminMiddleware matches huma's
// middleware signature. If huma changes its signature this fails to
// compile and we'll know immediately.
var _ func(ctx huma.Context, next func(huma.Context)) = HumaAdminMiddleware

// avoid unused context import in build environments without other tests.
var _ = context.Background
