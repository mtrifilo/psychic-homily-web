import { API_BASE_URL } from '@/lib/api-base'
import { fetchSeoList } from '@/lib/seo/fetchSeoList'

export interface ArtistListItem {
  slug: string
  name: string
}

/**
 * Deliberately wider than the shared build-time budget.
 *
 * `GET /artists` is unpaginated and has no `limit` parameter to bound it with,
 * unlike `/venues` — measured at 1.85 MB in ~0.4s against production on
 * 2026-07-29, to extract two fields per artist. The shared 10s ceiling is the
 * one the sitemap generator silently blew, and this call sits on the same
 * growth curve, needing only ~25x catalogue growth to cross it.
 *
 * This is a bandaid, and buys time proportional to the widening. The real fix
 * is to stop asking for the whole payload — a bounded endpoint, or the slug
 * projection `/sitemap/entries` already serves — and is a separate change.
 */
const ARTISTS_FETCH_TIMEOUT_MS = 30_000

export function getArtistsForMetadata(): Promise<ArtistListItem[]> {
  return fetchSeoList<ArtistListItem>({
    url: `${API_BASE_URL}/artists`,
    collection: 'artists',
    service: 'artists-listing',
    timeoutMs: ARTISTS_FETCH_TIMEOUT_MS,
  })
}
