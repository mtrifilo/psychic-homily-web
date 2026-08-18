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
import {
  formatCalendarMonthParts,
  formatShowMonth,
  formatShowMonthParts,
} from '@/lib/utils/formatters'

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
 * En dash, not a hyphen and never an em dash.
 *
 * Two jobs, one character, which is why it is declared once: it joins a range
 * ("Sep–Dec"), which is exactly what an en dash is for, and it stands in for a
 * value nobody recorded. Em dashes are banned in UI copy across this project.
 */
export const EN_DASH = '–'

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
 * "Jun{EN_DASH}Sep" across several. The span comes from the FIRST and LAST rows,
 * not from every distinct month, so it stays a two-part label no matter how many
 * months a page straddles.
 *
 * The ROW-DERIVED half of the page-label family, and the weaker one: rows can
 * only label a page that has been fetched. {@link monthRangeLabelsByPage} does
 * the same job from a month histogram and can therefore label every page at
 * once, which is what the venue archive uses (PSY-1769). This form survives for
 * the ARTIST archive, which has no month histogram endpoint yet.
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
  return monthSpanLabel(partsOf(rows[0]), partsOf(rows[rows.length - 1]))
}

/** A formatted calendar month, split so the halves can be compared. */
interface MonthParts {
  month: string
  year: string
}

/**
 * The label for a span running from `first` to `last`, in whichever direction
 * the list runs.
 *
 * Extracted because two callers now derive the SAME label from different inputs
 * — {@link monthRangeLabel} from a page's rows, {@link monthRangeLabelsByPage}
 * from a histogram — and a pager mixing two spellings of one rule would be a
 * defect nobody could see in either function alone.
 */
function monthSpanLabel(first: MonthParts, last: MonthParts): string {
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
 * One bar of a show histogram at month resolution, as the backend serves it.
 *
 * Already placed on the right calendar: the counts are bucketed venue-side, so
 * nothing downstream needs — or is allowed — a timezone to re-derive them with.
 */
export interface ArchiveMonthCount {
  /** Calendar year. */
  year: number
  /** Calendar month, 1-12. */
  month: number
  count: number
}

/**
 * A well-formed bucket carrying at least one row. Anything else is dropped
 * rather than trusted: these arrive over the wire, and a NaN or a thirteenth
 * month would silently slide every later page's label instead of failing.
 */
function isUsableMonthCount(bucket: ArchiveMonthCount): boolean {
  return (
    Number.isInteger(bucket.year) &&
    Number.isInteger(bucket.month) &&
    bucket.month >= 1 &&
    bucket.month <= 12 &&
    Number.isInteger(bucket.count) &&
    bucket.count > 0
  )
}

/**
 * The month-span label for EVERY page of an archive, derived from its month
 * histogram (PSY-1769).
 *
 * The point of taking counts rather than rows: a page's span is a function of
 * the row ORDINALS it covers, and cumulative counts answer that for every page
 * at once. Deriving it from rows — the shape this replaced — could only ever
 * label the pages the reader had already fetched, so an eight-page archive
 * showed one label and seven bare numerals on first paint. The rest of the
 * chrome (which pages are rendered at all, how a missing label degrades) is
 * unchanged: `Pagination` still renders a bare numeral for any page absent from
 * the returned record.
 *
 * `months` must be in the SAME ORDER the list pages in — the walk below maps
 * position in this array onto position in the list, and nothing else can tell it
 * that a descending histogram belongs to a descending list. `pageSize` must be
 * the limit the list actually requested.
 *
 * Labels are produced only for the pages ASKED for, because the pager renders at
 * most seven, and only while the histogram covers them: a page whose first row
 * lies past the last bucket gets no label rather than a clamped, wrong one. That
 * is the graceful edge when the counts and the list's own total disagree — a
 * show added between the two reads, a filter drifting apart — and it degrades to
 * exactly the numeral the pager rendered before this existed.
 */
export function monthRangeLabelsByPage({
  months,
  pageSize,
  pages,
}: {
  /** Histogram buckets in list order. */
  months: ArchiveMonthCount[]
  /** Rows per page, as requested. */
  pageSize: number
  /** 1-based page numbers to label. */
  pages: number[]
}): Record<number, string> {
  const labels: Record<number, string> = {}
  if (!Number.isInteger(pageSize) || pageSize < 1) return labels

  const buckets = months.filter(isUsableMonthCount)
  if (buckets.length === 0) return labels

  const totalRows = buckets.reduce((sum, bucket) => sum + bucket.count, 0)

  // The bucket a row ordinal falls in, by accumulating counts until the ordinal
  // is covered. Rescanned per lookup rather than precomputed: the pager asks at
  // most fourteen times (two ends of at most seven pages) over a list with one
  // entry per month a venue has ever booked.
  const bucketAt = (rowIndex: number): ArchiveMonthCount | null => {
    let covered = 0
    for (const bucket of buckets) {
      covered += bucket.count
      if (rowIndex < covered) return bucket
    }
    return null
  }

  for (const page of pages) {
    if (!Number.isInteger(page) || page < 1) continue
    const firstRow = (page - 1) * pageSize
    if (firstRow >= totalRows) continue
    const lastRow = Math.min(firstRow + pageSize - 1, totalRows - 1)

    const first = bucketAt(firstRow)
    const last = bucketAt(lastRow)
    if (!first || !last) continue

    labels[page] = monthSpanLabel(
      formatCalendarMonthParts(first.year, first.month),
      formatCalendarMonthParts(last.year, last.month)
    )
  }

  return labels
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
