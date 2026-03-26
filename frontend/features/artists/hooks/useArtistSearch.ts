'use client'

import { createSearchHook } from '@/lib/hooks/factories'
import { artistEndpoints, artistQueryKeys } from '@/features/artists/api'
import type { ArtistSearchResponse } from '../types'

/**
 * Hook for searching artists with debounced input
 * Used for autocomplete in the show submission form
 */
export const useArtistSearch = createSearchHook<ArtistSearchResponse>(
  artistEndpoints.SEARCH,
  artistQueryKeys.search,
)
