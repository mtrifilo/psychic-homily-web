package auth

import (
	"testing"
	"time"

	authm "psychic-homily-backend/internal/models/auth"
)

func hashPtr(s string) *string { return &s }

// All four branches of the rule, because each one is a different account shape
// and getting any of them wrong either weakens the gate or locks a whole class
// of account out of connecting a provider.
func TestLinkReauthFactorFor(t *testing.T) {
	now := time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)

	passwordAccount := &authm.User{PasswordHash: hashPtr("$2a$argon-ish")}
	passkeyAccount := &authm.User{
		PasskeyCredentials: []authm.WebAuthnCredential{{UserID: 1}},
	}
	oauthOnlyAccount := &authm.User{}
	// A password column that exists but holds nothing is not a password.
	emptyPasswordAccount := &authm.User{PasswordHash: hashPtr("")}

	cases := []struct {
		name            string
		user            *authm.User
		sessionIssuedAt time.Time
		want            linkReauthFactor
	}{
		{
			name:            "fresh session counts on its own",
			user:            passwordAccount,
			sessionIssuedAt: now.Add(-1 * time.Minute),
			want:            reauthAlreadySatisfied,
		},
		{
			name:            "a session at the window edge no longer counts",
			user:            passwordAccount,
			sessionIssuedAt: now.Add(-recentSessionWindow),
			want:            reauthPassword,
		},
		{
			name:            "stale session, account has a password",
			user:            passwordAccount,
			sessionIssuedAt: now.Add(-2 * time.Hour),
			want:            reauthPassword,
		},
		{
			name:            "stale session, no password but a passkey",
			user:            passkeyAccount,
			sessionIssuedAt: now.Add(-2 * time.Hour),
			want:            reauthPasskey,
		},
		{
			name:            "stale session, neither: the magic link is the floor",
			user:            oauthOnlyAccount,
			sessionIssuedAt: now.Add(-2 * time.Hour),
			want:            reauthMagicLink,
		},
		{
			name:            "an empty password hash is not a password",
			user:            emptyPasswordAccount,
			sessionIssuedAt: now.Add(-2 * time.Hour),
			want:            reauthMagicLink,
		},
		{
			// An API token carries no issue time. Unknown age is not evidence
			// of freshness, so it must fall to the account's own factor.
			name:            "unknown session age is not freshness",
			user:            passwordAccount,
			sessionIssuedAt: time.Time{},
			want:            reauthPassword,
		},
		{
			// A clock skew that puts issuance in the future must not read as
			// stale, and must not read as fresh forever either; inside the
			// window it counts, which is what this pins.
			name:            "a slightly future issue time still counts",
			user:            passwordAccount,
			sessionIssuedAt: now.Add(1 * time.Minute),
			want:            reauthAlreadySatisfied,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := linkReauthFactorFor(tc.user, tc.sessionIssuedAt, now); got != tc.want {
				t.Errorf("linkReauthFactorFor = %q, want %q", got, tc.want)
			}
		})
	}
}

// A nil principal must not resolve to "satisfied" by accident.
func TestLinkReauthFactorFor_NilUserFallsToTheFloor(t *testing.T) {
	now := time.Now()
	if got := linkReauthFactorFor(nil, now.Add(-time.Hour), now); got != reauthMagicLink {
		t.Errorf("linkReauthFactorFor(nil) = %q, want %q", got, reauthMagicLink)
	}
}
