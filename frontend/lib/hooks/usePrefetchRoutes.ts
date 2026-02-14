import { useEffect } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { apiRequest, API_ENDPOINTS } from '../api'
import { queryKeys } from '../queryClient'

/**
 * Prefetches data for /shows and /venues pages during browser idle time.
 * Called from the homepage after initial data loads, so these navigations
 * feel instant (served from TanStack Query cache).
 */
export function usePrefetchRoutes(timezone: string) {
  const queryClient = useQueryClient()

  useEffect(() => {
    const prefetch = () => {
      // Shows page: upcoming list (no limit/cursor = initial page load)
      queryClient.prefetchQuery({
        queryKey: queryKeys.shows.list({ timezone }),
        queryFn: () => {
          const params = new URLSearchParams({ timezone })
          return apiRequest(`${API_ENDPOINTS.SHOWS.UPCOMING}?${params}`)
        },
        staleTime: 5 * 60 * 1000,
      })

      // Shows page: city filters
      queryClient.prefetchQuery({
        queryKey: queryKeys.shows.cities(timezone),
        queryFn: () => {
          const params = new URLSearchParams({ timezone })
          return apiRequest(`${API_ENDPOINTS.SHOWS.CITIES}?${params}`)
        },
        staleTime: 5 * 60 * 1000,
      })

      // Venues page: initial list (limit=50, offset=0)
      queryClient.prefetchQuery({
        queryKey: queryKeys.venues.list({ limit: 50, offset: 0 }),
        queryFn: () =>
          apiRequest(`${API_ENDPOINTS.VENUES.LIST}?limit=50`),
        staleTime: 5 * 60 * 1000,
      })

      // Venues page: city filters
      queryClient.prefetchQuery({
        queryKey: queryKeys.venues.cities,
        queryFn: () => apiRequest(API_ENDPOINTS.VENUES.CITIES),
        staleTime: 10 * 60 * 1000,
      })
    }

    // Defer to idle time to avoid competing with rendering
    if ('requestIdleCallback' in window) {
      const id = requestIdleCallback(prefetch)
      return () => cancelIdleCallback(id)
    } else {
      const id = setTimeout(prefetch, 1000)
      return () => clearTimeout(id)
    }
  }, [queryClient, timezone])
}
