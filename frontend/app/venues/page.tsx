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
import { getVenuesForMetadata } from './venuesMetadata'

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
 * Data Cache exposure of the two fetches inside the Suspense boundary below,
 * measured against production on 2026-08-09. The cap is 2 MB per item, applied
 * to a base64 envelope; see `lib/data-cache-budget/budget.ts`.
 *
 *   GET /venues?limit=50   35,226 raw   46,968 base64   2.2% of the cap
 *   GET /venues/cities      5,668 raw    7,560 base64   0.4%
 *
 * Neither is exposed and neither grows with the catalogue: the first is bounded
 * by its `limit`, and the second is a facet aggregate of one row per city. The
 * third fetch, the unbounded one that does grow, is measured beside itself in
 * `venuesMetadata.ts`. `fetchListPayload` weighs these two against the budget on
 * the way through, so a breach fails a build rather than going quiet.
 *
 * The ItemList itself is NOT measured here, and it is the part that grows with
 * the catalogue: see `contracts.VenueListingEntry`, which records what it weighs
 * in the rendered document and why that, rather than the cache budget, is the
 * constraint that binds first. The slug guard that used to sit in this file now
 * lives in `venuesMetadata.ts`, beside the fetch whose contract it checks.
 */
export default async function VenuesPage() {
  const venues = await getVenuesForMetadata()

  return (
    <>
      {venues.length > 0 && (
        <JsonLd data={generateItemListSchema({
          name: 'Venues',
          description: 'Music venues in Phoenix and beyond.',
          listItems: venues.map(venue => ({
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
