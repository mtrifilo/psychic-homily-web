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
 * 100, the same number as `VENUE_LIST_LIMIT`, chosen to match rather than
 * argued from artist-specific evidence — how many entries an SEO `ItemList`
 * should carry is an open product question on all three list pages, and
 * `UPCOMING_SHOWS_LIMIT` says the same thing from the shows side.
 *
 * WHAT IT FIXES. The block serialised one entry per artist, TWICE per document
 * — once as the `<script>` a crawler reads, and once again inside the RSC
 * flight payload, which `<Link>` prefetch then pulled onto `/`, `/atlas` and
 * `/shows`. Reproduced locally against a 6,210-artist catalogue, production
 * build, `GET /artists`:
 *
 *                          gzipped     raw     ld+json    flight
 *   unbounded              124,286  1,733,120  794,370   910,196
 *   bounded at 100          PENDING    PENDING  PENDING   PENDING
 *
 * The `ItemList` is the entire reason those routes carried the catalogue, so
 * bounding it is what removes the leak. Moving the `await` below the Suspense
 * boundary does NOT: the flight carries a subtree's output whether it streamed
 * or was in the shell, so that only changes when the bytes arrive.
 *
 * WHAT IT COSTS, and why it is cheap here in a way it is not on `/venues`.
 * `VENUE_LIST_LIMIT` carries a warning that its bound silently drops 98 of 198
 * venues from the `ItemList` (now PSY-1764). The truncation is far larger here
 * — 100 of ~6,200 — but it is not the same defect, because `/artists` has the
 * discovery channel `/venues` is leaning on its `ItemList` for: `app/sitemap.ts`
 * prerenders a dedicated `/sitemap/artists.xml` shard covering EVERY slugged
 * artist, a superset of this activity-gated browse set. No artist URL becomes
 * undiscoverable by being cut from this block. What is lost is `ItemList`
 * enrichment on the least active, which is worth well under the ~1.6 MB per
 * document it was costing.
 *
 * WHICH 100. `GET /artists/listing` shares its ORDER BY with the browse page
 * (`artistBrowseOrder`: upcoming show count DESC, then name ASC), so taking the
 * HEAD keeps the 100 most active artists — the same selection `/venues` makes,
 * and the right end of the list to keep. That is a property of the endpoint,
 * not of this file, which is why a test asserts it rather than a comment.
 *
 * APPLIED AS A SLICE, NOT A QUERY PARAMETER, and that is a fact about the
 * endpoint rather than a preference. `ListArtistListingRequest` is an empty
 * struct — deliberately, per its own doc comment — and huma silently ignores a
 * query parameter no input field declares. So `?limit=100` would be accepted,
 * ignored, and return all 6,155 entries while READING as though it were
 * bounded: a bound that looks enforced and is not. The slice cannot lie that
 * way. When the endpoint grows a real `limit`, move it into the URL here and
 * drop the slice; the constant and the call site do not otherwise change.
 *
 * It does NOT bound the list a human reads. `ArtistList` fetches `GET /artists`
 * itself, client-side and unbounded, and that request is untouched by this —
 * bounding it needs backend `limit`/`offset` support (PSY-1774) before the
 * first screen can be server-seeded the way `/shows` and `/venues` are.
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
 * response is still the whole catalogue, so the 311,240-byte entry measured
 * above is exactly what this URL still caches; what the bound shrinks is the
 * rendered page. Two different problems, two different fixes, and confusing
 * them would make the next reader think this file protects the cache budget. It
 * does not — the projection endpoint does.
 *
 * What is local to this file: there is no `timeoutMs` override. The previous 30s
 * was a bandaid for a payload that took whole seconds to transfer, added with a
 * note that the real fix was to stop asking for it. That fix is this, so the
 * budget returns to the shared default.
 *
 * A failed fetch yields `[]`, and so no block at all — `fetchSeoList` fails open
 * on purpose, and `slice` on an empty array preserves that.
 */
export async function getArtistsForMetadata(): Promise<ArtistListItem[]> {
  const artists = await fetchSeoList<ArtistListItem>({
    url: `${API_BASE_URL}/artists/listing`,
    collection: 'artists',
    service: 'artists-listing',
  })
  return artists.slice(0, ARTIST_ITEM_LIST_LIMIT)
}
