'use client'

/**
 * Labeled isolate shelf — locked grammar decision 4.
 *
 * On Section-class surfaces (scene, station) the pinned isolate shelf reads
 * as a NAMED GROUP instead of an anonymous dot row: a faint containment band
 * (foreground ink at ~3.5%) with a 1px hairline top border behind the shelf,
 * plus a "+{N} not yet connected artists" caption. Everything here draws in
 * GRAPH (world) coordinates so the band and caption pan/zoom with the graph;
 * the band renders at ALL zoom levels — it IS the group boundary, so there is
 * deliberately no hull-style zoom fade.
 *
 * Module seams (kept deliberately narrow for the tiered-label work landing in
 * the same label pass next):
 *   - `isolateShelfGeometry` is the single source for shelf placement —
 *     ForceGraphView's pinning effect consumes it too, so the band can never
 *     drift from where the dots actually pin.
 *   - `drawIsolateShelfBand` paints from `onRenderFramePre` (under hulls,
 *     links, and nodes).
 *   - `drawIsolateShelfCaption` paints from the post-frame label pass, after
 *     the collision-culled node labels.
 *
 * {N} is the CURRENTLY RENDERED isolate count (post cluster filtering), not
 * the raw payload count — the caller derives it from its filtered render set.
 * Future "help connect them →" contribution hook: extend the caption draw
 * here; the seam is this module, not the component.
 */

import type { GraphPalette } from './graphPalette'
import { withHexAlpha } from './graphPalette'

export interface IsolateShelfGeometry {
  /** World-space y the isolate dots pin to. */
  y: number
  /** World-space left extent of the shelf (dots distribute from here;
   * a LONE isolate is centered between the extents instead). */
  startX: number
  /** World-space right extent of the shelf. */
  endX: number
}

// Shelf placement ratios — extracted verbatim from the ForceGraphView pinning
// effect (shelfY = height * 0.42, shelf x = ±width * 0.4) so band and pins
// share one definition.
const SHELF_Y_RATIO = 0.42
const SHELF_HALF_WIDTH_RATIO = 0.4

// Band metrics in world px (== screen px at zoom 1), eyeballed against the
// approved Figma mock (Grammar build-out mocks, node 1030-2): the band clears
// the caption row above the dots and the hover-revealed name below them.
const BAND_PAD_X = 32
const BAND_TOP_OFFSET = 44
const BAND_BOTTOM_OFFSET = 40
// Caption anchor: left-aligned to the band's left edge (the approved mock
// anchors the caption at the band inset, LEFT of the first dot — not at the
// first dot itself), on its own row above the dot centers (dots sit at
// geometry.y; hover names draw below them).
const CAPTION_INSET_X = 16
const CAPTION_TOP_OFFSET = 32

// Band fill: foreground ink at ~3.5% (locked treatment), hairline at 15% —
// both from the theme-resolved palette so light/dark can't drift.
const BAND_FILL_ALPHA_HEX = '09' // ≈ 3.5%
const HAIRLINE_ALPHA_HEX = '26' // ≈ 15%

// Caption targets a constant screen size (like the node labels, which
// counter-scale via labelFontSize) so the group stays named at every zoom.
// The world-space clamp bounds a far-zoomed-out caption (counter-scaling
// makes it GROW in world units) so its glyphs can never reach the dot row
// CAPTION_TOP_OFFSET below the anchor (26 + dot radius 5 < 32). It ENGAGES
// below zoom 12/26 ≈ 0.46 — reachable above the component's 0.4 minZoom —
// costing the caption ~1.6 screen px at the very bottom of the zoom range;
// deliberate trade so the group label never paints over its own dots.
const CAPTION_FONT_SCREEN_PX = 12
const CAPTION_MAX_WORLD_PX = 26
const CAPTION_FONT_WEIGHT = 500

/** Shelf placement for a given canvas size — single source shared by the
 * ForceGraphView isolate-pinning effect and the band/caption draws. */
export function isolateShelfGeometry(
  containerWidth: number,
  graphHeight: number
): IsolateShelfGeometry {
  return {
    y: graphHeight * SHELF_Y_RATIO,
    startX: -containerWidth * SHELF_HALF_WIDTH_RATIO,
    endX: containerWidth * SHELF_HALF_WIDTH_RATIO,
  }
}

/** Approved caption copy: "+{N} not yet connected artists" (singular form
 * for exactly one — the mock shows the plural template). */
export function isolateShelfCaption(count: number): string {
  return `+${count} not yet connected ${count === 1 ? 'artist' : 'artists'}`
}

/**
 * Containment band + 1px hairline top border behind the shelf row. Call from
 * `onRenderFramePre` (graph coords, before hulls/links/nodes paint). The
 * hairline counter-scales to a constant 1 SCREEN px; the band itself lives in
 * world coords so it pans/zooms with the pinned dots.
 */
export function drawIsolateShelfBand(
  ctx: CanvasRenderingContext2D,
  palette: GraphPalette,
  geometry: IsolateShelfGeometry,
  globalScale: number
): void {
  const x0 = geometry.startX - BAND_PAD_X
  const x1 = geometry.endX + BAND_PAD_X
  const y0 = geometry.y - BAND_TOP_OFFSET
  const y1 = geometry.y + BAND_BOTTOM_OFFSET
  ctx.save()
  ctx.fillStyle = withHexAlpha(palette.labelText, BAND_FILL_ALPHA_HEX)
  ctx.fillRect(x0, y0, x1 - x0, y1 - y0)
  ctx.strokeStyle = withHexAlpha(palette.labelText, HAIRLINE_ALPHA_HEX)
  ctx.lineWidth = 1 / globalScale
  ctx.beginPath()
  ctx.moveTo(x0, y0)
  ctx.lineTo(x1, y0)
  ctx.stroke()
  ctx.restore()
}

/**
 * Group caption, drawn in the post-frame label pass AFTER the collision-culled
 * node labels (the group label always wins). Anchored to the band's top-left
 * in world coords; the font counter-scales to a constant screen size, and the
 * theme-aware halo-under-fill recipe matches the node labels (muted ink — a
 * caption, not a competing artist name).
 */
export function drawIsolateShelfCaption(
  ctx: CanvasRenderingContext2D,
  palette: GraphPalette,
  geometry: IsolateShelfGeometry,
  count: number,
  globalScale: number
): void {
  if (count <= 0) return
  let fontSize = Math.min(
    CAPTION_FONT_SCREEN_PX / globalScale,
    CAPTION_MAX_WORLD_PX
  )
  const text = isolateShelfCaption(count)
  const x = geometry.startX - BAND_PAD_X + CAPTION_INSET_X
  const y = geometry.y - CAPTION_TOP_OFFSET
  ctx.save()
  ctx.font = `${CAPTION_FONT_WEIGHT} ${fontSize}px sans-serif`
  // Measured width clamp (PSY-1456, deferred from the PSY-1454 review): on a
  // very narrow container at min zoom the counter-scaled caption can grow
  // wider than the band itself in world units. Shrink the font just enough
  // that the measured text fits inside the band (mirrored inset on the
  // right), rather than letting the group label spill past its own boundary.
  // No lower floor: a smaller-but-contained caption beats an overflowing one,
  // and the clamp only engages on the smallest screens at the deepest zoom.
  const maxWidth = geometry.endX + BAND_PAD_X - CAPTION_INSET_X - x
  const measuredWidth = ctx.measureText(text).width
  if (measuredWidth > maxWidth && maxWidth > 0) {
    fontSize = fontSize * (maxWidth / measuredWidth)
    ctx.font = `${CAPTION_FONT_WEIGHT} ${fontSize}px sans-serif`
  }
  ctx.textAlign = 'left'
  ctx.textBaseline = 'top'
  ctx.lineJoin = 'round'
  ctx.lineWidth = fontSize / 4
  ctx.strokeStyle = palette.labelHalo
  ctx.strokeText(text, x, y)
  ctx.fillStyle = palette.mutedForeground
  ctx.fillText(text, x, y)
  ctx.restore()
}
