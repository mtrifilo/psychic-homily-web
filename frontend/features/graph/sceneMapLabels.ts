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

import {
  labelTierStyles,
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

/** A label candidate, positioned in the space the grid is measured in. */
export interface SceneMapLabelCandidate {
  x: number
  y: number
}

/**
 * Keep at most one candidate per grid cell, preferring earlier entries.
 *
 * `candidates` MUST already be in priority order (most important first) — the
 * function keeps the first candidate it sees in each cell, so the caller's sort
 * IS the tie-break, and an unsorted input silently labels arbitrary artists.
 *
 * The cell is a coarse stand-in for a label's footprint: one name per cell
 * leaves the exact overlap test in `renderGraphLabels` a much smaller set to
 * work through, while guaranteeing the map is labelled EVENLY rather than
 * spending its whole budget on the densest community. Cells are keyed off
 * `Math.floor`, so the grid is anchored at the origin and a pan cannot make
 * labels flicker between cells.
 */
export function cullLabelsToGrid<T extends SceneMapLabelCandidate>(
  candidates: readonly T[],
  cellWidth: number,
  cellHeight: number,
): T[] {
  if (cellWidth <= 0 || cellHeight <= 0) return [...candidates]
  const taken = new Set<string>()
  const kept: T[] = []
  for (const candidate of candidates) {
    if (!Number.isFinite(candidate.x) || !Number.isFinite(candidate.y)) continue
    const cell = `${Math.floor(candidate.x / cellWidth)}:${Math.floor(candidate.y / cellHeight)}`
    if (taken.has(cell)) continue
    taken.add(cell)
    kept.push(candidate)
  }
  return kept
}
