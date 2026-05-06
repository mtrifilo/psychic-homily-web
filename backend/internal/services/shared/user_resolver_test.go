package shared

import (
	"testing"

	"github.com/stretchr/testify/assert"

	authm "psychic-homily-backend/internal/models/auth"
)

// =============================================================================
// ResolveUserName — full-chain coverage
// =============================================================================

func TestResolveUserName_Nil(t *testing.T) {
	assert.Equal(t, "Anonymous", ResolveUserName(nil))
}

func TestResolveUserName_ZeroID(t *testing.T) {
	// A zero-ID user (e.g. an unloaded preload) should be treated as nil.
	assert.Equal(t, "Anonymous", ResolveUserName(&authm.User{}))
}

func TestResolveUserName_PrefersUsername(t *testing.T) {
	username := "ph_user"
	first := "Jane"
	last := "Doe"
	email := "jane@test.com"
	u := &authm.User{ID: 1, Username: &username, FirstName: &first, LastName: &last, Email: &email}
	assert.Equal(t, "ph_user", ResolveUserName(u))
}

func TestResolveUserName_FirstAndLast(t *testing.T) {
	first := "Jane"
	last := "Doe"
	email := "jane@test.com"
	u := &authm.User{ID: 1, FirstName: &first, LastName: &last, Email: &email}
	assert.Equal(t, "Jane Doe", ResolveUserName(u))
}

func TestResolveUserName_FirstOnly(t *testing.T) {
	first := "Jane"
	email := "jane@test.com"
	u := &authm.User{ID: 1, FirstName: &first, Email: &email}
	assert.Equal(t, "Jane", ResolveUserName(u))
}

func TestResolveUserName_FallbackToEmailPrefix(t *testing.T) {
	email := "dogfood@test.com"
	u := &authm.User{ID: 1, Email: &email}
	assert.Equal(t, "dogfood", ResolveUserName(u))
}

func TestResolveUserName_AnonymousWhenAllEmpty(t *testing.T) {
	emptyUsername := ""
	emptyFirst := ""
	u := &authm.User{ID: 1, Username: &emptyUsername, FirstName: &emptyFirst}
	assert.Equal(t, "Anonymous", ResolveUserName(u))
}

func TestResolveUserName_EmailWithoutAtSign(t *testing.T) {
	// Edge case: malformed email field that has no "@". Must fall through to
	// "Anonymous" rather than returning the raw value.
	bad := "no-at-sign"
	u := &authm.User{ID: 1, Email: &bad}
	assert.Equal(t, "Anonymous", ResolveUserName(u))
}

func TestResolveUserName_BlankUsernameFallsThrough(t *testing.T) {
	// A non-nil but empty username must fall through to first/last.
	blank := ""
	first := "Backup"
	u := &authm.User{ID: 1, Username: &blank, FirstName: &first}
	assert.Equal(t, "Backup", ResolveUserName(u))
}

// =============================================================================
// ResolveUserUsername — *string semantics
// =============================================================================

func TestResolveUserUsername_NilReturnsNil(t *testing.T) {
	assert.Nil(t, ResolveUserUsername(nil))
}

func TestResolveUserUsername_ZeroIDReturnsNil(t *testing.T) {
	assert.Nil(t, ResolveUserUsername(&authm.User{}))
}

func TestResolveUserUsername_NoUsernameReturnsNil(t *testing.T) {
	first := "Jane"
	u := &authm.User{ID: 1, FirstName: &first}
	assert.Nil(t, ResolveUserUsername(u))
}

func TestResolveUserUsername_BlankUsernameReturnsNil(t *testing.T) {
	blank := ""
	u := &authm.User{ID: 1, Username: &blank}
	assert.Nil(t, ResolveUserUsername(u))
}

func TestResolveUserUsername_ReturnsCopy(t *testing.T) {
	username := "ph_user"
	u := &authm.User{ID: 1, Username: &username}
	got := ResolveUserUsername(u)
	if assert.NotNil(t, got) {
		assert.Equal(t, "ph_user", *got)
		// Mutating the source must not affect the returned value.
		username = "mutated"
		assert.Equal(t, "ph_user", *got)
	}
}
