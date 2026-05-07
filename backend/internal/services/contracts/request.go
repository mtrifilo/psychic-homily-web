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
	Status            string  `json:"status"`
	RequesterID       uint    `json:"requester_id"`
	RequesterName     string  `json:"requester_name"`
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

// RequestServiceInterface defines the contract for community request operations.
type RequestServiceInterface interface {
	CreateRequest(userID uint, title, description, entityType string, requestedEntityID *uint) (*communitym.Request, error)
	GetRequest(requestID uint) (*communitym.Request, error)
	ListRequests(status string, entityType string, sortBy string, limit, offset int) ([]communitym.Request, int64, error)
	UpdateRequest(requestID, userID uint, title, description *string) (*communitym.Request, error)
	DeleteRequest(requestID, userID uint, isAdmin bool) error
	Vote(requestID, userID uint, isUpvote bool) error
	RemoveVote(requestID, userID uint) error
	FulfillRequest(requestID, fulfillerID uint, fulfilledEntityID *uint) error
	CloseRequest(requestID, userID uint, isAdmin bool) error
	GetUserVote(requestID, userID uint) (*communitym.RequestVote, error)
}
