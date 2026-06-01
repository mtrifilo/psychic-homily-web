package contracts

import (
	communitym "psychic-homily-backend/internal/models/community"
	"time"
)

// ──────────────────────────────────────────────
// Request types
// ──────────────────────────────────────────────

// RequestResponse represents a request returned to clients.
type RequestResponse struct {
	ID                uint    `json:"id"`
	Title             string  `json:"title"`
	Description       *string `json:"description,omitempty"`
	EntityType        string  `json:"entity_type"`
	RequestedEntityID *uint   `json:"requested_entity_id,omitempty"`
	// RequestedEntitySlug / RequestedEntityName resolve RequestedEntityID to
	// the linkable slug + display name of the referenced entity. Populated
	// only by the single-request detail path (GetRequestHandler) when
	// RequestedEntityID is set and the row still exists — list responses
	// leave them nil to avoid an N+1 resolve per row. PSY-917.
	//
	// Why this is needed: entity detail pages route by SLUG
	// (/artists/<slug>), not by numeric ID, and the requests table only
	// stores the ID. Without the resolved slug the "View proposed {entity}"
	// link in the fulfillment review panel can't be built. Slug is a pointer
	// because catalog rows can have a NULL slug — when it's nil the frontend
	// omits the link rather than emitting a broken /artists/<id> href.
	RequestedEntitySlug *string `json:"requested_entity_slug,omitempty"`
	RequestedEntityName *string `json:"requested_entity_name,omitempty"`
	Status              string  `json:"status"`
	RequesterID         uint    `json:"requester_id"`
	RequesterName       string  `json:"requester_name"`
	// RequesterUsername is the requester's username when set — pointer so
	// the JSON encodes null (not "") for accounts that never set a username.
	// Frontend uses this to render the byline as a link to /users/:username
	// when non-nil; nil renders as plain text. Mirrors the same shape PSY-353
	// standardized for collection contributor attribution. PSY-619.
	RequesterUsername *string    `json:"requester_username"`
	FulfillerID       *uint      `json:"fulfiller_id,omitempty"`
	FulfillerName     string     `json:"fulfiller_name,omitempty"`
	FulfillerUsername *string    `json:"fulfiller_username"`
	VoteScore         int        `json:"vote_score"`
	Upvotes           int        `json:"upvotes"`
	Downvotes         int        `json:"downvotes"`
	WilsonScore       float64    `json:"wilson_score"`
	FulfilledAt       *time.Time `json:"fulfilled_at,omitempty"`
	UserVote          *int       `json:"user_vote,omitempty"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
}

// EntityRef is the resolved, linkable identity of a knowledge-graph entity
// referenced by a request — the slug used to build its detail-page URL plus
// its display name. Returned by ResolveEntityRef. Slug is a pointer so the
// caller can distinguish "row exists but has no slug" (link suppressed) from
// "resolved with a slug" (link rendered). PSY-917.
type EntityRef struct {
	Slug *string
	Name string
}

// RequestServiceInterface defines the contract for community request operations.
//
// Fulfillment is a two-step flow (PSY-748):
//  1. Any authenticated user calls FulfillRequest with the entity they
//     believe satisfies the request. Status moves to pending_fulfillment.
//  2. The original requester or an admin calls ApproveFulfillment (→ fulfilled)
//     or RejectFulfillment (→ pending, clearing fulfiller/entity).
//
// This makes contribution community-open while keeping the original
// requester in the loop so they can't have their request silently hijacked.
type RequestServiceInterface interface {
	CreateRequest(userID uint, title, description, entityType string, requestedEntityID *uint) (*communitym.Request, error)
	GetRequest(requestID uint) (*communitym.Request, error)
	ListRequests(status string, entityType string, sortBy string, limit, offset int) ([]communitym.Request, int64, error)
	UpdateRequest(requestID, userID uint, title, description *string) (*communitym.Request, error)
	DeleteRequest(requestID, userID uint, isAdmin bool) error
	Vote(requestID, userID uint, isUpvote bool) error
	RemoveVote(requestID, userID uint) error
	// FulfillRequest moves a pending/in_progress request into pending_fulfillment
	// state and records the proposed entity. Open to any authenticated user;
	// entity-type validation enforced. Approval by requester or admin still required.
	FulfillRequest(requestID, fulfillerID uint, fulfilledEntityID *uint) error
	// ApproveFulfillment finalizes a pending_fulfillment request as fulfilled.
	// Only the original requester or an admin may approve.
	ApproveFulfillment(requestID, userID uint, isAdmin bool) error
	// RejectFulfillment returns a pending_fulfillment request to pending,
	// clearing the fulfiller and proposed entity link. Only the original
	// requester or an admin may reject.
	RejectFulfillment(requestID, userID uint, isAdmin bool) error
	// NotifyRequesterFulfillmentProposed writes an in-app notification to the
	// request's owner that a fulfillment has been proposed and awaits their
	// approval. No-op when the fulfiller IS the requester (self-fulfill) or
	// when requesterID is 0. Fire-and-forget: the error is for logging only.
	NotifyRequesterFulfillmentProposed(requestID, requesterID, fulfillerID uint) error
	CloseRequest(requestID, userID uint, isAdmin bool) error
	GetUserVote(requestID, userID uint) (*communitym.RequestVote, error)
	// ResolveEntityRef looks up the slug + display name of the entity
	// referenced by a request (entityType + entityID). Returns nil (no
	// error) when the row doesn't exist, so a stale RequestedEntityID
	// degrades to "no link" rather than a 500. PSY-917.
	ResolveEntityRef(entityType string, entityID uint) (*EntityRef, error)
}
