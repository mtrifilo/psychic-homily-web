'use client'

import Link from 'next/link'
import { Calendar, MapPin, Users } from 'lucide-react'
import { Badge } from '@/components/ui/badge'
import {
  getFestivalStatusLabel,
  getFestivalStatusVariant,
  formatFestivalDateRange,
} from '../types'
import type { FestivalListItem } from '../types'

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
  const festivalUrl = `/festivals/${festival.slug}`

  if (density === 'compact') {
    return (
      <article className="flex items-center gap-3 px-3 py-1.5 hover:bg-muted/50 rounded-md transition-colors">
        {festival.edition_year && (
          <span className="text-xs font-semibold text-muted-foreground shrink-0 tabular-nums">
            {festival.edition_year}
          </span>
        )}
        <Link
          href={festivalUrl}
          className="font-medium text-sm truncate flex-1 hover:text-primary"
        >
          {festival.name}
        </Link>
        {location && (
          <span className="text-xs text-muted-foreground shrink-0">{location}</span>
        )}
        {dateRange && (
          <span className="text-xs text-muted-foreground shrink-0">{dateRange}</span>
        )}
        <span className="text-xs text-muted-foreground shrink-0 tabular-nums">
          {festival.artist_count} artists
        </span>
      </article>
    )
  }

  if (density === 'expanded') {
    return (
      <article className="rounded-lg border border-border/50 bg-card transition-shadow hover:shadow-sm p-6">
        <div className="flex gap-4">
          {/* Date badge */}
          <div className="shrink-0 rounded-md bg-muted/50 flex flex-col items-center justify-center text-center h-20 w-20">
            <span className="text-lg font-bold leading-tight text-foreground">
              {festival.edition_year}
            </span>
          </div>

          {/* Text Content */}
          <div className="flex-1 min-w-0">
            <Link href={festivalUrl} className="block group">
              <h3
                className="font-bold text-xl text-foreground group-hover:text-primary transition-colors line-clamp-2"
                title={festival.name}
              >
                {festival.name}
              </h3>
            </Link>

            <div className="flex items-center gap-2 flex-wrap mt-2">
              <Badge
                variant={getFestivalStatusVariant(festival.status)}
                className="text-[10px] px-1.5 py-0"
              >
                {getFestivalStatusLabel(festival.status)}
              </Badge>
              {location && (
                <span className="flex items-center gap-1 text-sm text-muted-foreground">
                  <MapPin className="h-3.5 w-3.5" />
                  {location}
                </span>
              )}
            </div>

            <div className="mt-2 flex items-center gap-4 text-sm text-muted-foreground">
              <span className="flex items-center gap-1">
                <Calendar className="h-4 w-4" />
                {dateRange}
              </span>
              <span className="flex items-center gap-1">
                <Users className="h-4 w-4" />
                {festival.artist_count === 1
                  ? '1 artist'
                  : `${festival.artist_count} artists`}
              </span>
              {festival.venue_count > 0 && (
                <span className="text-sm text-muted-foreground">
                  {festival.venue_count === 1
                    ? '1 venue'
                    : `${festival.venue_count} venues`}
                </span>
              )}
            </div>
          </div>
        </div>
      </article>
    )
  }

  // Comfortable (default)
  return (
    <article className="rounded-lg border border-border/50 bg-card transition-shadow hover:shadow-sm p-4">
      <div className="flex gap-3">
        {/* Date badge */}
        <div className="shrink-0 rounded-md bg-muted/50 flex flex-col items-center justify-center text-center h-16 w-16">
          <span className="text-base font-bold leading-tight text-foreground">
            {festival.edition_year}
          </span>
        </div>

        {/* Text Content */}
        <div className="flex-1 min-w-0">
          <Link href={festivalUrl} className="block group">
            <h3
              className="font-bold text-base text-foreground group-hover:text-primary transition-colors line-clamp-2"
              title={festival.name}
            >
              {festival.name}
            </h3>
          </Link>

          <div className="flex items-center gap-2 flex-wrap mt-1">
            <Badge
              variant={getFestivalStatusVariant(festival.status)}
              className="text-[10px] px-1.5 py-0"
            >
              {getFestivalStatusLabel(festival.status)}
            </Badge>
            {location && (
              <span className="flex items-center gap-1 text-sm text-muted-foreground">
                <MapPin className="h-3 w-3" />
                {location}
              </span>
            )}
          </div>

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
        </div>
      </div>
    </article>
  )
}
