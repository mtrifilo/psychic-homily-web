import { Suspense, cache } from 'react'
import { Metadata } from 'next'
import * as Sentry from '@sentry/nextjs'
import { Loader2 } from 'lucide-react'
import { dehydrate, HydrationBoundary } from '@tanstack/react-query'
import { ExplorePage } from '@/features/explore'
import type {
  ExploreFeaturedResponse,
  ExploreUpcomingShowsResponse,
} from '@/features/explore'
import { API_BASE_URL } from '@/lib/api-base'
import { queryKeys, getQueryClient } from '@/lib/queryClient'

export const metadata: Metadata = {
  title: 'Explore | Psychic Homily',
  description:
    'Hand-picked bills, knowledge-graph traversal, and a chronological view of upcoming shows.',
  alternates: {
    canonical: 'https://psychichomily.com/explore',
  },
  openGraph: {
    title: 'Explore | Psychic Homily',
    description:
      'Hand-picked bills, knowledge-graph traversal, and a chronological view of upcoming shows.',
    url: '/explore',
    type: 'website',
  },
}

const UPCOMING_LIMIT = 5

// `React.cache()`-wrapped fetches share the request memo so any
// caller within the same render reuses the result. The /explore
// endpoints are public; ISR is safe (no per-user content). Featured
// data changes only when the admin curates, so revalidate every
// 5 minutes; upcoming shows tick by event_date so a similar window
// is fine — the section is small and chronological.
const getFeatured = cache(async (): Promise<ExploreFeaturedResponse | null> => {
  try {
    const res = await fetch(`${API_BASE_URL}/explore/featured`, {
      next: { revalidate: 300 },
    })
    if (res.ok) {
      return res.json()
    }
    if (res.status >= 500) {
      Sentry.captureMessage(`Explore featured fetch error: ${res.status}`, {
        level: 'error',
        tags: { service: 'explore-page' },
      })
    }
  } catch (error) {
    Sentry.captureException(error, {
      level: 'error',
      tags: { service: 'explore-page' },
    })
  }
  return null
})

const getUpcomingShows = cache(
  async (): Promise<ExploreUpcomingShowsResponse | null> => {
    try {
      const res = await fetch(
        `${API_BASE_URL}/explore/upcoming-shows?limit=${UPCOMING_LIMIT}`,
        { next: { revalidate: 300 } },
      )
      if (res.ok) {
        return res.json()
      }
      if (res.status >= 500) {
        Sentry.captureMessage(
          `Explore upcoming-shows fetch error: ${res.status}`,
          {
            level: 'error',
            tags: { service: 'explore-page' },
          },
        )
      }
    } catch (error) {
      Sentry.captureException(error, {
        level: 'error',
        tags: { service: 'explore-page' },
      })
    }
    return null
  },
)

function ExploreLoadingFallback() {
  return (
    <div className="flex min-h-[60vh] items-center justify-center">
      <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
    </div>
  )
}

// Next 16's cacheComponents config requires reading either a Request
// data source (cookies/headers/searchParams) or making an uncached
// fetch BEFORE any internal Date.now() call (TanStack Query's
// prefetch helpers call it). Awaiting `searchParams` here is a
// no-cost way to opt this route into request-time rendering without
// surrendering ISR on the upstream fetches.
interface ExploreRouteProps {
  searchParams: Promise<Record<string, string | string[] | undefined>>
}

export default async function ExploreRoute({ searchParams }: ExploreRouteProps) {
  await searchParams

  const [featured, upcoming] = await Promise.all([
    getFeatured(),
    getUpcomingShows(),
  ])

  // Seed both queries onto ONE shared client so we can hand a single
  // dehydrated state to the hydration boundary. Missing fetches (a
  // 500 surfaces as null upstream) simply skip their seed; the client
  // hook falls back to its own fetch on mount in that path.
  const queryClient = getQueryClient()
  if (featured !== null) {
    await queryClient.prefetchQuery({
      queryKey: queryKeys.explore.featured,
      queryFn: () => featured,
    })
  }
  if (upcoming !== null) {
    await queryClient.prefetchQuery({
      queryKey: queryKeys.explore.upcomingShows({ limit: UPCOMING_LIMIT }),
      queryFn: () => upcoming,
    })
  }
  const dehydratedState = dehydrate(queryClient)

  return (
    <HydrationBoundary state={dehydratedState}>
      <Suspense fallback={<ExploreLoadingFallback />}>
        <ExplorePage />
      </Suspense>
    </HydrationBoundary>
  )
}
