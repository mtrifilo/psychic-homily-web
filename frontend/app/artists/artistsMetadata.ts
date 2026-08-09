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
 * The `ItemList` feed for `/artists`: one slug and one name per artist.
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
 * What is local to this file: there is no `timeoutMs` override. The previous 30s
 * was a bandaid for a payload that took whole seconds to transfer, added with a
 * note that the real fix was to stop asking for it. That fix is this, so the
 * budget returns to the shared default.
 */
export function getArtistsForMetadata(): Promise<ArtistListItem[]> {
  return fetchSeoList<ArtistListItem>({
    url: `${API_BASE_URL}/artists/listing`,
    collection: 'artists',
    service: 'artists-listing',
  })
}
