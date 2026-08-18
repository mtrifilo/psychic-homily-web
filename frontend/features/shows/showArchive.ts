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
 * "Jun{EN_DASH}Sep" across several. The span comes from the two END rows,
 * not from every distinct month, so it stays a two-part label no matter how many
 * months a page straddles. The label always prints the earlier month first,
 * whichever way the list runs; see {@link monthSpanLabel}.
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
  zoneOf: ShowZoneResolver<T>,
  // REQUIRED, with no default. A default would be `'one-year'` — the form that
  // elides the year — so a caller who forgot it would get ambiguous labels and
  // no error, which is exactly the defect PSY-1769 found in the shipped code.
  scope: ArchiveLabelScope
): string | null {
  if (rows.length === 0) return null
  const partsOf = (row: T) => {
    const zone = zoneOf(row)
    return formatShowMonthParts(row.event_date, zone.state, zone.timezone)
  }
  const head = rows[0]
  const tail = rows[rows.length - 1]
  const [early, late] =
    Date.parse(head.event_date) <= Date.parse(tail.event_date)
      ? [head, tail]
      : [tail, head]
  return monthSpanLabel(partsOf(early), partsOf(late), scope)
}

/** A formatted calendar month, split so the halves can be compared. */
interface MonthParts {
  month: string
  year: string
}

/**
 * Whether the pager this label is going into is already scoped to ONE year.
 *
 * The distinction decides whether the year may be left out, and getting it wrong
 * is not cosmetic — see {@link monthSpanLabel}.
 */
export type ArchiveLabelScope = 'one-year' | 'all-years'

/**
 * The label for the span between `early` and `late`, which MUST arrive in
 * chronological order.
 *
 * CHRONOLOGICAL ORDER IS THE CONTRACT even though the archive lists run
 * newest-first: a date range reads left-to-right in time everywhere else
 * software prints one, and a reversed label like "Aug{EN_DASH}Jun" scans as a
 * wrap-around (August through the FOLLOWING June) rather than as a page span
 * (user call on PSY-1769). Callers own the swap because only they know their
 * input's order; both do it against a real ordering key, not an assumed list
 * direction.
 *
 * Extracted because two callers derive the SAME label from different inputs
 * ({@link monthRangeLabel} from a page's rows, {@link monthRangeLabelsByPage}
 * from a histogram) and a pager mixing two spellings of one rule would be a
 * defect nobody could see in either function alone.
 *
 * THE YEAR IS ONLY OPTIONAL ON A YEAR-SCOPED PAGER, and that qualifier is
 * load-bearing (PSY-1769). On `/venues/{slug}/shows/{year}` the year is in the
 * URL, the heading and every month row, so repeating it in seven page labels is
 * noise. On the all-years archive none of that is true: `YearStrip` renders with
 * no year selected, and at 50 rows a page most pages sit inside a single year —
 * so eliding it gives a ten-year archive several page links reading "Jun{EN_DASH}Aug"
 * with nothing on the page to say which August.
 *
 * The visible strip is where that bites: it renders `2 · Jun{EN_DASH}Aug` beside
 * `5 · Jun{EN_DASH}Aug`, and a reader choosing between them has only the page
 * number — which is the thing the label exists to explain. (The ACCESSIBLE name
 * is not ambiguous either way: `Pagination` prefixes it with "Page N". This is
 * about what the label communicates, not about name collisions.)
 *
 * When the year is kept and both ends share it, it is printed ONCE at the end
 * ("Nov{EN_DASH}Dec 2025") rather than on both halves. A span across a year boundary
 * has to name both ("Dec 2024{EN_DASH}Jan 2025"), because there the year IS the news.
 */
function monthSpanLabel(
  early: MonthParts,
  late: MonthParts,
  scope: ArchiveLabelScope
): string {
  const sameMonth = early.month === late.month && early.year === late.year
  if (scope === 'one-year') {
    if (sameMonth) return early.month
    return early.year === late.year
      ? `${early.month}${EN_DASH}${late.month}`
      : `${early.month} ${early.year}${EN_DASH}${late.month} ${late.year}`
  }

  if (sameMonth) return `${early.month} ${early.year}`
  return early.year === late.year
    ? `${early.month}${EN_DASH}${late.month} ${late.year}`
    : `${early.month} ${early.year}${EN_DASH}${late.month} ${late.year}`
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
 * A well-formed bucket carrying at least one row. These arrive over the wire, so
 * a NaN or a thirteenth month is possible in principle; see the caller for why
 * one bad bucket voids the whole histogram rather than being skipped.
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
 * THE HISTOGRAM AND THE ROWS MUST AGREE, and if they disagree this returns
 * NOTHING.
 *
 * The whole method rests on one premise — that row ordinal N in the histogram is
 * row ordinal N in the list — and the histogram is a separate read from the rows,
 * so a show that graduated from upcoming to past between them breaks it. It
 * cannot be repaired by clamping: this list is ordered newest-first, so a missing
 * row is missing from the FRONT, and every span after it is shifted by that much
 * while still looking perfectly well-formed. A histogram one row behind would
 * confidently label page 1 "Jun" for a page that opens in July.
 *
 * So a mismatch produces no labels at all, and the caller falls back to whatever
 * it can derive from rows it actually has. That is the pre-existing behaviour —
 * a bare numeral — and it is the right trade: the pager announces the current
 * page's label into a live region and never corrects it, so a wrong label costs
 * more than a missing one. The disagreement is transient; the next read clears
 * it.
 */
export function monthRangeLabelsByPage({
  months,
  pageSize,
  pages,
  listTotal,
  scope,
}: {
  /** Histogram buckets in list order. */
  months: ArchiveMonthCount[]
  /** Rows per page, as requested. */
  pageSize: number
  /** 1-based page numbers to label. */
  pages: number[]
  /**
   * The count that came back WITH the rows, when the caller has one that
   * describes the scope on screen. Omit when it does not — a placeholder page,
   * or a list that has not answered yet — and the histogram is trusted alone.
   *
   * Deliberately the ROW ENVELOPE's total and not some other aggregate: the
   * premise being checked is that the histogram's ordinals are the list's
   * ordinals, and only the list can attest to that. Two aggregates agreeing
   * with each other proves nothing about the rows, and two aggregates that
   * merely age in separate caches would blank every label for no reason.
   */
  listTotal?: number
  /** Whether the pager is already scoped to one year. */
  scope: ArchiveLabelScope
}): Record<number, string> {
  const labels: Record<number, string> = {}
  if (!Number.isInteger(pageSize) || pageSize < 1) return labels

  // FAIL CLOSED on a malformed bucket rather than dropping it. Filtering one out
  // of the MIDDLE would leave a punctured histogram whose ordinals are short
  // from that point on, so every later page would be labelled with months it
  // does not contain — the one outcome this whole function is arranged to avoid.
  if (months.length === 0 || !months.every(isUsableMonthCount)) return labels
  const buckets = months

  const totalRows = buckets.reduce((sum, bucket) => sum + bucket.count, 0)
  // The premise check. A caller with no trustworthy count of its own passes
  // none, and the histogram is trusted alone; one that HAS a count and disagrees
  // with it has proved the ordinals no longer line up.
  if (listTotal !== undefined && listTotal !== totalRows) return labels

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

    // Unreachable by construction — `totalRows` IS the sum of the buckets, so
    // any ordinal below it lands in one. Kept because `bucketAt` is typed to
    // admit null and a silent `undefined` in a label would be worse than a
    // skipped page.
    const first = bucketAt(firstRow)
    const last = bucketAt(lastRow)
    if (!first || !last) continue

    const ordinalOf = (bucket: ArchiveMonthCount) =>
      bucket.year * 12 + bucket.month
    const [early, late] =
      ordinalOf(first) <= ordinalOf(last) ? [first, last] : [last, first]
    labels[page] = monthSpanLabel(
      formatCalendarMonthParts(early.year, early.month),
      formatCalendarMonthParts(late.year, late.month),
      scope
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
 * NOTE: the SERVER-side `?page=` derivation deliberately does not live here. It
 * needs `nuqs/server`, and this module is imported by six client modules — both
 * archives' tables and lists, and both entities' hooks — so an import here ships
 * a second copy of nuqs to the browser (measured: 24,576 bytes against 134).
 * See `showArchive.server.ts`.
 */

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
