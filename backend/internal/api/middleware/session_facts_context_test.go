package middleware

import (
	"context"
	"testing"
	"time"

	authm "psychic-homily-backend/internal/models/auth"
	"psychic-homily-backend/internal/services/contracts"
)

// An anonymous context reports neither fact, so a handler on a route without
// the middleware cannot mistake it for a cookie session.
func TestSessionFactsAccessors_ZeroWhenAbsent(t *testing.T) {
	ctx := context.Background()
	if SessionCredentialSourceFromContext(ctx) != "" {
		t.Error("an anonymous context has no credential source")
	}
	if !GetSessionIssuedAtFromContext(ctx).IsZero() {
		t.Error("an anonymous context has no session age")
	}
}

// An API token presented in the auth cookie is not a cookie SESSION: the
// source is planted only for a session credential, so a renewing handler
// never hands a cookie to an API-token caller.
func TestWithPrincipal_APITokenRecordsNoSource(t *testing.T) {
	user := &authm.User{ID: 1}
	ctx := withPrincipal(context.Background(), user, contracts.SessionFacts{}, false, CredentialSourceCookie)
	if SessionCredentialSourceFromContext(ctx) != "" {
		t.Error("an API token in the cookie must record no source")
	}
	if IsSessionCredentialFromContext(ctx) {
		t.Error("an API token is not a session credential")
	}
	if GetUserFromContext(ctx) != user {
		t.Error("the principal is still planted")
	}
}

func TestWithPrincipal_SessionRecordsSourceAndFacts(t *testing.T) {
	issued := time.Now().Add(-time.Hour).Truncate(time.Second)
	facts := contracts.SessionFacts{AuthAt: issued, IssuedAt: issued}
	ctx := withPrincipal(context.Background(), &authm.User{ID: 1}, facts, true, CredentialSourceCookie)
	if SessionCredentialSourceFromContext(ctx) != CredentialSourceCookie {
		t.Error("a cookie session reads as one")
	}
	if !GetSessionIssuedAtFromContext(ctx).Equal(issued) || !GetSessionAuthTimeFromContext(ctx).Equal(issued) {
		t.Error("the session facts are planted")
	}
	header := withPrincipal(context.Background(), &authm.User{ID: 1}, facts, true, CredentialSourceHeader)
	if SessionCredentialSourceFromContext(header) != CredentialSourceHeader {
		t.Error("a Bearer session reads as one")
	}
}
