package engagement

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"gorm.io/gorm"

	apperrors "psychic-homily-backend/internal/errors"
	authm "psychic-homily-backend/internal/models/auth"
	engagementm "psychic-homily-backend/internal/models/engagement"
	notificationm "psychic-homily-backend/internal/models/notification"
	"psychic-homily-backend/internal/services/contracts"
)

// PSY-1893: following an artist or a venue subscribes the user to that
// entity's alerts; unfollowing unsubscribes.

// =============================================================================
// UNIT TESTS (No Database Required)
// =============================================================================

func boolPtr(b bool) *bool { return &b }

// shippedAlertDefaults is the account matrix a user who has configured nothing
// resolves to: the shipped defaults, which is what a NULL alert_defaults
// column means (PSY-1907). Most cases below are about the per-follow layer, so
// they hold the account layer at its untouched value.
func shippedAlertDefaults() authm.AccountAlertDefaults {
	return authm.ResolveAccountAlertDefaults(nil)
}

// The owner-locked defaults (PSY-1892 decision 4): in-app on, email off.
func TestDefaultFollowAlertPreference_InAppOnEmailOff(t *testing.T) {
	for _, entityType := range []string{"artist", "venue"} {
		for _, alertType := range []string{contracts.FollowAlertTypeShows, contracts.FollowAlertTypeReleases} {
			pref := defaultFollowAlertPreference(entityType, alertType, shippedAlertDefaults())
			assert.True(t, pref.Enabled, "%s/%s should default enabled", entityType, alertType)
			assert.True(t, pref.InApp, "%s/%s in-app should default ON", entityType, alertType)
			assert.False(t, pref.Email, "%s/%s email should default OFF", entityType, alertType)
		}
	}
}

// Decision 2: a new artist follow defaults to near-me scope. Venue shows and
// releases have no scope axis at all.
func TestDefaultFollowAlertPreference_ScopeAxis(t *testing.T) {
	assert.Equal(t, contracts.FollowAlertScopeNearMe,
		defaultFollowAlertPreference("artist", contracts.FollowAlertTypeShows, shippedAlertDefaults()).Scope)
	assert.Empty(t, defaultFollowAlertPreference("artist", contracts.FollowAlertTypeReleases, shippedAlertDefaults()).Scope)
	assert.Empty(t, defaultFollowAlertPreference("venue", contracts.FollowAlertTypeShows, shippedAlertDefaults()).Scope)
}

// PSY-1907: the channels a follow inherits come from the ACCOUNT matrix, so a
// user who turned email on (or in-app off) in settings gets that on every
// follow they never configured, with no data migration and no stamped rows.
func TestDefaultFollowAlertPreference_TakesChannelsFromAccountMatrix(t *testing.T) {
	raw := json.RawMessage(`{"shows":{"email":true},"releases":{"in_app":false}}`)
	account := authm.ResolveAccountAlertDefaults(&raw)

	shows := defaultFollowAlertPreference("artist", contracts.FollowAlertTypeShows, account)
	assert.True(t, shows.Email, "account email default reaches the follow")
	assert.True(t, shows.InApp, "channel the account left alone still inherits shipped ON")

	releases := defaultFollowAlertPreference("artist", contracts.FollowAlertTypeReleases, account)
	assert.False(t, releases.InApp, "per-alert-type, not one value for both")
	assert.False(t, releases.Email, "the shows override does not leak to releases")
}

// The account matrix is per alert type x CHANNEL only: it has no master switch
// and no area axis, so those two stay where PSY-1892 put them.
func TestDefaultFollowAlertPreference_AccountMatrixHasNoEnabledOrScopeAxis(t *testing.T) {
	raw := json.RawMessage(`{"shows":{"in_app":false,"email":false}}`)
	account := authm.ResolveAccountAlertDefaults(&raw)

	pref := defaultFollowAlertPreference("artist", contracts.FollowAlertTypeShows, account)
	assert.True(t, pref.Enabled, "both channels off is not the same as unsubscribed")
	assert.Equal(t, contracts.FollowAlertScopeNearMe, pref.Scope)
}

// Three layers, narrowest wins: a follow's own override beats the account
// default, which beats the shipped default.
func TestResolveFollowAlerts_FollowOverrideBeatsAccountDefault(t *testing.T) {
	accountRaw := json.RawMessage(`{"shows":{"email":true}}`)
	account := authm.ResolveAccountAlertDefaults(&accountRaw)
	settings := json.RawMessage(`{"alerts":{"shows":{"email":false}}}`)

	resolved := resolveFollowAlerts("artist", 42, &settings, account)

	assert.False(t, resolved.Shows.Email, "the follow's explicit opt-out wins")
	assert.False(t, resolved.Releases.Email, "releases inherit the shipped default")
}

// The account matrix's storage keys live in models/auth; the alert-type
// constants live in contracts. If those drift, an account default would be
// written under a key follow resolution never reads and the setting would
// silently stop applying with nothing failing.
func TestAccountAlertChannelsFor_KeysMatchFollowAlertTypes(t *testing.T) {
	for _, alertType := range []string{contracts.FollowAlertTypeShows, contracts.FollowAlertTypeReleases} {
		raw := json.RawMessage(fmt.Sprintf(`{%q:{"email":true}}`, alertType))
		account := authm.ResolveAccountAlertDefaults(&raw)

		assert.True(t, accountAlertChannelsFor(account, alertType).Email,
			"alert type %q must resolve through the account matrix", alertType)
	}
}

// Near me with no home area would scope to nothing and deliver nothing, so
// delivery degrades it to everywhere. The recorded value is untouched.
func TestEffectiveShowScope_NearMeFallsBackWithoutHomeArea(t *testing.T) {
	assert.Equal(t, contracts.FollowAlertScopeEverywhere,
		EffectiveShowScope(contracts.FollowAlertScopeNearMe, false))
	assert.Equal(t, contracts.FollowAlertScopeNearMe,
		EffectiveShowScope(contracts.FollowAlertScopeNearMe, true))
	assert.Equal(t, contracts.FollowAlertScopeEverywhere,
		EffectiveShowScope(contracts.FollowAlertScopeEverywhere, false))
	assert.Equal(t, contracts.FollowAlertScopeEverywhere,
		EffectiveShowScope(contracts.FollowAlertScopeEverywhere, true))
}

// A follow with no settings at all is already a full subscription.
func TestResolveFollowAlerts_AbsentSettingsInheritDefaults(t *testing.T) {
	resolved := resolveFollowAlerts("artist", 42, nil, shippedAlertDefaults())

	assert.Equal(t, "artist", resolved.EntityType)
	assert.Equal(t, uint(42), resolved.EntityID)
	assert.True(t, resolved.Shows.Enabled)
	assert.True(t, resolved.Shows.InApp)
	assert.False(t, resolved.Shows.Email)
	assert.Equal(t, contracts.FollowAlertScopeNearMe, resolved.Shows.Scope)
	if assert.NotNil(t, resolved.Releases) {
		assert.True(t, resolved.Releases.InApp)
		assert.False(t, resolved.Releases.Email)
	}
}

// A venue emits shows and nothing else, and its shows have no scope axis.
func TestResolveFollowAlerts_VenueHasNoReleasesAndNoScope(t *testing.T) {
	resolved := resolveFollowAlerts("venue", 9, nil, shippedAlertDefaults())

	assert.Nil(t, resolved.Releases)
	assert.Empty(t, resolved.Shows.Scope)
	assert.True(t, resolved.Shows.InApp)
	assert.False(t, resolved.Shows.Email)
}

// One stored override must not drag its siblings off the default.
func TestResolveFollowAlerts_PartialOverrideLeavesSiblingsInherited(t *testing.T) {
	settings := json.RawMessage(`{"alerts":{"shows":{"email":true}}}`)

	resolved := resolveFollowAlerts("artist", 42, &settings, shippedAlertDefaults())

	assert.True(t, resolved.Shows.Email, "explicit override applies")
	assert.True(t, resolved.Shows.InApp, "unset sibling still inherits")
	assert.Equal(t, contracts.FollowAlertScopeNearMe, resolved.Shows.Scope)
	assert.True(t, resolved.Releases.InApp)
	assert.False(t, resolved.Releases.Email)
}

// settings is a shared document: a key this file does not own, or a malformed
// alerts value, must not fail the read of a follow's subscription.
func TestResolveFollowAlerts_ToleratesForeignAndMalformedSettings(t *testing.T) {
	cases := map[string]string{
		"foreign key only": `{"scene_notify_mode":"off"}`,
		"empty document":   `{}`,
		"malformed alerts": `{"alerts":"nonsense"}`,
	}
	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			settings := json.RawMessage(raw)
			resolved := resolveFollowAlerts("artist", 42, &settings, shippedAlertDefaults())
			assert.True(t, resolved.Shows.InApp)
			assert.False(t, resolved.Shows.Email)
		})
	}
}

// A stale scope on an axis-less alert type is ignored rather than surfaced.
func TestResolveFollowAlerts_IgnoresScopeOnAxislessAlertType(t *testing.T) {
	settings := json.RawMessage(`{"alerts":{"shows":{"scope":"near_me"}}}`)

	assert.Empty(t, resolveFollowAlerts("venue", 9, &settings, shippedAlertDefaults()).Shows.Scope)
}

// Merging must preserve every key it does not own, at all three levels:
// the document, the alerts object, and one alert type's preference. settings
// is explicitly an additive shared document, so the first sibling key anyone
// adds must survive a write from this control.
func TestMergeFollowAlertSettings_PreservesUnmodelledKeysAtEveryLevel(t *testing.T) {
	settings := json.RawMessage(`{
		"scene_notify_mode":"off",
		"alerts":{
			"digest":{"weekly":true},
			"releases":{"email":true},
			"shows":{"future_axis":"keep-me"}
		}}`)

	merged, err := mergeFollowAlertSettings(&settings, contracts.FollowAlertUpdate{
		Shows: &contracts.FollowAlertPreferenceUpdate{InApp: boolPtr(false)},
	})
	assert.NoError(t, err)

	var doc map[string]json.RawMessage
	assert.NoError(t, json.Unmarshal(merged, &doc))
	assert.JSONEq(t, `"off"`, string(doc["scene_notify_mode"]), "document-level sibling")

	var alerts map[string]json.RawMessage
	assert.NoError(t, json.Unmarshal(doc["alerts"], &alerts))
	assert.JSONEq(t, `{"weekly":true}`, string(alerts["digest"]), "unmodelled alert type")

	var shows map[string]json.RawMessage
	assert.NoError(t, json.Unmarshal(alerts["shows"], &shows))
	assert.JSONEq(t, `"keep-me"`, string(shows["future_axis"]), "unmodelled axis")

	raw := json.RawMessage(merged)
	resolved := resolveFollowAlerts("artist", 42, &raw, shippedAlertDefaults())
	assert.False(t, resolved.Shows.InApp, "new override applied")
	assert.True(t, resolved.Releases.Email, "sibling alert type preserved")
}

// An update that sets no axis is not an override, whether the alert type is
// absent or present-but-empty: a stored empty object would say exactly what
// inheriting already says.
func TestMergeFollowAlertSettings_EmptyPreferenceUpdateWritesNothing(t *testing.T) {
	settings := json.RawMessage(`{}`)

	merged, err := mergeFollowAlertSettings(&settings, contracts.FollowAlertUpdate{
		Shows: &contracts.FollowAlertPreferenceUpdate{},
	})
	assert.NoError(t, err)
	assert.JSONEq(t, `{}`, string(merged))

	assert.True(t, followAlertUpdateSetsNothing(contracts.FollowAlertUpdate{
		Shows:    &contracts.FollowAlertPreferenceUpdate{},
		Releases: nil,
	}))
	assert.False(t, followAlertUpdateSetsNothing(contracts.FollowAlertUpdate{
		Releases: &contracts.FollowAlertPreferenceUpdate{Email: boolPtr(true)},
	}))
}

// The write path names its keys as literals while the read path names them as
// struct tags. If those ever drift, a write would land under a key the read
// never looks at and the setting would silently stop applying, so every axis
// has to survive a merge-then-resolve round trip.
func TestMergeFollowAlertSettings_EveryAxisRoundTrips(t *testing.T) {
	everywhere := contracts.FollowAlertScopeEverywhere
	settings := json.RawMessage(`{}`)

	merged, err := mergeFollowAlertSettings(&settings, contracts.FollowAlertUpdate{
		Shows: &contracts.FollowAlertPreferenceUpdate{
			Enabled: boolPtr(false),
			InApp:   boolPtr(false),
			Email:   boolPtr(true),
			Scope:   &everywhere,
		},
		Releases: &contracts.FollowAlertPreferenceUpdate{
			Enabled: boolPtr(false),
			InApp:   boolPtr(false),
			Email:   boolPtr(true),
		},
	})
	assert.NoError(t, err)

	raw := json.RawMessage(merged)
	resolved := resolveFollowAlerts("artist", 42, &raw, shippedAlertDefaults())
	assert.False(t, resolved.Shows.Enabled)
	assert.False(t, resolved.Shows.InApp)
	assert.True(t, resolved.Shows.Email)
	assert.Equal(t, everywhere, resolved.Shows.Scope)
	assert.False(t, resolved.Releases.Enabled)
	assert.False(t, resolved.Releases.InApp)
	assert.True(t, resolved.Releases.Email)
}

// Bad data in one alert type must not take its sibling's overrides with it:
// a whole-object decode would turn one unusable key into "every override on
// this follow disappeared", silently discarding an email opt-in the user made.
func TestResolveFollowAlerts_BadAlertTypeDoesNotPoisonItsSibling(t *testing.T) {
	settings := json.RawMessage(`{"alerts":{"shows":{"email":"not-a-bool"},"releases":{"email":true}}}`)

	resolved := resolveFollowAlerts("artist", 42, &settings, shippedAlertDefaults())

	assert.False(t, resolved.Shows.Email, "the unusable alert type falls back to the default")
	assert.True(t, resolved.Shows.InApp)
	assert.True(t, resolved.Releases.Email, "the healthy sibling keeps its override")
}

// A malformed nested value is unusable to every reader, so the write path is
// the one chance to repair it — without taking the document down with it.
func TestMergeFollowAlertSettings_RepairsMalformedNestedValues(t *testing.T) {
	settings := json.RawMessage(`{"scene_notify_mode":"off","alerts":{"shows":"nonsense"}}`)

	merged, err := mergeFollowAlertSettings(&settings, contracts.FollowAlertUpdate{
		Shows: &contracts.FollowAlertPreferenceUpdate{Email: boolPtr(true)},
	})
	assert.NoError(t, err)

	var doc map[string]json.RawMessage
	assert.NoError(t, json.Unmarshal(merged, &doc))
	assert.JSONEq(t, `"off"`, string(doc["scene_notify_mode"]))

	raw := json.RawMessage(merged)
	assert.True(t, resolveFollowAlerts("artist", 42, &raw, shippedAlertDefaults()).Shows.Email)
}

// An unparseable settings document is never overwritten: doing so would drop
// whatever else the row carried.
func TestMergeFollowAlertSettings_RefusesUnparseableDocument(t *testing.T) {
	settings := json.RawMessage(`not json`)

	_, err := mergeFollowAlertSettings(&settings, contracts.FollowAlertUpdate{
		Shows: &contracts.FollowAlertPreferenceUpdate{Email: boolPtr(true)},
	})
	assert.Error(t, err)
}

func TestFollowAlertSettings_InvalidEntityTypeRejectedBeforeDB(t *testing.T) {
	svc := &FollowService{db: &gorm.DB{}}

	for _, entityType := range []string{"label", "festival", "tag", "scene", "radio_show", "user", "banana"} {
		t.Run(entityType, func(t *testing.T) {
			_, err := svc.GetFollowAlertSettings(1, entityType, 1)
			assert.ErrorContains(t, err, "invalid entity type for follow")

			_, err = svc.SetFollowAlertSettings(1, entityType, 1, contracts.FollowAlertUpdate{})
			assert.ErrorContains(t, err, "invalid entity type for follow")
		})
	}
}

func TestValidateFollowAlertUpdate(t *testing.T) {
	nearMe := contracts.FollowAlertScopeNearMe
	bogus := "somewhere"

	// A rejected setting is a settings error, not an entity-type error: the
	// entity type is followable, the requested axis or value is not valid for
	// it. Sharing the entity-type constructor would prefix every one of these
	// with "invalid entity type for follow:" on a user-visible 422.
	assertAlertSettingsError := func(t *testing.T, err error, wantContains string) {
		t.Helper()
		var followErr *apperrors.FollowError
		if assert.ErrorAs(t, err, &followErr) {
			assert.Equal(t, apperrors.CodeFollowInvalidAlertSettings, followErr.Code)
			assert.Equal(t, wantContains, followErr.Message)
			assert.NotContains(t, followErr.Message, "invalid entity type")
		}
	}

	t.Run("venue rejects scope", func(t *testing.T) {
		err := validateFollowAlertUpdate("venue", contracts.FollowAlertUpdate{
			Shows: &contracts.FollowAlertPreferenceUpdate{Scope: &nearMe},
		})
		assertAlertSettingsError(t, err, "venue shows alerts have no scope axis")
	})

	t.Run("venue rejects releases", func(t *testing.T) {
		err := validateFollowAlertUpdate("venue", contracts.FollowAlertUpdate{
			Releases: &contracts.FollowAlertPreferenceUpdate{Email: boolPtr(true)},
		})
		assertAlertSettingsError(t, err, "venue follows have no release alerts")
	})

	t.Run("releases reject scope", func(t *testing.T) {
		err := validateFollowAlertUpdate("artist", contracts.FollowAlertUpdate{
			Releases: &contracts.FollowAlertPreferenceUpdate{Scope: &nearMe},
		})
		assertAlertSettingsError(t, err, "artist releases alerts have no scope axis")
	})

	t.Run("unknown scope rejected", func(t *testing.T) {
		err := validateFollowAlertUpdate("artist", contracts.FollowAlertUpdate{
			Shows: &contracts.FollowAlertPreferenceUpdate{Scope: &bogus},
		})
		assertAlertSettingsError(t, err, "invalid alert scope: somewhere")
	})

	t.Run("venue channel update accepted", func(t *testing.T) {
		assert.NoError(t, validateFollowAlertUpdate("venue", contracts.FollowAlertUpdate{
			Shows: &contracts.FollowAlertPreferenceUpdate{Email: boolPtr(true)},
		}))
	})
}

// The scope axis must be read from its predicate, never inferred from whether
// the default scope happens to be non-empty: a default that legitimately
// resolves to no scope would otherwise silently drop every stored override.
func TestFollowAlertHasScopeAxis(t *testing.T) {
	assert.True(t, followAlertHasScopeAxis("artist", contracts.FollowAlertTypeShows))
	assert.False(t, followAlertHasScopeAxis("artist", contracts.FollowAlertTypeReleases))
	assert.False(t, followAlertHasScopeAxis("venue", contracts.FollowAlertTypeShows))
}

// =============================================================================
// INTEGRATION TESTS (With Real Database)
// =============================================================================

// createTestFilter inserts a notification filter for a user. Column values are
// set explicitly rather than left to GORM zero-values, which the DB defaults
// would otherwise win.
func (suite *FollowServiceIntegrationTestSuite) createTestFilter(
	userID uint, name, source string, artistID uint, notifyEmail bool,
) *notificationm.NotificationFilter {
	filter := &notificationm.NotificationFilter{
		UserID:      userID,
		Name:        name,
		Source:      source,
		IsActive:    true,
		ArtistIDs:   pq.Int64Array{int64(artistID)},
		NotifyEmail: notifyEmail,
		NotifyInApp: true,
	}
	suite.Require().NoError(suite.db.Create(filter).Error)
	// GORM omits false booleans on Create, so the DB default (TRUE) wins;
	// re-assert the intended value.
	suite.Require().NoError(suite.db.Model(filter).
		Update("notify_email", notifyEmail).Error)
	filter.NotifyEmail = notifyEmail
	return filter
}

// rawSettings reads a follow row's settings column verbatim.
func (suite *FollowServiceIntegrationTestSuite) rawSettings(userID uint, entityType string, entityID uint) string {
	var settings []string
	suite.Require().NoError(suite.db.Model(&engagementm.UserBookmark{}).
		Where("user_id = ? AND entity_type = ? AND entity_id = ? AND action = ?",
			userID, entityType, entityID, engagementm.BookmarkActionFollow).
		Pluck("COALESCE(settings::text, '')", &settings).Error)
	suite.Require().Len(settings, 1)
	return settings[0]
}

// Following an artist IS subscribing: in-app on, email off, near-me scope,
// with no settings written to the row.
func (suite *FollowServiceIntegrationTestSuite) TestFollowArtist_SubscribesInAppOnEmailOff() {
	user := suite.createTestUser()
	artistID := suite.createTestArtist("alerts-artist")

	suite.Require().NoError(suite.followService.Follow(user.ID, "artist", artistID))

	settings, err := suite.followService.GetFollowAlertSettings(user.ID, "artist", artistID)
	suite.Require().NoError(err)
	suite.True(settings.Shows.Enabled)
	suite.True(settings.Shows.InApp, "in-app alerts default ON")
	suite.False(settings.Shows.Email, "email alerts default OFF (intentional opt-in)")
	suite.Equal(contracts.FollowAlertScopeNearMe, settings.Shows.Scope)
	suite.Require().NotNil(settings.Releases)
	suite.True(settings.Releases.InApp)
	suite.False(settings.Releases.Email)
	suite.Empty(settings.Releases.Scope, "releases are never scoped")

	suite.Empty(suite.rawSettings(user.ID, "artist", artistID),
		"a default subscription writes nothing, so the account default stays live")
}

// A venue follow subscribes to shows only, with no scope axis.
func (suite *FollowServiceIntegrationTestSuite) TestFollowVenue_SubscribesWithNoScopeAxis() {
	user := suite.createTestUser()
	venueID := suite.createTestVenue("alerts-venue")

	suite.Require().NoError(suite.followService.Follow(user.ID, "venue", venueID))

	settings, err := suite.followService.GetFollowAlertSettings(user.ID, "venue", venueID)
	suite.Require().NoError(err)
	suite.True(settings.Shows.InApp)
	suite.False(settings.Shows.Email)
	suite.Empty(settings.Shows.Scope)
	suite.Nil(settings.Releases)
}

// Unfollowing unsubscribes: the subscription goes with the follow row.
func (suite *FollowServiceIntegrationTestSuite) TestUnfollow_RemovesTheSubscription() {
	user := suite.createTestUser()
	artistID := suite.createTestArtist("unfollow-artist")

	suite.Require().NoError(suite.followService.Follow(user.ID, "artist", artistID))
	_, err := suite.followService.SetFollowAlertSettings(user.ID, "artist", artistID,
		contracts.FollowAlertUpdate{Shows: &contracts.FollowAlertPreferenceUpdate{Email: boolPtr(true)}})
	suite.Require().NoError(err)

	suite.Require().NoError(suite.followService.Unfollow(user.ID, "artist", artistID))

	_, err = suite.followService.GetFollowAlertSettings(user.ID, "artist", artistID)
	suite.Require().Error(err)
	var followErr *apperrors.FollowError
	suite.Require().ErrorAs(err, &followErr)
	suite.Equal(apperrors.CodeFollowNotFound, followErr.Code)

	// Re-following starts from the defaults again: unfollow left nothing behind.
	suite.Require().NoError(suite.followService.Follow(user.ID, "artist", artistID))
	settings, err := suite.followService.GetFollowAlertSettings(user.ID, "artist", artistID)
	suite.Require().NoError(err)
	suite.False(settings.Shows.Email)
}

// Re-POSTing follow must not reset a scope or channel the user chose.
func (suite *FollowServiceIntegrationTestSuite) TestFollowAgain_PreservesAlertOverrides() {
	user := suite.createTestUser()
	artistID := suite.createTestArtist("idempotent-artist")
	everywhere := contracts.FollowAlertScopeEverywhere

	suite.Require().NoError(suite.followService.Follow(user.ID, "artist", artistID))
	_, err := suite.followService.SetFollowAlertSettings(user.ID, "artist", artistID,
		contracts.FollowAlertUpdate{Shows: &contracts.FollowAlertPreferenceUpdate{
			Scope: &everywhere,
			Email: boolPtr(true),
		}})
	suite.Require().NoError(err)

	suite.Require().NoError(suite.followService.Follow(user.ID, "artist", artistID))

	settings, err := suite.followService.GetFollowAlertSettings(user.ID, "artist", artistID)
	suite.Require().NoError(err)
	suite.Equal(everywhere, settings.Shows.Scope)
	suite.True(settings.Shows.Email)

	var count int64
	suite.Require().NoError(suite.db.Model(&engagementm.UserBookmark{}).
		Where("user_id = ? AND entity_type = ? AND entity_id = ? AND action = ?",
			user.ID, "artist", artistID, engagementm.BookmarkActionFollow).
		Count(&count).Error)
	suite.Equal(int64(1), count, "follow stays idempotent")
}

// Scope is recorded as chosen; the no-home-area fallback belongs to delivery.
func (suite *FollowServiceIntegrationTestSuite) TestFollowAlerts_ScopeRoundTrips() {
	user := suite.createTestUser()
	artistID := suite.createTestArtist("scope-artist")
	suite.Require().NoError(suite.followService.Follow(user.ID, "artist", artistID))

	for _, scope := range []string{contracts.FollowAlertScopeEverywhere, contracts.FollowAlertScopeNearMe} {
		s := scope
		updated, err := suite.followService.SetFollowAlertSettings(user.ID, "artist", artistID,
			contracts.FollowAlertUpdate{Shows: &contracts.FollowAlertPreferenceUpdate{Scope: &s}})
		suite.Require().NoError(err)
		suite.Equal(scope, updated.Shows.Scope)

		read, err := suite.followService.GetFollowAlertSettings(user.ID, "artist", artistID)
		suite.Require().NoError(err)
		suite.Equal(scope, read.Shows.Scope)
	}

	// A near-me follow whose owner has no home area still reads back near_me;
	// only the delivery-time resolution degrades it.
	suite.Equal(contracts.FollowAlertScopeEverywhere,
		EffectiveShowScope(contracts.FollowAlertScopeNearMe, false))
}

// Per-channel control is per follow: turning email on for one alert type must
// not touch the other, nor the in-app channel.
func (suite *FollowServiceIntegrationTestSuite) TestFollowAlerts_PerChannelPerAlertType() {
	user := suite.createTestUser()
	artistID := suite.createTestArtist("channels-artist")
	suite.Require().NoError(suite.followService.Follow(user.ID, "artist", artistID))

	_, err := suite.followService.SetFollowAlertSettings(user.ID, "artist", artistID,
		contracts.FollowAlertUpdate{Shows: &contracts.FollowAlertPreferenceUpdate{Email: boolPtr(true)}})
	suite.Require().NoError(err)
	_, err = suite.followService.SetFollowAlertSettings(user.ID, "artist", artistID,
		contracts.FollowAlertUpdate{Releases: &contracts.FollowAlertPreferenceUpdate{Enabled: boolPtr(false)}})
	suite.Require().NoError(err)

	settings, err := suite.followService.GetFollowAlertSettings(user.ID, "artist", artistID)
	suite.Require().NoError(err)
	suite.True(settings.Shows.Email, "shows email override survived the second write")
	suite.True(settings.Shows.InApp, "in-app untouched")
	suite.False(settings.Releases.Enabled)
	suite.False(settings.Releases.Email, "releases email still off")
}

// A key another follow feature owns must survive an alerts write.
func (suite *FollowServiceIntegrationTestSuite) TestSetFollowAlertSettings_PreservesForeignSettingsKeys() {
	user := suite.createTestUser()
	artistID := suite.createTestArtist("foreign-key-artist")
	suite.Require().NoError(suite.followService.Follow(user.ID, "artist", artistID))
	suite.Require().NoError(suite.db.Model(&engagementm.UserBookmark{}).
		Where("user_id = ? AND entity_type = ? AND entity_id = ?", user.ID, "artist", artistID).
		Update("settings", gorm.Expr(`'{"experiment":"keep-me"}'::jsonb`)).Error)

	_, err := suite.followService.SetFollowAlertSettings(user.ID, "artist", artistID,
		contracts.FollowAlertUpdate{Shows: &contracts.FollowAlertPreferenceUpdate{Email: boolPtr(true)}})
	suite.Require().NoError(err)

	var doc map[string]json.RawMessage
	suite.Require().NoError(json.Unmarshal([]byte(suite.rawSettings(user.ID, "artist", artistID)), &doc))
	suite.JSONEq(`"keep-me"`, string(doc["experiment"]))
	suite.Contains(doc, "alerts")
}

// The subscription is per user: one user's override is invisible to another.
func (suite *FollowServiceIntegrationTestSuite) TestSetFollowAlertSettings_ScopedToTheCallingUser() {
	owner := suite.createTestUser()
	other := suite.createTestUser()
	artistID := suite.createTestArtist("multi-user-artist")
	suite.Require().NoError(suite.followService.Follow(owner.ID, "artist", artistID))
	suite.Require().NoError(suite.followService.Follow(other.ID, "artist", artistID))

	_, err := suite.followService.SetFollowAlertSettings(owner.ID, "artist", artistID,
		contracts.FollowAlertUpdate{Shows: &contracts.FollowAlertPreferenceUpdate{Email: boolPtr(true)}})
	suite.Require().NoError(err)

	othersSettings, err := suite.followService.GetFollowAlertSettings(other.ID, "artist", artistID)
	suite.Require().NoError(err)
	suite.False(othersSettings.Shows.Email, "another user's follow is untouched")
	suite.Empty(suite.rawSettings(other.ID, "artist", artistID))
}

// Configuring a follow that does not exist is a not-found, not a silent write.
func (suite *FollowServiceIntegrationTestSuite) TestFollowAlerts_NoFollowIsNotFound() {
	user := suite.createTestUser()
	artistID := suite.createTestArtist("unfollowed-artist")

	_, err := suite.followService.GetFollowAlertSettings(user.ID, "artist", artistID)
	suite.Require().Error(err)
	suite.Contains(err.Error(), apperrors.CodeFollowNotFound)

	_, err = suite.followService.SetFollowAlertSettings(user.ID, "artist", artistID,
		contracts.FollowAlertUpdate{Shows: &contracts.FollowAlertPreferenceUpdate{Email: boolPtr(true)}})
	suite.Require().Error(err)
	suite.Contains(err.Error(), apperrors.CodeFollowNotFound)
}

// An empty update is a no-op read, not an error and not a rewrite.
func (suite *FollowServiceIntegrationTestSuite) TestSetFollowAlertSettings_EmptyUpdateIsNoOp() {
	user := suite.createTestUser()
	artistID := suite.createTestArtist("noop-artist")
	suite.Require().NoError(suite.followService.Follow(user.ID, "artist", artistID))

	for name, update := range map[string]contracts.FollowAlertUpdate{
		"no alert types":           {},
		"present but all-axes-nil": {Shows: &contracts.FollowAlertPreferenceUpdate{}},
		"both present, all-nil":    {Shows: &contracts.FollowAlertPreferenceUpdate{}, Releases: &contracts.FollowAlertPreferenceUpdate{}},
	} {
		settings, err := suite.followService.SetFollowAlertSettings(user.ID, "artist", artistID, update)
		suite.Require().NoError(err, name)
		suite.True(settings.Shows.InApp, name)
		suite.False(settings.Shows.Email, name)
		suite.Empty(suite.rawSettings(user.ID, "artist", artistID),
			"%s: an update that sets nothing must not dirty the row", name)
	}
}

// The whole point of the source column: follow/unfollow must not create,
// mutate or delete a notification filter of either source. Existing Notify-me
// subscriptions and hand-built filters survive both operations.
func (suite *FollowServiceIntegrationTestSuite) TestFollowLifecycle_LeavesNotificationFiltersUntouched() {
	user := suite.createTestUser()
	artistID := suite.createTestArtist("filters-artist")
	userFilter := suite.createTestFilter(user.ID, "My hand-built filter", notificationm.FilterSourceUser, artistID, true)
	managedFilter := suite.createTestFilter(user.ID, "Notify me", notificationm.FilterSourceManaged, artistID, true)

	suite.Require().NoError(suite.followService.Follow(user.ID, "artist", artistID))
	_, err := suite.followService.SetFollowAlertSettings(user.ID, "artist", artistID,
		contracts.FollowAlertUpdate{Shows: &contracts.FollowAlertPreferenceUpdate{Email: boolPtr(true)}})
	suite.Require().NoError(err)
	suite.Require().NoError(suite.followService.Unfollow(user.ID, "artist", artistID))

	var filters []notificationm.NotificationFilter
	suite.Require().NoError(suite.db.Where("user_id = ?", user.ID).Order("id ASC").Find(&filters).Error)
	suite.Require().Len(filters, 2, "follow/unfollow neither created nor deleted a filter")

	for i, want := range []*notificationm.NotificationFilter{userFilter, managedFilter} {
		got := filters[i]
		suite.Equal(want.ID, got.ID)
		suite.Equal(want.Name, got.Name)
		suite.Equal(want.Source, got.Source)
		suite.Equal(want.IsActive, got.IsActive)
		suite.Equal(want.NotifyEmail, got.NotifyEmail)
		suite.Equal(want.NotifyInApp, got.NotifyInApp)
		suite.Equal(want.ArtistIDs, got.ArtistIDs)
	}
}

// Library pages carry each row's resolved subscription so the per-row alerts
// control renders without a request per row.
func (suite *FollowServiceIntegrationTestSuite) TestGetLibraryFollowing_CarriesAlertSettings() {
	user := suite.createTestUser()
	artistID := suite.createTestArtist("library-alerts-artist")
	venueID := suite.createTestVenue("library-alerts-venue")
	labelID := suite.createTestLabel("library-alerts-label")
	everywhere := contracts.FollowAlertScopeEverywhere

	suite.Require().NoError(suite.followService.Follow(user.ID, "artist", artistID))
	suite.Require().NoError(suite.followService.Follow(user.ID, "venue", venueID))
	suite.Require().NoError(suite.followService.Follow(user.ID, "label", labelID))
	_, err := suite.followService.SetFollowAlertSettings(user.ID, "artist", artistID,
		contracts.FollowAlertUpdate{Shows: &contracts.FollowAlertPreferenceUpdate{Scope: &everywhere}})
	suite.Require().NoError(err)

	artists, _, err := suite.followService.GetLibraryFollowing(user.ID, "artist", 50, nil)
	suite.Require().NoError(err)
	suite.Require().Len(artists, 1)
	suite.Require().NotNil(artists[0].Alerts)
	suite.Equal(everywhere, artists[0].Alerts.Shows.Scope, "stored override reaches the Library row")
	suite.True(artists[0].Alerts.Shows.InApp)
	suite.False(artists[0].Alerts.Shows.Email)

	venues, _, err := suite.followService.GetLibraryFollowing(user.ID, "venue", 50, nil)
	suite.Require().NoError(err)
	suite.Require().Len(venues, 1)
	suite.Require().NotNil(venues[0].Alerts)
	suite.Empty(venues[0].Alerts.Shows.Scope)
	suite.Nil(venues[0].Alerts.Releases)

	labels, _, err := suite.followService.GetLibraryFollowing(user.ID, "label", 50, nil)
	suite.Require().NoError(err)
	suite.Require().Len(labels, 1)
	suite.Nil(labels[0].Alerts, "alert-less types carry no subscription")
}

// setAccountAlertDefaults writes the user's account alert matrix directly.
// Written as raw JSON rather than through the user service because that
// service imports this package; the point under test is that this package
// READS the column, so the fixture stays at the storage layer.
func (suite *FollowServiceIntegrationTestSuite) setAccountAlertDefaults(userID uint, document string) {
	prefs := authm.UserPreferences{UserID: userID}
	suite.Require().NoError(suite.db.Create(&prefs).Error)
	suite.Require().NoError(suite.db.Model(&authm.UserPreferences{}).
		Where("user_id = ?", userID).
		Update("alert_defaults", gorm.Expr("?::jsonb", document)).Error)
}

// PSY-1907: the account matrix is the layer a follow with no overrides
// inherits, so changing it in settings reaches follows that already exist,
// including the ones made before the setting was ever touched.
func (suite *FollowServiceIntegrationTestSuite) TestFollowAlerts_InheritAccountDefaults() {
	user := suite.createTestUser()
	artistID := suite.createTestArtist("account-defaults-artist")
	suite.Require().NoError(suite.followService.Follow(user.ID, "artist", artistID))

	// The follow exists first, at the shipped defaults.
	before, err := suite.followService.GetFollowAlertSettings(user.ID, "artist", artistID)
	suite.Require().NoError(err)
	suite.False(before.Shows.Email)

	suite.setAccountAlertDefaults(user.ID, `{"shows":{"email":true},"releases":{"in_app":false}}`)

	after, err := suite.followService.GetFollowAlertSettings(user.ID, "artist", artistID)
	suite.Require().NoError(err)
	suite.True(after.Shows.Email, "the account email opt-in reaches an existing follow")
	suite.True(after.Shows.InApp, "a channel the account left alone still inherits shipped ON")
	suite.Require().NotNil(after.Releases)
	suite.False(after.Releases.InApp, "per alert type, not one value for both")

	suite.Empty(suite.rawSettings(user.ID, "artist", artistID),
		"inheritance is resolved at read time; nothing is stamped onto the follow row")

	rows, _, err := suite.followService.GetLibraryFollowing(user.ID, "artist", 50, nil)
	suite.Require().NoError(err)
	suite.Require().Len(rows, 1)
	suite.Require().NotNil(rows[0].Alerts)
	suite.True(rows[0].Alerts.Shows.Email, "the Library row resolves against the same matrix")
}

// A per-follow override is the narrower layer and must win over the account
// default, including when it opts OUT of an account-wide opt-in.
func (suite *FollowServiceIntegrationTestSuite) TestFollowAlerts_OverrideBeatsAccountDefault() {
	user := suite.createTestUser()
	artistID := suite.createTestArtist("account-override-artist")
	suite.Require().NoError(suite.followService.Follow(user.ID, "artist", artistID))
	suite.setAccountAlertDefaults(user.ID, `{"shows":{"email":true}}`)

	resolved, err := suite.followService.SetFollowAlertSettings(user.ID, "artist", artistID,
		contracts.FollowAlertUpdate{Shows: &contracts.FollowAlertPreferenceUpdate{Email: boolPtr(false)}})
	suite.Require().NoError(err)
	suite.False(resolved.Shows.Email, "the write response already reflects the override")

	read, err := suite.followService.GetFollowAlertSettings(user.ID, "artist", artistID)
	suite.Require().NoError(err)
	suite.False(read.Shows.Email, "this follow opted out of an account-wide opt-in")
	suite.True(read.Releases.InApp, "the untouched sibling still inherits")
}
