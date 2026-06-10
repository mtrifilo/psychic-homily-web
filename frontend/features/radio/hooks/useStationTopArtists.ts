'use client'

import { useQuery } from '@tanstack/react-query'
import { apiRequest } from '@/lib/api'
import { radioEndpoints, radioQueryKeys } from '../api'
import type { RadioTopArtistsResponse } from '../types'

interface UseStationTopArtistsOptions {
  stationSlug: string
  /** Period in days (default 90). */
  period?: number
  limit?: number
  enabled?: boolean
}

/**
 * A station's most-played artists across all of its shows (PSY-1048). Same
 * response shape as the show-level top-artists endpoint.
 */
export function useStationTopArtists({
  stationSlug,
  period = 90,
  limit = 8,
  enabled = true,
}: UseStationTopArtistsOptions) {
  const params = new URLSearchParams()
  if (period) params.set('period', period.toString())
  if (limit) params.set('limit', limit.toString())

  const queryString = params.toString()
  const endpoint = queryString
    ? `${radioEndpoints.STATION_TOP_ARTISTS(stationSlug)}?${queryString}`
    : radioEndpoints.STATION_TOP_ARTISTS(stationSlug)

  return useQuery({
    queryKey: radioQueryKeys.stationTopArtists(stationSlug, { period, limit }),
    queryFn: () =>
      apiRequest<RadioTopArtistsResponse>(endpoint, {
        method: 'GET',
      }),
    enabled: enabled && stationSlug.length > 0,
    staleTime: 5 * 60 * 1000,
  })
}
