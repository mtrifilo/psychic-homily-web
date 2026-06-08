'use client'

import { useRadioEpisodes } from './useRadioEpisodes'
import { useRadioEpisode } from './useRadioEpisode'
import type { RadioEpisodeDetail } from '../types'

/**
 * Fetch a radio show's most-recent episode WITH its full playlist (PSY-1016).
 *
 * Two-step chain because the public API has no "latest episode with plays"
 * endpoint: the episodes list (air_date DESC) gives the newest air-date, then
 * the by-date endpoint returns that episode's plays + entity links. This is
 * the v1 fallback the D2 "Now Playing" surface reads from; a future live
 * now-playing endpoint (PSY-1022) replaces this hook without changing the
 * panel components, since both produce a RadioEpisodeDetail-shaped playlist.
 */
export function useShowLatestEpisode(showSlug: string | undefined) {
  const slug = showSlug ?? ''

  const episodesQuery = useRadioEpisodes({
    showSlug: slug,
    limit: 1,
    enabled: slug.length > 0,
  })

  const latestDate = episodesQuery.data?.episodes[0]?.air_date ?? ''

  const episodeQuery = useRadioEpisode(slug, latestDate)

  const episode: RadioEpisodeDetail | undefined =
    latestDate.length > 0 ? episodeQuery.data : undefined

  // Loading while the list is in flight, or while we have a date but the
  // detail hasn't resolved yet. A show with zero episodes resolves the list
  // with an empty array, leaves latestDate empty, and is therefore "not
  // loading, no episode" — the graceful empty state.
  const isLoading =
    episodesQuery.isLoading ||
    (latestDate.length > 0 && episodeQuery.isLoading)

  return {
    episode,
    isLoading,
    error: episodesQuery.error ?? episodeQuery.error,
    hasEpisodes: (episodesQuery.data?.episodes.length ?? 0) > 0,
  }
}
