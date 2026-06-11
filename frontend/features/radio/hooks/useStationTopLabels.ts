'use client'

import { useQuery } from '@tanstack/react-query'
import { apiRequest } from '@/lib/api'
import { radioEndpoints, radioQueryKeys } from '../api'
import type { RadioTopLabelsResponse } from '../types'

interface UseStationTopLabelsOptions {
  stationSlug: string
  /** Period in days (default 90). */
  period?: number
  limit?: number
  enabled?: boolean
}

/**
 * A station's most-featured labels across all of its shows (PSY-1048). Same
 * response shape as the show-level top-labels endpoint.
 */
export function useStationTopLabels({
  stationSlug,
  period = 90,
  limit = 5,
  enabled = true,
}: UseStationTopLabelsOptions) {
  const params = new URLSearchParams()
  if (period) params.set('period', period.toString())
  if (limit) params.set('limit', limit.toString())

  const queryString = params.toString()
  const endpoint = queryString
    ? `${radioEndpoints.STATION_TOP_LABELS(stationSlug)}?${queryString}`
    : radioEndpoints.STATION_TOP_LABELS(stationSlug)

  return useQuery({
    queryKey: radioQueryKeys.stationTopLabels(stationSlug, { period, limit }),
    queryFn: () =>
      apiRequest<RadioTopLabelsResponse>(endpoint, {
        method: 'GET',
      }),
    enabled: enabled && stationSlug.length > 0,
    staleTime: 5 * 60 * 1000,
  })
}
