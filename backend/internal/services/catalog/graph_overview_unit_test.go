package catalog

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"math"
	"strconv"
	"testing"
	"time"

	"psychic-homily-backend/internal/services/contracts"
)

// =============================================================================
// UNIT TESTS (no database)
// =============================================================================
//
// These cover the pure transforms the snapshot's stability contract rests on:
// seeding, Procrustes alignment, quantization, centrality rank, and hulls. The
// pipeline that wires them together is exercised against a real database in
// graph_overview_test.go.

// --- seedLayoutPositions ---

func TestSeedLayoutPositions_ReturningNodeResumesExactly(t *testing.T) {
	ids := []uint{10, 20, 30}
	neighbors := [][]int32{{1}, {0, 2}, {1}}
	previous := map[uint]layoutPoint{
		10: {X: 1.5, Y: -2.25},
		20: {X: 4, Y: 8},
		30: {X: -7, Y: 0.5},
	}

	got := seedLayoutPositions(ids, neighbors, previous)

	for i, id := range ids {
		if got[i] != previous[id] {
			t.Errorf("node %d seeded at %+v, want its stored position %+v", id, got[i], previous[id])
		}
	}
}

func TestSeedLayoutPositions_NewNodeLandsAtNeighborBarycenter(t *testing.T) {
	// Node index 2 is new and adjacent to two placed nodes at (0,0) and (10,20);
	// its barycenter is (5,10) plus the deterministic anti-collision jitter.
	ids := []uint{1, 2, 3}
	neighbors := [][]int32{{2}, {2}, {0, 1}}
	previous := map[uint]layoutPoint{
		1: {X: 0, Y: 0},
		2: {X: 10, Y: 20},
	}

	got := seedLayoutPositions(ids, neighbors, previous)

	if math.Abs(float64(got[2].X)-5) > 0.01 || math.Abs(float64(got[2].Y)-10) > 0.01 {
		t.Errorf("new node seeded at %+v, want ~(5, 10)", got[2])
	}
}

func TestSeedLayoutPositions_ColdStartPlacesEveryNodeDistinctly(t *testing.T) {
	// ForceAtlas2 divides by inter-node distance, so a cold start that put two
	// nodes at the same point would produce a non-finite force. The spiral plus
	// per-index jitter exists to make that impossible.
	const n = 200
	ids := make([]uint, n)
	neighbors := make([][]int32, n)
	for i := range ids {
		ids[i] = uint(i + 1)
	}

	got := seedLayoutPositions(ids, neighbors, nil)

	seen := make(map[layoutPoint]int, n)
	for i, p := range got {
		if !isFinitePoint(p) {
			t.Fatalf("node %d seeded at a non-finite position %+v", i, p)
		}
		if prev, dup := seen[p]; dup {
			t.Fatalf("nodes %d and %d seeded at the same point %+v", prev, i, p)
		}
		seen[p] = i
	}
}

func TestSeedLayoutPositions_IsDeterministic(t *testing.T) {
	ids := []uint{5, 6, 7, 8}
	neighbors := [][]int32{{1}, {0, 2}, {1, 3}, {2}}
	previous := map[uint]layoutPoint{5: {X: 3, Y: 4}}

	first := seedLayoutPositions(ids, neighbors, previous)
	second := seedLayoutPositions(ids, neighbors, previous)

	for i := range first {
		if first[i] != second[i] {
			t.Fatalf("seed %d differed between runs: %+v vs %+v", i, first[i], second[i])
		}
	}
}

// --- Procrustes ---

func TestSolveProcrustes_UndoesRotationScaleAndTranslation(t *testing.T) {
	source := []layoutPoint{{X: 1, Y: 0}, {X: 0, Y: 1}, {X: -1, Y: 0}, {X: 0, Y: -1}, {X: 2, Y: 3}}
	// A 30-degree rotation, 2.5x scale, and a translation — exactly the class of
	// drift ForceAtlas2 is free to introduce between two otherwise-equal runs.
	const angle = math.Pi / 6
	const scale = 2.5
	target := make([]layoutPoint, len(source))
	for i, p := range source {
		x, y := float64(p.X), float64(p.Y)
		target[i] = layoutPoint{
			X: float32(scale*(math.Cos(angle)*x-math.Sin(angle)*y) + 11),
			Y: float32(scale*(math.Sin(angle)*x+math.Cos(angle)*y) - 4),
		}
	}

	// Solve source -> target, then check applying it to source lands on target.
	tr := solveProcrustes(source, target)
	for i, p := range source {
		got := tr.apply(p)
		if math.Abs(float64(got.X-target[i].X)) > 1e-3 || math.Abs(float64(got.Y-target[i].Y)) > 1e-3 {
			t.Errorf("point %d mapped to %+v, want %+v", i, got, target[i])
		}
	}
}

func TestSolveProcrustes_RecoversAReflection(t *testing.T) {
	// A layout that comes back mirrored is the same map; refusing the reflection
	// would leave every node maximally displaced.
	source := []layoutPoint{{X: 1, Y: 0}, {X: 0, Y: 2}, {X: -3, Y: 1}, {X: 2, Y: -1}}
	target := make([]layoutPoint, len(source))
	for i, p := range source {
		target[i] = layoutPoint{X: p.X, Y: -p.Y}
	}

	tr := solveProcrustes(source, target)

	det := tr.R[0]*tr.R[3] - tr.R[1]*tr.R[2]
	if det >= 0 {
		t.Errorf("determinant %v, want negative (a reflection)", det)
	}
	for i, p := range source {
		got := tr.apply(p)
		if math.Abs(float64(got.X-target[i].X)) > 1e-4 || math.Abs(float64(got.Y-target[i].Y)) > 1e-4 {
			t.Errorf("point %d mapped to %+v, want %+v", i, got, target[i])
		}
	}
}

func TestSolveProcrustes_DegenerateInputsStayFinite(t *testing.T) {
	identical := []layoutPoint{{X: 2, Y: 2}, {X: 2, Y: 2}, {X: 2, Y: 2}}
	target := []layoutPoint{{X: 5, Y: 5}, {X: 5, Y: 5}, {X: 5, Y: 5}}

	tr := solveProcrustes(identical, target)

	got := tr.apply(layoutPoint{X: 2, Y: 2})
	if !isFinitePoint(got) {
		t.Fatalf("degenerate fit produced %+v", got)
	}
	if tr.Scale <= 0 || math.IsNaN(tr.Scale) || math.IsInf(tr.Scale, 0) {
		t.Errorf("scale = %v, want a positive finite fallback", tr.Scale)
	}
}

func TestAlignToPrevious_PutsReturningNodesBackWhereTheyWere(t *testing.T) {
	ids := []uint{1, 2, 3, 4}
	previous := map[uint]layoutPoint{
		1: {X: 0, Y: 0},
		2: {X: 10, Y: 0},
		3: {X: 10, Y: 10},
		4: {X: 0, Y: 10},
	}
	// The "fresh layout": the same square, rotated 90 degrees and shifted. The
	// alignment must map it back onto the previous epoch exactly.
	fresh := []layoutPoint{{X: 100, Y: 0}, {X: 100, Y: 10}, {X: 90, Y: 10}, {X: 90, Y: 0}}

	got := alignToPrevious(ids, fresh, previous)

	for i, id := range ids {
		want := previous[id]
		if math.Abs(float64(got[i].X-want.X)) > 1e-3 || math.Abs(float64(got[i].Y-want.Y)) > 1e-3 {
			t.Errorf("node %d aligned to %+v, want %+v", id, got[i], want)
		}
	}
}

func TestAlignToPrevious_ColdStartIsAPassThrough(t *testing.T) {
	points := []layoutPoint{{X: 1, Y: 2}, {X: 3, Y: 4}}

	got := alignToPrevious([]uint{1, 2}, points, nil)

	for i := range points {
		if got[i] != points[i] {
			t.Errorf("point %d changed on a cold start: %+v", i, got[i])
		}
	}
}

// --- quantization ---

func TestQuantizePositions_UsesTheFullInt16Range(t *testing.T) {
	points := []layoutPoint{{X: -8, Y: 0}, {X: 4, Y: 8}, {X: 0, Y: -2}}

	xs, ys, extent := quantizePositions(points)

	if extent != 8 {
		t.Fatalf("extent = %v, want 8 (the largest absolute coordinate)", extent)
	}
	if xs[0] != -contracts.GraphOverviewCoordinateScale {
		t.Errorf("x[0] = %d, want the negative full scale", xs[0])
	}
	if ys[1] != contracts.GraphOverviewCoordinateScale {
		t.Errorf("y[1] = %d, want the positive full scale", ys[1])
	}
	// 4/8 of full scale, allowing for the rounding of the half-scale value.
	if want := int16(contracts.GraphOverviewCoordinateScale / 2); xs[1] < want-1 || xs[1] > want+1 {
		t.Errorf("x[1] = %d, want ~%d", xs[1], want)
	}
}

func TestQuantizePositions_DegenerateMapDoesNotDivideByZero(t *testing.T) {
	points := []layoutPoint{{X: 0, Y: 0}, {X: 0, Y: 0}}

	xs, ys, extent := quantizePositions(points)

	if extent != 1 {
		t.Errorf("extent = %v, want the 1 fallback", extent)
	}
	for i := range points {
		if xs[i] != 0 || ys[i] != 0 {
			t.Errorf("point %d quantized to (%d, %d), want the origin", i, xs[i], ys[i])
		}
	}
}

// --- centrality ---

func TestBetweennessCentrality_RanksTheBridgeHighest(t *testing.T) {
	// Two triangles joined through node 3: 0-1-2-3-4-5-6, with 3 the only path
	// between the halves.
	neighbors := [][]int32{
		{1, 2},
		{0, 2},
		{0, 1, 3},
		{2, 4},
		{3, 5, 6},
		{4, 6},
		{4, 5},
	}

	scores, metric := betweennessCentrality(neighbors)
	ranks := rankByScore(scores)

	if metric != contracts.GraphOverviewRankBetweenness {
		t.Errorf("metric = %q, want exact betweenness below the pivot threshold", metric)
	}

	if ranks[3] != 0 {
		t.Errorf("bridge node rank = %d, want 0 (highest betweenness)", ranks[3])
	}
	if scores[0] >= scores[3] {
		t.Errorf("leaf-ish node scored %v against the bridge's %v", scores[0], scores[3])
	}
}

func TestBetweennessCentrality_EmptyGraph(t *testing.T) {
	if got, _ := betweennessCentrality(nil); len(got) != 0 {
		t.Errorf("scores = %v, want empty", got)
	}
}

func TestBetweennessCentrality_ReportsSamplingAboveTheThreshold(t *testing.T) {
	// Above the threshold the ranks come from pivot sources, and the payload has
	// to say so — a silently approximate label tiering is the failure mode the
	// RankMetric field exists to prevent.
	n := betweennessPivotThreshold + 1
	neighbors := make([][]int32, n)
	for i := 0; i < n-1; i++ {
		neighbors[i] = append(neighbors[i], int32(i+1))
		neighbors[i+1] = append(neighbors[i+1], int32(i))
	}

	scores, metric := betweennessCentrality(neighbors)

	if metric != contracts.GraphOverviewRankBetweennessSampled {
		t.Errorf("metric = %q, want the sampled marker", metric)
	}
	if len(scores) != n {
		t.Errorf("got %d scores, want %d", len(scores), n)
	}
}

func TestRankByScore_TiesBreakByIndexSoRebuildsAgree(t *testing.T) {
	// Most of the sparse tail scores exactly 0; without an index tiebreak those
	// nodes would swap label tiers between otherwise-identical rebuilds.
	scores := []float64{0, 5, 0, 5, 0}

	ranks := rankByScore(scores)

	if ranks[1] != 0 || ranks[3] != 1 {
		t.Errorf("tied high scores ranked %d and %d, want 0 and 1 in index order", ranks[1], ranks[3])
	}
	if ranks[0] != 2 || ranks[2] != 3 || ranks[4] != 4 {
		t.Errorf("tied zero scores ranked %d, %d, %d; want 2, 3, 4 in index order", ranks[0], ranks[2], ranks[4])
	}
}

// --- hulls ---

func TestConvexHull_WrapsThePointSetAndDropsInteriorPoints(t *testing.T) {
	points := []layoutPoint{
		{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 10, Y: 10}, {X: 0, Y: 10},
		{X: 5, Y: 5}, // interior — must not appear on the hull
	}

	hull := convexHull(points)

	if len(hull) != 4 {
		t.Fatalf("hull has %d vertices, want the 4 corners: %+v", len(hull), hull)
	}
	for _, p := range hull {
		if p.X == 5 && p.Y == 5 {
			t.Error("interior point appeared on the hull")
		}
	}
}

func TestConvexHull_RefusesDegenerateInput(t *testing.T) {
	if got := convexHull([]layoutPoint{{X: 0, Y: 0}, {X: 1, Y: 1}}); got != nil {
		t.Errorf("two points produced a hull %+v, want nil", got)
	}
	collinear := []layoutPoint{{X: 0, Y: 0}, {X: 1, Y: 1}, {X: 2, Y: 2}, {X: 3, Y: 3}}
	if got := convexHull(collinear); got != nil {
		t.Errorf("collinear points produced a hull %+v, want nil", got)
	}
}

func TestPadHull_PushesVerticesOutwardFromTheCentroid(t *testing.T) {
	hull := []layoutPoint{{X: -10, Y: -10}, {X: 10, Y: -10}, {X: 10, Y: 10}, {X: -10, Y: 10}}

	padded := padHull(hull, 0.1)

	for i, p := range padded {
		if math.Abs(float64(p.X)) < math.Abs(float64(hull[i].X)) {
			t.Errorf("vertex %d moved inward on x: %v -> %v", i, hull[i].X, p.X)
		}
		if want := float32(11); math.Abs(math.Abs(float64(p.X))-float64(want)) > 1e-3 {
			t.Errorf("vertex %d |x| = %v, want %v", i, p.X, want)
		}
	}
}

func TestHullIsDegenerate(t *testing.T) {
	square := []layoutPoint{{X: -10, Y: -10}, {X: 10, Y: -10}, {X: 10, Y: 10}, {X: -10, Y: 10}}
	if hullIsDegenerate(square, 10) {
		t.Error("a full-extent square was called degenerate")
	}
	sliver := []layoutPoint{{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 10, Y: 1e-9}}
	if !hullIsDegenerate(sliver, 10) {
		t.Error("a zero-area sliver was not called degenerate")
	}
	if !hullIsDegenerate([]layoutPoint{{X: 0, Y: 0}, {X: 1, Y: 1}}, 10) {
		t.Error("a two-point polygon was not called degenerate")
	}
}

// --- appear times ---

func TestAppearSeconds_ClampsBelowTheEpoch(t *testing.T) {
	if got := appearSeconds(graphOverviewEpoch.Add(-time.Hour * 24 * 365)); got != 0 {
		t.Errorf("pre-epoch time = %d, want 0", got)
	}
	if got := appearSeconds(graphOverviewEpoch); got != 0 {
		t.Errorf("the epoch itself = %d, want 0", got)
	}
	if got := appearSeconds(graphOverviewEpoch.Add(90 * time.Second)); got != 90 {
		t.Errorf("epoch+90s = %d, want 90", got)
	}
}

// --- backbone ordering ---

func TestGraphOverviewBackbone_IsOrderedCanonically(t *testing.T) {
	// A path graph: every node has degree 1 or 2, so the disparity filter keeps
	// the leaf edges (significance 0) and the result is non-empty.
	edges := []WeightedEdge{
		{A: 30, B: 40, Weight: 1},
		{A: 10, B: 20, Weight: 1},
		{A: 20, B: 30, Weight: 1},
	}

	first := graphOverviewBackbone(edges)
	second := graphOverviewBackbone(edges)

	if len(first) == 0 {
		t.Fatal("backbone is empty; the test graph cannot exercise the ordering")
	}
	for i := range first {
		if first[i] != second[i] {
			t.Fatalf("edge %d differed between runs: %+v vs %+v", i, first[i], second[i])
		}
		if first[i].A > first[i].B {
			t.Errorf("edge %d is not in canonical (min,max) order: %+v", i, first[i])
		}
		if i > 0 && (first[i-1].A > first[i].A || (first[i-1].A == first[i].A && first[i-1].B > first[i].B)) {
			t.Errorf("edges %d and %d are out of ascending order: %+v, %+v", i-1, i, first[i-1], first[i])
		}
	}
}

func TestBackboneArtistIDs_AreAscendingAndDeduplicated(t *testing.T) {
	edges := []overviewEdge{{A: 5, B: 9}, {A: 2, B: 5}, {A: 9, B: 12}}

	got := backboneArtistIDs(edges)

	want := []uint{2, 5, 9, 12}
	if len(got) != len(want) {
		t.Fatalf("ids = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("ids = %v, want %v", got, want)
		}
	}
}

// --- the hub city column ---

// hubCityBuild indexes two artists and three hubs, so the column's SCOPE (hub
// indexes only) and its trimming are both visible in one assertion.
func hubCityBuild(cities ...string) *overviewBuild {
	slugA, slugB := "band-a", "band-b"
	artistIDs := []uint{1, 2}
	artistMeta := map[uint]overviewArtist{
		1: {ID: 1, Name: "Band A", Slug: &slugA},
		2: {ID: 2, Name: "Band B", Slug: &slugB},
	}
	hubs := labelHubs{}
	for i, city := range cities {
		hubs.Nodes = append(hubs.Nodes, contracts.SceneGraphNode{
			ID:         uint(labelHubNodeIDOffset + i + 1),
			EntityType: contracts.SceneNodeKindLabel,
			Name:       "Label " + strconv.Itoa(i+1),
			Slug:       "label-" + strconv.Itoa(i+1),
			City:       city,
		})
	}
	return newOverviewBuild(artistIDs, artistMeta, hubs, []overviewEdge{{A: 1, B: 2, Weight: 1}})
}

func TestOverviewBuild_HubCityIsHubScopedAndTrimmed(t *testing.T) {
	// The column is the HUB CAPTION, not a location column: an artist index is
	// empty because the map does not carry artist cities, and a hub index is
	// empty when its label has none on file. A regression that filled artist
	// indexes would caption the wrong nodes with no error anywhere.
	//
	// Whitespace is normalized here rather than at the caption, so " " and ""
	// are the same absent city for every client.
	b := hubCityBuild(" Austin ", "   ", "")

	want := []string{"", "", "Austin", "", ""}
	if len(b.nodeHubCity) != len(want) {
		t.Fatalf("hub city column has %d entries, want %d (one per node)", len(b.nodeHubCity), len(want))
	}
	for i := range want {
		if b.nodeHubCity[i] != want[i] {
			t.Errorf("hub city[%d] = %q, want %q", i, b.nodeHubCity[i], want[i])
		}
	}

	// And it reaches the payload at full node length, which is the invariant
	// every columnar consumer indexes against.
	n := len(b.nodeIDs)
	payload := b.payload(time.Now(), 1, make([]int16, n), make([]int16, n),
		make([]int32, n), contracts.GraphOverviewRankBetweenness, make([]int32, n),
		make([]uint8, n), nil, 0)
	if len(payload.Nodes.HubCity) != payload.NodeCount {
		t.Errorf("payload hub_city has %d entries, want node_count = %d",
			len(payload.Nodes.HubCity), payload.NodeCount)
	}
}

func TestOverviewBuild_HubCityDoesNotEnterTheStructureKey(t *testing.T) {
	// THE STABILITY CONTRACT, at the cheapest place to state it. The structure
	// key is what lets an unchanged night replay stored positions instead of
	// re-relaxing them; if an additive payload column entered it, shipping this
	// field — and every label edit afterwards — would re-lay-out the whole map
	// and reshuffle it under the reader.
	unset := hubCityBuild("", "", "")
	set := hubCityBuild("Austin", "Brooklyn", "London")

	if unset.structureKey() != set.structureKey() {
		t.Fatalf("hub cities changed the structure key (%s vs %s); an attribute must not move a dot",
			unset.structureKey(), set.structureKey())
	}
}

// --- the wire format ---

func TestGraphOverviewWireFormat_KindColumnsAreBase64Bytes(t *testing.T) {
	// Go writes any []uint8 as a base64 string, so the `kind` columns are
	// strings on the wire while every other column is a JSON array. That
	// asymmetry is documented in the contract and consumed by the client, so it
	// is pinned here: changing these fields to a numeric type is a WIRE BREAK
	// and has to break this test to happen.
	payload := contracts.GraphOverview{
		NodeCount: 3,
		EdgeCount: 1,
		Nodes: contracts.GraphOverviewNodes{
			Kind: []uint8{
				contracts.GraphOverviewNodeArtist,
				contracts.GraphOverviewNodeArtist,
				contracts.GraphOverviewNodeLabelHub,
			},
			X: []int16{1, 2, 3},
		},
		Edges: contracts.GraphOverviewEdges{
			Kind: []uint8{contracts.GraphOverviewEdgeSimilarity, contracts.GraphOverviewEdgeLabelSpoke},
		},
	}

	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var wire struct {
		Nodes struct {
			Kind string  `json:"kind"`
			X    []int16 `json:"x"`
		} `json:"nodes"`
		Edges struct {
			Kind string `json:"kind"`
		} `json:"edges"`
	}
	if err := json.Unmarshal(encoded, &wire); err != nil {
		t.Fatalf("the kind columns are not strings on the wire: %v", err)
	}

	gotNodeKinds, err := base64.StdEncoding.DecodeString(wire.Nodes.Kind)
	if err != nil {
		t.Fatalf("nodes.kind is not base64: %v", err)
	}
	if len(gotNodeKinds) != payload.NodeCount {
		t.Errorf("decoded %d node kinds, want node_count = %d", len(gotNodeKinds), payload.NodeCount)
	}
	if gotNodeKinds[2] != contracts.GraphOverviewNodeLabelHub {
		t.Errorf("decoded node kind = %d, want the label-hub constant", gotNodeKinds[2])
	}

	gotEdgeKinds, err := base64.StdEncoding.DecodeString(wire.Edges.Kind)
	if err != nil {
		t.Fatalf("edges.kind is not base64: %v", err)
	}
	if len(gotEdgeKinds) != 2*payload.EdgeCount {
		t.Errorf("decoded %d edge kinds, want 2 * edge_count = %d", len(gotEdgeKinds), 2*payload.EdgeCount)
	}

	// The other columns stay real arrays; only kind is a string.
	if len(wire.Nodes.X) != 3 {
		t.Errorf("nodes.x decoded to %v, want a 3-element array", wire.Nodes.X)
	}

	// And the whole thing round-trips back into the Go type unchanged, which is
	// what the snapshot read path depends on.
	var back contracts.GraphOverview
	if err := json.Unmarshal(encoded, &back); err != nil {
		t.Fatalf("round trip: %v", err)
	}
	for i := range payload.Nodes.Kind {
		if back.Nodes.Kind[i] != payload.Nodes.Kind[i] {
			t.Fatalf("node kind %d round-tripped to %d, want %d", i, back.Nodes.Kind[i], payload.Nodes.Kind[i])
		}
	}
}

// --- the nightly step's failure posture ---

func TestRunGraphOverviewSnapshot_IsNonFatal(t *testing.T) {
	// The snapshot is the last step of the affinity cycle and must never take
	// the cycle down with it: the table is append-only, so a failure leaves the
	// previous map live and there is nothing to unwind.
	svc := &RadioFetchService{
		radioService: &RadioService{db: nil},
		stopCh:       make(chan struct{}),
		logger:       testLogger(),
	}

	// Should log and return, not panic.
	svc.runGraphOverviewSnapshot(context.Background())
}

// --- chunking ---

func TestChunkIDs(t *testing.T) {
	ids := []uint{1, 2, 3, 4, 5, 6, 7}

	// Under the size: one batch, and the same backing slice.
	if got := chunkIDs(ids, 10); len(got) != 1 || len(got[0]) != 7 {
		t.Fatalf("chunkIDs(7 ids, 10) = %v, want one batch of 7", got)
	}
	// Exactly the size: still one batch, not two with an empty tail.
	if got := chunkIDs(ids, 7); len(got) != 1 {
		t.Fatalf("chunkIDs(7 ids, 7) = %v, want one batch", got)
	}
	// Over: split with the remainder in the last batch, order preserved, and
	// every id present exactly once.
	got := chunkIDs(ids, 3)
	if len(got) != 3 {
		t.Fatalf("chunkIDs(7 ids, 3) produced %d batches, want 3", len(got))
	}
	var flat []uint
	for _, batch := range got {
		if len(batch) == 0 {
			t.Fatal("chunkIDs produced an empty batch")
		}
		flat = append(flat, batch...)
	}
	if len(flat) != len(ids) {
		t.Fatalf("chunkIDs lost ids: %v", flat)
	}
	for i := range ids {
		if flat[i] != ids[i] {
			t.Fatalf("chunkIDs reordered ids: %v", flat)
		}
	}
	// Degenerate size must not spin.
	if got := chunkIDs(ids, 0); len(got) != 1 {
		t.Fatalf("chunkIDs(ids, 0) = %v, want a single batch", got)
	}
}

// --- node-set filtering ---

func TestFilterBackboneToArtists_DropsEdgesWithAnUnknownEndpoint(t *testing.T) {
	// A relationship whose endpoint has no artist row would otherwise become a
	// nameless node. Dropping it must happen BEFORE the hub builder sees the
	// set, because that set is the hub scope definition.
	known := map[uint]overviewArtist{
		1: {ID: 1, Name: "One"},
		2: {ID: 2, Name: "Two"},
	}
	edges := []overviewEdge{
		{A: 1, B: 2, Weight: 1},
		{A: 2, B: 99, Weight: 1},
		{A: 99, B: 100, Weight: 1},
	}

	got := filterBackboneToArtists(edges, known)

	if len(got) != 1 {
		t.Fatalf("kept %d edges, want 1: %+v", len(got), got)
	}
	if got[0].A != 1 || got[0].B != 2 {
		t.Errorf("kept %+v, want the 1-2 edge", got[0])
	}
}

func TestDropHubbedLabelEdges_KeepsAPairThatAlsoEncodesSomethingElse(t *testing.T) {
	hubs := labelHubs{
		labelsByArtist: map[uint]map[uint]struct{}{
			1: {7: {}},
			2: {7: {}},
			3: {7: {}},
		},
		hubbedLabels: map[uint]struct{}{7: {}},
	}
	edges := []overviewEdge{
		{A: 1, B: 2, Weight: 1}, // shared-label only, and hubbed -> dropped
		{A: 1, B: 3, Weight: 1}, // also a co-bill -> kept
	}
	labelOnly := map[EdgeKey]struct{}{
		canonicalKey(1, 2): {},
	}

	got := dropHubbedLabelEdges(edges, labelOnly, hubs)

	if len(got) != 1 {
		t.Fatalf("kept %d edges, want 1: %+v", len(got), got)
	}
	if got[0].A != 1 || got[0].B != 3 {
		t.Errorf("kept %+v, want the 1-3 edge (it carries a fact no spoke does)", got[0])
	}
}

func TestDropHubbedLabelEdges_NoPairSetIsAPassThrough(t *testing.T) {
	// The pair query is allowed to fail: the map then keeps the cliques the hubs
	// duplicate, which is uglier but correct.
	edges := []overviewEdge{{A: 1, B: 2, Weight: 1}, {A: 2, B: 3, Weight: 1}}

	got := dropHubbedLabelEdges(edges, nil, labelHubs{})

	if len(got) != len(edges) {
		t.Fatalf("kept %d edges, want all %d", len(got), len(edges))
	}
}

func isFinitePoint(p layoutPoint) bool {
	return !math.IsNaN(float64(p.X)) && !math.IsInf(float64(p.X), 0) &&
		!math.IsNaN(float64(p.Y)) && !math.IsInf(float64(p.Y), 0)
}
