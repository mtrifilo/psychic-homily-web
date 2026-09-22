'use client'

import { useMemo, useState } from 'react'
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

/** Rows the anonymous home list shows for the selected city. */
export const HOME_SHOW_LIMIT = 5

/**
 * Extra rows asked for when the caller drops the viewer's saved shows, so the
 * list can still fill its slots after the drop.
 *
 * Its own constant, not a Library row cap reused: the two must be free to move
 * independently. And a CONSTANT rather than "rows + however many are saved":
 * `limit` is part of the query key, so a limit that grew as the exclusions
 * resolved would re-key the query and fire a second request whose first
 * response is thrown away.
 *
 * The headroom is a budget, not a guarantee: a viewer who has saved more of
 * the page than this sees a shorter list, and the copy below says so rather
 * than claiming the city has nothing left.
 */
const HOME_SHOW_EXCLUSION_HEADROOM = 4

const DAY_MS = 24 * 60 * 60 * 1000

interface HomeShowListViewProps {
  /** The city selection this list renders, owned by the caller so a header that
   *  names the city reads the same value the rows were fetched with. */
  selection: HomeShowCitySelection
  /** Rows to paint. Defaults to the anonymous home's five. */
  rows?: number
  /**
   * Drop the rows this viewer has already saved. Set by the signed-in home,
   * where the saved-shows module directly above lists them.
   *
   * The saved state comes from the batch save-count read this list already
   * makes for its hearts, keyed on every fetched row, so the exclusion is exact
   * for the page without a second read. The list waits for that batch rather
   * than painting rows it is about to drop.
   */
  excludeSaved?: boolean
  /**
   * Keep only rows whose date falls within this many days of now. Set by a
   * header that promises a window ("this week"); absent, the list is the plain
   * soonest-first page.
   */
  withinDays?: number
  /** City name for the sentences that describe an exhausted page; omitted when
   *  no city resolved, which those sentences then leave out. */
  excludedLabel?: string
}

/**
 * The home upcoming-shows list: filter chips, geo affordance and a handful of
 * rows for the caller's city selection.
 *
 * Split out of `HomeShowList` (PSY-2103) so the signed-in "Shows near you this
 * week" section can render the same list beneath a header that names the same
 * city. `HomeShowList` remains the anonymous home's entry point and owns the
 * selection itself.
 */
export function HomeShowListView({
  selection,
  rows = HOME_SHOW_LIMIT,
  excludeSaved = false,
  withinDays,
  excludedLabel,
}: HomeShowListViewProps) {
  const { user, isAuthenticated } = useAuthContext()
  const isAdmin = user?.is_admin ?? false
  const {
    cities,
    favoriteCities,
    effectiveCities,
    source,
    geoAffordanceCity,
    selectionDiffersFromFavorites,
    onFilterChange,
  } = selection

  const limit = excludeSaved ? rows + HOME_SHOW_EXCLUSION_HEADROOM : rows

  const { data, isLoading, isFetching, error } = useUpcomingShows({
    limit,
    cities: effectiveCities.length > 0 ? effectiveCities : undefined,
  })

  // Prefetch /shows and /venues data during idle time
  usePrefetchRoutes()

  const fetchedShows = useMemo(() => data?.shows ?? [], [data?.shows])

  // The window is applied to the fetched page, not requested from the API: the
  // list endpoint has no "next N days" parameter. A page that is wholly
  // outside the window is an honest empty state, not a shortfall. "Now" is
  // read once per mount: render stays pure, and a page left open drifts by at
  // most the session, which the next visit corrects.
  const [mountedAt] = useState(() => Date.now())
  const windowedShows = useMemo(() => {
    if (withinDays === undefined) return fetchedShows
    const cutoff = mountedAt + withinDays * DAY_MS
    // An unparseable date keeps its row: a dropped row would turn a data
    // fault into a confident "no shows" sentence.
    return fetchedShows.filter(show => {
      const at = new Date(show.event_date).getTime()
      return Number.isNaN(at) || at <= cutoff
    })
  }, [fetchedShows, withinDays, mountedAt])

  // Keyed on every fetched row: the hearts need each visible row's count, and
  // the exclusion needs each candidate's saved state, and the batch answers
  // both in one request.
  const fetchedIds = useMemo(
    () => fetchedShows.map(show => show.id),
    [fetchedShows]
  )
  const { data: saveCounts, fetchStatus: batchFetchStatus } =
    useShowSaveCountBatch(fetchedIds, isAuthenticated, user?.id)

  // Wait only while the batch is actually in flight. An errored or disabled
  // batch also has no data, and holding the list on those would leave the
  // signed-in home's only discovery list on a spinner for good; a repeated
  // row is recoverable, a dead section is not.
  const isExclusionPending =
    excludeSaved &&
    fetchedShows.length > 0 &&
    saveCounts === undefined &&
    batchFetchStatus === 'fetching'

  const visibleShows = useMemo(() => {
    if (!excludeSaved || !saveCounts) return windowedShows.slice(0, rows)
    return windowedShows
      .filter(show => !saveCounts[String(show.id)]?.is_saved)
      .slice(0, rows)
  }, [windowedShows, excludeSaved, saveCounts, rows])

  if (isLoading || isExclusionPending) {
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

  // Four different facts, four different sentences. Only the first is "there
  // are none". The saved sentences claim exactly what was seen: a page cut off
  // at `limit` says nothing about the shows beyond it, and a window says
  // nothing about the shows outside it, so neither may call the city
  // exhausted. The caller's label, when given, names the place in every
  // sentence; the anonymous list keeps its bare city names.
  const placeLabel =
    excludedLabel ?? effectiveCities.map(c => c.city).join(', ')
  const inPlace = placeLabel ? ` in ${placeLabel}` : ''
  const pageWasComplete = fetchedShows.length < limit
  const windowLabel =
    withinDays === undefined ? '' : ` in the next ${withinDays} days`
  const emptyMessage =
    visibleShows.length > 0
      ? null
      : fetchedShows.length === 0
        ? placeLabel
          ? `No upcoming shows${inPlace}.`
          : 'No upcoming shows at this time.'
        : windowedShows.length === 0
          ? `No shows${inPlace}${windowLabel}.`
          : pageWasComplete
            ? `Every show${inPlace}${windowLabel} is already in your saved shows.`
            : `The next ${windowedShows.length} shows${inPlace} are all in your saved shows.`

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
            {/* Only a selection the viewer made themselves is offered as a
                default: a geo match or the liveliest-city guess is not theirs
                to persist with one click. */}
            {isAuthenticated &&
              source === 'user' &&
              selectionDiffersFromFavorites && (
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
        {emptyMessage ? (
          <div className="text-center py-8 text-muted-foreground">
            <p>{emptyMessage}</p>
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
