/**
 * The artist-shaped face of the shared show archive (PSY-1754).
 *
 * The derivations themselves live in `@/features/shows/showArchive`, which the
 * venue archive uses too. Only one thing differs between the two entities:
 * where a row's timezone comes from. A venue archive lists ONE venue's shows,
 * so the zone is the venue's for every row; an artist archive lists shows
 * ACROSS venues, so each row is placed in its own venue's zone. This module
 * binds the artist answer once, so no artist call site has to restate it.
 */

import type { ShowZone } from '@/features/shows/showArchive'
import type { ArtistShow } from './types'

/**
 * Where one of an artist's shows is read on the calendar: its own venue's
 * resolved IANA zone, falling back to the venue's state until the timezone
 * backfill reaches it (PSY-985/986).
 *
 * A row with no venue at all resolves to NEITHER, and `resolveShowTimezone`
 * then hands back its silent America/Phoenix default — the same zone for every
 * reader on earth, not the reader's own. That default is wrong by up to most of
 * a day for a show outside the US, which is why `isShowTimezoneResolved` exists;
 * this surface does not consult it yet, and neither does the venue archive.
 * Gating the clock column on it is a follow-up for both.
 */
export function artistShowZone(show: Pick<ArtistShow, 'venue'>): ShowZone {
  return { state: show.venue?.state, timezone: show.venue?.timezone }
}
