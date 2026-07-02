'use client'

import Link from 'next/link'
import { BracketLink } from '@/components/shared'
import { useStationNowPlaying } from '../hooks/useStationNowPlaying'
import { useShowLatestEpisode } from '../hooks/useShowLatestEpisode'
import { airDateCellText } from './AirDateCell'
import type { RadioStationDetail } from '../types'

interface StationOnAirBoxProps {
  station: RadioStationDetail
}

/**
 * The station page's "ON AIR" lead box (PSY-1050, Option A "The Dial").
 *
 * Consumes the PSY-1022 now-playing endpoint: the provider's live broadcast
 * (with current song where the source carries one) when available, the
 * latest-archive fallback otherwise. Labeling is honest — the ● ON AIR dot
 * renders only for a confirmed live broadcast; archive payloads lead with
 * "Latest playlist" instead. Unmatched live show names render as plain text
 * (PSY-1073 — no dead links).
 *
 * Renders nothing for stations with no live source AND no archived shows
 * (graceful for inactive-station archives where playlist data may be sparse).
 */
export function StationOnAirBox({ station }: StationOnAirBoxProps) {
  const { data } = useStationNowPlaying(station.slug)

  // The matched show's latest archived episode backs the playlist deep-link.
  const { episode, isLoading: episodeLoading } = useShowLatestEpisode(
    data?.show?.slug
  )

  if (!data) return null
  const showLabel = data.show?.name ?? data.show_name
  if (!showLabel) return null

  const current = data.current_track
  const hostName = data.show?.host_name ?? data.host_name
  const showUrl = data.show ? `/radio/${station.slug}/${data.show.slug}` : null
  const playlistUrl =
    showUrl && episode ? `${showUrl}/${episode.air_date}` : null
  // PSY-1306: "Latest playlist" renders viewer-local from the episode's
  // frozen window so it agrees with the playlists feed below it in the same
  // column. Prefer the now-playing payload's own episode fields; fall back to
  // the latest-episode hook (used for the deep-link) while the payload lacks
  // them or for older cached responses.
  const latestSrc = data.episode_air_date
    ? {
        starts: data.episode_starts_at,
        ends: data.episode_ends_at,
        date: data.episode_air_date,
      }
    : {
        starts: episode?.starts_at ?? null,
        ends: episode?.ends_at ?? null,
        date: episode?.air_date ?? '',
      }
  const latestCell = airDateCellText(latestSrc.starts, latestSrc.ends, latestSrc.date)
  const latestDate = latestCell.timeBlock
    ? `${latestCell.dateLine} · ${latestCell.timeBlock}`
    : latestCell.dateLine

  return (
    <section
      aria-label={data.on_air ? 'On air' : 'Latest playlist'}
      className="rounded-md border border-primary/60 px-4 py-3.5 flex flex-col gap-1.5"
    >
      <div className="flex items-baseline justify-between gap-2">
        <h2 className="font-mono text-[11px] uppercase tracking-[1.2px] text-muted-foreground">
          {data.on_air && (
            <>
              <span className="text-primary" aria-hidden>
                ●
              </span>{' '}
            </>
          )}
          {data.on_air ? 'On air' : 'Latest playlist'} — {station.name}
        </h2>
        {!data.on_air && latestDate && (
          <span className="font-mono text-[11px] uppercase tracking-wide text-muted-foreground tabular-nums">
            Latest: {latestDate}
          </span>
        )}
      </div>

      <div className="flex items-baseline gap-2 flex-wrap">
        {showUrl ? (
          <Link
            href={showUrl}
            className="text-lg font-semibold text-foreground hover:text-primary transition-colors"
          >
            {showLabel}
          </Link>
        ) : (
          <span className="text-lg font-semibold text-foreground">
            {showLabel}
          </span>
        )}
        {hostName && (
          <span className="text-sm text-muted-foreground">w/ {hostName}</span>
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

      {!playlistUrl && data.show && episodeLoading && (
        <span className="font-mono text-xs text-muted-foreground">
          loading latest playlist…
        </span>
      )}
    </section>
  )
}
