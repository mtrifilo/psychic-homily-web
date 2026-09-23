import { entityHref } from '@/lib/entity-slug'

/**
 * The one address a show is canonically served at.
 *
 * `GET /shows/{id_or_slug}` resolves a segment of digits as a numeric id and
 * anything else as a slug, so a show is reachable at two paths. The slug path
 * is the canonical one; the id path redirects to it.
 *
 * Imported by `proxy.ts`, so this module must stay free of `features/` imports
 * and of anything that renders.
 */

const NUMERIC_SHOW_SEGMENT = /^\d+$/

/** Whether the backend reads this route segment as a numeric show id. */
export function isNumericShowSegment(segment: string): boolean {
  return NUMERIC_SHOW_SEGMENT.test(segment)
}

/**
 * `/shows/{slug}` when a URL can address the show by its slug, else null.
 *
 * Null wherever `entityHref` refuses the slug, and for a slug of digits alone,
 * which the backend would read as an id and resolve to a different show.
 */
function showSlugPath(slug: string | null | undefined): string | null {
  if (slug && isNumericShowSegment(slug.trim())) return null
  return entityHref('/shows', slug)
}

/**
 * The site-relative path the show page declares canonical: the loaded show's
 * slug path, or the requested segment when the show has no usable slug.
 */
export function showCanonicalPath(
  requestedSegment: string,
  loadedSlug: string | null | undefined
): string {
  return (
    showSlugPath(loadedSlug) ?? `/shows/${encodeURIComponent(requestedSegment)}`
  )
}

/**
 * Where a request for `/shows/{requestedSegment}` permanently redirects, or
 * null when it is served where it is.
 *
 * Only a numeric request redirects, and only to a slug path. The target is
 * never numeric, so following it cannot redirect again.
 */
export function showSlugRedirectPath(
  requestedSegment: string,
  loadedSlug: string | null | undefined
): string | null {
  if (!isNumericShowSegment(requestedSegment)) return null
  return showSlugPath(loadedSlug)
}
