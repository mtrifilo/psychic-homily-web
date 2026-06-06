import {
  getTimezoneForState,
  formatDateInTimezone,
  formatDateWithYearInTimezone,
  formatTimeInTimezone,
} from './timeUtils'

/**
 * Resolve the IANA timezone for rendering a show time. Prefers the venue's
 * resolved `timezone` (PSY-985); falls back to the US state→tz map for venues
 * without one (pre-backfill rows). A malformed/unknown `timezone` string falls
 * through to the state map rather than crashing the render (`Intl` throws a
 * RangeError on a bad zone), mirroring the backend's EventLocation (PSY-996/986).
 */
export function resolveShowTimezone(
  state?: string | null,
  timezone?: string | null
): string {
  if (timezone && isValidTimeZone(timezone)) return timezone
  return getTimezoneForState(state || 'AZ')
}

function isValidTimeZone(tz: string): boolean {
  try {
    // Throws RangeError for an unknown/malformed IANA name.
    new Intl.DateTimeFormat('en-US', { timeZone: tz })
    return true
  } catch {
    return false
  }
}

/**
 * Format a show date in the venue's timezone: "Mon, Dec 1" or "Mon Dec 1, 2026".
 * Pass the venue's `timezone` when available; `state` is the fallback.
 */
export function formatShowDate(
  dateString: string,
  state?: string | null,
  includeYear = false,
  timezone?: string | null
): string {
  const tz = resolveShowTimezone(state, timezone)
  return includeYear
    ? formatDateWithYearInTimezone(dateString, tz)
    : formatDateInTimezone(dateString, tz)
}

/**
 * Format a show time in the venue's timezone: "7:30 PM".
 * Pass the venue's `timezone` when available; `state` is the fallback.
 */
export function formatShowTime(
  dateString: string,
  state?: string | null,
  timezone?: string | null
): string {
  return formatTimeInTimezone(dateString, resolveShowTimezone(state, timezone))
}

/**
 * Format price for display. Shows "Free" for $0, otherwise "$XX.XX".
 */
export function formatPrice(price: number): string {
  if (price === 0) return 'Free'
  return `$${price.toFixed(2)}`
}

/**
 * Format a content date: "January 15, 2026"
 */
export function formatContentDate(dateString: string): string {
  const date = new Date(dateString)
  return date.toLocaleDateString('en-US', {
    year: 'numeric',
    month: 'long',
    day: 'numeric',
  })
}

/**
 * Format an admin date with weekday: "Sat, Jan 15, 2026"
 */
export function formatAdminDate(dateString: string): string {
  const date = new Date(dateString)
  return date.toLocaleDateString('en-US', {
    weekday: 'short',
    month: 'short',
    day: 'numeric',
    year: 'numeric',
  })
}

/**
 * Format time in browser local timezone: "7:30 PM"
 */
export function formatAdminTime(dateString: string): string {
  const date = new Date(dateString)
  return date.toLocaleTimeString('en-US', {
    hour: 'numeric',
    minute: '2-digit',
    hour12: true,
  })
}

/**
 * Format a short date without weekday: "Jan 15, 2026"
 */
export function formatShortDate(dateString: string): string {
  const date = new Date(dateString)
  return date.toLocaleDateString('en-US', {
    month: 'short',
    day: 'numeric',
    year: 'numeric',
  })
}

/**
 * Format a timestamp with date and time: "Jan 15, 2026, 7:30 PM"
 */
export function formatTimestamp(dateString: string): string {
  const date = new Date(dateString)
  return date.toLocaleDateString('en-US', {
    month: 'short',
    day: 'numeric',
    year: 'numeric',
    hour: 'numeric',
    minute: '2-digit',
  })
}
