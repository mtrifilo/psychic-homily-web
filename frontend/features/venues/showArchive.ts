/**
 * Pure derivations for the venue page's past-shows archive (PSY-1753).
 *
 * Everything here is a function of rows the caller already has — no fetching,
 * no URL state, no React. Kept out of the components so the archive's fiddly
 * bits (month boundaries, page labels, the document title) can be tested
 * against fixtures rather than through a rendered table.
 */

import { formatShowMonth } from '@/lib/utils/formatters'

/** The minimum a row needs before this module can place it in time. */
export interface ArchiveRow {
  event_date: string
  /** The show's own state, when it differs from the venue's. */
  state?: string | null
}

/** How to resolve a row's venue-local timezone. */
export interface ArchiveZone {
  /** Fallback state for rows that carry none. */
  venueState: string
  /** The venue's resolved IANA zone; wins over the state map when known. */
  venueTimezone?: string | null
}

/**
 * En dash, not a hyphen and never an em dash: this is a range ("Sep–Dec"),
 * which is exactly what an en dash is for, and em dashes are banned in UI copy
 * across this project.
 */
const EN_DASH = '–'

/** The month label a row belongs under, in the venue's own zone. */
function monthOf(row: ArchiveRow, zone: ArchiveZone): string {
  return formatShowMonth(
    row.event_date,
    row.state ?? zone.venueState,
    zone.venueTimezone
  )
}

/** A run of consecutive rows that share a month. */
export interface MonthGroup<T> {
  /** Display heading, e.g. "Sep 2025". */
  label: string
  rows: T[]
}

/**
 * Split an already-sorted list into consecutive runs of the same venue-local
 * month.
 *
 * Groups RUNS rather than collecting by month, so the caller's ordering is
 * preserved exactly and a list that is not sorted by date degrades into more
 * headings instead of silently reordering rows under merged ones. Months with
 * no rows never appear, because a month only exists here if a row is in it.
 */
export function groupByMonth<T extends ArchiveRow>(
  rows: T[],
  zone: ArchiveZone
): MonthGroup<T>[] {
  const groups: MonthGroup<T>[] = []
  for (const row of rows) {
    const label = monthOf(row, zone)
    const current = groups[groups.length - 1]
    if (current && current.label === label) {
      current.rows.push(row)
    } else {
      groups.push({ label, rows: [row] })
    }
  }
  return groups
}

/**
 * The span of months a page of rows covers: "Sep" for a single month,
 * "Jun{EN_DASH}Sep" across several.
 *
 * This is the Gazelle `451-500` page label ported to the time axis — it tells
 * the reader what is behind a page number before they spend a click on it. The
 * span comes from the FIRST and LAST rows, not from every distinct month, so
 * it stays a two-part label no matter how many months a page straddles.
 *
 * Returns null for an empty page, so callers can omit the label rather than
 * render an empty separator.
 */
export function monthRangeLabel(
  rows: ArchiveRow[],
  zone: ArchiveZone
): string | null {
  if (rows.length === 0) return null
  const first = monthOf(rows[0], zone)
  const last = monthOf(rows[rows.length - 1], zone)
  if (first === last) return stripYear(first)
  // Page labels sit inside a year-scoped pager, and the year is already in the
  // strip above and the month headings below, so repeating it in every label
  // is noise. Dropped only when both ends agree on the year — an all-years page
  // that straddles a new year keeps both, because there the year IS the news.
  const firstYear = yearPart(first)
  return firstYear !== null && firstYear === yearPart(last)
    ? `${stripYear(first)}${EN_DASH}${stripYear(last)}`
    : `${first}${EN_DASH}${last}`
}

/** "Sep 2025" -> "2025"; null when the label has no trailing year. */
function yearPart(monthLabel: string): string | null {
  const parts = monthLabel.split(' ')
  return parts.length > 1 ? parts[parts.length - 1] : null
}

/** "Sep 2025" -> "Sep". */
function stripYear(monthLabel: string): string {
  const parts = monthLabel.split(' ')
  return parts.length > 1 ? parts.slice(0, -1).join(' ') : monthLabel
}

/**
 * The 1-based page a row offset falls on.
 *
 * Bounded below at 1 and above at `maxPage` so a hand-edited `?page=` cannot
 * turn into an unbounded offset the backend has to reject. A non-integer or
 * non-finite input resolves to page 1 rather than propagating NaN into the
 * offset arithmetic.
 */
export function clampPage(page: number, maxPage: number): number {
  if (!Number.isFinite(page)) return 1
  return Math.min(Math.max(1, Math.floor(page)), maxPage)
}

/**
 * The year a `?year=` param should be read as, or null for "every year".
 *
 * Anything that is not a plausible calendar year — a non-integer, a negative,
 * a four-digit-overflowing timestamp someone pasted — reads as null, so a
 * malformed URL lands on the unfiltered archive instead of an empty page or a
 * backend rejection. Membership in the venue's actual year list is NOT checked
 * here: a real year with no shows at THIS venue is a legitimate (if empty)
 * view, and the section says so rather than silently redirecting.
 */
export function parseArchiveYear(raw: number | null): number | null {
  if (raw === null || !Number.isInteger(raw)) return null
  return raw >= 1900 && raw <= 2999 ? raw : null
}

/**
 * The document title for the current archive scope.
 *
 * The brand suffix is carried over from `baseTitle` (whatever the route's
 * server metadata rendered) rather than re-stated here, so this cannot drift
 * out of sync with the root layout's title template.
 */
export function archiveDocumentTitle({
  baseTitle,
  venueName,
  year,
  page,
  totalPages,
}: {
  baseTitle: string
  venueName: string
  year: number | null
  page: number
  totalPages: number
}): string {
  if (year === null && page === 1) return baseTitle

  const separatorIndex = baseTitle.indexOf(' | ')
  const suffix = separatorIndex === -1 ? '' : baseTitle.slice(separatorIndex)

  const scope = year === null ? 'shows' : `shows in ${year}`
  // Page 1 is the canonical, bare-URL view of a scope; naming it in the title
  // would say something the address bar deliberately does not.
  const pagePart =
    page > 1 && totalPages > 1 ? ` (page ${page} of ${totalPages})` : ''
  return `${venueName} ${scope}${pagePart}${suffix}`
}
