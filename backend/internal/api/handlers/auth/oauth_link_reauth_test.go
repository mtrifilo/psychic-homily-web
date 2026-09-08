package auth

import (
	"testing"
	"time"

	authm "psychic-homily-backend/internal/models/auth"
)

// All four branches of the rule, because each one is a different account shape
// and getting any of them wrong either weakens the gate or locks a whole class
// of account out of connecting a provider.
func TestLinkReauthFactorFor(t *testing.T) {
	now := time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)

	cases := []struct {
		name                   string
		hasPassword            bool
		hasPasskey             bool
		sessionAuthenticatedAt time.Time
		want                   linkReauthFactor
	}{
		{
			name:                   "fresh session counts on its own",
			hasPassword:            true,
			sessionAuthenticatedAt: now.Add(-1 * time.Minute),
			want:                   reauthAlreadySatisfied,
		},
		{
			name:                   "a session at the window edge no longer counts",
			hasPassword:            true,
			sessionAuthenticatedAt: now.Add(-recentSessionWindow),
			want:                   reauthPassword,
		},
		{
			name:                   "stale session, account has a password",
			hasPassword:            true,
			sessionAuthenticatedAt: now.Add(-2 * time.Hour),
			want:                   reauthPassword,
		},
		{
			name:                   "stale session, no password but a passkey",
			hasPasskey:             true,
			sessionAuthenticatedAt: now.Add(-2 * time.Hour),
			want:                   reauthPasskey,
		},
		{
			name:                   "stale session, neither: the magic link is the floor",
			sessionAuthenticatedAt: now.Add(-2 * time.Hour),
			want:                   reauthMagicLink,
		},
		{
			// A credential that establishes no authentication time at all: an
			// API token, or a session carrying no auth_at claim. Absence is not
			// evidence of freshness, so it falls to the account's own factor.
			name:                   "no authentication time is not freshness",
			hasPassword:            true,
			sessionAuthenticatedAt: time.Time{},
			want:                   reauthPassword,
		},
		{
			// The reader's clock and the stamping clock need not agree to the
			// second, so a slightly future time still counts. Anything further
			// ahead never reaches this function: authTimeFromClaims discards it
			// past maxAuthTimeSkew and the caller sees the zero value.
			name:                   "a slightly future authentication time counts as fresh",
			hasPassword:            true,
			sessionAuthenticatedAt: now.Add(1 * time.Minute),
			want:                   reauthAlreadySatisfied,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := linkReauthFactorFor(tc.hasPassword, tc.hasPasskey, tc.sessionAuthenticatedAt, now); got != tc.want {
				t.Errorf("linkReauthFactorFor = %q, want %q", got, tc.want)
			}
		})
	}
}

// accountHasPassword is what turns a model into the fact the rule takes, so
// the shapes that are NOT a usable password are pinned here.
func TestAccountHasPassword(t *testing.T) {
	empty := ""
	hash := "$2a$not-a-real-hash"

	if accountHasPassword(nil) {
		t.Error("a nil principal has no password")
	}
	if accountHasPassword(&authm.User{}) {
		t.Error("an absent hash is not a password")
	}
	if accountHasPassword(&authm.User{PasswordHash: &empty}) {
		t.Error("an empty hash is not a password")
	}
	if !accountHasPassword(&authm.User{PasswordHash: &hash}) {
		t.Error("a set hash is a password")
	}
}
