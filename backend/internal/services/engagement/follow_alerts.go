package engagement

import (
	"encoding/json"
	"errors"
	"fmt"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	apperrors "psychic-homily-backend/internal/errors"
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
// subscription today. Scenes keep their own scene_notify_mode (PSY-1341); tag
// follows are display-only (PSY-1903 owns that gap); labels, festivals and
// radio shows have no alert trigger.
var followAlertEntityTypes = map[string]bool{
	string(engagementm.BookmarkEntityArtist): true,
	string(engagementm.BookmarkEntityVenue):  true,
}

// storedFollowAlertPreference is one alert type's stored overrides. Pointers,
// not values: nil is "inherit the default", which a bool cannot express.
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

// defaultFollowAlertPreference returns the account-level default for one alert
// type, which is what a follow with no stored override resolves to.
//
// PSY-1892 owner decisions (2026-08-22): in-app alerts default ON; ALL email
// alerts default OFF and require an intentional opt-in (decision 4); a new
// artist follow defaults to near-me scope (decision 2).
//
// This is the single place those defaults live. When the account-level alert
// matrix ships it replaces this function's body, and every follow that never
// overrode a key picks the new value up with no data migration.
func defaultFollowAlertPreference(entityType, alertType string) contracts.FollowAlertPreference {
	pref := contracts.FollowAlertPreference{Enabled: true, InApp: true, Email: false}
	if followAlertHasScopeAxis(entityType, alertType) {
		pref.Scope = contracts.FollowAlertScopeNearMe
	}
	return pref
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
// document. A missing key, a NULL column and an unparseable alerts value all
// mean "no overrides" — settings is a shared, forward-compatible document, so
// one malformed key must not fail the read of a follow's subscription.
func decodeStoredFollowAlerts(settings *json.RawMessage) storedFollowAlerts {
	var alerts storedFollowAlerts
	if settings == nil || len(*settings) == 0 {
		return alerts
	}
	var doc map[string]json.RawMessage
	if err := json.Unmarshal(*settings, &doc); err != nil {
		return alerts
	}
	raw, ok := doc[followAlertsKey]
	if !ok {
		return alerts
	}
	_ = json.Unmarshal(raw, &alerts)
	return alerts
}

// resolveFollowAlertPreference layers a follow's stored overrides over the
// account default for one alert type.
func resolveFollowAlertPreference(
	entityType, alertType string,
	stored *storedFollowAlertPreference,
) contracts.FollowAlertPreference {
	pref := defaultFollowAlertPreference(entityType, alertType)
	if stored == nil {
		return pref
	}
	if stored.Enabled != nil {
		pref.Enabled = *stored.Enabled
	}
	if stored.InApp != nil {
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

// resolveFollowAlerts builds the resolved subscription for one follow row.
func resolveFollowAlerts(entityType string, entityID uint, settings *json.RawMessage) *contracts.FollowAlertSettings {
	stored := decodeStoredFollowAlerts(settings)
	resolved := &contracts.FollowAlertSettings{
		EntityType: entityType,
		EntityID:   entityID,
		Shows:      resolveFollowAlertPreference(entityType, contracts.FollowAlertTypeShows, stored.Shows),
	}
	if followAlertsSupportsReleases(entityType) {
		releases := resolveFollowAlertPreference(entityType, contracts.FollowAlertTypeReleases, stored.Releases)
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
	return resolveFollowAlerts(entityType, entityID, bookmark.Settings), nil
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

		// An update that sets nothing is a read: writing an empty alerts object
		// would dirty the row (and its inheritance) for no reason. This covers
		// both an absent alert type and a present one with every axis unset.
		if followAlertUpdateSetsNothing(update) {
			resolved = resolveFollowAlerts(entityType, entityID, bookmark.Settings)
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
		resolved = resolveFollowAlerts(entityType, entityID, &raw)
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
		if pref == nil || pref.Scope == nil {
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
