package contracts

import (
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
	// (nil = none). On a duplicate PENDING request (same entity_type +
	// requester + normalized name), it returns the EXISTING pending row
	// idempotently rather than erroring (PSY-1008).
	CreateRequest(user *authm.User, entityType string, payload []byte, sourceContext string, sourceDetail []byte, confirmed bool) (*communitym.EntityRequest, error)

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
	ListRequests(filters *EntityRequestFilters) ([]communitym.EntityRequest, int64, error)

	// Decide records an admin's moderation decision atomically (the conditional
	// UPDATE guards against concurrent decisions). Marks approved/rejected +
	// decided_by/at + optional note. Creating the actual entity from the
	// payload is the HANDLER's responsibility (PSY-997), not the service's.
	Decide(requestID, adminID uint, newState communitym.EntityRequestDecisionState, note *string) (*communitym.EntityRequest, error)
}

// EntityRequestFilters holds the admin-queue list filters. Empty string for a
// field means "no filter on that dimension". The handler validates State and
// SourceContext against the model's allowed enums before calling.
type EntityRequestFilters struct {
	EntityType    string // "artist", "venue", ... ; "" = all types
	State         string // "pending", "approved", "rejected"; "" = pending (default)
	SourceContext string // "ai_extraction", "paste_mode", "manual"; "" = all sources
	Limit         int
	Offset        int
}

// EntityRequestFulfiller creates the actual catalog entity from an approved
// request's payload. It is intentionally narrow — only the create methods the
// approve path needs — so the decide handler doesn't depend on the full
// catalog service interfaces. The concrete implementation lives in the service
// container, composing the per-entity catalog services.
//
// Show is deliberately absent: its catalog create contract requires
// associations (≥1 venue + ≥1 artist with positions) that the entity_request
// payload does not carry, so fulfilling a show needs an admin-supplied
// association-resolution step (tracked as a follow-up to PSY-998); the decide
// handler returns a typed "fulfillment unsupported" error for it.
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
}
