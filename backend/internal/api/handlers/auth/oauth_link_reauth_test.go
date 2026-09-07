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
		name            string
		hasPassword     bool
		hasPasskey      bool
		sessionIssuedAt time.Time
		want            linkReauthFactor
	}{
		{
			name:            "fresh session counts on its own",
			hasPassword:     true,
			sessionIssuedAt: now.Add(-1 * time.Minute),
			want:            reauthAlreadySatisfied,
		},
		{
			name:            "a session at the window edge no longer counts",
			hasPassword:     true,
			sessionIssuedAt: now.Add(-recentSessionWindow),
			want:            reauthPassword,
		},
		{
			name:            "stale session, account has a password",
			hasPassword:     true,
			sessionIssuedAt: now.Add(-2 * time.Hour),
			want:            reauthPassword,
		},
		{
			name:            "stale session, no password but a passkey",
			hasPasskey:      true,
			sessionIssuedAt: now.Add(-2 * time.Hour),
			want:            reauthPasskey,
		},
		{
			name:            "stale session, neither: the magic link is the floor",
			sessionIssuedAt: now.Add(-2 * time.Hour),
			want:            reauthMagicLink,
		},
		{
			// An API token carries no issue time. Unknown age is not evidence
			// of freshness, so it must fall to the account's own factor.
			name:            "unknown session age is not freshness",
			hasPassword:     true,
			sessionIssuedAt: time.Time{},
			want:            reauthPassword,
		},
		{
			// Clock skew between issuer and reader is ordinary, so a future
			// issue time counts as fresh rather than being refused. It stays
			// fresh for as long as the skew lasts, which is the accepted
			// trade and is documented on the function.
			name:            "a future issue time counts as fresh",
			hasPassword:     true,
			sessionIssuedAt: now.Add(1 * time.Minute),
			want:            reauthAlreadySatisfied,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := linkReauthFactorFor(tc.hasPassword, tc.hasPasskey, tc.sessionIssuedAt, now); got != tc.want {
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
