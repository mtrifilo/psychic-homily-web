import { Suspense } from 'react'
import { ArtistList, ArtistListSkeleton } from '@/features/artists'
import { JsonLd } from '@/components/seo/JsonLd'
import { generateItemListSchema, generateBreadcrumbSchema } from '@/lib/seo/jsonld'
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
 * WHY THE FIRST SCREEN IS NOT SERVER-SEEDED, so the next reader does not read
 * the omission as an oversight.
 *
 * `/shows` and `/venues` server-render their first screen by fetching exactly
 * what their client hook requests and seeding that exact cache key
 * (`HydratedShowList`, `HydratedVenueList`). `/artists` cannot do that yet, and
 * the blocker is in the API, not here: `GET /artists` declares no `limit` —
 * `ListArtistsRequest` carries only the filter parameters — so `useArtists`
 * sends none, and its cache key belongs to a request for the WHOLE catalogue.
 * Seeding it therefore means fetching all of it server-side, which
 *
 *   (a) is 3,233,345 raw bytes (measured 2026-08-08, see
 *       `contracts.ArtistListingEntry`) against a ~1.5 MB raw ceiling, so
 *       `fetchListPayload` would raise `DataCacheBudgetError` and fail the
 *       build outright, and
 *   (b) would put the entire catalogue straight back into the flight payload
 *       that `ARTIST_ITEM_LIST_LIMIT` just removed it from.
 *
 * Seeding only `artistQueryKeys.cities` does not rescue it either: `ArtistList`
 * renders its spinner while EITHER query is loading, so the server would still
 * emit a spinner, having paid for a cache entry to do it.
 *
 * PSY-1774 bounds the browse request; the seed follows it, not the reverse.
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
            <ArtistList />
          </Suspense>
        </main>
      </div>
    </>
  )
}
