import { formatInTimezone } from './timeUtils'
import { resolveShowTimezone } from './formatters'

export interface DateBadgeParts {
  /** Short day of week, uppercase: "TUE" */
  dayOfWeek: string
  /** Short month + day: "MAR 17" */
  monthDay: string
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

  const month = formatInTimezone(dateString, tz, {
    month: 'short',
  }).toUpperCase()

  const day = formatInTimezone(dateString, tz, {
    day: 'numeric',
  })

  return {
    dayOfWeek,
    monthDay: `${month} ${day}`,
  }
}
