package catalog

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"time"

	"psychic-homily-backend/internal/services/contracts"
)

// ──────────────────────────────────────────────
// ForceAtlas2 layout bridge
// ──────────────────────────────────────────────
//
// The layout itself runs in a Node subprocess (backend/layout/fa2-layout.mjs)
// because there is no maintained pure-Go ForceAtlas2 with Barnes-Hut
// approximation. Everything that needs HISTORY lives here on the Go side —
// warm-start replay, barycenter seeding of new nodes, Procrustes alignment to
// the previous epoch, quantization — so the subprocess stays a pure,
// separately-testable function of its input.
//
// THE STABILITY CONTRACT. The map must not reshuffle nightly; a reader who
// learned where their corner of the scene sits has to find it there again. Four
// mechanisms, in the order they apply:
//
//  1. WARM START. A node present in the previous snapshot resumes at its stored
//     position, so an unchanged graph starts at its own fixed point.
//  2. BARYCENTER SEEDING. A new node starts at the mean of its already-placed
//     neighbours, so it arrives near where it belongs instead of flying in from
//     a random corner and dragging its neighbourhood with it.
//  3. FEW ITERATIONS. A warm run relaxes for graphOverviewWarmIterations, which
//     is enough to absorb the night's changes and not enough to re-derive the
//     whole layout. Layout quality is deliberately capped to buy stability.
//  4. PROCRUSTES ALIGNMENT. ForceAtlas2 has no preferred origin, orientation or
//     scale, so an otherwise-identical layout can come back translated,
//     rotated, reflected or resized. The best-fit similarity transform over the
//     RETURNING nodes is applied to every node before persisting, which removes
//     that free-floating drift.
//
// Determinism is a precondition for all four. The subprocess has no RNG, and
// every input here is built in a fixed order from a stable key, because
// ForceAtlas2 sums forces in node order and float addition is not associative —
// the same ULP discipline LoadCommunityGraphEdges' ORDER BY exists for.
//
// PRECISION IS FLOAT32 END TO END. graphology-layout-forceatlas2 stores
// coordinates in a Float32Array, so every value it returns is float32-rounded
// whatever we send. Positions are therefore stored as float32; replaying them
// is then an exact round trip, which is what makes a re-run over unchanged data
// reproduce byte-identical positions.

const (
	// graphOverviewWarmIterations is the relaxation budget for a warm run —
	// the stability-over-quality tradeoff above.
	graphOverviewWarmIterations = 50

	// graphOverviewColdIterations is the budget when there is no previous
	// snapshot to resume from. A cold run has no stability to protect and every
	// node starts on a seeded spiral, so it needs a real layout pass. Measured
	// at 5k nodes / 20k edges: ~10s, against ~1s for a warm run.
	graphOverviewColdIterations = 500

	// graphOverviewLayoutTimeout bounds the subprocess. A warm run at current
	// scale is ~1s and a cold run ~10s, so this is two orders of magnitude of
	// headroom for catalog growth while still guaranteeing the nightly cycle
	// cannot be parked forever on a wedged child process.
	graphOverviewLayoutTimeout = 10 * time.Minute

	// graphOverviewLayoutMaxOutput caps the bytes read back from the
	// subprocess. The response is two floats per node, so a 5k-node map is
	// ~200 KB; 64 MB is far past any plausible catalog and still bounds a
	// runaway child's ability to exhaust this process's memory.
	graphOverviewLayoutMaxOutput = 64 << 20

	// graphOverviewLayoutHeapMB caps the subprocess's V8 heap. Barnes-Hut at
	// 5k nodes needs single-digit MB; this leaves room for an order of
	// magnitude of growth and still fails the child fast instead of letting it
	// compete with the API server for the container's memory.
	graphOverviewLayoutHeapMB = 512
)

// layoutNode is one node as the subprocess wants it. Identity is the array
// index — ids never cross the boundary.
type layoutNode struct {
	X float32 `json:"x"`
	Y float32 `json:"y"`
	// Fixed pins a node in place. Unused by the nightly build today (it exists
	// in the wire format because the library supports it and the anchor case is
	// the obvious next use); layout_test asserts the attribute still works so a
	// dependency bump cannot silently drop it.
	Fixed bool `json:"fixed,omitempty"`
}

// layoutEdge references nodes by their index in the request's node array.
type layoutEdge struct {
	Source int     `json:"source"`
	Target int     `json:"target"`
	Weight float64 `json:"weight"`
}

type layoutRequest struct {
	Iterations int          `json:"iterations"`
	Nodes      []layoutNode `json:"nodes"`
	Edges      []layoutEdge `json:"edges"`
}

type layoutResponse struct {
	Positions [][2]float32 `json:"positions"`
}

// layoutPoint is a position in world units, at the float32 precision the
// layout library actually round-trips.
type layoutPoint struct {
	X float32
	Y float32
}

// graphLayoutRunner runs a ForceAtlas2 request. The nightly build uses
// nodeLayoutRunner; tests substitute a deterministic stub so the Go pipeline is
// testable without Node installed.
type graphLayoutRunner interface {
	Run(ctx context.Context, req layoutRequest) ([]layoutPoint, error)
}

// nodeLayoutRunner invokes backend/layout/fa2-layout.mjs as a subprocess.
type nodeLayoutRunner struct {
	// binary is the node executable; script is the layout entrypoint.
	binary string
	script string
}

// graphOverviewLayoutRunner resolves the layout subprocess's location.
//
// Both paths are operator configuration, never request data: nothing a client
// can influence reaches this function, and the command is built as a fixed argv
// with no shell, so there is no injection surface even if the environment were
// hostile. The defaults match the Docker image layout (WORKDIR /app, script
// vendored at /app/layout) and also work from `backend/` in local development.
func graphOverviewLayoutRunner() *nodeLayoutRunner {
	binary := os.Getenv("GRAPH_LAYOUT_NODE_BIN")
	if binary == "" {
		binary = "node"
	}
	script := os.Getenv("GRAPH_LAYOUT_SCRIPT")
	if script == "" {
		script = filepath.Join("layout", "fa2-layout.mjs")
	}
	return &nodeLayoutRunner{binary: binary, script: script}
}

// Run writes the request to the child's stdin and reads its positions back.
//
// The child gets a MINIMAL environment rather than this process's: it needs
// PATH to resolve its own interpreter bits and nothing else, and the server's
// environment holds database credentials and API secrets that a layout script
// has no business being able to read.
func (r *nodeLayoutRunner) Run(ctx context.Context, req layoutRequest) ([]layoutPoint, error) {
	if len(req.Nodes) == 0 {
		return nil, fmt.Errorf("layout request has no nodes")
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to encode layout request: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, graphOverviewLayoutTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, r.binary, r.script)
	cmd.Env = []string{
		"PATH=" + os.Getenv("PATH"),
		"NODE_OPTIONS=--max-old-space-size=" + strconv.Itoa(graphOverviewLayoutHeapMB),
	}
	cmd.Stdin = bytes.NewReader(body)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("layout subprocess exceeded %s: %w", graphOverviewLayoutTimeout, ctx.Err())
		}
		return nil, fmt.Errorf("layout subprocess failed: %w (stderr: %s)", err, truncateForLog(stderr.String()))
	}
	if stdout.Len() > graphOverviewLayoutMaxOutput {
		return nil, fmt.Errorf("layout subprocess wrote %d bytes, over the %d cap", stdout.Len(), graphOverviewLayoutMaxOutput)
	}

	var resp layoutResponse
	if err := json.Unmarshal(stdout.Bytes(), &resp); err != nil {
		return nil, fmt.Errorf("failed to decode layout response: %w", err)
	}
	if len(resp.Positions) != len(req.Nodes) {
		return nil, fmt.Errorf("layout returned %d positions for %d nodes", len(resp.Positions), len(req.Nodes))
	}

	out := make([]layoutPoint, len(resp.Positions))
	for i, p := range resp.Positions {
		x, y := p[0], p[1]
		if math.IsNaN(float64(x)) || math.IsInf(float64(x), 0) || math.IsNaN(float64(y)) || math.IsInf(float64(y), 0) {
			return nil, fmt.Errorf("layout returned a non-finite position for node %d", i)
		}
		out[i] = layoutPoint{X: x, Y: y}
	}
	return out, nil
}

// truncateForLog bounds a subprocess's stderr before it lands in an error
// string that will be logged and sent to Sentry.
func truncateForLog(s string) string {
	const max = 2000
	if len(s) <= max {
		return s
	}
	return s[:max] + "…(truncated)"
}

// seedLayoutPositions builds every node's starting position.
//
// nodeIDs must already be in the build's stable order, and neighbors[i] must
// list node INDEXES adjacent to node i in the same edge set the layout will
// run on. previous is the last snapshot's positions keyed by node id; nil or
// empty means a cold start.
//
// A returning node resumes exactly where it was. A new node lands at the mean
// of its already-placed neighbours — its own neighbourhood, not a random corner
// — falling back to a golden-angle spiral when none of its neighbours are
// placed (a brand-new component, or the whole graph on a cold start).
//
// Every seed gets a small deterministic offset along the spiral. ForceAtlas2
// divides by inter-node distance, so two nodes at the SAME point produce a
// non-finite force; new nodes sharing a neighbour set would otherwise collide
// exactly. The offset is a function of the index alone, so it is reproducible.
func seedLayoutPositions(nodeIDs []uint, neighbors [][]int32, previous map[uint]layoutPoint) []layoutPoint {
	// spiralRadius spaces the fallback ring so a cold start covers an area
	// proportional to the node count rather than piling everything at the
	// origin; the constant is a starting scale only, since ForceAtlas2 resizes
	// the whole map anyway.
	const spiralRadius = 10.0
	// goldenAngle spreads successive indexes as evenly as possible over the
	// circle, so consecutive ids do not seed on top of each other.
	const goldenAngle = math.Pi * (3.0 - 1.4142135623730951) // pi*(3-sqrt2), irrational turn
	// jitter is well below any distance the layout cares about but far enough
	// above float32 epsilon at spiral scale to guarantee distinct points.
	const jitter = 1e-3

	out := make([]layoutPoint, len(nodeIDs))
	placed := make([]bool, len(nodeIDs))

	for i, id := range nodeIDs {
		if p, ok := previous[id]; ok {
			out[i] = p
			placed[i] = true
		}
	}

	for i := range nodeIDs {
		if placed[i] {
			continue
		}
		angle := goldenAngle * float64(i)
		var x, y float64
		var placedNeighbors int
		for _, n := range neighbors[i] {
			if !placed[int(n)] {
				continue
			}
			x += float64(out[n].X)
			y += float64(out[n].Y)
			placedNeighbors++
		}
		if placedNeighbors > 0 {
			x /= float64(placedNeighbors)
			y /= float64(placedNeighbors)
		} else {
			// Spiral: radius grows with sqrt(index) so the ring fills at
			// constant density instead of crowding the rim.
			r := spiralRadius * math.Sqrt(float64(i)+1)
			x = r * math.Cos(angle)
			y = r * math.Sin(angle)
		}
		out[i] = layoutPoint{
			X: float32(x + jitter*math.Cos(angle)),
			Y: float32(y + jitter*math.Sin(angle)),
		}
		// NOT marked placed: barycenters are computed against the PREVIOUS
		// snapshot's positions only. Seeding one new node off another new
		// node's seed would make the result depend on iteration order among
		// new nodes, and chain a whole new component off one arbitrary guess.
	}
	return out
}

// similarityTransform is a translation + uniform scale + rotation-or-reflection.
type similarityTransform struct {
	Scale float64
	// R is a 2x2 orthogonal matrix in row-major order [r00, r01, r10, r11].
	// Its determinant is +1 for a rotation and -1 for a reflection.
	R [4]float64
	// Tx, Ty is the translation applied after scaling and rotating.
	Tx float64
	Ty float64
}

// identityTransform leaves points where they are.
func identityTransform() similarityTransform {
	return similarityTransform{Scale: 1, R: [4]float64{1, 0, 0, 1}}
}

// apply maps a point through the transform.
func (t similarityTransform) apply(p layoutPoint) layoutPoint {
	x, y := float64(p.X), float64(p.Y)
	return layoutPoint{
		X: float32(t.Scale*(t.R[0]*x+t.R[1]*y) + t.Tx),
		Y: float32(t.Scale*(t.R[2]*x+t.R[3]*y) + t.Ty),
	}
}

// solveProcrustes returns the similarity transform that best maps `from` onto
// `to` in the least-squares sense, allowing reflection.
//
// REFLECTION IS ALLOWED ON PURPOSE. ForceAtlas2's result is invariant under
// mirroring, so a night that comes back mirrored is not a different map — it is
// the same map that a rotation alone cannot undo, and refusing the reflection
// would leave every node maximally displaced.
//
// The two slices must be the same length and index-aligned (element i of each
// is the same node). Fewer than two pairs cannot determine a rotation, so the
// result degrades to translation only, and zero pairs to the identity.
//
// This is the closed-form 2x2 case of orthogonal Procrustes: with N = sum of
// x_i y_i^T over the centred point sets, the objective tr(R N) is maximized
// analytically over rotations and over reflections, and the better of the two
// wins. No SVD, no iteration, no dependency.
func solveProcrustes(from, to []layoutPoint) similarityTransform {
	n := len(from)
	if n != len(to) {
		return identityTransform()
	}
	if n == 0 {
		return identityTransform()
	}

	var fx, fy, tx, ty float64
	for i := 0; i < n; i++ {
		fx += float64(from[i].X)
		fy += float64(from[i].Y)
		tx += float64(to[i].X)
		ty += float64(to[i].Y)
	}
	fx /= float64(n)
	fy /= float64(n)
	tx /= float64(n)
	ty /= float64(n)

	if n < 2 {
		return similarityTransform{Scale: 1, R: [4]float64{1, 0, 0, 1}, Tx: tx - fx, Ty: ty - fy}
	}

	var n00, n01, n10, n11, fromNorm float64
	for i := 0; i < n; i++ {
		ax := float64(from[i].X) - fx
		ay := float64(from[i].Y) - fy
		bx := float64(to[i].X) - tx
		by := float64(to[i].Y) - ty
		n00 += ax * bx
		n01 += ax * by
		n10 += ay * bx
		n11 += ay * by
		fromNorm += ax*ax + ay*ay
	}

	rotValue := math.Hypot(n00+n11, n01-n10)
	refValue := math.Hypot(n00-n11, n01+n10)

	var r [4]float64
	var value float64
	if rotValue >= refValue {
		value = rotValue
		if value == 0 {
			r = [4]float64{1, 0, 0, 1}
		} else {
			c := (n00 + n11) / value
			s := (n01 - n10) / value
			r = [4]float64{c, -s, s, c}
		}
	} else {
		value = refValue
		c := (n00 - n11) / value
		s := (n01 + n10) / value
		r = [4]float64{c, s, s, -c}
	}

	scale := 1.0
	if fromNorm > 0 {
		scale = value / fromNorm
	}
	// A degenerate fit (every source point identical, so fromNorm is 0, or a
	// vanishing scale) would collapse the whole map onto a point. Fall back to
	// scale 1 and let the translation do what it can.
	if scale <= 0 || math.IsNaN(scale) || math.IsInf(scale, 0) {
		scale = 1
	}

	return similarityTransform{
		Scale: scale,
		R:     r,
		Tx:    tx - scale*(r[0]*fx+r[1]*fy),
		Ty:    ty - scale*(r[2]*fx+r[3]*fy),
	}
}

// alignToPrevious Procrustes-aligns a fresh layout onto the previous epoch,
// using the nodes present in both as the correspondence. Returns the points
// unchanged when there is no usable overlap (a cold start, or a catalog that
// turned over completely).
func alignToPrevious(nodeIDs []uint, points []layoutPoint, previous map[uint]layoutPoint) []layoutPoint {
	if len(previous) == 0 {
		return points
	}
	from := make([]layoutPoint, 0, len(nodeIDs))
	to := make([]layoutPoint, 0, len(nodeIDs))
	for i, id := range nodeIDs {
		if p, ok := previous[id]; ok {
			from = append(from, points[i])
			to = append(to, p)
		}
	}
	if len(from) < 2 {
		return points
	}
	t := solveProcrustes(from, to)
	out := make([]layoutPoint, len(points))
	for i, p := range points {
		out[i] = t.apply(p)
	}
	return out
}

// quantizePositions converts world coordinates to the int16 wire format,
// returning the extent (largest absolute coordinate) needed to invert it.
//
// A single shared extent — rather than a per-axis or per-region scale — is what
// keeps the map's aspect ratio and lets a client treat X and Y as one
// coordinate space.
func quantizePositions(points []layoutPoint) (xs, ys []int16, extent float64) {
	xs = make([]int16, len(points))
	ys = make([]int16, len(points))
	for _, p := range points {
		extent = math.Max(extent, math.Abs(float64(p.X)))
		extent = math.Max(extent, math.Abs(float64(p.Y)))
	}
	// A degenerate map (one node, or every node at the origin) has no extent to
	// divide by; 1 keeps every coordinate at 0 instead of producing NaN.
	if extent == 0 {
		extent = 1
	}
	for i, p := range points {
		xs[i] = quantizeCoordinate(float64(p.X), extent)
		ys[i] = quantizeCoordinate(float64(p.Y), extent)
	}
	return xs, ys, extent
}

// quantizeCoordinate maps one world coordinate into the int16 wire range.
func quantizeCoordinate(v, extent float64) int16 {
	q := math.Round(v / extent * contracts.GraphOverviewCoordinateScale)
	if q > contracts.GraphOverviewCoordinateScale {
		q = contracts.GraphOverviewCoordinateScale
	}
	if q < -contracts.GraphOverviewCoordinateScale {
		q = -contracts.GraphOverviewCoordinateScale
	}
	return int16(q)
}
