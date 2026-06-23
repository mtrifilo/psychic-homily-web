'use client'

import { useRadioEpisodes } from './useRadioEpisodes'
import { useRadioEpisode } from './useRadioEpisode'
import type { RadioEpisodeDetail } from '../types'

/**
 * Fetch a radio show's most-recent episode WITH its full playlist (PSY-1016).
 *
 * Two-step chain because the public API has no "latest episode with plays"
 * endpoint: the episodes list (air_date DESC) gives the newest air-date, then
 * the by-date endpoint returns that episode's plays + entity links. The live
 * on-air lines moved to PSY-1022's now-playing endpoint; this hook survives
 * to anchor the archive playlist deep-links (StationOnAirBox,
 * useStationOverview's [ live playlist ]).
 */
export function useShowLatestEpisode(showSlug: string | undefined) {
  const slug = showSlug ?? ''

  // Fetch a few rows, not just the newest: the list is air_date DESC and may be
  // led by upcoming (not-yet-aired) placeholders (PSY-1205). We want the latest
  // AIRED episode for the deep-link, so we skip leading is_upcoming rows. A
  // single show has at most a handful of upcoming pages; 8 sits comfortably
  // above that (if all 8 are upcoming there's no aired playlist to link to yet).
  const episodesQuery = useRadioEpisodes({
    showSlug: slug,
    limit: 8,
    enabled: slug.length > 0,
  })

  // useRadioEpisodes uses keepPreviousData, so on a slug change `.data` is the
  // PREVIOUS show's list until the new one resolves. Ignore that placeholder
  // here — otherwise we'd feed the old show's air-date into the new slug's
  // by-date query and fire a wasted (usually 404) request for the wrong show.
  const episodesData = episodesQuery.isPlaceholderData ? undefined : episodesQuery.data

  // The latest AIRED episode (first KNOWN-aired in air_date-DESC order) — never a
  // future placeholder, which would deep-link to an empty, not-yet-aired page.
  // `=== false` (not `!is_upcoming`) so a stale pre-deploy cache row missing the
  // flag is skipped rather than mistaken for aired; a fresh fetch self-heals.
  const latestDate =
    episodesData?.episodes.find(ep => ep.is_upcoming === false)?.air_date ?? ''

  const episodeQuery = useRadioEpisode(slug, latestDate)

  const episode: RadioEpisodeDetail | undefined =
    latestDate.length > 0 ? episodeQuery.data : undefined

  // Loading while the list is in flight (or showing stale placeholder data),
  // or while we have a date but the detail hasn't resolved yet. A show with
  // zero episodes resolves the list with an empty array, leaves latestDate
  // empty, and is therefore "not loading, no episode" — the graceful empty
  // state.
  const isLoading =
    episodesQuery.isLoading ||
    episodesQuery.isPlaceholderData ||
    (latestDate.length > 0 && episodeQuery.isLoading)

  return {
    episode,
    isLoading,
    error: episodesQuery.error ?? episodeQuery.error,
    hasEpisodes: (episodesData?.episodes.length ?? 0) > 0,
  }
}
