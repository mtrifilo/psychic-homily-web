import { Suspense, cache } from 'react'
import { Metadata } from 'next'
import { notFound } from 'next/navigation'
import * as Sentry from '@sentry/nextjs'
import { Loader2 } from 'lucide-react'
import { HydrationBoundary, dehydrate } from '@tanstack/react-query'
import { ArtistDetail } from '@/features/artists'
import type { Artist } from '@/features/artists/types'
import { JsonLd } from '@/components/seo/JsonLd'
import { generateMusicGroupSchema, generateBreadcrumbSchema } from '@/lib/seo/jsonld'
import { getQueryClient, queryKeys } from '@/lib/queryClient'

const API_BASE_URL =
  process.env.NEXT_PUBLIC_API_URL ||
  (process.env.NODE_ENV === 'development'
    ? 'http://localhost:8080'
    : 'https://api.psychichomily.com')

interface ArtistPageProps {
  params: Promise<{ slug: string }>
}

/**
 * PSY-796: wrap with `React.cache()` so `generateMetadata` and the page
 * body share ONE fetch per request instead of two. The result also seeds
 * the TanStack Query cache via `prefetchQuery` below, eliminating the
 * client-side refetch on first paint.
 */
const getArtist = cache(async (slug: string): Promise<Artist | null> => {
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
})

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
    notFound()
  }

  const artistData = await getArtist(slug)

  if (!artistData) {
    notFound()
  }

  // PSY-796: pilot SSR hydration. Seed a request-scoped QueryClient with the
  // artist payload the server already fetched, then dehydrate so the client
  // `useArtist` hook resolves from the cache instead of refetching. The
  // queryFn just returns the cached value — `cache()` above ensures the
  // network call already happened, so this is a synchronous cache write.
  const queryClient = getQueryClient()
  await queryClient.prefetchQuery({
    queryKey: queryKeys.artists.detail(slug),
    queryFn: () => artistData,
  })

  return (
    <>
      <JsonLd data={generateMusicGroupSchema({
        name: artistData.name,
        slug: artistData.slug || slug,
        city: artistData.city,
        state: artistData.state,
        // `ArtistSocial` is a struct of named optional fields; spread into a
        // plain object so it satisfies the schema helper's index-signature
        // parameter type without changing the cross-feature type.
        social: { ...artistData.social },
      })} />
      <JsonLd data={generateBreadcrumbSchema([
        { name: 'Home', url: 'https://psychichomily.com' },
        { name: 'Artists', url: 'https://psychichomily.com/artists' },
        { name: artistData.name, url: `https://psychichomily.com/artists/${artistData.slug || slug}` },
      ])} />
      <HydrationBoundary state={dehydrate(queryClient)}>
        <Suspense fallback={<ArtistLoadingFallback />}>
          <ArtistDetail artistId={slug} />
        </Suspense>
      </HydrationBoundary>
    </>
  )
}
