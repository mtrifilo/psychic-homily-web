package auth

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

// PSY-1907: account-level alert defaults. Absent means inherit at every level
// (NULL column, missing alert type, missing channel), and the shipped values
// are in-app ON / email OFF.

func alertBoolPtr(b bool) *bool { return &b }

// The owner-locked shipped defaults (PSY-1892 decision 4). A user who has
// never opened settings has a NULL column and must land exactly here.
func TestResolveAccountAlertDefaults_NullInheritsShippedDefaults(t *testing.T) {
	resolved := ResolveAccountAlertDefaults(nil)

	for name, channels := range map[string]AlertChannelDefaults{
		"shows":    resolved.Shows,
		"releases": resolved.Releases,
	} {
		assert.True(t, channels.InApp, "%s in-app should default ON", name)
		assert.False(t, channels.Email, "%s email should default OFF (opt-in)", name)
	}
}

// The asymmetry is the whole reason this is JSONB and not two bool columns:
// each of the four cells has to be settable independently, including to false,
// without the other three moving.
func TestResolveAccountAlertDefaults_EachCellIsIndependent(t *testing.T) {
	raw := json.RawMessage(`{"shows":{"in_app":false},"releases":{"email":true}}`)

	resolved := ResolveAccountAlertDefaults(&raw)

	assert.False(t, resolved.Shows.InApp, "explicit false is stored, not lost to a zero value")
	assert.False(t, resolved.Shows.Email, "untouched channel still inherits OFF")
	assert.True(t, resolved.Releases.InApp, "untouched channel still inherits ON")
	assert.True(t, resolved.Releases.Email, "explicit opt-in applies")
}

// Bad data in one alert type must not take its sibling's overrides with it.
// On the email axis a whole-document fallback would silently discard an opt-in
// the user made.
func TestResolveAccountAlertDefaults_BadAlertTypeDoesNotPoisonItsSibling(t *testing.T) {
	raw := json.RawMessage(`{"shows":{"email":"not-a-bool"},"releases":{"email":true}}`)

	resolved := ResolveAccountAlertDefaults(&raw)

	assert.False(t, resolved.Shows.Email, "the unusable alert type falls back to shipped")
	assert.True(t, resolved.Shows.InApp)
	assert.True(t, resolved.Releases.Email, "the healthy sibling keeps its override")
}

// An unparseable or empty document is "no overrides", never an error: the
// resolution runs on every follow read and must not fail one.
func TestResolveAccountAlertDefaults_ToleratesUnusableDocuments(t *testing.T) {
	for name, doc := range map[string]string{
		"empty object":  `{}`,
		"empty string":  ``,
		"not an object": `"nonsense"`,
		"not json":      `not json`,
	} {
		t.Run(name, func(t *testing.T) {
			raw := json.RawMessage(doc)
			resolved := ResolveAccountAlertDefaults(&raw)
			assert.True(t, resolved.Shows.InApp)
			assert.False(t, resolved.Shows.Email)
		})
	}
}

// The write path names its keys as literals while the read path names them as
// struct tags. If those drift a write lands under a key the read never looks
// at, and the setting silently stops applying, so every cell has to survive a
// merge-then-resolve round trip.
func TestMergeAccountAlertDefaults_EveryChannelRoundTrips(t *testing.T) {
	merged, err := MergeAccountAlertDefaults(nil, AccountAlertDefaultsUpdate{
		Shows:    &AlertChannelDefaultsUpdate{InApp: alertBoolPtr(false), Email: alertBoolPtr(true)},
		Releases: &AlertChannelDefaultsUpdate{InApp: alertBoolPtr(false), Email: alertBoolPtr(true)},
	})
	assert.NoError(t, err)

	raw := json.RawMessage(merged)
	resolved := ResolveAccountAlertDefaults(&raw)
	assert.False(t, resolved.Shows.InApp)
	assert.True(t, resolved.Shows.Email)
	assert.False(t, resolved.Releases.InApp)
	assert.True(t, resolved.Releases.Email)
}

// A partial write must leave the cells it does not mention ABSENT, not pinned
// at today's resolved value — otherwise the first toggle a user touches would
// freeze the shipped defaults into their row forever.
func TestMergeAccountAlertDefaults_LeavesUnmentionedChannelsAbsent(t *testing.T) {
	merged, err := MergeAccountAlertDefaults(nil, AccountAlertDefaultsUpdate{
		Shows: &AlertChannelDefaultsUpdate{Email: alertBoolPtr(true)},
	})
	assert.NoError(t, err)
	assert.JSONEq(t, `{"shows":{"email":true}}`, string(merged),
		"only the cell the user set is stored")
}

// A key this code does not model must survive a write: an alert type added by
// a newer server must not be erased by an older one mid-deploy.
func TestMergeAccountAlertDefaults_PreservesUnmodelledKeys(t *testing.T) {
	stored := json.RawMessage(`{"comments":{"email":true},"shows":{"future_channel":"keep-me"}}`)

	merged, err := MergeAccountAlertDefaults(&stored, AccountAlertDefaultsUpdate{
		Shows: &AlertChannelDefaultsUpdate{Email: alertBoolPtr(true)},
	})
	assert.NoError(t, err)

	var doc map[string]json.RawMessage
	assert.NoError(t, json.Unmarshal(merged, &doc))
	assert.JSONEq(t, `{"email":true}`, string(doc["comments"]), "unmodelled alert type")

	var shows map[string]json.RawMessage
	assert.NoError(t, json.Unmarshal(doc["shows"], &shows))
	assert.JSONEq(t, `"keep-me"`, string(shows["future_channel"]), "unmodelled channel")
	assert.JSONEq(t, `true`, string(shows["email"]), "the new override applied")
}

// An update that sets nothing is not an override: it must produce no document
// at all so the caller leaves the column NULL rather than storing an empty
// object that says what NULL already says.
func TestMergeAccountAlertDefaults_EmptyUpdateWritesNothing(t *testing.T) {
	for name, update := range map[string]AccountAlertDefaultsUpdate{
		"no alert types": {},
		"present but every cell unset": {
			Shows:    &AlertChannelDefaultsUpdate{},
			Releases: &AlertChannelDefaultsUpdate{},
		},
	} {
		t.Run(name, func(t *testing.T) {
			assert.True(t, update.SetsNothing())

			merged, err := MergeAccountAlertDefaults(nil, update)
			assert.NoError(t, err)
			assert.Nil(t, merged)
		})
	}

	assert.False(t, AccountAlertDefaultsUpdate{
		Releases: &AlertChannelDefaultsUpdate{Email: alertBoolPtr(false)},
	}.SetsNothing(), "an explicit false IS an override")
}

// This column has exactly one writer, so an unparseable value is corruption
// rather than a sibling's data, and the write path is the only chance to
// repair it — quietly, rather than locking the user out of their settings.
func TestMergeAccountAlertDefaults_RepairsUnparseableDocument(t *testing.T) {
	stored := json.RawMessage(`not json`)

	merged, err := MergeAccountAlertDefaults(&stored, AccountAlertDefaultsUpdate{
		Shows: &AlertChannelDefaultsUpdate{Email: alertBoolPtr(true)},
	})
	assert.NoError(t, err)
	assert.JSONEq(t, `{"shows":{"email":true}}`, string(merged))
}

// Turning a channel back off must store false, not delete the key: deleting it
// would re-inherit the shipped ON and the toggle would appear not to work.
func TestMergeAccountAlertDefaults_ExplicitFalseSurvives(t *testing.T) {
	stored := json.RawMessage(`{"shows":{"in_app":true}}`)

	merged, err := MergeAccountAlertDefaults(&stored, AccountAlertDefaultsUpdate{
		Shows: &AlertChannelDefaultsUpdate{InApp: alertBoolPtr(false)},
	})
	assert.NoError(t, err)

	raw := json.RawMessage(merged)
	assert.False(t, ResolveAccountAlertDefaults(&raw).Shows.InApp)
}
