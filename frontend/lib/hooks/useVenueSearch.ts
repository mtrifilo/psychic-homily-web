'use client'

import { useQuery } from '@tanstack/react-query'
import { useDebounce } from 'use-debounce'
import { apiRequest, API_ENDPOINTS } from '../api'
import { queryKeys } from '../queryClient'
import type { VenueSearchResponse } from '../types/venue'

interface UseVenueSearchOptions {
  query: string
  debounceMs?: number
}

/**
 * Hook for searching venues with debounced input
 * Used for autocomplete in the show submission form
 */
export function useVenueSearch({
  query,
  debounceMs = 50,
}: UseVenueSearchOptions) {
  const [debouncedQuery] = useDebounce(query, debounceMs)

  return useQuery({
    queryKey: queryKeys.venues.search(debouncedQuery),
    queryFn: async (): Promise<VenueSearchResponse> => {
      const url = `${API_ENDPOINTS.VENUES.SEARCH}?q=${encodeURIComponent(debouncedQuery)}`
      return apiRequest<VenueSearchResponse>(url)
    },
    enabled: debouncedQuery.length > 0,
    staleTime: 5 * 60 * 1000, // 5 minutes - venue data rarely changes
    gcTime: 30 * 60 * 1000, // 30 minutes - keep in memory longer
  })
}

