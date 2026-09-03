import {
  isShowTimezoneResolved,
  resolveShowTimezone,
} from '@/lib/utils/formatters'
import { formatCompactTimeInTimezone } from '@/lib/utils/timeUtils'
import { markGuessedShowDay } from '../showPageDate'
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
 * rendered as an estimate ("ENDS ~11PM") and is deliberately shallow: it is
 * never stored, never emitted into JSON-LD, and never shown without the "~"
 * that marks it as a guess. The tilde is the whole signal: an earlier draft
 * spelled it "(EST.)" for ESTIMATED, which reads as Eastern Time next to a
 * clock (user decision).
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
 * The register itself lives in `formatCompactTimeInTimezone`, shared with the
 * show page's discovery rails so the two cannot print one fact two ways. It is
 * assembled from `formatToParts` rather than by string-munging a formatted
 * time because `Intl` puts a narrow no-break space before the AM/PM in current
 * ICU and a plain space in older ones — invisible on screen and fatal across
 * hydration, since server and client can ship different ICU builds.
 *
 * The instant is passed as milliseconds, which the shared helper accepts:
 * serializing it first would throw on a non-finite value before the helper's
 * own guard could return "".
 */
function formatStripeTime(instant: number, timeZone: string): string {
  return formatCompactTimeInTimezone(instant, timeZone)
}

/**
 * The venue module's announced-times fragment: "DOORS 7PM / MUSIC 8PM",
 * "DOORS 7PM", or null when nothing is printable.
 *
 * Lives here, beside the stripe's copy, because it is the SAME statement the
 * stripe makes a few hundred pixels up and must obey the same two rules for
 * the same reasons: it hangs off `doors_at` (a lone music time is half a
 * schedule — see the stripe's comment), and it prints nothing when the venue
 * timezone is a guess (a fallback-zone clock can be hours out; the one thing
 * worse than no time is a confident wrong one). Same clock register too, so
 * the page cannot print "7PM" in the stripe and "7:00" here for one fact.
 *
 * The separator is a slash, not the middot: in the venue facts line the
 * middot separates FACTS (capacity · age · times), and doors/music are two
 * halves of one fact.
 *
 * Yes, this means DOORS can print twice on one page — the stripe states it
 * as status, the venue module as a fact sheet. That repetition is the
 * locked mock's, which renders both, and it is why the two MUST share this
 * module: twice is the design, twice-in-two-registers would be the bug.
 */
export function doorsMusicFactSegment(
  input: Pick<ShowStatusStripeInput, 'doorsAt' | 'musicAt' | 'state' | 'timezone'>
): string | null {
  if (!isShowTimezoneResolved(input.state, input.timezone)) return null
  const timeZone = resolveShowTimezone(input.state, input.timezone)
  const doorsAt = instantMs(input.doorsAt)
  if (doorsAt === null) return null
  const doors = `DOORS ${formatStripeTime(doorsAt, timeZone)}`
  const musicAt = instantMs(input.musicAt)
  return musicAt === null
    ? doors
    : `${doors} / MUSIC ${formatStripeTime(musicAt, timeZone)}`
}

/**
 * The show's start time in the stripe's clock register ("8PM", "8:30PM"),
 * venue-local, or null when it cannot be said honestly.
 *
 * Null in two cases, both deliberate: an unreadable `event_date`, and an
 * unresolved venue timezone. The old header meta row printed this clock
 * through the Arizona fallback; a page that refuses to print DOORS on a
 * guessed zone (see {@link doorsMusicFactSegment}) cannot print the start
 * time on the same guess two lines down. One register, one refusal rule,
 * for every clock on the page.
 */
export function startTimeFactSegment(
  input: Pick<ShowStatusStripeInput, 'eventDate' | 'state' | 'timezone'>
): string | null {
  if (!isShowTimezoneResolved(input.state, input.timezone)) return null
  const timeZone = resolveShowTimezone(input.state, input.timezone)
  const startedAt = instantMs(input.eventDate)
  if (startedAt === null) return null
  return formatStripeTime(startedAt, timeZone)
}

/**
 * "WED, AUG 12 2026", or "~WED, AUG 12 2026" when the zone that decided the day
 * is the fallback.
 *
 * Takes the input rather than the resolved zone: by the time
 * `resolveShowTimezone` has returned, a guessed America/Phoenix and a Phoenix
 * venue's real one are the same string, so the marker cannot be derived from it.
 */
function formatStripeFullDate(
  instant: number,
  input: Pick<ShowStatusStripeInput, 'state' | 'timezone'>
): string {
  const part = partsOf(instant, resolveShowTimezone(input.state, input.timezone), {
    weekday: 'short',
    month: 'short',
    day: 'numeric',
    year: 'numeric',
  })
  return markGuessedShowDay(
    `${part('weekday').toUpperCase()}, ${part('month').toUpperCase()} ${part(
      'day'
    )} ${part('year')}`,
    input.state,
    input.timezone
  )
}

/**
 * The stripe's segments, in order, to be joined by a middot.
 *
 * Returns `[]` when the show has no readable date and is not cancelled, and
 * the band renders nothing at all rather than an empty stamp. Callers must not
 * substitute a default: inventing a date for a show whose date we cannot read
 * is the one outcome worse than saying nothing.
 *
 * The state order is a precedence, not a switch: a cancelled show is cancelled
 * whether it was tonight or last year, and whether or not its date parses, so
 * that test comes first. Only states the payload can actually support are
 * here. `shows.status`
 * (pending/approved/rejected/private) is a MODERATION field, not a lifecycle
 * one, and postponed/moved have no column to read. Do not infer them.
 */
export function buildShowStatusStripeSegments(
  input: ShowStatusStripeInput
): string[] {
  const startedAt = instantMs(input.eventDate)
  const timeZone = resolveShowTimezone(input.state, input.timezone)

  // Before the date guard, deliberately. A cancellation is the one thing on
  // this page a reader must not miss, and it is knowable without a readable
  // date; the band the cancellation replaced said so with no date at all.
  //
  // The date is in the same weekday/month/day register as every other state
  // (user decision): a band that reordered its date fields per state would
  // read as a different kind of statement rather than the same one saying
  // something else.
  if (input.isCancelled) {
    return startedAt === null
      ? ['CANCELLED']
      : ['CANCELLED', ...upcomingDateSegments(startedAt, input)]
  }

  if (startedAt === null) return []

  // A venue with no resolved `timezone` outside the US state map is judged on
  // Arizona's clock, which can be many hours and a calendar day out. This band
  // then says the two things that fallback is least able to support: an hour,
  // and the word TONIGHT. So when the zone is a guess, the band prints the date
  // and stops. The date can still be a day off at the edges, but it is one
  // claim instead of four, and it never puts DOORS 3AM on a 7 PM Tokyo show.
  //
  // This does NOT fix the fallback, which is tracked separately. It declines to
  // build on it.
  const zoneIsKnown = isShowTimezoneResolved(input.state, input.timezone)

  const doorsAt = zoneIsKnown ? instantMs(input.doorsAt) : null
  const musicAt = zoneIsKnown ? instantMs(input.musicAt) : null

  if (input.lifecycle === 'past') {
    // The mock's tail ("SETLIST + RECORDINGS BELOW") is deliberately absent:
    // those modules are not built, and a band that points at them would be
    // pointing at nothing. It belongs to the ticket that ships them.
    return ['PAST SHOW', formatStripeFullDate(startedAt, input)]
  }

  // The whole times line hangs off `doors_at`. A show with only a music time
  // announced degrades to the date alone rather than half a schedule: the
  // announced-times line is one statement, and doors is the part of it a
  // reader plans around. Surfacing a lone music time is a copy decision that
  // has not been made.
  if (doorsAt === null) {
    // TONIGHT is a same-day claim, so it needs the same-day clock to be real.
    return input.lifecycle === 'today' && zoneIsKnown
      ? ['TONIGHT']
      : upcomingDateSegments(startedAt, input)
  }

  const doors = `DOORS ${formatStripeTime(doorsAt, timeZone)}`

  if (input.lifecycle === 'today') {
    const music =
      musicAt === null ? [] : [`MUSIC ${formatStripeTime(musicAt, timeZone)}`]
    // Added to the INSTANT, then formatted in the venue's zone, so a show
    // running across a DST jump ends at the wall-clock time the venue will
    // actually read. Guarded because the sum, unlike every other instant here,
    // has not been through `instantMs`: a `doors_at` near the maximum time
    // value would overflow it and throw out of `Intl`.
    const endsAt = doorsAt + ESTIMATED_SHOW_LENGTH_HOURS * 60 * 60 * 1000
    const ends = Number.isFinite(new Date(endsAt).getTime())
      ? [`ENDS ~${formatStripeTime(endsAt, timeZone)}`]
      : []
    return ['TONIGHT', doors, ...music, ...ends]
  }

  // Plain upcoming. No countdown and no "UPCOMING" label: the weekday and date
  // in the same register as TONIGHT are the whole statement. The music time is
  // dropped days out on purpose; the full call belongs to the day itself.
  return [...upcomingDateSegments(startedAt, input), doors]
}

/**
 * "SAT", "AUG 15": the plain-upcoming date, and the fallback whenever a state
 * has nothing more to say.
 *
 * Both parts come from `formatShowDateBadge`, the helper the show CARD already
 * uses, so a listing row and the page it links to cannot render one date two
 * ways. It is handed the instant this module already validated rather than the
 * raw field, so the shared helper is never the one deciding what an
 * unparseable date means.
 *
 * A guessed day is marked ONCE, on the leading segment. These two segments are
 * one date printed in two parts, so `~SAT · AUG 15` says the date is a guess
 * while `~SAT · ~AUG 15` would read as two separate estimates.
 */
function upcomingDateSegments(
  startedAt: number,
  input: ShowStatusStripeInput
): string[] {
  const { dayOfWeek, monthDay } = formatShowDateBadge(
    new Date(startedAt).toISOString(),
    input.state,
    input.timezone
  )
  return [
    markGuessedShowDay(dayOfWeek, input.state, input.timezone),
    monthDay,
  ]
}
