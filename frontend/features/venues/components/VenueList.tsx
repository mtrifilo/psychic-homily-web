'use client'

import { useState, useCallback, useMemo, useTransition } from 'react'
import { useSearchParams, useRouter } from 'next/navigation'
import { useQueryState } from 'nuqs'
import { useVenues, useVenueCities } from '../hooks/useVenues'
import type { VenueWithShowCount } from '../types'
import { VenueCard } from './VenueCard'
import { VenueSearch } from './VenueSearch'
import { CityFilters, type CityWithCount, type CityState } from '@/components/filters'
import { citiesParser, ALL_CITIES } from '@/components/filters/cityParams'
import { LoadingSpinner } from '@/components/shared'
import { Button } from '@/components/ui/button'
import {
  TagFacetPanel,
  TagFacetSheet,
  parseTagsParam,
  buildTagsParam,
} from '@/features/tags'

const VENUES_PER_PAGE = 50

export function VenueList() {
  const router = useRouter()
  const searchParams = useSearchParams()
  const [isPending, startTransition] = useTransition()
  const [offset, setOffset] = useState(0)
  const [accumulatedVenues, setAccumulatedVenues] = useState<VenueWithShowCount[]>([])

  // PSY-496: city filter is page-scoped — we don't auto-apply the user's
  // profile-level favorite_cities here. Favorites are shows-centric (the
  // canonical homepage). Users can still filter by city on /venues manually —
  // the URL drives state.

  // `?cities=` via nuqs (shared parser). /venues has NO derived default —
  // absent (null) simply means "all venues" (PSY-496 above) — so unlike
  // /shows, clearing writes a bare URL, not the ALL_CITIES sentinel. The
  // sentinel is still tolerated on deep links (?cities=all → []).
  const [citiesState, setCities] = useQueryState(
    'cities',
    citiesParser.withOptions({ history: 'push', startTransition })
  )
  const selectedCities: CityState[] = useMemo(() => {
    if (citiesState === ALL_CITIES || citiesState === null) return []
    return citiesState
  }, [citiesState])

  // Parse multi-tag from URL (PSY-309)
  const tagsParam = searchParams.get('tags')
  const tagMatchParam = searchParams.get('tag_match')
  const selectedTags = useMemo(() => parseTagsParam(tagsParam), [tagsParam])
  const tagMatch: 'all' | 'any' = tagMatchParam === 'any' ? 'any' : 'all'

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

  // City changes write `?cities=` via nuqs (empty → null → bare URL; no
  // default derivation on /venues, see PSY-496 note above).
  const handleFilterChange = useCallback(
    (cities: CityState[]) => {
      setOffset(0)
      setAccumulatedVenues([])
      void setCities(cities.length > 0 ? cities : null)
    },
    [setCities]
  )

  // Tag changes rewrite only the tag params, preserving `?cities=` verbatim.
  const writeTags = useCallback(
    (nextTags: string[], nextMatch: 'all' | 'any') => {
      setOffset(0)
      setAccumulatedVenues([])
      const params = new URLSearchParams(searchParams.toString())
      params.delete('tags')
      params.delete('tag_match')
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
    [searchParams, router]
  )

  const handleTagsChange = useCallback(
    (nextTags: string[]) => writeTags(nextTags, tagMatch),
    [tagMatch, writeTags]
  )

  const handleTagsClear = useCallback(
    () => writeTags([], tagMatch),
    [tagMatch, writeTags]
  )

  // "Clear filters" resets tags AND cities in a SINGLE navigation — mixing a
  // router push (tags) with nuqs's throttled setCities in one tick races
  // (nuqs aborts its queue on a foreign history update; see PSY-1388).
  const handleClearFilters = useCallback(() => {
    setOffset(0)
    setAccumulatedVenues([])
    startTransition(() => {
      router.push('/venues', { scroll: false })
    })
  }, [router])

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

      {/* Mobile: Sheet trigger row. Desktop hides the Sheet (the bar below
          takes over). */}
      <div className="flex items-center justify-between mb-4 gap-2">
        <TagFacetSheet
          selectedSlugs={selectedTags}
          onToggle={handleTagsChange}
          onClear={handleTagsClear}
          title="Filter venues by tag"
          entityType="venue"
        />
      </div>

      {/* PSY-1003: full-width top-bar tag filter above a full-width list (no
          left rail). Desktop only — mobile uses the Sheet trigger above. */}
      <div className="mb-4 hidden lg:block">
        <TagFacetPanel
          selectedSlugs={selectedTags}
          onToggle={handleTagsChange}
          onClear={handleTagsClear}
          heading="Filter venues by tag"
          entityType="venue"
          layout="bar"
        />
      </div>

      <div className={`min-w-0 ${isUpdating ? 'opacity-60 transition-opacity duration-75' : 'transition-opacity duration-75'}`}>
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
                      onClick={handleClearFilters}
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
    </section>
  )
}
