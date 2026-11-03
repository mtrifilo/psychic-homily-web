'use client'

import type { ReactNode } from 'react'
import Link from 'next/link'
import { SaveButton } from '@/components/shared/SaveButton'
import type { BatchedSaveData } from '@/components/shared/batchedSaveData'
import { useHydrated } from '@/lib/hooks/common/useHydrated'
import { cn } from '@/lib/utils'
import { formatShowTime } from '@/lib/utils/formatters'
import {
  formatShowDateBadge,
  showRelativeDay,
  type ShowRelativeDay,
} from '@/lib/utils/showDateBadge'
import type { UnsaveSyncMode } from '../hooks/useSavedShows'
import type { ShowResponse } from '../types'
import { ShowStatusBadge } from './ShowStatusBadge'

/**
 * The show row's grid: a fixed "when" column, a flexible bill, and a trailing
 * column for the heart or the save age, at every width.
 *
 * Exported because a state that sits directly above or below a run of these
 * rows (the signed-in home's prompt row) must line up with them. A second copy
 * of this string is how they stop lining up.
 *
 * `min-h-14` is the 44px heart target plus the 6px padding on each side, so a
 * row without a heart is the same height as one with it.
 */
export const SHOW_ROW_GRID =
  'grid min-h-14 grid-cols-[74px_minmax(0,1fr)_auto] items-center gap-x-3 border-b border-border py-1.5 md:grid-cols-[104px_minmax(0,1fr)_auto] md:gap-x-5'

const RELATIVE_DAY_LABEL: Record<ShowRelativeDay, string> = {
  today: 'Tonight',
  tomorrow: 'Tomorrow',
}

/** "SAT NOV 7", on the venue's calendar. */
function formatDateLabel(
  eventDate: string,
  state: string | null | undefined,
  timezone: string | null | undefined
): string {
  const { dayOfWeek, monthDay } = formatShowDateBadge(eventDate, state, timezone)
  return `${dayOfWeek} ${monthDay}`
}

/**
 * One show as a dense two-line row, shared by the signed-in home's saved and
 * nearby lists and Library's Shows tab, so the three read as one vocabulary.
 *
 * The "when" column says TONIGHT or TOMORROW on the venue's own calendar, and
 * the date otherwise, over the venue-local start time. Every word in it is
 * venue-local: a reader in another zone sees the venue's day and clock.
 *
 * The trailing column is a slot because the surfaces differ only there: home
 * puts a heart, Library a heart over the save age, Library's past rows the
 * save age alone.
 *
 * A `past` row is a record, not a plan: muted, and dated rather than named
 * relative to today.
 */
export function ShowRow({
  show,
  past = false,
  trailing,
}: {
  show: ShowResponse
  past?: boolean
  trailing: ReactNode
}) {
  // The server and the hydration render name no relative day: "now" is the
  // reader's clock, which the server does not share, so both print the date
  // and the word arrives a commit later (the same gate as `/shows`' TONIGHT).
  const hydrated = useHydrated()
  const venue = show.venues[0]
  const timezone = venue?.timezone
  const relativeDay =
    past || !hydrated
      ? null
      : showRelativeDay(show.event_date, show.state, timezone, new Date())
  const when = relativeDay
    ? RELATIVE_DAY_LABEL[relativeDay]
    : formatDateLabel(show.event_date, show.state, timezone)
  // Null on a guessed zone; the column then holds the date alone.
  const startTime = formatShowTime(show.event_date, show.state, timezone)
  const lineup = show.artists.map(a => a.name).join(' · ') || show.title

  return (
    <article
      aria-label={show.title}
      className={cn(SHOW_ROW_GRID, show.is_cancelled && 'opacity-60')}
    >
      <div className="font-data text-[11px] leading-[14px]">
        <div
          className={cn(
            'whitespace-nowrap font-bold uppercase tracking-[0.4px] md:text-xs',
            past ? 'text-muted-foreground' : 'text-primary'
          )}
        >
          {when}
        </div>
        {startTime && (
          <div className="mt-0.5 whitespace-nowrap text-muted-foreground tabular-nums">
            {startTime}
          </div>
        )}
      </div>

      <div className="min-w-0">
        <div className="flex min-w-0 items-center gap-1.5">
          <Link
            href={`/shows/${show.slug || show.id}`}
            className={cn(
              'min-w-0 truncate text-sm font-medium leading-[18px] transition-colors hover:text-primary md:text-[15px]',
              past ? 'text-muted-foreground' : 'text-foreground'
            )}
          >
            {lineup}
          </Link>
          <ShowStatusBadge
            show={show}
            cancelledVariant="default"
            className="inline-flex shrink-0 gap-1"
          />
        </div>

        {venue && (
          <div className="mt-0.5 truncate text-xs leading-4 text-muted-foreground md:text-[13px]">
            {venue.slug ? (
              <Link
                href={`/venues/${venue.slug}`}
                className="transition-colors hover:text-primary"
              >
                {venue.name}
              </Link>
            ) : (
              venue.name
            )}
            {(venue.city || venue.state) && (
              <span>
                {' '}
                &middot; {[venue.city, venue.state].filter(Boolean).join(', ')}
              </span>
            )}
          </div>
        )}
      </div>

      <div className="flex flex-col items-end">{trailing}</div>
    </article>
  )
}

/**
 * The row's heart: icon only, no count, an 18px glyph in a 44px target.
 *
 * The public count stays in SaveButton's accessible name; painting it here
 * would read as part of a list that is about this viewer's saves.
 */
export function ShowRowSaveButton({
  showId,
  saveData,
  unsaveSync,
  disabled,
}: {
  showId: number
  saveData?: BatchedSaveData
  unsaveSync?: UnsaveSyncMode
  disabled?: boolean
}) {
  return (
    <SaveButton
      showId={showId}
      saveData={saveData}
      unsaveSync={unsaveSync}
      disabled={disabled}
      showCount={false}
      // Important: Button's own `[&_svg:not([class*='size-'])]:size-4` is the
      // more specific selector and would hold the glyph at 16px.
      className="h-11 w-11 [&_svg]:size-[18px]!"
    />
  )
}
