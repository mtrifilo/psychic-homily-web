import { Suspense, cache } from 'react'
import { Metadata } from 'next'
import dynamic from 'next/dynamic'
import { notFound } from 'next/navigation'
import * as Sentry from '@sentry/nextjs'
import { Loader2 } from 'lucide-react'
import { HydrationBoundary } from '@tanstack/react-query'
import type { Artist, ArtistShowsResponse } from '@/features/artists/types'
import {
  ARTIST_UPCOMING_SHOWS_REQUEST,
  artistUpcomingShowsUrl,
} from '@/features/artists/api'
import { CURRENT_PERIOD_REVALIDATE } from '@/features/scenes/scenePeriodApi'
import { fetchListPayload } from '@/lib/ssr/fetchListPayload'
import { JsonLd } from '@/components/seo/JsonLd'
import { generateMusicGroupSchema, generateBreadcrumbSchema } from '@/lib/seo/jsonld'
import { artistSnippet } from '@/lib/seo/entitySnippets'
import { API_BASE_URL } from '@/lib/api-base'
import { queryKeys } from '@/lib/queryClient'
import { prefetchEntities } from '@/lib/query-hydration'

// Dynamic-imported from the component FILE (never the `@/features/artists` barrel)
// to evict ArtistDetail.tsx (~40 KB raw) from Turbopack's global shared client
// chunk that loads on every route incl. /explore. PSY-950 (rollout) / PSY-944
// (spike): `dynamic()` alone is a no-op — a barrel re-export keeps the module
// reachable from 2+ routes so Turbopack re-hoists it. The eviction only holds
// when this is paired with REMOVING ArtistDetail from features/artists/index.ts
// + features/artists/components/index.ts. Do NOT re-add the barrel export or this
// silently un-does the chunk split. ssr:true preserves the PSY-796/841
// prefetchEntity + HydrationBoundary SSR (verified byte-equivalent in the spike).
const ArtistDetail = dynamic(
  () =>
    import('@/features/artists/components/ArtistDetail').then((m) => ({
      default: m.ArtistDetail,
    })),
  { ssr: true },
)

interface ArtistPageProps {
  params: Promise<{ slug: string }>
}

/**
 * Wrapped with `React.cache()` so `generateMetadata` and the page body
 * share ONE backend fetch per request instead of two. The result also
 * seeds the TanStack Query cache via `prefetchEntities` below, eliminating
 * the client-side refetch on first paint.
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
    const { title, description } = artistSnippet({
      name: artist.name,
      city: artist.city,
      state: artist.state,
    })
    return {
      title,
      description,
      alternates: {
        canonical: `https://psychichomily.com/artists/${slug}`,
      },
      openGraph: {
        title,
        description,
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

  // Started BEFORE the artist is awaited, so the two backend reads overlap on a
  // Data Cache miss. It resolves to null rather than rejecting, so leaving it
  // in flight through the `notFound()` below cannot raise an unhandled
  // rejection.
  //
  // Through `fetchListPayload`: a request budget, so a slow backend cannot
  // hold the page open, and a shape guard, so a 200 without a `shows` array is
  // never seeded as an empty calendar. The short cache window is the one the
  // venue rail uses for the same kind of rows: each carries cancelled and
  // sold-out state, and a show leaves the upcoming window as its night passes.
  const upcomingShowsPromise = fetchListPayload<ArtistShowsResponse>({
    url: artistUpcomingShowsUrl(slug),
    collection: 'shows',
    service: 'artist-page-upcoming-shows',
    revalidateSeconds: CURRENT_PERIOD_REVALIDATE,
  })
  const artistData = await getArtist(slug)

  if (!artistData) {
    notFound()
  }

  const upcomingShows = await upcomingShowsPromise

  // Both keys through ONE client, because a `HydrationBoundary` carries one
  // dehydrated state. The shows key is the NUMERIC id, which is what
  // `ArtistShowsList` passes to its hook; the detail key is the slug the route
  // was addressed by, which is what `ArtistDetail` passes. A null seed is
  // skipped, and the list then fetches for itself behind its loader.
  //
  // The rows are seeded only when they belong to the artist this page loaded.
  // Both reads resolve the same slug, so a mismatch means one of them answered
  // for a different artist, and seeding it would paint that artist's calendar
  // here for a whole staleTime.
  const dehydratedState = await prefetchEntities([
    { queryKey: queryKeys.artists.detail(slug), data: artistData },
    {
      queryKey: queryKeys.artists.showsPage(
        artistData.id,
        ARTIST_UPCOMING_SHOWS_REQUEST,
      ),
      data: upcomingShows?.artist_id === artistData.id ? upcomingShows : null,
    },
  ])

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
      <HydrationBoundary state={dehydratedState}>
        <Suspense fallback={<ArtistLoadingFallback />}>
          <ArtistDetail artistId={slug} />
        </Suspense>
      </HydrationBoundary>
    </>
  )
}
