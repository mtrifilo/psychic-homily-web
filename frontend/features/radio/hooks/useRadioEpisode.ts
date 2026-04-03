'use client'

import { useQuery } from '@tanstack/react-query'
import { apiRequest } from '@/lib/api'
import { radioEndpoints, radioQueryKeys } from '../api'
import type { RadioEpisodeDetail } from '../types'

/**
 * Hook to fetch a single episode by show slug + date
 */
export function useRadioEpisode(showSlug: string, date: string) {
  return useQuery({
    queryKey: radioQueryKeys.episode(showSlug, date),
    queryFn: () =>
      apiRequest<RadioEpisodeDetail>(
        radioEndpoints.SHOW_EPISODE_BY_DATE(showSlug, date),
        { method: 'GET' }
      ),
    enabled: showSlug.length > 0 && date.length > 0,
    staleTime: 5 * 60 * 1000,
  })
}
