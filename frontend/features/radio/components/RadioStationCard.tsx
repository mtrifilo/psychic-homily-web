'use client'

import { Radio, MapPin, Music } from 'lucide-react'
import { Badge } from '@/components/ui/badge'
import { EntityCardTitle } from '@/components/shared'
import { getBroadcastTypeLabel } from '../types'
import type { RadioStationListItem } from '../types'

interface RadioStationCardProps {
  station: RadioStationListItem
}

export function RadioStationCard({ station }: RadioStationCardProps) {
  const stationUrl = `/radio/${station.slug}`
  const location = [station.city, station.state].filter(Boolean).join(', ')
  const broadcastLabel = getBroadcastTypeLabel(station.broadcast_type)
  // PSY-673: only flagship cards advertise sibling channels. Non-flagship
  // cards never reach this component on /radio (filtered by
  // isStationVisibleOnIndex); the is_flagship guard keeps the count from
  // appearing if the card is reused on a surface that doesn't filter.
  const siblingCount = station.sibling_stations.length
  const showChannels = station.network?.is_flagship && siblingCount > 0

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
          <EntityCardTitle name={station.name} href={stationUrl} />

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

          {showChannels && (
            <div className="mt-1.5 text-xs text-muted-foreground/80">
              + {siblingCount} {siblingCount === 1 ? 'channel' : 'channels'}
            </div>
          )}
        </div>
      </div>

    </article>
  )
}
