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
// ONE COLUMN IS NOT A JSON ARRAY. The `kind` columns are `[]uint8`, and Go's
// encoding/json writes any `[]uint8` as a BASE64 STRING, not a list of numbers
// — so on the wire (and in the generated TypeScript) `nodes.kind` and
// `edges.kind` are strings, while every other column is an array. Decoding one
// byte per element is what a client must do:
//
//	const kinds = Uint8Array.from(atob(overview.nodes.kind), (c) => c.charCodeAt(0));
//
// The decoded length is NodeCount (nodes) or 2*EdgeCount (edge slots), so the
// columnar invariant holds after decoding. This is called out here and in each
// field's `doc` tag because the asymmetry is invisible in the Go type and would
// otherwise be discovered by a client indexing a string.
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
//
// A client should not branch on these — the Rank column means the same thing
// either way. They exist so an operator can tell which tiering a given map
// shipped with, since the two differ in fidelity and not in shape.
const (
	// GraphOverviewRankBetweenness means Rank came from exact betweenness
	// centrality over every node.
	GraphOverviewRankBetweenness = "betweenness"
	// GraphOverviewRankBetweennessSampled means the graph was large enough that
	// betweenness was estimated from a fixed set of pivot sources instead of
	// every node. The ordering is approximate in the tail.
	GraphOverviewRankBetweennessSampled = "betweenness_sampled"
)

// Bits in GraphOverviewNodes.Flags. These are the map's two "worth clicking"
// signals, and both are point-in-time facts about the night the map was built —
// an upcoming show becomes a past show without the map changing, so a client
// should treat the flag as a hint rather than a promise.
const (
	// GraphOverviewFlagPlayableAudio marks an artist with something to play.
	GraphOverviewFlagPlayableAudio uint8 = 1 << 0
	// GraphOverviewFlagUpcomingShow marks an artist with an approved show on or
	// after the build date.
	GraphOverviewFlagUpcomingShow uint8 = 1 << 1
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
	// Kind is one of the GraphOverviewNode* constants, one byte per node.
	// BASE64 ON THE WIRE — see the package comment above. Decoded length is
	// NodeCount.
	Kind []uint8 `json:"kind" doc:"Base64-encoded byte per node: 0 = artist, 1 = label hub. Decode to a Uint8Array of length node_count."`
	// Name and Slug are the node's display name and its URL slug. A slug is
	// never empty: an unlinkable node is not emitted.
	Name []string `json:"name"`
	Slug []string `json:"slug"`
	// HubCity is a LABEL HUB's home city, and is empty for everything else.
	//
	// The name says "hub" because the scope is the point: it is empty at every
	// ARTIST index — not because artists have no city, but because the map does
	// not carry theirs — and empty at a hub whose label has no city on file.
	// Reading it as "this node's location" would silently claim that most of
	// the catalog is from nowhere.
	//
	// City ONLY: no state, no country, no "City, ST" composition. That is the
	// locked caption rule for this surface, and composing a fallback here would
	// put the presentation decision in the payload where a client cannot undo
	// it.
	//
	// OPTIONAL, like Appear. A snapshot written before this column existed
	// carries no `hub_city` at all — the payload version is unchanged, because
	// an absent additive column costs a client its captions and nothing else,
	// whereas a version bump would 503 the whole map until the next nightly
	// build. Length is NodeCount whenever the column is present.
	HubCity []string `json:"hub_city,omitempty" doc:"Label hub's home city, empty at every artist index and at a hub with no city on file. Absent entirely on a snapshot built before the column existed."`
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
	// Flags is a per-node bitfield of the GraphOverviewFlag* constants — the
	// two "is this dot worth clicking" affordances, packed into one byte
	// because they repeat once per node.
	// BASE64 ON THE WIRE, like the kind columns — see the package comment.
	Flags []uint8 `json:"flags" doc:"Base64-encoded bitfield per node: bit 0 = has playable audio, bit 1 = has an upcoming show. Decode to a Uint8Array of length node_count."`
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
	// Kind is one of the GraphOverviewEdge* constants, one byte per slot.
	// BASE64 ON THE WIRE — see the package comment above. Decoded length is
	// 2*EdgeCount, matching Targets.
	Kind []uint8 `json:"kind" doc:"Base64-encoded byte per edge slot: 0 = similarity, 1 = label spoke. Decode to a Uint8Array of length 2 * edge_count, index-aligned with targets."`
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
