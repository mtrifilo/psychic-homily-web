package user

import (
	"encoding/json"
	"time"

	"gorm.io/gorm"
	adminm "psychic-homily-backend/internal/models/admin"
)

// stringPtr returns a pointer to a string. Test helper.
func stringPtr(s string) *string { return &s }

// testAuditLogHelper writes audit log entries directly to the database,
// avoiding a circular import on the parent services package.
type testAuditLogHelper struct {
	db *gorm.DB
}

// LogAction creates an audit log entry directly in the database.
func (h *testAuditLogHelper) LogAction(actorID uint, action string, entityType string, entityID uint, metadata map[string]interface{}) {
	log := adminm.AuditLog{
		ActorID:    &actorID,
		Action:     action,
		EntityType: entityType,
		EntityID:   entityID,
		CreatedAt:  time.Now().UTC(),
	}

	if metadata != nil {
		metadataJSON, err := json.Marshal(metadata)
		if err == nil {
			raw := json.RawMessage(metadataJSON)
			log.Metadata = &raw
		}
	}

	h.db.Create(&log)
}
