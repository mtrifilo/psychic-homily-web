package catalog

import "math"

// Disparity-filter backbone extraction (PSY-1261).
//
// The radio co-occurrence graph is dense and hub-driven: a few prolific artists co-occur with
// almost everyone, drowning the niche-but-meaningful links that are the discovery gold. A single
// global weight cutoff would just delete the niche links (they have small absolute weights). The
// disparity filter (Serrano, Boguñá & Vespignani, "Extracting the multiscale backbone of complex
// weighted networks", PNAS 2009) instead tests each edge against a PER-NODE null model, so an
// edge that is a large FRACTION of a node's connections survives even if its absolute weight is
// small. This is the server-side, statistically-principled counterpart to the PSY-1258 client-side
// top-k edge cap. See docs/open-questions/graph-density-discovery-redesign.md §3.3.2.

// WeightedEdge is one undirected, positively-weighted edge for the disparity filter. For the radio
// graph A/B are artist ids and Weight is the co-occurrence count (radio_artist_affinity).
type WeightedEdge struct {
	A, B   uint
	Weight float64
}

// EdgeKey is the canonical (min,max) endpoint pair used to key an undirected edge.
type EdgeKey [2]uint

func canonicalKey(a, b uint) EdgeKey {
	if a > b {
		a, b = b, a
	}
	return EdgeKey{a, b}
}

// DisparitySignificance computes the disparity-filter significance for every edge of an undirected
// weighted graph. The returned value per edge is the SMALLER of its two endpoints' p-values — an
// edge is in the backbone at level alpha iff its significance < alpha (i.e. it is significant for
// AT LEAST ONE endpoint, the union form Serrano et al. use for undirected graphs). Lower = stronger.
//
// For a node of degree k and strength s (sum of its incident weights), an incident edge of weight w
// has normalized weight p = w/s, and the probability under the null model (the node's k normalized
// weights drawn uniformly at random) that a weight would be at least p is the closed form
// (1-p)^(k-1). A degree-1 node's single edge carries all of its strength (p=1) and has no
// alternative, so it is kept by convention (significance 0) — this is what preserves a niche
// artist's one meaningful link, which a global threshold would discard.
//
// Self-loops and non-positive weights are ignored (they don't belong in the null model). Parallel
// edges between the same pair are summed into one undirected edge. The result is keyed by the
// canonical (min,max) endpoint pair; an absent pair was not a valid input edge.
func DisparitySignificance(edges []WeightedEdge) map[EdgeKey]float64 {
	// Collapse to undirected edges (sum parallels) and accumulate per-node degree + strength in one
	// pass. Degree counts DISTINCT neighbors so a summed parallel pair counts once, matching the
	// node's neighbor count k the null model is defined over.
	weightByPair := make(map[EdgeKey]float64)
	for _, e := range edges {
		if e.A == e.B || e.Weight <= 0 {
			continue
		}
		weightByPair[canonicalKey(e.A, e.B)] += e.Weight
	}

	type node struct {
		degree   int
		strength float64
	}
	nodes := make(map[uint]*node)
	touch := func(id uint, w float64) {
		n := nodes[id]
		if n == nil {
			n = &node{}
			nodes[id] = n
		}
		n.degree++
		n.strength += w
	}
	for key, w := range weightByPair {
		touch(key[0], w)
		touch(key[1], w)
	}

	// endpointAlpha is the p-value of edge weight w from the perspective of node id.
	endpointAlpha := func(id uint, w float64) float64 {
		n := nodes[id]
		if n.degree <= 1 {
			return 0 // degree-1: the edge is all there is — keep it (niche-link preservation)
		}
		p := w / n.strength
		if p >= 1 {
			return 0 // carries (effectively) all the node's strength → fully significant
		}
		return math.Pow(1-p, float64(n.degree-1))
	}

	out := make(map[EdgeKey]float64, len(weightByPair))
	for key, w := range weightByPair {
		out[key] = math.Min(endpointAlpha(key[0], w), endpointAlpha(key[1], w))
	}
	return out
}
