'use client'

import { useMemo, useTransition } from 'react'
import { useSearchParams, useRouter } from 'next/navigation'
import { useArtists, useArtistCities } from '@/lib/hooks/useArtists'
import { ArtistCard } from './ArtistCard'
import { CityFilters, type CityWithCount, type CityState } from '@/components/filters'
import { LoadingSpinner } from '@/components/shared'
import { Button } from '@/components/ui/button'

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

export function ArtistList() {
  const router = useRouter()
  const searchParams = useSearchParams()
  const [isPending, startTransition] = useTransition()

  // Parse multi-city from URL
  const citiesParam = searchParams.get('cities')
  const selectedCities: CityState[] = useMemo(() => {
    return parseCitiesParam(citiesParam)
  }, [citiesParam])

  const { data: citiesData, isLoading: citiesLoading, isFetching: citiesFetching } = useArtistCities()
  const { data, isLoading, isFetching, error, refetch } = useArtists({
    cities: selectedCities.length > 0 ? selectedCities : undefined,
  })

  const handleFilterChange = (cities: CityState[]) => {
    const params = new URLSearchParams()
    if (cities.length > 0) {
      params.set('cities', buildCitiesParam(cities))
    }
    const queryString = params.toString()
    startTransition(() => {
      router.push(queryString ? `/artists?${queryString}` : '/artists')
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
        <p>Failed to load artists. Please try again later.</p>
        <Button variant="outline" className="mt-4" onClick={() => refetch()}>
          Retry
        </Button>
      </div>
    )
  }

  // Map ArtistCity to CityWithCount
  const cities: CityWithCount[] = citiesData?.cities?.map(c => ({
    city: c.city,
    state: c.state,
    count: c.artist_count,
  })) ?? []

  const artists = data?.artists ?? []

  return (
    <section className="w-full max-w-4xl">
      {cities.length > 1 && (
        <CityFilters
          cities={cities}
          selectedCities={selectedCities}
          onFilterChange={handleFilterChange}
        />
      )}

      {/* Dim content while fetching, don't hide it */}
      <div className={isUpdating ? 'opacity-60 transition-opacity duration-75' : 'transition-opacity duration-75'}>
        {artists.length === 0 ? (
          <div className="text-center py-12 text-muted-foreground">
            <p>
              {selectedCities.length > 0
                ? 'No artists found in the selected cities.'
                : 'No artists available at this time.'}
            </p>
            {selectedCities.length > 0 && (
              <button
                onClick={() => handleFilterChange([])}
                className="mt-4 text-primary hover:underline"
              >
                View all artists
              </button>
            )}
          </div>
        ) : (
          artists.map(artist => (
            <ArtistCard key={artist.id} artist={artist} />
          ))
        )}
      </div>
    </section>
  )
}
