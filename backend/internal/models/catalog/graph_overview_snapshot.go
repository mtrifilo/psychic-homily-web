package catalog

import (
	"encoding/json"
	"time"
)

// GraphOverviewSnapshot is one nightly build of the precomputed "Map of the
// Scene" layout.
//
// The table is APPEND-ONLY: the nightly job inserts a new row and prunes older
// ones, so a row is immutable once written and the previous snapshot survives
// any failure of the next build. Readers take the newest row in a single
// statement.
type GraphOverviewSnapshot struct {
	ID uint `gorm:"column:id;primaryKey"`

	// Payload is the complete serving body (contracts.GraphOverview), stored
	// pre-assembled so GET /graph/overview is one row read.
	Payload json.RawMessage `gorm:"column:payload;type:jsonb"`

	// Layout is the next epoch's warm-start input: full-precision positions
	// keyed by node id. Producer state, never served.
	Layout json.RawMessage `gorm:"column:layout;type:jsonb"`

	NodeCount    int `gorm:"column:node_count"`
	EdgeCount    int `gorm:"column:edge_count"`
	IsolateCount int `gorm:"column:isolate_count"`

	// StartingPointArtistIDs is the top slice of this build's centrality
	// ranking — artist ids, most central first — for the /graph fallback
	// hero's suggestions. Ids only: the display name and slug are read from
	// `artists` at request time so a suggestion can never promise an artist
	// that was renamed or removed since the build. Nil is legal and means
	// "nothing to suggest"; see the column's migration comment.
	StartingPointArtistIDs json.RawMessage `gorm:"column:starting_point_artist_ids;type:jsonb"`

	// ContentHash identifies the snapshot and is served as the response ETag.
	ContentHash string `gorm:"column:content_hash"`

	// StructureKey digests the node and edge sets the layout was computed from.
	// A matching key on the next build means the shape is unchanged and the
	// stored positions are reused instead of re-relaxed.
	StructureKey string `gorm:"column:structure_key"`

	ComputedAt time.Time `gorm:"column:computed_at"`
}

// TableName specifies the table name for GraphOverviewSnapshot.
func (GraphOverviewSnapshot) TableName() string {
	return "graph_overview_snapshots"
}
