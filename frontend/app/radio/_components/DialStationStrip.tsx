'use client'

import Link from 'next/link'
import { Loader2, Play } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { BracketLink } from '@/components/shared/BracketLink'
import {
  useStationNowPlaying,
  useRadioStation,
  formatStationLocation,
  getBroadcastTypeLabel,
  getRotationStatusLabel,
  getStationDetailUrl,
  previewToHops,
  ArtistHops,
} from '@/features/radio'
import type {
  RadioStationListItem,
  RadioSiblingStation,
  RadioNowPlaying,
} from '@/features/radio'

interface DialStationStripProps {
  station: RadioStationListItem
}

/**
 * One full-width strip on The Dial (PSY-1049): identity column (underlined
 * station name + frequency + location), ON AIR column (current show + track +
 * "earlier:" artist hops), and actions ([▶ Listen] + a playlist link).
 * Network flagships also list their channel sub-rows.
 *
 * Underline convention (locked 2026-06-09): station/channel names are
 * underlined foreground links — underline means "this identity is a page".
 * Orange links remain for content (shows/playlists).
 *
 * Both the ON AIR line and the actions column's playlist link consume the
 * same now-playing payload (useStationNowPlaying): real live broadcast data
 * where the station's provider exposes it, with an honestly labeled
 * latest-archive fallback otherwise. The link therefore always targets the
 * show the ON AIR line names — never a different show. useRadioStation
 * supplies only the [▶ Listen] external URL.
 */
export function DialStationStrip({ station }: DialStationStripProps) {
  const { data: detail } = useRadioStation(station.slug)
  // Same query key as OnAirBlock's call below — TanStack dedupes the fetch.
  const { data: nowPlaying } = useStationNowPlaying(station.slug)

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

  const playlistAction = nowPlaying
    ? playlistActionFor(station.slug, nowPlaying)
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
        <OnAirBlock stationSlug={station.slug} />
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
        {playlistAction && (
          <BracketLink
            label={playlistAction.label}
            href={playlistAction.href}
            className="font-mono text-xs text-primary hover:text-primary/80"
          />
        )}
      </div>
    </article>
  )
}

/**
 * The actions-column playlist link, derived from the SAME now-playing payload
 * the ON AIR line renders — so the link can never name a different show than
 * the line above it. Label ladder (mirrors OnAirBlock's On air / Latest
 * playlist header switch):
 *
 * - on air + dated payload → [live playlist] → the dated episode deep-link
 * - on air, no episode date (live payloads carry none today) → [playlists]
 *   → the show page
 * - off air (latest-archive fallback; carries the episode date) →
 *   [latest playlist] → the dated deep-link
 * - unmatched show (`show: null`) → no link at all
 *
 * "live playlist" is only ever claimed for a dated deep-link while on air.
 */
function playlistActionFor(
  stationSlug: string,
  data: RadioNowPlaying
): { label: string; href: string } | null {
  if (!data.show) return null
  const showUrl = `/radio/${stationSlug}/${data.show.slug}`
  if (!data.episode_air_date) {
    // No dated episode to target — link the show page under an honest label.
    return { label: 'playlists', href: showUrl }
  }
  const datedUrl = `${showUrl}/${data.episode_air_date}`
  return data.on_air
    ? { label: 'live playlist', href: datedUrl }
    : { label: 'latest playlist', href: datedUrl }
}

// ---------------------------------------------------------------------------
// ON AIR block (PSY-1022 now-playing endpoint)
// ---------------------------------------------------------------------------

/**
 * Header label + show identity shared by the strip's main ON AIR line and
 * the channel sub-rows: "● ON AIR" only when the backend confirmed a live
 * broadcast; the latest-archive fallback gets a mono "latest playlist"
 * prefix instead of the dot — same dense register, honest labeling.
 * Unmatched shows (PSY-1073) render `show_name` as plain text, not a link.
 */
function nowPlayingShowLabel(data: RadioNowPlaying): string | null {
  return data.show?.name ?? data.show_name
}

function OnAirBlock({ stationSlug }: { stationSlug: string }) {
  const { data, isLoading } = useStationNowPlaying(stationSlug)

  // Render cached data even when a background refetch errors — a transient
  // network blip shouldn't blank a line that was readable a second ago.
  if (!data) {
    if (isLoading) {
      return (
        <div className="flex items-center gap-2 py-1 text-sm text-muted-foreground">
          <Loader2 className="size-4 animate-spin" aria-hidden />
          <span className="sr-only">Loading on-air info</span>
        </div>
      )
    }
    return (
      <p className="text-sm text-muted-foreground">
        Couldn&apos;t load on-air info.
      </p>
    )
  }

  const showLabel = nowPlayingShowLabel(data)
  if (!showLabel) {
    return (
      <p className="text-sm text-muted-foreground">No playlists tracked yet.</p>
    )
  }

  const current = data.current_track
  const hostName = data.show?.host_name ?? data.host_name

  return (
    <div className="flex min-w-0 flex-col gap-1">
      {/* ● ON AIR (or "latest playlist")  Show name  w/ host */}
      <div className="flex flex-wrap items-baseline gap-x-2 gap-y-0.5">
        <span className="inline-flex items-baseline gap-1.5 font-mono text-[11px] uppercase tracking-[1.2px] text-muted-foreground">
          {data.on_air && (
            <span
              className="size-2 self-center rounded-full bg-primary"
              aria-hidden
            />
          )}
          {data.on_air ? 'On air' : 'Latest playlist'}
        </span>
        {data.show ? (
          <Link
            href={`/radio/${stationSlug}/${data.show.slug}`}
            className="text-[15px] font-semibold text-foreground transition-colors hover:text-primary"
          >
            {showLabel}
          </Link>
        ) : (
          <span className="text-[15px] font-semibold text-foreground">
            {showLabel}
          </span>
        )}
        {hostName && (
          <span className="text-[13px] text-muted-foreground">
            w/ {hostName}
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
      {data.recent_artists.length > 0 && (
        <div className="flex items-baseline gap-1.5">
          <span className="shrink-0 font-mono text-xs text-muted-foreground">
            earlier:
          </span>
          <ArtistHops
            hops={previewToHops(data.recent_artists)}
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
 * its OWN current broadcast (PSY-1022 — each stream has its own now-playing;
 * the old most-active-show heuristic showed every WFMU channel the same
 * show) + [ listen ]. Fetches its own now-playing + detail (bounded N —
 * networks have a handful of channels). A latest-archive payload is prefixed
 * "latest:" instead of claiming currency; unmatched show names render
 * unlinked (PSY-1073).
 */
function DialChannelRow({
  networkSlug,
  channel,
}: {
  networkSlug: string
  channel: RadioSiblingStation
}) {
  const { data: nowPlaying } = useStationNowPlaying(channel.slug)
  // Channel detail is only needed for the external listen URL.
  const { data: channelDetail } = useRadioStation(channel.slug)

  const channelUrl = getStationDetailUrl(channel.slug, {
    slug: networkSlug,
    is_flagship: false,
  })

  const showLabel = nowPlaying ? nowPlayingShowLabel(nowPlaying) : null
  const hostName = nowPlaying
    ? (nowPlaying.show?.host_name ?? nowPlaying.host_name)
    : null

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
      {nowPlaying && showLabel && (
        <span className="text-[13px] text-muted-foreground">
          —{' '}
          {!nowPlaying.on_air && (
            <span className="font-mono text-[11px]">latest: </span>
          )}
          {nowPlaying.show ? (
            <Link
              href={`/radio/${channel.slug}/${nowPlaying.show.slug}`}
              className="transition-colors hover:text-primary"
            >
              {showLabel}
            </Link>
          ) : (
            showLabel
          )}
          {hostName && ` w/ ${hostName}`}
        </span>
      )}
      {nowPlaying?.current_track && (
        <span className="text-[13px] text-muted-foreground">
          <span className="text-primary" aria-hidden>
            ♪
          </span>{' '}
          {nowPlaying.current_track.artist_name}
          {nowPlaying.current_track.track_title &&
            ` — ${nowPlaying.current_track.track_title}`}
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
