/**
 * Pure derivations shared by every paged show archive: the venue page's past
 * shows (PSY-1753) and the artist page's (PSY-1754).
 *
 * Everything here is a function of rows the caller already has — no fetching,
 * no URL state, no React. Kept out of the components so the archive's fiddly
 * bits (month boundaries, page labels, the document title) can be tested
 * against fixtures rather than through a rendered table.
 *
 * Entity-agnostic by construction. A venue archive lists one venue's shows, so
 * every row shares a timezone; an artist archive lists shows ACROSS venues, so
 * each row carries its own. The difference is confined to the {@link
 * ShowZoneResolver} the caller supplies, which is why these functions take one
 * instead of a single zone.
 */

import { toPageNumber } from '@/components/shared/paginationChrome'
import { formatShowMonth, formatShowMonthParts } from '@/lib/utils/formatters'

/** The minimum a row needs before this module can place it in time. */
export interface ArchiveRow {
  event_date: string
}

/**
 * What it takes to read one show's instant on a calendar.
 *
 * Rendering a show's date, time or month is always done where the show HAPPENS,
 * never where the reader is (PSY-985/986). `timezone` is the venue's resolved
 * IANA zone and wins when it is known; `state` is the fallback for venues that
 * predate the backfill.
 */
export interface ShowZone {
  state?: string | null
  timezone?: string | null
}

/**
 * How a caller derives a row's zone.
 *
 * A function rather than a fixed zone because the two archives differ exactly
 * here: the venue archive returns a constant, the artist archive reads each
 * row's own venue.
 */
export type ShowZoneResolver<T> = (row: T) => ShowZone

/**
 * En dash, not a hyphen and never an em dash: this is a range ("Sep–Dec"),
 * which is exactly what an en dash is for, and em dashes are banned in UI copy
 * across this project.
 */
const EN_DASH = '–'

/** A run of consecutive rows that share a month. */
export interface MonthGroup<T> {
  /** Display heading, e.g. "Sep 2025". */
  label: string
  rows: T[]
}

/**
 * Split an already-sorted list into consecutive runs of the same show-local
 * month.
 *
 * Groups RUNS rather than collecting by month, so the caller's ordering is
 * preserved exactly and a list that is not sorted by date degrades into more
 * headings instead of silently reordering rows under merged ones. Months with
 * no rows never appear, because a month only exists here if a row is in it.
 */
export function groupByMonth<T extends ArchiveRow>(
  rows: T[],
  zoneOf: ShowZoneResolver<T>
): MonthGroup<T>[] {
  const groups: MonthGroup<T>[] = []
  for (const row of rows) {
    const zone = zoneOf(row)
    const label = formatShowMonth(row.event_date, zone.state, zone.timezone)
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
export function monthRangeLabel<T extends ArchiveRow>(
  rows: T[],
  zoneOf: ShowZoneResolver<T>
): string | null {
  if (rows.length === 0) return null
  const partsOf = (row: T) => {
    const zone = zoneOf(row)
    return formatShowMonthParts(row.event_date, zone.state, zone.timezone)
  }
  const first = partsOf(rows[0])
  const last = partsOf(rows[rows.length - 1])

  if (first.month === last.month && first.year === last.year) return first.month
  // Page labels sit inside a year-scoped pager, and the year is already in the
  // strip above and the month headings below, so repeating it in every label
  // is noise. Dropped only when both ends agree on the year — an all-years page
  // that straddles a new year keeps both, because there the year IS the news.
  return first.year === last.year
    ? `${first.month}${EN_DASH}${last.month}`
    : `${first.month} ${first.year}${EN_DASH}${last.month} ${last.year}`
}

/**
 * The 1-based page a row offset falls on.
 *
 * Composed on the pagination family's own {@link toPageNumber} so the "a
 * non-finite page is page 1, a fractional one floors" contract has exactly one
 * definition, and adds the upper bound this surface needs: a hand-edited
 * `?page=` must not turn into an unbounded offset the backend has to reject.
 */
export function clampPage(page: number, maxPage: number): number {
  return Math.min(toPageNumber(page, 1), maxPage)
}

/**
 * The range a `?year=` param is accepted in.
 *
 * The upper bound is the backend's own (`maximum:"9999"` on the venue- and
 * artist-shows year params): anything above it would be rejected with a 422
 * rather than answered, so it is filtered out here instead of being sent. The
 * lower bound is older than recorded live music.
 */
export const ARCHIVE_YEAR_RANGE = { min: 1900, max: 9999 } as const

/**
 * The year a `?year=` param should be read as, or null for "every year".
 *
 * Anything that is not a plausible calendar year — a non-integer, a negative,
 * a four-digit-overflowing timestamp someone pasted — reads as null, so a
 * malformed URL lands on the unfiltered archive instead of an empty page or a
 * backend rejection. Membership in the entity's actual year list is NOT checked
 * here: a real year with no shows for THIS entity is a legitimate (if empty)
 * view, and the section says so rather than silently redirecting.
 */
export function parseArchiveYear(raw: number | null): number | null {
  if (raw === null || !Number.isInteger(raw)) return null
  return raw >= ARCHIVE_YEAR_RANGE.min && raw <= ARCHIVE_YEAR_RANGE.max
    ? raw
    : null
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
  entityName,
  year,
  page,
  totalPages,
}: {
  baseTitle: string
  /** The venue or artist the archive belongs to. */
  entityName: string
  year: number | null
  page: number
  totalPages: number
}): string {
  if (year === null && page === 1) return baseTitle

  // LAST separator, not the first: the brand is the trailing segment, and an
  // entity name is contributor-supplied free text that can itself contain
  // " | ".
  const separatorIndex = baseTitle.lastIndexOf(' | ')
  const suffix = separatorIndex === -1 ? '' : baseTitle.slice(separatorIndex)

  const scope = year === null ? 'shows' : `shows in ${year}`
  // Page 1 is the canonical, bare-URL view of a scope; naming it in the title
  // would say something the address bar deliberately does not.
  const pagePart =
    page > 1 && totalPages > 1 ? ` (page ${page} of ${totalPages})` : ''
  return `${entityName} ${scope}${pagePart}${suffix}`
}
