import type { SceneGraphInfo } from '../types'

/**
 * The ONE source for "how many artists is this graph showing" (PSY-1296) —
 * consumed by both the visual header (SceneGraph) and the canvas aria-label
 * (SceneGraphVisualization), so sighted users and assistive tech can never
 * hear different numbers or framings for the same graph.
 *
 * `artist_count` is the contract field the truncation flag is defined
 * against (backend guarantees it equals the shipped node count), so the
 * phrase reads it rather than a locally derived nodes.length.
 */
export function sceneArtistCountPhrase(scene: SceneGraphInfo): string {
  // Trust the server's truncated flag only when the numbers back it up — a
  // skewed/stale payload (total missing, zero, or ≤ the shown count) must
  // degrade to the plain count, not render "top 12 of 0 artists".
  if (scene.roster_truncated && scene.metro_roster_total > scene.artist_count) {
    return `showing top ${scene.artist_count} of ${scene.metro_roster_total} artists`
  }
  return `${scene.artist_count} ${scene.artist_count === 1 ? 'artist' : 'artists'}`
}
