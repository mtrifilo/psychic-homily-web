import { Suspense } from 'react'
import { HydrationBoundary } from '@tanstack/react-query'
import { TagBrowse } from '@/features/tags/components'
import {
  TAG_BROWSE_FIRST_SCREEN_DEFEATING_PARAMS,
  TAG_BROWSE_FIRST_SCREEN_READS,
} from '@/features/tags/browseFirstScreen'
import type { TagListResponse } from '@/features/tags/types'
import { LoadingSpinner } from '@/components/shared'
import {
  dehydrateListFirstScreen,
  type RouteSearchParams,
} from '@/lib/ssr/firstScreenSeed'
import { fetchListPayload } from '@/lib/ssr/fetchListPayload'

export const metadata = {
  title: 'Tags',
  description: 'Browse tags by category — genres, moods, eras, styles, and more.',
  alternates: {
    canonical: 'https://psychichomily.com/tags',
  },
  openGraph: {
    title: 'Tags | Psychic Homily',
    description: 'Browse tags by category — genres, moods, eras, styles, and more.',
    url: '/tags',
    type: 'website',
  },
}

/**
 * `TagBrowse` under a boundary seeded with the first page of the directory and
 * the per-category chip counts on a bare `/tags`, so both are in the server
 * HTML.
 *
 * The element is a `HydrationBoundary` on every path, seeded or not: a
 * different element type here would remount the directory on the first
 * navigation off the seeded URL. A failed read seeds nothing, and the directory
 * fetches for itself and owns the loading and error states.
 */
export async function HydratedTagBrowse({
  searchParams,
}: {
  searchParams: Promise<RouteSearchParams>
}) {
  const dehydratedState = await dehydrateListFirstScreen(
    await searchParams,
    TAG_BROWSE_FIRST_SCREEN_DEFEATING_PARAMS,
    TAG_BROWSE_FIRST_SCREEN_READS.map(({ url, queryKey }) => ({
      queryKey,
      read: () =>
        fetchListPayload<TagListResponse>({
          url,
          collection: 'tags',
          service: 'tags-first-screen',
        }),
    })),
  )

  return (
    <HydrationBoundary state={dehydratedState}>
      <TagBrowse />
    </HydrationBoundary>
  )
}

export default function TagsPage({
  searchParams,
}: {
  searchParams: Promise<RouteSearchParams>
}) {
  return (
    <div className="flex min-h-screen items-start justify-center">
      <main className="w-full max-w-6xl px-4 py-8 md:px-8">
        <h1 className="text-3xl font-bold text-center mb-8">Tags</h1>
        <Suspense
          fallback={
            <div className="flex justify-center items-center py-12">
              <LoadingSpinner />
            </div>
          }
        >
          <HydratedTagBrowse searchParams={searchParams} />
        </Suspense>
      </main>
    </div>
  )
}
