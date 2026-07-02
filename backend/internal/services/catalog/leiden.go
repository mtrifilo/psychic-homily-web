package catalog

import (
	"math/rand"
	"sort"
)

// Leiden community detection (PSY-1262).
//
// Implements Traag, Waltman & van Eck, "From Louvain to Leiden: guaranteeing
// well-connected communities" (Scientific Reports 9:5233, 2019) over the
// weighted undirected artist-similarity graph. Louvain's documented failure
// mode — up to ~16% disconnected and ~25% badly-connected communities — is
// what the refinement phase exists to prevent: within each coarse community,
// nodes re-cluster from singletons, merging only into sub-communities they
// are connected to and that are themselves well-connected, and aggregation
// happens on the REFINED partition with the coarse partition as the
// constraint set.
//
// Design points for this codebase:
//   - Quality function: modularity with a resolution parameter gamma
//     (gamma = 1 is classic modularity; higher gamma → more, smaller
//     communities). The consumer sweeps gamma against known scenes.
//   - DETERMINISM: iteration orders derive from a seeded RNG plus sorted
//     node indexing, and all tie-breaks prefer the lower index, so the same
//     (edges, resolution, seed) always yields the same partition — community
//     assignments must not reshuffle between nightly recomputes (the
//     persisted-partition requirement).
//   - Refinement takes the BEST-gain candidate (the paper's theta → 0
//     limit) rather than sampling gain-proportionally; the connectivity
//     guarantee comes from the merge constraints, not from the randomized
//     selection.
//   - Belt and braces: the PSY-1262 acceptance criteria require post-hoc
//     connectivity validation regardless of algorithm, so LeidenCommunities
//     splits any disconnected community into components before returning.
//     With a correct refinement this is a no-op.
//
// Input hygiene mirrors DisparitySignificance: self-loops and non-positive
// weights are ignored; parallel edges collapse into one undirected edge of
// summed weight. Isolated nodes never appear in `edges` and therefore get no
// assignment — callers treat a missing key as "no community".

// LeidenResolution is the default modularity resolution (gamma) for the
// artist-similarity partition. 1.0 = classic modularity; retune against
// known scenes/labels per the PSY-1262 acceptance criteria.
const LeidenResolution = 1.0

// leidenEdge is an internal, index-space undirected edge.
type leidenEdge struct {
	a, b int
	w    float64
}

// leidenArc is one direction of an edge in the adjacency list.
type leidenArc struct {
	to int
	w  float64
}

// leidenGraph is a weighted undirected graph over dense node indices. The
// canonical edge list is kept alongside the adjacency because aggregation
// iterates edges exactly once.
type leidenGraph struct {
	n        int
	edges    []leidenEdge
	adj      [][]leidenArc
	strength []float64 // weighted degree; self-loops count twice
	selfLoop []float64 // per-node self-loop weight (aggregate graphs only)
	m2       float64   // sum of strengths == 2m; the modularity normalizer
}

// leidenPartition assigns each node of one graph level to a community and
// tracks per-community strength totals for O(1) modularity deltas.
type leidenPartition struct {
	nodeComm  []int
	commTotal []float64
	commCount int
}

// LeidenCommunities partitions the graph and returns a community index per
// node ID. Indices are dense (0..k-1) and deterministically numbered by each
// community's smallest member node ID, so downstream storage diffs stay
// stable run-to-run.
func LeidenCommunities(edges []WeightedEdge, resolution float64, seed int64) map[uint]int {
	g, ids := buildLeidenGraph(edges)
	if g.n == 0 {
		return map[uint]int{}
	}

	rng := rand.New(rand.NewSource(seed))

	// nodeAgg maps each ORIGINAL node to its node in the current aggregate
	// graph; identity at level 0.
	nodeAgg := make([]int, g.n)
	for i := range nodeAgg {
		nodeAgg[i] = i
	}

	cur := g
	part := singletonPartition(cur)

	for {
		localMoveFast(cur, &part, resolution, rng)

		// Fully converged: no aggregate node merged with any other.
		if part.commCount == cur.n {
			break
		}

		refined := refinePartition(cur, &part, resolution, rng)
		if refined.commCount == cur.n {
			// Refinement kept everything singleton — aggregating would
			// reproduce the current graph and loop forever. The coarse
			// partition from localMove is the final answer at this level.
			break
		}

		next, refinedIndex, nextInit := aggregateGraph(cur, &refined, &part)
		for i := range nodeAgg {
			nodeAgg[i] = refinedIndex[nodeAgg[i]]
		}
		cur = next
		part = nextInit
	}

	// Final community of an original node is its aggregate node's coarse
	// community. Renumber deterministically by smallest member node ID
	// (ids is sorted, so first occurrence order == smallest-ID order).
	assignment := make([]int, g.n)
	for i := range assignment {
		assignment[i] = part.nodeComm[nodeAgg[i]]
	}
	assignment = splitDisconnected(g, assignment)

	renumber := make(map[int]int, part.commCount)
	out := make(map[uint]int, g.n)
	for i, comm := range assignment {
		id, ok := renumber[comm]
		if !ok {
			id = len(renumber)
			renumber[comm] = id
		}
		out[ids[i]] = id
	}
	return out
}

// buildLeidenGraph indexes node IDs (sorted for determinism), collapses
// parallel edges, and drops self-loops / non-positive weights.
func buildLeidenGraph(edges []WeightedEdge) (leidenGraph, []uint) {
	collapsed := make(map[EdgeKey]float64, len(edges))
	idSet := make(map[uint]struct{}, len(edges)*2)
	for _, e := range edges {
		if e.A == e.B || e.Weight <= 0 {
			continue
		}
		collapsed[canonicalKey(e.A, e.B)] += e.Weight
		idSet[e.A] = struct{}{}
		idSet[e.B] = struct{}{}
	}

	ids := make([]uint, 0, len(idSet))
	for id := range idSet {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	index := make(map[uint]int, len(ids))
	for i, id := range ids {
		index[id] = i
	}

	g := leidenGraph{
		n:        len(ids),
		edges:    make([]leidenEdge, 0, len(collapsed)),
		adj:      make([][]leidenArc, len(ids)),
		strength: make([]float64, len(ids)),
		selfLoop: make([]float64, len(ids)),
	}
	// Deterministic edge order: sort collapsed keys.
	keys := make([]EdgeKey, 0, len(collapsed))
	for k := range collapsed {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i][0] != keys[j][0] {
			return keys[i][0] < keys[j][0]
		}
		return keys[i][1] < keys[j][1]
	})
	for _, k := range keys {
		a, b, w := index[k[0]], index[k[1]], collapsed[k]
		g.edges = append(g.edges, leidenEdge{a: a, b: b, w: w})
		g.adj[a] = append(g.adj[a], leidenArc{to: b, w: w})
		g.adj[b] = append(g.adj[b], leidenArc{to: a, w: w})
		g.strength[a] += w
		g.strength[b] += w
		g.m2 += 2 * w
	}
	return g, ids
}

func singletonPartition(g leidenGraph) leidenPartition {
	p := leidenPartition{
		nodeComm:  make([]int, g.n),
		commTotal: make([]float64, g.n),
		commCount: g.n,
	}
	for i := 0; i < g.n; i++ {
		p.nodeComm[i] = i
		p.commTotal[i] = g.strength[i]
	}
	return p
}

// localMoveFast is the queue-based local moving phase: nodes visit in a
// seeded random order; a moved node re-queues its neighbors outside the new
// community. Returns whether any node moved.
func localMoveFast(g leidenGraph, p *leidenPartition, gamma float64, rng *rand.Rand) bool {
	queue := rng.Perm(g.n)
	inQueue := make([]bool, g.n)
	for _, v := range queue {
		inQueue[v] = true
	}

	// commWeight[c] = k_{v,c} for the node currently being evaluated;
	// touched tracks which entries to reset (avoids O(n) clears per node).
	commWeight := make([]float64, g.n)
	touched := make([]int, 0, 16)

	moved := false
	for head := 0; head < len(queue); head++ {
		v := queue[head]
		inQueue[v] = false

		old := p.nodeComm[v]
		// Remove v from its community for the evaluation.
		p.commTotal[old] -= g.strength[v]

		touched = touched[:0]
		for _, arc := range g.adj[v] {
			c := p.nodeComm[arc.to]
			if commWeight[c] == 0 {
				touched = append(touched, c)
			}
			commWeight[c] += arc.w
		}

		// Baseline: staying in the (now v-less) old community. An empty old
		// community scores 0 — i.e. staying singleton.
		best, bestGain := old, commWeight[old]-gamma*g.strength[v]*p.commTotal[old]/g.m2
		for _, c := range touched {
			gain := commWeight[c] - gamma*g.strength[v]*p.commTotal[c]/g.m2
			if gain > bestGain || (gain == bestGain && c < best) {
				best, bestGain = c, gain
			}
		}
		for _, c := range touched {
			commWeight[c] = 0
		}

		if best != old {
			moved = true
			// Re-queue neighbors that are not in v's new community.
			for _, arc := range g.adj[v] {
				if p.nodeComm[arc.to] != best && !inQueue[arc.to] {
					inQueue[arc.to] = true
					queue = append(queue, arc.to)
				}
			}
		}
		p.nodeComm[v] = best
		p.commTotal[best] += g.strength[v]
	}

	// Recount communities exactly (the decrement above misses re-merges).
	seen := make(map[int]struct{}, g.n)
	for _, c := range p.nodeComm {
		seen[c] = struct{}{}
	}
	p.commCount = len(seen)
	return moved
}

// refinePartition re-clusters each coarse community from singletons, merging
// a node only into a sub-community it is connected to, and only when both the
// node and the target sub-community are well-connected within the coarse
// community — the constraints Leiden's connectivity guarantee rests on.
func refinePartition(g leidenGraph, coarse *leidenPartition, gamma float64, rng *rand.Rand) leidenPartition {
	refined := singletonPartition(g)

	// Group members by coarse community.
	members := make(map[int][]int, coarse.commCount)
	for v, c := range coarse.nodeComm {
		members[c] = append(members[c], v)
	}
	// Deterministic community visit order.
	commIDs := make([]int, 0, len(members))
	for c := range members {
		commIDs = append(commIDs, c)
	}
	sort.Ints(commIDs)

	// Scratch: k_{v,T} accumulation per refined community.
	subWeight := make([]float64, g.n)
	touched := make([]int, 0, 16)

	for _, c := range commIDs {
		ms := members[c]
		if len(ms) < 2 {
			continue
		}
		// K_S and each member's edge weight into the rest of S.
		var totalS float64
		inS := make(map[int]struct{}, len(ms))
		for _, v := range ms {
			totalS += g.strength[v]
			inS[v] = struct{}{}
		}
		extWithin := make(map[int]float64, len(ms)) // E(v, S \ {v})
		for _, v := range ms {
			for _, arc := range g.adj[v] {
				if _, ok := inS[arc.to]; ok {
					extWithin[v] += arc.w
				}
			}
		}

		// Per refined sub-community bookkeeping (keyed by refined comm id):
		// K_T and E(T, S \ T). Starts as singletons.
		subTotal := make(map[int]float64, len(ms))
		subExt := make(map[int]float64, len(ms))
		for _, v := range ms {
			subTotal[refined.nodeComm[v]] = g.strength[v]
			subExt[refined.nodeComm[v]] = extWithin[v]
		}

		order := rng.Perm(len(ms))
		for _, oi := range order {
			v := ms[oi]
			rv := refined.nodeComm[v]
			if subTotal[rv] != g.strength[v] {
				// v is no longer singleton (something merged into it, or it
				// moved) — the paper only moves singleton nodes.
				continue
			}
			// Node well-connectedness within S.
			if extWithin[v] < gamma*g.strength[v]*(totalS-g.strength[v])/g.m2 {
				continue
			}

			// k_{v,T} per refined sub-community within S.
			touched = touched[:0]
			for _, arc := range g.adj[v] {
				if _, ok := inS[arc.to]; !ok {
					continue
				}
				t := refined.nodeComm[arc.to]
				if t == rv {
					continue
				}
				if subWeight[t] == 0 {
					touched = append(touched, t)
				}
				subWeight[t] += arc.w
			}

			best, bestGain := -1, 0.0
			for _, t := range touched {
				kT := subTotal[t]
				// Sub-community well-connectedness within S.
				if subExt[t] < gamma*kT*(totalS-kT)/g.m2 {
					continue
				}
				gain := subWeight[t] - gamma*g.strength[v]*kT/g.m2
				if gain > bestGain || (gain == bestGain && gain > 0 && (best == -1 || t < best)) {
					best, bestGain = t, gain
				}
			}
			if best >= 0 && bestGain > 0 {
				kvT := subWeight[best]
				refined.nodeComm[v] = best
				refined.commTotal[best] += g.strength[v]
				refined.commTotal[rv] = 0
				subTotal[best] += g.strength[v]
				delete(subTotal, rv)
				subExt[best] += extWithin[v] - 2*kvT
				delete(subExt, rv)
				refined.commCount--
			}
			for _, t := range touched {
				subWeight[t] = 0
			}
		}
	}
	return refined
}

// aggregateGraph collapses each refined community into one node. The next
// level's initial partition groups aggregate nodes by their COARSE community
// — Leiden's defining constraint. Returns the new graph, the original-level
// node → aggregate-node map, and that initial partition.
func aggregateGraph(g leidenGraph, refined, coarse *leidenPartition) (leidenGraph, []int, leidenPartition) {
	// Compact refined community ids in first-seen (== smallest member index)
	// order for determinism.
	compact := make(map[int]int, refined.commCount)
	refinedIndex := make([]int, g.n)
	for v := 0; v < g.n; v++ {
		rc := refined.nodeComm[v]
		id, ok := compact[rc]
		if !ok {
			id = len(compact)
			compact[rc] = id
		}
		refinedIndex[v] = id
	}
	nn := len(compact)

	next := leidenGraph{
		n:        nn,
		adj:      make([][]leidenArc, nn),
		strength: make([]float64, nn),
		selfLoop: make([]float64, nn),
		m2:       g.m2,
	}

	// Carry forward the fine graph's OWN self-loops (zero at level 0, but at
	// deeper levels each node already wraps internal weight — dropping it
	// deflates strengths against the conserved m2 and inflates merge gains,
	// which over-merges every level below the first; caught by the Zachary
	// modularity test).
	for v := 0; v < g.n; v++ {
		next.selfLoop[refinedIndex[v]] += g.selfLoop[v]
	}

	// Sum edge weights between aggregate nodes; internal edges become
	// self-loops (strength contribution 2w, matching the fine graph).
	type aggKey struct{ a, b int }
	acc := make(map[aggKey]float64, len(g.edges))
	for _, e := range g.edges {
		ra, rb := refinedIndex[e.a], refinedIndex[e.b]
		if ra == rb {
			next.selfLoop[ra] += e.w
			continue
		}
		if ra > rb {
			ra, rb = rb, ra
		}
		acc[aggKey{ra, rb}] += e.w
	}
	// Deterministic edge order.
	aggKeys := make([]aggKey, 0, len(acc))
	for k := range acc {
		aggKeys = append(aggKeys, k)
	}
	sort.Slice(aggKeys, func(i, j int) bool {
		if aggKeys[i].a != aggKeys[j].a {
			return aggKeys[i].a < aggKeys[j].a
		}
		return aggKeys[i].b < aggKeys[j].b
	})
	for _, k := range aggKeys {
		w := acc[k]
		next.edges = append(next.edges, leidenEdge{a: k.a, b: k.b, w: w})
		next.adj[k.a] = append(next.adj[k.a], leidenArc{to: k.b, w: w})
		next.adj[k.b] = append(next.adj[k.b], leidenArc{to: k.a, w: w})
		next.strength[k.a] += w
		next.strength[k.b] += w
	}
	for i := 0; i < nn; i++ {
		next.strength[i] += 2 * next.selfLoop[i]
	}

	// Initial partition: aggregate nodes grouped by coarse community. Use
	// the coarse community id of any member (all members of a refined
	// community share one coarse community by construction), compacted.
	init := leidenPartition{
		nodeComm:  make([]int, nn),
		commTotal: make([]float64, nn),
	}
	coarseCompact := make(map[int]int, coarse.commCount)
	for v := 0; v < g.n; v++ {
		agg := refinedIndex[v]
		cc := coarse.nodeComm[v]
		id, ok := coarseCompact[cc]
		if !ok {
			id = len(coarseCompact)
			coarseCompact[cc] = id
		}
		init.nodeComm[agg] = id
	}
	for agg := 0; agg < nn; agg++ {
		init.commTotal[init.nodeComm[agg]] += next.strength[agg]
	}
	init.commCount = len(coarseCompact)

	return next, refinedIndex, init
}

// splitDisconnected is the post-hoc validation the ticket requires: any
// community whose members do not form a single connected component in the
// ORIGINAL graph is split into its components. With a correct Leiden
// refinement this is a no-op.
func splitDisconnected(g leidenGraph, assignment []int) []int {
	out := make([]int, len(assignment))
	visited := make([]bool, g.n)
	next := 0
	for v := 0; v < g.n; v++ {
		if visited[v] {
			continue
		}
		// BFS within v's community.
		comm := assignment[v]
		stack := []int{v}
		visited[v] = true
		for len(stack) > 0 {
			u := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			out[u] = next
			for _, arc := range g.adj[u] {
				if !visited[arc.to] && assignment[arc.to] == comm {
					visited[arc.to] = true
					stack = append(stack, arc.to)
				}
			}
		}
		next++
	}
	return out
}
