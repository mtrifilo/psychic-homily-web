'use client'

import { useQuery, keepPreviousData } from '@tanstack/react-query'
import { apiRequest } from '@/lib/api'
import { radioEndpoints, radioQueryKeys } from '../api'
import type { RadioEpisodesListResponse } from '../types'

interface UseRadioEpisodesOptions {
  showSlug: string
  limit?: number
  offset?: number
  enabled?: boolean
}

/**
 * Hook to fetch episodes for a radio show with pagination
 */
export function useRadioEpisodes({
  showSlug,
  limit = 20,
  offset = 0,
  enabled = true,
}: UseRadioEpisodesOptions) {
  const params = new URLSearchParams()
  if (limit) params.set('limit', limit.toString())
  if (offset) params.set('offset', offset.toString())

  const queryString = params.toString()
  const endpoint = queryString
    ? `${radioEndpoints.SHOW_EPISODES(showSlug)}?${queryString}`
    : radioEndpoints.SHOW_EPISODES(showSlug)

  return useQuery({
    queryKey: radioQueryKeys.episodes(showSlug, { limit, offset }),
    queryFn: () =>
      apiRequest<RadioEpisodesListResponse>(endpoint, {
        method: 'GET',
      }),
    enabled: enabled && showSlug.length > 0,
    staleTime: 5 * 60 * 1000,
    placeholderData: keepPreviousData,
  })
}
