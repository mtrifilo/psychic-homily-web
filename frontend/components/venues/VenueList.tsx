'use client'

import { useState, useCallback, useTransition } from 'react'
import { useSearchParams, useRouter } from 'next/navigation'
import { useVenues, useVenueCities } from '@/lib/hooks/useVenues'
import type { VenueWithShowCount } from '@/lib/types/venue'
import { VenueCard } from './VenueCard'
import { CityFilters, type CityWithCount } from '@/components/filters'
import { LoadingSpinner } from '@/components/shared'
import { Button } from '@/components/ui/button'

const VENUES_PER_PAGE = 50

export function VenueList() {
  const router = useRouter()
  const searchParams = useSearchParams()
  const [isPending, startTransition] = useTransition()
  const [offset, setOffset] = useState(0)
  const [accumulatedVenues, setAccumulatedVenues] = useState<VenueWithShowCount[]>([])

  const selectedCity = searchParams.get('city')
  const selectedState = searchParams.get('state')

  const { data: citiesData, isLoading: citiesLoading, isFetching: citiesFetching } = useVenueCities()
  const { data, isLoading, isFetching, error, refetch } = useVenues({
    city: selectedCity || undefined,
    state: selectedState || undefined,
    limit: VENUES_PER_PAGE,
    offset,
  })

  const handleLoadMore = useCallback(() => {
    if (data) {
      setAccumulatedVenues(prev => [...prev, ...data.venues])
      setOffset(prev => prev + VENUES_PER_PAGE)
    }
  }, [data])

  const handleFilterChange = (city: string | null, state: string | null) => {
    // Reset pagination on filter change
    setOffset(0)
    setAccumulatedVenues([])

    const params = new URLSearchParams()
    if (city) params.set('city', city)
    if (state) params.set('state', state)

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
    <section className="w-full max-w-4xl">
      {cities.length > 1 && (
        <CityFilters
          cities={cities}
          selectedCity={selectedCity}
          selectedState={selectedState}
          onFilterChange={handleFilterChange}
        />
      )}

      {/* Dim content while fetching, don't hide it */}
      <div className={isUpdating ? 'opacity-60 transition-opacity duration-75' : 'transition-opacity duration-75'}>
        {(() => {
          const allVenues = [...accumulatedVenues, ...(data?.venues || [])]
          if (allVenues.length === 0) {
            return (
              <div className="text-center py-12 text-muted-foreground">
                <p>
                  {selectedCity
                    ? `No venues found in ${selectedCity}.`
                    : 'No venues available at this time.'}
                </p>
                {selectedCity && (
                  <button
                    onClick={() => handleFilterChange(null, null)}
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
