import type { AriaRole } from 'react'

type ElementProps = { role?: AriaRole; className?: string }
/** Cells and headers take only a role: each table owns their classes. */
type CellProps = { role?: AriaRole }

/** Spread onto each element of a `DenseTable` that stacks below `sm`. */
export interface StackedTableLayout {
  table: ElementProps
  head: ElementProps
  headRow: CellProps
  body: ElementProps
  row: ElementProps
  columnHeader: CellProps
  cell: CellProps
}

/**
 * The element props for a `DenseTable` whose rows stack into lines below
 * `sm`, or, with no `stackedColumns`, for one that stays a table at every width.
 *
 * Stacked, each row is a grid on `stackedColumns` (a `max-sm:grid-cols-*`
 * class) and each table places its own cells on it, and the header row is
 * visually hidden. ONE element tree across the breakpoint, reflowed by CSS
 * rather than swapped, so no row's text is in the document twice. Overriding
 * the display of the table's elements strips their implicit table semantics,
 * so a stacked table carries explicit ARIA table roles, and its header row
 * stays for assistive tech.
 *
 * The row's `[&>td]:p-0` cancels DenseTable's `[&_td]` cell padding (same
 * specificity, and the breakpoint variant comes later in the stylesheet), so
 * the row's own padding is the stacked row's only padding.
 *
 * A table that does not stack gets empty sets, so its markup carries nothing
 * extra.
 */
export function stackedTableLayout(stackedColumns?: string): StackedTableLayout {
  if (!stackedColumns) {
    return {
      table: {},
      head: {},
      headRow: {},
      body: {},
      row: {},
      columnHeader: {},
      cell: {},
    }
  }
  return {
    table: { role: 'table', className: 'max-sm:block' },
    head: { role: 'rowgroup', className: 'max-sm:sr-only' },
    headRow: { role: 'row' },
    body: { role: 'rowgroup', className: 'max-sm:block' },
    row: {
      role: 'row',
      className: `max-sm:grid ${stackedColumns} max-sm:items-baseline max-sm:gap-x-2 max-sm:gap-y-1 max-sm:px-2 max-sm:py-2 max-sm:[&>td]:p-0`,
    },
    columnHeader: { role: 'columnheader' },
    cell: { role: 'cell' },
  }
}

/**
 * Drops a cell's line from a stacked row while keeping the cell, so every row
 * still has one cell per column header for assistive tech.
 */
export const STACKED_OFF_LINE = 'max-sm:sr-only'
