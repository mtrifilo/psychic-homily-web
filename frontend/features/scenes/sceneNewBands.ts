/**
 * Wording for the latest-additions module.
 *
 * Every string here states a fact the payload carries, and no string states a
 * quantity or a period the payload does not. That rule is why the date helpers
 * below are so fussy about which clock they read and about which of a show's
 * two possible tenses they are wording.
 */

import { formatCalendarMonthDay } from './sceneWeek'
import type { SceneNewArtistShow } from './types'

/**
 * `first listed Jul 14` — the catalog bookkeeping timestamp, read in UTC.
 *
 * `first_listed_at` is an INSTANT (`created_at`), not a calendar date, and it is
 * the field the list is ordered on. Rendering it in the reader's own zone would
 * put a band "first listed Jul 13" for anyone west of UTC, which contradicts the
 * order it appears in. UTC is the clock the backend sorted in, so UTC is the
 * clock we print.
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
 * "first" is the one word not used, in either branch. A band listed three weeks
 * ago may already have played twice, and the payload carries no ordinal to say
 * otherwise.
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
