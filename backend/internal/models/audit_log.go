package models

import (
	"encoding/json"
	"time"
)

// AuditLog represents an admin action audit trail entry
type AuditLog struct {
	ID         uint             `gorm:"primaryKey"`
	ActorID    *uint            `gorm:"column:actor_id"`
	Action     string           `gorm:"column:action;not null"`
	EntityType string           `gorm:"column:entity_type;not null"`
	EntityID   uint             `gorm:"column:entity_id;not null"`
	Metadata   *json.RawMessage `gorm:"column:metadata;type:jsonb"`
	CreatedAt  time.Time        `gorm:"not null"`

	Actor *User `gorm:"foreignKey:ActorID"`
}

// TableName specifies the table name for AuditLog
func (AuditLog) TableName() string {
	return "audit_logs"
}
