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
const ARTIST_SLUG_PATTERN = /^[a-z0-9]+(?:-[a-z0-9]+)*$/

export function isArtistSlug(value: string): boolean {
  return ARTIST_SLUG_PATTERN.test(value)
}

/** The node kind the Observatory can centre on. */
const ARTIST_ENTITY_TYPE = 'artist'

/**
 * Whether a payload node is a candidate for the root param, as an ALLOWLIST.
 *
 * ONE rule for every surface that roots its link, for the reason `isArtistSlug`
 * is one rule: a denylist would hand the next node kind a payload gains to an
 * artist endpoint that 404s it, and the link would stop working with nothing to
 * catch it. Payloads that draw artists only carry no discriminator, so an
 * absent one is an artist.
 */
export function isArtistNode(node: { entity_type?: string }): boolean {
  return node.entity_type === undefined || node.entity_type === ARTIST_ENTITY_TYPE
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
