'use client'

/**
 * Shows Hooks
 *
 * TanStack Query hooks for fetching show data from the API.
 */

import { useQuery, keepPreviousData } from '@tanstack/react-query'
import { apiRequest } from '@/lib/api'
import {
  showEndpoints,
  showQueryKeys,
  SHOW_CITIES_FIRST_SCREEN_URL,
} from '@/features/shows/api'
import type {
  UpcomingShowsResponse,
  ShowsCalendarResponse,
  ShowMonthsResponse,
  ShowResponse,
  ShowCitiesResponse,
  ShowTimelineResponse,
} from '../types'
import { SHOWS_PAGE_SIZE } from '../showsListNavigation'
import {
  appendShowsCalendarWindow,
  showsCalendarWindowKey,
  type ShowsCalendarWindow,
} from '../showsCalendarRoute'
import type { ShowAlsoTonightResponse } from '../showRails'
import { buildCitiesParam } from '@/components/filters/cityParams'
import {
  appendCityCountScope,
  cityCountScopeKey,
  type CityCountScope,
} from '@/components/filters/cityCountScope'

/**
 * The filter contract every reader of the venue-local upcoming partition
 * shares: the cursor feed, the offset list, and the month histogram that labels
 * that list's pages.
 *
 * One declaration, because a histogram filtered differently from the list it
 * labels describes a different set of rows. The request half and the cache-key
 * half both derive from it below, so the two cannot drift either.
 */
interface ShowListFilterOptions {
  /** Legacy single-city filter */
  city?: string
  /** Legacy single-state filter */
  state?: string
  /** Multi-city filter (takes priority over city/state) */
  cities?: Array<{ city: string; state: string }>
  /** Multi-tag filter (PSY-309). Slugs applied with AND by default. */
  tags?: string[]
  /** Set to 'any' to switch the tag filter to OR semantics. */
  tagMatch?: 'all' | 'any'
}

/** Append the filter params to a request. */
function appendShowListFilters(
  params: URLSearchParams,
  { city, state, cities, tags, tagMatch }: ShowListFilterOptions
): void {
  if (cities && cities.length > 0) {
    // Multi-city takes priority over legacy single-city.
    params.set('cities', buildCitiesParam(cities))
  } else {
    if (city) params.set('city', city)
    if (state) params.set('state', state)
  }

  if (tags && tags.length > 0) {
    params.set('tags', tags.join(','))
    if (tagMatch === 'any') params.set('tag_match', 'any')
  }
}

/**
 * The cache-key half of the same contract.
 *
 * Normalizes the two fields with a non-obvious empty form: an empty tag list
 * and the default tag match both key as `undefined`, so the same filter state
 * lands on one entry however a caller spelled it. Written once, or the list and
 * its histogram could key apart under filters that request identically.
 */
function showListFilterKey({
  city,
  state,
  cities,
  tags,
  tagMatch,
}: ShowListFilterOptions): Record<string, unknown> {
  return {
    city,
    state,
    cities,
    tags: tags && tags.length > 0 ? tags : undefined,
    tagMatch: tagMatch === 'any' ? 'any' : undefined,
  }
}

interface UseUpcomingShowsOptions extends ShowListFilterOptions {
  cursor?: string
  limit?: number
}

interface UseShowsCalendarOptions extends ShowListFilterOptions {
  /** Rows to skip. Omitted from the request at 0, which is page 1. */
  offset?: number
  /** Rows per page. Always sent, so the pager's arithmetic and the request agree. */
  limit?: number
  /**
   * The venue-local calendar window the list is scoped to, or undefined for
   * the whole upcoming set. A month is `{year, month}`; a day adds `day`.
   *
   * The backend refuses a half-stated window (a month without a year, a day
   * without a month), so this travels as one value rather than three loose
   * fields a caller could set independently.
   */
  window?: ShowsCalendarWindow
}

/**
 * Hook to fetch upcoming shows with cursor-based pagination.
 *
 * The reader behind the home page's rail and `/explore`. `/shows` pages by
 * number and reads `useShowsCalendar` instead: a cursor is a position in one
 * response, and a page number has to survive being bookmarked.
 *
 * No PER-VIEWER input: whether a show is still upcoming is decided against its
 * own venue's zone, so the response is the same for every viewer (PSY-1678).
 * Do not reintroduce a per-viewer key segment here.
 *
 * A no-argument call sends NO query string at all, which is the bare endpoint.
 */
export const useUpcomingShows = (options: UseUpcomingShowsOptions = {}) => {
  const { cursor, limit, city, state, cities, tags, tagMatch } = options

  const params = new URLSearchParams()
  if (cursor) params.set('cursor', cursor)
  if (limit) params.set('limit', limit.toString())
  appendShowListFilters(params, { city, state, cities, tags, tagMatch })

  // The `?` only when there is something to put after it, so a bare call
  // produces the endpoint itself.
  const queryString = params.toString()
  const endpoint = queryString
    ? `${showEndpoints.UPCOMING}?${queryString}`
    : showEndpoints.UPCOMING

  return useQuery({
    queryKey: showQueryKeys.list({
      cursor,
      limit,
      ...showListFilterKey({ city, state, cities, tags, tagMatch }),
    }),
    queryFn: async (): Promise<UpcomingShowsResponse> => {
      return apiRequest<UpcomingShowsResponse>(endpoint, {
        method: 'GET',
      })
    },
    staleTime: 5 * 60 * 1000, // 5 minutes
    placeholderData: keepPreviousData, // Keep old data visible while fetching
  })
}

/**
 * Hook to fetch one OFFSET page of upcoming shows, the reader behind `/shows`.
 *
 * Offset rather than the cursor `useUpcomingShows` uses, because this list is
 * addressed by page number: `?page=3` has to name the same slice tomorrow as it
 * does today, which a cursor minted from one response cannot promise. The cost
 * is stated on the backend's `GetUpcomingShowsPage`: a show graduating out of
 * the partition between two page reads shifts later rows up one, so a reader
 * walking pages can skip a row.
 *
 * Carries no PER-VIEWER input, for the same reason and with the same
 * consequence as `useUpcomingShows`: page 1 of the unfiltered list is the entry
 * `app/shows/page.tsx` seeds server-side.
 *
 * `keepPreviousData` is load-bearing rather than cosmetic. The pager's
 * page-change announcement is a live region, and a consumer that tore its rows
 * down on every page click would unmount it and announce nothing at all.
 */
export const useShowsCalendar = (options: UseShowsCalendarOptions = {}) => {
  const {
    offset,
    limit = SHOWS_PAGE_SIZE,
    // Aliased: `window` is the global inside a client hook, and a later
    // `typeof window === 'undefined'` guard added here would silently read the
    // calendar window instead.
    window: calendarWindow,
    city,
    state,
    cities,
    tags,
    tagMatch,
  } = options

  const params = new URLSearchParams()
  params.set('limit', limit.toString())
  if (offset) params.set('offset', offset.toString())
  appendShowsCalendarWindow(params, calendarWindow)
  appendShowListFilters(params, { city, state, cities, tags, tagMatch })

  return useQuery({
    queryKey: showQueryKeys.calendar({
      limit,
      offset: offset || undefined,
      ...showsCalendarWindowKey(calendarWindow),
      ...showListFilterKey({ city, state, cities, tags, tagMatch }),
    }),
    queryFn: async (): Promise<ShowsCalendarResponse> => {
      return apiRequest<ShowsCalendarResponse>(
        `${showEndpoints.CALENDAR}?${params.toString()}`,
        { method: 'GET' }
      )
    },
    staleTime: 5 * 60 * 1000, // 5 minutes
    placeholderData: keepPreviousData,
  })
}

/**
 * Hook to fetch the upcoming-shows month histogram under the same filters as
 * the list.
 *
 * Keyed on the FILTERS alone, never the page, so paging never re-requests it.
 * `staleTime` matches the endpoint's own `max-age=60`: the counts move whenever
 * a show is approved or a day rolls over at a venue, and they are what the page
 * labels are derived from.
 */
export const useShowMonths = (options: ShowListFilterOptions = {}) => {
  const { city, state, cities, tags, tagMatch } = options

  const params = new URLSearchParams()
  appendShowListFilters(params, { city, state, cities, tags, tagMatch })
  const queryString = params.toString()

  return useQuery({
    queryKey: showQueryKeys.months(
      showListFilterKey({ city, state, cities, tags, tagMatch })
    ),
    queryFn: async (): Promise<ShowMonthsResponse> => {
      return apiRequest<ShowMonthsResponse>(
        queryString
          ? `${showEndpoints.MONTHS}?${queryString}`
          : showEndpoints.MONTHS,
        { method: 'GET' }
      )
    },
    staleTime: 60 * 1000, // the endpoint's own max-age
    placeholderData: keepPreviousData,
  })
}

/**
 * Hook to fetch a single show by ID
 */
export const useShow = (showId: string | number) => {
  return useQuery({
    queryKey: showQueryKeys.detail(String(showId)),
    queryFn: async (): Promise<ShowResponse> => {
      return apiRequest<ShowResponse>(showEndpoints.GET(showId), {
        method: 'GET',
      })
    },
    enabled: Boolean(showId),
    staleTime: 5 * 60 * 1000, // 5 minutes
  })
}

/**
 * Hook to fetch the show's also-tonight rail: other shows in this show's metro
 * on this show's own date (PSY-1683 / PSY-1689).
 *
 * A show with no scene to look at answers 200 with an empty rail rather than
 * 404, so an error here means the request failed, not that the night is quiet —
 * the rail hides in both cases, and only the empty case is normal.
 *
 * Same `staleTime` as the show itself: the rail is a property of the night, and
 * a reader who leaves the page open is not owed a refetch of what else was on.
 */
export const useShowAlsoTonight = (
  showId: string | number,
  /**
   * Lets a caller that will not RENDER this rail skip fetching it. Defaults to
   * on, so the only callers paying attention are the ones with a reason.
   */
  enabled = true,
  /**
   * The rail the SERVER already fetched for this same show, so the rows are in
   * the served HTML instead of arriving after hydration and pushing the
   * comment thread down (PSY-1967).
   *
   * Seeded here rather than through a `HydrationBoundary` on the route, the
   * same choice `useVenueShows` documents: the key is built from this hook's
   * own argument, so no seed can land on an entry this hook does not read.
   * Pass it only for the show this hook is asking about — react-query attaches
   * `initialData` to whatever key is current, so another show's rail would be
   * seeded as this one's.
   */
  initialData?: ShowAlsoTonightResponse
) => {
  return useQuery({
    queryKey: showQueryKeys.alsoTonight(String(showId)),
    queryFn: async (): Promise<ShowAlsoTonightResponse> => {
      return apiRequest<ShowAlsoTonightResponse>(
        showEndpoints.ALSO_TONIGHT(showId),
        { method: 'GET' }
      )
    },
    enabled: enabled && Boolean(showId),
    staleTime: 5 * 60 * 1000, // 5 minutes
    initialData,
  })
}

/**
 * Hook to fetch a show's gig timeline: the headliner's adjacent dates and each
 * billed act's recurrence in this show's place.
 *
 * Takes the NUMERIC id, not the route's slug, because the query key is the
 * numeric one -- see `showQueryKeys.timeline`. The show route seeds this key
 * server-side from the same id, so the modules are in the first paint rather
 * than shifting the page when they arrive.
 *
 * `staleTime` matches `useShow`'s rather than running long on the "it is all
 * archive data" argument, which is false for half the payload: `next` is by
 * construction a FUTURE date, and a newly announced show ahead of this one
 * invalidates it with no mutation on this page to observe. It also must not
 * compound with the route's own `revalidate: 3600`, since the server seed is
 * stamped "fetched now" and a longer window here would sit on a payload that
 * was already up to an hour old when it arrived.
 */
export const useShowTimeline = (showId: number | undefined) => {
  return useQuery({
    queryKey: showQueryKeys.timeline(showId ?? 0),
    queryFn: async (): Promise<ShowTimelineResponse> => {
      return apiRequest<ShowTimelineResponse>(
        showEndpoints.TIMELINE(showId as number),
        { method: 'GET' },
      )
    },
    enabled: Boolean(showId),
    staleTime: 5 * 60 * 1000, // 5 minutes
  })
}

/**
 * The half of the shows list's state a city facet may be scoped by: the tag
 * filter and the calendar window, and no place, because the response IS the
 * per-place breakdown.
 */
interface UseShowCitiesOptions extends CityCountScope {
  /** The venue-local window the list is reading, or undefined for all of it. */
  window?: ShowsCalendarWindow
}

/**
 * Hook to fetch cities that have upcoming shows with counts, optionally scoped
 * to the filters and window the list below the picker is reading.
 *
 * Nothing per-viewer is keyed either way: the counts cover the same venue-local
 * upcoming partition `useUpcomingShows` lists, so every viewer gets the same
 * answer for the same scope (PSY-1678).
 *
 * Unscoped it requests the seeded URL directly, which is the bare endpoint, and
 * keys exactly as the seed does — that is what keeps the first-screen payload a
 * hit. A filtered deep link is not covered by the seed, which is the same call
 * the list's own first-screen seeds already make.
 */
export const useShowCities = (options: UseShowCitiesOptions = {}) => {
  const { tags, tagMatch, window: calendarWindow } = options

  const params = new URLSearchParams()
  appendCityCountScope(params, { tags, tagMatch })
  appendShowsCalendarWindow(params, calendarWindow)
  const queryString = params.toString()

  return useQuery({
    queryKey: queryString
      ? [
          ...showQueryKeys.cities(),
          {
            ...cityCountScopeKey({ tags, tagMatch }),
            ...showsCalendarWindowKey(calendarWindow),
          },
        ]
      : showQueryKeys.cities(),
    queryFn: async (): Promise<ShowCitiesResponse> => {
      return apiRequest<ShowCitiesResponse>(
        queryString
          ? `${SHOW_CITIES_FIRST_SCREEN_URL}?${queryString}`
          : SHOW_CITIES_FIRST_SCREEN_URL,
        { method: 'GET' }
      )
    },
    staleTime: 5 * 60 * 1000, // 5 minutes
    placeholderData: keepPreviousData, // Keep old data visible while fetching
  })
}
