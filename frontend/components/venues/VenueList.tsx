'use client'

import { useTransition } from 'react'
import { useSearchParams, useRouter } from 'next/navigation'
import { useVenues, useVenueCities } from '@/lib/hooks/useVenues'
import { VenueCard } from './VenueCard'
import { CityFilters, type CityWithCount } from '@/components/filters'
import { LoadingSpinner } from '@/components/shared'

export function VenueList() {
  const router = useRouter()
  const searchParams = useSearchParams()
  const [isPending, startTransition] = useTransition()

  const selectedCity = searchParams.get('city')
  const selectedState = searchParams.get('state')

  const { data: citiesData, isLoading: citiesLoading, isFetching: citiesFetching } = useVenueCities()
  const { data, isLoading, isFetching, error } = useVenues({
    city: selectedCity || undefined,
    state: selectedState || undefined,
  })

  const handleFilterChange = (city: string | null, state: string | null) => {
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
        {!data?.venues || data.venues.length === 0 ? (
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
        ) : (
          <>
            {data.venues.map(venue => (
              <VenueCard key={venue.id} venue={venue} />
            ))}

            {data.total > data.venues.length && (
              <div className="text-center py-6">
                <p className="text-muted-foreground text-sm">
                  Showing {data.venues.length} of {data.total} venues
                </p>
              </div>
            )}
          </>
        )}
      </div>
    </section>
  )
}
