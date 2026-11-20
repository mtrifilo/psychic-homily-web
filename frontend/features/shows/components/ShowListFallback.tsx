'use client'

import { useSearchParams } from 'next/navigation'
import { useQueryClient, type QueryKey } from '@tanstack/react-query'
import type { CityState } from '@/components/filters/CityFilters'
import {
  CacheDrawnList,
  everyKeyCached,
} from '@/components/shared/CachedListFallback'
import { useHydrated } from '@/lib/hooks/common/useHydrated'
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
 * The only params a URL may carry for the fallback to draw: the city
 * selection's. Any other param (a tag filter, a page, or one the list adds
 * later) leaves the fallback on the skeleton, so the rows it names are always
 * the rows the list reads.
 */
const DRAWABLE_PARAMS: ReadonlySet<string> = new Set(['cities', 'city', 'state'])

/**
 * The `Suspense` fallback above the seeded, unwindowed `/shows` list: the list
 * itself, drawn from the browser's query cache, when the entries that decide
 * which rows it paints are already cached for this viewer, and the skeleton
 * everywhere else.
 *
 * The server render and the hydration render are always the skeleton
 * (`useHydrated`), so the server HTML is the route's loader whatever the
 * browser's cache holds.
 *
 * On a client navigation the entries are named from the viewer's own city
 * selection (`?cities=`, else favourites, else all cities), derived by the
 * hook the list itself uses, reading the location answer from the session
 * cache only and never requesting it.
 *
 * The page's seed writes the all-cities entries only, and never replaces an
 * entry the cache already holds (see `seedFirstScreen`). Those entries are
 * required to exist before the all-cities list is drawn, so drawing it creates
 * none of them.
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

  const onDrawableUrl = Array.from(searchParams.keys()).every(name =>
    DRAWABLE_PARAMS.has(name)
  )
  // `getQueryData` reads an entry without creating one.
  const citiesData = onDrawableUrl
    ? queryClient.getQueryData<ShowCitiesResponse>(SHOW_CITIES_FIRST_SCREEN_KEY)
    : undefined

  if (!citiesData) return <ShowListSkeleton />
  return <CachedSelectionGate citiesData={citiesData} />
}

function CachedSelectionGate({ citiesData }: { citiesData: ShowCitiesResponse }) {
  const queryClient = useQueryClient()
  const {
    selectedCities,
    appliedGeoDefault,
    isResolving,
    favoritesKnown,
    locationAnswerMissing,
  } = useShowListCitySelection({ citiesData, geoSource: 'cache' })

  // Drawn only for a selection that is settled and is the one the list's
  // first render reads. A geo-default selection fails the second test: the
  // list derives that default after it mounts, so its first render would paint
  // the all-cities rows.
  const selectionSettled =
    !isResolving && favoritesKnown && !locationAnswerMissing
  if (
    !selectionSettled ||
    appliedGeoDefault !== null ||
    !everyKeyCached(queryClient, showListFirstPaintKeys(selectedCities))
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
 * The entries that decide which rows the unwindowed, untagged first page of
 * the list paints for a selection: the city facet, the rows and the month
 * histogram.
 */
export function showListFirstPaintKeys(selectedCities: CityState[]): QueryKey[] {
  const filters = showListFilters(selectedCities, [], 'all')
  return [
    SHOW_CITIES_FIRST_SCREEN_KEY,
    showsCalendarQueryKey({ limit: SHOWS_PAGE_SIZE, ...filters }),
    showMonthsQueryKey(filters),
  ]
}
