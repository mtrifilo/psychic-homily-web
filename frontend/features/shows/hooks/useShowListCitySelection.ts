'use client'

import { useMemo, useState, type TransitionStartFunction } from 'react'
import { useSearchParams } from 'next/navigation'
import { useQueryState } from 'nuqs'
import { useAuthContext } from '@/lib/context/AuthContext'
import { useProfile } from '@/features/auth'
import { toAuthUser } from '@/features/auth/authUser'
import type { CityState, CityWithCount } from '@/components/filters/CityFilters'
import { citiesParser, ALL_CITIES } from '@/components/filters/cityParams'
import { useGeoDefaultCity } from '@/components/filters/useGeoDefaultCity'
import { readCachedGeoAnswer } from '@/lib/geo-client'
import type { ShowCitiesResponse } from '../types'
import type { ShowListFilterOptions } from './useShows'

interface UseShowListCitySelectionParams {
  /** The city facet the list is rendering, or undefined while it loads. */
  citiesData: ShowCitiesResponse | undefined
  /**
   * Wraps the `?cities=` writes, so a caller can show them as pending. A
   * caller that only reads the selection omits it.
   */
  startTransition?: TransitionStartFunction
  /**
   * Where the location answer behind the geo default comes from.
   *
   * `'request'` (the list): `useGeoDefaultCity` reads the session cache after
   * mount and requests `/api/geo` when nothing is cached, so the default
   * arrives after the first render.
   *
   * `'cache'`: the session cache only, read on the first render, and never a
   * request. When the answer the selection needs is not cached,
   * `locationAnswerMissing` is true and the selection reads as all cities.
   * Browser only: read it after hydration.
   */
  geoSource?: 'request' | 'cache'
}

/**
 * Whether this list's geo default applies to a signed-in viewer with no
 * favourites. Passed to `useGeoDefaultCity` and used by the cache-only
 * eligibility below, so the two cannot disagree.
 */
const GEO_DEFAULT_FOR_SIGNED_IN = false

/**
 * The `/shows` list's city state: which cities it lists, and the inputs that
 * selection is derived from.
 *
 * One hook for the list and for anything that must name the rows the list is
 * about to request, so both derive the selection from the same inputs by the
 * same rules. They differ in timing only through `geoSource`: with `'cache'`
 * the geo default is known on the first render, while the list's arrives after
 * it mounts, so the list's first render reads all cities. A caller that draws
 * the list's rows from this selection must therefore never draw one that came
 * from the geo default.
 */
export function useShowListCitySelection({
  citiesData,
  startTransition,
  geoSource = 'request',
}: UseShowListCitySelectionParams) {
  const searchParams = useSearchParams()
  const { authStatus, user } = useAuthContext()
  const { data: profileData } = useProfile()

  // The favourites are known once the profile read has answered for THIS
  // viewer. A sign-in can set the user before its profile refetch lands, and
  // until then the favourites read as none.
  const favoritesKnown =
    authStatus !== 'authenticated' ||
    (profileData?.success === true &&
      profileData.user != null &&
      toAuthUser(profileData.user).id === user?.id)

  // Read once, at mount, and only in cache-only mode.
  const [cachedGeo] = useState(() =>
    geoSource === 'cache' ? readCachedGeoAnswer() : undefined
  )

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
      allowAuthenticated: GEO_DEFAULT_FOR_SIGNED_IN,
      favoritesSettled: favoritesKnown,
      ...(geoSource === 'cache'
        ? { geoFromServer: cachedGeo ?? null, enableClientFetch: false }
        : { enableClientFetch: true }),
    })

  // `useGeoDefaultCity`'s eligibility for the options passed above, on a mount
  // with no filter interaction yet. Cache-only mode needs it to tell "no city"
  // from "not cached".
  const geoEligible =
    (authStatus === 'anonymous' ||
      (GEO_DEFAULT_FOR_SIGNED_IN &&
        authStatus === 'authenticated' &&
        favoritesKnown)) &&
    favoriteCities.length === 0 &&
    !hasExistingSelection
  const locationAnswerMissing =
    geoSource === 'cache' && geoEligible && cachedGeo === undefined

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
    /** False while a signed-in viewer's own profile, and so their favourites, has not answered. */
    favoritesKnown,
    /** Cache-only mode: the location answer this selection depends on is not cached. */
    locationAnswerMissing,
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
