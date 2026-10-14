package main

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	authm "psychic-homily-backend/internal/models/auth"
)

// isLocalDB decides whether the confirm gate applies, so a wrong answer either
// blocks local dev or silently lets a write through against a deployed database.
func TestIsLocalDB_LoopbackHosts(t *testing.T) {
	for _, raw := range []string{
		"postgres://u:p@localhost:5432/psychicdb?sslmode=disable",
		"postgres://u:p@127.0.0.1:5432/psychicdb",
		"postgres://u:p@[::1]:5432/psychicdb",
		"postgres://u:p@db.localhost:5432/psychicdb",
	} {
		assert.True(t, isLocalDB(raw), "expected local: %s", raw)
	}
}

func TestIsLocalDB_RemoteHosts(t *testing.T) {
	for _, raw := range []string{
		"postgres://u:p@shuttle.proxy.rlwy.net:24983/railway",
		"postgres://u:p@postgres.railway.internal:5432/railway",
		"postgres://u:p@db.example.com:5432/psychic",
	} {
		assert.False(t, isLocalDB(raw), "expected remote: %s", raw)
	}
}

// An unparseable URL must fail SAFE (treated as remote) so the confirm gate
// still applies rather than being skipped by a parsing accident.
func TestIsLocalDB_UnparseableFailsSafe(t *testing.T) {
	assert.False(t, isLocalDB("host=localhost user=x password=y dbname=z"))
	assert.False(t, isLocalDB("not a url"))
	assert.False(t, isLocalDB(""))
}

// ensureAccountUsable must agree with User.CanSignIn on every combination of
// the two fields, including the one users_live_account_check refuses, so the
// CLI never mints a token the API-token validator would reject.
func TestEnsureAccountUsable_AgreesWithCanSignIn(t *testing.T) {
	now := time.Now()
	cases := []struct {
		name        string
		user        *authm.User
		wantMessage string // empty when the account is usable
	}{
		{name: "live", user: &authm.User{IsActive: true, Email: strPtr("ok@test.com")}},
		{name: "deactivated", user: &authm.User{IsActive: false, Email: strPtr("off@test.com")}, wantMessage: "not active"},
		{name: "soft-deleted", user: &authm.User{IsActive: false, DeletedAt: &now, Email: strPtr("gone@test.com")}, wantMessage: "soft-deleted"},
		{name: "active with deleted_at set", user: &authm.User{IsActive: true, DeletedAt: &now, Email: strPtr("gap@test.com")}, wantMessage: "soft-deleted"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ensureAccountUsable(tc.user)
			require.Equal(t, tc.user.CanSignIn(), err == nil, "ensureAccountUsable and CanSignIn disagree: %v", err)
			if tc.wantMessage != "" {
				assert.Contains(t, err.Error(), tc.wantMessage)
			}
		})
	}
}

func TestEnsureAccountUsable_NilEmailDoesNotPanic(t *testing.T) {
	user := &authm.User{IsActive: false}
	assert.NotPanics(t, func() { _ = ensureAccountUsable(user) })
}

func TestRedactDBHost_DropsCredentials(t *testing.T) {
	got := redactDBHost("postgres://user:supersecret@db.example.com:5432/psychic")
	assert.Equal(t, "db.example.com:5432/psychic", got)
	assert.NotContains(t, got, "supersecret", "password must never survive redaction")
	assert.NotContains(t, got, "user", "username must never survive redaction")
}

func TestRedactDBHost_DropsQueryStringCredentials(t *testing.T) {
	got := redactDBHost("postgres://u:p@db.example.com:5432/psychic?password=leaky")
	assert.NotContains(t, got, "leaky", "query-string password must not survive redaction")
}

func TestRedactDBHost_Unparseable(t *testing.T) {
	assert.Equal(t, "<unparseable>", redactDBHost("not a url"))
	assert.Equal(t, "<unparseable>", redactDBHost(""))
}

func TestDeref(t *testing.T) {
	assert.Equal(t, "", deref(nil))
	assert.Equal(t, "x", deref(strPtr("x")))
}

func strPtr(s string) *string { return &s }
