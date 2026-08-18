/**
 * The venue-shaped face of the shared show archive (PSY-1753, generalized in
 * PSY-1754).
 *
 * The derivations themselves live in `@/features/shows/showArchive`, which the
 * artist archive uses too, and callers take the entity-agnostic half (year
 * parsing, page clamping, the en dash) straight from there. What is left here
 * is the one thing that is genuinely venue-shaped: where a row's timezone comes
 * from. A venue archive lists ONE venue's shows, so the zone is the venue's for
 * every row, with the row's own denormalized `state` still winning when the two
 * disagree; an artist archive lists shows across venues, so each row carries
 * its own. Binding that answer once is what stops every venue call site from
 * restating it.
 */

import {
  groupByMonth as groupRowsByMonth,
  monthRangeLabel as rowsMonthRangeLabel,
  archiveDocumentTitle as scopedDocumentTitle,
  type ArchiveLabelScope,
  type ArchiveRow as ShowArchiveRow,
  type ShowZoneResolver,
} from '@/features/shows/showArchive'
import type { VenueShowZone } from './types'

/**
 * What the shared module needs, plus the one field only a venue row has: its
 * own denormalized state.
 */
export interface ArchiveRow extends ShowArchiveRow {
  /** The show's own state, when it differs from the venue's. */
  state?: string | null
}

/**
 * Every row is read on the venue's calendar, except where the row's own
 * denormalized `state` disagrees with it — that is the older, per-show answer
 * and still wins over the venue-level fallback.
 */
function venueZoneResolver<T extends ArchiveRow>(
  zone: VenueShowZone
): ShowZoneResolver<T> {
  return row => ({
    state: row.state ?? zone.venueState,
    timezone: zone.venueTimezone,
  })
}

/** {@link groupRowsByMonth}, in the venue's zone. */
export function groupByMonth<T extends ArchiveRow>(
  rows: T[],
  zone: VenueShowZone
) {
  return groupRowsByMonth(rows, venueZoneResolver<T>(zone))
}

/**
 * {@link rowsMonthRangeLabel}, in the venue's zone.
 *
 * No longer the archive's PRIMARY page-label mechanism — the month histogram is
 * (PSY-1769) — but not dead either. It is the fallback for the CURRENT page when
 * the histogram is unavailable: a failed histogram fetch would otherwise strip
 * the label from every page link, and on mobile the pager renders no page links
 * at all, so the current page's label is the only one there is. Rows for the
 * page on screen are always in hand, so this costs nothing and keeps the mobile
 * pager's label a guarantee rather than a second request's good fortune.
 */
export function monthRangeLabel(
  rows: ArchiveRow[],
  zone: VenueShowZone,
  scope: ArchiveLabelScope
): string | null {
  return rowsMonthRangeLabel(rows, venueZoneResolver<ArchiveRow>(zone), scope)
}

/**
 * The fragment the venue page's past-shows section is addressed by.
 *
 * Declared here rather than imported from the component because this module
 * owns the venue archive's URL SPACE, and `venueArchiveHref` below has to
 * append it. The component re-exports it as `VENUE_PAST_SHOWS_ANCHOR` for the
 * markup that carries the id.
 */
export const VENUE_PAST_SHOWS_FRAGMENT = 'venue-past-shows'

/**
 * THE address of one view of a venue's past-show archive. One function, so the
 * client's navigation and the crawler's URLs are the same strings (PSY-1756).
 *
 * The space it defines, in full:
 *
 *   every year, page 1   /venues/{slug}#venue-past-shows
 *   every year, page N   /venues/{slug}?page=N#venue-past-shows
 *   one year,   page 1   /venues/{slug}/shows/{year}
 *   one year,   page N   /venues/{slug}/shows/{year}?page=N
 *
 * A YEAR is a path segment because a venue's year archive is its own document:
 * it gets a server-rendered body, a descriptive title and a self-referencing
 * canonical, none of which a query variant of the venue route can carry (the
 * venue page is ISR and reads no searchParams). A PAGE stays a query parameter
 * because it is not a document — every `?page=` canonicalizes to the year root
 * it slices, so minting path URLs for pages would advertise addresses that
 * immediately point somewhere else.
 *
 * There is deliberately NO `?year=` form here. It existed before the year
 * archives did, and leaving it in place would have put the same rows at two
 * addresses with no canonical relationship between them — the duplicate-content
 * shape PSY-1756 exists to avoid. `proxy.ts` 308s the old form to its year path
 * so links already in the wild keep meaning what they said. The ARTIST archive
 * still reads `?year=`, deliberately: it has no crawlable per-year route yet,
 * so there is no second address for its query form to duplicate.
 *
 * The all-years URLs carry the fragment because the archive is one section of a
 * long venue page; the year URLs do not, because there the archive IS the page.
 */
export function venueArchiveHref(
  venueSlug: string,
  year: number | null,
  page: number
): string {
  const query = page > 1 ? `?page=${page}` : ''
  return year === null
    ? `/venues/${venueSlug}${query}#${VENUE_PAST_SHOWS_FRAGMENT}`
    : `/venues/${venueSlug}/shows/${year}${query}`
}

/** {@link scopedDocumentTitle}, named for the venue it scopes. */
export function archiveDocumentTitle({
  venueName,
  ...rest
}: {
  baseTitle: string
  venueName: string
  year: number | null
  page: number
  totalPages: number
}): string {
  return scopedDocumentTitle({ ...rest, entityName: venueName })
}
