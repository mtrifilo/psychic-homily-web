'use client'

import { useQuery } from '@tanstack/react-query'
import { apiRequest } from '@/lib/api'
import { radioEndpoints, radioQueryKeys } from '../api'
import type { RadioShowsListResponse } from '../types'

export type RadioShowsSort = 'name' | 'latest'

interface UseRadioShowsOptions {
  /**
   * Server-side sort (PSY-1048): 'name' (alphabetical, default) or 'latest'
   * (active shows first, most recent playlist first).
   */
  sort?: RadioShowsSort
}

/**
 * Hook to fetch radio shows for a station
 */
export function useRadioShows(stationId?: number, options: UseRadioShowsOptions = {}) {
  const { sort } = options
  const params = new URLSearchParams()
  if (stationId) params.set('station_id', stationId.toString())
  if (sort) params.set('sort', sort)
  const queryString = params.toString()
  const endpoint = queryString
    ? `${radioEndpoints.SHOWS}?${queryString}`
    : radioEndpoints.SHOWS

  return useQuery({
    queryKey: radioQueryKeys.shows(stationId, sort),
    queryFn: () =>
      apiRequest<RadioShowsListResponse>(endpoint, {
        method: 'GET',
      }),
    enabled: !!stationId,
    staleTime: 5 * 60 * 1000,
  })
}
