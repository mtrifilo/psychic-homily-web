import { Suspense } from 'react'
import { HydrationBoundary } from '@tanstack/react-query'
import { ArtistList, ArtistListSkeleton } from '@/features/artists'
import {
  ARTIST_LIST_FIRST_SCREEN_KEY,
  ARTIST_LIST_FIRST_SCREEN_URL,
  artistEndpoints,
  artistQueryKeys,
} from '@/features/artists/api'
import type {
  ArtistCitiesResponse,
  ArtistsListResponse,
} from '@/features/artists/types'
import { JsonLd } from '@/components/seo/JsonLd'
import { generateItemListSchema, generateBreadcrumbSchema } from '@/lib/seo/jsonld'
import { seedFirstScreen } from '@/lib/query-hydration'
import { fetchListPayload } from '@/lib/ssr/fetchListPayload'
import { getArtistsForMetadata, type ArtistListItem } from './artistsMetadata'

export const metadata = {
  title: 'Artists',
  description: 'Browse artists and discover live music in your city.',
  alternates: {
    canonical: 'https://psychichomily.com/artists',
  },
  openGraph: {
    title: 'Artists | Psychic Homily',
    description: 'Browse artists and discover live music in your city.',
    url: '/artists',
    type: 'website',
  },
}

/**
 * HOW THE FIRST SCREEN CAME TO BE SERVER-SEEDED, since it was deliberately not
 * seeded one change ago and the reason is worth keeping.
 *
 * `/shows` and `/venues` server-render their first screen by fetching exactly
 * what their client hook requests and seeding that exact cache key
 * (`HydratedShowList`, `HydratedVenueList`). `/artists` could not, and the
 * blocker was in the API, not here: `GET /artists` declared no `limit` —
 * `ListArtistsRequest` carried only the filter parameters — so `useArtists`
 * sent none, and its cache key belonged to a request for the WHOLE catalogue.
 * Seeding it therefore meant fetching all of it server-side, which
 *
 *   (a) was 3,233,345 raw bytes (measured 2026-08-08, see
 *       `contracts.ArtistListingEntry`) against a ~1.5 MB raw ceiling, so
 *       `fetchListPayload` would raise `DataCacheBudgetError` and fail the
 *       build outright, and
 *   (b) would put the entire catalogue straight back into the flight payload
 *       that `ARTIST_ITEM_LIST_LIMIT` had just removed it from.
 *
 * PSY-1774 gave the endpoint a real `limit`, so `HydratedArtistList` below now
 * fetches ONE PAGE — `ARTIST_LIST_FIRST_SCREEN_URL`, 50 artists — and both
 * costs go with it: the Data Cache entry and the flight payload are sized by
 * the page, not by the catalogue, which is the part a server-side slice could
 * never have fixed.
 *
 * BOTH seeds are required, not just the rows. `ArtistList` renders its spinner
 * while EITHER query is loading, so seeding the rows alone would still
 * server-render a spinner, having paid for a cache entry to do it.
 *
 * The `ItemList` await below stays in the page body, matching `/shows` and
 * `/venues`. Moving it under the `Suspense` boundary was considered and buys
 * nothing: `fetchSeoList` fetches with a `revalidate` hint, so the block is
 * prerenderable and Next bakes it in wherever the await sits. Suspense does not
 * postpone what does not need a request. Only an explicit `connection()` would
 * push it into the dynamic resume, and there is no reason to now that it is
 * bounded.
 *
 * Where the block actually LIVES is worth knowing before reasoning about it,
 * because it is not where the phrase "prerendered shell" suggests. Under PPR
 * the fallback shell (`.next/server/app/artists.html`) contains NO `ItemList`;
 * the block is in the per-segment prefetch payload
 * (`artists.segments/artists/__PAGE__.segment.rsc`), which is also what `<Link>`
 * prefetch downloads. `/venues` and `/shows` behave identically, so this is PPR,
 * not anything this route does. A crawler still receives the block, because the
 * composed response it gets is shell plus resume — verify against `curl`, never
 * against the `.html` artifact, which is the trap here.
 */
/**
 * Seed the two cache entries `ArtistList` blocks its first paint on — the first
 * page of artists and the city facet counts — so the artist rows reach the
 * server HTML (PSY-1774, absorbing the seeding half of PSY-1773).
 *
 * A failed fetch renders `<ArtistList />` unseeded rather than throwing; the
 * component fetches for itself and owns the error state (see
 * `fetchListPayload`).
 */
async function HydratedArtistList() {
  const [artists, cities] = await Promise.all([
    fetchListPayload<ArtistsListResponse>({
      url: ARTIST_LIST_FIRST_SCREEN_URL,
      collection: 'artists',
      service: 'artists-first-screen',
    }),
    fetchListPayload<ArtistCitiesResponse>({
      url: artistEndpoints.CITIES,
      collection: 'cities',
      service: 'artist-cities-first-screen',
    }),
  ])

  if (!artists || !cities) {
    return <ArtistList />
  }

  const dehydratedState = await seedFirstScreen([
    { queryKey: ARTIST_LIST_FIRST_SCREEN_KEY, data: artists },
    { queryKey: artistQueryKeys.cities, data: cities },
  ])

  return (
    <HydrationBoundary state={dehydratedState}>
      <ArtistList />
    </HydrationBoundary>
  )
}

export default async function ArtistsPage() {
  const artists = await getArtistsForMetadata()

  const artistsWithSlugs = artists.filter(
    (a): a is ArtistListItem & { slug: string } => !!a.slug
  )

  return (
    <>
      {artistsWithSlugs.length > 0 && (
        <JsonLd data={generateItemListSchema({
          name: 'Artists',
          description: 'Artists performing live music in Phoenix and beyond.',
          listItems: artistsWithSlugs.map(artist => ({
            url: `https://psychichomily.com/artists/${artist.slug}`,
            name: artist.name,
          })),
        })} />
      )}
      <JsonLd data={generateBreadcrumbSchema([
        { name: 'Home', url: 'https://psychichomily.com' },
        { name: 'Artists', url: 'https://psychichomily.com/artists' },
      ])} />
      <div className="flex min-h-screen items-start justify-center">
        <main className="w-full max-w-6xl px-4 py-8 md:px-8">
          <h1 className="text-3xl font-bold text-center mb-8">Artists</h1>
          <Suspense fallback={<ArtistListSkeleton />}>
            <HydratedArtistList />
          </Suspense>
        </main>
      </div>
    </>
  )
}
