import { Suspense } from 'react'
import { Metadata } from 'next'
import * as Sentry from '@sentry/nextjs'
import { Loader2 } from 'lucide-react'
import { ArtistDetail } from '@/components/artists'
import { JsonLd } from '@/components/seo/JsonLd'
import { generateMusicGroupSchema } from '@/lib/seo/jsonld'

const API_BASE_URL =
  process.env.NEXT_PUBLIC_API_URL ||
  (process.env.NODE_ENV === 'development'
    ? 'http://localhost:8080'
    : 'https://api.psychichomily.com')

interface ArtistPageProps {
  params: Promise<{ slug: string }>
}

interface ArtistData {
  name: string
  slug?: string
  city?: string | null
  state?: string | null
  social?: Record<string, string | null>
}

async function getArtist(slug: string): Promise<ArtistData | null> {
  try {
    const res = await fetch(`${API_BASE_URL}/artists/${slug}`, {
      next: { revalidate: 3600 },
    })
    if (res.ok) {
      return res.json()
    }
    if (res.status >= 500) {
      Sentry.captureMessage(`Artist page fetch error: ${res.status}`, {
        level: 'error',
        tags: { service: 'artist-page' },
        extra: { slug, status: res.status },
      })
    }
  } catch (error) {
    Sentry.captureException(error, {
      level: 'error',
      tags: { service: 'artist-page' },
      extra: { slug },
    })
  }
  return null
}

export async function generateMetadata({ params }: ArtistPageProps): Promise<Metadata> {
  const { slug } = await params
  const artist = await getArtist(slug)

  if (artist) {
    return {
      title: artist.name,
      description: `${artist.name} - upcoming shows and artist details on Psychic Homily`,
      alternates: {
        canonical: `https://psychichomily.com/artists/${slug}`,
      },
      openGraph: {
        title: artist.name,
        description: `View upcoming shows featuring ${artist.name}`,
        type: 'website',
        url: `/artists/${slug}`,
      },
    }
  }

  return {
    title: 'Artist',
    description: 'View artist details and upcoming shows',
  }
}

function ArtistLoadingFallback() {
  return (
    <div className="flex min-h-[60vh] items-center justify-center">
      <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
    </div>
  )
}

export default async function ArtistPage({ params }: ArtistPageProps) {
  const { slug } = await params

  if (!slug) {
    return (
      <div className="flex min-h-[60vh] items-center justify-center">
        <div className="text-center">
          <h1 className="text-2xl font-bold mb-2">Invalid Artist</h1>
          <p className="text-muted-foreground">
            The artist could not be found.
          </p>
        </div>
      </div>
    )
  }

  const artistData = await getArtist(slug)

  return (
    <>
      {artistData && (
        <JsonLd data={generateMusicGroupSchema({
          name: artistData.name,
          slug: artistData.slug || slug,
          city: artistData.city,
          state: artistData.state,
          social: artistData.social,
        })} />
      )}
      <Suspense fallback={<ArtistLoadingFallback />}>
        <ArtistDetail artistId={slug} />
      </Suspense>
    </>
  )
}
