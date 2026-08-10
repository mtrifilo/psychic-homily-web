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
import { generateItemListSchema, generateBreadcrumbSchema } from '@/lib/seo/jsonld'
import { seedFirstScreen } from '@/lib/query-hydration'
import { fetchListPayload } from '@/lib/ssr/fetchListPayload'
import { getVenuesForMetadata, type VenueListItem } from './venuesMetadata'

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

/**
 * Data Cache exposure of everything this route fetches, measured against
 * production on 2026-08-09 at 297 verified venues. The cap is 2 MB per item and
 * the body is stored base64-encoded; see `lib/data-cache-budget/budget.ts`.
 *
 *   GET /venues/listing (all 297)   18,289 raw    24,385 base64     1.2% of the cap
 *   GET /venues?limit=50            35,226 raw    46,968 base64     2.2%
 *   GET /venues/cities               5,668 raw     7,560 base64     0.4%
 *
 * None is exposed, and only the middle one is bounded by a `limit` — the other
 * two grow with the catalogue and with the number of cities respectively. The
 * listing's growth is the one that matters and is measured beside the fetch that
 * owns it, in `venuesMetadata.ts`. Both `fetchSeoList` and `fetchListPayload`
 * weigh their response against the budget on the way through, so a breach fails
 * a build rather than going quiet.
 */
export default async function VenuesPage() {
  const venues = await getVenuesForMetadata()

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
