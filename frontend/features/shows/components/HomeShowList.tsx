'use client'

import { useState, useMemo, useCallback, useRef, useEffect } from 'react'
import { useUpcomingShows, useShowCities } from '../hooks/useShows'
import { useSavedShowBatch } from '../hooks/useSavedShows'
import { useBatchAttendance } from '../hooks/useAttendance'
import { usePrefetchRoutes } from '@/lib/hooks/common/usePrefetchRoutes'
import { useAuthContext } from '@/lib/context/AuthContext'
import { useProfile, useSetFavoriteCities } from '@/features/auth'
import type { ShowResponse } from '../types'
import type { CityState } from '@/components/filters'
import { ShowCard } from './ShowCard'
import { CityFilters, type CityWithCount } from '@/components/filters'
import { SaveDefaultsButton } from '@/components/filters/SaveDefaultsButton'

/** Compare two city arrays for equality (order-insensitive) */
function citiesEqual(a: CityState[], b: CityState[]): boolean {
  if (a.length !== b.length) return false
  const setA = new Set(a.map(c => `${c.city}|${c.state}`))
  return b.every(c => setA.has(`${c.city}|${c.state}`))
}

export function HomeShowList() {
  const { user, isAuthenticated } = useAuthContext()
  const isAdmin = user?.is_admin ?? false
  const { data: profileData } = useProfile()
  const [selectedCities, setSelectedCities] = useState<CityState[]>([])
  const hasManuallyInteracted = useRef(false)

  // Read favorites from profile
  const favoriteCities: CityState[] = useMemo(() => {
    const prefs = profileData?.user?.preferences
    if (!prefs?.favorite_cities) return []
    return prefs.favorite_cities
  }, [profileData?.user?.preferences])

  // Apply favorites as defaults when profile loads (only if user hasn't manually changed)
  useEffect(() => {
    if (!hasManuallyInteracted.current && favoriteCities.length > 0) {
      setSelectedCities(favoriteCities)
    }
  }, [favoriteCities])

  const handleFilterChange = useCallback((cities: CityState[]) => {
    hasManuallyInteracted.current = true
    setSelectedCities(cities)
  }, [])

  const timezone =
    typeof window !== 'undefined'
      ? Intl.DateTimeFormat().resolvedOptions().timeZone
      : 'America/Phoenix'

  const { data: citiesData } = useShowCities({ timezone })

  const { data, isLoading, isFetching, error } = useUpcomingShows({
    timezone,
    limit: 5,
    cities: selectedCities.length > 0 ? selectedCities : undefined,
  })

  // Prefetch /shows and /venues data during idle time
  usePrefetchRoutes(timezone)

  const showIds = useMemo(
    () => data?.shows?.map(s => s.id) ?? [],
    [data?.shows]
  )
  const { data: savedShowIds } = useSavedShowBatch(showIds, isAuthenticated)
  const { data: batchAttendance } = useBatchAttendance(showIds)

  const cities: CityWithCount[] = useMemo(
    () =>
      citiesData?.cities?.map(c => ({
        city: c.city,
        state: c.state,
        count: c.show_count,
      })) ?? [],
    [citiesData?.cities]
  )

  // Determine if "Save as default" / "Clear defaults" should show
  const selectionDiffersFromFavorites = !citiesEqual(selectedCities, favoriteCities)

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
      {cities.length > 1 && (
        <div className="mb-6">
          <CityFilters
            cities={cities}
            selectedCities={selectedCities}
            onFilterChange={handleFilterChange}
          >
            {isAuthenticated && selectionDiffersFromFavorites && (
              <SaveDefaultsButton
                selectedCities={selectedCities}
                favoriteCities={favoriteCities}
              />
            )}
          </CityFilters>
        </div>
      )}

      <div className={isFetching ? 'opacity-60 transition-opacity duration-75' : 'transition-opacity duration-75'}>
        {!data?.shows || data.shows.length === 0 ? (
          <div className="text-center py-8 text-muted-foreground">
            <p>
              {selectedCities.length > 0
                ? `No upcoming shows in ${selectedCities.map(c => c.city).join(', ')}.`
                : 'No upcoming shows at this time.'}
            </p>
          </div>
        ) : (
          data.shows.map(show => (
            <ShowCard
              key={show.id}
              show={show}
              isAdmin={isAdmin}
              isSaved={savedShowIds?.has(show.id)}
              attendanceData={batchAttendance?.[String(show.id)]}
            />
          ))
        )}
      </div>
    </div>
  )
}
