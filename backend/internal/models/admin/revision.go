package admin

import (
	"encoding/json"
	"time"

	"psychic-homily-backend/internal/models/auth"
)

// Revision tracks a single edit to an entity with field-level diffs.
type Revision struct {
	ID           uint             `json:"id" gorm:"primaryKey"`
	EntityType   string           `json:"entity_type" gorm:"column:entity_type;not null;size:50"`
	EntityID     uint             `json:"entity_id" gorm:"column:entity_id;not null"`
	UserID       uint             `json:"user_id" gorm:"column:user_id;not null"`
	FieldChanges *json.RawMessage `json:"field_changes" gorm:"column:field_changes;type:jsonb;not null"`
	Summary      *string          `json:"summary,omitempty" gorm:"column:summary"`
	CreatedAt    time.Time        `json:"created_at"`

	User auth.User `json:"-" gorm:"foreignKey:UserID"`
}

// TableName specifies the table name for Revision.
func (Revision) TableName() string { return "revisions" }

// FieldChange represents a single field's before/after values.
type FieldChange struct {
	Field    string      `json:"field"`
	OldValue interface{} `json:"old_value"`
	NewValue interface{} `json:"new_value"`
}
