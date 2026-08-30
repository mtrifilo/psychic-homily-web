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

/**
 * Seed ONE key. The single-seed spelling of {@link prefetchEntities}.
 *
 * `NonNullable` so the inherited null-SKIP is unreachable rather than merely
 * unused: every caller today is a `[slug]` route that has already
 * `notFound()`ed, and a future caller seeding a legitimately-null payload
 * should get a compile error instead of a silently dropped seed.
 */
export async function prefetchEntity<T>(
  queryKey: readonly unknown[],
  data: NonNullable<T>,
): Promise<DehydratedState> {
  return prefetchEntities([{ queryKey, data }])
}

/** One cache entry to seed: the key the client hook will read, and its data. */
export interface QuerySeed {
  queryKey: readonly unknown[]
  data: unknown
}

/**
 * `prefetchEntity` for a page whose first paint depends on more than one key.
 *
 * `getQueryClient()` mints a fresh client per call on the server, so two
 * `prefetchEntity` calls produce two dehydrated states and a
 * `<HydrationBoundary>` can only take one. Seeding all of a page's keys through
 * one client is the whole difference.
 *
 * Freshness semantics are `prefetchEntity`'s, NOT `seedFirstScreen`'s: each
 * entry is stamped "fetched now" and the client honours its `staleTime` instead
 * of revalidating on the first commit. Use this when every seed is an ANONYMOUS
 * read whose payload does not vary by viewer, so a stamp the client trusts
 * cannot hide a signed-in difference. Use `seedFirstScreen` when a seed's
 * payload WOULD differ for a signed-in viewer, which is what forces the
 * immediate revalidation there. (Both are routinely Data-Cached; that alone
 * does not decide it, since a `[slug]` detail read is the same bytes for
 * everyone.)
 *
 * Callers on a route with no dynamic input of its own must `await connection()`
 * first, for the reason `seedFirstScreen` documents: `dehydrate()` reads the
 * clock, and under `cacheComponents` a prerenderable scope may not. A `[slug]`
 * route has already awaited `params`, which satisfies the guard.
 *
 * A seed whose `data` is null is SKIPPED rather than cached, so a module whose
 * server fetch failed falls through to its own client fetch and its own error
 * state instead of hydrating into a permanent empty.
 */
export async function prefetchEntities(
  seeds: readonly QuerySeed[],
): Promise<DehydratedState> {
  const queryClient = getQueryClient()
  for (const seed of seeds) {
    if (seed.data == null) continue
    await queryClient.prefetchQuery({
      queryKey: seed.queryKey,
      queryFn: () => seed.data,
    })
  }
  return dehydrate(queryClient)
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
 * The forced revalidation it buys is not only a freshness nicety. The server
 * fetch forwards no cookies, so the seed is always the ANONYMOUS payload — an
 * admin, whose `/shows/upcoming` normally includes unapproved shows, would
 * otherwise sit on a first screen that silently omits them. Landing stale is
 * what lets the client correct that on its own. Two consequences to keep in
 * mind at the call site: the seeded query reports `isFetching` on its very
 * first commit (do not wire that to a "dimming" affordance — see `ShowList`),
 * and a failed revalidation leaves `error` set while `data` is still the
 * server payload (gate error states on `!data`).
 *
 * **`await connection()` is load-bearing, not defensive.** `dehydrate()` reads
 * the clock too — it stamps its own `dehydratedAt` — and that one is TanStack's
 * to spend, not ours to zero out, so the seed can only be produced where
 * reading the clock is legitimate: a request-time render. Without it these
 * three pages fail the `cacheComponents` prerender guard ("used `Date.now()`
 * before accessing either uncached data or Request data") — observed, not
 * theorised. `prefetchEntity` above needs no such call only because its
 * callers are `[slug]` routes that have already `await`ed `params`, which is
 * Request data and satisfies the same guard; a route with no dynamic input of
 * its own has nothing to satisfy it with.
 *
 * What this costs is close to nothing here: `AuthHydrator` in the root layout
 * already reads `cookies()` inside a `<Suspense>`, so every route in this app
 * already streams its content subtree. This adds a second hole to a request
 * that had one, not a first — and the payload underneath is still Data-Cached,
 * so a request-time render is not a request-time fetch.
 *
 * It lives in here rather than at each call site because a caller that forgot
 * it would fail at BUILD, on a message that names neither this function nor
 * the reason.
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
