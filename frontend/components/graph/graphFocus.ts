/**
 * Hover-focus ("foreground/background") helpers for the canvas graphs (PSY-1210).
 *
 * On hover, the hovered node + its 1-hop neighbors + the connecting links stay
 * foreground (full opacity, labeled); everything else fades to the background.
 * This module computes the foreground node set; the canvas components apply the
 * alpha (nodeCanvasObject / linkColor) and the label gating (nodeLabelsFrame).
 *
 * Extracted as a pure module so the neighborhood math is unit-tested in isolation
 * and shared — ArtistGraph uses it now; ForceGraphView is a planned follow-up.
 */

/** A link endpoint: a bare node id, or the resolved node object d3-force swaps in. */
export type LinkEndpoint = number | { id: number }

/**
 * Narrow a d3-force link endpoint to its node id. d3-force replaces the bare
 * numeric source/target with the resolved node object after the first tick, so
 * callers must handle both shapes — this is the one place that does (PSY-1210).
 */
export const endpointId = (e: LinkEndpoint): number => (typeof e === 'number' ? e : e.id)

/**
 * Build an adjacency map (node id → set of directly-connected node ids) from the
 * graph links. Bidirectional. Handles both link shapes d3-force produces: bare
 * numeric ids before the first tick, resolved `{ id }` node objects after.
 */
export function buildAdjacency(
  links: ReadonlyArray<{ source: LinkEndpoint; target: LinkEndpoint }>,
): Map<number, Set<number>> {
  const adjacency = new Map<number, Set<number>>()
  const link = (a: number, b: number) => {
    let set = adjacency.get(a)
    if (!set) {
      set = new Set<number>()
      adjacency.set(a, set)
    }
    set.add(b)
  }
  for (const l of links) {
    const s = endpointId(l.source)
    const t = endpointId(l.target)
    link(s, t)
    link(t, s)
  }
  return adjacency
}

/**
 * The foreground node-id set for hover-focus: the hovered node plus its 1-hop
 * neighbors. Returns `null` when nothing is hovered — the caller treats `null` as
 * "no focus" (the resting view, everything foreground). A hovered node with no
 * edges yields a singleton set (just itself).
 */
export function focusForeground(
  adjacency: Map<number, Set<number>>,
  hoveredId: number | null | undefined,
): Set<number> | null {
  if (hoveredId == null) return null
  const foreground = new Set<number>([hoveredId])
  const neighbors = adjacency.get(hoveredId)
  if (neighbors) {
    for (const n of neighbors) foreground.add(n)
  }
  return foreground
}
