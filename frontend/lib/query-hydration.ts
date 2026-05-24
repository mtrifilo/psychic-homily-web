/**
 * Server-side query hydration helpers.
 *
 * Companion to `lib/queryClient.ts` for server components (e.g.
 * `app/<entity>/[slug]/page.tsx`) that need to seed a TanStack Query
 * cache from data the server already fetched, so the matching client
 * hook resolves from the hydrated cache instead of refetching.
 *
 * No `'use client'` directive — this module is server-importable.
 *
 * Usage:
 *
 *   const data = await getEntity(slug)              // React.cache-wrapped fetch
 *   const state = await prefetchEntity(
 *     queryKeys.<entity>.detail(slug),
 *     data,
 *   )
 *   return (
 *     <HydrationBoundary state={state}>
 *       <EntityDetail entityId={slug} />
 *     </HydrationBoundary>
 *   )
 *
 * The `queryFn` returns the cached value synchronously — `cache()` on
 * the upstream fetch guarantees the network call has already happened,
 * so this is a no-op cache write rather than a second refetch.
 */

import { dehydrate, type DehydratedState } from '@tanstack/react-query'
import { getQueryClient } from '@/lib/queryClient'

export async function prefetchEntity<T>(
  queryKey: readonly unknown[],
  data: T,
): Promise<DehydratedState> {
  const queryClient = getQueryClient()
  await queryClient.prefetchQuery({
    queryKey,
    queryFn: () => data,
  })
  return dehydrate(queryClient)
}
