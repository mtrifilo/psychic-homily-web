/**
 * Hook Factories
 *
 * Factory functions for creating standardized TanStack Query hooks.
 * Eliminates boilerplate for common patterns: detail-by-ID/slug and search.
 *
 * Usage:
 *   const useRelease = createDetailHook<ReleaseDetail>(
 *     API_ENDPOINTS.RELEASES.GET,
 *     queryKeys.releases.detail,
 *   )
 *   // Returns: (options: { idOrSlug: string | number; enabled?: boolean }) => UseQueryResult<ReleaseDetail>
 */

import { useQuery } from '@tanstack/react-query'
import { useDebounce } from 'use-debounce'
import { apiRequest } from '@/lib/api'

// Standard enabled check for ID/slug values used across all detail hooks
function isValidIdOrSlug(value: string | number): boolean {
  return typeof value === 'string' ? value.length > 0 : value > 0
}

/**
 * Create a detail hook that fetches an entity by ID or slug.
 *
 * Produced hook signature:
 *   (options: { idOrSlug: string | number; enabled?: boolean }) => UseQueryResult<T>
 *
 * Includes standard enabled logic: disabled when idOrSlug is empty string or <= 0.
 */
export function createDetailHook<T>(
  endpoint: (idOrSlug: string | number) => string,
  queryKey: (idOrSlug: string | number) => readonly unknown[],
  factoryOptions?: { staleTime?: number }
) {
  return function useDetail(options: {
    idOrSlug: string | number
    enabled?: boolean
  }) {
    const { idOrSlug, enabled = true } = options
    return useQuery({
      queryKey: queryKey(idOrSlug),
      queryFn: () => apiRequest<T>(endpoint(idOrSlug), { method: 'GET' }),
      enabled: enabled && isValidIdOrSlug(idOrSlug),
      staleTime: factoryOptions?.staleTime ?? 5 * 60 * 1000,
    })
  }
}

/**
 * Create a detail hook where the ID/slug param has a custom name in the options object.
 *
 * Produced hook signature varies by paramName — for example with paramName='venueId':
 *   (options: { venueId: string | number; enabled?: boolean }) => UseQueryResult<T>
 *
 * This preserves existing call-site signatures like useVenue({ venueId: 'slug' }).
 */
export function createNamedDetailHook<
  T,
  K extends string,
>(
  paramName: K,
  endpoint: (idOrSlug: string | number) => string,
  queryKey: (idOrSlug: string | number) => readonly unknown[],
  factoryOptions?: { staleTime?: number }
) {
  return function useDetail(
    options: { enabled?: boolean } & Record<K, string | number>
  ) {
    const idOrSlug = options[paramName]
    const enabled = options.enabled ?? true
    return useQuery({
      queryKey: queryKey(idOrSlug),
      queryFn: () => apiRequest<T>(endpoint(idOrSlug), { method: 'GET' }),
      enabled: enabled && isValidIdOrSlug(idOrSlug),
      staleTime: factoryOptions?.staleTime ?? 5 * 60 * 1000,
    })
  }
}

/**
 * Create the query-options builder a search surface shares between its
 * reactive hook and imperative `queryClient.fetchQuery` callers — the single
 * owner of the search cache contract (key + URL + stale/gc tuning), so the
 * two paths can't drift. `createSearchHook` builds on this; feature modules
 * can export a specialization for imperative lookups (see
 * features/artists/hooks/useArtistSearch.ts).
 */
export function createSearchQueryOptions<T>(
  searchEndpoint: string,
  queryKey: (query: string) => readonly unknown[],
) {
  return function searchQueryOptions(query: string) {
    return {
      queryKey: queryKey(query),
      queryFn: () =>
        apiRequest<T>(`${searchEndpoint}?q=${encodeURIComponent(query)}`),
      staleTime: 5 * 60 * 1000,
      gcTime: 30 * 60 * 1000,
    }
  }
}

/**
 * Create a search hook with debounced input.
 *
 * Produced hook signature:
 *   (options: { query: string; debounceMs?: number }) => UseQueryResult<T>
 *
 * Query is auto-disabled when the search string is empty.
 * Includes gcTime of 30 minutes to keep results cached across searches.
 */
export function createSearchHook<T>(
  searchEndpoint: string,
  queryKey: (query: string) => readonly unknown[],
) {
  const searchQueryOptions = createSearchQueryOptions<T>(searchEndpoint, queryKey)
  return function useSearch(options: {
    query: string
    debounceMs?: number
  }) {
    const { query, debounceMs = 50 } = options
    const [debouncedQuery] = useDebounce(query, debounceMs)

    return useQuery({
      ...searchQueryOptions(debouncedQuery),
      enabled: debouncedQuery.length > 0,
    })
  }
}
