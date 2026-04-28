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

// ArtistGraph represents the relationship graph for an artist.
type ArtistGraph struct {
	Center    ArtistGraphNode   `json:"center"`
	Nodes     []ArtistGraphNode `json:"nodes"`
	Links     []ArtistGraphLink `json:"links"`
	UserVotes map[string]string `json:"user_votes,omitempty"` // "sourceID-targetID-type" -> "up"/"down"
}

// ArtistGraphNode represents a node in the artist relationship graph.
type ArtistGraphNode struct {
	ID                uint   `json:"id"`
	Name              string `json:"name"`
	Slug              string `json:"slug"`
	City              string `json:"city,omitempty"`
	State             string `json:"state,omitempty"`
	ImageURL          string `json:"image_url,omitempty"`
	UpcomingShowCount int    `json:"upcoming_show_count"`
}

// ArtistGraphLink represents an edge in the artist relationship graph.
type ArtistGraphLink struct {
	SourceID  uint    `json:"source_id"`
	TargetID  uint    `json:"target_id"`
	Type      string  `json:"type"`
	Score     float64 `json:"score"`
	VotesUp   int     `json:"votes_up"`
	VotesDown int     `json:"votes_down"`
	Detail    any     `json:"detail,omitempty"`
}

// ArtistBillComposition aggregates an artist's bill-slot history and top co-bill artists.
// Sourced from show_artists.position + set_type; is_headliner is derived
// (position = 0 OR set_type = 'headliner') — never queried as a column.
type ArtistBillComposition struct {
	Artist           ArtistGraphNode `json:"artist"` // center
	Stats            BillStats       `json:"stats"`
	OpensWith        []BillCoArtist  `json:"opens_with"`        // artists who open for this one (top 10)
	ClosesWith       []BillCoArtist  `json:"closes_with"`       // artists who headline above this one (top 10)
	Graph            ArtistGraph     `json:"graph"`             // mini-graph: scoped to shared_bills edges
	BelowThreshold   bool            `json:"below_threshold"`   // true if Stats.TotalShows < 3
	TimeFilterMonths int             `json:"time_filter_months"` // 0 = all-time
}

// BillStats summarizes how often an artist plays which slot.
type BillStats struct {
	TotalShows     int `json:"total_shows"`
	HeadlinerCount int `json:"headliner_count"`
	OpenerCount    int `json:"opener_count"`
}

// BillCoArtist is one row in the opens-with / closes-with tables.
type BillCoArtist struct {
	Artist      ArtistGraphNode `json:"artist"`
	SharedCount int             `json:"shared_count"`
	LastShared  string          `json:"last_shared"` // ISO date "2026-03-01"
}

// ArtistRelationshipServiceInterface defines the contract for artist relationship operations.
type ArtistRelationshipServiceInterface interface {
	// CRUD
	CreateRelationship(sourceID, targetID uint, relType string, autoDerived bool) (*models.ArtistRelationship, error)
	GetRelationship(artistA, artistB uint, relType string) (*models.ArtistRelationship, error)
	GetRelatedArtists(artistID uint, relType string, limit int) ([]RelatedArtistResponse, error)
	DeleteRelationship(artistA, artistB uint, relType string) error

	// Graph
	GetArtistGraph(artistID uint, types []string, userID uint) (*ArtistGraph, error)
	GetArtistBillComposition(artistID uint, months int) (*ArtistBillComposition, error)

	// Voting (only for non-auto-derived, typically "similar")
	Vote(artistA, artistB uint, relType string, userID uint, isUpvote bool) error
	RemoveVote(artistA, artistB uint, relType string, userID uint) error
	GetUserVote(artistA, artistB uint, relType string, userID uint) (*models.ArtistRelationshipVote, error)

	// Auto-derivation
	DeriveSharedBills(minShows int) (int64, error)
	DeriveSharedLabels(minLabels int) (int64, error)
}
