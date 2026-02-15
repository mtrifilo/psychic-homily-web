import {
  getTimezoneForState,
  formatDateInTimezone,
  formatDateWithYearInTimezone,
  formatTimeInTimezone,
} from './timeUtils'

/**
 * Format a show date in venue timezone: "Mon, Dec 1" or "Mon Dec 1, 2026"
 */
export function formatShowDate(
  dateString: string,
  state?: string | null,
  includeYear = false
): string {
  const timezone = getTimezoneForState(state || 'AZ')
  return includeYear
    ? formatDateWithYearInTimezone(dateString, timezone)
    : formatDateInTimezone(dateString, timezone)
}

/**
 * Format a show time in venue timezone: "7:30 PM"
 */
export function formatShowTime(
  dateString: string,
  state?: string | null
): string {
  const timezone = getTimezoneForState(state || 'AZ')
  return formatTimeInTimezone(dateString, timezone)
}

/**
 * Format price as "$XX.XX"
 */
export function formatPrice(price: number): string {
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
