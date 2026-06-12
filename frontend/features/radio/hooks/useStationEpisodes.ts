'use client'

import { keepPreviousData, useQuery } from '@tanstack/react-query'
import { apiRequest } from '@/lib/api'
import { radioEndpoints, radioQueryKeys } from '../api'
import type { RadioStationEpisodesResponse } from '../types'

interface UseStationEpisodesOptions {
  stationSlug: string
  /** Max rows (API caps at 100). */
  limit?: number
  offset?: number
  enabled?: boolean
}

/**
 * Latest playlists across all of a station's shows (PSY-1048), newest first.
 * Strictly per-station (PSY-1074): a network flagship's feed contains only
 * its own playlists — channel shows live under their own tabs.
 *
 * `keepPreviousData` so a "more playlists" limit bump re-renders the table in
 * place instead of flashing back to a loading state.
 */
export function useStationEpisodes({
  stationSlug,
  limit = 10,
  offset = 0,
  enabled = true,
}: UseStationEpisodesOptions) {
  const params = new URLSearchParams()
  if (limit) params.set('limit', limit.toString())
  if (offset) params.set('offset', offset.toString())

  const queryString = params.toString()
  const endpoint = queryString
    ? `${radioEndpoints.STATION_EPISODES(stationSlug)}?${queryString}`
    : radioEndpoints.STATION_EPISODES(stationSlug)

  return useQuery({
    queryKey: radioQueryKeys.stationEpisodes(stationSlug, { limit, offset }),
    queryFn: () =>
      apiRequest<RadioStationEpisodesResponse>(endpoint, {
        method: 'GET',
      }),
    enabled: enabled && stationSlug.length > 0,
    placeholderData: keepPreviousData,
    staleTime: 5 * 60 * 1000,
  })
}
