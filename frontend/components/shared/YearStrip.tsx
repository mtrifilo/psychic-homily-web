'use client'

import { useId, useState } from 'react'
import Link from 'next/link'
import { cn } from '@/lib/utils'

export interface YearStripEntry {
  /** Calendar year. Also the React key, so it must be unique in `years`. */
  year: number
  /** Rows in that year. Entries at 0 are dropped: an empty year is a dead end. */
  count: number
  /** Fully built href, anchors included. The consumer owns URL shape. */
  href: string
}

export interface YearStripProps {
  /**
   * Years in display order (newest first, by convention). Rendered as given —
   * this component never sorts, so the consumer's ordering is the contract.
   */
  years: YearStripEntry[]
  /** Selected year, or `null`/omitted when the unfiltered "all years" view is active. */
  currentYear?: number | null
  /** Href for the leading "all years" link. */
  allYearsHref: string
  /** Accessible name for the `<nav>` landmark. Unique per page. */
  ariaLabel: string
  /** Label for the leading link. Defaults to "All years". */
  allYearsLabel?: string
  /**
   * Hide years past this many behind an "older" disclosure. Off by default; set
   * it on surfaces with a deep archive, where the strip would otherwise be a
   * long horizontal scroll. Hidden years stay in the DOM as real links so
   * crawlers still reach them.
   */
  collapseAfter?: number
  /** Fired with the target year (`null` for "all years") alongside navigation. */
  onNavigate?: (year: number | null) => void
  /** Extra classes on the `<nav>`. */
  className?: string
}

const linkClass =
  'rounded-sm text-foreground transition-colors hover:text-primary focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring'

const currentClass = 'font-bold text-primary underline underline-offset-4'

/**
 * Locale is pinned so the server and client render identical digit grouping;
 * see the same note in Pagination.tsx.
 */
const formatCount = (value: number) => value.toLocaleString('en-US')

/**
 * Year filter strip for archive lists: `All years · 2026 (34) · 2025 (161) …`.
 * Composes above a {@link Pagination} row rather than being part of it — the
 * two navigate different axes and consumers mix and match.
 *
 * Every year is a real `<a href>`. Counts are hidden below the `sm` breakpoint
 * (the strip scrolls horizontally there and the numbers are the first thing to
 * lose), and the strip renders `null` when no year has any rows.
 *
 * Usage:
 *   <YearStrip
 *     ariaLabel="Filter shows by year"
 *     allYearsHref="/venues/the-rebel-lounge#past-shows"
 *     currentYear={2025}
 *     years={[{ year: 2026, count: 34, href: '?year=2026#past-shows' }]}
 *   />
 */
export function YearStrip({
  years,
  currentYear = null,
  allYearsHref,
  ariaLabel,
  allYearsLabel = 'All years',
  collapseAfter,
  onNavigate,
  className,
}: YearStripProps) {
  // A year with nothing in it navigates to an empty list, so it never renders.
  const visibleYears = years.filter((entry) => entry.count > 0)

  const collapseAt =
    collapseAfter !== undefined && collapseAfter > 0 ? collapseAfter : null
  const isCollapsible =
    collapseAt !== null && visibleYears.length > collapseAt + 1
  const headCount = isCollapsible ? collapseAt : visibleYears.length

  // Multiple strips can share a page (top and bottom of a list), so the
  // disclosure's aria-controls target has to be per-instance.
  const yearListId = useId()

  // Land expanded when the selected year is in the hidden tail, so the reader
  // can always see where they are without opening the disclosure first.
  const currentYearIsInTail =
    isCollapsible &&
    visibleYears.slice(headCount).some((entry) => entry.year === currentYear)

  const [expanded, setExpanded] = useState(currentYearIsInTail)

  // These are <Link>s, so a click is usually a soft navigation that re-renders
  // this component rather than remounting it. Without this, the mount-time
  // initializer above is the only thing that ever opens the disclosure, and
  // selecting a tail year would leave the reader on a strip where nothing is
  // marked current. Re-checked only when the selected year actually changes,
  // so a reader's manual collapse is not fought on every render.
  const [trackedYear, setTrackedYear] = useState(currentYear)
  if (trackedYear !== currentYear) {
    setTrackedYear(currentYear)
    if (currentYearIsInTail) setExpanded(true)
  }

  if (visibleYears.length === 0) return null

  return (
    <nav
      aria-label={ariaLabel}
      className={cn(
        'flex items-baseline gap-x-2 overflow-x-auto font-mono text-xs sm:flex-wrap sm:overflow-x-visible',
        className
      )}
      data-testid="year-strip"
    >
      <ul
        id={yearListId}
        className="flex items-baseline gap-x-2 whitespace-nowrap sm:flex-wrap sm:gap-y-1"
      >
        <li>
          <Link
            href={allYearsHref}
            aria-current={currentYear === null ? 'page' : undefined}
            onClick={() => onNavigate?.(null)}
            className={cn(linkClass, currentYear === null && currentClass)}
          >
            {allYearsLabel}
          </Link>
        </li>
        {visibleYears.map((entry, index) => (
          <li
            key={entry.year}
            // Collapsed years keep their href in the DOM for crawlers while
            // `hidden` takes them out of the accessibility tree and tab order.
            hidden={!expanded && index >= headCount}
            data-collapsed={
              !expanded && index >= headCount ? 'true' : undefined
            }
          >
            <span aria-hidden="true" className="mr-2 text-muted-foreground">
              ·
            </span>
            <Link
              href={entry.href}
              aria-current={entry.year === currentYear ? 'page' : undefined}
              onClick={() => onNavigate?.(entry.year)}
              // Named explicitly so the count survives the breakpoint that
              // hides it, and so name computation cannot drop the space
              // between the year and its count (engines disagree about
              // separators between sibling text elements). The visible text
              // stays a subset of the name, so voice control still works.
              aria-label={`${entry.year} (${formatCount(entry.count)})`}
              className={cn(
                linkClass,
                entry.year === currentYear && currentClass
              )}
            >
              {entry.year}
              {/* Counts are the first casualty of a narrow viewport. */}
              <span className="hidden sm:inline">
                {` (${formatCount(entry.count)})`}
              </span>
            </Link>
          </li>
        ))}
      </ul>
      {isCollapsible ? (
        <button
          type="button"
          aria-expanded={expanded}
          aria-controls={yearListId}
          onClick={() => setExpanded((open) => !open)}
          className={cn(linkClass, 'whitespace-nowrap text-muted-foreground')}
        >
          {expanded ? 'fewer' : 'older'}
          <span aria-hidden="true">{expanded ? ' ▴' : ' ▾'}</span>
        </button>
      ) : null}
    </nav>
  )
}
