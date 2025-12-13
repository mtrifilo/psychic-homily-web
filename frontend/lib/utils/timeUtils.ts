/**
 * Time utility functions for handling date/time conversions
 */

/**
 * Map of US states to their IANA timezones
 * Most shows are in Arizona which doesn't observe DST
 */
const STATE_TIMEZONES: Record<string, string> = {
  AZ: 'America/Phoenix',
  CA: 'America/Los_Angeles',
  NV: 'America/Los_Angeles',
  CO: 'America/Denver',
  NM: 'America/Denver',
  TX: 'America/Chicago',
  NY: 'America/New_York',
  // Add more as needed
}

/**
 * Get timezone for a US state (defaults to America/Phoenix for Arizona shows)
 */
export function getTimezoneForState(state: string): string {
  return STATE_TIMEZONES[state.toUpperCase()] || 'America/Phoenix'
}

/**
 * Combines a date string and time string into a UTC ISO timestamp
 * Treats the input as local time in the specified timezone
 *
 * @param dateString - Date in YYYY-MM-DD format (from date input)
 * @param timeString - Time in HH:MM format (from time input)
 * @param timezone - IANA timezone (e.g., 'America/Phoenix'). Defaults to browser timezone.
 * @returns ISO 8601 timestamp in UTC
 */
export function combineDateTimeToUTC(
  dateString: string,
  timeString: string,
  timezone?: string
): string {
  // Parse date and time parts manually to avoid JS Date timezone quirks
  const [year, month, day] = dateString.split('-').map(Number)
  const [hours, minutes] = timeString.split(':').map(Number)

  // Create date in local timezone (month is 0-indexed in JS Date)
  // This treats the input as the user's local time
  const date = new Date(year, month - 1, day, hours, minutes, 0, 0)

  // Convert to UTC and return as RFC3339 format (without milliseconds)
  // Go's time.Time parser expects this format
  return date.toISOString().replace(/\.\d{3}Z$/, 'Z')
}

/**
 * Format a UTC date string for display in a specific timezone
 *
 * @param utcDateString - ISO 8601 date string in UTC
 * @param timezone - IANA timezone (e.g., 'America/Phoenix')
 * @param options - Intl.DateTimeFormat options
 */
export function formatInTimezone(
  utcDateString: string,
  timezone: string,
  options: Intl.DateTimeFormatOptions
): string {
  const date = new Date(utcDateString)
  return date.toLocaleString('en-US', { ...options, timeZone: timezone })
}

/**
 * Format date as "Mon, Dec 1" in specified timezone
 */
export function formatDateInTimezone(
  utcDateString: string,
  timezone: string
): string {
  return formatInTimezone(utcDateString, timezone, {
    weekday: 'short',
    month: 'short',
    day: 'numeric',
  })
}

/**
 * Format time as "7:30 PM" in specified timezone
 */
export function formatTimeInTimezone(
  utcDateString: string,
  timezone: string
): string {
  return formatInTimezone(utcDateString, timezone, {
    hour: 'numeric',
    minute: '2-digit',
    hour12: true,
  })
}

/**
 * Parse an ISO date string into separate date and time strings for form inputs
 * Returns date in YYYY-MM-DD format and time in HH:MM format
 *
 * @param isoDateString - ISO 8601 date string
 * @returns Object with date and time strings
 */
export function parseISOToDateAndTime(isoDateString: string): {
  date: string
  time: string
} {
  const dateObj = new Date(isoDateString)

  // Format date as YYYY-MM-DD
  const year = dateObj.getFullYear()
  const month = String(dateObj.getMonth() + 1).padStart(2, '0')
  const day = String(dateObj.getDate()).padStart(2, '0')
  const date = `${year}-${month}-${day}`

  // Format time as HH:MM
  const hours = String(dateObj.getHours()).padStart(2, '0')
  const minutes = String(dateObj.getMinutes()).padStart(2, '0')
  const time = `${hours}:${minutes}`

  return { date, time }
}
