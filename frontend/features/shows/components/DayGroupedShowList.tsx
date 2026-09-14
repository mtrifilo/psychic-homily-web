'use client'

import { useMemo } from 'react'
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
import type { ShowResponse } from '../types'

export interface DayGroupedShowListProps {
  /** Rows in list order. Grouping preserves that order exactly. */
  shows: ShowResponse[]
  density: Density
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

function DayGroupSection({
  group,
  density,
  saveCounts,
  isFirst,
  showCity,
}: {
  group: ShowDayGroup
  density: Density
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
        {group.rows.map((show, index) => (
          <DayGroupedShowRow
            key={show.id}
            show={show}
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
          saveCounts={saveCounts}
          isFirst={index === 0}
          showCity={showCity}
        />
      ))}
    </div>
  )
}
