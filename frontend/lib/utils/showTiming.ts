import { resolveShowTimezone } from './formatters'

/**
 * Where a show sits relative to now, answered once. See each export for the
 * boundary it draws and which consumers that boundary serves.
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
 * Day formatters, one per zone, kept for the process's life.
 *
 * `Intl.DateTimeFormat` construction dominates the cost of reading a calendar
 * date: measured here at ~113 µs per constructed formatter against ~3 µs to
 * reuse one. `sceneShowEvents` asks this question once per show over an
 * uncapped list, so the difference is tens of milliseconds of server render on
 * a busy week. The key space is the set of IANA zone names that reach us, so
 * the map is bounded without an eviction policy.
 */
const dayFormatters = new Map<string, Intl.DateTimeFormat>()

function dayFormatter(timeZone: string): Intl.DateTimeFormat {
  const cached = dayFormatters.get(timeZone)
  if (cached) return cached
  const created = new Intl.DateTimeFormat('en-US', {
    timeZone,
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
  })
  dayFormatters.set(timeZone, created)
  return created
}

/**
 * The venue-local calendar date of an instant, as a comparable integer
 * (`20260314` for March 14 2026).
 *
 * Packed into one number rather than compared as `Date`s so the comparison is
 * over calendar days and nothing else: a `Date` reconstructed from local parts
 * would reintroduce a zone, and subtracting instants would reintroduce DST.
 * Ordering holds because each field occupies a fixed decimal width.
 */
function venueLocalDayOrdinal(instant: number, timeZone: string): number {
  const parts = dayFormatter(timeZone).formatToParts(new Date(instant))
  const part = (type: string) => Number(parts.find(p => p.type === type)?.value)
  return part('year') * 10000 + part('month') * 100 + part('day')
}

/** The start instant in epoch milliseconds, or `null` when it cannot be read. */
function startInstantMs(eventDate: string | null | undefined): number | null {
  const at = Date.parse(eventDate ?? '')
  return Number.isFinite(at) ? at : null
}

/**
 * Whether the show's calendar day has ended in the venue's own timezone.
 *
 * The boundary is venue-local midnight, not the start instant: a show that
 * started two hours ago is still tonight's show, and a listing that stops
 * describing it mid-set is wrong in a way readers notice. An 8 PM Phoenix show
 * is past at 00:00 Phoenix time, whatever the reader's clock says. This is the
 * boundary for anything that describes the show to the outside world — JSON-LD
 * offers, share-card cache lifetime — where the question is "is this still a
 * live listing".
 *
 * An undateable show counts as past. Guessing "upcoming" is what republished an
 * `InStock` offer forever, which is the bug this exists to remove.
 *
 * `now` is injectable so callers on the server can stay pure functions of their
 * input and so tests do not depend on the wall clock.
 */
export function isShowPast(show: ShowTimingInput, now: Date = new Date()): boolean {
  const startedAt = startInstantMs(show.eventDate)
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
 * Takes the instant alone, not a `ShowTimingInput`: an instant is the same
 * instant in every zone, and accepting zone fields it cannot use would invite
 * callers to pass one and expect it to matter. That zone-independence is the
 * point — the field-notes gate this backs exists to mirror the backend's own
 * `show.EventDate.After(time.Now())` rejection, and a frontend that picked a
 * different boundary would either hide a form the API would have accepted or
 * offer one it is about to reject with a 400.
 *
 * An undateable show counts as started, matching `isShowPast`: when the date
 * cannot be read, both treat the show as already happened.
 */
export function hasShowStarted(
  eventDate: string | null | undefined,
  now: Date = new Date()
): boolean {
  const startedAt = startInstantMs(eventDate)
  return startedAt === null || startedAt <= now.getTime()
}
