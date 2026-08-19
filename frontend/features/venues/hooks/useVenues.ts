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
import { ARCHIVE_YEAR_RANGE } from '@/features/shows/showArchive'
import type { VenueShowsTimeFilter } from '@/features/venues/api'
import { buildCitiesParam } from '@/components/filters/cityParams'
import type { CityState } from '@/components/filters/CityFilters'
import type {
  Venue,
  VenuesListResponse,
  VenueShowsResponse,
  VenueShowYearsResponse,
  VenueShowMonthsResponse,
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
  /**
   * Rows the SERVER already fetched for exactly this request, so the first
   * client render has them and the archive reaches the HTML (PSY-1756).
   *
   * The caller is responsible for passing it ONLY when the current arguments
   * describe the same request the server made — react-query attaches
   * `initialData` to whatever key is current, so handing page 1's rows to a
   * hook that is asking for page 2 would seed page 2 with page 1's content.
   *
   * Seeded rather than hydrated through a `HydrationBoundary` on purpose: the
   * key is built here, in the browser, from the browser's own arguments, so a
   * mismatch is impossible by construction rather than merely unlikely. No
   * `initialDataUpdatedAt` — the rows are treated as fresh for the usual
   * staleTime, which is the point: an archive page must not refetch what it
   * just server-rendered.
   */
  initialData?: VenueShowsResponse
}

/**
 * Hook to fetch shows for a specific venue by ID or slug (lazy-loaded)
 * @param timeFilter - Filter by time: 'upcoming' (default), 'past', or 'all'
 */
export const useVenueShows = (options: UseVenueShowsOptions) => {
  const {
    venueId,
    limit = 20,
    enabled = true,
    timeFilter = 'upcoming',
    offset = 0,
    year,
    keepPreviousPage = false,
    initialData,
  } = options

  // Resolved ONCE, because the URL and the cache key have to be built from the
  // same values or they disagree about what is in the entry. Anything falsy
  // drops out of the URL and lets the backend default apply, so the key records
  // what was SENT rather than the argument the caller passed. (`limit: 0` and
  // an omitted limit are two different requests — the hook defaults an omitted
  // limit to 20 and sends it — so they get two entries.) Same rule the venues
  // list hook above states for `metroRollup`: key on what was SENT.
  //
  // The load-bearing case is `offset`: page 1's zero offset must key as "not
  // sent", or the key the year-archive route's server-seeded `initialShows`
  // lands on is not the key this hook registers, and the rows silently drop out
  // of the served HTML. That is now the ONLY reason it is load-bearing: both
  // archives label their pages from a month histogram (PSY-1769, PSY-1842), so
  // neither reads a neighbouring page's cache entry any more.
  const sentTimeFilter = timeFilter || 'upcoming'
  const sentLimit = limit || undefined
  const sentOffset = offset > 0 ? offset : undefined
  // Last line of defence, not the URL guard. Callers own year validation — the
  // archive route runs `parseArchiveYear` (showArchive.ts) over its `{year}`
  // path segment before it ever reaches here, against the same bounds the
  // backend enforces. This drops anything outside those bounds so no caller can
  // turn a stray argument into a 422.
  const sentYear =
    year !== undefined &&
    Number.isInteger(year) &&
    year >= ARCHIVE_YEAR_RANGE.min &&
    year <= ARCHIVE_YEAR_RANGE.max
      ? year
      : undefined

  const params = new URLSearchParams()
  if (sentLimit) params.set('limit', sentLimit.toString())
  if (sentOffset) params.set('offset', sentOffset.toString())
  if (sentYear) params.set('year', sentYear.toString())
  params.set('time_filter', sentTimeFilter)

  const endpoint = `${venueEndpoints.SHOWS(venueId)}?${params.toString()}`

  return useQuery({
    // Keyed on the request, not just the venue: `limit`, `year` and `offset`
    // each change the response body, so callers asking different questions get
    // different entries (PSY-1698, extended by PSY-1753).
    queryKey: venueQueryKeys.showsPage(venueId, {
      timeFilter: sentTimeFilter,
      limit: sentLimit,
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
    initialData,
  })
}

interface UseVenueShowYearsOptions {
  venueId: string | number
  /** Which side of "today" to count. Defaults to 'past'. */
  timeFilter?: TimeFilter
  enabled?: boolean
  /**
   * The histogram the SERVER already fetched, so the year strip renders in the
   * HTML instead of appearing after the first client fetch (PSY-1756). Same
   * contract as `useVenueShows`'s: pass it only for the arguments it was
   * fetched under.
   */
  initialData?: VenueShowYearsResponse
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
  const { venueId, timeFilter = 'past', enabled = true, initialData } = options

  const endpoint = `${venueEndpoints.SHOW_YEARS(venueId)}?time_filter=${timeFilter}`

  return useQuery({
    queryKey: venueQueryKeys.showYears(venueId, timeFilter),
    queryFn: async (): Promise<VenueShowYearsResponse> => {
      return apiRequest<VenueShowYearsResponse>(endpoint, { method: 'GET' })
    },
    enabled:
      enabled && (typeof venueId === 'string' ? Boolean(venueId) : venueId > 0),
    staleTime: 5 * 60 * 1000, // 5 minutes — matches the pages it navigates
    initialData,
  })
}

interface UseVenueShowMonthsOptions {
  venueId: string | number
  /** Which side of "today" to count. Defaults to 'past'. */
  timeFilter?: TimeFilter
  enabled?: boolean
  /**
   * The histogram the SERVER already fetched, so the pager's labels are in the
   * HTML rather than popping in after the first client fetch (PSY-1769). Same
   * contract as `useVenueShowYears`'s: pass it only for the arguments it was
   * fetched under.
   */
  initialData?: VenueShowMonthsResponse
}

/**
 * Venue-local calendar months that have at least one show, newest first, with
 * per-month counts (PSY-1769).
 *
 * What the past-shows pager labels its page links from. Cumulative counts place
 * every page's month span at once, so the label under page 6 is there on first
 * paint rather than only after the reader has been to page 6.
 *
 * One request per venue, whatever year the reader is looking at: the histogram
 * spans every year and the year-scoped views slice it. Twelve times the rows of
 * the year histogram beside it, still one small row per month a venue has ever
 * booked, and it does not change as the reader pages.
 *
 * Seeded server-side on the YEAR-ARCHIVE route only, where the pager really is
 * in the served HTML and a label that arrived with a client fetch would be one a
 * crawler never sees. The venue page renders the archive after its first client
 * fetch, so there is no pager in its document to label and no seed worth paying
 * a full-history aggregate for.
 */
export const useVenueShowMonths = (options: UseVenueShowMonthsOptions) => {
  const { venueId, timeFilter = 'past', enabled = true, initialData } = options

  const endpoint = `${venueEndpoints.SHOW_MONTHS(venueId)}?time_filter=${timeFilter}`

  return useQuery({
    queryKey: venueQueryKeys.showMonths(venueId, timeFilter),
    queryFn: async (): Promise<VenueShowMonthsResponse> => {
      return apiRequest<VenueShowMonthsResponse>(endpoint, { method: 'GET' })
    },
    enabled:
      enabled && (typeof venueId === 'string' ? Boolean(venueId) : venueId > 0),
    staleTime: 5 * 60 * 1000, // 5 minutes — matches the pages it labels
    initialData,
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
