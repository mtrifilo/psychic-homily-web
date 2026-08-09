/**
 * The map motif: a snapshot's layout projected into a drawable picture of the
 * week's arrivals.
 *
 * Its own module rather than part of the share card's layout, because it is not
 * about the card. It knows nothing of `next/og`, of typography, or of 1200×630;
 * it turns `SceneMap` coordinates into dots and lines inside whatever box a
 * caller hands it. Two surfaces draw it today at very different sizes — the OG
 * card with Satori attributes, the share page with theme classes — and a third
 * (a digest, a thumbnail) should not have to import an OG module to get it.
 *
 * Pure, and returns plain numbers: the callers only interpolate them into SVG
 * attributes, which is what lets the projection and the sampling be asserted
 * without rasterising anything.
 */

import { isInGraphWeek, type GraphWeek } from './graphWeek'
import type { SceneMap, SceneMapNode } from './sceneMap'

/** A box to fit the motif into, in the caller's own coordinate space. */
export interface MotifBox {
  x: number
  y: number
  width: number
  height: number
}

/**
 * Caps on what the motif draws.
 *
 * THE COUNTS ARE NEVER SAMPLED — they come from `resolveGraphWeek`, which walks
 * everything. Only the drawing is capped, so a picture can show fewer orange
 * dots than the number printed beside it. That asymmetry is deliberate: the
 * alternative is a wrong number.
 *
 * A parameter rather than a constant because the two surfaces pay for elements
 * in completely different currencies — see `CARD_MOTIF` and `TEASER_MOTIF`.
 */
export interface MotifLimits {
  dots: number
  newDots: number
  connectors: number
}

/** How a surface paints the motif, in its own coordinate space. */
export interface MotifPaint {
  dotRadius: number
  newDotRadius: number
  connectorWidth: number
}

/** Everything a surface needs to draw the motif at its own size. */
export interface MotifSpec {
  box: MotifBox
  limits: MotifLimits
  paint: MotifPaint
}

/**
 * The share card's motif: right-anchored and bleeding off three edges.
 *
 * The map is roughly square and the card is 1.9:1, so a motif fitted to the
 * whole canvas would be a postage stamp in the middle with the headline on top
 * of it. Anchored right and over-sized, it reads as a window onto the map with
 * the text beside it, which is the composition the mock locks.
 *
 * The limits are sized for a RASTERISED PNG: every element is work inside an
 * edge function that already sits at ~96% of Vercel's 1 MB limit and has a CPU
 * budget to keep, but none of it reaches a browser.
 */
export const CARD_MOTIF: MotifSpec = {
  box: { x: 470, y: -60, width: 800, height: 750 },
  limits: { dots: 900, newDots: 400, connectors: 180 },
  paint: {
    dotRadius: 3.2,
    /** "Slightly larger" per the mock — what makes an arrival read as one. */
    newDotRadius: 7,
    connectorWidth: 1.6,
  },
}

/**
 * The share page's motif: the same picture, centred, at a third of the element
 * count.
 *
 * The caps are LOWER than the card's on purpose, and the reason is not the
 * drawing — it is that these elements are shipped. Measured at the card's caps,
 * the teaser serialised to roughly 68 KB of HTML plus 67 KB of RSC payload and
 * mounted ~1,300 DOM nodes, on a page whose whole job is to be a preview that
 * someone bounces off toward /graph. At the size it actually renders (~768 CSS
 * px wide) the picture is indistinguishable.
 */
export const TEASER_MOTIF: MotifSpec = {
  box: { x: 0, y: 0, width: 900, height: 380 },
  limits: { dots: 250, newDots: 120, connectors: 60 },
  paint: { dotRadius: 2.6, newDotRadius: 5, connectorWidth: 1.6 },
}

/** One dot, in the caller's coordinate space. */
export interface MotifDot {
  x: number
  y: number
}

/** One connector, in the caller's coordinate space. */
export interface MotifConnector {
  x1: number
  y1: number
  x2: number
  y2: number
}

export interface GraphWeekMotif {
  /** Nodes that were already on the map, sampled — the muted texture. */
  dots: MotifDot[]
  /** Nodes that arrived in the window, sampled — painted primary orange. */
  newDots: MotifDot[]
  /** Edges that arrived in the window, sampled. */
  connectors: MotifConnector[]
}

/**
 * Project a snapshot into a motif.
 *
 * Fitted to the snapshot's OWN node bounds rather than to the payload's nominal
 * extent, so a night whose layout happens to occupy a corner of the coordinate
 * space still fills the box instead of huddling in one part of it. Aspect ratio
 * is preserved — a squashed map is not this map.
 *
 * `box` and `limits` are both required. An earlier revision defaulted them to
 * the card's, which is how the share page came to ship a picture sized for a
 * PNG; a pure function should not carry one caller's frame as a default.
 */
export function buildGraphWeekMotif(
  map: SceneMap,
  week: GraphWeek,
  { box, limits }: Pick<MotifSpec, 'box' | 'limits'>
): GraphWeekMotif {
  const project = fitProjection(map.nodes, box)
  if (!project) return { dots: [], newDots: [], connectors: [] }

  const existing: SceneMapNode[] = []
  const arrivals: SceneMapNode[] = []
  // Every node's projected position, keyed by id — connectors need an endpoint
  // that SAMPLING may have dropped from the drawn dots. A line has to land on
  // the map even when the dot at its far end was not one of the few hundred
  // drawn. Measured at 0.3ms for a 5,000-node snapshot, so the extra positions
  // cost less than the invariant is worth.
  const positionById = new Map<number, MotifDot>()
  for (const node of map.nodes) {
    const at = project(node)
    // A node with a non-finite coordinate is dropped rather than drawn: a `NaN`
    // in an SVG attribute is not a visible bug, it is an element resvg may
    // refuse — and refusing one dot must not cost the whole motif.
    if (!at) continue
    positionById.set(node.id, at)
    if (week.newNodeIds.has(node.id)) arrivals.push(node)
    else existing.push(node)
  }

  // The SAME window predicate the counts use (`isInGraphWeek`), never a
  // re-derivation from the endpoints — that is what keeps a drawn connector and
  // a counted connection the same set, up to the cap.
  const inWindow: MotifConnector[] = []
  for (const edge of map.edges) {
    if (!isInGraphWeek(week, edge.appear)) continue
    const from = positionById.get(edge.source)
    const to = positionById.get(edge.target)
    if (!from || !to) continue
    inWindow.push({ x1: from.x, y1: from.y, x2: to.x, y2: to.y })
  }

  return {
    dots: sample(existing, limits.dots).map(node => positionById.get(node.id)!),
    newDots: sample(arrivals, limits.newDots).map(node => positionById.get(node.id)!),
    // Sampled on the SAME rule as the dots rather than truncated to the first
    // N. Edges arrive in CSR order, i.e. ascending source index, which the
    // backend groups by community — so taking the first N drew every connector
    // in one region of the map and left the rest bare.
    connectors: sample(inWindow, limits.connectors),
  }
}

/**
 * Even-stride sample, NOT a random one.
 *
 * The card has to be deterministic per snapshot — that is the whole point of a
 * snapshot-aligned window — so two renders of the same night must draw the same
 * picture. A stride also spreads the sample across the layout, where taking the
 * first N would draw only whichever corner of the map the payload happens to
 * enumerate first.
 */
function sample<T>(items: T[], limit: number): T[] {
  if (items.length <= limit) return items
  const stride = items.length / limit
  const picked: T[] = new Array(limit)
  for (let i = 0; i < limit; i += 1) {
    picked[i] = items[Math.floor(i * stride)]
  }
  return picked
}

/**
 * A world-to-box projection fitted to the nodes' bounding box, or null when
 * there is nothing to fit.
 *
 * A degenerate box (every node at one point, or a single node) would divide by
 * zero, so the scale falls back to 1 and the map lands centred as a single dot
 * — truthful for a one-node catalog, and unreachable in production.
 */
function fitProjection(
  nodes: SceneMapNode[],
  box: MotifBox
): ((node: SceneMapNode) => MotifDot | null) | null {
  if (nodes.length === 0) return null

  let minX = Infinity
  let maxX = -Infinity
  let minY = Infinity
  let maxY = -Infinity
  for (const node of nodes) {
    if (!Number.isFinite(node.x) || !Number.isFinite(node.y)) continue
    if (node.x < minX) minX = node.x
    if (node.x > maxX) maxX = node.x
    if (node.y < minY) minY = node.y
    if (node.y > maxY) maxY = node.y
  }
  if (!Number.isFinite(minX) || !Number.isFinite(minY)) return null

  const spanX = maxX - minX
  const spanY = maxY - minY
  const scale =
    spanX > 0 || spanY > 0
      ? Math.min(
          spanX > 0 ? box.width / spanX : Infinity,
          spanY > 0 ? box.height / spanY : Infinity
        )
      : 1
  // Centre the fitted map inside the box on whichever axis has slack.
  const offsetX = box.x + (box.width - spanX * scale) / 2
  const offsetY = box.y + (box.height - spanY * scale) / 2

  return node => {
    if (!Number.isFinite(node.x) || !Number.isFinite(node.y)) return null
    return {
      x: round(offsetX + (node.x - minX) * scale),
      y: round(offsetY + (node.y - minY) * scale),
    }
  }
}

/**
 * One decimal place. The card rasterises at 1200px wide, so a second decimal is
 * invisible — and on the page every digit is bytes in the serialised HTML, once
 * per element.
 */
function round(value: number): number {
  return Math.round(value * 10) / 10
}
