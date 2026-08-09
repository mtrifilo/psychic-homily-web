/**
 * Label selection for the Map of the Scene (PSY-1725).
 *
 * The map's labels differ from every other graph surface in two ways, so the
 * rules live here rather than in the shared `graphLabels` module:
 *
 *  1. THE SCORE IS THE SNAPSHOT'S RANK, NOT DEGREE. The nightly build ships a
 *     betweenness ordering (`rank`, 0 = most central), which is what "the hubs
 *     of the whole scene" actually means — degree would promote whoever played
 *     the most shared bills inside one clique.
 *  2. THE CANDIDATE SET IS CULLED BEFORE it reaches `renderGraphLabels`. That
 *     function culls by exact box overlap against everything already placed,
 *     which is the right rule for the ~150-node surfaces it was written for and
 *     quadratic at map scale. A grid pass first drops the candidates that
 *     cannot win anyway, so the per-frame cost is bounded by the number of
 *     screen cells instead of by the size of the catalog.
 *
 * Both functions are pure and deterministic, so the same snapshot labels the
 * same artists on every mount and in every browser.
 */

import { BACKGROUND_ALPHA } from '@/components/graph/graphFocus'
import {
  labelTierStyles,
  type GraphLabelSpec,
  type GraphLabelTierStyle,
} from '@/components/graph/graphLabels'

/** The node fields label tiering needs — structural, so tests need no fixtures. */
export interface SceneMapLabelNode {
  id: number
  rank: number
}

/**
 * Tier style per node id, from the snapshot's centrality rank.
 *
 * Two things make this a wrapper rather than a direct `labelTierStyles` call,
 * and both are silent-wrong if skipped:
 *
 *  - The score is NEGATED. `labelTierStyles` ranks by score DESCENDING, and
 *    rank 0 is the most central node, so passing rank straight through would
 *    give the top tier to the least central artists on the map.
 *  - `floorZeroScores` is OFF. That floor exists to stop a zero-DEGREE isolate
 *    wearing hub typography; here score 0 is rank 0 — the single most central
 *    artist in the catalog — and the floor would demote exactly the one label
 *    the map most needs to show.
 */
export function sceneMapLabelTiers(
  nodes: readonly SceneMapLabelNode[],
  tiers: readonly GraphLabelTierStyle[],
): Map<number, GraphLabelTierStyle> {
  const rankById = new Map<number, number>()
  for (const node of nodes) rankById.set(node.id, node.rank)
  return labelTierStyles(
    nodes.map(node => node.id),
    id => -(rankById.get(id) ?? 0),
    tiers,
    { floorZeroScores: false },
  )
}

/** A label candidate, in world coordinates, with everything zoom cannot change. */
export interface SceneMapLabelCandidate {
  id: number
  x: number
  y: number
  /** Node radius the label sits below. */
  radius: number
  /** Already truncated. */
  text: string
  /** Already truncated. Second line under `text`; see `GraphLabelSpec.caption`. */
  caption?: string
  /** SCREEN px at the tier — counter-scaled by zoom at draw time. */
  fontSize: number
  fontWeight: 400 | 500 | 600
  /** Higher wins a collision. */
  priority: number
  /** Drawn through any collision, and exempt from the grid (not from a cap). */
  force: boolean
  /** Non-default face — the mono of a region caption. */
  fontFamily?: string
  /** Ink opacity, 1 by default. */
  alpha?: number
  /** Region size, for ordering the forced list. Unset for node labels. */
  memberCount?: number
  /**
   * Part of the map's ORIENTATION layer rather than its content — a label hub's
   * name or a region caption, as against an artist's name. Only `alphaFor`
   * consumers read it; the selection rules themselves are indifferent.
   */
  isDecoration?: boolean
  /**
   * Where this label's node arrives along a growth replay, 0..1 (PSY-1737).
   * Carried on the candidate rather than looked up per frame: this module's inner
   * loop runs once per node per frame, and a hash lookup there measured ~200us a
   * frame at catalog scale.
   */
  revealAt?: number
}

/** Screen-px grid cell for the cull. Roughly a name plus its breathing room. */
export const LABEL_GRID_CELL_WIDTH = 120
export const LABEL_GRID_CELL_HEIGHT = 34

/**
 * Ceiling on non-forced labels drawn in one frame.
 *
 * The grid already bounds labels by SCREEN AREA, so this only bites when a
 * visitor zooms in far enough that most of the catalog has its own cell. It
 * exists because the per-frame cost has to be bounded by the viewport, never
 * by how big the catalog grew — and because the exact-overlap pass in
 * `renderGraphLabels` is quadratic in what it is handed.
 */
export const MAX_SCENE_MAP_LABELS = 400

/**
 * Ceiling on FORCED labels — region captions and label hubs — in one frame.
 *
 * They are exempt from the grid (a hub with no name is an unexplained square,
 * and a region that has a name must show it), but they cannot be exempt from a
 * ceiling as well: their count grows with the catalog, they join the
 * `placed` set every other label is then tested against, and each one costs a
 * `measureText`. Without this the per-frame cost is bounded by how big the
 * scene got rather than by the viewport, which is the one thing this module
 * promises not to do.
 *
 * Callers pass forced candidates in priority order, so what survives the
 * ceiling is the most central hubs and the region names — never an arbitrary
 * slice.
 */
export const MAX_SCENE_MAP_FORCED_LABELS = 150

/** Ink for a forced label while a neighbourhood is focused. */
function dimmed(alpha: number | undefined): number {
  return (alpha ?? 1) * BACKGROUND_ALPHA
}

/** Grid cell key. Numeric, so the hot path allocates no strings. */
function cellKey(x: number, y: number, cellWidth: number, cellHeight: number): number {
  // Bias into non-negative territory, then pack. The bias is far wider than any
  // real coordinate range, so two distinct cells cannot collide onto one key.
  const column = Math.floor(x / cellWidth) + 32768
  const row = Math.floor(y / cellHeight) + 32768
  return column * 65536 + row
}

/** The graph-space rectangle currently on screen. */
export interface WorldBounds {
  minX: number
  minY: number
  maxX: number
  maxY: number
}

function isVisible(
  candidate: SceneMapLabelCandidate,
  bounds: WorldBounds | null,
): boolean {
  if (!Number.isFinite(candidate.x) || !Number.isFinite(candidate.y)) return false
  if (!bounds) return true
  return (
    candidate.x >= bounds.minX &&
    candidate.x <= bounds.maxX &&
    candidate.y >= bounds.minY &&
    candidate.y <= bounds.maxY
  )
}

/**
 * The labels to draw this frame, at this zoom, under this hover.
 *
 * Runs on the per-frame path, so it allocates only for the labels it KEEPS —
 * the pre-sorted candidate lists are read, never copied, sorted, or filtered.
 *
 * `candidates` MUST already be in priority order (most important first): the
 * first candidate in a cell wins, so the caller's sort IS the tie-break, and an
 * unsorted input would label arbitrary artists differently on every frame.
 *
 * Three bounds stack, and each catches what the previous one cannot:
 *
 *  - THE VIEWPORT decides what is even eligible. Without it the label budget is
 *    spent by rank across the WHOLE map, so a visitor zoomed into one
 *    neighbourhood would watch the budget go to central artists parked
 *    off-screen while the region under the cursor went unlabelled.
 *  - THE GRID spreads what survives evenly, so a dense community cannot take
 *    every label on screen.
 *  - THE CAPS bound the per-frame cost outright — one for grid-culled labels,
 *    one for forced ones. The exact-overlap pass in `renderGraphLabels` is
 *    quadratic in what it is handed, and at high zoom the grid's cells get
 *    small enough in world terms that most of the catalog would earn its own.
 *    Forced labels need their own ceiling because the grid does not apply to
 *    them and their number tracks the catalog, not the screen.
 *
 * `forced` MUST also be in priority order — the ceiling truncates its tail.
 *
 * Cells are anchored at the world origin rather than at the viewport, so
 * panning cannot make a label flicker as it crosses a cell boundary.
 */
export function selectSceneMapLabels(
  candidates: readonly SceneMapLabelCandidate[],
  /** Always drawn (subject to the viewport) — region names and label hubs. */
  forced: readonly SceneMapLabelCandidate[],
  globalScale: number,
  focusedIds: ReadonlySet<number> | null,
  bounds: WorldBounds | null,
  /**
   * Opacity for ONE label, 0..1 — the single seam a caller uses to fade labels
   * in and out for its own reasons (the growth replay clears the orientation
   * layer while it runs, and fades each artist's name in with its own dot).
   *
   * A callback rather than a pair of parameters and a taxonomy flag, so this
   * module owns only the MECHANISM that is genuinely its own — an alpha at or
   * below the floor means skip the candidate entirely and release its collision
   * cell to a label that is actually visible — and the policy for which labels
   * are furniture stays with the caller that knows. Omitted at rest, where every
   * label is drawn at full strength and the loop calls nothing.
   */
  alphaFor?: (candidate: SceneMapLabelCandidate) => number,
): GraphLabelSpec[] {
  const cellWidth = LABEL_GRID_CELL_WIDTH / globalScale
  const cellHeight = LABEL_GRID_CELL_HEIGHT / globalScale
  const gridEnabled = cellWidth > 0 && cellHeight > 0
  const taken = new Set<number>()
  const specs: GraphLabelSpec[] = []

  const toSpec = (candidate: SceneMapLabelCandidate): GraphLabelSpec => ({
    x: candidate.x,
    // Tier sizes are screen px, so both the font and the gap below the node
    // are counter-scaled; the collision boxes stay in graph space.
    y: candidate.y + candidate.radius + LABEL_GAP_PX / globalScale,
    text: candidate.text,
    caption: candidate.caption,
    fontSize: candidate.fontSize / globalScale,
    fontWeight: candidate.fontWeight,
    priority: candidate.priority,
    force: candidate.force,
    fontFamily: candidate.fontFamily,
    alpha: candidate.alpha,
  })

  // Forced labels first. `force` means `renderGraphLabels` draws them THROUGH
  // any collision, so this list has to stay small and deliberate — it is
  // reserved for region names, which are the map's orientation layer and
  // cannot be allowed to lose a cell to an artist. They skip the grid but not
  // the viewport (an off-screen guarantee is worth nothing and costs a text
  // measure) and not their own ceiling.
  //
  // They also skip the FOCUS filter: a region is not a node, has no id in the
  // focused set, and filtering it out would delete the map's whole orientation
  // layer the moment a cursor touched a dot. It dims with everything else
  // instead, which is what focus-dim means everywhere else in the app.
  let forcedKept = 0
  for (const candidate of forced) {
    if (forcedKept >= MAX_SCENE_MAP_FORCED_LABELS) break
    if (!isVisible(candidate, bounds)) continue
    const scale = alphaFor?.(candidate) ?? 1
    if (scale <= LABEL_ALPHA_FLOOR) continue
    forcedKept += 1
    const spec = toSpec(candidate)
    spec.alpha = (focusedIds ? dimmed(candidate.alpha) : (candidate.alpha ?? 1)) * scale
    specs.push(spec)
  }

  let kept = 0
  for (const candidate of candidates) {
    if (kept >= MAX_SCENE_MAP_LABELS) break
    if (candidate.force) continue
    if (focusedIds && !focusedIds.has(candidate.id)) continue
    if (!isVisible(candidate, bounds)) continue
    // Cheapest tests first: this one runs a caller-supplied function, so it is
    // deliberately behind the viewport and focus filters rather than in front of
    // them — and in front of the grid claim, so a label the caller has faded out
    // does not hold a cell against one that is on screen.
    const scale = alphaFor?.(candidate) ?? 1
    if (scale <= LABEL_ALPHA_FLOOR) continue
    if (gridEnabled) {
      const key = cellKey(candidate.x, candidate.y, cellWidth, cellHeight)
      if (taken.has(key)) continue
      taken.add(key)
    }
    kept += 1
    const spec = toSpec(candidate)
    if (scale < 1) spec.alpha = (candidate.alpha ?? 1) * scale
    specs.push(spec)
  }
  return specs
}

/**
 * Below this a label is skipped rather than drawn transparent. A sub-percent
 * label still costs a `measureText` and still reserves a collision box against
 * names that ARE visible.
 */
const LABEL_ALPHA_FLOOR = 0.01

/** Gap in screen px between a node's edge and the top of its label. */
const LABEL_GAP_PX = 3
