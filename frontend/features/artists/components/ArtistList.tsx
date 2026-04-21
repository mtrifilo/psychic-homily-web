'use client'

import { useCallback, useMemo, useTransition } from 'react'
import { useSearchParams, useRouter } from 'next/navigation'
import { useArtists, useArtistCities } from '../hooks/useArtists'
import { ArtistCard } from './ArtistCard'
import { ArtistSearch } from './ArtistSearch'
import { CityFilters, type CityWithCount, type CityState } from '@/components/filters'
import { LoadingSpinner, DensityToggle } from '@/components/shared'
import { useDensity } from '@/lib/hooks/common/useDensity'
import { Button } from '@/components/ui/button'
import {
  TagFacetPanel,
  TagFacetSheet,
  parseTagsParam,
  buildTagsParam,
} from '@/features/tags'

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
  const { density, setDensity } = useDensity('artists')

  // PSY-496: city filter is page-scoped — we don't auto-apply the user's
  // profile-level favorite_cities here. Favorites are shows-centric (the
  // canonical homepage) and inheriting them on /artists produced the
  // "0 artists" confusion where most artists have city: null. Users can
  // still filter by city on /artists manually — the URL drives state.

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

  const { data: citiesData, isLoading: citiesLoading, isFetching: citiesFetching } = useArtistCities()
  const { data, isLoading, isFetching, error, refetch } = useArtists({
    cities: selectedCities.length > 0 ? selectedCities : undefined,
    tags: selectedTags.length > 0 ? selectedTags : undefined,
    tagMatch,
  })

  const writeParams = useCallback(
    (
      nextCities: CityState[] = selectedCities,
      nextTags: string[] = selectedTags,
      nextMatch: 'all' | 'any' = tagMatch
    ) => {
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
        router.push(queryString ? `/artists?${queryString}` : '/artists', {
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
    (nextTags: string[]) => {
      writeParams(selectedCities, nextTags, tagMatch)
    },
    [selectedCities, tagMatch, writeParams]
  )

  const handleTagsClear = useCallback(() => {
    writeParams(selectedCities, [], tagMatch)
  }, [selectedCities, tagMatch, writeParams])

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
  const hasTagFilter = selectedTags.length > 0
  const hasAnyFilter = hasTagFilter || selectedCities.length > 0

  return (
    <section className="w-full max-w-6xl">
      <div className="mb-6 space-y-4">
        <ArtistSearch />
        {cities.length > 0 && (
          <CityFilters
            cities={cities}
            selectedCities={selectedCities}
            onFilterChange={handleFilterChange}
          />
        )}
      </div>

      <div className="flex items-center justify-between mb-4 gap-2">
        <TagFacetSheet
          selectedSlugs={selectedTags}
          onToggle={handleTagsChange}
          onClear={handleTagsClear}
          title="Filter artists by tag"
          entityType="artist"
        />
        <DensityToggle density={density} onDensityChange={setDensity} />
      </div>

      <div className="flex flex-col gap-6 lg:flex-row">
        <aside className="hidden lg:block lg:w-64 lg:shrink-0">
          <TagFacetPanel
            selectedSlugs={selectedTags}
            onToggle={handleTagsChange}
            onClear={handleTagsClear}
            heading="Filter artists by tag"
            entityType="artist"
          />
        </aside>

        <div className={`flex-1 min-w-0 ${isUpdating ? 'opacity-60 transition-opacity duration-75' : 'transition-opacity duration-75'}`}>
          <p className="mb-3 text-sm text-muted-foreground" data-testid="artist-count">
            {artists.length} {artists.length === 1 ? 'artist' : 'artists'}
            {hasTagFilter && ` matching ${selectedTags.join(', ')}`}
          </p>
          {artists.length === 0 ? (
            <div className="text-center py-12 text-muted-foreground">
              <p>
                {hasAnyFilter
                  ? 'No artists match the current filters.'
                  : 'No artists available at this time.'}
              </p>
              {hasAnyFilter && (
                <button
                  onClick={() => {
                    if (hasTagFilter) handleTagsClear()
                    if (selectedCities.length > 0) handleFilterChange([])
                  }}
                  className="mt-4 text-primary hover:underline"
                >
                  Clear filters
                </button>
              )}
            </div>
          ) : (
            <div className="@container">
              <div className={
                density === 'compact'
                  ? 'flex flex-col gap-px'
                  : density === 'expanded'
                    ? 'grid grid-cols-1 gap-5'
                    : 'grid grid-cols-1 @sm:grid-cols-2 @2xl:grid-cols-3 gap-3'
              }>
                {artists.map(artist => (
                  <ArtistCard key={artist.id} artist={artist} density={density} />
                ))}
              </div>
            </div>
          )}
        </div>
      </div>
    </section>
  )
}
