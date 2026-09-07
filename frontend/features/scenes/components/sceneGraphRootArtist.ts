import { isArtistSlug } from '@/features/graph/graphRootLink'

/**
 * Which artist a scene's `/graph` link opens the map rooted on.
 *
 * Read off the scene-graph payload the section already holds rather than the
 * roster endpoint next door: `SceneArtistResponse.show_count` is every approved
 * show an artist has ever played, while the rule this implements is UPCOMING
 * activity, and only the graph node carries that count.
 *
 * The pool is therefore the drawn graph's node set, which the backend has
 * already narrowed to the top-N metro artists by all-time approved shows. A
 * band with more upcoming shows than anyone but too little history to make that
 * cut is not a candidate. That is a deliberate narrowing, not an oversight: the
 * link sits under the canvas and should name something on it. Hidden clusters
 * are NOT consulted for the same reason in reverse — the link describes the
 * scene, so toggling a legend pill must not change where it points.
 */

/** The node fields the pick reads, structural so callers keep their own types. */
interface SceneRootCandidate {
  slug: string
  name: string
  upcoming_show_count: number
  entity_type?: string
}

const ARTIST_ENTITY_TYPE = 'artist'

/**
 * Artists only, as an ALLOWLIST.
 *
 * A denylist on the label hub would hand the next node kind the backend emits
 * (a venue hub, a festival) straight to an artist endpoint that 404s it, and
 * the deep link would quietly stop working with nothing to catch it. Payloads
 * served before hubs shipped carry no discriminator, so an absent one is an
 * artist.
 */
function isArtistNode(node: SceneRootCandidate): boolean {
  return node.entity_type === undefined || node.entity_type === ARTIST_ENTITY_TYPE
}

/** The ranking rule, spelled once: most upcoming shows first, then by name. */
function byUpcomingThenName(a: SceneRootCandidate, b: SceneRootCandidate): number {
  return b.upcoming_show_count - a.upcoming_show_count || a.name.localeCompare(b.name)
}

/**
 * The slug of the scene artist with the most upcoming shows, ties broken by
 * name.
 *
 * NAMED FOR WHAT IT MEASURES. "Most active" is taken: the home scene graph
 * blends connections and upcoming shows under that phrase and publishes the
 * blend as user-facing copy. This is bookings alone.
 *
 * Null when nothing on the canvas has an upcoming show, which is what leaves
 * the scene's link on the plain map instead of rooting it on an arbitrary band.
 *
 * A node whose slug the Observatory would refuse is not a candidate either, so
 * the link falls through to the next artist rather than to nothing. The slug
 * column is a free nullable string: an empty one builds a link that resolves to
 * the artists index rather than a 404, and a value outside the generated shape
 * is one the reader will not honour.
 */
export function pickMostBookedSceneArtistSlug(
  nodes: readonly SceneRootCandidate[] | null | undefined,
): string | null {
  const candidates = (nodes ?? []).filter(
    node => isArtistSlug(node.slug) && node.upcoming_show_count > 0 && isArtistNode(node),
  )
  if (candidates.length === 0) return null
  return candidates.reduce((leader, node) =>
    byUpcomingThenName(node, leader) < 0 ? node : leader,
  ).slug
}
