'use client'

import { useMemo } from 'react'
import { cn } from '@/lib/utils'
import { useAuthContext } from '@/lib/context/AuthContext'
// By path, not through a barrel: the artist and venue tables reach this module,
// and both barrels pull far more than one control into their routes.
import { SaveButton } from '@/components/shared/SaveButton'
import {
  batchedSaveFor,
  type SaveCounts,
} from '@/components/shared/batchedSaveData'
import { useShowSaveCountBatch } from '../hooks/useSavedShows'

/**
 * The trailing Save column of an entity page's upcoming-show table, and the
 * stacked phone row a table with that column takes below `sm`.
 *
 * `counts` is the batch result map, `undefined` while the batch is pending or
 * disabled; every cell then renders its empty state without a request of its
 * own (see `batchedSaveFor`). A table renders the column exactly when it is
 * handed one of these, so the column cannot exist without the batch behind it.
 */
export interface ShowSaveColumnData {
  counts: Record<string, SaveCounts> | undefined
}

/**
 * One batched save-count request for the rows a table renders.
 *
 * Runs for anonymous viewers too, as the `/shows` list does: the public count
 * is part of every control's accessible name.
 */
export function useShowSaveColumn(
  shows: readonly { id: number }[]
): ShowSaveColumnData {
  const { isAuthenticated, user } = useAuthContext()
  const showIds = useMemo(() => shows.map(show => show.id), [shows])
  const { data } = useShowSaveCountBatch(showIds, isAuthenticated, user?.id)
  return { counts: data }
}

/**
 * The column's fixed width: the cell's 16px of padding plus the widest state
 * the control can take, "Saved" (87px measured: `px-3`, the 16px heart,
 * `gap-1.5` and the word), plus 3px of slack. Every row and the header share
 * it, so a saved state that arrives after hydration widens the control inside
 * its cell and moves no other column.
 */
const SAVE_COLUMN_WIDTH = 'w-[106px]'

/**
 * A table with a Save column has no room for it as a column on a phone, so
 * below `sm` each row stacks into lines (each table places its own cells) and
 * the header row is visually hidden. ONE element tree across the breakpoint,
 * reflowed by CSS rather than swapped, so no row's text is in the document
 * twice. Overriding the display of the table's elements strips their implicit
 * table semantics, so such a table carries explicit ARIA table roles, and its
 * header row stays for assistive tech.
 */
export const STACKED_TABLE = {
  table: 'max-sm:block',
  head: 'max-sm:sr-only',
  body: 'max-sm:block',
  /**
   * `[&>td]:p-0` cancels DenseTable's `[&_td]` cell padding (same specificity,
   * and the breakpoint variant comes later in the stylesheet), so the row's
   * own padding is the stacked row's only padding.
   */
  row: 'max-sm:grid max-sm:items-baseline max-sm:gap-x-2 max-sm:gap-y-1 max-sm:px-2 max-sm:py-2 max-sm:[&>td]:p-0',
  /**
   * Drops a cell's line from the stacked row while keeping the cell, so every
   * row still has one cell per column header for assistive tech.
   */
  offLine: 'max-sm:sr-only',
} as const

export function ShowSaveHeaderCell() {
  return (
    <th role="columnheader" className={SAVE_COLUMN_WIDTH}>
      <span className="sr-only">Save</span>
    </th>
  )
}

export function ShowSaveCell({
  showId,
  column,
  className,
}: {
  showId: number
  column: ShowSaveColumnData
  /** The cell's place in the stacked row. */
  className: string
}) {
  return (
    <td role="cell" className={cn(SAVE_COLUMN_WIDTH, className)}>
      <div className="flex justify-end">
        <SaveButton
          showId={showId}
          showLabel
          // The count is announced in the accessible name, not painted.
          showCount={false}
          saveData={batchedSaveFor(column.counts, showId)}
        />
      </div>
    </td>
  )
}
