package contracts

import (
	"time"

	authm "psychic-homily-backend/internal/models/auth"
	communitym "psychic-homily-backend/internal/models/community"
)

// ──────────────────────────────────────────────
// Entity Request Service Interface (PSY-997)
// ──────────────────────────────────────────────

// EntityRequestServiceInterface defines the contract for the polymorphic
// entity-creation moderation queue (PSY-869's service). The HTTP handlers
// (PSY-997) depend on this interface so they can be unit-tested with the
// generated mock instead of a live DB.
//
// CreateRequest / GetRequest / ListPending / Decide mirror the PSY-869
// service in services/community/entityrequest.go EXACTLY (that file is owned
// by PSY-869 and is not modified here). ListRequests is the admin-list query
// added by PSY-997 (sibling file entityrequest_list.go) — ListPending alone
// can't satisfy the state + source_context filtering the admin queue needs.
type EntityRequestServiceInterface interface {
	// CreateRequest persists a typed entity-creation request, applying
	// trust-tier gating to decide whether it auto-approves or queues.
	// sourceDetail is the optional, already-marshalled source-context JSONB
	// (nil = none).
	//
	// On a duplicate PENDING request (same entity_type + requester + normalized
	// name + occurrence, the key uq_entity_requests_pending_dedup enforces) it
	// does NOT error (PSY-1008): the resubmission REPLACES that pending row's
	// payload, source_context and source_detail, and the refreshed row is
	// returned with a non-nil superseded (PSY-1948). Only a PENDING row is ever
	// written, so the caller must not treat a replacement as a fresh decision —
	// and must not fulfill one, since the row is re-read in a separate statement
	// and can come back already decided by an admin.
	//
	// superseded carries the submission the replacement destroyed (PSY-1978). The
	// write is unrecoverable from the table, and the row's own
	// original_source_context records only the FIRST filing's source_context, so
	// this is the only place the overwritten payload and source detail exist after
	// the UPDATE commits.
	//
	// Its Payload and SourceDetail are CONTRIBUTOR-AUTHORED and unmoderated. A
	// caller that records them has to know where its record can be read: the HTTP
	// handler's audit row is served to anonymous callers through the contributor
	// profile, so it records a digest rather than the content. That handler's
	// write is also fire-and-forget, so what it records is best-effort; the
	// column is the durable half.
	//
	// superseded is always nil for an AUTO-APPROVING tier: its row is stamped
	// 'approved' before the insert and never meets the pending-only dedup index,
	// so it files a new approved row and leaves an earlier pending request queued
	// with its original payload.
	//
	// A replacement DESTROYS the stored payload, so the service re-checks the
	// payload's typed STRUCTURE before the overwrite and fails closed. That is the
	// only guard it can run: the show bill's set_type vocabulary
	// (validateShowPayloadBillRoles) and the image_url SSRF host guard
	// (validatePayloadImageURL) live at the HTTP boundary — the first because the
	// vocabulary is in a package that imports these models, the second because it
	// needs a request context — and NEITHER is re-run here. Any caller that is not
	// the HTTP handler MUST run both itself, or it can queue a payload that 422s
	// at fulfillment, after the row is claimed and past repair.
	CreateRequest(user *authm.User, entityType string, payload []byte, sourceContext string, sourceDetail []byte, confirmed bool) (req *communitym.EntityRequest, superseded *communitym.SupersededSubmission, err error)

	// RecordFulfillment persists created_entity_id on a request after its
	// payload has been fulfilled into a real catalog entity (PSY-1008). The
	// handler calls this on both the auto-approve create path and the admin
	// approve path once the fulfiller returns the new entity's id.
	RecordFulfillment(requestID, createdEntityID uint) error

	// GetRequest retrieves a request by ID (requester + decider preloaded);
	// returns (nil, nil) when not found.
	GetRequest(requestID uint) (*communitym.EntityRequest, error)

	// ListPending returns pending requests filtered by entity_type only.
	// PSY-869's original queue query. PSY-997's admin list uses ListRequests.
	ListPending(entityType string, limit, offset int) ([]communitym.EntityRequest, int64, error)

	// ListRequests returns requests for the admin queue, filterable by
	// entity_type + decision_state + source_context, paginated newest-first.
	// Unfulfilled (PSY-1088) additionally narrows to approved-but-unfulfilled
	// rows (created_entity_id IS NULL) — the rescue queue.
	ListRequests(filters *EntityRequestFilters) ([]communitym.EntityRequest, int64, error)

	// Decide records an admin's moderation decision atomically (the conditional
	// UPDATE guards against concurrent decisions). Marks approved/rejected +
	// decided_by/at + optional note. Creating the actual entity from the
	// payload is the HANDLER's responsibility (PSY-997), not the service's.
	//
	// expectedUpdatedAt makes the claim optimistic (PSY-1948). A caller that ran
	// checks against the stored payload MUST pass the updated_at its read saw:
	// the requester can replace a pending payload, and the approve path's SSRF
	// guard and show-bill admission are not re-run at fulfillment, so a claim that
	// ignores the version can fulfill content nothing validated. A revised row
	// then fails with ErrEntityRequestStale (409) and stays pending. nil skips the
	// check and is correct only for a decision that reads nothing off the row.
	Decide(requestID, adminID uint, newState communitym.EntityRequestDecisionState, note *string, expectedUpdatedAt *time.Time) (*communitym.EntityRequest, error)

	// Withdraw retracts the REQUESTER's own pending request (PSY-1992), moving
	// it to decision_state 'withdrawn' and stamping decided_by/at with the
	// requester. The conditional UPDATE carries the whole precondition (this row,
	// this requester, still pending), so authorization is not the caller's to
	// enforce here: another user's row and an already-decided row are both
	// refused by the write itself.
	//
	// Refusals are told apart: a row that does not exist OR belongs to someone
	// else is ErrEntityRequestNotFound (the caller learns nothing about a row
	// that is not theirs), and the caller's OWN non-pending row is
	// ErrEntityRequestInvalidState naming the state it is in.
	Withdraw(requestID, requesterID uint) (*communitym.EntityRequest, error)

	// ClaimRescueFulfillment atomically stamps created_entity_id on an
	// APPROVED-but-UNFULFILLED row (PSY-1088 rescue path). The conditional
	// UPDATE (WHERE decision_state='approved' AND created_entity_id IS NULL)
	// guards two concurrent rescues from both winning: the loser sees
	// claimed=false and the catalog entity it created is a recoverable stray.
	// Unlike RecordFulfillment, this never overwrites an existing link.
	// Returns claimed=false (no error) when the row is missing, not approved,
	// or already fulfilled — the handler then reports the right conflict.
	ClaimRescueFulfillment(requestID, createdEntityID uint) (claimed bool, err error)

	// VoidApprovedUnfulfilled atomically rejects an APPROVED-but-UNFULFILLED
	// row (PSY-1088 rescue path) so an admin can dismiss an orphan that should
	// never have been approved, without DB surgery. The conditional UPDATE
	// (WHERE decision_state='approved' AND created_entity_id IS NULL) ensures a
	// fulfilled row can never be voided out from under its created entity.
	// Returns voided=false (no error) when the row no longer qualifies.
	VoidApprovedUnfulfilled(requestID, adminID uint, note *string) (voided bool, err error)
}

// EntityRequestFilters holds the admin-queue list filters. Empty string for a
// field means "no filter on that dimension". The handler validates State and
// SourceContext against the model's allowed enums before calling.
type EntityRequestFilters struct {
	EntityType    string // "artist", "venue", ... ; "" = all types
	State         string // "pending", "approved", "rejected"; "" = pending (default)
	SourceContext string // "ai_extraction", "paste_mode", "manual"; "" = all sources
	// Unfulfilled (PSY-1088), when true, narrows to rows whose catalog entity
	// was never created (created_entity_id IS NULL) — the approved-but-
	// unfulfilled "needs attention" rescue queue. Combine with State="approved".
	Unfulfilled bool
	Limit       int
	Offset      int
}

// EntityRequestFulfiller creates the actual catalog entity from an approved
// request's payload. It is intentionally narrow — only the create methods the
// approve path needs — so the decide handler doesn't depend on the full
// catalog service interfaces. The concrete implementation lives in the service
// container, composing the per-entity catalog services.
//
// Show is fulfillable only when the admin supplies the venue + artist
// associations at approve time (PSY-1037) — its catalog create contract
// requires ≥1 venue + ≥1 artist that the entity_request payload does not
// carry. Without admin-supplied associations (e.g. the auto-approve create
// path), the dispatcher still returns a typed "fulfillment unsupported" error
// and the request defers gracefully.
//
// Festival IS fulfillable (PSY-998): the only field its create contract needs
// beyond the payload is series_slug, which the fulfiller derives from the
// festival name.
type EntityRequestFulfillerInterface interface {
	CreateArtist(req *CreateArtistRequest) (*ArtistDetailResponse, error)
	CreateVenue(req *CreateVenueRequest, isAdmin bool) (*VenueDetailResponse, error)
	CreateLabel(req *CreateLabelRequest) (*LabelDetailResponse, error)
	CreateRelease(req *CreateReleaseRequest) (*ReleaseDetailResponse, error)
	CreateFestival(req *CreateFestivalRequest) (*FestivalDetailResponse, error)
	CreateShow(req *CreateShowRequest) (*ShowResponse, error)
}
