'use client'

import { useQuery } from '@tanstack/react-query'
import { apiRequest } from '@/lib/api'
import { radioEndpoints, radioQueryKeys } from '../api'
import type { RadioTopArtistsResponse } from '../types'

interface UseRadioTopArtistsOptions {
  showSlug: string
  period?: number
  limit?: number
  enabled?: boolean
}

/**
 * Hook to fetch top artists for a radio show
 */
export function useRadioTopArtists({
  showSlug,
  period = 90,
  limit = 20,
  enabled = true,
}: UseRadioTopArtistsOptions) {
  const params = new URLSearchParams()
  if (period) params.set('period', period.toString())
  if (limit) params.set('limit', limit.toString())

  const queryString = params.toString()
  const endpoint = queryString
    ? `${radioEndpoints.SHOW_TOP_ARTISTS(showSlug)}?${queryString}`
    : radioEndpoints.SHOW_TOP_ARTISTS(showSlug)

  return useQuery({
    queryKey: radioQueryKeys.topArtists(showSlug, { period, limit }),
    queryFn: () =>
      apiRequest<RadioTopArtistsResponse>(endpoint, {
        method: 'GET',
      }),
    enabled: enabled && showSlug.length > 0,
    staleTime: 5 * 60 * 1000,
  })
}
