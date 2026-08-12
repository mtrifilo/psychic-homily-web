/**
 * Wording for the named new-bands module (PSY-1784).
 *
 * The module replaced Scene Pulse, whose defect was not its layout but its
 * arithmetic: numbers whose definition nobody could state. Every string here
 * therefore states the fact the payload actually selected on, which is why the
 * date helpers below are so fussy about which clock they read.
 */

import { isCalendarDate, parseCalendarDate } from './sceneWeek'
import type { SceneNewArtistShow } from './types'

/**
 * `Aug 22` from a `YYYY-MM-DD` calendar date, or null when it is not one.
 *
 * The SHAPE is the whole guard, and it has to run first — see `isCalendarDate`
 * for why a post-hoc NaN check would never fire.
 */
function formatCalendarMonthDay(iso: string): string | null {
  const trimmed = iso.trim()
  if (!isCalendarDate(trimmed)) return null
  return parseCalendarDate(trimmed).toLocaleDateString('en-US', {
    month: 'short',
    day: 'numeric',
  })
}

/**
 * `first listed Jul 14` — the catalog bookkeeping timestamp, read in UTC.
 *
 * `first_listed_at` is an INSTANT (`created_at`), not a calendar date, and it is
 * the field the window selected on. Rendering it in the reader's own zone would
 * put a band "first listed Jul 13" for anyone west of UTC, which contradicts the
 * cutoff that put it in the list at all. UTC is the clock the backend counted
 * in, so UTC is the clock we print.
 */
export function formatFirstListed(isoInstant: string): string | null {
  const at = new Date(isoInstant)
  if (Number.isNaN(at.getTime())) return null
  return at.toLocaleDateString('en-US', {
    month: 'short',
    day: 'numeric',
    timeZone: 'UTC',
  })
}

/**
 * The band's show clause, worded from `is_upcoming` rather than assumed.
 *
 * The mock draws every row as `first show {date}, {venue}`, and every band it
 * drew had one ahead of it. The payload does not promise that twice over: the
 * attached show is the soonest UPCOMING approved show when there is one and the
 * most recent PAST one otherwise, and it is any approved show — including one
 * on tour, outside this scene. So:
 *
 *   - upcoming → `next show Aug 22, Nile Theater`
 *   - past     → `last played Mar 2, Valley Bar`
 *   - absent   → `no show listed yet`
 *
 * "first" is the one word not used, in either branch. A band added three weeks
 * ago may already have played twice; the payload carries no ordinal, and
 * inventing one would be the same class of unsourced claim the pulse shipped.
 *
 * The venue clause drops when the show carries no venue name — a real state for
 * an intent-only listing — rather than printing a dangling comma.
 */
export function formatNewBandShow(show: SceneNewArtistShow | undefined): string {
  if (!show) return 'no show listed yet'

  const day = formatCalendarMonthDay(show.event_date ?? '')
  const lead = show.is_upcoming ? 'next show' : 'last played'
  const venue = show.venue_name?.trim()

  // A show we cannot date is still a show we can name a room for; and one we
  // can date but not place is still worth the date. Only when both are missing
  // does the clause carry nothing a reader could act on.
  if (!day) return venue ? `${lead} at ${venue}` : 'no show listed yet'
  return venue ? `${lead} ${day}, ${venue}` : `${lead} ${day}`
}
