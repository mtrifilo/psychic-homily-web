'use client'

import { useQuery } from '@tanstack/react-query'
import { apiRequest } from '@/lib/api'
import { radioEndpoints, radioQueryKeys } from '../api'
import type { RadioStationsListResponse } from '../types'

/**
 * Hook to fetch the list of radio stations
 */
export function useRadioStations() {
  return useQuery({
    queryKey: radioQueryKeys.stations(),
    queryFn: () =>
      apiRequest<RadioStationsListResponse>(radioEndpoints.STATIONS, {
        method: 'GET',
      }),
    staleTime: 5 * 60 * 1000,
  })
}
