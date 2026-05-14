'use client'

import type { TableHTMLAttributes, ReactNode } from 'react'
import { cn } from '@/lib/utils'

export type DenseTableVariant = 'standard' | 'alternating' | 'bare'

const VARIANT_CLASSES: Record<DenseTableVariant, string> = {
  standard:
    '[&_thead_th]:border-b [&_thead_th]:border-border [&_tbody_tr]:border-b [&_tbody_tr]:border-border/40 [&_tbody_tr:last-child]:border-b-0',
  alternating:
    '[&_thead_th]:border-b [&_thead_th]:border-border [&_tbody_tr:nth-child(odd)]:bg-muted/20 [&_tbody_tr]:border-b [&_tbody_tr]:border-border/30 [&_tbody_tr:last-child]:border-b-0',
  bare: '[&_tbody_tr]:border-b [&_tbody_tr]:border-border/20 [&_tbody_tr:last-child]:border-b-0',
}

export interface DenseTableProps extends TableHTMLAttributes<HTMLTableElement> {
  /**
   * Visual variant:
   * - `standard` (default) — header chrome + per-row bottom border.
   * - `alternating` — header chrome + Gazelle-style striped rows
   *   (`bg-muted/20` on odd rows).
   * - `bare` — minimal chrome; row dividers only, no header background.
   *   Use when the surrounding `<SectionHeader>` already establishes context.
   */
  variant?: DenseTableVariant
  /** Table children: `<thead>` / `<tbody>` / `<tr>` / `<td>` / `<th>`. */
  children?: ReactNode
}

/**
 * Density-first table primitive for entity-page lists (shows, discography,
 * similar artists, etc.). Tight typography (13–14px body, ~1.25 line-height),
 * `tabular-nums` baked in for numeric alignment, minimal padding (`py-1.5
 * px-2`). Built on native `<table>` — no shadcn Table wrapper in this project.
 *
 * Pair with `<SectionHeader>` for the section title and `<DenseTableGroupHeader>`
 * for grouped tables (e.g. Discography by release type).
 *
 * Usage:
 *   <DenseTable variant="alternating">
 *     <thead>
 *       <tr>
 *         <th>Title</th>
 *         <th className="text-right">Year</th>
 *       </tr>
 *     </thead>
 *     <tbody>
 *       <DenseTableGroupHeader title="Albums & EPs" colSpan={2} />
 *       <tr>
 *         <td>Heart Under</td>
 *         <td className="text-right">2022</td>
 *       </tr>
 *     </tbody>
 *   </DenseTable>
 */
export function DenseTable({
  variant = 'standard',
  className,
  children,
  ...rest
}: DenseTableProps) {
  return (
    <table
      className={cn(
        'w-full text-sm leading-tight tabular-nums',
        '[&_td]:py-1.5 [&_td]:px-2 [&_td]:align-baseline',
        '[&_th]:py-1.5 [&_th]:px-2 [&_th]:align-baseline [&_th]:text-left',
        '[&_thead_th]:text-xs [&_thead_th]:font-semibold [&_thead_th]:uppercase [&_thead_th]:tracking-wider [&_thead_th]:text-muted-foreground',
        VARIANT_CLASSES[variant],
        className
      )}
      {...rest}
    >
      {children}
    </table>
  )
}

export interface DenseTableGroupHeaderProps {
  /** Group title (rendered uppercase tracking-wider). */
  title: string
  /** Number of columns the header should span. Must match the table's column count. */
  colSpan: number
  /** Additional CSS classes on the wrapping `<tr>`. */
  className?: string
}

/**
 * Group-header row for use inside a `<DenseTable>`'s `<tbody>`. Renders a
 * single full-width `<th scope="rowgroup">` to group rows by category
 * (Albums & EPs / Singles / etc.) without breaking `<table>` semantics.
 */
export function DenseTableGroupHeader({
  title,
  colSpan,
  className,
}: DenseTableGroupHeaderProps) {
  return (
    <tr className={cn('bg-muted/40', className)}>
      <th
        scope="rowgroup"
        colSpan={colSpan}
        className="text-xs font-semibold uppercase tracking-wider text-muted-foreground"
      >
        {title}
      </th>
    </tr>
  )
}
