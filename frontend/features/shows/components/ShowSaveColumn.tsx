'use client'

import { useMemo } from 'react'
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
 * The trailing Save column of an entity page's upcoming-show table.
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

export function ShowSaveHeaderCell() {
  return (
    <th className={SAVE_COLUMN_WIDTH}>
      <span className="sr-only">Save</span>
    </th>
  )
}

export function ShowSaveCell({
  showId,
  column,
}: {
  showId: number
  column: ShowSaveColumnData
}) {
  return (
    <td className={SAVE_COLUMN_WIDTH}>
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
