import { Suspense } from 'react'
import { HydrationBoundary } from '@tanstack/react-query'
import { ReleaseList } from '@/features/releases/components'
import {
  RELEASE_LIST_FIRST_SCREEN_DEFEATING_PARAMS,
  RELEASE_LIST_FIRST_SCREEN_KEY,
  RELEASE_LIST_FIRST_SCREEN_URL,
} from '@/features/releases/listFirstScreen'
import type { ReleasesListResponse } from '@/features/releases/types'
import { LoadingSpinner } from '@/components/shared'
import { listRootCanonical } from '@/lib/seo/siteMetadata'
import {
  dehydrateListFirstScreen,
  type RouteSearchParams,
} from '@/lib/ssr/firstScreenSeed'
import { fetchListPayload } from '@/lib/ssr/fetchListPayload'

/**
 * The canonical is fixed at `/releases` for every query variant of this page.
 * `?page=`, `?type=`, `?sort=`, `?tags=`, `?year=`, `?label_id=` and `?search=`
 * all slice one list, and none of them mints a second document. PSY-1767
 * reviewed this and kept it; the reasoning is on `listRootCanonical`.
 */
export const metadata = {
  title: 'Releases',
  description: 'Browse music releases - albums, EPs, singles, and more.',
  alternates: {
    canonical: listRootCanonical('/releases'),
  },
  openGraph: {
    title: 'Releases | Psychic Homily',
    description: 'Browse music releases - albums, EPs, singles, and more.',
    url: '/releases',
    type: 'website',
  },
}

/**
 * `ReleaseList` under a boundary seeded with the first page of releases on a
 * bare `/releases`, so the rows are in the server HTML.
 *
 * Only the rows are seeded because only the rows gate the first paint; the
 * label dropdown and the tag facet fill in after hydration.
 *
 * The element is a `HydrationBoundary` on every path, seeded or not: a
 * different element type here would remount the list on the first navigation
 * off the seeded URL. A failed read seeds nothing, and the list fetches for
 * itself and owns the loading and error states.
 */
export async function HydratedReleaseList({
  searchParams,
}: {
  searchParams: Promise<RouteSearchParams>
}) {
  const dehydratedState = await dehydrateListFirstScreen(
    await searchParams,
    RELEASE_LIST_FIRST_SCREEN_DEFEATING_PARAMS,
    [
      {
        queryKey: RELEASE_LIST_FIRST_SCREEN_KEY,
        read: () =>
          fetchListPayload<ReleasesListResponse>({
            url: RELEASE_LIST_FIRST_SCREEN_URL,
            collection: 'releases',
            service: 'releases-first-screen',
          }),
      },
    ],
  )

  return (
    <HydrationBoundary state={dehydratedState}>
      <ReleaseList />
    </HydrationBoundary>
  )
}

export default function ReleasesPage({
  searchParams,
}: {
  searchParams: Promise<RouteSearchParams>
}) {
  return (
    <div className="flex min-h-screen items-start justify-center">
      <main className="w-full max-w-6xl px-4 py-8 md:px-8">
        <h1 className="text-3xl font-bold text-center mb-8">Releases</h1>
        <Suspense
          fallback={
            <div className="flex justify-center items-center py-12">
              <LoadingSpinner />
            </div>
          }
        >
          <HydratedReleaseList searchParams={searchParams} />
        </Suspense>
      </main>
    </div>
  )
}
