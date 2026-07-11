'use client'

import { useCallback, useMemo, useTransition } from 'react'
import { useSearchParams, useRouter } from 'next/navigation'
import { useQueryState } from 'nuqs'
import { useArtists, useArtistCities } from '../hooks/useArtists'
import { ArtistCard } from './ArtistCard'
import { ArtistSearch } from './ArtistSearch'
import { CityFilters, type CityWithCount, type CityState } from '@/components/filters'
import { citiesParser, ALL_CITIES } from '@/components/filters/cityParams'
import { LoadingSpinner, DensityToggle } from '@/components/shared'
import { useDensity } from '@/lib/hooks/common/useDensity'
import { Button } from '@/components/ui/button'
import {
  TagFacetPanel,
  TagFacetSheet,
  parseTagsParam,
  buildTagsParam,
} from '@/features/tags'

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

  // `?cities=` via nuqs (shared parser). /artists has NO derived default —
  // absent (null) simply means "all artists" (PSY-496 above) — so unlike
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

  const { data: citiesData, isLoading: citiesLoading, isFetching: citiesFetching } = useArtistCities()
  const { data, isLoading, isFetching, error, refetch } = useArtists({
    cities: selectedCities.length > 0 ? selectedCities : undefined,
    tags: selectedTags.length > 0 ? selectedTags : undefined,
    tagMatch,
  })

  // City changes write `?cities=` via nuqs (empty → null → bare URL; no
  // default derivation on /artists, see PSY-496 note above).
  const handleFilterChange = useCallback(
    (cities: CityState[]) => {
      void setCities(cities.length > 0 ? cities : null)
    },
    [setCities]
  )

  // Tag changes rewrite only the tag params, preserving `?cities=` verbatim.
  const writeTags = useCallback(
    (nextTags: string[], nextMatch: 'all' | 'any') => {
      const params = new URLSearchParams(searchParams.toString())
      params.delete('tags')
      params.delete('tag_match')
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
    [searchParams, router]
  )

  const handleTagsChange = useCallback(
    (nextTags: string[]) => writeTags(nextTags, tagMatch),
    [tagMatch, writeTags]
  )

  const handleTagsClear = useCallback(() => {
    writeTags([], tagMatch)
  }, [tagMatch, writeTags])

  // "Clear filters" resets tags AND cities in a SINGLE navigation — mixing a
  // router push (tags) with nuqs's throttled setCities in one tick races
  // (nuqs aborts its queue on a foreign history update; see PSY-1388).
  const handleClearFilters = useCallback(() => {
    startTransition(() => {
      router.push('/artists', { scroll: false })
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

      {/* Mobile: Sheet trigger + density toggle. Desktop hides the Sheet (the
          bar below takes over) but keeps the density toggle on this row. */}
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

      {/* PSY-1001: full-width top-bar tag filter above a full-width list (no
          left rail). Desktop only — mobile uses the Sheet trigger above. */}
      <div className="mb-4 hidden lg:block">
        <TagFacetPanel
          selectedSlugs={selectedTags}
          onToggle={handleTagsChange}
          onClear={handleTagsClear}
          heading="Filter artists by tag"
          entityType="artist"
          layout="bar"
        />
      </div>

      <div className={`min-w-0 ${isUpdating ? 'opacity-60 transition-opacity duration-75' : 'transition-opacity duration-75'}`}>
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
                onClick={handleClearFilters}
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
    </section>
  )
}
