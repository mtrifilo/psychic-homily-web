'use client'

import { useMemo } from 'react'
import Link from 'next/link'
import { cn } from '@/lib/utils'
import type { Density } from '@/lib/hooks/common/useDensity'
import { useHydrated } from '@/lib/hooks/common/useHydrated'
import { batchedSaveFor } from '@/components/shared/batchedSaveData'
import type { SaveCounts } from '@/components/shared/batchedSaveData'
import {
  DayGroupedShowListHeader,
  DayGroupedShowRow,
} from './DayGroupedShowRow'
import {
  dayGroupHeading,
  groupShowsByVenueLocalDay,
  type ShowDayGroup,
} from '../dayGroups'
import { showsDayPathFromDateKey } from '../showsCalendarRoute'
import type { ShowResponse } from '../types'

export interface DayGroupedShowListProps {
  /** Rows in list order. Grouping preserves that order exactly. */
  shows: ShowResponse[]
  density: Density
  isAdmin: boolean
  userId?: string
  /** The batch save-count map, as `useShowSaveCountBatch` returns it. */
  saveCounts?: Record<string, SaveCounts>
  /**
   * Whether the venue column names the city. Comes from the FILTER rather than
   * from the rows: a page of an All Cities list can happen to hold one metro,
   * and a rows-derived flag would make the column appear on page 1 and vanish
   * on page 2 of one filter.
   */
  showCity: boolean
}

/** Space above a day heading, per density. The first group takes none. */
const groupSpacingClass: Record<Density, string> = {
  compact: 'mt-4',
  comfortable: 'mt-6',
  expanded: 'mt-8',
}

/**
 * Space between the day's rule and its first row, per density.
 *
 * No gap BETWEEN rows: they carry an alternating fill, and a gap would break
 * the stripe into floating bands. Density moves the padding inside each row
 * instead, which is what `DayGroupedShowRow` does.
 */
const rowsSpacingClass: Record<Density, string> = {
  compact: 'mt-1',
  comfortable: 'mt-1.5',
  expanded: 'mt-2',
}

/**
 * A day heading's text, linked to that day's own page when the date can be
 * read.
 *
 * The LINK is inside the heading rather than around it so the heading keeps its
 * anchor id and its role: a reader jumping to `#d-2026-11-14` lands on the
 * heading, and a reader following the text lands on the day.
 *
 * An unlinked heading is the honest fallback for a date the route grammar
 * refuses, which is the same set of dates the badge above already declines to
 * name.
 */
function DayHeadingLabel({ group }: { group: ShowDayGroup }) {
  const heading = dayGroupHeading(group)
  const href = group.dateKey === null ? null : showsDayPathFromDateKey(group.dateKey)
  if (href === null) return <>{heading}</>
  return (
    <Link href={href} className="rounded-sm hover:underline focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring">
      {heading}
    </Link>
  )
}

function DayGroupSection({
  group,
  density,
  isAdmin,
  userId,
  saveCounts,
  isFirst,
  showCity,
}: {
  group: ShowDayGroup
  density: Density
  isAdmin: boolean
  userId?: string
  saveCounts?: Record<string, SaveCounts>
  isFirst: boolean
  showCity: boolean
}) {
  return (
    <section
      className={cn(!isFirst && groupSpacingClass[density])}
      data-testid="show-day-group"
      data-date={group.dateKey ?? undefined}
    >
      {/* A run whose date cannot be read gets NO heading. The formatters
          answer an unparseable date with the literal "INVALID DATE", and a
          heading is a much louder place to print that than the date tile
          inside a row: it would caption a run of otherwise readable rows with
          a non-date. Without one the rows simply appear ungrouped, which is
          what is actually known about them. */}
      {group.dateKey !== null && (
        <h2
          // Only the first group of a date carries the anchor: an id is
          // unique per document, and a page can hold two groups of one date.
          id={group.anchorId ?? undefined}
          // `scroll-mt` is the top bar alone and not the top bar plus this
          // heading's own height: the anchor target IS the sticky element, so
          // aligning its top with the offset it pins to is what leaves a
          // fragment jump exactly where scrolling to the day would.
          className={cn(
            'sticky top-[var(--topbar-height)] z-20 flex scroll-mt-[var(--topbar-height)] items-center gap-2',
            // Opaque, so the rows of this day pass underneath rather than
            // through the heading. `z-20` keeps it over those rows and under
            // the top bar (z-50) and any overlay.
            'bg-background px-2 pb-1.5 pt-3.5',
            'font-mono text-sm font-bold uppercase tracking-[1px] text-primary'
          )}
        >
          <span className="shrink-0">
            <DayHeadingLabel group={group} />
          </span>
          {/* Decoration: the heading text beside it already names the day. */}
          <span aria-hidden="true" className="h-px flex-1 bg-primary" />
        </h2>
      )}
      <div
        className={cn('shows-day-rows flex flex-col', rowsSpacingClass[density])}
      >
        {group.rows.map((show, index) => (
          <DayGroupedShowRow
            key={show.id}
            show={show}
            isAdmin={isAdmin}
            userId={userId}
            saveData={batchedSaveFor(saveCounts, show.id)}
            density={density}
            index={index}
            showCity={showCity}
          />
        ))}
      </div>
    </section>
  )
}

/**
 * The `/shows` list, with its rows grouped under day headings.
 *
 * Grouping is by the VENUE-LOCAL date, the same date the row's own tile prints,
 * so an All Cities list groups each show under the day it happens where it
 * happens rather than under the reader's day.
 *
 * The heading gains its TONIGHT prefix one commit after hydration rather than
 * in the server HTML. Naming today means reading a clock, and a clock read
 * straight into render gives the server pass and the hydration pass different
 * answers, which React reports as a hydration error and repairs by throwing the
 * server's markup away. `useHydrated` is the gate for exactly that: it returns
 * the same value in both passes and the refined answer arrives a commit later
 * (see its own doc). The prefix is a text change inside a full-width heading
 * row, so nothing beside it moves when it arrives, and
 * `DayGroupedShowList.test.tsx` pins the server render making no TONIGHT claim.
 */
export function DayGroupedShowList({
  shows,
  density,
  isAdmin,
  userId,
  saveCounts,
  showCity,
}: DayGroupedShowListProps) {
  const hydrated = useHydrated()

  const groups = useMemo(
    () => groupShowsByVenueLocalDay(shows, hydrated ? new Date() : null),
    [shows, hydrated]
  )

  return (
    <div className="min-w-0" data-testid="day-grouped-show-list">
      <DayGroupedShowListHeader density={density} />
      {groups.map((group, index) => (
        <DayGroupSection
          key={`${group.dateKey ?? 'undated'}-${index}`}
          group={group}
          density={density}
          isAdmin={isAdmin}
          userId={userId}
          saveCounts={saveCounts}
          isFirst={index === 0}
          showCity={showCity}
        />
      ))}
    </div>
  )
}
