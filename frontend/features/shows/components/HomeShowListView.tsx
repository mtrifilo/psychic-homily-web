'use client'

import { useMemo } from 'react'
import { useUpcomingShows } from '../hooks/useShows'
import { batchedSaveFor } from '@/components/shared/batchedSaveData'
import { useShowSaveCountBatch } from '../hooks/useSavedShows'
import { usePrefetchRoutes } from '@/lib/hooks/common/usePrefetchRoutes'
import { useAuthContext } from '@/lib/context/AuthContext'
import { ShowCard } from './ShowCard'
import { CityFilters } from '@/components/filters'
import { GeoDefaultAffordance } from '@/components/filters/GeoDefaultAffordance'
import { SaveDefaultsButton } from '@/components/filters/SaveDefaultsButton'
import type { HomeShowCitySelection } from './useHomeShowCitySelection'

interface HomeShowListViewProps {
  /** The city selection this list renders, owned by the caller so a header that
   *  names the city reads the same value the rows were fetched with. */
  selection: HomeShowCitySelection
  /**
   * Drop rows the viewer has already saved. Set by the signed-in home, where
   * the saved-shows module directly above already lists them, so the two lists
   * never repeat a row.
   *
   * `is_saved` comes from the batch save-count read, which is the same fact the
   * heart on each row paints — so a save made here removes the row and an
   * unsave made in the module above brings it back, with no second source of
   * truth to reconcile. The list waits for that batch rather than painting rows
   * it is about to drop.
   */
  excludeSavedShows?: boolean
}

/**
 * The home upcoming-shows list: filter chips, geo affordance and up to five
 * rows for the caller's city selection.
 *
 * Split out of `HomeShowList` (PSY-2103) so the signed-in "Shows near you this
 * week" section can render the same list beneath a header that names the same
 * city. `HomeShowList` remains the anonymous home's entry point and owns the
 * selection itself.
 */
export function HomeShowListView({
  selection,
  excludeSavedShows = false,
}: HomeShowListViewProps) {
  const { user, isAuthenticated } = useAuthContext()
  const isAdmin = user?.is_admin ?? false
  const {
    cities,
    favoriteCities,
    effectiveCities,
    geoAffordanceCity,
    selectionDiffersFromFavorites,
    onFilterChange,
  } = selection

  const { data, isLoading, isFetching, error } = useUpcomingShows({
    limit: 5,
    cities: effectiveCities.length > 0 ? effectiveCities : undefined,
  })

  // Prefetch /shows and /venues data during idle time
  usePrefetchRoutes()

  const showIds = useMemo(
    () => data?.shows?.map(s => s.id) ?? [],
    [data?.shows]
  )
  const { data: saveCounts, isLoading: isLoadingSaveCounts } =
    useShowSaveCountBatch(showIds, isAuthenticated, user?.id)

  // Filtering happens here rather than in the query so the batch above still
  // covers every fetched row: an excluded row's heart is what un-excludes it.
  const visibleShows = useMemo(() => {
    const shows = data?.shows ?? []
    if (!excludeSavedShows) return shows
    return shows.filter(show => !saveCounts?.[String(show.id)]?.is_saved)
  }, [data?.shows, excludeSavedShows, saveCounts])

  // `isLoading` is false while the batch is disabled (no ids yet), so an empty
  // list cannot strand this gate.
  const isAwaitingExclusions = excludeSavedShows && isLoadingSaveCounts

  if (isLoading || isAwaitingExclusions) {
    return (
      <div className="flex justify-center items-center py-8">
        <div className="animate-spin rounded-full h-6 w-6 border-b-2 border-foreground"></div>
      </div>
    )
  }

  if (error) {
    return (
      <div className="text-center py-8 text-muted-foreground">
        <p>Unable to load shows.</p>
      </div>
    )
  }

  return (
    <div className="w-full">
      {/* Show the filter whenever ≥1 city has shows (PSY-932) — consistent
          with /venues and /artists; hidden only when there are no cities. */}
      {cities.length > 0 && (
        <div className="mb-6">
          <CityFilters
            cities={cities}
            selectedCities={effectiveCities}
            onFilterChange={onFilterChange}
            resultNoun={{ singular: 'show', plural: 'shows' }}
          >
            {isAuthenticated && selectionDiffersFromFavorites && (
              <SaveDefaultsButton
                selectedCities={effectiveCities}
                favoriteCities={favoriteCities}
              />
            )}
          </CityFilters>
          {geoAffordanceCity && (
            <GeoDefaultAffordance
              city={geoAffordanceCity}
              onChange={() => onFilterChange([])}
            />
          )}
        </div>
      )}

      <div className={isFetching ? 'opacity-60 transition-opacity duration-75' : 'transition-opacity duration-75'}>
        {visibleShows.length === 0 ? (
          <div className="text-center py-8 text-muted-foreground">
            <p>
              {effectiveCities.length > 0
                ? `No upcoming shows in ${effectiveCities.map(c => c.city).join(', ')}.`
                : 'No upcoming shows at this time.'}
            </p>
          </div>
        ) : (
          <div className="flex flex-col gap-3">
            {visibleShows.map(show => (
              <ShowCard
                key={show.id}
                show={show}
                isAdmin={isAdmin}
                saveData={batchedSaveFor(saveCounts, show.id)}
              />
            ))}
          </div>
        )}
      </div>
    </div>
  )
}
