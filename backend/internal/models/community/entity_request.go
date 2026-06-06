package community

import (
	"encoding/json"
	"time"

	"psychic-homily-backend/internal/models/auth"
)

// PSY-869: entity_requests — a polymorphic moderation queue for user-requested
// ENTITY CREATION. Distinct from the `requests` wishlist (community.Request):
// see the migration header for the rationale. The envelope columns are shared
// across all entity types; the per-type shape lives in the JSONB payload,
// typed by the structs in entity_request_payloads.go.

// EntityRequestDecisionState enumerates moderation outcomes.
type EntityRequestDecisionState string

const (
	EntityRequestStatePending  EntityRequestDecisionState = "pending"
	EntityRequestStateApproved EntityRequestDecisionState = "approved"
	EntityRequestStateRejected EntityRequestDecisionState = "rejected"
)

// Source context enumerates how a request originated. Extensible the same way
// as entity_type — add a value here AND in the migration's CHECK constraint.
const (
	EntityRequestSourceAIExtraction = "ai_extraction"
	EntityRequestSourcePasteMode    = "paste_mode"
	EntityRequestSourceManual       = "manual"
)

// EntityRequest is the polymorphic envelope row. Payload is stored as
// *json.RawMessage (project convention for JSONB — datatypes.JSON is not in
// go.mod; mirrors admin.AuditLog / admin.PendingEntityEdit).
type EntityRequest struct {
	ID            uint                       `json:"id" gorm:"primaryKey"`
	EntityType    string                     `json:"entity_type" gorm:"column:entity_type;not null"`
	Payload       *json.RawMessage           `json:"payload" gorm:"column:payload;type:jsonb;not null"`
	RequesterID   uint                       `json:"requester_id" gorm:"column:requester_id;not null"`
	SourceContext string                     `json:"source_context" gorm:"column:source_context;not null;default:'manual'"`
	DecisionState EntityRequestDecisionState `json:"decision_state" gorm:"column:decision_state;not null;default:'pending'"`
	DecidedBy     *uint                      `json:"decided_by,omitempty" gorm:"column:decided_by"`
	DecidedAt     *time.Time                 `json:"decided_at,omitempty" gorm:"column:decided_at"`
	DecisionNote  *string                    `json:"decision_note,omitempty" gorm:"column:decision_note"`
	CreatedAt     time.Time                  `json:"created_at"`
	UpdatedAt     time.Time                  `json:"updated_at"`

	// Relationships
	Requester auth.User  `json:"-" gorm:"foreignKey:RequesterID"`
	Decider   *auth.User `json:"-" gorm:"foreignKey:DecidedBy"`
}

// TableName specifies the table name for EntityRequest.
func (EntityRequest) TableName() string { return "entity_requests" }

// IsValidEntityRequestState reports whether s is a recognized decision_state.
// Used at the admin-list trust boundary to validate the optional state filter
// before it reaches the query. PSY-997.
func IsValidEntityRequestState(s string) bool {
	switch EntityRequestDecisionState(s) {
	case EntityRequestStatePending, EntityRequestStateApproved, EntityRequestStateRejected:
		return true
	default:
		return false
	}
}

// IsValidEntityRequestSource reports whether s is a recognized source_context.
// Mirrors the migration's CHECK constraint and the service's (unexported)
// source-context guard; exported so the HTTP handlers can validate the
// queue-create body + the admin-list source filter. PSY-997.
func IsValidEntityRequestSource(s string) bool {
	switch s {
	case EntityRequestSourceAIExtraction, EntityRequestSourcePasteMode, EntityRequestSourceManual:
		return true
	default:
		return false
	}
}
