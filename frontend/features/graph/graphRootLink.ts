/**
 * The `/graph` deep-link shape: `/graph?artist=<slug>`.
 *
 * The param name lives here and is imported by both halves, so the surfaces
 * that build the link and the Observatory that reads it cannot drift onto
 * different spellings. A slug rather than an id because a shared URL has to be
 * readable, and because it is the form every other entity link already uses.
 *
 * KEEP THIS MODULE DEPENDENCY-FREE. It is imported by `features/scenes`, whose
 * chunk must not pull the Observatory (and the force-graph canvas behind it)
 * in through a link helper.
 */

const GRAPH_PATH = '/graph'

/** The query key naming the artist the Observatory opens rooted on. */
export const GRAPH_ROOT_PARAM = 'artist'

/**
 * The map's URL, rooted on `slug` when there is one to root on.
 *
 * Entity slugs are nullable in this schema, and `?artist=` names nothing the
 * Observatory can resolve, so an empty slug yields the plain path.
 */
export function graphRootHref(slug?: string | null): string {
  if (!slug) return GRAPH_PATH
  return `${GRAPH_PATH}?${GRAPH_ROOT_PARAM}=${encodeURIComponent(slug)}`
}
