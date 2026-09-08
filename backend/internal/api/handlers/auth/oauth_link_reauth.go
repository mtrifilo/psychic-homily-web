package auth

import (
	"time"

	authm "psychic-homily-backend/internal/models/auth"
)

// Attaching a provider identity adds a way to sign in as the account, so it is
// the kind of change a stolen session must not be able to make on its own. The
// caller has to have proven the account recently.
//
// recentSessionWindow is what "recently" means. A user who just signed in and
// walked to Settings is inside it; a week-old cookie riding in a browser
// someone else is sitting at is not.
const recentSessionWindow = 10 * time.Minute

// linkReauthFactor is what the caller must present before a link is allowed.
type linkReauthFactor string

const (
	// reauthAlreadySatisfied: the session's own authentication is recent
	// enough to count.
	reauthAlreadySatisfied linkReauthFactor = "recent_session"
	reauthPassword         linkReauthFactor = "password"
	reauthPasskey          linkReauthFactor = "passkey"
	reauthMagicLink        linkReauthFactor = "magic_link"
)

// linkReauthFactorFor decides what a caller must present to attach a provider
// identity. It is the ONE place the rule lives.
//
// The rule:
//
//   - a session authenticated within recentSessionWindow counts on its own
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
// KNOWN GAP: no caller resolves hasPasskey yet, so a passkey-only account is
// reported as magic_link. Every non-satisfied factor takes the same path
// today, so this changes only the log label, not the decision.
//
// sessionAuthenticatedAt is when an authentication factor last completed for
// the session credential, from the token's auth_at claim rather than from its
// issue time: a renewal moves the issue time with no factor behind it. A zero
// value establishes no authentication time and is not evidence of freshness.
//
// A slightly future sessionAuthenticatedAt counts as fresh: the stamping clock
// and the reading clock need not agree to the second. The reader that supplies
// this value bounds how far ahead it may sit.
func linkReauthFactorFor(hasPassword, hasPasskey bool, sessionAuthenticatedAt, now time.Time) linkReauthFactor {
	if !sessionAuthenticatedAt.IsZero() && now.Sub(sessionAuthenticatedAt) < recentSessionWindow {
		return reauthAlreadySatisfied
	}
	if hasPassword {
		return reauthPassword
	}
	if hasPasskey {
		return reauthPasskey
	}
	return reauthMagicLink
}

// accountHasPassword reports whether user can be challenged for a password.
func accountHasPassword(user *authm.User) bool {
	return user != nil && user.PasswordHash != nil && *user.PasswordHash != ""
}
