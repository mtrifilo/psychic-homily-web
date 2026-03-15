'use client'

import Link from 'next/link'
import { Calendar, UserCheck, MapPin } from 'lucide-react'
import type { ActiveVenue } from '../types'

interface ActiveVenuesListProps {
  venues: ActiveVenue[]
  compact?: boolean
}

export function ActiveVenuesList({ venues, compact = false }: ActiveVenuesListProps) {
  if (venues.length === 0) {
    return (
      <p className="text-sm text-muted-foreground py-4 text-center">
        No active venues right now.
      </p>
    )
  }

  return (
    <ol className="space-y-1">
      {venues.map((venue, index) => (
        <li key={venue.venue_id}>
          <Link
            href={`/venues/${venue.slug}`}
            className="group flex items-center gap-3 rounded-lg px-3 py-2.5 transition-colors hover:bg-muted/50"
          >
            <span className="flex h-6 w-6 shrink-0 items-center justify-center rounded-full bg-muted text-xs font-semibold text-muted-foreground">
              {index + 1}
            </span>
            <div className="min-w-0 flex-1">
              <p className="text-sm font-medium group-hover:text-primary truncate">
                {venue.name}
              </p>
              {!compact && (
                <div className="mt-0.5 flex items-center gap-1 text-xs text-muted-foreground">
                  <MapPin className="h-3 w-3" />
                  {venue.city}{venue.state ? `, ${venue.state}` : ''}
                </div>
              )}
            </div>
            {!compact && (
              <div className="flex shrink-0 items-center gap-3 text-xs text-muted-foreground">
                <span className="flex items-center gap-1" title="Upcoming shows">
                  <Calendar className="h-3 w-3" />
                  {venue.upcoming_show_count}
                </span>
                <span className="flex items-center gap-1" title="Followers">
                  <UserCheck className="h-3 w-3" />
                  {venue.follow_count}
                </span>
              </div>
            )}
          </Link>
        </li>
      ))}
    </ol>
  )
}
