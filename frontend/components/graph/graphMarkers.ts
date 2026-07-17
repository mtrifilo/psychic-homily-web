/**
 * Shared canvas node markers for every graph surface.
 *
 * The ego graph once re-implemented these markers with drifted geometry
 * (show dot radius 3 @ offset 2 vs ForceGraphView's 2.5 @ 1.5).
 * Single-sourcing the colors, geometry, AND the draw calls here
 * makes drift structurally impossible: ArtistGraphVisualization and
 * ForceGraphView both call these helpers instead of hand-rolling arcs.
 *
 * Both markers are FUNCTIONAL indicators, not theme/cluster tokens, so the
 * colors are deliberately hardcoded (same posture as pre-extraction):
 *   - green = "has upcoming shows" — matches the green used app-wide for
 *     upcoming-show affordances;
 *   - violet = "selecting this node opens a playable embed" —
 *     deliberately outside the warm chart palette AND distinct from the
 *     green dot, so it reads unambiguously on both themes and over any
 *     node fill.
 *
 * All geometry is in graph world-units (scales with zoom), relative to the
 * node's circle radius, so the markers hug nodes of any size identically.
 */

/** Upcoming-show indicator: green dot at the node's top-right edge. */
export const UPCOMING_SHOW_DOT_COLOR = '#22c55e'
export const UPCOMING_SHOW_DOT_RADIUS = 2.5
/** Inset of the dot's center from the node's bounding corner. */
export const UPCOMING_SHOW_DOT_INSET = 1.5

/** Playable-audio indicator: violet ring hugging the node. */
export const PLAYABLE_RING_COLOR = '#a855f7'
/** Gap between the node's edge and the ring's stroke center. */
export const PLAYABLE_RING_GAP = 2.5
export const PLAYABLE_RING_WIDTH = 1.5

/**
 * Draw the green upcoming-show dot at the top-right of a node circle.
 * Caller owns globalAlpha (hover-focus dim multiplies through).
 */
export function drawUpcomingShowDot(
  ctx: CanvasRenderingContext2D,
  x: number,
  y: number,
  nodeRadius: number,
): void {
  ctx.beginPath()
  ctx.arc(
    x + nodeRadius - UPCOMING_SHOW_DOT_INSET,
    y - nodeRadius + UPCOMING_SHOW_DOT_INSET,
    UPCOMING_SHOW_DOT_RADIUS,
    0,
    Math.PI * 2,
  )
  ctx.fillStyle = UPCOMING_SHOW_DOT_COLOR
  ctx.fill()
}

/**
 * Draw the violet playable-audio ring around a node circle. The ring (vs a
 * corner badge) never collides with the post-frame labels below the node.
 * Caller owns globalAlpha.
 */
export function drawPlayableRing(
  ctx: CanvasRenderingContext2D,
  x: number,
  y: number,
  nodeRadius: number,
): void {
  ctx.beginPath()
  ctx.arc(x, y, nodeRadius + PLAYABLE_RING_GAP, 0, Math.PI * 2)
  ctx.lineWidth = PLAYABLE_RING_WIDTH
  ctx.strokeStyle = PLAYABLE_RING_COLOR
  ctx.stroke()
}
