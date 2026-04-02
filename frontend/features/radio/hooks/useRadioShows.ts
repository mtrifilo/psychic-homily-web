'use client'

import { useQuery } from '@tanstack/react-query'
import { apiRequest } from '@/lib/api'
import { radioEndpoints, radioQueryKeys } from '../api'
import type { RadioShowsListResponse } from '../types'

/**
 * Hook to fetch radio shows for a station
 */
export function useRadioShows(stationId?: number) {
  const params = new URLSearchParams()
  if (stationId) params.set('station_id', stationId.toString())
  const queryString = params.toString()
  const endpoint = queryString
    ? `${radioEndpoints.SHOWS}?${queryString}`
    : radioEndpoints.SHOWS

  return useQuery({
    queryKey: radioQueryKeys.shows(stationId),
    queryFn: () =>
      apiRequest<RadioShowsListResponse>(endpoint, {
        method: 'GET',
      }),
    enabled: !!stationId,
    staleTime: 5 * 60 * 1000,
  })
}
