package middleware

import (
	"testing"
	"time"

	authm "psychic-homily-backend/internal/models/auth"
)

// All four branches of the rule, because each one is a different account shape
// and getting any of them wrong either weakens the gate or locks a whole class
// of account out of connecting a provider or minting a credential.
func TestReauthFactorFor(t *testing.T) {
	now := time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)

	cases := []struct {
		name                   string
		hasPassword            bool
		hasPasskey             bool
		sessionAuthenticatedAt time.Time
		want                   ReauthFactor
	}{
		{
			name:                   "fresh session counts on its own",
			hasPassword:            true,
			sessionAuthenticatedAt: now.Add(-1 * time.Minute),
			want:                   ReauthAlreadySatisfied,
		},
		{
			name:                   "a session at the window edge no longer counts",
			hasPassword:            true,
			sessionAuthenticatedAt: now.Add(-RecentSessionWindow),
			want:                   ReauthPassword,
		},
		{
			name:                   "stale session, account has a password",
			hasPassword:            true,
			sessionAuthenticatedAt: now.Add(-2 * time.Hour),
			want:                   ReauthPassword,
		},
		{
			name:                   "stale session, no password but a passkey",
			hasPasskey:             true,
			sessionAuthenticatedAt: now.Add(-2 * time.Hour),
			want:                   ReauthPasskey,
		},
		{
			name:                   "stale session, neither: the magic link is the floor",
			sessionAuthenticatedAt: now.Add(-2 * time.Hour),
			want:                   ReauthMagicLink,
		},
		{
			// A credential that establishes no authentication time at all: an
			// API token, or a session carrying no auth_at claim. Absence is not
			// evidence of freshness, so it falls to the account's own factor.
			name:                   "no authentication time is not freshness",
			hasPassword:            true,
			sessionAuthenticatedAt: time.Time{},
			want:                   ReauthPassword,
		},
		{
			// The reader's clock and the stamping clock need not agree to the
			// second, so a future time still counts here. How far ahead a stamp
			// may sit before it is discarded is decided by the reader that
			// supplies this value, not by this function.
			name:                   "a future authentication time counts as fresh",
			hasPassword:            true,
			sessionAuthenticatedAt: now.Add(1 * time.Minute),
			want:                   ReauthAlreadySatisfied,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ReauthFactorFor(tc.hasPassword, tc.hasPasskey, tc.sessionAuthenticatedAt, now); got != tc.want {
				t.Errorf("ReauthFactorFor = %q, want %q", got, tc.want)
			}
		})
	}
}

// AccountHasPassword is what turns a model into the fact the rule takes, so
// the shapes that are NOT a usable password are pinned here.
func TestAccountHasPassword(t *testing.T) {
	empty := ""
	hash := "$2a$not-a-real-hash"

	if AccountHasPassword(nil) {
		t.Error("a nil principal has no password")
	}
	if AccountHasPassword(&authm.User{}) {
		t.Error("an absent hash is not a password")
	}
	if AccountHasPassword(&authm.User{PasswordHash: &empty}) {
		t.Error("an empty hash is not a password")
	}
	if !AccountHasPassword(&authm.User{PasswordHash: &hash}) {
		t.Error("a set hash is a password")
	}
}
