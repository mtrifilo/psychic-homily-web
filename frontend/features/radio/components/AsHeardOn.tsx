'use client'

import { useId, useState } from 'react'
import Link from 'next/link'
import { ChevronDown, Radio } from 'lucide-react'
import { cn } from '@/lib/utils'
import { useArtistRadioPlays } from '../hooks/useArtistRadioPlays'
import { useReleaseRadioPlays } from '../hooks/useReleaseRadioPlays'
import { toEpisodeDate } from '../lib/episodeDate'
import { episodePlayHref } from '../lib/playAnchor'
import { formatShortAirDate } from '../lib/stationOverview'
import type { RadioAsHeardOn } from '../types'

interface AsHeardOnProps {
  entityType: 'artist' | 'release'
  entitySlug: string
  enabled?: boolean
}

/**
 * One station+show the entity was played on. The row links to `episodes[0]`,
 * the newest episode a day URL can open, at its play (`#play-{play_id}`);
 * with no listed episode it opens the `last_played` day unanchored, and
 * without that date it falls back to the show page and names no date. The
 * second line's date is always `last_played`, the true latest play. A show
 * with more than one play also gets a disclosure listing each episode as its
 * own deep link.
 */
function AsHeardOnRow({ item }: { item: RadioAsHeardOn }) {
  const [expanded, setExpanded] = useState(false)
  const listId = useId()
  const showHref = `/radio/${item.station_slug}/${item.show_slug}`
  const episodeDate = toEpisodeDate(item.last_played)
  const episodes = item.episodes ?? []
  const newest = episodes[0]
  const href = newest
    ? episodePlayHref(item.station_slug, item.show_slug, newest.air_date, newest.play_id)
    : episodeDate
      ? `${showHref}/${episodeDate}`
      : showHref
  const expandable = item.play_count > 1 && episodes.length > 0

  return (
    <div>
      <div className="flex items-center gap-1">
        <Link
          href={href}
          className="flex flex-1 min-w-0 items-center gap-2 text-sm text-muted-foreground hover:text-foreground transition-colors py-0.5 group"
        >
          <Radio className="h-3.5 w-3.5 shrink-0 text-muted-foreground/60 group-hover:text-foreground" />
          <div className="flex-1 min-w-0">
            <span className="truncate block">{item.show_name}</span>
            <span className="text-xs text-muted-foreground/60">
              {item.station_name} · {item.play_count} {item.play_count === 1 ? 'play' : 'plays'}
              {episodeDate && (
                <>
                  {' · last on '}
                  <span className="font-data">{formatShortAirDate(episodeDate)}</span>
                </>
              )}
            </span>
          </div>
        </Link>
        {expandable && (
          <button
            type="button"
            aria-expanded={expanded}
            aria-controls={listId}
            onClick={() => setExpanded(open => !open)}
            className="flex h-6 w-6 shrink-0 items-center justify-center rounded-sm text-muted-foreground hover:text-foreground transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
          >
            <ChevronDown
              aria-hidden="true"
              className={cn(
                'h-3.5 w-3.5 transition-transform motion-reduce:transition-none',
                expanded && 'rotate-180'
              )}
            />
            <span className="sr-only">Plays on {item.show_name}</span>
          </button>
        )}
      </div>
      {expandable && (
        <ul id={listId} hidden={!expanded} className="mt-1 mb-1 space-y-1 pl-5.5">
          {episodes.map(episode => (
            <li key={episode.play_id}>
              <Link
                href={episodePlayHref(item.station_slug, item.show_slug, episode.air_date, episode.play_id)}
                className="flex min-w-0 items-baseline gap-2 text-xs py-0.5 group/play"
              >
                <span className="shrink-0 font-data tabular-nums text-link group-hover/play:underline">
                  {formatShortAirDate(episode.air_date)}
                </span>
                {episode.track_title && (
                  <span className="truncate text-muted-foreground group-hover/play:text-foreground transition-colors">
                    {episode.track_title}
                  </span>
                )}
              </Link>
            </li>
          ))}
        </ul>
      )}
    </div>
  )
}

function AsHeardOnList({ items }: { items: RadioAsHeardOn[] }) {
  if (items.length === 0) return null

  return (
    <div>
      <h3 className="text-xs font-semibold text-muted-foreground mb-2">
        As heard on
      </h3>
      <div className="space-y-2">
        {items.map(item => (
          <AsHeardOnRow key={`${item.station_id}-${item.show_id}`} item={item} />
        ))}
      </div>
    </div>
  )
}

export function AsHeardOn({ entityType, entitySlug, enabled = true }: AsHeardOnProps) {
  const artistQuery = useArtistRadioPlays(
    entitySlug,
    enabled && entityType === 'artist'
  )
  const releaseQuery = useReleaseRadioPlays(
    entitySlug,
    enabled && entityType === 'release'
  )

  const query = entityType === 'artist' ? artistQuery : releaseQuery
  const items = query.data?.stations ?? []

  if (query.isLoading || items.length === 0) return null

  return <AsHeardOnList items={items} />
}
