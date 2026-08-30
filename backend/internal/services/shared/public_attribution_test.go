package shared

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	authm "psychic-homily-backend/internal/models/auth"
	"psychic-homily-backend/internal/services/contracts"
)

func strPtr(s string) *string { return &s }

// privacyWithContributions builds the stored blob from the real contract type
// rather than hand-written JSON, so a field rename cannot leave these tests
// asserting against a key production no longer reads.
func privacyWithContributions(t *testing.T, level contracts.PrivacyLevel) *json.RawMessage {
	t.Helper()
	settings := contracts.DefaultPrivacySettings()
	settings.Contributions = level
	encoded, err := json.Marshal(settings)
	require.NoError(t, err)
	raw := json.RawMessage(encoded)
	return &raw
}

// ---------------------------------------------------------------------------
// HasPublicName
// ---------------------------------------------------------------------------

func TestHasPublicName(t *testing.T) {
	cases := []struct {
		name string
		user *authm.User
		want bool
	}{
		{"nil user", nil, false},
		{"zero id", &authm.User{}, false},
		{"display name", &authm.User{ID: 1, DisplayName: strPtr("Matt T")}, true},
		{"username", &authm.User{ID: 1, Username: strPtr("mtrifilo")}, true},
		{"first name", &authm.User{ID: 1, FirstName: strPtr("Jane")}, true},
		// Last name alone is NOT a public name: the chain never renders it on
		// its own, so treating it as one would promise a name the resolver
		// cannot produce.
		{"last name only", &authm.User{ID: 1, LastName: strPtr("Doe")}, false},
		{"email only", &authm.User{ID: 1, Email: strPtr("asdf@example.com")}, false},
		{"empty strings", &authm.User{ID: 1, DisplayName: strPtr(""), Username: strPtr(""), FirstName: strPtr("")}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, HasPublicName(tc.user))
		})
	}
}

// ---------------------------------------------------------------------------
// ResolvePublicUserName
// ---------------------------------------------------------------------------

func TestResolvePublicUserName_MatchesCanonicalChainOnPublicTiers(t *testing.T) {
	// Where a public tier exists, the public chain and the canonical one must
	// agree exactly — the narrowing is a cut at the bottom, not a different
	// ordering. Asserted against ResolveUserName itself so the two cannot drift.
	users := []*authm.User{
		{ID: 1, DisplayName: strPtr("Matt T"), Username: strPtr("mtrifilo")},
		{ID: 1, Username: strPtr("mtrifilo"), FirstName: strPtr("Matt")},
		{ID: 1, FirstName: strPtr("Jane"), LastName: strPtr("Doe")},
	}
	for _, u := range users {
		assert.Equal(t, ResolveUserName(u), ResolvePublicUserName(u))
	}
}

func TestResolvePublicUserName_DropsEmailTier(t *testing.T) {
	u := &authm.User{ID: 1, Email: strPtr("asdf@example.com")}

	assert.Equal(t, "asdf", ResolveUserName(u), "canonical chain still exposes the local part")
	got := ResolvePublicUserName(u)
	assert.Equal(t, AnonymousUserName, got)
	assert.False(t, strings.Contains(got, "asdf"), "email local-part leaked: %q", got)
}

func TestResolvePublicUserName_NilAndNameless(t *testing.T) {
	assert.Equal(t, AnonymousUserName, ResolvePublicUserName(nil))
	assert.Equal(t, AnonymousUserName, ResolvePublicUserName(&authm.User{}))
	assert.Equal(t, AnonymousUserName, ResolvePublicUserName(&authm.User{ID: 7}))
}

// ---------------------------------------------------------------------------
// ResolvePublicContributorCredit
// ---------------------------------------------------------------------------

func TestResolvePublicContributorCredit_CreditsAPublicContributor(t *testing.T) {
	credit := ResolvePublicContributorCredit(&authm.User{
		ID:          5,
		DisplayName: strPtr("Matt T"),
		Username:    strPtr("mtrifilo"),
	})

	assert.True(t, credit.Renderable())
	assert.Equal(t, "Matt T", credit.Name)
	// The LINK comes from the username even when the NAME came from
	// display_name: display_name is not a URL slug.
	require.NotNil(t, credit.Username)
	assert.Equal(t, "mtrifilo", *credit.Username)
}

func TestResolvePublicContributorCredit_HiddenContributionsSuppressesEverything(t *testing.T) {
	credit := ResolvePublicContributorCredit(&authm.User{
		ID:              5,
		DisplayName:     strPtr("Matt T"),
		Username:        strPtr("mtrifilo"),
		PrivacySettings: privacyWithContributions(t, contracts.PrivacyHidden),
	})

	assert.False(t, credit.Renderable())
	assert.Equal(t, "", credit.Name)
	assert.Nil(t, credit.Username)
}

// count_only is a real level on this setting, and it is NOT hidden: the
// leaderboard reads it as "publish the number, not the detail". A byline is
// detail about a single contribution, not an aggregate, so the credit stands —
// pinned here so a future edit that folds count_only into the hidden branch has
// to argue with a test rather than slip through.
func TestResolvePublicContributorCredit_CountOnlyStillCredits(t *testing.T) {
	credit := ResolvePublicContributorCredit(&authm.User{
		ID:              5,
		Username:        strPtr("mtrifilo"),
		PrivacySettings: privacyWithContributions(t, contracts.PrivacyCountOnly),
	})

	assert.True(t, credit.Renderable())
	assert.Equal(t, "mtrifilo", credit.Name)
}

func TestResolvePublicContributorCredit_EmailOnlyContributorIsUncredited(t *testing.T) {
	credit := ResolvePublicContributorCredit(&authm.User{
		ID:    5,
		Email: strPtr("asdf@example.com"),
	})

	assert.False(t, credit.Renderable())
	assert.Equal(t, "", credit.Name, "an email-derived name must never reach a byline")
}

// The terminal of the canonical chain is swallowed too: "added Jul 12 by
// Anonymous" asserts a person where the honest rendering is no byline at all.
func TestResolvePublicContributorCredit_NamelessContributorIsUncredited(t *testing.T) {
	credit := ResolvePublicContributorCredit(&authm.User{ID: 5})

	assert.False(t, credit.Renderable())
	assert.NotEqual(t, AnonymousUserName, credit.Name)
	assert.Equal(t, "", credit.Name)
}

func TestResolvePublicContributorCredit_PrivateProfileKeepsNameDropsLink(t *testing.T) {
	credit := ResolvePublicContributorCredit(&authm.User{
		ID:                5,
		Username:          strPtr("mtrifilo"),
		ProfileVisibility: "private",
	})

	assert.True(t, credit.Renderable(), "a private profile is still credited, as plain text")
	assert.Equal(t, "mtrifilo", credit.Name)
	assert.Nil(t, credit.Username, "linking would be a dead link: /users/{username} 404s")
}

// Both defensive ways into the privacy fallback. The column is NOT NULL with a
// default and holds jsonb, so neither is reachable through normal writes; what
// is pinned is that both read as the DEFAULTS (contributions visible) rather
// than as "unknown, therefore hide" — a divergent local rule here would blank
// every byline on any column-restricted read that forgot the column.
func TestResolvePublicContributorCredit_MalformedPrivacyFallsBackToDefaults(t *testing.T) {
	shapeMismatch := json.RawMessage(`{"contributions": {"unexpected": "shape"}}`)

	for name, settings := range map[string]*json.RawMessage{
		"nil":            nil,
		"shape mismatch": &shapeMismatch,
	} {
		t.Run(name, func(t *testing.T) {
			credit := ResolvePublicContributorCredit(&authm.User{
				ID:              5,
				Username:        strPtr("mtrifilo"),
				PrivacySettings: settings,
			})
			assert.True(t, credit.Renderable())
			assert.Equal(t, "mtrifilo", credit.Name)
		})
	}
}

func TestResolvePublicContributorCredit_NilAndZeroUser(t *testing.T) {
	assert.False(t, ResolvePublicContributorCredit(nil).Renderable())
	assert.False(t, ResolvePublicContributorCredit(&authm.User{}).Renderable())
}
