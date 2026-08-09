'use client'

/**
 * Venues Hooks
 *
 * TanStack Query hooks for fetching venue data from the API.
 */

import { useQuery, keepPreviousData } from '@tanstack/react-query'
import { apiRequest } from '@/lib/api'
import { createNamedDetailHook } from '@/lib/hooks/factories'
import { venueEndpoints, venueQueryKeys } from '@/features/venues/api'
import type { VenueShowsTimeFilter } from '@/features/venues/api'
import { buildCitiesParam } from '@/components/filters/cityParams'
import type { CityState } from '@/components/filters/CityFilters'
import type {
  Venue,
  VenuesListResponse,
  VenueShowsResponse,
  VenueShowYearsResponse,
  VenueCitiesResponse,
  VenueGenreResponse,
  VenueBillNetworkResponse,
  VenueBillNetworkWindow,
} from '../types'

interface UseVenuesOptions {
  state?: string
  city?: string
  cities?: CityState[]
  limit?: number
  offset?: number
  /** Multi-tag filter (PSY-309). Slugs applied with AND by default. */
  tags?: string[]
  /** Set to 'any' to switch the tag filter to OR semantics. */
  tagMatch?: 'all' | 'any'
  /**
   * Ask the API for the Atlas city-view rail fields (next show, this-week
   * slice, dominant genre). Off by default — they cost three extra batched
   * aggregations server-side, and only the rail renders them.
   */
  includeRail?: boolean
  /**
   * Widen `city`/`state` from that literal city to the whole CBSA metro it
   * belongs to (PSY-1574) — the same scope an Atlas scene is keyed by, so a
   * Tempe venue lists under Phoenix. Off by default: the venue browse page's
   * city filter means the literal city there. Ignored by the API unless both
   * `city` and `state` are set, and when `cities` is used instead.
   */
  metroRollup?: boolean
  /**
   * Gate the request. Defaults to true (every existing caller wants the
   * fetch immediately). Set false when the scope isn't resolved yet — an
   * unscoped GET /venues is a whole-catalogue page, not a cheap no-op, and
   * the Atlas city view (PSY-1539) has long stretches with no active city.
   */
  enabled?: boolean
}

/**
 * Hook to fetch list of venues with show counts
 */
export const useVenues = (options: UseVenuesOptions = {}) => {
  const {
    state,
    city,
    cities,
    limit = 50,
    offset = 0,
    tags,
    tagMatch,
    includeRail = false,
    metroRollup = false,
    enabled = true,
  } = options

  // The rollup only means anything alongside a single city+state: the API
  // documents itself as ignoring it when `cities` is set. Resolved ONCE, here,
  // because it has to govern the URL and the cache key together — a key that
  // said "metro" over a URL that didn't would split one response across two
  // cache entries that can never disagree, which is pure fragmentation.
  const multiCity = Boolean(cities && cities.length > 0)
  const metroRollupApplies = metroRollup && !multiCity

  // Build query params
  const params = new URLSearchParams()
  if (multiCity) {
    params.set('cities', buildCitiesParam(cities as CityState[]))
  } else {
    if (state) params.set('state', state)
    if (city) params.set('city', city)
  }
  if (limit) params.set('limit', limit.toString())
  if (offset) params.set('offset', offset.toString())
  if (tags && tags.length > 0) {
    params.set('tags', tags.join(','))
    if (tagMatch === 'any') params.set('tag_match', 'any')
  }
  if (includeRail) params.set('include_rail', 'true')
  if (metroRollupApplies) params.set('metro_rollup', 'true')

  const queryString = params.toString()
  const endpoint = queryString
    ? `${venueEndpoints.LIST}?${queryString}`
    : venueEndpoints.LIST

  return useQuery({
    queryKey: venueQueryKeys.list({
      state,
      city,
      cities,
      limit,
      offset,
      tags: tags && tags.length > 0 ? tags : undefined,
      tagMatch: tagMatch === 'any' ? 'any' : undefined,
      // Part of the key: the same city with and without the rail fields are
      // two different payloads and must not share a cache entry.
      includeRail: includeRail || undefined,
      // Same reasoning: "Phoenix" and "the Phoenix metro" are different row
      // sets and must not share a cache entry. Keyed on what was actually
      // SENT, not what was asked for, so a rollup the URL dropped doesn't
      // mint a second entry for a byte-identical request.
      metroRollup: metroRollupApplies || undefined,
    }),
    queryFn: async (): Promise<VenuesListResponse> => {
      return apiRequest<VenuesListResponse>(endpoint, {
        method: 'GET',
      })
    },
    enabled,
    staleTime: 5 * 60 * 1000, // 5 minutes
    placeholderData: keepPreviousData, // Keep old data visible while fetching
  })
}

/**
 * Hook to fetch a single venue by ID or slug
 */
export const useVenue = createNamedDetailHook<Venue, 'venueId'>(
  'venueId',
  venueEndpoints.GET,
  venueQueryKeys.detail,
)

/**
 * Re-exported from `../api`, where the query key that consumes it lives.
 * Kept under this name because every existing caller imports it from here.
 */
export type TimeFilter = VenueShowsTimeFilter

interface UseVenueShowsOptions {
  venueId: string | number
  timezone?: string
  limit?: number
  enabled?: boolean
  timeFilter?: TimeFilter
  /** Rows to skip. Defaults to 0 (the first page). */
  offset?: number
  /**
   * Restrict to one venue-local calendar year. Omit (or pass 0) for every
   * year — the backend treats 0 as "unfiltered", so an explicit 0 is never
   * sent and never keyed.
   */
  year?: number
  /**
   * Hold the previous page's rows on screen while the next one loads, instead
   * of collapsing to a spinner (`placeholderData: keepPreviousData`).
   *
   * OFF by default and deliberately opt-in: it is right for a pager, where the
   * old rows and the new rows answer the same question one slice apart, and
   * wrong for a panel whose venue changed, where it would show one venue's
   * shows under another venue's name. Callers that turn it on must also dim
   * on `isPlaceholderData` so stale rows are never presented as current.
   */
  keepPreviousPage?: boolean
}

/**
 * Hook to fetch shows for a specific venue by ID or slug (lazy-loaded)
 * @param timeFilter - Filter by time: 'upcoming' (default), 'past', or 'all'
 */
export const useVenueShows = (options: UseVenueShowsOptions) => {
  const {
    venueId,
    timezone,
    limit = 20,
    enabled = true,
    timeFilter = 'upcoming',
    offset = 0,
    year,
    keepPreviousPage = false,
  } = options

  // Resolved ONCE, because the URL and the cache key have to be built from the
  // same values or they disagree about what is in the entry. A falsy limit or
  // an empty timezone drops out of the URL and lets the backend default apply,
  // so the key has to record that it was NOT sent rather than the argument the
  // caller happened to pass — otherwise `limit: 0` and `limit: undefined` mint
  // two entries for one identical request. Same rule the venues list hook
  // above states for `metroRollup`: key on what was SENT.
  const sentTimezone = timezone || undefined
  const sentLimit = limit || undefined
  const sentOffset = offset > 0 ? offset : undefined
  // A non-positive or fractional year is not a year. Dropping it here rather
  // than forwarding it means a hand-edited `?year=0`/`?year=-1` URL falls back
  // to the unfiltered archive instead of asking the backend to reject it.
  const sentYear =
    year !== undefined && Number.isInteger(year) && year > 0 ? year : undefined

  const params = new URLSearchParams()
  if (sentTimezone) params.set('timezone', sentTimezone)
  if (sentLimit) params.set('limit', sentLimit.toString())
  if (sentOffset) params.set('offset', sentOffset.toString())
  if (sentYear) params.set('year', sentYear.toString())
  params.set('time_filter', timeFilter)

  const endpoint = `${venueEndpoints.SHOWS(venueId)}?${params.toString()}`

  return useQuery({
    // Keyed on the request, not just the venue: `limit`, `timezone`, `year`
    // and `offset` each change the response body, so callers asking different
    // questions get different entries (PSY-1698, extended by PSY-1753).
    queryKey: venueQueryKeys.showsPage(venueId, {
      timeFilter,
      limit: sentLimit,
      timezone: sentTimezone,
      year: sentYear,
      offset: sentOffset,
    }),
    queryFn: async (): Promise<VenueShowsResponse> => {
      return apiRequest<VenueShowsResponse>(endpoint, {
        method: 'GET',
      })
    },
    enabled: enabled && (typeof venueId === 'string' ? Boolean(venueId) : venueId > 0),
    staleTime: 5 * 60 * 1000, // 5 minutes
    placeholderData: keepPreviousPage ? keepPreviousData : undefined,
  })
}

interface UseVenueShowYearsOptions {
  venueId: string | number
  /** Which side of "today" to count. Defaults to 'past'. */
  timeFilter?: TimeFilter
  enabled?: boolean
}

/**
 * Venue-local calendar years that have at least one show, newest first, with
 * per-year counts (PSY-1753).
 *
 * Cheap and stable relative to the pages it navigates — one row per year — so
 * it is fetched once per venue and reused across every year and page the
 * reader visits, instead of being re-derived from each page's envelope.
 */
export const useVenueShowYears = (options: UseVenueShowYearsOptions) => {
  const { venueId, timeFilter = 'past', enabled = true } = options

  const endpoint = `${venueEndpoints.SHOW_YEARS(venueId)}?time_filter=${timeFilter}`

  return useQuery({
    queryKey: venueQueryKeys.showYears(venueId, timeFilter),
    queryFn: async (): Promise<VenueShowYearsResponse> => {
      return apiRequest<VenueShowYearsResponse>(endpoint, { method: 'GET' })
    },
    enabled:
      enabled && (typeof venueId === 'string' ? Boolean(venueId) : venueId > 0),
    staleTime: 5 * 60 * 1000, // 5 minutes — matches the pages it navigates
  })
}

/**
 * Hook to fetch distinct cities with venue counts for filtering
 */
export const useVenueCities = () => {
  return useQuery({
    queryKey: venueQueryKeys.cities,
    queryFn: async (): Promise<VenueCitiesResponse> => {
      return apiRequest<VenueCitiesResponse>(venueEndpoints.CITIES, {
        method: 'GET',
      })
    },
    staleTime: 10 * 60 * 1000, // 10 minutes - cities don't change often
    placeholderData: keepPreviousData, // Keep old data visible while fetching
  })
}

/**
 * Hook to fetch a venue's genre profile (top 5 genres derived from artist tags)
 */
export const useVenueGenres = (venueIdOrSlug: string | number) => {
  return useQuery({
    queryKey: venueQueryKeys.genres(venueIdOrSlug),
    queryFn: async (): Promise<VenueGenreResponse> => {
      return apiRequest<VenueGenreResponse>(
        venueEndpoints.GENRES(venueIdOrSlug),
        { method: 'GET' }
      )
    },
    enabled: typeof venueIdOrSlug === 'string' ? Boolean(venueIdOrSlug) : venueIdOrSlug > 0,
    staleTime: 10 * 60 * 1000, // 10 minutes — genre profiles change infrequently
  })
}

interface UseVenueBillNetworkOptions {
  venueIdOrSlug: string | number
  /** All-time (default) / rolling 12 months / specific calendar year. */
  window?: VenueBillNetworkWindow
  /** Required when window === 'year'. Hook returns disabled if missing. */
  year?: number
  enabled?: boolean
}

/**
 * Hook to fetch a venue's co-bill network (PSY-365). Mirrors `useSceneGraph`
 * — same shape on the wire (nodes/links/clusters), narrower scope.
 *
 * Edge weights are AT-VENUE shared shows (not global), within the active
 * time window. The default window is "all" (matches the scene graph's
 * "all approved shows" precedent).
 */
export const useVenueBillNetwork = (options: UseVenueBillNetworkOptions) => {
  const { venueIdOrSlug, window = 'all', year, enabled = true } = options

  // Build query params. The backend accepts: window=all|12m|year, year=YYYY.
  const params = new URLSearchParams()
  if (window !== 'all') {
    params.set('window', window)
  }
  if (window === 'year' && year !== undefined) {
    params.set('year', String(year))
  }
  const queryString = params.toString()
  const endpoint = queryString
    ? `${venueEndpoints.BILL_NETWORK(venueIdOrSlug)}?${queryString}`
    : venueEndpoints.BILL_NETWORK(venueIdOrSlug)

  // year is required when window=year; if missing, gate the request rather
  // than send an invalid query that the backend would reject with a 400.
  const hasValidYear = window !== 'year' || (year !== undefined && year > 0)

  return useQuery({
    queryKey: venueQueryKeys.billNetwork(venueIdOrSlug, window, year),
    queryFn: async (): Promise<VenueBillNetworkResponse> => {
      return apiRequest<VenueBillNetworkResponse>(endpoint, { method: 'GET' })
    },
    enabled:
      enabled &&
      hasValidYear &&
      (typeof venueIdOrSlug === 'string'
        ? Boolean(venueIdOrSlug)
        : venueIdOrSlug > 0),
    staleTime: 5 * 60 * 1000, // 5 minutes — match useSceneGraph
    // Window/year changes swap the query key; without previous-data the
    // section's counts collapse to 0 for the fetch duration, which reads as
    // tooSparse and — since the filter row lives INSIDE the fullscreen
    // overlay — would trip useFullscreenGraphOverlay's auto-close and kick
    // the user out of fullscreen mid-interaction (PSY-1305 review finding).
    // Matches the placeholderData the sibling venue hooks already use.
    placeholderData: keepPreviousData,
  })
}
