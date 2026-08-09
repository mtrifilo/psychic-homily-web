import { Suspense } from 'react'
import { HydrationBoundary } from '@tanstack/react-query'
import { VenueList } from '@/features/venues'
import {
  VENUE_LIST_FIRST_SCREEN_KEY,
  VENUE_LIST_FIRST_SCREEN_URL,
  venueEndpoints,
  venueQueryKeys,
} from '@/features/venues/api'
import type {
  VenueCitiesResponse,
  VenuesListResponse,
} from '@/features/venues/types'
import { JsonLd } from '@/components/seo/JsonLd'
import { API_BASE_URL } from '@/lib/api-base'
import { fetchSeoList } from '@/lib/seo/fetchSeoList'
import { generateItemListSchema, generateBreadcrumbSchema } from '@/lib/seo/jsonld'
import { seedFirstScreen } from '@/lib/query-hydration'
import { fetchListPayload } from '@/lib/ssr/fetchListPayload'

export const metadata = {
  title: 'Venues',
  description: 'Browse music venues and discover upcoming shows.',
  alternates: {
    canonical: 'https://psychichomily.com/venues',
  },
  openGraph: {
    title: 'Venues | Psychic Homily',
    description: 'Browse music venues and discover upcoming shows.',
    url: '/venues',
    type: 'website',
  },
}

interface VenueListItem {
  slug: string
  name: string
}

/**
 * The endpoint's ceiling, not a product choice. `GET /venues` declares
 * `maximum:"100"` on `limit`, and huma enforces that as a 422 before the
 * handler runs. This call asked for 200, so it 422'd on every render and the
 * fail-open below dropped the `ItemList` — verified absent from the production
 * `/venues` HTML on 2026-07-29, and reproduced directly against the API. Raising
 * this past 100 needs the backend maximum raised first.
 *
 * This ALREADY truncates: production reports `total: 198` (2026-07-29), so the
 * `ItemList` covers 100 of 198, and the 98 omitted are the least active — the
 * list is sorted by upcoming show count. Nothing reports the shortfall; the
 * response carries `total` and this call discards it. That is a quieter version
 * of the same failure class, and it is why the fix is to raise the backend
 * maximum or paginate with `offset`, not to leave the cap here. Going from
 * "none" to "the 100 most active" is still strictly better than the 422 this
 * replaces, which is the only reason it ships in this state.
 *
 * NOT consolidated with the first-screen fetch below, unlike `/shows`, which
 * reads one response for both consumers. The two genuinely differ here: the
 * `ItemList` wants the 100 most active venues, the browse page's first screen
 * wants the 50 the client hook asks for. So `/venues` does keep two Data Cache
 * entries with independently expiring windows, and the two lists can disagree
 * across a window boundary. Harmless — one is schema, one is rows — but it is
 * a real divergence from the shows page, recorded so the next reader does not
 * assume the consolidation was applied everywhere.
 */
export const VENUE_LIST_LIMIT = 100

/**
 * Data Cache exposure, measured against production 2026-08-08 (PSY-1674), when
 * `/artists` was found 206% over the 2 MB cache-item cap and silently uncached:
 *
 *   GET /venues?limit=100    71,172 raw    94,896 base64     4.5% of the cap
 *   GET /venues?limit=50     35,226 raw    46,968 base64     2.2%
 *   GET /venues/cities        5,668 raw     7,560 base64     0.4%
 *
 * `/venues` is not exposed, and the reason is structural rather than luck: the
 * two list fetches carry an explicit `limit`, so they grow with the page size
 * rather than the catalogue. (`/venues/cities` carries none, but it is a facet
 * aggregate — one row per city — so it grows with cities, not venues.)
 * `GET /artists` had no limit at all, which is what let it run away.
 *
 * The truncation described on the constant above is a SEPARATE, live defect and
 * is now tracked as PSY-1764 rather than only recorded here.
 */
export function getVenues(): Promise<VenueListItem[]> {
  return fetchSeoList<VenueListItem>({
    url: `${API_BASE_URL}/venues?limit=${VENUE_LIST_LIMIT}`,
    collection: 'venues',
    service: 'venues-listing',
  })
}

function VenueListLoading() {
  return (
    <div className="flex justify-center items-center py-12">
      <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-foreground"></div>
    </div>
  )
}

/**
 * Seed the two cache entries `VenueList` blocks its first paint on — the first
 * page of venues and the city facet counts — so the venue rows reach the
 * server HTML (PSY-1624).
 *
 * BOTH are required. `VenueList` returns its spinner while either query is
 * still loading, so seeding the rows alone server-renders the spinner.
 *
 * A failed fetch renders `<VenueList />` unseeded rather than throwing; the
 * component fetches for itself and owns the error state (see
 * `fetchListPayload`).
 */
async function HydratedVenueList() {
  const [venues, cities] = await Promise.all([
    fetchListPayload<VenuesListResponse>({
      url: VENUE_LIST_FIRST_SCREEN_URL,
      collection: 'venues',
      service: 'venues-first-screen',
    }),
    fetchListPayload<VenueCitiesResponse>({
      url: venueEndpoints.CITIES,
      collection: 'cities',
      service: 'venue-cities-first-screen',
    }),
  ])

  if (!venues || !cities) {
    return <VenueList />
  }

  const dehydratedState = await seedFirstScreen([
    { queryKey: VENUE_LIST_FIRST_SCREEN_KEY, data: venues },
    { queryKey: venueQueryKeys.cities, data: cities },
  ])

  return (
    <HydrationBoundary state={dehydratedState}>
      <VenueList />
    </HydrationBoundary>
  )
}

export default async function VenuesPage() {
  const venues = await getVenues()

  const venuesWithSlugs = venues.filter(
    (v): v is VenueListItem & { slug: string } => !!v.slug
  )

  return (
    <>
      {venuesWithSlugs.length > 0 && (
        <JsonLd data={generateItemListSchema({
          name: 'Venues',
          description: 'Music venues in Phoenix and beyond.',
          listItems: venuesWithSlugs.map(venue => ({
            url: `https://psychichomily.com/venues/${venue.slug}`,
            name: venue.name,
          })),
        })} />
      )}
      <JsonLd data={generateBreadcrumbSchema([
        { name: 'Home', url: 'https://psychichomily.com' },
        { name: 'Venues', url: 'https://psychichomily.com/venues' },
      ])} />
      <div className="flex min-h-screen items-start justify-center">
        <main className="w-full max-w-6xl px-4 py-8 md:px-8">
          <h1 className="text-3xl font-bold text-center mb-8">Venues</h1>
          <Suspense fallback={<VenueListLoading />}>
            <HydratedVenueList />
          </Suspense>
        </main>
      </div>
    </>
  )
}
