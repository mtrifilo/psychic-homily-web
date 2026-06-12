'use client'

import { keepPreviousData, useQuery } from '@tanstack/react-query'
import { apiRequest } from '@/lib/api'
import { radioEndpoints, radioQueryKeys } from '../api'
import type { RadioRecentEpisodesResponse } from '../types'

interface UseRecentRadioEpisodesOptions {
  limit?: number
  offset?: number
  enabled?: boolean
}

/**
 * Fetch the dial-wide latest-playlists feed (PSY-1048): the newest episodes
 * across every active station, with show/station attribution and a short
 * artist preview per row. Drives the "Latest playlists — across the dial"
 * table on the /radio hub (PSY-1049) and the /radio/playlists full feed
 * (PSY-1076).
 *
 * `keepPreviousData` so a "more playlists" limit bump re-renders the table in
 * place instead of flashing back to a loading state (matches
 * useStationEpisodes' PSY-1050 behavior; harmless for the fixed-limit hub).
 */
export function useRecentRadioEpisodes({
  limit = 20,
  offset = 0,
  enabled = true,
}: UseRecentRadioEpisodesOptions = {}) {
  const params = new URLSearchParams()
  if (limit) params.set('limit', limit.toString())
  if (offset) params.set('offset', offset.toString())

  const queryString = params.toString()
  const endpoint = queryString
    ? `${radioEndpoints.RECENT_EPISODES}?${queryString}`
    : radioEndpoints.RECENT_EPISODES

  return useQuery({
    queryKey: radioQueryKeys.recentEpisodes({ limit, offset }),
    queryFn: () =>
      apiRequest<RadioRecentEpisodesResponse>(endpoint, {
        method: 'GET',
      }),
    enabled,
    placeholderData: keepPreviousData,
    staleTime: 5 * 60 * 1000,
  })
}
