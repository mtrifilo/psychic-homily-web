import { useEffect } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { apiRequest, API_ENDPOINTS } from '../../api'
import { queryKeys } from '../../queryClient'
import {
  SHOW_CITIES_FIRST_SCREEN_KEY,
  SHOW_CITIES_FIRST_SCREEN_URL,
  UPCOMING_SHOWS_FIRST_SCREEN_KEY,
  UPCOMING_SHOWS_FIRST_SCREEN_URL,
} from '@/features/shows/api'

/**
 * Prefetches data for /shows and /venues pages during browser idle time.
 * Called from the homepage after initial data loads, so these navigations
 * feel instant (served from TanStack Query cache).
 *
 * The two show entries reuse the `*_FIRST_SCREEN_*` constants rather than
 * rebuilding a key here, so this prefetch provably warms the entry an arriving
 * `/shows` reads instead of one that merely hashes the same today. It could not
 * before PSY-1678: this hook keyed on the viewer's real zone while `/shows`
 * first rendered against a canonical one, so the prefetch was a near-guaranteed
 * miss on arrival. Removing the timezone left one canonical answer for both.
 */
export function usePrefetchRoutes() {
  const queryClient = useQueryClient()

  useEffect(() => {
    const prefetch = () => {
      // Shows page: upcoming list (no limit/cursor = initial page load)
      queryClient.prefetchQuery({
        queryKey: UPCOMING_SHOWS_FIRST_SCREEN_KEY,
        queryFn: () => apiRequest(UPCOMING_SHOWS_FIRST_SCREEN_URL),
        staleTime: 5 * 60 * 1000,
      })

      // Shows page: city filters
      queryClient.prefetchQuery({
        queryKey: SHOW_CITIES_FIRST_SCREEN_KEY,
        queryFn: () => apiRequest(SHOW_CITIES_FIRST_SCREEN_URL),
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
      const id = window.requestIdleCallback(prefetch)
      return () => window.cancelIdleCallback(id)
    } else {
      const id = setTimeout(prefetch, 1000)
      return () => clearTimeout(id)
    }
  }, [queryClient])
}
