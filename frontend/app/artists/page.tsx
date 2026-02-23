import { Suspense } from 'react'
import * as Sentry from '@sentry/nextjs'
import { ArtistList, ArtistListSkeleton } from '@/components/artists'
import { JsonLd } from '@/components/seo/JsonLd'
import { generateItemListSchema, generateBreadcrumbSchema } from '@/lib/seo/jsonld'

const API_BASE_URL =
  process.env.NEXT_PUBLIC_API_URL ||
  (process.env.NODE_ENV === 'development'
    ? 'http://localhost:8080'
    : 'https://api.psychichomily.com')

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

interface ArtistListItem {
  slug: string
  name: string
}

interface ArtistsApiResponse {
  artists: ArtistListItem[]
}

async function getArtists(): Promise<ArtistListItem[]> {
  try {
    const res = await fetch(`${API_BASE_URL}/artists`, {
      next: { revalidate: 3600 },
    })
    if (res.ok) {
      const data: ArtistsApiResponse = await res.json()
      return data.artists ?? []
    }
    if (res.status >= 500) {
      Sentry.captureMessage(`Artists listing: API returned ${res.status}`, {
        level: 'error',
        tags: { service: 'artists-listing' },
        extra: { status: res.status },
      })
    }
  } catch (error) {
    Sentry.captureException(error, {
      level: 'error',
      tags: { service: 'artists-listing' },
    })
  }
  return []
}

export default async function ArtistsPage() {
  const artists = await getArtists()

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
        <main className="w-full max-w-4xl px-4 py-8 md:px-8">
          <h1 className="text-3xl font-bold text-center mb-8">Artists</h1>
          <Suspense fallback={<ArtistListSkeleton />}>
            <ArtistList />
          </Suspense>
        </main>
      </div>
    </>
  )
}
