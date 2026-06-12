'use client'

import { useMemo } from 'react'
import { useRadioStation } from './useRadioStation'
import { useRadioShows } from './useRadioShows'
import { useShowLatestEpisode } from './useShowLatestEpisode'
import { pickNowPlayingShow } from '../lib/stationOverview'
import type {
  RadioStationDetail,
  RadioShowListItem,
  RadioEpisodeDetail,
} from '../types'

export interface StationOverview {
  station: RadioStationDetail | undefined
  /** The station's signature show (v1 heuristic: most logged episodes). */
  nowPlayingShow: RadioShowListItem | null
  /**
   * nowPlayingShow's most-recent archived episode. Exposed so surfaces can
   * deep-link to the live playlist page (/radio/{station}/{show}/{air_date})
   * — PSY-1049's [ live playlist ].
   */
  latestEpisode: RadioEpisodeDetail | undefined
}

/**
 * Resolve a station's detail plus the archive deep-link target behind the
 * Dial strip's actions column (PSY-1016, slimmed in PSY-1075): the station
 * detail ([▶ Listen] external URL) and the signature show's latest episode
 * (the [ live playlist ] link).
 *
 * The on-air lines themselves moved to the live now-playing endpoint
 * (PSY-1022, useStationNowPlaying); this hook no longer derives a
 * now-playing surface.
 */
export function useStationOverview(stationSlug: string): StationOverview {
  const stationQuery = useRadioStation(stationSlug)
  const station = stationQuery.data

  const showsQuery = useRadioShows(station?.id)
  const shows = showsQuery.data?.shows

  const nowPlayingShow = useMemo(() => pickNowPlayingShow(shows), [shows])

  const latest = useShowLatestEpisode(nowPlayingShow?.slug)

  return {
    station,
    nowPlayingShow,
    latestEpisode: latest.episode,
  }
}
