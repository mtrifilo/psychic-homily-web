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
  /**
   * The route's loader: rendered on the server, through hydration, and on a
   * client navigation the cache cannot answer, so it must create no query.
   */
  loader: ReactNode
  /** The list itself, unseeded. */
  children: ReactNode
}

/** For a route that seeds every request. */
const NO_DEFEATING_PARAMS: readonly string[] = []

/**
 * The `Suspense` fallback above a server-seeded list: the list itself, read
 * from the browser's query cache, wherever rendering it cannot pre-empt the
 * seed, and `loader` everywhere else.
 *
 * The server render and the hydration render are always `loader`
 * (`useHydrated`), so the server HTML is the route's loader and the hydration
 * render agrees with it whatever the browser's cache holds.
 *
 * On a client navigation it renders `children` when the URL is off the first
 * screen (the server seeds nothing) or when every seeded key already holds
 * data (the seed never replaces an existing entry, see `seedFirstScreen`).
 * Otherwise the list's hooks would create a seeded key's entry before the
 * boundary renders, so it renders `loader`.
 *
 * `children` mounts as a fresh query observer, so `keepPreviousData` has
 * nothing to hold: another filter's, sort's or page's cached entry never
 * stands in for the current one. The list's other hooks (facets, counts,
 * per-row reads) may create their entries here; on a first-screen URL none of
 * them is a seeded key, since every seeded key already exists.
 *
 * The list drawn here is view-only (`inert`), links included, as the loader
 * it stands in for is. The boundary's list mounts as a separate instance and
 * replaces it, so text typed, focus taken or a sheet opened in this copy would
 * be discarded at the swap; `inert` also keeps the copy's mount effects from
 * taking focus. `inert` hides the copy from assistive tech too, so a status
 * outside it says the region is still loading.
 */
export function CachedListFallback({
  seededKeys,
  defeatingParams = NO_DEFEATING_PARAMS,
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

  if (!seedCannotBePreempted) return loader
  // `contents`: the wrapper adds no box, so the list lays out as it does in
  // the boundary.
  return (
    <>
      <span role="status" className="sr-only">
        Loading
      </span>
      <div inert className="contents">
        {children}
      </div>
    </>
  )
}
