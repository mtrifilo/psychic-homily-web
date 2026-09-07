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
	// reauthAlreadySatisfied: the session itself is fresh enough to count.
	reauthAlreadySatisfied linkReauthFactor = "recent_session"
	reauthPassword         linkReauthFactor = "password"
	reauthPasskey          linkReauthFactor = "passkey"
	reauthMagicLink        linkReauthFactor = "magic_link"
)

// linkReauthFactorFor decides what a caller must present to attach a provider
// identity to user. It is the ONE place the rule lives; every surface that
// needs to know asks here rather than re-deriving it.
//
// The rule:
//
//   - a session younger than recentSessionWindow counts on its own
//   - otherwise the account's own factor: its password if one is set, else a
//     passkey if one is registered, else a magic-link confirmation to the
//     account's address
//
// The fallback order is by what the account actually holds, not by preference:
// an OAuth-only account has no password and need not have a passkey, and
// demanding one it does not hold would lock exactly those users out of
// connecting a second provider. The magic link is the floor because an account
// without an address has no way in at all.
//
// sessionIssuedAt is the session credential's issue time. A zero value means
// the age could not be established, which is not evidence of freshness and
// falls through to the account's own factor.
func linkReauthFactorFor(user *authm.User, sessionIssuedAt, now time.Time) linkReauthFactor {
	if !sessionIssuedAt.IsZero() && now.Sub(sessionIssuedAt) < recentSessionWindow {
		return reauthAlreadySatisfied
	}
	if user != nil && user.PasswordHash != nil && *user.PasswordHash != "" {
		return reauthPassword
	}
	if user != nil && len(user.PasskeyCredentials) > 0 {
		return reauthPasskey
	}
	return reauthMagicLink
}
