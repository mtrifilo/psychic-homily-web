package community

import (
	"math"
	"time"

	"psychic-homily-backend/internal/models/auth"
)

// Request status constants
const (
	RequestStatusPending    = "pending"
	RequestStatusInProgress = "in_progress"
	RequestStatusFulfilled  = "fulfilled"
	RequestStatusRejected   = "rejected"
	RequestStatusCancelled  = "cancelled"
)

// Request entity type constants (reuse collection entity types)
const (
	RequestEntityArtist   = "artist"
	RequestEntityRelease  = "release"
	RequestEntityLabel    = "label"
	RequestEntityShow     = "show"
	RequestEntityVenue    = "venue"
	RequestEntityFestival = "festival"
)

// Request represents a community request for missing or incomplete data.
type Request struct {
	ID                uint       `json:"id" gorm:"primaryKey"`
	Title             string     `json:"title" gorm:"column:title;not null"`
	Description       *string    `json:"description,omitempty" gorm:"column:description"`
	EntityType        string     `json:"entity_type" gorm:"column:entity_type;not null;size:50"`
	RequestedEntityID *uint      `json:"requested_entity_id,omitempty" gorm:"column:requested_entity_id"`
	Status            string     `json:"status" gorm:"column:status;not null;default:'pending';size:20"`
	RequesterID       uint       `json:"requester_id" gorm:"column:requester_id;not null"`
	FulfillerID       *uint      `json:"fulfiller_id,omitempty" gorm:"column:fulfiller_id"`
	VoteScore         int        `json:"vote_score" gorm:"column:vote_score;not null;default:0"`
	Upvotes           int        `json:"upvotes" gorm:"column:upvotes;not null;default:0"`
	Downvotes         int        `json:"downvotes" gorm:"column:downvotes;not null;default:0"`
	FulfilledAt       *time.Time `json:"fulfilled_at,omitempty" gorm:"column:fulfilled_at"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`

	// Relationships
	Requester auth.User  `json:"-" gorm:"foreignKey:RequesterID"`
	Fulfiller *auth.User `json:"-" gorm:"foreignKey:FulfillerID"`
}

// TableName specifies the table name for Request
func (Request) TableName() string { return "requests" }

// RequestVote represents a user's vote on a request.
type RequestVote struct {
	RequestID uint      `json:"request_id" gorm:"column:request_id;primaryKey"`
	UserID    uint      `json:"user_id" gorm:"column:user_id;primaryKey"`
	Vote      int       `json:"vote" gorm:"column:vote;not null"`
	CreatedAt time.Time `json:"created_at"`

	// Relationships
	Request Request   `json:"-" gorm:"foreignKey:RequestID"`
	User    auth.User `json:"-" gorm:"foreignKey:UserID"`
}

// TableName specifies the table name for RequestVote
func (RequestVote) TableName() string { return "request_votes" }

// WilsonScore computes the Wilson score lower bound for ranking.
// Uses 90% confidence interval (z = 1.281728756502709).
func (r *Request) WilsonScore() float64 {
	n := float64(r.Upvotes + r.Downvotes)
	if n == 0 {
		return 0
	}
	z := 1.281728756502709
	phat := float64(r.Upvotes) / n
	return (phat + z*z/(2*n) - z*math.Sqrt((phat*(1-phat)+z*z/(4*n))/n)) / (1 + z*z/n)
}
