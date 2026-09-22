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
import { useShowCities } from './useShows'

/**
 * The home surfaces' city selection, owned in one place so the list and any
 * header that NAMES the city cannot disagree (PSY-2103).
 *
 * Home has no URL persistence, so `userSelection` is the only stored state: the
 * default (favorites, else the anonymous geo city) is DERIVED on every render
 * and never written into it. `[]` is a real choice ("All Cities"), distinct
 * from `null` ("the user has not touched the filter").
 *
 * Resolution order is favorites, then the IP-geo city, then the liveliest city
 * that has shows. The last two steps are opt-in per caller
 * ({@link HomeShowCitySelectionOptions}): the anonymous home takes only the
 * first two, which is its long-standing behavior, while a caller that NAMES the
 * resolved city in its copy takes all three so the name is almost always there
 * to render. Even then the result can be empty (a brand-new index with no
 * cities at all), so a caller must still handle that.
 */

export interface HomeShowCitySelectionOptions {
  /**
   * Resolve the IP-geo city for a settled authenticated viewer with no
   * favorites too, and fall back to the liveliest city that has shows when geo
   * yields nothing. Both are for a surface whose copy names the city; leaving
   * them off preserves the anonymous home's exact behavior.
   */
  resolveCityForCopy?: boolean
}
/**
 * Where `effectiveCities` came from. A header that claims proximity must know
 * whether the city is the viewer's own pick, a favorite, an IP-geo match, or
 * the last-resort guess; the bare list erases that.
 */
export type HomeShowCitySource =
  | 'user'
  | 'favorites'
  | 'geo'
  | 'liveliest'
  | 'none'

export interface HomeShowCitySelection {
  cities: CityWithCount[]
  favoriteCities: CityState[]
  effectiveCities: CityState[]
  source: HomeShowCitySource
  /** True while the IP-geo tier is still deciding. A surface whose copy names
   *  the city should hold rather than paint a city it may be about to swap. */
  isResolving: boolean
  /** The IP-geo city to explain in the "from your location" affordance, or null
   *  when there is nothing to explain. Carries the city rather than a flag so
   *  the consumer needs no non-null assertion to render it. */
  geoAffordanceCity: CityState | null
  selectionDiffersFromFavorites: boolean
  onFilterChange: (cities: CityState[]) => void
}

export function useHomeShowCitySelection({
  resolveCityForCopy = false,
}: HomeShowCitySelectionOptions = {}): HomeShowCitySelection {
  const { authStatus } = useAuthContext()
  const { data: profileData } = useProfile()
  // The favorites are known once the profile read has answered. For an
  // authenticated viewer that read can lag `authStatus` (a passkey sign-in
  // sets the user before its refetch lands), and geo must not run ahead of it.
  const favoritesSettled =
    authStatus !== 'authenticated' || profileData !== undefined
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
  const { appliedGeoDefault, notifyUserInteracted, isResolving } =
    useGeoDefaultCity({
      cities,
      authStatus,
      favoriteCities,
      hasExistingSelection: userSelection !== null,
      enableClientFetch: true,
      allowAuthenticated: resolveCityForCopy,
      favoritesSettled,
    })

  // The liveliest city that has shows, by the same count the filter chips
  // display. Last resort only: it is a guess about where the viewer is, so it
  // never outranks a favorite, a geo match, or the viewer's own pick, and it
  // waits for geo to answer rather than filling the slot geo may still claim.
  const liveliestCity: CityState | null = useMemo(() => {
    if (!resolveCityForCopy || isResolving || cities.length === 0) return null
    const top = cities.reduce((best, city) =>
      city.count > best.count ? city : best
    )
    return top.count > 0 ? { city: top.city, state: top.state } : null
  }, [resolveCityForCopy, isResolving, cities])

  // The effective selection, DERIVED during render: the user's explicit pick
  // wins; otherwise favorites; otherwise the anon geo default. No effect, no
  // ref — the default can't be dropped or applied late because it's computed
  // from the current inputs on every render.
  const resolved = useMemo((): {
    cities: CityState[]
    source: HomeShowCitySource
  } => {
    if (userSelection !== null) return { cities: userSelection, source: 'user' }
    if (favoriteCities.length > 0)
      return { cities: favoriteCities, source: 'favorites' }
    if (appliedGeoDefault) return { cities: [appliedGeoDefault], source: 'geo' }
    if (liveliestCity) return { cities: [liveliestCity], source: 'liveliest' }
    return { cities: [], source: 'none' }
  }, [userSelection, favoriteCities, appliedGeoDefault, liveliestCity])
  const effectiveCities = resolved.cities
  const source = resolved.source

  const onFilterChange = useCallback(
    (nextCities: CityState[]) => {
      notifyUserInteracted()
      setUserSelection(nextCities)
    },
    [notifyUserInteracted]
  )

  // Memoized as a whole, not only field by field: callers hold this object as
  // a prop, so a fresh identity each render would defeat any memo they put
  // around the list it feeds.
  return useMemo(
    () => ({
      cities,
      favoriteCities,
      effectiveCities,
      source,
      isResolving,
      geoAffordanceCity: shouldShowGeoAffordance(
        appliedGeoDefault,
        effectiveCities
      )
        ? appliedGeoDefault
        : null,
      // Determines whether "Save as default" / "Clear defaults" should show.
      selectionDiffersFromFavorites: !citiesEqual(
        effectiveCities,
        favoriteCities
      ),
      onFilterChange,
    }),
    [
      cities,
      favoriteCities,
      effectiveCities,
      source,
      isResolving,
      appliedGeoDefault,
      onFilterChange,
    ]
  )
}
