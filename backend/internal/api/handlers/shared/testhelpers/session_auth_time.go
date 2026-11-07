package testhelpers

import (
	"context"
	"time"

	"psychic-homily-backend/internal/api/middleware"
	authm "psychic-homily-backend/internal/models/auth"
)

// CtxWithSessionAuthTime builds what the JWT middlewares put in front of a
// handler: a principal and the time an authentication factor last completed
// for the credential the request presented. The zero time is what an API-token
// principal and a session minted before the auth_at claim both present.
func CtxWithSessionAuthTime(user *authm.User, authAt time.Time) context.Context {
	return context.WithValue(CtxWithUser(user), middleware.SessionAuthTimeContextKey, authAt)
}

// CtxWithSession is CtxWithSessionAuthTime for a principal the JWT
// middlewares established from a session JWT, as opposed to an API token
// (middleware.SessionCredentialContextKey).
func CtxWithSession(user *authm.User, authAt time.Time) context.Context {
	return context.WithValue(CtxWithSessionAuthTime(user, authAt), middleware.SessionCredentialContextKey, true)
}

// CtxWithSessionIssuedAt is CtxWithSession for a session token minted at
// issuedAt (middleware.SessionIssuedAtContextKey), which is how old the
// session is for renewal.
func CtxWithSessionIssuedAt(user *authm.User, authAt, issuedAt time.Time) context.Context {
	return context.WithValue(CtxWithSession(user, authAt), middleware.SessionIssuedAtContextKey, issuedAt)
}

// CtxWithCookieSession is CtxWithSessionIssuedAt for a session the request
// presented as the auth cookie rather than a Bearer header
// (middleware.SessionCredentialSourceContextKey).
func CtxWithCookieSession(user *authm.User, authAt, issuedAt time.Time) context.Context {
	return context.WithValue(CtxWithSessionIssuedAt(user, authAt, issuedAt), middleware.SessionCredentialSourceContextKey, middleware.CredentialSourceCookie)
}
