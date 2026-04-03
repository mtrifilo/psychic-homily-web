'use client'

import { useQuery } from '@tanstack/react-query'
import { apiRequest } from '@/lib/api'
import { radioEndpoints, radioQueryKeys } from '../api'
import type { RadioStationDetail } from '../types'

/**
 * Hook to fetch a single radio station by slug
 */
export function useRadioStation(slug: string) {
  return useQuery({
    queryKey: radioQueryKeys.station(slug),
    queryFn: () =>
      apiRequest<RadioStationDetail>(radioEndpoints.STATION(slug), {
        method: 'GET',
      }),
    enabled: slug.length > 0,
    staleTime: 5 * 60 * 1000,
  })
}
