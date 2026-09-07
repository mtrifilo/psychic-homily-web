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
 * A backend slug: lowercase alphanumerics joined by single hyphens
 * (`utils.GenerateSlug`).
 *
 * ONE rule for both halves of the link. The reader needs it because the artist
 * endpoint interpolates the value straight into a request path, so a value
 * carrying `/` or `?` would aim that request somewhere else. The writer needs
 * the SAME rule or it can emit links the reader will silently refuse.
 */
export function isArtistSlug(value: string): boolean {
  return /^[a-z0-9]+(?:-[a-z0-9]+)*$/.test(value)
}

/**
 * The map's URL, rooted on `slug` when there is one to root on.
 *
 * Anything that is not a slug yields the plain path: entity slugs are nullable
 * in this schema, and a param the Observatory will refuse is worse than no
 * param, because the link still looks rooted. `encodeURIComponent` stays as a
 * second line of defence for a slug rule that later widens.
 */
export function graphRootHref(slug?: string | null): string {
  if (!slug || !isArtistSlug(slug)) return GRAPH_PATH
  return `${GRAPH_PATH}?${GRAPH_ROOT_PARAM}=${encodeURIComponent(slug)}`
}
