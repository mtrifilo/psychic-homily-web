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
 * Why not simply raise that maximum: the projection is ~10x narrower per venue,
 * which moves the Data Cache build gate from ~1,900 venues to ~20,400 against
 * 297 today — and the catalogue went 198 → 297 in eleven days. The measurements,
 * the growth curve, and the comparison against paginating live in ONE place,
 * `contracts.VenueListingEntry` in the backend, beside the endpoint, so the
 * numbers cannot drift apart from the code they justify. `fetchSeoList` weighs
 * every response against that budget on the way through — this fetch included;
 * the mechanics are in `lib/data-cache-budget/budget.ts`.
 *
 * A future shortfall is no longer silent: the endpoint reports `total` counted
 * over the browse set independently of the array, and `fetchSeoList` raises a
 * Sentry event whenever the list comes back short of it. Equal today, and the
 * only way they can diverge is a venue that cannot form a URL.
 *
 * NOT consolidated with the browse page's own first-screen fetch, which reads
 * `GET /venues?limit=50` through `lib/ssr/fetchListPayload.ts` inside a Suspense
 * boundary. They want different things — every venue as schema, the first page
 * as rows — so `/venues` keeps two Data Cache entries with independently
 * expiring windows, and the two lists can disagree across a window boundary.
 * Harmless: one is schema, one is rows.
 *
 * This fetch is AWAITED IN THE PAGE BODY, which does serialise the Suspense
 * subtree's fetches behind it — that subtree's element does not exist until this
 * resolves. Deliberate: a JSON-LD block that streams in after the first flush is
 * worth less than one a crawler reads immediately, and the route is ISR on an
 * hourly window, so the cost lands on cold renders rather than on steady-state
 * TTFB. Hoisting it into a sibling async component would recover the overlap at
 * that price.
 */
export function getVenuesForMetadata(): Promise<VenueListItem[]> {
  return fetchSeoList<VenueListItem>({
    url: `${API_BASE_URL}/venues/listing`,
    collection: 'venues',
    service: 'venues-listing',
  })
}
