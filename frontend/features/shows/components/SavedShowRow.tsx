'use client'

import { formatRelativeTime } from '@/lib/formatRelativeTime'
import type { SavedShowResponse } from '../types'
import { SHOW_ROW_GRID, ShowRow, ShowRowSaveButton } from './ShowRow'

/**
 * The grid the signed-in home's saved-shows prompt row shares with the rows
 * it stands in for. The same string as {@link SHOW_ROW_GRID}, not a copy.
 */
export const SAVED_SHOW_ROW_GRID = SHOW_ROW_GRID

/**
 * Library's saved-show row: the shared {@link ShowRow} with the save age in
 * its trailing column.
 *
 * An upcoming row's unsave is the heart above the age, the same gesture that
 * made the save. Its saved state is asserted rather than read back, because
 * these rows ARE the viewer's saved shows, and it patches the row out of
 * Library's loaded pages instead of refetching them. A past row is a record:
 * the age alone, no heart.
 */
export function SavedShowRow({
  show,
  isPast,
  disabled,
}: {
  show: SavedShowResponse
  isPast: boolean
  /** Holds the heart while another change to these lists is in flight. */
  disabled?: boolean
}) {
  const savedAge = (
    // `relative`: SaveButton's root is positioned, and only a positioned
    // later sibling paints above it where the two overlap.
    <span className="relative whitespace-nowrap font-data text-[11px] leading-[14px] text-muted-foreground">
      saved {formatRelativeTime(show.saved_at, { short: true })}
    </span>
  )

  return (
    <ShowRow
      show={show}
      past={isPast}
      trailing={
        isPast ? (
          savedAge
        ) : (
          <>
            {/* The glyph is centred in its 44px target; pulling the age up
                into the target's empty lower edge keeps the row dense, and the
                age, painted above it, takes the taps that land on it. */}
            <div className="-mb-2.5 flex">
              <ShowRowSaveButton
                showId={show.id}
                saveData={{ save_count: 0, is_saved: true }}
                unsaveSync="patch-infinite"
                disabled={disabled}
              />
            </div>
            {savedAge}
          </>
        )
      }
    />
  )
}
