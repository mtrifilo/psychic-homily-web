'use client'

import { useQuery } from '@tanstack/react-query'
import { apiRequest } from '@/lib/api'
import { radioEndpoints, radioQueryKeys } from '../api'
import type { RadioNewReleasesResponse } from '../types'

interface UseNewReleaseRadarOptions {
  stationId?: number
  limit?: number
  enabled?: boolean
}

/**
 * Hook to fetch new releases discovered via radio
 */
export function useNewReleaseRadar({
  stationId,
  limit = 20,
  enabled = true,
}: UseNewReleaseRadarOptions = {}) {
  const params = new URLSearchParams()
  if (stationId) params.set('station_id', stationId.toString())
  if (limit) params.set('limit', limit.toString())

  const queryString = params.toString()
  const endpoint = queryString
    ? `${radioEndpoints.NEW_RELEASES}?${queryString}`
    : radioEndpoints.NEW_RELEASES

  return useQuery({
    queryKey: radioQueryKeys.newReleases({ stationId, limit }),
    queryFn: () =>
      apiRequest<RadioNewReleasesResponse>(endpoint, {
        method: 'GET',
      }),
    enabled,
    staleTime: 5 * 60 * 1000,
  })
}
