'use client'

import { useId, useState } from 'react'
import Link from 'next/link'
import { cn } from '@/lib/utils'
import { formatCalendarMonthParts } from '@/lib/utils/formatters'
import {
  formatCount,
  isPlainNavigationClick,
  navCurrentClass,
  navLinkClass,
  navStripClass,
  navStripListClass,
  navStripSeparatorClass,
} from './paginationChrome'

/** One month bar of a histogram: the calendar month and how many rows are in it. */
export interface MonthStripEntry {
  /** Calendar year. */
  year: number
  /** Calendar month, 1-12. */
  month: number
  /** Rows in that month. Entries at 0 are dropped: an empty month is a dead end. */
  count: number
}

/** A month the strip can point at. */
export interface MonthStripTarget {
  year: number
  month: number
}

export interface MonthStripProps {
  /**
   * Months in display order (soonest first, by convention). Rendered as given —
   * this component never sorts, so the consumer's ordering is the contract.
   */
  months: MonthStripEntry[]
  /** Href for a month. The consumer owns URL shape, anchors included. */
  hrefFor: (year: number, month: number) => string
  /** Href for the leading unscoped link. */
  allHref: string
  /** Accessible name for the `<nav>` landmark. Unique per page. */
  ariaLabel: string
  /** The month in view, or `null`/omitted when the unscoped view is active. */
  current?: MonthStripTarget | null
  /**
   * Label for the leading link ("All upcoming" on the shows list). Required
   * rather than defaulted: the copy belongs to the surface, not to this
   * component, and a default here would put one list's wording in a shared one.
   */
  allLabel: string
  /** Rows across every month, shown beside the leading link when given. */
  allCount?: number
  /** Fired with the target month (`null` for the leading link) alongside navigation. */
  onNavigate?: (target: MonthStripTarget | null) => void
  /** Extra classes on the `<nav>`. */
  className?: string
}

/** `2026-09`, the identity of a month wherever one has to be compared. */
function monthKey(target: MonthStripTarget): string {
  return `${target.year}-${String(target.month).padStart(2, '0')}`
}

/** A well-formed bar carrying at least one row. */
function isUsableEntry(entry: MonthStripEntry): boolean {
  return (
    Number.isInteger(entry.year) &&
    Number.isInteger(entry.month) &&
    entry.month >= 1 &&
    entry.month <= 12 &&
    Number.isInteger(entry.count) &&
    entry.count > 0
  )
}

function MonthLink({
  entry,
  href,
  isCurrent,
  onNavigate,
}: {
  entry: MonthStripEntry
  href: string
  isCurrent: boolean
  onNavigate?: (target: MonthStripTarget | null) => void
}) {
  const label = formatCalendarMonthParts(entry.year, entry.month).month
  return (
    <Link
      href={href}
      aria-current={isCurrent ? 'page' : undefined}
      onClick={event => {
        if (isPlainNavigationClick(event)) {
          onNavigate?.({ year: entry.year, month: entry.month })
        }
      }}
      // Named explicitly so the count survives the breakpoint that hides it,
      // and so name computation cannot drop the space between the month and its
      // count. The visible text stays a subset of the name, so voice control
      // still works.
      aria-label={`${label} (${formatCount(entry.count)})`}
      className={cn(navLinkClass, isCurrent && navCurrentClass)}
    >
      {label}
      {/* Counts are the first casualty of a narrow viewport. */}
      <span className="hidden sm:inline">{` (${formatCount(entry.count)})`}</span>
    </Link>
  )
}

/** The `·` that separates every item after the first. */
function Separator() {
  return (
    <span aria-hidden="true" className={navStripSeparatorClass}>
      ·
    </span>
  )
}

/**
 * Month filter strip for a forward-looking list:
 * `All upcoming (268) · Sep (64) · Oct (112) · Nov (68) · Dec (13) · 2027 ▸ (11)`.
 *
 * Composes above a `Pagination` row rather than being part of it — the two
 * navigate different axes. The month sibling of `YearStrip`, and it borrows
 * that component's collapse mechanics: months past the leading year stay in the
 * DOM as real links so crawlers reach them, hidden behind their year's
 * disclosure until a reader opens it.
 *
 * Every month is a real `<a href>`. The strip is bounded to the months it is
 * handed, so a consumer passing a histogram gets exactly the months that have
 * rows, and it renders `null` when none do.
 *
 * Usage:
 *   <MonthStrip
 *     ariaLabel="Filter shows by month"
 *     allHref="/shows"
 *     allLabel="All upcoming"
 *     allCount={268}
 *     current={{ year: 2026, month: 11 }}
 *     months={[{ year: 2026, month: 9, count: 64 }]}
 *     hrefFor={(year, month) => `/shows/${year}/${String(month).padStart(2, '0')}`}
 *   />
 */
export function MonthStrip({
  months,
  hrefFor,
  allHref,
  ariaLabel,
  current = null,
  allLabel,
  allCount,
  onNavigate,
  className,
}: MonthStripProps) {
  const visible = months.filter(isUsableEntry)

  // The strip leads with one year's months; anything in a later year folds
  // behind that year's own token. With one boundary — the shape the data has —
  // that is a single `2027 ▸ (11)` at the end of the row.
  const leadingYear = visible.length > 0 ? visible[0].year : null
  const head = visible.filter(entry => entry.year === leadingYear)
  const tail = visible.filter(entry => entry.year !== leadingYear)

  const tailYears: number[] = []
  for (const entry of tail) {
    if (!tailYears.includes(entry.year)) tailYears.push(entry.year)
  }

  const currentKey = current ? monthKey(current) : null
  const currentTailYear =
    current && tailYears.includes(current.year) ? current.year : null

  // Land expanded when the month in view is inside a folded year, so a reader
  // can always see where they are without opening the disclosure first.
  const [expandedYears, setExpandedYears] = useState<ReadonlySet<number>>(() =>
    currentTailYear === null ? new Set<number>() : new Set([currentTailYear])
  )

  // These are <Link>s, so a click is usually a soft navigation that re-renders
  // this component rather than remounting it. Re-checked only when the month in
  // view actually changes, so a reader's manual collapse is not fought on every
  // render.
  const [trackedKey, setTrackedKey] = useState(currentKey)
  if (trackedKey !== currentKey) {
    setTrackedKey(currentKey)
    if (currentTailYear !== null && !expandedYears.has(currentTailYear)) {
      const next = new Set(expandedYears)
      next.add(currentTailYear)
      setExpandedYears(next)
    }
  }

  const listId = useId()

  if (visible.length === 0) return null

  const toggleYear = (year: number) => {
    setExpandedYears(previous => {
      const next = new Set(previous)
      if (next.has(year)) next.delete(year)
      else next.add(year)
      return next
    })
  }

  return (
    <nav
      aria-label={ariaLabel}
      className={cn(navStripClass, className)}
      data-testid="month-strip"
    >
      <ul
        id={listId}
        className={navStripListClass}
      >
        <li>
          <Link
            href={allHref}
            aria-current={current === null ? 'page' : undefined}
            onClick={event => {
              if (isPlainNavigationClick(event)) onNavigate?.(null)
            }}
            className={cn(navLinkClass, current === null && navCurrentClass)}
          >
            {allLabel}
            {allCount !== undefined ? ` (${formatCount(allCount)})` : null}
          </Link>
        </li>
        {head.map(entry => (
          <li key={monthKey(entry)}>
            <Separator />
            <MonthLink
              entry={entry}
              href={hrefFor(entry.year, entry.month)}
              isCurrent={monthKey(entry) === currentKey}
              onNavigate={onNavigate}
            />
          </li>
        ))}
        {tailYears.flatMap(year => {
          const entries = tail.filter(entry => entry.year === year)
          const expanded = expandedYears.has(year)
          const yearCount = entries.reduce((sum, entry) => sum + entry.count, 0)
          const controlledIds = entries
            .map(entry => `${listId}-${year}-${entry.month}`)
            .join(' ')
          return [
            <li key={`year-${year}`}>
              <Separator />
              <button
                type="button"
                aria-expanded={expanded}
                aria-controls={controlledIds}
                onClick={() => toggleYear(year)}
                className={cn(navLinkClass, 'whitespace-nowrap')}
                data-testid={`month-strip-year-${year}`}
              >
                {year}
                <span aria-hidden="true">{expanded ? ' ▾' : ' ▸'}</span>
                {` (${formatCount(yearCount)})`}
              </button>
            </li>,
            ...entries.map(entry => (
              // Folded months keep their href in the DOM for crawlers while
              // `hidden` takes them out of the accessibility tree and the tab
              // order.
              <li
                key={monthKey(entry)}
                id={`${listId}-${year}-${entry.month}`}
                hidden={!expanded}
              >
                <Separator />
                <MonthLink
                  entry={entry}
                  href={hrefFor(entry.year, entry.month)}
                  isCurrent={monthKey(entry) === currentKey}
                  onNavigate={onNavigate}
                />
              </li>
            )),
          ]
        })}
      </ul>
    </nav>
  )
}
