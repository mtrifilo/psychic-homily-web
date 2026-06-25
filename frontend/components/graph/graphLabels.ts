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
 *   - labels are drawn in priority order (forced first, then by `priority` desc
 *     = node degree);
 *   - a label is skipped if its bounding box overlaps an already-placed label;
 *   - `force` labels always draw AND always reserve their box, so a lower-priority
 *     neighbor yields to them (the artist graph forces its center node).
 *
 * A node whose label is culled is still revealed by the existing hover tooltip in
 * each component, so no name is unreachable. Reveal-on-hover IN the canvas (the
 * foreground/background focus effect) is deliberately NOT here — it needs a
 * repaint-on-hover that `onRenderFramePost` can't do once the engine settles, and
 * it's the job of the dedicated hover-focus work (PSY-1210).
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
  /**
   * The already-truncated label string. Each surface truncates with its own
   * length/ellipsis (artist graph 20→18 `...`, ForceGraphView 22→20 `…`) — the
   * thresholds differ ON PURPOSE because the surfaces use different font sizes.
   */
  text: string
  fontSize: number
  bold?: boolean
  /** Higher wins a collision against a lower one (e.g. node degree). Default 0. */
  priority?: number
  /** Always draw + always reserve the box, even on collision (the center node). */
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
// A `textBaseline='top'` glyph occupies ~1.25x its fontSize vertically (ascent +
// descenders), so the collision box is taller than the bare fontSize — otherwise
// two vertically-stacked labels whose glyphs visually touch would both pass the
// overlap test and re-create the pile-up this module exists to prevent.
const LABEL_HEIGHT_FACTOR = 1.25

function boxesIntersect(a: LabelBox, b: LabelBox): boolean {
  return a.x0 < b.x1 && a.x1 > b.x0 && a.y0 < b.y1 && a.y1 > b.y0
}

function fontFor(spec: GraphLabelSpec): string {
  return `${spec.bold ? 'bold ' : ''}${spec.fontSize}px sans-serif`
}

/**
 * Degree (link count) per node id — the collision `priority`, so a more-connected
 * node's label survives over a leaf's when they overlap. Robust to d3-force
 * mutating `link.source`/`link.target` from a bare id to the resolved node object
 * (both branches resolve to the same id). Memoize on the graph data in the caller
 * — it is pure and must NOT run per frame.
 */
export function degreeMap<Id extends string | number>(
  links: ReadonlyArray<{ source: Id | { id: Id }; target: Id | { id: Id } }>,
): Map<Id, number> {
  const counts = new Map<Id, number>()
  for (const link of links) {
    const source = typeof link.source === 'object' ? link.source.id : link.source
    const target = typeof link.target === 'object' ? link.target.id : link.target
    counts.set(source, (counts.get(source) ?? 0) + 1)
    counts.set(target, (counts.get(target) ?? 0) + 1)
  }
  return counts
}

/**
 * Draw a set of node labels with overlap culling. Call EXACTLY ONCE per frame from
 * `onRenderFramePost(ctx, globalScale)` (after the nodes + links are painted, so
 * labels sit on top) — NOT from `nodeCanvasObject`, which would re-run the whole
 * cull once per node against a `placed` set that resets each call. Each spec
 * carries a `priority` (node degree is the convention — see `degreeMap`) and an
 * optional `force` for a pinned/center node. Mutates only the ctx, scoped in a
 * single save/restore so the halo's lineWidth/lineJoin and the text alignment
 * don't leak into later paints on the shared ctx.
 *
 * Ordering: `force` labels first (the artist graph's center), then by `priority`
 * desc; a stable sort keeps input order among equals. A non-forced label is
 * skipped when its box overlaps any already-placed box. The theme-aware halo
 * (background-color stroke ~1/4 the glyph) under the foreground fill is the
 * PSY-1091/1092 legibility recipe.
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

  ctx.save()
  // Invariant for every label in the pass — set once.
  ctx.textAlign = 'center'
  ctx.textBaseline = 'top'
  ctx.lineJoin = 'round'

  const placed: LabelBox[] = []
  for (const spec of ordered) {
    if (spec.text.trim() === '') continue
    ctx.font = fontFor(spec) // set once: drives both measureText and the draw
    const halfWidth = ctx.measureText(spec.text).width / 2 + LABEL_PADDING
    const box: LabelBox = {
      x0: spec.x - halfWidth,
      // The halo stroke (lineWidth fontSize/4) straddles the glyph top, painting
      // ~fontSize/8 above spec.y — reserve that so stacked halos can't kiss.
      y0: spec.y - LABEL_PADDING - spec.fontSize / 8,
      x1: spec.x + halfWidth,
      y1: spec.y + spec.fontSize * LABEL_HEIGHT_FACTOR + LABEL_PADDING,
    }
    if (!spec.force && placed.some((p) => boxesIntersect(p, box))) continue
    ctx.lineWidth = spec.fontSize / 4
    ctx.strokeStyle = palette.labelHalo
    ctx.strokeText(spec.text, spec.x, spec.y)
    ctx.fillStyle = palette.labelText
    ctx.fillText(spec.text, spec.x, spec.y)
    placed.push(box)
  }

  ctx.restore()
}
