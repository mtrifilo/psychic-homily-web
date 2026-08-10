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
 * The `ItemList` await stays in the page body, above the `Suspense` boundary,
 * exactly as it does on `/shows` and `/venues`.
 *
 * It was worth checking whether moving it below would shrink the RSC flight
 * payload, since that payload is what `<Link>` prefetch leaks onto `/`,
 * `/atlas` and `/shows`. It would not: the flight carries a subtree's rendered
 * output whether that subtree streamed or was in the shell, so relocating the
 * await changes when the bytes arrive and not how many there are. What made
 * this route's flight enormous was the SIZE of the block, and that is bounded
 * at `ARTIST_ITEM_LIST_LIMIT`. Keeping the await here also keeps the JSON-LD in
 * the prerendered shell, which is where a crawler should find it.
 *
 * WHAT IS STILL MISSING, so the next reader does not conclude it was
 * considered and rejected: `ArtistList` is the last list page whose rows a
 * human reads are client-only. `/shows` and `/venues` server-render their first
 * screen by fetching what their client hook requests and seeding that exact
 * cache key (`HydratedShowList`, `HydratedVenueList`). The same cannot be done
 * here yet. `useArtists` sends no `limit`, because `GET /artists` has none to
 * send — `ListArtistsRequest` declares only the filter parameters — so seeding
 * its key would mean fetching the whole 3.17 MB catalogue server-side, which
 * (a) is over the Data Cache raw budget and would fail the build gate in
 * lib/data-cache-budget, and (b) would put the entire catalogue back into the
 * flight payload this change just removed it from. Bounding the browse request
 * is PSY-1774; the seed follows it, not the other way around.
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
