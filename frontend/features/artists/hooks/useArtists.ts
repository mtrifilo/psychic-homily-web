'use client'

/**
 * Artists Hooks
 *
 * TanStack Query hooks for fetching artist data from the API.
 */

import { useQuery, keepPreviousData } from '@tanstack/react-query'
import { apiRequest } from '@/lib/api'
import { createNamedDetailHook } from '@/lib/hooks/factories'
import {
  ARTIST_LIST_PAGE_LIMIT,
  artistEndpoints,
  artistQueryKeys,
} from '@/features/artists/api'
import { ARCHIVE_YEAR_RANGE } from '@/features/shows/showArchive'
import { buildCitiesParam } from '@/components/filters/cityParams'
import type { CityState } from '@/components/filters'
import type {
  Artist,
  ArtistsListResponse,
  ArtistCitiesResponse,
  ArtistShowsResponse,
  ArtistShowMonthsResponse,
  ArtistShowYearsResponse,
  ArtistTimeFilter,
} from '../types'

interface UseArtistsOptions {
  cities?: CityState[]
  /** Multi-tag filter (PSY-309). Slugs applied with AND by default. */
  tags?: string[]
  /** Set to 'any' to switch the tag filter to OR semantics. */
  tagMatch?: 'all' | 'any'
  /** Rows per page. Defaults to the browse page size (PSY-1774). */
  limit?: number
  /** Rows to skip. Defaults to 0 (the first page). */
  offset?: number
}

/**
 * Hook to fetch one page of artists, with optional city and tag filtering.
 *
 * Paged rather than whole since PSY-1774. A caller that names no page still
 * gets one: an unbounded request is a request for the whole catalogue.
 */
export function useArtists(options: UseArtistsOptions = {}) {
  const {
    cities,
    tags,
    tagMatch,
    limit = ARTIST_LIST_PAGE_LIMIT,
    offset = 0,
  } = options

  // Build query params
  const params = new URLSearchParams()
  if (cities && cities.length > 0) {
    params.set('cities', buildCitiesParam(cities))
  }
  if (limit) params.set('limit', limit.toString())
  if (offset) params.set('offset', offset.toString())
  if (tags && tags.length > 0) {
    params.set('tags', tags.join(','))
    if (tagMatch === 'any') params.set('tag_match', 'any')
  }

  const queryString = params.toString()
  const endpoint = queryString
    ? `${artistEndpoints.LIST}?${queryString}`
    : artistEndpoints.LIST

  return useQuery({
    // `limit` and `offset` are part of the key: two pages of the same filter
    // are different payloads and must not share an entry. They are keyed
    // unconditionally, including `offset: 0`, which is what
    // `ARTIST_LIST_FIRST_SCREEN_KEY` mirrors — see that constant for why the
    // key and the URL are allowed to differ on a zero offset.
    queryKey: artistQueryKeys.list({
      cities: cities ?? undefined,
      tags: tags && tags.length > 0 ? tags : undefined,
      tagMatch: tagMatch === 'any' ? 'any' : undefined,
      limit,
      offset,
    } as Record<string, unknown>),
    queryFn: async (): Promise<ArtistsListResponse> => {
      return apiRequest<ArtistsListResponse>(endpoint, {
        method: 'GET',
      })
    },
    staleTime: 5 * 60 * 1000, // 5 minutes
    placeholderData: keepPreviousData,
  })
}

/**
 * Hook to fetch distinct cities with artist counts for filtering
 */
export function useArtistCities() {
  return useQuery({
    queryKey: artistQueryKeys.cities,
    queryFn: async (): Promise<ArtistCitiesResponse> => {
      return apiRequest<ArtistCitiesResponse>(artistEndpoints.CITIES, {
        method: 'GET',
      })
    },
    staleTime: 10 * 60 * 1000, // 10 minutes - cities don't change often
    placeholderData: keepPreviousData,
  })
}

/**
 * Hook to fetch a single artist by ID or slug
 */
export const useArtist = createNamedDetailHook<Artist, 'artistId'>(
  'artistId',
  artistEndpoints.GET,
  artistQueryKeys.detail,
)

interface UseArtistShowsOptions {
  artistId: string | number
  limit?: number
  enabled?: boolean
  timeFilter?: ArtistTimeFilter
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
   * wrong for a panel whose artist changed, where it would show one artist's
   * shows under another artist's name. Callers that turn it on must also dim
   * on `isPlaceholderData` so stale rows are never presented as current.
   */
  keepPreviousPage?: boolean
}

/**
 * Hook to fetch shows for a specific artist by ID or slug
 * @param timeFilter - Filter by time: 'upcoming' (default), 'past', or 'all'
 */
export function useArtistShows(options: UseArtistShowsOptions) {
  const {
    artistId,
    limit = 20,
    enabled = true,
    timeFilter = 'upcoming',
    offset = 0,
    year,
    keepPreviousPage = false,
  } = options

  // Resolved ONCE, because the URL and the cache key have to be built from the
  // same values or they disagree about what is in the entry. Anything falsy
  // drops out of the URL and lets the backend default apply, so the key records
  // what was SENT rather than the argument the caller passed.
  //
  // The load-bearing case is `offset`. Page 1's offset is 0, which is not sent,
  // and `artistPastShowsPageParams` has to say so with `undefined` or the key it
  // describes is not the key this hook registers. Nothing throws when they
  // drift; anything seeding or reading that entry just misses it.
  //
  // The archive used to have a second stake in this — it read neighbouring
  // pages' cache entries to label them — and no longer does: labels come from a
  // month histogram since PSY-1842, like the venue twin's.
  const sentLimit = limit || undefined
  const sentOffset = offset > 0 ? offset : undefined
  // Last line of defence, not the URL guard. Callers own year validation —
  // the artist archive runs `parseArchiveYear` over the raw `?year=` before it
  // ever reaches here, against the same bounds the backend enforces. This drops
  // anything outside those bounds so no caller can turn a stray argument into
  // a 422.
  const sentYear =
    year !== undefined &&
    Number.isInteger(year) &&
    year >= ARCHIVE_YEAR_RANGE.min &&
    year <= ARCHIVE_YEAR_RANGE.max
      ? year
      : undefined

  // Build query params
  const params = new URLSearchParams()
  if (sentLimit) params.set('limit', sentLimit.toString())
  if (sentOffset) params.set('offset', sentOffset.toString())
  if (sentYear) params.set('year', sentYear.toString())
  if (timeFilter) params.set('time_filter', timeFilter)

  const queryString = params.toString()
  const endpoint = queryString
    ? `${artistEndpoints.SHOWS(artistId)}?${queryString}`
    : artistEndpoints.SHOWS(artistId)

  return useQuery({
    // Keyed on the request, not just the artist: `limit`, `year` and `offset`
    // each change the response body, so callers asking different questions get
    // different entries (PSY-1754).
    queryKey: artistQueryKeys.showsPage(artistId, {
      timeFilter,
      limit: sentLimit,
      year: sentYear,
      offset: sentOffset,
    }),
    queryFn: async (): Promise<ArtistShowsResponse> => {
      return apiRequest<ArtistShowsResponse>(endpoint, {
        method: 'GET',
      })
    },
    enabled: enabled && (typeof artistId === 'string' ? Boolean(artistId) : artistId > 0),
    staleTime: 5 * 60 * 1000, // 5 minutes
    placeholderData: keepPreviousPage ? keepPreviousData : undefined,
  })
}

interface UseArtistShowYearsOptions {
  artistId: string | number
  /** Which side of "today" to count. Defaults to 'past'. */
  timeFilter?: ArtistTimeFilter
  enabled?: boolean
}

/**
 * Venue-local calendar years in which this artist played at least one show,
 * newest first, with per-year counts (PSY-1754).
 *
 * Cheap and stable relative to the pages it navigates — one row per year — so
 * it is fetched once per artist and reused across every year and page the
 * reader visits, instead of being re-derived from each page's envelope.
 */
export function useArtistShowYears(options: UseArtistShowYearsOptions) {
  const { artistId, timeFilter = 'past', enabled = true } = options

  const endpoint = `${artistEndpoints.SHOW_YEARS(artistId)}?time_filter=${timeFilter}`

  return useQuery({
    queryKey: artistQueryKeys.showYears(artistId, timeFilter),
    queryFn: async (): Promise<ArtistShowYearsResponse> => {
      return apiRequest<ArtistShowYearsResponse>(endpoint, { method: 'GET' })
    },
    enabled:
      enabled && (typeof artistId === 'string' ? Boolean(artistId) : artistId > 0),
    staleTime: 5 * 60 * 1000, // 5 minutes — matches the pages it navigates
  })
}

interface UseArtistShowMonthsOptions {
  artistId: string | number
  /** Which side of "today" to count. Defaults to 'past'. */
  timeFilter?: ArtistTimeFilter
  enabled?: boolean
}

/**
 * Venue-local calendar months in which this artist played at least one show,
 * newest first, with per-month counts (PSY-1842).
 *
 * What the past-shows pager labels its page links from. Cumulative counts place
 * every page's month span at once, so the label under page 6 is there on first
 * paint rather than only after the reader has been to page 6 — which is what the
 * artist archive did before this, leaving unvisited pages as bare numerals.
 *
 * One request per artist, whatever year the reader is looking at: the histogram
 * spans every year and the year-scoped views slice it. Twelve times the rows of
 * the year histogram beside it, still one small row per month the artist has ever
 * played, and it does not change as the reader pages.
 *
 * NO server seed, unlike the venue twin. The artist page renders its archive
 * after the first client fetch, so there is no pager in its document to label —
 * the venue's seed exists for the year-archive ROUTE, which the artist has no
 * equivalent of yet. When it gets one, this is the hook that takes an
 * `initialData` prop.
 */
export function useArtistShowMonths(options: UseArtistShowMonthsOptions) {
  const { artistId, timeFilter = 'past', enabled = true } = options

  const endpoint = `${artistEndpoints.SHOW_MONTHS(artistId)}?time_filter=${timeFilter}`

  return useQuery({
    queryKey: artistQueryKeys.showMonths(artistId, timeFilter),
    queryFn: async (): Promise<ArtistShowMonthsResponse> => {
      return apiRequest<ArtistShowMonthsResponse>(endpoint, { method: 'GET' })
    },
    enabled:
      enabled && (typeof artistId === 'string' ? Boolean(artistId) : artistId > 0),
    staleTime: 5 * 60 * 1000, // 5 minutes — matches the pages it labels
  })
}
