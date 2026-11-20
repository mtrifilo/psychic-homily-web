'use client'

import { useState, useTransition } from 'react'
import { useSearchParams } from 'next/navigation'
import {
  useQueryClient,
  type QueryClient,
  type QueryKey,
} from '@tanstack/react-query'
import type { CityState } from '@/components/filters/CityFilters'
import { CacheDrawnList } from '@/components/shared/CachedListFallback'
import { useAuthContext } from '@/lib/context/AuthContext'
import { readCachedGeoAnswer } from '@/lib/geo-client'
import { useHydrated } from '@/lib/hooks/common/useHydrated'
import {
  isFirstScreenRequest,
  routeSearchParamsOf,
} from '@/lib/ssr/firstScreenParams'
import { SHOW_CITIES_FIRST_SCREEN_KEY } from '../api'
import { showMonthsQueryKey, showsCalendarQueryKey } from '../hooks/useShows'
import {
  showListFilters,
  useShowListCitySelection,
} from '../hooks/useShowListCitySelection'
import { SHOWS_PAGE_SIZE } from '../showsListNavigation'
import type { ShowCitiesResponse } from '../types'
import { ShowList } from './ShowList'
import { ShowListSkeleton } from './ShowListSkeleton'

/**
 * The params that key the list's rows by more than the city selection. On a
 * URL carrying any of them the fallback is always the skeleton.
 */
export const SHOW_LIST_SKELETON_ONLY_PARAMS: readonly string[] = [
  'tags',
  'tag_match',
  'page',
]

/**
 * The `Suspense` fallback above the seeded, unwindowed `/shows` list: the list
 * itself, drawn from the browser's query cache, when every entry it reads
 * before its first paint is already cached for this viewer, and the skeleton
 * everywhere else.
 *
 * The server render and the hydration render are always the skeleton
 * (`useHydrated`), so the server HTML is the route's loader whatever the
 * browser's cache holds.
 *
 * On a client navigation the entries are named from the viewer's own city
 * selection (`?cities=`, else favourites, else all cities), resolved by the
 * hook the list itself uses. The fallback is the skeleton while that selection
 * is resolving, whenever it comes from the geo default, and whenever the
 * cached location answer it would need is missing, so it never draws one
 * city's rows and then another's. It reads the location answer from the
 * session cache only and never requests it.
 *
 * The page's seed writes the all-cities entries only, and never replaces an
 * entry the cache already holds (see `seedFirstScreen`). Every entry the list
 * reads before its first paint is required to exist here, so drawing it
 * creates none of the seeded ones.
 */
export function ShowListFallback() {
  const hydrated = useHydrated()
  if (!hydrated) return <ShowListSkeleton />
  return <CachedFacetGate />
}

/**
 * The URL and facet half. Its own component so the selection hook below runs
 * only over a facet that is already cached.
 */
function CachedFacetGate() {
  const queryClient = useQueryClient()
  const searchParams = useSearchParams()

  const onCityKeyedUrl = isFirstScreenRequest(
    routeSearchParamsOf(searchParams, SHOW_LIST_SKELETON_ONLY_PARAMS),
    SHOW_LIST_SKELETON_ONLY_PARAMS
  )
  // `getQueryData` reads an entry without creating one.
  const citiesData = onCityKeyedUrl
    ? queryClient.getQueryData<ShowCitiesResponse>(SHOW_CITIES_FIRST_SCREEN_KEY)
    : undefined

  if (!citiesData) return <ShowListSkeleton />
  return <CachedSelectionGate citiesData={citiesData} />
}

function CachedSelectionGate({ citiesData }: { citiesData: ShowCitiesResponse }) {
  const queryClient = useQueryClient()
  const { authStatus } = useAuthContext()
  // Required by the hook's `?cities=` setter, which nothing here calls.
  const [, startTransition] = useTransition()
  // Read once, at mount. This component renders only after hydration, so the
  // read cannot disagree with server HTML, and it makes no request.
  const [cachedGeo] = useState(readCachedGeoAnswer)
  const { selectedCities, appliedGeoDefault, isResolving, fallsBackToDefault } =
    useShowListCitySelection({
      citiesData,
      startTransition,
      knownGeo: cachedGeo ?? null,
    })

  // The geo default applies to anonymous viewers only (`useGeoDefaultCity`),
  // so with no cached answer an anonymous viewer's selection is unknown.
  const locationAnswerUnknown =
    fallsBackToDefault && authStatus === 'anonymous' && cachedGeo === undefined

  // A selection from the geo default is never drawn here: the list derives
  // that default after it mounts, so a drawn copy would paint the all-cities
  // rows first.
  if (
    isResolving ||
    appliedGeoDefault !== null ||
    locationAnswerUnknown ||
    !allCached(queryClient, showListFirstPaintKeys(selectedCities))
  ) {
    return <ShowListSkeleton />
  }
  return (
    <CacheDrawnList>
      <ShowList />
    </CacheDrawnList>
  )
}

/**
 * Every entry the unwindowed, untagged first page of the list reads before
 * its first paint, for a selection that did not come from the geo default.
 */
export function showListFirstPaintKeys(selectedCities: CityState[]): QueryKey[] {
  const filters = showListFilters(selectedCities, [], 'all')
  return [
    SHOW_CITIES_FIRST_SCREEN_KEY,
    showsCalendarQueryKey({ limit: SHOWS_PAGE_SIZE, ...filters }),
    showMonthsQueryKey(filters),
  ]
}

const allCached = (queryClient: QueryClient, keys: readonly QueryKey[]) =>
  keys.every(key => queryClient.getQueryData(key) !== undefined)
