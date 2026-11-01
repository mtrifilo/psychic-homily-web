package middleware

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"psychic-homily-backend/internal/logger"
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

// The gate names the factor from User.HasPassword, so an empty hash resolves
// like an absent one: the account is challenged for what it can actually
// present, never for a password nothing can verify against.
func TestRequireRecentSessionAuth_FactorFollowsTheAccountsPassword(t *testing.T) {
	empty := ""
	hash := "$2a$not-a-real-hash"
	stale := time.Now().Add(-2 * time.Hour)

	cases := []struct {
		name string
		user *authm.User
		want ReauthFactor
	}{
		{"absent hash", &authm.User{ID: 1}, ReauthMagicLink},
		{"empty hash", &authm.User{ID: 2, PasswordHash: &empty}, ReauthMagicLink},
		{"set hash", &authm.User{ID: 3, PasswordHash: &hash}, ReauthPassword},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			logs := newLogCapture(t)
			ctx := context.WithValue(context.Background(), UserContextKey, tc.user)
			ctx = context.WithValue(ctx, SessionAuthTimeContextKey, stale)
			ctx = logger.NewContext(ctx, logs.log)

			if err := RequireRecentSessionAuth(ctx, "test_mint"); err == nil {
				t.Fatal("a stale session must be refused")
			}
			var refusal map[string]any
			if err := json.Unmarshal(logs.buf.Bytes(), &refusal); err != nil {
				t.Fatalf("want exactly one JSON refusal line: %v: %s", err, logs.buf.String())
			}
			if refusal["msg"] != "credential_mint_refused_stale_session" {
				t.Fatalf("logged %v, want the stale-session refusal", refusal["msg"])
			}
			if got := refusal["required_factor"]; got != string(tc.want) {
				t.Errorf("required_factor = %v, want %q", got, tc.want)
			}
		})
	}
}
