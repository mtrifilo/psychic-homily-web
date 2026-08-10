import { API_BASE_URL } from '@/lib/api-base'
import { fetchSeoList } from '@/lib/seo/fetchSeoList'
import type { components } from '@/types/api'

/**
 * The generated schema, not a hand-written twin: a backend field or nullability
 * change then shows up as a type error here rather than passing silently.
 * Deliberately NOT the `Venue` shape `features/venues` exports — that one is the
 * browse row and is far wider.
 */
export type VenueListItem = components['schemas']['VenueListingEntry']

/**
 * The `ItemList` feed for `/venues`: one slug and one name per venue.
 *
 * IT READS A PROJECTION ENDPOINT, NOT `GET /venues`, AND CARRIES NO LIMIT. Both
 * halves are the fix, and the second is the one the ticket was about.
 *
 * What this replaces: `GET /venues?limit=100` against a set of 297, so the
 * `ItemList` advertised the hundred most active venues and dropped the rest —
 * the response carried `total` and the call threw it away. The 100 was never a
 * product decision. The call had asked for 200, `GET /venues` declares
 * `maximum:"100"`, huma answered 422 before the handler ran, and `fetchSeoList`
 * fails open, so `/venues` rendered with no `ItemList` at all for months.
 * Capping at 100 fixed the 422 and left a quieter version of the same silence.
 *
 * Why not simply raise that maximum: the row is 659 raw bytes wide, so the whole
 * verified set is 12.4% of the 2 MB Data Cache item cap today and reaches the
 * build gate at ~1,900 venues. The catalogue went 198 → 297 in eleven days. The
 * projection is 61.6 bytes per venue — 18,289 raw bytes for all 297, 1.2% of the
 * cap — and reaches the gate at ~20,400. The full measurement set and the
 * comparison against paginating live in ONE place, `contracts.VenueListingEntry`
 * in the backend, beside the endpoint, so the numbers cannot drift apart from
 * the code they justify. The cache mechanics that make the cap bind live in
 * `lib/data-cache-budget/budget.ts`, and `fetchSeoList` weighs every response
 * against it on the way through — this fetch included.
 *
 * A future shortfall is no longer silent: the endpoint reports `total` counted
 * over the browse set independently of the array, and `fetchSeoList` raises a
 * Sentry event whenever the list comes back short of it. Equal today, and the
 * only way they can diverge is a venue that cannot form a URL.
 *
 * NOT consolidated with the browse page's own first-screen fetch, which reads
 * `GET /venues?limit=50` through `lib/ssr/fetchListPayload.ts` inside a Suspense
 * boundary. They want different things — every venue as schema, the first page
 * as rows — and joining them would put the streamed fetch behind the blocking
 * one. So `/venues` keeps two Data Cache entries with independently expiring
 * windows, and the two lists can disagree across a window boundary. Harmless:
 * one is schema, one is rows.
 */
export function getVenuesForMetadata(): Promise<VenueListItem[]> {
  return fetchSeoList<VenueListItem>({
    url: `${API_BASE_URL}/venues/listing`,
    collection: 'venues',
    service: 'venues-listing',
  })
}
