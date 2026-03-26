'use client'

import { createSearchHook } from '@/lib/hooks/factories'
import { venueEndpoints, venueQueryKeys } from '@/features/venues/api'
import type { VenueSearchResponse } from '../types'

/**
 * Hook for searching venues with debounced input
 * Used for autocomplete in the show submission form
 */
export const useVenueSearch = createSearchHook<VenueSearchResponse>(
  venueEndpoints.SEARCH,
  venueQueryKeys.search,
)
