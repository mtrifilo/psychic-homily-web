import type { SceneGraphInfo } from '../types'
import { truncatedCountPhrase } from '@/components/graph/truncatedCountPhrase'

/**
 * The ONE source for "how many artists is this graph showing" (PSY-1296) —
 * consumed by both the visual header (SceneGraph) and the canvas aria-label
 * (SceneGraphVisualization), so sighted users and assistive tech can never
 * hear different numbers or framings for the same graph.
 *
 * `artist_count` is the contract field the truncation flag is defined
 * against (backend guarantees it equals the shipped node count), so the
 * phrase reads it rather than a locally derived nodes.length.
 *
 * The "top N of M" / plain-count logic (and its stale-payload guard) is the
 * shared `truncatedCountPhrase` (PSY-1476), which the venue and collection
 * graphs also use — this wrapper just binds the scene's field names and the
 * "artist(s)" noun. Deliberately NOT "showing top …": the header also renders
 * where the canvas doesn't (mobile gate, pre-measurement), so the phrase stays
 * surface-agnostic, honest with or without a canvas.
 */
export function sceneArtistCountPhrase(scene: SceneGraphInfo): string {
  return truncatedCountPhrase({
    shown: scene.artist_count,
    total: scene.metro_roster_total,
    truncated: scene.roster_truncated,
    singular: 'artist',
    plural: 'artists',
  })
}
