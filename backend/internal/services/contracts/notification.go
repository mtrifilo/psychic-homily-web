package contracts

import "time"

// ──────────────────────────────────────────────
// Audit Log types
// ──────────────────────────────────────────────

// AuditLogFilters represents optional filters for querying audit logs
type AuditLogFilters struct {
	EntityType string
	Action     string
	ActorID    *uint
}

// AuditLogResponse represents an audit log entry in API responses
type AuditLogResponse struct {
	ID         uint                   `json:"id"`
	ActorID    *uint                  `json:"actor_id"`
	ActorEmail string                 `json:"actor_email,omitempty"`
	Action     string                 `json:"action"`
	EntityType string                 `json:"entity_type"`
	EntityID   uint                   `json:"entity_id"`
	Metadata   map[string]interface{} `json:"metadata,omitempty"`
	CreatedAt  time.Time              `json:"created_at"`
}
