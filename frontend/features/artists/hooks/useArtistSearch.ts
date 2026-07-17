'use client'

import { createSearchHook, createSearchQueryOptions } from '@/lib/hooks/factories'
import { artistEndpoints, artistQueryKeys } from '@/features/artists/api'
import type { ArtistSearchResponse } from '../types'

/**
 * Single owner of the artist-search cache contract (key + queryFn +
 * lifetimes), derived from the same factory the sibling search hooks use.
 * Exported for imperative `queryClient.fetchQuery` callers (the Observatory's
 * clickable zero-state example) so they share the hook's cache exactly.
 */
export const artistSearchQueryOptions = createSearchQueryOptions<ArtistSearchResponse>(
  artistEndpoints.SEARCH,
  artistQueryKeys.search,
)

/**
 * Hook for searching artists with debounced input.
 * Used for autocomplete in the show submission form and graph surfaces.
 */
export const useArtistSearch = createSearchHook<ArtistSearchResponse>(
  artistEndpoints.SEARCH,
  artistQueryKeys.search,
)
