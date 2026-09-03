import {
  formatShowDate,
  isShowTimezoneResolved,
  resolveShowTimezone,
} from '@/lib/utils/formatters'
import { formatInTimezone } from '@/lib/utils/timeUtils'

/**
 * The show page's date renders, and the one place the guessed-day marking rule
 * is applied.
 *
 * A show whose venue has no resolved `timezone` and a state outside the US map
 * is dated on `FALLBACK_SHOW_TIMEZONE`. That day is still the best available
 * answer (see the constant in `lib/utils/timeUtils`), but the show page prints
 * it among facts that page has already qualified, so an unmarked one reads as a
 * checked one.
 *
 * WHICH RENDERS MARK is a decided list, not a property of the page: the header
 * date, both stripe registers, the meta description, and the share card. It
 * does NOT cover every date a reader can see on `/shows/{slug}` — the gig
 * timeline spine, the bill-recurrence line and the discovery rails print their
 * days through zones the payload already resolved, which is a shape this module
 * cannot reach. So a zone-less venue shows `~SEP 5` in the header above an
 * unmarked `SEP 5` on the spine.
 *
 * Sited here rather than in `lib/utils/formatters` so the list above has a home
 * a reader can find, and so a listing surface does not reach for a marking
 * formatter by accident.
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
 * ONE function for both so the two cannot answer differently, and so the mark
 * is not a step either call site can skip.
 *
 * Both of those renders travel: the description into a search snippet and a
 * chat unfurl, the card into the same. The mark goes with them, which is the
 * decided behaviour and not an oversight — a reader who cannot check the day
 * against the page is the reader most helped by being told it is a guess.
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
