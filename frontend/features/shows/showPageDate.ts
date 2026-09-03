import {
  formatShowDate,
  isShowTimezoneResolved,
  resolveShowTimezone,
} from '@/lib/utils/formatters'
import { formatInTimezone } from '@/lib/utils/timeUtils'

/**
 * The show page's date renders, and the one place the guessed-day marking rule
 * lives.
 *
 * A show whose venue has no resolved `timezone` and a state outside the US map
 * is dated on `FALLBACK_SHOW_TIMEZONE`. That day is still the best available
 * answer (see the constant in `lib/utils/timeUtils`), but on this page it is
 * printed among facts the page has already qualified, so an unmarked one reads
 * as a checked one.
 *
 * Scoped to the show page rather than sited in `lib/utils/formatters` because
 * the marking is a decision about THIS surface: a listing row carries no other
 * estimate to read the mark against (user decision). A module boundary states
 * that, where a docstring on a shared formatter only asks readers to remember
 * it.
 */

/**
 * The site's mark for a number it is estimating rather than reporting. Already
 * the register of `ENDS ~11PM` in the status stripe and `CAP ~500` in the venue
 * facts, which is why the guessed calendar day borrows it instead of
 * introducing a second vocabulary for the same idea.
 */
export const GUESSED_VALUE_MARKER = '~'

/**
 * Prefix a rendered date with {@link GUESSED_VALUE_MARKER} when the zone that
 * decided which calendar day it is was the fallback rather than one the row
 * supplies. Returns the string untouched otherwise.
 *
 * Takes the FORMATTED string, not the instant, because the renders it serves
 * are in different registers ("Fri, Nov 13", "Friday, November 13, 2026",
 * "WED, AUG 12 2026", and the stripe's "SAT · AUG 15" pair) and only the
 * marking rule is common to them. One prefix per date, never one per segment:
 * the reader is being told one thing is a guess, and a date printed in parts is
 * still one date.
 *
 * Exported for the two stripe registers that assemble their own strings
 * (`showStatusStripeCopy`). A render that does NOT assemble its own string
 * should reach for {@link showPageDate} or {@link showPageDateLong} instead, so
 * the marking is not a step it can forget.
 */
export function markGuessedShowDay(
  formatted: string,
  state?: string | null,
  timezone?: string | null
): string {
  if (isShowTimezoneResolved(state, timezone)) return formatted
  return `${GUESSED_VALUE_MARKER}${formatted}`
}

/** The header's date line: `Fri, Nov 13`, or `~Fri, Nov 13` on a guess. */
export function showPageDate(
  dateString: string,
  state?: string | null,
  timezone?: string | null
): string {
  return markGuessedShowDay(
    formatShowDate(dateString, state, false, timezone),
    state,
    timezone
  )
}

/**
 * The long form the meta description and the share card print: `Friday,
 * November 13, 2026`, or `Fri, Nov 13, 2026` abbreviated, marked on a guess.
 *
 * Both of those used to hand-roll `toLocaleDateString` over
 * `resolveShowTimezone`. Two private copies of one policy is how a marker gets
 * applied to three date renders and missed by the fourth.
 *
 * `abbreviated` is a FIT decision the caller owns (the share card narrows its
 * date beside a flyer plate; `showOgLayout.test.ts` holds the widths), not a
 * second policy: both widths mark the same days.
 */
export function showPageDateLong(
  dateString: string,
  state?: string | null,
  timezone?: string | null,
  abbreviated = false
): string {
  const formatted = formatInTimezone(
    dateString,
    resolveShowTimezone(state, timezone),
    {
      weekday: abbreviated ? 'short' : 'long',
      year: 'numeric',
      month: abbreviated ? 'short' : 'long',
      day: 'numeric',
    }
  )
  return markGuessedShowDay(formatted, state, timezone)
}
