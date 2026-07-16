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
