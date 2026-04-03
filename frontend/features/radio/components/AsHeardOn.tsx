'use client'

import Link from 'next/link'
import { Radio } from 'lucide-react'
import { useArtistRadioPlays } from '../hooks/useArtistRadioPlays'
import { useReleaseRadioPlays } from '../hooks/useReleaseRadioPlays'
import type { RadioAsHeardOn } from '../types'

interface AsHeardOnProps {
  entityType: 'artist' | 'release'
  entitySlug: string
  enabled?: boolean
}

function AsHeardOnList({ items }: { items: RadioAsHeardOn[] }) {
  if (items.length === 0) return null

  return (
    <div>
      <h3 className="text-xs font-semibold text-muted-foreground uppercase tracking-wider mb-2">
        As Heard On
      </h3>
      <div className="space-y-2">
        {items.map(item => (
          <Link
            key={`${item.station_id}-${item.show_id}`}
            href={`/radio/${item.station_slug}/${item.show_slug}`}
            className="flex items-center gap-2 text-sm text-muted-foreground hover:text-foreground transition-colors py-0.5 group"
          >
            <Radio className="h-3.5 w-3.5 shrink-0 text-muted-foreground/60 group-hover:text-foreground" />
            <div className="flex-1 min-w-0">
              <span className="truncate block">{item.show_name}</span>
              <span className="text-xs text-muted-foreground/60">
                {item.station_name} - {item.play_count} plays
              </span>
            </div>
          </Link>
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
