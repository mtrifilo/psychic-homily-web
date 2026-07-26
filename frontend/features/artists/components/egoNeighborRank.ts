/**
 * The ego graph's default neighbor ranking — "who is this artist closest to?"
 *
 * Two surfaces on the artist page answer that question off the SAME payload:
 * the sidebar Similar-artists list (RelatedArtists) and the inline Connections
 * map (ArtistConnectionsSection, which additionally caps to the top N for
 * legibility). If they ranked differently, the map's names and the top of the
 * list would disagree about who matters most on a page that shows both at
 * once — so the ranking lives here once instead of in each surface.
 *
 * The rank is the strongest edge each neighbor has TO THE CENTER (not its
 * total edge count, and not cross-connections to other neighbors), ties
 * breaking on ascending id so the order is a pure function of the payload
 * rather than of the backend's row order.
 */

import type { ArtistGraph } from '../types'

/**
 * Highest center-edge score per neighbor id. A neighbor absent from the map
 * has no edge to the center at all — cross-connected only — and both surfaces
 * treat that as "not a connection of this artist".
 */
export function maxCenterEdgeScoreByNeighbor(
  graph: Pick<ArtistGraph, 'center' | 'links'>
): Map<number, number> {
  const maxScore = new Map<number, number>()
  for (const link of graph.links) {
    const otherId =
      link.source_id === graph.center.id
        ? link.target_id
        : link.target_id === graph.center.id
          ? link.source_id
          : null
    if (otherId === null) continue
    const prev = maxScore.get(otherId)
    if (prev === undefined || link.score > prev) {
      maxScore.set(otherId, link.score)
    }
  }
  return maxScore
}

/**
 * Comparator for the ranking above: score descending, then id ascending.
 * Neighbors missing from `scores` sort last (they have no center edge).
 */
export function compareByCenterEdgeScore(
  scores: Map<number, number>
): (a: { id: number }, b: { id: number }) => number {
  return (a, b) => {
    const scoreDiff =
      (scores.get(b.id) ?? -Infinity) - (scores.get(a.id) ?? -Infinity)
    if (scoreDiff !== 0) return scoreDiff
    return a.id - b.id
  }
}
