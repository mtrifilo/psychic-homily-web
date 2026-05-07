package admin

import (
	"encoding/json"
	"time"

	"psychic-homily-backend/internal/models/auth"
)

// EntityEditAuditLog records direct content-edit events on knowledge-graph
// entities (artist, venue, release, label, festival, show). Edits are split
// out of audit_logs (PSY-618) so stats counters can read a single source of
// truth and the contributor activity feed stops dual-rendering trusted-user
// direct-edits (which also write a pending_entity_edits row).
//
// Moderation events ("approve_show", "verify_venue", "create_artist", ...)
// stay in audit_logs.
type EntityEditAuditLog struct {
	ID         uint             `gorm:"primaryKey"`
	ActorID    *uint            `gorm:"column:actor_id"`
	EntityType string           `gorm:"column:entity_type;not null"`
	EntityID   uint             `gorm:"column:entity_id;not null"`
	Metadata   *json.RawMessage `gorm:"column:metadata;type:jsonb"`
	CreatedAt  time.Time        `gorm:"not null"`

	Actor *auth.User `gorm:"foreignKey:ActorID"`
}

// TableName specifies the table name for EntityEditAuditLog.
func (EntityEditAuditLog) TableName() string {
	return "entity_edit_audit_logs"
}
