'use client'

import { useQuery } from '@tanstack/react-query'
import { apiRequest } from '@/lib/api'
import { radioEndpoints, radioQueryKeys } from '../api'
import { walkEpisodeNeighbors } from '../lib/episodeArchive'
import type { EpisodeNeighbors } from '../lib/episodeArchive'
import type { RadioEpisodesListResponse } from '../types'

/**
 * Prev/next episode neighbors for the playlist page nav (PSY-1051).
 *
 * Derived from the show's episodes list (air_date DESC) — there is no
 * dedicated neighbors endpoint. The page walk lives in walkEpisodeNeighbors;
 * this hook just wires it to the API and caches per (show, date).
 */
export function useEpisodeNeighbors(showSlug: string, date: string) {
  return useQuery<EpisodeNeighbors>({
    queryKey: radioQueryKeys.episodeNeighbors(showSlug, date),
    queryFn: () =>
      walkEpisodeNeighbors(date, (offset, limit) => {
        const params = new URLSearchParams({ limit: String(limit) })
        if (offset > 0) params.set('offset', String(offset))
        return apiRequest<RadioEpisodesListResponse>(
          `${radioEndpoints.SHOW_EPISODES(showSlug)}?${params.toString()}`,
          { method: 'GET' }
        )
      }),
    enabled: showSlug.length > 0 && date.length > 0,
    staleTime: 5 * 60 * 1000,
  })
}
