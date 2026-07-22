'use client'

import { useQuery } from '@tanstack/react-query'
import { apiRequest, API_ENDPOINTS } from '@/lib/api'
import type { CommunityPulseResponse } from '../types'

export const communityPulseQueryKey = ['community', 'pulse'] as const

/**
 * Global homepage pulse counts (PSY-1431). Same numbers for every visitor —
 * no auth, no scene scope. staleTime mirrors the charts masthead budget
 * (server TTL 1m + Cache-Control max-age=30).
 */
export function useCommunityPulse() {
  return useQuery({
    queryKey: communityPulseQueryKey,
    queryFn: () =>
      apiRequest<CommunityPulseResponse>(API_ENDPOINTS.COMMUNITY.PULSE, {
        method: 'GET',
      }),
    staleTime: 60_000,
  })
}
