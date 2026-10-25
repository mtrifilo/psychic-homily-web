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
import { withoutPassedShows } from '@/lib/ssr/withoutPassedShows'
import { JsonLd } from '@/components/seo/JsonLd'
import { generateMusicGroupSchema, generateBreadcrumbSchema } from '@/lib/seo/jsonld'
import { artistSnippet } from '@/lib/seo/entitySnippets'
import { API_BASE_URL } from '@/lib/api-base'
import { reportServerFetchFailure } from '@/lib/ssr/reportServerFetchFailure'
import { serverReadHeaders } from '@/lib/ssr/serverReadHeaders'
import { queryKeys } from '@/lib/queryClient'
import { prefetchEntity, seedFirstScreen } from '@/lib/query-hydration'

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
 * seeds the TanStack Query cache via `prefetchEntity` below, eliminating
 * the client-side refetch on first paint.
 */
const getArtist = cache(async (slug: string): Promise<Artist | null> => {
  try {
    const res = await fetch(`${API_BASE_URL}/artists/${encodeURIComponent(slug)}`, {
      headers: serverReadHeaders(),
      next: { revalidate: 3600 },
    })
    if (res.ok) {
      return res.json()
    }
    reportServerFetchFailure({
      response: res,
      service: 'artist-page',
      pathFamily: '/artists/{slug}',
      notFoundIsAnswer: true,
      context: { slug },
    })
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

  // Started before the artist is awaited so the two reads overlap. It resolves
  // to null rather than rejecting, so leaving it in flight through `notFound()`
  // cannot raise an unhandled rejection. Revalidated every
  // `CURRENT_PERIOD_REVALIDATE` because each row carries cancelled and sold-out
  // state; that window does not bound the age of what a render receives (see
  // `withoutPassedShows` below).
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

  const dehydratedState = await prefetchEntity(
    queryKeys.artists.detail(slug),
    artistData,
  )

  // The upcoming rows travel by cache KEY because `ArtistDetail` renders
  // `ArtistShowsList` with no rows prop: the key is the NUMERIC id that list
  // passes to `useArtistShows`, and `ARTIST_UPCOMING_SHOWS_REQUEST` is the one
  // spelling of the rest of it.
  //
  // Stamped stale (`seedFirstScreen`), unlike the detail above: the rows paint
  // from the server and the browser then re-asks the live endpoint on mount,
  // so a Data Cache entry older than a show mutation is corrected after paint,
  // and a stale stamp never overwrites a fresher entry the client already holds.
  // Rows that have provably left the upcoming window since the read are dropped
  // (`withoutPassedShows`). No seed when the read failed or answered for another
  // artist; the list then fetches for itself behind its loader.
  const upcomingShowsState = await seedFirstScreen(
    upcomingShows?.artist_id === artistData.id
      ? [
          {
            queryKey: queryKeys.artists.showsPage(
              artistData.id,
              ARTIST_UPCOMING_SHOWS_REQUEST,
            ),
            data: withoutPassedShows(upcomingShows),
          },
        ]
      : [],
  )

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
        <HydrationBoundary state={upcomingShowsState}>
          <Suspense fallback={<ArtistLoadingFallback />}>
            <ArtistDetail artistId={slug} />
          </Suspense>
        </HydrationBoundary>
      </HydrationBoundary>
    </>
  )
}
