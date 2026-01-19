'use client'

import { useSearchParams, useRouter } from 'next/navigation'
import { useVenues, useVenueCities } from '@/lib/hooks/useVenues'
import { VenueCard } from './VenueCard'
import type { VenueCity } from '@/lib/types/venue'

interface FilterChipProps {
  label: string
  isActive: boolean
  onClick: () => void
  count?: number
}

function FilterChip({ label, isActive, onClick, count }: FilterChipProps) {
  return (
    <button
      onClick={onClick}
      className={`px-3 py-1.5 rounded-full text-sm font-medium transition-colors ${
        isActive
          ? 'bg-primary text-primary-foreground'
          : 'bg-muted hover:bg-muted/80 text-muted-foreground hover:text-foreground'
      }`}
    >
      {label}
      {count !== undefined && (
        <span className={`ml-1.5 ${isActive ? 'opacity-80' : 'opacity-60'}`}>
          ({count})
        </span>
      )}
    </button>
  )
}

interface CityFiltersProps {
  cities: VenueCity[]
  selectedCity: string | null
  selectedState: string | null
  onFilterChange: (city: string | null, state: string | null) => void
}

function CityFilters({
  cities,
  selectedCity,
  selectedState,
  onFilterChange,
}: CityFiltersProps) {
  const isAllSelected = !selectedCity && !selectedState

  return (
    <div className="flex flex-wrap gap-2 mb-6">
      <FilterChip
        label="All Cities"
        isActive={isAllSelected}
        onClick={() => onFilterChange(null, null)}
      />
      {cities.map(city => {
        const isActive =
          selectedCity === city.city && selectedState === city.state
        const label =
          cities.filter(c => c.city === city.city).length > 1
            ? `${city.city}, ${city.state}`
            : city.city

        return (
          <FilterChip
            key={`${city.city}-${city.state}`}
            label={label}
            isActive={isActive}
            onClick={() => onFilterChange(city.city, city.state)}
            count={city.venue_count}
          />
        )
      })}
    </div>
  )
}

export function VenueList() {
  const router = useRouter()
  const searchParams = useSearchParams()

  const selectedCity = searchParams.get('city')
  const selectedState = searchParams.get('state')

  const { data: citiesData, isLoading: citiesLoading } = useVenueCities()
  const { data, isLoading, error } = useVenues({
    city: selectedCity || undefined,
    state: selectedState || undefined,
  })

  const handleFilterChange = (city: string | null, state: string | null) => {
    const params = new URLSearchParams()
    if (city) params.set('city', city)
    if (state) params.set('state', state)

    const queryString = params.toString()
    router.push(queryString ? `/venues?${queryString}` : '/venues')
  }

  if (isLoading || citiesLoading) {
    return (
      <div className="flex justify-center items-center py-12">
        <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-foreground"></div>
      </div>
    )
  }

  if (error) {
    return (
      <div className="text-center py-12 text-destructive">
        <p>Failed to load venues. Please try again later.</p>
      </div>
    )
  }

  return (
    <section className="w-full max-w-4xl">
      {citiesData?.cities && citiesData.cities.length > 1 && (
        <CityFilters
          cities={citiesData.cities}
          selectedCity={selectedCity}
          selectedState={selectedState}
          onFilterChange={handleFilterChange}
        />
      )}

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
    </section>
  )
}
