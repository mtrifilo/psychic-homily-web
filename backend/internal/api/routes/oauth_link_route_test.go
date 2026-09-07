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
// Registration is asserted against the BUILT tree, which is the only place the
// parameter name and the single registration are observable at all.
//
// The served request then pins 400. Only cmd/server calls SetupGoth, so this
// binary has no goth provider registered and gothic answers 400 for one it
// cannot resolve. That exact status is what proves the request REACHED the
// handler with no credential: 401 would mean the route gained a credential
// requirement, and anything earlier would mean it never arrived.
func TestAuthLoginRouteStaysUnauthenticated(t *testing.T) {
	router := newTestRouter(t)

	registered := matching(chiRoutes(t, router), http.MethodGet, "/auth/login/{}")
	if len(registered) != 1 || registered[0] != "/auth/login/{provider}" {
		t.Fatalf("GET /auth/login/{provider} registrations = %v, want exactly [/auth/login/{provider}]", registered)
	}

	req := httptest.NewRequest("GET", "/auth/login/google", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("GET /auth/login/google with no credential = %d, want %d (gothic's answer for an unregistered provider, which is how far an uncredentialed request gets)",
			w.Code, http.StatusBadRequest)
	}
}
