'use client'

import * as React from 'react'
import { cn } from '@/lib/utils'

/**
 * AdminTable — the canonical admin entity-list table (PSY-910). Unifies the
 * top-level admin entity-list tables (Collections, Radio Stations, Pipeline
 * Import History) behind one dense, hairline-divided table: a muted
 * column-header band with mono-uppercase labels (the editorial upgrade locked in
 * PSY-912), dense rows, and per-column alignment + render. Nested / detail
 * tables (collection items, venue config, extraction-run history, unmatched
 * plays) intentionally stay hand-rolled — they aren't single-click-to-detail
 * lists and don't fit this config-driven shape.
 *
 * Render-only by design. Sorting isn't included (no admin table sorts today);
 * pagination stays with the parent (e.g. Pipeline Import History owns its
 * Prev/Next + offset) — AdminTable just renders the rows it's given. The column
 * API leaves room to add a `sortable` hook later without a breaking change.
 *
 * Loading / error states stay in the parent (it early-returns before rendering
 * the table); pass `empty` for the no-rows message.
 *
 * Distinct from `components/shared/DenseTable`: that is a *composition* primitive
 * (you pass `<thead>/<tbody>` children, variants) for public entity pages;
 * AdminTable is *config-driven* (a columns array) for admin entity lists. Reach
 * for DenseTable on entity pages, AdminTable for admin lists. Their header
 * treatments intentionally differ — AdminTable uses a mono-uppercase band per
 * PSY-912; DenseTable does not. Keep them divergent unless a cohesion pass
 * deliberately reconciles both.
 */
export interface AdminTableColumn<T> {
  /** Stable column id (React key for the cell). */
  key: string
  header: React.ReactNode
  /** Cell content for a row. Closures here capture the parent's state/mutations
   *  (e.g. a Featured toggle), so AdminTable stays presentation-only. */
  render: (row: T) => React.ReactNode
  align?: 'left' | 'center' | 'right'
  /** Stop the row's onClick when interacting with this cell (e.g. a toggle in a
   *  click-to-detail row). Covers the MOUSE click only — keyboard isolation for a
   *  focused in-cell control relies on the row's `e.target === e.currentTarget`
   *  keydown guard (don't remove that guard or child controls double-fire). */
  stopRowClick?: boolean
  headerClassName?: string
  cellClassName?: string
}

export interface AdminTableProps<T> {
  columns: AdminTableColumn<T>[]
  rows: T[]
  /** Stable React key per row. */
  rowKey: (row: T) => React.Key
  /** Click-to-detail affordance. When set, rows become keyboard-operable
   *  (focusable, Enter/Space) and get hover + cursor styling + role="button". */
  onRowClick?: (row: T) => void
  /** Accessible label for a clickable row (announced by screen readers, since a
   *  role="button" row otherwise reads only its cells). Strongly recommended
   *  whenever onRowClick is set — e.g. the entity name. */
  rowLabel?: (row: T) => string
  /** Extra per-row classes — e.g. a selected-row highlight. */
  rowClassName?: (row: T) => string | undefined
  /** Rendered (spanning all columns) when `rows` is empty. Omit to render an
   *  empty body — most parents early-return their own empty state instead. */
  empty?: React.ReactNode
  /** Override the container (e.g. width). */
  className?: string
}

const ALIGN: Record<NonNullable<AdminTableColumn<unknown>['align']>, string> = {
  left: 'text-left',
  center: 'text-center',
  right: 'text-right',
}

export function AdminTable<T>({
  columns,
  rows,
  rowKey,
  onRowClick,
  rowLabel,
  rowClassName,
  empty,
  className,
}: AdminTableProps<T>) {
  const clickable = !!onRowClick

  return (
    <div className={cn('overflow-hidden rounded-lg border border-border', className)}>
      <table className="w-full text-sm">
        <thead className="bg-muted/50">
          <tr className="border-b border-border">
            {columns.map(col => (
              <th
                key={col.key}
                className={cn(
                  'px-3 py-2 font-mono text-xs font-medium uppercase tracking-wider text-muted-foreground',
                  ALIGN[col.align ?? 'left'],
                  col.headerClassName
                )}
              >
                {col.header}
              </th>
            ))}
          </tr>
        </thead>
        <tbody className="divide-y divide-border">
          {rows.length === 0 && empty != null ? (
            <tr>
              <td
                colSpan={columns.length}
                className="px-3 py-8 text-center text-muted-foreground"
              >
                {empty}
              </td>
            </tr>
          ) : (
            rows.map(row => (
              <tr
                key={rowKey(row)}
                role={clickable ? 'button' : undefined}
                aria-label={clickable ? rowLabel?.(row) : undefined}
                onClick={clickable ? () => onRowClick!(row) : undefined}
                onKeyDown={
                  clickable
                    ? e => {
                        // Activate only from the row itself, so focused child
                        // controls (e.g. a Featured toggle) keep their own keys.
                        if (
                          e.target === e.currentTarget &&
                          (e.key === 'Enter' || e.key === ' ')
                        ) {
                          e.preventDefault()
                          onRowClick!(row)
                        }
                      }
                    : undefined
                }
                tabIndex={clickable ? 0 : undefined}
                className={cn(
                  clickable &&
                    'cursor-pointer transition-colors hover:bg-muted/30 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-ring',
                  rowClassName?.(row)
                )}
              >
                {columns.map(col => (
                  <td
                    key={col.key}
                    onClick={col.stopRowClick ? e => e.stopPropagation() : undefined}
                    className={cn('px-3 py-2', ALIGN[col.align ?? 'left'], col.cellClassName)}
                  >
                    {col.render(row)}
                  </td>
                ))}
              </tr>
            ))
          )}
        </tbody>
      </table>
    </div>
  )
}
