package catalog

import (
	"fmt"
	"testing"
)

// modularityOf evaluates classic modularity (gamma=1) of an assignment —
// the quality yardstick for the algorithm tests.
func modularityOf(edges []WeightedEdge, assignment map[uint]int) float64 {
	var m float64
	strength := map[uint]float64{}
	for _, e := range edges {
		if e.A == e.B || e.Weight <= 0 {
			continue
		}
		m += e.Weight
		strength[e.A] += e.Weight
		strength[e.B] += e.Weight
	}
	if m == 0 {
		return 0
	}
	var intra float64
	for _, e := range edges {
		if e.A == e.B || e.Weight <= 0 {
			continue
		}
		if assignment[e.A] == assignment[e.B] {
			intra += e.Weight
		}
	}
	commStrength := map[int]float64{}
	for id, s := range strength {
		commStrength[assignment[id]] += s
	}
	q := intra / m
	for _, s := range commStrength {
		q -= (s / (2 * m)) * (s / (2 * m))
	}
	return q
}

// assertConnectedCommunities fails if any community's members do not form a
// single connected component — the guarantee Leiden exists to provide.
func assertConnectedCommunities(t *testing.T, edges []WeightedEdge, assignment map[uint]int) {
	t.Helper()
	adj := map[uint][]uint{}
	for _, e := range edges {
		if e.A == e.B || e.Weight <= 0 {
			continue
		}
		adj[e.A] = append(adj[e.A], e.B)
		adj[e.B] = append(adj[e.B], e.A)
	}
	members := map[int][]uint{}
	for id, c := range assignment {
		members[c] = append(members[c], id)
	}
	for c, ms := range members {
		seen := map[uint]bool{ms[0]: true}
		stack := []uint{ms[0]}
		for len(stack) > 0 {
			u := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			for _, v := range adj[u] {
				if assignment[v] == c && !seen[v] {
					seen[v] = true
					stack = append(stack, v)
				}
			}
		}
		if len(seen) != len(ms) {
			t.Fatalf("community %d is disconnected: reached %d of %d members", c, len(seen), len(ms))
		}
	}
}

func clique(start uint, size int, weight float64) []WeightedEdge {
	var out []WeightedEdge
	for i := 0; i < size; i++ {
		for j := i + 1; j < size; j++ {
			out = append(out, WeightedEdge{A: start + uint(i), B: start + uint(j), Weight: weight})
		}
	}
	return out
}

func TestLeiden_Empty(t *testing.T) {
	if got := LeidenCommunities(nil, LeidenResolution, 1); len(got) != 0 {
		t.Fatalf("expected empty assignment, got %v", got)
	}
}

func TestLeiden_SingleEdge(t *testing.T) {
	got := LeidenCommunities([]WeightedEdge{{A: 1, B: 2, Weight: 1}}, LeidenResolution, 1)
	if len(got) != 2 || got[1] != got[2] {
		t.Fatalf("two connected nodes should share one community: %v", got)
	}
	if got[1] != 0 {
		t.Fatalf("community ids must be dense from 0: %v", got)
	}
}

func TestLeiden_TwoDisconnectedTriangles(t *testing.T) {
	edges := append(clique(1, 3, 1), clique(10, 3, 1)...)
	got := LeidenCommunities(edges, LeidenResolution, 1)

	if got[1] != got[2] || got[2] != got[3] {
		t.Fatalf("first triangle split: %v", got)
	}
	if got[10] != got[11] || got[11] != got[12] {
		t.Fatalf("second triangle split: %v", got)
	}
	if got[1] == got[10] {
		t.Fatalf("disconnected triangles must not share a community: %v", got)
	}
	// Deterministic numbering: smallest member ID first.
	if got[1] != 0 || got[10] != 1 {
		t.Fatalf("communities not numbered by smallest member: %v", got)
	}
}

func TestLeiden_RingOfCliques(t *testing.T) {
	// Four 5-cliques joined in a ring by single light bridges — the classic
	// resolution benchmark. At gamma=1 each clique is its own community.
	var edges []WeightedEdge
	starts := []uint{1, 101, 201, 301}
	for _, s := range starts {
		edges = append(edges, clique(s, 5, 1)...)
	}
	for i, s := range starts {
		nextStart := starts[(i+1)%len(starts)]
		edges = append(edges, WeightedEdge{A: s, B: nextStart, Weight: 1})
	}

	got := LeidenCommunities(edges, LeidenResolution, 7)
	assertConnectedCommunities(t, edges, got)

	commSet := map[int]bool{}
	for _, c := range got {
		commSet[c] = true
	}
	if len(commSet) != 4 {
		t.Fatalf("expected 4 communities (one per clique), got %d: %v", len(commSet), got)
	}
	for _, s := range starts {
		for i := uint(1); i < 5; i++ {
			if got[s] != got[s+i] {
				t.Fatalf("clique starting at %d split across communities: %v", s, got)
			}
		}
	}
}

func TestLeiden_WeightedClusters(t *testing.T) {
	// Two heavy 4-cliques connected by several LIGHT cross edges: weight,
	// not topology, must drive the split.
	edges := append(clique(1, 4, 10), clique(11, 4, 10)...)
	for i := uint(0); i < 4; i++ {
		edges = append(edges, WeightedEdge{A: 1 + i, B: 11 + i, Weight: 0.1})
	}
	got := LeidenCommunities(edges, LeidenResolution, 3)
	assertConnectedCommunities(t, edges, got)
	if got[1] == got[11] {
		t.Fatalf("heavy clusters should separate despite light bridges: %v", got)
	}
	for i := uint(1); i < 4; i++ {
		if got[1] != got[1+i] || got[11] != got[11+i] {
			t.Fatalf("cluster split: %v", got)
		}
	}
}

// zacharyEdges is the standard 34-node, 78-edge karate club graph (1-indexed).
func zacharyEdges() []WeightedEdge {
	pairs := [][2]uint{
		{1, 2}, {1, 3}, {1, 4}, {1, 5}, {1, 6}, {1, 7}, {1, 8}, {1, 9}, {1, 11}, {1, 12},
		{1, 13}, {1, 14}, {1, 18}, {1, 20}, {1, 22}, {1, 32},
		{2, 3}, {2, 4}, {2, 8}, {2, 14}, {2, 18}, {2, 20}, {2, 22}, {2, 31},
		{3, 4}, {3, 8}, {3, 9}, {3, 10}, {3, 14}, {3, 28}, {3, 29}, {3, 33},
		{4, 8}, {4, 13}, {4, 14},
		{5, 7}, {5, 11},
		{6, 7}, {6, 11}, {6, 17},
		{7, 17},
		{9, 31}, {9, 33}, {9, 34},
		{10, 34},
		{14, 34},
		{15, 33}, {15, 34},
		{16, 33}, {16, 34},
		{19, 33}, {19, 34},
		{20, 34},
		{21, 33}, {21, 34},
		{23, 33}, {23, 34},
		{24, 26}, {24, 28}, {24, 30}, {24, 33}, {24, 34},
		{25, 26}, {25, 28}, {25, 32},
		{26, 32},
		{27, 30}, {27, 34},
		{28, 34},
		{29, 32}, {29, 34},
		{30, 33}, {30, 34},
		{31, 33}, {31, 34},
		{32, 33}, {32, 34},
		{33, 34},
	}
	out := make([]WeightedEdge, len(pairs))
	for i, p := range pairs {
		out[i] = WeightedEdge{A: p[0], B: p[1], Weight: 1}
	}
	return out
}

func TestLeiden_ZacharyKarateClub(t *testing.T) {
	edges := zacharyEdges()
	if len(edges) != 78 {
		t.Fatalf("fixture error: expected 78 edges, have %d", len(edges))
	}
	got := LeidenCommunities(edges, LeidenResolution, 42)
	assertConnectedCommunities(t, edges, got)

	q := modularityOf(edges, got)
	// Known optimum at gamma=1 is ~0.4198; a correct Leiden lands >= 0.40.
	if q < 0.40 {
		t.Fatalf("modularity %v below 0.40 — partition quality is off", q)
	}
	// The two faction leaders (instructor=1, president=34) famously split.
	if got[1] == got[34] {
		t.Fatalf("faction leaders 1 and 34 should be in different communities: %v", got)
	}
	commSet := map[int]bool{}
	for _, c := range got {
		commSet[c] = true
	}
	if n := len(commSet); n < 2 || n > 6 {
		t.Fatalf("expected 2-6 communities on the karate club, got %d", n)
	}
}

func TestLeiden_Deterministic(t *testing.T) {
	edges := zacharyEdges()
	a := LeidenCommunities(edges, LeidenResolution, 42)
	b := LeidenCommunities(edges, LeidenResolution, 42)
	if len(a) != len(b) {
		t.Fatalf("size differs across runs: %d vs %d", len(a), len(b))
	}
	for id, c := range a {
		if b[id] != c {
			t.Fatalf("node %d differs across identical runs: %d vs %d", id, c, b[id])
		}
	}
}

func TestLeiden_ResolutionIncreasesCommunities(t *testing.T) {
	edges := zacharyEdges()
	count := func(gamma float64) int {
		got := LeidenCommunities(edges, gamma, 42)
		set := map[int]bool{}
		for _, c := range got {
			set[c] = true
		}
		return len(set)
	}
	low, high := count(0.5), count(4.0)
	if high < low {
		t.Fatalf("higher resolution should not produce fewer communities: gamma=0.5→%d, gamma=4.0→%d", low, high)
	}
	if high <= 1 {
		t.Fatalf("gamma=4.0 should split the karate club into multiple communities, got %d", high)
	}
}

func TestLeiden_ConnectivityGuaranteeUnderStress(t *testing.T) {
	// A deterministic pseudo-random graph (LCG) — 120 nodes, ~600 edges —
	// swept across seeds. Every community must come back connected; this is
	// the property Louvain violates and Leiden guarantees.
	var edges []WeightedEdge
	state := uint64(12345)
	next := func(mod uint64) uint64 {
		state = state*6364136223846793005 + 1442695040888963407
		return (state >> 33) % mod
	}
	seen := map[EdgeKey]bool{}
	for len(edges) < 600 {
		a := uint(next(120)) + 1
		b := uint(next(120)) + 1
		if a == b {
			continue
		}
		k := canonicalKey(a, b)
		if seen[k] {
			continue
		}
		seen[k] = true
		w := float64(next(10)+1) / 10.0
		edges = append(edges, WeightedEdge{A: a, B: b, Weight: w})
	}

	for seed := int64(1); seed <= 5; seed++ {
		got := LeidenCommunities(edges, LeidenResolution, seed)
		assertConnectedCommunities(t, edges, got)
		if len(got) == 0 {
			t.Fatalf("seed %d produced empty assignment", seed)
		}
	}
}

func TestLeiden_InputHygiene(t *testing.T) {
	got := LeidenCommunities([]WeightedEdge{
		{A: 1, B: 1, Weight: 5},  // self-loop ignored
		{A: 1, B: 2, Weight: 0},  // zero weight ignored
		{A: 1, B: 2, Weight: -1}, // negative ignored
		{A: 1, B: 2, Weight: 1},
		{A: 2, B: 1, Weight: 1}, // parallel edge collapses
	}, LeidenResolution, 1)
	if len(got) != 2 || got[1] != got[2] {
		t.Fatalf("hygiene handling broke the trivial partition: %v", got)
	}
}

func BenchmarkLeiden_Zachary(b *testing.B) {
	edges := zacharyEdges()
	for i := 0; i < b.N; i++ {
		LeidenCommunities(edges, LeidenResolution, 42)
	}
}

func ExampleLeidenCommunities() {
	edges := append(clique(1, 3, 1), clique(10, 3, 1)...)
	got := LeidenCommunities(edges, LeidenResolution, 1)
	fmt.Println(got[1] == got[2], got[1] == got[10])
	// Output: true false
}
