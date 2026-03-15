package contracts

import "psychic-homily-backend/internal/models"

// ──────────────────────────────────────────────
// Artist Relationship types
// ──────────────────────────────────────────────

// RelatedArtistResponse represents a related artist with relationship info.
type RelatedArtistResponse struct {
	ArtistID         uint    `json:"artist_id"`
	Name             string  `json:"name"`
	Slug             string  `json:"slug"`
	RelationshipType string  `json:"relationship_type"`
	Score            float32 `json:"score"`
	Upvotes          int     `json:"upvotes"`
	Downvotes        int     `json:"downvotes"`
	WilsonScore      float64 `json:"wilson_score"`
	AutoDerived      bool    `json:"auto_derived"`
	UserVote         *int    `json:"user_vote,omitempty"`
}

// ArtistRelationshipServiceInterface defines the contract for artist relationship operations.
type ArtistRelationshipServiceInterface interface {
	// CRUD
	CreateRelationship(sourceID, targetID uint, relType string, autoDerived bool) (*models.ArtistRelationship, error)
	GetRelationship(artistA, artistB uint, relType string) (*models.ArtistRelationship, error)
	GetRelatedArtists(artistID uint, relType string, limit int) ([]RelatedArtistResponse, error)
	DeleteRelationship(artistA, artistB uint, relType string) error

	// Voting (only for non-auto-derived, typically "similar")
	Vote(artistA, artistB uint, relType string, userID uint, isUpvote bool) error
	RemoveVote(artistA, artistB uint, relType string, userID uint) error
	GetUserVote(artistA, artistB uint, relType string, userID uint) (*models.ArtistRelationshipVote, error)

	// Auto-derivation
	DeriveSharedBills(minShows int) (int64, error)
}
