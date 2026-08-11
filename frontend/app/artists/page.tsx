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
 * Any search param that moves `useArtists` off the first-screen cache key.
 *
 * `ARTIST_LIST_FIRST_SCREEN_KEY` describes exactly one request: the unfiltered
 * first page. When the URL carries any of these the hook keys on something
 * else, so the seed is fetched, dehydrated, shipped in the flight payload, and
 * never read. Skipping it on those URLs is not an optimization for a rare case
 * — `/artists` gained one `?page=` URL per page of the catalogue in PSY-1774,
 * so the miss is now the common deep link.
 */
const FIRST_SCREEN_DEFEATING_PARAMS = ['page', 'cities', 'tags', 'tag_match'] as const

/**
 * Seed the two cache entries `ArtistList` blocks its first paint on — the first
 * page of artists and the city facet counts — so the artist rows reach the
 * server HTML (PSY-1774, absorbing the seeding half of PSY-1773).
 *
 * BOTH seeds are required, not just the rows: `ArtistList` renders its spinner
 * while EITHER query is loading, so seeding the rows alone would still
 * server-render a spinner, having paid for a cache entry to do it.
 *
 * A failed fetch renders `<ArtistList />` unseeded rather than throwing; the
 * component fetches for itself and owns the error state (see
 * `fetchListPayload`).
 *
 * WHY THIS COULD NOT EXIST BEFORE PSY-1774, because the omission was
 * deliberate and the reasoning is what keeps the shape honest. `GET /artists`
 * declared no `limit` — `ListArtistsRequest` carried only the filter
 * parameters — so `useArtists` sent none and its cache key belonged to a
 * request for the WHOLE catalogue. Seeding it meant fetching all of it
 * server-side, which was 3,233,345 raw bytes (measured 2026-08-08, see
 * `contracts.ArtistListingEntry`) against a ~1.5 MB raw ceiling: `fetchListPayload`
 * raised `DataCacheBudgetError` and failed the build outright, and the
 * catalogue went straight back into the flight payload `ARTIST_ITEM_LIST_LIMIT`
 * had just removed it from. A real `limit` is what fixed BOTH, which a
 * server-side slice could not have.
 */
async function HydratedArtistList({
  searchParams,
}: {
  searchParams: Promise<Record<string, string | string[] | undefined>>
}) {
  const params = await searchParams
  if (FIRST_SCREEN_DEFEATING_PARAMS.some(key => params[key] !== undefined)) {
    return <ArtistList />
  }

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

/**
 * The `ItemList` await stays in the page body, matching `/shows` and `/venues`.
 * Moving it under the `Suspense` boundary was considered and buys nothing:
 * `fetchSeoList` fetches with a `revalidate` hint, so the block is prerenderable
 * and Next bakes it in wherever the await sits. Suspense does not postpone what
 * does not need a request.
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
export default async function ArtistsPage({
  searchParams,
}: {
  searchParams: Promise<Record<string, string | string[] | undefined>>
}) {
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
            <HydratedArtistList searchParams={searchParams} />
          </Suspense>
        </main>
      </div>
    </>
  )
}
