/**
 * Calendar chart windows (`YYYY` / `YYYY-q1..q4`) — mirrors the backend
 * grammar in contracts.ParseChartWindow / CalendarBounds (PSY-1421).
 *
 * Archive permalinks (PSY-1422): `/charts/2026` → `window=2026`,
 * `/charts/2026/q2` → `window=2026-q2`. Numeric-year first segments are
 * reserved so they never collide with `[module]` slugs.
 */

/** Public lower bound — periods ending at or before this are pre-launch. */
export const CHART_CALENDAR_LAUNCH_UTC = Date.UTC(2026, 0, 1)

const CALENDAR_WINDOW_PATTERN = /^([0-9]{4})(?:-q([1-4]))?$/

export type ChartCalendarWindow = string & { readonly __brand?: 'ChartCalendarWindow' }

export interface CalendarBounds {
  start: Date
  /** Exclusive UTC end. */
  end: Date
  year: number
  quarter: 1 | 2 | 3 | 4 | null
}

export function calendarBounds(window: string): CalendarBounds | null {
  const matches = CALENDAR_WINDOW_PATTERN.exec(window)
  if (!matches) return null
  const year = Number(matches[1])
  if (!Number.isInteger(year) || year === 0) return null
  const quarter = matches[2]
    ? (Number(matches[2]) as 1 | 2 | 3 | 4)
    : null
  const startMonth = quarter ? (quarter - 1) * 3 : 0
  const months = quarter ? 3 : 12
  const start = new Date(Date.UTC(year, startMonth, 1))
  const end = new Date(Date.UTC(year, startMonth + months, 1))
  return { start, end, year, quarter }
}

/**
 * Validate a calendar window against launch + future gates (same rules as
 * the backend). Returns the window string when valid, otherwise null.
 */
export function parseCalendarChartWindow(
  value: string,
  now: Date = new Date()
): ChartCalendarWindow | null {
  const bounds = calendarBounds(value)
  if (!bounds) return null
  if (bounds.end.getTime() <= CHART_CALENDAR_LAUNCH_UTC) return null
  if (bounds.start.getTime() > now.getTime()) return null
  return value as ChartCalendarWindow
}

export function isCalendarChartWindow(value: string): boolean {
  return calendarBounds(value) !== null
}

/** Closed when the exclusive end is at or before `now` (immutable period). */
export function isCalendarWindowClosed(
  window: string,
  now: Date = new Date()
): boolean {
  const bounds = calendarBounds(window)
  if (!bounds) return false
  return bounds.end.getTime() <= now.getTime()
}

export function isChartArchiveYearSegment(segment: string): boolean {
  return /^[0-9]{4}$/.test(segment)
}

export function isChartArchiveQuarterSegment(segment: string): boolean {
  return /^q[1-4]$/.test(segment)
}

/** Build `YYYY` or `YYYY-qN` from route segments. */
export function calendarWindowFromRoute(
  yearSegment: string,
  quarterSegment?: string,
  now: Date = new Date()
): ChartCalendarWindow | null {
  if (!isChartArchiveYearSegment(yearSegment)) return null
  if (quarterSegment !== undefined) {
    if (!isChartArchiveQuarterSegment(quarterSegment)) return null
    return parseCalendarChartWindow(`${yearSegment}-${quarterSegment}`, now)
  }
  return parseCalendarChartWindow(yearSegment, now)
}

export function archiveHref(window: string): string {
  const bounds = calendarBounds(window)
  if (!bounds) return '/charts'
  if (bounds.quarter) return `/charts/${bounds.year}/q${bounds.quarter}`
  return `/charts/${bounds.year}`
}

export function formatArchiveTitle(window: string): string {
  const bounds = calendarBounds(window)
  if (!bounds) return 'Charts'
  if (bounds.quarter) return `Charts — Q${bounds.quarter} ${bounds.year}`
  return `Charts — ${bounds.year}`
}

function formatClosedDay(endExclusive: Date): string {
  const last = new Date(endExclusive.getTime() - 1)
  return last.toLocaleDateString('en-US', {
    month: 'long',
    day: 'numeric',
    year: 'numeric',
    timeZone: 'UTC',
  })
}

export function formatArchiveSubtitle(
  window: string,
  now: Date = new Date()
): string {
  const bounds = calendarBounds(window)
  if (!bounds) return ''
  const noun = bounds.quarter ? 'quarter' : 'year'
  if (isCalendarWindowClosed(window, now)) {
    return `The ${noun} in the ledger — closed ${formatClosedDay(bounds.end)}.`
  }
  const endLabel = formatClosedDay(bounds.end)
  return `The ${noun} in the ledger — open through ${endLabel}.`
}

export function formatWindowContext(window: string): string {
  const bounds = calendarBounds(window)
  if (!bounds) return window
  if (bounds.quarter) return `Q${bounds.quarter} ${bounds.year}`
  return String(bounds.year)
}

export function formatWindowSummary(window: string): string {
  const bounds = calendarBounds(window)
  if (!bounds) return window.toUpperCase()
  if (bounds.quarter) return `Q${bounds.quarter} ${bounds.year}`
  return String(bounds.year)
}

export interface AdjacentPeriodNav {
  prevYear: { href: string; label: string } | null
  nextYear: { href: string; label: string } | null
  quarters: Array<{
    quarter: 1 | 2 | 3 | 4
    href: string
    label: string
    current: boolean
    available: boolean
  }>
  yearHref: string
  viewingYear: boolean
}

export function adjacentPeriodNav(
  window: string,
  now: Date = new Date()
): AdjacentPeriodNav | null {
  const bounds = calendarBounds(window)
  if (!bounds) return null

  const prevYearValue = String(bounds.year - 1)
  const nextYearValue = String(bounds.year + 1)
  const prevParsed = parseCalendarChartWindow(prevYearValue, now)
  const nextParsed = parseCalendarChartWindow(nextYearValue, now)

  const quarters = ([1, 2, 3, 4] as const).map(quarter => {
    const value = `${bounds.year}-q${quarter}`
    const available = parseCalendarChartWindow(value, now) !== null
    return {
      quarter,
      href: `/charts/${bounds.year}/q${quarter}`,
      label: `Q${quarter}`,
      current: bounds.quarter === quarter,
      available,
    }
  })

  return {
    prevYear: prevParsed
      ? { href: `/charts/${prevYearValue}`, label: prevYearValue }
      : null,
    nextYear: nextParsed
      ? { href: `/charts/${nextYearValue}`, label: nextYearValue }
      : null,
    quarters,
    yearHref: `/charts/${bounds.year}`,
    viewingYear: bounds.quarter === null,
  }
}

/** Front-page entry links: current year + previous quarter (when linkable). */
export function frontPageArchiveLinks(now: Date = new Date()): {
  year: { href: string; label: string } | null
  previousQuarter: { href: string; label: string } | null
} {
  const year = now.getUTCFullYear()
  const currentQuarter = (Math.floor(now.getUTCMonth() / 3) + 1) as
    | 1
    | 2
    | 3
    | 4
  let prevQuarter: 1 | 2 | 3 | 4
  let prevQuarterYear = year
  if (currentQuarter === 1) {
    prevQuarter = 4
    prevQuarterYear = year - 1
  } else {
    prevQuarter = (currentQuarter - 1) as 1 | 2 | 3
  }

  const yearWindow = parseCalendarChartWindow(String(year), now)
  const prevQWindow = parseCalendarChartWindow(
    `${prevQuarterYear}-q${prevQuarter}`,
    now
  )

  return {
    year: yearWindow
      ? { href: `/charts/${year}`, label: String(year) }
      : null,
    previousQuarter: prevQWindow
      ? {
          href: `/charts/${prevQuarterYear}/q${prevQuarter}`,
          label: `Q${prevQuarter} ${prevQuarterYear}`,
        }
      : null,
  }
}
