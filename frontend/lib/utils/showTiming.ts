import { resolveShowTimezone } from './formatters'

/**
 * Where a show sits relative to now, answered once.
 *
 * Two questions, two exports, because the surfaces asking are not asking the
 * same thing and collapsing them is how this went wrong before:
 *
 * - "Can a reader still BUY this?" is a question about the start instant.
 *   Doors open once, at one moment, everywhere in the world.
 * - "Is this still a live LISTING?" is a question about the venue's calendar
 *   day. A show is over when it is over WHERE IT HAPPENED, not when the
 *   reader's clock rolls over.
 *
 * Pick by the claim being made, not by which reads better. An `Offer` is the
 * first kind; a cache lifetime is the second.
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
 * Day formatters, one per resolved zone, kept for the process's life.
 *
 * Constructing an `Intl.DateTimeFormat` costs one to two orders of magnitude
 * more than formatting with one, and `sceneShowEvents` asks this question once
 * per show over an uncapped list. Measure before quoting a number here: the
 * ratio moves with the ICU build, and `resolveShowTimezone` still constructs a
 * throwaway formatter of its own to validate the zone on every call, so the
 * saving is a fraction of the per-call cost rather than all of it.
 *
 * Capped rather than left to grow. `Intl` matches IANA names
 * case-insensitively and accepts legacy aliases, so one zone has many accepted
 * spellings and the key is whatever spelling reached us. Today every spelling
 * is canonical, because `venues.timezone` is written only by the server-side
 * GeoNames lookup and never from request input, but that is an invariant of
 * another service. The cap makes the bound a property of THIS module, so a
 * future admin override or CSV import cannot turn a memo into a leak.
 */
const MAX_CACHED_ZONES = 512
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
  // IANA has a few hundred zones, so passing the cap means the keys have become
  // spellings rather than zones and this has stopped working as a cache.
  // Dropping all of it is the right response: the next calls rebuild what they
  // actually need, and memory stays flat.
  if (dayFormatters.size >= MAX_CACHED_ZONES) dayFormatters.clear()
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
 * started two hours ago is still tonight's listing, and an 8 PM Phoenix show is
 * past at 00:00 Phoenix time whatever the reader's clock says.
 *
 * Use this for LISTING LIVENESS — how long a share card may be cached, whether
 * a row is still "tonight". Do NOT use it to decide whether tickets are still
 * on sale: doors close at an instant, and stretching that to the end of the
 * local day would publish a purchasable ticket for a show already in progress,
 * or for nearly a full day after an after-midnight one. `hasShowStarted` is the
 * predicate for that question.
 *
 * Both inputs are guarded: an undateable show counts as past, and an unreadable
 * `now` counts as not-past, because the alternative is `Intl` throwing
 * `RangeError` out of a server component.
 *
 * `now` is injectable so callers on the server can stay pure functions of their
 * input and so tests do not depend on the wall clock.
 */
export function isShowPast(show: ShowTimingInput, now: Date = new Date()): boolean {
  const startedAt = startInstantMs(show.eventDate)
  if (startedAt === null) return true
  const readAt = now.getTime()
  if (!Number.isFinite(readAt)) return false
  const timeZone = resolveShowTimezone(show.state, show.timezone)
  return (
    venueLocalDayOrdinal(readAt, timeZone) >
    venueLocalDayOrdinal(startedAt, timeZone)
  )
}

/**
 * Whether the show's advertised start instant has passed.
 *
 * Use this whenever the answer is a claim about the SHOW ITSELF rather than
 * about the listing: whether an `Offer` may still say `InStock`, whether the
 * field-notes form may open. Doors open at one moment, and no reader's calendar
 * moves it.
 *
 * Takes the instant alone, not a `ShowTimingInput`: an instant is the same
 * instant in every zone, and accepting zone fields it cannot use would invite
 * callers to pass one and expect it to matter. That zone-independence is the
 * point for the field-notes gate, which exists to mirror the backend's own
 * `show.EventDate.After(time.Now())` rejection; a frontend that picked a
 * different boundary would either hide a form the API would have accepted or
 * offer one it is about to reject with a 400.
 *
 * An undateable show counts as started. Note this is the SAME rule as
 * `isShowPast`'s undateable case but NOT the same fail direction: there,
 * "already happened" withholds an offer, which is safe; here it opens a form
 * the API may then reject. It is deliberate anyway, because it is what this
 * gate already did before the derivation moved here, and because the API is the
 * authority on that rejection either way. The alternative renders "available
 * after Invalid Date".
 */
export function hasShowStarted(
  eventDate: string | null | undefined,
  now: Date = new Date()
): boolean {
  const startedAt = startInstantMs(eventDate)
  return startedAt === null || startedAt <= now.getTime()
}
