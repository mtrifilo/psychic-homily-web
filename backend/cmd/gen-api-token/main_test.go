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

func TestEnsureAccountUsable_ActiveUserPasses(t *testing.T) {
	user := &authm.User{IsActive: true, Email: strPtr("ok@test.com")}
	assert.NoError(t, ensureAccountUsable(user))
}

func TestEnsureAccountUsable_SoftDeletedRejected(t *testing.T) {
	now := time.Now()
	user := &authm.User{IsActive: true, DeletedAt: &now, Email: strPtr("gone@test.com")}
	err := ensureAccountUsable(user)
	require.Error(t, err, "soft-deleted accounts can be self-restored, reviving any token minted here")
	assert.Contains(t, err.Error(), "soft-deleted")
}

func TestEnsureAccountUsable_InactiveRejected(t *testing.T) {
	user := &authm.User{IsActive: false, Email: strPtr("off@test.com")}
	err := ensureAccountUsable(user)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not active")
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
