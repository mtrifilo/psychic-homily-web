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
