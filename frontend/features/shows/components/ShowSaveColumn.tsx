'use client'

import { useMemo, type AriaRole } from 'react'
import { cn } from '@/lib/utils'
import { useAuthContext } from '@/lib/context/AuthContext'
import { SaveButton } from '@/components/shared/SaveButton'
import type {
  BatchedSaveData,
  SaveCounts,
} from '@/components/shared/batchedSaveData'
import { useShowSaveCountBatch } from '../hooks/useSavedShows'

/**
 * The trailing Save column of an entity page's upcoming-show table.
 *
 * `counts` is the batch result map, `undefined` while the batch is pending or
 * disabled; every cell then renders its empty state without a request of its
 * own (see `batchedSaveFor`). A table renders the column exactly when it is
 * handed one of these, so the column cannot exist without the batch behind it.
 * A phone row has no room for it as a column, so a table with the column
 * stacks its rows below `sm` (`stackedTableLayout`).
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
 * The column's fixed width from `sm` up: the cell's 16px of padding plus the
 * widest state the control can take, "Saved" (87px measured: `px-3`, the 16px
 * heart, `gap-1.5` and the word), plus 3px of slack. Every row and the header
 * share it, so a saved state that arrives after hydration widens the control
 * inside its cell and moves no other column.
 */
const SAVE_COLUMN_WIDTH = 'w-[106px]'

/**
 * Below `sm` the stacked row zeroes cell padding, so the cell reserves the
 * control's width alone (87px plus the same slack), for the same reason: the
 * price and time beside it on the first line do not move.
 */
const STACKED_SAVE_CELL_WIDTH = 'max-sm:w-[90px]'

export function ShowSaveHeaderCell({ role }: { role?: AriaRole }) {
  return (
    <th role={role} className={SAVE_COLUMN_WIDTH}>
      <span className="sr-only">Save</span>
    </th>
  )
}

export function ShowSaveCell({
  showId,
  saveData,
  role,
  className,
}: {
  showId: number
  /** This row's entry, from `batchedSaveFor`. */
  saveData: BatchedSaveData | undefined
  role?: AriaRole
  /** The cell's place in the stacked row. */
  className: string
}) {
  return (
    <td
      role={role}
      className={cn(SAVE_COLUMN_WIDTH, STACKED_SAVE_CELL_WIDTH, className)}
    >
      <div className="flex justify-end">
        <SaveButton
          showId={showId}
          showLabel
          // The count is announced in the accessible name, not painted.
          showCount={false}
          saveData={saveData}
        />
      </div>
    </td>
  )
}
