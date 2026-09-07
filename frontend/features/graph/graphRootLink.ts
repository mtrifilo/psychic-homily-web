/**
 * The `/graph` deep-link shape: `/graph?artist=<slug>`.
 *
 * One module owns the param name and the href, so the surfaces that WRITE the
 * link and the Observatory that READS it cannot drift onto different
 * spellings. A slug, not an id: it is the form a shared URL can be read in,
 * and the form every other entity link on the site already carries.
 *
 * KEEP THIS MODULE DEPENDENCY-FREE. It is imported by `features/scenes`, whose
 * chunk must not pull the Observatory (and the force-graph canvas behind it)
 * in through a link helper.
 */

/** The map itself, with nothing rooted. */
export const GRAPH_PATH = '/graph'

/** The query key naming the artist the Observatory opens rooted on. */
export const GRAPH_ROOT_PARAM = 'artist'

/**
 * The map's URL, rooted on `slug` when there is one to root on.
 *
 * An empty or absent slug yields the plain path. Entity slugs are nullable in
 * this schema, and `?artist=` names nothing the Observatory can resolve, so a
 * blank param is worse than no param.
 */
export function graphRootHref(slug?: string | null): string {
  if (!slug) return GRAPH_PATH
  return `${GRAPH_PATH}?${GRAPH_ROOT_PARAM}=${encodeURIComponent(slug)}`
}
