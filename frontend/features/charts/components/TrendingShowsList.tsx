'use client'

import Link from 'next/link'
import { Calendar, MapPin, Users, Eye } from 'lucide-react'
import type { TrendingShow } from '../types'

interface TrendingShowsListProps {
  shows: TrendingShow[]
  compact?: boolean
}

export function TrendingShowsList({ shows, compact = false }: TrendingShowsListProps) {
  if (shows.length === 0) {
    return (
      <p className="text-sm text-muted-foreground py-4 text-center">
        No upcoming shows right now.
      </p>
    )
  }

  return (
    <ol className="space-y-1">
      {shows.map((show, index) => (
        <li key={show.show_id}>
          <Link
            href={`/shows/${show.slug}`}
            className="group flex items-start gap-3 rounded-lg px-3 py-2.5 transition-colors hover:bg-muted/50"
          >
            <span className="mt-0.5 flex h-6 w-6 shrink-0 items-center justify-center rounded-full bg-muted text-xs font-semibold text-muted-foreground">
              {index + 1}
            </span>
            <div className="min-w-0 flex-1">
              <p className="text-sm font-medium leading-tight group-hover:text-primary truncate">
                {show.title}
              </p>
              {!compact && (
                <div className="mt-1 flex flex-wrap items-center gap-x-3 gap-y-0.5 text-xs text-muted-foreground">
                  <span className="flex items-center gap-1">
                    <Calendar className="h-3 w-3" />
                    {new Date(show.date).toLocaleDateString('en-US', {
                      month: 'short',
                      day: 'numeric',
                    })}
                  </span>
                  <span className="flex items-center gap-1">
                    <MapPin className="h-3 w-3" />
                    <span className="truncate">{show.venue_name}</span>
                  </span>
                  {show.city && (
                    <span className="text-muted-foreground/70">{show.city}</span>
                  )}
                </div>
              )}
            </div>
            <div className="flex shrink-0 items-center gap-3 text-xs text-muted-foreground">
              <span className="flex items-center gap-1" title="Going">
                <Users className="h-3 w-3" />
                {show.going_count}
              </span>
              <span className="flex items-center gap-1" title="Interested">
                <Eye className="h-3 w-3" />
                {show.interested_count}
              </span>
            </div>
          </Link>
        </li>
      ))}
    </ol>
  )
}
