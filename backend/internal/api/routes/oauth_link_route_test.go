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
// Registration is asserted against the BUILT tree rather than inferred from a
// status, because a status cannot separate the two facts. This binary never
// calls SetupGoth, so no goth provider is registered and the handler answers
// gothic's own 400 for one it cannot resolve; that answer belongs to goth's
// configuration, not to the routing contract this file owns. The served
// request is left to carry the one claim it can carry alone: no credential,
// no 401.
func TestAuthLoginRouteStaysUnauthenticated(t *testing.T) {
	router := newTestRouter(t)

	registered := matching(chiRoutes(t, router), http.MethodGet, "/auth/login/{}")
	if len(registered) != 1 || registered[0] != "/auth/login/{provider}" {
		t.Fatalf("GET /auth/login/{provider} registrations = %v, want exactly [/auth/login/{provider}]", registered)
	}

	req := httptest.NewRequest("GET", "/auth/login/google", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code == http.StatusUnauthorized {
		t.Errorf("GET /auth/login/google with no credential = %d: the sign-in route must not require one", w.Code)
	}
}
