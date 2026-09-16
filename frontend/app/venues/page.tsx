import { Suspense } from 'react'
import type { Metadata } from 'next'
import { HydrationBoundary } from '@tanstack/react-query'
import { VenueList } from '@/features/venues'
import { venueEndpoints, venueQueryKeys } from '@/features/venues/api'
import type { VenueCitiesResponse } from '@/features/venues/types'
import { JsonLd } from '@/components/seo/JsonLd'
import { generateItemListSchema, generateBreadcrumbSchema } from '@/lib/seo/jsonld'
import { seedFirstScreen } from '@/lib/query-hydration'
import { fetchListPayload } from '@/lib/ssr/fetchListPayload'
import { getVenuesForMetadata } from './venuesMetadata'
import {
  buildVenuesMetadata,
  resolveVenuesPage,
  resolveVenuesScope,
  venuesUrlCities,
} from './venuesPageMetadata'

/** The city facet, for naming a city. Null when it could not be read. */
async function fetchCityFacet(): Promise<VenueCitiesResponse | null> {
  return fetchListPayload<VenueCitiesResponse>({
    url: venueEndpoints.CITIES,
    collection: 'cities',
    service: 'venue-cities-first-screen',
  })
}

/**
 * The directory's title, description, canonical and robots, per city.
 *
 * READS `searchParams`, which makes this route's metadata dynamic: under
 * `cacheComponents` the `<title>`, the canonical and the robots directive are
 * streamed into the BODY of the document rather than the `<head>`, and React
 * hoists them into the head as it renders. The e2e spec asserts them with
 * `head >` selectors for exactly that reason. The page's own shell keeps its
 * prerender because nothing below reads `searchParams`.
 *
 * The facet is fetched ONLY when the URL names exactly one city. When it is
 * fetched it is the same url, method and headers the body's seed fetch uses,
 * which is the whole Data Cache key, so the two read one entry.
 */
export async function generateMetadata({
  searchParams,
}: {
  searchParams: Promise<Record<string, string | string[] | undefined>>
}): Promise<Metadata> {
  const params = await searchParams
  const selected = venuesUrlCities(params)
  const facet = selected.length === 1 ? await fetchCityFacet() : null

  return buildVenuesMetadata(
    resolveVenuesScope(selected, facet?.cities ?? null),
    resolveVenuesPage(params)
  )
}

function VenueListLoading() {
  return (
    <div
      role="status"
      aria-label="Loading venues"
      className="flex justify-center items-center py-12"
    >
      <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-foreground"></div>
    </div>
  )
}

/**
 * Seed the cache entry `VenueList` blocks its first paint on: the city facet
 * counts.
 *
 * The ROWS are not seeded, and cannot be. The directory lists ONE CITY's rooms,
 * and which city that is resolves in the browser (the viewer's favourites, else
 * the IP-geo read) on a route that stays ISR, so the server has no scoped list
 * to fetch. An unscoped first page would answer only an explicit `?cities=all`.
 *
 * The facet entry seeded here is the UNSCOPED one. The counts are scoped to the
 * list's tag filter, so a `?tags=` deep link asks for a different entry, misses
 * this one, and renders its loading state on both passes. Seeding the filtered
 * entry would mean reading `searchParams` in this body, which costs the route
 * its prerendered shell.
 *
 * A failed fetch renders `<VenueList />` unseeded rather than throwing; the
 * component fetches for itself and owns the error state (see
 * `fetchListPayload`).
 */
async function HydratedVenueList() {
  const cities = await fetchCityFacet()

  if (!cities) {
    return <VenueList />
  }

  const dehydratedState = await seedFirstScreen([
    { queryKey: venueQueryKeys.cities, data: cities },
  ])

  return (
    <HydrationBoundary state={dehydratedState}>
      <VenueList />
    </HydrationBoundary>
  )
}

/**
 * Data Cache exposure of the fetch inside the Suspense boundary below, measured
 * against production on 2026-08-09. The cap is 2 MB per item, applied to a
 * base64 envelope; see `lib/data-cache-budget/budget.ts`.
 *
 *   GET /venues/cities      5,668 raw    7,560 base64   0.4% of the cap
 *
 * It is not exposed and does not grow with the catalogue: it is a facet
 * aggregate of one row per city. The other fetch, the unbounded one that does
 * grow, is measured beside itself in `venuesMetadata.ts`. `fetchListPayload`
 * weighs this one against the budget on the way through, so a breach fails a
 * build rather than going quiet.
 *
 * The ItemList itself is NOT measured here, and it is the part that grows with
 * the catalogue: see `contracts.VenueListingEntry`, which records what it weighs
 * in the rendered document and why that, rather than the cache budget, is the
 * constraint that binds first.
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
        {/* The `<h1>` belongs to `VenueList`: it names the city, and which city
            that is resolves in the browser. */}
        <main className="w-full max-w-6xl px-4 py-8 md:px-8">
          <Suspense fallback={<VenueListLoading />}>
            <HydratedVenueList />
          </Suspense>
        </main>
      </div>
    </>
  )
}
