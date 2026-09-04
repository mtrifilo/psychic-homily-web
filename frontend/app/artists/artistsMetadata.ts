import { API_BASE_URL } from '@/lib/api-base'
import { fetchSeoList } from '@/lib/seo/fetchSeoList'
import type { components } from '@/types/api'

/**
 * The generated schema, not a hand-written twin: a backend field or nullability
 * change then shows up as a type error here rather than passing silently.
 * Deliberately NOT the `ArtistListItem` exported from `features/artists` — that
 * one extends `Artist` and is a different shape.
 */
export type ArtistListItem = components['schemas']['ArtistListingEntry']

/**
 * How many artists the `/artists` JSON-LD `ItemList` advertises.
 *
 * WHY A BOUND. The block emitted one entry per artist, and the document carried
 * that TWICE — once as the `<script>` a crawler reads, once more inside the RSC
 * flight payload. `<Link>` prefetch is served from that same prerendered buffer
 * (`collectSegmentData` slices the prerendered page into per-segment prefetch
 * payloads), so the block rode along onto every route linking here. Unbounded,
 * against ~6,200 artists, that was a 1.65 MiB document carrying 794 KB of
 * JSON-LD, and the bound removes the great majority of both.
 *
 * The before/after byte table is in the PSY-1773 PR rather than here on
 * purpose. It is a measurement of one catalogue size on one day; pinned in a
 * comment it would be re-read as a promise and would rot the first time the
 * catalogue grew. What belongs here is why the bound exists, which does not
 * change with the row count.
 *
 * WHY 100. A product default, and nothing in this file constrains it — a larger
 * number would work. `VENUE_LIST_LIMIT` is also 100, but for an unrelated
 * reason (it is `GET /venues`' declared `maximum`, which huma 422s past), so the
 * agreement is a coincidence and not a convention to preserve. What an SEO
 * `ItemList` is worth on a route whose URLs are already in a sitemap shard is
 * the question all three list pages defer; PSY-1794 is where it gets settled.
 *
 * WHAT IT COSTS. 100 of ~6,200 is a real truncation and nothing reports the
 * shortfall — the same defect PSY-1764 tracks on `/venues` at 100 of 198, an
 * order of magnitude larger. It does NOT cost discoverability: every slugged
 * artist is in the `/sitemap/artists-b0.xml` shard, a superset of this
 * activity-gated set. That is not an advantage over `/venues`, which has its own
 * shard on the same footing; sitemap coverage is why the truncation is
 * survivable on both, not why it is cheaper here. What is lost is `ItemList`
 * enrichment on the least active artists. Tracked in PSY-1794.
 *
 * WHICH 100. `GET /artists/listing` shares `artistBrowseOrder` with the browse
 * page (upcoming show count DESC, then name ASC), so taking the HEAD keeps the
 * most active. That is a property of the endpoint rather than of this file,
 * which is why a test pins it.
 *
 * WHY A SLICE, NOT `?limit=`. `ListArtistListingRequest` is an empty struct by
 * design, and huma ignores a query parameter no input field declares — so
 * `?limit=100` would be accepted, ignored, and return the whole catalogue while
 * READING as though it were enforced. That argues against FAKING the bound in
 * the URL, not against declaring the parameter: the endpoint has exactly one
 * consumer, so giving it a real `limit` is a few lines of Go and strictly
 * better than this — it would shrink the cache entry and the origin transfer
 * too, neither of which the slice touches. That is PSY-1794, deliberately not
 * done here because this ticket is frontend-only. When it lands, move the bound
 * into the URL and drop the slice.
 *
 * It bounds the JSON-LD ONLY. The list a human reads is untouched and is still
 * client-fetched unbounded; `app/artists/page.tsx` documents why that half is
 * deferred.
 */
export const ARTIST_ITEM_LIST_LIMIT = 100

/**
 * The `ItemList` feed for `/artists`: one slug and one name per artist, bounded
 * at `ARTIST_ITEM_LIST_LIMIT`.
 *
 * IT READS A PROJECTION ENDPOINT, NOT `GET /artists`. That endpoint sends
 * sixteen fields per artist and this page reads two, which stopped being merely
 * wasteful once the response outgrew Vercel's 2 MB cache-item cap: over the cap
 * nothing is cached, so every render re-pulled the whole catalogue from origin.
 * Measured 2026-08-08 at 3,233,345 raw bytes, 206% of the cap once base64
 * encoded, against 311,240 bytes for the projection.
 *
 * The full measurement set, the dated growth curve, why trimming beat sharding
 * and paginating, and why the set of artists is unchanged all live in ONE place
 * — `contracts.ArtistListingEntry` in the backend, beside the endpoint. The
 * cache mechanics that make the cap bind (the base64 envelope, Next's warn-and-
 * drop behaviour) live in `lib/data-cache-budget/budget.ts`. Both are linked
 * rather than restated so the numbers cannot drift apart.
 *
 * THE SLICE DOES NOT SHRINK THAT CACHE ENTRY. The request is unchanged and the
 * response is still the whole catalogue, so this URL still caches the same
 * 311,240-byte body (414,988 as the base64 entry the cap is applied to). What
 * the bound shrinks is the rendered page. Two different problems with two
 * different fixes, and confusing them would leave the next reader thinking this
 * file protects the cache budget. It does not — the projection endpoint does.
 *
 * What is local to this file: there is no `timeoutMs` override. The previous 30s
 * was a bandaid for a payload that took whole seconds to transfer, added with a
 * note that the real fix was to stop asking for it. Moving to the projection
 * endpoint was that fix, so the budget returns to the shared default.
 *
 * A failed fetch yields `[]`, and so no block at all — `fetchSeoList` fails open
 * on purpose, and `slice` on an empty array preserves that. The ONE exception is
 * `DataCacheBudgetError`, which that helper deliberately re-throws; it is the
 * build gate and it is meant to escape here uncaught.
 */
export async function getArtistsForMetadata(): Promise<ArtistListItem[]> {
  const artists = await fetchSeoList<ArtistListItem>({
    url: `${API_BASE_URL}/artists/listing`,
    collection: 'artists',
    service: 'artists-listing',
  })
  return artists.slice(0, ARTIST_ITEM_LIST_LIMIT)
}
