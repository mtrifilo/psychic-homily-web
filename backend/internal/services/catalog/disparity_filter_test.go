package catalog

import (
	"math"
	"testing"
)

const eps = 1e-9

func sig(t *testing.T, m map[EdgeKey]float64, a, b uint) float64 {
	t.Helper()
	v, ok := m[canonicalKey(a, b)]
	if !ok {
		t.Fatalf("edge (%d,%d) missing from result", a, b)
	}
	return v
}

func TestDisparitySignificance_Empty(t *testing.T) {
	if got := DisparitySignificance(nil); len(got) != 0 {
		t.Fatalf("expected empty result, got %v", got)
	}
}

func TestDisparitySignificance_IgnoresSelfLoopsAndNonPositiveWeights(t *testing.T) {
	got := DisparitySignificance([]WeightedEdge{
		{A: 1, B: 1, Weight: 5}, // self-loop
		{A: 1, B: 2, Weight: 0}, // zero weight
		{A: 1, B: 3, Weight: -2}, // negative weight
		{A: 1, B: 2, Weight: 4}, // the one valid edge
	})
	if len(got) != 1 {
		t.Fatalf("expected exactly 1 valid edge, got %d: %v", len(got), got)
	}
	if _, ok := got[canonicalKey(1, 2)]; !ok {
		t.Fatalf("expected edge (1,2) to survive filtering")
	}
}

func TestDisparitySignificance_SingleEdgeBothDegreeOne(t *testing.T) {
	// Two artists linked only to each other: both degree-1 → significance 0 (always kept).
	got := DisparitySignificance([]WeightedEdge{{A: 1, B: 2, Weight: 7}})
	if v := sig(t, got, 1, 2); v != 0 {
		t.Fatalf("degree-1 edge should have significance 0, got %v", v)
	}
}

func TestDisparitySignificance_StarKeepsAllLeafLinks(t *testing.T) {
	// A hub with N leaves. The leaves are degree-1, so EVERY spoke is kept (significance 0) — this
	// is the niche-link preservation the disparity filter exists for: a global weight cutoff would
	// instead delete every one of these small-weight edges. (min over endpoints picks the leaf's 0.)
	got := DisparitySignificance([]WeightedEdge{
		{A: 100, B: 1, Weight: 1},
		{A: 100, B: 2, Weight: 1},
		{A: 100, B: 3, Weight: 1},
		{A: 100, B: 4, Weight: 1},
	})
	for _, leaf := range []uint{1, 2, 3, 4} {
		if v := sig(t, got, 100, leaf); v != 0 {
			t.Fatalf("spoke (100,%d) to a degree-1 leaf should be 0, got %v", leaf, v)
		}
	}
}

func TestDisparitySignificance_TriangleEqualWeights(t *testing.T) {
	// Each node has degree 2, strength 2, so every edge has p = 0.5 from both endpoints and
	// significance (1-0.5)^(2-1) = 0.5. Backbone at alpha < 0.5 keeps none; at alpha > 0.5 keeps all.
	got := DisparitySignificance([]WeightedEdge{
		{A: 1, B: 2, Weight: 1},
		{A: 2, B: 3, Weight: 1},
		{A: 1, B: 3, Weight: 1},
	})
	for _, e := range []EdgeKey{{1, 2}, {2, 3}, {1, 3}} {
		if v := sig(t, got, e[0], e[1]); math.Abs(v-0.5) > eps {
			t.Fatalf("triangle edge (%d,%d) significance = %v, want 0.5", e[0], e[1], v)
		}
	}
}

func TestDisparitySignificance_PrunesWeakHubEdgeKeepsStrongFraction(t *testing.T) {
	// The discriminating case. Both endpoints of the tested edges have degree > 1 (so neither side
	// trivially returns 0). A hub H links to A with a dominant weight and to B with a weak weight;
	// A and B each carry a second edge so they're degree 2.
	//
	//   H: edges to A(w=10), B(w=1)        → degree 2, strength 11
	//   A: edges to H(w=10), X(w=10)       → degree 2, strength 20
	//   B: edges to H(w=1),  Y(w=1)        → degree 2, strength 2
	//   X, Y: degree 1
	const (
		H = 1
		A = 2
		B = 3
		X = 4
		Y = 5
	)
	got := DisparitySignificance([]WeightedEdge{
		{A: H, B: A, Weight: 10},
		{A: H, B: B, Weight: 1},
		{A: A, B: X, Weight: 10},
		{A: B, B: Y, Weight: 1},
	})

	// H–A: from H p=10/11 → (1-10/11)^1 ≈ 0.0909; from A p=10/20=0.5 → (1-0.5)^1 = 0.5. min ≈ 0.0909.
	hA := sig(t, got, H, A)
	if math.Abs(hA-(1.0-10.0/11.0)) > 1e-6 {
		t.Fatalf("H-A significance = %v, want ≈ %v", hA, 1.0-10.0/11.0)
	}
	// H–B: from H p=1/11 → (1-1/11)^1 ≈ 0.909; from B p=1/2 → (1-0.5)^1 = 0.5. min = 0.5.
	hB := sig(t, got, H, B)
	if math.Abs(hB-0.5) > 1e-6 {
		t.Fatalf("H-B significance = %v, want 0.5", hB)
	}
	// The strong-fraction edge is far more significant (smaller) than the weak one.
	if !(hA < hB) {
		t.Fatalf("expected the dominant edge H-A (%v) to be more significant than H-B (%v)", hA, hB)
	}
	// At alpha = 0.1 the backbone keeps H-A and drops H-B — the filter's whole point.
	const alpha = 0.1
	if !(hA < alpha) {
		t.Fatalf("H-A should be in the backbone at alpha=%v", alpha)
	}
	if hB < alpha {
		t.Fatalf("H-B should be pruned at alpha=%v", alpha)
	}
}

func TestDisparitySignificance_SumsParallelEdges(t *testing.T) {
	// Two input edges for the same pair collapse into one undirected edge of summed weight, counted
	// once toward each node's degree.
	got := DisparitySignificance([]WeightedEdge{
		{A: 1, B: 2, Weight: 3},
		{A: 2, B: 1, Weight: 4}, // same pair, reversed order
	})
	if len(got) != 1 {
		t.Fatalf("parallel edges should collapse to one, got %d", len(got))
	}
	// Both still degree-1 after collapse → significance 0.
	if v := sig(t, got, 1, 2); v != 0 {
		t.Fatalf("collapsed edge significance = %v, want 0", v)
	}
}

func TestDisparitySignificance_CanonicalKeyOrderIndependent(t *testing.T) {
	a := DisparitySignificance([]WeightedEdge{{A: 9, B: 4, Weight: 2}, {A: 4, B: 7, Weight: 2}})
	b := DisparitySignificance([]WeightedEdge{{A: 4, B: 9, Weight: 2}, {A: 7, B: 4, Weight: 2}})
	if len(a) != len(b) {
		t.Fatalf("result size differs by input order: %d vs %d", len(a), len(b))
	}
	for k, va := range a {
		if vb, ok := b[k]; !ok || math.Abs(va-vb) > eps {
			t.Fatalf("edge %v differs by input order: %v vs %v (present=%v)", k, va, vb, ok)
		}
	}
}
