import { formatInTimezone } from './timeUtils'
import { resolveShowTimezone } from './formatters'

export interface DateBadgeParts {
  /** Short day of week, uppercase: "TUE" */
  dayOfWeek: string
  /** Short month + day: "MAR 17" */
  monthDay: string
}

/**
 * Format a show date as a compact month/day label in the venue's timezone.
 * This is the single-line mobile form used where the weekday would wrap.
 */
export function formatShowMonthDay(
  dateString: string,
  state?: string | null,
  timezone?: string | null
): string {
  const tz = resolveShowTimezone(state, timezone)
  return formatInTimezone(dateString, tz, {
    month: 'short',
    day: 'numeric',
  }).toUpperCase()
}

/**
 * `SEP 04` — the same label as {@link formatShowMonthDay}, zero-padded.
 *
 * For fixed-width ledger columns (the show page's discovery rails), where the
 * day is the only variable-width part of the cell and an unpadded one
 * un-aligns every row beneath it.
 *
 * A formatter of its own rather than a `replace()` over its sibling's output:
 * patching a shared helper's return value couples a caller to a string shape
 * nothing enforces, and the sibling serves cards, badges and the status stripe
 * — any of which could change what it ends with.
 */
export function formatShowMonthDayPadded(
  dateString: string,
  state?: string | null,
  timezone?: string | null
): string {
  const tz = resolveShowTimezone(state, timezone)
  return formatInTimezone(dateString, tz, {
    month: 'short',
    day: '2-digit',
  }).toUpperCase()
}

/**
 * The calendar YEAR a show falls in, read on the venue's clock.
 *
 * Pairs with {@link formatShowMonthDayPadded}: a caller that decides whether to
 * print a year by comparing years must resolve BOTH in the same zone, or the
 * two disagree across the New Year boundary. `new Date(x).getFullYear()` reads
 * the RUNTIME's zone, so a Chicago show at 20:00 on Dec 31 is "2027" to a
 * reader in Berlin while its own label says `DEC 31` — the year suffix then
 * says the opposite of the date beside it, on one of the most heavily booked
 * nights of the year.
 */
export function showYearInZone(
  dateString: string,
  state?: string | null,
  timezone?: string | null
): number {
  const tz = resolveShowTimezone(state, timezone)
  return Number(formatInTimezone(dateString, tz, { year: 'numeric' }))
}

/**
 * Format a show date into stacked badge parts for the card layout.
 * Returns { dayOfWeek: "TUE", monthDay: "MAR 17" } in the venue's timezone.
 * Pass the venue's `timezone` when available; `state` is the fallback.
 */
export function formatShowDateBadge(
  dateString: string,
  state?: string | null,
  timezone?: string | null
): DateBadgeParts {
  const tz = resolveShowTimezone(state, timezone)

  const dayOfWeek = formatInTimezone(dateString, tz, {
    weekday: 'short',
  }).toUpperCase()

  return {
    dayOfWeek,
    monthDay: formatShowMonthDay(dateString, state, timezone),
  }
}
