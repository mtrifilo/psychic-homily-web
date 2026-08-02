import { resolveShowTimezone } from '@/lib/utils/formatters'
import type { ShowLifecycleState } from '@/lib/utils/showTiming'
import type { ShowResponse } from '../types'

/**
 * Copy for the show page's status stripe: the one typographic band at the top
 * of the page that says where this show sits in time.
 *
 * Kept apart from the component because every hard part here is a string, not
 * a DOM tree: which segments appear, in which order, in whose timezone. The
 * component's job is one row of spans.
 *
 * Every segment is venue-local and derived from the show payload alone. The
 * only clock-dependent input is `lifecycle`, which the SERVER computes and
 * passes in, so the band renders identically on both sides of hydration and
 * never tells a reader in Berlin that a Phoenix show is on tonight.
 */

/**
 * How long a show is assumed to run when nobody has said. Doors plus this is
 * rendered as an estimate ("ENDS ~11PM (EST.)") and is deliberately shallow:
 * it is never stored, never emitted into JSON-LD, and never shown without the
 * "~" and the "(EST.)" that mark it as a guess. "(EST.)" abbreviates ESTIMATED
 * and is not a timezone; the times beside it are already venue-local.
 *
 * A flat constant rather than a per-venue or per-genre curve on purpose: a
 * wrong-but-uniform estimate reads as the convention it is, while a
 * confidently-varying one reads as knowledge we do not have.
 */
export const ESTIMATED_SHOW_LENGTH_HOURS = 4

export interface ShowStatusStripeInput {
  eventDate: string
  doorsAt?: string | null
  musicAt?: string | null
  isCancelled: boolean
  /** Venue-local timezone inputs, from {@link showStatusStripeZone}. */
  state?: string | null
  timezone?: string | null
  /** Server-computed. See the module comment. */
  lifecycle: ShowLifecycleState
}

/**
 * The timezone inputs for one show, resolved in ONE place.
 *
 * The server computes the lifecycle state and the client renders the copy; if
 * they disagreed about which zone the show is in, the band could say TONIGHT
 * above a date on the following day. Both call this.
 *
 * The venue's own `state` wins over the show's, because the venue is where the
 * show happens; a show row's `state` is denormalized and can lag an edit.
 */
export function showStatusStripeZone(show: ShowResponse): {
  state?: string | null
  timezone?: string | null
} {
  const venue = show.venues?.[0]
  return {
    state: venue?.state ?? show.state,
    timezone: venue?.timezone,
  }
}

/** Milliseconds since the epoch, or `null` when the string is not a date. */
function instantMs(value: string | null | undefined): number | null {
  const at = Date.parse(value ?? '')
  return Number.isFinite(at) ? at : null
}

function partsOf(
  instant: number,
  timeZone: string,
  options: Intl.DateTimeFormatOptions
): (type: string) => string {
  const parts = new Intl.DateTimeFormat('en-US', {
    ...options,
    timeZone,
  }).formatToParts(new Date(instant))
  return type => parts.find(p => p.type === type)?.value ?? ''
}

/**
 * "7PM", "8:30PM", "12AM": the stripe's clock register.
 *
 * Assembled from parts rather than by string-munging a formatted time because
 * `Intl` puts a narrow no-break space before the AM/PM in current ICU and a
 * plain space in older ones. That difference is invisible on screen and fatal
 * across hydration: server and client can ship different ICU builds.
 */
function formatStripeTime(instant: number, timeZone: string): string {
  const part = partsOf(instant, timeZone, {
    hour: 'numeric',
    minute: '2-digit',
    hour12: true,
  })
  const minute = part('minute')
  return `${part('hour')}${minute === '00' ? '' : `:${minute}`}${part(
    'dayPeriod'
  ).toUpperCase()}`
}

/** "SAT" */
function formatStripeWeekday(instant: number, timeZone: string): string {
  return partsOf(instant, timeZone, { weekday: 'short' })('weekday').toUpperCase()
}

/** "AUG 15" */
function formatStripeMonthDay(instant: number, timeZone: string): string {
  const part = partsOf(instant, timeZone, { month: 'short', day: 'numeric' })
  return `${part('month').toUpperCase()} ${part('day')}`
}

/** "14 JUL", the cancelled register, day before month, per the locked copy. */
function formatStripeDayMonth(instant: number, timeZone: string): string {
  const part = partsOf(instant, timeZone, { month: 'short', day: 'numeric' })
  return `${part('day')} ${part('month').toUpperCase()}`
}

/** "WED, AUG 12 2026" */
function formatStripeFullDate(instant: number, timeZone: string): string {
  const part = partsOf(instant, timeZone, {
    weekday: 'short',
    month: 'short',
    day: 'numeric',
    year: 'numeric',
  })
  return `${part('weekday').toUpperCase()}, ${part('month').toUpperCase()} ${part(
    'day'
  )} ${part('year')}`
}

/**
 * The stripe's segments, in order, to be joined by a middot.
 *
 * Returns `[]` when the show has no readable date, and the band renders
 * nothing at all rather than an empty stamp. Callers must not substitute a
 * default: inventing a date for a show whose date we cannot read is the one
 * outcome worse than saying nothing.
 *
 * The state order is a precedence, not a switch: a cancelled show is cancelled
 * whether it was tonight or last year, so that test comes first. Only states
 * the payload can actually support are here. `shows.status`
 * (pending/approved/rejected/private) is a MODERATION field, not a lifecycle
 * one, and postponed/moved have no column to read. Do not infer them.
 */
export function buildShowStatusStripeSegments(
  input: ShowStatusStripeInput
): string[] {
  const startedAt = instantMs(input.eventDate)
  if (startedAt === null) return []

  const timeZone = resolveShowTimezone(input.state, input.timezone)
  const doorsAt = instantMs(input.doorsAt)
  const musicAt = instantMs(input.musicAt)

  if (input.isCancelled) {
    return ['CANCELLED', formatStripeDayMonth(startedAt, timeZone)]
  }

  if (input.lifecycle === 'past') {
    // The mock's tail ("SETLIST + RECORDINGS BELOW") is deliberately absent:
    // those modules are not built, and a band that points at them would be
    // pointing at nothing. It belongs to the ticket that ships them.
    return ['PAST SHOW', formatStripeFullDate(startedAt, timeZone)]
  }

  const doors =
    doorsAt === null ? [] : [`DOORS ${formatStripeTime(doorsAt, timeZone)}`]

  if (input.lifecycle === 'today') {
    const music =
      musicAt === null ? [] : [`MUSIC ${formatStripeTime(musicAt, timeZone)}`]
    const ends =
      doorsAt === null
        ? []
        : // Added to the INSTANT, then formatted in the venue's zone, so a show
          // running across a DST jump ends at the wall-clock time the venue
          // will actually read.
          [
            `ENDS ~${formatStripeTime(
              doorsAt + ESTIMATED_SHOW_LENGTH_HOURS * 60 * 60 * 1000,
              timeZone
            )} (EST.)`,
          ]
    return ['TONIGHT', ...doors, ...music, ...ends]
  }

  // Plain upcoming. No countdown and no "UPCOMING" label: the weekday and date
  // in the same register as TONIGHT are the whole statement. Music time is
  // dropped here on purpose: the doors time is the one a reader plans around
  // days out, and the full call belongs to the day itself.
  return [
    formatStripeWeekday(startedAt, timeZone),
    formatStripeMonthDay(startedAt, timeZone),
    ...doors,
  ]
}
