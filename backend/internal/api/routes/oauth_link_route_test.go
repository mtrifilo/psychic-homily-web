package routes

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// The whole difference between /auth/link and /auth/login is that the link
// resolves the account from a session. Registered without the JWT middleware
// it becomes a second, unauthenticated way to attach a provider identity to
// an account id, so the guard belongs on the ROUTER, not only in the handler.
func TestAuthLinkRouteRequiresASession(t *testing.T) {
	router := newTestRouter(t)

	req := httptest.NewRequest("GET", "/auth/link/google", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("GET /auth/link/google with no credential = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

// Its sibling stays open, because it is how a caller gets a session at all.
//
// Two assertions rather than one: "not 401" alone would also hold for a route
// that had been deleted, so the route's existence is checked separately. The
// status is pinned no tighter than that. This binary never calls SetupGoth, so
// no provider is registered and the handler answers with gothic's own 400
// rather than the provider redirect a configured server sends. That answer
// belongs to goth's configuration, not to the routing contract this file owns.
func TestAuthLoginRouteStaysUnauthenticated(t *testing.T) {
	router := newTestRouter(t)

	req := httptest.NewRequest("GET", "/auth/login/google", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code == http.StatusNotFound || w.Code == http.StatusMethodNotAllowed {
		t.Fatalf("GET /auth/login/google = %d: the sign-in route is not registered", w.Code)
	}
	if w.Code == http.StatusUnauthorized {
		t.Errorf("GET /auth/login/google with no credential = %d: the sign-in route must not require one", w.Code)
	}
}
