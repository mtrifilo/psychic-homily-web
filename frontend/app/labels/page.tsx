import { Suspense } from 'react'
import { HydrationBoundary } from '@tanstack/react-query'
import { LabelList } from '@/features/labels/components/LabelList'
import {
  LABEL_LIST_FIRST_SCREEN_DEFEATING_PARAMS,
  LABEL_LIST_FIRST_SCREEN_KEY,
  LABEL_LIST_FIRST_SCREEN_URL,
} from '@/features/labels/listFirstScreen'
import { LoadingSpinner } from '@/components/shared'
import {
  dehydrateListFirstScreen,
  type RouteSearchParams,
} from '@/lib/ssr/firstScreenSeed'

export const metadata = {
  title: 'Labels',
  description: 'Browse record labels and their rosters and catalogs.',
  alternates: {
    canonical: 'https://psychichomily.com/labels',
  },
  openGraph: {
    title: 'Labels | Psychic Homily',
    description: 'Browse record labels and their rosters and catalogs.',
    url: '/labels',
    type: 'website',
  },
}

/**
 * `LabelList` under a boundary seeded with the labels on a bare `/labels`, so
 * the rows are in the server HTML.
 *
 * Only the rows are seeded because only the rows gate the first paint; the tag
 * facet fills in after hydration.
 *
 * The element is a `HydrationBoundary` on every path, seeded or not: a
 * different element type here would remount the list on the first navigation
 * off the seeded URL. A failed read seeds nothing, and the list fetches for
 * itself and owns the loading and error states.
 */
export async function HydratedLabelList({
  searchParams,
}: {
  searchParams: Promise<RouteSearchParams>
}) {
  const dehydratedState = await dehydrateListFirstScreen(
    await searchParams,
    LABEL_LIST_FIRST_SCREEN_DEFEATING_PARAMS,
    [
      {
        url: LABEL_LIST_FIRST_SCREEN_URL,
        queryKey: LABEL_LIST_FIRST_SCREEN_KEY,
        collection: 'labels',
        service: 'labels-first-screen',
      },
    ],
  )

  return (
    <HydrationBoundary state={dehydratedState}>
      <LabelList />
    </HydrationBoundary>
  )
}

export default function LabelsPage({
  searchParams,
}: {
  searchParams: Promise<RouteSearchParams>
}) {
  return (
    <div className="flex min-h-screen items-start justify-center">
      <main className="w-full max-w-6xl px-4 py-8 md:px-8">
        <h1 className="text-3xl font-bold text-center mb-8">Labels</h1>
        <Suspense
          fallback={
            <div className="flex justify-center items-center py-12">
              <LoadingSpinner />
            </div>
          }
        >
          <HydratedLabelList searchParams={searchParams} />
        </Suspense>
      </main>
    </div>
  )
}
