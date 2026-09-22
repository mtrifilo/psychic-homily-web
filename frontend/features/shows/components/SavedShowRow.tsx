'use client'

import type { ReactNode } from 'react'
import Link from 'next/link'
import { formatShowTime } from '@/lib/utils/formatters'
import { formatShowDateBadge } from '@/lib/utils/showDateBadge'
import { formatRelativeTime } from '@/lib/formatRelativeTime'
import type { SavedShowResponse } from '../types'

/**
 * The saved-show ledger row's grid: a fixed date column, a flexible bill, and
 * a trailing stack that becomes a third column at `md`.
 *
 * Exported because the states that sit directly above and below a run of these
 * rows (the signed-in home's prompt row) must line up with them. A second copy
 * of this string is how they stop lining up.
 */
export const SAVED_SHOW_ROW_GRID =
  'grid grid-cols-[74px_minmax(0,1fr)] gap-x-3 border-b border-border py-2.5 md:grid-cols-[104px_minmax(0,1fr)_auto] md:gap-x-5 md:py-3'

/**
 * One saved-show ledger row, shared by the Library Shows tab and the signed-in
 * home module (PSY-2103).
 *
 * The two surfaces differ only in the trailing control, so that control is a
 * slot rather than a variant flag: Library owns an "✕ remove" affordance
 * alongside its expand/collapse table, home renders the pressed SaveButton so
 * an unsave happens in place. The `saved {relative}` stamp above it is common
 * to both and stays here.
 *
 * The markup is the Library row verbatim. Its grid template, mobile/desktop
 * date split and venue line are one contract shared by both tables, not a
 * per-caller starting point.
 */
export function SavedShowRow({
  show,
  isPast,
  action,
}: {
  show: SavedShowResponse
  isPast: boolean
  action: ReactNode
}) {
  const venue = show.venues[0]
  const artists = show.artists
  const dateBadge = formatShowDateBadge(
    show.event_date,
    show.state,
    show.venues?.[0]?.timezone
  )
  // Null on a guessed zone; the date column then holds the date alone.
  const startTime = formatShowTime(
    show.event_date,
    show.state,
    show.venues?.[0]?.timezone
  )

  return (
    <article
      aria-label={show.title}
      className={SAVED_SHOW_ROW_GRID}
    >
      <div
        className={`row-span-2 font-mono text-[11px] font-bold uppercase md:row-span-1 md:text-xs ${
          isPast ? 'text-muted-foreground' : 'text-primary'
        }`}
      >
        <span className="md:hidden">{dateBadge.monthDay}</span>
        <span className="hidden md:inline">
          {dateBadge.dayOfWeek} {dateBadge.monthDay}
        </span>
        {startTime && (
          <div className="mt-0.5 hidden text-[11px] font-normal normal-case text-muted-foreground md:block">
            {startTime}
          </div>
        )}
      </div>

      <div className="min-w-0 self-center">
        <Link
          href={`/shows/${show.slug || show.id}`}
          className="block truncate text-sm font-medium leading-tight transition-colors hover:text-primary md:text-[15px]"
        >
          {artists.map(a => a.name).join(' · ')}
        </Link>

        <div className="mt-0.5 truncate text-xs text-muted-foreground md:text-[13px]">
          {venue && (
            <>
              {venue.slug ? (
                <Link
                  href={`/venues/${venue.slug}`}
                  className={`transition-colors hover:text-primary ${
                    isPast ? '' : 'md:text-primary/80'
                  }`}
                >
                  {venue.name}
                </Link>
              ) : (
                <span className={isPast ? undefined : 'md:text-primary/80'}>
                  {venue.name}
                </span>
              )}
              {(venue.city || venue.state) && (
                <span>
                  {' '}
                  &middot;{' '}
                  {[venue.city, venue.state].filter(Boolean).join(', ')}
                </span>
              )}
            </>
          )}
        </div>
      </div>

      <div className="col-start-2 mt-1 flex items-center justify-between gap-3 font-mono text-[11px] text-muted-foreground md:col-start-3 md:row-start-1 md:mt-0 md:flex-col md:items-end md:self-center">
        <span className="whitespace-nowrap">
          saved {formatRelativeTime(show.saved_at, { short: true })}
        </span>
        {action}
      </div>
    </article>
  )
}
