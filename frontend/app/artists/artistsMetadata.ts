import { API_BASE_URL } from '@/lib/api-base'
import { fetchSeoList } from '@/lib/seo/fetchSeoList'

export interface ArtistListItem {
  slug: string
  name: string
}

/**
 * The `ItemList` feed for `/artists`: one slug and one name per artist.
 *
 * IT READS A PROJECTION ENDPOINT, NOT `GET /artists`, and that is the whole
 * point of PSY-1674 — measured against production on 2026-08-08, 6,279 artists:
 *
 *                          raw bytes    base64      % of the 2 MB item cap
 *   GET /artists           3,233,345    4,311,128   206%   (not cached)
 *   GET /artists/listing     311,240      414,988    20%
 *
 * Vercel caps a single Data Cache / Runtime Cache item at 2 MB, and "items
 * larger won't be cached". Next enforces the same cap itself: over it, the
 * entry is simply not written. There IS one signal — `console.warn` in a build,
 * a thrown error in dev — but nothing fails, so it scrolled past unread for at
 * least ten days. The build that fixed this printed:
 *
 *   Failed to set Next.js data cache for https://api.psychichomily.com/artists,
 *   items over 2MB can not be cached (4312227 bytes)
 *
 * Note the 4.3 MB against a 3.2 MB response: the stored body is base64-encoded,
 * which is not folklore. Decoding a real `.next/cache/fetch-cache` entry for
 * this URL showed 1,149,272 raw bytes held as a 1,533,430-byte entry, 1.334x.
 * So the effective RAW budget is ~1.5 MB, and the encoded column above binds.
 *
 * `GET /artists` crossed that line between 2026-07-26 (the last entry that
 * cached, at 73% of the cap) and 2026-07-29, and every render since re-pulled
 * the full body from origin. Nothing anywhere reported it. `lib/data-cache-budget`
 * is the gate that now fails the build rather than letting it recur quietly.
 *
 * WHY TRIMMING RATHER THAN SHARDING OR PAGINATING. The fields were the problem,
 * not the row count: `GET /artists` sends sixteen per artist and this page reads
 * two. Sharding (the sitemap's answer, `app/sitemap.ts`) would keep carrying the
 * other fourteen and need its shard count revisited on every growth spurt;
 * truncating with a `limit` would silently drop URLs from the ItemList, which is
 * the defect `/venues` already exhibits at 100 of 198. The projection is a 10.4x
 * reduction — roughly 32,000 artists of headroom against 6,279 today.
 *
 * The set is unchanged: `/artists/listing` applies the same activity gate and
 * sort as the unfiltered list endpoint, so the ItemList still advertises exactly
 * the artists the page lists. Slugless rows are dropped server-side now; the
 * page's own `slug` filter stays as the type narrowing it also performs.
 *
 * No timeout override. The previous 30s was a bandaid for a payload that took
 * whole seconds to transfer, added with a note that the real fix was to stop
 * asking for it; that fix is this, so the budget returns to the shared default.
 */
export function getArtistsForMetadata(): Promise<ArtistListItem[]> {
  return fetchSeoList<ArtistListItem>({
    url: `${API_BASE_URL}/artists/listing`,
    collection: 'artists',
    service: 'artists-listing',
  })
}
