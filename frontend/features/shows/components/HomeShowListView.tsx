'use client'

import { useMemo } from 'react'
import { useUpcomingShows } from '../hooks/useShows'
import { batchedSaveFor } from '@/components/shared/batchedSaveData'
import { useShowSaveCountBatch } from '../hooks/useSavedShows'
import type { HomeShowCitySelection } from '../hooks/useHomeShowCitySelection'
import { usePrefetchRoutes } from '@/lib/hooks/common/usePrefetchRoutes'
import { useAuthContext } from '@/lib/context/AuthContext'
import { ShowCard } from './ShowCard'
import { CityFilters } from '@/components/filters'
import { GeoDefaultAffordance } from '@/components/filters/GeoDefaultAffordance'
import { SaveDefaultsButton } from '@/components/filters/SaveDefaultsButton'

/** Rows the home list shows for the selected city. */
const HOME_SHOW_LIMIT = 5

/**
 * Shows to leave out, or 'pending' while the caller is still resolving them.
 *
 * 'pending' is the same idiom `batchedSaveFor` uses for a batch in flight, and
 * it exists for the same reason: an absent exclusion set is indistinguishable
 * from an empty one, so without it the list paints rows it is about to drop.
 */
export type HomeShowExclusions = readonly number[] | 'pending'

interface HomeShowListViewProps {
  /** The city selection this list renders, owned by the caller so a header that
   *  names the city reads the same value the rows were fetched with. */
  selection: HomeShowCitySelection
  /**
   * Show ids to leave out. Set by the signed-in home, where the saved-shows
   * module directly above already lists them, so the two lists never repeat a
   * row.
   *
   * The caller resolves these from a request of its own that runs BESIDE this
   * component's, rather than from anything keyed on the rows fetched here: an
   * exclusion set derived from the response would serialize two round-trips
   * before a single row could paint.
   */
  excludeShowIds?: HomeShowExclusions
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
  excludeShowIds,
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

  const excludedIds = Array.isArray(excludeShowIds) ? excludeShowIds : undefined

  const { data, isLoading, isFetching, error } = useUpcomingShows({
    // Ask for the excluded rows on top, so the section still fills its five
    // slots instead of shrinking by however many the viewer has saved.
    limit: HOME_SHOW_LIMIT + (excludedIds?.length ?? 0),
    cities: effectiveCities.length > 0 ? effectiveCities : undefined,
  })

  // Prefetch /shows and /venues data during idle time
  usePrefetchRoutes()

  const fetchedShows = useMemo(() => data?.shows ?? [], [data?.shows])

  const visibleShows = useMemo(() => {
    if (!excludedIds) return fetchedShows.slice(0, HOME_SHOW_LIMIT)
    const excluded = new Set(excludedIds)
    return fetchedShows
      .filter(show => !excluded.has(show.id))
      .slice(0, HOME_SHOW_LIMIT)
  }, [fetchedShows, excludedIds])

  const showIds = useMemo(
    () => visibleShows.map(show => show.id),
    [visibleShows]
  )
  const { data: saveCounts } = useShowSaveCountBatch(
    showIds,
    isAuthenticated,
    user?.id
  )

  if (isLoading || excludeShowIds === 'pending') {
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
          // Only the genuinely-empty page gets the "no shows" sentence. When
          // rows came back and exclusions removed all of them, the viewer has
          // saved everything on offer here and they are listed directly above,
          // so saying there are none would be false.
          fetchedShows.length === 0 && (
            <div className="text-center py-8 text-muted-foreground">
              <p>
                {effectiveCities.length > 0
                  ? `No upcoming shows in ${effectiveCities.map(c => c.city).join(', ')}.`
                  : 'No upcoming shows at this time.'}
              </p>
            </div>
          )
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
