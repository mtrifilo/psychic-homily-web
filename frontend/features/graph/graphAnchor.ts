/**
 * The anchor every /graph surface re-roots on, and the one rule that decides
 * whether a catalog target is offerable at all.
 *
 * Extracted so the rule has a single home. The Observatory takes artists from
 * four sources — the search box, a graph node, the random rabbit hole, and the
 * zero state's ranked suggestions — and a source that writes its own
 * "is this usable" check is a source that will drift from the others. The
 * failure it drifts into is specific: a button whose accessible name promises
 * an artist, wired to an id or slug that centres nothing.
 */

/** An artist the graph can be centred on. */
export interface GraphAnchor {
  id: number
  slug: string
  name: string
}

/**
 * A catalog target as the API hands it over: the same three fields on every
 * endpoint that returns "an artist to go to", any of which the wire type
 * allows to be missing.
 *
 * Structural on purpose. Both `RandomArtistTargetResponse` (nullable by
 * contract) and `GraphStartingPoint` (non-null by contract) satisfy it, so one
 * narrowing serves both without either endpoint's generated type being
 * imported here.
 */
export interface CatalogArtistTarget {
  artist_id?: number | null
  artist_slug?: string | null
  artist_name?: string | null
}

/**
 * Narrows a catalog target to an anchor, or null when it cannot be honoured.
 *
 * All three fields are required. The id is what re-roots the graph, and the
 * slug is what the artist page link needs — an entity slug is nullable in this
 * schema, and an empty one resolves to the artists INDEX rather than a 404, so
 * a slug-less anchor sends a click somewhere actively wrong.
 */
export function anchorFromCatalogTarget(
  target: CatalogArtistTarget | null | undefined,
): GraphAnchor | null {
  if (!target?.artist_id || !target.artist_slug || !target.artist_name) {
    return null
  }
  return { id: target.artist_id, slug: target.artist_slug, name: target.artist_name }
}
