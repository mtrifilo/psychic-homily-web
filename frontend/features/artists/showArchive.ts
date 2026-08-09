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
 * A row with no venue at all falls back to the reader's locale, which is the
 * only calendar left once the show has no place.
 */
export function artistShowZone(show: Pick<ArtistShow, 'venue'>): ShowZone {
  return { state: show.venue?.state, timezone: show.venue?.timezone }
}
