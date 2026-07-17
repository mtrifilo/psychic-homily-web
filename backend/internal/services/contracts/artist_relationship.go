package contracts

import (
	"time"

	catalogm "psychic-homily-backend/internal/models/catalog"
)

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
	// HasPlayableAudio reports whether selecting this node opens a playable
	// embed — drives the shared violet playable-marker ring on the ego graph
	// canvas. Mirrors SceneGraphNode's flag.
	HasPlayableAudio bool `json:"has_playable_audio"`
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
	OpensWith        []BillCoArtist  `json:"opens_with"`         // artists who open for this one (top 10)
	ClosesWith       []BillCoArtist  `json:"closes_with"`        // artists who headline above this one (top 10)
	Graph            ArtistGraph     `json:"graph"`              // mini-graph: scoped to shared_bills edges
	BelowThreshold   bool            `json:"below_threshold"`    // true if Stats.TotalShows < 3
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

// ──────────────────────────────────────────────
// Edge provenance (PSY-1335)
// ──────────────────────────────────────────────

// RelationshipProvenanceEntity is one resolvable entity behind a connection
// claim — a shared show, label, festival, or station — with the slug the
// frontend needs to link into the knowledge graph.
type RelationshipProvenanceEntity struct {
	Kind string `json:"kind"` // show | label | festival | station
	ID   uint   `json:"id"`
	Slug string `json:"slug"`
	Name string `json:"name"`
	// Date is the display date: ISO event date for shows, edition year for
	// festivals. Empty for undated kinds (labels, stations).
	Date string `json:"date,omitempty"`
}

// RelationshipProvenanceConnection is one typed connection between the pair
// with the entities that substantiate it. Entity-less types (similar,
// member_of, side_project — score/votes already suffice) carry an empty list.
type RelationshipProvenanceConnection struct {
	Type   string  `json:"type"`
	Score  float64 `json:"score"`
	Detail any     `json:"detail,omitempty"`
	// Entities is capped at RelationshipProvenanceEntityCap; EntityTotal is
	// the uncapped count so the client can disclose "and N more".
	Entities    []RelationshipProvenanceEntity `json:"entities,omitempty"`
	EntityTotal int                            `json:"entity_total,omitempty"`
}

// RelationshipProvenance is the response body for the edge-inspect endpoint.
type RelationshipProvenance struct {
	Connections []RelationshipProvenanceConnection `json:"connections"`
}

// RelationshipProvenanceEntityCap bounds the per-connection entity list.
// Shows between prolific co-billers (and episode-heavy radio pairs, which is
// why radio provenance is station-level only) can be large; the panel is a
// glance surface, not a browse surface.
const RelationshipProvenanceEntityCap = 10

// ArtistRelationshipServiceInterface defines the contract for artist relationship operations.
type ArtistRelationshipServiceInterface interface {
	// CRUD
	CreateRelationship(sourceID, targetID uint, relType string, autoDerived bool) (*catalogm.ArtistRelationship, error)
	GetRelationship(artistA, artistB uint, relType string) (*catalogm.ArtistRelationship, error)
	GetRelatedArtists(artistID uint, relType string, limit int) ([]RelatedArtistResponse, error)
	DeleteRelationship(artistA, artistB uint, relType string) error

	// Graph
	GetArtistGraph(artistID uint, types []string, userID uint) (*ArtistGraph, error)
	GetArtistBillComposition(artistID uint, months int) (*ArtistBillComposition, error)
	// CountRelationshipsByType returns this artist's relationship counts
	// keyed by relationship_type (PSY-1345 graph card). Types with zero
	// rows are absent from the map.
	CountRelationshipsByType(artistID uint) (map[string]int, error)
	GetRelationshipProvenance(artistA, artistB uint) (*RelationshipProvenance, error)

	// Voting (only for non-auto-derived, typically "similar")
	Vote(artistA, artistB uint, relType string, userID uint, isUpvote bool) error
	RemoveVote(artistA, artistB uint, relType string, userID uint) error
	GetUserVote(artistA, artistB uint, relType string, userID uint) (*catalogm.ArtistRelationshipVote, error)

	// Auto-derivation
	DeriveSharedBills(minShows int) (int64, error)
	DeriveSharedLabels(minLabels int) (int64, error)
}

// Production thresholds for the auto-derivation steps, shared by the admin
// trigger endpoint and the scheduled derivation cycle so they can never
// diverge.
//
// PSY-1323: minShows dropped from 2 to 1 — one-off co-bills are the bulk of
// the live co-appearance signal (only 45 shared_bills edges survived
// minShows=2 on stage despite dense show data); the score formula already
// gives a single shared show a low weight (count/10), so noise is bounded by
// weight rather than by exclusion.
const (
	DefaultSharedBillsMinShows   = 1
	DefaultSharedLabelsMinLabels = 1
)

// ============================================================================
// Artist graph card (PSY-1345)
// ============================================================================

// ArtistGraphCard is the node-select summary card for graph surfaces (the
// homepage scene-graph section today; the unshipped /graph Observatory is
// the intended second consumer): who this artist
// is, their next show, labels, radio presence, and how connected they are.
// Purpose-built so a node click costs one small request instead of the full
// artist-detail payload plus three sibling endpoints.
type ArtistGraphCard struct {
	ID    uint    `json:"id"`
	Name  string  `json:"name"`
	Slug  string  `json:"slug"`
	City  *string `json:"city"`
	State *string `json:"state"`
	// Playable audio (PSY-1302) so the node-select card can play a sample
	// without leaving the graph — the same MusicEmbed the Atlas scene preview
	// uses (PSY-1224). Both nil ⟹ the card renders no player. BandcampEmbedURL
	// is the embed-specific column (PSY-1187/1189), distinct from the Bandcamp
	// social profile; Spotify is the artist link, used as the fallback embed.
	BandcampEmbedURL *string `json:"bandcamp_embed_url"`
	Spotify          *string `json:"spotify"`
	// NextShow is nil when the artist has no upcoming approved show.
	NextShow *ArtistGraphCardShow `json:"next_show"`
	// Labels is empty (never nil on the wire) when the artist has none;
	// capped at 5 chips (the artist page carries the full list).
	Labels []ArtistGraphCardLabel `json:"labels"`
	// Radio is nil when the artist has no matched radio plays.
	Radio       *ArtistGraphCardRadio      `json:"radio"`
	Connections ArtistGraphCardConnections `json:"connections"`
}

// ArtistGraphCardShow is the card's next-show line.
type ArtistGraphCardShow struct {
	ID        uint      `json:"id"`
	EventDate time.Time `json:"event_date"`
	VenueName string    `json:"venue_name"`
	VenueCity string    `json:"venue_city"`
	// VenueState may be empty for venues without one (international).
	VenueState string `json:"venue_state"`
	// VenueTimezone is the venue's IANA zone (PSY-985) so clients render the
	// show date in venue-local time, not the viewer's. Nil when unresolved.
	VenueTimezone *string `json:"venue_timezone"`
}

// ArtistGraphCardLabel is one label chip on the card.
type ArtistGraphCardLabel struct {
	Name string `json:"name"`
	Slug string `json:"slug"`
}

// ArtistGraphCardRadio is the "as heard on" line: distinct station names
// ordered by each station's TOTAL play count (summed across its radio
// shows), descending, plus the artist's total matched play count.
type ArtistGraphCardRadio struct {
	Stations  []string `json:"stations"`
	PlayCount int      `json:"play_count"`
}

// ArtistGraphCardConnections carries per-type relationship counts. Members
// folds member_of + side_project (one "people" number for the card);
// SharedLabels is carried separately from Labels above (a count of
// labelmate EDGES, not the artist's own label list).
//
// These are the artist's GLOBAL stored-relationship totals — deliberately
// not the edge counts of whatever capped/filtered graph the card floats
// over (scene graphs draw in-metro edges only; ego graphs cap at 30), so
// the numbers routinely exceed the visible lines. That mismatch is the
// point: the card says how connected the artist is site-wide.
type ArtistGraphCardConnections struct {
	Bills        int `json:"bills"`
	Similar      int `json:"similar"`
	Members      int `json:"members"`
	Radio        int `json:"radio"`
	SharedLabels int `json:"shared_labels"`
}
