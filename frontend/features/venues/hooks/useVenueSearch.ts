'use client'

import { createSearchHook } from '@/lib/hooks/factories'
import { API_ENDPOINTS } from '@/lib/api'
import { queryKeys } from '@/lib/queryClient'
import type { VenueSearchResponse } from '../types'

/**
 * Hook for searching venues with debounced input
 * Used for autocomplete in the show submission form
 */
export const useVenueSearch = createSearchHook<VenueSearchResponse>(
  API_ENDPOINTS.VENUES.SEARCH,
  queryKeys.venues.search,
)
