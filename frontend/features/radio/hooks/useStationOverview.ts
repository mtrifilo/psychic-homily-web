'use client'

import { useMemo } from 'react'
import { useRadioStation } from './useRadioStation'
import { useRadioShows } from './useRadioShows'
import { useRadioShow } from './useRadioShow'
import { useShowLatestEpisode } from './useShowLatestEpisode'
import {
  pickNowPlayingShow,
  orderRecentShows,
  deriveNowPlaying,
  type NowPlaying,
} from '../lib/stationOverview'
import type {
  RadioStationDetail,
  RadioShowListItem,
  RadioShowDetail,
} from '../types'

export interface StationOverview {
  station: RadioStationDetail | undefined
  /** The show driving the Now Playing card (v1: most-active show). */
  nowPlayingShow: RadioShowListItem | null
  /** Detail for nowPlayingShow (host + description for the card). */
  nowPlayingShowDetail: RadioShowDetail | undefined
  /** Derived now-playing surface (current track + recent artists). */
  nowPlaying: NowPlaying
  /** Shows to list under "Recent shows" (excludes nowPlayingShow). */
  recentShows: RadioShowListItem[]
  isLoading: boolean
  /** True once the station resolved but it has no shows at all. */
  isEmpty: boolean
  error: unknown
}

/**
 * Assemble everything the D2 station-overview panel renders for one station
 * (PSY-1016). Orchestrates the station detail, its shows, and the now-playing
 * show's most-recent episode into the panel's render shape.
 *
 * "Now Playing" is the v1 fallback (the most-recent playlist of the station's
 * most-active show) — not live on-air data. See lib/stationOverview and
 * PSY-1022 for the live-data successor; this hook is the single seam that
 * would change when that lands.
 *
 * Recent-show artist hops are fetched per-row by the RecentShowRow component
 * (a bounded N — `recentShows` is capped) so this hook stays a fixed,
 * Rules-of-Hooks-safe set of queries.
 */
export function useStationOverview(stationSlug: string): StationOverview {
  const stationQuery = useRadioStation(stationSlug)
  const station = stationQuery.data

  const showsQuery = useRadioShows(station?.id)
  const shows = showsQuery.data?.shows

  const nowPlayingShow = useMemo(() => pickNowPlayingShow(shows), [shows])

  const recentShows = useMemo(
    () => orderRecentShows(shows, { excludeShowId: nowPlayingShow?.id, limit: 3 }),
    [shows, nowPlayingShow]
  )

  // Full detail for the now-playing show: the list item lacks host_name +
  // description, which the Now Playing card needs (show title + "with {host}"
  // + the vibe line).
  const nowPlayingShowDetailQuery = useRadioShow(nowPlayingShow?.slug ?? '')

  const latest = useShowLatestEpisode(nowPlayingShow?.slug)
  const nowPlaying = useMemo(() => deriveNowPlaying(latest.episode), [latest.episode])

  const isLoading =
    stationQuery.isLoading ||
    (!!station && showsQuery.isLoading) ||
    (!!nowPlayingShow && (nowPlayingShowDetailQuery.isLoading || latest.isLoading))

  const isEmpty = !!station && !showsQuery.isLoading && !nowPlayingShow

  return {
    station,
    nowPlayingShow,
    nowPlayingShowDetail: nowPlayingShowDetailQuery.data,
    nowPlaying,
    recentShows,
    isLoading,
    isEmpty,
    error: stationQuery.error ?? showsQuery.error ?? latest.error,
  }
}
