'use client'

import Link from 'next/link'
import { cn } from '@/lib/utils'
import {
  adjacentPeriodNav,
  formatArchiveSubtitle,
  formatArchiveTitle,
  type ChartCalendarWindow,
} from '../calendarWindows'

const linkClass =
  'hover:text-primary focus-visible:text-primary focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring'

interface ArchiveMastheadProps {
  window: ChartCalendarWindow
  now?: Date
}

/**
 * Approved Figma archive masthead (node 996:28): period title, mono ARCHIVE
 * marker, adjacent-period nav. No live window tabs / ticker.
 */
export function ArchiveMasthead({ window, now }: ArchiveMastheadProps) {
  const nav = adjacentPeriodNav(window, now)
  if (!nav) return null

  return (
    <header
      className="flex flex-col gap-4 md:flex-row md:items-end md:justify-between"
      data-testid="chart-archive-masthead"
    >
      <div className="min-w-0">
        <h1 className="font-display text-3xl font-bold leading-none">
          {formatArchiveTitle(window)}
        </h1>
        <p className="mt-1.5 text-[13px] text-muted-foreground">
          {formatArchiveSubtitle(window, now)}
        </p>
      </div>
      <div className="flex flex-col items-start gap-2 md:items-end">
        <span className="font-mono text-[10px] font-bold uppercase tracking-[0.08em] text-primary">
          ARCHIVE
        </span>
        <nav
          aria-label="Adjacent chart periods"
          className="flex flex-wrap items-center gap-x-3 gap-y-1 font-mono text-xs"
        >
          {nav.prevYear ? (
            <Link href={nav.prevYear.href} className={linkClass}>
              ← {nav.prevYear.label}
            </Link>
          ) : (
            <span className="text-muted-foreground/50" aria-hidden>
              ← —
            </span>
          )}
          <span className="flex items-center gap-1.5" aria-label="Quarters">
            {nav.quarters.map((quarter, index) => (
              <span key={quarter.quarter} className="contents">
                {index > 0 ? (
                  <span className="text-muted-foreground" aria-hidden>
                    ·
                  </span>
                ) : null}
                {quarter.available ? (
                  <Link
                    href={quarter.href}
                    aria-current={quarter.current ? 'page' : undefined}
                    className={cn(
                      linkClass,
                      quarter.current && 'text-primary underline underline-offset-4'
                    )}
                  >
                    {quarter.label}
                  </Link>
                ) : (
                  <span className="text-muted-foreground/50">{quarter.label}</span>
                )}
              </span>
            ))}
          </span>
          {nav.nextYear ? (
            <Link href={nav.nextYear.href} className={linkClass}>
              {nav.nextYear.label} →
            </Link>
          ) : (
            <span className="text-muted-foreground/50" aria-hidden>
              — →
            </span>
          )}
        </nav>
        <p className="text-[11px] text-muted-foreground">
          {nav.viewingYear ? (
            'viewing the full year · pick a quarter to narrow'
          ) : (
            <>
              viewing {formatArchiveTitle(window).replace('Charts — ', '')} ·{' '}
              <Link href={nav.yearHref} className={linkClass}>
                full year
              </Link>
            </>
          )}
        </p>
      </div>
    </header>
  )
}
