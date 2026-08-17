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
//
// DO NOT BUMP FOR AN ADDITIVE FIELD. A bump costs two things, and both are
// worse than the absent field it would announce:
//
//   - The read path refuses any snapshot whose version is not this one
//     (GetGraphOverview), so the map 503s from the deploy until the next
//     nightly build commits.
//   - The nightly build refuses to warm-start across a version change
//     (previousOverviewLayout), so that build cold-starts the layout and
//     reshuffles every dot for every reader.
//
// A client reading an older snapshot that simply lacks a new column loses that
// column's feature for one nightly cycle and nothing else. Bump only when an
// EXISTING field's meaning changes, or when node ids are re-scoped — the cases
// where reading the old payload would be silently wrong rather than merely
// incomplete.
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
	// HubCity, HubState and HubCountry are a LABEL HUB's home location, and are
	// empty for everything else.
	//
	// The names say "hub" because the scope is the point: they are empty at
	// every ARTIST index — not because artists have no location, but because
	// the map does not carry theirs — and empty at a hub whose label has that
	// part missing. Reading them as "this node's location" would silently claim
	// that most of the catalog is from nowhere.
	//
	// THREE COLUMNS, NOT A COMPOSED CAPTION (PSY-1792). The map captions a hub
	// with the same city -> state -> country fallback every other surface uses
	// (`/scenes`, artist and venue pages), so a label known only as "England"
	// captions there too. That rule is the site-wide PSY-558/780 location rule,
	// and it is implemented ONCE, on the client, as `formatLocation` /
	// `labelHubHomeCaption`. Composing the caption here instead would mean
	// porting that rule into Go — a second implementation of a rule whose whole
	// value is that there is only one — so the wire carries the PARTS and the
	// client composes. This supersedes the PSY-1721 "city only" caption lock,
	// which the payload previously enforced by carrying nothing else.
	//
	// TRIMMED. A present value is drawable text: the builder normalizes
	// whitespace, so "  " arrives as "" rather than as a blank caption part.
	//
	// OPTIONAL, like Appear. A snapshot written before a column existed carries
	// it not at all — see GraphOverviewVersion for why that is preferable to a
	// version bump. In particular a snapshot built before PSY-1792 has
	// `hub_city` and neither of the other two, which degrades to exactly the
	// old city-only caption for one nightly cycle. Length is NodeCount whenever
	// present, and each column is length-checked independently.
	HubCity    []string `json:"hub_city,omitempty" doc:"Label hub's home city, trimmed; empty at every artist index and at a hub with no city on file. Absent entirely on a snapshot built before the column existed."`
	HubState   []string `json:"hub_state,omitempty" doc:"Label hub's home state, trimmed; empty at every artist index and at a hub with no state on file. Absent entirely on a snapshot built before the column existed."`
	HubCountry []string `json:"hub_country,omitempty" doc:"Label hub's home country, trimmed; empty at every artist index and at a hub with no country on file. Absent entirely on a snapshot built before the column existed."`
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

// ──────────────────────────────────────────────
// Graph starting points
// ──────────────────────────────────────────────
//
// The handful of well-connected artists the /graph fallback hero offers as
// "try searching for …". Ranked by the nightly build, hydrated from the live
// catalog on every read.
//
// WHY THIS IS NOT PART OF GraphOverview. The hero exists precisely for the
// states where the overview is NOT available — a catalog before its first
// nightly build, a payload version the running code refuses, a decode failure —
// and on every phone, where the map's canvas is a smear and the hero renders
// above its list. A suggestion list carried inside the map payload would be
// missing in exactly the three states that need it most, which is why this is a
// separate resource with its own read path and no version gate.

// GraphStartingPoint is one suggestion: an artist anchor the client can center
// on directly. Every field is populated from the `artists` row that is live AT
// READ TIME, not from the snapshot — the ranking is a night old, the identity
// never is.
type GraphStartingPoint struct {
	ArtistID   uint   `json:"artist_id"`
	ArtistName string `json:"artist_name"`
	ArtistSlug string `json:"artist_slug"`
}

// GraphStartingPointsResponse is the body of GET /graph/starting-points.
//
// AN EMPTY LIST IS A 200, not a 503. Unlike the map — where an empty payload is
// indistinguishable from "the scene is empty" and would be cached as truth — an
// empty suggestion list has a graceful client answer that already exists: fall
// back to a random catalog artist. Failing the request instead would turn the
// ordinary cold-start state into an error the client has to special-case.
type GraphStartingPointsResponse struct {
	// Artists is ordered most-connected first. It is never nil (an empty list
	// encodes as `[]`, not `null`) and is capped by the nightly build.
	Artists []GraphStartingPoint `json:"artists" doc:"Well-connected artists to suggest as a starting point, most connected first. Empty before the first nightly build."`
}

// GraphOverviewServiceInterface is the READ side of everything the nightly
// overview build publishes: the map itself, and the starting suggestions
// derived from the same centrality pass. The build lives on the radio/catalog
// compute path and is deliberately not on this interface — nothing over HTTP
// may trigger a rebuild.
type GraphOverviewServiceInterface interface {
	// GetGraphOverview returns the newest snapshot and its ETag. It returns a
	// nil payload with a nil error when no snapshot has been built yet (a cold
	// database before the first nightly run), which callers surface as a 503
	// rather than an empty map.
	GetGraphOverview() (*GraphOverview, string, error)

	// GetGraphStartingPoints returns the nightly-ranked starting suggestions,
	// resolved against the live catalog. An empty slice with a nil error means
	// there is nothing to suggest yet (no snapshot, or none of the ranked
	// artists still exist) — a normal state, not a failure.
	GetGraphStartingPoints() ([]GraphStartingPoint, error)
}
