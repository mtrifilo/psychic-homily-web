import { API_BASE_URL } from '@/lib/api-base'
import { fetchSeoList } from '@/lib/build-time-api'

export interface ArtistListItem {
  slug: string
  name: string
}

/**
 * Deliberately wider than the shared 10s build-time budget.
 *
 * `GET /artists` is unpaginated and has no `limit` parameter to bound it with,
 * unlike `/venues` — measured at 1.85 MB in ~0.4s against production on
 * 2026-07-29. The shared 10s ceiling is the one the sitemap generator silently
 * blew, and this call sits on the same growth curve, needing only ~25x
 * catalogue growth to cross it.
 *
 * Widening the ceiling buys headroom; it does not remove the growth curve. It
 * is the right knob for "stop a wedged backend hanging the render" and the
 * wrong one for "bound a response that keeps getting bigger" — that bound is a
 * `limit` parameter on the backend endpoint, which is a separate change.
 */
const ARTISTS_FETCH_TIMEOUT_MS = 30_000

/** Feeds the JSON-LD `ItemList` only — see `fetchSeoList` for why it fails open. */
export function getArtistsForMetadata(
  fetchArtists: typeof fetch = fetch
): Promise<ArtistListItem[]> {
  return fetchSeoList<ArtistListItem>({
    url: `${API_BASE_URL}/artists`,
    collection: 'artists',
    service: 'artists-listing',
    timeoutMs: ARTISTS_FETCH_TIMEOUT_MS,
    fetchImpl: fetchArtists,
  })
}
