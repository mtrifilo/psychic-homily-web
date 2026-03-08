'use client'

import { useState, useCallback, useMemo, useTransition, useRef, useEffect } from 'react'
import { useSearchParams, useRouter } from 'next/navigation'
import { useUpcomingShows, useShowCities } from '@/lib/hooks/useShows'
import { useSavedShowBatch } from '@/lib/hooks/useSavedShows'
import { useAuthContext } from '@/lib/context/AuthContext'
import { useProfile } from '@/lib/hooks/useAuth'
import { useSetFavoriteCities } from '@/lib/hooks/useFavoriteCities'
import type { ShowResponse } from '@/lib/types/show'
import type { CityState } from '@/components/filters'
import { Button } from '@/components/ui/button'
import { DensityToggle } from '@/components/shared'
import { useDensity } from '@/lib/hooks/useDensity'
import { ShowCard } from './ShowCard'
import { ShowListSkeleton } from './ShowListSkeleton'
import { CityFilters, type CityWithCount } from '@/components/filters'
import { SaveDefaultsButton } from '@/components/filters/SaveDefaultsButton'

/** Parse cities param from URL: "Phoenix,AZ|Mesa,AZ" -> CityState[] */
function parseCitiesParam(param: string | null): CityState[] {
  if (!param) return []
  return param
    .split('|')
    .map(pair => {
      const [city, state] = pair.split(',')
      return city && state ? { city: city.trim(), state: state.trim() } : null
    })
    .filter((c): c is CityState => c !== null)
}

/** Build cities param for URL: CityState[] -> "Phoenix,AZ|Mesa,AZ" */
function buildCitiesParam(cities: CityState[]): string {
  return cities.map(c => `${c.city},${c.state}`).join('|')
}

/** Compare two city arrays for equality (order-insensitive) */
function citiesEqual(a: CityState[], b: CityState[]): boolean {
  if (a.length !== b.length) return false
  const setA = new Set(a.map(c => `${c.city}|${c.state}`))
  return b.every(c => setA.has(`${c.city}|${c.state}`))
}

export function ShowList() {
  const router = useRouter()
  const searchParams = useSearchParams()
  const { user, isAuthenticated } = useAuthContext()
  const isAdmin = user?.is_admin ?? false
  const [isPending, startTransition] = useTransition()
  const { data: profileData } = useProfile()
  const hasAppliedDefaults = useRef(false)
  const { density } = useDensity('shows')

  // Parse multi-city or legacy single-city from URL
  const citiesParam = searchParams.get('cities')
  const legacyCity = searchParams.get('city')
  const legacyState = searchParams.get('state')

  const selectedCities: CityState[] = useMemo(() => {
    if (citiesParam) return parseCitiesParam(citiesParam)
    if (legacyCity && legacyState) return [{ city: legacyCity, state: legacyState }]
    return []
  }, [citiesParam, legacyCity, legacyState])

  // Read favorites from profile
  const favoriteCities: CityState[] = useMemo(() => {
    const prefs = profileData?.user?.preferences
    if (!prefs?.favorite_cities) return []
    return prefs.favorite_cities
  }, [profileData?.user?.preferences])

  // Apply favorites as default URL params on initial load (no URL params + not yet applied)
  useEffect(() => {
    if (
      !hasAppliedDefaults.current &&
      favoriteCities.length > 0 &&
      !citiesParam &&
      !legacyCity &&
      !legacyState
    ) {
      hasAppliedDefaults.current = true
      const params = new URLSearchParams()
      params.set('cities', buildCitiesParam(favoriteCities))
      startTransition(() => {
        router.replace(`/shows?${params.toString()}`, { scroll: false })
      })
    }
  }, [favoriteCities, citiesParam, legacyCity, legacyState, router])

  const timezone = Intl.DateTimeFormat().resolvedOptions().timeZone
  const [cursor, setCursor] = useState<string | undefined>(undefined)
  const [accumulatedShows, setAccumulatedShows] = useState<ShowResponse[]>([])

  const {
    data: citiesData,
    isLoading: citiesLoading,
    isFetching: citiesFetching,
  } = useShowCities({
    timezone,
  })

  const { data, isLoading, isFetching, error, refetch } = useUpcomingShows({
    timezone,
    cursor,
    cities: selectedCities.length > 0 ? selectedCities : undefined,
  })

  // Batch-check saved status for all visible shows (1 request instead of N)
  const allShows = useMemo(
    () => [...accumulatedShows, ...(data?.shows || [])],
    [accumulatedShows, data?.shows]
  )
  const allShowIds = useMemo(() => allShows.map(s => s.id), [allShows])
  const { data: savedShowIds } = useSavedShowBatch(allShowIds, isAuthenticated)

  const handleLoadMore = useCallback(() => {
    if (data?.pagination.next_cursor) {
      // Accumulate current shows before loading next page
      const currentShows = data.shows || []
      setAccumulatedShows(prev => [...prev, ...currentShows])
      setCursor(data.pagination.next_cursor!)
    }
  }, [data])

  const handleFilterChange = (cities: CityState[]) => {
    // Reset pagination on filter change
    setCursor(undefined)
    setAccumulatedShows([])
    const params = new URLSearchParams()
    if (cities.length > 0) {
      params.set('cities', buildCitiesParam(cities))
    }

    const queryString = params.toString()
    startTransition(() => {
      router.push(queryString ? `/shows?${queryString}` : '/shows')
    })
  }

  // Determine if "Save as default" / "Clear defaults" should show
  const selectionDiffersFromFavorites = !citiesEqual(selectedCities, favoriteCities)

  // Only show skeleton on FIRST load (no data yet)
  if ((isLoading && !data) || (citiesLoading && !citiesData)) {
    return <ShowListSkeleton />
  }

  // Track if we're updating (fetching but already have data)
  const isUpdating = isFetching || citiesFetching || isPending

  if (error) {
    return (
      <div className="text-center py-12 text-destructive">
        <p>Failed to load shows. Please try again later.</p>
        <Button variant="outline" className="mt-4" onClick={() => refetch()}>
          Retry
        </Button>
      </div>
    )
  }

  // Map ShowCity to CityWithCount
  const cities: CityWithCount[] =
    citiesData?.cities?.map(c => ({
      city: c.city,
      state: c.state,
      count: c.show_count,
    })) ?? []

  return (
    <section className="w-full max-w-6xl">
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

      <div className="flex justify-end mb-4">
        <DensityToggle storageKey="shows" />
      </div>

      {/* Dim content while fetching, don't hide it */}
      <div
        className={
          isUpdating
            ? 'opacity-60 transition-opacity duration-75'
            : 'transition-opacity duration-75'
        }
      >
        {allShows.length === 0 ? (
          <div className="text-center py-12 text-muted-foreground">
            <p>
              {selectedCities.length > 0
                ? `No upcoming shows in ${selectedCities.map(c => c.city).join(', ')}.`
                : 'No upcoming shows at this time.'}
            </p>
            {selectedCities.length > 0 && (
              <button
                onClick={() => handleFilterChange([])}
                className="mt-4 text-primary hover:underline"
              >
                View all shows
              </button>
            )}
          </div>
        ) : (
          <>
            {allShows.map(show => (
              <ShowCard
                key={show.id}
                show={show}
                isAdmin={isAdmin}
                userId={user?.id}
                isSaved={savedShowIds?.has(show.id)}
                density={density}
              />
            ))}

            {data?.pagination.has_more && (
              <div className="text-center py-6">
                <Button
                  variant="outline"
                  onClick={handleLoadMore}
                  disabled={isFetching}
                >
                  {isFetching ? 'Loading...' : 'Load More'}
                </Button>
              </div>
            )}
          </>
        )}
      </div>
    </section>
  )
}
