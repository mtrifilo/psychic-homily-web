'use client'

import type { ReactNode } from 'react'
import { useSearchParams } from 'next/navigation'
import { useQueryClient, type QueryKey } from '@tanstack/react-query'
import { useHydrated } from '@/lib/hooks/common/useHydrated'
import {
  isFirstScreenRequest,
  routeSearchParamsOf,
} from '@/lib/ssr/firstScreenParams'

interface CachedListFallbackProps {
  /** Every key the route's server render seeds on its first screen. */
  seededKeys: readonly QueryKey[]
  /**
   * The params that move the list off its first screen, as the route's seed
   * gate reads them. Empty for a route that seeds every request.
   */
  defeatingParams?: readonly string[]
  /** What the route showed in this slot before: its loader. */
  loader: ReactNode
  /** The list itself, unseeded. */
  children: ReactNode
}

/**
 * The `Suspense` fallback above a server-seeded list: the list itself, read
 * from the browser's query cache, wherever rendering it cannot pre-empt the
 * seed, and `loader` everywhere else.
 *
 * The server render and the hydration render are always `loader`
 * (`useHydrated`), so the server HTML is the route's loader and the hydration
 * render agrees with it whatever the browser's cache holds.
 *
 * On a client navigation it renders `children` when either holds:
 * - the URL is off the first screen, so the server seeds nothing;
 * - every seeded key already holds data in the cache. A seed is stamped 0 and
 *   `HydrationBoundary` replaces an existing entry only with newer data, so
 *   the list reading those entries changes nothing the seed would have done.
 *
 * Otherwise `loader`: the list's hooks would create a seeded key's entry with
 * no data before the boundary renders, and the seed would never replace it
 * (see `seedFirstScreen`).
 *
 * `children` mounts here as a fresh query observer, so `keepPreviousData` has
 * nothing to hold: an entry cached for another filter, sort or page never
 * stands in for the current one, and the list shows its own loading state for
 * a query the cache lacks. Its other hooks (facets, counts, per-row reads) run
 * too and may create their entries; on a first-screen URL none of them is a
 * seeded key, since every seeded key already exists.
 */
export function CachedListFallback({
  seededKeys,
  defeatingParams = [],
  loader,
  children,
}: CachedListFallbackProps) {
  const hydrated = useHydrated()
  if (!hydrated) return loader
  return (
    <WarmCacheGate
      seededKeys={seededKeys}
      defeatingParams={defeatingParams}
      loader={loader}
    >
      {children}
    </WarmCacheGate>
  )
}

/**
 * The client-navigation half. Its own component so `useSearchParams` never
 * runs in a server or prerender pass, where the gate above returns first.
 */
function WarmCacheGate({
  seededKeys,
  defeatingParams,
  loader,
  children,
}: Required<CachedListFallbackProps>) {
  const queryClient = useQueryClient()
  const searchParams = useSearchParams()

  const seeded = isFirstScreenRequest(
    routeSearchParamsOf(searchParams, defeatingParams),
    defeatingParams,
  )
  // `getQueryData` reads an entry without creating one.
  const seedCannotBePreempted =
    !seeded ||
    seededKeys.every(key => queryClient.getQueryData(key) !== undefined)

  return seedCannotBePreempted ? children : loader
}
