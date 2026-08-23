package auth

import (
	"encoding/json"
	"fmt"
)

// Account-level alert defaults (PSY-1907).
//
// These are the per-alert-type, per-channel defaults every follow inherits
// (PSY-1892 decisions 2 and 4, locked 2026-08-22). They live in
// user_preferences.alert_defaults as a nullable JSONB document:
//
//	{"shows":    {"in_app": true, "email": false},
//	 "releases": {"in_app": true, "email": false}}
//
// Three layers resolve one effective value, each narrower than the last:
//
//	shipped defaults  ->  account defaults  ->  per-follow overrides
//
// and at EVERY layer ABSENT MEANS INHERIT, the same philosophy PSY-1893 gave
// the per-follow layer. A NULL column, a missing alert type and a missing
// channel key are all "inherit", never "false". That is why this is a JSONB
// document and not a grid of boolean columns: a bool has no third state, so a
// stored false could not be told apart from an unset key, and the shipped
// defaults would be frozen into every row the day they were written.
//
// The same property is what keeps this clear of GORM's zero-value trap. GORM
// drops a false on Create for any field carrying a `default` tag, so a boolean
// alert matrix whose values are deliberately ASYMMETRIC (in-app ON, email OFF)
// would silently take the column default for every channel the user turned off.
// Nothing here is a bare bool at the storage layer: the column is a pointer to
// a JSON document, and inside it every channel is an optional key.
//
// This package (not services/contracts) is the home for the shape because
// contracts imports models/auth, and both the user service that writes these
// and the engagement service that seeds follows from them need it.

// Shipped alert-channel defaults, the innermost layer. In-app alerts default
// ON so a subscription is useful the moment it is made; email defaults OFF and
// stays an intentional opt-in on every alert type (decision 4).
const (
	shippedAlertInAppDefault = true
	shippedAlertEmailDefault = false
)

// Storage keys for the alert_defaults document. They are deliberately equal to
// contracts.FollowAlertTypeShows / FollowAlertTypeReleases so a stored document
// reads the same as a follow's settings document, and the two must stay in
// step. TestAccountAlertChannelsFor_KeysMatchFollowAlertTypes is what notices
// if they drift; it lives in services/engagement, not here, because this
// package cannot import contracts (contracts imports this one).
const (
	alertDefaultsKeyShows    = "shows"
	alertDefaultsKeyReleases = "releases"
)

// AlertChannelDefaults is the RESOLVED per-channel default for one alert type:
// each field is the effective value after the stored account overrides are
// applied over the shipped defaults.
type AlertChannelDefaults struct {
	InApp bool `json:"in_app"`
	Email bool `json:"email"`
}

// AccountAlertDefaults is the resolved account-level alert matrix.
type AccountAlertDefaults struct {
	Shows    AlertChannelDefaults `json:"shows"`
	Releases AlertChannelDefaults `json:"releases"`
}

// AlertPreferences is everything the account-level alerts surface reads: the
// user's home area and the resolved alert matrix. The matrix is served resolved
// rather than raw because "unset" is only meaningful against the shipped
// defaults, and duplicating those in the client is exactly the drift this
// three-layer design exists to avoid.
type AlertPreferences struct {
	// HomeMetro is a US Census CBSA code, or nil when the user has set no home
	// area. Same code space as venues.metro (PSY-1892 decision 9).
	HomeMetro *string `json:"home_metro"`
	// AlertDefaults is resolved, never raw: every field is an effective value.
	AlertDefaults AccountAlertDefaults `json:"alert_defaults"`
}

// AlertChannelDefaultsUpdate is a partial update to one alert type's channels.
// A nil field leaves that channel untouched, which is what keeps a channel the
// user never configured inheriting the shipped default rather than pinning
// today's value.
type AlertChannelDefaultsUpdate struct {
	InApp *bool
	Email *bool
}

// AccountAlertDefaultsUpdate is a partial update to the account alert matrix.
type AccountAlertDefaultsUpdate struct {
	Shows    *AlertChannelDefaultsUpdate
	Releases *AlertChannelDefaultsUpdate
}

// SetsNothing reports whether the update would change no channel on any alert
// type, i.e. whether applying it is a read.
func (u AccountAlertDefaultsUpdate) SetsNothing() bool {
	return u.Shows.setsNothing() && u.Releases.setsNothing()
}

// setsNothing reports whether one alert type's update is absent or present with
// every channel unset. Both mean "no override", so neither may be written: a
// stored empty object would say exactly what inheriting already says while
// pinning nothing.
func (u *AlertChannelDefaultsUpdate) setsNothing() bool {
	return u == nil || (u.InApp == nil && u.Email == nil)
}

// shippedAlertChannelDefaults returns the innermost layer for one alert type.
// One function so the shipped values have exactly one home.
func shippedAlertChannelDefaults() AlertChannelDefaults {
	return AlertChannelDefaults{
		InApp: shippedAlertInAppDefault,
		Email: shippedAlertEmailDefault,
	}
}

// ResolveAccountAlertDefaults layers a user's stored account overrides over the
// shipped defaults.
//
// A NULL column, an unparseable document and an unparseable alert type all mean
// "no overrides". Each alert type is decoded on its own so damage stays
// proportional: a value the typed shape cannot hold reverts THAT alert type to
// the shipped defaults instead of taking its sibling down with it. A
// whole-document decode would turn one bad key into "every alert default this
// user chose disappeared", which on the email axis silently discards an opt-in
// the user made.
func ResolveAccountAlertDefaults(raw *json.RawMessage) AccountAlertDefaults {
	byType, err := decodeAlertJSONObject(raw)
	if err != nil {
		byType = nil
	}
	return AccountAlertDefaults{
		Shows:    resolveAlertChannelDefaults(byType[alertDefaultsKeyShows]),
		Releases: resolveAlertChannelDefaults(byType[alertDefaultsKeyReleases]),
	}
}

// storedAlertChannelDefaults is one alert type's stored overrides. Pointers,
// not values: nil is "inherit the shipped default", which a bool cannot say.
//
// This struct is the READ shape only. Writes go through raw key maps
// (MergeAccountAlertDefaults) because re-marshalling a struct would delete every
// key the struct does not model. The JSON tags here and the key literals there
// must stay in step; TestMergeAccountAlertDefaults_EveryChannelRoundTrips is
// what notices if they drift.
type storedAlertChannelDefaults struct {
	InApp *bool `json:"in_app,omitempty"`
	Email *bool `json:"email,omitempty"`
}

// resolveAlertChannelDefaults overlays one alert type's stored overrides on the
// shipped defaults.
func resolveAlertChannelDefaults(raw json.RawMessage) AlertChannelDefaults {
	channels := shippedAlertChannelDefaults()
	if len(raw) == 0 {
		return channels
	}
	var stored storedAlertChannelDefaults
	if err := json.Unmarshal(raw, &stored); err != nil {
		return channels
	}
	if stored.InApp != nil {
		channels.InApp = *stored.InApp
	}
	if stored.Email != nil {
		channels.Email = *stored.Email
	}
	return channels
}

// MergeAccountAlertDefaults folds a partial update into a stored alert-defaults
// document and returns the new document, or nil when the update sets nothing
// (in which case the caller must leave the column alone rather than write an
// empty object that means what NULL already means).
//
// The merge runs on raw key maps at both levels rather than on the typed
// structs, so a key this code does not model survives a write. An alert type
// added by a newer server must not be erased by an older one mid-deploy, and a
// channel the update does not mention must keep inheriting instead of being
// pinned to today's resolved value.
func MergeAccountAlertDefaults(raw *json.RawMessage, update AccountAlertDefaultsUpdate) ([]byte, error) {
	if update.SetsNothing() {
		return nil, nil
	}

	byType, err := decodeAlertJSONObject(raw)
	if err != nil {
		// This column has exactly one writer, so an unparseable value is not a
		// sibling's data to protect. It is corruption, and the write path is
		// the only chance to repair it.
		byType = map[string]json.RawMessage{}
	}

	for key, channels := range map[string]*AlertChannelDefaultsUpdate{
		alertDefaultsKeyShows:    update.Shows,
		alertDefaultsKeyReleases: update.Releases,
	} {
		merged, err := mergeAlertChannelDefaults(byType[key], channels)
		if err != nil {
			return nil, err
		}
		if merged != nil {
			byType[key] = merged
		}
	}

	encoded, err := json.Marshal(byType)
	if err != nil {
		return nil, fmt.Errorf("failed to encode alert defaults: %w", err)
	}
	return encoded, nil
}

// mergeAlertChannelDefaults overlays the set channels of an update onto one
// stored alert type, preserving keys it does not model. Returns nil when there
// is nothing to write.
func mergeAlertChannelDefaults(
	stored json.RawMessage,
	update *AlertChannelDefaultsUpdate,
) (json.RawMessage, error) {
	if update.setsNothing() {
		return nil, nil
	}

	channels, err := decodeAlertJSONObject(rawAlertMessageOrNil(stored))
	if err != nil {
		// Same repair rule as the document one level up.
		channels = map[string]json.RawMessage{}
	}

	set := func(key string, value bool) error {
		encoded, err := json.Marshal(value)
		if err != nil {
			return fmt.Errorf("failed to encode alert default %q: %w", key, err)
		}
		channels[key] = encoded
		return nil
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

	encoded, err := json.Marshal(channels)
	if err != nil {
		return nil, fmt.Errorf("failed to encode alert default channels: %w", err)
	}
	return encoded, nil
}

// decodeAlertJSONObject decodes a raw value into a key map. A nil or empty
// value is an empty object, not an error.
func decodeAlertJSONObject(raw *json.RawMessage) (map[string]json.RawMessage, error) {
	out := map[string]json.RawMessage{}
	if raw == nil || len(*raw) == 0 {
		return out, nil
	}
	if err := json.Unmarshal(*raw, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// rawAlertMessageOrNil adapts a map lookup to the pointer decodeAlertJSONObject
// takes; an absent key reads as nil rather than an empty non-nil value.
func rawAlertMessageOrNil(raw json.RawMessage) *json.RawMessage {
	if len(raw) == 0 {
		return nil
	}
	return &raw
}
