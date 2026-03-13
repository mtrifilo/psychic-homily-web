package contracts

import (
	"time"

	"psychic-homily-backend/internal/models"
)

// ──────────────────────────────────────────────
// Request types
// ──────────────────────────────────────────────

// RequestResponse represents a request returned to clients.
type RequestResponse struct {
	ID                uint       `json:"id"`
	Title             string     `json:"title"`
	Description       *string    `json:"description,omitempty"`
	EntityType        string     `json:"entity_type"`
	RequestedEntityID *uint      `json:"requested_entity_id,omitempty"`
	Status            string     `json:"status"`
	RequesterID       uint       `json:"requester_id"`
	RequesterName     string     `json:"requester_name"`
	FulfillerID       *uint      `json:"fulfiller_id,omitempty"`
	FulfillerName     string     `json:"fulfiller_name,omitempty"`
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
	CreateRequest(userID uint, title, description, entityType string, requestedEntityID *uint) (*models.Request, error)
	GetRequest(requestID uint) (*models.Request, error)
	ListRequests(status string, entityType string, sortBy string, limit, offset int) ([]models.Request, int64, error)
	UpdateRequest(requestID, userID uint, title, description *string) (*models.Request, error)
	DeleteRequest(requestID, userID uint, isAdmin bool) error
	Vote(requestID, userID uint, isUpvote bool) error
	RemoveVote(requestID, userID uint) error
	FulfillRequest(requestID, fulfillerID uint, fulfilledEntityID *uint) error
	CloseRequest(requestID, userID uint, isAdmin bool) error
	GetUserVote(requestID, userID uint) (*models.RequestVote, error)
}
