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
