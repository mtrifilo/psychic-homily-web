'use client'

import { useQuery } from '@tanstack/react-query'

import { apiRequest, API_ENDPOINTS } from '@/lib/api'
import { queryKeys } from '@/lib/queryClient'
import type { RandomArtistTargetResponse } from './types'

export type { RandomArtistTargetResponse } from './types'

/**
 * Interaction-driven random artist lookup shared by discovery surfaces.
 * Disabled on mount so every explicit refetch asks the backend for a fresh pick.
 */
export function useRandomArtistTarget() {
  return useQuery({
    queryKey: queryKeys.discovery.randomArtistTarget,
    queryFn: () =>
      apiRequest<RandomArtistTargetResponse>(
        API_ENDPOINTS.DISCOVERY.RANDOM_ARTIST_TARGET,
        { method: 'GET' },
      ),
    enabled: false,
    staleTime: 0,
    gcTime: 0,
  })
}
