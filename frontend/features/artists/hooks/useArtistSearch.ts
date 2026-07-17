'use client'

import { useQuery } from '@tanstack/react-query'
import { useDebounce } from 'use-debounce'

import { apiRequest } from '@/lib/api'
import { artistEndpoints, artistQueryKeys } from '@/features/artists/api'
import type { ArtistSearchResponse } from '../types'

/**
 * Single owner of the artist-search cache contract — key + queryFn +
 * stale/gc tuning in one place (same pattern as `graphQueryOptions` in
 * useArtistGraph.ts). Used by the reactive `useArtistSearch` hook AND by
 * imperative `queryClient.fetchQuery` callers (the Observatory's clickable
 * zero-state example), so the two paths can't drift on the cache key, URL,
 * or lifetimes. Values mirror `createSearchHook` in lib/hooks/factories.ts.
 */
export function artistSearchQueryOptions(query: string) {
  return {
    queryKey: artistQueryKeys.search(query),
    queryFn: () =>
      apiRequest<ArtistSearchResponse>(
        `${artistEndpoints.SEARCH}?q=${encodeURIComponent(query)}`
      ),
    staleTime: 5 * 60 * 1000,
    gcTime: 30 * 60 * 1000,
  }
}

/**
 * Hook for searching artists with debounced input.
 * Used for autocomplete in the show submission form and graph surfaces.
 */
export function useArtistSearch(options: { query: string; debounceMs?: number }) {
  const { query, debounceMs = 50 } = options
  const [debouncedQuery] = useDebounce(query, debounceMs)

  return useQuery({
    ...artistSearchQueryOptions(debouncedQuery),
    enabled: debouncedQuery.length > 0,
  })
}
