'use client'

import Link from 'next/link'
import { Loader2, Play } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { BracketLink } from '@/components/shared/BracketLink'
import {
  useStationOverview,
  useRadioShows,
  useRadioStation,
  pickNowPlayingShow,
  formatStationLocation,
  getBroadcastTypeLabel,
  getRotationStatusLabel,
  getStationDetailUrl,
  ArtistHops,
} from '@/features/radio'
import type {
  RadioStationListItem,
  RadioSiblingStation,
  RadioShowDetail,
  NowPlaying,
} from '@/features/radio'

interface DialStationStripProps {
  station: RadioStationListItem
}

/**
 * One full-width strip on The Dial (PSY-1049): identity column (underlined
 * station name + frequency + location), ON AIR column (current show + track +
 * "earlier:" artist hops, via the PSY-1016 v1 heuristic), and actions
 * ([▶ Listen] + [ live playlist ]). Network flagships also list their channel
 * sub-rows.
 *
 * Underline convention (locked 2026-06-09): station/channel names are
 * underlined foreground links — underline means "this identity is a page".
 * Orange links remain for content (shows/playlists).
 *
 * "ON AIR" reflects the v1 heuristic — the most-active show's latest logged
 * playlist, not a live signal. PSY-1022's live endpoint swaps in at the
 * useStationOverview seam without changing this layout.
 */
export function DialStationStrip({ station }: DialStationStripProps) {
  const {
    station: detail,
    nowPlayingShowDetail,
    nowPlaying,
    latestEpisode,
    isLoading,
    isEmpty,
    error,
  } = useStationOverview(station.slug)

  // Non-flagship siblings = the network's channels (sibling_stations excludes
  // self, so on a flagship every non-flagship entry is a channel sub-row).
  const channels = station.sibling_stations.filter(s => !s.is_flagship)

  const identityLine = [
    formatStationLocation(station.city, station.state),
    getBroadcastTypeLabel(station.broadcast_type),
    channels.length > 0
      ? `${channels.length} ${channels.length === 1 ? 'channel' : 'channels'}`
      : '',
  ]
    .filter(Boolean)
    .join(' · ')

  const livePlaylistUrl =
    nowPlayingShowDetail && latestEpisode
      ? `/radio/${station.slug}/${nowPlayingShowDetail.slug}/${latestEpisode.air_date}`
      : null

  return (
    <article className="grid gap-3 border-b border-border/60 py-5 last:border-b-0 md:grid-cols-[200px_minmax(0,1fr)_auto] md:gap-6">
      {/* Identity column */}
      <div className="min-w-0">
        <div className="flex flex-wrap items-baseline gap-x-2">
          <Link
            href={`/radio/${station.slug}`}
            className="text-xl font-bold text-foreground underline decoration-1 underline-offset-4 transition-colors hover:decoration-primary"
          >
            {station.name}
          </Link>
          {station.frequency_mhz != null && (
            <span className="font-mono text-xs text-muted-foreground">
              {station.frequency_mhz.toFixed(1)} FM
            </span>
          )}
        </div>
        {identityLine && (
          <p className="mt-1 font-mono text-[11px] leading-4 text-muted-foreground">
            {identityLine}
          </p>
        )}
      </div>

      {/* ON AIR column + channel sub-rows */}
      <div className="flex min-w-0 flex-col gap-3">
        <OnAirBlock
          stationSlug={station.slug}
          isLoading={isLoading}
          isEmpty={isEmpty}
          error={error}
          showDetail={nowPlayingShowDetail}
          nowPlaying={nowPlaying}
        />
        {channels.length > 0 && (
          <ul className="flex flex-col gap-1.5">
            {channels.map(channel => (
              <DialChannelRow
                key={channel.id}
                networkSlug={station.network?.slug ?? station.slug}
                channel={channel}
              />
            ))}
          </ul>
        )}
      </div>

      {/* Actions column */}
      <div className="flex items-center gap-4 md:flex-col md:items-end md:gap-2">
        {detail?.website && (
          <Button asChild size="sm">
            <a href={detail.website} target="_blank" rel="noopener noreferrer">
              <Play className="size-3.5 fill-current" aria-hidden />
              Listen
            </a>
          </Button>
        )}
        {livePlaylistUrl && (
          <BracketLink
            label="live playlist"
            href={livePlaylistUrl}
            className="font-mono text-xs text-primary hover:text-primary/80"
          />
        )}
      </div>
    </article>
  )
}

// ---------------------------------------------------------------------------
// ON AIR block (v1 heuristic)
// ---------------------------------------------------------------------------

function OnAirBlock({
  stationSlug,
  isLoading,
  isEmpty,
  error,
  showDetail,
  nowPlaying,
}: {
  stationSlug: string
  isLoading: boolean
  isEmpty: boolean
  error: unknown
  showDetail: RadioShowDetail | undefined
  nowPlaying: NowPlaying
}) {
  if (isLoading && !showDetail) {
    return (
      <div className="flex items-center gap-2 py-1 text-sm text-muted-foreground">
        <Loader2 className="size-4 animate-spin" aria-hidden />
        <span className="sr-only">Loading on-air info</span>
      </div>
    )
  }

  if (error) {
    return (
      <p className="text-sm text-muted-foreground">
        Couldn&apos;t load on-air info.
      </p>
    )
  }

  if (isEmpty || !showDetail) {
    return (
      <p className="text-sm text-muted-foreground">No playlists tracked yet.</p>
    )
  }

  const { current, recentArtists } = nowPlaying

  return (
    <div className="flex min-w-0 flex-col gap-1">
      {/* ● ON AIR  Show name  w/ host */}
      <div className="flex flex-wrap items-baseline gap-x-2 gap-y-0.5">
        <span className="inline-flex items-baseline gap-1.5 font-mono text-[11px] uppercase tracking-[1.2px] text-muted-foreground">
          <span
            className="size-2 self-center rounded-full bg-primary"
            aria-hidden
          />
          On air
        </span>
        <Link
          href={`/radio/${stationSlug}/${showDetail.slug}`}
          className="text-[15px] font-semibold text-foreground transition-colors hover:text-primary"
        >
          {showDetail.name}
        </Link>
        {showDetail.host_name && (
          <span className="text-[13px] text-muted-foreground">
            w/ {showDetail.host_name}
          </span>
        )}
      </div>

      {/* ♪ current track + rotation tag */}
      {current && (
        <div className="flex flex-wrap items-baseline gap-x-1.5">
          <span className="text-primary" aria-hidden>
            ♪
          </span>
          {current.artist_slug ? (
            <Link
              href={`/artists/${current.artist_slug}`}
              className="text-sm font-medium text-foreground transition-colors hover:text-primary"
            >
              {current.artist_name}
            </Link>
          ) : (
            <span className="text-sm font-medium text-foreground">
              {current.artist_name}
            </span>
          )}
          {current.track_title && (
            <span className="text-sm text-muted-foreground">
              — {current.track_title}
            </span>
          )}
          {current.rotation_status && (
            <span className="font-mono text-[11px] lowercase text-primary">
              {getRotationStatusLabel(current.rotation_status)}
            </span>
          )}
        </div>
      )}

      {/* earlier: artist hops */}
      {recentArtists.length > 0 && (
        <div className="flex items-baseline gap-1.5">
          <span className="shrink-0 font-mono text-xs text-muted-foreground">
            earlier:
          </span>
          <ArtistHops
            hops={recentArtists}
            className="font-mono text-xs text-foreground"
          />
        </div>
      )}
    </div>
  )
}

// ---------------------------------------------------------------------------
// Network channel sub-row
// ---------------------------------------------------------------------------

/**
 * One channel sub-row under a network flagship: underlined channel name +
 * its current show (v1 heuristic: most-active show) + [ listen ]. Fetches its
 * own shows + detail (bounded N — networks have a handful of channels), the
 * same per-row pattern as PSY-1016's RecentShowRow.
 */
function DialChannelRow({
  networkSlug,
  channel,
}: {
  networkSlug: string
  channel: RadioSiblingStation
}) {
  const showsQuery = useRadioShows(channel.id)
  const currentShow = pickNowPlayingShow(showsQuery.data?.shows)
  // Channel detail is only needed for the external listen URL.
  const { data: channelDetail } = useRadioStation(channel.slug)

  const channelUrl = getStationDetailUrl(channel.slug, {
    slug: networkSlug,
    is_flagship: false,
  })

  return (
    <li className="flex flex-wrap items-baseline gap-x-2 gap-y-0.5">
      <span
        className="size-1.5 self-center rounded-full bg-primary/60"
        aria-hidden
      />
      <Link
        href={channelUrl}
        className="text-sm font-semibold text-foreground underline decoration-1 underline-offset-4 transition-colors hover:decoration-primary"
      >
        {channel.name}
      </Link>
      {currentShow && (
        <span className="text-[13px] text-muted-foreground">
          —{' '}
          <Link
            href={`/radio/${channel.slug}/${currentShow.slug}`}
            className="transition-colors hover:text-primary"
          >
            {currentShow.name}
          </Link>
          {currentShow.host_name && ` w/ ${currentShow.host_name}`}
        </span>
      )}
      {channelDetail?.website && (
        // Hand-rolled bracket link (not BracketLink) because the target is an
        // external stream URL needing target="_blank"; text matches
        // BracketLink's tight [label] idiom.
        <a
          href={channelDetail.website}
          target="_blank"
          rel="noopener noreferrer"
          className="font-mono text-xs text-primary transition-colors hover:text-primary/80"
        >
          [listen]
        </a>
      )}
    </li>
  )
}
