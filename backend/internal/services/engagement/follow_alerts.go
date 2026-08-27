package engagement

import (
	"encoding/json"
	"errors"
	"fmt"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	apperrors "psychic-homily-backend/internal/errors"
	authm "psychic-homily-backend/internal/models/auth"
	engagementm "psychic-homily-backend/internal/models/engagement"
	"psychic-homily-backend/internal/services/contracts"
)

// Follow-driven alert subscriptions (PSY-1893).
//
// Following an artist or a venue IS subscribing to that entity's alerts, and
// unfollowing unsubscribes (PSY-1892 owner decision 1, 2026-08-22). The
// subscription needs no row of its own: the follow row in user_bookmarks is
// the subscription, and per-follow overrides ride along in its settings JSONB
// beside the scene_notify_mode precedent (PSY-1341):
//
//	{
//	  "alerts": {
//	    "shows":    {"enabled": true, "scope": "near_me", "in_app": true, "email": false},
//	    "releases": {"enabled": true, "in_app": true, "email": false}
//	  }
//	}
//
// The three axes are kept orthogonal on purpose. "Off" is its own switch
// rather than "both channels false" because the Library control offers one
// three-way choice (near me / everywhere / off): folding off into the channels
// would make toggling off and back on forget the scope and the channels the
// user had picked, with nowhere to remember them.
//
// Every key is optional and ABSENT MEANS INHERIT. Follow deliberately writes
// nothing: a follow with no settings is already a full subscription at the
// account defaults, so a later change to those defaults reaches every follow
// the user never explicitly configured (and re-following can never reset an
// override the user did make).
//
// notification_filters are NOT touched by this lifecycle. Criteria filters
// remain the separate power-user path: user-source rows are settings-authored,
// and managed rows belong to the standalone Notify-me control, which keeps its
// own create/delete lifecycle until the UI merge lands. Deriving or deleting
// filter rows from follows would make an unfollow destroy a subscription the
// user created by another route, and a managed row cannot express the near-me
// scope anyway (its city matching is city-equality, which PSY-1892 decision 7
// rejects in favour of metro).

// followAlertsKey is the settings JSONB key holding the alerts object.
const followAlertsKey = "alerts"

// followAlertEntityTypes lists the follow targets that carry an alert
// subscription today. Tag follows are display-only (PSY-1903 owns that gap);
// labels, festivals and radio shows have no alert trigger.
//
// Scenes joined the list in PSY-1926. Their new-show fanout was the one stream
// that emailed on the strength of nothing but "is email configured at all",
// which is the exact posture PSY-1892 decision 4 forbids. Routing it through
// this chain is what makes a scene follow's email an intentional opt-in like
// every sibling, with no per-stream copy of the layering to drift.
//
// A scene follow keeps scene_notify_mode alongside the alerts object, and the
// two answer different questions: the MODE decides which of the scene's shows
// qualify (every show, only your followed bands, or none), and the alerts
// object decides whether and how a qualifying show is delivered. The scene UI
// writes the mode; the account alert matrix and the sweep behind the
// unsubscribe link write the channels.
var followAlertEntityTypes = map[string]bool{
	string(engagementm.BookmarkEntityArtist): true,
	string(engagementm.BookmarkEntityVenue):  true,
	string(engagementm.BookmarkEntityScene):  true,
}

// storedFollowAlertPreference is one alert type's stored overrides. Pointers,
// not values: nil is "inherit the default", which a bool cannot express.
//
// These structs are the READ shape only. Writes go through raw key maps
// (mergeFollowAlertSettings) because re-marshalling a struct would delete
// every key the struct does not model, and settings is an additive shared
// document. The JSON tags here and the key literals there must stay in step;
// TestMergeFollowAlertSettings_EveryAxisRoundTrips is what notices if they
// drift.
type storedFollowAlertPreference struct {
	Enabled *bool   `json:"enabled,omitempty"`
	InApp   *bool   `json:"in_app,omitempty"`
	Email   *bool   `json:"email,omitempty"`
	Scope   *string `json:"scope,omitempty"`
}

// storedFollowAlerts is the "alerts" object inside user_bookmarks.settings.
type storedFollowAlerts struct {
	Shows    *storedFollowAlertPreference `json:"shows,omitempty"`
	Releases *storedFollowAlertPreference `json:"releases,omitempty"`
}

// defaultFollowAlertPreference returns the default for one alert type, which is
// what a follow with no stored override resolves to.
//
// The channels come from the user's ACCOUNT alert matrix (PSY-1907), which is
// itself the shipped defaults under whatever the user changed in settings: the
// shipped values are in-app ON and email OFF, and email stays an intentional
// opt-in on every alert type (PSY-1892 decision 4). Because a follow stores
// nothing it never overrode, editing the account matrix reaches every such
// follow with no data migration.
//
// Enabled and Scope have no account-level axis: the account matrix is per alert
// type x CHANNEL, and a master switch or an area choice made once for every
// entity the user follows is not a setting PSY-1892 decided on. A new artist
// follow defaults to near-me scope (decision 2).
func defaultFollowAlertPreference(entityType, alertType string, account authm.AccountAlertDefaults) contracts.FollowAlertPreference {
	channels := accountAlertChannelsFor(account, alertType)
	pref := contracts.FollowAlertPreference{
		Enabled: true,
		InApp:   channels.InApp,
		Email:   channels.Email,
	}
	if !followAlertHasInAppAxis(entityType) {
		pref.InApp = true
	}
	if followAlertHasScopeAxis(entityType, alertType) {
		pref.Scope = contracts.FollowAlertScopeNearMe
	}
	return pref
}

// accountAlertChannelsFor maps a follow alert type onto the matching field of
// the account matrix. This switch is the ONE place the contracts alert-type
// constants meet the account matrix's fields, which
// TestAccountAlertChannelsFor_KeysMatchFollowAlertTypes pins to the storage
// keys in models/auth.
func accountAlertChannelsFor(account authm.AccountAlertDefaults, alertType string) authm.AlertChannelDefaults {
	switch alertType {
	case contracts.FollowAlertTypeReleases:
		return account.Releases
	case contracts.FollowAlertTypeShows:
		return account.Shows
	default:
		// Unreachable: every caller passes one of the two constants above. A
		// third alert type must add its case HERE as well as its field in
		// models/auth, or it silently inherits the show row's channels.
		return account.Shows
	}
}

// accountAlertDefaults loads a user's resolved account alert matrix.
//
// A user with no preferences row is not an error: they have overridden nothing,
// which is what a NULL document resolves to. Any other read failure propagates,
// because silently substituting the shipped defaults would report a
// subscription the user may not have, and on the email axis it would misreport
// an opt-in in either direction.
//
// Takes the handle rather than reading s.db so a caller inside a transaction
// sees the same snapshot as the follow row it just locked.
func accountAlertDefaults(db *gorm.DB, userID uint) (authm.AccountAlertDefaults, error) {
	var prefs authm.UserPreferences
	err := db.Select("alert_defaults").Where("user_id = ?", userID).Take(&prefs).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return authm.ResolveAccountAlertDefaults(nil), nil
	}
	if err != nil {
		return authm.AccountAlertDefaults{}, fmt.Errorf("failed to read account alert defaults: %w", err)
	}
	return authm.ResolveAccountAlertDefaults(prefs.AlertDefaults), nil
}

// EffectiveShowScope applies the no-home-area fallback to a recorded artist
// show-alert scope. "Near me" cannot be honoured for a user with no home area:
// scoping to nothing would silently deliver nothing, which is the exact
// failure the merged follow control exists to remove, so it degrades to
// everywhere (PSY-1892 decision 2).
//
// The fallback is applied at delivery time and NOT baked into the stored or
// read value, so a user who sets a home area later gets the near-me scope they
// chose without touching a single follow row. Delivery (PSY-1896) supplies
// hasHomeArea from the user's home metro.
func EffectiveShowScope(scope string, hasHomeArea bool) string {
	if scope == contracts.FollowAlertScopeNearMe && !hasHomeArea {
		return contracts.FollowAlertScopeEverywhere
	}
	return scope
}

// ResolveFollowAlerts is the delivery-side seam onto the three-layer resolution
// this file already implements: shipped defaults, then the account matrix, then
// the follow's own stored overrides (PSY-1893/1907).
//
// It exists so the alert matcher (PSY-1896) can resolve a page of followers
// WITHOUT going through GetFollowAlertSettings, which loads the follow row and
// the account matrix one user at a time. A show with two hundred followers would
// be four hundred queries; the matcher instead reads both in bulk and calls this
// per row. The alternative — re-deriving the layering at the delivery site — is
// exactly the drift this seam exists to prevent, and it would drift on the axis
// where a mistake is worst: email is an intentional opt-in, so a resolver that
// disagreed by one layer would mail people who never asked.
//
// account is passed in rather than read here for the same reason: one read of
// the matrix per user, not one per follow.
func ResolveFollowAlerts(
	entityType string,
	entityID uint,
	settings *json.RawMessage,
	account authm.AccountAlertDefaults,
) *contracts.FollowAlertSettings {
	return resolveFollowAlerts(entityType, entityID, settings, account)
}

// DisableFollowAlertEmailChannel turns the email channel OFF on every one of a
// user's follows of entityType for alertType, and is what makes a one-click
// unsubscribe from those emails actually stick (PSY-1896).
//
// The account matrix alone cannot deliver that promise. Per-follow overrides sit
// BELOW the account defaults in the inherit chain, so a user who once switched
// email on for a particular band keeps receiving mail for that band no matter
// what the account row says. RFC 8058 requires the link to stop the stream, so
// the unsubscribe has to reach the overrides too. The caller pairs this with the
// account-level write; neither half is sufficient alone.
//
// Follows that never overrode the channel are left untouched: they already
// inherit, so writing an explicit false would only pin them against a future
// change to the account default. That keeps this a repair of explicit opt-ins
// rather than a mass write across the user's whole library.
//
// The merge runs through mergeFollowAlertSettings so sibling keys in the shared
// settings document survive, and so this path cannot drift from the one the API
// uses. Row-by-row rather than one jsonb_set statement because the document is
// shared and may be malformed; the merge already knows how to repair that, and a
// SQL path would either error on a non-object or clobber the whole column.
func (s *FollowService) DisableFollowAlertEmailChannel(userID uint, entityType, alertType string) error {
	if s.db == nil {
		return apperrors.ErrFollowInternal(fmt.Errorf("database not initialized"))
	}
	if err := validateFollowAlertEntityType(entityType); err != nil {
		return err
	}

	off := false
	update := contracts.FollowAlertUpdate{}
	switch alertType {
	case contracts.FollowAlertTypeShows:
		update.Shows = &contracts.FollowAlertPreferenceUpdate{Email: &off}
	case contracts.FollowAlertTypeReleases:
		update.Releases = &contracts.FollowAlertPreferenceUpdate{Email: &off}
	default:
		return apperrors.ErrFollowInvalidAlertSettings(
			fmt.Sprintf("unknown alert type: %s", alertType))
	}

	return s.db.Transaction(func(tx *gorm.DB) error {
		var bookmarks []engagementm.UserBookmark
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("user_id = ? AND entity_type = ? AND action = ?",
				userID, engagementm.BookmarkEntityType(entityType), engagementm.BookmarkActionFollow).
			Find(&bookmarks).Error; err != nil {
			return apperrors.ErrFollowInternal(fmt.Errorf("failed to load follows: %w", err))
		}

		for i := range bookmarks {
			stored := decodeStoredFollowAlerts(bookmarks[i].Settings)
			if !storedFollowAlertEmailIsOn(stored, alertType) {
				continue
			}
			merged, err := mergeFollowAlertSettings(bookmarks[i].Settings, update)
			if err != nil {
				return apperrors.ErrFollowInternal(err)
			}
			if err := tx.Model(&engagementm.UserBookmark{}).
				Where("id = ?", bookmarks[i].ID).
				Update("settings", gorm.Expr("?::jsonb", string(merged))).Error; err != nil {
				return apperrors.ErrFollowInternal(
					fmt.Errorf("failed to clear follow email override: %w", err))
			}
		}
		return nil
	})
}

// storedFollowAlertEmailIsOn reports whether a follow carries an EXPLICIT
// email:true override for the alert type. Absent is not "on" here even when the
// account default says on, because the account write is the other half of the
// unsubscribe and reaches every inheriting follow by itself.
func storedFollowAlertEmailIsOn(stored storedFollowAlerts, alertType string) bool {
	var pref *storedFollowAlertPreference
	switch alertType {
	case contracts.FollowAlertTypeShows:
		pref = stored.Shows
	case contracts.FollowAlertTypeReleases:
		pref = stored.Releases
	}
	return pref != nil && pref.Email != nil && *pref.Email
}

// validateFollowAlertEntityType rejects follow targets with no alert
// subscription before any database access.
func validateFollowAlertEntityType(entityType string) error {
	if !followAlertEntityTypes[entityType] {
		return apperrors.ErrFollowInvalidEntityType(entityType)
	}
	return nil
}

// followAlertsSupportsReleases reports whether the entity type emits releases.
func followAlertsSupportsReleases(entityType string) bool {
	return entityType == string(engagementm.BookmarkEntityArtist)
}

// followAlertHasInAppAxis is the ONE predicate for "can this follow type's
// in-app channel be switched off". Read it, never infer the axis from a
// resolved value, for the same reason the scope axis has its own predicate.
//
// Scene follows cannot. Their fanout writes a SINGLE notification_log row per
// (user, show) which is at once the bell entry AND the cross-system dedup
// marker that notifiedAboutShow reads, so suppressing it to honour an in-app
// switch would also erase the record that the user had been told, and the next
// pass over that show would notify them again. Reporting the channel as
// always-on is the honest reading of what the delivery path can do; offering a
// switch the notifier ignores would be a control that silently does nothing.
//
// Turning scene notifications off entirely is scene_notify_mode's job, and it
// works: an "off" follow is skipped before any row is written.
func followAlertHasInAppAxis(entityType string) bool {
	return entityType != string(engagementm.BookmarkEntityScene)
}

// followAlertHasScopeAxis is the ONE predicate for "does this alert type on
// this entity type have an area scope". Read it, never infer the axis from
// whether a default scope happens to be non-empty: a default that legitimately
// resolves to no scope would then silently drop every stored override.
//
// Only artist show alerts have one. A venue sits in one place, and a record has
// no location.
func followAlertHasScopeAxis(entityType, alertType string) bool {
	return entityType == string(engagementm.BookmarkEntityArtist) &&
		alertType == contracts.FollowAlertTypeShows
}

// decodeStoredFollowAlerts extracts the alerts object from a settings
// document. A missing key, a NULL column and an unparseable value all mean "no
// overrides" — settings is a shared, forward-compatible document, so bad data
// must not fail the read of a follow's subscription.
//
// Each alert type is decoded on its own, so damage stays proportional: a value
// the typed shape cannot hold silently reverts THAT alert type to the account
// defaults rather than taking its sibling down with it. A whole-object decode
// would turn one bad key into "every override on this follow disappeared",
// which for the email axis means silently re-subscribing nobody but also
// silently discarding an opt-in the user made.
func decodeStoredFollowAlerts(settings *json.RawMessage) storedFollowAlerts {
	var alerts storedFollowAlerts
	doc, err := decodeJSONObject(settings)
	if err != nil {
		return alerts
	}
	byType, err := decodeJSONObject(rawMessageOrNil(doc[followAlertsKey]))
	if err != nil {
		return alerts
	}
	alerts.Shows = decodeStoredFollowAlertPreference(byType[contracts.FollowAlertTypeShows])
	alerts.Releases = decodeStoredFollowAlertPreference(byType[contracts.FollowAlertTypeReleases])
	return alerts
}

// decodeStoredFollowAlertPreference decodes one alert type's stored overrides,
// or nil when it is absent or unusable.
func decodeStoredFollowAlertPreference(raw json.RawMessage) *storedFollowAlertPreference {
	if len(raw) == 0 {
		return nil
	}
	var pref storedFollowAlertPreference
	if err := json.Unmarshal(raw, &pref); err != nil {
		return nil
	}
	return &pref
}

// resolveFollowAlertPreference layers a follow's stored overrides over the
// account default for one alert type.
func resolveFollowAlertPreference(
	entityType, alertType string,
	stored *storedFollowAlertPreference,
	account authm.AccountAlertDefaults,
) contracts.FollowAlertPreference {
	pref := defaultFollowAlertPreference(entityType, alertType, account)
	if stored == nil {
		return pref
	}
	if stored.Enabled != nil {
		pref.Enabled = *stored.Enabled
	}
	// A stored in-app value on a follow type with no in-app axis is ignored
	// rather than surfaced, the same rule the scope axis follows below: the
	// write path rejects it, so any such value is stale data, and honouring it
	// would report a channel the delivery path cannot switch off.
	if stored.InApp != nil && followAlertHasInAppAxis(entityType) {
		pref.InApp = *stored.InApp
	}
	if stored.Email != nil {
		pref.Email = *stored.Email
	}
	// A stored scope on an alert type with no scope axis is ignored rather than
	// surfaced: the write path rejects it, so any such value is stale data.
	if stored.Scope != nil && followAlertHasScopeAxis(entityType, alertType) {
		pref.Scope = *stored.Scope
	}
	return pref
}

// resolveFollowAlerts builds the resolved subscription for one follow row by
// layering its stored overrides over the user's account defaults. The account
// matrix is passed in rather than loaded here so a page of follows costs one
// read of it, not one per row.
func resolveFollowAlerts(
	entityType string,
	entityID uint,
	settings *json.RawMessage,
	account authm.AccountAlertDefaults,
) *contracts.FollowAlertSettings {
	stored := decodeStoredFollowAlerts(settings)
	resolved := &contracts.FollowAlertSettings{
		EntityType: entityType,
		EntityID:   entityID,
		Shows:      resolveFollowAlertPreference(entityType, contracts.FollowAlertTypeShows, stored.Shows, account),
	}
	if followAlertsSupportsReleases(entityType) {
		releases := resolveFollowAlertPreference(entityType, contracts.FollowAlertTypeReleases, stored.Releases, account)
		resolved.Releases = &releases
	}
	return resolved
}

// GetFollowAlertSettings returns the resolved alert subscription a user's
// follow carries. Reports CodeFollowNotFound when the user does not follow the
// entity — there is no subscription without a follow.
func (s *FollowService) GetFollowAlertSettings(userID uint, entityType string, entityID uint) (*contracts.FollowAlertSettings, error) {
	if s.db == nil {
		return nil, apperrors.ErrFollowInternal(fmt.Errorf("database not initialized"))
	}
	if err := validateFollowAlertEntityType(entityType); err != nil {
		return nil, err
	}

	var bookmark engagementm.UserBookmark
	err := s.db.Where("user_id = ? AND entity_type = ? AND entity_id = ? AND action = ?",
		userID, engagementm.BookmarkEntityType(entityType), entityID, engagementm.BookmarkActionFollow).
		Take(&bookmark).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, apperrors.ErrFollowNotFound(entityType, entityID)
	}
	if err != nil {
		return nil, apperrors.ErrFollowInternal(fmt.Errorf("failed to read follow alert settings: %w", err))
	}

	account, err := accountAlertDefaults(s.db, userID)
	if err != nil {
		return nil, apperrors.ErrFollowInternal(err)
	}
	return resolveFollowAlerts(entityType, entityID, bookmark.Settings, account), nil
}

// SetFollowAlertSettings applies a partial update to a follow's alert
// subscription and returns the resolved result. The follow must already exist
// (call Follow first); configuring a follow that isn't there reports
// CodeFollowNotFound so a client cannot silently configure nothing.
func (s *FollowService) SetFollowAlertSettings(
	userID uint,
	entityType string,
	entityID uint,
	update contracts.FollowAlertUpdate,
) (*contracts.FollowAlertSettings, error) {
	if s.db == nil {
		return nil, apperrors.ErrFollowInternal(fmt.Errorf("database not initialized"))
	}
	if err := validateFollowAlertEntityType(entityType); err != nil {
		return nil, err
	}
	if err := validateFollowAlertUpdate(entityType, update); err != nil {
		return nil, err
	}

	var resolved *contracts.FollowAlertSettings
	err := s.db.Transaction(func(tx *gorm.DB) error {
		// Row lock, then read-modify-write in Go. settings is a shared document
		// (scene_notify_mode today, more later) and this update touches a nested
		// subtree of it, so a lock plus a whole-document rewrite is both simpler
		// to read and safer against a concurrent sibling write than a stack of
		// nested jsonb_build_object merges.
		var bookmark engagementm.UserBookmark
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("user_id = ? AND entity_type = ? AND entity_id = ? AND action = ?",
				userID, engagementm.BookmarkEntityType(entityType), entityID, engagementm.BookmarkActionFollow).
			Take(&bookmark).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return apperrors.ErrFollowNotFound(entityType, entityID)
		}
		if err != nil {
			return apperrors.ErrFollowInternal(fmt.Errorf("failed to load follow: %w", err))
		}

		account, err := accountAlertDefaults(tx, userID)
		if err != nil {
			return apperrors.ErrFollowInternal(err)
		}

		// An update that sets nothing is a read: writing an empty alerts object
		// would dirty the row (and its inheritance) for no reason. This covers
		// both an absent alert type and a present one with every axis unset.
		if followAlertUpdateSetsNothing(update) {
			resolved = resolveFollowAlerts(entityType, entityID, bookmark.Settings, account)
			return nil
		}

		merged, err := mergeFollowAlertSettings(bookmark.Settings, update)
		if err != nil {
			return apperrors.ErrFollowInternal(err)
		}

		// Cast the parameter explicitly: the column is jsonb and the driver
		// would otherwise have to infer the type of a bare string literal.
		if err := tx.Model(&engagementm.UserBookmark{}).
			Where("id = ?", bookmark.ID).
			Update("settings", gorm.Expr("?::jsonb", string(merged))).Error; err != nil {
			return apperrors.ErrFollowInternal(fmt.Errorf("failed to save follow alert settings: %w", err))
		}

		raw := json.RawMessage(merged)
		resolved = resolveFollowAlerts(entityType, entityID, &raw, account)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return resolved, nil
}

// followAlertUpdateSetsNothing reports whether an update would change no axis
// on any alert type, i.e. whether applying it is a read.
func followAlertUpdateSetsNothing(update contracts.FollowAlertUpdate) bool {
	return followAlertPreferenceUpdateSetsNothing(update.Shows) &&
		followAlertPreferenceUpdateSetsNothing(update.Releases)
}

// followAlertPreferenceUpdateSetsNothing reports whether one alert type's
// update is absent or present with every axis unset. Both mean "no override",
// so neither may be written: a stored empty object would say exactly what
// inheriting already says, while pinning nothing.
func followAlertPreferenceUpdateSetsNothing(update *contracts.FollowAlertPreferenceUpdate) bool {
	return update == nil ||
		(update.Enabled == nil && update.InApp == nil && update.Email == nil && update.Scope == nil)
}

// validateFollowAlertUpdate rejects updates that the entity type cannot carry,
// so an unsupported axis fails loudly instead of being written and ignored.
func validateFollowAlertUpdate(entityType string, update contracts.FollowAlertUpdate) error {
	if update.Releases != nil && !followAlertsSupportsReleases(entityType) {
		return apperrors.ErrFollowInvalidAlertSettings(
			fmt.Sprintf("%s follows have no release alerts", entityType))
	}
	for alertType, pref := range map[string]*contracts.FollowAlertPreferenceUpdate{
		contracts.FollowAlertTypeShows:    update.Shows,
		contracts.FollowAlertTypeReleases: update.Releases,
	} {
		if pref == nil {
			continue
		}
		// Rejected rather than stored-and-ignored: a write that appears to
		// succeed and changes nothing is the worst shape a control can have,
		// and on a delivery channel it is the shape that makes a user believe
		// they silenced something they did not.
		if pref.InApp != nil && !followAlertHasInAppAxis(entityType) {
			return apperrors.ErrFollowInvalidAlertSettings(
				fmt.Sprintf("%s alerts are always delivered in-app", entityType))
		}
		if pref.Scope == nil {
			continue
		}
		if !followAlertHasScopeAxis(entityType, alertType) {
			return apperrors.ErrFollowInvalidAlertSettings(
				fmt.Sprintf("%s %s alerts have no scope axis", entityType, alertType))
		}
		switch *pref.Scope {
		case contracts.FollowAlertScopeNearMe, contracts.FollowAlertScopeEverywhere:
		default:
			return apperrors.ErrFollowInvalidAlertSettings(
				fmt.Sprintf("invalid alert scope: %s", *pref.Scope))
		}
	}
	return nil
}

// mergeFollowAlertSettings folds a partial update into a settings document and
// returns the new document.
//
// The merge runs on raw key maps at all three levels (document, alerts object,
// per-alert-type preference) rather than on the typed structs, so a key this
// code does not model survives a write. That matters because settings is
// explicitly an additive, shared document: decoding to a struct and
// re-marshalling would silently delete the first sibling key anyone adds.
func mergeFollowAlertSettings(settings *json.RawMessage, update contracts.FollowAlertUpdate) ([]byte, error) {
	doc, err := decodeJSONObject(settings)
	if err != nil {
		// A settings document we cannot parse is not something to overwrite:
		// it would silently drop whatever else the row carried.
		return nil, fmt.Errorf("failed to parse follow settings: %w", err)
	}

	// A malformed alerts value (not an object) is the one thing this write does
	// replace: it is unusable to every reader, and the write path is the only
	// chance to repair it.
	alerts, err := decodeJSONObject(rawMessageOrNil(doc[followAlertsKey]))
	if err != nil {
		alerts = map[string]json.RawMessage{}
	}

	for alertType, pref := range map[string]*contracts.FollowAlertPreferenceUpdate{
		contracts.FollowAlertTypeShows:    update.Shows,
		contracts.FollowAlertTypeReleases: update.Releases,
	} {
		merged, err := mergeFollowAlertPreference(rawMessageOrNil(alerts[alertType]), pref)
		if err != nil {
			return nil, err
		}
		if merged != nil {
			alerts[alertType] = merged
		}
	}

	if len(alerts) > 0 {
		encodedAlerts, err := json.Marshal(alerts)
		if err != nil {
			return nil, fmt.Errorf("failed to encode follow alert settings: %w", err)
		}
		doc[followAlertsKey] = encodedAlerts
	}

	encoded, err := json.Marshal(doc)
	if err != nil {
		return nil, fmt.Errorf("failed to encode follow settings: %w", err)
	}
	return encoded, nil
}

// mergeFollowAlertPreference overlays the set fields of an update onto one
// stored preference object, preserving keys it does not model. Returns nil when
// there is nothing to write, so an update that sets no field leaves the row
// alone instead of storing an empty object that means what inheriting means.
func mergeFollowAlertPreference(
	stored *json.RawMessage,
	update *contracts.FollowAlertPreferenceUpdate,
) (json.RawMessage, error) {
	if followAlertPreferenceUpdateSetsNothing(update) {
		return nil, nil
	}

	pref, err := decodeJSONObject(stored)
	if err != nil {
		// Same repair rule as the alerts object one level up.
		pref = map[string]json.RawMessage{}
	}

	set := func(key string, value any) error {
		encoded, err := json.Marshal(value)
		if err != nil {
			return fmt.Errorf("failed to encode alert preference %q: %w", key, err)
		}
		pref[key] = encoded
		return nil
	}
	if update.Enabled != nil {
		if err := set("enabled", *update.Enabled); err != nil {
			return nil, err
		}
	}
	if update.InApp != nil {
		if err := set("in_app", *update.InApp); err != nil {
			return nil, err
		}
	}
	if update.Email != nil {
		if err := set("email", *update.Email); err != nil {
			return nil, err
		}
	}
	if update.Scope != nil {
		if err := set("scope", *update.Scope); err != nil {
			return nil, err
		}
	}

	encoded, err := json.Marshal(pref)
	if err != nil {
		return nil, fmt.Errorf("failed to encode alert preference: %w", err)
	}
	return encoded, nil
}

// decodeJSONObject decodes a raw value into a key map. A nil or empty value is
// an empty object, not an error.
func decodeJSONObject(raw *json.RawMessage) (map[string]json.RawMessage, error) {
	out := map[string]json.RawMessage{}
	if raw == nil || len(*raw) == 0 {
		return out, nil
	}
	if err := json.Unmarshal(*raw, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// rawMessageOrNil adapts a map lookup to the pointer decodeJSONObject takes;
// an absent key reads as nil rather than an empty non-nil value.
func rawMessageOrNil(raw json.RawMessage) *json.RawMessage {
	if len(raw) == 0 {
		return nil
	}
	return &raw
}
