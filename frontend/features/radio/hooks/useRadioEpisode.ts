'use client'

import { useQuery } from '@tanstack/react-query'
import { apiRequest } from '@/lib/api'
import { radioEndpoints, radioQueryKeys } from '../api'
import { liveEpisodePollMs } from '../lib/episodeArchive'
import type { RadioEpisodeDetail } from '../types'

/**
 * Hook to fetch a single episode by show slug + date.
 *
 * While the episode is live (inside its frozen air window) the query polls
 * so the playlist page's live ledger picks up newly-scraped tracks without
 * a reload. The gate lives in liveEpisodePollMs: it stops on a failing
 * query (the PSY-1136 infinite-poll class) and past ends_at, so archive
 * pages never poll.
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
    refetchInterval: query =>
      liveEpisodePollMs(
        query.state.error,
        query.state.data?.starts_at,
        query.state.data?.ends_at
      ),
  })
}
