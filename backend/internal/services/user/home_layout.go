package user

import (
	"encoding/json"
	"fmt"

	authm "psychic-homily-backend/internal/models/auth"
)

// Signed-in home layout (PSY-386). The document shape and its rules live in
// models/auth/home_layout.go; this file owns persistence only.

// SetHomeLayout replaces the user's stored home layout and returns what was
// stored, so the caller echoes the persisted document rather than its own
// input.
//
// REPLACE, not merge: the document is an ordered list, and a merge has no
// defensible answer for where a section the update omits belongs. The writer
// is one popover holding the whole arrangement, so it always has the full
// list to send.
//
// Validation runs here rather than only in the handler so the rule has one
// home for every caller of the interface. Rejections wrap
// authm.ErrInvalidHomeLayout, which is how the handler tells a bad document
// from a failed write.
func (s *UserService) SetHomeLayout(userID uint, layout *authm.HomeLayout) (*authm.HomeLayout, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	if err := layout.Validate(); err != nil {
		return nil, err
	}

	encoded, err := json.Marshal(layout)
	if err != nil {
		return nil, fmt.Errorf("failed to encode home layout: %w", err)
	}
	raw := json.RawMessage(encoded)

	if err := s.upsertPreference(userID, "home_layout", &raw, func(prefs *authm.UserPreferences) {
		prefs.HomeLayout = &raw
	}); err != nil {
		return nil, err
	}
	return layout, nil
}

// ClearHomeLayout resets the user's layout to the shipped default by setting
// the column back to NULL.
//
// A user with no preferences row is already at the default, so this must not
// create one: an empty row would say what its absence already says. That makes
// the no-row case a successful no-op rather than the row-creating path
// upsertPreference would take.
func (s *UserService) ClearHomeLayout(userID uint) error {
	if s.db == nil {
		return fmt.Errorf("database not initialized")
	}

	var null *json.RawMessage
	result := s.db.Model(&authm.UserPreferences{}).
		Where("user_id = ?", userID).
		Update("home_layout", null)
	if result.Error != nil {
		return fmt.Errorf("failed to clear home layout: %w", result.Error)
	}
	return nil
}
