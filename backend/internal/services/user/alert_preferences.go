package user

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	autherrors "psychic-homily-backend/internal/errors"
	authm "psychic-homily-backend/internal/models/auth"
	engagementm "psychic-homily-backend/internal/models/engagement"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/services/engagement"
	"psychic-homily-backend/internal/services/geo"
)

// Account-level alert preferences (PSY-1907): the user's home area and the
// alert matrix every follow inherits from. The shape, the shipped defaults and
// the absent-means-inherit merge live in models/auth/alert_defaults.go; this
// file owns validation and persistence only.

// homeMetroMaxLength matches the user_preferences.home_metro column width,
// which in turn matches venues.metro. US CBSA codes are five digits.
const homeMetroMaxLength = 10

// GetAlertPreferences returns the user's home area and RESOLVED account alert
// matrix. A user with no preferences row is not an error: they simply have no
// home area and inherit every shipped default, which is exactly what a NULL
// alert_defaults resolves to.
func (s *UserService) GetAlertPreferences(userID uint) (*authm.AlertPreferences, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	var prefs authm.UserPreferences
	err := s.db.Select("home_metro", "alert_defaults").
		Where("user_id = ?", userID).
		Take(&prefs).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("failed to get alert preferences: %w", err)
	}

	return &authm.AlertPreferences{
		HomeMetro:     prefs.HomeMetro,
		AlertDefaults: authm.ResolveAccountAlertDefaults(prefs.AlertDefaults),
	}, nil
}

// SetHomeMetro replaces the user's home area. A nil or blank metro clears it.
//
// Validation posture: the code must resolve in the embedded GeoNames/Census
// dataset via geo.MetroPrincipalByCBSA, the SAME source that assigns
// venues.metro and artists.metro. Anything else is rejected rather than stored,
// because a home metro that no venue can ever match would make near-me scoping
// deliver nothing while looking configured. The dataset is US CBSA only, so a
// non-US user cannot set a home area today. That is the same limit venues
// already carry, and it degrades correctly: no home area means near-me falls back to
// everywhere (engagement.EffectiveShowScope) instead of silently delivering
// nothing.
func (s *UserService) SetHomeMetro(userID uint, metro *string) error {
	if s.db == nil {
		return fmt.Errorf("database not initialized")
	}

	var value *string
	if metro != nil {
		trimmed := strings.TrimSpace(*metro)
		if trimmed != "" {
			// Length-check first so neither the lookup nor the error message
			// ever handles more than a code's worth of caller-supplied text,
			// and so an over-long value cannot reach a VARCHAR(10) column if
			// this validation is ever loosened.
			if len(trimmed) > homeMetroMaxLength {
				return autherrors.ErrUnknownHomeMetro(
					fmt.Sprintf("code is longer than %d characters", homeMetroMaxLength))
			}
			if _, ok := geo.MetroPrincipalByCBSA(trimmed); !ok {
				return autherrors.ErrUnknownHomeMetro(trimmed)
			}
			value = &trimmed
		}
	}

	return s.upsertPreference(userID, "home_metro", value, func(prefs *authm.UserPreferences) {
		prefs.HomeMetro = value
	})
}

// SetAccountAlertDefaults applies a partial update to the account alert matrix.
// An update that sets no channel is a no-op rather than a write: storing an
// empty document would dirty the row (and its inheritance) to say what NULL
// already says.
func (s *UserService) SetAccountAlertDefaults(userID uint, update authm.AccountAlertDefaultsUpdate) error {
	if s.db == nil {
		return fmt.Errorf("database not initialized")
	}
	if update.SetsNothing() {
		return nil
	}

	// Create the row first, if it is not already there, so the merge below
	// ALWAYS has a row to lock. Without this the first-ever write has nothing
	// to take a lock on, and two concurrent first writes would each merge from
	// NULL and the loser would overwrite the winner's whole document rather
	// than only its own cell: a settings card saving two alert types at once
	// would report success and silently drop one of them.
	if err := ensureUserPreferencesRow(s.db, userID); err != nil {
		return err
	}

	// Row lock, then read-modify-write in Go. The merge has to preserve alert
	// types and channels this update does not mention, so it needs the current
	// document; locking it keeps a concurrent sibling write from being lost
	// between the read and the write.
	return s.db.Transaction(func(tx *gorm.DB) error {
		var prefs authm.UserPreferences
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("user_id = ?", userID).
			Take(&prefs).Error; err != nil {
			// Not-found is a real failure now: the row was just ensured, so its
			// absence means it was deleted underneath us (account deletion).
			return fmt.Errorf("failed to load user preferences: %w", err)
		}

		merged, err := authm.MergeAccountAlertDefaults(prefs.AlertDefaults, update)
		if err != nil {
			return fmt.Errorf("failed to merge alert defaults: %w", err)
		}
		if merged == nil {
			// SetsNothing() already covered this; belt and braces so a future
			// axis cannot start writing an empty document by accident.
			return nil
		}

		raw := json.RawMessage(merged)
		if err := tx.Model(&authm.UserPreferences{}).
			Where("user_id = ?", userID).
			Update("alert_defaults", &raw).Error; err != nil {
			return fmt.Errorf("failed to update alert defaults: %w", err)
		}
		return nil
	})
}

// UnsubscribeArtistShowAlertEmails stops the new-show alert EMAILS for a user
// across every follow type that sends them (artist, venue and scene), and is
// what the RFC 8058 one-click link behind those emails calls (PSY-1896,
// PSY-1895, PSY-1926).
//
// The name is narrower than the behaviour and is kept in step with
// engagement.UnsubscribeScopeArtistShowAlerts, whose string value is frozen by
// the links already in recipients' inboxes. One method because it is one
// mutation: a second entry point for the same write is how the two drift.
//
// It takes two writes because the preference is resolved from two layers and
// either one alone can keep the mail flowing:
//
//  1. The account matrix's shows.email goes false. That reaches every follow the
//     user never configured, which is most of them.
//  2. Every EXPLICIT per-follow email:true override on an artist follow is
//     cleared. Those sit BELOW the account defaults in the inherit chain, so
//     without this step a user who once switched email on for one band would
//     click unsubscribe, be told it worked, and keep getting mail for that band.
//     An unsubscribe that does not unsubscribe is worse than no link at all: the
//     recipient's next move is Report Spam, which costs sending reputation for
//     every other email the platform sends.
//
// The IN-APP channel is deliberately untouched. The user refused an email, not
// the product's notifications, and silently emptying their inbox as well would
// be a bigger change than the button they pressed.
//
// Not atomic across the two writes, and the order is chosen so a failure between
// them fails SAFE in the direction that matters: the account write lands first,
// so a crash before the override sweep leaves the majority of follows already
// silenced and the endpoint reports an error the caller surfaces. The reverse
// order would report success with the account default still on.
func (s *UserService) UnsubscribeArtistShowAlertEmails(userID uint) error {
	if s.db == nil {
		return fmt.Errorf("database not initialized")
	}

	off := false
	if err := s.SetAccountAlertDefaults(userID, authm.AccountAlertDefaultsUpdate{
		Shows: &authm.AlertChannelDefaultsUpdate{Email: &off},
	}); err != nil {
		return fmt.Errorf("failed to clear account show-alert email default: %w", err)
	}

	// FollowService is a thin handle over the same *gorm.DB (NewFollowService
	// stores it and nothing else), so constructing one here costs nothing and
	// keeps the user_bookmarks JSONB merge in the package that owns it rather
	// than growing a second implementation of it. If it ever acquires real
	// dependencies, this becomes a constructor argument.
	follows := engagement.NewFollowService(s.db)

	// EVERY follow type that carries show alerts, because the account write
	// above is not narrower than that: alert_defaults has ONE `shows` key
	// covering artist, venue and scene show alerts alike. Sweeping a subset
	// would leave the two halves of this unsubscribe disagreeing about what it
	// silenced, and the half that disagreed would be the one that keeps sending.
	//
	// Scenes joined the list in PSY-1926, when their fanout stopped emailing
	// unconditionally and started resolving the same chain. A scene follow that
	// carries no override is already silenced by the account write; this reaches
	// the ones that pinned email on.
	for _, entityType := range []string{
		string(engagementm.BookmarkEntityArtist),
		string(engagementm.BookmarkEntityVenue),
		string(engagementm.BookmarkEntityScene),
	} {
		if err := follows.DisableFollowAlertEmailChannel(
			userID, entityType, contracts.FollowAlertTypeShows,
		); err != nil {
			return fmt.Errorf("failed to clear per-follow show-alert email overrides for %s: %w",
				entityType, err)
		}
	}
	return nil
}

// ensureUserPreferencesRow creates the user's preferences row if it has none,
// leaving every column at its DDL default. DO NOTHING rather than DO UPDATE:
// this call must never change a value, only guarantee that a row exists for a
// caller that is about to lock and merge one.
func ensureUserPreferencesRow(db *gorm.DB, userID uint) error {
	prefs := authm.UserPreferences{UserID: userID}
	if err := db.Clauses(clause.OnConflict{DoNothing: true}).Create(&prefs).Error; err != nil {
		return fmt.Errorf("failed to create user preferences: %w", err)
	}
	return nil
}

// upsertPreference updates one column on the user's preferences row, creating
// the row when there is none. apply seeds the same value onto the new row so
// the create and the update cannot drift.
func (s *UserService) upsertPreference(
	userID uint,
	column string,
	value any,
	apply func(*authm.UserPreferences),
) error {
	result := s.db.Model(&authm.UserPreferences{}).
		Where("user_id = ?", userID).
		Update(column, value)
	if result.Error != nil {
		return fmt.Errorf("failed to update %s: %w", column, result.Error)
	}
	if result.RowsAffected > 0 {
		return nil
	}

	prefs := &authm.UserPreferences{UserID: userID}
	apply(prefs)
	return upsertUserPreferences(s.db, prefs, column)
}

// upsertUserPreferences inserts a preferences row, or applies just this
// column's value when a concurrent request created the row first. A plain
// Create would fail the user_id unique index in that window, turning a benign
// race between two settings toggles into a 500. DoUpdates names ONE column so
// the losing side of that race cannot reset a sibling preference to the zero
// value it happens to be carrying.
//
// The row-creating path is the only one without a lock, because there is no
// row to lock yet: two first-ever alert-defaults writes racing here can leave
// only the later one's cell set. Every subsequent write locks the row and
// merges, so the window is one request wide and self-heals on the next save.
//
// Every column the row does not set is left to its DDL default on purpose:
// GORM omits a zero value for any field carrying a `default` tag, so the
// opt-out booleans insert as TRUE and the opt-in ones as FALSE exactly as the
// migrations declare, instead of all landing on Go's false. The two alert
// columns are nullable pointers precisely so they stay OUT of that mechanism:
// NULL there is a meaningful value ("inherit"), not an artefact of a zero value.
func upsertUserPreferences(db *gorm.DB, prefs *authm.UserPreferences, column string) error {
	err := db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "user_id"}},
		DoUpdates: clause.AssignmentColumns([]string{column}),
	}).Create(prefs).Error
	if err != nil {
		return fmt.Errorf("failed to create user preferences: %w", err)
	}
	return nil
}
