/**
 * Shared canvas node markers for every graph surface.
 *
 * The ego graph once re-implemented these markers with drifted geometry
 * (show dot radius 3 @ offset 2 vs ForceGraphView's 2.5 @ 1.5).
 * Single-sourcing the colors, geometry, AND the draw calls here removes
 * that drift class: ArtistGraphVisualization and ForceGraphView both call
 * these helpers instead of hand-rolling arcs. (DOM legend swatches that
 * name these markers should read the color constants too.)
 *
 * Both markers are FUNCTIONAL indicators, not theme/cluster tokens, so the
 * colors are deliberately hardcoded (same posture as pre-extraction):
 *   - green = "has upcoming shows" — matches the green used app-wide for
 *     upcoming-show affordances;
 *   - violet = "this artist has playable audio" (surfaces with a
 *     selection panel open an embed from it) — deliberately outside the
 *     warm chart palette AND distinct from the green dot, so it reads
 *     unambiguously on both themes and over any node fill.
 *
 * All geometry is in graph world-units (scales with zoom), relative to the
 * node's circle radius, so the markers hug nodes of any size identically.
 */

/**
 * Legend keys for the two markers, single-sourced so the surfaces that name
 * them cannot describe one marker two ways (they did: the home scene-graph
 * teaser said "has upcoming shows" while the ego legend still said "playing
 * soon" for a release cycle).
 *
 * The upcoming-show key states the PREDICATE, not a window: the dot fires on
 * `upcoming_show_count > 0` — any approved future show, at any distance — so
 * "soon" promised a bound the data has no field for. Bounding it instead
 * would be a four-site cross-surface change (ForceGraphView's canvas draw and
 * hover tooltip, plus ArtistGraph's own canvas draw and legend gate); the
 * ruling was to reword.
 *
 * "HAS upcoming shows", not the bare "upcoming shows": both host pages render
 * an <h2>Upcoming shows</h2> section near their graph and legend swatches are
 * aria-hidden, so a bare key reaches a screen reader as a second,
 * subject-less copy of that heading. The verb makes it read as a property of
 * a NAME on the canvas.
 */
export const UPCOMING_SHOW_MARKER_LABEL = 'has upcoming shows'
export const PLAYABLE_MARKER_LABEL = 'playable audio'

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

/**
 * Label hub marker: a rounded square (PSY-1530, Figma node 1137:2).
 *
 * Shape — not hue — is what separates a hub from an artist at a glance: the
 * fill is the label family's own `--chart-6` token (single-sourced from
 * egoPalette's locked mapping, so the app keeps one color language), and a
 * cluster artist can legitimately carry that same hue. A square among circles
 * reads instantly at any zoom, including the far-out fitted view where a
 * hue difference washes out.
 *
 * Drawn filled + stroked like the node circles (the caller owns globalAlpha so
 * hover-focus dim multiplies through identically).
 */
const LABEL_HUB_CORNER_RATIO = 0.28

export function drawLabelHubMarker(
  ctx: CanvasRenderingContext2D,
  x: number,
  y: number,
  /** Half-extent — the square's "radius", matched to the node radius scale. */
  half: number,
  fill: string,
  stroke: string,
): void {
  const radius = half * 2 * LABEL_HUB_CORNER_RATIO
  ctx.beginPath()
  // roundRect is available in every browser target this canvas already
  // requires; the manual arc fallback keeps jsdom-based tests (which stub a
  // partial 2D context) from throwing on an unimplemented method.
  if (typeof ctx.roundRect === 'function') {
    ctx.roundRect(x - half, y - half, half * 2, half * 2, radius)
  } else {
    ctx.rect(x - half, y - half, half * 2, half * 2)
  }
  ctx.fillStyle = fill
  ctx.fill()
  ctx.lineWidth = 1
  ctx.strokeStyle = stroke
  ctx.stroke()
}
