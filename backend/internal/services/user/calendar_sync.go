package user

import (
	"encoding/json"
	"errors"
	"fmt"

	"gorm.io/gorm"

	authm "psychic-homily-backend/internal/models/auth"
	"psychic-homily-backend/internal/services/shared"
)

// Calendar sync choice persistence. The document shape and its rules live in
// models/auth/calendar_sync.go; this file owns storage only.

// SetCalendarSync replaces the user's stored calendar sync choice and returns
// the stored document. REPLACE, not merge: the client sends the whole choice,
// including the merged provider list.
//
// Validation runs here, not only in the handler, so the rule has one home for
// every caller of the interface. Rejections wrap authm.ErrInvalidCalendarSync,
// and nothing reaches the column until they pass.
func (s *UserService) SetCalendarSync(userID uint, choice *authm.CalendarSync) (*authm.CalendarSync, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	if err := choice.Validate(); err != nil {
		return nil, err
	}

	encoded, err := json.Marshal(choice)
	if err != nil {
		return nil, fmt.Errorf("failed to encode calendar sync choice: %w", err)
	}
	raw := json.RawMessage(encoded)

	if err := s.upsertPreference(userID, "calendar_sync", &raw, func(prefs *authm.UserPreferences) {
		prefs.CalendarSync = &raw
	}); err != nil {
		return nil, err
	}
	return choice, nil
}

// GetCalendarSync returns the user's stored calendar sync choice, or nil when
// they have not decided (NULL column or no preferences row). The stored
// document is returned as written: an expired not_now stays not_now.
func (s *UserService) GetCalendarSync(userID uint) (*authm.CalendarSync, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	var prefs authm.UserPreferences
	err := s.db.Select("calendar_sync").Where("user_id = ?", userID).Take(&prefs).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to read calendar sync choice: %w", err)
	}
	if prefs.CalendarSync == nil {
		return nil, nil
	}

	var choice authm.CalendarSync
	if err := json.Unmarshal(*prefs.CalendarSync, &choice); err != nil {
		return nil, fmt.Errorf("failed to decode calendar sync choice: %w", err)
	}
	return &choice, nil
}

// ClearCalendarSync resets the user's choice to "not decided".
func (s *UserService) ClearCalendarSync(userID uint) error {
	if s.db == nil {
		return fmt.Errorf("database not initialized")
	}
	return shared.ClearCalendarSyncTx(s.db, userID)
}
