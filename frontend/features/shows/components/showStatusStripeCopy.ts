import { resolveShowTimezone } from '@/lib/utils/formatters'
import { formatShowDateBadge } from '@/lib/utils/showDateBadge'
import type { ShowLifecycleState } from '@/lib/utils/showTiming'

/**
 * Copy for the show page's status stripe: the one typographic band at the top
 * of the page that says where this show sits in time.
 *
 * Kept apart from the component because every hard part here is a string, not
 * a DOM tree: which segments appear, in which order, in whose timezone. The
 * component's job is one row of spans.
 *
 * Every segment is venue-local and derived from the show payload alone. The
 * only clock-dependent input is `lifecycle`, computed once on the server; see
 * `getShowLifecycleState` for why that boundary is where it is.
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
 *
 * KNOWN DIVERGENCE, do not silently "fix" it in either direction:
 * `ShowAddToCalendar`'s `SHOW_DURATION_MS` is THREE hours from the show's
 * START, mirroring the backend's `defaultShowDuration` so the two calendar
 * export paths agree. This is FOUR hours from DOORS, which is the number the
 * stripe copy was specified with. They can appear in one viewport and
 * disagree. Reconciling them is a product call about what a show's assumed
 * length is, not a refactor.
 */
export const ESTIMATED_SHOW_LENGTH_HOURS = 4

/**
 * Everything the band needs, flat.
 *
 * Deliberately not a `ShowResponse`: a flat input is the whole test surface,
 * and building a fifteen-field show fixture per case would bury what each case
 * is actually about. The zone fields come from `showTimingInput`, which is the
 * one place that decides which calendar a show is on.
 */
export interface ShowStatusStripeInput {
  /**
   * Nullable like the rest, because it arrives over the wire: a TYPE is not a
   * runtime guarantee, and the band's answer to an unreadable date is to say
   * nothing rather than to crash the page it sits on.
   */
  eventDate: string | null | undefined
  doorsAt?: string | null
  musicAt?: string | null
  isCancelled: boolean
  state?: string | null
  timezone?: string | null
  /** Server-computed. See the module comment. */
  lifecycle: ShowLifecycleState
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
  //
  // The weekday and date come from `formatShowDateBadge`, the same helper the
  // show CARD uses, so a listing row and the page it links to cannot render
  // the same date two ways.
  // Handed the instant this function already validated, not the raw field, so
  // the shared helper is never the one deciding what an unparseable date means.
  const { dayOfWeek, monthDay } = formatShowDateBadge(
    new Date(startedAt).toISOString(),
    input.state,
    input.timezone
  )
  return [dayOfWeek, monthDay, ...doors]
}
