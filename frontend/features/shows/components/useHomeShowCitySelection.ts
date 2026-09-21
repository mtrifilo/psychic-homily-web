'use client'

import { useCallback, useMemo, useState } from 'react'
import { useProfile } from '@/features/auth'
import { useAuthContext } from '@/lib/context/AuthContext'
import type { CityState } from '@/components/filters'
import type { CityWithCount } from '@/components/filters'
import { citiesEqual } from '@/components/filters/cityParams'
import {
  useGeoDefaultCity,
  shouldShowGeoAffordance,
} from '@/components/filters/useGeoDefaultCity'
import { useShowCities } from '../hooks/useShows'

/**
 * The home surfaces' city selection, owned in one place so the list and any
 * header that NAMES the city cannot disagree (PSY-2103).
 *
 * Home has no URL persistence, so `userSelection` is the only stored state: the
 * default (favorites, else the anonymous geo city) is DERIVED on every render
 * and never written into it. `[]` is a real choice ("All Cities"), distinct
 * from `null` ("the user has not touched the filter").
 *
 * Geo applies to a settled ANONYMOUS visitor only — that gate lives in
 * `useGeoDefaultCity` and is deliberate. A signed-in viewer with no favorite
 * cities therefore resolves to no city at all, and a caller that renders the
 * city's NAME must handle the empty case rather than assume one exists.
 */
export interface HomeShowCitySelection {
  cities: CityWithCount[]
  favoriteCities: CityState[]
  effectiveCities: CityState[]
  /** The IP-geo city to explain in the "from your location" affordance, or null
   *  when there is nothing to explain. Carries the city rather than a flag so
   *  the consumer needs no non-null assertion to render it. */
  geoAffordanceCity: CityState | null
  selectionDiffersFromFavorites: boolean
  onFilterChange: (cities: CityState[]) => void
}

export function useHomeShowCitySelection(): HomeShowCitySelection {
  const { authStatus } = useAuthContext()
  const { data: profileData } = useProfile()
  const [userSelection, setUserSelection] = useState<CityState[] | null>(null)

  // Read favorites from profile
  const favoriteCities: CityState[] = useMemo(() => {
    const prefs = profileData?.user?.preferences
    if (!prefs?.favorite_cities) return []
    return prefs.favorite_cities
  }, [profileData?.user?.preferences])

  const { data: citiesData } = useShowCities()

  const cities: CityWithCount[] = useMemo(
    () =>
      citiesData?.cities?.map(c => ({
        city: c.city,
        state: c.state,
        count: c.show_count,
        // Geocoded centroid (PSY-981) — drives the nearest-has-shows-city geo
        // default when the visitor's exact city has no shows.
        latitude: c.latitude,
        longitude: c.longitude,
      })) ?? [],
    [citiesData?.cities]
  )

  // IP-geo soft default for anon visitors (PSY-946). The hook RETURNS the
  // derived canonical city and it's folded into `effectiveCities` below —
  // nothing is ever written into state. The page stays fully static; geo
  // arrives via the `/api/geo` edge route handler client-side. Favorites win
  // (the hook stands down when favoriteCities is non-empty); a user
  // interaction nulls the derived value.
  const { appliedGeoDefault, notifyUserInteracted } = useGeoDefaultCity({
    cities,
    authStatus,
    favoriteCities,
    hasExistingSelection: userSelection !== null,
    enableClientFetch: true,
  })

  // The effective selection, DERIVED during render: the user's explicit pick
  // wins; otherwise favorites; otherwise the anon geo default. No effect, no
  // ref — the default can't be dropped or applied late because it's computed
  // from the current inputs on every render.
  const effectiveCities: CityState[] = useMemo(() => {
    if (userSelection !== null) return userSelection
    if (favoriteCities.length > 0) return favoriteCities
    return appliedGeoDefault ? [appliedGeoDefault] : []
  }, [userSelection, favoriteCities, appliedGeoDefault])

  const onFilterChange = useCallback(
    (nextCities: CityState[]) => {
      notifyUserInteracted()
      setUserSelection(nextCities)
    },
    [notifyUserInteracted]
  )

  return {
    cities,
    favoriteCities,
    effectiveCities,
    geoAffordanceCity: shouldShowGeoAffordance(appliedGeoDefault, effectiveCities)
      ? appliedGeoDefault
      : null,
    // Determines whether "Save as default" / "Clear defaults" should show.
    selectionDiffersFromFavorites: !citiesEqual(effectiveCities, favoriteCities),
    onFilterChange,
  }
}
