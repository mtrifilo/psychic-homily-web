import { getTimezoneForState, formatInTimezone } from './timeUtils'

export interface DateBadgeParts {
  /** Short day of week, uppercase: "TUE" */
  dayOfWeek: string
  /** Short month + day: "MAR 17" */
  monthDay: string
}

/**
 * Format a show date into stacked badge parts for the card layout.
 * Returns { dayOfWeek: "TUE", monthDay: "MAR 17" } in the venue's timezone.
 */
export function formatShowDateBadge(
  dateString: string,
  state?: string | null
): DateBadgeParts {
  const timezone = getTimezoneForState(state || 'AZ')

  const dayOfWeek = formatInTimezone(dateString, timezone, {
    weekday: 'short',
  }).toUpperCase()

  const month = formatInTimezone(dateString, timezone, {
    month: 'short',
  }).toUpperCase()

  const day = formatInTimezone(dateString, timezone, {
    day: 'numeric',
  })

  return {
    dayOfWeek,
    monthDay: `${month} ${day}`,
  }
}
