package community

import (
	"encoding/json"
	"strings"
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
	ID            uint             `json:"id" gorm:"primaryKey"`
	EntityType    string           `json:"entity_type" gorm:"column:entity_type;not null"`
	Payload       *json.RawMessage `json:"payload" gorm:"column:payload;type:jsonb;not null"`
	RequesterID   uint             `json:"requester_id" gorm:"column:requester_id;not null"`
	SourceContext string           `json:"source_context" gorm:"column:source_context;not null;default:'manual'"`
	// SourceDetail (PSY-1008) is optional structured origin context — chiefly
	// the AI source article URL + excerpt — stored opaquely as JSONB and typed
	// in Go via EntityRequestSourceDetail. NULL for requests with no source
	// context. Distinct from SourceContext (the origin enum discriminator).
	SourceDetail  *json.RawMessage           `json:"source_detail,omitempty" gorm:"column:source_detail;type:jsonb"`
	DecisionState EntityRequestDecisionState `json:"decision_state" gorm:"column:decision_state;not null;default:'pending'"`
	DecidedBy     *uint                      `json:"decided_by,omitempty" gorm:"column:decided_by"`
	DecidedAt     *time.Time                 `json:"decided_at,omitempty" gorm:"column:decided_at"`
	DecisionNote  *string                    `json:"decision_note,omitempty" gorm:"column:decision_note"`
	// CreatedEntityID (PSY-1008) is the catalog entity created when this request
	// was fulfilled (auto-approve create or admin approve). Cross-type id keyed
	// by EntityType (no FK). NULL while pending/rejected, or when an approval is
	// orphaned (approved but fulfillment failed/deferred — e.g. show).
	CreatedEntityID *uint     `json:"created_entity_id,omitempty" gorm:"column:created_entity_id"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`

	// Relationships
	Requester auth.User  `json:"-" gorm:"foreignKey:RequesterID"`
	Decider   *auth.User `json:"-" gorm:"foreignKey:DecidedBy"`
}

// EntityRequestSourceDetail is the typed shape of the source_detail JSONB
// column (PSY-1008): the origin context a requester saw, primarily for
// AI-extracted requests. Both fields are optional; an all-empty detail is
// normalized to a nil column rather than an empty object (see Normalize).
type EntityRequestSourceDetail struct {
	URL     *string `json:"url,omitempty" doc:"Source article / page URL the request was extracted from"`
	Excerpt *string `json:"excerpt,omitempty" doc:"Source text excerpt the request was extracted from"`
}

// Normalize trims both fields, drops them when empty, and reports whether any
// content remains. A detail with no content after trimming returns ok=false so
// the caller stores NULL instead of an empty {} object. Keeps the trust-boundary
// cleanup in the model next to the struct it cleans.
func (d *EntityRequestSourceDetail) Normalize() (clean EntityRequestSourceDetail, ok bool) {
	if d == nil {
		return EntityRequestSourceDetail{}, false
	}
	if d.URL != nil {
		if t := strings.TrimSpace(*d.URL); t != "" {
			clean.URL = &t
			ok = true
		}
	}
	if d.Excerpt != nil {
		if t := strings.TrimSpace(*d.Excerpt); t != "" {
			clean.Excerpt = &t
			ok = true
		}
	}
	return clean, ok
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
