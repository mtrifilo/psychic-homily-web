'use client'

import { useMemo, type TransitionStartFunction } from 'react'
import { useSearchParams } from 'next/navigation'
import { useQueryState } from 'nuqs'
import { useAuthContext } from '@/lib/context/AuthContext'
import { useProfile } from '@/features/auth'
import type { CityState, CityWithCount } from '@/components/filters/CityFilters'
import { citiesParser, ALL_CITIES } from '@/components/filters/cityParams'
import { useGeoDefaultCity } from '@/components/filters/useGeoDefaultCity'
import type { ShowCitiesResponse } from '../types'
import type { ShowListFilterOptions } from './useShows'

interface UseShowListCitySelectionParams {
  /** The city facet the list is rendering, or undefined while it loads. */
  citiesData: ShowCitiesResponse | undefined
  /** Wraps the `?cities=` writes, so a caller can show them as pending. */
  startTransition: TransitionStartFunction
}

/**
 * The `/shows` list's city state: which cities it lists, and the inputs that
 * selection is derived from.
 *
 * One hook for the list and for anything that must name the rows the list is
 * about to request, so the two derive the selection from the same inputs on
 * the same schedule, the geo default included.
 */
export function useShowListCitySelection({
  citiesData,
  startTransition,
}: UseShowListCitySelectionParams) {
  const searchParams = useSearchParams()
  const { authStatus } = useAuthContext()
  const { data: profileData } = useProfile()

  // Read favorites from profile: the per-user default city.
  const favoriteCities: CityState[] = useMemo(() => {
    const prefs = profileData?.user?.preferences
    if (!prefs?.favorite_cities) return []
    return prefs.favorite_cities
  }, [profileData?.user?.preferences])

  // `?cities=` is the source of truth, read/written via nuqs. Three states:
  //   null        -> param absent -> apply the default (favorites/geo), derived below
  //   ALL_CITIES  -> ?cities=all -> explicit "all cities"
  //   CityState[] -> explicit selection
  // Filter changes push a history entry so the back button steps through them.
  const [citiesState, setCities] = useQueryState(
    'cities',
    citiesParser.withOptions({ history: 'push', startTransition })
  )

  // Legacy single-city params (?city=&state=): read-only, for back-compat.
  const legacyCity = searchParams.get('city')
  const legacyState = searchParams.get('state')

  // Any explicit selection (?cities=<pick>, ?cities=all, or legacy single-city)
  // means geo must not seed. Authed favorites also stand the geo hook down
  // (handled inside the hook via favoriteCities + authStatus).
  const hasExistingSelection = citiesState !== null || !!(legacyCity && legacyState)

  // Map ShowCity to CityWithCount (the has-shows list) for the filter and the
  // geo hook.
  const cities: CityWithCount[] = useMemo(
    () =>
      citiesData?.cities?.map(c => ({
        city: c.city,
        state: c.state,
        count: c.show_count,
        // Geocoded centroid: drives the nearest-has-shows-city geo default
        // when the visitor's exact city has no shows.
        latitude: c.latitude,
        longitude: c.longitude,
      })) ?? [],
    [citiesData?.cities]
  )

  // IP-geo soft default for anon visitors. /shows reads geo via the
  // `/api/geo` route handler client-side (the page must not read
  // `next/headers`). The hook RETURNS the derived canonical city; it is folded
  // into the derived selection below (never written to the URL). Favorites and
  // an existing `?cities=`/legacy selection both win (the hook stands down).
  const { appliedGeoDefault, notifyUserInteracted, isResolving } =
    useGeoDefaultCity({
      cities,
      authStatus,
      favoriteCities,
      hasExistingSelection,
      enableClientFetch: true,
    })

  // The effective city filter, DERIVED during render, never seeded into the
  // URL by an effect. A bare /shows resolves to the user's favorite (or, for
  // anon visitors, the geo default); an explicit ?cities=all or ?cities=<pick>
  // wins. Deriving (rather than writing the default from a mount effect) is
  // what makes the default survive client-side navigation: the URL, not a
  // mount ref, is the source of truth.
  const selectedCities: CityState[] = useMemo(() => {
    if (citiesState === ALL_CITIES) return []
    if (citiesState) return citiesState
    if (legacyCity && legacyState) return [{ city: legacyCity, state: legacyState }]
    if (favoriteCities.length > 0) return favoriteCities
    return appliedGeoDefault ? [appliedGeoDefault] : []
  }, [citiesState, legacyCity, legacyState, favoriteCities, appliedGeoDefault])

  return {
    /** The facet as filter options: the cities that have shows. */
    cities,
    selectedCities,
    favoriteCities,
    /** The geo city in `selectedCities`, or null when it did not come from geo. */
    appliedGeoDefault,
    /** True while the selection may still change without input: see `useGeoDefaultCity`. */
    isResolving,
    notifyUserInteracted,
    setCities,
  }
}

/**
 * The filters the list's rows and month histogram are keyed by. An empty
 * selection or tag set is absent, so every spelling of "unfiltered" reads one
 * entry.
 */
export function showListFilters(
  selectedCities: CityState[],
  selectedTags: string[],
  tagMatch: 'all' | 'any'
): ShowListFilterOptions {
  return {
    cities: selectedCities.length > 0 ? selectedCities : undefined,
    tags: selectedTags.length > 0 ? selectedTags : undefined,
    tagMatch,
  }
}
