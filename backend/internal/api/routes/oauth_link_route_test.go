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
// Asserting the redirect rather than "not 401": a deleted route answers 404,
// which is also not 401, so the weaker check would pass on a missing route.
func TestAuthLoginRouteStaysUnauthenticated(t *testing.T) {
	router := newTestRouter(t)

	req := httptest.NewRequest("GET", "/auth/login/google", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusTemporaryRedirect {
		t.Errorf("GET /auth/login/google with no credential = %d, want %d (the provider redirect)",
			w.Code, http.StatusTemporaryRedirect)
	}
}
