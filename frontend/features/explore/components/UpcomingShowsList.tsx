'use client'

/**
 * UpcomingShowsList (PSY-837)
 *
 * Vertical list of upcoming-show rows for the /explore landing. Matches
 * the homepage's compact-density show row pattern (date · artists ·
 * venue) without the homepage's filters / save / attendance affordances
 * — /explore is a discovery surface, not a saved-list manager.
 *
 * Reads from `useExploreUpcomingShows` which is seeded by the page-
 * level SSR prefetch, so this renders synchronously from cache on
 * first paint.
 */

import Link from 'next/link'
import { useExploreUpcomingShows } from '../hooks'
import { formatShowDateBadge } from '@/lib/utils/showDateBadge'

interface UpcomingShowsListProps {
  limit?: number
}

export function UpcomingShowsList({ limit = 5 }: UpcomingShowsListProps) {
  const { data, isLoading, error } = useExploreUpcomingShows({ limit })

  if (isLoading) {
    return (
      <div className="flex justify-center items-center py-8">
        <div className="animate-spin rounded-full h-6 w-6 border-b-2 border-foreground"></div>
      </div>
    )
  }

  if (error) {
    return (
      <div className="text-center py-8 text-muted-foreground">
        <p>Unable to load shows.</p>
      </div>
    )
  }

  if (!data || data.shows.length === 0) {
    return (
      <div className="text-center py-8 text-muted-foreground">
        <p>No upcoming shows at this time.</p>
      </div>
    )
  }

  return (
    <ul className="flex flex-col divide-y divide-border/40 rounded-lg border border-border/50 bg-card/30">
      {data.shows.map(show => {
        const state = show.state ?? show.venue_state ?? null
        const cityLabel = show.city ?? show.venue_city ?? ''
        const dateBadge = formatShowDateBadge(show.event_date, state)
        const detailsHref = `/shows/${show.slug || show.id}`

        return (
          <li key={show.id}>
            <Link
              href={detailsHref}
              className="flex items-center gap-3 px-3 py-2.5 hover:bg-muted/40 transition-colors"
              aria-label={show.title}
            >
              <span className="text-xs text-muted-foreground shrink-0 w-20 tabular-nums">
                {dateBadge.dayOfWeek} {dateBadge.monthDay}
              </span>
              <span className="font-medium text-sm flex-1 truncate">
                {show.headliner_name || show.title}
              </span>
              <span className="text-xs text-muted-foreground shrink-0 hidden sm:inline truncate max-w-[40%]">
                {show.venue_name}
                {(cityLabel || state) && (
                  <span className="text-muted-foreground/70">
                    {' '}&middot; {[cityLabel, state].filter(Boolean).join(', ')}
                  </span>
                )}
              </span>
            </Link>
          </li>
        )
      })}
    </ul>
  )
}
