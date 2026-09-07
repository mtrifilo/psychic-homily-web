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

/** The ranking rule, spelled once: most upcoming shows first, then by name. */
function byActivityThenName(a: SceneRootCandidate, b: SceneRootCandidate): number {
  return b.upcoming_show_count - a.upcoming_show_count || a.name.localeCompare(b.name)
}

/**
 * The slug of the scene's most active artist: most upcoming shows, ties by name.
 *
 * Null when nothing on the canvas has an upcoming show, which is what leaves
 * the scene's link on the plain map instead of rooting it on an arbitrary
 * band. Two populations are never candidates: a label hub is not an artist,
 * and a slugless node would build a link that resolves to the artists index
 * rather than a 404.
 */
export function pickSceneGraphRootSlug(
  nodes: readonly SceneRootCandidate[] | null | undefined,
): string | null {
  const candidates = (nodes ?? []).filter(
    node => node.slug && node.upcoming_show_count > 0 && !isLabelHubNode(node),
  )
  if (candidates.length === 0) return null
  return candidates.reduce((leader, node) =>
    byActivityThenName(node, leader) < 0 ? node : leader,
  ).slug
}
