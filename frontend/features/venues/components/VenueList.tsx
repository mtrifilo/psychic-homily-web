'use client'

import { useState, useCallback, useMemo, useTransition, useRef, useEffect } from 'react'
import { useSearchParams, useRouter } from 'next/navigation'
import { useVenues, useVenueCities } from '../hooks/useVenues'
import { useProfile, useIsAuthenticated } from '@/features/auth'
import type { VenueWithShowCount } from '../types'
import { VenueCard } from './VenueCard'
import { VenueSearch } from './VenueSearch'
import { CityFilters, type CityWithCount, type CityState } from '@/components/filters'
import { SaveDefaultsButton } from '@/components/filters/SaveDefaultsButton'
import { LoadingSpinner } from '@/components/shared'
import { Button } from '@/components/ui/button'
import {
  TagFacetPanel,
  TagFacetSheet,
  parseTagsParam,
  buildTagsParam,
} from '@/features/tags'

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

/** Compare two city arrays for equality (order-insensitive) */
function citiesEqual(a: CityState[], b: CityState[]): boolean {
  if (a.length !== b.length) return false
  const setA = new Set(a.map(c => `${c.city}|${c.state}`))
  return b.every(c => setA.has(`${c.city}|${c.state}`))
}

export function VenueList() {
  const router = useRouter()
  const searchParams = useSearchParams()
  const { isAuthenticated } = useIsAuthenticated()
  const [isPending, startTransition] = useTransition()
  const { data: profileData } = useProfile()
  const hasAppliedDefaults = useRef(false)
  const [offset, setOffset] = useState(0)
  const [accumulatedVenues, setAccumulatedVenues] = useState<VenueWithShowCount[]>([])

  // Parse multi-city from URL
  const citiesParam = searchParams.get('cities')
  const selectedCities: CityState[] = useMemo(() => {
    return parseCitiesParam(citiesParam)
  }, [citiesParam])

  // Parse multi-tag from URL (PSY-309)
  const tagsParam = searchParams.get('tags')
  const tagMatchParam = searchParams.get('tag_match')
  const selectedTags = useMemo(() => parseTagsParam(tagsParam), [tagsParam])
  const tagMatch: 'all' | 'any' = tagMatchParam === 'any' ? 'any' : 'all'

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
      !citiesParam
    ) {
      hasAppliedDefaults.current = true
      const params = new URLSearchParams()
      params.set('cities', buildCitiesParam(favoriteCities))
      startTransition(() => {
        router.replace(`/venues?${params.toString()}`, { scroll: false })
      })
    }
  }, [favoriteCities, citiesParam, router])

  const { data: citiesData, isLoading: citiesLoading, isFetching: citiesFetching } = useVenueCities()
  const { data, isLoading, isFetching, error, refetch } = useVenues({
    cities: selectedCities.length > 0 ? selectedCities : undefined,
    tags: selectedTags.length > 0 ? selectedTags : undefined,
    tagMatch,
    limit: VENUES_PER_PAGE,
    offset,
  })

  const handleLoadMore = useCallback(() => {
    if (data) {
      setAccumulatedVenues(prev => [...prev, ...data.venues])
      setOffset(prev => prev + VENUES_PER_PAGE)
    }
  }, [data])

  const writeParams = useCallback(
    (
      nextCities: CityState[] = selectedCities,
      nextTags: string[] = selectedTags,
      nextMatch: 'all' | 'any' = tagMatch
    ) => {
      setOffset(0)
      setAccumulatedVenues([])
      const params = new URLSearchParams()
      if (nextCities.length > 0) {
        params.set('cities', buildCitiesParam(nextCities))
      }
      if (nextTags.length > 0) {
        params.set('tags', buildTagsParam(nextTags))
        if (nextMatch === 'any') params.set('tag_match', 'any')
      }
      const queryString = params.toString()
      startTransition(() => {
        router.push(queryString ? `/venues?${queryString}` : '/venues', {
          scroll: false,
        })
      })
    },
    [selectedCities, selectedTags, tagMatch, router]
  )

  const handleFilterChange = (cities: CityState[]) => {
    writeParams(cities, selectedTags, tagMatch)
  }

  const handleTagsChange = useCallback(
    (nextTags: string[]) => writeParams(selectedCities, nextTags, tagMatch),
    [selectedCities, tagMatch, writeParams]
  )

  const handleTagsClear = useCallback(
    () => writeParams(selectedCities, [], tagMatch),
    [selectedCities, tagMatch, writeParams]
  )

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

  // Determine if "Save as default" / "Clear defaults" should show
  const selectionDiffersFromFavorites = !citiesEqual(selectedCities, favoriteCities)

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
          >
            {isAuthenticated && selectionDiffersFromFavorites && (
              <SaveDefaultsButton
                selectedCities={selectedCities}
                favoriteCities={favoriteCities}
              />
            )}
          </CityFilters>
        )}
      </div>

      <div className="flex items-center justify-between mb-4 gap-2">
        <TagFacetSheet
          selectedSlugs={selectedTags}
          onToggle={handleTagsChange}
          onClear={handleTagsClear}
          title="Filter venues by tag"
        />
      </div>

      <div className="flex flex-col gap-6 lg:flex-row">
        <aside className="hidden lg:block lg:w-64 lg:shrink-0">
          <TagFacetPanel
            selectedSlugs={selectedTags}
            onToggle={handleTagsChange}
            onClear={handleTagsClear}
            heading="Filter venues by tag"
          />
        </aside>

        <div className={`flex-1 min-w-0 ${isUpdating ? 'opacity-60 transition-opacity duration-75' : 'transition-opacity duration-75'}`}>
          {(() => {
            const allVenues = [...accumulatedVenues, ...(data?.venues || [])]
            const hasAnyFilter = selectedTags.length > 0 || selectedCities.length > 0
            return (
              <>
                <p className="mb-3 text-sm text-muted-foreground" data-testid="venue-count">
                  {allVenues.length} of {data?.total ?? allVenues.length}{' '}
                  {(data?.total ?? allVenues.length) === 1 ? 'venue' : 'venues'}
                  {selectedTags.length > 0 && ` matching ${selectedTags.join(', ')}`}
                </p>
                {allVenues.length === 0 ? (
                  <div className="text-center py-12 text-muted-foreground">
                    <p>
                      {hasAnyFilter
                        ? 'No venues match the current filters.'
                        : 'No venues available at this time.'}
                    </p>
                    {hasAnyFilter && (
                      <button
                        onClick={() => {
                          if (selectedTags.length > 0) handleTagsClear()
                          if (selectedCities.length > 0) handleFilterChange([])
                        }}
                        className="mt-4 text-primary hover:underline"
                      >
                        Clear filters
                      </button>
                    )}
                  </div>
                ) : (
                  <>
                    {allVenues.map(venue => (
                      <VenueCard key={venue.id} venue={venue} />
                    ))}

                    {data && allVenues.length < data.total && (
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
              </>
            )
          })()}
        </div>
      </div>
    </section>
  )
}
