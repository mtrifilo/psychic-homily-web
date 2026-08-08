/**
 * Geometry of the "this week in the graph" share card, from the approved mock
 * (PSY-1726 board, share-artifact section, Figma node `1311:2`).
 *
 * Separated from the card that renders it for the reason the rest of the family
 * is: a unit test can import these without pulling in `next/og`, so the budgets
 * the tests assert against are the ones the card actually uses. Both HIGH
 * findings on the first weekly card were invisible to CI precisely because its
 * fit budgets lived inside the route.
 *
 * Sizes are px on the 1200×630 canvas. The family's rule is to design at full
 * size and VERIFY AT 300px — a link renders about that wide in a group chat —
 * and to treat anything under ~8px effective as decoration that must not carry
 * meaning. Divide by 4 to check: headline 23 · counts 8.5-11 · eyebrow 8.5 ·
 * range 8.5.
 *
 * The map motif is the one thing here that is genuinely decoration: the counts
 * are what the card asserts, and the dots are the texture that says WHICH map
 * they are about. It is sized and sampled accordingly.
 */

import { OG_SIZE } from '@/lib/og/brand'
import { measureMono, measureSans } from '@/lib/og/textFit'

import type { SceneMap, SceneMapNode } from './sceneMap'
import { isInGraphWeek, type GraphWeek } from './graphWeek'

export const PAD_X = 72
export const PAD_Y = 64

/** The box every element on the card has to fit inside. */
export const CONTENT_WIDTH = OG_SIZE.width - PAD_X * 2

/**
 * `PSYCHIC HOMILY · THE MAP OF THE SCENE`, top left, Space Mono.
 *
 * 34px, matching the weekly city card's mono floor: it is the only line on the
 * card carrying the brand, and 30px lands at 7.5px effective — under the
 * family's own floor. It gets the FULL content width rather than the headline
 * column's, which is what makes 34px fit; the motif behind its tail is drawn at
 * low opacity for the same reason.
 */
export const EYEBROW_SIZE = 34
export const EYEBROW_TRACKING = 2
export const EYEBROW_TEXT = 'PSYCHIC HOMILY · THE MAP OF THE SCENE'

/**
 * `This week in the graph`, Satoshi Bold.
 *
 * A FIXED size with no fit function, and that is a property of the copy rather
 * than an omission: this string is a constant, so it is measured once — by the
 * test next to this file — instead of being re-measured on every render. The
 * two variable-length strings on the card (the counts and the range) are both
 * mono and both bounded, and only the counts need a step-down.
 *
 * It wraps to two lines inside `TEXT_WIDTH` by design. 92px is 23px at the
 * 300px share size, which keeps it the dominant element there too.
 */
export const HEADLINE_SIZE = 92
export const HEADLINE_LINE_HEIGHT = 88
export const HEADLINE_TEXT = 'This week in the graph'

/**
 * `+12 ARTISTS · +34 CONNECTIONS`, Space Mono, primary orange.
 *
 * Steps down rather than clipping, because the count is what the card exists to
 * say. The floor is the family's 8.5px-effective mono floor; the widest
 * realistic line (`+9,999 ARTISTS · +99,999 CONNECTIONS`) fits inside
 * `COUNTS_MAX_WIDTH` at it.
 */
export const COUNTS_SIZE_MAX = 40
export const COUNTS_SIZE_MIN = 34
export const COUNTS_TRACKING = 1

/** `JUL 27 - AUG 2 2026`, Space Mono, muted. */
export const RANGE_SIZE = 34
export const RANGE_TRACKING = 2

/** Optical gaps down the left column. */
export const HEADLINE_GAP = 22
export const RANGE_GAP = 14

/**
 * Width of the left text column.
 *
 * Narrower than the content box on purpose: the headline wraps inside it, and
 * the wrap is what keeps the map motif on the right visible as a map rather
 * than as a strip behind a single long line.
 */
export const TEXT_WIDTH = 660

/**
 * The counts line gets more room than the headline column.
 *
 * It is a single unwrappable mono run — a wrap would split `+12 ARTISTS` across
 * two lines — so it is allowed to reach past `TEXT_WIDTH` into the motif's
 * fade, where the gradient is still near-opaque.
 *
 * The VALUE is not chosen, it is derived, and the test beside this file is what
 * derives it: 810 is the smallest budget that seats the widest count line this
 * card can produce (`+9,999 ARTISTS · +99,999 CONNECTIONS`) at the 34px mono
 * floor, while still letting an ordinary week run at the full 40px. It also
 * keeps the line's right edge inside `MOTIF_FADE_CLEAR_STOP`, so no part of it
 * is ever set over undimmed dots.
 */
export const COUNTS_MAX_WIDTH = 810

/**
 * Where the map motif is drawn, in card coordinates.
 *
 * RIGHT-ANCHORED AND BLEEDING off three edges. The map is roughly square and
 * the card is 1.9:1, so a motif fitted to the whole canvas would be a
 * postage-stamp in the middle with the headline on top of it. Anchored right
 * and over-sized, it reads as a window onto the map with the text beside it —
 * which is the composition the mock locks.
 */
export const MOTIF_BOX = { x: 470, y: -60, width: 800, height: 750 } as const

/**
 * A box to fit the motif into. The card uses `MOTIF_BOX`; the share PAGE passes
 * its own, because there the motif is a centred teaser rather than the backdrop
 * to a text column, and re-fitting is cheaper than a second projection.
 */
export interface MotifBox {
  x: number
  y: number
  width: number
  height: number
}

/**
 * Horizontal fade that puts the text on solid brand background.
 *
 * Expressed as gradient stops rather than a box, because Satori paints it as
 * one `linear-gradient` over the motif. Opaque until the headline column ends,
 * fully clear by the time the motif's dense middle starts.
 */
export const MOTIF_FADE_OPAQUE_STOP = 46
export const MOTIF_FADE_CLEAR_STOP = 74

/** Radius of a dot that was already on the map. */
export const MOTIF_DOT_RADIUS = 3.2
/** Radius of a dot that arrived in the window — "slightly larger" per the mock. */
export const MOTIF_NEW_DOT_RADIUS = 7
/** Thin connector lines between this window's arrivals. */
export const MOTIF_CONNECTOR_WIDTH = 1.6

export const MOTIF_DOT_OPACITY = 0.34
export const MOTIF_NEW_DOT_OPACITY = 0.95
export const MOTIF_CONNECTOR_OPACITY = 0.5

/**
 * Caps on what the motif draws.
 *
 * Satori serialises an inline `<svg>` subtree and hands it to resvg to
 * rasterise, so every element is real work inside an edge function that already
 * sits at ~96.5% of Vercel's 1 MB limit and has a CPU budget to keep. A
 * production snapshot is thousands of nodes and tens of thousands of edges; the
 * motif needs enough of them to read as a map and no more.
 *
 * The COUNTS ARE NEVER SAMPLED — they come from `resolveGraphWeek`, which walks
 * everything. Only the drawing is capped, so a card can show fewer orange dots
 * than the number beside them. That asymmetry is deliberate: the alternative is
 * a wrong number.
 */
export const MOTIF_DOT_LIMIT = 900
export const MOTIF_NEW_DOT_LIMIT = 400
export const MOTIF_CONNECTOR_LIMIT = 300

/**
 * Next requires a STATIC alt, so it cannot name the counts — they change every
 * night. The page supplies the real numbers through `openGraph.images[].alt`.
 */
export const GRAPH_WEEK_OG_ALT =
  'A map of the Psychic Homily music graph with this week’s new artists and connections highlighted'

/** What the counts line gets, once its own tracking is accounted for. */
export function fitCountsSize(counts: string): number {
  for (let size = COUNTS_SIZE_MAX; size > COUNTS_SIZE_MIN; size -= 1) {
    if (measureMono(counts, size, COUNTS_TRACKING) <= COUNTS_MAX_WIDTH) return size
  }
  return COUNTS_SIZE_MIN
}

/** Measured widths the test asserts the fixed-size copy against. */
export function eyebrowWidth(): number {
  return measureMono(EYEBROW_TEXT, EYEBROW_SIZE, EYEBROW_TRACKING)
}

/** The headline's longest unbreakable run — what has to fit on one line. */
export function headlineLongestWordWidth(): number {
  return Math.max(
    ...HEADLINE_TEXT.split(' ').map(word => measureSans(word, 'satoshiBold', HEADLINE_SIZE))
  )
}

/** One dot on the motif, in card coordinates. */
export interface MotifDot {
  x: number
  y: number
}

/** One connector, in card coordinates. */
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
 * Project a snapshot into the card's motif.
 *
 * Fitted to the snapshot's OWN node bounds rather than to the payload's nominal
 * extent, so a night whose layout happens to occupy a corner of the coordinate
 * space still fills the motif box instead of huddling in one part of it. Aspect
 * ratio is preserved — a squashed map is not this map.
 *
 * Pure, and returns plain numbers: the card only interpolates them into SVG
 * attributes, which is what lets the sampling and the projection be asserted
 * without rasterising anything.
 */
export function buildGraphWeekMotif(
  map: SceneMap,
  week: GraphWeek,
  box: MotifBox = MOTIF_BOX
): GraphWeekMotif {
  const project = fitProjection(map.nodes, box)
  if (!project) return { dots: [], newDots: [], connectors: [] }

  const existing: SceneMapNode[] = []
  const arrivals: SceneMapNode[] = []
  // Every node's projected position, keyed by id — connectors need an endpoint
  // that SAMPLING may have dropped from the drawn dots. A line has to land on
  // the map even when the dot at its far end was not one of the 900 drawn.
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
  // a counted connection the same set, up to the cap below.
  const connectors: MotifConnector[] = []
  for (const edge of map.edges) {
    if (connectors.length >= MOTIF_CONNECTOR_LIMIT) break
    if (!isInGraphWeek(week, edge.appear)) continue
    const from = positionById.get(edge.source)
    const to = positionById.get(edge.target)
    if (!from || !to) continue
    connectors.push({ x1: from.x, y1: from.y, x2: to.x, y2: to.y })
  }

  return {
    dots: sample(existing, MOTIF_DOT_LIMIT).map(node => positionById.get(node.id)!),
    newDots: sample(arrivals, MOTIF_NEW_DOT_LIMIT).map(node => positionById.get(node.id)!),
    connectors,
  }
}

/**
 * Even-stride sample, NOT a random one.
 *
 * The card has to be deterministic per snapshot — that is the whole point of a
 * snapshot-aligned window — so two renders of the same night must draw the same
 * dots. A stride also spreads the sample across the layout, where taking the
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
 * A world-to-card projection fitted to the nodes' bounding box, or null when
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
 * One decimal place. The motif is rasterised at 1200px wide, so a second
 * decimal is invisible — and every digit is bytes in the SVG string Satori
 * serialises, multiplied by up to 1,300 elements.
 */
function round(value: number): number {
  return Math.round(value * 10) / 10
}
