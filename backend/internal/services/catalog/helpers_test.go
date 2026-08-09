package catalog

import (
	"encoding/json"
	"testing"

	"gorm.io/gorm"

	adminm "psychic-homily-backend/internal/models/admin"
)

// stringPtr returns a pointer to the given string. Test helper.
func stringPtr(s string) *string {
	return &s
}

// intPtr returns a pointer to the given int. Test helper.
func intPtr(i int) *int {
	return &i
}

// seedRevision records one edit against an entity and returns its id.
//
// Written through the model rather than raw SQL so a column rename breaks the
// build instead of the test run, and so the row carries whatever defaults the
// model declares — from_unverified_venue in particular, which the merge tests
// assert on.
func seedRevision(t *testing.T, db *gorm.DB, entityType string, entityID, userID uint, summary string) uint {
	t.Helper()

	changes := json.RawMessage(`[{"field":"title","old_value":"before","new_value":"after"}]`)
	rev := &adminm.Revision{
		EntityType:   entityType,
		EntityID:     entityID,
		UserID:       userID,
		FieldChanges: &changes,
		Summary:      &summary,
	}
	if err := db.Create(rev).Error; err != nil {
		t.Fatalf("failed to seed %s revision: %v", entityType, err)
	}
	return rev.ID
}
