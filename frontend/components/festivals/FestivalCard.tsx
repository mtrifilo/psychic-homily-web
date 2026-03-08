'use client'

import Link from 'next/link'
import { Calendar, MapPin, Users } from 'lucide-react'
import { cn } from '@/lib/utils'
import { Badge } from '@/components/ui/badge'
import {
  getFestivalStatusLabel,
  getFestivalStatusVariant,
  formatFestivalDateRange,
} from '@/lib/types/festival'
import type { FestivalListItem } from '@/lib/types/festival'

export type FestivalCardDensity = 'compact' | 'comfortable' | 'expanded'

interface FestivalCardProps {
  festival: FestivalListItem
  density?: FestivalCardDensity
}

export function FestivalCard({
  festival,
  density = 'comfortable',
}: FestivalCardProps) {
  const location =
    festival.city && festival.state
      ? `${festival.city}, ${festival.state}`
      : festival.city ?? festival.state ?? null
  const dateRange = formatFestivalDateRange(festival.start_date, festival.end_date)

  return (
    <article
      className={cn(
        'rounded-lg border border-border/50 bg-card transition-shadow hover:shadow-sm',
        density === 'compact' && 'p-3',
        density === 'comfortable' && 'p-4',
        density === 'expanded' && 'p-5'
      )}
    >
      <div className="flex gap-3">
        {/* Date badge */}
        <div
          className={cn(
            'shrink-0 rounded-md bg-muted/50 flex flex-col items-center justify-center text-center',
            density === 'compact' && 'h-12 w-12',
            density === 'comfortable' && 'h-16 w-16',
            density === 'expanded' && 'h-20 w-20'
          )}
        >
          <span
            className={cn(
              'font-bold leading-tight text-foreground',
              density === 'compact' && 'text-sm',
              density === 'comfortable' && 'text-base',
              density === 'expanded' && 'text-lg'
            )}
          >
            {festival.edition_year}
          </span>
        </div>

        {/* Text Content */}
        <div className="flex-1 min-w-0">
          <Link href={`/festivals/${festival.slug}`} className="block group">
            <h3
              className={cn(
                'font-bold text-foreground group-hover:text-primary transition-colors truncate',
                density === 'compact' && 'text-sm',
                density === 'comfortable' && 'text-base',
                density === 'expanded' && 'text-lg'
              )}
            >
              {festival.name}
            </h3>
          </Link>

          <div
            className={cn(
              'flex items-center gap-2 flex-wrap',
              density === 'compact' && 'mt-0.5',
              density === 'comfortable' && 'mt-1',
              density === 'expanded' && 'mt-1.5'
            )}
          >
            <Badge
              variant={getFestivalStatusVariant(festival.status)}
              className="text-[10px] px-1.5 py-0"
            >
              {getFestivalStatusLabel(festival.status)}
            </Badge>
            {location && (
              <span
                className={cn(
                  'flex items-center gap-1 text-muted-foreground',
                  density === 'compact' ? 'text-xs' : 'text-sm'
                )}
              >
                <MapPin className="h-3 w-3" />
                {location}
              </span>
            )}
          </div>

          {density !== 'compact' && (
            <div className="mt-1 flex items-center gap-3 text-sm text-muted-foreground">
              <span className="flex items-center gap-1">
                <Calendar className="h-3.5 w-3.5" />
                {dateRange}
              </span>
              <span className="flex items-center gap-1">
                <Users className="h-3.5 w-3.5" />
                {festival.artist_count === 1
                  ? '1 artist'
                  : `${festival.artist_count} artists`}
              </span>
            </div>
          )}
        </div>
      </div>
    </article>
  )
}
