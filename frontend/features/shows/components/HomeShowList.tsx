'use client'

import { useState, useMemo, useCallback } from 'react'
import { useUpcomingShows, useShowCities } from '../hooks/useShows'
import { useShowSaveCountBatch } from '../hooks/useSavedShows'
import { usePrefetchRoutes } from '@/lib/hooks/common/usePrefetchRoutes'
import { useAuthContext } from '@/lib/context/AuthContext'
import { useProfile } from '@/features/auth'
import type { CityState } from '@/components/filters'
import { ShowCard } from './ShowCard'
import { CityFilters, type CityWithCount } from '@/components/filters'
import { citiesEqual } from '@/components/filters/cityParams'
import {
  useGeoDefaultCity,
  shouldShowGeoAffordance,
} from '@/components/filters/useGeoDefaultCity'
import { GeoDefaultAffordance } from '@/components/filters/GeoDefaultAffordance'
import { SaveDefaultsButton } from '@/components/filters/SaveDefaultsButton'

export function HomeShowList() {
  const { user, isAuthenticated, isLoading: authLoading } = useAuthContext()
  const isAdmin = user?.is_admin ?? false
  const { data: profileData } = useProfile()
  // The user's explicit pick this visit — null until they touch the filter.
  // Home has no URL persistence, so this one state is the only "selection";
  // the default (favorites, else anon geo) is DERIVED below, never written
  // into it. `[]` is a real choice ("All Cities"), distinct from null.
  const [userSelection, setUserSelection] = useState<CityState[] | null>(null)

  // Read favorites from profile
  const favoriteCities: CityState[] = useMemo(() => {
    const prefs = profileData?.user?.preferences
    if (!prefs?.favorite_cities) return []
    return prefs.favorite_cities
  }, [profileData?.user?.preferences])

  const timezone =
    typeof window !== 'undefined'
      ? Intl.DateTimeFormat().resolvedOptions().timeZone
      : 'America/Phoenix'

  const { data: citiesData } = useShowCities({ timezone })

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

  // IP-geo soft default for anon visitors (PSY-946). Home has no URL
  // persistence; the hook RETURNS the derived canonical city and it's folded
  // into `effectiveCities` below — nothing is ever written into state. The
  // page stays fully static; geo arrives via the `/api/geo` edge route
  // handler client-side. Favorites win (the hook stands down when
  // favoriteCities is non-empty); a user interaction nulls the derived value.
  const { appliedGeoDefault, notifyUserInteracted } = useGeoDefaultCity({
    cities,
    isAuthenticated,
    authLoading,
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

  const handleFilterChange = useCallback(
    (cities: CityState[]) => {
      notifyUserInteracted()
      setUserSelection(cities)
    },
    [notifyUserInteracted]
  )

  const { data, isLoading, isFetching, error } = useUpcomingShows({
    timezone,
    limit: 5,
    cities: effectiveCities.length > 0 ? effectiveCities : undefined,
  })

  // Prefetch /shows and /venues data during idle time
  usePrefetchRoutes(timezone)

  const showIds = useMemo(
    () => data?.shows?.map(s => s.id) ?? [],
    [data?.shows]
  )
  const { data: saveCounts } = useShowSaveCountBatch(showIds, isAuthenticated)

  // Determine if "Save as default" / "Clear defaults" should show
  const selectionDiffersFromFavorites = !citiesEqual(effectiveCities, favoriteCities)

  const showGeoAffordance = shouldShowGeoAffordance(
    appliedGeoDefault,
    effectiveCities
  )

  if (isLoading) {
    return (
      <div className="flex justify-center items-center py-8">
        <div className="animate-spin rounded-full h-6 w-6 border-b-2 border-foreground"></div>
      </div>
    )
  }

  if (error) {
    return (
      <div className="text-center py-8 text-muted-foreground">
        <p>Unable to load shows.</p>
      </div>
    )
  }

  return (
    <div className="w-full">
      {/* Show the filter whenever ≥1 city has shows (PSY-932) — consistent
          with /venues and /artists; hidden only when there are no cities. */}
      {cities.length > 0 && (
        <div className="mb-6">
          <CityFilters
            cities={cities}
            selectedCities={effectiveCities}
            onFilterChange={handleFilterChange}
          >
            {isAuthenticated && selectionDiffersFromFavorites && (
              <SaveDefaultsButton
                selectedCities={effectiveCities}
                favoriteCities={favoriteCities}
              />
            )}
          </CityFilters>
          {showGeoAffordance && (
            <GeoDefaultAffordance
              city={appliedGeoDefault}
              onChange={() => handleFilterChange([])}
            />
          )}
        </div>
      )}

      <div className={isFetching ? 'opacity-60 transition-opacity duration-75' : 'transition-opacity duration-75'}>
        {!data?.shows || data.shows.length === 0 ? (
          <div className="text-center py-8 text-muted-foreground">
            <p>
              {effectiveCities.length > 0
                ? `No upcoming shows in ${effectiveCities.map(c => c.city).join(', ')}.`
                : 'No upcoming shows at this time.'}
            </p>
          </div>
        ) : (
          <div className="flex flex-col gap-3">
            {data.shows.map(show => (
              <ShowCard
                key={show.id}
                show={show}
                isAdmin={isAdmin}
                saveData={saveCounts?.[String(show.id)]}
              />
            ))}
          </div>
        )}
      </div>
    </div>
  )
}
