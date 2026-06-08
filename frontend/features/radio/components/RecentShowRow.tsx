'use client'

import Link from 'next/link'
import { useShowLatestEpisode } from '../hooks/useShowLatestEpisode'
import {
  formatShortAirDate,
  recentArtistsFromEpisode,
} from '../lib/stationOverview'
import { ArtistHops } from './ArtistHops'
import type { RadioShowListItem } from '../types'

interface RecentShowRowProps {
  show: RadioShowListItem
  stationSlug: string
  /** Called when a link is followed (e.g. to close the nav panel). */
  onNavigate?: () => void
}

/**
 * One entry in the D2 "Recent shows" list (PSY-1016): show name (linking to
 * the show page) + its latest air-date + a one-line vibe + a few artists
 * played, each an entity hop. Fetches its own latest episode so the panel can
 * render a bounded handful of these without an N+1 in the parent hook.
 *
 * The "vibe" line uses the show's genre tags when present (the show-list item
 * doesn't carry a description). When there are no tags it's omitted — the row
 * degrades to name + date + artists rather than inventing copy.
 */
export function RecentShowRow({ show, stationSlug, onNavigate }: RecentShowRowProps) {
  const { episode } = useShowLatestEpisode(show.slug)

  const airDate = formatShortAirDate(episode?.air_date)
  const artists = recentArtistsFromEpisode(episode?.plays, { limit: 3 })
  const vibe = (show.genre_tags ?? []).slice(0, 2).join(' · ')

  // Don't render a recent-show row with no recoverable detail at all — no
  // air-date and no artists means the latest episode hasn't loaded / has no
  // plays, and a bare title adds nothing here.
  if (!airDate && artists.length === 0) return null

  const showUrl = `/radio/${stationSlug}/${show.slug}`

  return (
    <div className="flex flex-col gap-0.5">
      <div className="flex items-center gap-1.5 flex-wrap">
        <Link
          href={showUrl}
          onClick={onNavigate}
          className="text-sm font-semibold text-foreground hover:text-primary transition-colors"
        >
          {show.name}
        </Link>
        {(airDate || vibe) && (
          <span className="text-[13px] text-muted-foreground">
            {airDate && `· ${airDate}`}
            {airDate && vibe && ' — '}
            {!airDate && vibe && '· '}
            {vibe}
          </span>
        )}
      </div>
      {artists.length > 0 && (
        <ArtistHops
          hops={artists}
          onNavigate={onNavigate}
          className="text-[13px] font-medium text-foreground"
        />
      )}
    </div>
  )
}
