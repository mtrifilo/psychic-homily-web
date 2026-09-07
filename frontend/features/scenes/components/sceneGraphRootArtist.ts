import { isLabelHubNode } from '@/components/graph/labelHub'

/**
 * Which artist a scene's `/graph` link opens the map rooted on.
 *
 * Read off the scene-graph payload the section already holds rather than the
 * roster endpoint next door: `SceneArtistResponse.show_count` is every approved
 * show an artist has ever played, while the rule this implements is UPCOMING
 * activity, and only the graph node carries that count.
 */

/** The node fields the pick reads, structural so callers keep their own types. */
interface SceneRootCandidate {
  slug: string
  name: string
  upcoming_show_count: number
  entity_type?: string
}

/**
 * The scene's most active artist: most upcoming shows, ties broken by name.
 *
 * Null when nothing on the canvas has an upcoming show, which is what leaves
 * the scene's link on the plain map instead of rooting it on an arbitrary
 * band. Two populations are never candidates: a label hub is not an artist,
 * and a slugless node would build a link that resolves to the artists index
 * rather than a 404.
 */
export function pickSceneGraphRootArtist<T extends SceneRootCandidate>(
  nodes: readonly T[] | null | undefined,
): T | null {
  let best: T | null = null
  for (const node of nodes ?? []) {
    if (isLabelHubNode(node)) continue
    if (!node.slug) continue
    if (node.upcoming_show_count <= 0) continue
    if (best === null) {
      best = node
      continue
    }
    if (node.upcoming_show_count > best.upcoming_show_count) {
      best = node
      continue
    }
    if (
      node.upcoming_show_count === best.upcoming_show_count &&
      node.name.localeCompare(best.name) < 0
    ) {
      best = node
    }
  }
  return best
}
