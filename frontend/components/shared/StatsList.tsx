'use client'

import type { ReactNode } from 'react'
import { cn } from '@/lib/utils'

export interface StatsListItem {
  /** Stat label, shown verbatim (e.g. "Releases"). */
  label: string
  /**
   * Stat value. Numbers are formatted with thousands separators; strings and
   * nodes render as-is so callers can pass formatted values, links, or icons.
   */
  value: number | string | ReactNode
}

export type StatsListVariant = 'sidebar' | 'inline'

export interface StatsListProps {
  /** Stat rows in display order. Empty arrays render nothing. */
  items: StatsListItem[]
  /**
   * Layout variant:
   * - `sidebar` (default) — vertical `<dl>` stacked rows ("Releases  4").
   *   Tight typography for the artist-page sidebar.
   * - `inline` — single horizontal middot-separated strip ("4 releases · 2 labels · …").
   *   For stat headers above main-column content. NOTE: the inline variant
   *   lowercases labels for natural prose reading — pass plain-noun labels
   *   ("Releases", "Shows tracked"), not proper nouns or already-styled text.
   */
  variant?: StatsListVariant
  /** Additional CSS classes on the wrapping element. */
  className?: string
}

/** Shared en-US thousands-separator formatter for stat surfaces. */
export const statNumberFormatter = new Intl.NumberFormat('en-US')

function formatValue(value: StatsListItem['value']): ReactNode {
  if (typeof value === 'number') {
    return statNumberFormatter.format(value)
  }
  return value
}

/**
 * Density-first stat block for entity pages (PSY-639). Pairs with
 * `<SectionHeader title="Statistics" />` in the artist sidebar.
 *
 * Sidebar variant: vertical `<dl>` with one row per stat — label on the
 * left, value right-aligned with `tabular-nums`.
 *
 * Inline variant: a single row of middot-separated `<value> <label>` pairs
 * for placement above main-column content ("4 releases · 2 labels · 13 shows").
 */
export function StatsList({
  items,
  variant = 'sidebar',
  className,
}: StatsListProps) {
  if (items.length === 0) return null

  if (variant === 'inline') {
    return (
      <p
        className={cn(
          'text-sm text-muted-foreground tabular-nums',
          className
        )}
      >
        {items.map((item, idx) => (
          <span key={item.label}>
            {idx > 0 && (
              <span aria-hidden="true" className="mx-1.5">
                ·
              </span>
            )}
            <span className="text-foreground font-medium">{formatValue(item.value)}</span>{' '}
            <span>{item.label.toLowerCase()}</span>
          </span>
        ))}
      </p>
    )
  }

  return (
    <dl className={cn('text-sm space-y-0.5', className)}>
      {items.map(item => (
        <div
          key={item.label}
          className="flex items-baseline justify-between gap-2"
        >
          <dt className="text-muted-foreground">{item.label}</dt>
          <dd className="font-medium tabular-nums">{formatValue(item.value)}</dd>
        </div>
      ))}
    </dl>
  )
}
