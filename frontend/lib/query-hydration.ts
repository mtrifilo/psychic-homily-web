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
import { connection } from 'next/server'
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

/** One cache entry to seed: the key the client hook will read, and its data. */
export interface QuerySeed {
  queryKey: readonly unknown[]
  data: unknown
}

/**
 * Seed several cache entries into ONE dehydrated state, each marked as
 * already stale.
 *
 * Two things separate this from `prefetchEntity`, and both are why the list
 * pages (PSY-1624) need their own function rather than a second call to it.
 *
 * **It seeds several keys at once.** `getQueryClient()` mints a fresh client
 * per call on the server, so two `prefetchEntity` calls produce two states and
 * only one can be handed to a `<HydrationBoundary>`. A list page blocks its
 * first paint on BOTH its rows and its filter facets, so seeding one without
 * the other server-renders the skeleton regardless.
 *
 * **It stamps `dataUpdatedAt: 0` instead of "now".** `prefetchQuery` records
 * the current time, which is two problems at once here. Honesty: these
 * payloads come out of Next's Data Cache and can be up to
 * `FIRST_SCREEN_REVALIDATE_SECONDS` old, so "fetched just now" would suppress
 * the client's revalidation for a `staleTime` it never earned. And mechanics:
 * under `cacheComponents`, reading the clock in a prerenderable scope is a
 * build error ("used `Date.now()` before accessing either uncached data or
 * Request data") — the guard exists precisely because a timestamp baked into a
 * cached shell goes on lying as the shell ages. Seeding at 0 says "paint this,
 * then go check", which is what a server-rendered first screen actually is.
 *
 * **`await connection()` is load-bearing, not defensive.** `dehydrate()` reads
 * the clock too — it stamps its own `dehydratedAt` — and that one is TanStack's
 * to spend, not ours to zero out, so the seed can only be produced where
 * reading the clock is legitimate: a request-time render. This marks the
 * CALLING BOUNDARY dynamic, which under `cacheComponents` means a PPR hole
 * streamed into the prerendered shell, not a whole-route opt-out — the page
 * chrome still comes from the static shell, and the payload underneath is
 * still Data-Cached, so a request-time render is not a request-time fetch. It
 * lives in here rather than at each call site because a caller that forgot it
 * would fail at BUILD, on a message that names neither this function nor the
 * reason.
 */
export async function seedFirstScreen(
  seeds: readonly QuerySeed[],
): Promise<DehydratedState> {
  await connection()
  const queryClient = getQueryClient()
  for (const seed of seeds) {
    queryClient.setQueryData(seed.queryKey, seed.data, { updatedAt: 0 })
  }
  return dehydrate(queryClient)
}
