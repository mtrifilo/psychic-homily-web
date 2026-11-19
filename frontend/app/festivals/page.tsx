import { Suspense } from 'react'
import { HydrationBoundary } from '@tanstack/react-query'
import { FestivalList } from '@/features/festivals/components'
import {
  FESTIVAL_LIST_FIRST_SCREEN_DEFEATING_PARAMS,
  FESTIVAL_LIST_FIRST_SCREEN_KEY,
  FESTIVAL_LIST_FIRST_SCREEN_URL,
} from '@/features/festivals/listFirstScreen'
import { LoadingSpinner } from '@/components/shared'
import { CachedListFallback } from '@/components/shared/CachedListFallback'
import {
  dehydrateListFirstScreen,
  type RouteSearchParams,
} from '@/lib/ssr/firstScreenSeed'

export const metadata = {
  title: 'Festivals',
  description: 'Browse music festivals, lineups, and schedules.',
  alternates: {
    canonical: 'https://psychichomily.com/festivals',
  },
  openGraph: {
    title: 'Festivals | Psychic Homily',
    description: 'Browse music festivals, lineups, and schedules.',
    url: '/festivals',
    type: 'website',
  },
}

/**
 * `FestivalList` under a boundary seeded with the festivals on a bare
 * `/festivals`, so the rows are in the server HTML.
 *
 * Only the rows are seeded because only the rows gate the first paint; the tag
 * facet fills in after hydration.
 *
 * The element is a `HydrationBoundary` on every path, seeded or not: a
 * different element type here would remount the list on the first navigation
 * off the seeded URL. A failed read seeds nothing, and the list fetches for
 * itself and owns the loading and error states.
 */
export async function HydratedFestivalList({
  searchParams,
}: {
  searchParams: Promise<RouteSearchParams>
}) {
  const dehydratedState = await dehydrateListFirstScreen(
    await searchParams,
    FESTIVAL_LIST_FIRST_SCREEN_DEFEATING_PARAMS,
    [
      {
        url: FESTIVAL_LIST_FIRST_SCREEN_URL,
        queryKey: FESTIVAL_LIST_FIRST_SCREEN_KEY,
        collection: 'festivals',
        service: 'festivals-first-screen',
      },
    ],
  )

  return (
    <HydrationBoundary state={dehydratedState}>
      <FestivalList />
    </HydrationBoundary>
  )
}

export default function FestivalsPage({
  searchParams,
}: {
  searchParams: Promise<RouteSearchParams>
}) {
  return (
    <div className="flex min-h-screen items-start justify-center">
      <main className="w-full max-w-6xl px-4 py-8 md:px-8">
        <h1 className="text-3xl font-bold text-center mb-8">Festivals</h1>
        <Suspense
          fallback={
            <CachedListFallback
              seededKeys={[FESTIVAL_LIST_FIRST_SCREEN_KEY]}
              defeatingParams={FESTIVAL_LIST_FIRST_SCREEN_DEFEATING_PARAMS}
              loader={
                <div className="flex justify-center items-center py-12">
                  <LoadingSpinner />
                </div>
              }
            >
              <FestivalList />
            </CachedListFallback>
          }
        >
          <HydratedFestivalList searchParams={searchParams} />
        </Suspense>
      </main>
    </div>
  )
}
