package contracts

import (
	adminm "psychic-homily-backend/internal/models/admin"
	"time"
)

// ──────────────────────────────────────────────
// Pending Edit Service Interface
// ──────────────────────────────────────────────

// PendingEditServiceInterface defines the contract for managing pending entity edits.
type PendingEditServiceInterface interface {
	// CreatePendingEdit submits a new pending edit for an entity.
	CreatePendingEdit(req *CreatePendingEditRequest) (*PendingEditResponse, error)

	// GetPendingEdit returns a single pending edit by ID.
	GetPendingEdit(editID uint) (*PendingEditResponse, error)

	// GetPendingEditsForEntity returns all pending edits for a specific entity.
	GetPendingEditsForEntity(entityType string, entityID uint) ([]PendingEditResponse, error)

	// GetUserPendingEdits returns all pending edits submitted by a user.
	GetUserPendingEdits(userID uint, limit, offset int) ([]PendingEditResponse, int64, error)

	// ListPendingEdits returns pending edits for the admin review queue.
	ListPendingEdits(filters *PendingEditFilters) ([]PendingEditResponse, int64, error)

	// ApprovePendingEdit approves a pending edit, applying changes to the entity.
	ApprovePendingEdit(editID uint, reviewerID uint) (*PendingEditResponse, error)

	// RejectPendingEdit rejects a pending edit with a reason.
	RejectPendingEdit(editID uint, reviewerID uint, reason string) (*PendingEditResponse, error)

	// CancelPendingEdit allows the submitter to cancel their own pending edit.
	CancelPendingEdit(editID uint, userID uint) error
}

// ──────────────────────────────────────────────
// Request / Response Types
// ──────────────────────────────────────────────

// CreatePendingEditRequest contains the data needed to submit a pending edit.
type CreatePendingEditRequest struct {
	EntityType string               `json:"entity_type"`
	EntityID   uint                 `json:"entity_id"`
	UserID     uint                 `json:"-"`
	Changes    []adminm.FieldChange `json:"changes"`
	Summary    string               `json:"summary"`
}

// PendingEditFilters contains filters for listing pending edits.
type PendingEditFilters struct {
	Status     string `json:"status,omitempty"`      // "pending", "approved", "rejected"
	EntityType string `json:"entity_type,omitempty"` // "artist", "venue", "festival"
	Limit      int    `json:"limit,omitempty"`
	Offset     int    `json:"offset,omitempty"`
}

// PendingEditResponse is the API response for a pending entity edit.
type PendingEditResponse struct {
	ID              uint                     `json:"id"`
	EntityType      string                   `json:"entity_type"`
	EntityID        uint                     `json:"entity_id"`
	EntityName      string                   `json:"entity_name,omitempty"`
	SubmittedBy     uint                     `json:"submitted_by"`
	SubmitterName   string                   `json:"submitter_name,omitempty"`
	FieldChanges    []adminm.FieldChange     `json:"field_changes"`
	Summary         string                   `json:"summary"`
	Status          adminm.PendingEditStatus `json:"status"`
	ReviewedBy      *uint                    `json:"reviewed_by,omitempty"`
	ReviewerName    string                   `json:"reviewer_name,omitempty"`
	ReviewedAt      *time.Time               `json:"reviewed_at,omitempty"`
	RejectionReason *string                  `json:"rejection_reason,omitempty"`
	CreatedAt       time.Time                `json:"created_at"`
	UpdatedAt       time.Time                `json:"updated_at"`
}
