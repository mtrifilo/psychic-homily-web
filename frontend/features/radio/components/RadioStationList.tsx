'use client'

import { Star } from 'lucide-react'
import { cn } from '@/lib/utils'
import { formatStationLocation } from '../lib/stationOverview'
import type { RadioStationListItem } from '../types'

interface RadioStationListProps {
  stations: RadioStationListItem[]
  selectedSlug: string
  onSelect: (slug: string) => void
}

/**
 * The D2 left pane (PSY-1016): a selectable list of stations, each with its
 * city and a follow star, with the active station highlighted.
 *
 * The star is presentational for now — following radio stations isn't a
 * supported backend follow target yet (validFollowEntityTypes covers
 * artist/venue/label/festival/user only; radio-follow is deferred to BE-2 /
 * PSY-1022). It marks the active station so the affordance is visible without
 * promising a toggle that would 400.
 */
export function RadioStationList({
  stations,
  selectedSlug,
  onSelect,
}: RadioStationListProps) {
  return (
    <div
      role="tablist"
      aria-label="Radio stations"
      aria-orientation="vertical"
      className="flex flex-col gap-0.5"
    >
      <h2 className="mb-1 font-mono text-[11px] uppercase tracking-[1.2px] text-muted-foreground">
        Stations
      </h2>
      {stations.map(station => {
        const selected = station.slug === selectedSlug
        const location = formatStationLocation(station.city, station.state)
        return (
          <button
            key={station.id}
            type="button"
            role="tab"
            aria-selected={selected}
            onClick={() => onSelect(station.slug)}
            className={cn(
              'flex w-full items-center gap-2 rounded-md p-2 text-left outline-none transition-colors focus-visible:ring-2 focus-visible:ring-ring/50',
              selected ? 'bg-muted' : 'hover:bg-muted/50'
            )}
          >
            <Star
              className={cn(
                'size-3.5 shrink-0',
                selected
                  ? 'fill-primary text-primary'
                  : 'text-muted-foreground/60'
              )}
              aria-hidden
            />
            <span className="flex min-w-0 flex-col">
              <span
                className={cn(
                  'truncate text-[15px]',
                  selected ? 'font-semibold text-foreground' : 'font-medium text-foreground'
                )}
              >
                {station.name}
              </span>
              {location && (
                <span className="truncate text-[11px] text-muted-foreground">{location}</span>
              )}
            </span>
          </button>
        )
      })}
    </div>
  )
}
