'use client'

import { useQuery } from '@tanstack/react-query'
import { apiRequest } from '@/lib/api'
import { radioEndpoints, radioQueryKeys } from '../api'
import type { RadioNowPlaying } from '../types'

/**
 * Hook to fetch a station's now-playing payload (PSY-1022): the provider's
 * live broadcast when one exists, the latest-archive fallback otherwise.
 *
 * staleTime tracks the backend's per-station TTL cache (~90s) — refetching
 * faster would just re-read the server cache.
 */
export function useStationNowPlaying(slug: string) {
  return useQuery({
    queryKey: radioQueryKeys.stationNowPlaying(slug),
    queryFn: () =>
      apiRequest<RadioNowPlaying>(radioEndpoints.STATION_NOW_PLAYING(slug), {
        method: 'GET',
      }),
    enabled: slug.length > 0,
    staleTime: 60 * 1000,
  })
}
