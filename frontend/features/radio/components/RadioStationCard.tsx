'use client'

import Link from 'next/link'
import { Radio, MapPin, Music } from 'lucide-react'
import { Badge } from '@/components/ui/badge'
import { getBroadcastTypeLabel } from '../types'
import type { RadioStationListItem } from '../types'

interface RadioStationCardProps {
  station: RadioStationListItem
}

export function RadioStationCard({ station }: RadioStationCardProps) {
  const stationUrl = `/radio/${station.slug}`
  const location = [station.city, station.state].filter(Boolean).join(', ')
  const broadcastLabel = getBroadcastTypeLabel(station.broadcast_type)

  return (
    <article className="rounded-lg border border-border/50 bg-card p-5 transition-shadow hover:shadow-sm">
      <div className="flex gap-4">
        {/* Station Icon / Logo */}
        <div className="shrink-0 rounded-lg bg-muted/50 flex items-center justify-center overflow-hidden h-16 w-16">
          {station.logo_url ? (
            <img
              src={station.logo_url}
              alt={`${station.name} logo`}
              className="h-full w-full object-cover"
            />
          ) : (
            <Radio className="h-8 w-8 text-muted-foreground/40" />
          )}
        </div>

        {/* Content */}
        <div className="flex-1 min-w-0">
          <Link href={stationUrl} className="block group">
            <h3 className="font-bold text-lg text-foreground group-hover:text-primary transition-colors truncate">
              {station.name}
            </h3>
          </Link>

          <div className="flex items-center gap-2 flex-wrap mt-1">
            <Badge variant="secondary" className="text-[10px] px-1.5 py-0">
              {broadcastLabel}
            </Badge>
            {station.frequency_mhz && (
              <span className="text-sm text-muted-foreground tabular-nums">
                {station.frequency_mhz} MHz
              </span>
            )}
          </div>

          <div className="flex items-center gap-4 mt-2 text-sm text-muted-foreground">
            {location && (
              <span className="flex items-center gap-1">
                <MapPin className="h-3.5 w-3.5" />
                {location}
              </span>
            )}
            {station.show_count > 0 && (
              <span className="flex items-center gap-1">
                <Music className="h-3.5 w-3.5" />
                {station.show_count} {station.show_count === 1 ? 'show' : 'shows'}
              </span>
            )}
          </div>
        </div>
      </div>

    </article>
  )
}
