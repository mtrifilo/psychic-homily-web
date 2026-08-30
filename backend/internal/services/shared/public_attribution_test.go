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
		{"display name", &authm.User{ID: 1, DisplayName: strptr("Matt T")}, true},
		{"username", &authm.User{ID: 1, Username: strptr("mtrifilo")}, true},
		{"first name", &authm.User{ID: 1, FirstName: strptr("Jane")}, true},
		// Last name alone is NOT a public name: the chain never renders it on
		// its own, so treating it as one would promise a name the resolver
		// cannot produce.
		{"last name only", &authm.User{ID: 1, LastName: strptr("Doe")}, false},
		{"email only", &authm.User{ID: 1, Email: strptr("asdf@example.com")}, false},
		{"empty strings", &authm.User{ID: 1, DisplayName: strptr(""), Username: strptr(""), FirstName: strptr("")}, false},
		// Whitespace is not a name. Registration stores first_name raw (only the
		// profile PATCH trims), so all of these are storable today, and
		// untrimmed each one is a non-empty string that satisfies every `!= ""`
		// gate and every `name &&` guard on the frontend — producing a byline
		// that reads "added Jul 12 by " with nothing after the "by".
		{"space", &authm.User{ID: 1, FirstName: strptr(" ")}, false},
		{"tab", &authm.User{ID: 1, FirstName: strptr("\t")}, false},
		{"newline", &authm.User{ID: 1, DisplayName: strptr("\n")}, false},
		{"padded name is still a name", &authm.User{ID: 1, FirstName: strptr("  Jane  ")}, true},
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
		{ID: 1, DisplayName: strptr("Matt T"), Username: strptr("mtrifilo")},
		{ID: 1, Username: strptr("mtrifilo"), FirstName: strptr("Matt")},
		{ID: 1, FirstName: strptr("Jane"), LastName: strptr("Doe")},
	}
	for _, u := range users {
		assert.Equal(t, ResolveUserName(u), ResolvePublicUserName(u))
	}
}

func TestResolvePublicUserName_DropsEmailTier(t *testing.T) {
	u := &authm.User{ID: 1, Email: strptr("asdf@example.com")}

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
		DisplayName: strptr("Matt T"),
		Username:    strptr("mtrifilo"),
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
		DisplayName:     strptr("Matt T"),
		Username:        strptr("mtrifilo"),
		PrivacySettings: privacyWithContributions(t, contracts.PrivacyHidden),
	})

	assert.False(t, credit.Renderable())
	assert.Equal(t, "", credit.Name)
	assert.Nil(t, credit.Username)
}

// count_only currently credits. This test pins the BEHAVIOUR, not a decision:
// the owner ruled on "hidden" (PSY-1866) and was never asked about count_only,
// and crediting is what the code did before PSY-1940 touched it, so this
// preserves the status quo rather than settling the question.
//
// The question is real and is recorded on ResolvePublicContributorCredit's
// gate 1: /users/{username}/contributions answers a count_only user with an
// empty list while this names them on an individual edit. If the owner rules
// that count_only should suppress, widen the gate to `!= PrivacyVisible`,
// update leaderboard.go's SQL to match, and rewrite this test — do not read it
// as prior approval.
func TestResolvePublicContributorCredit_CountOnlyStillCredits(t *testing.T) {
	credit := ResolvePublicContributorCredit(&authm.User{
		ID:              5,
		Username:        strptr("mtrifilo"),
		PrivacySettings: privacyWithContributions(t, contracts.PrivacyCountOnly),
	})

	assert.True(t, credit.Renderable())
	assert.Equal(t, "mtrifilo", credit.Name)
}

func TestResolvePublicContributorCredit_EmailOnlyContributorIsUncredited(t *testing.T) {
	credit := ResolvePublicContributorCredit(&authm.User{
		ID:    5,
		Email: strptr("asdf@example.com"),
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
		Username:          strptr("mtrifilo"),
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
	// Each of these leaves Contributions at the merged default (visible), for a
	// reason worth knowing: Go unmarshals `null` into a non-pointer field as a
	// NO-OP, and on a type mismatch it records the error but leaves that one
	// field at its existing value and carries on. Both are easy to guess wrong.
	shapeMismatch := json.RawMessage(`{"contributions": {"unexpected": "shape"}}`)
	explicitNull := json.RawMessage(`{"contributions": null}`)
	wrongType := json.RawMessage(`{"contributions": 123}`)
	notAnObject := json.RawMessage(`[]`)

	for name, settings := range map[string]*json.RawMessage{
		"nil":            nil,
		"shape mismatch": &shapeMismatch,
		"explicit null":  &explicitNull,
		"wrong type":     &wrongType,
		"not an object":  &notAnObject,
	} {
		t.Run(name, func(t *testing.T) {
			credit := ResolvePublicContributorCredit(&authm.User{
				ID:              5,
				Username:        strptr("mtrifilo"),
				PrivacySettings: settings,
			})
			assert.True(t, credit.Renderable())
			assert.Equal(t, "mtrifilo", credit.Name)
		})
	}
}

// A whitespace-only name must not be renderable, and a padded one must come
// back trimmed. The failure this prevents is a byline reading "added Jul 12 by "
// with an empty span after the "by"; the newline case additionally keeps a raw
// newline out of notification subjects, which is what display_name's own
// validator (handlers/auth) guards against and first_name never got.
//
// SCOPE, honestly: this is unicode.IsSpace, via strings.TrimSpace. An
// INVISIBLE-but-not-space name — a lone zero-width space (U+200B), or a bidi
// override (U+202E) — still renders as a credit, because those are format
// characters rather than whitespace. Stripping the whole Cf category here would
// be wrong: it also contains joiners that legitimate names in several scripts
// need. That is an input-validation problem at the write side, where
// first_name currently has no validator at all, not a resolver problem.
func TestResolvePublicContributorCredit_WhitespaceNameIsNotAName(t *testing.T) {
	for name, value := range map[string]string{
		"space":   " ",
		"tab":     "\t",
		"newline": "\n",
		"mixed":   " \t\n ",
	} {
		t.Run(name, func(t *testing.T) {
			credit := ResolvePublicContributorCredit(&authm.User{ID: 5, DisplayName: strptr(value)})
			assert.False(t, credit.Renderable(), "whitespace is not a name")
			assert.Equal(t, "", credit.Name)
		})
	}

	t.Run("padded name is trimmed", func(t *testing.T) {
		credit := ResolvePublicContributorCredit(&authm.User{ID: 5, DisplayName: strptr("  Matt T  ")})
		assert.True(t, credit.Renderable())
		assert.Equal(t, "Matt T", credit.Name)
	})
}

func TestResolvePublicContributorCredit_NilAndZeroUser(t *testing.T) {
	assert.False(t, ResolvePublicContributorCredit(nil).Renderable())
	assert.False(t, ResolvePublicContributorCredit(&authm.User{}).Renderable())
}

// ---------------------------------------------------------------------------
// ContributorProfileLink
// ---------------------------------------------------------------------------

// The link gate is shared by every tier, including admin, because a private
// profile 404s for everyone. Pinned separately from the credit so a future
// unmasking tier that reaches for ResolveUserUsername directly has something to
// fail against.
func TestContributorProfileLink(t *testing.T) {
	linked := ContributorProfileLink(&authm.User{ID: 1, Username: strptr("mtrifilo")})
	require.NotNil(t, linked)
	assert.Equal(t, "mtrifilo", *linked)

	assert.Nil(t, ContributorProfileLink(nil))
	assert.Nil(t, ContributorProfileLink(&authm.User{ID: 1}), "no username, no link")
	assert.Nil(t, ContributorProfileLink(&authm.User{ID: 1, Username: strptr("")}), "empty username is unset")
	assert.Nil(t,
		ContributorProfileLink(&authm.User{ID: 1, Username: strptr("mtrifilo"), ProfileVisibility: "private"}),
		"a private profile 404s, so the link would be dead")
}

// The tier list itself, guarded end to end rather than by sampling. For every
// user that HAS a public tier the two chains must agree exactly; for every user
// that does not, the public chain must refuse to publish whatever the canonical
// one produced. This is what makes "public is canonical minus the last tier"
// checkable rather than merely asserted, and it is the test that fails if a
// future edit inserts a tier into one chain and not the other.
func TestPublicChainIsCanonicalChainMinusEmailTier(t *testing.T) {
	cases := []struct {
		name string
		user *authm.User
	}{
		{"display name", &authm.User{ID: 1, DisplayName: strptr("Matt T"), Username: strptr("mtrifilo"), Email: strptr("m@example.com")}},
		{"username", &authm.User{ID: 1, Username: strptr("mtrifilo"), FirstName: strptr("Matt"), Email: strptr("m@example.com")}},
		{"first name", &authm.User{ID: 1, FirstName: strptr("Jane"), Email: strptr("j@example.com")}},
		{"first and last", &authm.User{ID: 1, FirstName: strptr("Jane"), LastName: strptr("Doe"), Email: strptr("j@example.com")}},
		{"blank tiers fall through", &authm.User{ID: 1, DisplayName: strptr(""), Username: strptr(""), FirstName: strptr("Jane")}},
		{"email only", &authm.User{ID: 1, Email: strptr("asdf@example.com")}},
		{"nothing at all", &authm.User{ID: 1}},
		{"zero id", &authm.User{}},
		{"nil", nil},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			canonical := ResolveUserName(tc.user)
			public := ResolvePublicUserName(tc.user)

			if HasPublicName(tc.user) {
				assert.Equal(t, canonical, public,
					"a user with a public tier must resolve identically on both chains")
				assert.NotEqual(t, AnonymousUserName, public)
				return
			}
			assert.Equal(t, AnonymousUserName, public,
				"a user with no public tier must not reach the canonical chain's answer")
			if tc.user != nil && tc.user.Email != nil && *tc.user.Email != "" && tc.user.ID != 0 {
				assert.NotEqual(t, canonical, public,
					"the email tier is exactly what the public chain must drop")
			}
		})
	}
}
