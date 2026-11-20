'use client'

import { useTransition } from 'react'
import { useSearchParams } from 'next/navigation'
import {
  useQueryClient,
  type QueryClient,
  type QueryKey,
} from '@tanstack/react-query'
import type { CityState } from '@/components/filters/CityFilters'
import { CacheDrawnList } from '@/components/shared/CachedListFallback'
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

/** The all-cities selection. Stable, so the keys built from it are too. */
const ALL_CITIES_SELECTION: CityState[] = []

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
 * selection (`?cities=`, else favourites, else the geo default), resolved by
 * the hook the list itself uses. While that selection is still resolving the
 * fallback is the skeleton, so it never draws one city's rows and then
 * another's.
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
 * The URL and facet half. Its own component so the selection hook below, and
 * the geo request it can make, run only when the facet is already cached.
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
  // Required by the hook's `?cities=` setter, which nothing here calls.
  const [, startTransition] = useTransition()
  const { selectedCities, appliedGeoDefault, isResolving } =
    useShowListCitySelection({ citiesData, startTransition })

  if (
    isResolving ||
    !allCached(
      queryClient,
      showListFirstPaintKeys(selectedCities, appliedGeoDefault !== null)
    )
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
 * its first paint, for a selection.
 *
 * `fromGeoDefault` adds the all-cities entries: the list derives its geo
 * default after it mounts (`useGeoDefaultCity` answers null on a fresh
 * mount's first render), so a list whose selection came from geo reads those
 * entries first.
 */
export function showListFirstPaintKeys(
  selectedCities: CityState[],
  fromGeoDefault: boolean
): QueryKey[] {
  const selections = fromGeoDefault
    ? [selectedCities, ALL_CITIES_SELECTION]
    : [selectedCities]
  return [
    SHOW_CITIES_FIRST_SCREEN_KEY,
    ...selections.flatMap(cities => {
      const filters = showListFilters(cities, [], 'all')
      return [
        showsCalendarQueryKey({ limit: SHOWS_PAGE_SIZE, ...filters }),
        showMonthsQueryKey(filters),
      ]
    }),
  ]
}

const allCached = (queryClient: QueryClient, keys: readonly QueryKey[]) =>
  keys.every(key => queryClient.getQueryData(key) !== undefined)
