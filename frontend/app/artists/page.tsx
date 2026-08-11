import { Suspense } from 'react'
import { HydrationBoundary, type DehydratedState } from '@tanstack/react-query'
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
 * Search params that can move `useArtists` off the first-screen cache key.
 *
 * `ARTIST_LIST_FIRST_SCREEN_KEY` describes exactly one request: the unfiltered
 * first page. When the URL keys the hook somewhere else, the seed is fetched,
 * dehydrated, shipped in the flight payload, and never read. Skipping it there
 * is not an optimization for a rare case — PSY-1774 gave `/artists` one
 * `?page=` URL per 50 artists, so the miss is now the common deep link.
 *
 * TWO THINGS THIS LIST IS NOT.
 *
 * It is not exact. It tests for a non-empty value, not for a value that
 * actually moves the key, so `?cities=all` (the tolerated ALL_CITIES sentinel)
 * and `?page=1` still skip a seed they would have hit. Both directions of
 * inexactness are safe in only one direction, and this is the safe one: a
 * false positive costs a server-rendered spinner — the behaviour `/artists`
 * had before this change — while a false NEGATIVE would seed a key the hook
 * does not ask for, which is silent and wasteful. Never wrong, sometimes
 * merely unhelpful.
 *
 * It is not self-maintaining. The mapping from URL to cache key lives in
 * `ArtistList` and `useArtists`, so a filter added there and not here silently
 * starts paying for an unread seed. Deriving this by comparing
 * `hashKey(artistQueryKeys.list(...))` against the first-screen key would be
 * exact and drift-proof; it needs the params→options mapping lifted out of
 * `ArtistList` first, which is a change of its own.
 */
const FIRST_SCREEN_DEFEATING_PARAMS = ['page', 'cities', 'tags', 'tag_match'] as const

/**
 * Whether a URL's params keep it on the seeded first screen.
 *
 * Empty values (`?tags=`, `?page=`) read as absent, because that is how the
 * client parses them — `parseTagsParam('')` is `[]` and an unparseable page is
 * page 1 — so treating them as present would skip a seed that would have hit.
 */
function isFirstScreenRequest(
  params: Record<string, string | string[] | undefined>
): boolean {
  return !FIRST_SCREEN_DEFEATING_PARAMS.some(key => {
    const value = params[key]
    // An array means the param was repeated; the client reads the first one,
    // and a repeated filter param is never the bare first screen either way.
    if (Array.isArray(value)) return value.some(v => v !== '')
    return value !== undefined && value !== ''
  })
}

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
export async function HydratedArtistList({
  searchParams,
}: {
  searchParams: Promise<Record<string, string | string[] | undefined>>
}) {
  const dehydratedState = await dehydratedFirstScreen(await searchParams)

  // ALWAYS the same element type at this position, seeded or not. Returning a
  // bare <ArtistList /> on the skip paths changes the child's type, which makes
  // React unmount and remount the whole list on the first page click off page 1
  // — replacing the rows with the centred spinner (`isLoading && !data` is true
  // on a fresh observer, and keepPreviousData yields nothing on a first mount)
  // and dropping keyboard focus to <body>. An empty boundary hydrates nothing
  // and costs nothing.
  return (
    <HydrationBoundary state={dehydratedState}>
      <ArtistList />
    </HydrationBoundary>
  )
}

/**
 * The seeds for this request, or an empty state when there are none to give.
 *
 * Split out so the caller's returned element type never varies — see the note
 * at its return. `seedFirstScreen` is only reached on the path that has
 * something to seed, so the skip paths do not pay for its `connection()`.
 */
async function dehydratedFirstScreen(
  params: Record<string, string | string[] | undefined>
): Promise<DehydratedState> {
  if (!isFirstScreenRequest(params)) {
    return EMPTY_DEHYDRATED_STATE
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
    return EMPTY_DEHYDRATED_STATE
  }

  return seedFirstScreen([
    { queryKey: ARTIST_LIST_FIRST_SCREEN_KEY, data: artists },
    { queryKey: artistQueryKeys.cities, data: cities },
  ])
}

/** What `dehydrate()` produces for an empty client: hydrates nothing. */
const EMPTY_DEHYDRATED_STATE: DehydratedState = { mutations: [], queries: [] }

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
/**
 * The `ItemList` block, emitted only on the URL it actually describes.
 *
 * Its own async component under Suspense because reading `searchParams` in the
 * page body would make the whole route dynamic; here only this block resumes
 * per-request, and a crawler receives it either way (shell plus resume — see
 * the PPR note below).
 */
export async function FirstScreenItemList({
  searchParams,
  artists,
}: {
  searchParams: Promise<Record<string, string | string[] | undefined>>
  artists: Array<ArtistListItem & { slug: string }>
}) {
  if (!isFirstScreenRequest(await searchParams)) return null

  return (
    <JsonLd
      data={generateItemListSchema({
        name: 'Artists',
        description: 'Artists performing live music in Phoenix and beyond.',
        listItems: artists.map(artist => ({
          url: `https://psychichomily.com/artists/${artist.slug}`,
          name: artist.name,
        })),
      })}
    />
  )
}

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
        /*
          Gated on the first screen because `getArtistsForMetadata` is
          page-INDEPENDENT: it always describes the 100 most active artists, so
          on `?page=7` the block advertised artists 1-100 while the page showed
          301-350. Structured data that contradicts the document is worse than
          none, and PSY-1774 minted ~124 such URLs where one existed before.
          What the bound should BE remains PSY-1794; this only stops the block
          appearing on pages it does not describe.
        */
        <Suspense fallback={null}>
          <FirstScreenItemList
            searchParams={searchParams}
            artists={artistsWithSlugs}
          />
        </Suspense>
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
