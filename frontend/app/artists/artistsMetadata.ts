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
 * unlike `/venues` — measured at 1,881,989 bytes in ~0.4s against production on
 * 2026-07-29 (3,644 artists), to extract two fields per artist.
 *
 * The timeout is NOT the ceiling that binds first. Vercel's Data Cache drops
 * any item over 2 MB, and this response is at 94% of that — roughly 6%
 * catalogue growth away, versus the ~25x growth needed to blow even the
 * original 10s budget. When it crosses, `next: { revalidate }` silently stops
 * caching and every revalidation re-pulls 1.9 MB from origin, which is the same
 * shape of silent degradation this ticket exists to remove. Widening the
 * timeout does nothing about that; only bounding the payload does.
 *
 * The cost side, which the 10s ceiling existed to hold down: this await sits
 * ABOVE the `<Suspense>` boundary in `page.tsx`, so nothing streams until it
 * settles — the budget is a worst-case blank-page window on a user-facing page.
 * 30s is accepted only because the route is `revalidate: 3600` ISR, so the path
 * is cold render and on-demand revalidation, not per-request.
 *
 * This is a bandaid, and buys time proportional to the widening. The real fix
 * is to stop asking for the whole payload: either a `limit` parameter on
 * `GET /artists`, or a slug+name projection. Note `/sitemap/entries` is NOT
 * usable as-is — `contracts.SitemapEntry` is `{slug, updated_at}` with no
 * `name`, so reusing it would silently drop the name from every `ListItem`.
 * Moving this fetch below the Suspense boundary would remove the tradeoff
 * entirely. All are separate changes.
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
