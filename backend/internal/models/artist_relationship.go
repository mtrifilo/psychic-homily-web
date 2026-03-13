package models

import (
	"encoding/json"
	"math"
	"time"
)

// Relationship type constants
const (
	RelationshipTypeSimilar    = "similar"
	RelationshipTypeSharedBills = "shared_bills"
	RelationshipTypeSharedLabel = "shared_label"
	RelationshipTypeSideProject = "side_project"
	RelationshipTypeMemberOf    = "member_of"
)

// ArtistRelationship represents a relationship between two artists.
// The composite primary key is (source_artist_id, target_artist_id, relationship_type).
// A CHECK constraint ensures source_artist_id < target_artist_id (canonical ordering).
type ArtistRelationship struct {
	SourceArtistID   uint              `json:"source_artist_id" gorm:"column:source_artist_id;primaryKey"`
	TargetArtistID   uint              `json:"target_artist_id" gorm:"column:target_artist_id;primaryKey"`
	RelationshipType string            `json:"relationship_type" gorm:"column:relationship_type;primaryKey;size:20"`
	Score            float32           `json:"score" gorm:"column:score;not null;default:0"`
	AutoDerived      bool              `json:"auto_derived" gorm:"column:auto_derived;not null;default:false"`
	Detail           *json.RawMessage  `json:"detail,omitempty" gorm:"column:detail;type:jsonb"`
	CreatedAt        time.Time         `json:"created_at" gorm:"not null"`
	UpdatedAt        time.Time         `json:"updated_at" gorm:"not null"`

	// Relationships
	SourceArtist Artist `json:"-" gorm:"foreignKey:SourceArtistID"`
	TargetArtist Artist `json:"-" gorm:"foreignKey:TargetArtistID"`
}

// TableName specifies the table name for ArtistRelationship
func (ArtistRelationship) TableName() string { return "artist_relationships" }

// WilsonScore computes the Wilson score lower bound for ranking voted relationships.
// Uses 90% confidence interval (z = 1.281728756502709).
// upvotes and downvotes are passed as parameters since they are tracked via
// the artist_relationship_votes table rather than denormalized on this struct.
func (r *ArtistRelationship) WilsonScore(upvotes, downvotes int) float64 {
	n := float64(upvotes + downvotes)
	if n == 0 {
		return 0
	}
	z := 1.281728756502709
	phat := float64(upvotes) / n
	return (phat + z*z/(2*n) - z*math.Sqrt((phat*(1-phat)+z*z/(4*n))/n)) / (1 + z*z/n)
}

// ArtistRelationshipVote represents a user's vote on an artist relationship.
// The composite primary key is (source_artist_id, target_artist_id, relationship_type, user_id).
type ArtistRelationshipVote struct {
	SourceArtistID   uint      `json:"source_artist_id" gorm:"column:source_artist_id;primaryKey"`
	TargetArtistID   uint      `json:"target_artist_id" gorm:"column:target_artist_id;primaryKey"`
	RelationshipType string    `json:"relationship_type" gorm:"column:relationship_type;primaryKey;size:20"`
	UserID           uint      `json:"user_id" gorm:"column:user_id;primaryKey"`
	Direction        int16     `json:"direction" gorm:"column:direction;not null"`
	CreatedAt        time.Time `json:"created_at" gorm:"not null"`

	// Relationships
	User User `json:"-" gorm:"foreignKey:UserID"`
}

// TableName specifies the table name for ArtistRelationshipVote
func (ArtistRelationshipVote) TableName() string { return "artist_relationship_votes" }

// CanonicalOrder returns (a, b) such that a < b, ensuring canonical ordering
// for the artist_relationships table's CHECK constraint (source_artist_id < target_artist_id).
func CanonicalOrder(a, b uint) (uint, uint) {
	if a < b {
		return a, b
	}
	return b, a
}
