'use client'

/**
 * Collision-aware canvas node-label rendering for the force graphs (PSY-1209).
 *
 * Before this, ArtistGraph and ForceGraphView each painted every node's label
 * unconditionally inside `nodeCanvasObject`, so in a dense 1-hop graph the labels
 * piled up and overlapped (e.g. "Bleary Eyed" + "They Are Gutting a…" merging on
 * /artists/snooper). This module moves label drawing into a single post-frame
 * pass both components call from `onRenderFramePost`, where we control draw order
 * and can cull overlaps:
 *
 *   - labels are drawn in priority order (forced first — center + hovered — then
 *     by `priority` desc, e.g. node degree);
 *   - a label is skipped if its bounding box overlaps an already-placed label;
 *   - `force` labels (the center + the hovered node) always draw AND always
 *     reserve their box, so a lower-priority neighbor yields to them.
 *
 * The halo-stroke-then-fill recipe (theme-aware `labelHalo` under `labelText`,
 * PSY-1091/1092) now lives ONLY here — it was previously duplicated byte-for-byte
 * across both components.
 */

import type { GraphPalette } from './graphPalette'

/**
 * One candidate label. Positioned for `textAlign='center'` / `textBaseline='top'`
 * — the caller passes the label's center-x and its top-y (node y + radius +
 * offset), plus the per-graph font/weight and a collision priority.
 */
export interface GraphLabelSpec {
  /** Center x of the label (textAlign center). */
  x: number
  /** Top y of the label (textBaseline top) — caller adds radius + offset. */
  y: number
  text: string
  fontSize: number
  bold?: boolean
  /** Higher wins a collision against a lower one (e.g. node degree). Default 0. */
  priority?: number
  /** Always draw + always reserve the box, even on collision (center, hovered). */
  force?: boolean
}

interface LabelBox {
  x0: number
  y0: number
  x1: number
  y1: number
}

// 1px breathing room so adjacent labels are culled before they visually kiss.
const LABEL_PADDING = 1

function boxesIntersect(a: LabelBox, b: LabelBox): boolean {
  return a.x0 < b.x1 && a.x1 > b.x0 && a.y0 < b.y1 && a.y1 > b.y0
}

function fontFor(spec: GraphLabelSpec): string {
  return `${spec.bold ? 'bold ' : ''}${spec.fontSize}px sans-serif`
}

/**
 * Paint one label with the theme-aware halo (PSY-1091/1092): stroke the
 * background color as a thin halo (~1/4 the glyph) so the text stays legible over
 * colored node circles / cluster hulls on either theme, then fill the resolved
 * foreground. Scoped in save/restore so lineWidth/lineJoin don't leak into later
 * paints on the shared ctx.
 */
function paintLabel(ctx: CanvasRenderingContext2D, spec: GraphLabelSpec, palette: GraphPalette): void {
  ctx.save()
  ctx.font = fontFor(spec)
  ctx.textAlign = 'center'
  ctx.textBaseline = 'top'
  ctx.lineWidth = spec.fontSize / 4
  ctx.lineJoin = 'round'
  ctx.strokeStyle = palette.labelHalo
  ctx.strokeText(spec.text, spec.x, spec.y)
  ctx.fillStyle = palette.labelText
  ctx.fillText(spec.text, spec.x, spec.y)
  ctx.restore()
}

/**
 * Draw a set of node labels with overlap culling. Intended to run once per frame
 * from `onRenderFramePost(ctx, globalScale)` after the nodes + links are painted,
 * so labels sit on top. Mutates only the ctx; returns nothing.
 *
 * Ordering: `force` labels first (center + hovered), then by `priority` desc; a
 * stable sort keeps input order among equals. A non-forced label is skipped when
 * its box overlaps any already-placed box (forced or not).
 */
export function renderGraphLabels(
  ctx: CanvasRenderingContext2D,
  palette: GraphPalette,
  specs: GraphLabelSpec[],
): void {
  if (specs.length === 0) return

  const ordered = [...specs].sort(
    (a, b) =>
      Number(b.force ?? false) - Number(a.force ?? false) || (b.priority ?? 0) - (a.priority ?? 0),
  )

  const placed: LabelBox[] = []
  for (const spec of ordered) {
    if (spec.text === '') continue
    ctx.font = fontFor(spec) // set before measureText so the width matches the draw
    const halfWidth = ctx.measureText(spec.text).width / 2 + LABEL_PADDING
    const box: LabelBox = {
      x0: spec.x - halfWidth,
      y0: spec.y - LABEL_PADDING,
      x1: spec.x + halfWidth,
      y1: spec.y + spec.fontSize + LABEL_PADDING,
    }
    if (!spec.force && placed.some((p) => boxesIntersect(p, box))) continue
    paintLabel(ctx, spec, palette)
    placed.push(box)
  }
}
