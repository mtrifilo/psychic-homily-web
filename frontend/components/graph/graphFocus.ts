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
 * graph links. Bidirectional. Uses `endpointId`, so it accepts either link shape —
 * but note the artist-graph call site builds this from freshly-rebuilt links (bare
 * numeric ids); the resolved `{ id }` shape only appears later, in the per-frame
 * `linkColor` lookups (also via `endpointId`), not here.
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
 * The foreground node-id set for hover-focus: the hovered node, its 1-hop
 * neighbors, and an optional `alwaysInclude` anchor. Returns `null` when nothing is
 * hovered — the caller treats `null` as "no focus" (the resting view, everything
 * foreground). A hovered node with no edges yields a singleton set (just itself).
 *
 * `alwaysInclude` is a node the surface keeps foreground regardless of adjacency —
 * the artist graph passes its center (the page subject), which can sit 2 hops from
 * the hovered node once its direct edge is filtered out. Owning the rule here (not at
 * the call site) keeps it discoverable + tested, and lets surfaces with no such
 * anchor (ForceGraphView, PSY-1225) simply omit it. Always returns a fresh,
 * caller-safe Set.
 */
export function focusForeground(
  adjacency: Map<number, Set<number>>,
  hoveredId: number | null | undefined,
  alwaysInclude?: number | null,
): Set<number> | null {
  if (hoveredId == null) return null
  const foreground = new Set<number>([hoveredId])
  const neighbors = adjacency.get(hoveredId)
  if (neighbors) {
    for (const n of neighbors) foreground.add(n)
  }
  if (alwaysInclude != null) foreground.add(alwaysInclude)
  return foreground
}

// Background fade alpha for hover-focus, shared by both canvas surfaces (ArtistGraph +
// ForceGraphView) so they dim by the SAME amount — a design token, not a per-surface
// tuning, kept here (next to the neighborhood math) so the two can't drift. BACKGROUND_ALPHA
// is the canvas globalAlpha for nodes; BACKGROUND_ALPHA_HEX is the same value as a 2-char hex
// pair for withHexAlpha on (6-hex) link colors — derived, so tuning the constant moves both.
// (They share the source number, not the PERCEIVED opacity: the node globalAlpha multiplies
// the node's already-semi-transparent fill, so backgrounded nodes read a touch fainter than
// the flat-alpha links. Note withHexAlpha passes any non-6-hex color through UNCHANGED, so if
// an --edge-* token ever became oklch/rgb the background links would silently render at FULL
// color (no fade) while nodes still dim — the same latent gap the resting cross-connection
// dim already has. All current --edge-* tokens are 6-hex; ForceGraphView's UNTYPED links,
// which carry rgba() colors withHexAlpha can't touch, fade via an explicit rgba() using
// BACKGROUND_ALPHA directly.)
export const BACKGROUND_ALPHA = 0.15
export const BACKGROUND_ALPHA_HEX = Math.round(BACKGROUND_ALPHA * 255).toString(16).padStart(2, '0')
