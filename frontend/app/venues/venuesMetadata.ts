import * as Sentry from '@sentry/nextjs'
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
 * Why not simply raise that maximum: the projection is roughly ten times
 * narrower per venue, which moves the Data Cache build gate out by the same
 * factor. Every measurement, the growth curve, the comparison against
 * paginating, and the separate question of how much this weighs in the RENDERED
 * page live beside the endpoint in `contracts.VenueListingEntry`. They are not
 * restated here on purpose: the catalogue grew 50% in eleven days, so a figure
 * copied into a second file is wrong within weeks and there would be no way to
 * tell which copy was current. `fetchSeoList` weighs every response against the
 * cache budget on the way through — this fetch included; the mechanics are in
 * `lib/data-cache-budget/budget.ts`.
 *
 * A future shortfall is no longer silent: the endpoint reports `total` over the
 * browse set, read in the same snapshot as the rows, and `fetchSeoList` reports
 * whenever the list comes back short of it. Equal today, and the only way they
 * can diverge is a venue that cannot form a URL.
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
export async function getVenuesForMetadata(): Promise<VenueListItem[]> {
  const venues = await fetchSeoList<VenueListItem>({
    url: `${API_BASE_URL}/venues/listing`,
    collection: 'venues',
    service: 'venues-listing',
  })

  // A boundary check, not a restatement of the endpoint's contract. The schema
  // types `slug` as a required string and the endpoint drops rows that have
  // none, so this can only fire if that breaks — and if it does, the caller
  // would build `https://psychichomily.com/venues/` and advertise it to
  // crawlers inside an otherwise valid ItemList. `total` cannot catch that
  // case: a slug-bearing-but-empty row is counted on both sides of the
  // comparison, so it is the one shortfall the endpoint reports as complete.
  const linkable = venues.filter(venue => !!venue.slug)
  if (linkable.length < venues.length) {
    Sentry.captureMessage(
      'venues-listing: entry without a slug reached the ItemList — the endpoint is ' +
        'supposed to have dropped it, and total/count will read as complete',
      {
        level: 'error',
        tags: { service: 'venues-listing' },
        extra: { received: venues.length, linkable: linkable.length },
      }
    )
  }

  return linkable
}
