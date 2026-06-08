'use client'

import Link from 'next/link'
import { ArrowUpRight, Loader2, Radio } from 'lucide-react'
import { useStationOverview } from '../hooks/useStationOverview'
import {
  formatStationLocation,
  type NowPlaying,
} from '../lib/stationOverview'
import { getBroadcastTypeLabel } from '../types'
import type { RadioStationDetail, RadioShowDetail } from '../types'
import { ArtistHops } from './ArtistHops'
import { RecentShowRow } from './RecentShowRow'

interface RadioStationOverviewProps {
  stationSlug: string
  /** Called when an internal link is followed (e.g. to close the nav panel). */
  onNavigate?: () => void
}

/**
 * The D2 "station overview" right pane (PSY-1016): station identity + Now
 * Playing card + Recent shows. Shared verbatim between the Radio nav panel and
 * the /radio page (the page just renders it wider). Every artist / release /
 * label is a one-click graph hop.
 *
 * "Now Playing" is the v1 fallback (most-recent playlist of the station's
 * most-active show); see useStationOverview / PSY-1022.
 */
export function RadioStationOverview({ stationSlug, onNavigate }: RadioStationOverviewProps) {
  const {
    station,
    nowPlayingShow,
    nowPlayingShowDetail,
    nowPlaying,
    recentShows,
    isLoading,
    isEmpty,
    error,
  } = useStationOverview(stationSlug)

  if (isLoading && !station) {
    return (
      <div className="flex flex-1 items-center justify-center py-16">
        <Loader2 className="size-6 animate-spin text-muted-foreground" />
      </div>
    )
  }

  if (error || !station) {
    return (
      <div className="flex flex-1 flex-col items-center justify-center gap-1 py-16 text-center">
        <Radio className="size-7 text-muted-foreground/30" />
        <p className="text-sm text-muted-foreground">Couldn&apos;t load this station.</p>
      </div>
    )
  }

  return (
    <div className="flex flex-1 flex-col gap-4 px-5 py-[18px] min-w-0">
      <StationHeader station={station} onNavigate={onNavigate} />

      <NowPlayingCard
        station={station}
        showDetail={nowPlayingShowDetail}
        nowPlaying={nowPlaying}
        hasNowPlayingShow={!!nowPlayingShow}
        onNavigate={onNavigate}
      />

      {!isEmpty && recentShows.length > 0 && (
        <section className="flex flex-col gap-2.5">
          <h3 className="font-mono text-[11px] uppercase tracking-[1.2px] text-muted-foreground">
            Recent shows
          </h3>
          <div className="flex flex-col gap-2.5">
            {recentShows.map(show => (
              <RecentShowRow
                key={show.id}
                show={show}
                stationSlug={station.slug}
                onNavigate={onNavigate}
              />
            ))}
          </div>
        </section>
      )}

      {isEmpty && (
        <p className="text-sm text-muted-foreground">
          No shows tracked for this station yet.
        </p>
      )}
    </div>
  )
}

// ---------------------------------------------------------------------------
// Station identity header
// ---------------------------------------------------------------------------

function StationHeader({
  station,
  onNavigate,
}: {
  station: RadioStationDetail
  onNavigate?: () => void
}) {
  const location = formatStationLocation(station.city, station.state)
  // Data-honest identity sub-line: location + broadcast type (both from the
  // model). The Figma's "listener-supported" is editorial copy not carried by
  // the data, so we don't fabricate it per station.
  const subline = [location, getBroadcastTypeLabel(station.broadcast_type)]
    .filter(Boolean)
    .join(' · ')

  return (
    <header className="flex flex-col gap-1 min-w-0">
      <div className="flex items-center justify-between gap-3">
        <div className="flex items-baseline gap-2 min-w-0">
          <Link
            href={`/radio/${station.slug}`}
            onClick={onNavigate}
            className="text-lg font-semibold text-foreground hover:text-primary transition-colors"
          >
            {station.name}
          </Link>
          {subline && (
            <span className="truncate text-[13px] text-muted-foreground">{subline}</span>
          )}
        </div>
        {station.website && (
          <a
            href={station.website}
            target="_blank"
            rel="noopener noreferrer"
            className="inline-flex shrink-0 items-center gap-0.5 text-[13px] font-medium text-muted-foreground hover:text-foreground transition-colors"
          >
            Listen
            <ArrowUpRight className="size-3.5" aria-hidden />
          </a>
        )}
      </div>
      {station.description && (
        <p className="text-[13px] leading-[19px] text-muted-foreground">
          {station.description}
        </p>
      )}
    </header>
  )
}

// ---------------------------------------------------------------------------
// Now Playing card
// ---------------------------------------------------------------------------

function NowPlayingCard({
  station,
  showDetail,
  nowPlaying,
  hasNowPlayingShow,
  onNavigate,
}: {
  station: RadioStationDetail
  showDetail: RadioShowDetail | undefined
  nowPlaying: NowPlaying
  hasNowPlayingShow: boolean
  onNavigate?: () => void
}) {
  // No show / no playlist data: skip the card entirely rather than render an
  // empty shell (graceful v1 fallback for stations with no episode data).
  if (!hasNowPlayingShow || !showDetail) return null

  const { current, recentArtists } = nowPlaying
  const showUrl = `/radio/${station.slug}/${showDetail.slug}`

  return (
    <section className="flex flex-col gap-1.5 rounded-lg border border-border bg-muted px-4 py-3.5">
      <div className="flex items-center justify-between">
        <h3 className="font-mono text-[11px] uppercase tracking-[1.2px] text-muted-foreground">
          Now playing
        </h3>
        {/* v1 fallback: "on air" reflects the most-recent logged playlist, not
            a live on-air signal (PSY-1022). */}
        <span className="inline-flex items-center gap-1.5 text-xs text-muted-foreground">
          <span className="size-2 rounded-full bg-primary" aria-hidden />
          on air
        </span>
      </div>

      <div className="flex items-baseline gap-2 flex-wrap">
        <Link
          href={showUrl}
          onClick={onNavigate}
          className="text-base font-semibold text-foreground hover:text-primary transition-colors"
        >
          {showDetail.name}
        </Link>
        {showDetail.host_name && (
          <span className="text-[13px] text-muted-foreground">
            with {showDetail.host_name}
          </span>
        )}
      </div>

      {showDetail.description && (
        <p className="text-[13px] leading-[18px] text-muted-foreground">
          {showDetail.description}
        </p>
      )}

      {current && (
        <div className="flex items-center gap-1.5 flex-wrap">
          <span className="text-primary" aria-hidden>
            ♪
          </span>
          {current.artist_slug ? (
            <Link
              href={`/artists/${current.artist_slug}`}
              onClick={onNavigate}
              className="text-sm font-semibold text-foreground hover:text-primary transition-colors"
            >
              {current.artist_name}
            </Link>
          ) : (
            <span className="text-sm font-semibold text-foreground">
              {current.artist_name}
            </span>
          )}
          {(current.track_title || current.label_name) && (
            <span className="text-sm text-muted-foreground">
              {current.track_title ? `— ${current.track_title}` : ''}
              {current.track_title && current.label_name ? ' — ' : ''}
              {!current.track_title && current.label_name ? '— ' : ''}
              {current.label_slug ? (
                <Link
                  href={`/labels/${current.label_slug}`}
                  onClick={onNavigate}
                  className="hover:text-foreground transition-colors"
                >
                  {current.label_name}
                </Link>
              ) : (
                current.label_name
              )}
            </span>
          )}
        </div>
      )}

      {recentArtists.length > 0 && (
        <div className="flex items-baseline gap-1.5">
          <span className="shrink-0 text-[13px] text-muted-foreground">Recently:</span>
          <ArtistHops
            hops={recentArtists}
            onNavigate={onNavigate}
            className="text-[13px] font-medium text-foreground"
          />
        </div>
      )}
    </section>
  )
}
