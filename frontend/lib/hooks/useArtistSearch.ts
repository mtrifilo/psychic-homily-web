'use client'

import { useQuery } from '@tanstack/react-query'
import { useDebounce } from 'use-debounce'
import { apiRequest, API_ENDPOINTS } from '../api'
import { queryKeys } from '../queryClient'
import type { ArtistSearchResponse } from '../types/artist'

interface UseArtistSearchOptions {
  query: string
  debounceMs?: number
}

/**
 * Hook for searching artists with debounced input
 * Used for autocomplete in the show submission form
 */
export function useArtistSearch({
  query,
  debounceMs = 50,
}: UseArtistSearchOptions) {
  const [debouncedQuery] = useDebounce(query, debounceMs)

  return useQuery({
    queryKey: queryKeys.artists.search(debouncedQuery),
    queryFn: async (): Promise<ArtistSearchResponse> => {
      const url = `${API_ENDPOINTS.ARTISTS.SEARCH}?q=${encodeURIComponent(debouncedQuery)}`
      return apiRequest<ArtistSearchResponse>(url)
    },
    enabled: debouncedQuery.length > 0,
    staleTime: 5 * 60 * 1000, // 5 minutes - artist data rarely changes
    gcTime: 30 * 60 * 1000, // 30 minutes - keep in memory longer
  })
}
