/**
 * Deterministic concentric-ring layout for the artist ego graph (PSY-1257/1259/1275).
 *
 * The center sits at the origin; every satellite is PINNED (fx/fy) at an even angle on the
 * ring for its hop distance, so the subject reads as the hub and each expand-on-demand step
 * lands its new neighbors evenly on the next ring out. Hop-1 neighbors sit at EGO_RING_RADIUS;
 * each further hop adds RING_GAP (radius = EGO_RING_RADIUS + (hop-1)*RING_GAP).
 *
 * Pure + canvas-free (it only mutates plain x/y/fx/fy on the render nodes), so it's unit-tested
 * in isolation like mergeEgoGraphs — the ArtistGraph canvas just calls it in its graphData memo.
 * Kept OUT of mergeEgoGraphs.ts on purpose: that module is the hop-merge DATA model; this is the
 * hop-driven LAYOUT (it turns hops into screen coordinates) — a distinct concern.
 */

/**
 * Hop-1 ring radius and the gap added per additional hop (world units). Tuned visually on the
 * dense radio ego graph. Exported so the layout helper, the canvas, AND the unit test all read
 * ONE source — a divergent copy in the test would silently keep passing against stale geometry.
 *
 * PSY-1275: the ring is PINNED, not force-settled. The original PSY-1257 design pulled satellites
 * toward the ring with a custom radial force and let charge spread the angle — but d3's default
 * link force (~30px target distance, cached at `initialize` so an after-the-fact override doesn't
 * take without a reheat) won the tug-of-war on the dense radio graph and collapsed the ring
 * inward, bunching the satellites near the center. Pinning sidesteps it: even angles are computed
 * up front, the link/charge forces can't move a pinned node, and zoomToFit frames a stable bbox
 * with no settle transient. Deterministic + reduced-motion-safe.
 */
export const EGO_RING_RADIUS = 130
export const RING_GAP = 120

/**
 * The subset of a force-graph render node this layout reads/writes. Structural (not the canvas's
 * full `GraphNode`) so this stays a pure, canvas-free helper with no import back into the
 * component — `GraphNode[]` is assignable to `PinnableNode[]` at the call site.
 */
export interface PinnableNode {
  id: number
  isCenter?: boolean
  x?: number
  y?: number
  fx?: number
  fy?: number
}

/**
 * Deterministically pin the whole ego layout — the center at the origin and every satellite at an
 * even angle on its concentric hop-ring — by mutating both the position (x/y) and the d3-force pin
 * (fx/fy) in place. (Named "...Layout", not "...Ring", because it positions the center too, which
 * is not on any ring.) Radius = ringRadius + (hop-1)*ringGap.
 *
 * Satellites are grouped by hop and each ring divided evenly in incoming array order, so an EXPAND
 * preserves the existing inner ring's angular order (its membership is stable) and only the new
 * outer ring is freshly laid out. The angle is index-based (2π·i / ring.length), so the spread is
 * deterministic FOR A GIVEN NODE SET but NOT stable across set changes: toggling an edge-type
 * filter that drops satellites from a ring shrinks ring.length and re-spreads the survivors to new
 * angles. That's expected (the layout recomputes from the current visible set on each memo run),
 * not a settle drift — don't mistake the filter-toggle reshuffle for a regression.
 *
 * Why pin rather than force-settle: d3's default link force (~30px, cached at initialize) otherwise
 * collapses a dense ring inward — see the EGO_RING_RADIUS doc above. A pinned node can't be moved
 * by the link/charge forces, so the layout is fully deterministic and reduced-motion-safe.
 *
 * Nodes with an unknown hop (absent from hopByNodeId) default to hop 1 — matching the memo's
 * fallback for a freshly-fetched neighbor before its hop is assigned.
 */
export function pinEgoLayoutPositions(
  nodes: PinnableNode[],
  hopByNodeId: ReadonlyMap<number, number> | undefined,
  ringRadius: number,
  ringGap: number,
): void {
  const ringByHop = new Map<number, PinnableNode[]>()
  for (const n of nodes) {
    if (n.isCenter) {
      n.fx = n.x = 0
      n.fy = n.y = 0
      continue
    }
    const hop = hopByNodeId?.get(n.id) ?? 1
    const ring = ringByHop.get(hop)
    if (ring) ring.push(n)
    else ringByHop.set(hop, [n])
  }
  for (const [hop, ring] of ringByHop) {
    const r = ringRadius + (hop - 1) * ringGap
    ring.forEach((n, i) => {
      const angle = (2 * Math.PI * i) / ring.length
      n.fx = n.x = r * Math.cos(angle)
      n.fy = n.y = r * Math.sin(angle)
    })
  }
}
