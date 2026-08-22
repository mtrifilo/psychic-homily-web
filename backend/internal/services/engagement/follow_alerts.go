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
	if entityType == string(engagementm.BookmarkEntityArtist) && alertType == contracts.FollowAlertTypeShows {
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

// followAlertsSupportsScope reports whether the entity type's show alerts have
// a scope axis.
func followAlertsSupportsScope(entityType string) bool {
	return entityType == string(engagementm.BookmarkEntityArtist)
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
	if stored.Scope != nil && pref.Scope != "" {
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
		// would dirty the row (and its inheritance) for no reason.
		if update.Shows == nil && update.Releases == nil {
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

// validateFollowAlertUpdate rejects updates that the entity type cannot carry,
// so an unsupported axis fails loudly instead of being written and ignored.
func validateFollowAlertUpdate(entityType string, update contracts.FollowAlertUpdate) error {
	if update.Releases != nil && !followAlertsSupportsReleases(entityType) {
		return apperrors.ErrFollowInvalidEntityType(
			fmt.Sprintf("%s follows have no release alerts", entityType))
	}
	if update.Shows != nil && update.Shows.Scope != nil {
		if !followAlertsSupportsScope(entityType) {
			return apperrors.ErrFollowInvalidEntityType(
				fmt.Sprintf("%s show alerts have no scope axis", entityType))
		}
		switch *update.Shows.Scope {
		case contracts.FollowAlertScopeNearMe, contracts.FollowAlertScopeEverywhere:
		default:
			return apperrors.ErrFollowInvalidEntityType(
				fmt.Sprintf("invalid alert scope: %s", *update.Shows.Scope))
		}
	}
	if update.Releases != nil && update.Releases.Scope != nil {
		return apperrors.ErrFollowInvalidEntityType("release alerts have no scope axis")
	}
	return nil
}

// mergeFollowAlertSettings folds a partial update into a settings document and
// returns the new document. Keys the update does not mention are preserved
// verbatim, at both the document level (scene_notify_mode and anything added
// later) and inside the alerts object.
func mergeFollowAlertSettings(settings *json.RawMessage, update contracts.FollowAlertUpdate) ([]byte, error) {
	doc := map[string]json.RawMessage{}
	if settings != nil && len(*settings) > 0 {
		if err := json.Unmarshal(*settings, &doc); err != nil {
			// A settings document we cannot parse is not something to overwrite:
			// it would silently drop whatever else the row carried.
			return nil, fmt.Errorf("failed to parse follow settings: %w", err)
		}
	}

	alerts := decodeStoredFollowAlerts(settings)
	alerts.Shows = applyFollowAlertPreferenceUpdate(alerts.Shows, update.Shows)
	alerts.Releases = applyFollowAlertPreferenceUpdate(alerts.Releases, update.Releases)

	encodedAlerts, err := json.Marshal(alerts)
	if err != nil {
		return nil, fmt.Errorf("failed to encode follow alert settings: %w", err)
	}
	doc[followAlertsKey] = encodedAlerts

	encoded, err := json.Marshal(doc)
	if err != nil {
		return nil, fmt.Errorf("failed to encode follow settings: %w", err)
	}
	return encoded, nil
}

// applyFollowAlertPreferenceUpdate overlays the set fields of an update onto a
// stored preference, leaving unset fields (and therefore their inheritance of
// the account default) alone.
func applyFollowAlertPreferenceUpdate(
	stored *storedFollowAlertPreference,
	update *contracts.FollowAlertPreferenceUpdate,
) *storedFollowAlertPreference {
	if update == nil {
		return stored
	}
	if stored == nil {
		stored = &storedFollowAlertPreference{}
	}
	if update.Enabled != nil {
		enabled := *update.Enabled
		stored.Enabled = &enabled
	}
	if update.InApp != nil {
		inApp := *update.InApp
		stored.InApp = &inApp
	}
	if update.Email != nil {
		email := *update.Email
		stored.Email = &email
	}
	if update.Scope != nil {
		scope := *update.Scope
		stored.Scope = &scope
	}
	return stored
}
