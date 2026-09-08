package shared

import (
	"context"
	"net/http"
	"time"

	"psychic-homily-backend/internal/api/middleware"
	autherrors "psychic-homily-backend/internal/errors"
	"psychic-homily-backend/internal/logger"
	authm "psychic-homily-backend/internal/models/auth"
)

// Issuing a credential, or attaching a new way to sign in, is the kind of
// change a stolen session must not be able to make on its own. The caller has
// to have proven the account recently.
//
// RecentSessionWindow is what "recently" means. A user who just signed in and
// walked to Settings is inside it; a week-old cookie riding in a browser
// someone else is sitting at is not.
const RecentSessionWindow = 10 * time.Minute

// ReauthFactor is what the caller must present before the operation is allowed.
type ReauthFactor string

const (
	// ReauthAlreadySatisfied: the session's own authentication is recent
	// enough to count.
	ReauthAlreadySatisfied ReauthFactor = "recent_session"
	ReauthPassword         ReauthFactor = "password"
	ReauthPasskey          ReauthFactor = "passkey"
	ReauthMagicLink        ReauthFactor = "magic_link"
)

// ReauthFactorFor decides what a caller must present to add a credential to an
// account. It is the ONE place the rule lives: the OAuth link path and every
// credential mint read it, and a second encoding of it would be a second rule.
//
// The rule:
//
//   - a session authenticated within RecentSessionWindow counts on its own
//   - otherwise the account's own factor: its password if one is set, else a
//     passkey if one is registered, else a magic-link confirmation
//
// The fallback order is by what the account actually holds, not by preference:
// an OAuth-only account has no password and need not have a passkey, and
// demanding one it does not hold would lock exactly those users out.
//
// The facts are parameters rather than a *User because the principal the JWT
// middleware supplies is loaded without its passkey relation, so a version
// that read the model would silently answer magic_link for every passkey
// account. Callers state what they know.
//
// KNOWN GAP: no caller resolves hasPasskey, so a passkey-only account is
// reported as magic_link. Every non-satisfied factor takes the same path, so
// this changes the log label, not the decision.
//
// sessionAuthenticatedAt is when an authentication factor last completed for
// the session credential, from the token's auth_at claim rather than from its
// issue time: a renewal moves the issue time with no factor behind it. A zero
// value establishes no authentication time and is not evidence of freshness,
// which is also what an API-token principal presents.
//
// A slightly future sessionAuthenticatedAt counts as fresh: the stamping clock
// and the reading clock need not agree to the second. The reader that supplies
// this value bounds how far ahead it may sit.
func ReauthFactorFor(hasPassword, hasPasskey bool, sessionAuthenticatedAt, now time.Time) ReauthFactor {
	if !sessionAuthenticatedAt.IsZero() && now.Sub(sessionAuthenticatedAt) < RecentSessionWindow {
		return ReauthAlreadySatisfied
	}
	if hasPassword {
		return ReauthPassword
	}
	if hasPasskey {
		return ReauthPasskey
	}
	return ReauthMagicLink
}

// AccountHasPassword reports whether user can be challenged for a password.
func AccountHasPassword(user *authm.User) bool {
	return user != nil && user.PasswordHash != nil && *user.PasswordHash != ""
}

// ReauthRequiredError is the refusal a credential mint answers with when the
// request's session did not authenticate recently enough.
//
// 403 rather than the browser redirect the OAuth link path uses: these are API
// calls made by fetch and by the CLI, and neither can follow a redirect to a
// sign-in page.
//
// The body is middleware.JWTErrorResponse, the shape the auth-layer middleware
// denials already write, so the frontend parses one envelope for every such
// refusal. Embedding it rather than restating its fields is what keeps that
// true. Success is left at its zero value, which is the false the envelope
// means.
type ReauthRequiredError struct {
	middleware.JWTErrorResponse
}

func (e *ReauthRequiredError) Error() string { return e.Message }

// GetStatus satisfies huma.StatusError, which is what makes huma serialize
// this value as the response body rather than wrapping it in an ErrorModel.
func (e *ReauthRequiredError) GetStatus() int { return http.StatusForbidden }

// RequireRecentSessionAuth returns nil when the request may mint a credential,
// and a *ReauthRequiredError when it may not.
//
// It reads the principal and the authentication time from the request context,
// so every mint asks the same question of the same facts. operation names the
// mint in the refusal log.
func RequireRecentSessionAuth(ctx context.Context, operation string) error {
	user := middleware.GetUserFromContext(ctx)

	// hasPasskey is false: see the KNOWN GAP on ReauthFactorFor.
	factor := ReauthFactorFor(
		AccountHasPassword(user),
		false,
		middleware.GetSessionAuthTimeFromContext(ctx),
		time.Now(),
	)
	if factor == ReauthAlreadySatisfied {
		return nil
	}

	var userID uint
	if user != nil {
		userID = user.ID
	}
	logger.AuthWarn(ctx, "credential_mint_refused_stale_session",
		"operation", operation,
		"user_id", userID,
		"required_factor", string(factor),
	)

	return &ReauthRequiredError{JWTErrorResponse: middleware.JWTErrorResponse{
		Message:   autherrors.ToExternalMessage(autherrors.CodeReauthRequired),
		ErrorCode: autherrors.CodeReauthRequired,
		RequestID: logger.GetRequestID(ctx),
	}}
}
