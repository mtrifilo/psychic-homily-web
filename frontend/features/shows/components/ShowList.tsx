'use client'

import { useState, useCallback, useMemo, useTransition } from 'react'
import { useSearchParams, useRouter } from 'next/navigation'
import { useQueryState } from 'nuqs'
import { useUpcomingShows, useShowCities } from '../hooks/useShows'
import { useShowSaveCountBatch } from '../hooks/useSavedShows'
import { useAuthContext } from '@/lib/context/AuthContext'
import { useProfile } from '@/features/auth'
import type { ShowResponse } from '../types'
import type { CityState } from '@/components/filters'
import { cn } from '@/lib/utils'
import { Button } from '@/components/ui/button'
import { DensityToggle } from '@/components/shared'
import { useDensity } from '@/lib/hooks/common/useDensity'
import { ShowCard } from './ShowCard'
import { ShowListSkeleton } from './ShowListSkeleton'
import { CityFilters, type CityWithCount } from '@/components/filters'
import {
  citiesEqual,
  citiesParser,
  ALL_CITIES,
} from '@/components/filters/cityParams'
import {
  useGeoDefaultCity,
  shouldShowGeoAffordance,
} from '@/components/filters/useGeoDefaultCity'
import { GeoDefaultAffordance } from '@/components/filters/GeoDefaultAffordance'
import { SaveDefaultsButton } from '@/components/filters/SaveDefaultsButton'
import {
  TagFacetPanel,
  TagFacetSheet,
  parseTagsParam,
  buildTagsParam,
} from '@/features/tags'

export function ShowList() {
  const router = useRouter()
  const searchParams = useSearchParams()
  const { user, isAuthenticated, isLoading: authLoading } = useAuthContext()
  const isAdmin = user?.is_admin ?? false
  const [isPending, startTransition] = useTransition()
  const { data: profileData } = useProfile()
  const { density, setDensity } = useDensity('shows')

  // Read favorites from profile — the per-user default city.
  const favoriteCities: CityState[] = useMemo(() => {
    const prefs = profileData?.user?.preferences
    if (!prefs?.favorite_cities) return []
    return prefs.favorite_cities
  }, [profileData?.user?.preferences])

  // `?cities=` is the source of truth, read/written via nuqs. Three states:
  //   null        → param absent → apply the default (favorites/geo), derived below
  //   ALL_CITIES  → ?cities=all → explicit "all cities"
  //   CityState[] → explicit selection
  // Filter changes push a history entry so the back button steps through them.
  const [citiesState, setCities] = useQueryState(
    'cities',
    citiesParser.withOptions({ history: 'push', startTransition })
  )

  // Legacy single-city params (?city=&state=) — read-only, for back-compat.
  const legacyCity = searchParams.get('city')
  const legacyState = searchParams.get('state')

  // Parse multi-tag from URL (PSY-309)
  const tagsParam = searchParams.get('tags')
  const tagMatchParam = searchParams.get('tag_match')
  const selectedTags = useMemo(() => parseTagsParam(tagsParam), [tagsParam])
  const tagMatch: 'all' | 'any' = tagMatchParam === 'any' ? 'any' : 'all'

  const timezone = Intl.DateTimeFormat().resolvedOptions().timeZone

  // Any explicit selection (?cities=<pick>, ?cities=all, or legacy single-city)
  // means geo must not seed. Authed favorites also stand the geo hook down
  // (handled inside the hook via favoriteCities + isAuthenticated).
  const hasExistingSelection = citiesState !== null || !!(legacyCity && legacyState)
  const [cursor, setCursor] = useState<string | undefined>(undefined)
  const [accumulatedShows, setAccumulatedShows] = useState<ShowResponse[]>([])

  const {
    data: citiesData,
    isLoading: citiesLoading,
    isFetching: citiesFetching,
  } = useShowCities({
    timezone,
  })

  // Map ShowCity → CityWithCount (the has-shows list). Lifted above the early
  // returns so the geo hook can read it unconditionally.
  const cities: CityWithCount[] = useMemo(
    () =>
      citiesData?.cities?.map(c => ({
        city: c.city,
        state: c.state,
        count: c.show_count,
        // Geocoded centroid (PSY-981) — drives the nearest-has-shows-city geo
        // default when the visitor's exact city has no shows.
        latitude: c.latitude,
        longitude: c.longitude,
      })) ?? [],
    [citiesData?.cities]
  )

  // IP-geo soft default for anon visitors (PSY-946). /shows reads geo via the
  // `/api/geo` edge route handler client-side (the page stays ISR — it must
  // not read `next/headers`). The hook RETURNS the derived canonical city;
  // it's folded into the derived selection below (never written to the URL).
  // Favorites and an existing `?cities=`/legacy selection both win (the hook
  // stands down).
  const { appliedGeoDefault, notifyUserInteracted } = useGeoDefaultCity({
    cities,
    isAuthenticated,
    authLoading,
    favoriteCities,
    hasExistingSelection,
    enableClientFetch: true,
  })

  // The effective city filter, DERIVED during render — never seeded into the
  // URL by an effect. A bare /shows resolves to the user's favorite (or, for
  // anon visitors, the geo default); an explicit ?cities=all or ?cities=<pick>
  // wins. Deriving (rather than writing the default from a mount effect) is
  // what makes the default survive client-side navigation: the URL, not a
  // mount ref, is the source of truth.
  const selectedCities: CityState[] = useMemo(() => {
    if (citiesState === ALL_CITIES) return []
    if (citiesState) return citiesState
    if (legacyCity && legacyState) return [{ city: legacyCity, state: legacyState }]
    if (favoriteCities.length > 0) return favoriteCities
    return appliedGeoDefault ? [appliedGeoDefault] : []
  }, [citiesState, legacyCity, legacyState, favoriteCities, appliedGeoDefault])

  const { data, isLoading, isFetching, error, refetch } = useUpcomingShows({
    timezone,
    cursor,
    cities: selectedCities.length > 0 ? selectedCities : undefined,
    tags: selectedTags.length > 0 ? selectedTags : undefined,
    tagMatch,
  })

  // Batch-check saved status for all visible shows (1 request instead of N)
  const allShows = useMemo(
    () => [...accumulatedShows, ...(data?.shows || [])],
    [accumulatedShows, data?.shows]
  )
  const allShowIds = useMemo(() => allShows.map(s => s.id), [allShows])
  const { data: saveCounts } = useShowSaveCountBatch(
    allShowIds,
    isAuthenticated,
    user?.id
  )

  const handleLoadMore = useCallback(() => {
    if (data?.pagination.next_cursor) {
      // Accumulate current shows before loading next page
      const currentShows = data.shows || []
      setAccumulatedShows(prev => [...prev, ...currentShows])
      setCursor(data.pagination.next_cursor!)
    }
  }, [data])

  // City filter changes write the `?cities=` param via nuqs (which preserves
  // other params). An empty selection becomes the explicit ALL_CITIES sentinel
  // (?cities=all), NOT a bare URL — a bare URL means "apply my default".
  const handleFilterChange = useCallback(
    (cities: CityState[]) => {
      // Any manual city change is an override — block a still-in-flight geo seed
      // and drop the affordance.
      notifyUserInteracted()
      setCursor(undefined)
      setAccumulatedShows([])
      void setCities(cities.length > 0 ? cities : ALL_CITIES)
    },
    [notifyUserInteracted, setCities]
  )

  // Tag changes rewrite only the tag params via the router, preserving the raw
  // `?cities=` state (absent / all / selection) so a tag change never
  // materializes the derived default into the URL.
  const writeTags = useCallback(
    (nextTags: string[], nextMatch: 'all' | 'any') => {
      setCursor(undefined)
      setAccumulatedShows([])
      const params = new URLSearchParams(searchParams.toString())
      params.delete('tags')
      params.delete('tag_match')
      if (nextTags.length > 0) {
        params.set('tags', buildTagsParam(nextTags))
        if (nextMatch === 'any') params.set('tag_match', 'any')
      }
      const queryString = params.toString()
      startTransition(() => {
        router.push(queryString ? `/shows?${queryString}` : '/shows', {
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

  // "Clear filters" (the empty-state affordance) resets tags AND cities in a
  // SINGLE navigation. Doing it as two writes would race: the router push for
  // tags and nuqs's throttled `setCities` fire in the same tick, and nuqs's
  // adapter aborts its pending queue when it sees a foreign history update — so
  // the `?cities=all` reset could be silently dropped. One write avoids that.
  const handleClearFilters = useCallback(() => {
    notifyUserInteracted()
    setCursor(undefined)
    setAccumulatedShows([])
    startTransition(() => {
      router.push('/shows?cities=all', { scroll: false })
    })
  }, [notifyUserInteracted, router])

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

  const showGeoAffordance = shouldShowGeoAffordance(
    appliedGeoDefault,
    selectedCities
  )

  return (
    <section className="w-full max-w-6xl">
      {/* Show the filter whenever ≥1 city has shows (PSY-932) — consistent
          with /venues and /artists; hidden only when there are no cities. */}
      {cities.length > 0 && (
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
          {showGeoAffordance && (
            <GeoDefaultAffordance
              city={appliedGeoDefault}
              onChange={() => handleFilterChange([])}
            />
          )}
        </div>
      )}

      {/* Mobile: Sheet trigger + density toggle. Desktop hides the Sheet (the
          bar below takes over) but keeps the density toggle on this row. */}
      <div className="flex items-center justify-between mb-4 gap-2">
        <TagFacetSheet
          selectedSlugs={selectedTags}
          onToggle={handleTagsChange}
          onClear={handleTagsClear}
          title="Filter shows by tag"
          entityType="show"
          selectedCities={selectedCities}
        />
        <DensityToggle density={density} onDensityChange={setDensity} />
      </div>

      {/* PSY-1000: full-width top-bar tag filter above a full-width list (no
          left rail). Desktop only — mobile uses the Sheet trigger above. */}
      <div className="mb-4 hidden lg:block">
        <TagFacetPanel
          selectedSlugs={selectedTags}
          onToggle={handleTagsChange}
          onClear={handleTagsClear}
          heading="Filter shows by tag"
          entityType="show"
          selectedCities={selectedCities}
          layout="bar"
        />
      </div>

      <div className={cn('min-w-0', isUpdating ? 'opacity-60 transition-opacity duration-75' : 'transition-opacity duration-75')}>
        <p className="mb-3 text-sm text-muted-foreground" data-testid="show-count">
          {allShows.length} {allShows.length === 1 ? 'show' : 'shows'}
          {selectedTags.length > 0 && ` matching ${selectedTags.join(', ')}`}
        </p>
        {allShows.length === 0 ? (
          <div className="text-center py-12 text-muted-foreground">
            <p>
              {selectedTags.length > 0 || selectedCities.length > 0
                ? 'No upcoming shows match the current filters.'
                : 'No upcoming shows at this time.'}
            </p>
            {(selectedTags.length > 0 || selectedCities.length > 0) && (
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
            <div className={cn(
              'flex flex-col',
              density === 'compact' && 'gap-0.5',
              density === 'comfortable' && 'gap-3',
              density === 'expanded' && 'gap-5'
            )}>
              {allShows.map(show => (
                <ShowCard
                  key={show.id}
                  show={show}
                  isAdmin={isAdmin}
                  userId={user?.id}
                  saveData={saveCounts?.[String(show.id)]}
                  density={density}
                />
              ))}
            </div>

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
