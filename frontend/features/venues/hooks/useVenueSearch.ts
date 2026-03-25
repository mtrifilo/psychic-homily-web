'use client'

import { useQuery } from '@tanstack/react-query'
import { useDebounce } from 'use-debounce'
import { apiRequest } from '@/lib/api'
import { venueEndpoints, venueQueryKeys } from '../api'
import type { VenueSearchResponse } from '../types'

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
    queryKey: venueQueryKeys.search(debouncedQuery),
    queryFn: async (): Promise<VenueSearchResponse> => {
      const url = `${venueEndpoints.SEARCH}?q=${encodeURIComponent(debouncedQuery)}`
      return apiRequest<VenueSearchResponse>(url)
    },
    enabled: debouncedQuery.length > 0,
    staleTime: 5 * 60 * 1000, // 5 minutes - venue data rarely changes
    gcTime: 30 * 60 * 1000, // 30 minutes - keep in memory longer
  })
}
