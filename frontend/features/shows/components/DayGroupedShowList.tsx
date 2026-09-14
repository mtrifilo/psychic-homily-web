'use client'

import { useMemo } from 'react'
import { cn } from '@/lib/utils'
import type { Density } from '@/lib/hooks/common/useDensity'
import { useHydrated } from '@/lib/hooks/common/useHydrated'
import { batchedSaveFor } from '@/components/shared/batchedSaveData'
import type { SaveCounts } from '@/components/shared/batchedSaveData'
import { ShowCard } from './ShowCard'
import {
  dayGroupHeading,
  groupShowsByVenueLocalDay,
  type ShowDayGroup,
} from '../dayGroups'
import type { ShowResponse } from '../types'

export interface DayGroupedShowListProps {
  /** Rows in list order. Grouping preserves that order exactly. */
  shows: ShowResponse[]
  density: Density
  isAdmin: boolean
  userId?: string
  /** The batch save-count map, as `useShowSaveCountBatch` returns it. */
  saveCounts?: Record<string, SaveCounts>
}

/** Space above a day heading, per density. The first group takes none. */
const groupSpacingClass: Record<Density, string> = {
  compact: 'mt-4',
  comfortable: 'mt-6',
  expanded: 'mt-8',
}

/** Space between the day's rule and its first row, per density. */
const rowsSpacingClass: Record<Density, string> = {
  compact: 'mt-1 gap-0.5',
  comfortable: 'mt-3 gap-3',
  expanded: 'mt-4 gap-5',
}

function DayGroupSection({
  group,
  density,
  isAdmin,
  userId,
  saveCounts,
  isFirst,
}: {
  group: ShowDayGroup
  density: Density
  isAdmin: boolean
  userId?: string
  saveCounts?: Record<string, SaveCounts>
  isFirst: boolean
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
        <>
          <h2
            // Only the first group of a date carries the anchor: an id is
            // unique per document, and a page can hold two groups of one date.
            id={group.anchorId ?? undefined}
            className={cn(
              'scroll-mt-20 pb-1.5 font-mono text-[10.5px] font-bold tracking-[1px] uppercase',
              group.isToday ? 'text-primary' : 'text-muted-foreground'
            )}
          >
            {dayGroupHeading(group)}
          </h2>
          <div
            className={cn(
              'border-t',
              group.isToday ? 'border-primary' : 'border-border'
            )}
          />
        </>
      )}
      <div className={cn('flex flex-col', rowsSpacingClass[density])}>
        {group.rows.map(show => (
          <ShowCard
            key={show.id}
            show={show}
            isAdmin={isAdmin}
            userId={userId}
            saveData={batchedSaveFor(saveCounts, show.id)}
            density={density}
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
 * in the server HTML. Naming today means reading a clock, and this list renders
 * inside the route's prerendered shell, where a clock read would move the whole
 * subtree into the dynamic resume. The prefix is a text change inside a
 * full-width heading row, so nothing beside it moves when it arrives.
 */
export function DayGroupedShowList({
  shows,
  density,
  isAdmin,
  userId,
  saveCounts,
}: DayGroupedShowListProps) {
  const hydrated = useHydrated()

  const groups = useMemo(
    () => groupShowsByVenueLocalDay(shows, hydrated ? new Date() : null),
    [shows, hydrated]
  )

  return (
    <div className="min-w-0" data-testid="day-grouped-show-list">
      {groups.map((group, index) => (
        <DayGroupSection
          key={`${group.dateKey ?? 'undated'}-${index}`}
          group={group}
          density={density}
          isAdmin={isAdmin}
          userId={userId}
          saveCounts={saveCounts}
          isFirst={index === 0}
        />
      ))}
    </div>
  )
}
