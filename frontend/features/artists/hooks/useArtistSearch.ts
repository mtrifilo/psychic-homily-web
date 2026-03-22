'use client'

import { createSearchHook } from '@/lib/hooks/factories'
import { API_ENDPOINTS } from '@/lib/api'
import { queryKeys } from '@/lib/queryClient'
import type { ArtistSearchResponse } from '../types'

/**
 * Hook for searching artists with debounced input
 * Used for autocomplete in the show submission form
 */
export const useArtistSearch = createSearchHook<ArtistSearchResponse>(
  API_ENDPOINTS.ARTISTS.SEARCH,
  queryKeys.artists.search,
)
