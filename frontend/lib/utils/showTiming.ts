import { resolveShowTimezone } from './formatters'

/**
 * Where a show sits relative to now, answered once, in the VENUE's timezone.
 *
 * Four surfaces used to answer this separately and disagree: the show page's
 * JSON-LD, the scene pages' JSON-LD, the field-notes gate, and the share card's
 * cache lifetime. Each one compared the start instant against the reader's or
 * the server's clock, so "is this show over" changed answer depending on where
 * the reader was sitting. A show is over when it is over WHERE IT HAPPENED.
 *
 * Two boundaries live here because the consumers genuinely need two, and the
 * distinction is the thing that kept getting lost:
 *
 * - `isShowPast` — the show's calendar day has ended at the venue. This is the
 *   boundary for anything that describes the show to the outside world, where
 *   the question is "is this still a live listing" and a show at 9 PM is still
 *   one at 11 PM.
 * - `hasShowStarted` — the advertised start instant has passed. This is the
 *   boundary for anything the BACKEND also enforces on the same instant, where
 *   the frontend's job is to agree with it rather than to be more correct.
 *
 * Both fail the same way: a show whose start cannot be read counts as already
 * happened. The failure directions are not symmetric — guessing "upcoming"
 * republishes an `InStock` offer forever, which is the bug this exists to
 * remove — and one rule for both keeps the module's contract to a sentence.
 *
 * There is deliberately NO post-midnight grace window ("a Friday show is still
 * Friday at 1 AM Saturday"). The lifecycle design calls for one, its duration
 * is an open product decision, and inventing a constant here would quietly bind
 * every consumer to a number nobody chose. It belongs in `isShowPast` when it
 * is decided, and nowhere else.
 */

/**
 * The venue-facing slice of a show payload. All three fields are optional or
 * nullable because every payload that carries them is delivered over the wire:
 * a TYPE is not a runtime guarantee when the frontend and backend deploy
 * separately and Next's data cache can serve a body fetched before a schema
 * change.
 */
export interface ShowTimingInput {
  /** The UTC start instant, as the API delivers it. */
  eventDate: string | null | undefined
  /** `venues.timezone` (GeoNames-backed), when the venue has one resolved. */
  timezone?: string | null
  /** US state code, the fallback zone for venues predating the backfill. */
  state?: string | null
}

/**
 * The venue-local calendar date of an instant, as a comparable integer
 * (`20260314` for March 14 2026).
 *
 * Packed into one number rather than compared as `Date`s so the comparison is
 * over calendar days and nothing else: a `Date` reconstructed from local parts
 * would reintroduce a zone, and subtracting instants would reintroduce DST.
 * Ordering is preserved because each field occupies a fixed decimal width.
 */
function venueLocalDayOrdinal(instant: number, timeZone: string): number {
  const parts = new Intl.DateTimeFormat('en-US', {
    timeZone,
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
  }).formatToParts(new Date(instant))
  const part = (type: string) => Number(parts.find(p => p.type === type)?.value)
  return part('year') * 10000 + part('month') * 100 + part('day')
}

/** The start instant in epoch milliseconds, or `null` when it cannot be read. */
function startInstantMs(show: ShowTimingInput): number | null {
  const at = Date.parse(show.eventDate ?? '')
  return Number.isFinite(at) ? at : null
}

/**
 * Whether the show's calendar day has ended in the venue's own timezone.
 *
 * The boundary is venue-local midnight, not the start instant: a show that
 * started two hours ago is still tonight's show, and a listing that stops
 * describing it mid-set is wrong in a way readers notice. An 8 PM Phoenix show
 * is past at 00:00 Phoenix time, whatever the reader's clock says.
 *
 * `now` is injectable so callers on the server can stay pure functions of their
 * input and so tests do not depend on the wall clock.
 */
export function isShowPast(show: ShowTimingInput, now: Date = new Date()): boolean {
  const startedAt = startInstantMs(show)
  if (startedAt === null) return true
  const timeZone = resolveShowTimezone(show.state, show.timezone)
  return (
    venueLocalDayOrdinal(now.getTime(), timeZone) >
    venueLocalDayOrdinal(startedAt, timeZone)
  )
}

/**
 * Whether the show's advertised start instant has passed.
 *
 * Zone-independent by construction — an instant is the same instant everywhere
 * — and that is the point: the field-notes gate this backs exists to mirror the
 * backend's own `show.EventDate.After(time.Now())` rejection, and a frontend
 * that picked a different boundary would either hide a form the API would have
 * accepted or offer one it is about to reject with a 400.
 */
export function hasShowStarted(show: ShowTimingInput, now: Date = new Date()): boolean {
  const startedAt = startInstantMs(show)
  return startedAt === null || startedAt <= now.getTime()
}
