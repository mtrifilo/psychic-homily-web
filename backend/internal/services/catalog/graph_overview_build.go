package catalog

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"sort"
	"strconv"
	"strings"
	"time"

	"psychic-homily-backend/internal/services/contracts"
)

// ──────────────────────────────────────────────
// Overview payload assembly
// ──────────────────────────────────────────────
//
// overviewBuild owns the map's INDEX SPACE. Everything downstream — the layout
// request, the CSR arrays, the centrality ranks, the region hulls — addresses
// nodes by position in `nodeIDs`, so that ordering is assigned exactly once,
// here, and never re-derived. It is ascending by node id, which puts artists
// first (serial ids in the low thousands) and label hubs last (offset into a
// reserved range), and is reproducible from the node set alone.

// overviewBuild is the indexed graph the payload is assembled from.
type overviewBuild struct {
	nodeIDs       []uint
	nodeKind      []uint8
	nodeName      []string
	nodeSlug      []string
	nodeCommunity []int32
	// nodeHubCity is the hub caption column: a label hub's home city, and ""
	// at every artist index. See contracts.GraphOverviewNodes.HubCity for why
	// it is hub-scoped and city-only.
	nodeHubCity []string
	index       map[uint]int32

	// edge* describe each undirected edge ONCE, in canonical (min, max) index
	// order. They are the input to the CSR expansion, not part of the payload.
	edgeSrc  []int32
	edgeDst  []int32
	edgeKind []uint8

	// adjacency[i] lists node i's incident slots. Both directions of every
	// edge appear, which is what makes a neighbourhood lookup O(degree).
	adjacency [][]overviewAdjSlot

	// neighbors is adjacency projected to target indexes only — the shape the
	// seeding and centrality passes want.
	neighbors [][]int32

	// spokeArtists maps a hub node index to its roster's node indexes, so a
	// hub can inherit an appearance time from the artists it connects.
	spokeArtists map[int32][]int32
}

// overviewAdjSlot is one directed half of an undirected edge.
type overviewAdjSlot struct {
	target int32
	edge   int32
}

// newOverviewBuild assigns the index space and builds the adjacency.
//
// artistIDs must already be ascending and must be EXACTLY the set passed to the
// label hub builder — the spokes below are minted against that set, and a
// mismatch would index a node the payload does not contain.
func newOverviewBuild(
	artistIDs []uint,
	artistMeta map[uint]overviewArtist,
	hubs labelHubs,
	backbone []overviewEdge,
) *overviewBuild {
	hubNodes := make([]contracts.SceneGraphNode, len(hubs.Nodes))
	copy(hubNodes, hubs.Nodes)
	sort.SliceStable(hubNodes, func(i, j int) bool { return hubNodes[i].ID < hubNodes[j].ID })

	total := len(artistIDs) + len(hubNodes)
	b := &overviewBuild{
		nodeIDs:       make([]uint, 0, total),
		nodeKind:      make([]uint8, 0, total),
		nodeName:      make([]string, 0, total),
		nodeSlug:      make([]string, 0, total),
		nodeCommunity: make([]int32, 0, total),
		nodeHubCity:   make([]string, 0, total),
		index:         make(map[uint]int32, total),
		spokeArtists:  make(map[int32][]int32),
	}

	for _, id := range artistIDs {
		meta := artistMeta[id]
		community := int32(-1)
		if meta.CommunityID != nil {
			community = int32(*meta.CommunityID)
		}
		b.index[id] = int32(len(b.nodeIDs))
		b.nodeIDs = append(b.nodeIDs, id)
		b.nodeKind = append(b.nodeKind, contracts.GraphOverviewNodeArtist)
		b.nodeName = append(b.nodeName, meta.Name)
		b.nodeSlug = append(b.nodeSlug, derefString(meta.Slug))
		b.nodeCommunity = append(b.nodeCommunity, community)
		// An artist's own city is deliberately NOT emitted here — the column is
		// the hub caption, not a location column (see the contract).
		b.nodeHubCity = append(b.nodeHubCity, "")
	}
	for _, hub := range hubNodes {
		b.index[hub.ID] = int32(len(b.nodeIDs))
		b.nodeIDs = append(b.nodeIDs, hub.ID)
		b.nodeKind = append(b.nodeKind, contracts.GraphOverviewNodeLabelHub)
		b.nodeName = append(b.nodeName, hub.Name)
		b.nodeSlug = append(b.nodeSlug, hub.Slug)
		// A hub belongs to no community: communities describe artist
		// similarity, and a hub is a membership fact. -1 keeps it out of every
		// region hull and every region's member count.
		b.nodeCommunity = append(b.nodeCommunity, -1)
		// Trimmed HERE rather than at the caption, so "  " and "" are the same
		// absent city everywhere downstream and no client has to re-derive the
		// rule. The hub node already carries the city the roster read selected;
		// nothing extra is loaded for it.
		b.nodeHubCity = append(b.nodeHubCity, strings.TrimSpace(hub.City))
	}

	// Canonical, deduplicated edge set. A pair could in principle arrive from
	// both the backbone and a spoke only if a hub id collided with an artist
	// id, which buildLabelHubs refuses outright — but deduping here costs
	// nothing and makes the CSR arrays' length claim unconditional.
	seen := make(map[[2]int32]struct{}, len(backbone)+len(hubs.Spokes))
	addEdge := func(a, bIdx int32, kind uint8) {
		if a == bIdx {
			return
		}
		if a > bIdx {
			a, bIdx = bIdx, a
		}
		key := [2]int32{a, bIdx}
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		b.edgeSrc = append(b.edgeSrc, a)
		b.edgeDst = append(b.edgeDst, bIdx)
		b.edgeKind = append(b.edgeKind, kind)
	}

	for _, e := range backbone {
		ai, aok := b.index[e.A]
		bi, bok := b.index[e.B]
		if !aok || !bok {
			continue
		}
		addEdge(ai, bi, contracts.GraphOverviewEdgeSimilarity)
	}
	for _, spoke := range hubs.Spokes {
		hi, hok := b.index[spoke.SourceID]
		ai, aok := b.index[spoke.TargetID]
		if !hok || !aok {
			continue
		}
		addEdge(hi, ai, contracts.GraphOverviewEdgeLabelSpoke)
		b.spokeArtists[hi] = append(b.spokeArtists[hi], ai)
	}

	// Sort the edge set into canonical order, carrying kind along. Map
	// iteration above preserved insertion order, but the backbone and the
	// spokes were appended in two passes — a single sort is what makes the
	// index-to-edge mapping identical between two runs.
	order := make([]int32, len(b.edgeSrc))
	for i := range order {
		order[i] = int32(i)
	}
	sort.SliceStable(order, func(i, j int) bool {
		a, c := order[i], order[j]
		if b.edgeSrc[a] != b.edgeSrc[c] {
			return b.edgeSrc[a] < b.edgeSrc[c]
		}
		return b.edgeDst[a] < b.edgeDst[c]
	})
	src := make([]int32, len(order))
	dst := make([]int32, len(order))
	kind := make([]uint8, len(order))
	for newIdx, oldIdx := range order {
		src[newIdx] = b.edgeSrc[oldIdx]
		dst[newIdx] = b.edgeDst[oldIdx]
		kind[newIdx] = b.edgeKind[oldIdx]
	}
	b.edgeSrc, b.edgeDst, b.edgeKind = src, dst, kind

	b.adjacency = make([][]overviewAdjSlot, len(b.nodeIDs))
	for e := range b.edgeSrc {
		u, v := b.edgeSrc[e], b.edgeDst[e]
		b.adjacency[u] = append(b.adjacency[u], overviewAdjSlot{target: v, edge: int32(e)})
		b.adjacency[v] = append(b.adjacency[v], overviewAdjSlot{target: u, edge: int32(e)})
	}
	b.neighbors = make([][]int32, len(b.adjacency))
	for i, slots := range b.adjacency {
		sort.Slice(slots, func(a, c int) bool { return slots[a].target < slots[c].target })
		targets := make([]int32, len(slots))
		for j, s := range slots {
			targets[j] = s.target
		}
		b.neighbors[i] = targets
	}
	return b
}

// nodeFlags packs each node's "worth clicking" signals.
//
// A label hub gets the OR of its roster's flags: a hub is worth opening when
// anything on the label is, and the client draws hubs from the same array.
func (b *overviewBuild) nodeFlags(artistMeta map[uint]overviewArtist, upcoming map[uint]struct{}) []uint8 {
	flags := make([]uint8, len(b.nodeIDs))
	for i, id := range b.nodeIDs {
		if b.nodeKind[i] != contracts.GraphOverviewNodeArtist {
			continue
		}
		meta := artistMeta[id]
		if meta.BandcampEmbedURL != nil && *meta.BandcampEmbedURL != "" {
			flags[i] |= contracts.GraphOverviewFlagPlayableAudio
		}
		if _, ok := upcoming[id]; ok {
			flags[i] |= contracts.GraphOverviewFlagUpcomingShow
		}
	}
	for hubIdx, artists := range b.spokeArtists {
		var merged uint8
		for _, a := range artists {
			merged |= flags[a]
		}
		flags[hubIdx] = merged
	}
	return flags
}

// appearTimes returns each node's appearance time in epoch-relative seconds.
//
// An artist appears at the earlier of its own created_at and its first show —
// a band that played in 2019 and was catalogued last week belongs on the
// timeline in 2019. A label hub inherits the earliest appearance among the
// artists it connects, because the hub is a statement about that roster and
// cannot predate its first member.
func (b *overviewBuild) appearTimes(artistMeta map[uint]overviewArtist, firstShow map[uint]time.Time) []int32 {
	appear := make([]int32, len(b.nodeIDs))
	for i, id := range b.nodeIDs {
		if b.nodeKind[i] != contracts.GraphOverviewNodeArtist {
			continue
		}
		meta := artistMeta[id]
		when := meta.CreatedAt
		if show, ok := firstShow[id]; ok && show.Before(when) {
			when = show
		}
		appear[i] = appearSeconds(when)
	}
	for hubIdx, artists := range b.spokeArtists {
		earliest := int32(0)
		for j, a := range artists {
			if j == 0 || appear[a] < earliest {
				earliest = appear[a]
			}
		}
		appear[hubIdx] = earliest
	}
	return appear
}

// csr expands the edge set into the payload's CSR arrays.
//
// Both directions are emitted (see the contract's note on drawing each edge
// once) so a neighbourhood lookup is a slice of one array. Per-slot kind and
// appear are carried alongside rather than looked up client-side, which keeps
// the client from needing the edge list the CSR replaces.
func (b *overviewBuild) csr(appear []int32) contracts.GraphOverviewEdges {
	offsets := make([]int32, len(b.adjacency)+1)
	slots := 0
	for i, adj := range b.adjacency {
		offsets[i] = int32(slots)
		slots += len(adj)
	}
	offsets[len(b.adjacency)] = int32(slots)

	targets := make([]int32, 0, slots)
	kinds := make([]uint8, 0, slots)
	appears := make([]int32, 0, slots)
	for i, adj := range b.adjacency {
		for _, slot := range adj {
			targets = append(targets, slot.target)
			kinds = append(kinds, b.edgeKind[slot.edge])
			// An edge cannot exist before both of its endpoints do.
			edgeAppear := appear[i]
			if appear[slot.target] > edgeAppear {
				edgeAppear = appear[slot.target]
			}
			appears = append(appears, edgeAppear)
		}
	}
	return contracts.GraphOverviewEdges{
		Offsets: offsets,
		Targets: targets,
		Kind:    kinds,
		Appear:  appears,
	}
}

// regions builds the labelled community areas.
//
// Only communities with a label row are emitted: a region's whole job is to
// name a part of the map, and an unnamed blob is chrome. Nodes in an unlabelled
// community keep their community id and their colour, they just get no outline.
func (b *overviewBuild) regions(points []layoutPoint, labels map[int]string, extent float64) []contracts.GraphOverviewRegion {
	members := make(map[int32][]int32)
	for i, community := range b.nodeCommunity {
		if community < 0 {
			continue
		}
		members[community] = append(members[community], int32(i))
	}
	communities := make([]int32, 0, len(members))
	for c := range members {
		communities = append(communities, c)
	}
	sort.Slice(communities, func(i, j int) bool { return communities[i] < communities[j] })

	out := make([]contracts.GraphOverviewRegion, 0, len(communities))
	for _, community := range communities {
		label, ok := labels[int(community)]
		if !ok {
			continue
		}
		idxs := members[community]
		pts := make([]layoutPoint, len(idxs))
		for i, idx := range idxs {
			pts[i] = points[idx]
		}
		hull := padHull(convexHull(pts), graphOverviewHullPadding)
		// Empty, never nil: the contract says "empty when the region has too few
		// members", and a nil slice marshals to JSON null, which is what a client
		// doing region.hull.length trips over.
		quantized := [][2]int16{}
		if !hullIsDegenerate(hull, extent) {
			quantized = make([][2]int16, len(hull))
			for i, p := range hull {
				quantized[i] = [2]int16{
					quantizeCoordinate(float64(p.X), extent),
					quantizeCoordinate(float64(p.Y), extent),
				}
			}
		}
		out = append(out, contracts.GraphOverviewRegion{
			Community:   community,
			Label:       "Around " + label,
			MemberCount: len(idxs),
			Hull:        quantized,
		})
	}
	return out
}

// payload assembles the served snapshot.
func (b *overviewBuild) payload(
	builtAt time.Time,
	extent float64,
	xs, ys []int16,
	ranks []int32,
	rankMetric string,
	appear []int32,
	flags []uint8,
	regions []contracts.GraphOverviewRegion,
	isolates int,
) contracts.GraphOverview {
	degree := make([]int32, len(b.adjacency))
	for i, adj := range b.adjacency {
		degree[i] = int32(len(adj))
	}
	return contracts.GraphOverview{
		Version:      contracts.GraphOverviewVersion,
		LastMapped:   builtAt,
		Epoch:        graphOverviewEpoch,
		Extent:       extent,
		NodeCount:    len(b.nodeIDs),
		EdgeCount:    len(b.edgeSrc),
		IsolateCount: isolates,
		RankMetric:   rankMetric,
		HullKind:     contracts.GraphOverviewHullConvex,
		Nodes: contracts.GraphOverviewNodes{
			ID:        b.nodeIDs,
			Kind:      b.nodeKind,
			Name:      b.nodeName,
			Slug:      b.nodeSlug,
			HubCity:   b.nodeHubCity,
			X:         xs,
			Y:         ys,
			Community: b.nodeCommunity,
			Degree:    degree,
			Rank:      ranks,
			Flags:     flags,
			Appear:    appear,
		},
		Edges:   b.csr(appear),
		Regions: regions,
	}
}

// structureKey digests everything the LAYOUT depends on: the node set and the
// edge set, in the build's canonical order. It deliberately excludes names,
// slugs, hub cities, communities, appear times and every other attribute —
// those change the payload but must not move a single dot.
//
// That exclusion is why an ADDITIVE payload column is free: a column the key
// does not read cannot invalidate a warm start, so the night this one first
// ships reuses the stored positions verbatim and every dot stays where it was.
//
// It is what lets an unchanged night skip the physics entirely and reuse the
// previous positions verbatim. Without it, 50 more relaxation iterations run on
// an already-relaxed layout and every node drifts a little every night, which is
// precisely the "map reshuffles under the reader" failure the stability contract
// exists to prevent.
func (b *overviewBuild) structureKey() string {
	h := sha256.New()
	var buf [8]byte
	write := func(v uint64) {
		binary.BigEndian.PutUint64(buf[:], v)
		h.Write(buf[:])
	}
	write(uint64(len(b.nodeIDs)))
	for _, id := range b.nodeIDs {
		write(uint64(id))
	}
	write(uint64(len(b.edgeSrc)))
	for i := range b.edgeSrc {
		write(uint64(b.edgeSrc[i]))
		write(uint64(b.edgeDst[i]))
	}
	return hex.EncodeToString(h.Sum(nil))
}

// layoutState is the next epoch's warm-start input: full float32 positions
// keyed by node id, so a node that keeps its id resumes where it was even if
// the index space shifts around it.
func (b *overviewBuild) layoutState(points []layoutPoint) map[string][2]float32 {
	out := make(map[string][2]float32, len(points))
	for i, id := range b.nodeIDs {
		out[strconv.FormatUint(uint64(id), 10)] = [2]float32{points[i].X, points[i].Y}
	}
	return out
}
