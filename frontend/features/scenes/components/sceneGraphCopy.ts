import type { SceneGraphInfo } from '../types'

/**
 * The select-gesture sentence appended to graph canvas aria-labels under the
 * locked click-selects grammar: click no longer navigates, so the label must
 * set that expectation. One shared literal so assistive tech can't hear
 * different gesture affordances for the same interaction across surfaces
 * (HomeSceneGraph currently inlines the identical sentence — candidate to
 * adopt this constant when that surface is next touched).
 */
export const graphSelectGestureHint = 'Click a node for that artist’s details.'

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
  //
  // Deliberately NOT "showing top …": the header also renders on surfaces
  // where the canvas doesn't (mobile gate, pre-measurement), and "showing"
  // would assert a visualization that isn't there. "top N of M" is
  // surface-agnostic scale info, honest with or without a canvas.
  if (scene.roster_truncated && scene.metro_roster_total > scene.artist_count) {
    return `top ${scene.artist_count} of ${scene.metro_roster_total} artists`
  }
  return `${scene.artist_count} ${scene.artist_count === 1 ? 'artist' : 'artists'}`
}
