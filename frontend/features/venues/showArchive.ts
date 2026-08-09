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

/** {@link rowsMonthRangeLabel}, in the venue's zone. */
export function monthRangeLabel(
  rows: ArchiveRow[],
  zone: VenueShowZone
): string | null {
  return rowsMonthRangeLabel(rows, venueZoneResolver<ArchiveRow>(zone))
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
