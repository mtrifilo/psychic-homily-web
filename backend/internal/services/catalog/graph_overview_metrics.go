package catalog

import (
	"math"
	"sort"

	"psychic-homily-backend/internal/services/contracts"
)

// ──────────────────────────────────────────────
// Overview map metrics: centrality rank and region hulls
// ──────────────────────────────────────────────

const (
	// betweennessPivotThreshold is the node count above which betweenness
	// switches from exact to pivot-sampled.
	//
	// MEASURED, not assumed (the ticket asked for exactly this): Brandes at
	// 5,000 nodes / 25,000 edges is ~1.1s exact against ~120ms sampled, on a
	// nightly job that already spends 1-10s in the layout subprocess. Sampling
	// is not free either — against a uniform-random graph at that scale only
	// ~120 of the exact top-200 ranks survive into the sampled top-200, and
	// that rank IS the label tiering the map draws. So exact wins at any
	// plausible near-term catalog size and the threshold sits far above it.
	//
	// The sampled path stays as a blowup guard, not as an optimization:
	// Brandes is O(V*E), so a catalog an order of magnitude larger would turn
	// ~1s into minutes, and a nightly job should degrade its label tiers rather
	// than stall. GraphOverview.RankMetric reports which path produced a given
	// map so the degradation is visible rather than silent.
	betweennessPivotThreshold = 25000

	// betweennessPivots is the number of BFS sources used above the threshold.
	// Sampled betweenness converges on the RANK ordering long before it
	// converges on the values, and rank is the entire use here.
	betweennessPivots = 512
)

// betweennessCentrality computes Brandes' betweenness over an undirected,
// unweighted graph given as adjacency lists of node indexes.
//
// UNWEIGHTED ON PURPOSE. Edge scores here mix relationship types on
// incomparable scales (a co-billing count against a normalized label overlap),
// so treating them as path LENGTHS would rank on an artifact of how each
// derivation happens to be scaled. Hop count answers the question the label
// tiers actually ask: who sits between otherwise-distant parts of the map.
//
// When len(neighbors) exceeds betweennessPivotThreshold the scores are
// estimated from betweennessPivots evenly-spaced sources rather than every
// node. The sources are picked by stride from index 0, so the choice is
// deterministic and the result is reproducible — an RNG here would put a
// wobble into the label tiers on every rebuild.
//
// Returned scores are unnormalized and, when sampled, not on the same scale as
// an exact run. Compare them to each other, never across runs.
func betweennessCentrality(neighbors [][]int32) ([]float64, string) {
	n := len(neighbors)
	scores := make([]float64, n)
	if n == 0 {
		return scores, contracts.GraphOverviewRankBetweenness
	}

	sources := make([]int32, 0, n)
	metric := contracts.GraphOverviewRankBetweenness
	if n <= betweennessPivotThreshold {
		for i := 0; i < n; i++ {
			sources = append(sources, int32(i))
		}
	} else {
		metric = contracts.GraphOverviewRankBetweennessSampled
		stride := float64(n) / float64(betweennessPivots)
		for k := 0; k < betweennessPivots; k++ {
			idx := int(float64(k) * stride)
			if idx >= n {
				break
			}
			sources = append(sources, int32(idx))
		}
	}

	// Brandes scratch, allocated once and reset per source.
	sigma := make([]float64, n)
	delta := make([]float64, n)
	dist := make([]int32, n)
	queue := make([]int32, 0, n)
	stack := make([]int32, 0, n)
	// predecessors[v] holds v's predecessors on shortest paths from the current
	// source. Reusing the backing arrays across sources keeps this out of the
	// allocator's way on the inner loop.
	predecessors := make([][]int32, n)

	for _, s := range sources {
		for i := 0; i < n; i++ {
			sigma[i] = 0
			delta[i] = 0
			dist[i] = -1
			predecessors[i] = predecessors[i][:0]
		}
		sigma[s] = 1
		dist[s] = 0
		queue = append(queue[:0], s)
		stack = stack[:0]

		for head := 0; head < len(queue); head++ {
			v := queue[head]
			stack = append(stack, v)
			for _, w := range neighbors[v] {
				if dist[w] < 0 {
					dist[w] = dist[v] + 1
					queue = append(queue, w)
				}
				if dist[w] == dist[v]+1 {
					sigma[w] += sigma[v]
					predecessors[w] = append(predecessors[w], v)
				}
			}
		}

		for i := len(stack) - 1; i >= 0; i-- {
			w := stack[i]
			for _, v := range predecessors[w] {
				delta[v] += sigma[v] / sigma[w] * (1 + delta[w])
			}
			if w != s {
				scores[w] += delta[w]
			}
		}
	}
	return scores, metric
}

// rankByScore turns centrality scores into dense ranks, 0 = highest score.
//
// Ties break by node INDEX, which is itself derived from the build's stable
// node ordering — without that, two nodes with equal centrality (very common in
// the sparse tail, where most scores are exactly 0) would swap tiers between
// otherwise-identical rebuilds.
func rankByScore(scores []float64) []int32 {
	order := make([]int32, len(scores))
	for i := range order {
		order[i] = int32(i)
	}
	sort.SliceStable(order, func(a, b int) bool {
		ia, ib := order[a], order[b]
		if scores[ia] != scores[ib] {
			return scores[ia] > scores[ib]
		}
		return ia < ib
	})
	ranks := make([]int32, len(scores))
	for rank, idx := range order {
		ranks[idx] = int32(rank)
	}
	return ranks
}

// graphOverviewStartingPoints is how many top-connected artists each build
// stores for the /graph fallback hero (PSY-1749).
//
// Sized for the CLIENT'S rotation, not for a browse list: the hero shows a
// small random subset per visit, so the pool only has to be deep enough that
// two consecutive visits rarely draw the same names, and shallow enough that
// every entry is genuinely one of the map's hubs. A pool the size of a page of
// search results would put the graph's mediocre middle into a sentence that
// says "start here".
const graphOverviewStartingPoints = 12

// pickStartingPointArtists returns the most connected ARTIST ids on the map,
// most connected first, capped at graphOverviewStartingPoints.
//
// THE SAME CENTRALITY THAT TIERS THE MAP'S LABELS. The suggestion a visitor is
// offered when the map cannot be drawn is therefore the same band the map would
// have drawn largest if it could — one ranking, not two ideas of "important"
// that drift apart. Betweenness also answers the question the hero actually
// asks: a leaf scores exactly 0 and can never enter the pool, while a band that
// sits between otherwise-distant parts of the scene scores highest, and its ego
// graph is the dense first neighbourhood the hero is promising.
//
// LABEL HUBS ARE EXCLUDED. A hub is a membership fact with a very high degree
// by construction (every roster member is a spoke), so it would dominate this
// ranking — and it is not an artist, so its id would center nothing.
//
// Ties break on degree and then node index, so an all-zero centrality (a tiny
// dev catalog where every path is unique) still yields the best-connected
// artists rather than the lowest ids, and two runs over unchanged data pick the
// same set in the same order.
func pickStartingPointArtists(b *overviewBuild, centrality []float64) []uint {
	if b == nil || len(centrality) != len(b.nodeIDs) {
		return nil
	}

	candidates := make([]int, 0, len(b.nodeIDs))
	for i, kind := range b.nodeKind {
		if kind != contracts.GraphOverviewNodeArtist {
			continue
		}
		candidates = append(candidates, i)
	}
	sort.SliceStable(candidates, func(x, y int) bool {
		i, j := candidates[x], candidates[y]
		if centrality[i] != centrality[j] {
			return centrality[i] > centrality[j]
		}
		if len(b.neighbors[i]) != len(b.neighbors[j]) {
			return len(b.neighbors[i]) > len(b.neighbors[j])
		}
		return i < j
	})

	if len(candidates) > graphOverviewStartingPoints {
		candidates = candidates[:graphOverviewStartingPoints]
	}
	ids := make([]uint, len(candidates))
	for i, idx := range candidates {
		ids[i] = b.nodeIDs[idx]
	}
	return ids
}

// convexHull returns the convex hull of a point set in counter-clockwise order,
// without repeating the first point, using Andrew's monotone chain.
//
// CONVEX, NOT CONCAVE (v1). The mock's regions are soft blobs, and a convex
// hull over-claims area on an elongated or crescent-shaped community. A concave
// hull is the better fit but needs either an alpha-shape implementation or a
// new dependency, and this ships a region outline that is always valid and
// never self-intersecting. See the ticket's open question.
//
// Returns nil for fewer than three input points, or when every point is
// collinear — there is no area to enclose, and a degenerate polygon would draw
// as a stray line across the map.
func convexHull(points []layoutPoint) []layoutPoint {
	if len(points) < 3 {
		return nil
	}
	pts := make([]layoutPoint, len(points))
	copy(pts, points)
	sort.SliceStable(pts, func(i, j int) bool {
		if pts[i].X != pts[j].X {
			return pts[i].X < pts[j].X
		}
		return pts[i].Y < pts[j].Y
	})

	// cross is the z component of (o->a) x (o->b): positive when o,a,b turn
	// counter-clockwise. Computed in float64 because float32 coordinates
	// multiplied together lose enough precision to mis-classify a near-collinear
	// turn, which would drop a hull vertex.
	cross := func(o, a, b layoutPoint) float64 {
		return (float64(a.X)-float64(o.X))*(float64(b.Y)-float64(o.Y)) -
			(float64(a.Y)-float64(o.Y))*(float64(b.X)-float64(o.X))
	}

	build := func(in []layoutPoint) []layoutPoint {
		var out []layoutPoint
		for _, p := range in {
			for len(out) >= 2 && cross(out[len(out)-2], out[len(out)-1], p) <= 0 {
				out = out[:len(out)-1]
			}
			out = append(out, p)
		}
		return out
	}

	lower := build(pts)
	reversed := make([]layoutPoint, len(pts))
	for i, p := range pts {
		reversed[len(pts)-1-i] = p
	}
	upper := build(reversed)

	// Drop each chain's last point: it is the other chain's first.
	hull := append(lower[:len(lower)-1], upper[:len(upper)-1]...)
	if len(hull) < 3 {
		return nil
	}
	return hull
}

// padHull pushes each hull vertex outward from the polygon's centroid.
//
// The regions in the design are soft areas around a community, not tight
// wrappers on its outermost members, and an unpadded hull passes exactly
// THROUGH the boundary nodes — so the artists that define a region would be
// drawn half outside it. factor is a fraction of each vertex's distance from
// the centroid, so the padding scales with the region instead of swamping a
// small one.
func padHull(hull []layoutPoint, factor float64) []layoutPoint {
	if len(hull) == 0 || factor == 0 {
		return hull
	}
	var cx, cy float64
	for _, p := range hull {
		cx += float64(p.X)
		cy += float64(p.Y)
	}
	cx /= float64(len(hull))
	cy /= float64(len(hull))

	out := make([]layoutPoint, len(hull))
	for i, p := range hull {
		dx := float64(p.X) - cx
		dy := float64(p.Y) - cy
		out[i] = layoutPoint{
			X: float32(cx + dx*(1+factor)),
			Y: float32(cy + dy*(1+factor)),
		}
	}
	return out
}

// hullIsDegenerate reports whether a polygon encloses no meaningful area,
// relative to the map's extent. A community whose members are collinear (or
// effectively so, after quantization) draws as a sliver rather than a region.
func hullIsDegenerate(hull []layoutPoint, extent float64) bool {
	if len(hull) < 3 {
		return true
	}
	var area2 float64
	for i := range hull {
		j := (i + 1) % len(hull)
		area2 += float64(hull[i].X)*float64(hull[j].Y) - float64(hull[j].X)*float64(hull[i].Y)
	}
	// One part in a million of the map's bounding box: below this a region is
	// visually a line no matter the zoom.
	return math.Abs(area2/2) < extent*extent*1e-6
}
