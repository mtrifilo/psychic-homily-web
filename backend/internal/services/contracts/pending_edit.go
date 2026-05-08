package contracts

import (
	adminm "psychic-homily-backend/internal/models/admin"
	engagementm "psychic-homily-backend/internal/models/engagement"
	"time"
)

// MaxPendingEditSummaryLength is the maximum length, in bytes, accepted for
// the suggest-edit `summary` (the contributor's reason for the edit) and the
// admin's `rejection_reason`. Both are rendered through utils.MarkdownRenderer
// (PSY-605), so the cap mirrors engagementm.MaxCommentBodyLength to share the
// same surface-level UX guarantees as comments + collection descriptions.
const MaxPendingEditSummaryLength = engagementm.MaxCommentBodyLength

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
	ID            uint   `json:"id"`
	EntityType    string `json:"entity_type"`
	EntityID      uint   `json:"entity_id"`
	EntityName    string `json:"entity_name,omitempty"`
	SubmittedBy   uint   `json:"submitted_by"`
	SubmitterName string `json:"submitter_name,omitempty"`
	// SubmitterUsername is the submitter's username when set — pointer so
	// the JSON encodes null for accounts that never set a username. Frontend
	// renders the byline as a link to /users/:username when non-nil; nil
	// renders as plain text. PSY-619.
	SubmitterUsername *string                  `json:"submitter_username"`
	FieldChanges      []adminm.FieldChange     `json:"field_changes"`
	// Summary is the raw markdown source of the contributor's reason for the
	// edit; SummaryHTML is the sanitized rendered HTML produced on each read
	// via utils.MarkdownRenderer (goldmark + bluemonday, comment-system
	// allowlist). Mirrors the comment + collection-description shape so the
	// moderation queue can render formatted bold/links/quotes consistently
	// (PSY-605). Summary (raw) is preserved alongside HTML so the contributor
	// can re-populate the textarea in a future edit-the-pending-edit flow.
	Summary          string                   `json:"summary"`
	SummaryHTML      string                   `json:"summary_html,omitempty"`
	Status           adminm.PendingEditStatus `json:"status"`
	ReviewedBy       *uint                    `json:"reviewed_by,omitempty"`
	ReviewerName     string                   `json:"reviewer_name,omitempty"`
	ReviewerUsername *string                  `json:"reviewer_username"`
	ReviewedAt       *time.Time               `json:"reviewed_at,omitempty"`
	// RejectionReason is the raw markdown source of the admin's rejection
	// note; RejectionReasonHTML is the sanitized rendered HTML, computed on
	// read for the same reason as Summary above (PSY-605). Same renderer +
	// allowlist; consumed by the contributor-side pending-edits view (when
	// PSY-600 ships) so submitters see the moderator's note with the same
	// formatting they get from comments.
	RejectionReason     *string   `json:"rejection_reason,omitempty"`
	RejectionReasonHTML string    `json:"rejection_reason_html,omitempty"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
}
