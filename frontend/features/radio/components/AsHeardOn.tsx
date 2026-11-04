'use client'

import Link from 'next/link'
import { Radio } from 'lucide-react'
import { useArtistRadioPlays } from '../hooks/useArtistRadioPlays'
import { useReleaseRadioPlays } from '../hooks/useReleaseRadioPlays'
import { toEpisodeDate } from '../lib/episodeDate'
import { formatShortAirDate } from '../lib/stationOverview'
import type { RadioAsHeardOn } from '../types'

interface AsHeardOnProps {
  entityType: 'artist' | 'release'
  entitySlug: string
  enabled?: boolean
}

/**
 * One station+show the entity was played on. Links to the playlist of the
 * latest episode it was played on; without a `last_played` date the row
 * falls back to the show page and names no date.
 */
function AsHeardOnRow({ item }: { item: RadioAsHeardOn }) {
  const showHref = `/radio/${item.station_slug}/${item.show_slug}`
  const episodeDate = toEpisodeDate(item.last_played)

  return (
    <Link
      href={episodeDate ? `${showHref}/${episodeDate}` : showHref}
      className="flex items-center gap-2 text-sm text-muted-foreground hover:text-foreground transition-colors py-0.5 group"
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
