package contracts

import "time"

// ──────────────────────────────────────────────
// Graph overview ("Map of the Scene")
// ──────────────────────────────────────────────
//
// GraphOverview is the precomputed catalog-wide map served by
// GET /graph/overview. It ships POSITIONS, NOT PHYSICS: a client-side force
// warmup at this node count costs seconds of blocked main thread, so the
// nightly job runs the layout once and every visitor renders the same map.
//
// The payload is COLUMNAR — parallel arrays indexed by node position rather
// than an array of objects. Two reasons: repeated JSON keys dominate an
// object-per-node encoding at this scale, and the client feeds these arrays
// straight into typed arrays without a per-node transform. Every `Nodes.*`
// slice has length NodeCount and index i describes the same node in all of
// them.
//
// The same struct is BOTH the stored snapshot and the response body. That is
// deliberate: a separate serving type would be a second place for the schema to
// drift, and the payload is stored pre-assembled precisely so serving does no
// assembly.

// GraphOverviewVersion is the payload schema version. Bump it when a field's
// meaning changes so a client can refuse a shape it does not understand; a
// stale snapshot written by an older build keeps its own version.
const GraphOverviewVersion = 1

// Node kinds in GraphOverviewNodes.Kind. They mirror the scene graph's node
// kinds, one byte wide because the value repeats once per node.
const (
	// GraphOverviewNodeArtist marks an artist node; its ID is an artist ID.
	GraphOverviewNodeArtist uint8 = 0
	// GraphOverviewNodeLabelHub marks a label hub; its ID is offset into the
	// reserved hub range (see labelHubNodeIDOffset in services/catalog).
	GraphOverviewNodeLabelHub uint8 = 1
)

// Edge kinds in GraphOverviewEdges.Kind.
const (
	// GraphOverviewEdgeSimilarity is a stored artist-to-artist relationship
	// that survived the disparity backbone.
	GraphOverviewEdgeSimilarity uint8 = 0
	// GraphOverviewEdgeLabelSpoke is a label hub's membership spoke.
	GraphOverviewEdgeLabelSpoke uint8 = 1
)

// Rank metrics reported in GraphOverview.RankMetric.
const (
	// GraphOverviewRankBetweenness means Rank came from betweenness centrality.
	GraphOverviewRankBetweenness = "betweenness"
	// GraphOverviewRankDegree means the betweenness computation was skipped and
	// Rank is degree order. A client should not change behaviour on this — it
	// exists so an operator can tell which tiering a given map shipped with.
	GraphOverviewRankDegree = "degree"
)

// GraphOverviewHullConvex marks region hulls built as padded convex hulls.
const GraphOverviewHullConvex = "convex"

// GraphOverviewCoordinateScale is the full-scale quantized coordinate. Node and
// hull coordinates are int16 in [-32767, 32767]; multiply by Extent/32767 to
// recover world units. int16 at this scale resolves ~1/16000 of the map's half
// width, roughly two orders of magnitude finer than a screen pixel on the
// widest realistic viewport, so quantization is invisible.
const GraphOverviewCoordinateScale = 32767

// GraphOverviewNodes holds the columnar node arrays. Every slice has length
// GraphOverview.NodeCount.
type GraphOverviewNodes struct {
	// ID is the entity id. Pair it with Kind before treating it as a database
	// key — artist ids and label-hub ids share this one numeric space.
	ID []uint `json:"id"`
	// Kind is one of the GraphOverviewNode* constants.
	Kind []uint8 `json:"kind"`
	// Name and Slug are the node's display name and its URL slug. A slug is
	// never empty: an unlinkable node is not emitted.
	Name []string `json:"name"`
	Slug []string `json:"slug"`
	// X and Y are quantized positions; see GraphOverviewCoordinateScale.
	X []int16 `json:"x"`
	Y []int16 `json:"y"`
	// Community is the Leiden community id, or -1 for a node that belongs to
	// none (every label hub, plus any artist the partition did not assign).
	Community []int32 `json:"community"`
	// Degree is the node's degree in THIS map's edge set, not in the catalog.
	Degree []int32 `json:"degree"`
	// Rank orders nodes for label tiering, 0 = most central. Which centrality
	// produced it is reported in GraphOverview.RankMetric.
	Rank []int32 `json:"rank"`
	// Appear is when the node entered the catalog, in seconds after
	// GraphOverview.Epoch, for the "grow the map over time" scrub. Clock is
	// the entity's created_at and its earliest show date — never a
	// relationship's created_at, which stamps a derive run rather than an
	// event.
	Appear []int32 `json:"appear"`
}

// GraphOverviewEdges is the edge set in CSR (compressed sparse row) form.
//
// CSR IS THE ONLY EDGE ENCODING — there is no separate edge list. Targets holds
// BOTH directions of every edge (so Offsets[i]..Offsets[i+1] is node i's full
// neighbourhood, which is what makes hover and expand O(degree)), which means a
// renderer that draws every slot draws every edge twice. Draw a slot only when
// its target index is greater than its source index and each edge is drawn
// exactly once.
type GraphOverviewEdges struct {
	// Offsets has length NodeCount+1; node i's neighbour slots are
	// Targets[Offsets[i]:Offsets[i+1]].
	Offsets []int32 `json:"offsets"`
	// Targets has length 2*EdgeCount: neighbour node indexes.
	Targets []int32 `json:"targets"`
	// Kind is one of the GraphOverviewEdge* constants, per slot.
	Kind []uint8 `json:"kind"`
	// Appear is the edge's appearance time in seconds after
	// GraphOverview.Epoch, per slot. An edge cannot predate either endpoint, so
	// it is the later of the two.
	Appear []int32 `json:"appear"`
}

// GraphOverviewRegion is one community's labelled area on the map.
type GraphOverviewRegion struct {
	// Community matches GraphOverviewNodes.Community.
	Community int32 `json:"community"`
	// Label is the community's display name ("Around {artist}"), anchored on
	// its highest-strength member by the Leiden compute.
	Label string `json:"label"`
	// MemberCount counts the community's members ON THIS MAP, which can be
	// fewer than artist_communities.member_count: an artist whose every edge
	// was pruned by the backbone is not drawn.
	MemberCount int `json:"member_count"`
	// Hull is the region outline as a closed polygon in the same quantized
	// coordinate space as node X/Y, in counter-clockwise order and without a
	// repeated final point. Empty when the region has too few members to
	// enclose an area.
	Hull [][2]int16 `json:"hull"`
}

// GraphOverview is the whole precomputed map.
type GraphOverview struct {
	// Version is GraphOverviewVersion at build time.
	Version int `json:"version"`
	// LastMapped is when the snapshot was built.
	LastMapped time.Time `json:"last_mapped"`
	// Epoch is the origin for every Appear value.
	Epoch time.Time `json:"epoch"`
	// Extent is the half-width of the map in world units — the largest absolute
	// coordinate before quantization. World units are only meaningful relative
	// to each other, so a client normally ignores this and works in quantized
	// space.
	Extent float64 `json:"extent"`

	NodeCount int `json:"node_count"`
	EdgeCount int `json:"edge_count"`
	// IsolateCount counts catalog artists that are NOT on the map because they
	// have no edge surviving the backbone. They are reported as a number rather
	// than drawn: a few thousand unconnected dots would double the payload to
	// say "nothing is known about these yet".
	IsolateCount int `json:"isolate_count"`

	// RankMetric is one of the GraphOverviewRank* constants.
	RankMetric string `json:"rank_metric"`
	// HullKind describes how Region.Hull was built.
	HullKind string `json:"hull_kind"`

	Nodes   GraphOverviewNodes    `json:"nodes"`
	Edges   GraphOverviewEdges    `json:"edges"`
	Regions []GraphOverviewRegion `json:"regions"`
}

// GraphOverviewServiceInterface is the read side of the overview map. The
// nightly build lives on the radio/catalog compute path and is deliberately not
// on this interface — nothing over HTTP may trigger a rebuild.
type GraphOverviewServiceInterface interface {
	// GetGraphOverview returns the newest snapshot and its ETag. It returns a
	// nil payload with a nil error when no snapshot has been built yet (a cold
	// database before the first nightly run), which callers surface as a 503
	// rather than an empty map.
	GetGraphOverview() (*GraphOverview, string, error)
}
