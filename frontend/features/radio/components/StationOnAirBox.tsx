'use client'

import Link from 'next/link'
import { BracketLink } from '@/components/shared'
import { useRadioShows } from '../hooks/useRadioShows'
import { useShowLatestEpisode } from '../hooks/useShowLatestEpisode'
import {
  pickNowPlayingShow,
  deriveNowPlaying,
  formatShortAirDate,
} from '../lib/stationOverview'
import type { RadioStationDetail } from '../types'

interface StationOnAirBoxProps {
  station: RadioStationDetail
}

/**
 * The station page's "ON AIR" lead box (PSY-1050, Option A "The Dial").
 *
 * v1 heuristic (PSY-1016 stationOverview helpers): the box surfaces the
 * most-active show's latest logged playlist — `pickNowPlayingShow` +
 * `deriveNowPlaying` — NOT a live on-air signal. PSY-1022's live now-playing
 * endpoint replaces the data source behind this same layout: everything
 * below the hooks renders from (show, episode, nowPlaying) shapes that the
 * live endpoint would also produce.
 *
 * Renders nothing for stations with no shows (graceful for inactive-station
 * archives where playlist data may be sparse).
 */
export function StationOnAirBox({ station }: StationOnAirBoxProps) {
  // Same query the shows directory uses (sort=latest) so the page fetches
  // the show list once.
  const { data: showsData } = useRadioShows(station.id, { sort: 'latest' })
  const show = pickNowPlayingShow(showsData?.shows)

  const { episode, isLoading: episodeLoading } = useShowLatestEpisode(show?.slug)
  const nowPlaying = deriveNowPlaying(episode)
  const current = nowPlaying.current

  if (!show) return null

  const showUrl = `/radio/${station.slug}/${show.slug}`
  const playlistUrl = episode ? `${showUrl}/${episode.air_date}` : null
  const latestDate = formatShortAirDate(episode?.air_date ?? show.latest_air_date)

  return (
    <section
      aria-label="On air"
      className="rounded-md border border-primary/60 px-4 py-3.5 flex flex-col gap-1.5"
    >
      <div className="flex items-baseline justify-between gap-2">
        <h2 className="font-mono text-[11px] uppercase tracking-[1.2px] text-muted-foreground">
          <span className="text-primary" aria-hidden>
            ●
          </span>{' '}
          On air — {station.name}
        </h2>
        {(show.schedule_display || latestDate) && (
          <span className="font-mono text-[11px] uppercase tracking-wide text-muted-foreground tabular-nums">
            {show.schedule_display ?? `Latest: ${latestDate}`}
          </span>
        )}
      </div>

      <div className="flex items-baseline gap-2 flex-wrap">
        <Link
          href={showUrl}
          className="text-lg font-semibold text-foreground hover:text-primary transition-colors"
        >
          {show.name}
        </Link>
        {show.host_name && (
          <span className="text-sm text-muted-foreground">w/ {show.host_name}</span>
        )}
      </div>

      {current && (
        <div className="flex items-baseline gap-1.5 flex-wrap">
          <span className="text-primary" aria-hidden>
            ♪
          </span>
          {current.artist_slug ? (
            <Link
              href={`/artists/${current.artist_slug}`}
              className="text-sm font-semibold text-foreground hover:text-primary transition-colors"
            >
              {current.artist_name}
            </Link>
          ) : (
            <span className="text-sm font-semibold text-foreground">
              {current.artist_name}
            </span>
          )}
          {current.track_title && (
            <span className="text-sm text-foreground">— {current.track_title}</span>
          )}
          {(current.album_title || current.label_name || current.release_year) && (
            <span className="font-mono text-xs text-muted-foreground">
              {[current.album_title, current.label_name, current.release_year]
                .filter(Boolean)
                .join(' · ')}
            </span>
          )}
        </div>
      )}

      {playlistUrl && (
        <div className="flex items-baseline gap-2 mt-0.5">
          <BracketLink
            label="Open latest playlist →"
            href={playlistUrl}
            className="text-primary hover:text-primary/80"
          />
          {latestDate && (
            <span className="font-mono text-xs text-muted-foreground">
              aired {latestDate}
            </span>
          )}
        </div>
      )}

      {!playlistUrl && episodeLoading && (
        <span className="font-mono text-xs text-muted-foreground">
          loading latest playlist…
        </span>
      )}
    </section>
  )
}
