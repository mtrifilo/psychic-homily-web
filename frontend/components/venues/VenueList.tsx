'use client'

import { useState, useCallback, useMemo, useTransition } from 'react'
import { useSearchParams, useRouter } from 'next/navigation'
import { useVenues, useVenueCities } from '@/lib/hooks/venues/useVenues'
import type { VenueWithShowCount } from '@/lib/types/venue'
import { VenueCard } from './VenueCard'
import { VenueSearch } from './VenueSearch'
import { CityFilters, type CityWithCount, type CityState } from '@/components/filters'
import { LoadingSpinner } from '@/components/shared'
import { Button } from '@/components/ui/button'

const VENUES_PER_PAGE = 50

/** Parse cities param from URL: "Phoenix,AZ|Tucson,AZ" -> CityState[] */
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

/** Build cities param for URL: CityState[] -> "Phoenix,AZ|Tucson,AZ" */
function buildCitiesParam(cities: CityState[]): string {
  return cities.map(c => `${c.city},${c.state}`).join('|')
}

export function VenueList() {
  const router = useRouter()
  const searchParams = useSearchParams()
  const [isPending, startTransition] = useTransition()
  const [offset, setOffset] = useState(0)
  const [accumulatedVenues, setAccumulatedVenues] = useState<VenueWithShowCount[]>([])

  // Parse multi-city from URL
  const citiesParam = searchParams.get('cities')
  const selectedCities: CityState[] = useMemo(() => {
    return parseCitiesParam(citiesParam)
  }, [citiesParam])

  const { data: citiesData, isLoading: citiesLoading, isFetching: citiesFetching } = useVenueCities()
  const { data, isLoading, isFetching, error, refetch } = useVenues({
    cities: selectedCities.length > 0 ? selectedCities : undefined,
    limit: VENUES_PER_PAGE,
    offset,
  })

  const handleLoadMore = useCallback(() => {
    if (data) {
      setAccumulatedVenues(prev => [...prev, ...data.venues])
      setOffset(prev => prev + VENUES_PER_PAGE)
    }
  }, [data])

  const handleFilterChange = (cities: CityState[]) => {
    // Reset pagination on filter change
    setOffset(0)
    setAccumulatedVenues([])

    const params = new URLSearchParams()
    if (cities.length > 0) {
      params.set('cities', buildCitiesParam(cities))
    }

    const queryString = params.toString()
    startTransition(() => {
      router.push(queryString ? `/venues?${queryString}` : '/venues')
    })
  }

  // Only show full spinner on FIRST load (no data yet)
  if ((isLoading && !data) || (citiesLoading && !citiesData)) {
    return (
      <div className="flex justify-center items-center py-12">
        <LoadingSpinner />
      </div>
    )
  }

  // Track if we're updating (fetching but already have data)
  const isUpdating = isFetching || citiesFetching || isPending

  if (error) {
    return (
      <div className="text-center py-12 text-destructive">
        <p>Failed to load venues. Please try again later.</p>
        <Button variant="outline" className="mt-4" onClick={() => refetch()}>
          Retry
        </Button>
      </div>
    )
  }

  // Map VenueCity to CityWithCount
  const cities: CityWithCount[] = citiesData?.cities?.map(c => ({
    city: c.city,
    state: c.state,
    count: c.venue_count,
  })) ?? []

  return (
    <section className="w-full max-w-6xl">
      <div className="mb-6 space-y-4">
        <VenueSearch />
        {cities.length > 0 && (
          <CityFilters
            cities={cities}
            selectedCities={selectedCities}
            onFilterChange={handleFilterChange}
          />
        )}
      </div>

      {/* Dim content while fetching, don't hide it */}
      <div className={isUpdating ? 'opacity-60 transition-opacity duration-75' : 'transition-opacity duration-75'}>
        {(() => {
          const allVenues = [...accumulatedVenues, ...(data?.venues || [])]
          if (allVenues.length === 0) {
            return (
              <div className="text-center py-12 text-muted-foreground">
                <p>
                  {selectedCities.length > 0
                    ? 'No venues found in the selected cities.'
                    : 'No venues available at this time.'}
                </p>
                {selectedCities.length > 0 && (
                  <button
                    onClick={() => handleFilterChange([])}
                    className="mt-4 text-primary hover:underline"
                  >
                    View all venues
                  </button>
                )}
              </div>
            )
          }
          const hasMore = data ? allVenues.length < data.total : false
          return (
            <>
              {allVenues.map(venue => (
                <VenueCard key={venue.id} venue={venue} />
              ))}

              {hasMore && (
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
          )
        })()}
      </div>
    </section>
  )
}
